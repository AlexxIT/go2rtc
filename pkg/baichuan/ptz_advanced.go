package baichuan

import (
	"context"
	"fmt"
)

// PtzGuard sets the guard position or patrol for a PTZ camera.
func (c *Client) PtzGuard(ctx context.Context, channel uint8, enable int, cmdStr string, timeout int, setPos int) error {
	if err := c.Login(ctx); err != nil {
		return err
	}

	body := fmt.Sprintf(ptzGuardXML, channel, enable, cmdStr, timeout, setPos)

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDPtzGuardSet,
		ChannelID: channel,
		Class:     classModernWithOffset,
		Body:      []byte(body),
	})
	if err != nil {
		return err
	}

	return resp.success()
}

// Ptz3DLocation zooms or centers the camera onto a specific 3D box region.
func (c *Client) Ptz3DLocation(ctx context.Context, channel uint8, posX, posY, posWidth, posHeight, speed, width, height int) error {
	if err := c.Login(ctx); err != nil {
		return err
	}

	body := fmt.Sprintf(ptz3DLocationXML, channel, posX, posY, posWidth, posHeight, speed, width, height)

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDPtz3DLocation,
		ChannelID: channel,
		Class:     classModernWithOffset,
		Body:      []byte(body),
	})
	if err != nil {
		return err
	}

	return resp.success()
}
