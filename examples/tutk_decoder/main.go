package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/tutk"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: tutk_decoder wireshark.json decoded.txt")
		return
	}

	src, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer src.Close()

	dst, err := os.Create(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	defer dst.Close()

	var items []item
	if err = json.NewDecoder(src).Decode(&items); err != nil {
		log.Fatal(err)
	}

	var b []byte

	for _, v := range items {
		if v.Source.Layers.Data.DataData == "" {
			continue
		}

		s := strings.ReplaceAll(v.Source.Layers.Data.DataData, ":", "")
		b, err = hex.DecodeString(s)
		if err != nil {
			log.Fatal(err)
		}

		tutk.ReverseTransCodePartial(b, b)

		ts := v.Source.Layers.Frame.FrameTimeRelative

		_, _ = fmt.Fprintf(dst, "%8s: %s -> %s [%4d] %x\n",
			ts[:len(ts)-6],
			v.Source.Layers.Ip.IpSrc, v.Source.Layers.Ip.IpDst,
			len(b), b)
	}
}

type item struct {
	Source struct {
		Layers struct {
			Frame struct {
				FrameTimeRelative string `json:"frame.time_relative"`
				FrameNumber       string `json:"frame.number"`
			} `json:"frame"`
			Ip struct {
				IpSrc string `json:"ip.src"`
				IpDst string `json:"ip.dst"`
			} `json:"ip"`
			Udp struct {
				UdpSrcport string `json:"udp.srcport"`
				UdpDstport string `json:"udp.dstport"`
			} `json:"udp"`
			Data struct {
				DataData string `json:"data.data"`
				DataLen  string `json:"data.len"`
			} `json:"data"`
		} `json:"layers"`
	} `json:"_source"`
}
