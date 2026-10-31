package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mydisha/lega/internal/clean"
	"github.com/mydisha/lega/internal/oplog"
	"github.com/mydisha/lega/internal/safety"
)

// settle runs cmd and every command that follows from it, like the Bubble
// Tea loop does, until nothing is left. Timers are skipped.
func settle(s screen, cmd tea.Cmd) screen {
	queue := collect(cmd)
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]
		var next tea.Cmd
		s, next = s.Update(msg)
		queue = append(queue, collect(next)...)
	}
	return s
}

func cleanContext(t *testing.T, sh *shared) *clean.Context {
	t.Helper()
	t.Setenv("LEGA_NO_OPLOG", "")
	t.Setenv("GOMODCACHE", filepath.Join(sh.env.Home, "go", "pkg", "mod"))
	g := safety.NewGuard(sh.env.Home, sh.env.UID, nil)
	return &clean.Context{
		Env: sh.env, Guard: g, SysGuard: g,
		Log: oplog.Open(sh.env.StateDir),
		Run: func(context.Context, string, ...string) ([]byte, error) {
			return nil, errors.New("no commands in tests")
		},
		LookPath: func(string) (string, error) { return "", errors.New("missing") },
		Now:      time.Now,
		Running:  map[string]bool{},
	}
}

// fakeRoot stands in for a system rule; it never runs.
type fakeRoot struct{}

func (fakeRoot) Info() clean.Info {
	return clean.Info{ID: "fake-root", Name: "Fake system cache", Group: clean.GroupRoot, NeedsRoot: true, Description: "Test."}
}
func (fakeRoot) Scan(context.Context, *clean.Context) ([]clean.Target, error) {
	return []clean.Target{{Path: "/var/fake", Bytes: 1_000_000}}, nil
}
func (fakeRoot) Clean(context.Context, *clean.Context, []clean.Target) []clean.Removal { return nil }

func userRules(c *clean.Context) []clean.Rule {
	var out []clean.Rule
	for _, r := range clean.Available(c, clean.Rules()) {
		if !r.Info().NeedsRoot {
			out = append(out, r)
		}
	}
	return out
}

func junkHome(t *testing.T, sh *shared) {
	t.Helper()
	write(t, filepath.Join(sh.env.CacheDir, "thumbnails", "normal", "a.png"), 300_000)
	write(t, filepath.Join(sh.env.CacheDir, "pip", "http", "x"), 200_000)
	write(t, filepath.Join(sh.env.CacheDir, "some-app", "blob"), 100_000)
	write(t, filepath.Join(sh.env.CacheDir, "mozilla", "firefox", "p.default", "cache2", "e"), 50_000)
}

func TestCleanFlow(t *testing.T) {
	sh := testShared(t, 80)
	junkHome(t, sh)
	c := cleanContext(t, sh)
	m := newCleanerWith(sh, c, userRules(c))
	if !strings.Contains(ansi.Strip(m.View()), "Looking for junk") {
		t.Error("scanning state missing")
	}
	m = settle(m, m.Init()).(*cleaner)

	view := ansi.Strip(m.View())
	for _, want := range []string{"System", "Thumbnail cache", "pip cache", "Other app caches", "review", "Firefox cache", "Selected:"} {
		if !strings.Contains(view, want) {
			t.Errorf("pick view lacks %q:\n%s", want, view)
		}
	}
	fitsWidth(t, m.View(), 80)
	if m.rows[m.l.cursor].f < 0 {
		t.Error("cursor must not rest on a group title")
	}

	// Safe rules start selected, review rules do not.
	for i, f := range m.found {
		want := f.Rule.Info().Risk == clean.Safe
		if anyTrue(m.sel[i]) != want {
			t.Errorf("%s selected = %v, want %v", f.Rule.Info().ID, anyTrue(m.sel[i]), want)
		}
	}

	sendKeys(m, "enter")
	if m.phase != phaseConfirm || !strings.Contains(ansi.Strip(m.View()), "Remove 3 items") {
		t.Fatalf("confirm view:\n%s", ansi.Strip(m.View()))
	}
	sendKeys(m, "n")
	if m.phase != phasePick {
		t.Fatal("n should go back to the list")
	}

	sendKeys(m, "enter")
	_, cmd := m.Update(press("y"))
	m = settle(m, cmd).(*cleaner)
	if m.phase != phaseDone {
		t.Fatalf("phase %v after cleaning", m.phase)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Freed") {
		t.Errorf("summary missing:\n%s", ansi.Strip(m.View()))
	}
	if _, err := os.Stat(filepath.Join(sh.env.CacheDir, "pip", "http")); !os.IsNotExist(err) {
		t.Error("pip cache should be cleaned")
	}
	if _, err := os.Stat(filepath.Join(sh.env.CacheDir, "some-app", "blob")); err != nil {
		t.Error("unselected review item was cleaned")
	}
}

