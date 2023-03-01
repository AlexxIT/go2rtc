module github.com/AlexxIT/go2rtc

go 1.19

require (
	github.com/brutella/hap v0.0.17
	github.com/deepch/vdk v0.0.19
	github.com/gorilla/websocket v1.5.0
	github.com/hashicorp/mdns v1.0.5
	github.com/pion/ice/v2 v2.3.1
	github.com/pion/interceptor v0.1.12
	github.com/pion/rtcp v1.2.10
	github.com/pion/rtp v1.7.13
	github.com/pion/sdp/v3 v3.0.6
	github.com/pion/srtp/v2 v2.0.12
	github.com/pion/stun v0.4.0
	github.com/pion/webrtc/v3 v3.1.56
	github.com/rs/zerolog v1.29.0
	github.com/stretchr/testify v1.8.1
	github.com/tadglines/go-pkgs v0.0.0-20210623144937-b983b20f54f9
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/brutella/dnssd v1.2.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-chi/chi v1.5.4 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/kr/pretty v0.2.1 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/miekg/dns v1.1.50 // indirect
	github.com/pion/datachannel v1.5.5 // indirect
	github.com/pion/dtls/v2 v2.2.6 // indirect
	github.com/pion/logging v0.2.2 // indirect
	github.com/pion/mdns v0.0.7 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/sctp v1.8.6 // indirect
	github.com/pion/transport/v2 v2.0.2 // indirect
	github.com/pion/turn/v2 v2.1.0 // indirect
	github.com/pion/udp/v2 v2.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/xiam/to v0.0.0-20200126224905-d60d31e03561 // indirect
	golang.org/x/crypto v0.6.0 // indirect
	golang.org/x/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4 // indirect
	golang.org/x/net v0.7.0 // indirect
	golang.org/x/sys v0.5.0 // indirect
	golang.org/x/text v0.7.0 // indirect
	golang.org/x/tools v0.1.12 // indirect
)

replace (
	// windows support: https://github.com/brutella/dnssd/pull/35
	github.com/brutella/dnssd v1.2.2 => github.com/rblenkinsopp/dnssd v1.2.3-0.20220516082132-0923f3c787a1
	// RTP tlv8 fix
	github.com/brutella/hap v0.0.17 => github.com/AlexxIT/hap v0.0.15-0.20221108133010-d8a45b7a7045
	// fix reading AAC config bytes
	github.com/deepch/vdk v0.0.19 => github.com/AlexxIT/vdk v0.0.18-0.20221108193131-6168555b4f92
)
