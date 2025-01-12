package main

import (
	"log"
	"net"
	"net/url"
	"os"

	"github.com/AlexxIT/go2rtc/pkg/onvif"
)

func main() {
	var rawURL = os.Args[1]
	var operation = os.Args[2]
	var token string
	if len(os.Args) > 3 {
		token = os.Args[3]
	}

	client, err := onvif.NewClient(rawURL)
	if err != nil {
		log.Panic(err)
	}

	var b []byte

	switch operation {
	case onvif.ServiceGetServiceCapabilities:
		b, err = client.MediaRequest(operation)
	case onvif.DeviceGetCapabilities,
		onvif.DeviceGetDeviceInformation,
		onvif.DeviceGetDiscoveryMode,
		onvif.DeviceGetDNS,
		onvif.DeviceGetHostname,
		onvif.DeviceGetNetworkDefaultGateway,
		onvif.DeviceGetNetworkInterfaces,
		onvif.DeviceGetNetworkProtocols,
		onvif.DeviceGetNTP,
		onvif.DeviceGetScopes,
		onvif.DeviceGetServices,
		onvif.DeviceGetSystemDateAndTime,
		onvif.DeviceSystemReboot:
		b, err = client.DeviceRequest(operation)
	case onvif.MediaGetProfiles, onvif.MediaGetVideoSources:
		b, err = client.MediaRequest(operation)
	case onvif.MediaGetProfile:
		b, err = client.GetProfile(token)
	case onvif.MediaGetVideoSourceConfiguration:
		b, err = client.GetVideoSourceConfiguration(token)
	case onvif.MediaGetStreamUri:
		b, err = client.GetStreamUri(token)
	case onvif.MediaGetSnapshotUri:
		b, err = client.GetSnapshotUri(token)
	default:
		log.Printf("unknown action\n")
	}

	if err != nil {
		log.Printf("%s\n", err)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		log.Fatal(err)
	}

	host, _, _ := net.SplitHostPort(u.Host)

	if err = os.WriteFile(host+"_"+operation+".xml", b, 0644); err != nil {
		log.Printf("%s\n", err)
	}
}
