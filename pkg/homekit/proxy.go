package homekit

import (
	"bufio"
	"bytes"
	"net"
	"net/http"

	"github.com/AlexxIT/go2rtc/pkg/hap"
)

func ProxyHandler(pair ServerPair, dial func() (net.Conn, error)) hap.HandlerFunc {
	return func(controller net.Conn) error {
		accessory, err := dial()
		if err != nil {
			return err
		}

		// accessory (ex. Camera) => controller (ex. iPhone)
		go proxy(accessory, controller, nil)

		// controller => accessory
		return proxy(controller, accessory, pair)
	}
}

func proxy(r, w net.Conn, pair ServerPair) error {
	b := make([]byte, 64*1024)
	for {
		n, err := r.Read(b)
		if err != nil {
			break
		}

		if pair != nil && bytes.HasPrefix(b[:n], []byte("POST /pairings HTTP/1.1")) {
			buf := bytes.NewBuffer(b[:n])
			req, err := http.ReadRequest(bufio.NewReader(buf))
			if err != nil {
				return err
			}

			res, err := handlePairings(r, req, pair)
			if err != nil {
				return err
			}

			buf.Reset()

			if err = res.Write(buf); err != nil {
				return err
			}
			if _, err = buf.WriteTo(r); err != nil {
				return err
			}
			continue
		}

		//log.Printf("[hap] %d bytes => %s\n%.512s", n, w.RemoteAddr(), b[:n])

		if _, err = w.Write(b[:n]); err != nil {
			break
		}
	}
	_ = r.Close()
	_ = w.Close()
	return nil
}
