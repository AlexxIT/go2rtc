package camera

import (
	"errors"

	"github.com/AlexxIT/go2rtc/pkg/hap"
)

type Client struct {
	client *hap.Client
}

func NewClient(client *hap.Client) *Client {
	return &Client{client: client}
}

func (c *Client) StartStream(ses *Session) error {
	// Step 1. Check if camera ready (free) to stream
	srv, err := c.GetFreeStream()
	if err != nil {
		return err
	}
	if srv == nil {
		return errors.New("no free streams")
	}

	if ses.Answer, err = c.SetupEndpoins(srv, ses.Offer); err != nil {
		return err
	}

	return c.SetConfig(srv, ses.Config)
}

// GetFreeStream search free streaming service.
// Usual every HomeKit camera can stream only to two clients simultaniosly.
// So it has two similar services for streaming.
func (c *Client) GetFreeStream() (srv *hap.Service, err error) {
	accs, err := c.client.GetAccessories()
	if err != nil {
		return
	}

	for _, srv = range accs[0].Services {
		for _, char := range srv.Characters {
			if char.Type == TypeStreamingStatus {
				var status StreamingStatus
				if err = char.ReadTLV8(&status); err != nil {
					return
				}

				if status.Status == StreamingStatusAvailable {
					return
				}
			}
		}
	}

	return nil, nil
}

func (c *Client) SetupEndpoins(srv *hap.Service, req *SetupEndpoints) (res *SetupEndpointsResponse, err error) {
	// get setup endpoint character ID
	char := srv.GetCharacter(TypeSetupEndpoints)
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
	res = &SetupEndpointsResponse{}
	if err = char.ReadTLV8(res); err != nil {
		return
	}

	return
}

func (c *Client) SetConfig(srv *hap.Service, config *SelectedStreamConfig) error {
	// get setup endpoint character ID
	char := srv.GetCharacter(TypeSelectedStreamConfiguration)
	char.Event = nil
	// encode new character value
	if err := char.Write(config); err != nil {
		return err
	}
	// write (put) new character value to device
	return c.client.PutCharacters(char)
}