func TestCleanPickTargets(t *testing.T) {
	sh := testShared(t, 60)
	junkHome(t, sh)
	write(t, filepath.Join(sh.env.CacheDir, "other-app", "blob"), 100_000)
	c := cleanContext(t, sh)
	m := newCleanerWith(sh, c, userRules(c))
	m = settle(m, m.Init()).(*cleaner)

	// Move to "Other app caches", open it and pick one folder.
	for m.rows[m.l.cursor].f < 0 || m.found[m.rows[m.l.cursor].f].Rule.Info().ID != "app-caches" {
		sendKeys(m, "down")
	}
	f := m.rows[m.l.cursor].f
	sendKeys(m, "right")
	if !strings.Contains(ansi.Strip(m.View()), "~/.cache/other-app") {
		t.Fatalf("expanded targets missing:\n%s", ansi.Strip(m.View()))
	}
	fitsWidth(t, m.View(), 60)
	sendKeys(m, "down", "space")
	if n, _ := countSel(m.sel[f], m.found[f].Targets); n != 1 {
		t.Errorf("one target should be selected, got %d", n)
	}
	sendKeys(m, "left")
	if m.rows[m.l.cursor].target != -1 {
		t.Error("fold should return to the rule line")
	}

	sendKeys(m, "a")
	if n, _ := m.selection(); n != 0 {
		t.Error("a with a selection should clear it")
	}
	sendKeys(m, "a")
	if n, _ := m.selection(); n == 0 {
		t.Error("a should select everything")
	}
}

func TestCleanLockedAndRoot(t *testing.T) {
	sh := testShared(t, 80)
	junkHome(t, sh)
	c := cleanContext(t, sh)
	c.Running["firefox"] = true
	m := newCleanerWith(sh, c, append(userRules(c), fakeRoot{}))
	m = settle(m, m.Init()).(*cleaner)

	for i, f := range m.found {
		switch f.Rule.Info().ID {
		case "firefox":
			if anyTrue(m.sel[i]) || f.Locked == "" {
				t.Error("firefox should be locked and unselected")
			}
			m.toggle(row{f: i, target: -1})
			if anyTrue(m.sel[i]) {
				t.Error("locked rule must not be selectable")
			}
		case "fake-root":
			if !anyTrue(m.sel[i]) {
				t.Error("safe root rule should start selected")
			}
		}
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "locked") || !strings.Contains(view, "System (needs sudo)") {
		t.Errorf("view:\n%s", view)
	}

	sendKeys(m, "enter")
	if !strings.Contains(ansi.Strip(m.View()), "sudo password") {
		t.Error("confirm should warn about sudo")
	}
	_, cmd := m.Update(press("y"))
	m = settle(m, cmd).(*cleaner)
	if m.phase != phaseWork || m.current != "waiting for sudo" {
		t.Fatalf("should wait for sudo, phase %v current %q", m.phase, m.current)
	}
	// The reason sudo gave is shown, not a generic message.
	m.Update(sudoMsg{err: errors.New("sudo: 3 incorrect password attempts")})
	if m.phase != phaseDone || !strings.Contains(ansi.Strip(m.View()), "System items skipped: sudo: 3 incorrect password attempts") {
		t.Errorf("sudo failure not reported:\n%s", ansi.Strip(m.View()))
	}
}

