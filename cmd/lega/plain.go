package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mydisha/lega/internal/clean"
	"github.com/mydisha/lega/internal/disk"
	"github.com/mydisha/lega/internal/format"
	"github.com/mydisha/lega/internal/oplog"
	"github.com/mydisha/lega/internal/platform"
	"github.com/mydisha/lega/internal/purge"
	"github.com/mydisha/lega/internal/safety"
	"github.com/mydisha/lega/internal/scan"
)

// This file holds the output for scripts and pipes: JSON for status and
// analyze, and plain lists for clean and purge.

type jsonEntry struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Files int64  `json:"files"`
	Dir   bool   `json:"dir"`
}

type jsonFolder struct {
	Path     string      `json:"path"`
	Bytes    int64       `json:"bytes"`
	Files    int64       `json:"files"`
	Children []jsonEntry `json:"children"`
}

func folderJSON(n *scan.Node) jsonFolder {
	f := jsonFolder{Path: n.Path(), Bytes: n.Size, Files: n.Files, Children: []jsonEntry{}}
	for _, c := range n.Sorted(scan.BySize) {
		f.Children = append(f.Children, jsonEntry{c.Name, c.Size, c.Files, c.IsDir})
	}
	return f
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printStatusJSON(ctx context.Context, env platform.Env, rel platform.OSRelease, w io.Writer) error {
	mounts, err := disk.List()
	if err != nil {
		return err
	}
	type jsonDisk struct {
		MountPoint string `json:"mount_point"`
		Device     string `json:"device"`
		FSType     string `json:"fs_type"`
		Total      int64  `json:"total_bytes"`
		Used       int64  `json:"used_bytes"`
		Free       int64  `json:"free_bytes"`
	}
	out := struct {
		System string     `json:"system"`
		Disks  []jsonDisk `json:"disks"`
		Home   jsonFolder `json:"home"`
	}{System: rel.DisplayName(), Disks: []jsonDisk{}}
	for _, m := range mounts {
		out.Disks = append(out.Disks, jsonDisk{m.MountPoint, m.Device, m.FSType, m.Total, m.Used, m.Free})
	}
	var s scan.Scanner
	home, err := s.Scan(ctx, env.Home)
	if err != nil {
		return err
	}
	out.Home = folderJSON(home)
	return writeJSON(w, out)
}

func printAnalyzeJSON(ctx context.Context, env platform.Env, args []string, w io.Writer) error {
	root := env.Home
	if len(args) > 0 {
		root = args[0]
	}
	var s scan.Scanner
	n, err := s.Scan(ctx, root)
	if err != nil {
		return err
	}
	return writeJSON(w, folderJSON(n))
}

func loadWhitelist(env platform.Env) ([]string, error) {
	wl, err := safety.LoadWhitelist(clean.WhitelistPath(env), env.Home)
	if err != nil {
		return nil, fmt.Errorf("%w; fix or remove %s, then try again", err, clean.WhitelistPath(env))
	}
	return wl, nil
}

// plainClean lists findings (--dry-run) or cleans the safe ones (--yes).
func plainClean(ctx context.Context, env platform.Env, o options, w io.Writer) error {
	wl, err := loadWhitelist(env)
	if err != nil {
		return err
	}
	c := clean.NewContext(env, wl)
	found := clean.Scan(ctx, c, clean.Available(c, clean.Rules()), nil)

	var picked, rootPicked []clean.Finding
	var total int64
	for _, f := range found {
		info := f.Rule.Info()
		note := ""
		switch {
		case f.Err != nil:
			note = "error: " + f.Err.Error()
		case f.Locked != "":
			note = f.Locked
		case info.Risk == clean.Review:
			note = "review, not selected"
		case info.NeedsRoot && !o.system:
			note = "needs --system"
		}
		size := format.Size(f.Bytes)
		switch {
		case f.Unknown && f.Bytes == 0:
			size = "size unknown"
		case f.Unknown:
			size = "up to " + size
		}
		if note != "" {
			note = "  (" + note + ")"
		}
		fmt.Fprintf(w, "%14s  %s%s\n", size, info.Name, note)
		if note != "" {
			continue
		}
		total += f.Bytes
		if info.NeedsRoot {
			rootPicked = append(rootPicked, f)
		} else {
			picked = append(picked, f)
		}
	}
	fmt.Fprintf(w, "\nSelected: %s\n", format.Size(total))
	if o.dryRun {
		fmt.Fprintln(w, "Dry run: nothing was removed.")
		return nil
	}

	// Settle sudo before anything is removed, so a missing password does
	// not leave the clean half done.
	if len(rootPicked) > 0 {
		err := clean.SudoReady(ctx, c, func(s *clean.SudoCall) error {
			s.Cmd.Stdin, s.Cmd.Stdout = os.Stdin, os.Stdout
			return s.Cmd.Run()
		})
		if err != nil {
			return fmt.Errorf("--system: %w; nothing was removed", err)
		}
	}

	var freed int64
	var failed int
	for _, f := range picked {
		rep := clean.Clean(ctx, c, f)
		freed += rep.Freed
		failed += len(rep.Failures)
	}
	if len(rootPicked) > 0 {
		reps, err := cleanRoot(ctx, c, rootPicked)
		if err != nil {
			return fmt.Errorf("system items: %w", err)
		}
		for _, rep := range reps {
			freed += rep.Freed
			failed += len(rep.Failures)
		}
	}
	fmt.Fprintf(w, "Freed %s.", format.Size(freed))
	if failed > 0 {
		fmt.Fprintf(w, " %d items could not be removed; see %s.", failed, c.Log.Path())
	}
	fmt.Fprintln(w)
	return nil
}

// cleanRoot runs the root findings through sudo. When sudo kept no login
// after SudoReady, the helper runs on the terminal so sudo can ask again.
func cleanRoot(ctx context.Context, c *clean.Context, found []clean.Finding) ([]clean.Report, error) {
	reps, err := clean.CleanAsRoot(ctx, c, found)
	if !errors.Is(err, clean.ErrSudoNotCached) {
		return reps, err
	}
	h, err := clean.RootHelper(found)
	if err != nil {
		return nil, err
	}
	h.Cmd.Stdin = os.Stdin
	return h.Reports(c, h.Cmd.Run())
}

// plainPurge lists artifacts (--dry-run) or removes the ones not used in
// the last week (--yes).
func plainPurge(ctx context.Context, env platform.Env, o options, w io.Writer) error {
	wl, err := loadWhitelist(env)
	if err != nil {
		return err
	}
	var userDirs []string
	for _, d := range env.UserDirs() {
		userDirs = append(userDirs, d)
	}
	g := safety.NewGuard(env.Home, env.UID, userDirs)
	g.Whitelist = wl
	roots := o.args
	if len(roots) == 0 {
		roots = purge.DefaultRoots(env.Home)
	}
	if len(roots) == 0 {
		return usageError{"no project folders found in your home; pass one: lega purge <folder>"}
	}
	found, err := purge.NewFinder(g).Find(ctx, roots)
	if err != nil {
		return err
	}

	var total int64
	for _, a := range found {
		note := ""
		if a.Recent {
			note = "  (used in the last 7 days, skipped)"
		} else {
			total += a.Bytes
		}
		fmt.Fprintf(w, "%10s  %s%s\n", format.Size(a.Bytes), a.Path, note)
	}
	fmt.Fprintf(w, "\nSelected: %s\n", format.Size(total))
	if o.dryRun {
		fmt.Fprintln(w, "Dry run: nothing was removed.")
		return nil
	}
	log := oplog.Open(env.StateDir)
	var freed int64
	for _, a := range found {
		if a.Recent {
			continue
		}
		if err := purge.Remove(g, log, a); err != nil {
			fmt.Fprintf(w, "could not remove %s: %v\n", a.Path, err)
			continue
		}
		freed += a.Bytes
	}
	fmt.Fprintf(w, "Freed %s.\n", format.Size(freed))
	return nil
}
