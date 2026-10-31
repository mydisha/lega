// Package scripts_test runs the release scripts against throwaway git repos.
package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cleanEnv drops GIT_* variables, which point git at another repository
// inside hooks and rebases, and ignores the user's git config.
func cleanEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return append(env, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
}

type repo struct {
	t   *testing.T
	dir string
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	for _, tool := range []string{"bash", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " not found")
		}
	}
	r := &repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	return r
}

func (r *repo) git(args ...string) string {
	r.t.Helper()
	args = append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	cmd.Env = cleanEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (r *repo) commit(msgs ...string) {
	r.t.Helper()
	for _, m := range msgs {
		r.git("commit", "-q", "--allow-empty", "-m", m)
	}
}

func (r *repo) tag(tags ...string) {
	r.t.Helper()
	for _, tg := range tags {
		r.git("tag", "-a", tg, "-m", tg)
	}
}

// run executes a script from this folder inside the repo.
func (r *repo) run(script string, args ...string) (string, error) {
	r.t.Helper()
	path, _ := filepath.Abs(script)
	cmd := exec.Command("bash", append([]string{path}, args...)...)
	cmd.Dir = r.dir
	cmd.Env = cleanEnv()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestNextVersion(t *testing.T) {
	r := newRepo(t)
	r.commit("feat: start")

	first := map[string]string{
		"patch": "v0.0.1", "minor": "v0.1.0", "major": "v1.0.0",
		"0.3.0": "v0.3.0", "v1.0.0-rc.1": "v1.0.0-rc.1",
	}
	for in, want := range first {
		if got, err := r.run("next-version.sh", in); err != nil || strings.TrimSpace(got) != want {
			t.Errorf("no tags, %s: got %q, %v; want %s", in, got, err, want)
		}
	}

	r.tag("v0.1.0", "v0.2.3", "v0.3.0-rc.1")
	next := map[string]string{
		"patch": "v0.2.4", "minor": "v0.3.0", "major": "v1.0.0", "v0.10.0": "v0.10.0",
	}
	for in, want := range next {
		if got, err := r.run("next-version.sh", in); err != nil || strings.TrimSpace(got) != want {
			t.Errorf("after v0.2.3, %s: got %q, %v; want %s", in, got, err, want)
		}
	}

	bad := map[string]string{
		"v0.2.3":       "already exists",
		"v0.3.0-rc.1":  "already exists",
		"v0.2.0":       "not newer",
		"v0.2.3-rc.1":  "not newer",
		"v0.9":         "not a valid version",
		"bogus":        "expected patch",
		"v1.0.0; rm x": "not a valid version",
	}
	for in, want := range bad {
		out, err := r.run("next-version.sh", in)
		if err == nil || !strings.Contains(out, want) {
			t.Errorf("%q: got %q, %v; want error %q", in, out, err, want)
		}
	}
	if _, err := r.run("next-version.sh"); err == nil {
		t.Error("no argument: no error")
	}
}

func TestReleaseNotes(t *testing.T) {
	r := newRepo(t)
	r.commit("chore: init", "feat(ui): add menu", "fix: crash on empty folder", "docs: write readme")
	r.tag("v0.1.0")
	r.commit("feat(clean): add yarn rule", "perf(scan): walk faster", "refactor: tidy names", "feat(cli)!: rename --all to --everything")
	r.git("checkout", "-q", "-b", "topic")
	r.commit("fix(purge): skip folders git tracks")
	r.git("checkout", "-q", "main")
	r.git("merge", "-q", "--no-ff", "-m", "Merge pull request #9 from x/topic", "topic")

	notes, err := r.run("release-notes.sh")
	if err != nil {
		t.Fatalf("%v\n%s", err, notes)
	}
	for _, want := range []string{
		"## Breaking changes\n\n- rename --all to --everything",
		"## New features\n\n- clean: add yarn rule\n- cli: rename --all to --everything",
		"## Bug fixes\n\n- purge: skip folders git tracks",
		"## Performance\n\n- scan: walk faster",
		"Run `lega update`",
		"install.sh | bash",
	} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes lack %q:\n%s", want, notes)
		}
	}
	for _, unwanted := range []string{"add menu", "Merge pull request", "tidy names", "readme", "first release", "Full changelog"} {
		if strings.Contains(notes, unwanted) {
			t.Errorf("notes contain %q:\n%s", unwanted, notes)
		}
	}
	order := []string{"## Breaking", "## New features", "## Bug fixes", "## Performance", "## Install or update"}
	for i := 1; i < len(order); i++ {
		if strings.Index(notes, order[i-1]) > strings.Index(notes, order[i]) {
			t.Errorf("%q comes after %q", order[i-1], order[i])
		}
	}

	first, err := r.run("release-notes.sh", "v0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first, "The first release of lega.") || !strings.Contains(first, "- ui: add menu") ||
		!strings.Contains(first, "- crash on empty folder") || strings.Contains(first, "yarn") {
		t.Errorf("first release notes:\n%s", first)
	}

	r.tag("v0.2.0")
	tagged, _ := r.run("release-notes.sh", "v0.2.0")
	if !strings.Contains(tagged, "compare/v0.1.0...v0.2.0") {
		t.Errorf("no changelog link:\n%s", tagged)
	}

	r.commit("docs: fix typo", "ci: bump action")
	if maint, _ := r.run("release-notes.sh"); !strings.Contains(maint, "Maintenance release") {
		t.Errorf("docs-only release:\n%s", maint)
	}

	if out, err := r.run("release-notes.sh", "v9.9.9"); err == nil {
		t.Errorf("unknown tag: no error\n%s", out)
	}
}
