package core

import (
	"crypto/rand"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	BufferSize      = 64 * 1024 // 64K
	ConnDialTimeout = 5 * time.Second
	ConnDeadline    = 5 * time.Second
	ProbeTimeout    = 5 * time.Second
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

func Before(s, sep string) string {
	if i := strings.Index(s, sep); i > 0 {
		return s[:i]
	}
	return s
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

// ParseByte - fast parsing string to byte function
func ParseByte(s string) (b byte) {
	for i, ch := range []byte(s) {
		ch -= '0'
		if ch > 9 {
			return 0
		}
		if i > 0 {
			b *= 10
		}
		b += ch
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
