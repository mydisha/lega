package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/mydisha/lega/internal/format"
	"github.com/mydisha/lega/internal/platform"
)

// logo is the lega wordmark on the start screen, in the figlet
// "ANSI Shadow" style.
var logo = []string{
	"██╗     ███████╗ ██████╗  █████╗ ",
	"██║     ██╔════╝██╔════╝ ██╔══██╗",
	"██║     █████╗  ██║  ███╗███████║",
	"██║     ██╔══╝  ██║   ██║██╔══██║",
	"███████╗███████╗╚██████╔╝██║  ██║",
	"╚══════╝╚══════╝ ╚═════╝ ╚═╝  ╚═╝",
}

const (
	logoGap      = 4  // cells between the logo and the system summary
	infoLabelW   = 8  // "memory" plus two spaces
	minInfoValue = 16 // narrowest value worth showing beside the logo
)

// sysRows returns the label and value of each known system fact.
func sysRows(rel platform.OSRelease, si platform.SysInfo) [][2]string {
	var rows [][2]string
	add := func(label, value string) {
		if value != "" {
			rows = append(rows, [2]string{label, value})
		}
	}
	add("host", si.Host)
	add("os", rel.DisplayName())
	add("kernel", si.Kernel)
	cpu := si.CPU
	if si.Threads > 0 {
		cpu = strings.TrimPrefix(fmt.Sprintf("%s · %d threads", cpu, si.Threads), " · ")
	}
	add("cpu", cpu)
	switch {
	case si.MemTotal > 0 && si.MemAvail > 0:
		add("memory", fmt.Sprintf("%s used of %s", format.Size(si.MemTotal-si.MemAvail), format.Size(si.MemTotal)))
	case si.MemTotal > 0:
		add("memory", format.Size(si.MemTotal))
	}
	return rows
}

// banner draws the logo with the system summary beside it, in at most
// maxRows rows of w cells. It falls back to the summary under the logo,
// then to the summary alone, then to one title line, as space runs out.
func banner(t Theme, rel platform.OSRelease, si platform.SysInfo, w, maxRows int) string {
	rows := sysRows(rel, si)
	logoW := ansi.StringWidth(logo[0])
	info := func(valueW int) []string {
		lines := make([]string, len(rows))
		for i, r := range rows {
			lines[i] = t.Dim.Render(fit(r[0], infoLabelW)) + strings.TrimRight(fit(r[1], valueW), " ")
		}
		return lines
	}
	var lines []string
	switch {
	case w >= logoW+logoGap+infoLabelW+minInfoValue && maxRows >= max(len(logo), len(rows)):
		side := info(w - logoW - logoGap - infoLabelW)
		for i := range max(len(logo), len(side)) {
			line := strings.Repeat(" ", logoW)
			if i < len(logo) {
				line = t.Accent.Render(logo[i])
			}
			if i < len(side) {
				line += strings.Repeat(" ", logoGap) + side[i]
			}
			lines = append(lines, strings.TrimRight(line, " "))
		}
	case w >= logoW && maxRows >= len(logo)+1+len(rows):
		for _, l := range logo {
			lines = append(lines, t.Accent.Render(strings.TrimRight(l, " ")))
		}
		lines = append(lines, "")
		lines = append(lines, info(w-infoLabelW)...)
	case maxRows >= 2+len(rows) && w > infoLabelW:
		lines = append(lines, t.Accent.Bold(true).Render("lega"), "")
		lines = append(lines, info(w-infoLabelW)...)
	default:
		title := t.Bold.Render("lega") + "  " + t.Dim.Render(rel.DisplayName())
		lines = append(lines, strings.TrimRight(fit(title, w), " "))
	}
	return strings.Join(lines, "\n")
}
