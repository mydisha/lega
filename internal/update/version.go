package update

import (
	"strconv"
	"strings"
)

// Newer reports whether release tag latest is newer than version current.
// A current version that is not a release, such as "dev", is always older.
func Newer(latest, current string) bool {
	l, ok := parse(latest)
	if !ok {
		return false
	}
	c, ok := parse(current)
	if !ok {
		return true
	}
	for i := range 3 {
		if l.num[i] != c.num[i] {
			return l.num[i] > c.num[i]
		}
	}
	// 1.0.0-rc.1 comes before 1.0.0.
	switch {
	case l.pre == c.pre:
		return false
	case l.pre == "":
		return true
	case c.pre == "":
		return false
	}
	return l.pre > c.pre
}

type semver struct {
	num [3]int
	pre string
}

func parse(v string) (semver, bool) {
	var s semver
	v = strings.TrimPrefix(v, "v")
	v, s.pre, _ = strings.Cut(v, "-")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return s, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return s, false
		}
		s.num[i] = n
	}
	return s, true
}