func TestCleanRootAsksAgainWhenSudoKeepsNoLogin(t *testing.T) {
	sh := testShared(t, 80)
	c := cleanContext(t, sh)
	m := newCleanerWith(sh, c, []clean.Rule{fakeRoot{}})
	m = settle(m, m.Init()).(*cleaner)
	m.phase = phaseWork

	_, cmd := m.Update(rootDoneMsg{err: clean.ErrSudoNotCached})
	if cmd == nil || m.phase != phaseWork || m.current != "waiting for sudo" {
		t.Fatalf("should hand the terminal to sudo again: cmd %v, phase %v, current %q", cmd != nil, m.phase, m.current)
	}
	if len(m.failures) != 0 {
		t.Errorf("not a failure yet: %v", m.failures)
	}

	m.Update(rootDoneMsg{err: errors.New("sudo did not get a password")})
	if m.phase != phaseDone || !strings.Contains(ansi.Strip(m.View()), "System items failed: sudo did not get a password") {
		t.Errorf("second sudo failure not reported:\n%s", ansi.Strip(m.View()))
	}
}

func TestCleanNothingFound(t *testing.T) {
	sh := testShared(t, 80)
	c := cleanContext(t, sh)
	m := newCleanerWith(sh, c, userRules(c))
	m = settle(m, m.Init()).(*cleaner)
	if !strings.Contains(ansi.Strip(m.View()), "Nothing to clean") {
		t.Errorf("empty state:\n%s", ansi.Strip(m.View()))
	}
	sendKeys(m, "space", "enter", "right")
	if m.phase != phasePick {
		t.Error("keys on an empty list should do nothing")
	}
}

func TestCleanBadWhitelist(t *testing.T) {
	sh := testShared(t, 80)
	wl := clean.WhitelistPath(sh.env)
	write(t, wl, 10)
	os.WriteFile(wl, []byte("relative/path\n"), 0o644)
	m := newCleaner(sh)
	if m.Init() != nil {
		t.Error("no scan should start with a broken whitelist")
	}
	if !strings.Contains(ansi.Strip(m.View()), "Fix or remove") {
		t.Errorf("view:\n%s", ansi.Strip(m.View()))
	}
}

// On a wide terminal the size must sit next to the name it belongs to, not
// at the far edge of the screen.
func TestCleanSizeNextToName(t *testing.T) {
	sh := testShared(t, 200)
	junkHome(t, sh)
	c := cleanContext(t, sh)
	m := newCleanerWith(sh, c, userRules(c))
	m = settle(m, m.Init()).(*cleaner)

	for _, name := range []string{"Thumbnail cache", "pip cache", "Other app caches"} {
		line := lineWith(t, ansi.Strip(m.View()), name)
		size := strings.Fields(line[:strings.Index(line, name)])
		if len(size) < 2 || !strings.HasSuffix(size[len(size)-1], "B") {
			t.Errorf("%s: size should come right before the name: %q", name, line)
		}
		if n := len(strings.TrimRight(line, " ")); n > 60 {
			t.Errorf("%s: row is %d cells wide on a 200-cell screen, size drifted away: %q", name, n, line)
		}
	}
}

// Long and unicode paths in an expanded rule are cut, never pushing the
// size off the row.
func TestCleanLongTargets(t *testing.T) {
	for _, w := range []int{60, 80} {
		sh := testShared(t, w)
		long := filepath.Join(sh.env.CacheDir, strings.Repeat("very-long-folder-name-", 6)+"日本語のキャッシュ")
		write(t, filepath.Join(long, "blob"), 100_000)
		c := cleanContext(t, sh)
		m := newCleanerWith(sh, c, userRules(c))
		m = settle(m, m.Init()).(*cleaner)
		for m.rows[m.l.cursor].f < 0 || m.found[m.rows[m.l.cursor].f].Rule.Info().ID != "app-caches" {
			sendKeys(m, "down")
		}
		sendKeys(m, "right")
		fitsWidth(t, m.View(), w)
		line := lineWith(t, ansi.Strip(m.View()), "…")
		if !strings.Contains(line, " kB  ") || !strings.Contains(line, "キャッシュ") {
			t.Errorf("width %d: target row should keep its size and the folder name: %q", w, line)
		}
	}
}

func lineWith(t *testing.T, view, s string) string {
	t.Helper()
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	t.Fatalf("no line with %q:\n%s", s, view)
	return ""
}
