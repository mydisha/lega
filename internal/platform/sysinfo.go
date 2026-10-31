package platform

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// SysInfo is a short summary of the machine for the start screen. Each
// field is read on its own; one that cannot be read is left empty.
type SysInfo struct {
	Host     string
	Kernel   string
	CPU      string // model name, without trademarks and clock speed
	Threads  int
	MemTotal int64 // bytes
	MemAvail int64 // bytes the kernel can hand out without swapping
}

// ReadSysInfo reads the host name, kernel, CPU and memory from /proc.
func ReadSysInfo() SysInfo {
	var s SysInfo
	s.Host, _ = os.Hostname()
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		s.Kernel = strings.TrimSpace(string(b))
	}
	if f, err := os.Open("/proc/cpuinfo"); err == nil {
		s.CPU, s.Threads = ParseCPUInfo(f)
		f.Close()
	}
	if s.Threads == 0 {
		s.Threads = runtime.NumCPU()
	}
	if f, err := os.Open("/proc/meminfo"); err == nil {
		s.MemTotal, s.MemAvail = ParseMemInfo(f)
		f.Close()
	}
	return s
}

// ParseCPUInfo returns the CPU model and the number of logical CPUs listed
// in /proc/cpuinfo. ARM boards often have no "model name"; the board model
// is used then.
func ParseCPUInfo(r io.Reader) (model string, threads int) {
	var board string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		key, val, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "processor":
			threads++
		case "model name":
			if model == "" {
				model = val
			}
		case "Hardware", "Model":
			if board == "" {
				board = val
			}
		}
	}
	if model == "" {
		model = board
	}
	return CleanCPUName(model), threads
}

var (
	cpuTrademark = regexp.MustCompile(`(?i)\((r|tm)\)`)
	// Everything after these is clock speed or the built-in GPU.
	cpuTail  = regexp.MustCompile(`(?i)\s+(cpu\s+)?@.*$|\s+(with|w/)\s.*$`)
	cpuCores = regexp.MustCompile(`(?i)\s+\d+-core\s+processor\s*$|\s+(cpu|processor)\s*$`)
)

// CleanCPUName shortens "Intel(R) Core(TM) i7-8550U CPU @ 1.80GHz" to
// "Intel Core i7-8550U" and "AMD Ryzen 5 5600X 6-Core Processor" to
// "AMD Ryzen 5 5600X".
func CleanCPUName(s string) string {
	s = cpuTrademark.ReplaceAllString(s, "")
	s = cpuTail.ReplaceAllString(s, "")
	s = cpuCores.ReplaceAllString(s, "")
	return strings.Join(strings.Fields(s), " ")
}

// ParseMemInfo returns MemTotal and MemAvailable from /proc/meminfo, in
// bytes.
func ParseMemInfo(r io.Reader) (total, avail int64) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 2 {
			continue
		}
		kb, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil || kb < 0 {
			continue
		}
		switch f[0] {
		case "MemTotal:":
			total = kb * 1024
		case "MemAvailable:":
			avail = kb * 1024
		}
	}
	return total, min(avail, total)
}
