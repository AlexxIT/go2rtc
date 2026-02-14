//go:build !linux && !darwin && !windows

package api

func getMemoryInfo() (total, used uint64) {
	return 0, 0
}

func getCPUUsage() float64 {
	return 0
}
