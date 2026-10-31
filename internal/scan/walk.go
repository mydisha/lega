package scan

import (
	"context"
	"io/fs"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

// walker measures folder trees in parallel. Each folder is opened once: its
// entries are listed with getdents into a reused buffer and measured with
// fstatat relative to the open folder, so the kernel does not resolve the
// full path again for every file, and no fs.DirEntry or fs.FileInfo is
// allocated per entry.
type walker struct {
	cross bool          // enter other mounted filesystems
	tree  bool          // keep a Node for every entry; Size only needs totals
	sem   chan struct{} // limits how many extra goroutines walk at once
	seen  sync.Map      // inodes with several links that were already counted

	files  atomic.Int64
	bytes  atomic.Int64
	errors atomic.Int64
}

// workers is how many folders are read at once. The work is mostly
// syscalls, so it goes a little past the number of usable CPUs.
func workers() int {
	return runtime.GOMAXPROCS(0) * 2
}

// sizeSem is shared by every Size call, so callers that measure many paths
// in parallel do not multiply the number of walking goroutines.
var sizeSem = make(chan struct{}, workers())

// direntBufs holds getdents buffers. 32 KiB lists several hundred entries
// per syscall.
var direntBufs = sync.Pool{New: func() any {
	b := make([]byte, 32<<10)
	return &b
}}

// subdir is a folder found by read that still has to be walked.
type subdir struct {
	node *Node
	dev  uint64
}

// walk measures the folder n at path, then its subfolders: in a new
// goroutine while the semaphore has room, otherwise in this one. n.Size
// must already hold the folder's own blocks.
func (w *walker) walk(ctx context.Context, n *Node, path string, dev uint64) {
	if ctx.Err() != nil {
		return
	}
	subs, err := w.read(n, path, dev)
	if err != nil {
		n.Err = err
		w.errors.Add(1)
	}

	var wg sync.WaitGroup
	for _, sd := range subs {
		p := join(path, sd.node.Name)
		select {
		case w.sem <- struct{}{}:
			wg.Go(func() {
				w.walk(ctx, sd.node, p, sd.dev)
				<-w.sem
			})
		default:
			w.walk(ctx, sd.node, p, sd.dev)
		}
	}
	wg.Wait()

	for _, sd := range subs {
		n.Size += sd.node.Size
		n.Files += sd.node.Files
	}
}

// read lists the folder at path. Files are counted into n straight away;
// subfolders on the same device (or any device when w.cross is set) are
// returned with their own blocks filled in. The error is set when the
// folder could not be opened or fully listed; what was read stays counted.
func (w *walker) read(n *Node, path string, dev uint64) ([]subdir, error) {
	fd, err := openDir(path)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	defer unix.Close(fd)

	bp := direntBufs.Get().(*[]byte)
	defer direntBufs.Put(bp)
	buf := *bp

	var (
		subs  []subdir
		names []string
		st    unix.Stat_t
	)
	for {
		nb, err := getdents(fd, buf)
		if err != nil {
			return subs, &fs.PathError{Op: "readdirent", Path: path, Err: err}
		}
		if nb <= 0 {
			return subs, nil
		}
		_, count, parsed := unix.ParseDirent(buf[:nb], -1, names[:0])
		names = parsed
		if w.tree {
			n.Children = slices.Grow(n.Children, count)
		}
		for _, name := range names {
			if fstatat(fd, name, &st) != nil {
				continue // removed while scanning
			}
			if st.Mode&unix.S_IFMT != unix.S_IFDIR {
				size := w.fileSize(&st)
				n.Size += size
				n.Files++
				if w.tree {
					n.Children = append(n.Children, &Node{Name: name, Size: size, Files: 1, Parent: n})
				}
				continue
			}
			if st.Dev != dev && !w.cross {
				continue // a mount point; it belongs to another filesystem
			}
			child := &Node{Name: name, IsDir: true, Size: blocks(&st)}
			if w.tree {
				child.Parent = n
				n.Children = append(n.Children, child)
			}
			subs = append(subs, subdir{child, st.Dev})
		}
	}
}

// fileSize counts the space of a file once, even when it has several hard
// links.
func (w *walker) fileSize(st *unix.Stat_t) int64 {
	w.files.Add(1)
	if st.Nlink > 1 {
		key := [2]uint64{st.Dev, st.Ino}
		if _, dup := w.seen.LoadOrStore(key, true); dup {
			return 0
		}
	}
	size := blocks(st)
	w.bytes.Add(size)
	return size
}

// blocks returns the allocated size. st_blocks is always in 512-byte units,
// whatever the filesystem block size is.
func blocks(st *unix.Stat_t) int64 {
	return st.Blocks * 512
}

func isDir(st *unix.Stat_t) bool {
	return st.Mode&unix.S_IFMT == unix.S_IFDIR
}

// join appends a base name to a folder path. Names never contain a slash,
// so this skips the cleaning filepath.Join does.
func join(dir, name string) string {
	if strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + "/" + name
}

// openDir opens a folder for listing. O_NOFOLLOW keeps a folder swapped for
// a symlink mid-scan from leading the walk elsewhere.
func openDir(path string) (int, error) {
	for {
		fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != unix.EINTR {
			return fd, err
		}
	}
}

func getdents(fd int, buf []byte) (int, error) {
	for {
		n, err := unix.Getdents(fd, buf)
		if err != unix.EINTR {
			return n, err
		}
	}
}

func fstatat(dirfd int, name string, st *unix.Stat_t) error {
	for {
		err := unix.Fstatat(dirfd, name, st, unix.AT_SYMLINK_NOFOLLOW)
		if err != unix.EINTR {
			return err
		}
	}
}
