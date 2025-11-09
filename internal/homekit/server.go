package homekit

import (
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg"
	srtp2 "github.com/AlexxIT/go2rtc/internal/srtp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/hap/hds"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	"github.com/AlexxIT/go2rtc/pkg/homekit"
	"github.com/AlexxIT/go2rtc/pkg/magic"
	"github.com/AlexxIT/go2rtc/pkg/mdns"
)

type server struct {
	hap  *hap.Server // server for HAP connection and encryption
	mdns *mdns.ServiceEntry

	pairings []string // pairings list
	conns    []any
	mu       sync.Mutex

	accessory *hap.Accessory // HAP accessory
	consumer  *homekit.Consumer
	proxyURL  string
	stream    string // stream name from YAML
}

func (s *server) MarshalJSON() ([]byte, error) {
	v := struct {
		Name     string `json:"name"`
		DeviceID string `json:"device_id"`
		Paired   int    `json:"paired"`
		Conns    []any  `json:"connections"`
	}{
		Name:     s.mdns.Name,
		DeviceID: s.mdns.Info[hap.TXTDeviceID],
		Paired:   len(s.pairings),
		Conns:    s.conns,
	}
	return json.Marshal(v)
}

func (s *server) Handle(w http.ResponseWriter, r *http.Request) {
	conn, rw, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}

	defer conn.Close()

	// Fix reading from Body after Hijack.
	r.Body = io.NopCloser(rw)

	switch r.RequestURI {
	case hap.PathPairSetup:
		id, key, err := s.hap.PairSetup(r, rw)
		if err != nil {
			log.Error().Err(err).Caller().Send()
			return
		}

		s.AddPair(id, key, hap.PermissionAdmin)

	case hap.PathPairVerify:
		id, key, err := s.hap.PairVerify(r, rw)
		if err != nil {
			log.Debug().Err(err).Caller().Send()
			return
		}

		log.Debug().Str("stream", s.stream).Str("client_id", id).Msgf("[homekit] %s: new conn", conn.RemoteAddr())

		controller, err := hap.NewConn(conn, rw, key, false)
		if err != nil {
			log.Error().Err(err).Caller().Send()
			return
		}

		s.AddConn(controller)
		defer s.DelConn(controller)

		var handler homekit.HandlerFunc

		switch {
		case s.accessory != nil:
			handler = homekit.ServerHandler(s)
		case s.proxyURL != "":
			client, err := hap.Dial(s.proxyURL)
			if err != nil {
				log.Error().Err(err).Caller().Send()
				return
			}
			handler = homekit.ProxyHandler(s, client.Conn)
		}

		// If your iPhone goes to sleep, it will be an EOF error.
		if err = handler(controller); err != nil && !errors.Is(err, io.EOF) {
			log.Error().Err(err).Caller().Send()
			return
		}
	}
}

type logger struct {
	v any
}

func (l logger) String() string {
	switch v := l.v.(type) {
	case *hap.Conn:
		return "hap " + v.RemoteAddr().String()
	case *hds.Conn:
		return "hds " + v.RemoteAddr().String()
	case *homekit.Consumer:
		return "rtp " + v.RemoteAddr
	}
	return "unknown"
}

func (s *server) AddConn(v any) {
	log.Trace().Str("stream", s.stream).Msgf("[homekit] add conn %s", logger{v})
	s.mu.Lock()
	s.conns = append(s.conns, v)
	s.mu.Unlock()
}

func (s *server) DelConn(v any) {
	log.Trace().Str("stream", s.stream).Msgf("[homekit] del conn %s", logger{v})
	s.mu.Lock()
	if i := slices.Index(s.conns, v); i >= 0 {
		s.conns = slices.Delete(s.conns, i, i+1)
	}
	s.mu.Unlock()
}

func (s *server) UpdateStatus() {
	// true status is important, or device may be offline in Apple Home
	if len(s.pairings) == 0 {
		s.mdns.Info[hap.TXTStatusFlags] = hap.StatusNotPaired
	} else {
		s.mdns.Info[hap.TXTStatusFlags] = hap.StatusPaired
	}
}

func (s *server) pairIndex(id string) int {
	id = "client_id=" + id
	for i, pairing := range s.pairings {
		if strings.HasPrefix(pairing, id) {
			return i
		}
	}
	return -1
}

