package baichuan

import (
	"context"
	"fmt"
)

// SetAutoFocus enables or disables auto-focus on supported cameras.
// disable: 1 = manual focus (auto-focus off), 0 = auto-focus on
func (c *Client) SetAutoFocus(ctx context.Context, channel uint8, disable int) error {
	if err := c.Login(ctx); err != nil {
		return err
	}

	body := fmt.Sprintf(setAutoFocusXML, channel, disable)

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDAutoFocusSet,
		ChannelID: channel,
		Class:     classModernWithOffset,
		Body:      []byte(body),
	})
	if err != nil {
		return err
	}

	return resp.success()
}

// GetAutoFocus retrieves the current state of auto-focus.
func (c *Client) GetAutoFocus(ctx context.Context, channel uint8) (*Message, error) {
	if err := c.Login(ctx); err != nil {
		return nil, err
	}

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDAutoFocusGet,
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
