// Package platform reads facts about the running system: distro, user
// directories and running processes.
package platform

import (
	"bufio"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
)

// OSRelease holds the fields of /etc/os-release that lega cares about.
type OSRelease struct {
	ID         string
	IDLike     []string
	Name       string
	PrettyName string
	VersionID  string
}

// ParseOSRelease reads os-release key=value lines.
func ParseOSRelease(r io.Reader) (OSRelease, error) {
	var o OSRelease
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		val = unquote(val)
		switch key {
		case "ID":
			o.ID = strings.ToLower(val)
		case "ID_LIKE":
			o.IDLike = strings.Fields(strings.ToLower(val))
		case "NAME":
			o.Name = val
		case "PRETTY_NAME":
			o.PrettyName = val
		case "VERSION_ID":
			o.VersionID = val
		}
	}
	return o, sc.Err()
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		if u, err := strconv.Unquote(`"` + s[1:len(s)-1] + `"`); err == nil {
			return u
		}
		return s[1 : len(s)-1]
	}
	return s
}

// ReadOSRelease reads /etc/os-release, falling back to /usr/lib/os-release.
func ReadOSRelease() (OSRelease, error) {
	for _, p := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		defer f.Close()
		return ParseOSRelease(f)
	}
	return OSRelease{}, os.ErrNotExist
}

// Supported reports whether the distro is Ubuntu, Debian or built on them.
func (o OSRelease) Supported() bool {
	if o.ID == "ubuntu" || o.ID == "debian" {
		return true
	}
	return slices.Contains(o.IDLike, "ubuntu") || slices.Contains(o.IDLike, "debian")
}

// DisplayName returns the friendliest name available.
func (o OSRelease) DisplayName() string {
	switch {
	case o.PrettyName != "":
		return o.PrettyName
	case o.Name != "":
		return o.Name
	case o.ID != "":
		return o.ID
	}
	return "Linux"
}
