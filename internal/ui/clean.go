package ui

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/mydisha/lega/internal/clean"
	"github.com/mydisha/lega/internal/disk"
	"github.com/mydisha/lega/internal/format"
	"github.com/mydisha/lega/internal/safety"
)

type phase int

const (
	phaseScan phase = iota
	phasePick
	phaseConfirm
	phaseWork
	phaseDone
)

// row is one line of the clean list: a group title, a rule, or one target
// of an expanded rule.
type row struct {
	group  string
	f      int // finding index, -1 for group titles
	target int // target index, -1 for the rule line
}

// cleaner scans for junk, lets the user pick, and cleans.
type cleaner struct {
	sh     *shared
	c      *clean.Context
	rules  []clean.Rule
	err    error // setup failed, for example an unreadable whitelist
	phase  phase
	done   atomic.Int32
	spin   spinner.Model
	cancel context.CancelFunc

	found []clean.Finding
	sel   [][]bool // selection per finding, per target
	open  map[int]bool
	rows  []row
	l     list

	queue     []int // findings left to clean
	current   string
	reports   []clean.Report
	failures  []string
	freeStart int64
	freeEnd   int64
}

type scannedMsg struct{ found []clean.Finding }

type cleanedMsg struct{ rep clean.Report }

type sudoMsg struct{ err error }

type rootDoneMsg struct {
	reps []clean.Report
	err  error
}

// newCleaner builds the clean screen for the real system.
func newCleaner(sh *shared) screen {
	wl, err := safety.LoadWhitelist(clean.WhitelistPath(sh.env), sh.env.Home)
	if err != nil {
		return &cleaner{sh: sh, err: fmt.Errorf("%w. Fix or remove %s, then try again", err, clean.WhitelistPath(sh.env))}
	}
	c := clean.NewContext(sh.env, wl)
	return newCleanerWith(sh, c, clean.Available(c, clean.Rules()))
}

func newCleanerWith(sh *shared, c *clean.Context, rules []clean.Rule) *cleaner {
	return &cleaner{
		sh: sh, c: c, rules: rules,
		spin: spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		open: map[int]bool{},
	}
}

func (m *cleaner) Init() tea.Cmd {
	if m.err != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(m.sh.ctx)
	m.cancel = cancel
	c, rules, done := m.c, m.rules, &m.done
	scanCmd := func() tea.Msg {
		return scannedMsg{clean.Scan(ctx, c, rules, func(clean.Rule) { done.Add(1) })}
	}
	return tea.Batch(scanCmd, m.spin.Tick, tick())
}

// Close stops a running scan.
func (m *cleaner) Close() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *cleaner) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case scannedMsg:
		m.found = msg.found
		m.sel = make([][]bool, len(m.found))
		for i, f := range m.found {
			on := f.Rule.Info().Risk == clean.Safe && m.usable(i)
			m.sel[i] = make([]bool, len(f.Targets))
			for j := range m.sel[i] {
				m.sel[i][j] = on
			}
		}
		m.buildRows()
		m.phase = phasePick
	case tickMsg:
		if m.phase == phaseScan {
			return m, tick()
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case cleanedMsg:
		m.addReport(msg.rep)
		return m, m.next()
	case sudoMsg:
		if msg.err != nil {
			m.failures = append(m.failures, "System items skipped: sudo did not get a password")
			return m, m.finish()
		}
		m.current = "system items (sudo)"
		return m, m.cleanRoot()
	case rootDoneMsg:
		if msg.err != nil {
			m.failures = append(m.failures, "System items failed: "+msg.err.Error())
		}
		for _, r := range msg.reps {
			m.addReport(r)
		}
		return m, m.finish()
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *cleaner) key(msg tea.KeyPressMsg) (screen, tea.Cmd) {
	switch m.phase {
	case phaseScan:
		if key.Matches(msg, keyBack) {
			return m, pop
		}
	case phasePick:
		return m.pickKey(msg)
	case phaseConfirm:
		switch {
		case key.Matches(msg, keyConfirm):
			return m, m.start()
		case key.Matches(msg, keyCancel):
			m.phase = phasePick
		}
	case phaseDone:
		if key.Matches(msg, keyDone) {
			return m, pop
		}
	}
	return m, nil
}

