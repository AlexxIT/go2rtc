package camera

import (
	"errors"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/brutella/hap/characteristic"
	"github.com/brutella/hap/rtp"
)

type Client struct {
	client *hap.Conn
}

func NewClient(client *hap.Conn) *Client {
	return &Client{client: client}
}

func (c *Client) StartStream(ses *Session) (err error) {
	// Step 1. Check if camera ready (free) to stream
	var srv *hap.Service
	if srv, err = c.GetFreeStream(); err != nil {
		return err
	}
	if srv == nil {
		return errors.New("no free streams")
	}

	if ses.Answer, err = c.SetupEndpoins(srv, ses.Offer); err != nil {
		return
	}

	return c.SetConfig(srv, ses.Config)
}

// GetFreeStream search free streaming service.
// Usual every HomeKit camera can stream only to two clients simultaniosly.
// So it has two similar services for streaming.
func (c *Client) GetFreeStream() (srv *hap.Service, err error) {
	var accs []*hap.Accessory
	if accs, err = c.client.GetAccessories(); err != nil {
		return
	}

	for _, srv = range accs[0].Services {
		for _, char := range srv.Characters {
			if char.Type == characteristic.TypeStreamingStatus {
				status := rtp.StreamingStatus{}
				if err = char.ReadTLV8(&status); err != nil {
					return
				}

				if status.Status == rtp.SessionStatusSuccess {
					return
				}
			}
		}
	}

	return nil, nil
}

func (c *Client) SetupEndpoins(
	srv *hap.Service, req *rtp.SetupEndpoints,
) (res *rtp.SetupEndpointsResponse, err error) {
	// get setup endpoint character ID
	char := srv.GetCharacter(characteristic.TypeSetupEndpoints)
	char.Event = nil
	// encode new character value
	if err = char.Write(req); err != nil {
		return
	}
	// write (put) new endpoint value to device
	if err = c.client.PutCharacters(char); err != nil {
		return
	}

	// get new endpoint value from device (response)
	if err = c.client.GetCharacter(char); err != nil {
		return
	}
	// decode new endpoint value
	res = &rtp.SetupEndpointsResponse{}
	if err = char.ReadTLV8(res); err != nil {
		return
	}

	return
}

func (c *Client) SetConfig(srv *hap.Service, config *rtp.StreamConfiguration) (err error) {
	// get setup endpoint character ID
	char := srv.GetCharacter(characteristic.TypeSelectedStreamConfiguration)
	char.Event = nil
	// encode new character value
	if err = char.Write(config); err != nil {
		panic(err)
	}
	// write (put) new character value to device
	return c.client.PutCharacters(char)
}
