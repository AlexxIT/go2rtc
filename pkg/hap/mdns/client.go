package mdns

import (
	"fmt"
	"github.com/hashicorp/mdns"
	"strings"
)

const Suffix = "._hap._tcp.local."

func GetAll() chan *mdns.ServiceEntry {
	entries := make(chan *mdns.ServiceEntry)
	params := &mdns.QueryParam{
		Service: "_hap._tcp", Entries: entries, DisableIPv6: true,
	}

	go func() {
		_ = mdns.Query(params)
		close(entries)
	}()

	return entries
}

func GetAddress(deviceID string) string {
	for entry := range GetAll() {
		if strings.Contains(entry.Info, deviceID) {
			return fmt.Sprintf("%s:%d", entry.AddrV4.String(), entry.Port)
		}
	}

	return ""
}

func GetEntry(deviceID string) *mdns.ServiceEntry {
	for entry := range GetAll() {
		if strings.Contains(entry.Info, deviceID) {
			return entry
		}
	}
	return nil
}
