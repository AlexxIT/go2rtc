package miss

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/crypto"
	"github.com/stretchr/testify/require"
)

type testFrame struct{ header, payload []byte }

type testConn struct {
	Conn
	frames chan testFrame
	writes [][]byte
	closed bool
}

func (c *testConn) WriteCommand(_ uint32, data []byte) error {
	c.writes = append(c.writes, data)
	return nil
}

func (*testConn) SetDeadline(time.Time) error { return nil }

func (c *testConn) ReadPacket() ([]byte, []byte, error) {
	f := <-c.frames
	if f.header == nil {
		return nil, nil, io.EOF
	}
	return f.header, f.payload, nil
}

func (c *testConn) Close() error {
	c.closed = true
	close(c.frames)
	return nil
}

func lensTrack(t *testing.T, v *lens, index int) *core.Receiver {
	t.Helper()
	m := v.GetMedias()[index]
	r, err := v.GetTrack(m, m.Codecs[0])
	require.NoError(t, err)
	return r
}

// Wait until Start and the shared reader are blocked before sending packets.
func startLens(v *lens) <-chan error {
	done := make(chan error, 1)
	go func() { done <- v.Start() }()
	synctest.Wait()
	return done
}

func TestDialErrorReturnsNilProducer(t *testing.T) {
	if p, err := Dial("%"); err == nil || p != nil {
		t.Errorf("Dial returned %T, %v", p, err)
	}

	var pair LensPair
	if p, err := pair.Open(true, func() (string, error) { return "", io.EOF }); err != io.EOF || p != nil {
		t.Fatalf("Open returned %T, %v", p, err)
	}
}

func TestDualLensStartMedia(t *testing.T) {
	for _, audio := range []int{0, 1} {
		conn := new(testConn)
		client := &Client{Conn: conn, key: make([]byte, 32), model: "xiaomi.camera.082ac1"}
		require.NoError(t, client.startMedia("", "3", strconv.Itoa(audio), true))
		require.Len(t, conn.writes, 1)
		data, err := crypto.Decode(conn.writes[0], client.key)
		require.NoError(t, err)
		require.Equal(t, uint32(cmdVideoStart), binary.BigEndian.Uint32(data))

		var body map[string]int
		require.NoError(t, json.Unmarshal(data[4:], &body))
		require.Subset(t, body, map[string]int{"videoquality": 3, "videoquality2": 3, "enableaudio": audio})
	}
}

func makeTestFrame(t *testing.T, key []byte, codec, flags, sequence uint32, data []byte) testFrame {
	t.Helper()
	payload, err := crypto.Encode(data, key)
	require.NoError(t, err)
	header := make([]byte, hdrSize)
	binary.LittleEndian.PutUint32(header, uint32(len(payload)))
	binary.LittleEndian.PutUint32(header[4:], codec)
	binary.LittleEndian.PutUint32(header[8:], sequence)
	binary.LittleEndian.PutUint32(header[12:], flags)
	return testFrame{header, payload}
}

