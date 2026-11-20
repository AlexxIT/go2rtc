// Author: Sergei "svk" Krashevich <svk@svk.su>
package hksv

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/hap/hds"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	"github.com/pion/rtp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock implementations ---

type mockStreamProvider struct {
	mu        sync.Mutex
	consumers map[string][]core.Consumer
	addErr    error
}

func newMockStreamProvider() *mockStreamProvider {
	return &mockStreamProvider{consumers: make(map[string][]core.Consumer)}
}

func (m *mockStreamProvider) AddConsumer(streamName string, consumer core.Consumer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.addErr != nil {
		return m.addErr
	}
	m.consumers[streamName] = append(m.consumers[streamName], consumer)
	return nil
}

func (m *mockStreamProvider) RemoveConsumer(streamName string, consumer core.Consumer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cs := m.consumers[streamName]
	for i, c := range cs {
		if c == consumer {
			m.consumers[streamName] = append(cs[:i], cs[i+1:]...)
			return
		}
	}
}

func (m *mockStreamProvider) count(streamName string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.consumers[streamName])
}

type mockPairingStore struct {
	mu    sync.Mutex
	saved map[string][]string
	err   error
}

func newMockPairingStore() *mockPairingStore {
	return &mockPairingStore{saved: make(map[string][]string)}
}

func (m *mockPairingStore) SavePairings(streamName string, pairings []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	cp := make([]string, len(pairings))
	copy(cp, pairings)
	m.saved[streamName] = cp
	return nil
}

func (m *mockPairingStore) get(streamName string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saved[streamName]
}

type mockSnapshotProvider struct {
	data   []byte
	err    error
	called bool
	width  int
	height int
}

func (m *mockSnapshotProvider) GetSnapshot(streamName string, width, height int) ([]byte, error) {
	m.called = true
	m.width = width
	m.height = height
	return m.data, m.err
}

type mockLiveStreamHandler struct {
	setupCalled  bool
	startCalled  bool
	stopCalled   bool
	setupErr     error
	startErr     error
	endpointsVal any
}

func (m *mockLiveStreamHandler) SetupEndpoints(conn net.Conn, offer *camera.SetupEndpointsRequest) (any, error) {
	m.setupCalled = true
	return "setup-resp", m.setupErr
}
func (m *mockLiveStreamHandler) GetEndpointsResponse() any {
	return m.endpointsVal
}
func (m *mockLiveStreamHandler) StartStream(streamName string, conf *camera.SelectedStreamConfiguration, connTracker ConnTracker) error {
	m.startCalled = true
	return m.startErr
}
func (m *mockLiveStreamHandler) StopStream(sessionID string, connTracker ConnTracker) error {
	m.stopCalled = true
	return nil
}

// --- Test helpers ---

