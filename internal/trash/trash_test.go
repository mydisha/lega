package trash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMove(t *testing.T) {
	home := t.TempDir()
	tr := Trash{DataDir: filepath.Join(home, ".local", "share"), UID: os.Getuid()}

	src := filepath.Join(home, "Downloads", "my file ü.iso")
	os.MkdirAll(filepath.Dir(src), 0o755)
	os.WriteFile(src, []byte("data"), 0o644)

	dir, err := tr.Move(src)
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(tr.DataDir, "Trash") {
		t.Errorf("used %s, want home trash", dir)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source still exists")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "files", "my file ü.iso")); string(b) != "data" {
		t.Error("file not in trash/files")
	}
	info, err := os.ReadFile(filepath.Join(dir, "info", "my file ü.iso.trashinfo"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(info)
	if !strings.HasPrefix(s, "[Trash Info]\n") || !strings.Contains(s, "my%20file%20%C3%BC.iso\n") || !strings.Contains(s, "\nDeletionDate=") {
		t.Errorf("bad trashinfo:\n%s", s)
	}
}

func TestMoveNameClash(t *testing.T) {
	home := t.TempDir()
	tr := Trash{DataDir: filepath.Join(home, "data"), UID: os.Getuid()}
	for i := 0; i < 3; i++ {
		p := filepath.Join(home, "report.pdf")
		os.WriteFile(p, []byte{byte(i)}, 0o644)
		if _, err := tr.Move(p); err != nil {
			t.Fatal(err)
		}
	}
	dot := filepath.Join(home, ".hidden")
	os.WriteFile(dot, nil, 0o644)
	tr.Move(dot)
	os.WriteFile(dot, nil, 0o644)
	tr.Move(dot)

	files := filepath.Join(tr.DataDir, "Trash", "files")
	for _, name := range []string{"report.pdf", "report.2.pdf", "report.3.pdf", ".hidden", ".hidden.2"} {
		if _, err := os.Stat(filepath.Join(files, name)); err != nil {
			t.Errorf("missing %s", name)
		}
		if _, err := os.Stat(filepath.Join(tr.DataDir, "Trash", "info", name+".trashinfo")); err != nil {
			t.Errorf("missing info for %s", name)
		}
	}
}

func TestMoveFolder(t *testing.T) {
	home := t.TempDir()
	tr := Trash{DataDir: filepath.Join(home, "data"), UID: os.Getuid()}
	dir := filepath.Join(home, "old-project")
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	if _, err := tr.Move(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(tr.DataDir, "Trash", "files", "old-project", "src")); err != nil {
		t.Error("folder not moved whole")
	}
}

func TestMoveErrors(t *testing.T) {
	tr := Trash{DataDir: t.TempDir(), UID: os.Getuid()}
	if _, err := tr.Move("/does/not/exist"); err == nil {
		t.Error("expected error for missing file")
	}
	// A trash folder that is a symlink must be refused.
	data := t.TempDir()
	os.Symlink(t.TempDir(), filepath.Join(data, "Trash"))
	f := filepath.Join(t.TempDir(), "x")
	os.WriteFile(f, nil, 0o644)
	if _, err := (Trash{DataDir: data, UID: os.Getuid()}).Move(f); err == nil {
		t.Error("symlinked trash folder should be refused")
	}
}

func TestMountTop(t *testing.T) {
	dir := t.TempDir()
	if got := mountTop(filepath.Join(dir, "a"), device(dir)); got == "" || !strings.HasPrefix(dir, got) {
		t.Errorf("mountTop = %q for %q", got, dir)
	}
}
