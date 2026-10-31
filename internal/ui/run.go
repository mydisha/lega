package ui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/mydisha/lega/internal/platform"
)

// Config is what the interface needs from the command line.
type Config struct {
	Env     platform.Env
	Release platform.OSRelease
}

// Run starts the interface on the named screen (menu, status, analyze,
// clean or purge) and blocks until it quits. args are the command's arguments, such
// as the folder to analyze.
func Run(ctx context.Context, cfg Config, start string, args []string) error {
	var first func(*shared) screen
	switch start {
	case "menu":
		first = newMenu
	case "status":
		first = newStatus
	case "analyze":
		root := ""
		if len(args) > 0 {
			root = args[0]
		}
		first = newAnalyze(root)
	case "clean":
		first = newCleaner
	case "purge":
		first = newPurger(args)
	default:
		return fmt.Errorf("unknown screen %q", start)
	}
	sh := &shared{theme: NewTheme(true), env: cfg.Env, release: cfg.Release, ctx: ctx}
	_, err := tea.NewProgram(newApp(sh, first), tea.WithContext(ctx)).Run()
	return err
}
