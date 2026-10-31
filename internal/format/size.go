// Package format turns raw numbers into short, human-readable text.
package format

import "fmt"

var units = []string{"kB", "MB", "GB", "TB", "PB", "EB"}

// Size formats a byte count with decimal (SI) units, the same way
// GNOME Files and Disks show sizes on Ubuntu.
func Size(n int64) string {
	if n < 0 {
		return "-" + Size(-n)
	}
	if n < 1000 {
		return fmt.Sprintf("%d B", n)
	}
	v := float64(n) / 1000
	i := 0
	// 999.95 would round up to "1000.0", so move to the next unit early.
	for v >= 999.95 && i < len(units)-1 {
		v /= 1000
		i++
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}

// Count formats a number with thousands separators, like 412,345.
func Count(n int64) string {
	if n < 0 {
		return "-" + Count(-n)
	}
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