func (m *cleaner) pickKey(msg tea.KeyPressMsg) (screen, tea.Cmd) {
	if key.Matches(msg, keyBack) {
		return m, pop
	}
	if moveList(&m.l, msg, len(m.rows), m.listHeight()) {
		step := 1
		if key.Matches(msg, keyUp, keyPgUp) {
			step = -1
		}
		m.skipGroups(step)
		return m, nil
	}
	if len(m.rows) == 0 {
		return m, nil
	}
	r := m.rows[m.l.cursor]
	switch {
	case key.Matches(msg, keyToggle):
		m.toggle(r)
	case key.Matches(msg, keyAll):
		m.toggleAll()
	case key.Matches(msg, keyExpand):
		if r.f >= 0 && len(m.found[r.f].Targets) > 0 {
			m.open[r.f] = true
			m.buildRows()
		}
	case key.Matches(msg, keyFold):
		if r.f >= 0 {
			delete(m.open, r.f)
			m.buildRows()
			m.focus(r.f)
		}
	case key.Matches(msg, keyProceed):
		if n, _ := m.selection(); n > 0 {
			m.phase = phaseConfirm
		}
	}
	return m, nil
}

// skipGroups moves the cursor off a group title: onward in the direction
// of travel, or back if the title is at the edge of the list.
func (m *cleaner) skipGroups(step int) {
	for _, dir := range []int{step, -step} {
		for c := m.l.cursor; c >= 0 && c < len(m.rows); c += dir {
			if m.rows[c].f >= 0 {
				m.l.cursor = c
				return
			}
		}
	}
}

// usable reports whether finding i can be selected at all.
func (m *cleaner) usable(i int) bool {
	return m.found[i].Locked == "" && len(m.found[i].Targets) > 0
}

func (m *cleaner) toggle(r row) {
	if r.f < 0 || !m.usable(r.f) {
		return
	}
	sel := m.sel[r.f]
	if r.target >= 0 && m.found[r.f].Pickable() {
		sel[r.target] = !sel[r.target]
		return
	}
	on := !anyTrue(sel)
	for j := range sel {
		sel[j] = on
	}
}

func (m *cleaner) toggleAll() {
	on := true
	if n, _ := m.selection(); n > 0 {
		on = false
	}
	for i := range m.sel {
		for j := range m.sel[i] {
			m.sel[i][j] = on && m.usable(i)
		}
	}
}

// selection returns how many rules have something selected, and the
// total size selected.
func (m *cleaner) selection() (n int, bytes int64) {
	for i, sel := range m.sel {
		if cn, cb := countSel(sel, m.found[i].Targets); cn > 0 {
			n++
			bytes += cb
		}
	}
	return n, bytes
}

func (m *cleaner) buildRows() {
	m.rows = nil
	last := ""
	for _, g := range []string{clean.GroupSystem, clean.GroupBrowsers, clean.GroupDev, clean.GroupApps, clean.GroupFiles, clean.GroupRoot} {
		for i, f := range m.found {
			if f.Rule.Info().Group != g {
				continue
			}
			if g != last {
				m.rows = append(m.rows, row{group: g, f: -1, target: -1})
				last = g
			}
			m.rows = append(m.rows, row{f: i, target: -1})
			if m.open[i] {
				for j := range f.Targets {
					m.rows = append(m.rows, row{f: i, target: j})
				}
			}
		}
	}
	m.l.move(0, len(m.rows))
	m.skipGroups(1)
}

// focus puts the cursor on the rule line of finding f.
func (m *cleaner) focus(f int) {
	for i, r := range m.rows {
		if r.f == f && r.target < 0 {
			m.l.cursor = i
		}
	}
}

// start cleans the user's findings one by one, then the root ones.
func (m *cleaner) start() tea.Cmd {
	m.phase = phaseWork
	if _, _, free, err := disk.Usage(m.sh.env.Home); err == nil {
		m.freeStart = free
	}
	m.queue = nil
	for i := range m.found {
		if anyTrue(m.sel[i]) && !m.found[i].Rule.Info().NeedsRoot {
			m.queue = append(m.queue, i)
		}
	}
	return m.next()
}

// picked returns finding i limited to the selected targets.
func (m *cleaner) picked(i int) clean.Finding {
	f := m.found[i]
	var ts []clean.Target
	for j, t := range f.Targets {
		if m.sel[i][j] {
			ts = append(ts, t)
		}
	}
	f.Targets = ts
	return f
}

