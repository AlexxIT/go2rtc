package log

import (
	"bytes"
	"io"
	"log"
	"net/http"
)

func Debug(v any) {
	switch v := v.(type) {
	case *http.Request:
		if v == nil {
			return
		}
		if v.ContentLength != 0 {
			b, err := io.ReadAll(v.Body)
			if err != nil {
				panic(err)
			}
			v.Body = io.NopCloser(bytes.NewReader(b))
			log.Printf("[homekit] request: %s %s\n%s", v.Method, v.RequestURI, b)
		} else {
			log.Printf("[homekit] request: %s %s <nobody>", v.Method, v.RequestURI)
		}
	case *http.Response:
		if v == nil {
			return
		}
		if v.Header.Get("Content-Type") == "image/jpeg" {
			log.Printf("[homekit] response: %d <jpeg>", v.StatusCode)
			return
		}
		if v.ContentLength != 0 {
			b, err := io.ReadAll(v.Body)
			if err != nil {
				panic(err)
			}
			v.Body = io.NopCloser(bytes.NewReader(b))
			log.Printf("[homekit] response: %s %d\n%s", v.Proto, v.StatusCode, b)
		} else {
			log.Printf("[homekit] response: %s %d <nobody>", v.Proto, v.StatusCode)
		}
	}
}
