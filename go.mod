module github.com/AlexxIT/go2rtc

go 1.17

require (
	github.com/deepch/vdk v0.0.19
	github.com/gorilla/websocket v1.5.0
	github.com/pion/ice/v2 v2.2.6
	github.com/pion/interceptor v0.1.11
	github.com/pion/rtcp v1.2.9
	github.com/pion/rtp v1.7.13
	github.com/pion/sdp/v3 v3.0.5
	github.com/pion/stun v0.3.5
	github.com/pion/webrtc/v3 v3.1.43
	github.com/rs/zerolog v1.27.0
	github.com/stretchr/testify v1.7.1
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/pion/datachannel v1.5.2 // indirect
	github.com/pion/dtls/v2 v2.1.5 // indirect
	github.com/pion/logging v0.2.2 // indirect
	github.com/pion/mdns v0.0.5 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/sctp v1.8.2 // indirect
	github.com/pion/srtp/v2 v2.0.10 // indirect
	github.com/pion/transport v0.13.1 // indirect
	github.com/pion/turn/v2 v2.0.8 // indirect
	github.com/pion/udp v0.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/crypto v0.0.0-20220516162934-403b01795ae8 // indirect
	golang.org/x/net v0.0.0-20220630215102-69896b714898 // indirect
	golang.org/x/sys v0.0.0-20220622161953-175b2fd9d664 // indirect
)

replace (
	// windows support: https://github.com/brutella/dnssd/pull/35
	github.com/brutella/dnssd v1.2.2 => github.com/rblenkinsopp/dnssd v1.2.3-0.20220516082132-0923f3c787a1
	// RTP tlv8 fix
	github.com/brutella/hap v0.0.17 => github.com/AlexxIT/hap v0.0.15-0.20220823033740-ce7d1564e657
	// MSE update
	github.com/deepch/vdk v0.0.19 => github.com/AlexxIT/vdk v0.0.18-0.20220616041030-b0d122807b2e
	// AES_256_CM_HMAC_SHA1_80 support
	github.com/pion/srtp/v2 v2.0.10 => github.com/AlexxIT/srtp/v2 v2.0.10-0.20220608200505-3191d4f19c10
)
