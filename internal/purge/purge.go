// Package purge finds build output and dependency folders in projects
// (node_modules, target, .venv, ...). They can be rebuilt with one command
// and are often the biggest folders in a developer's home.
package purge

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mydisha/lega/internal/oplog"
	"github.com/mydisha/lega/internal/safety"
	"github.com/mydisha/lega/internal/scan"
)

// kind is a folder name that is an artifact when one of its markers sits
// next to it (or, for inside, within it). Markers may contain glob wildcards.
type kind struct {
	name    string
	markers []string
	inside  bool // markers are looked up inside the folder
}

var kinds = []kind{
	{name: "node_modules", markers: []string{"package.json"}},
	{name: ".next", markers: []string{"package.json"}},
	{name: ".nuxt", markers: []string{"package.json"}},
	{name: ".svelte-kit", markers: []string{"package.json"}},
	{name: ".parcel-cache", markers: []string{"package.json"}},
	{name: ".turbo", markers: []string{"package.json"}},
	{name: ".angular", markers: []string{"angular.json", "package.json"}},
	{name: ".astro", markers: []string{"package.json"}},
	{name: ".vite", markers: []string{"package.json"}},
	{name: ".output", markers: []string{"nuxt.config.ts", "nuxt.config.js", "nitro.config.ts", "package.json"}},
	{name: ".webpack", markers: []string{"package.json"}},
	{name: ".nyc_output", markers: []string{"package.json"}},
	{name: "storybook-static", markers: []string{"package.json"}},
	{name: ".storybook-static", markers: []string{"package.json"}},
	{name: "target", markers: []string{"Cargo.toml", "pom.xml", "build.sbt"}},
	{name: ".bloop", markers: []string{"build.sbt", "build.sc"}},
	{name: ".metals", markers: []string{"build.sbt", "build.sc"}},
	{name: ".venv", markers: []string{"pyvenv.cfg"}, inside: true},
	{name: "venv", markers: []string{"pyvenv.cfg"}, inside: true},
	{name: "__pycache__"},
	{name: ".pytest_cache"},
	{name: ".mypy_cache"},
	{name: ".ruff_cache"},
	{name: ".tox", markers: []string{"tox.ini", "pyproject.toml", "setup.cfg"}},
	{name: ".nox", markers: []string{"noxfile.py"}},
	{name: ".hypothesis", markers: []string{"pyproject.toml", "setup.py", "pytest.ini"}},
	{name: "htmlcov", markers: []string{"pyproject.toml", "setup.py", "pytest.ini"}},
	{name: ".eggs", markers: []string{"setup.py", "pyproject.toml"}},
	{name: "vendor", markers: []string{"composer.json"}},
	{name: "_build", markers: []string{"mix.exs"}},
	{name: "deps", markers: []string{"mix.exs"}},
	{name: ".gradle", markers: []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"}},
	{name: ".dart_tool", markers: []string{"pubspec.yaml"}},
	{name: "cmake-build-debug", markers: []string{"CMakeLists.txt"}},
	{name: "cmake-build-release", markers: []string{"CMakeLists.txt"}},
	{name: "zig-out", markers: []string{"build.zig"}},
	{name: ".zig-cache", markers: []string{"build.zig"}},
	{name: "zig-cache", markers: []string{"build.zig"}},
	{name: "obj", markers: []string{"*.csproj", "*.fsproj"}},
	{name: ".terraform", markers: []string{".terraform.lock.hcl", "*.tf"}},
	{name: "coverage", markers: []string{"package.json", "pyproject.toml", "Cargo.toml"}},
	{name: "build", markers: []string{"package.json", "build.gradle", "build.gradle.kts", "pubspec.yaml", "pyproject.toml", "setup.py"}},
	{name: "dist", markers: []string{"package.json", "pyproject.toml", "setup.py"}},
}

// RecentAge is how fresh an artifact must be to start unselected.
const RecentAge = 7 * 24 * time.Hour

// maxDepth stops the walk from crawling huge trees forever.
const maxDepth = 10

// gitTimeout bounds how long git ls-files may take on a single folder.
const gitTimeout = 5 * time.Second

// skipDirs are version-control and system folders purge never descends into.
var skipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true, ".bzr": true,
	".cache": true, ".local": true, ".Trash": true,
}

// Artifact is one folder that can be purged.
type Artifact struct {
	Path    string
	Kind    string
	Root    string // search folder it was found under
	Bytes   int64
	Files   int64
	ModTime time.Time
	Recent  bool // changed in the last week, so not selected by default
}

// Project returns the folder that owns the artifact.
func (a Artifact) Project() string { return filepath.Dir(a.Path) }

// Finder searches folders for artifacts.
type Finder struct {
	Guard *safety.Guard
	Now   func() time.Time
	// Tracked reports whether git tracks anything inside path. Artifacts
	// that git tracks are skipped.
	Tracked func(path string) bool
}

// NewFinder returns a Finder that asks git about tracked files.
func NewFinder(g *safety.Guard) *Finder {
	return &Finder{Guard: g, Now: time.Now, Tracked: gitTracked}
}

// RootsPath is where users may list custom project folders for purge.
func RootsPath(configDir string) string {
	return filepath.Join(configDir, "lega", "purge_paths")
}

