package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/mydisha/lega/internal/clean"
)

func TestNodeIcon(t *testing.T) {
	home := "/home/tester"
	userDirs := map[string]string{"DOWNLOAD": home + "/Unduhan", "PICTURES": home + "/Gambar", "TEMPLATES": home + "/Templat"}
	cases := []struct {
		path  string
		isDir bool
		want  string
	}{
		{home + "/Unduhan", true, icoDownload},
		{home + "/Gambar", true, icoPictures},
		{home + "/Templat", true, icoFolder},   // a user dir without its own icon
		{home + "/Downloads", true, icoFolder}, // renamed away by user-dirs.dirs
		{home + "/code/app/node_modules", true, icoNode},
		{home + "/code/app/.git", true, icoGit},
		{home + "/code/app/target", true, icoFolder},
		{home + "/foto liburan", true, icoFolder},
		{home + "/IMG_0001.JPG", false, icoImage},
		{home + "/film.mkv", false, icoVideo},
		{home + "/lagu.flac", false, icoAudio},
		{home + "/ubuntu-24.04.iso", false, icoDisk},
		{home + "/app.AppImage", false, icoPackage},
		{home + "/backup.tar.gz", false, icoArchive},
		{home + "/cv.pdf", false, icoPDF},
		{home + "/main.go", false, icoCode},
		{home + "/README", false, icoFile},
		{home + "/.bashrc", false, icoFile},
		{home + "/写真.png", false, icoImage},
	}
	for _, c := range cases {
		if got := nodeIcon(c.path, c.isDir, userDirs); got != c.want {
			t.Errorf("nodeIcon(%q, %v) = %U, want %U", c.path, c.isDir, []rune(got), []rune(c.want))
		}
	}
}

// Every built-in clean rule gets its own icon, so a new rule without one
// shows up here instead of quietly falling back to a folder.
func TestEveryRuleHasIcon(t *testing.T) {
	for _, r := range clean.Rules() {
		id := r.Info().ID
		if _, ok := ruleIcons[id]; !ok {
			t.Errorf("rule %q has no icon in ruleIcons", id)
		}
	}
	if ruleIcon("user-defined") != icoFolder {
		t.Error("unknown rules should get the folder icon")
	}
}

// The width math assumes each glyph is one cell; a glyph that measures
// wider would push rows past the screen edge.
func TestIconsAreOneCell(t *testing.T) {
	all := []string{icoFolder, icoFile}
	for _, g := range ruleIcons {
		all = append(all, g)
	}
	for _, g := range dirIcons {
		all = append(all, g)
	}
	for _, g := range extIcons {
		all = append(all, g)
	}
	for _, g := range all {
		if w := ansi.StringWidth(g); w != 1 {
			t.Errorf("%U is %d cells wide", []rune(g), w)
		}
	}
}

func TestIconsOff(t *testing.T) {
	sh := testShared(t, 80)
	if sh.icon(icoFolder, sh.theme.Dim) != "" || sh.iconWidth() != 0 {
		t.Error("icons must stay off unless asked for")
	}
	sh.icons = true
	if got := ansi.Strip(sh.icon(icoFolder, sh.theme.Dim)); got != icoFolder+" " || sh.iconWidth() != ansi.StringWidth(got) {
		t.Errorf("icon = %q, width %d", got, sh.iconWidth())
	}
}

// Rows with icons on still fit the screen with very long names.
func TestIconRowsFit(t *testing.T) {
	longDir := strings.Repeat("ディレクトリ-very-long-folder-name-", 4)
	for _, w := range []int{60, 80} {
		sh := testShared(t, w)
		sh.icons = true
		write(t, filepath.Join(sh.env.Home, longDir, "写真-"+strings.Repeat("x", 120)+".png"), 50_000)
		write(t, filepath.Join(sh.env.Home, "Projects", longDir, "package.json"), 100)
		write(t, filepath.Join(sh.env.Home, "Projects", longDir, "node_modules", "a.js"), 300_000)
		old := time.Now().Add(-40 * 24 * time.Hour)
		chtimesTree(t, filepath.Join(sh.env.Home, "Projects"), old)
		junkHome(t, sh)

		a := startAnalyze(t, sh, "")
		checkIconView(t, "analyze", a.View(), w, icoFolder)
		sendKeys(a, "f")
		checkIconView(t, "largest files", a.View(), w, icoImage)

		s := newStatus(sh).(*status)
		s = run(s, s.scanHome()).(*status)
		checkIconView(t, "status", s.View(), w, icoFolder)

		p := newPurger(nil)(sh)
		p = settle(p, p.Init())
		checkIconView(t, "purge", p.View(), w, icoNode)

		c := cleanContext(t, sh)
		m := newCleanerWith(sh, c, userRules(c))
		m = settle(m, m.Init()).(*cleaner)
		checkIconView(t, "clean", m.View(), w, icoFirefox)
	}
}

func checkIconView(t *testing.T, name, view string, w int, glyph string) {
	t.Helper()
	fitsWidth(t, view, w)
	if !strings.Contains(view, glyph) {
		t.Errorf("%s at %d columns lacks icon %U:\n%s", name, w, []rune(glyph), ansi.Strip(view))
	}
}

// chtimesTree sets the times of everything under root, so purge treats it
// as not recently used.
func chtimesTree(t *testing.T, root string, when time.Time) {
	t.Helper()
	err := filepath.Walk(root, func(p string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(p, when, when)
	})
	if err != nil {
		t.Fatal(err)
	}
}
