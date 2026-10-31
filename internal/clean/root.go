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

// SudoValidate asks for the sudo password up front, so the terminal can be
// handed to sudo before the actual cleaning starts.
func SudoValidate() *exec.Cmd {
	return exec.Command("sudo", "-v", "-p", "[lega] sudo password for %u: ")
}

// CleanAsRoot runs the given root findings through "sudo -n lega
// _root-clean <ids>". Only rule IDs cross the privilege boundary: the
// helper finds the targets again by itself. sudo must already be validated.
func CleanAsRoot(ctx context.Context, c *Context, findings []Finding) ([]Report, error) {
	var ids []string
	for _, f := range findings {
		ids = append(ids, f.Rule.Info().ID)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	out, err := c.Run(ctx, "sudo", append([]string{"-n", exe, RootCommand}, ids...)...)
	if err != nil {
		return nil, err
	}
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
