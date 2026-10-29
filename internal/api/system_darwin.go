//go:build darwin

package api

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

func getMemoryInfo() (total, used uint64) {
	total = sysctl64("hw.memsize")
	if total == 0 {
		return 0, 0
	}

	pageSize, err := syscall.SysctlUint32("hw.pagesize")
	if err != nil {
		return total, 0
	}

	freeCount, _ := syscall.SysctlUint32("vm.page_free_count")
	purgeableCount, _ := syscall.SysctlUint32("vm.page_purgeable_count")
	speculativeCount, _ := syscall.SysctlUint32("vm.page_speculative_count")

	// inactive pages not available via sysctl, parse vm_stat
	inactiveCount := vmStatPages("Pages inactive")

	available := uint64(freeCount+purgeableCount+speculativeCount)*uint64(pageSize) +
		inactiveCount*uint64(pageSize)
	if available > total {
		return total, 0
	}
	return total, total - available
}

// vmStatPages parses vm_stat output for a specific counter
func vmStatPages(key string) uint64 {
	out, err := exec.Command("vm_stat").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, key) {
			// format: "Pages inactive:          479321."
			parts := strings.Split(line, ":")
			if len(parts) < 2 {
				continue
			}
			s := strings.TrimSpace(parts[1])
			s = strings.TrimSuffix(s, ".")
			val, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				return 0
			}
			return val
		}
	}
	return 0
}

func sysctl64(name string) uint64 {
	s, err := syscall.Sysctl(name)
	if err != nil {
		return 0
	}
	b := []byte(s)
	for len(b) < 8 {
		b = append(b, 0)
	}
	return *(*uint64)(unsafe.Pointer(&b[0]))
}

func getCPUUsage() float64 {
	s, err := syscall.Sysctl("vm.loadavg")
	if err != nil {
		return 0
	}

	raw := []byte(s)
	for len(raw) < 24 {
		raw = append(raw, 0)
	}

	// struct loadavg { fixpt_t ldavg[3]; long fscale; }
	ldavg0 := *(*uint32)(unsafe.Pointer(&raw[0]))
	fscale := *(*int64)(unsafe.Pointer(&raw[16]))

	if fscale == 0 {
		return 0
	}

	load1 := float64(ldavg0) / float64(fscale)
	numCPU := float64(runtime.NumCPU())

	usage := load1 / numCPU * 100
	if usage > 100 {
		usage = 100
	}
	return usage
}
