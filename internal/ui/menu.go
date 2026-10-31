package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mydisha/lega/internal/disk"
	"github.com/mydisha/lega/internal/format"
)

type menuItem struct {
	name string
	desc string
	open func(*shared) screen
}

// menu is the start screen when lega runs without a command.
type menu struct {
	sh    *shared
	items []menuItem
	l     list
	total int64
	free  int64
}

type usageMsg struct{ total, free int64 }

var keyQuit = key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "quit"))

func newMenu(sh *shared) screen {
	return &menu{sh: sh, items: []menuItem{
		{"Status", "Disks and what fills your home folder", newStatus},
		{"Clean", "Remove caches, logs and other junk", newCleaner},
		{"Analyze", "Explore your home folder by size", newAnalyze("")},
		{"Purge", "Remove build folders from your projects", newPurger(nil)},
	}}
}

func (m *menu) Init() tea.Cmd { return m.usage }

// usage reads the free space of the home disk. It runs again whenever the
// menu comes back into view, since cleaning changes it.
func (m *menu) usage() tea.Msg {
	total, _, free, err := disk.Usage(m.sh.env.Home)
	if err != nil {
		return nil
	}
	return usageMsg{total, free}
}

func (m *menu) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case usageMsg:
		m.total, m.free = msg.total, msg.free
	case tea.KeyPressMsg:
		if moveList(&m.l, msg, len(m.items), len(m.items)) {
			return m, nil
		}
		switch {
		case key.Matches(msg, keyQuit):
			return m, pop
		case key.Matches(msg, keyEnter, keyOpenDir):
			return m, tea.Batch(push(m.items[m.l.cursor].open(m.sh)), m.usage)
		}
		// Number keys jump straight to an item.
		if s := msg.String(); len(s) == 1 && s[0] >= '1' && int(s[0]-'1') < len(m.items) {
			m.l.cursor = int(s[0] - '1')
			return m, push(m.items[m.l.cursor].open(m.sh))
		}
	}
	return m, nil
}

func (m *menu) Keys() []key.Binding {
	return []key.Binding{keyUp, keyDown, keyEnter, keyQuit}
}

func (m *menu) View() string {
	t, w := m.sh.theme, m.sh.w()
	var b strings.Builder
	b.WriteString(t.Bold.Render("lega") + "  " + t.Dim.Render(m.sh.release.DisplayName()) + "\n")
	if m.total > 0 {
		fmt.Fprintf(&b, "%s free of %s in your home disk\n", t.Accent.Render(format.Size(m.free)), format.Size(m.total))
	}
	b.WriteString("\n")
	nameW := 10
	for i, it := range m.items {
		marker, name := "  ", fit(it.name, nameW)
		if i == m.l.cursor {
			marker, name = t.Accent.Render("▸ "), t.Accent.Render(name)
		}
		fmt.Fprintf(&b, "%s%d  %s%s\n", marker, i+1, name, t.Dim.Render(fit(it.desc, max(w-nameW-5, 0))))
	}
	return b.String()
}
