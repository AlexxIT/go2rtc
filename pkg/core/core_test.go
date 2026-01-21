package core

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type producer struct {
	Medias    []*Media
	Receivers []*Receiver

	id byte
}

func (p *producer) GetMedias() []*Media {
	return p.Medias
}

func (p *producer) GetTrack(_ *Media, codec *Codec) (*Receiver, error) {
	for _, receiver := range p.Receivers {
		if receiver.Codec == codec {
			return receiver, nil
		}
	}
	receiver := NewReceiver(nil, codec)
	p.Receivers = append(p.Receivers, receiver)
	return receiver, nil
}

func (p *producer) Start() error {
	pkt := &Packet{Payload: []byte{p.id}}
	p.Receivers[0].Input(pkt)
	return nil
}

func (p *producer) Stop() error {
	for _, receiver := range p.Receivers {
		receiver.Close()
	}
	return nil
}

type consumer struct {
	Medias  []*Media
	Senders []*Sender

	cache chan byte
}

func (c *consumer) GetMedias() []*Media {
	return c.Medias
}

func (c *consumer) AddTrack(_ *Media, _ *Codec, track *Receiver) error {
	c.cache = make(chan byte, 1)
	sender := NewSender(nil, track.Codec)
	sender.Output = func(packet *Packet) {
		c.cache <- packet.Payload[0]
	}
	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *consumer) Stop() error {
	for _, sender := range c.Senders {
		sender.Close()
	}
	return nil
}

func (c *consumer) read() byte {
	return <-c.cache
}

func TestName(t *testing.T) {
	GetProducer := func(b byte) Producer {
		return &producer{
			Medias: []*Media{
				{
					Kind:      KindVideo,
					Direction: DirectionRecvonly,
					Codecs: []*Codec{
						{Name: CodecH264},
					},
				},
			},
			id: b,
		}
	}

	// stage1
	prod1 := GetProducer(1)
	cons2 := &consumer{}

	media1 := prod1.GetMedias()[0]
	track1, _ := prod1.GetTrack(media1, media1.Codecs[0])

	_ = cons2.AddTrack(nil, nil, track1)

	_ = prod1.Start()
	require.Equal(t, byte(1), cons2.read())

	// stage2
	prod2 := GetProducer(2)
	media2 := prod2.GetMedias()[0]
	require.NotEqual(t, fmt.Sprintf("%p", media1), fmt.Sprintf("%p", media2))
	track2, _ := prod2.GetTrack(media2, media2.Codecs[0])
	track1.Replace(track2)

	_ = prod1.Stop()

	_ = prod2.Start()
	require.Equal(t, byte(2), cons2.read())

	// stage3
	_ = prod2.Stop()
}

func TestStripUserinfo(t *testing.T) {
	s := `streams:
  test:
    - ffmpeg:rtsp://username:password@10.1.2.3:554/stream1
    - ffmpeg:rtsp://10.1.2.3:554/stream1@#video=copy
`
	s = StripUserinfo(s)
	require.Equal(t, `streams:
  test:
    - ffmpeg:rtsp://***@10.1.2.3:554/stream1
    - ffmpeg:rtsp://10.1.2.3:554/stream1@#video=copy
`, s)
}

func TestConnectionMarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		conn     *Connection
		expected string
	}{
		{
			name: "URL with credentials",
			conn: &Connection{
				ID:         12345,
				FormatName: "rtsp",
				Protocol:   "tcp",
				URL:        "rtsp://username:password@192.168.1.204:554/live",
				RemoteAddr: "192.168.1.204:554",
			},
			expected: `{"id":12345,"format_name":"rtsp","protocol":"tcp","remote_addr":"192.168.1.204:554","url":"rtsp://***@192.168.1.204:554/live"}`,
		},
		{
			name: "URL without credentials",
			conn: &Connection{
				ID:         67890,
				FormatName: "webrtc",
				Protocol:   "udp",
				URL:        "rtsp://192.168.1.100:554/stream",
			},
			expected: `{"id":67890,"format_name":"webrtc","protocol":"udp","url":"rtsp://192.168.1.100:554/stream"}`,
		},
		{
			name: "URL with complex credentials (encoded)",
			conn: &Connection{
				ID:  11111,
				URL: "rtsp://user%40domain:p%40ssw0rd!@camera.local:8554/path?query=1",
			},
			expected: `{"id":11111,"url":"rtsp://***@camera.local:8554/path?query=1"}`,
		},
		{
			name: "Empty URL",
			conn: &Connection{
				ID:         22222,
				FormatName: "mjpeg",
			},
			expected: `{"id":22222,"format_name":"mjpeg"}`,
		},
		{
			name: "HTTP URL with credentials",
			conn: &Connection{
				ID:  33333,
				URL: "http://admin:secret@192.168.1.1:8080/stream",
			},
			expected: `{"id":33333,"url":"http://***@192.168.1.1:8080/stream"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.conn.MarshalJSON()
			require.NoError(t, err)
			require.JSONEq(t, tt.expected, string(data))
		})
	}
}