func (s *server) GetPair(id string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	if i := s.pairIndex(id); i >= 0 {
		query, _ := url.ParseQuery(s.pairings[i])
		b, _ := hex.DecodeString(query.Get("client_public"))
		return b
	}
	return nil
}

func (s *server) AddPair(id string, public []byte, permissions byte) {
	log.Debug().Str("stream", s.stream).Msgf("[homekit] add pair id=%s public=%x perm=%d", id, public, permissions)

	s.mu.Lock()
	if s.pairIndex(id) < 0 {
		s.pairings = append(s.pairings, fmt.Sprintf(
			"client_id=%s&client_public=%x&permissions=%d", id, public, permissions,
		))
		s.UpdateStatus()
		s.PatchConfig()
	}
	s.mu.Unlock()
}

func (s *server) DelPair(id string) {
	log.Debug().Str("stream", s.stream).Msgf("[homekit] del pair id=%s", id)

	s.mu.Lock()
	if i := s.pairIndex(id); i >= 0 {
		s.pairings = append(s.pairings[:i], s.pairings[i+1:]...)
		s.UpdateStatus()
		s.PatchConfig()
	}
	s.mu.Unlock()
}

func (s *server) PatchConfig() {
	if err := app.PatchConfig([]string{"homekit", s.stream, "pairings"}, s.pairings); err != nil {
		log.Error().Err(err).Msgf(
			"[homekit] can't save %s pairings=%v", s.stream, s.pairings,
		)
	}
}

func (s *server) GetAccessories(_ net.Conn) []*hap.Accessory {
	return []*hap.Accessory{s.accessory}
}

func (s *server) GetCharacteristic(conn net.Conn, aid uint8, iid uint64) any {
	log.Trace().Str("stream", s.stream).Msgf("[homekit] get char aid=%d iid=0x%x", aid, iid)

	char := s.accessory.GetCharacterByID(iid)
	if char == nil {
		log.Warn().Msgf("[homekit] get unknown characteristic: %d", iid)
		return nil
	}

	switch char.Type {
	case camera.TypeSetupEndpoints:
		consumer := s.consumer
		if consumer == nil {
			return nil
		}

		answer := consumer.GetAnswer()
		v, err := tlv8.MarshalBase64(answer)
		if err != nil {
			return nil
		}

		return v
	}

	return char.Value
}

func (s *server) SetCharacteristic(conn net.Conn, aid uint8, iid uint64, value any) {
	log.Trace().Str("stream", s.stream).Msgf("[homekit] set char aid=%d iid=0x%x value=%v", aid, iid, value)

	char := s.accessory.GetCharacterByID(iid)
	if char == nil {
		log.Warn().Msgf("[homekit] set unknown characteristic: %d", iid)
		return
	}

	switch char.Type {
	case camera.TypeSetupEndpoints:
		var offer camera.SetupEndpointsRequest
		if err := tlv8.UnmarshalBase64(value, &offer); err != nil {
			return
		}

		consumer := homekit.NewConsumer(conn, srtp2.Server)
		consumer.SetOffer(&offer)
		s.consumer = consumer

	case camera.TypeSelectedStreamConfiguration:
		var conf camera.SelectedStreamConfiguration
		if err := tlv8.UnmarshalBase64(value, &conf); err != nil {
			return
		}

		log.Trace().Str("stream", s.stream).Msgf("[homekit] stream id=%x cmd=%d", conf.Control.SessionID, conf.Control.Command)

		switch conf.Control.Command {
		case camera.SessionCommandEnd:
			for _, consumer := range s.conns {
				if consumer, ok := consumer.(*homekit.Consumer); ok {
					if consumer.SessionID() == conf.Control.SessionID {
						_ = consumer.Stop()
						return
					}
				}
			}

		case camera.SessionCommandStart:
			consumer := s.consumer
			if consumer == nil {
				return
			}

			if !consumer.SetConfig(&conf) {
				log.Warn().Msgf("[homekit] wrong config")
				return
			}

			s.AddConn(consumer)

			stream := streams.Get(s.stream)
			if err := stream.AddConsumer(consumer); err != nil {
				return
			}

			go func() {
				_, _ = consumer.WriteTo(nil)
				stream.RemoveConsumer(consumer)

				s.DelConn(consumer)
			}()
		}
	}
}

func (s *server) GetImage(conn net.Conn, width, height int) []byte {
	log.Trace().Str("stream", s.stream).Msgf("[homekit] get image width=%d height=%d", width, height)

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
