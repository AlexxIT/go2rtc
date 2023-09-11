package homekit

import (
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg"
	srtp2 "github.com/AlexxIT/go2rtc/internal/srtp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	"github.com/AlexxIT/go2rtc/pkg/homekit"
	"github.com/AlexxIT/go2rtc/pkg/magic"
	"github.com/AlexxIT/go2rtc/pkg/mdns"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
)

type server struct {
	stream    string      // stream name from YAML
	hap       *hap.Server // server for HAP connection and encryption
	mdns      *mdns.ServiceEntry
	srtp      *srtp.Server
	accessory *hap.Accessory // HAP accessory
	pairings  []string       // pairings list

	streams  map[string]*homekit.Consumer
	consumer *homekit.Consumer
}

func (s *server) UpdateStatus() {
	// true status is important, or device may be offline in Apple Home
	if len(s.pairings) == 0 {
		s.mdns.Info[hap.TXTStatusFlags] = hap.StatusNotPaired
	} else {
		s.mdns.Info[hap.TXTStatusFlags] = hap.StatusPaired
	}
}

func (s *server) GetAccessories(_ net.Conn) []*hap.Accessory {
	return []*hap.Accessory{s.accessory}
}

func (s *server) GetCharacteristic(conn net.Conn, aid uint8, iid uint64) any {
	log.Trace().Msgf("[homekit] %s: get char aid=%d iid=0x%x", conn.RemoteAddr(), aid, iid)

	char := s.accessory.GetCharacterByID(iid)
	if char == nil {
		log.Warn().Msgf("[homekit] get unknown characteristic: %d", iid)
		return nil
	}

	switch char.Type {
	case camera.TypeSetupEndpoints:
		if s.consumer == nil {
			return nil
		}

		answer := s.consumer.GetAnswer()
		v, err := tlv8.MarshalBase64(answer)
		if err != nil {
			return nil
		}

		return v
	}

	return char.Value
}

func (s *server) SetCharacteristic(conn net.Conn, aid uint8, iid uint64, value any) {
	log.Trace().Msgf("[homekit] %s: set char aid=%d iid=0x%x value=%v", conn.RemoteAddr(), aid, iid, value)

	char := s.accessory.GetCharacterByID(iid)
	if char == nil {
		log.Warn().Msgf("[homekit] set unknown characteristic: %d", iid)
		return
	}

	switch char.Type {
	case camera.TypeSetupEndpoints:
		var offer camera.SetupEndpoints
		if err := tlv8.UnmarshalBase64(value.(string), &offer); err != nil {
			return
		}

		s.consumer = homekit.NewConsumer(conn, srtp2.Server)
		s.consumer.SetOffer(&offer)

	case camera.TypeSelectedStreamConfiguration:
		var conf camera.SelectedStreamConfig
		if err := tlv8.UnmarshalBase64(value.(string), &conf); err != nil {
			return
		}

		log.Trace().Msgf("[homekit] %s stream id=%x cmd=%d", conn.RemoteAddr(), conf.Control.SessionID, conf.Control.Command)

		switch conf.Control.Command {
		case camera.SessionCommandEnd:
			if consumer := s.streams[conf.Control.SessionID]; consumer != nil {
				_ = consumer.Stop()
			}

		case camera.SessionCommandStart:
			if s.consumer == nil {
				return
			}

			if !s.consumer.SetConfig(&conf) {
				log.Warn().Msgf("[homekit] wrong config")
				return
			}

			if s.streams == nil {
				s.streams = map[string]*homekit.Consumer{}
			}

			s.streams[conf.Control.SessionID] = s.consumer

			stream := streams.Get(s.stream)
			if err := stream.AddConsumer(s.consumer); err != nil {
				return
			}

			go func() {
				_, _ = s.consumer.WriteTo(nil)
				stream.RemoveConsumer(s.consumer)

				delete(s.streams, conf.Control.SessionID)
			}()
		}
	}
}

func (s *server) GetImage(conn net.Conn, width, height int) []byte {
	log.Trace().Msgf("[homekit] %s: get image width=%d height=%d", conn.RemoteAddr(), width, height)

	stream := streams.Get(s.stream)
	cons := magic.NewKeyframe()

	if err := stream.AddConsumer(cons); err != nil {
		return nil
	}

	once := &core.OnceBuffer{} // init and first frame
	_, _ = cons.WriteTo(once)
	b := once.Buffer()

	stream.RemoveConsumer(cons)

	switch cons.CodecName() {
	case core.CodecH264, core.CodecH265:
		var err error
		if b, err = ffmpeg.JPEGWithScale(b, width, height); err != nil {
			return nil
		}
	}

	return b
}

func (s *server) GetPair(conn net.Conn, id string) []byte {
	log.Trace().Msgf("[homekit] %s: get pair id=%s", conn.RemoteAddr(), id)

	for _, pairing := range s.pairings {
		if !strings.Contains(pairing, id) {
			continue
		}

		query, err := url.ParseQuery(pairing)
		if err != nil {
			continue
		}

		if query.Get("client_id") != id {
			continue
		}

		s := query.Get("client_public")
		b, _ := hex.DecodeString(s)
		return b
	}
	return nil
}

func (s *server) AddPair(conn net.Conn, id string, public []byte, permissions byte) {
	log.Trace().Msgf("[homekit] %s: add pair id=%s public=%x perm=%d", conn.RemoteAddr(), id, public, permissions)

	query := url.Values{
		"client_id":     []string{id},
		"client_public": []string{hex.EncodeToString(public)},
		"permissions":   []string{string('0' + permissions)},
	}
	if s.GetPair(conn, id) == nil {
		s.pairings = append(s.pairings, query.Encode())
		s.UpdateStatus()
		s.PatchConfig()
	}
}

func (s *server) DelPair(conn net.Conn, id string) {
	log.Trace().Msgf("[homekit] %s: del pair id=%s", conn.RemoteAddr(), id)

	id = "client_id=" + id
	for i, pairing := range s.pairings {
		if !strings.Contains(pairing, id) {
			continue
		}

		s.pairings = append(s.pairings[:i], s.pairings[i+1:]...)
		s.UpdateStatus()
		s.PatchConfig()
		break
	}
}

func (s *server) PatchConfig() {
	if err := app.PatchConfig("pairings", s.pairings, "homekit", s.stream); err != nil {
		log.Error().Err(err).Msgf(
			"[homekit] can't save %s pairings=%v", s.stream, s.pairings,
		)
	}
}

func calcName(name, seed string) string {
	if name != "" {
		return name
	}
	b := sha512.Sum512([]byte(seed))
	return fmt.Sprintf("go2rtc-%02X%02X", b[0], b[2])
}

func calcDeviceID(deviceID, seed string) string {
	if deviceID != "" {
		if len(deviceID) >= 17 {
			// 1. Returd device_id as is (ex. AA:BB:CC:DD:EE:FF)
			return deviceID
		}
		// 2. Use device_id as seed if not zero
		seed = deviceID
	}
	b := sha512.Sum512([]byte(seed))
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X", b[32], b[34], b[36], b[38], b[40], b[42])
}

func calcDevicePrivate(private, seed string) []byte {
	if private != "" {
		// 1. Decode private from HEX string
		if b, _ := hex.DecodeString(private); len(b) == ed25519.PrivateKeySize {
			// 2. Return if OK
			return b
		}
		// 3. Use private as seed if not zero
		seed = private
	}
	b := sha512.Sum512([]byte(seed))
	return ed25519.NewKeyFromSeed(b[:ed25519.SeedSize])
}
