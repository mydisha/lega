package clean

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mydisha/lega/internal/scan"
)

// spot is a folder a path rule cleans inside, plus glob patterns (relative
// to that folder) for what to remove. The root may hold a glob too.
type spot struct {
	root  string
	globs []string
}

// pathRule deletes files and folders that match its spots.
type pathRule struct {
	info      Info
	spots     []spot
	olderThan time.Duration // only targets not modified for this long
	filesOnly bool
	skip      []string // base names to leave alone
}

func (r *pathRule) Info() Info { return r.info }

func (r *pathRule) Scan(ctx context.Context, c *Context) ([]Target, error) {
	guard := c.guardFor(r.info)
	var out []Target
	for _, s := range r.spots {
		roots, _ := filepath.Glob(expand(c, s.root))
		for _, root := range roots {
			for _, g := range s.globs {
				matches, _ := filepath.Glob(filepath.Join(root, g))
				for _, m := range matches {
					if ctx.Err() != nil {
						return out, ctx.Err()
					}
					if t, ok := r.target(ctx, c, m, root); ok && guard.Check(m, []string{root}) == nil {
						out = append(out, t)
					}
				}
			}
		}
	}
	return out, nil
}

func (r *pathRule) target(ctx context.Context, c *Context, path, root string) (Target, bool) {
	for _, s := range r.skip {
		if filepath.Base(path) == s {
			return Target{}, false
		}
	}
	info, err := os.Lstat(path)
	if err != nil || (r.filesOnly && !info.Mode().IsRegular()) {
		return Target{}, false
	}
	if r.olderThan > 0 && c.Now().Sub(info.ModTime()) < r.olderThan {
		return Target{}, false
	}
	bytes, files, err := scan.Size(ctx, path)
	if err != nil {
		return Target{}, false
	}
	return Target{Path: path, Root: root, Bytes: bytes, Files: files}, true
}

func (r *pathRule) Clean(ctx context.Context, c *Context, targets []Target) []Removal {
	guard := c.guardFor(r.info)
	var out []Removal
	for _, t := range targets {
		if ctx.Err() != nil {
			break
		}
		rm := Removal{Path: t.Path, Bytes: t.Bytes}
		if err := guard.Remove(t.Path, []string{t.Root}); err != nil {
			rm.Err = err.Error()
		}
		out = append(out, rm)
	}
	return out
}

// expand turns the placeholders used in rule paths into real folders:
// ~ (home), $CACHE, $CONFIG, $DATA, $DOWNLOAD, $USER and $UID.
func expand(c *Context, p string) string {
	e := c.Env
	switch {
	case p == "~" || strings.HasPrefix(p, "~/"):
		p = e.Home + p[1:]
	case strings.HasPrefix(p, "$CACHE"):
		p = e.CacheDir + p[len("$CACHE"):]
	case strings.HasPrefix(p, "$CONFIG"):
		p = e.ConfigDir + p[len("$CONFIG"):]
	case strings.HasPrefix(p, "$DATA"):
		p = e.DataDir + p[len("$DATA"):]
	case strings.HasPrefix(p, "$DOWNLOAD"):
		dl, ok := e.UserDirs()["DOWNLOAD"]
		if !ok {
			return ""
		}
		p = dl + p[len("$DOWNLOAD"):]
	}
	p = strings.ReplaceAll(p, "$USER", e.User)
	p = strings.ReplaceAll(p, "$UID", strconv.Itoa(e.UID))
	return p
}
