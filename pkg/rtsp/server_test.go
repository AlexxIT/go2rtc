package rtsp

import (
	"bufio"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func announce(t *testing.T, deny bool) (string, bool, error) {
	defer func(d time.Duration) { Timeout = d }(Timeout)
	Timeout = time.Second

	server, client := net.Pipe()
	defer client.Close()

	c := NewServer(server)
	c.DenyPublish = deny

	var announced bool
	c.Listen(func(msg any) {
		if msg == MethodAnnounce {
			announced = true
		}
	})

	errs := make(chan error, 1)
	go func() { errs <- c.Accept() }()

	sdp := "v=0\r\ns=-\r\nt=0 0\r\nm=video 0 RTP/AVP 96\r\na=rtpmap:96 H264/90000\r\n"
	_, err := client.Write([]byte("ANNOUNCE rtsp://localhost/camera1 RTSP/1.0\r\nCSeq: 1\r\n" +
		"Content-Type: application/sdp\r\nContent-Length: " + strconv.Itoa(len(sdp)) + "\r\n\r\n" + sdp))
	require.NoError(t, err)

	status, err := bufio.NewReader(client).ReadString('\n')
	require.NoError(t, err)

	_ = client.Close()
	return status, announced, <-errs
}

func TestDenyPublish(t *testing.T) {
	status, announced, err := announce(t, true)
	require.Equal(t, "RTSP/1.0 403 Forbidden\r\n", status)
	require.False(t, announced)
	require.ErrorIs(t, err, FailedPublish)

	status, announced, _ = announce(t, false)
	require.Equal(t, "RTSP/1.0 200 OK\r\n", status)
	require.True(t, announced)
}
