package clean

import (
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mydisha/lega/internal/oplog"
	"github.com/mydisha/lega/internal/platform"
	"github.com/mydisha/lega/internal/safety"
)

// Rules returns every rule lega knows. Specific rules come first and
// catch-all rules last, so overlapping paths go to the specific one.
func Rules() []Rule {
	var all []Rule
	all = append(all, systemRules()...)
	all = append(all, generalRules()...)
	return all
}

// Lookup finds a rule by ID.
func Lookup(id string) (Rule, bool) {
	for _, r := range Rules() {
		if r.Info().ID == id {
			return r, true
		}
	}
	return nil, false
}

// WhitelistPath is where users list paths lega must keep.
func WhitelistPath(env platform.Env) string {
	return filepath.Join(env.ConfigDir, "lega", "whitelist")
}

// NewContext builds a Context for the real system.
func NewContext(env platform.Env, whitelist []string) *Context {
	var userDirs []string
	for _, d := range env.UserDirs() {
		userDirs = append(userDirs, d)
	}
	g := safety.NewGuard(env.Home, env.UID, userDirs)
	g.Whitelist = whitelist
	sg := safety.NewGuard(env.Home, -1, userDirs)
	sg.Whitelist = whitelist
	running, _ := platform.Processes("/proc", env.UID)
	return &Context{
		Env:      env,
		Guard:    g,
		SysGuard: sg,
		Log:      oplog.Open(env.StateDir),
		Run:      ExecRunner,
		LookPath: exec.LookPath,
		Now:      time.Now,
		Running:  running,
	}
}