func newTestServer(t *testing.T, opts ...func(*Config)) *Server {
	t.Helper()
	streams := newMockStreamProvider()
	cfg := Config{
		StreamName: "test-camera",
		Pin:        "27041991",
		HKSV:       true,
		Streams:    streams,
		Logger:     zerolog.Nop(),
		Port:       0,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	srv, err := NewServer(cfg)
	require.NoError(t, err)
	return srv
}

// ====================================================================
// NewServer
// ====================================================================

func TestNewServer_MinimalHKSV(t *testing.T) {
	streams := newMockStreamProvider()
	srv, err := NewServer(Config{
		StreamName: "cam1",
		Pin:        "27041991",
		HKSV:       true,
		Streams:    streams,
		Logger:     zerolog.Nop(),
	})
	require.NoError(t, err)
	require.NotNil(t, srv)

	require.Equal(t, "cam1", srv.StreamName())
	require.NotNil(t, srv.Accessory())
	require.NotNil(t, srv.MDNSEntry())

	// Verify mDNS entry fields
	mdns := srv.MDNSEntry()
	require.NotEmpty(t, mdns.Name)
	require.Equal(t, hap.CategoryCamera, mdns.Info[hap.TXTCategory])
	require.Equal(t, hap.StatusNotPaired, mdns.Info[hap.TXTStatusFlags])
}

func TestNewServer_DefaultPin(t *testing.T) {
	srv, err := NewServer(Config{
		StreamName: "cam1",
		HKSV:       true,
		Streams:    newMockStreamProvider(),
		Logger:     zerolog.Nop(),
	})
	require.NoError(t, err)
	require.NotNil(t, srv)
}

func TestNewServer_InvalidPin(t *testing.T) {
	_, err := NewServer(Config{
		StreamName: "cam1",
		Pin:        "123", // too short
		HKSV:       true,
		Streams:    newMockStreamProvider(),
		Logger:     zerolog.Nop(),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid pin")
}

func TestNewServer_DoorbellCategory(t *testing.T) {
	srv := newTestServer(t, func(c *Config) {
		c.CategoryID = "doorbell"
	})
	require.Equal(t, hap.CategoryDoorbell, srv.MDNSEntry().Info[hap.TXTCategory])

	// Doorbell accessory should have ProgrammableSwitchEvent char
	char := srv.accessory.GetCharacter("73")
	require.NotNil(t, char, "doorbell should have ProgrammableSwitchEvent characteristic")
}

func TestNewServer_CameraCategory(t *testing.T) {
	srv := newTestServer(t)
	require.Equal(t, hap.CategoryCamera, srv.MDNSEntry().Info[hap.TXTCategory])
}

func TestNewServer_ProxyMode(t *testing.T) {
	srv, err := NewServer(Config{
		StreamName: "cam1",
		Pin:        "27041991",
		ProxyURL:   "http://192.168.1.100:51827",
		Streams:    newMockStreamProvider(),
		Logger:     zerolog.Nop(),
	})
	require.NoError(t, err)
	require.Nil(t, srv.Accessory(), "proxy mode should not create local accessory")
	require.Equal(t, "http://192.168.1.100:51827", srv.proxyURL)
}

func TestNewServer_SpeakerDisabledByDefault(t *testing.T) {
	srv := newTestServer(t)
	// Speaker service type is "113"
	svc := srv.accessory.GetService("113")
	require.Nil(t, svc, "speaker service should be removed by default")
}

func TestNewServer_SpeakerEnabled(t *testing.T) {
	speakerOn := true
	srv := newTestServer(t, func(c *Config) {
		c.Speaker = &speakerOn
	})
	svc := srv.accessory.GetService("113")
	require.NotNil(t, svc, "speaker service should be present when enabled")
}

func TestNewServer_CustomName(t *testing.T) {
	srv := newTestServer(t, func(c *Config) {
		c.Name = "Living Room Camera"
	})
	require.Equal(t, "Living Room Camera", srv.MDNSEntry().Name)
}

func TestNewServer_CustomDeviceID(t *testing.T) {
	srv := newTestServer(t, func(c *Config) {
		c.DeviceID = "AA:BB:CC:DD:EE:FF"
	})
	require.Equal(t, "AA:BB:CC:DD:EE:FF", srv.MDNSEntry().Info[hap.TXTDeviceID])
}

func TestNewServer_MotionThresholdDefault(t *testing.T) {
	srv := newTestServer(t, func(c *Config) {
		c.MotionMode = "detect"
	})
	require.Equal(t, defaultThreshold, srv.motionThreshold)
}

func TestNewServer_MotionThresholdCustom(t *testing.T) {
	srv := newTestServer(t, func(c *Config) {
		c.MotionMode = "detect"
		c.MotionThreshold = 3.5
	})
	require.Equal(t, 3.5, srv.motionThreshold)
}

func TestNewServer_NonHKSV(t *testing.T) {
	srv, err := NewServer(Config{
		StreamName: "cam1",
		Pin:        "27041991",
		HKSV:       false,
		Streams:    newMockStreamProvider(),
		Logger:     zerolog.Nop(),
	})
	require.NoError(t, err)
	require.NotNil(t, srv.Accessory())
	// Non-HKSV accessory should NOT have motion sensor
	char := srv.accessory.GetCharacter("22")
	require.Nil(t, char, "non-HKSV should not have MotionDetected")
}

// ====================================================================
// Pairing Management
// ====================================================================

func TestPairing_AddAndGet(t *testing.T) {
	srv := newTestServer(t)

	pub := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	srv.AddPair("client-1", pub, hap.PermissionAdmin)

	got := srv.GetPair("client-1")
	require.Equal(t, pub, got)
}

func TestPairing_GetUnknown(t *testing.T) {
	srv := newTestServer(t)
	require.Nil(t, srv.GetPair("nonexistent"))
}

func TestPairing_Delete(t *testing.T) {
	srv := newTestServer(t)
	pub := []byte{1, 2, 3, 4}
	srv.AddPair("client-1", pub, hap.PermissionAdmin)
	require.NotNil(t, srv.GetPair("client-1"))

	srv.DelPair("client-1")
	require.Nil(t, srv.GetPair("client-1"))
}

func TestPairing_DeleteNonexistent(t *testing.T) {
	srv := newTestServer(t)
	// Should not panic
	srv.DelPair("nonexistent")
}

func TestPairing_NoDuplicates(t *testing.T) {
	srv := newTestServer(t)
	pub := []byte{1, 2, 3, 4}
	srv.AddPair("client-1", pub, hap.PermissionAdmin)
	srv.AddPair("client-1", pub, hap.PermissionAdmin) // duplicate
	require.Len(t, srv.pairings, 1)
}

func TestPairing_MultiplePairs(t *testing.T) {
	srv := newTestServer(t)
	srv.AddPair("client-1", []byte{1}, hap.PermissionAdmin)
	srv.AddPair("client-2", []byte{2}, hap.PermissionAdmin)
	srv.AddPair("client-3", []byte{3}, hap.PermissionAdmin)

	require.Len(t, srv.pairings, 3)
	require.NotNil(t, srv.GetPair("client-1"))
	require.NotNil(t, srv.GetPair("client-2"))
	require.NotNil(t, srv.GetPair("client-3"))

	srv.DelPair("client-2")
	require.Len(t, srv.pairings, 2)
	require.Nil(t, srv.GetPair("client-2"))
	require.NotNil(t, srv.GetPair("client-1"))
	require.NotNil(t, srv.GetPair("client-3"))
}

func TestPairing_Persistence(t *testing.T) {
	store := newMockPairingStore()
	srv := newTestServer(t, func(c *Config) {
		c.Store = store
	})

	srv.AddPair("client-1", []byte{1, 2, 3, 4}, hap.PermissionAdmin)

	saved := store.get("test-camera")
	require.Len(t, saved, 1)
	require.Contains(t, saved[0], "client_id=client-1")

	srv.DelPair("client-1")
	saved = store.get("test-camera")
	require.Len(t, saved, 0)
}

func TestPairing_PersistenceError(t *testing.T) {
	store := newMockPairingStore()
	store.err = errors.New("disk full")
	srv := newTestServer(t, func(c *Config) {
		c.Store = store
	})

	// Should not panic, just log the error
	srv.AddPair("client-1", []byte{1}, hap.PermissionAdmin)
	require.Len(t, srv.pairings, 1) // pairing is still added in memory
}

func TestPairing_PreExisting(t *testing.T) {
	srv, err := NewServer(Config{
		StreamName: "cam1",
		Pin:        "27041991",
		HKSV:       true,
		Pairings:   []string{"client_id=pre-existing&client_public=0102&permissions=1"},
		Streams:    newMockStreamProvider(),
		Logger:     zerolog.Nop(),
	})
	require.NoError(t, err)

	got := srv.GetPair("pre-existing")
	require.Equal(t, []byte{1, 2}, got)
}

// ====================================================================
// UpdateStatus
// ====================================================================

func TestUpdateStatus_NotPaired(t *testing.T) {
	srv := newTestServer(t)
	require.Equal(t, hap.StatusNotPaired, srv.MDNSEntry().Info[hap.TXTStatusFlags])
}

func TestUpdateStatus_Paired(t *testing.T) {
	srv := newTestServer(t)
	srv.AddPair("client-1", []byte{1}, hap.PermissionAdmin)
	require.Equal(t, hap.StatusPaired, srv.MDNSEntry().Info[hap.TXTStatusFlags])
}

func TestUpdateStatus_UnpairedAfterDelete(t *testing.T) {
	srv := newTestServer(t)
	srv.AddPair("client-1", []byte{1}, hap.PermissionAdmin)
	require.Equal(t, hap.StatusPaired, srv.MDNSEntry().Info[hap.TXTStatusFlags])

	srv.DelPair("client-1")
	require.Equal(t, hap.StatusNotPaired, srv.MDNSEntry().Info[hap.TXTStatusFlags])
}

// ====================================================================
// Connection Tracking
// ====================================================================

func TestConnTracking_AddDel(t *testing.T) {
	srv := newTestServer(t)
	require.Empty(t, srv.conns)

	conn1 := "conn1"
	conn2 := "conn2"
	srv.AddConn(conn1)
	srv.AddConn(conn2)
	require.Len(t, srv.conns, 2)

	srv.DelConn(conn1)
	require.Len(t, srv.conns, 1)

	srv.DelConn(conn2)
	require.Empty(t, srv.conns)
}

func TestConnTracking_DelNonexistent(t *testing.T) {
	srv := newTestServer(t)
	// Should not panic
	srv.DelConn("never-added")
	require.Empty(t, srv.conns)
}

func TestConnTracking_Concurrent(t *testing.T) {
	srv := newTestServer(t)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			conn := fmt.Sprintf("conn-%d", n)
			srv.AddConn(conn)
			time.Sleep(time.Millisecond)
			srv.DelConn(conn)
		}(i)
	}
	wg.Wait()
	require.Empty(t, srv.conns)
}

// ====================================================================
// MarshalJSON
// ====================================================================

func TestMarshalJSON_Unpaired(t *testing.T) {
	srv := newTestServer(t, func(c *Config) {
		c.Name = "TestCam"
	})

	data, err := srv.MarshalJSON()
	require.NoError(t, err)

	var v map[string]any
	require.NoError(t, json.Unmarshal(data, &v))

	require.Equal(t, "TestCam", v["name"])
	require.NotEmpty(t, v["device_id"])
	require.NotEmpty(t, v["setup_code"])
	require.NotEmpty(t, v["setup_id"])
	_, hasPaired := v["paired"]
	require.False(t, hasPaired, "paired=0 should be omitted with omitempty")
}

func TestMarshalJSON_Paired(t *testing.T) {
	srv := newTestServer(t)
	srv.AddPair("client-1", []byte{1}, hap.PermissionAdmin)

	data, err := srv.MarshalJSON()
	require.NoError(t, err)

	var v map[string]any
	require.NoError(t, json.Unmarshal(data, &v))

	require.Equal(t, float64(1), v["paired"])
	// Setup code should be hidden when paired
	_, hasSetupCode := v["setup_code"]
	require.False(t, hasSetupCode || v["setup_code"] == "", "setup code should not be in paired JSON")
}

// ====================================================================
// GetAccessories
// ====================================================================

func TestGetAccessories(t *testing.T) {
	srv := newTestServer(t)
	accs := srv.GetAccessories(nil)
	require.Len(t, accs, 1)
	require.Equal(t, srv.accessory, accs[0])
}

// ====================================================================
// SetMotionDetected
// ====================================================================

func TestSetMotionDetected(t *testing.T) {
	srv := newTestServer(t)

	char := srv.accessory.GetCharacter("22") // MotionDetected
	require.NotNil(t, char)

	srv.SetMotionDetected(true)
	require.Equal(t, true, char.Value)

	srv.SetMotionDetected(false)
	require.Equal(t, false, char.Value)
}

func TestSetMotionDetected_NoAccessory(t *testing.T) {
	srv := newTestServer(t)
	srv.accessory = nil
	// Should not panic
	srv.SetMotionDetected(true)
}

// ====================================================================
// TriggerDoorbell
// ====================================================================

func TestTriggerDoorbell(t *testing.T) {
	srv := newTestServer(t, func(c *Config) {
		c.CategoryID = "doorbell"
	})

	char := srv.accessory.GetCharacter("73") // ProgrammableSwitchEvent
	require.NotNil(t, char)

	srv.TriggerDoorbell()
	require.Equal(t, 0, char.Value) // SINGLE_PRESS
}

func TestTriggerDoorbell_CameraAccessory(t *testing.T) {
	srv := newTestServer(t) // camera, not doorbell
	// Should not panic (GetCharacter returns nil, function returns early)
	srv.TriggerDoorbell()
}

// ====================================================================
// GetImage (snapshots)
// ====================================================================

func TestGetImage_NoProvider(t *testing.T) {
	srv := newTestServer(t)
	result := srv.GetImage(nil, 640, 480)
	require.Nil(t, result)
}

func TestGetImage_WithProvider(t *testing.T) {
	snapshot := &mockSnapshotProvider{data: []byte("fake-jpeg-data")}
	srv := newTestServer(t, func(c *Config) {
		c.Snapshots = snapshot
	})

	result := srv.GetImage(nil, 1920, 1080)
	require.Equal(t, []byte("fake-jpeg-data"), result)
	require.True(t, snapshot.called)
	require.Equal(t, 1920, snapshot.width)
	require.Equal(t, 1080, snapshot.height)
}

func TestGetImage_ProviderError(t *testing.T) {
	snapshot := &mockSnapshotProvider{err: errors.New("no camera")}
	srv := newTestServer(t, func(c *Config) {
		c.Snapshots = snapshot
	})

	result := srv.GetImage(nil, 640, 480)
	require.Nil(t, result)
}

// ====================================================================
// GetCharacteristic / SetCharacteristic
// ====================================================================

func TestGetCharacteristic_KnownChar(t *testing.T) {
	srv := newTestServer(t)

	// MotionDetected (type "22") should be accessible
	char := srv.accessory.GetCharacter("22")
	require.NotNil(t, char)

	val := srv.GetCharacteristic(nil, 1, char.IID)
	require.Equal(t, char.Value, val)
}

func TestGetCharacteristic_UnknownIID(t *testing.T) {
	srv := newTestServer(t)
	val := srv.GetCharacteristic(nil, 1, 0xFFFFFF)
	require.Nil(t, val)
}

func TestGetCharacteristic_SetupEndpoints_NoLiveStream(t *testing.T) {
	srv := newTestServer(t)

	char := srv.accessory.GetCharacter(camera.TypeSetupEndpoints)
	require.NotNil(t, char)

	val := srv.GetCharacteristic(nil, 1, char.IID)
	require.Nil(t, val)
}

func TestGetCharacteristic_SetupEndpoints_WithLiveStream(t *testing.T) {
	handler := &mockLiveStreamHandler{endpointsVal: "test-endpoints"}
	srv := newTestServer(t, func(c *Config) {
		c.LiveStream = handler
	})

	char := srv.accessory.GetCharacter(camera.TypeSetupEndpoints)
	val := srv.GetCharacteristic(nil, 1, char.IID)
	require.Equal(t, "test-endpoints", val)
}

func TestSetCharacteristic_GenericChar(t *testing.T) {
	srv := newTestServer(t)

	// Active (type "B0") — generic set
	char := srv.accessory.GetCharacter("B0")
	require.NotNil(t, char)

	srv.SetCharacteristic(nil, 1, char.IID, 0)
	require.Equal(t, 0, char.Value)
}

func TestSetCharacteristic_UnknownIID(t *testing.T) {
	srv := newTestServer(t)
	// Should not panic
	srv.SetCharacteristic(nil, 1, 0xFFFFFF, "value")
}

func TestSetCharacteristic_SetupEndpoints_WithLiveStream(t *testing.T) {
	handler := &mockLiveStreamHandler{}
	srv := newTestServer(t, func(c *Config) {
		c.LiveStream = handler
	})

	char := srv.accessory.GetCharacter(camera.TypeSetupEndpoints)
	require.NotNil(t, char)

	// Create valid TLV8 base64 data for SetupEndpointsRequest
	req := camera.SetupEndpointsRequest{
		SessionID: "test-session-id-1234",
	}
	encoded, err := tlv8.MarshalBase64(req)
	require.NoError(t, err)

	srv.SetCharacteristic(nil, 1, char.IID, encoded)
	require.True(t, handler.setupCalled)
}

func TestSetCharacteristic_SetupEndpoints_NoLiveStream(t *testing.T) {
	srv := newTestServer(t) // no live stream handler

	char := srv.accessory.GetCharacter(camera.TypeSetupEndpoints)
	require.NotNil(t, char)

	req := camera.SetupEndpointsRequest{SessionID: "test"}
	encoded, _ := tlv8.MarshalBase64(req)

	// Should not panic
	srv.SetCharacteristic(nil, 1, char.IID, encoded)
}

func TestSetCharacteristic_SetupEndpoints_InvalidTLV8(t *testing.T) {
	handler := &mockLiveStreamHandler{}
	srv := newTestServer(t, func(c *Config) {
		c.LiveStream = handler
	})

	char := srv.accessory.GetCharacter(camera.TypeSetupEndpoints)
	srv.SetCharacteristic(nil, 1, char.IID, "not-valid-base64-tlv8")
	require.False(t, handler.setupCalled, "invalid TLV8 should not call handler")
}

func TestSetCharacteristic_SelectedStream_Start(t *testing.T) {
	handler := &mockLiveStreamHandler{}
	srv := newTestServer(t, func(c *Config) {
		c.LiveStream = handler
	})

	char := srv.accessory.GetCharacter(camera.TypeSelectedStreamConfiguration)
	require.NotNil(t, char)

	conf := camera.SelectedStreamConfiguration{
		Control: camera.SessionControl{
			SessionID: "session-123",
			Command:   camera.SessionCommandStart,
		},
	}
	encoded, err := tlv8.MarshalBase64(conf)
	require.NoError(t, err)

	srv.SetCharacteristic(nil, 1, char.IID, encoded)
	require.True(t, handler.startCalled)
}

func TestSetCharacteristic_SelectedStream_End(t *testing.T) {
	handler := &mockLiveStreamHandler{}
	srv := newTestServer(t, func(c *Config) {
		c.LiveStream = handler
	})

	char := srv.accessory.GetCharacter(camera.TypeSelectedStreamConfiguration)
	conf := camera.SelectedStreamConfiguration{
		Control: camera.SessionControl{
			SessionID: "session-123",
			Command:   camera.SessionCommandEnd,
		},
	}
	encoded, _ := tlv8.MarshalBase64(conf)

	srv.SetCharacteristic(nil, 1, char.IID, encoded)
	require.True(t, handler.stopCalled)
}

func TestSetCharacteristic_SelectedStream_NoLiveStream(t *testing.T) {
	srv := newTestServer(t)

	char := srv.accessory.GetCharacter(camera.TypeSelectedStreamConfiguration)
	conf := camera.SelectedStreamConfiguration{
		Control: camera.SessionControl{Command: camera.SessionCommandStart},
	}
	encoded, _ := tlv8.MarshalBase64(conf)

	// Should not panic
	srv.SetCharacteristic(nil, 1, char.IID, encoded)
}

func TestSetCharacteristic_SelectedRecordingConfig(t *testing.T) {
	streams := newMockStreamProvider()
	srv := newTestServer(t, func(c *Config) {
		c.MotionMode = "detect"
		c.Streams = streams
	})

	char := srv.accessory.GetCharacter(camera.TypeSelectedCameraRecordingConfiguration)
	require.NotNil(t, char)

	srv.SetCharacteristic(nil, 1, char.IID, "some-config-value")
	require.Equal(t, "some-config-value", char.Value)
}

func TestSetCharacteristic_DataStreamTransport_CloseRequest(t *testing.T) {
	srv := newTestServer(t)

	char := srv.accessory.GetCharacter(camera.TypeSetupDataStreamTransport)
	require.NotNil(t, char)

	// Create a close request (SessionCommandType != 0)
	req := camera.SetupDataStreamTransportRequest{
		SessionCommandType: 1, // close
	}
	encoded, err := tlv8.MarshalBase64(req)
	require.NoError(t, err)

	// Should not panic (no active session)
	srv.SetCharacteristic(nil, 1, char.IID, encoded)
}

func TestSetCharacteristic_DataStreamTransport_InvalidTLV8(t *testing.T) {
	srv := newTestServer(t)

	char := srv.accessory.GetCharacter(camera.TypeSetupDataStreamTransport)
	// Invalid TLV8 — should log error and return
	srv.SetCharacteristic(nil, 1, char.IID, "bad-data")
}

// ====================================================================
// prepareHKSVConsumer / takePreparedConsumer
// ====================================================================

func TestTakePreparedConsumer_None(t *testing.T) {
	srv := newTestServer(t)
	require.Nil(t, srv.takePreparedConsumer())
}

func TestTakePreparedConsumer_Available(t *testing.T) {
	srv := newTestServer(t)
	consumer := NewHKSVConsumer(zerolog.Nop())
	srv.preparedConsumer = consumer

	got := srv.takePreparedConsumer()
	require.Equal(t, consumer, got)
	require.Nil(t, srv.preparedConsumer, "should be cleared after take")
}

func TestTakePreparedConsumer_OnlyOnce(t *testing.T) {
	srv := newTestServer(t)
	srv.preparedConsumer = NewHKSVConsumer(zerolog.Nop())

	first := srv.takePreparedConsumer()
	require.NotNil(t, first)

	second := srv.takePreparedConsumer()
	require.Nil(t, second, "second take should return nil")
}

// ====================================================================
// startMotionDetector
// ====================================================================

func TestStartMotionDetector_AddsAndRemoves(t *testing.T) {
	streams := newMockStreamProvider()
	srv := newTestServer(t, func(c *Config) {
		c.MotionMode = "detect"
		c.Streams = streams
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.startMotionDetector()
	}()

	// Wait for consumer to be added
	require.Eventually(t, func() bool {
		return streams.count("test-camera") == 1
	}, 2*time.Second, 10*time.Millisecond)

	// Motion detector should be set
	srv.mu.Lock()
	det := srv.motionDetector
	srv.mu.Unlock()
	require.NotNil(t, det)

	// Stop the detector
	_ = det.Stop()
	<-done

	// Should be cleaned up
	require.Equal(t, 0, streams.count("test-camera"))
	srv.mu.Lock()
	require.Nil(t, srv.motionDetector)
	srv.mu.Unlock()
}

func TestStartMotionDetector_Idempotent(t *testing.T) {
	streams := newMockStreamProvider()
	srv := newTestServer(t, func(c *Config) {
		c.MotionMode = "detect"
		c.Streams = streams
	})

	// Start first detector
	done1 := make(chan struct{})
	go func() {
		defer close(done1)
		srv.startMotionDetector()
	}()

	require.Eventually(t, func() bool {
		return streams.count("test-camera") == 1
	}, 2*time.Second, 10*time.Millisecond)

	// Second start should be no-op
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		srv.startMotionDetector()
	}()
	<-done2 // returns immediately

	// Should still have only 1 consumer
	require.Equal(t, 1, streams.count("test-camera"))

	srv.stopMotionDetector()
	<-done1
}

func TestStartMotionDetector_StreamError(t *testing.T) {
	streams := newMockStreamProvider()
	streams.addErr = errors.New("stream not found")
	srv := newTestServer(t, func(c *Config) {
		c.MotionMode = "detect"
		c.Streams = streams
	})

	srv.startMotionDetector()

	// Should clean up and not leave a dangling detector
	srv.mu.Lock()
	require.Nil(t, srv.motionDetector)
	srv.mu.Unlock()
}

// ====================================================================
// isClosedConnErr
// ====================================================================

func TestIsClosedConnErr(t *testing.T) {
	require.False(t, isClosedConnErr(nil))
	require.False(t, isClosedConnErr(errors.New("something")))
	require.True(t, isClosedConnErr(errors.New("use of closed network connection")))
	require.True(t, isClosedConnErr(fmt.Errorf("wrapped: %w",
		errors.New("read: use of closed network connection"))))
}

// ====================================================================
// Consumer Integration: realistic fMP4 flow via AddTrack
// ====================================================================

func TestConsumer_AddTrack_H264(t *testing.T) {
	c := NewHKSVConsumer(zerolog.Nop())

	videoMedia := c.Medias[0]
	videoCodec := &core.Codec{
		Name:      core.CodecH264,
		ClockRate: 90000,
		FmtpLine:  "profile-level-id=42e01f",
	}
	receiver := core.NewReceiver(videoMedia, videoCodec)

	err := c.AddTrack(videoMedia, videoCodec, receiver)
	require.NoError(t, err)
	require.Len(t, c.Senders, 1)
}

func TestConsumer_AddTrack_H264AndAAC(t *testing.T) {
	c := NewHKSVConsumer(zerolog.Nop())

	videoCodec := &core.Codec{
		Name:      core.CodecH264,
		ClockRate: 90000,
		FmtpLine:  "profile-level-id=42e01f",
	}
	audioCodec := &core.Codec{
		Name:      core.CodecAAC,
		ClockRate: 16000,
		Channels:  1,
	}

	vReceiver := core.NewReceiver(c.Medias[0], videoCodec)
	aReceiver := core.NewReceiver(c.Medias[1], audioCodec)

	err := c.AddTrack(c.Medias[0], videoCodec, vReceiver)
	require.NoError(t, err)

	err = c.AddTrack(c.Medias[1], audioCodec, aReceiver)
	require.NoError(t, err)

	require.Len(t, c.Senders, 2)

	// Init should be built after both tracks added
	select {
	case <-c.initDone:
		require.NoError(t, c.initErr)
		require.NotEmpty(t, c.initData)
	default:
		t.Fatal("initDone should be closed after both tracks are added")
	}
}

func TestConsumer_AddTrack_UnsupportedCodec(t *testing.T) {
	c := NewHKSVConsumer(zerolog.Nop())

	codec := &core.Codec{Name: core.CodecVP9, ClockRate: 90000}
	receiver := core.NewReceiver(c.Medias[0], codec)

	err := c.AddTrack(c.Medias[0], codec, receiver)
	require.NoError(t, err) // returns nil for unsupported
	require.Len(t, c.Senders, 0, "unsupported codec should not add sender")
}

func TestConsumer_AddTrack_LateTrackIgnored(t *testing.T) {
	c := NewHKSVConsumer(zerolog.Nop())

	// Build init with one track
	videoCodec := &core.Codec{Name: core.CodecH264, ClockRate: 90000}
	vReceiver := core.NewReceiver(c.Medias[0], videoCodec)
	_ = c.AddTrack(c.Medias[0], videoCodec, vReceiver)

	audioCodec := &core.Codec{Name: core.CodecAAC, ClockRate: 16000, Channels: 1}
	aReceiver := core.NewReceiver(c.Medias[1], audioCodec)
	_ = c.AddTrack(c.Medias[1], audioCodec, aReceiver)

	// Init is built
	<-c.initDone

	// Late track should be ignored
	lateCodec := &core.Codec{Name: core.CodecH264, ClockRate: 90000}
	lateReceiver := core.NewReceiver(c.Medias[0], lateCodec)
	err := c.AddTrack(c.Medias[0], lateCodec, lateReceiver)
	require.NoError(t, err)
	require.Len(t, c.Senders, 2, "late track should not add another sender")
}

// ====================================================================
// Full HKSV Recording Flow (integration)
// ====================================================================

func TestConsumer_FullRecordingFlow(t *testing.T) {
	// This test simulates a realistic HKSV recording:
	// 1. Create consumer with H264+AAC tracks
	// 2. Activate with HDS session
	// 3. Send keyframe + P-frames as GOP
	// 4. Send next keyframe (triggers flush)
	// 5. Verify fragment received on controller side

	acc, ctrl := newTestSessionPair(t)
	c := NewHKSVConsumer(zerolog.Nop())

	// Add tracks
	videoCodec := &core.Codec{Name: core.CodecH264, ClockRate: 90000}
	audioCodec := &core.Codec{Name: core.CodecAAC, ClockRate: 16000, Channels: 1}
	vReceiver := core.NewReceiver(c.Medias[0], videoCodec)
	aReceiver := core.NewReceiver(c.Medias[1], audioCodec)
	require.NoError(t, c.AddTrack(c.Medias[0], videoCodec, vReceiver))
	require.NoError(t, c.AddTrack(c.Medias[1], audioCodec, aReceiver))

	// Read init from controller side
	initDone := make(chan struct{})
	go func() {
		defer close(initDone)
		msg, err := ctrl.ReadMessage()
		assert.NoError(t, err)
		assert.Equal(t, "dataSend", msg.Protocol)
		packets := msg.Body["packets"].([]any)
		pkt := packets[0].(map[string]any)
		meta := pkt["metadata"].(map[string]any)
		assert.Equal(t, "mediaInitialization", meta["dataType"])
	}()

	// Activate
	require.NoError(t, c.Activate(acc, 1))
	<-initDone

	require.True(t, c.active)
	require.Equal(t, 2, c.seqNum)

	// Simulate GOP: keyframe + P-frames
	// Send a fake keyframe (IDR NAL type 5)
	keyframePayload := make([]byte, 2000)
	keyframePayload[4] = 0x65 // NAL type 5 = IDR
	c.mu.Lock()
	b := c.muxer.GetPayload(0, &rtp.Packet{
		Header:  rtp.Header{Timestamp: 0, SequenceNumber: 1},
		Payload: keyframePayload,
	})
	c.fragBuf = append(c.fragBuf, b...)

	// Add some P-frames
	for i := 0; i < 5; i++ {
		pFramePayload := make([]byte, 500)
		pFramePayload[4] = 0x41 // NAL type 1 = non-IDR
		b = c.muxer.GetPayload(0, &rtp.Packet{
			Header:  rtp.Header{Timestamp: uint32(3000 * (i + 1)), SequenceNumber: uint16(i + 2)},
			Payload: pFramePayload,
		})
		c.fragBuf = append(c.fragBuf, b...)
	}
	c.mu.Unlock()

	// Read fragment from controller side
	fragDone := make(chan struct{})
	go func() {
		defer close(fragDone)
		msg, err := ctrl.ReadMessage()
		assert.NoError(t, err)
		packets := msg.Body["packets"].([]any)
		pkt := packets[0].(map[string]any)
		meta := pkt["metadata"].(map[string]any)
		assert.Equal(t, "mediaFragment", meta["dataType"])
		assert.Equal(t, int64(2), meta["dataSequenceNumber"].(int64))
	}()

	// Flush the fragment
	c.mu.Lock()
	c.flushFragment()
	c.mu.Unlock()

	<-fragDone
	require.Equal(t, 3, c.seqNum)

	// Stop
	require.NoError(t, c.Stop())
	require.False(t, c.active)
}

// ====================================================================
// Motion Detector Integration with Server
// ====================================================================

func TestMotionDetector_IntegrationWithServer(t *testing.T) {
	// Simulates: server starts motion detector, detector triggers motion,
	// server updates MotionDetected characteristic

	streams := newMockStreamProvider()
	srv := newTestServer(t, func(c *Config) {
		c.MotionMode = "detect"
		c.MotionThreshold = 2.0
		c.Streams = streams
	})

	motionChar := srv.accessory.GetCharacter("22")
	require.NotNil(t, motionChar)

	// Start motion detector in background
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.startMotionDetector()
	}()

	// Wait for detector to be registered
	require.Eventually(t, func() bool {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		return srv.motionDetector != nil
	}, 2*time.Second, 10*time.Millisecond)

	// Manually trigger motion through the detector
	srv.mu.Lock()
	det := srv.motionDetector
	srv.mu.Unlock()

	// Feed warmup frames
	for i := 0; i < motionWarmupFrames; i++ {
		det.handlePacket(makePFrame(500))
	}
	det.holdBudget = 10
	det.cooldownBudget = 5

	// Trigger motion with large frame
	det.handlePacket(makePFrame(5000))

	// MotionDetected characteristic should be true
	require.Equal(t, true, motionChar.Value)

	// Expire hold
	for i := 0; i < 10; i++ {
		det.handlePacket(makePFrame(500))
	}

	// MotionDetected should be false
	require.Equal(t, false, motionChar.Value)

	// Clean up
	_ = det.Stop()
	<-done
}

// ====================================================================
// connLabel
// ====================================================================

func TestConnLabel(t *testing.T) {
	require.Contains(t, connLabel("hello"), "string")
	require.Contains(t, connLabel(42), "int")
}

// ====================================================================
// connLabel with HDS conn
// ====================================================================

func TestConnLabel_HDSConn(t *testing.T) {
	key := []byte(core.RandString(16, 0))
	salt := core.RandString(32, 0)
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	hdsConn, err := hds.NewConn(c1, key, salt, false)
	require.NoError(t, err)

	label := connLabel(hdsConn)
	require.Contains(t, label, "hds")
}
