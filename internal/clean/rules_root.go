package clean

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// rootRules clean system files. They run through the sudo helper, never in
// the normal user process.
func rootRules() []Rule {
	return []Rule{
		&commandRule{
			info: Info{
				ID: "apt-cache", Name: "APT package cache", Group: GroupRoot, NeedsRoot: true, Needs: []string{"apt-get"},
				Description: "Downloaded .deb files of installed updates, removed with \"apt-get clean\".",
			},
			measure: []string{"/var/cache/apt/archives/*.deb", "/var/cache/apt/archives/partial/*", "/var/cache/apt/*.bin"},
			cmd:     []string{"apt-get", "clean"},
		},
		&commandRule{
			info: Info{
				ID: "apt-autoremove", Name: "Unused packages and old kernels", Group: GroupRoot, Risk: Review,
				NeedsRoot: true, Needs: []string{"apt-get", "dpkg-query"},
				Description: "Packages nothing depends on any more, removed with \"apt-get autoremove --purge\". Check the list first.",
			},
			measureFn: autoremovePackages,
			cmd:       []string{"apt-get", "autoremove", "--purge", "-y"},
		},
		&commandRule{
			info: Info{
				ID: "journal", Name: "Old system journal", Group: GroupRoot, NeedsRoot: true, Needs: []string{"journalctl"},
				Description: "System logs older than 14 days, removed with \"journalctl --vacuum-time=14d\".",
			},
			measureFn: oldJournals,
			cmd:       []string{"journalctl", "--vacuum-time=14d"},
		},
		&pathRule{
			info: Info{
				ID: "rotated-logs", Name: "Rotated log files", Group: GroupRoot, NeedsRoot: true,
				Description: "Compressed and numbered old logs in /var/log. Current logs are kept.",
			},
			spots:     []spot{{"/var/log", []string{"*.gz", "*.[0-9]", "*.old", "*/*.gz", "*/*.[0-9]", "*/*.old"}}},
			filesOnly: true,
		},
		&pathRule{
			info: Info{
				ID: "crash-reports", Name: "Crash reports and core dumps", Group: GroupRoot, NeedsRoot: true,
				Description: "Reports left by crashed programs in /var/crash and systemd core dumps.",
			},
			spots: []spot{
				{"/var/crash", []string{"*"}},
				{"/var/lib/systemd/coredump", []string{"*"}},
			},
		},
		&snapRevisions{},
		&commandRule{
			info: Info{
				ID: "flatpak-unused", Name: "Unused Flatpak runtimes", Group: GroupRoot, Risk: Review,
				NeedsRoot: true, Needs: []string{"flatpak"},
				Description: "Runtimes no installed app uses, removed with \"flatpak uninstall --unused\". The size is known only after cleaning.",
			},
			measureFn: flatpakSize,
			cmd:       []string{"flatpak", "uninstall", "--system", "--unused", "-y", "--noninteractive"},
			hideSize:  true,
		},
	}
}

// autoremovePackages lists what "apt-get autoremove" would remove, with the
// installed size of each package.
func autoremovePackages(ctx context.Context, c *Context) ([]Target, error) {
	out, err := c.Run(ctx, "apt-get", "-s", "autoremove", "--purge")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if f := strings.Fields(line); len(f) >= 2 && (f[0] == "Remv" || f[0] == "Purg") {
			names = append(names, f[1])
		}
	}
	if len(names) == 0 {
		return nil, nil
	}
	args := append([]string{"-W", "-f=${Package}\t${Installed-Size}\n"}, names...)
	out, err = c.Run(ctx, "dpkg-query", args...)
	if err != nil {
		return nil, err
	}
	var ts []Target
	for _, line := range strings.Split(string(out), "\n") {
		name, kb, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		n, _ := strconv.ParseInt(strings.TrimSpace(kb), 10, 64)
		ts = append(ts, Target{Path: name, Bytes: n * 1024, Files: 1})
	}
	return ts, nil
}

// journalDir is where systemd keeps persistent logs. Tests point it
// elsewhere.
var journalDir = "/var/log/journal"

