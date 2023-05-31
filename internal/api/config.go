package api

import (
	"github.com/AlexxIT/go2rtc/internal/app"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"os"
)

func configHandler(w http.ResponseWriter, r *http.Request) {
	if app.ConfigPath == "" {
		http.Error(w, "", http.StatusGone)
		return
	}

	switch r.Method {
	case "GET":
		data, err := os.ReadFile(app.ConfigPath)
		if err != nil {
			http.Error(w, "", http.StatusNotFound)
			return
		}
		// https://www.ietf.org/archive/id/draft-ietf-httpapi-yaml-mediatypes-00.html
		Response(w, data, "application/yaml")

	case "POST", "PATCH":
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if r.Method == "PATCH" {
			// no need to validate after merge
			data, err = mergeYAML(app.ConfigPath, data)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		} else {
			// validate config
			var tmp struct{}
			if err = yaml.Unmarshal(data, &tmp); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		if err = os.WriteFile(app.ConfigPath, data, 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func mergeYAML(file1 string, yaml2 []byte) ([]byte, error) {
	// Read the contents of the first YAML file
	data1, err := os.ReadFile(file1)
	if err != nil {
		return nil, err
	}

	// Unmarshal the first YAML file into a map
	var config1 map[string]any
	if err = yaml.Unmarshal(data1, &config1); err != nil {
		return nil, err
	}

	// Unmarshal the second YAML document into a map
	var config2 map[string]any
	if err = yaml.Unmarshal(yaml2, &config2); err != nil {
		return nil, err
	}

	// Merge the two maps
	config1 = merge(config1, config2)

	// Marshal the merged map into YAML
	return yaml.Marshal(&config1)
}

func merge(dst, src map[string]any) map[string]any {
	for k, v := range src {
		if vv, ok := dst[k]; ok {
			switch vv := vv.(type) {
			case map[string]any:
				v := v.(map[string]any)
				dst[k] = merge(vv, v)
			case []any:
				v := v.([]any)
				dst[k] = v
			default:
				dst[k] = v
			}
		} else {
			dst[k] = v
		}
	}
	return dst
}
