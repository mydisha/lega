package platform

import (
	"strings"
	"testing"
)

func TestCleanCPUName(t *testing.T) {
	cases := map[string]string{
		"Intel(R) Core(TM) i7-8550U CPU @ 1.80GHz":   "Intel Core i7-8550U",
		"12th Gen Intel(R) Core(TM) i7-1260P":        "12th Gen Intel Core i7-1260P",
		"AMD Ryzen 5 PRO 6650U with Radeon Graphics": "AMD Ryzen 5 PRO 6650U",
		"AMD Ryzen 7 7840U w/ Radeon  780M Graphics": "AMD Ryzen 7 7840U",
		"AMD Ryzen 5 5600X 6-Core Processor":         "AMD Ryzen 5 5600X",
		"Intel(R) Pentium(R) CPU N3710 @ 1.60GHz":    "Intel Pentium CPU N3710",
		"Intel(R) Celeron(R) N4020 CPU @ 1.10GHz":    "Intel Celeron N4020",
		"Raspberry Pi 4 Model B Rev 1.4":             "Raspberry Pi 4 Model B Rev 1.4",
		"  Genuine   Processor  ":                    "Genuine",
		"":                                           "",
		"龙芯 3A5000 (R)":                              "龙芯 3A5000",
	}
	for in, want := range cases {
		if got := CleanCPUName(in); got != want {
			t.Errorf("CleanCPUName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseCPUInfo(t *testing.T) {
	x86 := strings.Repeat("processor\t: 0\nvendor_id\t: AuthenticAMD\nmodel name\t: AMD Ryzen 5 PRO 6650U with Radeon Graphics\nflags\t\t: fpu vme\n\n", 12)
	model, n := ParseCPUInfo(strings.NewReader(x86))
	if model != "AMD Ryzen 5 PRO 6650U" || n != 12 {
		t.Errorf("x86: got %q, %d", model, n)
	}

	arm := "processor\t: 0\nBogoMIPS\t: 108.00\n\nprocessor\t: 1\nBogoMIPS\t: 108.00\n\nRevision\t: c03114\nModel\t\t: Raspberry Pi 4 Model B Rev 1.4\n"
	model, n = ParseCPUInfo(strings.NewReader(arm))
	if model != "Raspberry Pi 4 Model B Rev 1.4" || n != 2 {
		t.Errorf("arm: got %q, %d", model, n)
	}

	model, n = ParseCPUInfo(strings.NewReader(""))
	if model != "" || n != 0 {
		t.Errorf("empty: got %q, %d", model, n)
	}
}

func TestParseMemInfo(t *testing.T) {
	in := "MemTotal:       15096136 kB\nMemFree:          812344 kB\nMemAvailable:    7163512 kB\nBuffers:  12 kB\n"
	total, avail := ParseMemInfo(strings.NewReader(in))
	if total != 15096136*1024 || avail != 7163512*1024 {
		t.Errorf("got %d, %d", total, avail)
	}

	// Old kernels have no MemAvailable; garbage lines are skipped.
	total, avail = ParseMemInfo(strings.NewReader("MemTotal: 1000 kB\nMemAvailable: lots kB\nbroken\n"))
	if total != 1024000 || avail != 0 {
		t.Errorf("old kernel: got %d, %d", total, avail)
	}

	// Available is never reported above the total.
	total, avail = ParseMemInfo(strings.NewReader("MemAvailable: 2000 kB\nMemTotal: 1000 kB\n"))
	if avail != total {
		t.Errorf("avail %d above total %d", avail, total)
	}
}
