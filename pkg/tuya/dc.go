package tuya

import (
	"encoding/json"
	"errors"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"
)

type DCConn struct {
	core.Connection

	client      *Client
	dc          *pion.DataChannel
	dem         *mp4.Demuxer
	queue       *FrameBufferQueue
	msgs        chan pion.DataChannelMessage
	connected   core.Waiter
	closed      core.Waiter
	initialized bool
}

type DataChannelMessage struct {
	Type string `json:"type"`
	Msg  string `json:"msg"`
}

func NewDCConn(pc *pion.PeerConnection, c *Client) (*DCConn, error) {
	maxRetransmits := uint16(5)
	ordered := true
	dc, err := pc.CreateDataChannel("fmp4Stream", &pion.DataChannelInit{
		MaxRetransmits: &maxRetransmits,
		Ordered:        &ordered,
	})

	if err != nil {
		return nil, err
	}

	conn := &DCConn{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "webrtc/fmp4",
			Transport:  dc,
		},
		client:      c,
		dc:          dc,
		dem:         &mp4.Demuxer{},
		queue:       NewFrameBufferQueue(),
		msgs:        make(chan pion.DataChannelMessage, 10), // Saw max 4 messages in a row, 10 should be enough
		initialized: false,
	}

	dc.OnMessage(func(msg pion.DataChannelMessage) {
		conn.msgs <- msg
	})

	dc.OnError(func(err error) {
		conn.connected.Done(err)
	})

	dc.OnClose(func() {
		close(conn.msgs)
		conn.connected.Done(errors.New("datachannel: closed"))
	})

	go conn.initializationLoop()

	return conn, nil
}

func (c *DCConn) initializationLoop() {
	for msg := range c.msgs {
		if c.initialized {
			return
		}

		err := c.probe(msg)
		if err != nil {
			c.connected.Done(err)
			return
		}

		if c.initialized {
			c.connected.Done(nil)
			return
		}
	}
}

func (c *DCConn) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	if media.Direction == core.DirectionSendRecv || media.Direction == core.DirectionSendonly {
		return c.client.GetTrack(media, codec)
	}

	for _, receiver := range c.Receivers {
		if receiver.Codec == codec {
			return receiver, nil
		}
	}
	receiver := core.NewReceiver(media, codec)
	c.Receivers = append(c.Receivers, receiver)
	return receiver, nil
}

func (c *DCConn) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	payloadType := codec.PayloadType
	localTrack := c.client.conn.GetSenderTrack(media.ID)
	sender := core.NewSender(media, codec)
	sender.Handler = func(packet *rtp.Packet) {
		c.Send += packet.MarshalSize()
		//important to send with remote PayloadType
		_ = localTrack.WriteRTP(payloadType, packet)
	}

	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)

	return nil
}

func (c *DCConn) Start() error {
	receivers := make(map[uint32]*core.Receiver)
	for _, receiver := range c.Receivers {
		trackID := c.dem.GetTrackID(receiver.Codec)
		receivers[trackID] = receiver
	}

	ch := make(chan []byte, 10)
	defer close(ch)

	go func() {
		for data := range ch {
			allTracks := c.dem.DemuxAll(data)
			for _, trackData := range allTracks {
				trackID := trackData.TrackID
				packets := trackData.Packets
				receiver := receivers[trackID]
				if receiver == nil {
					continue
				}

				for _, packet := range packets {
					receiver.WriteRTP(packet)
				}
			}
		}
	}()

	go func() {
		for msg := range c.msgs {
			if len(msg.Data) >= 4 {
				segmentNum := int(msg.Data[1])
				fragmentCount := int(msg.Data[2])
				fragmentSeq := int(msg.Data[3])
				mp4Data := msg.Data[4:]

				c.queue.AddFragment(segmentNum, fragmentCount, fragmentSeq, mp4Data)

				if c.queue.IsSegmentComplete(segmentNum, fragmentCount) {
					b := c.queue.GetCombinedBuffer(segmentNum)
					c.Recv += len(b)
					ch <- b
				}
			}
		}
	}()

	c.closed.Wait()
	return nil
}

func (c *DCConn) sendMessageToDataChannel(message string) error {
	if c.dc != nil {
		return c.dc.SendText(message)
	}

	return nil
}

func (c *DCConn) probe(msg pion.DataChannelMessage) (err error) {
	if msg.IsString {
		var message DataChannelMessage
		if err = json.Unmarshal(msg.Data, &message); err != nil {
			return err
		}

		switch message.Type {
		case "codec":
			response, _ := json.Marshal(DataChannelMessage{
				Type: "start",
				Msg:  "fmp4",
			})

			err = c.sendMessageToDataChannel(string(response))
			if err != nil {
				return err
			}

		case "recv":
			response, _ := json.Marshal(DataChannelMessage{
				Type: "complete",
				Msg:  "",
			})

			err = c.sendMessageToDataChannel(string(response))
			if err != nil {
				return err
			}
		}

	} else {
		if len(msg.Data) >= 4 {
			messageType := msg.Data[0]
			segmentNum := int(msg.Data[1])
			fragmentCount := int(msg.Data[2])
			fragmentSeq := int(msg.Data[3])
			mp4Data := msg.Data[4:]

			// initialization segment
			if messageType == 0 && segmentNum == 1 && fragmentCount == 1 && fragmentSeq == 1 {
				medias := c.dem.Probe(mp4Data)
				c.Medias = append(c.Medias, medias...)

				// Add backchannel
				webrtcMedias := c.client.GetMedias()
				for _, media := range webrtcMedias {
					if media.Kind == core.KindAudio {
						if media.Direction == core.DirectionSendRecv || media.Direction == core.DirectionSendonly {
							c.Medias = append(c.Medias, media)
						}
					}
				}

				c.initialized = true
			}
		}
	}

	return nil
}

func (c *DCConn) Stop() error {
	if c.dc != nil && c.dc.ReadyState() == pion.DataChannelStateOpen {
		_ = c.dc.Close()
	}

	c.closed.Done(nil)
	return nil
}
