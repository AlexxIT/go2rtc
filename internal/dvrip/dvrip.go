package dvrip

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/dvrip"
	"github.com/rs/zerolog/log"
)

func Init() {
	streams.HandleFunc("dvrip", handle)
	// DVRIP client autodiscovery
	api.HandleFunc("api/dvrip", apiDvrip)
}

func handle(url string) (core.Producer, error) {
	conn := dvrip.NewClient(url)
	if err := conn.Dial(); err != nil {
		return nil, err
	}
	if err := conn.Play(); err != nil {
		return nil, err
	}
	if err := conn.Handle(); err != nil {
		return nil, err
	}
	return conn, nil
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

const (
	Port    = 34569           // UDP port number for dvrip discovery
	Timeout = 1 * time.Second // Timeout for receiving responses
)

func discover() ([]api.Stream, error) {
	log.Info().Msgf("[dvrip] discovering.")
	address := net.UDPAddr{
		Port: Port,
		IP:   net.IP{239, 255, 255, 250},
	}

	connection, err := net.ListenUDP("udp", &address)
	if err != nil {
		return nil, err
	}
	defer connection.Close()

	responseChan := make(chan []byte)

	// Wait group to synchronize goroutines
	var wg sync.WaitGroup
	wg.Add(1)
	go receiveResponses(connection, responseChan, &wg)
	wg.Add(1)
	go sendBroadcasts(connection, &wg)
	var items []api.Stream

	// Process received responses
	for response := range responseChan {
		n := len(response)
		if n < 20+1 {
			log.Debug().Msg("[dvrip] No valid JSON data found in the message")
			continue
		}

		jsonData := response[20 : n-1]
		if len(jsonData) == 0 {
			log.Err(err).Msgf("[dvrip] No valid JSON data found in the message")
			continue
		}
		var msg Message
		err = json.Unmarshal(jsonData, &msg)
		if err != nil {
			log.Err(err).Msgf("[dvrip] Error parsing JSON: %s", err)
			continue
		}

		if msg.NetCommon.HostIP != "" && msg.NetCommon.HostName != "" {
			hostIP := hexToDecimalBytes(msg.NetCommon.HostIP)

			u := &url.URL{
				Scheme: "dvrip",
				Host:   hostIP,
				Path:   "",
				User:   url.UserPassword("admin", "pass"),
			}
			queryParams := url.Values{}
			queryParams.Add("channel", "0")
			queryParams.Add("subtype", "0")
			u.RawQuery = queryParams.Encode()

			// Check if the URL already exists in the array
			exists := false
			for _, otherUrl := range items {
				if otherUrl.URL == u.String() {
					exists = true
					break
				}
			}
			if !exists {
				items = append(items, api.Stream{Name: msg.NetCommon.HostName, URL: u.String()})
			}
		}
	}

	wg.Wait()
	return items, nil
}

func receiveResponses(conn *net.UDPConn, responseChan chan<- []byte, wg *sync.WaitGroup) {
	defer wg.Done()

	buffer := make([]byte, 1024)

	for {
		conn.SetReadDeadline(time.Now().Add(Timeout))
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(*net.OpError); ok && netErr.Timeout() {
				close(responseChan)
				return
			}

			log.Info().Msgf("Error while receiving response:", err)
			continue
		}

		// Copy received response to a new slice to avoid data race
		responseCopy := make([]byte, n)
		copy(responseCopy, buffer[:n])

		responseChan <- responseCopy
	}
}

func sendBroadcasts(conn *net.UDPConn, wg *sync.WaitGroup) {
	defer wg.Done()

	// broadcasting the same multiple times because the devies some times don't answer
	hexStreams := []string{
		"ff00000000000000000000000000fa0500000000",
		"ff00000000000000000000000000fa0500000000",
		"ff00000000000000000000000000fa0500000000",
	}

	for _, hexStream := range hexStreams {
		data, err := hex.DecodeString(hexStream)
		if err != nil {
			log.Err(err).Msgf("[dvrip] Failed to decode hex stream:", err)
			continue
		}
		address := net.UDPAddr{
			Port: Port,
			IP:   net.IP{255, 255, 255, 255},
		}

		_, err = conn.WriteToUDP(data, &address)
		if err != nil {
			log.Err(err).Msgf("[dvrip] Error while sending broadcast:", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func reverseArray(arr []byte) {
	for i, j := 0, len(arr)-1; i < j; i, j = i+1, j-1 {
		arr[i], arr[j] = arr[j], arr[i]
	}
}

func hexToDecimalBytes(hexIP string) string {
	// Remove the '0x' prefix if present
	hexIP = hexIP[2:]

	// Decode the hexadecimal string to a byte slice
	decimalBytes, err := hex.DecodeString(hexIP)
	if err != nil {
		return "0.0.0.0"
	}
	reverseArray(decimalBytes)
	return fmt.Sprintf("%d.%d.%d.%d", decimalBytes[0], decimalBytes[1], decimalBytes[2], decimalBytes[3])
}

func apiDvrip(w http.ResponseWriter, r *http.Request) {
	items, err := discover()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	api.ResponseStreams(w, items)
}
