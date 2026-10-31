package purge

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mydisha/lega/internal/oplog"
	"github.com/mydisha/lega/internal/safety"
)

func write(t *testing.T, path string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, make([]byte, 2000), 0o644); err != nil {
		t.Fatal(err)
	}
}

func age(t *testing.T, dir string, d time.Duration) {
	t.Helper()
	when := time.Now().Add(-d)
	filepath.WalkDir(dir, func(p string, _ os.DirEntry, _ error) error {
		os.Chtimes(p, when, when)
		return nil
	})
}

func newFinder(home string) *Finder {
	return &Finder{
		Guard:   safety.NewGuard(home, os.Getuid(), nil),
		Now:     time.Now,
		Tracked: func(string) bool { return false },
	}
}

func rel(root string, as []Artifact) []string {
	var out []string
	for _, a := range as {
		r, _ := filepath.Rel(root, a.Path)
		out = append(out, r)
	}
	slices.Sort(out)
	return out
}

func TestFind(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "Projects")
	write(t, filepath.Join(root, "web", "package.json"))
	write(t, filepath.Join(root, "web", "node_modules", "react", "index.js"))
	write(t, filepath.Join(root, "web", "node_modules", "react", "node_modules", "x", "i.js"))
	write(t, filepath.Join(root, "web", ".next", "cache", "a"))
	write(t, filepath.Join(root, "loose", "node_modules", "x"))
	write(t, filepath.Join(root, "rs", "Cargo.toml"))
	write(t, filepath.Join(root, "rs", "target", "debug", "app"))
	write(t, filepath.Join(root, "py", ".venv", "pyvenv.cfg"))
	write(t, filepath.Join(root, "py", "venv", "not-a-venv.txt"))
	write(t, filepath.Join(root, "py", "pkg", "__pycache__", "m.pyc"))
	write(t, filepath.Join(root, "c", "build", "out.o"))
	write(t, filepath.Join(root, "web", ".git", "node_modules", "x"))

	got, err := newFinder(home).Find(context.Background(), []string{root})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"py/.venv", "py/pkg/__pycache__", "rs/target", "web/.next", "web/node_modules",
	}
	if r := rel(root, got); !slices.Equal(r, want) {
		t.Errorf("found %v, want %v", r, want)
	}
	for _, a := range got {
		if a.Bytes <= 0 || a.Root != root || !a.Recent {
			t.Errorf("artifact %+v: want size, root and recent", a)
		}
	}
	if got[0].Bytes < got[len(got)-1].Bytes {
		t.Error("results should be biggest first")
	}
}

func TestRecent(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "code")
	write(t, filepath.Join(root, "old", "package.json"))
	write(t, filepath.Join(root, "old", "node_modules", "a", "i.js"))
	age(t, filepath.Join(root, "old", "node_modules"), 30*24*time.Hour)

	got, _ := newFinder(home).Find(context.Background(), []string{root})
	if len(got) != 1 || got[0].Recent {
		t.Errorf("old artifact should not be recent: %+v", got)
	}
}

func TestSkipsTrackedAndWhitelisted(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "dev")
	write(t, filepath.Join(root, "a", "package.json"))
	write(t, filepath.Join(root, "a", "dist", "bundle.js"))
	write(t, filepath.Join(root, "b", "package.json"))
	write(t, filepath.Join(root, "b", "node_modules", "x"))

	f := newFinder(home)
	f.Tracked = func(p string) bool { return strings.HasSuffix(p, "dist") }
	f.Guard.Whitelist = []string{filepath.Join(root, "b")}
	if got, _ := f.Find(context.Background(), []string{root}); len(got) != 0 {
		t.Errorf("expected nothing, got %v", rel(root, got))
	}
}

func TestGitTracked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	write(t, filepath.Join(repo, "dist", "committed.js"))
	write(t, filepath.Join(repo, "node_modules", "x"))
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		cmd.Env = gitEnv() // never touch the repository running the tests
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("add", "dist")
	git("commit", "-q", "-m", "x")

	if !gitTracked(filepath.Join(repo, "dist")) {
		t.Error("dist is tracked")
	}
	if gitTracked(filepath.Join(repo, "node_modules")) {
		t.Error("node_modules is not tracked")
	}
	if gitTracked(filepath.Join(t.TempDir(), "node_modules")) {
		t.Error("outside a repo nothing is tracked")
	}

	// Run from a git hook or rebase, GIT_DIR points at another repository.
	// The answer must still come from the repository around the path.
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "elsewhere.git"))
	t.Setenv("GIT_WORK_TREE", t.TempDir())
	if !gitTracked(filepath.Join(repo, "dist")) || gitTracked(filepath.Join(repo, "node_modules")) {
		t.Error("GIT_DIR from the environment changed the answer")
	}
}

func TestRemove(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LEGA_NO_OPLOG", "")
	root := filepath.Join(home, "src")
	write(t, filepath.Join(root, "p", "package.json"))
	write(t, filepath.Join(root, "p", "node_modules", "x"))
	f := newFinder(home)
	got, _ := f.Find(context.Background(), []string{root})
	log := oplog.Open(filepath.Join(home, "state"))
	if err := Remove(f.Guard, log, got[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(got[0].Path); !os.IsNotExist(err) {
		t.Error("artifact still exists")
	}
	if _, err := os.Stat(filepath.Join(root, "p", "package.json")); err != nil {
		t.Error("project file was removed")
	}
	if b, _ := os.ReadFile(log.Path()); !strings.Contains(string(b), "\tpurge\tnode_modules\t") {
		t.Errorf("oplog: %q", b)
	}
	// Outside its root the guard refuses.
	bad := got[0]
	bad.Root = filepath.Join(home, "elsewhere")
	if err := Remove(f.Guard, log, bad); err == nil {
		t.Error("expected refusal outside root")
	}
}

func TestDefaultRoots(t *testing.T) {
	home := t.TempDir()
	os.Mkdir(filepath.Join(home, "Projects"), 0o755)
	os.Mkdir(filepath.Join(home, "www"), 0o755)
	write(t, filepath.Join(home, "code")) // a file, not a folder
	got := DefaultRoots(home)
	if !slices.Equal(got, []string{filepath.Join(home, "Projects"), filepath.Join(home, "www")}) {
		t.Errorf("got %v", got)
	}
}

func TestFindCancelled(t *testing.T) {
	home := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newFinder(home).Find(ctx, []string{home}); err == nil {
		t.Error("expected context error")
	}
}
