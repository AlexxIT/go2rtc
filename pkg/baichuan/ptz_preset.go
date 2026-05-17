package baichuan

import (
	"context"
	"fmt"
)

// PTZPreset moves the camera to a saved PTZ preset ID.
func (c *Client) PTZPreset(ctx context.Context, channel uint8, presetID int) error {
	if err := c.Login(ctx); err != nil {
		return err
	}

	body := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?><PtzPreset><channelId>%d</channelId><op>ToPos</op><id>%d</id></PtzPreset>`, channel, presetID)

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDPTZControlPreset,
		ChannelID: channel,
		Class:     classModernWithOffset,
		Body:      []byte(body),
	})
	if err != nil {
		return err
	}

	return resp.success()
}
