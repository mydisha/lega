package safety

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadWhitelist(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "whitelist")

	got, err := LoadWhitelist(file, "/home/u")
	if err != nil || got != nil {
		t.Fatalf("missing file: %v, %v", got, err)
	}

	os.WriteFile(file, []byte("# keep these\n\n~/.cache/JetBrains\n/var/log/*.gz\n  ~/x/../y  \n"), 0o644)
	got, err = LoadWhitelist(file, "/home/u")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/home/u/.cache/JetBrains", "/var/log/*.gz", "/home/u/y"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLoadWhitelistErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "whitelist")

	os.WriteFile(file, []byte("relative/path\n"), 0o644)
	if _, err := LoadWhitelist(file, "/home/u"); err == nil {
		t.Error("relative line should fail")
	}

	os.WriteFile(file, []byte("/bad/[pattern\n"), 0o644)
	if _, err := LoadWhitelist(file, "/home/u"); err == nil {
		t.Error("bad glob should fail")
	}

	if os.Geteuid() != 0 {
		os.WriteFile(file, []byte("/ok\n"), 0o644)
		os.Chmod(file, 0)
		if _, err := LoadWhitelist(file, "/home/u"); err == nil {
			t.Error("unreadable whitelist must be an error, not an empty list")
		}
	}
}
