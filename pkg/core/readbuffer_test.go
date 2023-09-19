package core

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadSeeker(t *testing.T) {
	b := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	buf := bytes.NewReader(b)

	rd := NewReadBuffer(buf)
	rd.BufferSize = ProbeSize

	// 1. Read to buffer
	b = make([]byte, 3)
	n, err := rd.Read(b)
	require.Nil(t, err)
	require.Equal(t, []byte{0, 1, 2}, b[:n])

	// 2. Seek to start
	_, err = rd.Seek(0, io.SeekStart)
	require.Nil(t, err)

	// 3. Read from buffer
	b = make([]byte, 2)
	n, err = rd.Read(b)
	require.Nil(t, err)
	require.Equal(t, []byte{0, 1}, b[:n])

	// 4. Read from buffer
	n, err = rd.Read(b)
	require.Nil(t, err)
	require.Equal(t, []byte{2}, b[:n])

	// 5. Read to buffer
	n, err = rd.Read(b)
	require.Nil(t, err)
	require.Equal(t, []byte{3, 4}, b[:n])

	// 6. Seek to start
	_, err = rd.Seek(0, io.SeekStart)
	require.Nil(t, err)

	// 7. Disable buffer
	rd.BufferSize = -1

	// 8. Read from buffer
	b = make([]byte, 10)
	n, err = rd.Read(b)
	require.Nil(t, err)
	require.Equal(t, []byte{0, 1, 2, 3, 4}, b[:n])

	// 9. Direct read
	n, err = rd.Read(b)
	require.Nil(t, err)
	require.Equal(t, []byte{5, 6, 7, 8, 9}, b[:n])

	// 10. Check buffer empty
	require.Nil(t, rd.buf)
}
