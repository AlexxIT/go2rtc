package baichuan

import (
	"context"
	"fmt"
)

// Siren triggers the camera's internal siren alarm to sound continuously (manual mode).
// Set enable to 1 to turn on, 0 to turn off.
func (c *Client) Siren(ctx context.Context, channel uint8, enable int) error {
	if err := c.Login(ctx); err != nil {
		return err
	}

	body := fmt.Sprintf(sirenManualXML, channel, enable)

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDPlayAudio,
		ChannelID: channel,
		Class:     classModernWithOffset,
		Body:      []byte(body),
	})
	if err != nil {
		return err
	}

	return resp.success()
}

// SirenTimes triggers the camera's internal siren alarm to sound for a specific number of times.
func (c *Client) SirenTimes(ctx context.Context, channel uint8, times int) error {
	if err := c.Login(ctx); err != nil {
		return err
	}

	body := fmt.Sprintf(sirenTimesXML, channel, times)

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDPlayAudio,
		ChannelID: channel,
		Class:     classModernWithOffset,
		Body:      []byte(body),
	})
	if err != nil {
		return err
	}

	return resp.success()
}

// SirenHub triggers the Hub's internal siren alarm to sound continuously (manual mode).
// Set enable to 1 to turn on, 0 to turn off.
func (c *Client) SirenHub(ctx context.Context, enable int) error {
	if err := c.Login(ctx); err != nil {
		return err
	}

	body := fmt.Sprintf(sirenHubManualXML, enable)

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDPlayAudio,
		ChannelID: 0, // Hubs use channel 0 or omit it
		Class:     classModernWithOffset,
		Body:      []byte(body),
	})
	if err != nil {
		return err
	}

	return resp.success()
}

// SirenHubTimes triggers the Hub's internal siren alarm to sound for a specific number of times.
func (c *Client) SirenHubTimes(ctx context.Context, times int) error {
	if err := c.Login(ctx); err != nil {
		return err
	}

	body := fmt.Sprintf(sirenHubTimesXML, times)

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDPlayAudio,
		ChannelID: 0,
		Class:     classModernWithOffset,
		Body:      []byte(body),
	})
	if err != nil {
		return err
	}

	return resp.success()
}
