// Example CLI application that exports an RTSP camera stream as a HomeKit
// Secure Video (HKSV) camera using the pkg/hksv library.
//
// Author: Sergei "svk" Krashevich <svk@svk.su>
//
// Usage:
//
//	go run ./pkg/hksv/example -url rtsp://camera:554/stream
//	go run ./pkg/hksv/example -url rtsp://admin:pass@192.168.1.100:554/h264
//
// Then open the Home app on your iPhone/iPad, tap "+" → "Add Accessory",
// and scan the QR code or enter the PIN manually (default: 270-41-991).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hksv"
	"github.com/AlexxIT/go2rtc/pkg/mdns"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/rs/zerolog"
)

func main() {
	streamURL := flag.String("url", "", "RTSP stream URL (required)")
	pin := flag.String("pin", "27041991", "HomeKit pairing PIN")
	port := flag.Int("port", 0, "HAP HTTP port (0 = auto)")
	motion := flag.String("motion", "detect", "Motion mode: detect, continuous, api")
	threshold := flag.Float64("threshold", 2.0, "Motion detection threshold (lower = more sensitive)")
	pairFile := flag.String("pairings", "pairings.json", "Pairings persistence file")
	flag.Parse()

	if *streamURL == "" {
		fmt.Fprintln(os.Stderr, "Usage: hksv-camera -url rtsp://camera/stream")
		flag.PrintDefaults()
		os.Exit(1)
	}

	log := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()

	// 1. Connect to RTSP source
	client := rtsp.NewClient(*streamURL)
	if err := client.Dial(); err != nil {
		log.Fatal().Err(err).Msg("RTSP dial failed")
	}
	if err := client.Describe(); err != nil {
		log.Fatal().Err(err).Msg("RTSP describe failed")
	}

	log.Info().Str("url", *streamURL).Int("tracks", len(client.Medias)).Msg("RTSP connected")

	// Pre-setup all recvonly tracks so consumers can share receivers
	for _, media := range client.Medias {
		if media.Direction == core.DirectionRecvonly && len(media.Codecs) > 0 {
			if _, err := client.GetTrack(media, media.Codecs[0]); err != nil {
				log.Warn().Err(err).Str("media", media.String()).Msg("track setup failed")
			} else {
				log.Info().Str("media", media.String()).Msg("track ready")
			}
		}
	}

	// 2. Listen for HAP connections
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal().Err(err).Msg("listen failed")
	}
	actualPort := uint16(ln.Addr().(*net.TCPAddr).Port)

	// 3. Load saved pairings
	store := &filePairingStore{path: *pairFile}
	pairings := store.Load()

	// 4. Create HKSV server
	srv, err := hksv.NewServer(hksv.Config{
		StreamName:      "camera",
		Pin:             *pin,
		HKSV:            true,
		MotionMode:      *motion,
		MotionThreshold: *threshold,
		Streams:         &streamProvider{client: client, log: log},
		Store:           store,
		Pairings:        pairings,
		Logger:          log,
		Port:            actualPort,
		UserAgent:       "hksv-example",
		Version:         "1.0.0",
	})
	if err != nil {
		log.Fatal().Err(err).Msg("server create failed")
	}

	// 5. Start mDNS advertisement
	go func() {
		if err := mdns.Serve(mdns.ServiceHAP, []*mdns.ServiceEntry{srv.MDNSEntry()}); err != nil {
			log.Error().Err(err).Msg("mDNS failed")
		}
	}()

	// 6. Start RTSP streaming (after everything is set up)
	go func() {
		if err := client.Start(); err != nil {
			log.Error().Err(err).Msg("RTSP stream ended")
		}
	}()

	// 7. Start HTTP server for HAP protocol
	mux := http.NewServeMux()
	mux.HandleFunc(hap.PathPairSetup, srv.Handle)
	mux.HandleFunc(hap.PathPairVerify, srv.Handle)
	go func() {
		if err := http.Serve(ln, mux); err != nil {
			log.Fatal().Err(err).Msg("HTTP server failed")
		}
	}()

	// Print server info
	info, _ := json.MarshalIndent(srv, "", "  ")
	fmt.Fprintf(os.Stderr, "\nHomeKit camera ready on port %d\n%s\n\n", actualPort, info)

	// Wait for shutdown signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Info().Msg("shutting down")
	_ = client.Stop()
}

// streamProvider connects HKSV consumers to the RTSP producer.
// It implements hksv.StreamProvider.
type streamProvider struct {
	client *rtsp.Conn
	log    zerolog.Logger
	mu     sync.Mutex
}

func (p *streamProvider) AddConsumer(_ string, cons core.Consumer) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var matched int

	for _, consMedia := range cons.GetMedias() {
		if consMedia.Direction != core.DirectionSendonly {
			continue
		}
		for _, prodMedia := range p.client.Medias {
			prodCodec, consCodec := prodMedia.MatchMedia(consMedia)
			if prodCodec == nil {
				continue
			}

			track, err := p.client.GetTrack(prodMedia, prodCodec)
			if err != nil {
				p.log.Warn().Err(err).Str("codec", prodCodec.Name).Msg("get track failed")
				continue
			}

			if err := cons.AddTrack(consMedia, consCodec, track); err != nil {
				p.log.Warn().Err(err).Str("codec", consCodec.Name).Msg("add track failed")
				continue
			}

			matched++
			break
		}
	}

	if matched == 0 {
		return fmt.Errorf("no matching codecs between RTSP stream and consumer")
	}

	return nil
}

func (p *streamProvider) RemoveConsumer(_ string, _ core.Consumer) {}

// filePairingStore persists HomeKit pairings to a JSON file.
type filePairingStore struct {
	path string
}

func (s *filePairingStore) Load() []string {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil
	}
	var pairings []string
	_ = json.Unmarshal(data, &pairings)
	return pairings
}

func (s *filePairingStore) SavePairings(_ string, pairings []string) error {
	data, err := json.Marshal(pairings)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}
