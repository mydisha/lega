package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestPurgeFlow(t *testing.T) {
	sh := testShared(t, 80)
	proj := filepath.Join(sh.env.Home, "Projects")
	write(t, filepath.Join(proj, "old", "package.json"), 100)
	write(t, filepath.Join(proj, "old", "node_modules", "react", "i.js"), 300_000)
	write(t, filepath.Join(proj, "new", "Cargo.toml"), 100)
	write(t, filepath.Join(proj, "new", "target", "debug", "app"), 100_000)
	past := time.Now().Add(-40 * 24 * time.Hour)
	filepath.Walk(filepath.Join(proj, "old", "node_modules"), func(p string, _ os.FileInfo, _ error) error {
		return os.Chtimes(p, past, past)
	})

	p := newPurger(nil)(sh)
	p = settle(p, p.Init())
	view := ansi.Strip(p.View())
	for _, want := range []string{"In ~/Projects", "~/Projects/old/node_modules", "~/Projects/new/target", "used today"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	fitsWidth(t, p.View(), 80)

	pp := p.(*purger)
	if !pp.sel[0] || pp.sel[1] {
		t.Errorf("old artifact selected, recent one not: %v", pp.sel)
	}
	sendKeys(p, "enter")
	if !strings.Contains(ansi.Strip(p.View()), "Delete 1 build folders") {
		t.Fatalf("confirm:\n%s", ansi.Strip(p.View()))
	}
	_, cmd := p.Update(press("y"))
	p = settle(p, cmd)
	if !strings.Contains(ansi.Strip(p.View()), "Freed") {
		t.Errorf("summary:\n%s", ansi.Strip(p.View()))
	}
	if _, err := os.Stat(filepath.Join(proj, "old", "node_modules")); !os.IsNotExist(err) {
		t.Error("node_modules should be gone")
	}
	if _, err := os.Stat(filepath.Join(proj, "new", "target")); err != nil {
		t.Error("recent target folder should stay")
	}
}

func TestPurgeEmpty(t *testing.T) {
	sh := testShared(t, 60)
	p := newPurger(nil)(sh)
	p = settle(p, p.Init())
	if !strings.Contains(ansi.Strip(p.View()), "No project folders found") {
		t.Errorf("no roots:\n%s", ansi.Strip(p.View()))
	}

	os.Mkdir(filepath.Join(sh.env.Home, "code"), 0o755)
	p = newPurger(nil)(sh)
	p = settle(p, p.Init())
	if !strings.Contains(ansi.Strip(p.View()), "Nothing to purge") {
		t.Errorf("empty root:\n%s", ansi.Strip(p.View()))
	}
	fitsWidth(t, p.View(), 60)
}

func TestAgo(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	cases := map[time.Duration]string{0: "today", 30 * time.Hour: "yesterday", 5 * 24 * time.Hour: "5 days ago"}
	for d, want := range cases {
		if got := ago(now, now.Add(-d)); got != want {
			t.Errorf("ago(%v) = %q, want %q", d, got, want)
		}
	}
	if tilde("/home/u", "/home/u/x") != "~/x" || tilde("/home/u", "/etc") != "/etc" || tilde("/home/u", "/home/u") != "~" {
		t.Error("tilde")
	}
}
