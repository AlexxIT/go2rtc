package wsdiscovery

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"net"
	"net/netip"
	"os"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

const (
	multicastAddr = "239.255.255.250:3702" // Standard WS-Discovery multicast address and port
	metadataVer   = "1"                    // Metadata version

	maxConcurrentProbes = 32
)

var (
	serviceUUID = uuid.Must(uuid.NewRandom()).URN() // Unique service identifier, generated on each startup
	log         zerolog.Logger

	servicePort string
)

func Init() {
	log = app.GetLogger("ws-discovery")

	if api.Port > 0 {
		servicePort = strconv.Itoa(api.Port)

		buf := make([]byte, 16)
		hostname, _ := os.Hostname()
		if len(hostname) > 16 {
			hostname = hostname[:16]
		}
		copy(buf, []byte(hostname))
		binary.LittleEndian.PutUint16(buf[14:], uint16(api.Port))

		var u = uuid.Must(uuid.NewV7FromReader(bytes.NewReader(buf)))
		serviceUUID = u.URN()

		go HandleProbe()
	} else {
		log.Info().Msg("WS-Discovery disabled")
	}
}

func HandleProbe() {
	log = app.GetLogger("ws-discovery")

	// 1. Resolve the UDP multicast address.
	addr, err := net.ResolveUDPAddr("udp", multicastAddr)
	if err != nil {
		log.Error().Err(err).Msg("failed to resolve multicast address")
		return
	}

	// 2. Start listening for multicast messages.
	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		log.Error().Err(err).Msg("failed to listen on multicast address")
		return
	}
	defer conn.Close()

	err = conn.SetReadBuffer(2 << 20)
	if err != nil {
		log.Error().Err(err).Msg("failed to set read buffer")
	}

	log.Info().Str("addr", multicastAddr).Msg("started listening for multicast probes")

	// 3. Process incoming messages.
	pool := &sync.Pool{
		New: func() any {
			return make([]byte, 2048)
		},
	}
	workers := make(chan struct{}, maxConcurrentProbes)
	for {
		buffer := pool.Get().([]byte)
		n, remoteAddr, err := conn.ReadFromUDPAddrPort(buffer)
		if err != nil {
			log.Error().Err(err).Msg("failed to read packet")
			continue
		}

		// Handle each message concurrently.
		select {
		case workers <- struct{}{}:
			go func() {
				defer func() {
					<-workers
					if r := recover(); r != nil {
						log.Error().
							Interface("panic", r).
							Bytes("stack", debug.Stack()).
							Msg("panic while handling Probe request")
					}
				}()

				handleProbe(conn, remoteAddr, buffer, n, pool)
			}()
		default:
			pool.Put(buffer)
			log.Warn().Str("remoteAddr", remoteAddr.String()).Msg("too many pending Probe requests, dropping packet")
		}
	}
}

// handleProbe handles an incoming Probe request.
func handleProbe(conn *net.UDPConn, remoteAddr netip.AddrPort, buffer []byte, n int, pool *sync.Pool) {
	defer pool.Put(buffer)

	data := buffer[:n]

	// 1. Parse XML and extract the Probe message ID.
	messageID, probeTypes, ok := extractMessageIDAndTypes(data)
	if !ok {
		// Ignore invalid Probe messages.
		return
	}

	log.Info().
		Str("messageID", messageID).
		Str("remoteAddr", remoteAddr.String()).
		Str("probe", probeTypes).Msg("received Probe request")

	switch {
	case strings.HasSuffix(probeTypes, ":Device"):
		fallthrough
	case strings.EqualFold(probeTypes, "Device"):
		probeTypes = "Device"
	case strings.HasSuffix(probeTypes, ":NetworkVideoTransmitter"):
		fallthrough
	case strings.EqualFold(probeTypes, "NetworkVideoTransmitter"):
		probeTypes = "NetworkVideoTransmitter"
	default:
		log.Warn().
			Str("messageID", messageID).
			Str("remoteAddr", remoteAddr.String()).
			Str("probe", probeTypes).
			Msg("unsupported Probe type, ignoring request")
		return
	}

	localIP, err := getOutboundIP(remoteAddr)
	if err != nil {
		log.Err(err).
			Msg("failed to resolve outbound IP")
		return
	}

	log.Info().Str("addr", localIP.String()).Msg("Local Addr")

	// 2. Build the ProbeMatch response.
	responseXML := buildProbeMatchResponse(messageID, probeTypes, localIP.String())

	// 3. Unicast the response back to the client.
	_, err = conn.WriteToUDPAddrPort([]byte(responseXML), remoteAddr)
	if err != nil {
		log.Error().Err(err).Msg("failed to send ProbeMatch")
	} else {
		log.Info().Str("messageID", messageID).Str("remoteAddr", remoteAddr.String()).Msg("sent ProbeMatch response")
	}
}