func (m *cleaner) next() tea.Cmd {
	if len(m.queue) > 0 {
		i := m.queue[0]
		m.queue = m.queue[1:]
		f, c, ctx := m.picked(i), m.c, m.sh.ctx
		m.current = f.Rule.Info().Name
		return func() tea.Msg { return cleanedMsg{clean.Clean(ctx, c, f)} }
	}
	if len(m.rootPicked()) > 0 {
		m.current = "waiting for sudo"
		return tea.ExecProcess(clean.SudoValidate(), func(err error) tea.Msg { return sudoMsg{err} })
	}
	return m.finish()
}

func (m *cleaner) rootPicked() []clean.Finding {
	var out []clean.Finding
	for i, f := range m.found {
		if f.Rule.Info().NeedsRoot && anyTrue(m.sel[i]) {
			out = append(out, f)
		}
	}
	return out
}

func (m *cleaner) cleanRoot() tea.Cmd {
	fs, c, ctx := m.rootPicked(), m.c, m.sh.ctx
	return func() tea.Msg {
		reps, err := clean.CleanAsRoot(ctx, c, fs)
		return rootDoneMsg{reps, err}
	}
}

func (m *cleaner) addReport(r clean.Report) {
	m.reports = append(m.reports, r)
	for _, f := range r.Failures {
		m.failures = append(m.failures, fmt.Sprintf("%s: %s", tilde(m.sh.env.Home, f.Path), f.Err))
	}
}

func (m *cleaner) finish() tea.Cmd {
	if _, _, free, err := disk.Usage(m.sh.env.Home); err == nil {
		m.freeEnd = free
	}
	m.phase = phaseDone
	return nil
}

func (m *cleaner) Keys() []key.Binding {
	switch m.phase {
	case phasePick:
		if len(m.rows) == 0 {
			return []key.Binding{keyBack}
		}
		return []key.Binding{keyToggle, keyAll, keyExpand, keyProceed, keyBack}
	case phaseConfirm:
		return []key.Binding{keyConfirm, keyCancel}
	case phaseDone:
		return []key.Binding{keyDone}
	case phaseWork:
		return nil
	}
	return []key.Binding{keyBack}
}

func (m *cleaner) listHeight() int { return max(m.sh.h()-8, 1) }

func (m *cleaner) View() string {
	t, w := m.sh.theme, m.sh.w()
	var b strings.Builder
	b.WriteString(t.Bold.Render("Clean") + "\n\n")

	if m.err != nil {
		b.WriteString(t.Danger.Render(fit(m.err.Error(), w)) + "\n")
		return b.String()
	}
	switch m.phase {
	case phaseScan:
		fmt.Fprintf(&b, "  %s %s\n", m.spin.View(), t.Dim.Render(fmt.Sprintf("Looking for junk… %d of %d checks done", m.done.Load(), len(m.rules))))
	case phasePick:
		m.viewPick(&b, w)
	case phaseConfirm:
		m.viewConfirm(&b, w)
	case phaseWork:
		fmt.Fprintf(&b, "  %s Cleaning %s…\n", m.spin.View(), m.current)
	case phaseDone:
		m.viewDone(&b, w)
	}
	return b.String()
}

func (m *cleaner) viewPick(b *strings.Builder, w int) {
	t := m.sh.theme
	if len(m.rows) == 0 {
		b.WriteString("  Nothing to clean. Caches and logs are already small.\n")
		return
	}
	start, end := m.l.window(len(m.rows), m.listHeight())
	for i := start; i < end; i++ {
		b.WriteString(m.rowView(m.rows[i], i == m.l.cursor, w) + "\n")
	}
	n, bytes := m.selection()
	b.WriteString("\n")
	if r := m.rows[m.l.cursor]; r.f >= 0 {
		f := m.found[r.f]
		desc := f.Rule.Info().Description
		switch {
		case f.Locked != "":
			desc = f.Locked
		case f.Err != nil:
			desc = f.Err.Error()
		}
		b.WriteString(t.Dim.Render(fit(desc, w)) + "\n")
	}
	fmt.Fprintf(b, "Selected: %s in %d items", t.Accent.Render(format.Size(bytes)), n)
}

