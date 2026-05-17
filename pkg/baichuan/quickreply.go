package baichuan

import (
	"context"
	"fmt"
)

// PlayQuickReply plays a pre-recorded audio file on the camera's speaker.
func (c *Client) PlayQuickReply(ctx context.Context, channel uint8, fileID int) error {
	if err := c.Login(ctx); err != nil {
		return err
	}

	body := fmt.Sprintf(quickReplyPlayXML, channel, fileID)

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDQuickReplyPlay,
		ChannelID: channel,
		Class:     classModernWithOffset,
		Body:      []byte(body),
	})
	if err != nil {
		return err
	}

	return resp.success()
}
