package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/mydisha/lega/internal/platform"
	"github.com/mydisha/lega/internal/update"
)

// Seams for tests: the binary to replace and where dpkg lists its files.
var (
	executable = os.Executable
	dpkgInfo   = "/var/lib/dpkg/info"
)

// currentVersion is the release version, or the module version for
// "go install" builds.
func currentVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return strings.TrimPrefix(bi.Main.Version, "v")
	}
	return version
}

// selfPath returns the running binary with symlinks resolved.
func selfPath() (string, error) {
	exe, err := executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func selfUpdate(ctx context.Context, check bool, stdout io.Writer) error {
	cur := currentVersion()
	u := update.New(cur)
	tag, err := u.Latest(ctx)
	if err != nil {
		return err
	}
	latest := strings.TrimPrefix(tag, "v")
	if !update.Newer(tag, cur) {
		fmt.Fprintf(stdout, "lega %s is the latest version.\n", cur)
		return nil
	}
	if check {
		fmt.Fprintf(stdout, "lega %s is available (you have %s). Run: lega update\n", latest, cur)
		return nil
	}

	exe, err := selfPath()
	if err != nil {
		return err
	}
	if err := update.CheckReplace(exe, dpkgInfo); err != nil {
		if errors.Is(err, update.ErrPackaged) {
			return fmt.Errorf("%w: download the new .deb from %s", err, update.DefaultReleases)
		}
		return err
	}
	fmt.Fprintf(stdout, "Downloading lega %s...\n", latest)
	bin, err := u.Download(ctx, tag)
	if err != nil {
		return err
	}
	if err := update.Replace(exe, bin); err != nil {
		return fmt.Errorf("installing %s: %w", exe, err)
	}
	fmt.Fprintf(stdout, "Updated lega %s → %s (%s)\n", cur, latest, exe)
	return nil
}

// selfRemove deletes the lega binary, its settings and its log. confirm is
// nil when there is no terminal to ask on.
func selfRemove(env platform.Env, root, yes bool, confirm io.Reader, stdout io.Writer) error {
	exe, err := selfPath()
	if err != nil {
		return err
	}
	if err := update.CheckReplace(exe, dpkgInfo); err != nil {
		if errors.Is(err, update.ErrPackaged) {
			return fmt.Errorf("%w: sudo apt remove lega", err)
		}
		return err
	}

	targets := []string{exe}
	// Under sudo, HOME belongs to root, so the user's own folders are not ours to find.
	if !root {
		for _, dir := range []string{filepath.Join(env.ConfigDir, "lega"), filepath.Join(env.StateDir, "lega")} {
			if _, err := os.Lstat(dir); err == nil {
				targets = append(targets, dir)
			}
		}
	}

	fmt.Fprintln(stdout, "This removes:")
	for _, t := range targets {
		fmt.Fprintln(stdout, "  "+t)
	}
	if !yes {
		if confirm == nil {
			return usageError{"no terminal to confirm on; add --yes"}
		}
		fmt.Fprint(stdout, "Remove lega? [y/N] ")
		line, _ := bufio.NewReader(confirm).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			fmt.Fprintln(stdout, "Nothing was removed.")
			return nil
		}
	}

	// The binary goes last so a failure leaves a working lega behind.
	for i := len(targets) - 1; i >= 0; i-- {
		if err := os.RemoveAll(targets[i]); err != nil {
			return err
		}
	}
	fmt.Fprintln(stdout, "lega was removed.")
	if root {
		fmt.Fprintln(stdout, "Your settings in ~/.config/lega and log in ~/.local/state/lega were kept.")
	}
	return nil
}
