#!/bin/sh

check_command() {
    if ! command -v $1 &> /dev/null
    then
        echo "Error: $1 could not be found. Please install it."
        exit 1
    fi
}

# Check for required commands
check_command go
check_command 7z
check_command upx

# Windows amd64
export GOOS=windows
export GOARCH=amd64
FILENAME="go2rtc_win64.zip"
go build -ldflags "-s -w" -trimpath && 7z a -mx9 -bso0 -sdel $FILENAME go2rtc.exe

# Windows 386
export GOOS=windows
export GOARCH=386
FILENAME="go2rtc_win32.zip"
go build -ldflags "-s -w" -trimpath && 7z a -mx9 -bso0 -sdel $FILENAME go2rtc.exe

# Windows arm64
export GOOS=windows
export GOARCH=arm64
FILENAME="go2rtc_win_arm64.zip"
go build -ldflags "-s -w" -trimpath && 7z a -mx9 -bso0 -sdel $FILENAME go2rtc.exe

# Linux amd64
export GOOS=linux
export GOARCH=amd64
FILENAME="go2rtc_linux_amd64"
go build -ldflags "-s -w" -trimpath -o $FILENAME && upx --lzma --force-overwrite -q --no-progress $FILENAME

# Linux 386
export GOOS=linux
export GOARCH=386
FILENAME="go2rtc_linux_i386"
go build -ldflags "-s -w" -trimpath -o $FILENAME && upx --lzma --force-overwrite -q --no-progress $FILENAME

# Linux arm64
export GOOS=linux
export GOARCH=arm64
FILENAME="go2rtc_linux_arm64"
go build -ldflags "-s -w" -trimpath -o $FILENAME && upx --lzma --force-overwrite -q --no-progress $FILENAME

# Linux arm v7
export GOOS=linux
export GOARCH=arm
export GOARM=7
FILENAME="go2rtc_linux_arm"
go build -ldflags "-s -w" -trimpath -o $FILENAME && upx --lzma --force-overwrite -q --no-progress $FILENAME

# Linux arm v6
export GOOS=linux
export GOARCH=arm
export GOARM=6
FILENAME="go2rtc_linux_armv6"
go build -ldflags "-s -w" -trimpath -o $FILENAME && upx --lzma --force-overwrite -q --no-progress $FILENAME

# Linux mipsle
export GOOS=linux
export GOARCH=mipsle
FILENAME="go2rtc_linux_mipsel"
go build -ldflags "-s -w" -trimpath -o $FILENAME && upx --lzma --force-overwrite -q --no-progress $FILENAME

# Darwin amd64
export GOOS=darwin
export GOARCH=amd64
FILENAME="go2rtc_mac_amd64.zip"
go build -ldflags "-s -w" -trimpath && 7z a -mx9 -bso0 -sdel $FILENAME go2rtc

# Darwin arm64
export GOOS=darwin
export GOARCH=arm64
FILENAME="go2rtc_mac_arm64.zip"
go build -ldflags "-s -w" -trimpath && 7z a -mx9 -bso0 -sdel $FILENAME go2rtc