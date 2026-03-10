package hds

import (
	"net"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

func TestEncryption(t *testing.T) {
	key := []byte(core.RandString(16, 0))
	salt := core.RandString(32, 0)

	c1, c2 := net.Pipe()
	t.Cleanup(func() { c1.Close(); c2.Close() })

	writer, err := NewConn(c1, key, salt, true)
	require.NoError(t, err)

	reader, err := NewConn(c2, key, salt, false)
	require.NoError(t, err)

	go func() {
		n, err := writer.Write([]byte("test"))
		require.NoError(t, err)
		require.Equal(t, 4, n)
	}()

	b := make([]byte, 32)
	n, err := reader.Read(b)
	require.NoError(t, err)

	require.Equal(t, "test", string(b[:n]))
}
