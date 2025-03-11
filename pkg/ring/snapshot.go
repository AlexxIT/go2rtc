package ring

import (
	"fmt"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type SnapshotProducer struct {
	core.Connection

	client *RingRestClient
	camera *CameraData
}

func NewSnapshotProducer(client *RingRestClient, camera *CameraData) *SnapshotProducer {
	return &SnapshotProducer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "ring/snapshot",
			Protocol:   "https",
			RemoteAddr: "app-snaps.ring.com",
			Medias: []*core.Media{
				{
					Kind:      core.KindVideo,
					Direction: core.DirectionRecvonly,
					Codecs: []*core.Codec{
						{
							Name:        core.CodecJPEG,
							ClockRate:   90000,
							PayloadType: core.PayloadTypeRAW,
						},
					},
				},
			},
		},
		client: client,
		camera: camera,
	}
}

func (p *SnapshotProducer) Start() error {
	// Fetch snapshot
	response, err := p.client.Request("GET", fmt.Sprintf("https://app-snaps.ring.com/snapshots/next/%d", int(p.camera.ID)), nil)
	if err != nil {
		return err
	}

	pkt := &rtp.Packet{
		Header:  rtp.Header{Timestamp: core.Now90000()},
		Payload: response,
	}

	p.Receivers[0].WriteRTP(pkt)

	return nil
}

func (p *SnapshotProducer) Stop() error {
	return p.Connection.Stop()
}
