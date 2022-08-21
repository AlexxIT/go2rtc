@SET GOOS=linux
@SET GOARCH=amd64
cd ..
go build -ldflags "-s -w" -trimpath && upx-3.96 go2rtc