// Package trash moves files to the desktop Trash, following the
// freedesktop.org Trash specification, so they can be restored from the
// file manager.
package trash

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ErrNoTrash means no trash folder can take the file, for example on a
// read-only or foreign filesystem. Callers may offer to delete instead.
var ErrNoTrash = errors.New("no trash folder available for this file")

// Trash knows where the user's trash folders are.
type Trash struct {
	DataDir string // $XDG_DATA_HOME, the home trash lives in DataDir/Trash
	UID     int
}

// Move puts path into the trash folder of its filesystem and returns the
// trash folder used.
func (t Trash) Move(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	dev := info.Sys().(*syscall.Stat_t).Dev

	dir := filepath.Join(t.DataDir, "Trash")
	if err := ensure(dir); err == nil && device(dir) != dev {
		dir, err = t.topDirTrash(path, dev)
		if err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	name, err := reserve(dir, path)
	if err != nil {
		return "", err
	}
	if err := os.Rename(path, filepath.Join(dir, "files", name)); err != nil {
		_ = os.Remove(filepath.Join(dir, "info", name+".trashinfo"))
		if errors.Is(err, syscall.EXDEV) {
			return "", ErrNoTrash
		}
		return "", err
	}
	return dir, nil
}

// topDirTrash returns $topdir/.Trash-$uid for files on other disks.
// The shared $topdir/.Trash/$uid form is used when the admin set it up.
func (t Trash) topDirTrash(path string, dev uint64) (string, error) {
	top := mountTop(path, dev)
	uid := strconv.Itoa(t.UID)

	shared := filepath.Join(top, ".Trash")
	if info, err := os.Lstat(shared); err == nil && info.IsDir() && info.Mode()&os.ModeSticky != 0 {
		dir := filepath.Join(shared, uid)
		if ensure(dir) == nil {
			return dir, nil
		}
	}
	dir := filepath.Join(top, ".Trash-"+uid)
	if err := ensure(dir); err != nil {
		return "", ErrNoTrash
	}
	return dir, nil
}

// mountTop walks up from path until the filesystem changes.
func mountTop(path string, dev uint64) string {
	dir := filepath.Dir(path)
	for dir != "/" {
		parent := filepath.Dir(dir)
		if device(parent) != dev {
			return dir
		}
		dir = parent
	}
	return "/"
}

func device(path string) uint64 {
	info, err := os.Lstat(path)
	if err != nil {
		return 0
	}
	return info.Sys().(*syscall.Stat_t).Dev
}

// ensure creates a trash folder with its files/ and info/ parts. It refuses
// folders that are symlinks, as the spec requires.
func ensure(dir string) error {
	for _, d := range []string{dir, filepath.Join(dir, "files"), filepath.Join(dir, "info")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
		if info, err := os.Lstat(d); err != nil || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("trash folder %s is not usable", d)
		}
	}
	return nil
}

// reserve writes the .trashinfo file under a free name and returns that
// name. Creating the info file first with O_EXCL is how the spec avoids two
// programs picking the same name.
func reserve(dir, path string) (string, error) {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" { // dotfiles like ".bashrc"
		stem, ext = base, ""
	}
	body := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
		encode(path), time.Now().Format("2006-01-02T15:04:05"))

	for i := 1; i < 10000; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s.%d%s", stem, i, ext)
		}
		f, err := os.OpenFile(filepath.Join(dir, "info", name+".trashinfo"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, werr := f.WriteString(body)
		cerr := f.Close()
		if werr != nil || cerr != nil {
			_ = os.Remove(f.Name())
			return "", errors.Join(werr, cerr)
		}
		return name, nil
	}
	return "", errors.New("too many files with the same name in trash")
}

// encode percent-encodes a path the way file managers expect, keeping "/".
func encode(path string) string {
	return (&url.URL{Path: path}).EscapedPath()
}
