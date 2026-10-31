package oplog

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWrite(t *testing.T) {
	t.Setenv("LEGA_NO_OPLOG", "")
	l := Open(t.TempDir())
	when := time.Date(2026, 10, 31, 9, 30, 0, 0, time.UTC)
	if err := l.Write(Entry{Time: when, Action: "clean", Rule: "thumbnails", Path: "/home/u/.cache/thumbnails/a.png", Bytes: 4096}); err != nil {
		t.Fatal(err)
	}
	if err := l.Write(Entry{Time: when, Action: "trash", Path: "/home/u/odd\tname\n.txt", Err: errors.New("permission\ndenied")}); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(l.Path())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %q", b)
	}
	if lines[0] != "2026-10-31T09:30:00Z\tclean\tthumbnails\t4096\tok\t/home/u/.cache/thumbnails/a.png" {
		t.Errorf("line 1 = %q", lines[0])
	}
	if lines[1] != `2026-10-31T09:30:00Z	trash	-	0	error: permission\ndenied	/home/u/odd\tname\n.txt` {
		t.Errorf("line 2 = %q", lines[1])
	}

	info, _ := os.Stat(l.Path())
	if info.Mode().Perm() != 0o600 {
		t.Errorf("log should be private, mode %v", info.Mode().Perm())
	}
}

func TestDisabled(t *testing.T) {
	t.Setenv("LEGA_NO_OPLOG", "1")
	l := Open(t.TempDir())
	if l != nil || l.Path() != "" {
		t.Fatal("LEGA_NO_OPLOG=1 should disable the log")
	}
	if err := l.Write(Entry{Action: "clean"}); err != nil {
		t.Errorf("nil log write: %v", err)
	}
}
