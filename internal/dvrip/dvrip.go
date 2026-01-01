package dvrip

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/dvrip"
)

func Init() {
	streams.HandleFunc("dvrip", dvrip.Dial)

	// DVRIP client autodiscovery
	api.HandleFunc("api/dvrip", apiDvrip)
}

const Port = 34569 // UDP port number for dvrip discovery

func apiDvrip(w http.ResponseWriter, r *http.Request) {
	items, err := discover()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	api.ResponseSources(w, items)
}

func discover() ([]*api.Source, error) {
	addr := &net.UDPAddr{
		Port: Port,
		IP:   net.IP{239, 255, 255, 250},
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}

	defer conn.Close()

	go sendBroadcasts(conn)

	var items []*api.Source

	for _, info := range getResponses(conn) {
		if info.HostIP == "" || info.HostName == "" {
			continue
		}

		host, err := hexToDecimalBytes(info.HostIP)
		if err != nil {
			continue
		}

		items = append(items, &api.Source{
			Name: info.HostName,
			URL:  "dvrip://user:pass@" + host + "?channel=0&subtype=0",
		})
	}

	return items, nil
}

func sendBroadcasts(conn *net.UDPConn) {
	// broadcasting the same multiple times because the devies some times don't answer
	data, err := hex.DecodeString("ff00000000000000000000000000fa0500000000")
	if err != nil {
		return
	}

	addr := &net.UDPAddr{
		Port: Port,
		IP:   net.IP{255, 255, 255, 255},
	}

	for i := 0; i < 3; i++ {
		time.Sleep(100 * time.Millisecond)
		_, _ = conn.WriteToUDP(data, addr)
	}
}

type Message struct {
	NetCommon NetCommon `json:"NetWork.NetCommon"`
	Ret       int       `json:"Ret"`
	SessionID string    `json:"SessionID"`
}

type NetCommon struct {
	BuildDate       string `json:"BuildDate"`
	ChannelNum      int    `json:"ChannelNum"`
	DeviceType      int    `json:"DeviceType"`
	GateWay         string `json:"GateWay"`
	HostIP          string `json:"HostIP"`
	HostName        string `json:"HostName"`
	HttpPort        int    `json:"HttpPort"`
	MAC             string `json:"MAC"`
	MonMode         string `json:"MonMode"`
	NetConnectState int    `json:"NetConnectState"`
	OtherFunction   string `json:"OtherFunction"`
	SN              string `json:"SN"`
	SSLPort         int    `json:"SSLPort"`
	Submask         string `json:"Submask"`
	TCPMaxConn      int    `json:"TCPMaxConn"`
	TCPPort         int    `json:"TCPPort"`
	UDPPort         int    `json:"UDPPort"`
	UseHSDownLoad   bool   `json:"UseHSDownLoad"`
	Version         string `json:"Version"`
}

func getResponses(conn *net.UDPConn) (infos []*NetCommon) {
	if err := conn.SetReadDeadline(time.Now().Add(time.Second * 2)); err != nil {
		return
	}

	var ips []net.IP // processed IPs

	b := make([]byte, 4096)
loop:
	for {
		n, addr, err := conn.ReadFromUDP(b)
		if err != nil {
			break
		}

		for _, ip := range ips {
			if ip.Equal(addr.IP) {
				continue loop
			}
		}

		if n <= 20+1 {
			continue
		}

		var msg Message

		if err = json.Unmarshal(b[20:n-1], &msg); err != nil {
			continue
		}

		infos = append(infos, &msg.NetCommon)
		ips = append(ips, addr.IP)
	}

	return
}

func hexToDecimalBytes(hexIP string) (string, error) {
	b, err := hex.DecodeString(hexIP[2:]) // remove the '0x' prefix
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%d.%d.%d", b[3], b[2], b[1], b[0]), nil
}
