package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/mydisha/lega/internal/platform"
)

var testSys = platform.SysInfo{
	Host: "thinkpad", Kernel: "7.0.0-42-generic", CPU: "AMD Ryzen 5 PRO 6650U", Threads: 12,
	MemTotal: 15_458_443_264, MemAvail: 7_335_436_288,
}

func TestBannerLayouts(t *testing.T) {
	th := NewTheme(true)
	rel := platform.OSRelease{PrettyName: "Ubuntu 24.04.1 LTS"}
	long := platform.SysInfo{
		Host:   "ノートパソコン-" + strings.Repeat("very-long-hostname-", 6),
		Kernel: strings.Repeat("7.0.0-42-generic-", 7), CPU: strings.Repeat("Ünïcode Prozessor ", 6), Threads: 256,
		MemTotal: 1 << 40, MemAvail: 1 << 39,
	}
	longRel := platform.OSRelease{PrettyName: strings.Repeat("Zorin OS 18.1 Pro Édition ", 5)}

	cases := []struct {
		name       string
		w, rows    int
		logo       bool // the big wordmark is drawn
		sideBySide bool
		info       bool // the system rows are drawn
	}{
		{"wide", 120, 20, true, true, true},
		{"80 columns", 80, 20, true, true, true},
		{"60 columns", 60, 20, true, false, true},
		{"narrower than the logo", 30, 20, false, false, true},
		{"short terminal", 80, 4, false, false, false},
		{"tiny", 4, 1, false, false, false},
	}
	for _, c := range cases {
		for _, si := range []struct {
			name string
			rel  platform.OSRelease
			sys  platform.SysInfo
		}{{"normal", rel, testSys}, {"long values", longRel, long}} {
			t.Run(c.name+"/"+si.name, func(t *testing.T) {
				view := banner(th, si.rel, si.sys, c.w, c.rows)
				fitsWidth(t, view, c.w)
				lines := strings.Split(view, "\n")
				if len(lines) > max(c.rows, 1) {
					t.Errorf("%d lines, want at most %d:\n%s", len(lines), c.rows, ansi.Strip(view))
				}
				plain := ansi.Strip(view)
				if got := strings.Contains(plain, "███████╗"); got != c.logo {
					t.Errorf("logo drawn = %v, want %v:\n%s", got, c.logo, plain)
				}
				if got := strings.Contains(plain, "memory"); got != c.info {
					t.Errorf("system rows drawn = %v, want %v:\n%s", got, c.info, plain)
				}
				if c.sideBySide && !strings.Contains(ansi.Strip(lines[0]), "host") {
					t.Errorf("summary should sit beside the logo:\n%s", plain)
				}
				if !strings.HasPrefix(plain, "lega") && !c.logo && c.w > len("lega") {
					t.Errorf("fallback should start with the name:\n%s", plain)
				}
			})
		}
	}
}

func TestBannerValues(t *testing.T) {
	rel := platform.OSRelease{PrettyName: "Ubuntu 24.04.1 LTS"}
	plain := ansi.Strip(banner(NewTheme(true), rel, testSys, 100, 20))
	for _, want := range []string{"host    thinkpad", "os      Ubuntu 24.04.1 LTS", "kernel  7.0.0-42-generic",
		"cpu     AMD Ryzen 5 PRO 6650U · 12 threads", "memory  8.1 GB used of 15.5 GB"} {
		if !strings.Contains(plain, want) {
			t.Errorf("banner lacks %q:\n%s", want, plain)
		}
	}
}

func TestSysRowsSkipsUnknown(t *testing.T) {
	rows := sysRows(platform.OSRelease{}, platform.SysInfo{Threads: 4, MemTotal: 8e9})
	got := map[string]string{}
	for _, r := range rows {
		got[r[0]] = r[1]
	}
	want := map[string]string{"os": "Linux", "cpu": "4 threads", "memory": "8.0 GB"}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

// The menu still fits a standard 80x24 terminal with the banner, and drops
// the logo instead of pushing the menu items off a short one.
func TestMenuBannerFits(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {60, 24}, {80, 12}} {
		sh := testShared(t, size[0])
		sh.height = size[1]
		m := newMenu(sh).(*menu)
		m.sys, m.total, m.free = testSys, 1e12, 4e11
		view := m.View()
		fitsWidth(t, view, size[0])
		if n := strings.Count(view, "\n") + 2; n > size[1] {
			t.Errorf("%dx%d: menu takes %d rows:\n%s", size[0], size[1], n, ansi.Strip(view))
		}
		if !strings.Contains(ansi.Strip(view), "4  Purge") {
			t.Errorf("%dx%d: menu items missing:\n%s", size[0], size[1], ansi.Strip(view))
		}
	}
}
