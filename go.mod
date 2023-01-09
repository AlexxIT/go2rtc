module github.com/AlexxIT/go2rtc

go 1.19

require (
	github.com/brutella/hap v0.0.17
	github.com/deepch/vdk v0.0.19
	github.com/gorilla/websocket v1.5.0
	github.com/hashicorp/mdns v1.0.5
	github.com/pion/ice/v2 v2.2.6
	github.com/pion/interceptor v0.1.11
	github.com/pion/rtcp v1.2.9
	github.com/pion/rtp v1.7.13
	github.com/pion/sdp/v3 v3.0.5
	github.com/pion/srtp/v2 v2.0.10
	github.com/pion/stun v0.3.5
	github.com/pion/webrtc/v3 v3.1.43
	github.com/rs/zerolog v1.27.0
	github.com/stretchr/testify v1.7.1
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
	github.com/pion/datachannel v1.5.2 // indirect
	github.com/pion/dtls/v2 v2.1.5 // indirect
	github.com/pion/logging v0.2.2 // indirect
	github.com/pion/mdns v0.0.5 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/sctp v1.8.2 // indirect
	github.com/pion/transport v0.13.1 // indirect
	github.com/pion/turn/v2 v2.0.8 // indirect
	github.com/pion/udp v0.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/xiam/to v0.0.0-20200126224905-d60d31e03561 // indirect
	golang.org/x/crypto v0.0.0-20220516162934-403b01795ae8 // indirect
	golang.org/x/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4 // indirect
	golang.org/x/net v0.0.0-20220630215102-69896b714898 // indirect
	golang.org/x/sys v0.0.0-20220811171246-fbc7d0a398ab // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/tools v0.1.11 // indirect
)

replace (
	// windows support: https://github.com/brutella/dnssd/pull/35
	github.com/brutella/dnssd v1.2.2 => github.com/rblenkinsopp/dnssd v1.2.3-0.20220516082132-0923f3c787a1
	// RTP tlv8 fix
	github.com/brutella/hap v0.0.17 => github.com/AlexxIT/hap v0.0.15-0.20221108133010-d8a45b7a7045
	// fix reading AAC config bytes
	github.com/deepch/vdk v0.0.19 => github.com/AlexxIT/vdk v0.0.18-0.20221108193131-6168555b4f92
)
