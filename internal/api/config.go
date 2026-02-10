package api

import (
	"errors"
	"io"
	"maps"
	"net/http"
	"os"
	"slices"

	"github.com/AlexxIT/go2rtc/internal/app"
	pkgyaml "github.com/AlexxIT/go2rtc/pkg/yaml"
	"gopkg.in/yaml.v3"
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
			if err = yaml.Unmarshal(data, map[string]any{}); err != nil {
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
	data1, err := os.ReadFile(file1)
	if err != nil {
		return nil, err
	}

	var patch map[string]any
	if err = yaml.Unmarshal(yaml2, &patch); err != nil {
		return nil, err
	}

	data1, err = mergeYAMLMap(data1, nil, patch)
	if err != nil {
		return nil, err
	}

	// validate config after merge
	if err = yaml.Unmarshal(data1, map[string]any{}); err != nil {
		return nil, err
	}

	return data1, nil
}

// mergeYAMLMap recursively applies patch values onto config bytes.
func mergeYAMLMap(data []byte, path []string, patch map[string]any) ([]byte, error) {
	for _, key := range slices.Sorted(maps.Keys(patch)) {
		value := patch[key]
		currPath := append(append([]string(nil), path...), key)

		if valueMap, ok := value.(map[string]any); ok {
			isMap, exists, err := pathIsMapping(data, currPath)
			if err != nil {
				return nil, err
			}

			if exists && isMap {
				data, err = mergeYAMLMap(data, currPath, valueMap)
			} else {
				data, err = pkgyaml.Patch(data, currPath, valueMap)
			}
			if err != nil {
				return nil, err
			}
			continue
		}

		var err error
		data, err = pkgyaml.Patch(data, currPath, value)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

// pathIsMapping reports whether path exists and ends with a mapping node.
func pathIsMapping(data []byte, path []string) (isMap, exists bool, err error) {
	var root yaml.Node
	if err = yaml.Unmarshal(data, &root); err != nil {
		return false, false, err
	}

	if len(root.Content) == 0 {
		return false, false, nil
	}

	if len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return false, false, errors.New("yaml: expected mapping document")
	}

	node := root.Content[0]
	for i, part := range path {
		idx := -1
		for j := 0; j < len(node.Content); j += 2 {
			if node.Content[j].Value == part {
				idx = j
				break
			}
		}
		if idx < 0 {
			return false, false, nil
		}

		valueNode := node.Content[idx+1]
		if i == len(path)-1 {
			return valueNode.Kind == yaml.MappingNode, true, nil
		}

		if valueNode.Kind != yaml.MappingNode {
			return false, false, nil
		}

		node = valueNode
	}

	return false, false, nil
}
