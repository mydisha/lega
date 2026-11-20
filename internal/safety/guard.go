// Package safety decides which paths lega is allowed to delete. Every
// removal goes through a Guard, and the guard says no unless it is sure.
package safety

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

// Blocked explains why a path may not be deleted.
type Blocked struct {
	Path   string
	Reason string
}

func (b *Blocked) Error() string { return fmt.Sprintf("refusing to delete %s: %s", b.Path, b.Reason) }

// ErrWhitelisted marks paths the user asked lega to keep.
var ErrWhitelisted = errors.New("path is whitelisted")

// systemPaths may never be deleted, nor anything that contains them.
var systemPaths = []string{
	"/", "/bin", "/boot", "/boot/efi", "/dev", "/etc", "/etc/passwd", "/etc/shadow",
	"/etc/sudoers", "/home", "/lib", "/lib32", "/lib64", "/lost+found",
	"/media", "/mnt", "/opt", "/proc", "/root", "/run", "/run/user", "/sbin",
	"/snap", "/srv", "/sys", "/tmp", "/usr", "/var", "/var/cache", "/var/lib",
	"/var/lib/apt", "/var/lib/docker", "/var/lib/dpkg", "/var/lib/flatpak",
	"/var/lib/snapd", "/var/lib/systemd", "/var/log", "/var/spool",
}

// Guard checks paths before they are removed.
type Guard struct {
	// UID is the owner every target must have. Use -1 to skip the check,
	// as the root helper does for system files.
	UID int
	// Protected paths and their parents can never be deleted.
	Protected []string
	// Secret folders: nothing at, below, or containing them can be deleted.
	Secret []string
	// Whitelist holds the user's keep rules: plain paths or glob patterns.
	Whitelist []string
}

// NewGuard returns a guard with the default protections for a user whose
// home is home. userDirs are the personal folders (Documents, Pictures...).
func NewGuard(home string, uid int, userDirs []string) *Guard {
	g := &Guard{UID: uid}
	g.Protected = append(g.Protected, systemPaths...)
	for _, rel := range []string{
		"", ".cache", ".config", ".local", ".local/share", ".local/state",
		"snap", ".var", ".var/app",
	} {
		g.Protected = append(g.Protected, filepath.Join(home, rel))
	}
	g.Protected = append(g.Protected, userDirs...)
	for _, rel := range []string{
		".ssh", ".gnupg", ".pki", ".password-store",
		".aws", ".azure", ".kube", ".docker/config.json",
		".netrc", ".git-credentials", ".npmrc", ".pypirc",
		".config/gh", ".config/gcloud", ".config/sops", ".config/age",
		".config/1Password", ".config/Bitwarden", ".config/KeePassXC",
		".local/share/keyrings", ".local/share/kwalletd", ".local/share/gnupg",
		".mozilla/firefox/profiles.ini",
	} {
		g.Secret = append(g.Secret, filepath.Join(home, rel))
	}
	return g
}

// Check returns nil when path may be deleted as part of a rule that works
// inside roots. Symlinks in the parent folders are resolved, so a link can
// not trick the guard into leaving the roots. The target itself is never
// followed: deleting a symlink removes the link only.
func (g *Guard) Check(path string, roots []string) error {
	_, _, _, _, err := g.checkResolved(path, roots)
	return err
}