// LoadRoots reads custom search folders from file: one path per line, "#"
// for comments, "~" for home. A missing file returns nil, nil.
func LoadRoots(file, home string) ([]string, error) {
	f, err := os.Open(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read purge paths: %w", err)
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for i := 0; i < len(line); i++ {
			if line[i] < 0x20 || line[i] == 0x7f {
				return nil, fmt.Errorf("purge paths line %d: invalid characters", n)
			}
		}
		if line == "~" || strings.HasPrefix(line, "~/") {
			line = filepath.Join(home, strings.TrimPrefix(line, "~"))
		}
		if !filepath.IsAbs(line) {
			return nil, fmt.Errorf("purge paths line %d: %q must start with / or ~/", n, line)
		}
		p := filepath.Clean(line)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			out = append(out, p)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read purge paths: %w", err)
	}
	return out, nil
}

// DefaultRoots returns the usual project folders that exist in home.
func DefaultRoots(home string) []string {
	var out []string
	names := []string{
		"Projects", "projects", "Code", "code", "dev", "Developer", "src", "www",
		"workspace", "Workspace", "Work", "work", "repos", "git",
		"GitHub", "github", "gitlab", "Sites",
		"IdeaProjects", "PycharmProjects", "AndroidStudioProjects", "WebstormProjects", "CLionProjects",
	}
	for _, name := range names {
		p := filepath.Join(home, name)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

// Find walks roots and returns artifacts, biggest first.
func (f *Finder) Find(ctx context.Context, roots []string) ([]Artifact, error) {
	var found []Artifact
	seenRoots := map[string]bool{}
	seenArtifacts := map[string]bool{}
	for _, root := range roots {
		root, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		realRoot := root
		if r, err := filepath.EvalSymlinks(root); err == nil {
			realRoot = r
		}
		if seenRoots[realRoot] {
			continue
		}
		seenRoots[realRoot] = true

		rinfo, err := os.Lstat(realRoot)
		if err != nil {
			return nil, err
		}
		var rootDev uint64
		if st, ok := rinfo.Sys().(*syscall.Stat_t); ok {
			rootDev = st.Dev
		}

		depth0 := strings.Count(root, string(filepath.Separator))
		err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil || !d.IsDir() {
				return nil
			}
			if p != root {
				if strings.Count(p, string(filepath.Separator))-depth0 > maxDepth {
					return fs.SkipDir
				}
				if skipDirs[d.Name()] {
					return fs.SkipDir
				}
				// Stay on the same filesystem so mounted drives are never crawled.
				if info, ierr := d.Info(); ierr == nil {
					if st, ok := info.Sys().(*syscall.Stat_t); ok && rootDev != 0 && st.Dev != rootDev {
						return fs.SkipDir
					}
				}
				if isArtifact(p, d.Name()) {
					key := p
					if rp, rerr := filepath.EvalSymlinks(p); rerr == nil {
						key = rp
					}
					if !seenArtifacts[key] {
						seenArtifacts[key] = true
						found = append(found, Artifact{Path: p, Kind: d.Name(), Root: root})
					}
					return fs.SkipDir
				}
				// Never descend into orphan node_modules folders.
				if d.Name() == "node_modules" {
					return fs.SkipDir
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return f.measure(ctx, found), ctx.Err()
}

func isArtifact(path, name string) bool {
	for _, k := range kinds {
		if k.name != name {
			continue
		}
		if len(k.markers) == 0 {
			return true
		}
		dir := filepath.Dir(path)
		if k.inside {
			dir = path
		}
		for _, m := range k.markers {
			if strings.ContainsAny(m, "*?[") {
				if matches, _ := filepath.Glob(filepath.Join(dir, m)); len(matches) > 0 {
					return true
				}
				continue
			}
			if _, err := os.Lstat(filepath.Join(dir, m)); err == nil {
				return true
			}
		}
	}
	return false
}

// measure fills in sizes and dates in parallel, and drops artifacts the
// guard or git protect.
func (f *Finder) measure(ctx context.Context, found []Artifact) []Artifact {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	keep := make([]bool, len(found))
	for i := range found {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			a := &found[i]
			if f.Guard.Check(a.Path, []string{a.Root}) != nil || f.Tracked(a.Path) {
				return
			}
			a.Bytes, a.Files, _ = scan.Size(ctx, a.Path)
			a.ModTime = newest(a.Path)
			a.Recent = f.Now().Sub(a.ModTime) < RecentAge
			keep[i] = true
		}()
	}
	wg.Wait()

	var out []Artifact
	for i, a := range found {
		if keep[i] {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	return out
}

// newest returns the latest change time of a folder and its direct
// children. Installing a package touches these, so it is a cheap signal
// for "used recently".
func newest(dir string) time.Time {
	var t time.Time
	if info, err := os.Lstat(dir); err == nil {
		t = info.ModTime()
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.ModTime().After(t) {
			t = info.ModTime()
		}
	}
	return t
}

// Remove deletes an artifact after checking it with the guard again.
func Remove(g *safety.Guard, log *oplog.Log, a Artifact) error {
	err := g.Remove(a.Path, []string{a.Root})
	log.Write(oplog.Entry{Action: "purge", Rule: a.Kind, Path: a.Path, Bytes: a.Bytes, Err: err})
	return err
}

// gitTracked asks git whether it tracks files under path. Outside a repo
// the answer is no. If git is needed but missing, it plays safe and says
// yes.
func gitTracked(path string) bool {
	if !inGitRepo(path) {
		return false
	}
	if _, err := exec.LookPath("git"); err != nil {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", filepath.Dir(path), "ls-files", "-z", "--", filepath.Base(path))
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	return err != nil || len(out) > 0
}

// gitEnv drops GIT_* variables. Inside a git hook or rebase they point git
// at another repository instead of the one around path.
func gitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return env
}

func inGitRepo(path string) bool {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		if dir == "/" || dir == "." {
			return false
		}
	}
}
