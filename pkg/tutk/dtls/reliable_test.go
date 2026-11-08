package dtls

import (
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func wrappedMedia(seq uint16, channel, flags byte, payload string) []byte {
	offset := 16
	if flags&8 != 0 {
		offset += 8
	}
	b := make([]byte, offset+20+len(payload))
	b[0], b[1] = 12, flags
	binary.LittleEndian.PutUint16(b[2:], protoVersion)
	binary.LittleEndian.PutUint16(b[4:], seq)
	b[offset], b[offset+1] = channel, flags&1
	binary.LittleEndian.PutUint16(b[offset+2:], seq)
	binary.LittleEndian.PutUint16(b[offset+4:], 1)
	binary.LittleEndian.PutUint16(b[offset+8:], uint16(len(payload)))
	copy(b[offset+20:], payload)
	return b
}

func TestReliableMediaRetransmission(t *testing.T) {
	var r reliableMedia
	r.enable()
	now := time.Unix(100, 0)
	packets, _, handled, err := r.handle(wrappedMedia(0, 5, 0, "first"), now)
	require.NoError(t, err)
	require.True(t, handled)
	require.Len(t, packets, 1)
	packets, _, _, err = r.handle(wrappedMedia(2, 7, 1, "last"), now)
	require.NoError(t, err)
	require.Empty(t, packets)
	ack, err := r.ack(now)
	require.NoError(t, err)
	require.Equal(t, uint16(0xffff), binary.LittleEndian.Uint16(ack[8:]))
	require.Equal(t, uint16(2), binary.LittleEndian.Uint16(ack[10:]))
	require.Equal(t, uint16(1), binary.LittleEndian.Uint16(ack[14:]))
	require.Equal(t, uint16(2), binary.LittleEndian.Uint16(ack[22:]))
	ack, err = r.ack(now.Add(20 * time.Millisecond))
	require.NoError(t, err)
	require.Equal(t, uint16(0), binary.LittleEndian.Uint16(ack[8:]))
	require.Equal(t, uint16(1), binary.LittleEndian.Uint16(ack[22:]))
	packets, _, _, err = r.handle(wrappedMedia(1, 6, 4, "resent"), now.Add(30*time.Millisecond))
	require.NoError(t, err)
	require.Len(t, packets, 2)
	require.Equal(t, byte(5), packets[0][0])
	require.Equal(t, byte(5), packets[0][8])
	require.Equal(t, "resent", string(packets[0][28:]))
	require.Equal(t, "last", string(packets[1][28:]))
	packets, _, _, err = r.handle(wrappedMedia(1, 6, 4, "duplicate"), now.Add(40*time.Millisecond))
	require.NoError(t, err)
	require.Empty(t, packets)
	ack, err = r.ack(now.Add(50 * time.Millisecond))
	require.NoError(t, err)
	require.Zero(t, binary.LittleEndian.Uint16(ack[14:]))
}

func TestReliableMediaWrapAndMultipleLosses(t *testing.T) {
	var r reliableMedia
	r.enable()
	r.next = 65534
	r.highest = 65533
	r.ackBase = 65533
	r.started = true
	now := time.Now()
	packets, _, _, err := r.handle(wrappedMedia(1, 3, 0, "one"), now)
	require.NoError(t, err)
	require.Empty(t, packets)
	ack, err := r.ack(now)
	require.NoError(t, err)
	require.Equal(t, uint16(3), binary.LittleEndian.Uint16(ack[14:]))
	for i, want := range []uint16{1, 2, 3} {
		require.Equal(t, want, binary.LittleEndian.Uint16(ack[22+i*2:]))
	}
	for _, seq := range []uint16{65535, 0} {
		packets, _, _, err = r.handle(wrappedMedia(seq, 4, 4, "audio"), now)
		require.NoError(t, err)
		require.Empty(t, packets)
	}
	packets, _, _, err = r.handle(wrappedMedia(65534, 4, 4, "first"), now)
	require.NoError(t, err)
	require.Len(t, packets, 4)
	for _, p := range packets {
		require.Equal(t, byte(3), p[0])
	}
}

func TestReliableMediaBoundedLoss(t *testing.T) {
	now := time.Now()
	var r reliableMedia
	r.enable()
	_, _, _, err := r.handle(wrappedMedia(mediaWindow, 5, 0, "too far"), now)
	require.ErrorIs(t, err, errMediaGap)
	require.Empty(t, r.pending)
	_, _, _, err = r.handle(wrappedMedia(mediaWindow-1, 5, 0, "inside window"), now)
	require.NoError(t, err)
	ack, err := r.ack(now)
	require.NoError(t, err)
	require.LessOrEqual(t, len(ack), 1200)
	_, err = r.ack(now.Add(mediaTimeout))
	require.ErrorIs(t, err, errMediaGap)
}

func TestReliableMediaStatisticsAndCommands(t *testing.T) {
	var r reliableMedia
	r.enable()
	now := time.Now()
	p := wrappedMedia(0, 5, 8, "video")
	p[8], p[9] = 0x34, 0x12
	packets, stats, handled, err := r.handle(p, now)
	require.NoError(t, err)
	require.True(t, handled)
	require.Len(t, packets, 1)
	require.Equal(t, "video", string(packets[0][28:]))
	require.Equal(t, byte(11), stats[0])
	require.Equal(t, []byte{0x34, 0x12}, stats[8:10])
	p = wrappedMedia(3, 0, 0, "command")
	p[17] = 0x70
	_, _, handled, err = r.handle(p, now)
	require.NoError(t, err)
	require.False(t, handled)
	ack, err := r.ack(now)
	require.NoError(t, err)
	require.Equal(t, uint16(3), binary.LittleEndian.Uint16(ack[12:]))
	_, _, handled, err = r.handle(wrappedMedia(0, 12, 0, "parity"), now)
	require.NoError(t, err)
	require.True(t, handled)
}

func TestReliableMediaConcurrentACK(t *testing.T) {
	var r reliableMedia
	r.enable()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_, err := r.ack(time.Now())
			require.NoError(t, err)
		}
	}()
	for i := 0; i < 1000; i++ {
		_, _, _, err := r.handle(wrappedMedia(uint16(i), 5, 0, "video"), time.Now())
		require.NoError(t, err)
	}
	wg.Wait()
}

func TestResendLoginFlag(t *testing.T) {
	c := DTLSConn{}
	for _, resend := range []bool{false, true} {
		b := c.msgAVLogin(magicAVLogin1, 570, 1, []byte{1, 2, 3, 4}, resend)
		require.Equal(t, resend, b[538] == 1)
		require.Equal(t, byte(0), b[536])
	}
}

func TestReliableMediaRejectsTruncatedFragments(t *testing.T) {
	var r reliableMedia
	r.enable()
	now := time.Now()
	p := wrappedMedia(0, 5, 8, "complete payload")
	for _, length := range []int{0, 1, 7, 15, 23, 43, len(p) - 1} {
		packets, _, _, err := r.handle(p[:length], now)
		require.NoError(t, err)
		require.Empty(t, packets)
	}
	require.Zero(t, r.next)
	packets, _, _, err := r.handle(p, now)
	require.NoError(t, err)
	require.Len(t, packets, 1)
}

func TestReliableMediaDisabled(t *testing.T) {
	var r reliableMedia
	packets, stats, handled, err := r.handle(wrappedMedia(0, 5, 8, "video"), time.Now())
	require.NoError(t, err)
	require.False(t, handled)
	require.Empty(t, packets)
	require.Nil(t, stats)
	ack, err := r.ack(time.Now())
	require.NoError(t, err)
	require.Nil(t, ack)
}
