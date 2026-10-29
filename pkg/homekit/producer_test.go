package homekit

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
)

// Model a camera that keeps its stream reserved after the HAP connection closes.
func TestProducerReleasesHomeKitStream(t *testing.T) {
	for _, failStart := range []bool{false, true} {
		t.Run(fmt.Sprint("failStart=", failStart), func(t *testing.T) {
			acc := camera.NewAccessory("test", "camera", "test", "1", "1", "")
			service := acc.GetService("110")
			status := service.GetCharacter(camera.TypeStreamingStatus)
			setup := service.GetCharacter(camera.TypeSetupEndpoints)
			selected := service.GetCharacter(camera.TypeSelectedStreamConfiguration)
			var ends atomic.Int32
			var reserved atomic.Bool
			_, deviceKey, _ := ed25519.GenerateKey(nil)
			_, clientKey, _ := ed25519.GenerateKey(nil)
			server := &hap.Server{DeviceID: "test-camera", DevicePrivate: deviceKey}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
				conn, rw, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				rw.Reader = bufio.NewReader(conn)
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				_, key, err := server.PairVerify(r, rw)
				if err != nil {
					t.Error(err)
					return
				}
				secure, err := hap.NewConn(conn, rw, key, false)
				if err != nil {
					t.Error(err)
					return
				}
				reader := bufio.NewReader(secure)
				writer := bufio.NewWriter(secure)
				for {
					req, err := http.ReadRequest(reader)
					if err != nil {
						if err != io.EOF {
							t.Error(err)
						}
						return
					}
					code := http.StatusOK
					var body any
					switch req.URL.Path {
					case hap.PathAccessories:
						body = hap.JSONAccessories{Value: []*hap.Accessory{acc}}
					case hap.PathCharacteristics:
						if req.Method == "GET" {
							var iid uint64
							fmt.Sscanf(req.URL.Query().Get("id"), "1.%d", &iid)
							body = hap.JSONCharacters{Value: []hap.JSONCharacter{{AID: 1, IID: iid, Value: acc.GetCharacterByID(iid).Value}}}
						} else {
							var v hap.JSONCharacters
							if err = json.NewDecoder(req.Body).Decode(&v); err != nil {
								t.Error(err)
								return
							}
							code = http.StatusNoContent
							for _, v := range v.Value {
								ch := acc.GetCharacterByID(v.IID)
								ch.Value = v.Value
								if ch == setup {
									var offer camera.SetupEndpointsRequest
									if err = ch.ReadTLV8(&offer); err != nil {
										t.Error(err)
										return
									}
									reserved.Store(true)
									status.Write(camera.StreamingStatus{Status: camera.StreamingStatusInUse})
									setup.Write(camera.SetupEndpointsResponse{SessionID: offer.SessionID, Address: offer.Address, VideoCrypto: offer.VideoCrypto, AudioCrypto: offer.AudioCrypto, VideoSSRC: 1, AudioSSRC: 2})
								} else if ch == selected {
									var config camera.SelectedStreamConfiguration
									if err = ch.ReadTLV8(&config); err != nil {
										t.Error(err)
										return
									}
									if config.Control.Command == camera.SessionCommandEnd {
										ends.Add(1)
										reserved.Store(false)
										status.Write(camera.StreamingStatus{Status: camera.StreamingStatusAvailable})
									} else if failStart {
										code = http.StatusInternalServerError
									}
								}
							}
						}
					}
					req.Body.Close()
					data, _ := json.Marshal(body)
					if code == http.StatusNoContent {
						data = nil
					}
					if err = hap.WriteResponse(writer, code, hap.MimeJSON, data); err != nil {
						t.Error(err)
						return
					}
				}
			}))
			defer srv.Close()
			url := fmt.Sprintf("homekit://%s?device_id=test-camera&device_public=%x&client_id=test-client&client_private=%x", strings.TrimPrefix(srv.URL, "http://"), deviceKey[32:], clientKey)
			h, err := hap.Dial(url)
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()
			video := &srtp.Session{Local: &srtp.Endpoint{Addr: "127.0.0.1", Port: 1234}}
			audio := &srtp.Session{Local: &srtp.Endpoint{Addr: "127.0.0.1", Port: 1234}}
			stream, err := camera.NewStream(h, &camera.VideoCodecConfiguration{}, &camera.AudioCodecConfiguration{}, video, audio, 0)
			if failStart {
				if err == nil {
					t.Fatal("expected failed start")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				c := &Client{Connection: core.Connection{Transport: h}, hap: h, stream: stream}
				if err = c.Stop(); err != nil {
					t.Fatal(err)
				}
				if err = c.Stop(); err != nil {
					t.Fatal(err)
				}
				c.Receivers = []*core.Receiver{core.NewReceiver(&core.Media{Kind: core.KindVideo}, &core.Codec{Name: core.CodecH264})}
				if err = c.Start(); !errors.Is(err, net.ErrClosed) {
					t.Fatalf("start after stop: %v", err)
				}
			}
			if reserved.Load() || ends.Load() != 1 {
				t.Fatalf("camera slot leaked: reserved=%v end commands=%d", reserved.Load(), ends.Load())
			}
		})
	}
}
