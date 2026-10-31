package scan

import (
	"context"
	"errors"
	"io/fs"

	"golang.org/x/sys/unix"
)

// Size returns the disk space used by path and everything below it,
// without keeping a tree in memory. A missing path has size 0.
// Unreadable folders below path are skipped, not reported as errors.
// Like a plain directory walk, it enters other mounted filesystems.
func Size(ctx context.Context, path string) (bytes, files int64, err error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		if err == unix.ENOENT {
			return 0, 0, nil
		}
		return 0, 0, &fs.PathError{Op: "lstat", Path: path, Err: err}
	}
	w := &walker{cross: true, sem: sizeSem}
	if !isDir(&st) {
		return w.fileSize(&st), 1, nil
	}
	n := &Node{Name: path, IsDir: true, Size: blocks(&st)}
	w.walk(ctx, n, path, st.Dev)
	if err := ctx.Err(); err != nil {
		return n.Size, n.Files, err
	}
	if n.Err != nil {
		if errors.Is(n.Err, fs.ErrNotExist) {
			return 0, 0, nil // removed while measuring
		}
		return n.Size, n.Files, n.Err
	}
	return n.Size, n.Files, nil
}
