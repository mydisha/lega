package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mydisha/lega/internal/platform"
	"github.com/mydisha/lega/internal/update"
)

// fakeInstall points the update code at a temp binary with the given
// content and an empty dpkg database, and sets the running version.
func fakeInstall(t *testing.T, ver, content string) (exe, dpkg string) {
	t.Helper()
	dir := t.TempDir()
	exe = filepath.Join(dir, "bin", "lega")
	write(t, exe, 0)
	os.WriteFile(exe, []byte(content), 0o755)
	dpkg = filepath.Join(dir, "dpkg")
	os.MkdirAll(dpkg, 0o755)

	oldVer, oldExe, oldDpkg := version, executable, dpkgInfo
	version, dpkgInfo = ver, dpkg
	executable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { version, executable, dpkgInfo = oldVer, oldExe, oldDpkg })
	return exe, dpkg
}

// fakeReleases serves release tag holding a lega binary with content bin,
// and points LEGA_RELEASES_URL at it. An empty tag means no release.
func fakeReleases(t *testing.T, tag, bin string) {
	t.Helper()
	var archive bytes.Buffer
	zw := gzip.NewWriter(&archive)
	tw := tar.NewWriter(zw)
	tw.WriteHeader(&tar.Header{Name: "lega", Mode: 0o755, Size: int64(len(bin)), Typeflag: tar.TypeReg})
	tw.Write([]byte(bin))
	tw.Close()
	zw.Close()
	sum := sha256.Sum256(archive.Bytes())
	asset := update.Asset(runtime.GOARCH)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			if tag == "" {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
			http.Redirect(w, r, "/tag/"+tag, http.StatusFound)
		case "/download/" + tag + "/" + asset:
			w.Write(archive.Bytes())
		case "/download/" + tag + "/SHA256SUMS":
			w.Write([]byte(hex.EncodeToString(sum[:]) + "  " + asset + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LEGA_RELEASES_URL", srv.URL)
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestUpdateInstallsNewer(t *testing.T) {
	exe, _ := fakeInstall(t, "0.1.0", "old")
	fakeReleases(t, "v0.2.0", "new")

	out, _, err := lega("update")
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, exe); got != "new" {
		t.Errorf("binary = %q, want new", got)
	}
	if !strings.Contains(out, "0.1.0 → 0.2.0") {
		t.Errorf("output: %q", out)
	}
}

func TestUpdateCheckAndLatest(t *testing.T) {
	exe, _ := fakeInstall(t, "0.1.0", "old")
	fakeReleases(t, "v0.2.0", "new")

	out, _, err := lega("update", "--check")
	if err != nil || !strings.Contains(out, "0.2.0 is available") {
		t.Errorf("--check: %q, %v", out, err)
	}
	if readFile(t, exe) != "old" {
		t.Error("--check changed the binary")
	}

	version = "0.2.0"
	out, _, err = lega("update")
	if err != nil || !strings.Contains(out, "latest version") {
		t.Errorf("up to date: %q, %v", out, err)
	}
	if readFile(t, exe) != "old" {
		t.Error("up to date, but the binary changed")
	}
}

func TestUpdateRefusals(t *testing.T) {
	t.Run("deb package", func(t *testing.T) {
		exe, dpkg := fakeInstall(t, "0.1.0", "old")
		fakeReleases(t, "v0.2.0", "new")
		os.WriteFile(filepath.Join(dpkg, "lega.list"), []byte(exe+"\n"), 0o644)
		_, _, err := lega("update")
		if !errors.Is(err, update.ErrPackaged) || !strings.Contains(err.Error(), ".deb") {
			t.Errorf("err = %v", err)
		}
		if readFile(t, exe) != "old" {
			t.Error("binary changed")
		}
	})
	t.Run("no release", func(t *testing.T) {
		fakeInstall(t, "0.1.0", "old")
		fakeReleases(t, "", "")
		if _, _, err := lega("update"); !errors.Is(err, update.ErrNoRelease) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("arguments", func(t *testing.T) {
		fakeInstall(t, "0.1.0", "old")
		var u usageError
		if _, _, err := lega("update", "v9.9.9"); !errors.As(err, &u) {
			t.Errorf("err = %v, want usage error", err)
		}
	})
}

// removeHome makes a fake home holding lega's settings, log and an
// unrelated config folder.
func removeHome(t *testing.T) platform.Env {
	t.Helper()
	home := fakeHome(t)
	env, err := platform.CurrentEnv()
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(home, ".config/lega/whitelist"), 3)
	write(t, filepath.Join(home, ".local/state/lega/operations.log"), 3)
	write(t, filepath.Join(home, ".config/other/keep"), 3)
	return env
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func TestRemove(t *testing.T) {
	env := removeHome(t)
	exe, _ := fakeInstall(t, "0.1.0", "bin")

	out, _, err := lega("remove", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{exe, filepath.Join(env.ConfigDir, "lega"), filepath.Join(env.StateDir, "lega")} {
		if exists(p) {
			t.Errorf("%s still exists", p)
		}
		if !strings.Contains(out, p) {
			t.Errorf("output does not list %s: %q", p, out)
		}
	}
	if !exists(filepath.Join(env.ConfigDir, "other/keep")) {
		t.Error("removed an unrelated config folder")
	}
}

func TestRemoveConfirm(t *testing.T) {
	env := removeHome(t)
	exe, _ := fakeInstall(t, "0.1.0", "bin")
	var out bytes.Buffer

	for _, answer := range []string{"n\n", "\n", ""} {
		if err := selfRemove(env, false, false, strings.NewReader(answer), &out); err != nil {
			t.Fatal(err)
		}
		if !exists(exe) || !exists(filepath.Join(env.ConfigDir, "lega")) {
			t.Fatalf("answer %q removed files", answer)
		}
	}

	var u usageError
	if err := selfRemove(env, false, false, nil, &out); !errors.As(err, &u) || !exists(exe) {
		t.Errorf("no terminal: err = %v, binary kept = %v", err, exists(exe))
	}

	if err := selfRemove(env, false, false, strings.NewReader("Yes\n"), &out); err != nil {
		t.Fatal(err)
	}
	if exists(exe) || exists(filepath.Join(env.ConfigDir, "lega")) {
		t.Error("answer yes kept files")
	}
}

func TestRemoveAsRootKeepsUserFolders(t *testing.T) {
	env := removeHome(t)
	exe, _ := fakeInstall(t, "0.1.0", "bin")
	var out bytes.Buffer
	if err := selfRemove(env, true, true, nil, &out); err != nil {
		t.Fatal(err)
	}
	if exists(exe) {
		t.Error("binary kept")
	}
	if !exists(filepath.Join(env.ConfigDir, "lega")) || !exists(filepath.Join(env.StateDir, "lega")) {
		t.Error("removed folders from HOME while running as root")
	}
}

func TestRemovePackaged(t *testing.T) {
	env := removeHome(t)
	exe, dpkg := fakeInstall(t, "0.1.0", "bin")
	os.WriteFile(filepath.Join(dpkg, "lega.list"), []byte(exe+"\n"), 0o644)
	var out bytes.Buffer
	err := selfRemove(env, false, true, nil, &out)
	if !errors.Is(err, update.ErrPackaged) || !strings.Contains(err.Error(), "apt remove") {
		t.Errorf("err = %v", err)
	}
	if !exists(exe) {
		t.Error("binary removed")
	}
}
