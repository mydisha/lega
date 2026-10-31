package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func startAnalyze(t *testing.T, sh *shared, root string) *analyze {
	t.Helper()
	a := newAnalyze(root)(sh).(*analyze)
	return run(a, a.rescan()).(*analyze)
}

func sendKeys(s screen, keys ...string) screen {
	for _, k := range keys {
		s, _ = s.Update(press(k))
	}
	return s
}

func homeTree(t *testing.T, sh *shared) {
	t.Helper()
	write(t, filepath.Join(sh.env.Home, "media", "big.mkv"), 600_000)
	write(t, filepath.Join(sh.env.Home, "media", "small.mkv"), 20_000)
	write(t, filepath.Join(sh.env.Home, "docs", "a.txt"), 90_000)
	write(t, filepath.Join(sh.env.Home, "Documents", "cv.pdf"), 5_000)
	os.Mkdir(filepath.Join(sh.env.Home, "empty"), 0o755)
}

func TestAnalyzeBrowse(t *testing.T) {
	sh := testShared(t, 80)
	homeTree(t, sh)
	a := startAnalyze(t, sh, "")

	view := ansi.Strip(a.View())
	if !strings.Contains(view, "media/") || strings.Index(view, "media/") > strings.Index(view, "docs/") {
		t.Fatalf("media should be listed first:\n%s", view)
	}
	fitsWidth(t, a.View(), 80)

	sendKeys(a, "enter")
	if a.cur.Name != "media" || a.rows[0].Name != "big.mkv" {
		t.Fatalf("enter should open media, at %q", a.cur.Name)
	}
	sendKeys(a, "backspace")
	if a.cur != a.tree || a.selected().Name != "media" {
		t.Error("going up should put the cursor back on media")
	}

	sendKeys(a, "s")
	if a.sortBy != 1 || a.rows[0].Name != "docs" {
		t.Errorf("sort by name: first row %q", a.rows[0].Name)
	}
	if !strings.Contains(ansi.Strip(a.View()), "sorted by name") {
		t.Error("sort order not shown")
	}
}

func TestAnalyzeLargestFiles(t *testing.T) {
	sh := testShared(t, 60)
	homeTree(t, sh)
	a := startAnalyze(t, sh, "")

	sendKeys(a, "f")
	view := ansi.Strip(a.View())
	if !strings.Contains(view, "Largest files in") || !strings.Contains(view, filepath.Join("media", "big.mkv")) {
		t.Fatalf("largest files view:\n%s", view)
	}
	fitsWidth(t, a.View(), 60)

	sendKeys(a, "enter")
	if a.filesView || a.cur.Name != "media" || a.selected().Name != "big.mkv" {
		t.Error("enter on a file should jump to its folder")
	}
}

func TestAnalyzeTrash(t *testing.T) {
	sh := testShared(t, 80)
	homeTree(t, sh)
	a := startAnalyze(t, sh, "")
	before := a.tree.Size

	sendKeys(a, "d")
	if !strings.Contains(ansi.Strip(a.View()), "Move media") {
		t.Fatal("delete should ask first")
	}
	sendKeys(a, "n")
	if a.confirm != nil {
		t.Fatal("n should cancel")
	}

	sendKeys(a, "d", "y")
	if _, err := os.Stat(filepath.Join(sh.env.Home, "media")); !os.IsNotExist(err) {
		t.Error("media should be gone from home")
	}
	if _, err := os.Stat(filepath.Join(sh.env.DataDir, "Trash", "files", "media", "big.mkv")); err != nil {
		t.Error("media should be in the Trash")
	}
	if a.tree.Size >= before || a.rows[0].Name == "media" {
		t.Error("tree not updated after removal")
	}
	if !strings.Contains(ansi.Strip(a.View()), "Moved to Trash: media") {
		t.Error("no confirmation note")
	}
}

func TestAnalyzeGuardRefuses(t *testing.T) {
	sh := testShared(t, 80)
	homeTree(t, sh)
	a := startAnalyze(t, sh, "")
	for a.selected().Name != "Documents" {
		sendKeys(a, "down")
	}
	sendKeys(a, "d", "y")
	if _, err := os.Stat(filepath.Join(sh.env.Home, "Documents", "cv.pdf")); err != nil {
		t.Fatal("Documents must not be removed")
	}
	if !a.noteErr || !strings.Contains(a.note, "protected") {
		t.Errorf("expected a refusal note, got %q", a.note)
	}
}

func TestAnalyzeStates(t *testing.T) {
	sh := testShared(t, 80)
	a := newAnalyze(filepath.Join(sh.env.Home, "missing"))(sh).(*analyze)
	if !strings.Contains(ansi.Strip(a.View()), "Scanning") {
		t.Error("scanning state missing")
	}
	a = run(a, a.rescan()).(*analyze)
	if !strings.Contains(ansi.Strip(a.View()), "Cannot scan") {
		t.Error("error state missing")
	}

	empty := filepath.Join(sh.env.Home, "empty")
	os.Mkdir(empty, 0o755)
	a = startAnalyze(t, sh, empty)
	if !strings.Contains(ansi.Strip(a.View()), "This folder is empty") {
		t.Error("empty state missing")
	}
	sendKeys(a, "f")
	if !strings.Contains(ansi.Strip(a.View()), "No files here") {
		t.Error("empty files view missing")
	}
}
