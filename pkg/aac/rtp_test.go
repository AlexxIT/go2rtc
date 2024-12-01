package aac

import (
	"encoding/hex"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestBuggy_RTSP_AAC(t *testing.T) {
	// https: //github.com/AlexxIT/go2rtc/issues/1328
	payload, _ := hex.DecodeString("fff16080431ffc211ad4458aa309a1c0a8761a230502b7c74b2b5499252a010555e32e460128303c8ace4fd3260d654a424f7e7c65eddc96735fc6f1ac0edf94fdefa0e0bd6370da1c07b9c0e77a9d6e86b196a1ac7439dcafadcffcf6d89f60ac67f8884868e931383ad3e40cf5495470d1f606ef6f7624d285b951ebfa0e42641ab98f1371182b237d14f1bd16ad714fa2f1c6a7d23ebde7a0e34a2eca156a608a4caec49d9dca4b6fe2a09e9cdbf762c5b4148a3914abb7959c991228b0837b5988334b9fc18b8fac689b5ca1e4661573bbb8b253a86cae7ec14ace49969a9a76fd571ab6e650764cb59114d61dcedf07ac61b39e4ac66adebfd0d0ab45d518dd3c161049823f150864d977cf0855172ac8482e4b25fe911325d19617558c5405af74aff5492e4599bee53f2dbdf0503730af37078550f84c956b7ee89aae83c154fa2fa6e6792c5ddd5cd5cf6bb96bf055fee7f93bed59ffb039daee5ea7e5593cb194e9091e417c67d8f73026a6a6ae056e808f7c65c03d1b9197d3709ceb63bc7b979f7ba71df5e7c6395d99d6ea229000a6bc16fb4346d6b27d32f5d8d1200736d9366d59c0c9547210813b602473da9c46f9015bbb37594c1dd90cd6a36e96bd5d6a1445ab93c9e65505ec2c722bb4cc27a10600139a48c83594dde145253c386f6627d8c6e5102fe3828a590c709bc87f55b37e97d1ae72b017b09c6bb2c13299817bb45cc67318e10b6822075b97c6a03ec1c0")
	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			SequenceNumber: 36944,
			Timestamp:      4217191328,
			SSRC:           12892774,
		},
		Payload: payload,
	}

	var size int

	RTPDepay(func(packet *core.Packet) {
		size = len(packet.Payload)
	})(packet)

	require.Equal(t, len(payload), size+ADTSHeaderSize)
}
