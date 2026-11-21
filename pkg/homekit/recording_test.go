package homekit

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	pion "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

func TestNTPRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	ntp := TimeToNTP(now)
	back := NTPToTime(ntp)
	// allow 1ms of fractional rounding
	require.WithinDuration(t, now, back, time.Millisecond)

	require.True(t, NTPToTime(0).IsZero())
	require.NotZero(t, TimeToNTP(time.Time{}))
}

func TestCredentialsCSRAndTLS(t *testing.T) {
	c := NewCredentials()
	require.True(t, c.NeedsUpdate())

	nonce := make([]byte, 32)
	_, err := rand.Read(nonce)
	require.NoError(t, err)

	csrDER, sig, err := c.HandleCSR(nonce)
	require.NoError(t, err)
	require.NotEmpty(t, csrDER)
	require.NotEmpty(t, sig)

	csr, err := x509.ParseCertificateRequest(csrDER)
	require.NoError(t, err)
	require.NoError(t, csr.CheckSignature())

	// issue a self-signed leaf matching the CSR public key
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "go2rtc-hksv"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	// re-use CSR public key by signing with a throwaway CA key is fine for storage
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	// For InstallClientCertificate we just need parseable DER; TLSConfig needs matching privKey
	// so issue leaf with the credentials private key by generating cert from CSR public key
	// using caKey as issuer (leaf pubkey = CSR pubkey = credentials privKey.Public)
	leafDER, err := x509.CreateCertificate(rand.Reader, template, template, csr.PublicKey, caKey)
	require.NoError(t, err)

	c.InstallClientCertificate(leafDER, nil)
	require.False(t, c.NeedsUpdate())

	// TLSConfig will fail Leaf parse against private key mismatch? Actually Certificate.Leaf
	// is set from clientCert; PrivateKey is credentials privKey which matches CSR public key
	// CreateCertificate used csr.PublicKey so leaf pubkey matches credentials privKey
	cfg, err := c.TLSConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Len(t, cfg.Certificates, 1)

	id := c.SetKey([]byte("0123456789abcdef"), 3)
	require.Equal(t, uint64(3), id)
	kid, key := c.CurrentKey()
	require.Equal(t, uint64(3), kid)
	require.Equal(t, []byte("0123456789abcdef"), key)

	c.SetPublishingPoint("https://example.com/ingest/", [][]byte{leafDER})
	require.Equal(t, "https://example.com/ingest/", c.PublishingPoint())
}

func TestRingBufferSlice(t *testing.T) {
	b := NewRingBuffer(5 * time.Second)
	codec := &core.Codec{Name: core.CodecH265, ClockRate: 90000}

	base := time.Now().Add(-4 * time.Second)
	for i := 0; i < 10; i++ {
		b.Push(Packet{
			Track:   0,
			Codec:   codec,
			Payload: []byte{0x40, byte(i)}, // IDR-like NAL type 19 would be better; Key flag set explicitly
			RTPTime: uint32(i * 3000),
			Wall:    base.Add(time.Duration(i) * 400 * time.Millisecond),
			Key:     i%3 == 0,
		})
	}
	require.Greater(t, b.Len(), 0)

	start := base.Add(1 * time.Second)
	end := base.Add(3 * time.Second)
	slice := b.Slice(start, end)
	require.NotEmpty(t, slice)
	// first video sample should be a keyframe at or before start when available
	require.True(t, slice[0].Key || !slice[0].Wall.After(start))

	v, a := b.Codecs()
	require.Equal(t, codec, v)
	require.Nil(t, a)
	require.False(t, b.OldestWall().IsZero())
}

func TestBuildClipClear(t *testing.T) {
	codec := &core.Codec{Name: core.CodecH265, ClockRate: 90000}
	// minimal AVCC-style payload (length-prefixed NAL)
	nal := []byte{0x40, 0x01, 0x0c, 0x01}
	payload := make([]byte, 4+len(nal))
	payload[0] = 0
	payload[1] = 0
	payload[2] = 0
	payload[3] = byte(len(nal))
	copy(payload[4:], nal)

	packets := []Packet{
		{Track: 0, Codec: codec, Payload: payload, RTPTime: 0, Wall: time.Now(), Key: true},
		{Track: 0, Codec: codec, Payload: payload, RTPTime: 3000, Wall: time.Now().Add(100 * time.Millisecond), Key: false},
	}
	clip, err := BuildClip(packets, nil)
	require.NoError(t, err)
	require.NotEmpty(t, clip.Init)
	require.NotEmpty(t, clip.Fragments)
	// ftyp box starts with size then 'ftyp'
	require.GreaterOrEqual(t, len(clip.Init), 8)
	require.Equal(t, "ftyp", string(clip.Init[4:8]))
}