func (g *Guard) checkResolved(path string, roots []string) (parent, base, real string, st unix.Stat_t, err error) {
	block := func(reason string) error { return &Blocked{Path: path, Reason: reason} }

	if path == "" || hasControl(path) {
		return "", "", "", st, block("invalid path")
	}
	if !filepath.IsAbs(path) {
		return "", "", "", st, block("not an absolute path")
	}
	if filepath.Clean(path) != path {
		return "", "", "", st, block("path is not clean")
	}
	parent, err = filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", "", "", st, block("parent folder cannot be resolved")
	}
	base = filepath.Base(path)
	real = filepath.Join(parent, base)

	inRoot := false
	for _, r := range roots {
		rr, err := filepath.EvalSymlinks(r)
		if err != nil {
			continue
		}
		if under(real, rr) {
			inRoot = true
			break
		}
	}
	if !inRoot {
		return "", "", "", st, block("outside the folders this rule may clean")
	}

	if err := g.checkPolicy(path, real); err != nil {
		return "", "", "", st, err
	}

	if lerr := unix.Lstat(real, &st); lerr != nil {
		return "", "", "", st, &fs.PathError{Op: "lstat", Path: real, Err: lerr}
	}
	if g.UID >= 0 && int(st.Uid) != g.UID {
		return "", "", "", st, block("owned by another user")
	}
	if st.Mode&unix.S_IFMT == unix.S_IFDIR {
		var pst unix.Stat_t
		if unix.Lstat(parent, &pst) == nil && pst.Dev != st.Dev {
			return "", "", "", st, block("is a mount point")
		}
	}
	return parent, base, real, st, nil
}

// checkPolicy rejects paths that match protected folders, secrets or the whitelist.
func (g *Guard) checkPolicy(path, real string) error {
	block := func(reason string) error { return &Blocked{Path: path, Reason: reason} }
	for _, p := range g.Protected {
		for _, cand := range []string{p, resolve(p)} {
			if real == cand || under(cand, real) {
				return block("protected folder")
			}
		}
	}
	for _, s := range g.Secret {
		for _, cand := range []string{s, resolve(s)} {
			if real == cand || under(real, cand) || under(cand, real) {
				return block("holds keys or passwords")
			}
		}
	}
	if g.whitelisted(path) || g.whitelisted(real) {
		return fmt.Errorf("%w: %s", ErrWhitelisted, path)
	}
	return nil
}

// Remove checks path and deletes it, including everything inside. Removal
// uses directory-relative syscalls without following symlinks, verifies the
// target inode did not change after Check, and refuses to cross mount points
// or remove protected, secret, whitelisted or other-user nested entries.
func (g *Guard) Remove(path string, roots []string) error {
	parent, base, real, st, err := g.checkResolved(path, roots)
	if err != nil {
		return err
	}
	pfd, err := openDir(parent)
	if err != nil {
		return &fs.PathError{Op: "open", Path: parent, Err: err}
	}
	defer unix.Close(pfd)

	var cur unix.Stat_t
	if err := fstatat(pfd, base, &cur); err != nil {
		if err == unix.ENOENT {
			return nil
		}
		return &fs.PathError{Op: "fstatat", Path: real, Err: err}
	}
	if cur.Dev != st.Dev || cur.Ino != st.Ino {
		return &Blocked{Path: path, Reason: "file changed during check"}
	}
	if cur.Mode&unix.S_IFMT != unix.S_IFDIR {
		if err := unlinkat(pfd, base, 0); err != nil && err != unix.ENOENT {
			return &fs.PathError{Op: "unlink", Path: real, Err: err}
		}
		return nil
	}
	return g.removeDirAt(pfd, base, real, cur.Dev)
}

var direntBufs = sync.Pool{New: func() any {
	b := make([]byte, 32<<10)
	return &b
}}

