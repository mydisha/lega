package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
)

// Keys shared by the clean and purge screens.
var (
	keyToggle  = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "select"))
	keyAll     = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "all/none"))
	keyExpand  = key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→", "details"))
	keyFold    = key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←", "fold"))
	keyProceed = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "continue"))
	keyConfirm = key.NewBinding(key.WithKeys("enter", "y"), key.WithHelp("y", "clean"))
	keyCancel  = key.NewBinding(key.WithKeys("n", "esc", "q"), key.WithHelp("n", "back"))
	keyDone    = key.NewBinding(key.WithKeys("enter", "q", "esc"), key.WithHelp("q", "close"))
)

// checkbox draws a selection box. Locked items get a dash.
func checkbox(t Theme, on, locked bool) string {
	switch {
	case locked:
		return t.Dim.Render("[-]")
	case on:
		return t.Accent.Render("[x]")
	}
	return "[ ]"
}

// tilde shortens paths inside home to ~/...
func tilde(home, p string) string {
	if p == home {
		return "~"
	}
	if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
		return "~/" + rel
	}
	return p
}

// ago describes how long ago t was, in days.
func ago(now, t time.Time) string {
	days := int(now.Sub(t).Hours() / 24)
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "yesterday"
	}
	return fmt.Sprintf("%d days ago", days)
}

// failureLines lists what went wrong, capped so the summary stays short.
func failureLines(t Theme, items []string, w int) string {
	const limit = 8
	var b strings.Builder
	for i, it := range items {
		if i == limit {
			b.WriteString("  " + t.Dim.Render(fmt.Sprintf("…and %d more", len(items)-limit)) + "\n")
			break
		}
		b.WriteString("  " + t.Danger.Render(fit(it, w-2)) + "\n")
	}
	return b.String()
}
