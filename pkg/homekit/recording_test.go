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