func (m *cleaner) rowView(r row, focused bool, w int) string {
	t := m.sh.theme
	if r.f < 0 {
		return t.Bold.Render(r.group)
	}
	f := m.found[r.f]
	marker := "  "
	if focused {
		marker = t.Accent.Render("▸ ")
	}
	sizeW := 12
	if r.target >= 0 {
		tg := f.Targets[r.target]
		on := m.sel[r.f][r.target]
		box := checkbox(t, on, !m.usable(r.f))
		if !f.Pickable() {
			box = "   "
		}
		size := ""
		if !tg.Unknown {
			size = format.Size(tg.Bytes)
		}
		nameW := w - 2 - 4 - 4 - sizeW
		return marker + "    " + box + " " + fitMiddle(tilde(m.sh.env.Home, tg.Path), nameW) + right(size, sizeW)
	}

	info := f.Rule.Info()
	name := info.Name
	tag := ""
	switch {
	case f.Locked != "":
		tag = "locked"
	case info.Risk == clean.Review:
		tag = "review"
	}
	if info.NeedsRoot {
		tag = strings.TrimPrefix(tag+", sudo", ", ")
	}
	size := format.Size(f.Bytes)
	switch {
	case f.Unknown && f.Bytes == 0:
		size = "size unknown"
	case f.Unknown:
		size = "up to " + size
	}
	if f.Err != nil {
		size = "error"
	}
	nameW := w - 2 - 4 - sizeW - 1 - m.sh.iconWidth()
	label := name
	if tag != "" {
		label += t.Dim.Render(" · " + tag)
	}
	if len(f.Targets) > 1 {
		label += t.Dim.Render(fmt.Sprintf(" (%d)", len(f.Targets)))
	}
	icoStyle := t.Dim
	if focused {
		label = t.Accent.Render(name) + strings.TrimPrefix(label, name)
		icoStyle = t.Accent
	}
	ico := m.sh.icon(ruleIcon(info.ID), icoStyle)
	return marker + checkbox(t, anyTrue(m.sel[r.f]), !m.usable(r.f)) + " " + ico + fit(label, nameW) + right(size, sizeW+1)
}

func (m *cleaner) viewConfirm(b *strings.Builder, w int) {
	t := m.sh.theme
	n, bytes := m.selection()
	fmt.Fprintf(b, "Remove %d items, %s?\n\n", n, t.Accent.Render(format.Size(bytes)))
	root := false
	for i, f := range m.found {
		if !anyTrue(m.sel[i]) {
			continue
		}
		_, sel := countSel(m.sel[i], f.Targets)
		b.WriteString("  " + fit(f.Rule.Info().Name, w-16) + right(format.Size(sel), 14) + "\n")
		root = root || f.Rule.Info().NeedsRoot
	}
	b.WriteString("\n" + t.Dim.Render("Files are deleted for good, not moved to the Trash.") + "\n")
	if root {
		b.WriteString(t.Warn.Render("System items need your sudo password next.") + "\n")
	}
}

func (m *cleaner) viewDone(b *strings.Builder, w int) {
	t := m.sh.theme
	var freed int64
	var removed int
	for _, r := range m.reports {
		freed += r.Freed
		removed += r.Removed
	}
	fmt.Fprintf(b, "Freed %s from %d items.\n", t.Accent.Render(format.Size(freed)), removed)
	if m.freeStart > 0 && m.freeEnd > 0 {
		fmt.Fprintf(b, "Free space in your home disk: %s → %s\n", format.Size(m.freeStart), t.Bold.Render(format.Size(m.freeEnd)))
	}
	if len(m.failures) > 0 {
		fmt.Fprintf(b, "\n%d items could not be removed:\n", len(m.failures))
		b.WriteString(failureLines(t, m.failures, w))
	}
	if p := m.c.Log.Path(); p != "" {
		b.WriteString("\n" + t.Dim.Render(fit("Details: "+tilde(m.sh.env.Home, p), w)) + "\n")
	}
}

func countSel(sel []bool, ts []clean.Target) (int, int64) {
	var n int
	var bytes int64
	for j, on := range sel {
		if on {
			n++
			bytes += ts[j].Bytes
		}
	}
	return n, bytes
}

func anyTrue(bs []bool) bool {
	for _, b := range bs {
		if b {
			return true
		}
	}
	return false
}