func TestBuildClipEmpty(t *testing.T) {
	_, err := BuildClip(nil, nil)
	require.Error(t, err)
}

func TestCMAFClientPublishHTTP(t *testing.T) {
	var posts atomic.Int32
	var paths []string
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// empty probe posts are ok
		if len(body) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewCMAFClient(srv.URL+"/publish", nil)
	clip := &Clip{
		Init:      []byte("init-bytes"),
		Fragments: [][]byte{[]byte("frag1"), []byte("frag2")},
	}
	code, err := client.PublishClip(42, clip)
	require.NoError(t, err)
	require.Equal(t, CMAFErrNone, code)
	require.GreaterOrEqual(t, posts.Load(), int32(3)) // probe + init + 2 frags (probe optional)

	mu.Lock()
	defer mu.Unlock()
	joined := ""
	for _, p := range paths {
		joined += p + " "
	}
	require.Contains(t, joined, "video-42")
	require.Contains(t, joined, "init.mp4")
	require.Contains(t, joined, "seg-1.m4s")
}

func TestCMAFClientHTTPErrorMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewCMAFClient(srv.URL, nil)
	code, err := client.PublishClip(1, &Clip{Init: []byte("x"), Fragments: [][]byte{[]byte("y")}})
	require.Error(t, err)
	require.Equal(t, CMAFErrHTTPNotFound, code)
}

func TestEventQueue(t *testing.T) {
	q := NewEventQueue(3)
	s1 := q.Push(Event{Type: camera.BufferEventTypeMotion, Motion: true})
	s2 := q.Push(Event{Type: camera.BufferEventTypeMotion, Motion: false})
	s3 := q.Push(Event{Type: camera.BufferEventTypeCMAFSessionStart, Session: 9})
	s4 := q.Push(Event{Type: camera.BufferEventTypeCMAFError, Session: 9, CMAFErr: CMAFErrTimeout})
	require.Equal(t, uint64(1), s1)
	require.Equal(t, uint64(4), s4)
	require.Equal(t, uint64(4), q.Sequence())

	// max 3 retained
	all := q.Query(0, 10)
	require.Len(t, all, 3)
	require.Equal(t, s2, all[0].Sequence)

	q.Acknowledge(s3)
	left := q.Query(0, 10)
	require.Len(t, left, 1)
	require.Equal(t, s4, left[0].Sequence)
}

func TestRecordingManagerMotionAndEvents(t *testing.T) {
	m := NewRecordingManager()
	var lastSeq uint32
	m.OnEventSeq = func(seq uint32) { lastSeq = seq }

	m.ReportMotion(true)
	require.Equal(t, uint32(1), lastSeq)
	require.Equal(t, uint32(1), m.EventSequence())

	res := m.HandleEventCommand(&camera.BufferEventCommandRequest{
		Command:        camera.BufferEventQuery,
		SequenceNumber: 0,
		Limit:          10,
	})
	require.Len(t, res.Events, 1)
	require.Equal(t, byte(camera.BufferEventTypeMotion), res.Events[0].Type)
	require.True(t, res.Events[0].Motion.Active)

	m.HandleEventCommand(&camera.BufferEventCommandRequest{
		Command:        camera.BufferEventAcknowledge,
		SequenceNumber: 1,
	})
	res = m.HandleEventCommand(&camera.BufferEventCommandRequest{
		Command: camera.BufferEventQuery,
		Limit:   10,
	})
	require.Empty(t, res.Events)
}

