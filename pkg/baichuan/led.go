package baichuan

import (
	"context"
	"fmt"
)

// SetWhiteLed enables or disables the white LED (floodlight).
// status: 1 = on, 0 = off
func (c *Client) SetWhiteLed(ctx context.Context, channel uint8, status int) error {
	if err := c.Login(ctx); err != nil {
		return err
	}

	body := fmt.Sprintf(setWhiteLedXML, channel, status)

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDWhiteLedSet,
		ChannelID: channel,
		Class:     classModernWithOffset,
		Body:      []byte(body),
	})
	if err != nil {
		return err
	}

	return resp.success()
}

// GetWhiteLed retrieves the current state of the floodlight.
func (c *Client) GetWhiteLed(ctx context.Context, channel uint8) (*Message, error) {
	if err := c.Login(ctx); err != nil {
		return nil, err
	}

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDWhiteLedGet,
		ChannelID: channel,
		Class:     classModernWithOffset,
	})
	if err != nil {
		return nil, err
	}

	if err := resp.success(); err != nil {
		return nil, err
	}

	return resp, nil
}
