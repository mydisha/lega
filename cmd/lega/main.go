// Command lega frees up disk space on Ubuntu and its derivatives.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/charmbracelet/x/term"

	"github.com/mydisha/lega/internal/clean"
	"github.com/mydisha/lega/internal/platform"
	"github.com/mydisha/lega/internal/ui"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `lega frees up disk space on Ubuntu, Linux Mint, Zorin OS, Pop!_OS and
other Ubuntu-based systems.

Usage:
  lega                     open the interactive menu
  lega status [--json]     show disks and what fills your home folder
  lega clean [flags]       remove caches, logs and other junk
  lega analyze [path]      explore a folder by size (default: home)
  lega purge [paths...]    remove build folders (node_modules, target, ...)
  lega update [--check]    install the latest release
  lega remove [--yes]      uninstall lega, its settings and its log
  lega version             print the version

Flags for clean and purge:
  --dry-run   list what would be removed, remove nothing
  --yes       remove the safe, preselected items without asking
  --system    with clean --yes, also clean system items (asks for sudo)

Flags for status and analyze:
  --json      print JSON instead of opening the interface

Flags for the interface (menu, status, clean, analyze, purge):
  --icons     show icons next to files, folders and apps; needs a Nerd Font
              (https://www.nerdfonts.com). LEGA_ICONS=nerd does the same.

Keep files safe from clean and purge by listing them, one per line,
in ~/.config/lega/whitelist. Every removal is logged in
~/.local/state/lega/operations.log.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "lega:", err)
		var u usageError
		if errors.As(err, &u) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

// usageError marks mistakes in the command line.
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg + " (see lega --help)" }

// options are the flags shared by all commands.
type options struct {
	json, dryRun, yes, system, check, icons bool
	args                                    []string
}

func parse(name string, args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	switch name {
	case "menu", "status", "clean", "analyze", "purge":
		fs.BoolVar(&o.icons, "icons", false, "show Nerd Font icons")
	}
	switch name {
	case "status", "analyze":
		fs.BoolVar(&o.json, "json", false, "print JSON")
	case "clean", "purge":
		fs.BoolVar(&o.dryRun, "dry-run", false, "list only")
		fs.BoolVar(&o.yes, "yes", false, "do not ask")
		if name == "clean" {
			fs.BoolVar(&o.system, "system", false, "include system items")
		}
	case "update":
		fs.BoolVar(&o.check, "check", false, "only check")
	case "remove":
		fs.BoolVar(&o.yes, "yes", false, "do not ask")
	}
	// Allow flags after positional arguments: lega analyze ~/x --json.
	var rest []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return o, err
		}
		args = fs.Args()
		if len(args) > 0 {
			rest = append(rest, args[0])
			args = args[1:]
		}
	}
	o.args = rest
	if o.system && !o.yes {
		return o, usageError{"--system only works together with --yes"}
	}
	return o, nil
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cmd := "menu"
	// "lega --icons" opens the menu; other words and -h/-v are commands.
	if len(args) > 0 && (!strings.HasPrefix(args[0], "-") || isCommandFlag(args[0])) {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return nil
	case "-v", "--version", "version":
		fmt.Fprintln(stdout, "lega", currentVersion())
		return nil
	case clean.RootCommand:
		return clean.RootMain(ctx, args, stdout)
	case "update", "remove":
		return runSelf(ctx, cmd, args, stdout, stderr)
	case "menu", "status", "clean", "analyze", "purge":
	default:
		return usageError{fmt.Sprintf("unknown command %q", cmd)}
	}

	if platform.IsRoot() {
		return errors.New("run lega as your normal user, not with sudo; it asks for sudo itself when it needs it")
	}
	o, err := parse(cmd, args, stderr)
	if err != nil {
		return err
	}
	env, err := platform.CurrentEnv()
	if err != nil {
		return err
	}
	rel, _ := platform.ReadOSRelease()
	if !rel.Supported() {
		fmt.Fprintf(stderr, "lega: %s is not Ubuntu-based; system cleaning may not apply.\n", rel.DisplayName())
	}

	switch {
	case o.json && cmd == "status":
		return printStatusJSON(ctx, env, rel, stdout)
	case o.json && cmd == "analyze":
		return printAnalyzeJSON(ctx, env, o.args, stdout)
	case o.dryRun || o.yes:
		if cmd == "clean" {
			return plainClean(ctx, env, o, stdout)
		}
		return plainPurge(ctx, env, o, stdout)
	}

	if !term.IsTerminal(os.Stdout.Fd()) || !term.IsTerminal(os.Stdin.Fd()) {
		return usageError{"no terminal to draw on; use --json, --dry-run or --yes"}
	}
	return ui.Run(ctx, ui.Config{Env: env, Release: rel, Icons: wantIcons(o.icons)}, cmd, o.args)
}

// wantIcons reports whether to draw Nerd Font icons: with --icons, or when
// LEGA_ICONS=nerd is set.
func wantIcons(flagSet bool) bool {
	return flagSet || os.Getenv("LEGA_ICONS") == "nerd"
}

// isCommandFlag reports whether arg is a flag that works as a command.
func isCommandFlag(arg string) bool {
	switch arg {
	case "-h", "--help", "-v", "--version":
		return true
	}
	return false
}

// runSelf handles update and remove. Both may run under sudo, for a lega
// installed in a folder the user cannot write to.
func runSelf(ctx context.Context, cmd string, args []string, stdout, stderr io.Writer) error {
	o, err := parse(cmd, args, stderr)
	if err != nil {
		return err
	}
	if len(o.args) > 0 {
		return usageError{fmt.Sprintf("%s takes no arguments", cmd)}
	}
	if cmd == "update" {
		return selfUpdate(ctx, o.check, stdout)
	}
	env, err := platform.CurrentEnv()
	if err != nil {
		return err
	}
	var confirm io.Reader
	if term.IsTerminal(os.Stdin.Fd()) {
		confirm = os.Stdin
	}
	return selfRemove(env, platform.IsRoot(), o.yes, confirm, stdout)
}
