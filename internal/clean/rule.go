// Package clean finds junk files and removes them. Each kind of junk is a
// Rule; the engine scans all rules, shows what they found, and cleans the
// ones the user picks.
package clean

import (
	"context"
	"time"

	"github.com/mydisha/lega/internal/oplog"
	"github.com/mydisha/lega/internal/platform"
	"github.com/mydisha/lega/internal/safety"
)

// Risk tells how safe a rule is to run without looking.
type Risk int

const (
	// Safe rules only remove data apps rebuild on their own. They are
	// selected by default.
	Safe Risk = iota
	// Review rules remove things the user may still want, or that are slow
	// to get back. They start unselected.
	Review
)

// Groups shown in the UI, in display order.
const (
	GroupSystem   = "System"
	GroupBrowsers = "Browsers"
	GroupDev      = "Developer tools"
	GroupApps     = "Apps"
	GroupFiles    = "Files"
	GroupRoot     = "System (needs sudo)"
)

// Info describes a rule.
type Info struct {
	ID          string
	Name        string
	Group       string
	Description string
	Risk        Risk
	NeedsRoot   bool
	// Processes that use the files. The rule is locked while one runs.
	Processes []string
	// Needs lists commands that must be installed for the rule to apply.
	Needs []string
}

// Target is one file, folder or package a rule can remove.
type Target struct {
	Path    string
	Root    string // folder the guard allows this target to be removed from
	Bytes   int64
	Files   int64
	Unknown bool // the size cannot be measured before cleaning
}

// Removal is the outcome for one target or command.
type Removal struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Err   string `json:"err,omitempty"`
}

// Rule finds and removes one kind of junk.
type Rule interface {
	Info() Info
	Scan(ctx context.Context, c *Context) ([]Target, error)
	Clean(ctx context.Context, c *Context, targets []Target) []Removal
}

// Runner runs a command and returns its standard output.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Context holds everything rules need from the outside world. Tests swap
// these fields for fakes.
type Context struct {
	Env platform.Env
	// Guard checks user files; SysGuard checks system files and skips the
	// owner check.
	Guard    *safety.Guard
	SysGuard *safety.Guard
	Log      *oplog.Log
	Run      Runner
	LookPath func(string) (string, error)
	Now      func() time.Time
	// Running holds the names of the user's running processes.
	Running map[string]bool
}

func (c *Context) guardFor(i Info) *safety.Guard {
	if i.NeedsRoot {
		return c.SysGuard
	}
	return c.Guard
}
