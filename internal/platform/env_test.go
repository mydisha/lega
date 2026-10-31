package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentEnvXDG(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "/custom/cache")
	t.Setenv("XDG_CONFIG_HOME", "relative/ignored")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	e, err := CurrentEnv()
	if err != nil {
		t.Fatal(err)
	}
	if e.Home != home || e.CacheDir != "/custom/cache" {
		t.Errorf("home/cache = %q, %q", e.Home, e.CacheDir)
	}
	if e.ConfigDir != filepath.Join(home, ".config") {
		t.Errorf("relative XDG_CONFIG_HOME should be ignored, got %q", e.ConfigDir)
	}
	if e.DataDir != filepath.Join(home, ".local/share") || e.StateDir != filepath.Join(home, ".local/state") {
		t.Errorf("data/state = %q, %q", e.DataDir, e.StateDir)
	}
}

func TestCurrentEnvRejectsRelativeHome(t *testing.T) {
	t.Setenv("HOME", "not/absolute")
	if _, err := CurrentEnv(); err == nil {
		t.Fatal("expected error for relative HOME")
	}
}

func TestUserDirs(t *testing.T) {
	home := t.TempDir()
	e := Env{Home: home, ConfigDir: filepath.Join(home, ".config")}

	if got := e.UserDirs()["DOCUMENTS"]; got != filepath.Join(home, "Documents") {
		t.Errorf("default DOCUMENTS = %q", got)
	}

	os.MkdirAll(e.ConfigDir, 0o755)
	conf := `# written by xdg-user-dirs-update
XDG_DOCUMENTS_DIR="$HOME/Dokumen"
XDG_PICTURES_DIR="$HOME/Gambar Saya"
XDG_PUBLICSHARE_DIR="$HOME/"
XDG_MUSIC_DIR="relative"
`
	if err := os.WriteFile(filepath.Join(e.ConfigDir, "user-dirs.dirs"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	d := e.UserDirs()
	if d["DOCUMENTS"] != filepath.Join(home, "Dokumen") || d["PICTURES"] != filepath.Join(home, "Gambar Saya") {
		t.Errorf("translated dirs not read: %v", d)
	}
	if _, ok := d["PUBLICSHARE"]; ok {
		t.Error("dir set to $HOME should be dropped")
	}
	if d["MUSIC"] != filepath.Join(home, "Music") {
		t.Errorf("relative value should keep default, got %q", d["MUSIC"])
	}
}

func TestProcesses(t *testing.T) {
	proc := t.TempDir()
	write := func(pid, comm, cmdline string) {
		dir := filepath.Join(proc, pid)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "comm"), []byte(comm+"\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "cmdline"), []byte(cmdline), 0o644)
	}
	write("100", "GeckoMain", "/snap/firefox/123/usr/lib/firefox/firefox\x00-contentproc")
	write("200", "code", "/usr/share/code/code\x00--type=zygote")
	write("self", "ignored", "ignored")
	os.MkdirAll(filepath.Join(proc, "300"), 0o755) // no comm or cmdline

	got, err := Processes(proc, os.Getuid())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"GeckoMain", "firefox", "code"} {
		if !got[want] {
			t.Errorf("missing %q in %v", want, got)
		}
	}
	if got["ignored"] {
		t.Error("non-numeric dirs must be skipped")
	}

	other, _ := Processes(proc, os.Getuid()+1)
	if len(other) != 0 {
		t.Errorf("processes of another uid should be skipped, got %v", other)
	}
}
