// Package scan measures how much disk space files and folders use.
package scan

import (
	"context"
	"io/fs"
	"path/filepath"

	"golang.org/x/sys/unix"
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

	w walker
}

// Progress returns the files and bytes counted so far, and how many
// folders could not be read.
func (s *Scanner) Progress() (files, bytes, errors int64) {
	return s.w.files.Load(), s.w.bytes.Load(), s.w.errors.Load()
}

// Scan builds the tree under root. When ctx is cancelled it stops early and
// returns the partial tree together with ctx.Err().
func (s *Scanner) Scan(ctx context.Context, root string) (*Node, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var st unix.Stat_t
	if err := unix.Lstat(root, &st); err != nil {
		return nil, &fs.PathError{Op: "lstat", Path: root, Err: err}
	}
	s.w.cross, s.w.tree, s.w.sem = s.CrossDevices, true, make(chan struct{}, workers())
	n := &Node{Name: root, IsDir: isDir(&st)}
	if !n.IsDir {
		n.Size, n.Files = s.w.fileSize(&st), 1
		return n, nil
	}
	n.Size = blocks(&st)
	s.w.walk(ctx, n, root, st.Dev)
	if n.Err != nil && len(n.Children) == 0 {
		return nil, n.Err
	}
	return n, ctx.Err()
}
