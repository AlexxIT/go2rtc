package baichuan

import (
	"context"
	"encoding/xml"
)

// AlarmEventList contains a list of alarm events from the camera.
type AlarmEventList struct {
	AlarmEvents []AlarmEvent `xml:"AlarmEvent"`
}

// AlarmEvent represents a single motion or AI alarm event.
type AlarmEvent struct {
	ChannelID uint8  `xml:"channelId"`
	Status    string `xml:"status"`
	AIType    string `xml:"AItype"`
}

// AlarmMessage is the XML payload containing an AlarmEventList.
type AlarmMessage struct {
	AlarmEventList *AlarmEventList `xml:"AlarmEventList"`
}

// ListenForMotion subscribes to motion events and invokes the callback when motion is detected.
func (c *Client) ListenForMotion(ctx context.Context, channel uint8, callback func(bool)) (func(), error) {
	if err := c.Login(ctx); err != nil {
		return nil, err
	}

	if err := c.requireAbilityRW(ctx, channel, "motion"); err != nil {
		return nil, err
	}

	if _, err := c.sendRequest(ctx, request{
		MsgID:     msgIDMotionRequest,
		ChannelID: channel,
		Class:     classModernWithOffset,
		Body:      nil,
	}); err != nil {
		return nil, err
	}

	motionSub, unsubscribeMotion := c.Subscribe(msgIDMotion)

	go func() {
		defer unsubscribeMotion()

		for {
			select {
			case <-ctx.Done():
				return
			case <-c.closed:
				return
			case msg := <-motionSub:
				if msg == nil {
					continue
				}
				motionDetected, matched, err := parseMotionState(msg.XML, channel)
				if err == nil && matched {
					callback(motionDetected)
				}
			}
		}
	}()

	return unsubscribeMotion, nil
}

func parseMotionState(xmlText string, channel uint8) (bool, bool, error) {
	if xmlText == "" {
		return false, false, nil
	}

	var payload AlarmMessage
	if err := xml.Unmarshal([]byte(xmlText), &payload); err != nil {
		return false, false, err
	}

	if payload.AlarmEventList == nil {
		return false, false, nil
	}

	for _, ev := range payload.AlarmEventList.AlarmEvents {
		if ev.ChannelID != channel {
			continue
		}
		if ev.Status != "none" || (ev.AIType != "" && ev.AIType != "none") {
			return true, true, nil
		}

		return false, true, nil
	}

	return false, false, nil
}
