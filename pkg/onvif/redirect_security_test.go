package onvif

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

// A redirected ONVIF SOAP request contains a WS-Security envelope.
// A camera response must not send that envelope to another destination.
func TestRequestRejectsSOAPRedirects(t *testing.T) {
	for _, status := range []int{
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var destinationRequests atomic.Int32
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				destinationRequests.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer destination.Close()

			var sourceRequests atomic.Int32
			camera := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sourceRequests.Add(1)
				if r.Method != http.MethodPost {
					t.Errorf("unexpected method: %s", r.Method)
				}
				soap, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("cannot read SOAP envelope: %v", err)
				}
				if !bytes.Contains(soap, []byte("<wsse:Username>testuser</wsse:Username>")) {
					t.Error("SOAP credentials not present in the initial test request")
				}
				w.Header().Set("Location", destination.URL+"/unexpected")
				w.WriteHeader(status)
			}))
			defer camera.Close()

			c := &Client{url: &url.URL{User: url.UserPassword("testuser", "testpass")}}
			if _, err := c.Request(camera.URL, "<trt:GetProfiles/>"); err == nil {
				t.Fatal("redirect unexpectedly accepted")
			}
			if sourceRequests.Load() != 1 || destinationRequests.Load() != 0 {
				t.Fatalf("source requests=%d, redirect destination requests=%d",
					sourceRequests.Load(), destinationRequests.Load())
			}
		})
	}
}

func TestRequestPreservesSuccessfulSOAPResponse(t *testing.T) {
	camera := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/soap+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<Envelope><ok/></Envelope>"))
	}))
	defer camera.Close()
	c := &Client{url: &url.URL{}}
	body, err := c.Request(camera.URL, "<trt:GetProfiles/>")
	if err != nil || string(body) != "<Envelope><ok/></Envelope>" {
		t.Fatalf("successful SOAP response changed: body=%q error=%v", body, err)
	}
}
