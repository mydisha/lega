// Package update installs newer lega releases from GitHub.
package update

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// DefaultReleases is where lega releases are published.
const DefaultReleases = "https://github.com/mydisha/lega/releases"

// SumsFile lists the SHA-256 of every release file.
const SumsFile = "SHA256SUMS"

// ErrNoRelease means nothing has been published yet.
var ErrNoRelease = errors.New("no lega release has been published yet")

// maxSize caps downloads; a lega binary is about 10 MB.
const maxSize = 200 << 20

var tagRe = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)

// Asset is the archive name for a CPU architecture, as the release builds it.
func Asset(goarch string) string { return "lega_linux_" + goarch + ".tar.gz" }

// Updater downloads releases. The zero value is not usable; call New.
type Updater struct {
	Releases string // base URL, e.g. DefaultReleases
	Arch     string // GOARCH of the binary to fetch
	Client   *http.Client
	Agent    string // User-Agent header
}

// New returns an Updater for this machine. LEGA_RELEASES_URL overrides the
// release URL, for mirrors and tests.
func New(version string) Updater {
	base := DefaultReleases
	if v := os.Getenv("LEGA_RELEASES_URL"); v != "" {
		base = strings.TrimRight(v, "/")
	}
	return Updater{
		Releases: base,
		Arch:     runtime.GOARCH,
		Client:   &http.Client{Timeout: 2 * time.Minute},
		Agent:    "lega/" + version,
	}
}

// Latest returns the tag of the newest release, such as "v0.2.0".
func (u Updater) Latest(ctx context.Context) (string, error) {
	// GitHub answers /releases/latest with a redirect to /releases/tag/<tag>.
	// Reading the redirect avoids the API and its rate limit.
	c := *u.Client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := u.get(ctx, &c, u.Releases+"/latest")
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if resp.StatusCode/100 != 3 || loc == "" {
		if resp.StatusCode == http.StatusNotFound {
			return "", ErrNoRelease
		}
		return "", fmt.Errorf("checking for updates: %s", resp.Status)
	}
	i := strings.LastIndex(loc, "/tag/")
	if i < 0 {
		return "", ErrNoRelease
	}
	tag := loc[i+len("/tag/"):]
	if !tagRe.MatchString(tag) {
		return "", fmt.Errorf("unexpected release tag %q", tag)
	}
	return tag, nil
}

// Download fetches the lega binary of a release and checks it against the
// release checksums.
func (u Updater) Download(ctx context.Context, tag string) ([]byte, error) {
	if !tagRe.MatchString(tag) {
		return nil, fmt.Errorf("invalid version %q", tag)
	}
	asset := Asset(u.Arch)
	sums, err := u.fetch(ctx, tag, SumsFile)
	if err != nil {
		return nil, err
	}
	want, err := findSum(sums, asset)
	if err != nil {
		return nil, err
	}
	archive, err := u.fetch(ctx, tag, asset)
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("%s does not match its checksum; nothing was changed", asset)
	}
	return extract(archive)
}

func (u Updater) fetch(ctx context.Context, tag, name string) ([]byte, error) {
	resp, err := u.get(ctx, u.Client, u.Releases+"/download/"+tag+"/"+name)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s %s: %s", name, tag, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", name, err)
	}
	if len(b) > maxSize {
		return nil, fmt.Errorf("%s is too large", name)
	}
	return b, nil
}

func (u Updater) get(ctx context.Context, c *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", u.Agent)
	return c.Do(req)
}

// findSum returns the checksum listed for name in a sha256sum file.
func findSum(sums []byte, name string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name && len(f[0]) == 64 {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("%s is not listed in %s", name, SumsFile)
}

// extract returns the lega binary from a .tar.gz archive.
func extract(archive []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("reading archive: %w", err)
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, errors.New("the archive has no lega binary")
		}
		if err != nil {
			return nil, fmt.Errorf("reading archive: %w", err)
		}
		if h.Typeflag == tar.TypeReg && filepath.Base(h.Name) == "lega" {
			return io.ReadAll(io.LimitReader(tr, maxSize))
		}
	}
}

// CheckReplace returns an error explaining why exe cannot be replaced or
// removed by lega itself. dpkgInfo is normally /var/lib/dpkg/info.
func CheckReplace(exe, dpkgInfo string) error {
	if list, err := os.ReadFile(filepath.Join(dpkgInfo, "lega.list")); err == nil {
		for _, line := range strings.Split(string(list), "\n") {
			if line == exe {
				return ErrPackaged
			}
		}
	}
	dir := filepath.Dir(exe)
	// W_OK; lega needs to create a file next to the binary.
	if err := syscall.Access(dir, 2); err != nil {
		return fmt.Errorf("no write access to %s; run the command with sudo", dir)
	}
	return nil
}

// ErrPackaged means apt installed this binary, so apt must manage it.
var ErrPackaged = errors.New("lega was installed from the .deb package; manage it with apt")

// Replace puts bin in place of the file at exe. The swap is a rename, so a
// failure leaves the old binary untouched.
func Replace(exe string, bin []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".lega-update-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(bin)
	if err == nil {
		err = tmp.Chmod(0o755)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), exe)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}
