package core

import (
	"crypto/rand"
	"fmt"

	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

const (
	BufferSize      = 64 * 1024 // 64K
	ConnDialTimeout = time.Second * 3
	ConnDeadline    = time.Second * 5
	ProbeTimeout    = time.Second * 3
)

// Now90000 - timestamp for Video (clock rate = 90000 samples per second)
func Now90000() uint32 {
	return uint32(time.Duration(time.Now().UnixNano()) * 90000 / time.Second)
}

const symbols = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-_"

// RandString base10 - numbers, base16 - hex, base36 - digits+letters
// base64 - URL safe symbols, base0 - crypto random
func RandString(size, base byte) string {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	if base == 0 {
		return string(b)
	}
	for i := byte(0); i < size; i++ {
		b[i] = symbols[b[i]%base]
	}
	return string(b)
}

func Any(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func Between(s, sub1, sub2 string) string {
	i := strings.Index(s, sub1)
	if i < 0 {
		return ""
	}
	s = s[i+len(sub1):]

	if i = strings.Index(s, sub2); i >= 0 {
		return s[:i]
	}

	return s
}

func Atoi(s string) (i int) {
	if s != "" {
		i, _ = strconv.Atoi(s)
	}
	return
}

func Assert(ok bool) {
	if !ok {
		_, file, line, _ := runtime.Caller(1)
		panic(file + ":" + strconv.Itoa(line))
	}
}

func Caller() string {
	_, file, line, _ := runtime.Caller(1)
	return file + ":" + strconv.Itoa(line)
}

// GetCPUUsage calculates the CPU usage percentage over a specified interval.
// It returns the average CPU usage as a float64 and any error encountered.
//
// The function works by first fetching the CPU usage percentage before the sleep interval,
// then calculating the average CPU usage over that interval. This is useful for monitoring
// or logging system performance metrics.
//
// Parameters:
// - interval: A time.Duration value specifying how long to measure CPU usage for.
//
// Returns:
// - A float64 representing the average CPU usage percentage over the interval.
// - An error if there was an issue fetching the CPU usage data.
func GetCPUUsage(interval time.Duration) (float64, error) {
	percentages, err := cpu.Percent(interval, false)
	if err != nil {
		return 0, err
	}

	if len(percentages) == 0 {
		return 0, fmt.Errorf("no CPU usage data available")
	}

	var total float64
	for _, percent := range percentages {
		total += percent
	}
	avgCPUUsage := total / float64(len(percentages))

	return avgCPUUsage, nil
}

// GetRAMUsage fetches the current virtual memory statistics.
// It returns a pointer to a VirtualMemoryStat struct containing detailed memory usage stats
// and any error encountered.
//
// This function is useful for retrieving comprehensive memory usage data, such as total and available RAM,
// used and free amounts, and various other metrics related to system memory performance.
//
// Returns:
// - A pointer to a mem.VirtualMemoryStat struct containing the memory usage statistics.
// - An error if there was an issue fetching the memory usage data.
func GetRAMUsage() (*mem.VirtualMemoryStat, error) {
	return mem.VirtualMemory()
}
