package core

import (
	"crypto/rand"
	"github.com/Masterminds/semver/v3"
	"runtime"
	"strconv"
	"strings"
	"time"
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

// MaxCPUThreads calculates the maximum number of CPU threads available for use,
// taking into account a specified number of cores to reserve.
//
// The function determines the total number of CPU cores available on the system using runtime.NumCPU()
// and subtracts the number of reservedCores from this total. This calculation is intended to allow
// applications to reserve a certain number of cores for critical tasks, while using the remaining
// cores for other operations.
//
// Parameters:
// - reservedCores: An int specifying the number of CPU cores to reserve.
//
// Returns:
//   - An int representing the maximum number of CPU threads that can be used after reserving the specified
//     number of cores. This function ensures that at least one thread is always available, so it returns
//     a minimum of 1, even if the number of reservedCores equals or exceeds the total number of CPU cores.
//
// Example usage:
//
//	maxThreads := MaxCPUThreads(2)
//	fmt.Printf("Maximum available CPU threads: %d\n", maxThreads)
//
// Note: It's important to consider the workload and performance characteristics of your application
// when deciding how many cores to reserve. Reserving too many cores could lead to underutilization
// of system resources, while reserving too few could impact the performance of critical tasks.
func MaxCPUThreads(reservedCores int) int {
	numCPU := runtime.NumCPU()
	maxThreads := numCPU - reservedCores
	if maxThreads < 1 {
		return 1 // Ensure at least one thread is always available
	}
	return maxThreads
}

// CompareVersions compares two semantic version strings and returns the comparison result.
//
// The function takes two version strings, v1 and v2, as input and attempts to parse them into semantic versions.
// It then compares these versions according to semantic versioning rules:
//
// - If v1 is less than v2, it returns -1.
// - If v1 is equal to v2, it returns 0.
// - If v1 is greater than v2, it returns 1.
//
// This function panics if either version string cannot be parsed. This means that callers must ensure
// the version strings are valid semantic versions before calling CompareVersions.
//
// Example usage:
//
//	result := CompareVersions("1.0.0", "2.0.0")
//	if result < 0 {
//	    fmt.Println("Version 1 is older than Version 2")
//	} else if result > 0 {
//	    fmt.Println("Version 1 is newer than Version 2")
//	} else {
//	    fmt.Println("Version 1 and Version 2 are equal")
//	}
//
// Note: It's highly recommended to handle versions in a way that avoids panicking for invalid inputs.
// Consider validating version strings or using a safer approach to error handling.
func CompareVersions(v1, v2 string) int {
	version1, err := semver.NewVersion(v1)
	if err != nil {
		panic("Error parsing version 1: " + err.Error())
	}

	version2, err := semver.NewVersion(v2)
	if err != nil {
		panic("Error parsing version 2: " + err.Error())
	}

	return version1.Compare(version2)
}
