package ui

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestMenu(t *testing.T) {
	sh := testShared(t, 60)
	m := newMenu(sh)
	m = settle(m, m.Init())
	view := ansi.Strip(m.View())
	for _, want := range []string{"Zorin OS 18.1", "free of", "1  Status", "2  Clean", "3  Analyze", "4  Purge"} {
		if !strings.Contains(view, want) {
			t.Errorf("menu lacks %q:\n%s", want, view)
		}
	}
	fitsWidth(t, m.View(), 60)

	sendKeys(m, "down", "down")
	_, cmd := m.Update(press("enter"))
	msgs := collect(cmd)
	if len(msgs) == 0 {
		t.Fatal("enter should open a screen")
	}
	if p, ok := msgs[0].(pushMsg); !ok {
		t.Errorf("want pushMsg, got %T", msgs[0])
	} else if _, ok := p.s.(*analyze); !ok {
		t.Errorf("third item should open the analyzer, got %T", p.s)
	}

	_, cmd = m.Update(press("2"))
	if p, ok := cmd().(pushMsg); !ok {
		t.Error("number key should open a screen")
	} else if _, ok := p.s.(*cleaner); !ok {
		t.Errorf("2 should open clean, got %T", p.s)
	}

	_, cmd = m.Update(press("q"))
	if cmd() != (popMsg{}) {
		t.Error("q should quit")
	}
}

func TestThemeFollowsBackground(t *testing.T) {
	sh := testShared(t, 80)
	a := newApp(sh, newMenu)
	a.Update(tea.BackgroundColorMsg{Color: color.White})
	if got := ansi.Strip(sh.theme.Accent.Render("x")); got != "x" {
		t.Fatalf("render changed text: %q", got)
	}
	r, g, b, _ := sh.theme.Accent.GetForeground().RGBA()
	if r>>8 != 0xa0 || g>>8 != 0x3f || b>>8 != 0x15 {
		t.Errorf("light background should use the darker accent, got %x %x %x", r>>8, g>>8, b>>8)
	}
	a.Update(tea.BackgroundColorMsg{Color: color.Black})
	r, g, b, _ = sh.theme.Accent.GetForeground().RGBA()
	if r>>8 != 0xf5 || g>>8 != 0xa1 || b>>8 != 0x73 {
		t.Errorf("dark background should use the lighter accent, got %x %x %x", r>>8, g>>8, b>>8)
	}
}
