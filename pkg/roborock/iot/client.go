package iot

import (
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/rpc"
	"net/url"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/mqtt"
)

type Codec struct {
	mqtt *mqtt.Client

	devTopic string
	devKey   string

	body json.RawMessage
}

type dps struct {
	Dps struct {
		Req string `json:"101,omitempty"`
		Res string `json:"102,omitempty"`
	} `json:"dps"`
	T uint32 `json:"t"`
}

type response struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Codec) WriteRequest(r *rpc.Request, v any) error {
	if v == nil {
		v = "[]"
	}

	ts := uint32(time.Now().Unix())
	msg := dps{T: ts}
	msg.Dps.Req = fmt.Sprintf(
		`{"id":%d,"method":"%s","params":%s}`, r.Seq, r.ServiceMethod, v,
	)

	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	//log.Printf("[roborock] send: %s", payload)

	payload = c.Encrypt(payload, ts, ts, ts)

	return c.mqtt.Publish("rr/m/i/"+c.devTopic, payload)
}

func (c *Codec) ReadResponseHeader(r *rpc.Response) error {
	for {
		// receive any message from MQTT
		_, payload, err := c.mqtt.Read()
		if err != nil {
			return err
		}

		// skip if it is not PUBLISH message
		if payload == nil {
			continue
		}

		// decrypt MQTT PUBLISH payload
		if payload, err = c.Decrypt(payload); err != nil {
			continue
		}

		// skip if we can't decrypt this payload (ex. binary payload)
		if payload == nil {
			continue
		}

		//log.Printf("[roborock] recv %s", payload)

		// get content from response payload:
		// {"t":1676871268,"dps":{"102":"{\"id\":315003,\"result\":[\"ok\"]}"}}
		var msg dps
		if err = json.Unmarshal(payload, &msg); err != nil {
			continue
		}

		var res response
		if err = json.Unmarshal([]byte(msg.Dps.Res), &res); err != nil {
			continue
		}

		r.Seq = res.ID
		if res.Error.Code != 0 {
			r.Error = res.Error.Message
		} else {
			c.body = res.Result
		}

		return nil
	}
}

func (c *Codec) ReadResponseBody(v any) error {
	switch vv := v.(type) {
	case *[]byte:
		*vv = c.body
	case *string:
		*vv = string(c.body)
	case *bool:
		*vv = string(c.body) == `["ok"]`
	}
	return nil
}

func (c *Codec) Close() error {
	return c.mqtt.Close()
}

func Dial(rawURL string) (*rpc.Client, error) {
	link, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	// dial to MQTT
	conn, err := net.DialTimeout("tcp", link.Host, time.Second*5)
	if err != nil {
		return nil, err
	}

	// process MQTT SSL
	conf := &tls.Config{ServerName: link.Hostname()}
	sconn := tls.Client(conn, conf)
	if err = sconn.Handshake(); err != nil {
		return nil, err
	}

	query := link.Query()

	// send MQTT login
	uk := md5.Sum([]byte(query.Get("u") + ":" + query.Get("k")))
	sk := md5.Sum([]byte(query.Get("s") + ":" + query.Get("k")))
	user := hex.EncodeToString(uk[1:5])
	pass := hex.EncodeToString(sk[8:])

	c := &Codec{
		mqtt:     mqtt.NewClient(sconn),
		devKey:   query.Get("key"),
		devTopic: query.Get("u") + "/" + user + "/" + query.Get("did"),
	}

	if err = c.mqtt.Connect("com.roborock.smart:mbrriot", user, pass); err != nil {
		return nil, err
	}

	// subscribe on device topic
	if err = c.mqtt.Subscribe("rr/m/o/" + c.devTopic); err != nil {
		return nil, err
	}

	return rpc.NewClientWithCodec(c), nil
}
