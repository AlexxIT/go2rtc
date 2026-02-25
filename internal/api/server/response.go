package server

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ResponseJSON important always add Content-Type
// so go won't need to call http.DetectContentType
func ResponseJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", MimeJSON)
	_ = json.NewEncoder(w).Encode(v)
}

func ResponsePrettyJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", MimeJSON)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func Response(w http.ResponseWriter, body any, contentType string) {
	w.Header().Set("Content-Type", contentType)

	switch v := body.(type) {
	case []byte:
		_, _ = w.Write(v)
	case string:
		_, _ = w.Write([]byte(v))
	default:
		_, _ = fmt.Fprint(w, body)
	}
}

func ResponseSources(w http.ResponseWriter, sources []*Source) {
	if len(sources) == 0 {
		http.Error(w, "no sources", http.StatusNotFound)
		return
	}

	var response = struct {
		Sources []*Source `json:"sources"`
	}{
		Sources: sources,
	}
	ResponseJSON(w, response)
}

func Error(w http.ResponseWriter, err error) {
	log.Error().Err(err).Caller(1).Send()

	http.Error(w, err.Error(), http.StatusInsufficientStorage)
}
