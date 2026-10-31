package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mydisha/lega/internal/format"
	"github.com/mydisha/lega/internal/oplog"
	"github.com/mydisha/lega/internal/safety"
	"github.com/mydisha/lega/internal/scan"
	"github.com/mydisha/lega/internal/trash"
)

// largestLimit is how many files the "largest files" view lists.
const largestLimit = 100

var (
	keyOpenDir = key.NewBinding(key.WithKeys("enter", "right", "l"), key.WithHelp("→", "open"))
	keyUpDir   = key.NewBinding(key.WithKeys("backspace", "left", "h"), key.WithHelp("←", "up"))
	keySort    = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort"))
	keyFiles   = key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "largest files"))
	keyReveal  = key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "show in Files"))
	keyDelete  = key.NewBinding(key.WithKeys("d", "delete"), key.WithHelp("d", "trash"))
	keyRescan  = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rescan"))
	keyYes     = key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yes"))
	keyNo      = key.NewBinding(key.WithKeys("n", "esc", "q"), key.WithHelp("n", "no"))
)

var sortNames = map[scan.SortBy]string{scan.BySize: "size", scan.ByName: "name", scan.ByFiles: "file count"}

// analyze is an interactive explorer that shows which folders use space.
type analyze struct {
	sh      *shared
	root    string
	scanner *scan.Scanner
	tree    *scan.Node
	cur     *scan.Node
	err     error
	cancel  context.CancelFunc
	spin    spinner.Model

	l         list
	sortBy    scan.SortBy
	filesView bool
	rows      []*scan.Node
	cursors   map[*scan.Node]int // cursor to restore when coming back up

	confirm   *scan.Node // waiting for y/n before removal
	permanent bool       // the confirm is for deleting without Trash
	note      string
	noteErr   bool

	trash trash.Trash
	guard *safety.Guard
	log   *oplog.Log
}

type treeMsg struct {
	node *scan.Node
	err  error
}

type noteMsg struct {
	text string
	err  bool
}

func newAnalyze(root string) func(*shared) screen {
	return func(sh *shared) screen {
		if root == "" {
			root = sh.env.Home
		}
		var userDirs []string
		for _, d := range sh.env.UserDirs() {
			userDirs = append(userDirs, d)
		}
		return &analyze{
			sh:      sh,
			root:    root,
			scanner: &scan.Scanner{},
			spin:    spinner.New(spinner.WithSpinner(spinner.MiniDot)),
			cursors: map[*scan.Node]int{},
			trash:   trash.Trash{DataDir: sh.env.DataDir, UID: sh.env.UID},
			guard:   safety.NewGuard(sh.env.Home, sh.env.UID, userDirs),
			log:     oplog.Open(sh.env.StateDir),
		}
	}
}

func (a *analyze) Init() tea.Cmd { return tea.Batch(a.rescan(), a.spin.Tick, tick()) }

func (a *analyze) rescan() tea.Cmd {
	a.Close()
	ctx, cancel := context.WithCancel(a.sh.ctx)
	a.cancel = cancel
	a.scanner = &scan.Scanner{}
	a.tree, a.cur, a.err, a.rows = nil, nil, nil, nil
	a.cursors = map[*scan.Node]int{}
	a.l = list{}
	sc, root := a.scanner, a.root
	return func() tea.Msg {
		n, err := sc.Scan(ctx, root)
		return treeMsg{n, err}
	}
}

// Close stops a running scan.
func (a *analyze) Close() {
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *analyze) scanning() bool { return a.tree == nil && a.err == nil }

// refresh rebuilds the visible rows after the folder, sort or view changed.
func (a *analyze) refresh() {
	if a.filesView {
		a.rows = a.cur.LargestFiles(largestLimit)
	} else {
		a.rows = a.cur.Sorted(a.sortBy)
	}
}

func (a *analyze) selected() *scan.Node {
	if a.l.cursor < len(a.rows) {
		return a.rows[a.l.cursor]
	}
	return nil
}

func (a *analyze) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case treeMsg:
		if errors.Is(msg.err, context.Canceled) {
			return a, nil
		}
		a.tree, a.err = msg.node, msg.err
		if a.tree != nil {
			a.err = nil
			a.cur = a.tree
			a.refresh()
		}
	case tickMsg:
		if a.scanning() {
			return a, tick()
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spin, cmd = a.spin.Update(msg)
		return a, cmd
	case noteMsg:
		a.note, a.noteErr = msg.text, msg.err
	case tea.KeyPressMsg:
		return a.key(msg)
	}
	return a, nil
}

