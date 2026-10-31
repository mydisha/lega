package platform

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeFcList puts an fc-list script first on PATH. It saves its arguments
// next to itself and runs body.
func fakeFcList(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$@\" > \"" + filepath.Join(dir, "args") + "\"\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "fc-list"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return dir
}

func TestNerdFontInstalled(t *testing.T) {
	dir := fakeFcList(t, `echo "MesloLGS NF"`)
	if !NerdFontInstalled() {
		t.Error("a font with the glyphs was listed")
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if got, want := string(args), nerdProbe+" family\n"; got != want {
		t.Errorf("fc-list args = %q, want %q", got, want)
	}

	fakeFcList(t, `echo "   "`)
	if NerdFontInstalled() {
		t.Error("blank output means no font")
	}
	fakeFcList(t, `echo "Broken"; exit 1`)
	if NerdFontInstalled() {
		t.Error("a failing fc-list means no font")
	}
	t.Setenv("PATH", t.TempDir())
	if NerdFontInstalled() {
		t.Error("no fc-list means no font")
	}
}
