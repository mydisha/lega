package clean

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const snapListOutput = `Name               Version          Rev    Tracking         Publisher   Notes
core22             20240111         1122   latest/stable    canonical✓  base,disabled
core22             20240408         1380   latest/stable    canonical✓  base
firefox            125.0-2          4173   latest/stable/…  mozilla✓    disabled
firefox            126.0-1          4259   latest/stable/…  mozilla✓    -
Bad_Name           1                10     latest/stable    x           disabled
weird              1                ../1   latest/stable    x           disabled
`

func TestParseDisabledSnaps(t *testing.T) {
	got := parseDisabledSnaps(snapListOutput)
	want := [][2]string{{"core22", "1122"}, {"firefox", "4173"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSnapFromPath(t *testing.T) {
	cases := map[string]bool{
		"/var/lib/snapd/snaps/core22_1122.snap":         true,
		"/var/lib/snapd/snaps/gnome-42-2204_176.snap":   true,
		"/var/lib/snapd/snaps/../x_1.snap":              false,
		"/tmp/core22_1122.snap":                         false,
		"/var/lib/snapd/snaps/noversion.snap":           false,
		"/var/lib/snapd/snaps/core22_--revision=1.snap": false,
	}
	for p, ok := range cases {
		if _, _, got := snapFromPath(p); got != ok {
			t.Errorf("snapFromPath(%q) ok = %v, want %v", p, got, ok)
		}
	}
}

func TestSnapRevisionsClean(t *testing.T) {
	c := testContext(t)
	var calls [][]string
	c.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		if len(args) > 0 && args[0] == "list" {
			return []byte(snapListOutput), nil
		}
		return nil, nil
	}
	r := &snapRevisions{}
	ts, _ := r.Scan(context.Background(), c)
	if len(ts) != 2 {
		t.Fatalf("targets %+v", ts)
	}
	calls = nil
	rms := r.Clean(context.Background(), c, append(ts, Target{Path: "/etc/passwd"}))
	want := [][]string{
		{"snap", "remove", "core22", "--revision=1122"},
		{"snap", "remove", "firefox", "--revision=4173"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls %v", calls)
	}
	if rms[2].Err == "" {
		t.Error("a path that is not a snap must fail without running anything")
	}
}

func TestAutoremovePackages(t *testing.T) {
	c := testContext(t)
	c.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "apt-get":
			return []byte("NOTE: simulation\nPurg linux-image-6.8.0-31-generic [6.8.0-31.31]\nPurg libfoo1 [1.0]\nConf x\n"), nil
		case "dpkg-query":
			if !strings.Contains(strings.Join(args, " "), "linux-image-6.8.0-31-generic libfoo1") {
				t.Errorf("dpkg-query args %v", args)
			}
			return []byte("linux-image-6.8.0-31-generic\t14000\nlibfoo1\t200\n"), nil
		}
		return nil, nil
	}
	ts, err := autoremovePackages(context.Background(), c)
	if err != nil || len(ts) != 2 || ts[0].Bytes != 14000*1024 || ts[1].Path != "libfoo1" {
		t.Errorf("got %+v, %v", ts, err)
	}

	c.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("0 upgraded, 0 newly installed, 0 to remove\n"), nil
	}
	if ts, _ := autoremovePackages(context.Background(), c); ts != nil {
		t.Errorf("nothing to remove: %+v", ts)
	}
}

func TestOldJournals(t *testing.T) {
	c := testContext(t)
	old := journalDir
	t.Cleanup(func() { journalDir = old })
	journalDir = filepath.Join(t.TempDir(), "journal")

	archived := filepath.Join(journalDir, "abc", "system@0001-0002.journal")
	recent := filepath.Join(journalDir, "abc", "user-1000@0003-0004.journal")
	active := filepath.Join(journalDir, "abc", "system.journal")
	for _, p := range []string{archived, recent, active} {
		write(t, p, 4000)
	}
	past := time.Now().Add(-30 * 24 * time.Hour)
	os.Chtimes(archived, past, past)

	ts, _ := oldJournals(context.Background(), c)
	if len(ts) != 1 || ts[0].Path != archived {
		t.Errorf("want only the old archived journal, got %+v", ts)
	}

	// Unreadable journal folder: fall back to journalctl's total.
	journalDir = filepath.Join(t.TempDir(), "missing")
	c.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("Archived and active journals take up 2.2G in the file system.\n"), nil
	}
	ts, _ = oldJournals(context.Background(), c)
	if len(ts) != 1 || !ts[0].Unknown || ts[0].Bytes != 2_200_000_000 {
		t.Errorf("fallback: %+v", ts)
	}
}

