package clean

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/mydisha/lega/internal/oplog"
)

// Finding is what one rule found during a scan.
type Finding struct {
	Rule    Rule
	Targets []Target
	Bytes   int64
	Unknown bool   // at least one target has an unknown size
	Locked  string // why the rule cannot run now, such as an open browser
	Err     error
}

// Report sums up cleaning one finding.
type Report struct {
	RuleID   string
	Freed    int64
	Removed  int
	Failures []Removal
}

// Available returns the rules whose required commands are installed.
func Available(c *Context, rules []Rule) []Rule {
	var out []Rule
	for _, r := range rules {
		ok := true
		for _, cmd := range r.Info().Needs {
			if _, err := c.LookPath(cmd); err != nil {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, r)
		}
	}
	return out
}

// Scan runs every rule in parallel and returns findings in rule order.
// Rules that found nothing are left out. onDone, if set, is called after
// each rule finishes, from any goroutine.
func Scan(ctx context.Context, c *Context, rules []Rule, onDone func(Rule)) []Finding {
	found := make([]Finding, len(rules))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, r := range rules {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			targets, err := r.Scan(ctx, c)
			if errors.Is(err, fs.ErrNotExist) {
				err = nil
			}
			found[i] = Finding{Rule: r, Targets: targets, Err: err}
			if onDone != nil {
				onDone(r)
			}
		}()
	}
	wg.Wait()

	dedupe(found)
	var out []Finding
	for _, f := range found {
		for _, t := range f.Targets {
			f.Bytes += t.Bytes
			f.Unknown = f.Unknown || t.Unknown
		}
		if len(f.Targets) == 0 || (f.Bytes == 0 && !f.Unknown) {
			if f.Err == nil {
				continue
			}
		}
		f.Locked = c.locked(f.Rule.Info())
		out = append(out, f)
	}
	return out
}

// dedupe drops targets that overlap a target of an earlier rule, so one
// path is never counted or deleted twice. Rules are ordered from specific
// to general, so the specific rule keeps the path.
func dedupe(found []Finding) {
	var claimed []string
	for i := range found {
		kept := found[i].Targets[:0]
		for _, t := range found[i].Targets {
			if overlaps(t.Path, claimed) {
				continue
			}
			kept = append(kept, t)
		}
		found[i].Targets = kept
		for _, t := range kept {
			claimed = append(claimed, t.Path)
		}
	}
}

func overlaps(p string, claimed []string) bool {
	for _, c := range claimed {
		if p == c || strings.HasPrefix(p, c+"/") || strings.HasPrefix(c, p+"/") {
			return true
		}
	}
	return false
}

// locked returns a reason when one of the rule's processes is running.
func (c *Context) locked(i Info) string {
	for _, p := range i.Processes {
		if c.Running[p] {
			return fmt.Sprintf("%s is running; close it first", p)
		}
	}
	return ""
}

// Clean removes the targets of a finding and logs each removal.
func Clean(ctx context.Context, c *Context, f Finding) Report {
	info := f.Rule.Info()
	rep := Report{RuleID: info.ID}
	if reason := c.locked(info); reason != "" {
		rep.Failures = append(rep.Failures, Removal{Path: info.Name, Err: reason})
		return rep
	}
	for _, r := range f.Rule.Clean(ctx, c, f.Targets) {
		rep.add(r)
		var err error
		if r.Err != "" {
			err = errors.New(r.Err)
		}
		c.Log.Write(oplog.Entry{Action: "clean", Rule: info.ID, Path: r.Path, Bytes: r.Bytes, Err: err})
	}
	return rep
}

func (rep *Report) add(r Removal) {
	if r.Err != "" {
		rep.Failures = append(rep.Failures, r)
		return
	}
	rep.Freed += r.Bytes
	rep.Removed++
}

// Pickable reports whether the user may choose single targets of this
// finding. Commands and root rules always work on all their targets.
func (f Finding) Pickable() bool {
	_, ok := f.Rule.(*pathRule)
	return ok && !f.Rule.Info().NeedsRoot
}