func (a *analyze) key(msg tea.KeyPressMsg) (screen, tea.Cmd) {
	if a.confirm != nil {
		switch {
		case key.Matches(msg, keyYes):
			return a, a.remove()
		case key.Matches(msg, keyNo):
			a.confirm, a.note = nil, ""
		}
		return a, nil
	}
	if key.Matches(msg, keyBack) {
		return a, pop
	}
	if a.cur == nil {
		if key.Matches(msg, keyRescan) && a.err != nil {
			return a, tea.Batch(a.rescan(), tick())
		}
		return a, nil
	}
	a.note = ""
	if moveList(&a.l, msg, len(a.rows), a.listHeight()) {
		return a, nil
	}
	sel := a.selected()
	switch {
	case key.Matches(msg, keyOpenDir):
		switch {
		case sel == nil:
		case a.filesView:
			// Jump to the folder that holds the file.
			a.filesView = false
			a.goTo(sel.Parent, sel)
		case sel.IsDir:
			a.cursors[a.cur] = a.l.cursor
			a.goTo(sel, nil)
		}
	case key.Matches(msg, keyUpDir):
		if a.filesView {
			a.filesView = false
			a.refresh()
		} else if a.cur.Parent != nil {
			a.goTo(a.cur.Parent, a.cur)
		}
	case key.Matches(msg, keySort):
		a.sortBy = (a.sortBy + 1) % 3
		a.refresh()
	case key.Matches(msg, keyFiles):
		a.filesView = !a.filesView
		a.l = list{}
		a.refresh()
	case key.Matches(msg, keyReveal):
		if sel != nil {
			return a, reveal(sel)
		}
	case key.Matches(msg, keyDelete):
		if sel != nil {
			a.confirm, a.permanent = sel, false
		}
	case key.Matches(msg, keyRescan):
		return a, tea.Batch(a.rescan(), tick())
	}
	return a, nil
}

// goTo shows folder dir with the cursor on focus (or where it was before).
func (a *analyze) goTo(dir, focus *scan.Node) {
	a.cur = dir
	a.refresh()
	a.l = list{cursor: a.cursors[dir]}
	for i, r := range a.rows {
		if r == focus {
			a.l.cursor = i
		}
	}
}

// remove moves the confirmed item to the Trash, or deletes it when the
// user agreed to a permanent delete.
func (a *analyze) remove() tea.Cmd {
	n := a.confirm
	a.confirm = nil
	path := n.Path()
	if err := a.guard.Check(path, []string{a.tree.Path()}); err != nil {
		a.note, a.noteErr = err.Error(), true
		return nil
	}
	action := "trash"
	var err error
	if a.permanent {
		action = "delete"
		err = os.RemoveAll(path)
	} else {
		_, err = a.trash.Move(path)
		if errors.Is(err, trash.ErrNoTrash) {
			a.confirm, a.permanent = n, true
			return nil
		}
	}
	a.log.Write(oplog.Entry{Action: action, Path: path, Bytes: n.Size, Err: err})
	if err != nil {
		a.note, a.noteErr = err.Error(), true
		return nil
	}
	if n.Parent != nil {
		n.Parent.Remove(n)
	}
	a.refresh()
	verb := "Moved to Trash"
	if a.permanent {
		verb = "Deleted"
	}
	a.note, a.noteErr = fmt.Sprintf("%s: %s (%s)", verb, n.Name, format.Size(n.Size)), false
	return nil
}

// reveal opens the folder (or the folder holding the file) in the file
// manager.
func reveal(n *scan.Node) tea.Cmd {
	dir := n.Path()
	if !n.IsDir {
		dir = filepath.Dir(dir)
	}
	return func() tea.Msg {
		if err := exec.Command("xdg-open", dir).Start(); err != nil {
			return noteMsg{"Cannot open the file manager: " + err.Error(), true}
		}
		return noteMsg{"Opened " + dir, false}
	}
}

func (a *analyze) Keys() []key.Binding {
	if a.confirm != nil {
		return []key.Binding{keyYes, keyNo}
	}
	if a.cur == nil {
		return []key.Binding{keyRescan, keyBack}
	}
	return []key.Binding{keyOpenDir, keyUpDir, keySort, keyFiles, keyDelete, keyReveal, keyBack}
}