func TestHideSize(t *testing.T) {
	c := testContext(t)
	dir := filepath.Join(c.Env.Home, "runtimes")
	r := &commandRule{info: Info{ID: "x", Name: "X"}, measure: []string{dir}, cmd: []string{"true"}, hideSize: true}
	if ts, _ := r.Scan(context.Background(), c); ts != nil {
		t.Errorf("nothing measured, nothing to show: %+v", ts)
	}
	write(t, filepath.Join(dir, "a"), 5000)
	ts, _ := r.Scan(context.Background(), c)
	if len(ts) != 1 || !ts[0].Unknown || ts[0].Bytes != 0 {
		t.Errorf("want one unknown-size target, got %+v", ts)
	}
}

func TestRunRoot(t *testing.T) {
	c := testContext(t)
	logs := filepath.Join(c.Env.Home, "fakelog")
	write(t, filepath.Join(logs, "syslog.1"), 3000)
	write(t, filepath.Join(logs, "syslog"), 3000)
	fake := &pathRule{
		info:  Info{ID: "fake-logs", Name: "Fake", NeedsRoot: true},
		spots: []spot{{logs, []string{"*.[0-9]"}}},
	}
	user := &pathRule{info: Info{ID: "user-rule"}}
	lookup := func(id string) (Rule, bool) {
		switch id {
		case "fake-logs":
			return fake, true
		case "user-rule":
			return user, true
		}
		return nil, false
	}

	var out bytes.Buffer
	if err := runRoot(context.Background(), c, lookup, []string{"nope"}, &out); err == nil {
		t.Error("unknown ID must be rejected")
	}
	if err := runRoot(context.Background(), c, lookup, []string{"user-rule"}, &out); err == nil {
		t.Error("a rule that does not need root must be rejected")
	}
	if err := runRoot(context.Background(), c, lookup, []string{"fake-logs"}, &out); err != nil {
		t.Fatal(err)
	}
	var res []rootResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || len(res[0].Removals) != 1 || res[0].Removals[0].Err != "" {
		t.Errorf("results %+v", res)
	}
	if exists(filepath.Join(logs, "syslog.1")) || !exists(filepath.Join(logs, "syslog")) {
		t.Error("wrong files removed")
	}
}

func TestCleanAsRoot(t *testing.T) {
	c := testContext(t)
	var calls [][]string
	c.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return []byte(`[{"id":"apt-cache","removals":[{"path":"apt-get clean","bytes":5000}]},` +
			`{"id":"journal","removals":[{"path":"journalctl --vacuum-time=14d","bytes":0,"err":"failed"}]}]`), nil
	}
	apt, _ := Lookup("apt-cache")
	journal, _ := Lookup("journal")
	reps, err := CleanAsRoot(context.Background(), c, []Finding{{Rule: apt}, {Rule: journal}})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || !reflect.DeepEqual(calls[0], []string{"sudo", "-n", "-v"}) {
		t.Fatalf("calls %v, want a sudo -n -v check then the helper", calls)
	}
	argv := calls[1]
	if argv[0] != "sudo" || argv[1] != "-n" || argv[3] != RootCommand || argv[4] != "apt-cache" || argv[5] != "journal" {
		t.Errorf("argv %v", argv)
	}
	if len(reps) != 2 || reps[0].Freed != 5000 || len(reps[1].Failures) != 1 {
		t.Errorf("reports %+v", reps)
	}
	if b, _ := os.ReadFile(c.Log.Path()); strings.Count(string(b), "\n") != 2 {
		t.Errorf("oplog should have 2 lines: %q", b)
	}

	if reps, err := CleanAsRoot(context.Background(), c, nil); reps != nil || err != nil {
		t.Error("no findings, no sudo call")
	}
}

func TestRootMainNeedsRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}
	if err := RootMain(context.Background(), []string{"apt-cache"}, &bytes.Buffer{}); err != ErrNotRoot {
		t.Errorf("got %v", err)
	}
}
