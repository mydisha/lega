package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// release is a fake GitHub release served by fakeServer.
type release struct {
	tag    string
	latest string            // Location of /latest; "" means 404
	files  map[string][]byte // name -> body under /download/<tag>/
}

// newRelease builds a release whose archive for arch holds a lega binary
// with the given content.
func newRelease(t *testing.T, tag, arch string, bin []byte) *release {
	t.Helper()
	archive := tarGz(t, map[string][]byte{"LICENSE": []byte("MIT"), "lega": bin})
	sum := sha256.Sum256(archive)
	return &release{
		tag:    tag,
		latest: "/tag/" + tag,
		files: map[string][]byte{
			Asset(arch): archive,
			SumsFile:    []byte(hex.EncodeToString(sum[:]) + "  " + Asset(arch) + "\n"),
		},
	}
}

func tarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write(body)
	}
	tw.Close()
	zw.Close()
	return buf.Bytes()
}

// fakeServer serves r with the URL layout of GitHub releases.
func fakeServer(t *testing.T, r *release) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.URL.Path == "/latest":
			if r.latest == "" {
				http.NotFound(w, req)
				return
			}
			http.Redirect(w, req, r.latest, http.StatusFound)
		case strings.HasPrefix(req.URL.Path, "/tag/"):
			w.Write([]byte("release page"))
		case strings.HasPrefix(req.URL.Path, "/download/"+r.tag+"/"):
			body, ok := r.files[strings.TrimPrefix(req.URL.Path, "/download/"+r.tag+"/")]
			if !ok {
				http.NotFound(w, req)
				return
			}
			w.Write(body)
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testUpdater(srv *httptest.Server) Updater {
	u := New("test")
	u.Releases = srv.URL
	u.Arch = "amd64"
	return u
}

func TestLatest(t *testing.T) {
	ctx := context.Background()
	r := newRelease(t, "v0.2.0", "amd64", []byte("bin"))
	u := testUpdater(fakeServer(t, r))

	tag, err := u.Latest(ctx)
	if err != nil || tag != "v0.2.0" {
		t.Fatalf("Latest = %q, %v", tag, err)
	}

	// With no release, GitHub redirects /latest to the release list.
	r.latest = "/releases"
	if _, err := u.Latest(ctx); !errors.Is(err, ErrNoRelease) {
		t.Errorf("no release: err = %v, want ErrNoRelease", err)
	}
	r.latest = ""
	if _, err := u.Latest(ctx); !errors.Is(err, ErrNoRelease) {
		t.Errorf("404: err = %v, want ErrNoRelease", err)
	}
	r.latest = "/tag/../../evil"
	if _, err := u.Latest(ctx); err == nil {
		t.Error("odd tag accepted")
	}
}

func TestLatestUnreachable(t *testing.T) {
	srv := fakeServer(t, &release{})
	u := testUpdater(srv)
	srv.Close()
	if _, err := u.Latest(context.Background()); err == nil {
		t.Fatal("no error for a closed server")
	}
}

func TestDownload(t *testing.T) {
	ctx := context.Background()
	r := newRelease(t, "v0.2.0", "amd64", []byte("new binary"))
	u := testUpdater(fakeServer(t, r))

	bin, err := u.Download(ctx, "v0.2.0")
	if err != nil || string(bin) != "new binary" {
		t.Fatalf("Download = %q, %v", bin, err)
	}

	if _, err := u.Download(ctx, "v0.3.0"); err == nil {
		t.Error("missing release: no error")
	}
	if _, err := u.Download(ctx, "../x"); err == nil {
		t.Error("bad tag: no error")
	}
	u.Arch = "riscv64"
	if _, err := u.Download(ctx, "v0.2.0"); err == nil || !strings.Contains(err.Error(), "not listed") {
		t.Errorf("unknown arch: err = %v", err)
	}
}

func TestDownloadChecksumMismatch(t *testing.T) {
	r := newRelease(t, "v0.2.0", "amd64", []byte("good"))
	r.files[Asset("amd64")] = tarGz(t, map[string][]byte{"lega": []byte("tampered")})
	_, err := testUpdater(fakeServer(t, r)).Download(context.Background(), "v0.2.0")
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v, want checksum error", err)
	}
}

