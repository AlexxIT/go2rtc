package baichuan

import (
	"context"
	"fmt"
)

// SetPrivacyMode puts the camera into privacy/sleep mode.
// enable: 1 = sleep/privacy, 0 = awake
func (c *Client) SetPrivacyMode(ctx context.Context, channel uint8, enable int) error {
	if err := c.Login(ctx); err != nil {
		return err
	}

	body := fmt.Sprintf(setPrivacyModeXML, enable)

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDPrivacyModeSet,
		ChannelID: channel,
		Class:     classModernWithOffset,
		Body:      []byte(body),
	})
	if err != nil {
		return err
	}

	return resp.success()
}

// GetPrivacyMode retrieves the current privacy/sleep mode state.
func (c *Client) GetPrivacyMode(ctx context.Context, channel uint8) (*Message, error) {
	if err := c.Login(ctx); err != nil {
		return nil, err
	}

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDPrivacyModeGet,
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
