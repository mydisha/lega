package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mydisha/lega/internal/ui"
)

// fakeHome points HOME and the XDG folders at a temp folder.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, k := range []string{"XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		t.Setenv(k, "")
	}
	t.Setenv("LEGA_NO_OPLOG", "")
	return home
}

func write(t *testing.T, path string, size int) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func lega(args ...string) (string, string, error) {
	var out, errOut bytes.Buffer
	err := run(context.Background(), args, &out, &errOut)
	return out.String(), errOut.String(), err
}

func TestHelpAndVersion(t *testing.T) {
	out, _, err := lega("--help")
	if err != nil || !strings.Contains(out, "lega clean") {
		t.Errorf("help: %v %q", err, out)
	}
	out, _, _ = lega("version")
	if out != "lega dev\n" {
		t.Errorf("version: %q", out)
	}
}

func TestUsageErrors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("lega refuses to run as root")
	}
	fakeHome(t)
	var u usageError
	if _, _, err := lega("explode"); !errors.As(err, &u) {
		t.Errorf("unknown command: %v", err)
	}
	if _, _, err := lega("clean", "--system"); !errors.As(err, &u) {
		t.Errorf("--system without --yes: %v", err)
	}
	if _, _, err := lega("clean", "--nope"); err == nil || errors.Is(err, flag.ErrHelp) {
		t.Errorf("unknown flag: %v", err)
	}
	// Tests have no terminal, so the interface cannot start.
	if _, _, err := lega("status"); !errors.As(err, &u) || !strings.Contains(err.Error(), "no terminal") {
		t.Errorf("no tty: %v", err)
	}
}

func TestParseFlagsAfterArgs(t *testing.T) {
	o, err := parse("analyze", []string{"/tmp/x", "--json"}, &bytes.Buffer{})
	if err != nil || !o.json || len(o.args) != 1 || o.args[0] != "/tmp/x" {
		t.Errorf("got %+v, %v", o, err)
	}
	o, err = parse("purge", []string{"--dry-run", "a", "b"}, &bytes.Buffer{})
	if err != nil || !o.dryRun || len(o.args) != 2 {
		t.Errorf("got %+v, %v", o, err)
	}
}

func TestAnalyzeJSON(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("lega refuses to run as root")
	}
	home := fakeHome(t)
	write(t, filepath.Join(home, "big", "a"), 300_000)
	write(t, filepath.Join(home, "small"), 1_000)
	out, _, err := lega("analyze", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got jsonFolder
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if got.Path != home || len(got.Children) != 2 || got.Children[0].Name != "big" || !got.Children[0].Dir {
		t.Errorf("got %+v", got)
	}
}

func TestPurgePlain(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("lega refuses to run as root")
	}
	home := fakeHome(t)
	proj := filepath.Join(home, "Projects", "app")
	write(t, filepath.Join(proj, "package.json"), 10)
	write(t, filepath.Join(proj, "node_modules", "x", "i.js"), 50_000)
	write(t, filepath.Join(home, "Projects", "fresh", "package.json"), 10)
	write(t, filepath.Join(home, "Projects", "fresh", "node_modules", "y"), 50_000)
	past := time.Now().Add(-30 * 24 * time.Hour)
	filepath.Walk(filepath.Join(proj, "node_modules"), func(p string, _ os.FileInfo, _ error) error {
		return os.Chtimes(p, past, past)
	})

	out, _, err := lega("purge", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, filepath.Join(proj, "node_modules")) || !strings.Contains(out, "skipped") || !strings.Contains(out, "Dry run") {
		t.Errorf("dry run output:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(proj, "node_modules")); err != nil {
		t.Fatal("dry run removed files")
	}

	out, _, err = lega("purge", "--yes")
	if err != nil || !strings.Contains(out, "Freed") {
		t.Fatalf("purge --yes: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(proj, "node_modules")); !os.IsNotExist(err) {
		t.Error("old node_modules should be removed")
	}
	if _, err := os.Stat(filepath.Join(home, "Projects", "fresh", "node_modules")); err != nil {
		t.Error("recent node_modules must stay")
	}
	if b, _ := os.ReadFile(filepath.Join(home, ".local/state/lega/operations.log")); !strings.Contains(string(b), "purge") {
		t.Error("purge not logged")
	}
}

func TestBadWhitelistStopsPurge(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("lega refuses to run as root")
	}
	home := fakeHome(t)
	write(t, filepath.Join(home, ".config", "lega", "whitelist"), 0)
	os.WriteFile(filepath.Join(home, ".config", "lega", "whitelist"), []byte("not/absolute\n"), 0o644)
	os.Mkdir(filepath.Join(home, "code"), 0o755)
	if _, _, err := lega("purge", "--yes"); err == nil || !strings.Contains(err.Error(), "whitelist") {
		t.Errorf("want whitelist error, got %v", err)
	}
}

