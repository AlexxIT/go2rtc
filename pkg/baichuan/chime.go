package baichuan

import (
	"context"
	"fmt"
)

// RingChimeWithTone rings the chime using a specific tone
func (c *Client) RingChimeWithTone(ctx context.Context, channel uint8, chimeID int, toneID int) error {
	if err := c.Login(ctx); err != nil {
		return err
	}

	body := fmt.Sprintf(dingDongOpt4XML, chimeID, toneID)

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDDingDongOpt2, // in python opt 4 actually uses 485 (msgIDDingDongOpt2) for setting
		ChannelID: channel,
		Class:     classModernWithOffset,
		Body:      []byte(body),
	})
	if err != nil {
		return err
	}

	return resp.success()
}

// GetChimeConfig retrieves the configuration of a paired chime
func (c *Client) GetChimeConfig(ctx context.Context, channel uint8) (*Message, error) {
	if err := c.Login(ctx); err != nil {
		return nil, err
	}

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDDingDongGet,
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

// SetChimeSilentMode sets the silent mode (DND) on the chime for a specific duration (in minutes)
func (c *Client) SetChimeSilentMode(ctx context.Context, channel uint8, chimeID int, time int) error {
	if err := c.Login(ctx); err != nil {
		return err
	}

	body := fmt.Sprintf(setDingDongSilentXML, chimeID, time)

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDDingDongSilentSet,
		ChannelID: channel,
		Class:     classModernWithOffset,
		Body:      []byte(body),
	})
	if err != nil {
		return err
	}

	return resp.success()
}
