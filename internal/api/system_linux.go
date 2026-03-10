//go:build linux

package api

import (
	"bytes"
	"os"
	"strconv"
	"strings"
)

func getMemoryInfo() (total, used uint64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}

	var memTotal, memAvailable uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			memTotal = val * 1024 // kB to bytes
		case "MemAvailable:":
			memAvailable = val * 1024
		}
	}

	if memTotal > 0 && memAvailable <= memTotal {
		return memTotal, memTotal - memAvailable
	}
	return memTotal, 0
}

// previous CPU times for delta calculation
var prevIdle, prevTotal uint64

func getCPUUsage() float64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}

	// first line: cpu  user nice system idle iowait irq softirq steal
	idx := bytes.IndexByte(data, '\n')
	if idx < 0 {
		return 0
	}
	line := string(data[:idx])
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0
	}

	var total, idle uint64
	for i := 1; i < len(fields); i++ {
		val, err := strconv.ParseUint(fields[i], 10, 64)
		if err != nil {
			continue
		}
		total += val
		if i == 4 { // idle is the 4th value (index 4 in fields, 1-based field 4)
			idle = val
		}
	}

	deltaTotal := total - prevTotal
	deltaIdle := idle - prevIdle
	prevIdle = idle
	prevTotal = total

	if deltaTotal == 0 {
		return 0
	}

	return float64(deltaTotal-deltaIdle) / float64(deltaTotal) * 100
}
