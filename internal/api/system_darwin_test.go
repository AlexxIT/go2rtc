//go:build darwin

package api

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestGetMemoryInfo(t *testing.T) {
	total, used := getMemoryInfo()

	if total == 0 {
		t.Fatal("mem_total is 0")
	}
	if total < 512*1024*1024 {
		t.Fatalf("mem_total too small: %d", total)
	}

	// total should match sysctl64("hw.memsize")
	expectedTotal := sysctl64("hw.memsize")
	if total != expectedTotal {
		t.Errorf("mem_total %d != hw.memsize %d", total, expectedTotal)
	}

	if used == 0 {
		t.Fatal("mem_used is 0")
	}
	if used > total {
		t.Fatalf("mem_used (%d) > mem_total (%d)", used, total)
	}

	// cross-check: used should be >= wired+active pages (minimum real usage)
	pageSize, _ := syscall.SysctlUint32("hw.pagesize")
	wired := vmStatPages("Pages wired down")
	active := vmStatPages("Pages active")
	minUsed := (wired + active) * uint64(pageSize)

	if used < minUsed/2 {
		t.Errorf("mem_used (%d) is less than half of wired+active (%d)", used, minUsed)
	}

	avail := total - used
	t.Logf("RAM total: %.1f GB, used: %.1f GB, avail: %.1f GB",
		float64(total)/1024/1024/1024,
		float64(used)/1024/1024/1024,
		float64(avail)/1024/1024/1024)
}

func TestGetCPUUsage(t *testing.T) {
	usage := getCPUUsage()

	// cross-check with sysctl vm.loadavg
	out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		t.Fatal("sysctl vm.loadavg:", err)
	}

	// format: { 4.24 4.57 5.76 } or { 4,24 4,57 5,76 }
	s := strings.Trim(string(out), "{ }\n")
	fields := strings.Fields(s)
	if len(fields) < 1 {
		t.Fatal("cannot parse vm.loadavg:", string(out))
	}
	load1Str := strings.ReplaceAll(fields[0], ",", ".")
	load1, err := strconv.ParseFloat(load1Str, 64)
	if err != nil {
		t.Fatal("parse load1:", err)
	}

	numCPU := float64(runtime.NumCPU())
	expected := load1 / numCPU * 100
	if expected > 100 {
		expected = 100
	}

	if usage < 0 || usage > 100 {
		t.Fatalf("cpu_usage out of range: %.1f%%", usage)
	}

	// allow 15% absolute deviation (load average fluctuates between reads)
	diff := usage - expected
	if diff < 0 {
		diff = -diff
	}
	if diff > 15 {
		t.Errorf("cpu_usage %.1f%% deviates from expected %.1f%% (load1=%.2f, cpus=%d) by %.1f%%",
			usage, expected, load1, int(numCPU), diff)
	}

	t.Logf("CPU usage: %.1f%%, expected: %.1f%% (load1=%.2f, cpus=%d)",
		usage, expected, load1, int(numCPU))
}

func TestVmStatPages(t *testing.T) {
	inactive := vmStatPages("Pages inactive")
	if inactive == 0 {
		t.Error("Pages inactive returned 0")
	}

	free := vmStatPages("Pages free")
	if free == 0 {
		t.Error("Pages free returned 0")
	}

	bogus := vmStatPages("Pages nonexistent")
	if bogus != 0 {
		t.Errorf("nonexistent key returned %d", bogus)
	}

	t.Logf("inactive=%d, free=%d pages", inactive, free)
}
