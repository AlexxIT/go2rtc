package streams

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
)

type Consumer struct {
	element streamer.Consumer
	tracks  []*streamer.Track
}

func (c *Consumer) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.element)
}
