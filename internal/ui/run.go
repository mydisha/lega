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

// screens maps a start name to the screen it opens.
var screens = map[string]func(*shared) screen{
	"status": newStatus,
}

// Run starts the interface on the named screen and blocks until it quits.
func Run(ctx context.Context, cfg Config, start string) error {
	first, ok := screens[start]
	if !ok {
		return fmt.Errorf("unknown screen %q", start)
	}
	sh := &shared{theme: NewTheme(true), env: cfg.Env, release: cfg.Release, ctx: ctx}
	_, err := tea.NewProgram(newApp(sh, first), tea.WithContext(ctx)).Run()
	return err
}
