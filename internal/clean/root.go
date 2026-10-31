package clean

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mydisha/lega/internal/oplog"
	"github.com/mydisha/lega/internal/platform"
	"github.com/mydisha/lega/internal/safety"
)

// RootCommand is the hidden subcommand that runs root rules under sudo.
const RootCommand = "_root-clean"

// ErrNotRoot is returned when the root helper runs without root.
var ErrNotRoot = errors.New("this command must run as root")

// rootResult is what the helper sends back for one rule.
type rootResult struct {
	ID       string    `json:"id"`
	Removals []Removal `json:"removals"`
}

// ErrSudoNotCached means sudo took the password but did not keep it, as
// with "timestamp_timeout=0" in sudoers. The helper then has to run through
// RootHelper, so sudo can ask again on the terminal.
var ErrSudoNotCached = errors.New("sudo did not keep the password")

var (
	errNoSudo     = errors.New("sudo is not installed")
	errNoPassword = errors.New("sudo did not get a password")
	errNoTerminal = errors.New("sudo needs a password and there is no terminal to ask on")
)

// canPrompt reports whether sudo has a terminal to ask for a password on.
// sudo reads the password from /dev/tty, not from stdin.
var canPrompt = func() bool {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// SudoCall is a sudo command that runs on the terminal, so sudo can ask for
// the password. What sudo prints on stderr still reaches the terminal and
// is also kept, so a failure can be explained after the screen is redrawn.
type SudoCall struct {
	Cmd    *exec.Cmd
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func newSudoCall(args ...string) *SudoCall {
	s := &SudoCall{Cmd: exec.Command("sudo", args...)}
	s.Cmd.Stderr = io.MultiWriter(os.Stderr, &s.stderr)
	return s
}

// SudoValidate asks for the sudo password up front, so the terminal can be
// handed to sudo before the actual cleaning starts.
func SudoValidate() *SudoCall {
	return newSudoCall("-v", "-p", "[lega] sudo password for %u: ")
}

// RootHelper runs the root helper with sudo free to ask for the password,
// for when CleanAsRoot returns ErrSudoNotCached. Its output is collected;
// read it with Reports once the command has run.
func RootHelper(findings []Finding) (*SudoCall, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := []string{"-p", "[lega] sudo did not keep the password; enter it again for %u: ", exe, RootCommand}
	s := newSudoCall(append(args, ruleIDs(findings)...)...)
	s.Cmd.Stdout = &s.stdout
	return s, nil
}

// Err explains why the call failed with runErr: sudo's own last message
// when it printed one ("3 incorrect password attempts", "not in the
// sudoers file"), otherwise the likely cause. It returns nil for nil.
func (s *SudoCall) Err(runErr error) error {
	switch {
	case runErr == nil:
		return nil
	case errors.Is(runErr, exec.ErrNotFound):
		return errNoSudo
	}
	if msg := lastLine(strings.TrimSpace(s.stderr.String())); msg != "" {
		return errors.New(msg)
	}
	return errNoPassword
}

// Reports reads the results of a RootHelper call that ended with runErr.
func (s *SudoCall) Reports(c *Context, runErr error) ([]Report, error) {
	if runErr != nil {
		return nil, s.Err(runErr)
	}
	return rootReports(c, s.stdout.Bytes())
}

// SudoReady makes sure root findings can be cleaned, before anything is
// removed: sudo is installed, and it either needs no password right now
// or there is a terminal for ask to run SudoValidate on.
func SudoReady(ctx context.Context, c *Context, ask func(*SudoCall) error) error {
	if _, err := c.LookPath("sudo"); err != nil {
		return errNoSudo
	}
	if _, err := c.Run(ctx, "sudo", "-n", "-v"); err == nil {
		return nil // no password needed, or one was given moments ago
	}
	if !canPrompt() {
		return errNoTerminal
	}
	s := SudoValidate()
	return s.Err(ask(s))
}

func ruleIDs(findings []Finding) []string {
	var ids []string
	for _, f := range findings {
		ids = append(ids, f.Rule.Info().ID)
	}
	return ids
}

// CleanAsRoot runs the given root findings through "sudo -n lega
// _root-clean <ids>". Only rule IDs cross the privilege boundary: the
// helper finds the targets again by itself. sudo must already be
// validated; when it kept no login, ErrSudoNotCached is returned and
// nothing has run.
func CleanAsRoot(ctx context.Context, c *Context, findings []Finding) ([]Report, error) {
	ids := ruleIDs(findings)
	if len(ids) == 0 {
		return nil, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if _, err := c.Run(ctx, "sudo", "-n", "-v"); err != nil {
		return nil, ErrSudoNotCached
	}
	out, err := c.Run(ctx, "sudo", append([]string{"-n", exe, RootCommand}, ids...)...)
	if err != nil {
		return nil, err
	}
	return rootReports(c, out)
}

// rootReports turns the helper's JSON into reports and logs every removal.
func rootReports(c *Context, out []byte) ([]Report, error) {
	var results []rootResult
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("read sudo helper output: %w", err)
	}
	var reps []Report
	for _, res := range results {
		rep := Report{RuleID: res.ID}
		for _, r := range res.Removals {
			rep.add(r)
			var err error
			if r.Err != "" {
				err = errors.New(r.Err)
			}
			c.Log.Write(oplog.Entry{Action: "clean", Rule: res.ID, Path: r.Path, Bytes: r.Bytes, Err: err})
		}
		reps = append(reps, rep)
	}
	return reps, nil
}

// RootMain is the body of "lega _root-clean". It accepts rule IDs only,
// rejects anything that is not a known root rule, and writes JSON results.
func RootMain(ctx context.Context, ids []string, out io.Writer) error {
	if !platform.IsRoot() {
		return ErrNotRoot
	}
	c, err := rootContext()
	if err != nil {
		return err
	}
	return runRoot(ctx, c, Lookup, ids, out)
}

func runRoot(ctx context.Context, c *Context, lookup func(string) (Rule, bool), ids []string, out io.Writer) error {
	var rules []Rule
	for _, id := range ids {
		r, ok := lookup(id)
		if !ok || !r.Info().NeedsRoot {
			return fmt.Errorf("unknown root rule %q", id)
		}
		rules = append(rules, r)
	}
	var results []rootResult
	for _, f := range Scan(ctx, c, Available(c, rules), nil) {
		results = append(results, rootResult{ID: f.Rule.Info().ID, Removals: f.Rule.Clean(ctx, c, f.Targets)})
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(results); err != nil {
		return err
	}
	_, err := out.Write(buf.Bytes())
	return err
}

// rootContext builds a Context for the helper. Protections and the
// whitelist come from the user who ran sudo, not from root.
func rootContext() (*Context, error) {
	env := platform.Env{Home: "/root", User: "root", UID: 0}
	if name := os.Getenv("SUDO_USER"); name != "" {
		if u, err := user.Lookup(name); err == nil {
			uid, _ := strconv.Atoi(u.Uid)
			env = platform.Env{Home: u.HomeDir, User: u.Username, UID: uid}
		}
	}
	env.CacheDir = filepath.Join(env.Home, ".cache")
	env.ConfigDir = filepath.Join(env.Home, ".config")
	env.DataDir = filepath.Join(env.Home, ".local/share")
	env.StateDir = filepath.Join(env.Home, ".local/state")

	wl, err := safety.LoadWhitelist(WhitelistPath(env), env.Home)
	if err != nil {
		return nil, err
	}
	var userDirs []string
	for _, d := range env.UserDirs() {
		userDirs = append(userDirs, d)
	}
	g := safety.NewGuard(env.Home, -1, userDirs)
	g.Whitelist = wl
	return &Context{
		Env:      env,
		Guard:    g,
		SysGuard: g,
		Run:      ExecRunner,
		LookPath: exec.LookPath,
		Now:      time.Now,
		Running:  map[string]bool{},
	}, nil
}
