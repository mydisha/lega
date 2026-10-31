package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mydisha/lega/internal/platform"
)

// testShared returns shared state for a fake home of the given width.
func testShared(t *testing.T, width int) *shared {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LEGA_NO_OPLOG", "1")
	return &shared{
		theme: NewTheme(true), width: width, height: 30, ctx: context.Background(),
		env: platform.Env{
			Home: home, User: "tester", UID: os.Getuid(),
			CacheDir: filepath.Join(home, ".cache"), ConfigDir: filepath.Join(home, ".config"),
			DataDir: filepath.Join(home, ".local/share"), StateDir: filepath.Join(home, ".local/state"),
		},
		release: platform.OSRelease{PrettyName: "Zorin OS 18.1"},
	}
}

func press(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	}
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

// fitsWidth fails the test when any line of view is wider than w.
func fitsWidth(t *testing.T, view string, w int) {
	t.Helper()
	for i, line := range strings.Split(view, "\n") {
		if got := ansi.StringWidth(line); got > w {
			t.Errorf("line %d is %d cells wide, limit %d: %q", i, got, w, ansi.Strip(line))
		}
	}
}

func write(t *testing.T, path string, size int) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

// run executes a command and feeds the message back, as Bubble Tea does.
// Batches are flattened; ticks are skipped so tests stay fast.
func run(s screen, cmd tea.Cmd) screen {
	for _, msg := range collect(cmd) {
		s, _ = s.Update(msg)
	}
	return s
}

func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	switch m := msg.(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range m {
			out = append(out, collect(c)...)
		}
		return out
	case tickMsg, spinner.TickMsg, nil:
		return nil
	}
	return []tea.Msg{msg}
}

var errTest = errorString("boom")

type errorString string

func (e errorString) Error() string { return string(e) }
