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
		want  glyph
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
			t.Errorf("nodeIcon(%q, %v) = %v, want %v", c.path, c.isDir, got, c.want)
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

// The width math assumes each Nerd glyph is one cell and each emoji two;
// a glyph that measures otherwise would push rows past the screen edge.
// Emoji must not need a variation selector or joiner, since terminals
// disagree on how wide those draw.
func TestGlyphWidths(t *testing.T) {
	all := []glyph{icoFolder, icoFile}
	for _, m := range []map[string]glyph{ruleIcons, dirIcons, extIcons, userDirIcons} {
		for _, g := range m {
			all = append(all, g)
		}
	}
	for _, g := range all {
		if w := ansi.StringWidth(g.nerd); w != 1 || len([]rune(g.nerd)) != 1 {
			t.Errorf("nerd %U is %d cells wide", []rune(g.nerd), w)
		}
		if w := ansi.StringWidth(g.emoji); w != 2 || len([]rune(g.emoji)) != 1 {
			t.Errorf("emoji %q (%U) is %d cells wide", g.emoji, []rune(g.emoji), w)
		}
		if strings.ContainsAny(g.emoji, "\ufe0f\u200d") {
			t.Errorf("emoji %q needs a variation selector or joiner", g.emoji)
		}
	}
}

func TestIconSets(t *testing.T) {
	sh := testShared(t, 80)
	if sh.icon(icoFolder, sh.theme.Dim) != "" || sh.iconWidth() != 0 {
		t.Error("the test default is no icons")
	}
	for _, c := range []struct {
		set  IconSet
		want string
	}{{IconsNerd, icoFolder.nerd + " "}, {IconsEmoji, icoFolder.emoji + " "}} {
		sh.icons = c.set
		got := ansi.Strip(sh.icon(icoFolder, sh.theme.Dim))
		if got != c.want || sh.iconWidth() != ansi.StringWidth(got) {
			t.Errorf("set %d: icon = %q, width %d", c.set, got, sh.iconWidth())
		}
	}
}

// Rows with icons on still fit the screen with very long names, in both
// sets.
func TestIconRowsFit(t *testing.T) {
	longDir := strings.Repeat("ディレクトリ-very-long-folder-name-", 4)
	for _, set := range []IconSet{IconsNerd, IconsEmoji} {
		for _, w := range []int{60, 80} {
			sh := testShared(t, w)
			sh.icons = set
			write(t, filepath.Join(sh.env.Home, longDir, "写真-"+strings.Repeat("x", 120)+".png"), 50_000)
			write(t, filepath.Join(sh.env.Home, "Projects", longDir, "package.json"), 100)
			write(t, filepath.Join(sh.env.Home, "Projects", longDir, "node_modules", "a.js"), 300_000)
			old := time.Now().Add(-40 * 24 * time.Hour)
			chtimesTree(t, filepath.Join(sh.env.Home, "Projects"), old)
			junkHome(t, sh)

			a := startAnalyze(t, sh, "")
			checkIconView(t, sh, "analyze", a.View(), icoFolder)
			sendKeys(a, "f")
			checkIconView(t, sh, "largest files", a.View(), icoImage)

			s := newStatus(sh).(*status)
			s = run(s, s.scanHome()).(*status)
			checkIconView(t, sh, "status", s.View(), icoFolder)

			p := newPurger(nil)(sh)
			p = settle(p, p.Init())
			checkIconView(t, sh, "purge", p.View(), icoNode)

			c := cleanContext(t, sh)
			m := newCleanerWith(sh, c, userRules(c))
			m = settle(m, m.Init()).(*cleaner)
			checkIconView(t, sh, "clean", m.View(), icoFirefox)
		}
	}
}

func checkIconView(t *testing.T, sh *shared, name, view string, g glyph) {
	t.Helper()
	fitsWidth(t, view, sh.w())
	want := g.nerd
	if sh.icons == IconsEmoji {
		want = g.emoji
	}
	if !strings.Contains(view, want) {
		t.Errorf("%s (set %d, %d columns) lacks icon %q:\n%s", name, sh.icons, sh.w(), want, ansi.Strip(view))
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
