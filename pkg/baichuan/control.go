package baichuan

import (
	"context"
	"fmt"
)

// Reboot sends a reboot command to the camera channel.
func (c *Client) Reboot(ctx context.Context, channel uint8) error {
	if err := c.Login(ctx); err != nil {
		return err
	}

	body := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?><Reboot><channel>%d</channel></Reboot>`, channel)

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDReboot,
		ChannelID: channel,
		Class:     classModernWithOffset,
		Body:      []byte(body),
	})
	if err != nil {
		return err
	}

	return resp.success()
}
