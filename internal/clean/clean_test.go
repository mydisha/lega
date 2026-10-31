package clean

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mydisha/lega/internal/oplog"
	"github.com/mydisha/lega/internal/platform"
	"github.com/mydisha/lega/internal/safety"
)

// testContext returns a Context for a fake home in a temp folder. Commands
// fail unless a test sets Run.
func testContext(t *testing.T) *Context {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	env := platform.Env{
		Home: home, User: "tester", UID: os.Getuid(),
		CacheDir:  filepath.Join(home, ".cache"),
		ConfigDir: filepath.Join(home, ".config"),
		DataDir:   filepath.Join(home, ".local/share"),
		StateDir:  filepath.Join(home, ".local/state"),
	}
	os.MkdirAll(env.CacheDir, 0o755)
	t.Setenv("LEGA_NO_OPLOG", "")
	// Keep rules that read env vars inside the fake home.
	t.Setenv("GOMODCACHE", filepath.Join(home, "go", "pkg", "mod"))
	userDirs := []string{filepath.Join(home, "Documents"), filepath.Join(home, "Downloads")}
	return &Context{
		Env:      env,
		Guard:    safety.NewGuard(home, env.UID, userDirs),
		SysGuard: safety.NewGuard(home, -1, userDirs),
		Log:      oplog.Open(env.StateDir),
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return nil, errors.New("no commands in tests")
		},
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
		Now:      time.Now,
		Running:  map[string]bool{},
	}
}

func write(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func findingFor(fs []Finding, id string) *Finding {
	for i := range fs {
		if fs[i].Rule.Info().ID == id {
			return &fs[i]
		}
	}
	return nil
}

func paths(ts []Target) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Path)
	}
	slices.Sort(out)
	return out
}

func TestThumbnailsScanAndClean(t *testing.T) {
	c := testContext(t)
	thumbs := filepath.Join(c.Env.CacheDir, "thumbnails")
	write(t, filepath.Join(thumbs, "normal", "a.png"), 8000)
	write(t, filepath.Join(thumbs, "large", "b.png"), 8000)
	keep := filepath.Join(c.Env.Home, "Documents", "photo.png")
	write(t, keep, 8000)

	found := Scan(context.Background(), c, userRules(), nil)
	f := findingFor(found, "thumbnails")
	if f == nil || len(f.Targets) != 2 || f.Bytes <= 0 {
		t.Fatalf("thumbnails finding: %+v", f)
	}
	if other := findingFor(found, "app-caches"); other != nil && slices.Contains(paths(other.Targets), thumbs) {
		t.Error("app-caches must not claim the thumbnails folder too")
	}

	rep := Clean(context.Background(), c, *f)
	if rep.Removed != 2 || len(rep.Failures) != 0 || rep.Freed != f.Bytes {
		t.Errorf("report %+v", rep)
	}
	if exists(filepath.Join(thumbs, "normal")) || !exists(thumbs) {
		t.Error("thumbnail contents should go, the folder should stay")
	}
	if !exists(keep) {
		t.Error("file outside the rule was removed")
	}
	log, err := os.ReadFile(c.Log.Path())
	if err != nil || strings.Count(string(log), "\tclean\tthumbnails\t") != 2 {
		t.Errorf("oplog: %q %v", log, err)
	}
}

func TestWhitelistedPathsAreNotFound(t *testing.T) {
	c := testContext(t)
	keep := filepath.Join(c.Env.CacheDir, "precious-cache")
	write(t, filepath.Join(keep, "x"), 100)
	write(t, filepath.Join(c.Env.CacheDir, "junk", "y"), 100)
	c.Guard.Whitelist = []string{keep}

	f := findingFor(Scan(context.Background(), c, userRules(), nil), "app-caches")
	if f == nil {
		t.Fatal("app-caches should find the junk folder")
	}
	if got := paths(f.Targets); len(got) != 1 || filepath.Base(got[0]) != "junk" {
		t.Errorf("targets %v", got)
	}
}

func TestLockedWhileAppRuns(t *testing.T) {
	c := testContext(t)
	r := &pathRule{
		info:  Info{ID: "x", Processes: []string{"firefox"}},
		spots: []spot{{"$CACHE/mozilla", []string{"*"}}},
	}
	write(t, filepath.Join(c.Env.CacheDir, "mozilla", "cache2", "a"), 100)
	c.Running["firefox"] = true

	found := Scan(context.Background(), c, []Rule{r}, nil)
	if len(found) != 1 || !strings.Contains(found[0].Locked, "firefox") {
		t.Fatalf("expected locked finding, got %+v", found)
	}
	rep := Clean(context.Background(), c, found[0])
	if rep.Removed != 0 || len(rep.Failures) != 1 {
		t.Errorf("locked rule must not clean: %+v", rep)
	}
	if !exists(filepath.Join(c.Env.CacheDir, "mozilla", "cache2")) {
		t.Error("files of a locked rule were removed")
	}
}

func TestOldInstallers(t *testing.T) {
	c := testContext(t)
	dl := filepath.Join(c.Env.Home, "Downloads")
	old := filepath.Join(dl, "app_1.0_amd64.deb")
	fresh := filepath.Join(dl, "new.iso")
	doc := filepath.Join(dl, "notes.pdf")
	for _, p := range []string{old, fresh, doc} {
		write(t, p, 5000)
	}
	os.MkdirAll(filepath.Join(dl, "folder.iso"), 0o755)
	past := time.Now().Add(-60 * 24 * time.Hour)
	os.Chtimes(old, past, past)
	os.Chtimes(filepath.Join(dl, "folder.iso"), past, past)

	f := findingFor(Scan(context.Background(), c, userRules(), nil), "old-installers")
	if f == nil || !slices.Equal(paths(f.Targets), []string{old}) {
		t.Fatalf("want only the old .deb, got %+v", f)
	}
	if f.Rule.Info().Risk != Review {
		t.Error("installers must need review")
	}
}

