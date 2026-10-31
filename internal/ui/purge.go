package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/mydisha/lega/internal/clean"
	"github.com/mydisha/lega/internal/format"
	"github.com/mydisha/lega/internal/oplog"
	"github.com/mydisha/lega/internal/purge"
	"github.com/mydisha/lega/internal/safety"
)

// purger finds build folders in projects and removes the chosen ones.
type purger struct {
	sh     *shared
	roots  []string
	finder *purge.Finder
	log    *oplog.Log
	err    error
	phase  phase
	spin   spinner.Model
	cancel context.CancelFunc

	found []purge.Artifact
	sel   []bool
	l     list

	queue    []int
	current  string
	freed    int64
	removed  int
	failures []string
}

type artifactsMsg struct {
	found []purge.Artifact
	err   error
}

type purgedMsg struct {
	a   purge.Artifact
	err error
}

// newPurger builds the purge screen. Without roots it searches the usual
// project folders in home.
func newPurger(roots []string) func(*shared) screen {
	return func(sh *shared) screen {
		wl, err := safety.LoadWhitelist(clean.WhitelistPath(sh.env), sh.env.Home)
		if err != nil {
			err = fmt.Errorf("%w. Fix or remove %s, then try again", err, clean.WhitelistPath(sh.env))
		}
		var userDirs []string
		for _, d := range sh.env.UserDirs() {
			userDirs = append(userDirs, d)
		}
		g := safety.NewGuard(sh.env.Home, sh.env.UID, userDirs)
		g.Whitelist = wl
		if len(roots) == 0 {
			roots = purge.DefaultRoots(sh.env.Home)
		}
		return &purger{
			sh: sh, roots: roots, err: err,
			finder: purge.NewFinder(g),
			log:    oplog.Open(sh.env.StateDir),
			spin:   spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		}
	}
}

func (p *purger) Init() tea.Cmd {
	if p.err != nil || len(p.roots) == 0 {
		p.phase = phasePick
		return nil
	}
	ctx, cancel := context.WithCancel(p.sh.ctx)
	p.cancel = cancel
	f, roots := p.finder, p.roots
	return tea.Batch(func() tea.Msg {
		found, err := f.Find(ctx, roots)
		return artifactsMsg{found, err}
	}, p.spin.Tick)
}

// Close stops a running search.
func (p *purger) Close() {
	if p.cancel != nil {
		p.cancel()
	}
}

func (p *purger) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case artifactsMsg:
		if msg.err == context.Canceled {
			return p, nil
		}
		p.found, p.err = msg.found, msg.err
		p.sel = make([]bool, len(p.found))
		for i, a := range p.found {
			p.sel[i] = !a.Recent
		}
		p.phase = phasePick
	case spinner.TickMsg:
		var cmd tea.Cmd
		p.spin, cmd = p.spin.Update(msg)
		return p, cmd
	case purgedMsg:
		if msg.err != nil {
			p.failures = append(p.failures, fmt.Sprintf("%s: %v", tilde(p.sh.env.Home, msg.a.Path), msg.err))
		} else {
			p.freed += msg.a.Bytes
			p.removed++
		}
		return p, p.next()
	case tea.KeyPressMsg:
		return p.key(msg)
	}
	return p, nil
}

func (p *purger) key(msg tea.KeyPressMsg) (screen, tea.Cmd) {
	switch p.phase {
	case phaseScan:
		if key.Matches(msg, keyBack) {
			return p, pop
		}
	case phasePick:
		if key.Matches(msg, keyBack) {
			return p, pop
		}
		if moveList(&p.l, msg, len(p.found), p.listHeight()) || len(p.found) == 0 {
			return p, nil
		}
		switch {
		case key.Matches(msg, keyToggle):
			p.sel[p.l.cursor] = !p.sel[p.l.cursor]
		case key.Matches(msg, keyAll):
			on := !anyTrue(p.sel)
			for i := range p.sel {
				p.sel[i] = on
			}
		case key.Matches(msg, keyProceed):
			if anyTrue(p.sel) {
				p.phase = phaseConfirm
			}
		}
	case phaseConfirm:
		switch {
		case key.Matches(msg, keyConfirm):
			p.phase = phaseWork
			p.queue = nil
			for i, on := range p.sel {
				if on {
					p.queue = append(p.queue, i)
				}
			}
			return p, p.next()
		case key.Matches(msg, keyCancel):
			p.phase = phasePick
		}
	case phaseDone:
		if key.Matches(msg, keyDone) {
			return p, pop
		}
	}
	return p, nil
}

