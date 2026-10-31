package format

import "testing"

func TestSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{999, "999 B"},
		{1000, "1.0 kB"},
		{1500, "1.5 kB"},
		{999_949, "999.9 kB"},
		{999_950, "1.0 MB"},
		{12_345_678, "12.3 MB"},
		{5_000_000_000, "5.0 GB"},
		{2_500_000_000_000, "2.5 TB"},
		{-2048, "-2.0 kB"},
		{9_000_000_000_000_000_000, "9.0 EB"},
	}
	for _, c := range cases {
		if got := Size(c.in); got != c.want {
			t.Errorf("Size(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCount(t *testing.T) {
	cases := map[int64]string{0: "0", 999: "999", 1000: "1,000", 412345: "412,345", 1234567: "1,234,567", -1500: "-1,500"}
	for in, want := range cases {
		if got := Count(in); got != want {
			t.Errorf("Count(%d) = %q, want %q", in, got, want)
		}
	}
}
