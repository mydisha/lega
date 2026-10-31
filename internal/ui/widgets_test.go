package ui

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestBar(t *testing.T) {
	th := NewTheme(true)
	plain := lipgloss.NewStyle()
	cases := []struct {
		w    int
		frac float64
		full int
	}{{10, 0, 0}, {10, 0.5, 5}, {10, 1, 10}, {10, 2, 10}, {10, 0.01, 1}, {10, -1, 0}}
	for _, c := range cases {
		got := ansi.Strip(bar(th, c.w, c.frac, plain))
		if ansi.StringWidth(got) != c.w {
			t.Errorf("bar(%d, %v) width %d", c.w, c.frac, ansi.StringWidth(got))
		}
		if n := countRune(got, '█'); n != c.full {
			t.Errorf("bar(%d, %v) has %d filled, want %d", c.w, c.frac, n, c.full)
		}
	}
	if bar(th, 0, 0.5, plain) != "" {
		t.Error("zero width bar should be empty")
	}
}

func countRune(s string, r rune) int {
	n := 0
	for _, c := range s {
		if c == r {
			n++
		}
	}
	return n
}

func TestFit(t *testing.T) {
	cases := []struct {
		in   string
		w    int
		want string
	}{
		{"abc", 5, "abc  "},
		{"abcdef", 4, "abc…"},
		{"日本語", 4, "日… "},
		{"x", 0, ""},
	}
	for _, c := range cases {
		if got := fit(c.in, c.w); got != c.want {
			t.Errorf("fit(%q, %d) = %q, want %q", c.in, c.w, got, c.want)
		}
	}
}

func TestFitMiddle(t *testing.T) {
	got := fitMiddle("/home/user/Projects/web/node_modules", 20)
	if ansi.StringWidth(got) != 20 || got[:5] != "/home" || got[len(got)-7:] != "modules" {
		t.Errorf("fitMiddle = %q", got)
	}
	if fitMiddle("/short", 10) != "/short    " {
		t.Error("short paths are padded")
	}
}

func TestRight(t *testing.T) {
	if right("1.2 GB", 9) != "   1.2 GB" || right("toolongvalue", 3) != "toolongvalue" {
		t.Error("right alignment")
	}
}

func TestListWindow(t *testing.T) {
	var l list
	if s, e := l.window(0, 5); s != 0 || e != 0 {
		t.Errorf("empty list window %d-%d", s, e)
	}
	l.move(7, 20)
	if s, e := l.window(20, 5); s != 3 || e != 8 {
		t.Errorf("window after moving to 7: %d-%d", s, e)
	}
	l.move(-100, 20)
	if s, e := l.window(20, 5); l.cursor != 0 || s != 0 || e != 5 {
		t.Errorf("window at top: %d-%d cursor %d", s, e, l.cursor)
	}
	l.move(100, 20)
	if s, e := l.window(20, 5); l.cursor != 19 || s != 15 || e != 20 {
		t.Errorf("window at bottom: %d-%d", s, e)
	}
	// The list shrinks under the cursor, for example after a delete.
	if s, e := l.window(3, 5); l.cursor != 2 || s != 0 || e != 3 {
		t.Errorf("shrunk list: %d-%d cursor %d", s, e, l.cursor)
	}
}
