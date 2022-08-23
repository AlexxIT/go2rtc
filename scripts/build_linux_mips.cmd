@ECHO OFF
@SET GOOS=linux
@SET GOARCH=mipsle
cd ..
go build -ldflags "-s -w" -trimpath && upx-3.95 go2rtc