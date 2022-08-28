@SET GOOS=linux
@SET GOARCH=arm
@cd ..
del go2rtc
go build -ldflags "-s -w" -trimpath