package unifiprotect

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/flv"
	"github.com/AlexxIT/go2rtc/pkg/flv/amf"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/stretchr/testify/require"
)

func TestManagerSyntheticCamera(t *testing.T) {
	mediaListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer mediaListener.Close()

	m := newManager("", mediaListener.Addr().(*net.TCPAddr).Port, "test", "controller-id")
	go m.serveMedia(mediaListener)
	ws, server := connectCamera(t, m)
	defer server.Close()
	defer ws.Close()

	type openResult struct {
		producer core.Producer
		err      error
	}
	result := make(chan openResult, 1)
	go func() {
		producer, err := m.open("unifi-protect://02AABBCCDDEE?channel=video2&audio=0")
		result <- openResult{producer: producer, err: err}
	}()

	start := readController(t, ws)
	require.Equal(t, "ChangeVideoSettings", start.FunctionName)
	command := decodeVideoCommand(t, start, "video2")
	require.True(t, command.Parameters.SuppressAudio)
	require.False(t, command.Parameters.WithOpus)
	require.Len(t, command.Destinations, 1)
	require.NotEmpty(t, command.Parameters.StreamName)

	media, err := net.Dial("tcp", mediaListener.Addr().String())
	require.NoError(t, err)
	defer media.Close()
	_, err = media.Write(syntheticMedia(command.Parameters.StreamName))
	require.NoError(t, err)

	var first core.Producer
	select {
	case got := <-result:
		require.NoError(t, got.err)
		require.Len(t, got.producer.GetMedias(), 1)
		require.Equal(t, core.CodecH264, got.producer.GetMedias()[0].Codecs[0].Name)
		first = got.producer
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for producer")
	}

	result2 := make(chan openResult, 1)
	go func() {
		producer, err := m.open("unifi-protect://02AABBCCDDEE?channel=video3&audio=0")
		result2 <- openResult{producer: producer, err: err}
	}()
	start2 := readController(t, ws)
	command2 := decodeVideoCommand(t, start2, "video3")
	media2, err := net.Dial("tcp", mediaListener.Addr().String())
	require.NoError(t, err)
	defer media2.Close()
	_, err = media2.Write(syntheticMedia(command2.Parameters.StreamName))
	require.NoError(t, err)

	var second core.Producer
	select {
	case got := <-result2:
		require.NoError(t, got.err)
		second = got.producer
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for second producer")
	}

	require.NoError(t, first.Stop())
	stop := readController(t, ws)
	stopped := decodeVideoCommand(t, stop, "video2")
	require.Equal(t, []string{"file:///dev/null"}, stopped.Destinations)

	require.NoError(t, second.Stop())
	stop2 := readController(t, ws)
	stopped2 := decodeVideoCommand(t, stop2, "video3")
	require.Equal(t, []string{"file:///dev/null"}, stopped2.Destinations)
}

func TestLoadOrCreateCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, defaultTLSCert)
	keyPath := filepath.Join(dir, defaultTLSKey)

	cert, err := loadOrCreateCertificate(certPath, keyPath)
	require.NoError(t, err)
	require.NotEmpty(t, cert.Certificate)

	keyInfo, err := os.Stat(keyPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), keyInfo.Mode().Perm())

	reloaded, err := loadOrCreateCertificate(certPath, keyPath)
	require.NoError(t, err)
	require.Equal(t, cert.Certificate[0], reloaded.Certificate[0])
	require.Equal(t, certificateUUID(cert), certificateUUID(reloaded))

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)
	require.True(t, leaf.IsCA)
	require.Greater(t, time.Until(leaf.NotAfter), 9*365*24*time.Hour)
}

func TestLoadOrCreateCertificateRejectsIncompleteIdentity(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, defaultTLSCert)
	keyPath := filepath.Join(dir, defaultTLSKey)
	require.NoError(t, os.WriteFile(certPath, []byte("incomplete"), 0600))

	_, err := loadOrCreateCertificate(certPath, keyPath)
	require.ErrorContains(t, err, "incomplete TLS identity")
}