func TestRecordingManagerUploadToHTTP(t *testing.T) {
	var gotInit atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			if len(body) > 0 {
				gotInit.Store(true)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewRecordingManager()
	m.Creds.SetPublishingPoint(srv.URL+"/cmaf/", nil)
	m.SetRecordingActive(true)
	m.SetAudioActive(false)

	codec := &core.Codec{Name: core.CodecH265, ClockRate: 90000}
	nal := []byte{0x40, 0x01, 0x0c, 0x01}
	payload := append([]byte{0, 0, 0, byte(len(nal))}, nal...)
	now := time.Now()
	for i := 0; i < 5; i++ {
		m.Buffer.Push(Packet{
			Track:   0,
			Codec:   codec,
			Payload: payload,
			RTPTime: uint32(i * 3000),
			Wall:    now.Add(time.Duration(i) * 200 * time.Millisecond),
			Key:     i == 0,
		})
	}

	var seqs []uint32
	var mu sync.Mutex
	m.OnEventSeq = func(seq uint32) {
		mu.Lock()
		seqs = append(seqs, seq)
		mu.Unlock()
	}

	res := m.HandleUpload(&camera.BufferUploadCommandRequest{
		SessionID: 7,
		Command:   camera.BufferUploadStartAndStop,
		Start:     TimeToNTP(now.Add(-time.Second)),
		Stop:      TimeToNTP(now.Add(2 * time.Second)),
	})
	require.NotZero(t, res.ClipID)

	// wait for async publish
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(seqs)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(seqs), 2) // start + stop (and maybe error)
	require.True(t, gotInit.Load())
}

func TestRecordingManagerActivity(t *testing.T) {
	m := NewRecordingManager()
	m.HandleActivity(&camera.BufferActivityCommandRequest{
		Activity: camera.BufferActivityShouldRecord,
	})
	require.Equal(t, uint32(1), m.EventSequence())
	m.HandleActivity(&camera.BufferActivityCommandRequest{
		Activity: camera.BufferActivityShouldNotRecord,
	})
	require.Equal(t, uint32(2), m.EventSequence())
}

func TestRecordingActiveFlags(t *testing.T) {
	m := NewRecordingManager()
	require.False(t, m.RecordingActive())
	require.False(t, m.AudioActive())
	m.SetRecordingActive(true)
	m.SetAudioActive(true)
	require.True(t, m.RecordingActive())
	require.True(t, m.AudioActive())
	_ = m.EnsureConsumer()
	require.NotNil(t, m.Consumer)
}

func TestMapHTTPError(t *testing.T) {
	require.Equal(t, CMAFErrNone, mapHTTPError(nil))
	require.Equal(t, CMAFErrHTTPBadRequest, mapHTTPError(&httpStatusError{code: 400}))
	require.Equal(t, CMAFErrHTTPInternalServer, mapHTTPError(&httpStatusError{code: 500}))
	require.Equal(t, CMAFErrCannotFindHost, mapHTTPError(errString("no such host")))
	require.Equal(t, CMAFErrTimeout, mapHTTPError(errString("i/o timeout")))
}

func TestRingBufferTrimAndEmpty(t *testing.T) {
	b := NewRingBuffer(200 * time.Millisecond)
	codec := &core.Codec{Name: core.CodecH264, ClockRate: 90000}
	old := time.Now().Add(-time.Second)
	b.Push(Packet{Track: 0, Codec: codec, Payload: []byte{1}, Wall: old, Key: true})
	// force trim by pushing fresh
	b.Push(Packet{Track: 0, Codec: codec, Payload: []byte{2}, Wall: time.Now(), Key: true})
	require.Equal(t, 1, b.Len())
	require.True(t, b.OldestWall().After(old))

	empty := NewRingBuffer(time.Second)
	require.Equal(t, 0, empty.Len())
	require.True(t, empty.OldestWall().IsZero())
	require.Empty(t, empty.Slice(time.Time{}, time.Time{}))
}

func TestRingBufferPayloadCopy(t *testing.T) {
	b := NewRingBuffer(time.Second)
	codec := &core.Codec{Name: core.CodecH265, ClockRate: 90000}
	payload := []byte{0x40, 0x01}
	b.Push(Packet{Track: 0, Codec: codec, Payload: payload, Wall: time.Now(), Key: true})
	payload[0] = 0xFF // mutate caller buffer
	slice := b.Slice(time.Now().Add(-time.Second), time.Now().Add(time.Second))
	require.Len(t, slice, 1)
	require.Equal(t, byte(0x40), slice[0].Payload[0])
}

