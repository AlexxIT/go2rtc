package gopro

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/mpegts"
)

type Status int
type Error int

const (
	StatusOff Status = iota
	StatusIdle
	StatusHigh_Power_Preview
	StatusLow_Power_Preview
	StatusUnavailable
)

const (
	ErrorNone Error = iota
	ErrorSet_Preset
	ErrorSet_Window_Size
	ErrorExec_Stream
	ErrorShutter
	ErrorCom_Timeout
	ErrorInvalid_Param
	ErrorUnavailable
	ErrorExit
)

type StatusResponse struct {
	Status Status `json:"status"`
	Error  Error  `json:"error"`
}

func Dial(rawURL string) (*mpegts.Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	r := &listener{host: u.Host}

	// check if webcam is already active, if so, stop it before starting a new one
	status, err := r.command("/gopro/webcam/status")
	// fmt.Printf("[gopro] webcam status: %d\n", status)
	if err != nil {
		return nil, err
	}

	if status == StatusHigh_Power_Preview || status == StatusLow_Power_Preview {
		// fmt.Printf("[gopro] webcam is active, stopping it before starting a new one\n")
		if _, err = r.command("/gopro/webcam/stop"); err != nil {
			return nil, err
		}
	}

	if err = r.listen(); err != nil {
		return nil, err
	}

	// check if webcam is active, if not, start it
	status, err = r.command("/gopro/webcam/status")
	// fmt.Printf("[gopro] webcam status: %d\n", status)
	if err != nil {
		return nil, err
	}

	if status == StatusOff || status == StatusIdle {
		// fmt.Printf("[gopro] webcam is not active, starting it\n")
		if _, err = r.command("/gopro/webcam/start"); err != nil {
			return nil, err
		}
	}

	prod, err := mpegts.Open(r)
	if err != nil {
		return nil, err
	}

	prod.FormatName = "gopro"
	prod.RemoteAddr = u.Host

	return prod, nil
}

type listener struct {
	conn    net.PacketConn
	host    string
	packet  []byte
	packets chan []byte
}

func (r *listener) Read(p []byte) (n int, err error) {
	if r.packet == nil {
		var ok bool
		if r.packet, ok = <-r.packets; !ok {
			return 0, io.EOF // channel closed
		}
	}

	n = copy(p, r.packet)

	if n < len(r.packet) {
		r.packet = r.packet[n:]
	} else {
		r.packet = nil
	}

	return
}

func (r *listener) Close() error {
	return r.conn.Close()
}

func (r *listener) command(api string) (Status, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	res, err := client.Get("http://" + r.host + ":8080" + api)
	if err != nil {
		return 0, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return 0, errors.New("gopro: wrong response: " + res.Status)
	}

	var statusResponse StatusResponse
	if err = json.NewDecoder(res.Body).Decode(&statusResponse); err != nil {
		return 0, err
	}

	if statusResponse.Error > 0 {
		return 0, fmt.Errorf("gopro: error in status response: %d", statusResponse.Error)
	}

	return statusResponse.Status, nil
}

func (r *listener) listen() (err error) {
	if r.conn, err = net.ListenPacket("udp4", ":8554"); err != nil {
		return
	}

	r.packets = make(chan []byte, 1024)
	go r.worker()

	return
}

func (r *listener) worker() {
	b := make([]byte, 1500)
	for {
		if err := r.conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
			break
		}

		n, _, err := r.conn.ReadFrom(b)
		if err != nil {
			break
		}

		packet := make([]byte, n)
		copy(packet, b)

		r.packets <- packet
	}

	close(r.packets)

	_, _ = r.command("/gopro/webcam/stop")
}
