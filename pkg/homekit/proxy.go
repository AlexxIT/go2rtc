package homekit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/hap/hds"
	"github.com/AlexxIT/go2rtc/pkg/hap/secure"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
)

func ProxyHandler(pair ServerPair, dial func() (net.Conn, error)) hap.HandlerFunc {
	return func(con net.Conn) error {
		defer con.Close()

		acc, err := dial()
		if err != nil {
			return err
		}
		defer acc.Close()

		pr := &Proxy{
			con: con.(*secure.Conn),
			acc: acc.(*secure.Conn),
			res: make(chan *http.Response),
		}

		// accessory (ex. Camera) => controller (ex. iPhone)
		go pr.handleAcc()

		// controller => accessory
		return pr.handleCon(pair)
	}
}

type Proxy struct {
	con *secure.Conn
	acc *secure.Conn
	res chan *http.Response
}

func (p *Proxy) handleCon(pair ServerPair) error {
	var hdsCharIID uint64

	rd := bufio.NewReader(p.con)
	for {
		req, err := http.ReadRequest(rd)
		if err != nil {
			return err
		}

		var hdsConSalt string

		switch {
		case req.Method == "POST" && req.URL.Path == hap.PathPairings:
			var res *http.Response
			if res, err = handlePairings(p.con, req, pair); err != nil {
				return err
			}
			if err = res.Write(p.con); err != nil {
				return err
			}
			continue
		case req.Method == "PUT" && req.URL.Path == hap.PathCharacteristics && hdsCharIID != 0:
			body, _ := io.ReadAll(req.Body)
			var v hap.JSONCharacters
			_ = json.Unmarshal(body, &v)
			for _, char := range v.Value {
				if char.IID == hdsCharIID {
					var hdsReq camera.SetupDataStreamRequest
					_ = tlv8.UnmarshalBase64(char.Value, &hdsReq)
					hdsConSalt = hdsReq.ControllerKeySalt
					break
				}
			}
			req.Body = io.NopCloser(bytes.NewReader(body))
		}

		if err = req.Write(p.acc); err != nil {
			return err
		}

		res := <-p.res

		switch {
		case req.Method == "GET" && req.URL.Path == hap.PathAccessories:
			body, _ := io.ReadAll(res.Body)
			var v hap.JSONAccessories
			if err = json.Unmarshal(body, &v); err != nil {
				return err
			}
			for _, acc := range v.Value {
				if char := acc.GetCharacter(camera.TypeSetupDataStreamTransport); char != nil {
					hdsCharIID = char.IID
				}
				break
			}
			res.Body = io.NopCloser(bytes.NewReader(body))

		case hdsConSalt != "":
			body, _ := io.ReadAll(res.Body)
			var v hap.JSONCharacters
			_ = json.Unmarshal(body, &v)
			for i, char := range v.Value {
				if char.IID == hdsCharIID {
					var hdsRes camera.SetupDataStreamResponse
					_ = tlv8.UnmarshalBase64(char.Value, &hdsRes)

					hdsAccSalt := hdsRes.AccessoryKeySalt
					hdsPort := int(hdsRes.TransportTypeSessionParameters.TCPListeningPort)

					// swtich accPort to conPort
					hdsPort, err = p.listenHDS(hdsPort, hdsConSalt+hdsAccSalt)
					if err != nil {
						return err
					}

					hdsRes.TransportTypeSessionParameters.TCPListeningPort = uint16(hdsPort)
					if v.Value[i].Value, err = tlv8.MarshalBase64(hdsRes); err != nil {
						return err
					}
					body, _ = json.Marshal(v)
					res.ContentLength = int64(len(body))
					break
				}
			}
			res.Body = io.NopCloser(bytes.NewReader(body))
		}

		if err = res.Write(p.con); err != nil {
			return err
		}
	}
}

func (p *Proxy) handleAcc() error {
	rd := bufio.NewReader(p.acc)
	for {
		res, err := hap.ReadResponse(rd, nil)
		if err != nil {
			return err
		}

		if res.Proto == hap.ProtoEvent {
			if err = res.Write(p.con); err != nil {
				return err
			}
			continue
		}

		// important to read body before next read response
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}
		res.Body = io.NopCloser(bytes.NewReader(body))

		p.res <- res
	}
}

func (p *Proxy) listenHDS(accPort int, salt string) (int, error) {
	ln, err := net.ListenTCP("tcp", nil)
	if err != nil {
		return 0, err
	}

	go func() {
		defer ln.Close()

		// raw controller conn
		con, err := ln.Accept()
		if err != nil {
			return
		}
		defer con.Close()

		// secured controller conn (controlle=false because we are accessory)
		con, err = hds.Client(con, p.con.SharedKey, salt, false)
		if err != nil {
			return
		}

		accIP := p.acc.RemoteAddr().(*net.TCPAddr).IP

		// raw accessory conn
		acc, err := net.Dial("tcp", fmt.Sprintf("%s:%d", accIP, accPort))
		if err != nil {
			return
		}
		defer acc.Close()

		// secured accessory conn (controller=true because we are controller)
		acc, err = hds.Client(acc, p.acc.SharedKey, salt, true)
		if err != nil {
			return
		}

		go io.Copy(con, acc)
		_, _ = io.Copy(acc, con)
	}()

	conPort := ln.Addr().(*net.TCPAddr).Port
	return conPort, nil
}
