package api

import (
	"github.com/AlexxIT/go2rtc/www"
	"net/http"
)

func initStatic(staticDir string) {
	var root http.FileSystem
	if staticDir != "" {
		log.Info().Str("dir", staticDir).Msg("[api] serve static")
		root = http.Dir(staticDir)
	} else {
		root = http.FS(www.Static)
	}

	fileServer := http.FileServer(root)

	HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if basePath != "" {
			r.URL.Path = r.URL.Path[len(basePath):]
		}

		fileServer.ServeHTTP(w, r)
	})
}
