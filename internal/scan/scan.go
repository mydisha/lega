// Package scan measures how much disk space files and folders use.
package scan

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
)

// Node is a file or folder in a scanned tree. Size is the space it takes on
// disk, including everything below it.
type Node struct {
	Name     string // base name; the root holds its full path
	Size     int64
	Files    int64 // number of files, counting this one if it is a file
	IsDir    bool
	Err      error // set when a folder could not be read
	Parent   *Node
	Children []*Node
}

// Path returns the absolute path of the node.
func (n *Node) Path() string {
	if n.Parent == nil {
		return n.Name
	}
	return filepath.Join(n.Parent.Path(), n.Name)
}

// Scanner walks a folder tree in parallel. Its counters can be read from
// another goroutine while a scan runs.
type Scanner struct {
	// CrossDevices lets the scan enter other mounted filesystems.
	CrossDevices bool

	files  atomic.Int64
	bytes  atomic.Int64
	errors atomic.Int64
	sem    chan struct{}
	seen   sync.Map // hardlinked inodes already counted
}

// Progress returns the files and bytes counted so far, and how many
// folders could not be read.
func (s *Scanner) Progress() (files, bytes, errors int64) {
	return s.files.Load(), s.bytes.Load(), s.errors.Load()
}

// Scan builds the tree under root. When ctx is cancelled it stops early and
// returns the partial tree together with ctx.Err().
func (s *Scanner) Scan(ctx context.Context, root string) (*Node, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, fs.ErrInvalid
	}
	s.sem = make(chan struct{}, runtime.NumCPU()*2)
	n := &Node{Name: root, IsDir: info.IsDir()}
	if !n.IsDir {
		n.Size, n.Files = s.fileSize(st), 1
		return n, nil
	}
	s.walk(ctx, n, root, st.Dev)
	if n.Err != nil && len(n.Children) == 0 {
		return nil, n.Err
	}
	return n, ctx.Err()
}

func (s *Scanner) walk(ctx context.Context, n *Node, dir string, dev uint64) {
	if ctx.Err() != nil {
		return
	}
	if st, err := os.Lstat(dir); err == nil {
		n.Size += blocks(st.Sys().(*syscall.Stat_t))
	}
	f, err := os.Open(dir)
	if err != nil {
		n.Err = err
		s.errors.Add(1)
		return
	}
	// Unsorted read: sorting happens later, only for what is shown.
	entries, err := f.ReadDir(-1)
	_ = f.Close()
	if err != nil {
		n.Err = err
		s.errors.Add(1)
	}

	var wg sync.WaitGroup
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue // removed while scanning
		}
		st := info.Sys().(*syscall.Stat_t)
		child := &Node{Name: e.Name(), Parent: n, IsDir: e.IsDir()}
		if !child.IsDir {
			child.Size, child.Files = s.fileSize(st), 1
			n.Children = append(n.Children, child)
			continue
		}
		if st.Dev != dev && !s.CrossDevices {
			continue // a mount point; it belongs to another filesystem
		}
		n.Children = append(n.Children, child)
		path := filepath.Join(dir, e.Name())
		select {
		case s.sem <- struct{}{}:
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.walk(ctx, child, path, st.Dev)
				<-s.sem
			}()
		default:
			s.walk(ctx, child, path, st.Dev)
		}
	}
	wg.Wait()

	for _, c := range n.Children {
		n.Size += c.Size
		n.Files += c.Files
	}
}

// fileSize counts the space of a file once, even when it has several hard
// links.
func (s *Scanner) fileSize(st *syscall.Stat_t) int64 {
	s.files.Add(1)
	if st.Nlink > 1 {
		key := [2]uint64{st.Dev, st.Ino}
		if _, dup := s.seen.LoadOrStore(key, true); dup {
			return 0
		}
	}
	size := blocks(st)
	s.bytes.Add(size)
	return size
}

// blocks returns the allocated size. st_blocks is always in 512-byte units,
// whatever the filesystem block size is.
func blocks(st *syscall.Stat_t) int64 {
	return st.Blocks * 512
}
