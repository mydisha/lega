package platform

import (
	"bufio"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// Env describes the current user and their standard directories.
type Env struct {
	Home      string
	User      string
	UID       int
	CacheDir  string // $XDG_CACHE_HOME, usually ~/.cache
	ConfigDir string // $XDG_CONFIG_HOME, usually ~/.config
	DataDir   string // $XDG_DATA_HOME, usually ~/.local/share
	StateDir  string // $XDG_STATE_HOME, usually ~/.local/state
}

// CurrentEnv builds an Env from the environment, following the XDG base
// directory spec.
func CurrentEnv() (Env, error) {
	home := os.Getenv("HOME")
	if home == "" {
		u, err := user.Current()
		if err != nil {
			return Env{}, err
		}
		home = u.HomeDir
	}
	if !filepath.IsAbs(home) {
		return Env{}, errors.New("home directory is not an absolute path")
	}
	home = filepath.Clean(home)
	name := os.Getenv("USER")
	if name == "" {
		if u, err := user.Current(); err == nil {
			name = u.Username
		}
	}
	return Env{
		Home:      home,
		User:      name,
		UID:       os.Getuid(),
		CacheDir:  xdgDir("XDG_CACHE_HOME", home, ".cache"),
		ConfigDir: xdgDir("XDG_CONFIG_HOME", home, ".config"),
		DataDir:   xdgDir("XDG_DATA_HOME", home, ".local/share"),
		StateDir:  xdgDir("XDG_STATE_HOME", home, ".local/state"),
	}, nil
}

// xdgDir returns the env var if it holds an absolute path, as the spec says
// relative values must be ignored.
func xdgDir(key, home, fallback string) string {
	if v := os.Getenv(key); filepath.IsAbs(v) {
		return filepath.Clean(v)
	}
	return filepath.Join(home, fallback)
}

// UserDirs returns the user's personal folders (Documents, Pictures, ...)
// from ~/.config/user-dirs.dirs. Folder names can be translated, so the file
// is the source of truth; English defaults fill the gaps.
func (e Env) UserDirs() map[string]string {
	dirs := map[string]string{
		"DESKTOP":     filepath.Join(e.Home, "Desktop"),
		"DOCUMENTS":   filepath.Join(e.Home, "Documents"),
		"DOWNLOAD":    filepath.Join(e.Home, "Downloads"),
		"MUSIC":       filepath.Join(e.Home, "Music"),
		"PICTURES":    filepath.Join(e.Home, "Pictures"),
		"PUBLICSHARE": filepath.Join(e.Home, "Public"),
		"TEMPLATES":   filepath.Join(e.Home, "Templates"),
		"VIDEOS":      filepath.Join(e.Home, "Videos"),
	}
	f, err := os.Open(filepath.Join(e.ConfigDir, "user-dirs.dirs"))
	if err != nil {
		return dirs
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, val, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || !strings.HasPrefix(key, "XDG_") || !strings.HasSuffix(key, "_DIR") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(key, "XDG_"), "_DIR")
		val = strings.Trim(val, `"`)
		val = strings.Replace(val, "$HOME", e.Home, 1)
		if !filepath.IsAbs(val) {
			continue
		}
		// A dir set to $HOME means "disabled" in xdg-user-dirs.
		if filepath.Clean(val) == e.Home {
			delete(dirs, name)
			continue
		}
		dirs[name] = filepath.Clean(val)
	}
	return dirs
}

// IsRoot reports whether lega runs with root privileges.
func IsRoot() bool { return os.Geteuid() == 0 }
