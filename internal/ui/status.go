package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mydisha/lega/internal/disk"
	"github.com/mydisha/lega/internal/format"
	"github.com/mydisha/lega/internal/scan"
)

// homeRows is how many of the biggest home folders the status shows.
const homeRows = 10

var keyRefresh = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh"))

// status shows every disk and what takes space in the home folder.
type status struct {
	sh      *shared
	mounts  []disk.Mount
	diskErr error
	scanner *scan.Scanner
	home    *scan.Node
	homeErr error
	cancel  context.CancelFunc
	spin    spinner.Model
}

type mountsMsg struct {
	mounts []disk.Mount
	err    error
}

type homeMsg struct {
	node *scan.Node
	err  error
}

type tickMsg struct{}

func newStatus(sh *shared) screen {
	return &status{
		sh:      sh,
		scanner: &scan.Scanner{}, // so View works before the first scan starts
		spin:    spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}
}

func (s *status) Init() tea.Cmd {
	return tea.Batch(loadMounts, s.scanHome(), s.spin.Tick, tick())
}

func loadMounts() tea.Msg {
	m, err := disk.List()
	return mountsMsg{m, err}
}

func tick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func (s *status) scanHome() tea.Cmd {
	s.Close()
	ctx, cancel := context.WithCancel(s.sh.ctx)
	s.cancel = cancel
	s.scanner = &scan.Scanner{}
	s.home, s.homeErr = nil, nil
	sc, root := s.scanner, s.sh.env.Home
	return func() tea.Msg {
		n, err := sc.Scan(ctx, root)
		return homeMsg{n, err}
	}
}

// Close stops a running home scan.
func (s *status) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *status) scanning() bool { return s.home == nil && s.homeErr == nil }

func (s *status) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case mountsMsg:
		s.mounts, s.diskErr = msg.mounts, msg.err
	case homeMsg:
		s.home, s.homeErr = msg.node, msg.err
		if msg.err == context.Canceled {
			s.homeErr = nil
		}
	case tickMsg:
		if s.scanning() {
			return s, tick()
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		s.spin, cmd = s.spin.Update(msg)
		return s, cmd
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keyBack):
			return s, pop
		case key.Matches(msg, keyRefresh):
			return s, tea.Batch(loadMounts, s.scanHome(), tick())
		}
	}
	return s, nil
}

func (s *status) Keys() []key.Binding { return []key.Binding{keyRefresh, keyBack} }

func (s *status) View() string {
	t, w := s.sh.theme, s.sh.w()
	var b strings.Builder

	b.WriteString(t.Bold.Render("lega") + "  " + t.Dim.Render(s.sh.release.DisplayName()) + "\n\n")
	b.WriteString(t.Bold.Render("Disks") + "\n")
	switch {
	case s.diskErr != nil:
		b.WriteString("  " + t.Danger.Render("Cannot read disks: "+s.diskErr.Error()) + "\n")
	case s.mounts == nil:
		b.WriteString("  " + t.Dim.Render("Reading disks…") + "\n")
	default:
		b.WriteString(s.mountRows(w))
	}

	// The home line is "Home  <path>  <summary>", with the path shortened
	// to keep the summary on screen.
	summary := ""
	if s.home != nil {
		summary = fmt.Sprintf("%s in %s files", format.Size(s.home.Size), format.Count(s.home.Files))
	}
	pathW := max(w-lipgloss.Width("Home  ")-lipgloss.Width(summary)-2, 10)
	b.WriteString("\n" + t.Bold.Render("Home") + "  " + t.Dim.Render(strings.TrimRight(fitMiddle(s.sh.env.Home, pathW), " ")))
	switch {
	case s.homeErr != nil:
		b.WriteString("\n  " + t.Danger.Render("Cannot scan: "+s.homeErr.Error()) + "\n")
	case s.scanning():
		files, bytes, _ := s.scanner.Progress()
		fmt.Fprintf(&b, "\n  %s %s\n", s.spin.View(), t.Dim.Render(fmt.Sprintf("Scanning… %s files, %s", format.Count(files), format.Size(bytes))))
	default:
		b.WriteString("  " + t.Dim.Render(summary) + "\n")
		b.WriteString(s.homeRows(w))
	}
	return b.String()
}

func (s *status) mountRows(w int) string {
	t := s.sh.theme
	nameW := 4
	for _, m := range s.mounts {
		nameW = max(nameW, lipgloss.Width(m.MountPoint))
	}
	nameW = min(nameW, max(w/4, 10))

	var b strings.Builder
	for _, m := range s.mounts {
		pct := m.UsedPercent()
		info := fmt.Sprintf("%3.0f%%  %s free of %s", pct, t.Accent.Render(format.Size(m.Free)), format.Size(m.Total))
		barW := min(w-2-nameW-2-lipgloss.Width(info)-2, 30)
		fill := lipgloss.NewStyle()
		if pct >= 90 {
			fill = t.Danger
		}
		line := "  " + fitMiddle(m.MountPoint, nameW) + "  "
		if barW >= 6 {
			line += bar(t, barW, pct/100, fill) + "  "
		}
		b.WriteString(line + info + "\n")
	}
	if len(s.mounts) == 0 {
		b.WriteString("  " + t.Dim.Render("No disks found.") + "\n")
	}
	return b.String()
}

func (s *status) homeRows(w int) string {
	t := s.sh.theme
	rows := s.home.Sorted(scan.BySize)
	if len(rows) > homeRows {
		rows = rows[:homeRows]
	}
	if len(rows) == 0 {
		return "  " + t.Dim.Render("Your home folder is empty.") + "\n"
	}
	nameW := min(max(w/4, 12), 24)
	var b strings.Builder
	for _, n := range rows {
		frac := 0.0
		if rows[0].Size > 0 {
			frac = float64(n.Size) / float64(rows[0].Size)
		}
		size := right(format.Size(n.Size), 9)
		barW := min(w-2-nameW-2-9-2, 30)
		ico := s.sh.icon(nodeIcon(n.Path(), n.IsDir, s.sh.userDirs), t.Dim)
		line := "  " + ico + fit(n.Name, nameW-s.sh.iconWidth()) + "  "
		if barW >= 6 {
			line += bar(t, barW, frac, lipgloss.NewStyle()) + "  "
		}
		b.WriteString(line + size + "\n")
	}
	return b.String()
}