type Envelope struct {
	Header struct {
		MessageID string `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing MessageID"`
	} `xml:"http://www.w3.org/2003/05/soap-envelope Header"`
	Body struct {
		Probe struct {
			Types  string `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Types"`
			Scopes string `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Scopes"`
		} `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Probe"`
	} `xml:"http://www.w3.org/2003/05/soap-envelope Body"`
}

var (
	messageIDRegex = regexp.MustCompile(`<[^\s>]*MessageID[^<>]*>(.*)</[^\s>]*MessageID>`)
)

// extractMessageIDAndTypes extracts the MessageID and Types from a Probe request.
func extractMessageIDAndTypes(xmlData []byte) (msgID, probeTypes string, ok bool) {
	var envelope Envelope

	err := xml.Unmarshal(xmlData, &envelope)
	if err != nil {
		log.Err(err).Msg("xml.Unmarshal failed")
		matches := messageIDRegex.FindStringSubmatch(string(xmlData))
		if len(matches) > 1 {
			msgID = matches[1]
			if bytes.Contains(xmlData, []byte("NetworkVideoTransmitter")) {
				probeTypes = "NetworkVideoTransmitter"
			} else {
				probeTypes = "Device"
			}
			ok = true
			return
		}

		log.Error().Err(err).Bytes("xml", xmlData).Msg("failed to parse XML")
		return "", "", false
	}

	return envelope.Header.MessageID, envelope.Body.Probe.Types, true
}

// buildProbeMatchResponse builds a ProbeMatch response that follows the WS-Discovery specification.
func buildProbeMatchResponse(relatesToID, probeType, localIP string) string {
	if probeType == "Device" {
		return `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope 
	xmlns:s="http://www.w3.org/2003/05/soap-envelope"
	xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing"
	xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
	xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
  <s:Header>
	<wsa:MessageID>` + serviceUUID + `</wsa:MessageID>
	<wsa:RelatesTo>` + relatesToID + `</wsa:RelatesTo>
	<wsa:To>http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</wsa:To>
	<wsa:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/ProbeMatches</wsa:Action>
  </s:Header>
  <s:Body>
	<d:ProbeMatches>
		<d:ProbeMatch>
			<wsa:EndpointReference><wsa:Address>` + serviceUUID + `</wsa:Address></wsa:EndpointReference>
			<d:Types>tds:Device</d:Types>
			<d:Scopes>onvif://www.onvif.org/type/Device</d:Scopes>
			<d:XAddrs>http://` + net.JoinHostPort(localIP, servicePort) + `/onvif/device_service</d:XAddrs>
			<d:MetadataVersion>` + metadataVer + `</d:MetadataVersion>
		</d:ProbeMatch>
	</d:ProbeMatches>
  </s:Body>
</s:Envelope>`
	}

	return `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope
	xmlns:s="http://www.w3.org/2003/05/soap-envelope"
	xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing"
	xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
	xmlns:dn="http://www.onvif.org/ver10/network/wsdl">
	<s:Header>
		<wsa:MessageID>` + serviceUUID + `</wsa:MessageID>
		<wsa:RelatesTo>` + relatesToID + `</wsa:RelatesTo>
		<wsa:To>http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</wsa:To>
		<wsa:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/ProbeMatches</wsa:Action>
		<d:AppSequence InstanceId="1617190918" MessageNumber="1" />
	</s:Header>
	<s:Body>
		<d:ProbeMatches>
			<d:ProbeMatch>
				<wsa:EndpointReference><wsa:Address>` + serviceUUID + `</wsa:Address></wsa:EndpointReference>
				<d:Types>dn:NetworkVideoTransmitter</d:Types>
				<d:Scopes>onvif://www.onvif.org/type/NetworkVideoTransmitter</d:Scopes>
				<d:XAddrs>http://` + net.JoinHostPort(localIP, servicePort) + `/onvif/device_service</d:XAddrs>
				<d:MetadataVersion>` + metadataVer + `</d:MetadataVersion>
			</d:ProbeMatch>
		</d:ProbeMatches>
	</s:Body>
</s:Envelope>`
}

func getOutboundIP(remote netip.AddrPort) (net.IP, error) {
	// Create a temporary connection to determine the route.
	d := net.Dialer{LocalAddr: nil, Timeout: 5 * time.Second}
	conn, err := d.Dial("udp", remote.String())
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP, nil
}
