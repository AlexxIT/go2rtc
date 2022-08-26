@SET GOOS=darwin
@SET GOARCH=amd64
@cd ..
del go2rtc
go build -ldflags "-s -w" -trimpath