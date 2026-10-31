package ui

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mydisha/lega/internal/platform"
)

// screen is one view of the app: status, analyzer, cleaner, ...
type screen interface {
	Init() tea.Cmd
	Update(tea.Msg) (screen, tea.Cmd)
	View() string
	Keys() []key.Binding
}

// closer is implemented by screens that run background work to stop.
type closer interface{ Close() }

// shared is the state every screen can read.
type shared struct {
	theme   Theme
	width   int
	height  int
	env     platform.Env
	release platform.OSRelease
	ctx     context.Context
	// icons turns on Nerd Font icons next to names.
	icons bool
	// userDirs are the XDG user folders by key, read once for the icons.
	userDirs map[string]string
}

// pushMsg opens a screen on top of the current one.
type pushMsg struct{ s screen }

// popMsg closes the current screen.
type popMsg struct{}

func push(s screen) tea.Cmd { return func() tea.Msg { return pushMsg{s} } }

func pop() tea.Msg { return popMsg{} }

// app is the root Bubble Tea model. It keeps a stack of screens; closing
// the last one quits.
type app struct {
	sh    *shared
	stack []screen
	help  help.Model
}

func newApp(sh *shared, first func(*shared) screen) *app {
	return &app{sh: sh, stack: []screen{first(sh)}, help: help.New()}
}

func (a *app) top() screen { return a.stack[len(a.stack)-1] }

func (a *app) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, a.top().Init())
}

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.sh.width, a.sh.height = msg.Width, msg.Height
		a.help.SetWidth(msg.Width)
	case tea.BackgroundColorMsg:
		a.sh.theme = NewTheme(msg.IsDark())
		a.help.Styles = a.sh.theme.Help
		return a, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			a.closeAll()
			return a, tea.Quit
		}
	case pushMsg:
		a.stack = append(a.stack, msg.s)
		return a, msg.s.Init()
	case popMsg:
		if c, ok := a.top().(closer); ok {
			c.Close()
		}
		a.stack = a.stack[:len(a.stack)-1]
		if len(a.stack) == 0 {
			return a, tea.Quit
		}
		return a, nil
	}
	s, cmd := a.top().Update(msg)
	a.stack[len(a.stack)-1] = s
	return a, cmd
}

func (a *app) closeAll() {
	for _, s := range a.stack {
		if c, ok := s.(closer); ok {
			c.Close()
		}
	}
}

func (a *app) View() tea.View {
	if len(a.stack) == 0 {
		return tea.NewView("")
	}
	body := a.top().View()
	helpLine := a.help.ShortHelpView(a.top().Keys())
	// Keep the help line at the bottom of the screen.
	gap := a.sh.height - strings.Count(body, "\n") - 2
	if gap < 1 {
		gap = 1
	}
	v := tea.NewView(body + strings.Repeat("\n", gap) + helpLine)
	v.AltScreen = true
	v.WindowTitle = "lega"
	return v
}

// w returns the screen width, with a default until the terminal reports
// its size.
func (sh *shared) w() int {
	if sh.width <= 0 {
		return 80
	}
	return sh.width
}

// h returns the screen height, with a default like w.
func (sh *shared) h() int {
	if sh.height <= 0 {
		return 24
	}
	return sh.height
}

// Common key bindings.
var (
	keyUp    = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up"))
	keyDown  = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down"))
	keyPgUp  = key.NewBinding(key.WithKeys("pgup", "ctrl+u"))
	keyPgDn  = key.NewBinding(key.WithKeys("pgdown", "ctrl+d"))
	keyHome  = key.NewBinding(key.WithKeys("home", "g"))
	keyEnd   = key.NewBinding(key.WithKeys("end", "G"))
	keyBack  = key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "back"))
	keyEnter = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open"))
)

// moveList applies the shared navigation keys to l. It reports whether the
// key was one of them.
func moveList(l *list, msg tea.KeyPressMsg, n, page int) bool {
	switch {
	case key.Matches(msg, keyUp):
		l.move(-1, n)
	case key.Matches(msg, keyDown):
		l.move(1, n)
	case key.Matches(msg, keyPgUp):
		l.move(-page, n)
	case key.Matches(msg, keyPgDn):
		l.move(page, n)
	case key.Matches(msg, keyHome):
		l.cursor = 0
	case key.Matches(msg, keyEnd):
		l.cursor = max(n-1, 0)
	default:
		return false
	}
	return true
}
