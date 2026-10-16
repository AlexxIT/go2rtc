package xiaomi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// rewriteTransport sends the requests for account.xiaomi.com to a test server.
type rewriteTransport struct{ host string }

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "account.xiaomi.com" {
		return http.DefaultTransport.RoundTrip(req)
	}
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = "http", rt.host
	return http.DefaultTransport.RoundTrip(req)
}

func newTestCloud(t *testing.T, handler http.Handler) *Cloud {
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := NewCloud("xiaomiio")
	c.client.Transport = rewriteTransport{host: srv.Listener.Addr().String()}
	return c
}

// Simulates Xiaomi stopping on a /fe/ page after phone verification; the skipUrl
// endpoint sets the pass token, then redirects to the STS endpoint.
func TestFinishVerifyFollowsSkipURL(t *testing.T) {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/location", func(w http.ResponseWriter, r *http.Request) {
		skip := url.QueryEscape(srv.URL + "/skip?x=1")
		http.Redirect(w, r, "/fe/service/identity/verifyPhone?skipUrl="+skip, http.StatusFound)
	})
	mux.HandleFunc("/fe/service/identity/verifyPhone", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>confirm phone</html>"))
	})
	mux.HandleFunc("/skip", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "passToken", Value: "V1:pt"})
		http.SetCookie(w, &http.Cookie{Name: "userId", Value: "42"})
		w.Header().Set("Extension-Pragma", `{"ssecurity":"c2VjcmV0"}`)
		http.Redirect(w, r, "/sts", http.StatusFound)
	})
	mux.HandleFunc("/sts", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "serviceToken", Value: "st"})
		http.SetCookie(w, &http.Cookie{Name: "cUserId", Value: "c42"})
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	c := NewCloud("xiaomiio")
	if err := c.finishVerify(srv.URL + "/location"); err != nil {
		t.Fatal(err)
	}
	if c.userID != "42" || c.passToken != "V1:pt" || string(c.ssecurity) != "secret" {
		t.Fatalf("user=%q pass=%q ssecurity=%q", c.userID, c.passToken, c.ssecurity)
	}
	if !strings.Contains(c.cookies, "serviceToken=st") || !strings.Contains(c.cookies, "cUserId=c42") {
		t.Fatalf("cookies=%q", c.cookies)
	}
}

// Without any pass token the login must fail loudly instead of saving an empty account.
func TestFinishVerifyFailsWithoutPassToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("nothing"))
	}))
	defer srv.Close()

	c := NewCloud("xiaomiio")
	err := c.finishVerify(srv.URL + "/fe/x")
	if err == nil || !strings.Contains(err.Error(), "stopped at /fe/x") {
		t.Fatalf("err=%v", err)
	}
}

// Xiaomi may give skipUrl without a host.
func TestFinishVerifyFollowsRelativeSkipURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/location", func(w http.ResponseWriter, r *http.Request) {
		skip := url.QueryEscape("/skip?x=1")
		http.Redirect(w, r, "/fe/service/identity/verifyPhone?skipUrl="+skip, http.StatusFound)
	})
	mux.HandleFunc("/fe/service/identity/verifyPhone", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>confirm phone</html>"))
	})
	mux.HandleFunc("/skip", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "passToken", Value: "V1:pt"})
		http.SetCookie(w, &http.Cookie{Name: "userId", Value: "42"})
		w.Header().Set("Extension-Pragma", `{"ssecurity":"c2VjcmV0"}`)
		http.Redirect(w, r, "/sts", http.StatusFound)
	})
	mux.HandleFunc("/sts", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "serviceToken", Value: "st"})
		http.SetCookie(w, &http.Cookie{Name: "cUserId", Value: "c42"})
	})

	c := newTestCloud(t, mux)
	if err := c.finishVerify("https://account.xiaomi.com/location"); err != nil {
		t.Fatal(err)
	}
	if c.userID != "42" || c.passToken != "V1:pt" || !strings.Contains(c.cookies, "serviceToken=st") {
		t.Fatalf("user=%q pass=%q cookies=%q", c.userID, c.passToken, c.cookies)
	}
}

