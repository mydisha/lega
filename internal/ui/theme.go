// Package ui is lega's terminal interface, built with Bubble Tea.
package ui

import (
	"image/color"

	"charm.land/bubbles/v2/help"
	"charm.land/lipgloss/v2"
)

// Theme holds the few styles lega uses. Colors stay neutral (the
// terminal's own foreground and faint text) with a single orange accent for
// the numbers that matter. Red and yellow only mean danger and warning.
type Theme struct {
	Accent  lipgloss.Style // free space, selected totals, the cursor
	Bold    lipgloss.Style
	Dim     lipgloss.Style
	Danger  lipgloss.Style
	Warn    lipgloss.Style
	BarFill color.Color // filled part of a highlighted bar
	Help    help.Styles
}

// NewTheme returns the theme for a dark or light terminal background.
func NewTheme(isDark bool) Theme {
	ld := lipgloss.LightDark(isDark)
	return Theme{
		Accent:  lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#a03f15"), lipgloss.Color("#f5a173"))),
		Bold:    lipgloss.NewStyle().Bold(true),
		Dim:     lipgloss.NewStyle().Faint(true),
		Danger:  lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#b42318"), lipgloss.Color("#f97066"))),
		Warn:    lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#8a5a00"), lipgloss.Color("#fdb022"))),
		BarFill: lipgloss.Color("#ec6a2c"),
		Help:    help.DefaultStyles(isDark),
	}
}
