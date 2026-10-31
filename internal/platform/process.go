package platform

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// Processes returns the names of processes owned by uid, read from procDir
// (normally /proc). Both the kernel name (comm, cut to 15 bytes) and the
// base name of argv[0] are included, so "firefox" matches either way.
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
	}
	return names, nil
}
