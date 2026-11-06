package homekit

import (
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/mdns"
)

// serveHAP keeps the legacy API listener unless a separate address is configured.
// Bind before advertising so a failed listener never publishes an unusable port.
func serveHAP(address string, entries []*mdns.ServiceEntry) (*http.Server, error) {
	if address == "" {
		api.HandleFunc(hap.PathPairSetup, hapHandler)
		api.HandleFunc(hap.PathPairVerify, hapHandler)
		for _, entry := range entries {
			entry.Port = uint16(api.Port)
		}
		return nil, nil
	}

	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	bound := ln.Addr().(*net.TCPAddr)
	// Discovery currently publishes IPv4 A records only. Do not advertise a
	// listener that can only be reached at a specific IPv6 address.
	if !bound.IP.IsUnspecified() && bound.IP.To4() == nil {
		_ = ln.Close()
		return nil, errors.New("homekit: IPv6-only listen addresses are not supported by IPv4 mDNS discovery")
	}

	srv := newHAPHTTPServer(ln.Addr().String())
	port := uint16(bound.Port)
	for _, entry := range entries {
		entry.Port = port
		entry.IP = nil
		if !bound.IP.IsUnspecified() {
			entry.IP = bound.IP.To4()
		}
	}

	log.Info().Str("addr", srv.Addr).Msg("[homekit] listen")
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("[homekit] serve")
		}
	}()
	return srv, nil
}

func newHAPHTTPServer(address string) *http.Server {
	// Only HAP handshakes are reachable here. Administrative HomeKit endpoints
	// remain on the API mux, with its authentication and allowlist intact.
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+hap.PathPairSetup, hapHandler)
	mux.HandleFunc("POST "+hap.PathPairVerify, hapHandler)
	return &http.Server{
		Addr:              address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
}
