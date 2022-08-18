package main

import (
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/pion/rtp"
	"os"
	"time"
)

func main() {
	client, err := rtsp.NewClient(os.Args[1])
	if err != nil {
		panic(err)
	}

	if err = client.Dial(); err != nil {
		panic(err)
	}
	if err = client.Describe(); err != nil {
		panic(err)
	}

	for _, media := range client.GetMedias() {
		fmt.Printf("Media: %v\n", media)

		if media.AV() {
			track := client.GetTrack(media, media.Codecs[0])
			fmt.Printf("Track: %v, %v\n", track, track.Codec)

			track.Bind(func(packet *rtp.Packet) error {
				nalUnitType := packet.Payload[0] & 0x1F
				fmt.Printf(
					"[RTP] codec: %s, nalu: %2d, size: %6d, ts: %10d, pt: %2d, ssrc: %d\n",
					track.Codec.Name, nalUnitType, len(packet.Payload), packet.Timestamp,
					packet.PayloadType, packet.SSRC,
				)
				return nil
			})
		}
	}

	if err = client.Play(); err != nil {
		panic(err)
	}

	time.AfterFunc(time.Second*5, func() {
		if err = client.Close(); err != nil {
			panic(err)
		}
	})

	if err = client.Handle(); err != nil {
		panic(err)
	}

	fmt.Println("The End")
}
