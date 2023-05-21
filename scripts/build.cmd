@ECHO OFF

@SET GOOS=windows
@SET GOARCH=amd64
@SET FILENAME=go2rtc_win64.zip
go build -ldflags "-s -w" -trimpath && 7z a -mx9 -sdel %FILENAME% go2rtc.exe

@SET GOOS=windows
@SET GOARCH=386
@SET FILENAME=go2rtc_win32.zip
go build -ldflags "-s -w" -trimpath && 7z a -mx9 -sdel %FILENAME% go2rtc.exe

@SET GOOS=windows
@SET GOARCH=arm64
@SET FILENAME=go2rtc_win_arm64.zip
go build -ldflags "-s -w" -trimpath && 7z a -mx9 -sdel %FILENAME% go2rtc.exe

@SET GOOS=linux
@SET GOARCH=amd64
@SET FILENAME=go2rtc_linux_amd64
go build -ldflags "-s -w" -trimpath -o %FILENAME% && upx %FILENAME%

@SET GOOS=linux
@SET GOARCH=386
@SET FILENAME=go2rtc_linux_i386
go build -ldflags "-s -w" -trimpath -o %FILENAME% && upx %FILENAME%

@SET GOOS=linux
@SET GOARCH=arm64
@SET FILENAME=go2rtc_linux_arm64
go build -ldflags "-s -w" -trimpath -o %FILENAME% && upx %FILENAME%

@SET GOOS=linux
@SET GOARCH=arm
@SET GOARM=7
@SET FILENAME=go2rtc_linux_arm
go build -ldflags "-s -w" -trimpath -o %FILENAME% && upx %FILENAME%

@SET GOOS=linux
@SET GOARCH=arm
@SET GOARM=6
@SET FILENAME=go2rtc_linux_armv6
go build -ldflags "-s -w" -trimpath -o %FILENAME% && upx %FILENAME%

@SET GOOS=linux
@SET GOARCH=mipsle
@SET FILENAME=go2rtc_linux_mipsel
go build -ldflags "-s -w" -trimpath -o %FILENAME% && upx %FILENAME%

@SET GOOS=darwin
@SET GOARCH=amd64
@SET FILENAME=go2rtc_mac_amd64.zip
go build -ldflags "-s -w" -trimpath && 7z a -mx9 -sdel %FILENAME% go2rtc

@SET GOOS=darwin
@SET GOARCH=arm64
@SET FILENAME=go2rtc_mac_arm64.zip
go build -ldflags "-s -w" -trimpath && 7z a -mx9 -sdel %FILENAME% go2rtc