func (p *purger) next() tea.Cmd {
	if len(p.queue) == 0 {
		p.phase = phaseDone
		return nil
	}
	a := p.found[p.queue[0]]
	p.queue = p.queue[1:]
	p.current = tilde(p.sh.env.Home, a.Path)
	g, log := p.finder.Guard, p.log
	return func() tea.Msg { return purgedMsg{a, purge.Remove(g, log, a)} }
}

func (p *purger) selection() (n int, bytes int64) {
	for i, on := range p.sel {
		if on {
			n++
			bytes += p.found[i].Bytes
		}
	}
	return n, bytes
}

func (p *purger) Keys() []key.Binding {
	switch p.phase {
	case phasePick:
		if len(p.found) == 0 {
			return []key.Binding{keyBack}
		}
		return []key.Binding{keyToggle, keyAll, keyProceed, keyBack}
	case phaseConfirm:
		return []key.Binding{keyConfirm, keyCancel}
	case phaseDone:
		return []key.Binding{keyDone}
	case phaseWork:
		return nil
	}
	return []key.Binding{keyBack}
}

func (p *purger) listHeight() int { return max(p.sh.h()-8, 1) }

func (p *purger) View() string {
	t, w := p.sh.theme, p.sh.w()
	var b strings.Builder
	b.WriteString(t.Bold.Render("Purge project build folders") + "\n")
	var shown []string
	for _, r := range p.roots {
		shown = append(shown, tilde(p.sh.env.Home, r))
	}
	b.WriteString(t.Dim.Render(fit("In "+strings.Join(shown, ", "), w)) + "\n\n")

	switch {
	case p.err != nil:
		b.WriteString(t.Danger.Render(fit(p.err.Error(), w)) + "\n")
		return b.String()
	case len(p.roots) == 0:
		b.WriteString("No project folders found in your home.\n")
		b.WriteString(t.Dim.Render("Run \"lega purge <folder>\" to search a folder of your choice.") + "\n")
		return b.String()
	}

	switch p.phase {
	case phaseScan:
		fmt.Fprintf(&b, "  %s %s\n", p.spin.View(), t.Dim.Render("Looking for node_modules, target, .venv and other build folders…"))
	case phasePick:
		if len(p.found) == 0 {
			b.WriteString("  Nothing to purge here.\n")
			break
		}
		now := time.Now()
		start, end := p.l.window(len(p.found), p.listHeight())
		for i := start; i < end; i++ {
			a := p.found[i]
			marker := "  "
			if i == p.l.cursor {
				marker = t.Accent.Render("▸ ")
			}
			head := marker + checkbox(t, p.sel[i], false) + " " + right(format.Size(a.Bytes), 9) + "  "
			tail := ""
			if a.Recent {
				tail = t.Dim.Render("  used " + ago(now, a.ModTime))
			}
			pathW := w - 2 - 4 - 9 - 2 - len("  used yesterday")
			b.WriteString(head + fitMiddle(tilde(p.sh.env.Home, a.Path), pathW) + tail + "\n")
		}
		n, bytes := p.selection()
		b.WriteString("\n" + t.Dim.Render("Folders used in the last 7 days start unselected.") + "\n")
		fmt.Fprintf(&b, "Selected: %s in %d folders", t.Accent.Render(format.Size(bytes)), n)
	case phaseConfirm:
		n, bytes := p.selection()
		fmt.Fprintf(&b, "Delete %d build folders, %s?\n\n", n, t.Accent.Render(format.Size(bytes)))
		b.WriteString(t.Dim.Render("They are deleted for good. Run your install or build command to get them back.") + "\n")
	case phaseWork:
		fmt.Fprintf(&b, "  %s Removing %s…\n", p.spin.View(), fitMiddle(p.current, w-16))
	case phaseDone:
		fmt.Fprintf(&b, "Freed %s from %d folders.\n", t.Accent.Render(format.Size(p.freed)), p.removed)
		if len(p.failures) > 0 {
			fmt.Fprintf(&b, "\n%d folders could not be removed:\n", len(p.failures))
			b.WriteString(failureLines(t, p.failures, w))
		}
	}
	return b.String()
}
