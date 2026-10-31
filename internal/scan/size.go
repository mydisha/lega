package scan

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"syscall"
)

// Size returns the disk space used by path and everything below it,
// without keeping a tree in memory. A missing path has size 0.
// Unreadable folders are skipped, not reported as errors.
func Size(ctx context.Context, path string) (bytes, files int64, err error) {
	seen := map[[2]uint64]bool{}
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			if p == path {
				return werr
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		st := info.Sys().(*syscall.Stat_t)
		if !d.IsDir() {
			files++
			if st.Nlink > 1 {
				key := [2]uint64{st.Dev, st.Ino}
				if seen[key] {
					return nil
				}
				seen[key] = true
			}
		}
		bytes += blocks(st)
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return 0, 0, nil
	}
	return bytes, files, err
}
