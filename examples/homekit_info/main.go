package main

import (
	"encoding/json"
	"os"

	"github.com/AlexxIT/go2rtc/pkg/hap"
)

var servs = map[string]string{
	"3E":  "Accessory Information",
	"7E":  "Security System",
	"85":  "Motion Sensor",
	"96":  "Battery",
	"A2":  "Protocol Information",
	"110": "Camera RTP Stream Management",
	"112": "Microphone",
	"113": "Speaker",
	"121": "Doorbell",
	"129": "Data Stream Transport Management",
	"204": "Camera Recording Management",
	"21A": "Camera Operating Mode",
	"22A": "Wi-Fi Transport",
	"239": "Accessory Runtime Information",
}

var chars = map[string]string{
	"14":  "Identify",
	"20":  "Manufacturer",
	"21":  "Model",
	"23":  "Name",
	"30":  "Serial Number",
	"52":  "Firmware Revision",
	"53":  "Hardware Revision",
	"220": "Product Data",
	"A6":  "Accessory Flags",

	"22": "Motion Detected",
	"75": "Status Active",

	"11A": "Mute",
	"119": "Volume",

	"B0":  "Active",
	"209": "Selected Camera Recording Configuration",
	"207": "Supported Audio Recording Configuration",
	"205": "Supported Camera Recording Configuration",
	"206": "Supported Video Recording Configuration",
	"226": "Recording Audio Active",

	"223": "Event Snapshots Active",
	"225": "Periodic Snapshots Active",
	"21B": "HomeKit Camera Active",
	"21C": "Third Party Camera Active",
	"21D": "Camera Operating Mode Indicator",
	"11B": "Night Vision",
	"129": "Supported Data Stream Transport Configuration",
	"37":  "Version",
	"131": "Setup Data Stream Transport",
	"130": "Supported Data Stream Transport Configuration",

	"120": "Streaming Status",
	"115": "Supported Audio Stream Configuration",
	"116": "Supported RTP Configuration",
	"114": "Supported Video Stream Configuration",
	"117": "Selected RTP Stream Configuration",
	"118": "Setup Endpoints",

	"22B": "Current Transport",
	"22C": "Wi-Fi Capabilities",
	"22D": "Wi-Fi Configuration Control",

	"23C": "Ping",

	"68": "Battery Level",
	"79": "Status Low Battery",
	"8F": "Charging State",

	"73":  "Programmable Switch Event",
	"232": "Operating State Response",

	"66": "Security System Current State",
	"67": "Security System Target State",
}

func main() {
	src := os.Args[1]
	dst := os.Args[2]

	f, err := os.Open(src)
	if err != nil {
		panic(err)
	}

	var v hap.JSONAccessories
	if err = json.NewDecoder(f).Decode(&v); err != nil {
		panic(err)
	}

	for _, acc := range v.Value {
		for _, srv := range acc.Services {
			if srv.Desc == "" {
				srv.Desc = servs[srv.Type]
			}
			for _, chr := range srv.Characters {
				if chr.Desc == "" {
					chr.Desc = chars[chr.Type]
				}
			}
		}
	}

	f, err = os.Create(dst)
	if err != nil {
		panic(err)
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err = enc.Encode(v); err != nil {
		panic(err)
	}
}
