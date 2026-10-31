package safety

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fixture struct {
	home, cache, outside string
	g                    *Guard
}

func setup(t *testing.T) fixture {
	t.Helper()
	base := t.TempDir()
	f := fixture{
		home:    filepath.Join(base, "home"),
		outside: filepath.Join(base, "outside"),
	}
	f.cache = filepath.Join(f.home, ".cache")
	for _, d := range []string{f.cache, f.outside, filepath.Join(f.home, ".ssh"), filepath.Join(f.home, "Documents"), filepath.Join(f.home, ".local", "share")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.g = NewGuard(f.home, os.Getuid(), []string{filepath.Join(f.home, "Documents")})
	return f
}

func touch(t *testing.T, p string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func blocked(t *testing.T, err error) {
	t.Helper()
	var b *Blocked
	if !errors.As(err, &b) {
		t.Errorf("expected *Blocked, got %v", err)
	}
}

func TestCheckAllowsCacheContent(t *testing.T) {
	f := setup(t)
	p := filepath.Join(f.cache, "pip", "wheel.whl")
	touch(t, p)
	if err := f.g.Check(p, []string{f.cache}); err != nil {
		t.Errorf("cache file should be allowed: %v", err)
	}
	if err := f.g.Check(filepath.Join(f.cache, "pip"), []string{f.cache}); err != nil {
		t.Errorf("cache folder should be allowed: %v", err)
	}
}

func TestCheckBlocks(t *testing.T) {
	f := setup(t)
	touch(t, filepath.Join(f.home, ".ssh", "id_ed25519"))
	touch(t, filepath.Join(f.outside, "keep"))

	cases := map[string]struct {
		path  string
		roots []string
	}{
		"relative":             {"relative/path", []string{f.cache}},
		"unclean":              {f.cache + "/x/../../.ssh", []string{f.cache}},
		"the root itself":      {f.cache, []string{f.cache}},
		"outside roots":        {filepath.Join(f.outside, "keep"), []string{f.cache}},
		"home":                 {f.home, []string{filepath.Dir(f.home)}},
		"parent of protected":  {filepath.Join(f.home, ".local"), []string{f.home}},
		"user folder":          {filepath.Join(f.home, "Documents"), []string{f.home}},
		"inside secret folder": {filepath.Join(f.home, ".ssh", "id_ed25519"), []string{f.home}},
		"system root":          {"/", []string{"/"}},
		"system folder":        {"/usr", []string{"/"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			blocked(t, f.g.Check(c.path, c.roots))
		})
	}
}

func TestCheckMissingTarget(t *testing.T) {
	f := setup(t)
	if err := f.g.Check(filepath.Join(f.cache, "gone"), []string{f.cache}); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing target: %v", err)
	}
}

func TestSymlinkCannotEscape(t *testing.T) {
	f := setup(t)
	touch(t, filepath.Join(f.outside, "precious"))
	link := filepath.Join(f.cache, "link")
	if err := os.Symlink(f.outside, link); err != nil {
		t.Fatal(err)
	}

	// Going through the link leaves the root, so it is blocked.
	blocked(t, f.g.Check(filepath.Join(link, "precious"), []string{f.cache}))

	// The link itself may go, and removing it leaves the target alone.
	if err := f.g.Remove(link, []string{f.cache}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.outside, "precious")); err != nil {
		t.Error("symlink target was deleted")
	}
}

func TestSymlinkedRoot(t *testing.T) {
	// ~/.cache may itself be a link to another disk.
	f := setup(t)
	realCache := filepath.Join(f.outside, "cache")
	touch(t, filepath.Join(realCache, "npm", "x"))
	linked := filepath.Join(f.home, "linked-cache")
	os.Symlink(realCache, linked)
	if err := f.g.Check(filepath.Join(linked, "npm"), []string{linked}); err != nil {
		t.Errorf("target inside a symlinked root should be allowed: %v", err)
	}
}

func TestOwnerCheck(t *testing.T) {
	f := setup(t)
	p := filepath.Join(f.cache, "x")
	touch(t, p)
	f.g.UID = os.Getuid() + 1
	blocked(t, f.g.Check(p, []string{f.cache}))
	f.g.UID = -1
	if err := f.g.Check(p, []string{f.cache}); err != nil {
		t.Errorf("UID -1 skips the owner check: %v", err)
	}
}

func TestWhitelist(t *testing.T) {
	f := setup(t)
	keepDir := filepath.Join(f.cache, "keep-me")
	touch(t, filepath.Join(keepDir, "a"))
	touch(t, filepath.Join(f.cache, "thing.keep"))
	touch(t, filepath.Join(f.cache, "other"))
	f.g.Whitelist = []string{keepDir, filepath.Join(f.cache, "*.keep")}

	for _, p := range []string{keepDir, filepath.Join(keepDir, "a"), filepath.Join(f.cache, "thing.keep")} {
		if err := f.g.Check(p, []string{f.cache}); !errors.Is(err, ErrWhitelisted) {
			t.Errorf("%s should be whitelisted, got %v", p, err)
		}
	}
	if err := f.g.Remove(filepath.Join(f.cache, "other"), []string{f.cache}); err != nil {
		t.Errorf("non-whitelisted file: %v", err)
	}
	if _, err := os.Stat(keepDir); err != nil {
		t.Error("whitelisted folder was removed")
	}
}

func TestRemoveRefusesBlocked(t *testing.T) {
	f := setup(t)
	touch(t, filepath.Join(f.outside, "keep"))
	if err := f.g.Remove(filepath.Join(f.outside, "keep"), []string{f.cache}); err == nil {
		t.Fatal("expected refusal")
	}
	if _, err := os.Stat(filepath.Join(f.outside, "keep")); err != nil {
		t.Error("blocked file was deleted")
	}
}