func TestRingBufferConcurrentPush(t *testing.T) {
	b := NewRingBuffer(2 * time.Second)
	codec := &core.Codec{Name: core.CodecH265, ClockRate: 90000}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				b.Push(Packet{
					Track:   0,
					Codec:   codec,
					Payload: []byte{byte(g), byte(i)},
					Wall:    time.Now(),
					Key:     i%10 == 0,
				})
			}
		}(g)
	}
	wg.Wait()
	require.Greater(t, b.Len(), 0)
}

func TestBuildClipNoVideo(t *testing.T) {
	audio := &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}
	_, err := BuildClip([]Packet{{
		Track: 1, Codec: audio, Payload: []byte{1, 2, 3}, RTPTime: 0, Wall: time.Now(),
	}}, nil)
	require.Error(t, err)
}

func TestBuildClipWithAudioAndKey(t *testing.T) {
	vcodec := &core.Codec{Name: core.CodecH265, ClockRate: 90000}
	acodec := &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}
	nal := []byte{0x40, 0x01, 0x0c, 0x01}
	vpay := append([]byte{0, 0, 0, byte(len(nal))}, nal...)
	packets := []Packet{
		{Track: 0, Codec: vcodec, Payload: vpay, RTPTime: 0, Wall: time.Now(), Key: true},
		{Track: 1, Codec: acodec, Payload: []byte{0x01, 0x02}, RTPTime: 0, Wall: time.Now()},
		{Track: 0, Codec: vcodec, Payload: vpay, RTPTime: 3000, Wall: time.Now().Add(40 * time.Millisecond)},
	}
	key := []byte("0123456789abcdef")
	clip, err := BuildClip(packets, key)
	require.NoError(t, err)
	require.NotEmpty(t, clip.Init)
	require.Len(t, clip.Fragments, 3)
	// encrypted payload should differ from clear
	clear, err := BuildClip(packets, nil)
	require.NoError(t, err)
	require.NotEqual(t, clear.Fragments[0], clip.Fragments[0])
}

func TestCredentialsNeedsCertForHTTPS(t *testing.T) {
	c := NewCredentials()
	_, err := c.TLSConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), "certificate")

	// CSR twice reuses key
	n1 := make([]byte, 32)
	n2 := make([]byte, 32)
	_, _ = rand.Read(n1)
	_, _ = rand.Read(n2)
	csr1, _, err := c.HandleCSR(n1)
	require.NoError(t, err)
	csr2, _, err := c.HandleCSR(n2)
	require.NoError(t, err)
	// both valid CSRs
	_, err = x509.ParseCertificateRequest(csr1)
	require.NoError(t, err)
	_, err = x509.ParseCertificateRequest(csr2)
	require.NoError(t, err)
}

func TestCMAFClientInvalidClip(t *testing.T) {
	c := NewCMAFClient("http://127.0.0.1:9/", nil)
	code, err := c.PublishClip(1, nil)
	require.Error(t, err)
	require.Equal(t, CMAFErrMP4Error, code)

	code, err = c.PublishClip(1, &Clip{})
	require.Error(t, err)
	require.Equal(t, CMAFErrMP4Error, code)

	c2 := NewCMAFClient("", nil)
	code, err = c2.PublishClip(1, &Clip{Init: []byte("x")})
	require.Error(t, err)
	require.Equal(t, CMAFErrInvalidState, code)
}

func TestCMAFClientConnectionRefused(t *testing.T) {
	// nothing listening
	c := NewCMAFClient("http://127.0.0.1:1/", nil)
	code, err := c.PublishClip(1, &Clip{Init: []byte("init"), Fragments: [][]byte{[]byte("f")}})
	require.Error(t, err)
	require.NotEqual(t, CMAFErrNone, code)
}

