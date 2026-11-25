package miss

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/crypto"
	"github.com/stretchr/testify/require"
)

type fakeConn struct {
	key  []byte
	cmds []uint32
}

func (f *fakeConn) Protocol() string                     { return "fake" }
func (f *fakeConn) Version() string                      { return "fake" }
func (f *fakeConn) ReadCommand() (uint32, []byte, error) { return 0, nil, nil }
func (f *fakeConn) ReadPacket() ([]byte, []byte, error)  { return nil, nil, nil }
func (f *fakeConn) WritePacket([]byte, []byte) error     { return nil }
func (f *fakeConn) RemoteAddr() net.Addr                 { return &net.UDPAddr{} }
func (f *fakeConn) SetDeadline(time.Time) error          { return nil }
func (f *fakeConn) Close() error                         { return nil }

func (f *fakeConn) WriteCommand(cmd uint32, data []byte) error {
	if cmd == cmdEncoded {
		dec, err := crypto.Decode(data, f.key)
		if err != nil {
			return err
		}
		f.cmds = append(f.cmds, binary.BigEndian.Uint32(dec[:4]))
	} else {
		f.cmds = append(f.cmds, cmd)
	}
	return nil
}

func TestSpeakerIdleReleaseAndResume(t *testing.T) {
	key := make([]byte, 32)
	conn := &fakeConn{key: key}
	client := &Client{Conn: conn, key: key}
	p := &Producer{
		Connection: core.Connection{Transport: client},
		client:     client,
		done:       make(chan struct{}),
	}

	require.NoError(t, p.startSpeaker())
	require.Equal(t, []uint32{cmdSpeakerStartReq}, conn.cmds)
	require.True(t, p.speakerOn.Load())

	// Active talk must not release the speaker.
	p.checkSpeakerIdle(talkIdle)
	require.Len(t, conn.cmds, 1)
	require.True(t, p.speakerOn.Load())

	// Idle past talkIdle releases the speaker.
	p.lastTalk.Add(-int64(talkIdle + time.Second))
	p.checkSpeakerIdle(talkIdle)
	require.Equal(t, []uint32{cmdSpeakerStartReq, cmdSpeakerStop}, conn.cmds)
	require.False(t, p.speakerOn.Load())

	// Next talk packet reopens the speaker.
	p.ensureSpeaker()
	require.Equal(t, []uint32{cmdSpeakerStartReq, cmdSpeakerStop, cmdSpeakerStartReq}, conn.cmds)
	require.True(t, p.speakerOn.Load())

	// Stop releases the speaker and stops media.
	require.NoError(t, p.Stop())
	require.Equal(t, []uint32{cmdSpeakerStartReq, cmdSpeakerStop, cmdSpeakerStartReq, cmdSpeakerStop, cmdVideoStop}, conn.cmds)
	require.False(t, p.speakerOn.Load())
}
