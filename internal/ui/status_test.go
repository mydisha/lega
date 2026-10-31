package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mydisha/lega/internal/disk"
)

func TestStatusView(t *testing.T) {
	for _, w := range []int{120, 80, 60} {
		sh := testShared(t, w)
		write(t, filepath.Join(sh.env.Home, "Downloads", "big.iso"), 400_000)
		write(t, filepath.Join(sh.env.Home, ".cache", "pip", "x"), 50_000)

		s := newStatus(sh).(*status)
		if !strings.Contains(ansi.Strip(s.View()), "Reading disks") {
			t.Error("loading state missing")
		}
		s.Update(mountsMsg{mounts: []disk.Mount{
			{MountPoint: "/", FSType: "ext4", Total: 500e9, Used: 470e9, Free: 30e9},
			{MountPoint: "/media/tester/A very long backup drive name", FSType: "exfat", Total: 1e12, Used: 1e11, Free: 9e11},
		}})
		s = run(s, s.scanHome()).(*status)

		view := ansi.Strip(s.View())
		for _, want := range []string{"Zorin OS 18.1", "30.0 GB free of 500.0 GB", "94%", "Downloads", ".cache"} {
			if !strings.Contains(view, want) {
				t.Errorf("width %d: view lacks %q:\n%s", w, want, view)
			}
		}
		if strings.Index(view, "Downloads") > strings.Index(view, ".cache") {
			t.Error("biggest home folder should come first")
		}
		fitsWidth(t, s.View(), w)
	}
}

func TestStatusEmptyAndErrors(t *testing.T) {
	sh := testShared(t, 80)
	s := newStatus(sh).(*status)
	s.Update(mountsMsg{mounts: []disk.Mount{}})
	s = run(s, s.scanHome()).(*status)
	view := ansi.Strip(s.View())
	if !strings.Contains(view, "No disks found") || !strings.Contains(view, "home folder is empty") {
		t.Errorf("empty states missing:\n%s", view)
	}

	s.Update(mountsMsg{err: errTest})
	s.Update(homeMsg{err: errTest})
	view = ansi.Strip(s.View())
	if !strings.Contains(view, "Cannot read disks") || !strings.Contains(view, "Cannot scan") {
		t.Errorf("error states missing:\n%s", view)
	}
}

func TestStatusBack(t *testing.T) {
	s := newStatus(testShared(t, 80))
	_, cmd := s.Update(press("q"))
	if cmd == nil || cmd() != (popMsg{}) {
		t.Error("q should close the screen")
	}
}

func TestAppStack(t *testing.T) {
	sh := testShared(t, 80)
	a := newApp(sh, newStatus)
	a.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	if sh.width != 70 || sh.height != 20 {
		t.Error("window size not stored")
	}
	a.Update(pushMsg{newStatus(sh)})
	if len(a.stack) != 2 {
		t.Fatal("push failed")
	}
	a.Update(popMsg{})
	_, cmd := a.Update(popMsg{})
	if len(a.stack) != 0 || cmd == nil {
		t.Error("closing the last screen should quit")
	}
	if v := a.View(); v.Content != "" {
		t.Error("empty app should render nothing")
	}

	a = newApp(sh, newStatus)
	_, cmd = a.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Error("ctrl+c should quit")
	}
	v := a.View()
	if !v.AltScreen || !strings.Contains(ansi.Strip(v.Content), "back") {
		t.Error("view should use the alt screen and show help")
	}
}

func TestStatusLongHomePath(t *testing.T) {
	sh := testShared(t, 60)
	sh.env.Home = filepath.Join(sh.env.Home, strings.Repeat("deep-folder/", 6))
	write(t, filepath.Join(sh.env.Home, "f"), 1000)
	s := newStatus(sh).(*status)
	s.Update(mountsMsg{mounts: nil})
	s = run(s, s.scanHome()).(*status)
	view := s.View()
	fitsWidth(t, view, 60)
	if !strings.Contains(ansi.Strip(view), "in 1 files") {
		t.Errorf("summary should stay visible:\n%s", ansi.Strip(view))
	}
}