func TestRecordingManagerMissingPublishingPoint(t *testing.T) {
	m := NewRecordingManager()
	codec := &core.Codec{Name: core.CodecH265, ClockRate: 90000}
	nal := []byte{0x40, 0x01}
	payload := append([]byte{0, 0, 0, byte(len(nal))}, nal...)
	m.Buffer.Push(Packet{Track: 0, Codec: codec, Payload: payload, Wall: time.Now(), Key: true})

	var types []byte
	var mu sync.Mutex
	m.OnEventSeq = func(seq uint32) {
		mu.Lock()
		defer mu.Unlock()
		// pull latest events
		for _, ev := range m.Events.Query(0, 32) {
			_ = seq
			types = append(types, ev.Type)
		}
	}

	res := m.HandleUpload(&camera.BufferUploadCommandRequest{
		SessionID: 1,
		Command:   camera.BufferUploadStartAndStop,
	})
	require.Equal(t, uint64(1), res.ClipID)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		hasErr := false
		for _, typ := range types {
			if typ == camera.BufferEventTypeCMAFError {
				hasErr = true
			}
		}
		mu.Unlock()
		if hasErr {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	evs := m.Events.Query(0, 32)
	var sawStart, sawErr, sawStop bool
	for _, ev := range evs {
		switch ev.Type {
		case camera.BufferEventTypeCMAFSessionStart:
			sawStart = true
		case camera.BufferEventTypeCMAFError:
			sawErr = true
			require.NotZero(t, ev.CMAFErr)
		case camera.BufferEventTypeCMAFSessionStop:
			sawStop = true
		}
	}
	require.True(t, sawStart)
	require.True(t, sawErr)
	require.True(t, sawStop)
}

func TestRecordingManagerEmptyBufferError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewRecordingManager()
	m.Creds.SetPublishingPoint(srv.URL+"/", nil)
	// no packets in buffer
	res := m.HandleUpload(&camera.BufferUploadCommandRequest{
		SessionID: 3,
		Command:   camera.BufferUploadStartAndStop,
	})
	require.NotZero(t, res.ClipID)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		evs := m.Events.Query(0, 32)
		for _, ev := range evs {
			if ev.Type == camera.BufferEventTypeCMAFError {
				require.Equal(t, byte(CMAFErrMP4Error), ev.CMAFErr)
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected CMAF error for empty buffer")
}

func TestRecordingManagerStopFinalize(t *testing.T) {
	// slow ingest so we can stop mid-flight
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-started:
		default:
			close(started)
		}
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewRecordingManager()
	m.Creds.SetPublishingPoint(srv.URL+"/", nil)
	codec := &core.Codec{Name: core.CodecH265, ClockRate: 90000}
	nal := []byte{0x40, 0x01, 0x0c}
	payload := append([]byte{0, 0, 0, byte(len(nal))}, nal...)
	now := time.Now()
	for i := 0; i < 3; i++ {
		m.Buffer.Push(Packet{
			Track: 0, Codec: codec, Payload: payload,
			RTPTime: uint32(i * 3000), Wall: now.Add(time.Duration(i) * 100 * time.Millisecond), Key: i == 0,
		})
	}

	start := m.HandleUpload(&camera.BufferUploadCommandRequest{
		SessionID: 99,
		Command:   camera.BufferUploadStart,
		Start:     TimeToNTP(now.Add(-time.Second)),
		Stop:      TimeToNTP(now.Add(time.Second)),
	})
	require.Equal(t, uint64(1), start.ClipID)

	<-started
	stop := m.HandleUpload(&camera.BufferUploadCommandRequest{
		SessionID:  99,
		Command:    camera.BufferUploadStop,
		StopAction: camera.BufferStopActionFinalize,
	})
	require.Equal(t, uint64(1), stop.ClipID)

	// wait for lifecycle
	deadline := time.Now().Add(2 * time.Second)
	var sawStart, sawStop bool
	for time.Now().Before(deadline) {
		sawStart, sawStop = false, false
		for _, ev := range m.Events.Query(0, 32) {
			if ev.Type == camera.BufferEventTypeCMAFSessionStart && ev.Session == 99 {
				sawStart = true
			}
			if ev.Type == camera.BufferEventTypeCMAFSessionStop && ev.Session == 99 {
				sawStop = true
			}
		}
		if sawStart && sawStop {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("lifecycle incomplete start=%v stop=%v events=%+v", sawStart, sawStop, m.Events.Query(0, 32))
}

func TestRecordingManagerStopUnknownSession(t *testing.T) {
	m := NewRecordingManager()
	res := m.HandleUpload(&camera.BufferUploadCommandRequest{
		SessionID:  404,
		Command:    camera.BufferUploadStop,
		StopAction: camera.BufferStopActionFinalize,
	})
	require.Equal(t, uint64(0), res.ClipID)
}

func TestRecordingManagerEventTypesRoundTrip(t *testing.T) {
	m := NewRecordingManager()
	m.Events.Push(Event{Type: camera.BufferEventTypeCMAFSessionStart, Session: 5})
	m.Events.Push(Event{Type: camera.BufferEventTypeCMAFSessionStop, Session: 5})
	m.Events.Push(Event{Type: camera.BufferEventTypeMotion, Motion: true})
	m.Events.Push(Event{Type: camera.BufferEventTypeCMAFError, Session: 5, CMAFErr: CMAFErrTimeout})

	res := m.HandleEventCommand(&camera.BufferEventCommandRequest{
		Command: camera.BufferEventQuery,
		Limit:   10,
	})
	require.Len(t, res.Events, 4)
	require.Equal(t, uint64(5), res.Events[0].CMAFSessionStart.CMAFSessionID)
	require.Equal(t, uint64(5), res.Events[1].CMAFSessionStop.CMAFSessionID)
	require.True(t, res.Events[2].Motion.Active)
	require.Equal(t, byte(CMAFErrTimeout), res.Events[3].CMAFError.CMAFError)
}

func TestRecordingManagerAudioFilter(t *testing.T) {
	var paths []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		_, _ = io.ReadAll(r.Body)
		_ = r.Body.Close()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewRecordingManager()
	m.Creds.SetPublishingPoint(srv.URL+"/", nil)
	m.SetAudioActive(false)
	vcodec := &core.Codec{Name: core.CodecH265, ClockRate: 90000}
	acodec := &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}
	nal := []byte{0x40, 0x01}
	vpay := append([]byte{0, 0, 0, byte(len(nal))}, nal...)
	now := time.Now()
	m.Buffer.Push(Packet{Track: 0, Codec: vcodec, Payload: vpay, Wall: now, Key: true})
	m.Buffer.Push(Packet{Track: 1, Codec: acodec, Payload: []byte{9}, Wall: now})

	_ = m.HandleUpload(&camera.BufferUploadCommandRequest{
		SessionID: 11,
		Command:   camera.BufferUploadStartAndStop,
		Start:     TimeToNTP(now.Add(-time.Second)),
		Stop:      TimeToNTP(now.Add(time.Second)),
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.Events.Sequence() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// publish should have succeeded (video only)
	evs := m.Events.Query(0, 32)
	for _, ev := range evs {
		require.NotEqual(t, byte(camera.BufferEventTypeCMAFError), ev.Type)
	}
}

func TestMapPublishError(t *testing.T) {
	require.Equal(t, CMAFErrNone, mapPublishError(nil))
	require.Equal(t, CMAFErrHTTPNotFound, mapPublishError(&cmafPublishError{code: CMAFErrHTTPNotFound, err: errString("x")}))
	require.Equal(t, CMAFErrUnknown, mapPublishError(errString("something else")))
}

func TestEventQueueConcurrent(t *testing.T) {
	q := NewEventQueue(100)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				q.Push(Event{Type: camera.BufferEventTypeMotion, Motion: j%2 == 0})
			}
		}(i)
	}
	wg.Wait()
	require.Equal(t, uint64(400), q.Sequence())
	// max retained
	require.LessOrEqual(t, len(q.Query(0, 1000)), 100)
}

func TestWebRTCReofferUnknown(t *testing.T) {
	m := NewWebRTCManager(testFactory)
	res := m.Reoffer(&camera.WebRTCReofferRequest{SessionIdentifier: "nope", SDPOffer: "v=0\r\n"})
	require.Equal(t, byte(camera.WebRTCStatusUnknownSessionIdentifier), res.Status)
}

func TestWebRTCSolicitFactoryError(t *testing.T) {
	m := NewWebRTCManager(func() (*pion.PeerConnection, error) {
		return nil, errString("boom")
	})
	res, err := m.SolicitOffer(false)
	require.NoError(t, err)
	require.Equal(t, byte(camera.WebRTCSolicitError), res.Status)
}