func (g *Guard) removeDirAt(parentFD int, name, realPath string, startDev uint64) error {
	fd, err := openDirAt(parentFD, name)
	if err != nil {
		if err == unix.ENOENT {
			return nil
		}
		return &fs.PathError{Op: "open", Path: realPath, Err: err}
	}

	var st unix.Stat_t
	if err := fstat(fd, &st); err != nil {
		unix.Close(fd)
		return &fs.PathError{Op: "fstat", Path: realPath, Err: err}
	}
	if st.Dev != startDev {
		unix.Close(fd)
		return &Blocked{Path: realPath, Reason: "is a mount point"}
	}
	if g.UID >= 0 && int(st.Uid) != g.UID {
		unix.Close(fd)
		return &Blocked{Path: realPath, Reason: "owned by another user"}
	}

	names, readErr := readDirNames(fd, realPath)
	for _, child := range names {
		childPath := filepath.Join(realPath, child)
		var cst unix.Stat_t
		if err := fstatat(fd, child, &cst); err != nil {
			if err == unix.ENOENT {
				continue
			}
			unix.Close(fd)
			return &fs.PathError{Op: "fstatat", Path: childPath, Err: err}
		}
		if err := g.checkPolicy(childPath, childPath); err != nil {
			unix.Close(fd)
			return err
		}
		if g.UID >= 0 && int(cst.Uid) != g.UID {
			unix.Close(fd)
			return &Blocked{Path: childPath, Reason: "owned by another user"}
		}
		if cst.Mode&unix.S_IFMT == unix.S_IFDIR {
			if cst.Dev != startDev {
				unix.Close(fd)
				return &Blocked{Path: childPath, Reason: "is a mount point"}
			}
			if err := g.removeDirAt(fd, child, childPath, startDev); err != nil {
				unix.Close(fd)
				return err
			}
			continue
		}
		if err := unlinkat(fd, child, 0); err != nil && err != unix.ENOENT {
			unix.Close(fd)
			return &fs.PathError{Op: "unlink", Path: childPath, Err: err}
		}
	}
	unix.Close(fd)
	if readErr != nil {
		return readErr
	}
	if err := unlinkat(parentFD, name, unix.AT_REMOVEDIR); err != nil && err != unix.ENOENT {
		return &fs.PathError{Op: "rmdir", Path: realPath, Err: err}
	}
	return nil
}

func readDirNames(fd int, path string) ([]string, error) {
	bp := direntBufs.Get().(*[]byte)
	defer direntBufs.Put(bp)
	buf := *bp

	var names []string
	for {
		nb, err := getdents(fd, buf)
		if err != nil {
			return names, &fs.PathError{Op: "readdirent", Path: path, Err: err}
		}
		if nb <= 0 {
			return names, nil
		}
		_, _, names = unix.ParseDirent(buf[:nb], -1, names)
	}
}

func (g *Guard) whitelisted(path string) bool {
	for _, w := range g.Whitelist {
		for _, cand := range []string{w, resolve(w)} {
			if path == cand || under(path, cand) || under(cand, path) {
				return true
			}
			if ok, _ := filepath.Match(cand, path); ok {
				return true
			}
		}
	}
	return false
}

// hasControl reports whether s contains ASCII control characters (including NUL and newlines).
func hasControl(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return true
		}
	}
	return false
}

// under reports whether p is strictly inside dir.
func under(p, dir string) bool {
	if dir == "/" {
		return p != "/" && strings.HasPrefix(p, "/")
	}
	return strings.HasPrefix(p, dir+"/")
}

// resolve follows symlinks when it can and returns the path unchanged when
// it cannot (for example when the path does not exist).
func resolve(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

func openDir(path string) (int, error) {
	for {
		fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		if err != unix.EINTR {
			return fd, err
		}
	}
}

func openDirAt(dirfd int, name string) (int, error) {
	for {
		fd, err := unix.Openat(dirfd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != unix.EINTR {
			return fd, err
		}
	}
}

func fstat(fd int, st *unix.Stat_t) error {
	for {
		err := unix.Fstat(fd, st)
		if err != unix.EINTR {
			return err
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

func getdents(fd int, buf []byte) (int, error) {
	for {
		n, err := unix.Getdents(fd, buf)
		if err != unix.EINTR {
			return n, err
		}
	}
}

func unlinkat(dirfd int, name string, flags int) error {
	for {
		err := unix.Unlinkat(dirfd, name, flags)
		if err != unix.EINTR {
			return err
		}
	}
}