func TestManagerSyntheticCameraAACFallback(t *testing.T) {
	mediaListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer mediaListener.Close()

	m := newManager("", mediaListener.Addr().(*net.TCPAddr).Port, "test", "controller-id")
	go m.serveMedia(mediaListener)
	ws, server := connectCamera(t, m)
	defer server.Close()
	defer ws.Close()

	s, err := m.waitSession("02AABBCCDDEE", time.Now().Add(time.Second))
	require.NoError(t, err)
	s.opusRate = 0

	type openResult struct {
		producer core.Producer
		err      error
	}
	result := make(chan openResult, 1)
	go func() {
		producer, err := m.open("unifi-protect://02AABBCCDDEE?channel=video1")
		result <- openResult{producer: producer, err: err}
	}()

	start := readController(t, ws)
	command := decodeVideoCommand(t, start, "video1")
	require.False(t, command.Parameters.SuppressAudio)
	require.False(t, command.Parameters.WithOpus)

	media, err := net.Dial("tcp", mediaListener.Addr().String())
	require.NoError(t, err)
	defer media.Close()
	_, err = media.Write(syntheticMediaAAC(command.Parameters.StreamName))
	require.NoError(t, err)

	select {
	case got := <-result:
		require.NoError(t, got.err)
		require.Len(t, got.producer.GetMedias(), 2)
		require.Equal(t, core.CodecAAC, got.producer.GetMedias()[1].Codecs[0].Name)
		require.NoError(t, got.producer.Stop())
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for producer")
	}

	stop := readController(t, ws)
	require.Equal(t, "ChangeVideoSettings", stop.FunctionName)
}

type videoCommand struct {
	Destinations []string `json:"destinations"`
	Parameters   struct {
		StreamName    string `json:"streamName"`
		SuppressAudio bool   `json:"suppressAudio"`
		WithOpus      bool   `json:"withOpus"`
	} `json:"parameters"`
}

func decodeVideoCommand(t *testing.T, msg controlMessage, channel string) videoCommand {
	t.Helper()
	var payload struct {
		Video map[string]struct {
			AVSerializer videoCommand `json:"avSerializer"`
		} `json:"video"`
	}
	require.NoError(t, json.Unmarshal(msg.Payload, &payload))
	require.Len(t, payload.Video, 1)
	command, ok := payload.Video[channel]
	require.True(t, ok)
	return command.AVSerializer
}

func syntheticMedia(token string) []byte {
	metadata := amf.EncodeItems("onMetaData", map[string]any{
		"streamName":  token,
		"videoWidth":  1920,
		"videoHeight": 1080,
	})
	sps := []byte{0x67, 0x42, 0, 0x1f, 0xe5, 0x88}
	pps := []byte{0x68, 0xce, 0x38, 0x80}
	config := append([]byte{0x17, 0, 0, 0, 0}, h264.EncodeConfig(sps, pps)...)
	frame := []byte{0x17, 1, 0, 0, 0, 0, 0, 0, 2, 0x65, 0x88}

	wire := []byte{'F', 'L', 'V', 1, 5, 0, 0, 0, 9, 0, 0, 0, 0}
	for i, tag := range []struct {
		typeID byte
		data   []byte
	}{
		{18, metadata},
		{9, config},
		{9, frame},
	} {
		wire = append(wire, flv.EncodeTag(tag.typeID, uint32(i*20), tag.data)...)
		if i != 2 {
			wire = append(wire, bytes.Repeat([]byte{0x55}, 16)...)
		}
	}
	return wire
}

func syntheticMediaAAC(token string) []byte {
	metadata := amf.EncodeItems("onMetaData", map[string]any{"streamName": token})
	sps := []byte{0x67, 0x42, 0, 0x1f, 0xe5, 0x88}
	pps := []byte{0x68, 0xce, 0x38, 0x80}
	videoConfig := append([]byte{0x17, 0, 0, 0, 0}, h264.EncodeConfig(sps, pps)...)
	audioConfig := []byte{0xaf, 0, 0x14, 0x08}
	videoFrame := []byte{0x17, 1, 0, 0, 0, 0, 0, 0, 2, 0x65, 0x88}
	audioFrame := []byte{0xaf, 1, 1, 2, 3, 4}

	wire := []byte{'F', 'L', 'V', 1, 5, 0, 0, 0, 9, 0, 0, 0, 0}
	for i, tag := range []struct {
		typeID byte
		data   []byte
	}{
		{18, metadata},
		{9, videoConfig},
		{8, audioConfig},
		{9, videoFrame},
		{8, audioFrame},
	} {
		wire = append(wire, flv.EncodeTag(tag.typeID, uint32(i*20), tag.data)...)
		if i != 4 {
			wire = append(wire, bytes.Repeat([]byte{0x55}, 16)...)
		}
	}
	return wire
}
