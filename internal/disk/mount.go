// Package disk lists mounted filesystems and how full they are.
package disk

import (
	"bufio"
	"io"
	"os"
	"slices"
	"strings"
	"syscall"
)

// Mount is one real filesystem with its usage in bytes.
type Mount struct {
	Device     string
	MountPoint string
	FSType     string
	Total      int64
	Used       int64
	Free       int64 // space an unprivileged user can still write
}

// UsedPercent returns how full the filesystem is, from 0 to 100.
func (m Mount) UsedPercent() float64 {
	if m.Total == 0 {
		return 0
	}
	return float64(m.Used) / float64(m.Total) * 100
}

// realFS are filesystem types that store user data. Everything else
// (tmpfs, proc, squashfs from snaps, overlay, ...) is skipped.
var realFS = []string{
	"ext2", "ext3", "ext4", "btrfs", "xfs", "f2fs", "zfs", "jfs",
	"vfat", "exfat", "ntfs", "ntfs3", "fuseblk", "reiserfs", "bcachefs",
}

// ParseMountInfo reads /proc/self/mountinfo and returns real filesystems.
// When a device is mounted more than once (bind mounts, btrfs subvolumes),
// only the mount with the shortest path is kept.
func ParseMountInfo(r io.Reader) ([]Mount, error) {
	var mounts []Mount
	seen := map[string]int{} // major:minor -> index in mounts
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		// Format: id parent major:minor root mountpoint opts [optional...] - fstype source superopts
		pre, post, ok := strings.Cut(sc.Text(), " - ")
		if !ok {
			continue
		}
		left := strings.Fields(pre)
		right := strings.Fields(post)
		if len(left) < 5 || len(right) < 2 {
			continue
		}
		fstype := right[0]
		if !slices.Contains(realFS, fstype) {
			continue
		}
		m := Mount{
			Device:     unescape(right[1]),
			MountPoint: unescape(left[4]),
			FSType:     fstype,
		}
		devID := left[2]
		if i, dup := seen[devID]; dup {
			if len(m.MountPoint) < len(mounts[i].MountPoint) {
				mounts[i] = m
			}
			continue
		}
		seen[devID] = len(mounts)
		mounts = append(mounts, m)
	}
	return mounts, sc.Err()
}

// unescape decodes the octal escapes (\040 for space, ...) used in mountinfo.
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) && isOctal(s[i+1:i+4]) {
			b.WriteByte((s[i+1]-'0')<<6 | (s[i+2]-'0')<<3 | (s[i+3] - '0'))
			i += 3
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isOctal(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '7' {
			return false
		}
	}
	return true
}

// Usage returns total, used and free bytes for the filesystem holding path.
func Usage(path string) (total, used, free int64, err error) {
	var st syscall.Statfs_t
	if err = syscall.Statfs(path, &st); err != nil {
		return 0, 0, 0, err
	}
	bs := int64(st.Bsize)
	total = int64(st.Blocks) * bs
	used = int64(st.Blocks-st.Bfree) * bs
	free = int64(st.Bavail) * bs
	return total, used, free, nil
}

// List returns every real mounted filesystem with its usage filled in.
// Mounts that cannot be read (unplugged, no permission) are left out.
func List() ([]Mount, error) {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	mounts, err := ParseMountInfo(f)
	if err != nil {
		return nil, err
	}
	out := mounts[:0]
	for _, m := range mounts {
		t, u, fr, err := Usage(m.MountPoint)
		if err != nil || t == 0 {
			continue
		}
		m.Total, m.Used, m.Free = t, u, fr
		out = append(out, m)
	}
	return out, nil
}
