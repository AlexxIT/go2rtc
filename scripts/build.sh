#!/bin/sh

set -eu

check_command() {
    if ! command -v "$1" >/dev/null
    then
        echo "Error: $1 could not be found. Please install it." >&2
        return 1
    fi
}

# Check for required commands
check_command go
check_command 7z
check_command upx

set -x

export CGO_ENABLED=0

# Windows amd64
FILENAME="go2rtc_win64.zip"
GOOS=windows GOARCH=amd64 \
    go build -ldflags "-s -w" -trimpath
7z a -mx9 -bso0 -sdel $FILENAME go2rtc.exe

# Windows 386
FILENAME="go2rtc_win32.zip"
GOOS=windows GOARCH=386 GOTOOLCHAIN=go1.20.14 \
    go build -ldflags "-s -w" -trimpath
7z a -mx9 -bso0 -sdel $FILENAME go2rtc.exe

# Windows arm64
FILENAME="go2rtc_win_arm64.zip"
GOOS=windows GOARCH=arm64 \
    go build -ldflags "-s -w" -trimpath
7z a -mx9 -bso0 -sdel $FILENAME go2rtc.exe

# Linux amd64
FILENAME="go2rtc_linux_amd64"
GOOS=linux GOARCH=amd64 \
    go build -ldflags "-s -w" -trimpath -o $FILENAME
upx --lzma --force-overwrite -q --no-progress $FILENAME

# Linux 386
FILENAME="go2rtc_linux_i386"
GOOS=linux GOARCH=386 \
    go build -ldflags "-s -w" -trimpath -o $FILENAME
upx --lzma --force-overwrite -q --no-progress $FILENAME

# Linux arm64
FILENAME="go2rtc_linux_arm64"
GOOS=linux GOARCH=arm64 \
    go build -ldflags "-s -w" -trimpath -o $FILENAME
upx --lzma --force-overwrite -q --no-progress $FILENAME

# Linux arm v7
FILENAME="go2rtc_linux_arm"
GOOS=linux GOARCH=arm GOARM=7 \
    go build -ldflags "-s -w" -trimpath -o $FILENAME
upx --lzma --force-overwrite -q --no-progress $FILENAME

# Linux arm v6
FILENAME="go2rtc_linux_armv6"
GOOS=linux GOARCH=arm GOARM=6 \
    go build -ldflags "-s -w" -trimpath -o $FILENAME
upx --lzma --force-overwrite -q --no-progress $FILENAME

# Linux mipsle
FILENAME="go2rtc_linux_mipsel"
GOOS=linux GOARCH=mipsle \
    go build -ldflags "-s -w" -trimpath -o $FILENAME
upx --lzma --force-overwrite -q --no-progress $FILENAME

# Darwin amd64
FILENAME="go2rtc_mac_amd64.zip"
GOOS=darwin GOARCH=amd64 GOTOOLCHAIN=go1.20.14 \
    go build -ldflags "-s -w" -trimpath
7z a -mx9 -bso0 -sdel $FILENAME go2rtc

# Darwin arm64
FILENAME="go2rtc_mac_arm64.zip"
GOOS=darwin GOARCH=arm64 \
    go build -ldflags "-s -w" -trimpath
7z a -mx9 -bso0 -sdel $FILENAME go2rtc

# FreeBSD amd64
FILENAME="go2rtc_freebsd_amd64.zip"
GOOS=freebsd GOARCH=amd64 \
    go build -ldflags "-s -w" -trimpath
7z a -mx9 -bso0 -sdel $FILENAME go2rtc

# FreeBSD arm64
FILENAME="go2rtc_freebsd_arm64.zip"
GOOS=freebsd GOARCH=arm64 \
    go build -ldflags "-s -w" -trimpath
7z a -mx9 -bso0 -sdel $FILENAME go2rtc
