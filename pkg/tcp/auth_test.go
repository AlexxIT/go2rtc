package tcp

import (
	"fmt"
	"net/textproto"
	"net/url"
	"strings"
	"testing"
)

const (
	benchUser     = "bench"
	benchPass     = "correct-horse"
	benchMD5Realm = "go2rtc-bench-md5"
	benchSHARealm = "go2rtc-bench-sha256"
	benchMD5Nonce = "11111111111111111111111111111111"
	benchSHANonce = "22222222222222222222222222222222"
	benchURI      = "rtsp://host.docker.internal:18554/stream"
	benchMD5Resp  = "228bc516fa4f435af2972451f91111fa"
	benchSHAResp  = "49ca071b729baa998e6fe57a2d25ba5952a40fe6f47e2044be367ae6b464b859"
)

func TestHexHashMD5BenchResponse(t *testing.T) {
	ha1 := hexHash("MD5", benchUser, benchMD5Realm, benchPass)
	ha2 := hexHash("MD5", "DESCRIBE", benchURI)
	got := hexHash("MD5", ha1, benchMD5Nonce, ha2)
	if got != benchMD5Resp {
		t.Fatalf("MD5 response = %s, want %s", got, benchMD5Resp)
	}
}

func TestHexHashSHA256BenchResponse(t *testing.T) {
	ha1 := hexHash("SHA-256", benchUser, benchSHARealm, benchPass)
	ha2 := hexHash("SHA-256", "DESCRIBE", benchURI)
	got := hexHash("SHA-256", ha1, benchSHANonce, ha2)
	if got != benchSHAResp {
		t.Fatalf("SHA-256 response = %s, want %s", got, benchSHAResp)
	}
}

func TestAlgorithmRank(t *testing.T) {
	cases := []struct {
		token string
		rank  int
	}{
		{"", 1},
		{"MD5", 1},
		{"md5", 1},
		{"MD5-sess", 2},
		{"md5-SESS", 2},
		{"SHA-256", 3},
		{"sha-256", 3},
		{"SHA-256-sess", 4},
		{"sha-256-SESS", 4},
		{"SHA-512-256", 0},
		{"unknown", 0},
	}
	for _, tc := range cases {
		if got := algorithmRank(tc.token); got != tc.rank {
			t.Errorf("algorithmRank(%q) = %d, want %d", tc.token, got, tc.rank)
		}
	}
	if !(algorithmRank("SHA-256-sess") > algorithmRank("SHA-256") &&
		algorithmRank("SHA-256") > algorithmRank("MD5-sess") &&
		algorithmRank("MD5-sess") > algorithmRank("MD5") &&
		algorithmRank("MD5") > algorithmRank("SHA-512-256")) {
		t.Fatal("algorithmRank does not order known tokens above unimplemented ones")
	}
}

func TestReadSelectsStrongestDigest(t *testing.T) {
	md5Chal := fmt.Sprintf(
		`Digest realm="%s", nonce="%s", algorithm=MD5`,
		benchMD5Realm, benchMD5Nonce,
	)
	shaChal := fmt.Sprintf(
		`Digest realm="%s", nonce="%s", algorithm=SHA-256`,
		benchSHARealm, benchSHANonce,
	)
	orders := [][]string{
		{md5Chal, shaChal},
		{shaChal, md5Chal},
	}
	for _, values := range orders {
		a := NewAuth(url.UserPassword(benchUser, benchPass))
		res := &Response{Header: textproto.MIMEHeader{
			"Www-Authenticate": values,
		}}
		if !a.Read(res) {
			t.Fatalf("Read failed for order %q", values)
		}
		if a.Method != AuthDigest {
			t.Fatalf("Method = %d, want AuthDigest", a.Method)
		}
		if a.algorithm != "SHA-256" || a.realm != benchSHARealm || a.nonce != benchSHANonce {
			t.Fatalf("selected algorithm=%q realm=%q nonce=%q, want SHA-256/%s/%s (order %q)",
				a.algorithm, a.realm, a.nonce, benchSHARealm, benchSHANonce, values)
		}
		reqURL, err := url.Parse(benchURI)
		if err != nil {
			t.Fatal(err)
		}
		req := &Request{Method: "DESCRIBE", URL: reqURL, Header: textproto.MIMEHeader{}}
		a.Write(req)
		auth := req.Header.Get("Authorization")
		want := fmt.Sprintf(`response="%s"`, benchSHAResp)
		if !strings.Contains(auth, want) {
			t.Fatalf("Authorization = %q, want SHA-256 response %s", auth, benchSHAResp)
		}
	}
}

func TestWriteImplicitMD5MatchesLegacyHeader(t *testing.T) {
	user, pass := "user", "pass"
	realm := "testrealm"
	nonce := "dcd98b7102dd2f0e8b11d0f600bfb0c093"
	uri := "rtsp://cam.example/stream"

	a := NewAuth(url.UserPassword(user, pass))
	res := &Response{Header: textproto.MIMEHeader{
		"Www-Authenticate": []string{
			fmt.Sprintf(`Digest realm="%s", nonce="%s"`, realm, nonce),
		},
	}}
	if !a.Read(res) {
		t.Fatal("Read failed")
	}
	if a.algorithm != "" || a.qop != "" || a.opaque != "" {
		t.Fatalf("implicit MD5 fields: algorithm=%q qop=%q opaque=%q", a.algorithm, a.qop, a.opaque)
	}

	reqURL, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	req := &Request{Method: "DESCRIBE", URL: reqURL, Header: textproto.MIMEHeader{}}
	a.Write(req)

	h2 := HexMD5("DESCRIBE", uri)
	response := HexMD5(HexMD5(user, realm, pass)+":"+nonce, h2)
	want := fmt.Sprintf(
		`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
		user, realm, nonce, uri, response,
	)
	got := req.Header.Get("Authorization")
	if got != want {
		t.Fatalf("legacy header mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestWriteBenchVectors(t *testing.T) {
	cases := []struct {
		name, challenge, want string
	}{
		{
			name:      "md5",
			challenge: fmt.Sprintf(`Digest realm="%s", nonce="%s", algorithm=MD5`, benchMD5Realm, benchMD5Nonce),
			want:      benchMD5Resp,
		},
		{
			name:      "sha256",
			challenge: fmt.Sprintf(`Digest realm="%s", nonce="%s", algorithm=SHA-256`, benchSHARealm, benchSHANonce),
			want:      benchSHAResp,
		},
	}
	reqURL, err := url.Parse(benchURI)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAuth(url.UserPassword(benchUser, benchPass))
			res := &Response{Header: textproto.MIMEHeader{
				"Www-Authenticate": []string{tc.challenge},
			}}
			if !a.Read(res) {
				t.Fatal("Read failed")
			}
			req := &Request{Method: "DESCRIBE", URL: reqURL, Header: textproto.MIMEHeader{}}
			a.Write(req)
			auth := req.Header.Get("Authorization")
			if !strings.Contains(auth, fmt.Sprintf(`response="%s"`, tc.want)) {
				t.Fatalf("Authorization = %q, want response %s", auth, tc.want)
			}
		})
	}
}
