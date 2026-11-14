package tcp

import (
	"bufio"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContentLength(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		body    string
		want    string
		wantErr bool
	}{
		{name: "valid", value: "4", body: "abcd", want: "abcd"},
		{name: "zero", value: "0", want: ""},
		{name: "negative", value: "-1", wantErr: true},
		{name: "not a number", value: "abc", wantErr: true},
		{name: "too large", value: strconv.Itoa(maxContentLength + 1), wantErr: true},
		{name: "overflow", value: "99999999999999999999999", wantErr: true},
	}

	for _, tt := range tests {
		t.Run("response/"+tt.name, func(t *testing.T) {
			msg := "RTSP/1.0 200 OK\r\nCSeq: 1\r\nContent-Length: " + tt.value + "\r\n\r\n" + tt.body
			res, err := ReadResponse(bufio.NewReader(strings.NewReader(msg)))
			if tt.wantErr {
				require.ErrorIs(t, err, errContentLength)
				require.Nil(t, res)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, string(res.Body))
		})
		t.Run("request/"+tt.name, func(t *testing.T) {
			msg := "ANNOUNCE rtsp://example.invalid/x RTSP/1.0\r\nCSeq: 1\r\nContent-Length: " + tt.value + "\r\n\r\n" + tt.body
			req, err := ReadRequest(bufio.NewReader(strings.NewReader(msg)))
			if tt.wantErr {
				require.ErrorIs(t, err, errContentLength)
				require.Nil(t, req)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, string(req.Body))
		})
	}
}