func TestDownloadArchiveWithoutBinary(t *testing.T) {
	archive := tarGz(t, map[string][]byte{"README.md": []byte("hi")})
	sum := sha256.Sum256(archive)
	r := &release{tag: "v0.2.0", files: map[string][]byte{
		Asset("amd64"): archive,
		SumsFile:       []byte(hex.EncodeToString(sum[:]) + " *" + Asset("amd64") + "\n"),
	}}
	_, err := testUpdater(fakeServer(t, r)).Download(context.Background(), "v0.2.0")
	if err == nil || !strings.Contains(err.Error(), "no lega binary") {
		t.Fatalf("err = %v", err)
	}
}

func TestFindSum(t *testing.T) {
	h := strings.Repeat("ab", 32)
	sums := []byte("garbage line\n" + strings.Repeat("cd", 32) + "  other.tar.gz\n" + strings.ToUpper(h) + " *lega_linux_amd64.tar.gz\n")
	got, err := findSum(sums, "lega_linux_amd64.tar.gz")
	if err != nil || got != h {
		t.Fatalf("findSum = %q, %v", got, err)
	}
	if _, err := findSum(sums, "lega_linux_arm64.tar.gz"); err == nil {
		t.Error("missing entry: no error")
	}
	if _, err := findSum([]byte("short  lega_linux_amd64.tar.gz"), "lega_linux_amd64.tar.gz"); err == nil {
		t.Error("short checksum accepted")
	}
}

func TestReplace(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "lega")
	os.WriteFile(exe, []byte("old"), 0o700)

	if err := Replace(exe, []byte("new")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(exe)
	fi, _ := os.Stat(exe)
	if string(b) != "new" || fi.Mode().Perm() != 0o755 {
		t.Errorf("after Replace: %q %v", b, fi.Mode())
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("left %d files behind", len(entries))
	}

	if err := Replace(filepath.Join(dir, "missing", "lega"), []byte("x")); err == nil {
		t.Error("missing folder: no error")
	}
}

func TestCheckReplace(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "bin", "lega")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	dpkg := filepath.Join(dir, "dpkg")
	os.MkdirAll(dpkg, 0o755)

	if err := CheckReplace(exe, dpkg); err != nil {
		t.Errorf("writable, not packaged: %v", err)
	}

	os.WriteFile(filepath.Join(dpkg, "lega.list"), []byte("/.\n/usr\n/usr/bin\n"+exe+"\n"), 0o644)
	if err := CheckReplace(exe, dpkg); !errors.Is(err, ErrPackaged) {
		t.Errorf("packaged: err = %v", err)
	}
	if err := CheckReplace(exe+"-other", dpkg); err != nil {
		t.Errorf("other path flagged as packaged: %v", err)
	}

	if os.Getuid() == 0 {
		t.Skip("root can write anywhere")
	}
	os.Chmod(filepath.Dir(exe), 0o555)
	t.Cleanup(func() { os.Chmod(filepath.Dir(exe), 0o755) })
	if err := CheckReplace(exe, filepath.Join(dir, "none")); err == nil || !strings.Contains(err.Error(), "sudo") {
		t.Errorf("read-only folder: err = %v", err)
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.2.0", "0.1.0", true},
		{"v0.2.0", "v0.2.0", false},
		{"v0.2.0", "0.2.0", false},
		{"v0.1.9", "0.2.0", false},
		{"v1.0.0", "0.99.99", true},
		{"v0.10.0", "0.9.0", true},
		{"v1.0.0", "1.0.0-rc.1", true},
		{"v1.0.0-rc.1", "1.0.0", false},
		{"v1.0.0-rc.2", "1.0.0-rc.1", true},
		{"v0.1.0", "dev", true},
		{"v0.1.0", "", true},
		{"garbage", "0.1.0", false},
		{"v1.2", "0.1.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.latest, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}
