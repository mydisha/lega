package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The install script lives at the repo root; it is tested here because it
// must agree with this package on the release layout.
func runInstall(t *testing.T, releases string, args ...string) (string, error) {
	t.Helper()
	for _, tool := range []string{"bash", "curl", "tar", "sha256sum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " not found")
		}
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("no release build for " + runtime.GOARCH)
	}
	script, _ := filepath.Abs("../../install.sh")
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Env = append(os.Environ(), "LEGA_RELEASES_URL="+releases, "HOME="+t.TempDir())
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestInstallScript(t *testing.T) {
	r := newRelease(t, "v0.2.0", runtime.GOARCH, []byte("#!/bin/sh\necho fake\n"))
	srv := fakeServer(t, r)

	t.Run("latest", func(t *testing.T) {
		prefix := filepath.Join(t.TempDir(), "bin")
		out, err := runInstall(t, srv.URL, "--prefix", prefix)
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		fi, err := os.Stat(filepath.Join(prefix, "lega"))
		if err != nil || fi.Mode().Perm() != 0o755 {
			t.Fatalf("installed binary: %v %v", fi, err)
		}
		if b, _ := os.ReadFile(filepath.Join(prefix, "lega")); string(b) != "#!/bin/sh\necho fake\n" {
			t.Errorf("content = %q", b)
		}
		if entries, _ := os.ReadDir(prefix); len(entries) != 1 {
			t.Errorf("prefix holds %d files, want 1", len(entries))
		}
		if !strings.Contains(out, "Installed lega 0.2.0") {
			t.Errorf("output: %s", out)
		}
	})

	t.Run("pinned version", func(t *testing.T) {
		r.latest = "/tag/v9.9.9"
		defer func() { r.latest = "/tag/v0.2.0" }()
		prefix := t.TempDir()
		if out, err := runInstall(t, srv.URL, "0.2.0", "--prefix="+prefix); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if _, err := os.Stat(filepath.Join(prefix, "lega")); err != nil {
			t.Error(err)
		}
	})
}

func TestInstallScriptFailures(t *testing.T) {
	cases := []struct {
		name  string
		setup func(r *release)
		args  []string
		want  string
	}{
		{"tampered archive", func(r *release) {
			r.files[Asset(runtime.GOARCH)] = tarGz(t, map[string][]byte{"lega": []byte("evil")})
		}, nil, "does not match its checksum"},
		{"no release", func(r *release) { r.latest = "/releases" }, nil, "no lega release found"},
		{"missing version", func(*release) {}, []string{"v0.3.0"}, "cannot download"},
		{"unknown argument", func(*release) {}, []string{"--force"}, "unknown argument"},
		{"no checksum entry", func(r *release) { r.files[SumsFile] = []byte("") }, nil, "not listed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRelease(t, "v0.2.0", runtime.GOARCH, []byte("good"))
			c.setup(r)
			prefix := filepath.Join(t.TempDir(), "bin")
			out, err := runInstall(t, fakeServer(t, r).URL, append(c.args, "--prefix", prefix)...)
			if err == nil {
				t.Fatalf("succeeded:\n%s", out)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("output does not mention %q:\n%s", c.want, out)
			}
			if _, err := os.Stat(filepath.Join(prefix, "lega")); err == nil {
				t.Error("a binary was installed")
			}
		})
	}
}
