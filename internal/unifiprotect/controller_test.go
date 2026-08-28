package unifiprotect

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestControllerHandshake(t *testing.T) {
	m := newManager("", 7550, "test", "controller-id")
	ws, server := connectCamera(t, m)
	defer server.Close()
	defer ws.Close()

	s, err := m.waitSession("02AABBCCDDEE", time.Now().Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, 16000, s.opusRate)
	require.Equal(t, "127.0.0.1", s.cameraIP)
}

func TestControllerRejectsMalformedCameraMAC(t *testing.T) {
	m := newManager("", 7550, "test", "controller-id")
	request := httptest.NewRequest(http.MethodGet, websocketPath, nil)
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
	request.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	request.Header.Set("Sec-WebSocket-Protocol", websocketProtocol)
	request.Header.Set("camera-mac", "not-a-mac")
	recorder := httptest.NewRecorder()

	newController(m).ServeHTTP(recorder, request)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestControllerClosesIncompleteHandshake(t *testing.T) {
	m := newManager("", 7550, "test", "controller-id")
	controller := newController(m)
	controller.handshakeTimeout = 20 * time.Millisecond
	server := httptest.NewServer(controller)
	defer server.Close()

	dialer := websocket.Dialer{Subprotocols: []string{websocketProtocol}}
	header := http.Header{"camera-mac": []string{"02AABBCCDDEE"}}
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + websocketPath
	ws, _, err := dialer.Dial(wsURL, header)
	require.NoError(t, err)
	defer ws.Close()

	require.NoError(t, ws.SetReadDeadline(time.Now().Add(time.Second)))
	started := time.Now()
	_, _, err = ws.ReadMessage()
	require.Error(t, err)
	require.Less(t, time.Since(started), 500*time.Millisecond)
}

func connectCamera(t *testing.T, m *Manager) (*websocket.Conn, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(newController(m))

	dialer := websocket.Dialer{Subprotocols: []string{websocketProtocol}}
	header := http.Header{"camera-mac": []string{"02AABBCCDDEE"}}
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + websocketPath
	ws, response, err := dialer.Dial(wsURL, header)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)
	require.Equal(t, websocketProtocol, ws.Subprotocol())

	sendCamera(t, ws, controlMessage{
		From:         "ubnt_avclient",
		FunctionName: "ubnt_avclient_timeSync",
		MessageID:    1,
		Payload:      json.RawMessage(`{"timeDelta":0}`),
		To:           "UniFiVideo",
	})
	timeSync := readController(t, ws)
	require.Equal(t, "ubnt_avclient_timeSync", timeSync.FunctionName)
	require.Equal(t, int64(1), timeSync.InResponseTo)

	hello, err := json.Marshal(map[string]any{
		"protocolVersion": 67,
		"features": map[string]any{
			"audioCodecs":     []string{"aac", "opus"},
			"opusSampleRates": []int{16000},
		},
	})
	require.NoError(t, err)
	sendCamera(t, ws, controlMessage{
		From:         "ubnt_avclient",
		FunctionName: "ubnt_avclient_hello",
		MessageID:    2,
		Payload:      hello,
		To:           "UniFiVideo",
	})
	helloResponse := readController(t, ws)
	require.Equal(t, "ubnt_avclient_hello", helloResponse.FunctionName)
	require.Equal(t, int64(2), helloResponse.InResponseTo)

	agreement := readController(t, ws)
	require.Equal(t, "ubnt_avclient_paramAgreement", agreement.FunctionName)
	require.True(t, agreement.ResponseExpected)
	sendCamera(t, ws, controlMessage{
		From:         "ubnt_avclient",
		FunctionName: "ubnt_avclient_paramAgreement",
		InResponseTo: agreement.MessageID,
		MessageID:    3,
		Payload:      json.RawMessage(`{"authToken":"test"}`),
		To:           "UniFiVideo",
	})

	quiesce := readController(t, ws)
	require.Equal(t, "ChangeVideoSettings", quiesce.FunctionName)
	return ws, server
}

func sendCamera(t *testing.T, ws *websocket.Conn, msg controlMessage) {
	t.Helper()
	b, err := json.Marshal(msg)
	require.NoError(t, err)
	require.NoError(t, ws.WriteMessage(websocket.BinaryMessage, b))
}

func readController(t *testing.T, ws *websocket.Conn) controlMessage {
	t.Helper()
	require.NoError(t, ws.SetReadDeadline(time.Now().Add(3*time.Second)))
	messageType, b, err := ws.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.BinaryMessage, messageType)
	var msg controlMessage
	require.NoError(t, json.Unmarshal(b, &msg))
	return msg
}
