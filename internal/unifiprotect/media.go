package unifiprotect

import (
	"bytes"
	"io"
	"net"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/unifiprotect"
)

func (m *Manager) serveMedia(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go m.handleMedia(conn)
	}
}

func (m *Manager) handleMedia(conn net.Conn) {
	log.Debug().Str("remote", conn.RemoteAddr().String()).Msg("[unifi-protect] media connection")
	owned := true
	defer func() {
		if owned {
			_ = conn.Close()
		}
	}()

	_ = conn.SetReadDeadline(time.Now().Add(probeTimeout))
	var prefix bytes.Buffer
	limited := &io.LimitedReader{R: conn, N: m.mediaProbeLimit}
	rd := unifiprotect.NewReader(io.TeeReader(limited, &prefix))

	var token string
	for i := 0; i < 8; i++ {
		tag, err := rd.ReadTag()
		if err != nil {
			log.Debug().Err(err).Str("remote", conn.RemoteAddr().String()).Msg("[unifi-protect] media header")
			return
		}
		metadata, ok, err := unifiprotect.ParseMetadata(tag)
		if err != nil {
			log.Debug().Err(err).Str("remote", conn.RemoteAddr().String()).Msg("[unifi-protect] media metadata")
			return
		}
		if ok {
			token = metadata.StreamName
			break
		}
	}
	if token == "" {
		log.Debug().Str("remote", conn.RemoteAddr().String()).Msg("[unifi-protect] media stream name missing")
		return
	}

	m.mu.Lock()
	req := m.pending[token]
	m.mu.Unlock()
	if req == nil || req.cameraIP != remoteHost(conn.RemoteAddr().String()) {
		log.Debug().Str("remote", conn.RemoteAddr().String()).Str("stream_name", token).Msg("[unifi-protect] unrouted media")
		return
	}

	replay := &replayReadCloser{
		Reader: io.MultiReader(bytes.NewReader(append([]byte(nil), prefix.Bytes()...)), conn),
		Closer: conn,
	}
	if !req.offerCandidate(candidate{conn: conn, rd: replay}) {
		return
	}
	log.Debug().Str("camera", req.key.mac).Str("channel", req.key.channel).Msg("[unifi-protect] routed media")
	owned = false
}

type replayReadCloser struct {
	io.Reader
	io.Closer
}
