package homekit

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/stretchr/testify/require"
)

type testResult struct {
	value  any
	status int
}

type testServer struct {
	value  any
	status int
	writes int
	perIID map[uint64]testResult
}

func (s *testServer) GetPair(string) []byte                         { return nil }
func (s *testServer) AddPair(string, []byte, byte)                  {}
func (s *testServer) DelPair(string)                                {}
func (s *testServer) GetAccessories(net.Conn) []*hap.Accessory      { return nil }
func (s *testServer) GetCharacteristic(net.Conn, uint8, uint64) any { return nil }
func (s *testServer) GetImage(net.Conn, int, int) []byte            { return nil }

func (s *testServer) SetCharacteristic(_ net.Conn, _ uint8, iid uint64, _ any) (any, int) {
	s.writes++
	if s.perIID != nil {
		if r, ok := s.perIID[iid]; ok {
			return r.value, r.status
		}
	}
	return s.value, s.status
}

// put drives ServerHandler over a pipe with a raw HTTP request, the same way a
// paired controller does over the encrypted HAP connection.
func put(t *testing.T, srv Server, body string) *http.Response {
	t.Helper()

	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	go func() {
		_ = ServerHandler(srv)(server)
		_ = server.Close()
	}()

	req := fmt.Sprintf(
		"PUT /characteristics HTTP/1.1\r\nContent-Type: %s\r\nContent-Length: %d\r\n\r\n%s",
		hap.MimeJSON, len(body), body,
	)
	_, err := client.Write([]byte(req))
	require.NoError(t, err)

	res, err := http.ReadResponse(bufio.NewReader(client), nil)
	require.NoError(t, err)

	return res
}

// TestWriteNoResponse is the regression guard for every characteristic go2rtc
// already supported: a plain successful write must stay 204 with no body.
func TestWriteNoResponse(t *testing.T) {
	srv := &testServer{value: "ignored", status: hap.StatusSuccess}

	res := put(t, srv, `{"characteristics":[{"aid":1,"iid":16,"value":"AQE="}]}`)

	require.Equal(t, http.StatusNoContent, res.StatusCode)
	require.Equal(t, 1, srv.writes)

	b, _ := io.ReadAll(res.Body)
	require.Empty(t, b)
}

func TestWriteResponse(t *testing.T) {
	srv := &testServer{value: "AQIDBA==", status: hap.StatusSuccess}

	res := put(t, srv, `{"characteristics":[{"aid":1,"iid":16,"value":"AQE=","r":true}]}`)

	require.Equal(t, http.StatusMultiStatus, res.StatusCode)

	b, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"characteristics":[{"aid":1,"iid":16,"status":0,"value":"AQIDBA=="}]}`, string(b))
}

func TestWriteError(t *testing.T) {
	srv := &testServer{status: hap.StatusInvalidValue}

	res := put(t, srv, `{"characteristics":[{"aid":1,"iid":16,"value":"AQE=","r":true}]}`)

	require.Equal(t, http.StatusMultiStatus, res.StatusCode)

	b, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"characteristics":[{"aid":1,"iid":16,"status":-70410}]}`, string(b))
}

// TestWriteErrorWithoutResponseRequest - a failure must be reported even when
// the controller did not ask for a write response.
func TestWriteErrorWithoutResponseRequest(t *testing.T) {
	srv := &testServer{status: hap.StatusInsufficientPrivilege}

	res := put(t, srv, `{"characteristics":[{"aid":1,"iid":16,"value":"AQE="}]}`)

	require.Equal(t, http.StatusMultiStatus, res.StatusCode)

	b, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"characteristics":[{"aid":1,"iid":16,"status":-70401}]}`, string(b))
}

// TestWriteMixed - HAP requires every entry of a multi-status response to
// carry a status, including the ones that succeeded.
func TestWriteMixed(t *testing.T) {
	srv := &testServer{perIID: map[uint64]testResult{
		16: {value: "T0s=", status: hap.StatusSuccess},
		17: {status: hap.StatusReadOnly},
	}}

	res := put(t, srv, `{"characteristics":[{"aid":1,"iid":16,"value":"AQE=","r":true},{"aid":1,"iid":17,"value":"AQE="}]}`)

	require.Equal(t, http.StatusMultiStatus, res.StatusCode)

	b, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"characteristics":[`+
		`{"aid":1,"iid":16,"status":0,"value":"T0s="},`+
		`{"aid":1,"iid":17,"status":-70404}]}`, string(b))
}