// The skipUrl request has to carry the cookies set earlier in the flow.
func TestFinishVerifyKeepsCookies(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/location", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc", Path: "/"})
		skip := url.QueryEscape("https://account.xiaomi.com/skip")
		http.Redirect(w, r, "/fe/service/identity/verifyPhone?skipUrl="+skip, http.StatusFound)
	})
	mux.HandleFunc("/fe/service/identity/verifyPhone", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>confirm phone</html>"))
	})
	mux.HandleFunc("/skip", func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("session"); err != nil {
			http.Error(w, "no session", http.StatusForbidden)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "passToken", Value: "V1:pt"})
		http.SetCookie(w, &http.Cookie{Name: "userId", Value: "42"})
		http.Redirect(w, r, "/sts", http.StatusFound)
	})
	mux.HandleFunc("/sts", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "serviceToken", Value: "st"})
	})

	c := newTestCloud(t, mux)
	if err := c.finishVerify("https://account.xiaomi.com/location"); err != nil {
		t.Fatal(err)
	}
	if c.userID != "42" || c.passToken != "V1:pt" || !strings.Contains(c.cookies, "serviceToken=st") {
		t.Fatalf("user=%q pass=%q cookies=%q", c.userID, c.passToken, c.cookies)
	}
}

// The pass token alone is enough: without a service token the login is repeated with it.
func TestFinishVerifyLogsInWithPassToken(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/location", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "passToken", Value: "V1:pt", Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "userId", Value: "42", Path: "/"})
		http.Redirect(w, r, "/fe/service/identity/verifyPhone", http.StatusFound)
	})
	mux.HandleFunc("/fe/service/identity/verifyPhone", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>confirm phone</html>"))
	})
	mux.HandleFunc("/pass/serviceLogin", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("passToken"); err != nil || c.Value != "V1:pt" {
			_, _ = w.Write([]byte(`&&&START&&&{"code":70016}`))
			return
		}
		_, _ = w.Write([]byte(`&&&START&&&{"ssecurity":"c2VjcmV0","location":"https://account.xiaomi.com/sts"}`))
	})
	mux.HandleFunc("/sts", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "serviceToken", Value: "st"})
		http.SetCookie(w, &http.Cookie{Name: "cUserId", Value: "c42"})
	})

	c := newTestCloud(t, mux)
	if err := c.finishVerify("https://account.xiaomi.com/location"); err != nil {
		t.Fatal(err)
	}
	if c.userID != "42" || c.passToken != "V1:pt" || string(c.ssecurity) != "secret" {
		t.Fatalf("user=%q pass=%q ssecurity=%q", c.userID, c.passToken, c.ssecurity)
	}
	if !strings.Contains(c.cookies, "serviceToken=st") {
		t.Fatalf("cookies=%q", c.cookies)
	}
}

// The skipUrl is followed once, a page that sends us back to a /fe/ page cannot loop.
func TestFinishVerifyFollowsSkipURLOnce(t *testing.T) {
	var hits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/location", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/fe/service/identity/verifyPhone?skipUrl="+url.QueryEscape("/skip"), http.StatusFound)
	})
	mux.HandleFunc("/fe/service/identity/verifyPhone", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>confirm phone</html>"))
	})
	mux.HandleFunc("/skip", func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) > 3 {
			http.Error(w, "loop", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/fe/service/identity/verifyPhone?skipUrl="+url.QueryEscape("/skip"), http.StatusFound)
	})

	c := newTestCloud(t, mux)
	err := c.finishVerify("https://account.xiaomi.com/location")
	if err == nil || !strings.Contains(err.Error(), "stopped at /fe/service/identity/verifyPhone -> /fe/service/identity/verifyPhone") {
		t.Fatalf("err=%v", err)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("skipUrl requested %d times", n)
	}
}
