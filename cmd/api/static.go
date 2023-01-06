package api

import (
	"html/template"
	"io"
	"net/http"
	"strings"

	"github.com/AlexxIT/go2rtc/www"
)

type TemplateData struct {
	Host     string
	RTSPPort string
}

func initStatic(cfg apiCfg) {
	var root http.FileSystem
	var staticDir = cfg.Mod.StaticDir
	if staticDir != "" {
		log.Info().Str("dir", staticDir).Msg("[api] serve static")
		root = http.Dir(staticDir)
	} else {
		root = http.FS(www.Static)
	}

	base := len(basePath)

	HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if base > 0 {
			r.URL.Path = r.URL.Path[base:]
		}

		if r.URL.Path == "/" {
			r.URL.Path = r.URL.Path + "index.html"
		}

		host := strings.Split(r.Host, ":")[0]

		// Open the file using the root FileSystem
		f, err := root.Open(r.URL.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()

		// Read the file contents
		b, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Parse the template
		t, err := template.New("template").Parse(string(b))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data := TemplateData{
			Host:     host,
			RTSPPort: cfg.RTSP.Listen[1:],
		}

		// Execute the template with the Host header value
		err = t.Execute(w, data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}
