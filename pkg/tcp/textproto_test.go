package tcp

import (
	"bufio"
	"bytes"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	data := []byte(`RTSP/1.0 401 Unauthorized
WWW-Authenticate: Digest realm="testrealm@host.com",
                        nonce="dcd98b7102dd2f0e8b11d0f600bfb0c093",

`)

	buf := bytes.NewBuffer(data)
	r := bufio.NewReader(buf)

	res, err := ReadResponse(r)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

func TestRequestStringHeaderOrder(t *testing.T) {
	u, err := url.Parse("rtsp://example.com/stream")
	require.NoError(t, err)

	req := &Request{
		Method: "DESCRIBE",
		URL:    u,
		Proto:  "RTSP/1.0",
		Header: textproto.MIMEHeader{
			"User-Agent":     {"go2rtc"},
			"Accept":         {"application/sdp"},
			"CSeq":           {"2"},
			"Authorization":  {"Basic dGVzdA=="},
			"Content-Length": {"0"},
		},
	}

	expected := "DESCRIBE rtsp://example.com/stream RTSP/1.0\r\n" +
		"CSeq: 2\r\n" +
		"Authorization: Basic dGVzdA==\r\n" +
		"User-Agent: go2rtc\r\n" +
		"Accept: application/sdp\r\n" +
		"Content-Length: 0\r\n" +
		"\r\n"

	require.Equal(t, expected, req.String())
}

func TestResponseStringHeaderOrder(t *testing.T) {
	res := &Response{
		Proto:  "RTSP/1.0",
		Status: "200 OK",
		Header: textproto.MIMEHeader{
			"Public":         {"OPTIONS, DESCRIBE"},
			"Content-Length": {"0"},
			"CSeq":           {"3"},
		},
	}

	expected := "RTSP/1.0 200 OK\r\n" +
		"CSeq: 3\r\n" +
		"Public: OPTIONS, DESCRIBE\r\n" +
		"Content-Length: 0\r\n" +
		"\r\n"

	require.Equal(t, expected, res.String())
}

func TestWriteHeaders(t *testing.T) {
	tests := []struct {
		name     string
		header   textproto.MIMEHeader
		expected string
	}{
		{
			name: "nil headers",
		},
		{
			name:   "empty headers",
			header: textproto.MIMEHeader{},
		},
		{
			name: "without CSeq",
			header: textproto.MIMEHeader{
				"User-Agent": {"go2rtc"},
				"Accept":     {"application/sdp"},
			},
			expected: "User-Agent: go2rtc\r\n" +
				"Accept: application/sdp\r\n",
		},
		{
			name: "empty values",
			header: textproto.MIMEHeader{
				"CSeq":           nil,
				"Accept":         {},
				"User-Agent":     {"go2rtc"},
				"Content-Length": nil,
			},
			expected: "User-Agent: go2rtc\r\n",
		},
		{
			name: "multiple values",
			header: textproto.MIMEHeader{
				"CSeq":   {"1"},
				"Accept": {"application/sdp", "application/json"},
			},
			expected: "CSeq: 1\r\n" +
				"Accept: application/sdp\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s strings.Builder
			writeHeaders(&s, tt.header)
			require.Equal(t, tt.expected, s.String())
		})
	}
}

func TestWriteHeadersOrder(t *testing.T) {
	header := textproto.MIMEHeader{
		"CSeq":             {"1"},
		"Authorization":    {"Basic dGVzdA=="},
		"User-Agent":       {"go2rtc"},
		"Accept":           {"application/sdp"},
		"Content-Type":     {"application/sdp"},
		"Range":            {"npt=0.000-"},
		"Transport":        {"RTP/AVP/TCP"},
		"Require":          {"www.onvif.org/ver20/backchannel"},
		"Session":          {"12345"},
		"Server":           {"go2rtc"},
		"Public":           {"OPTIONS, DESCRIBE"},
		"Www-Authenticate": {`Basic realm="go2rtc"`},
		"Content-Length":   {"0"},
	}

	expected := "CSeq: 1\r\n" +
		"Authorization: Basic dGVzdA==\r\n" +
		"User-Agent: go2rtc\r\n" +
		"Accept: application/sdp\r\n" +
		"Content-Type: application/sdp\r\n" +
		"Range: npt=0.000-\r\n" +
		"Transport: RTP/AVP/TCP\r\n" +
		"Require: www.onvif.org/ver20/backchannel\r\n" +
		"Session: 12345\r\n" +
		"Server: go2rtc\r\n" +
		"Public: OPTIONS, DESCRIBE\r\n" +
		"Www-Authenticate: Basic realm=\"go2rtc\"\r\n" +
		"Content-Length: 0\r\n"

	var s strings.Builder
	writeHeaders(&s, header)
	require.Equal(t, expected, s.String())
}

func TestStringBody(t *testing.T) {
	u, err := url.Parse("rtsp://example.com/stream")
	require.NoError(t, err)

	header := textproto.MIMEHeader{
		"CSeq":           {"1"},
		"Content-Length": {"4"},
	}

	req := &Request{
		Method: "SET_PARAMETER",
		URL:    u,
		Proto:  "RTSP/1.0",
		Header: header,
		Body:   []byte("test"),
	}

	expected := "SET_PARAMETER rtsp://example.com/stream RTSP/1.0\r\n" +
		"CSeq: 1\r\n" +
		"Content-Length: 4\r\n" +
		"\r\n" +
		"test"

	require.Equal(t, expected, req.String())

	res := &Response{
		Proto:  "RTSP/1.0",
		Status: "200 OK",
		Header: header,
		Body:   []byte("test"),
	}

	expected = "RTSP/1.0 200 OK\r\n" +
		"CSeq: 1\r\n" +
		"Content-Length: 4\r\n" +
		"\r\n" +
		"test"

	require.Equal(t, expected, res.String())
}