func TestTrashRule(t *testing.T) {
	c := testContext(t)
	trash := filepath.Join(c.Env.DataDir, "Trash")
	write(t, filepath.Join(trash, "files", "old.txt"), 3000)
	write(t, filepath.Join(trash, "info", "old.txt.trashinfo"), 50)

	f := findingFor(Scan(context.Background(), c, userRules(), nil), "trash")
	if f == nil || len(f.Targets) != 2 {
		t.Fatalf("trash finding %+v", f)
	}
	Clean(context.Background(), c, *f)
	if exists(filepath.Join(trash, "files", "old.txt")) || !exists(filepath.Join(trash, "files")) {
		t.Error("trash should be emptied but keep its folders")
	}
}

func TestDedupe(t *testing.T) {
	found := []Finding{
		{Targets: []Target{{Path: "/c/mozilla/firefox/p/cache2"}}},
		{Targets: []Target{{Path: "/c/mozilla"}, {Path: "/c/other"}, {Path: "/c/mozilla/firefox/p/cache2/x"}}},
		{Targets: []Target{{Path: "/c/other"}, {Path: "/c/otherness"}}},
	}
	dedupe(found)
	if got := paths(found[1].Targets); !slices.Equal(got, []string{"/c/other"}) {
		t.Errorf("second rule kept %v", got)
	}
	if got := paths(found[2].Targets); !slices.Equal(got, []string{"/c/otherness"}) {
		t.Errorf("third rule kept %v", got)
	}
}

func TestCommandRule(t *testing.T) {
	c := testContext(t)
	cache := filepath.Join(c.Env.CacheDir, "tool")
	write(t, filepath.Join(cache, "blob"), 20000)

	var ran []string
	c.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		ran = append([]string{name}, args...)
		os.RemoveAll(cache) // what the real tool would do
		return nil, nil
	}
	r := &commandRule{
		info:    Info{ID: "tool", Needs: []string{"tool"}},
		measure: []string{"$CACHE/tool"},
		cmd:     []string{"tool", "cache", "clean"},
	}

	if len(Available(c, []Rule{r})) != 0 {
		t.Error("rule must be hidden when its command is missing")
	}
	c.LookPath = func(string) (string, error) { return "/usr/bin/tool", nil }
	if len(Available(c, []Rule{r})) != 1 {
		t.Error("rule should be available once its command exists")
	}

	found := Scan(context.Background(), c, []Rule{r}, nil)
	if len(found) != 1 || found[0].Bytes <= 0 {
		t.Fatalf("finding %+v", found)
	}
	rep := Clean(context.Background(), c, found[0])
	if !slices.Equal(ran, []string{"tool", "cache", "clean"}) {
		t.Errorf("ran %v", ran)
	}
	if rep.Freed != found[0].Bytes || rep.Removed != 1 {
		t.Errorf("report %+v, want freed %d", rep, found[0].Bytes)
	}

	c.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, &CommandError{Cmd: name, Msg: "permission denied"}
	}
	write(t, filepath.Join(cache, "blob"), 20000)
	rep = Clean(context.Background(), c, Scan(context.Background(), c, []Rule{r}, nil)[0])
	if len(rep.Failures) != 1 || !strings.Contains(rep.Failures[0].Err, "permission denied") {
		t.Errorf("failure not reported: %+v", rep)
	}
}

func TestExpand(t *testing.T) {
	c := testContext(t)
	e := c.Env
	cases := map[string]string{
		"~":                      e.Home,
		"~/snap":                 e.Home + "/snap",
		"$CACHE/x":               e.CacheDir + "/x",
		"$CONFIG/Code":           e.ConfigDir + "/Code",
		"$DATA/Trash":            e.DataDir + "/Trash",
		"$DOWNLOAD":              e.Home + "/Downloads",
		"/media/$USER/*/.T-$UID": "/media/tester/*/.T-" + itoa(e.UID),
	}
	for in, want := range cases {
		if got := expand(c, in); got != want {
			t.Errorf("expand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRuleIDsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range Rules() {
		i := r.Info()
		if i.ID == "" || i.Name == "" || i.Group == "" || i.Description == "" {
			t.Errorf("rule %q is missing info", i.ID)
		}
		if seen[i.ID] {
			t.Errorf("duplicate rule ID %q", i.ID)
		}
		seen[i.ID] = true
		if r2, ok := Lookup(i.ID); !ok || r2.Info().ID != i.ID {
			t.Errorf("Lookup(%q) failed", i.ID)
		}
	}
	if _, ok := Lookup("nope"); ok {
		t.Error("Lookup of unknown ID should fail")
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

// userRules leaves out root rules, which read real system folders.
func userRules() []Rule {
	var out []Rule
	for _, r := range Rules() {
		if !r.Info().NeedsRoot {
			out = append(out, r)
		}
	}
	return out
}

func TestPickable(t *testing.T) {
	cases := map[string]bool{"thumbnails": true, "go-mod": false, "rotated-logs": false, "snap-revisions": false}
	for id, want := range cases {
		r, _ := Lookup(id)
		if got := (Finding{Rule: r}).Pickable(); got != want {
			t.Errorf("%s pickable = %v, want %v", id, got, want)
		}
	}
}