// oldJournals sums archived journal files older than 14 days. When the
// journal folder cannot be read, it falls back to the total journal size
// as an upper bound.
func oldJournals(ctx context.Context, c *Context) ([]Target, error) {
	files, _ := filepath.Glob(filepath.Join(journalDir, "*", "*@*.journal*"))
	cutoff := c.Now().Add(-14 * 24 * time.Hour)
	var ts []Target
	for _, f := range files {
		info, err := os.Lstat(f)
		if err == nil && info.ModTime().Before(cutoff) {
			ts = append(ts, Target{Path: f, Bytes: info.Size(), Files: 1})
		}
	}
	if len(files) > 0 {
		return ts, nil
	}
	out, err := c.Run(ctx, "journalctl", "--disk-usage")
	if err != nil {
		return nil, nil
	}
	// "Archived and active journals take up 2.2G in the file system."
	f := strings.Fields(string(out))
	for i, w := range f {
		if w == "up" && i+1 < len(f) {
			if n := parseUnitSize(f[i+1]); n > 0 {
				return []Target{{Path: "system journal", Bytes: n, Unknown: true}}, nil
			}
		}
	}
	return nil, nil
}

// flatpakSize measures the system Flatpak folder, used to compute what
// "flatpak uninstall --unused" really freed.
func flatpakSize(ctx context.Context, c *Context) ([]Target, error) {
	return measurePaths(func(*Context) []string { return []string{"/var/lib/flatpak/runtime"} })(ctx, c)
}

// snapRevisions removes old snap versions that snapd keeps around after an
// update ("disabled" in "snap list --all").
type snapRevisions struct{}

var (
	snapName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	snapRev  = regexp.MustCompile(`^x?[0-9]+$`)
)

func (*snapRevisions) Info() Info {
	return Info{
		ID: "snap-revisions", Name: "Old snap versions", Group: GroupRoot, NeedsRoot: true, Needs: []string{"snap"},
		Description: "Previous versions snapd keeps after updates. The current version of each snap stays.",
	}
}

func (*snapRevisions) Scan(ctx context.Context, c *Context) ([]Target, error) {
	out, err := c.Run(ctx, "snap", "list", "--all")
	if err != nil {
		return nil, nil // snapd not running
	}
	var ts []Target
	for _, d := range parseDisabledSnaps(string(out)) {
		t := Target{Path: fmt.Sprintf("/var/lib/snapd/snaps/%s_%s.snap", d[0], d[1]), Files: 1}
		if info, err := os.Lstat(t.Path); err == nil {
			t.Bytes = info.Size()
		}
		ts = append(ts, t)
	}
	return ts, nil
}

func (*snapRevisions) Clean(ctx context.Context, c *Context, targets []Target) []Removal {
	var out []Removal
	for _, t := range targets {
		name, rev, ok := snapFromPath(t.Path)
		rm := Removal{Path: t.Path, Bytes: t.Bytes}
		switch {
		case !ok:
			rm.Err = "not a snap file"
		default:
			if _, err := c.Run(ctx, "snap", "remove", name, "--revision="+rev); err != nil {
				rm.Err = err.Error()
			}
		}
		out = append(out, rm)
	}
	return out
}

// parseDisabledSnaps returns name and revision of each disabled snap.
func parseDisabledSnaps(out string) [][2]string {
	var res [][2]string
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 6 || !strings.Contains(f[len(f)-1], "disabled") {
			continue
		}
		if snapName.MatchString(f[0]) && snapRev.MatchString(f[2]) {
			res = append(res, [2]string{f[0], f[2]})
		}
	}
	return res
}

// snapFromPath turns /var/lib/snapd/snaps/<name>_<rev>.snap back into its
// parts, checking both so nothing odd reaches the snap command.
func snapFromPath(p string) (name, rev string, ok bool) {
	base := strings.TrimSuffix(filepath.Base(p), ".snap")
	i := strings.LastIndexByte(base, '_')
	if i < 0 || filepath.Dir(p) != "/var/lib/snapd/snaps" {
		return "", "", false
	}
	name, rev = base[:i], base[i+1:]
	return name, rev, snapName.MatchString(name) && snapRev.MatchString(rev)
}
