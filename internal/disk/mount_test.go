package disk

import (
	"os"
	"strings"
	"testing"
)

func TestParseMountInfo(t *testing.T) {
	f, err := os.Open("testdata/mountinfo")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := ParseMountInfo(f)
	if err != nil {
		t.Fatal(err)
	}
	want := []Mount{
		{Device: "/dev/nvme0n1p2", MountPoint: "/", FSType: "btrfs"},
		{Device: "/dev/nvme0n1p1", MountPoint: "/boot/efi", FSType: "vfat"},
		{Device: "/dev/sdb1", MountPoint: "/media/dias/My Backup", FSType: "exfat"},
		{Device: "/dev/sdc1", MountPoint: "/mnt/data", FSType: "ext4"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d mounts: %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("mount %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDuplicateKeepsShortestPath(t *testing.T) {
	in := "1 0 8:1 /sub /a/long/path rw - ext4 /dev/sda1 rw\n" +
		"2 0 8:1 / /data rw - ext4 /dev/sda1 rw\n"
	got, _ := ParseMountInfo(strings.NewReader(in))
	if len(got) != 1 || got[0].MountPoint != "/data" {
		t.Errorf("got %+v", got)
	}
}

func TestUnescape(t *testing.T) {
	cases := map[string]string{
		`plain`:         "plain",
		`a\040b`:        "a b",
		`tab\011x`:      "tab\tx",
		`back\134slash`: `back\slash`,
		`bad\09x`:       `bad\09x`,
		`end\04`:        `end\04`,
	}
	for in, want := range cases {
		if got := unescape(in); got != want {
			t.Errorf("unescape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUsedPercent(t *testing.T) {
	if p := (Mount{Total: 200, Used: 50}).UsedPercent(); p != 25 {
		t.Errorf("got %v", p)
	}
	if p := (Mount{}).UsedPercent(); p != 0 {
		t.Errorf("zero total should be 0, got %v", p)
	}
}

func TestUsageAndList(t *testing.T) {
	total, used, free, err := Usage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if total <= 0 || used < 0 || free < 0 || free > total {
		t.Errorf("odd usage: total=%d used=%d free=%d", total, used, free)
	}
	if _, _, _, err := Usage("/does/not/exist"); err == nil {
		t.Error("expected error for missing path")
	}
	mounts, err := List()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range mounts {
		if m.Total <= 0 || m.MountPoint == "" {
			t.Errorf("listed mount without usage: %+v", m)
		}
	}
}
