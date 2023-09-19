package homekit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
)

type Server interface {
	ServerPair
	ServerAccessory
}

type ServerPair interface {
	GetPair(conn net.Conn, id string) []byte
	AddPair(conn net.Conn, id string, public []byte, permissions byte)
	DelPair(conn net.Conn, id string)
}

type ServerAccessory interface {
	GetAccessories(conn net.Conn) []*hap.Accessory
	GetCharacteristic(conn net.Conn, aid uint8, iid uint64) any
	SetCharacteristic(conn net.Conn, aid uint8, iid uint64, value any)
	GetImage(conn net.Conn, width, height int) []byte
}

func ServerHandler(server Server) hap.HandlerFunc {
	return handleRequest(func(conn net.Conn, req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case hap.PathPairings:
			return handlePairings(conn, req, server)

		case hap.PathAccessories:
			body := hap.JSONAccessories{Value: server.GetAccessories(conn)}
			return makeResponse(hap.MimeJSON, body)

		case hap.PathCharacteristics:
			switch req.Method {
			case "GET":
				var v hap.JSONCharacters

				id := req.URL.Query().Get("id")
				for _, id = range strings.Split(id, ",") {
					s1, s2, _ := strings.Cut(id, ".")
					aid, _ := strconv.Atoi(s1)
					iid, _ := strconv.ParseUint(s2, 10, 64)
					val := server.GetCharacteristic(conn, uint8(aid), iid)

					v.Value = append(v.Value, hap.JSONCharacter{AID: uint8(aid), IID: iid, Value: val})
				}

				return makeResponse(hap.MimeJSON, v)

			case "PUT":
				var v struct {
					Value []struct {
						AID   uint8  `json:"aid"`
						IID   uint64 `json:"iid"`
						Value any    `json:"value"`
					} `json:"characteristics"`
				}
				if err := json.NewDecoder(req.Body).Decode(&v); err != nil {
					return nil, err
				}

				for _, char := range v.Value {
					server.SetCharacteristic(conn, char.AID, char.IID, char.Value)
				}

				res := &http.Response{
					StatusCode: http.StatusNoContent,
					Proto:      "HTTP",
					ProtoMajor: 1,
					ProtoMinor: 1,
				}
				return res, nil
			}

		case hap.PathResource:
			var v struct {
				Width  int    `json:"image-width"`
				Height int    `json:"image-height"`
				Type   string `json:"resource-type"`
			}
			if err := json.NewDecoder(req.Body).Decode(&v); err != nil {
				return nil, err
			}

			body := server.GetImage(conn, v.Width, v.Height)
			return makeResponse("image/jpeg", body)
		}

		return nil, errors.New("hap: unsupported path: " + req.RequestURI)
	})
}

func handleRequest(handle func(conn net.Conn, req *http.Request) (*http.Response, error)) hap.HandlerFunc {
	return func(conn net.Conn) error {
		rw := bufio.NewReaderSize(conn, 16*1024)
		wr := bufio.NewWriterSize(conn, 16*1024)
		for {
			req, err := http.ReadRequest(rw)
			//debug(req)
			if err != nil {
				return err
			}

			res, err := handle(conn, req)
			//debug(res)
			if err != nil {
				return err
			}

			if err = res.Write(wr); err != nil {
				return err
			}
			if err = wr.Flush(); err != nil {
				return err
			}
		}
	}
}

func handlePairings(conn net.Conn, req *http.Request, pair ServerPair) (*http.Response, error) {
	cmd := struct {
		Method      byte   `tlv8:"0"`
		Identifier  string `tlv8:"1"`
		PublicKey   string `tlv8:"3"`
		State       byte   `tlv8:"6"`
		Permissions byte   `tlv8:"11"`
	}{}

	if err := tlv8.UnmarshalReader(req.Body, &cmd); err != nil {
		return nil, err
	}

	switch cmd.Method {
	case 3: // add
		pair.AddPair(conn, cmd.Identifier, []byte(cmd.PublicKey), cmd.Permissions)
	case 4: // delete
		pair.DelPair(conn, cmd.Identifier)
	}

	body := struct {
		State byte `tlv8:"6"`
	}{
		State: hap.StateM2,
	}

	return makeResponse(hap.MimeTLV8, body)
}

func makeResponse(mime string, v any) (*http.Response, error) {
	var body []byte
	var err error

	switch mime {
	case hap.MimeJSON:
		body, err = json.Marshal(v)
	case hap.MimeTLV8:
		body, err = tlv8.Marshal(v)
	case "image/jpeg":
		body = v.([]byte)
	}

	if err != nil {
		return nil, err
	}

	res := &http.Response{
		StatusCode: http.StatusOK,
		Proto:      "HTTP",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"Content-Type":   []string{mime},
			"Content-Length": []string{strconv.Itoa(len(body))},
		},
		ContentLength: int64(len(body)),
		Body:          io.NopCloser(bytes.NewReader(body)),
	}
	return res, nil
}

//func debug(v any) {
//	switch v := v.(type) {
//	case *http.Request:
//		if v == nil {
//			return
//		}
//		if v.ContentLength != 0 {
//			b, err := io.ReadAll(v.Body)
//			if err != nil {
//				panic(err)
//			}
//			v.Body = io.NopCloser(bytes.NewReader(b))
//			log.Printf("[homekit] request: %s %s\n%s", v.Method, v.RequestURI, b)
//		} else {
//			log.Printf("[homekit] request: %s %s <nobody>", v.Method, v.RequestURI)
//		}
//	case *http.Response:
//		if v == nil {
//			return
//		}
//		if v.Header.Get("Content-Type") == "image/jpeg" {
//			log.Printf("[homekit] response: %d <jpeg>", v.StatusCode)
//			return
//		}
//		if v.ContentLength != 0 {
//			b, err := io.ReadAll(v.Body)
//			if err != nil {
//				panic(err)
//			}
//			v.Body = io.NopCloser(bytes.NewReader(b))
//			log.Printf("[homekit] response: %d\n%s", v.StatusCode, b)
//		} else {
//			log.Printf("[homekit] response: %d <nobody>", v.StatusCode)
//		}
//	}
//}