// listHeight is the number of rows left for the list after the header and
// the note and help lines.
func (a *analyze) listHeight() int { return max(a.sh.h()-6, 1) }

func (a *analyze) View() string {
	t, w := a.sh.theme, a.sh.w()
	var b strings.Builder

	switch {
	case a.err != nil:
		b.WriteString(t.Bold.Render("Analyze") + "  " + t.Dim.Render(a.root) + "\n\n")
		b.WriteString("  " + t.Danger.Render("Cannot scan: "+a.err.Error()) + "\n")
		return b.String()
	case a.scanning():
		b.WriteString(t.Bold.Render("Analyze") + "  " + t.Dim.Render(a.root) + "\n\n")
		files, bytes, _ := a.scanner.Progress()
		fmt.Fprintf(&b, "  %s %s\n", a.spin.View(), t.Dim.Render(fmt.Sprintf("Scanning… %s files, %s", format.Count(files), format.Size(bytes))))
		return b.String()
	}

	title := "Analyze"
	if a.filesView {
		title = "Largest files in"
	}
	b.WriteString(t.Bold.Render(title) + "  " + fitMiddle(a.cur.Path(), max(w-lipgloss.Width(title)-2, 10)) + "\n")
	info := fmt.Sprintf("%s · %s files", format.Size(a.cur.Size), format.Count(a.cur.Files))
	if !a.filesView {
		info += " · sorted by " + sortNames[a.sortBy]
	}
	if _, _, errs := a.scanner.Progress(); errs > 0 {
		info += fmt.Sprintf(" · %d folders not readable", errs)
	}
	b.WriteString(t.Dim.Render(info) + "\n\n")

	if len(a.rows) == 0 {
		msg := "This folder is empty."
		if a.filesView {
			msg = "No files here."
		}
		b.WriteString("  " + t.Dim.Render(msg) + "\n")
	}
	start, end := a.l.window(len(a.rows), a.listHeight())
	for i := start; i < end; i++ {
		b.WriteString(a.row(a.rows[i], i == a.l.cursor, w) + "\n")
	}

	switch {
	case a.confirm != nil && a.permanent:
		b.WriteString("\n" + t.Danger.Render(fmt.Sprintf("No Trash on this drive. Delete %s (%s) for good? y/n", a.confirm.Name, format.Size(a.confirm.Size))))
	case a.confirm != nil:
		b.WriteString("\n" + t.Warn.Render(fmt.Sprintf("Move %s (%s) to the Trash? y/n", a.confirm.Name, format.Size(a.confirm.Size))))
	case a.note != "" && a.noteErr:
		b.WriteString("\n" + t.Danger.Render(fit(a.note, w)))
	case a.note != "":
		b.WriteString("\n" + t.Accent.Render(fit(a.note, w)))
	}
	return b.String()
}

func (a *analyze) row(n *scan.Node, focused bool, w int) string {
	t := a.sh.theme
	name := n.Name
	if a.filesView {
		if rel, err := filepath.Rel(a.cur.Path(), n.Path()); err == nil {
			name = rel
		}
	} else if n.IsDir {
		name += "/"
	}
	pct := 0.0
	if a.cur.Size > 0 {
		pct = float64(n.Size) / float64(a.cur.Size)
	}

	marker, fill := "  ", lipgloss.NewStyle()
	if focused {
		marker = t.Accent.Render("▸ ")
		fill = lipgloss.NewStyle().Foreground(t.BarFill)
	}
	line := marker + right(format.Size(n.Size), 9) + "  " + right(fmt.Sprintf("%.0f%%", pct*100), 4) + "  "
	barW := min(max(w/5, 6), 20)
	if w-lipgloss.Width(line)-barW-2 < 12 {
		barW = 0 // too narrow: names matter more than bars
	}
	if barW > 0 {
		line += bar(t, barW, pct, fill) + "  "
	}
	nameW := w - lipgloss.Width(line) - a.sh.iconWidth()
	if n.Err != nil {
		name += " (no access)"
	}
	if a.filesView {
		name = fitMiddle(name, nameW)
	} else {
		name = fit(name, nameW)
	}
	ico := a.sh.icon(nodeIcon(n.Path(), n.IsDir, a.sh.userDirs), t.Dim)
	switch {
	case n.Err != nil:
		name = t.Dim.Render(name)
	case focused:
		ico = a.sh.icon(nodeIcon(n.Path(), n.IsDir, a.sh.userDirs), t.Accent)
		name = t.Accent.Render(name)
	}
	return line + ico + name
}
