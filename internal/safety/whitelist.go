package safety

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// LoadWhitelist reads the user's keep list: one path or glob per line,
// "#" for comments, "~" for the home folder. A missing file is fine. Any
// other read error is returned, and callers must stop instead of cleaning
// without the user's protections.
func LoadWhitelist(file, home string) ([]string, error) {
	f, err := os.Open(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read whitelist: %w", err)
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "~" || strings.HasPrefix(line, "~/") {
			line = filepath.Join(home, strings.TrimPrefix(line, "~"))
		}
		if !filepath.IsAbs(line) {
			return nil, fmt.Errorf("whitelist line %d: %q must start with / or ~/", n, line)
		}
		if _, err := filepath.Match(line, ""); err != nil {
			return nil, fmt.Errorf("whitelist line %d: bad pattern %q", n, line)
		}
		out = append(out, filepath.Clean(line))
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read whitelist: %w", err)
	}
	return out, nil
}
