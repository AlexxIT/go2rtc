package tcp

import (
	"bufio"
	"bytes"
	"net/http"
	"testing"
)

func assert(t *testing.T, one, two interface{}) {
	if one != two {
		t.FailNow()
	}
}

func TestName(t *testing.T) {
	data := []byte(`RTSP/1.0 401 Unauthorized
WWW-Authenticate: Digest realm="testrealm@host.com",
                        nonce="dcd98b7102dd2f0e8b11d0f600bfb0c093",

`)

	buf := bytes.NewBuffer(data)
	r := bufio.NewReader(buf)

	res, err := ReadResponse(r)
	assert(t, err, nil)

	assert(t, res.StatusCode, http.StatusUnauthorized)
}
