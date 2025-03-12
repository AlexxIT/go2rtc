package hds

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

func TestEncryption(t *testing.T) {
	key := []byte(core.RandString(16, 0))
	salt := core.RandString(32, 0)

	c, err := Client(nil, key, salt, true)
	require.NoError(t, err)

	buf := bytes.NewBuffer(nil)
	c.wr = bufio.NewWriter(buf)

	n, err := c.Write([]byte("test"))
	require.NoError(t, err)
	require.Equal(t, 4, n)

	c, err = Client(nil, key, salt, false)
	c.rd = bufio.NewReader(buf)
	require.NoError(t, err)

	b := make([]byte, 32)
	n, err = c.Read(b)
	require.NoError(t, err)

	require.Equal(t, "test", string(b[:n]))
}
