package platform

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// nerdProbe holds glyphs only Nerd Fonts have: the Node.js logo and the
// VS Code logo from the Material Design range. Plain Font Awesome fonts
// share many Nerd codepoints, so a common one like the folder would match
// them too.
const nerdProbe = ":charset=e718 f0a1e"

// NerdFontInstalled asks fontconfig whether any installed font has Nerd
// Font glyphs. Terminals that use fontconfig fall back to such a font for
// the glyphs even when it is not the terminal font. It reports false when
// fc-list is missing or slow.
func NerdFontInstalled() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "fc-list", nerdProbe, "family").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}
