PLATFORM_TARGETS := \
	build_linux_amd64 build_linux_386 build_linux_arm64 \
	build_linux_armv7 build_linux_armv6 build_linux_mipsle build_linux_riscv64 \
	build_windows_amd64 build_windows_386 build_windows_arm64 \
	build_darwin_amd64 build_darwin_arm64 \
	build_freebsd_amd64 build_freebsd_arm64 build_freebsd_riscv64

.PHONY: build build_all build_release run test vet update test_race_win64 $(PLATFORM_TARGETS)
.NOTPARALLEL: build_all build_release

build:
	go build -ldflags="-s -w" -trimpath

run:
	go run .

test:
	go test ./... -timeout 15s

vet:
	go vet ./...

update:
	go get -u
	go mod tidy

test_race_windows:
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 go test ./... -race -timeout 15s

define BUILD_BIN
build_$(1):
	CGO_ENABLED=0 GOOS=$(2) GOARCH=$(3) GOARM=$(4) go build -ldflags="-s -w" -trimpath -o $(5)
endef

$(eval $(call BUILD_BIN,linux_amd64,linux,amd64,,go2rtc_linux_amd64))
$(eval $(call BUILD_BIN,linux_386,linux,386,,go2rtc_linux_i386))
$(eval $(call BUILD_BIN,linux_arm64,linux,arm64,,go2rtc_linux_arm64))
$(eval $(call BUILD_BIN,linux_armv7,linux,arm,7,go2rtc_linux_arm))
$(eval $(call BUILD_BIN,linux_armv6,linux,arm,6,go2rtc_linux_armv6))
$(eval $(call BUILD_BIN,linux_mipsle,linux,mipsle,,go2rtc_linux_mipsel))
$(eval $(call BUILD_BIN,linux_riscv64,linux,riscv64,,go2rtc_linux_riscv64))

$(eval $(call BUILD_BIN,windows_amd64,windows,amd64,,go2rtc_windows_amd64.exe))
$(eval $(call BUILD_BIN,windows_arm64,windows,arm64,,go2rtc_windows_arm64.exe))
$(eval $(call BUILD_BIN,windows_386,windows,386,,go2rtc_windows_386.exe))

$(eval $(call BUILD_BIN,darwin_amd64,darwin,amd64,,go2rtc_darwin_amd64))
$(eval $(call BUILD_BIN,darwin_arm64,darwin,arm64,,go2rtc_darwin_arm64))
$(eval $(call BUILD_BIN,freebsd_amd64,freebsd,amd64,,go2rtc_freebsd_amd64))
$(eval $(call BUILD_BIN,freebsd_arm64,freebsd,arm64,,go2rtc_freebsd_arm64))
$(eval $(call BUILD_BIN,freebsd_riscv64,freebsd,riscv64,,go2rtc_freebsd_riscv64))

build_all: $(PLATFORM_TARGETS)

# Use UPX where it is well supported.
# Use tar.gz to preserve file permissions after extraction.
build_release: build_all
	upx --best --lzma go2rtc_linux_amd64
	upx --best --lzma go2rtc_linux_i386
	upx --best --lzma go2rtc_linux_arm64
	upx --best --lzma go2rtc_linux_arm
	upx --best --lzma go2rtc_linux_armv6
	upx --best --lzma go2rtc_linux_mipsel
	zip -9 --move go2rtc_win64.zip go2rtc_windows_amd64.exe
	zip -9 --move go2rtc_win32.zip go2rtc_windows_386.exe
	zip -9 --move go2rtc_win_arm64.zip go2rtc_windows_arm64.exe
	tar --mode=755 --remove-files -czf go2rtc_darwin_amd64.tar.gz go2rtc_darwin_amd64
	tar --mode=755 --remove-files -czf go2rtc_darwin_arm64.tar.gz go2rtc_darwin_arm64
	tar --mode=755 --remove-files -czf go2rtc_freebsd_amd64.tar.gz go2rtc_freebsd_amd64
	tar --mode=755 --remove-files -czf go2rtc_freebsd_arm64.tar.gz go2rtc_freebsd_arm64
	tar --mode=755 --remove-files -czf go2rtc_freebsd_riscv64.tar.gz go2rtc_freebsd_riscv64
	tar --mode=755 --remove-files -czf go2rtc_linux_riscv64.tar.gz go2rtc_linux_riscv64