func TestIconsFlag(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("lega refuses to run as root")
	}
	fakeHome(t)
	var u usageError
	// "lega --icons" opens the menu (and stops at the missing terminal)
	// instead of failing as an unknown command.
	for _, args := range [][]string{{"--icons"}, {"analyze", "--icons"}, {"purge", "/tmp", "--icons"}} {
		if _, _, err := lega(args...); !errors.As(err, &u) || !strings.Contains(err.Error(), "no terminal") {
			t.Errorf("%v: %v", args, err)
		}
	}
	if _, _, err := lega("--nope"); err == nil || errors.As(err, &u) && strings.Contains(err.Error(), "no terminal") {
		t.Errorf("unknown flag before a command: %v", err)
	}
	if out, _, err := lega("-v"); err != nil || out != "lega dev\n" {
		t.Errorf("-v: %q %v", out, err)
	}

	o, err := parse("clean", []string{"--dry-run", "--icons"}, &bytes.Buffer{})
	if err != nil || !o.icons || !o.dryRun {
		t.Errorf("got %+v, %v", o, err)
	}

	// update and remove draw no interface, so --icons means nothing there.
	for _, cmd := range []string{"update", "remove"} {
		if _, err := parse(cmd, []string{"--icons"}, &bytes.Buffer{}); err == nil {
			t.Errorf("%s --icons should be refused", cmd)
		}
	}

	o, err = parse("menu", []string{"--no-icons"}, &bytes.Buffer{})
	if err != nil || !o.noIcons {
		t.Errorf("got %+v, %v", o, err)
	}
}

func TestPickIcons(t *testing.T) {
	cases := []struct {
		name    string
		o       options
		env     map[string]string
		font    bool
		want    ui.IconSet
		askFont bool // the fontconfig check is needed for the answer
	}{
		{"nerd font installed", options{}, map[string]string{"TERM": "xterm-256color"}, true, ui.IconsNerd, true},
		{"no nerd font falls back to emoji", options{}, map[string]string{"TERM": "xterm-256color"}, false, ui.IconsEmoji, true},
		{"ghostty ships the symbols", options{}, map[string]string{"TERM": "xterm-ghostty"}, false, ui.IconsNerd, false},
		{"wezterm ships the symbols", options{}, map[string]string{"TERM_PROGRAM": "WezTerm"}, false, ui.IconsNerd, false},
		{"linux console has neither", options{}, map[string]string{"TERM": "linux"}, true, ui.IconsOff, false},
		{"--icons on the console", options{icons: true}, map[string]string{"TERM": "linux"}, false, ui.IconsEmoji, true},
		{"--no-icons", options{noIcons: true}, map[string]string{"LEGA_ICONS": "nerd"}, true, ui.IconsOff, false},
		{"--no-icons beats --icons", options{icons: true, noIcons: true}, nil, true, ui.IconsOff, false},
		{"LEGA_ICONS=nerd", options{}, map[string]string{"LEGA_ICONS": " NERD "}, false, ui.IconsNerd, false},
		{"LEGA_ICONS=emoji", options{}, map[string]string{"LEGA_ICONS": "emoji", "TERM": "xterm-ghostty"}, true, ui.IconsEmoji, false},
		{"LEGA_ICONS=off", options{}, map[string]string{"LEGA_ICONS": "off"}, true, ui.IconsOff, false},
		{"--icons beats LEGA_ICONS=off", options{icons: true}, map[string]string{"LEGA_ICONS": "off"}, true, ui.IconsNerd, true},
		{"unknown LEGA_ICONS is auto", options{}, map[string]string{"LEGA_ICONS": "ikon"}, false, ui.IconsEmoji, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			asked := false
			got := pickIcons(c.o, func(k string) string { return c.env[k] }, func() bool { asked = true; return c.font })
			if got != c.want {
				t.Errorf("icons = %v, want %v", got, c.want)
			}
			if asked && !c.askFont {
				t.Error("fontconfig was asked although the answer did not need it")
			}
		})
	}
}
