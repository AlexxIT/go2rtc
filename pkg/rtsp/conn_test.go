package rtsp

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultActiveProducerTimeout(t *testing.T) {
	require.Equal(t, 20*time.Second, defaultActiveProducerTimeout(&url.URL{Host: "127.0.0.1:8554"}))
	require.Equal(t, 20*time.Second, defaultActiveProducerTimeout(&url.URL{Host: "localhost:8554"}))
	require.Equal(t, 5*time.Second, defaultActiveProducerTimeout(&url.URL{Host: "192.168.2.238:554"}))
	require.Equal(t, 5*time.Second, defaultActiveProducerTimeout(nil))
}

func TestDefaultPassiveProducerTimeout(t *testing.T) {
	require.Equal(t, 60*time.Second, defaultPassiveProducerTimeout("127.0.0.1:8554"))
	require.Equal(t, 60*time.Second, defaultPassiveProducerTimeout("[::1]:8554"))
	require.Equal(t, 15*time.Second, defaultPassiveProducerTimeout("192.168.2.238:554"))
	require.Equal(t, 15*time.Second, defaultPassiveProducerTimeout("not-an-addr"))
}