func TestDualLensProbeAndRouting(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		codec                     uint32
		params, aud, intra, inter string
	}{
		{"H264", codecH264, "6764001fac2484014016ec0440000003004000000c23c60c92", "09f0", "6501", "4101"},
		{"H265", codecH265, "40010c01", "460150", "2601", "0201"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				conn := &testConn{frames: make(chan testFrame, 4)}
				client := &Client{Conn: conn, key: make([]byte, 32)}
				video := func(flags, seq uint32, nalu string) {
					data, err := hex.DecodeString("00000001" + nalu)
					require.NoError(t, err)
					conn.frames <- makeTestFrame(t, client.key, tc.codec, flags, seq, data)
				}
				mic := func(seq uint32) {
					conn.frames <- makeTestFrame(t, client.key, codecOPUS, 0, seq, []byte{1})
				}

				// Both lenses have identical parameters; only bit 24 changes.
				const primary = 0x001e2000
				const secondary = primary | 1<<24
				video(primary, 0, tc.aud)
				video(primary, 1, tc.params)
				video(secondary, 2, tc.params)
				mic(3)
				close(conn.frames) // an incomplete probe must reach EOF
				medias, err := probe(client, true, 2)
				require.NoError(t, err)
				conn.frames = make(chan testFrame)

				var pair LensPair
				var views [2]*lens
				var gotVideo, gotMic [2][]uint16
				dial := func() (*Producer, error) {
					return &Producer{Connection: core.Connection{Medias: medias}, client: client}, nil
				}
				for i := range views {
					v, err := pair.open(i, dial)
					require.NoError(t, err)
					views[i] = v
					defer v.Stop()
					require.Len(t, v.Medias, 3)
					require.Equal(t, core.DirectionSendonly, v.Medias[2].Direction)
					lensTrack(t, v, 0).Input = func(p *core.Packet) { gotVideo[i] = append(gotVideo[i], p.SequenceNumber) }
					lensTrack(t, v, 1).Input = func(p *core.Packet) {
						gotMic[i] = append(gotMic[i], p.SequenceNumber)
						if !p.Marker {
							t.Error("microphone RTP header changed by the other view")
						}
						p.Marker = false
					}
				}
				require.Empty(t, conn.writes, "opening a view must not start the speaker")

				startLens(views[0])
				video(secondary, 8, tc.intra) // before the secondary starts
				video(primary, 9, tc.inter)
				video(primary, 10, tc.intra)
				video(primary, 11, tc.aud)
				video(primary, 12, tc.inter)
				synctest.Wait()

				startLens(views[1])
				video(secondary, 19, tc.inter)
				video(secondary, 20, tc.intra)
				video(secondary, 21, tc.inter)
				mic(30)
				synctest.Wait()
				require.Equal(t, [2][]uint16{{10, 12}, {20, 21}}, gotVideo)
				require.Equal(t, [2][]uint16{{30}, {30}}, gotMic)
			})
		})
	}
}

func TestDualLensProbeRequiresBothLenses(t *testing.T) {
	conn := &testConn{frames: make(chan testFrame, 1)}
	client := &Client{Conn: conn, key: make([]byte, 32)}
	conn.frames <- makeTestFrame(t, client.key, codecH265, 0, 1, []byte{0, 0, 0, 1, 0x40, 1, 0x0c, 1})
	close(conn.frames)
	_, err := probe(client, false, 2)
	require.ErrorContains(t, err, "missing video channel")
}

func TestLensPairLifecycle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var pair LensPair
		var conns []*testConn
		video := &core.Media{Kind: core.KindVideo, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{{Name: core.CodecH265}}}
		open := func(channel int) *lens {
			t.Helper()
			v, err := pair.open(channel, func() (*Producer, error) {
				conn := &testConn{frames: make(chan testFrame)}
				conns = append(conns, conn)
				c := &Client{Conn: conn, key: make([]byte, 32)}
				return &Producer{Connection: core.Connection{Medias: []*core.Media{video, video.Clone()}}, client: c}, nil
			})
			require.NoError(t, err)
			return v
		}

		subscribe := func(v *lens) <-chan error {
			lensTrack(t, v, 0)
			return startLens(v)
		}
		second := open(1)
		secondDone := subscribe(second)
		first, duplicate := open(0), open(1)
		m := duplicate.Medias[0]
		_, err := duplicate.GetTrack(m, m.Codecs[0])
		require.Error(t, err)
		require.NoError(t, duplicate.Stop())
		require.Len(t, conns, 1)

		firstDone := subscribe(first)
		require.NoError(t, second.Stop())
		require.NoError(t, <-secondDone)
		require.False(t, conns[0].closed, "the primary still needs the camera")
		second = open(1)
		secondDone = subscribe(second)
		conns[0].frames <- testFrame{} // peer EOF with both views subscribed
		require.ErrorIs(t, <-firstDone, io.EOF)
		require.ErrorIs(t, <-secondDone, io.EOF)
		require.True(t, conns[0].closed)

		newSecond, newFirst := open(1), open(0)
		require.Len(t, conns, 2, "both views must share one replacement connection")
		require.NoError(t, second.Stop())
		require.NoError(t, first.Stop())
		require.NoError(t, newFirst.Stop())
		require.False(t, conns[1].closed, "the replacement still has a viewer")
		require.NoError(t, newSecond.Stop())
		require.NoError(t, newSecond.Stop())
		require.True(t, conns[1].closed, "the last viewer must release the camera")
	})
}
