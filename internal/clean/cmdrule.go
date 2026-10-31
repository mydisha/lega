package clean

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mydisha/lega/internal/scan"
)

// commandRule cleans by running a tool's own cleanup command, such as
// "apt-get clean". It measures folders (or calls measureFn) before and after
// to report the space really freed.
type commandRule struct {
	info      Info
	measure   []string // folders whose size the command shrinks
	measureFn func(ctx context.Context, c *Context) ([]Target, error)
	cmd       []string
	// partial means the command frees only part of what is measured, so
	// the size is shown as "up to".
	partial bool
	// hideSize means the measurement says nothing about what will be
	// freed; it is only used to report the result after cleaning.
	hideSize bool
}

func (r *commandRule) Info() Info { return r.info }

func (r *commandRule) Scan(ctx context.Context, c *Context) ([]Target, error) {
	out, err := r.measureAll(ctx, c)
	if r.hideSize {
		if len(out) == 0 {
			return nil, err
		}
		return []Target{{Path: r.info.Name, Unknown: true}}, err
	}
	if r.partial {
		for i := range out {
			out[i].Unknown = true
		}
	}
	return out, err
}

func (r *commandRule) measureAll(ctx context.Context, c *Context) ([]Target, error) {
	if r.measureFn != nil {
		return r.measureFn(ctx, c)
	}
	var out []Target
	for _, m := range r.measure {
		paths, _ := filepath.Glob(expand(c, m))
		for _, p := range paths {
			bytes, files, err := scan.Size(ctx, p)
			if err != nil || bytes == 0 {
				continue
			}
			out = append(out, Target{Path: p, Bytes: bytes, Files: files})
		}
	}
	return out, nil
}

func (r *commandRule) Clean(ctx context.Context, c *Context, _ []Target) []Removal {
	measured, _ := r.measureAll(ctx, c)
	before := sum(measured)
	rm := Removal{Path: strings.Join(r.cmd, " ")}
	if _, err := c.Run(ctx, r.cmd[0], r.cmd[1:]...); err != nil {
		rm.Err = err.Error()
		return []Removal{rm}
	}
	if after, err := r.measureAll(ctx, c); err == nil {
		rm.Bytes = max(before-sum(after), 0)
	}
	return []Removal{rm}
}

func sum(ts []Target) int64 {
	var n int64
	for _, t := range ts {
		n += t.Bytes
	}
	return n
}

// ExecRunner runs commands for real. Output is forced to English so it can
// be parsed, and apt never stops to ask questions.
func ExecRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(cmd.Environ(), "LC_ALL=C", "DEBIAN_FRONTEND=noninteractive")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return out, &CommandError{Cmd: name, Msg: lastLine(msg), Err: err}
		}
		return out, &CommandError{Cmd: name, Err: err}
	}
	return out, nil
}

// CommandError wraps a failed command with the last line it printed.
type CommandError struct {
	Cmd string
	Msg string
	Err error
}

func (e *CommandError) Error() string {
	if e.Msg != "" {
		return e.Cmd + ": " + e.Msg
	}
	return e.Cmd + ": " + e.Err.Error()
}

func (e *CommandError) Unwrap() error { return e.Err }

func lastLine(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}
