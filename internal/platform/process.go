package platform

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Processes returns the names of processes owned by uid, read from procDir
// (normally /proc). The kernel name (comm, cut to 15 bytes), the base name of
// argv[0], and the target binary of exe are all included so wrapped or
// truncated process names still match.
func Processes(procDir string, uid int) (map[string]bool, error) {
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		dir := filepath.Join(procDir, e.Name())
		info, err := os.Stat(dir)
		if err != nil {
			continue // the process exited while we looked
		}
		if st, ok := info.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != uid {
			continue
		}
		if b, err := os.ReadFile(filepath.Join(dir, "comm")); err == nil {
			if n := string(bytes.TrimSpace(b)); n != "" {
				names[n] = true
			}
		}
		if b, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
			argv0, _, _ := bytes.Cut(b, []byte{0})
			if len(argv0) > 0 {
				names[filepath.Base(string(argv0))] = true
			}
		}
		if target, err := os.Readlink(filepath.Join(dir, "exe")); err == nil {
			target = strings.TrimSuffix(target, " (deleted)")
			if base := filepath.Base(target); base != "" && base != "." && base != "/" {
				names[base] = true
			}
		}
	}
	return names, nil
}
