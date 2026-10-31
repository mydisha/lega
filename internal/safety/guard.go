// Package safety decides which paths lega is allowed to delete. Every
// removal goes through a Guard, and the guard says no unless it is sure.
package safety

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
	"/", "/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/lib32", "/lib64",
	"/media", "/mnt", "/opt", "/proc", "/root", "/run", "/sbin", "/snap",
	"/srv", "/sys", "/tmp", "/usr", "/var", "/var/cache", "/var/lib",
	"/var/lib/apt", "/var/lib/dpkg", "/var/lib/snapd", "/var/log",
}

// Guard checks paths before they are removed.
type Guard struct {
	// UID is the owner every target must have. Use -1 to skip the check,
	// as the root helper does for system files.
	UID int
	// Protected paths and their parents can never be deleted.
	Protected []string
	// Secret folders: nothing at or below them can be deleted.
	Secret []string
	// Whitelist holds the user's keep rules: plain paths or glob patterns.
	Whitelist []string
}

// NewGuard returns a guard with the default protections for a user whose
// home is home. userDirs are the personal folders (Documents, Pictures...).
func NewGuard(home string, uid int, userDirs []string) *Guard {
	g := &Guard{UID: uid}
	g.Protected = append(g.Protected, systemPaths...)
	for _, rel := range []string{"", ".cache", ".config", ".local", ".local/share", ".local/state", "snap", ".var", ".var/app"} {
		g.Protected = append(g.Protected, filepath.Join(home, rel))
	}
	g.Protected = append(g.Protected, userDirs...)
	for _, rel := range []string{".ssh", ".gnupg", ".pki", ".password-store", ".local/share/keyrings", ".mozilla/firefox/profiles.ini"} {
		g.Secret = append(g.Secret, filepath.Join(home, rel))
	}
	return g
}

// Check returns nil when path may be deleted as part of a rule that works
// inside roots. Symlinks in the parent folders are resolved, so a link can
// not trick the guard into leaving the roots. The target itself is never
// followed: deleting a symlink removes the link only.
func (g *Guard) Check(path string, roots []string) error {
	block := func(reason string) error { return &Blocked{Path: path, Reason: reason} }

	if !filepath.IsAbs(path) {
		return block("not an absolute path")
	}
	if filepath.Clean(path) != path {
		return block("path is not clean")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return block("parent folder cannot be resolved")
	}
	real := filepath.Join(parent, filepath.Base(path))

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
		return block("outside the folders this rule may clean")
	}

	for _, p := range g.Protected {
		for _, cand := range []string{p, resolve(p)} {
			if real == cand || under(cand, real) {
				return block("protected folder")
			}
		}
	}
	for _, s := range g.Secret {
		for _, cand := range []string{s, resolve(s)} {
			if real == cand || under(real, cand) {
				return block("holds keys or passwords")
			}
		}
	}
	if g.whitelisted(path) || g.whitelisted(real) {
		return fmt.Errorf("%w: %s", ErrWhitelisted, path)
	}

	info, err := os.Lstat(real)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return block("cannot read file owner")
	}
	if g.UID >= 0 && int(st.Uid) != g.UID {
		return block("owned by another user")
	}
	if info.IsDir() {
		if pinfo, err := os.Lstat(parent); err == nil && pinfo.Sys().(*syscall.Stat_t).Dev != st.Dev {
			return block("is a mount point")
		}
	}
	return nil
}

// Remove checks path and deletes it, including everything inside.
func (g *Guard) Remove(path string, roots []string) error {
	if err := g.Check(path, roots); err != nil {
		return err
	}
	return os.RemoveAll(path)
}

func (g *Guard) whitelisted(path string) bool {
	for _, w := range g.Whitelist {
		if path == w || under(path, w) {
			return true
		}
		if ok, _ := filepath.Match(w, path); ok {
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
