@SET GOOS=windows
@SET GOARCH=amd64
cd ..
go build -ldflags "-w -s" -trimpath