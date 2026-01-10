package tutk

import (
	"fmt"
	"net"
	"time"
)

type ChannelAdapter struct {
	conn    *Conn
	channel uint8
}

func (a *ChannelAdapter) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	var buf chan []byte
	if a.channel == IOTCChannelMain {
		buf = a.conn.mainBuf
	} else {
		buf = a.conn.speakerBuf
	}

	select {
	case data := <-buf:
		n = copy(p, data)
		if a.conn.verbose && len(data) >= 1 {
			fmt.Printf("[ChannelAdapter] ch=%d ReadFrom: len=%d contentType=%d\n",
				a.channel, len(data), data[0])
		}
		return n, a.conn.addr, nil
	case <-a.conn.done:
		return 0, nil, net.ErrClosed
	}
}

func (a *ChannelAdapter) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	if a.conn.verbose {
		fmt.Printf("[IOTC TX] channel=%d size=%d\n", a.channel, len(p))
	}
	_, err = a.conn.sendIOTC(p, a.channel)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (a *ChannelAdapter) Close() error {
	return nil
}

func (a *ChannelAdapter) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: 0}
}

func (a *ChannelAdapter) SetDeadline(time.Time) error {
	return nil
}

func (a *ChannelAdapter) SetReadDeadline(time.Time) error {
	return nil
}

func (a *ChannelAdapter) SetWriteDeadline(time.Time) error {
	return nil
}
