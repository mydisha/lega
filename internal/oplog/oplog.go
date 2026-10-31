// Package oplog keeps a plain-text record of everything lega removed, so
// users can check what happened later.
package oplog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Entry is one removal.
type Entry struct {
	Time   time.Time
	Action string // clean, purge or trash
	Rule   string // rule ID, empty for manual actions
	Path   string // file path, or the command that ran
	Bytes  int64
	Err    error
}

// Log appends entries to a file. A nil *Log accepts writes and does
// nothing.
type Log struct {
	mu   sync.Mutex
	path string
}

// Open returns the log at stateDir/lega/operations.log. Setting
// LEGA_NO_OPLOG=1 turns logging off.
func Open(stateDir string) *Log {
	if os.Getenv("LEGA_NO_OPLOG") == "1" {
		return nil
	}
	return &Log{path: filepath.Join(stateDir, "lega", "operations.log")}
}

// Path returns where the log is written, or "" when logging is off.
func (l *Log) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Write appends one tab-separated line:
// time, action, rule, bytes, result, path.
func (l *Log) Write(e Entry) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	result := "ok"
	if e.Err != nil {
		result = "error: " + oneLine(e.Err.Error())
	}
	rule := e.Rule
	if rule == "" {
		rule = "-"
	}
	_, err = fmt.Fprintf(f, "%s\t%s\t%s\t%d\t%s\t%s\n",
		e.Time.Format(time.RFC3339), e.Action, rule, e.Bytes, result, oneLine(e.Path))
	return err
}

// oneLine keeps each entry on a single line even for odd file names.
func oneLine(s string) string {
	return strings.NewReplacer("\n", `\n`, "\t", `\t`, "\r", `\r`).Replace(s)
}
