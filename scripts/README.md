## Versions

[Go 1.20](https://go.dev/doc/go1.20) is last version with support Windows 7 and macOS 10.13.
Go 1.21 support only Windows 10 and macOS 10.15.

So we will set `go 1.20` (minimum version) inside `go.mod` file. And will use env `GOTOOLCHAIN=go1.20.14` for building
`win32` and `mac_amd64` binaries. All other binaries will use latest go version.

## Build

- UPX-3.96 pack broken bin for `linux_mipsel`
- UPX-3.95 pack broken bin for `mac_amd64`
- UPX pack broken bin for `mac_arm64`
- UPX windows pack is recognised by anti-viruses as malicious
- `aarch64` = `arm64`
- `armv7` = `arm`

## Go

```
go get -u
go mod tidy
go mod why github.com/pion/rtcp
go list -deps .\cmd\go2rtc_rtsp\
./goweight
```

## Dependencies

```
- gopkg.in/yaml.v3
  - github.com/kr/pretty
- github.com/AlexxIT/go2rtc/pkg/hap
  - github.com/tadglines/go-pkgs
  - golang.org/x/crypto
- github.com/AlexxIT/go2rtc/pkg/mdns
  - github.com/miekg/dns
- github.com/AlexxIT/go2rtc/pkg/pcm
  - github.com/sigurn/crc16
  - github.com/sigurn/crc8
- github.com/pion/ice/v2
  - github.com/google/uuid
  - github.com/wlynxg/anet
- github.com/rs/zerolog
  - github.com/mattn/go-colorable
  - github.com/mattn/go-isatty
- github.com/stretchr/testify
  - github.com/davecgh/go-spew
  - github.com/pmezard/go-difflib
- ???
  - golang.org/x/mod
  - golang.org/x/net
  - golang.org/x/sys
  - golang.org/x/tools
```

## Licenses

- github.com/asticode/go-astits - MIT
- github.com/expr-lang/expr - MIT
- github.com/gorilla/websocket - BSD-2
- github.com/mattn/go-isatty - MIT
- github.com/miekg/dns - BSD-3
- github.com/pion/ice/v2 - MIT
- github.com/pion/interceptor - MIT
- github.com/pion/rtcp - MIT
- github.com/pion/rtp - MIT
- github.com/pion/sdp/v3 - MIT
- github.com/pion/srtp/v2 - MIT
- github.com/pion/stun - MIT
- github.com/pion/webrtc/v3 - MIT
- github.com/rs/zerolog - MIT
- github.com/sigurn/crc16 - MIT
- github.com/sigurn/crc8 - MIT
- github.com/stretchr/testify - MIT
- github.com/tadglines/go-pkgs - Apache
- golang.org/x/crypto - BSD-3
- gopkg.in/yaml.v3 - MIT and Apache
- github.com/asticode/go-astikit - MIT
- github.com/davecgh/go-spew - ISC (BSD/MIT like)
- github.com/google/uuid - BSD-3
- github.com/kr/pretty - MIT
- github.com/mattn/go-colorable - MIT
- github.com/pmezard/go-difflib - ???
- github.com/wlynxg/anet - BSD-3
- golang.org/x/mod - BSD-3
- golang.org/x/net - BSD-3
- golang.org/x/sync - BSD-3
- golang.org/x/sys - BSD-3
- golang.org/x/tools - BSD-3

## Virus

- https://go.dev/doc/faq#virus
- https://groups.google.com/g/golang-nuts/c/lPwiWYaApSU

## Useful links

- https://github.com/golang-standards/project-layout
- https://github.com/micro/micro
- https://github.com/golang/go/wiki/GoArm
- https://gist.github.com/asukakenji/f15ba7e588ac42795f421b48b8aede63
- https://en.wikipedia.org/wiki/AArch64
- https://stackoverflow.com/questions/22267189/what-does-the-w-flag-mean-when-passed-in-via-the-ldflags-option-to-the-go-comman
