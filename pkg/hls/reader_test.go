package hls

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetSegment(t *testing.T) {
	const playlist = "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2.000,\n1.ts?session=abc\n#EXTINF:2.000,\n2.ts?session=abc\n"

	require.Equal(t, "1.ts?session=abc", string(getSegment([]byte(playlist))))

	// some servers (e.g. Blue Iris) end playlist lines with CRLF
	crlf := strings.ReplaceAll(playlist, "\n", "\r\n")
	require.Equal(t, "1.ts?session=abc", string(getSegment([]byte(crlf))))
}

func TestReaderCRLFPlaylist(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/temp.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "#EXTM3U\r\n#EXT-X-TARGETDURATION:2\r\n#EXTINF:2.000,\r\n1.ts?session=abc\r\n")
	})
	mux.HandleFunc("/1.ts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "segment1")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	u, err := url.Parse(srv.URL + "/temp.m3u8")
	require.NoError(t, err)

	rd, err := NewReader(u, io.NopCloser(strings.NewReader("#EXTM3U\r\n")))
	require.NoError(t, err)

	b := make([]byte, 64)
	n, err := rd.Read(b)
	require.NoError(t, err)
	require.Equal(t, "segment1", string(b[:n]))
}
