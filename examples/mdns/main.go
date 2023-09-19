package main

import (
	"log"
	"os"

	"github.com/AlexxIT/go2rtc/pkg/mdns"
)

func main() {
	var service = mdns.ServiceHAP

	if len(os.Args) >= 2 {
		service = os.Args[1]
	}

	onentry := func(entry *mdns.ServiceEntry) bool {
		log.Printf("name=%s, addr=%s, info=%s\n", entry.Name, entry.Addr(), entry.Info)
		return false
	}

	var err error

	if len(os.Args) >= 3 {
		host := os.Args[2]

		log.Printf("run discovery service=%s host=%s\n", service, host)

		err = mdns.QueryOrDiscovery(host, service, onentry)
	} else {
		log.Printf("run discovery service=%s\n", service)

		err = mdns.Discovery(service, onentry)
	}

	if err != nil {
		log.Println(err)
	}
}
