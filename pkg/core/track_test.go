package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSenser(t *testing.T) {
	recv := make(chan *Packet) // blocking receiver

	sender := NewSender(nil, &Codec{})
	sender.Output = func(packet *Packet) {
		recv <- packet
	}
	require.Equal(t, "new", sender.State())

	sender.Start()
	require.Equal(t, "connected", sender.State())

	sender.Input(&Packet{})
	sender.Input(&Packet{})

	require.Equal(t, 2, sender.Packets)
	require.Equal(t, 0, sender.Drops)

	// important to read one before close
	// because goroutine in Start() can run with nil chan
	// it's OK in real life, but bad for test
	_, ok := <-recv
	require.True(t, ok)

	sender.Close()
	require.Equal(t, "closed", sender.State())

	sender.Input(&Packet{})

	require.Equal(t, 2, sender.Packets)
	require.Equal(t, 1, sender.Drops)

	// read 2nd
	_, ok = <-recv
	require.True(t, ok)

	// read 3rd
	select {
	case <-recv:
		ok = true
	default:
		ok = false
	}
	require.False(t, ok)
}
