package main

import (
	"log"
	"os"

	"github.com/Pinggy-io/pinggy-go/pinggy"
)

func main() {
	tunType := os.Args[1]
	address := os.Args[2]

	log.SetFlags(log.Llongfile | log.LstdFlags)

	config := pinggy.Config{
		Type:              pinggy.TunnelType(tunType),
		TcpForwardingAddr: address,

		//SshOverSsl: true,
		//Stdout:     os.Stderr,
		//Stderr:     os.Stderr,
	}

	if tunType == "http" {
		hman := pinggy.CreateHeaderManipulationAndAuthConfig()
		//hman.SetReverseProxy(address)
		//hman.SetPassPreflight(true)
		//hman.SetNoReverseProxy()
		config.HeaderManipulationAndAuth = hman
	}

	pl, err := pinggy.ConnectWithConfig(config)
	if err != nil {
		log.Panicln(err)
	}
	log.Println("Addrs: ", pl.RemoteUrls())
	//err = pl.InitiateWebDebug("localhost:3424")
	//log.Println(err)
	pl.StartForwarding()
}
