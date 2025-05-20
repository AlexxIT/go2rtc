package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/yaml"
)

var secrets [][]byte

var templateRegex = regexp.MustCompile(`\{\{\s*([^\}]+)\s*\}\}`)

func ResolveSecrets(template string) string {
    if !templateRegex.MatchString(template) {
        return template
    }

	var secretsMap map[string]interface{}
	LoadSecret(&secretsMap)

	// ex template: rtsp://{{ my_camera.username }}:{{ my_camera.password }}@192.168.178.1:554/stream
	result := templateRegex.ReplaceAllStringFunc(template, func(match string) string {
		varName := strings.TrimSpace(templateRegex.FindStringSubmatch(match)[1])
		pathParts := strings.Split(varName, ".")
		value := getNestedValue(secretsMap, pathParts)
		
		if value != nil {
			return stringify(value)
		}
		
		return ""
	})
	
	return result
}

func LoadSecret(v any) {
    for _, data := range secrets {
        if err := yaml.Unmarshal(data, v); err != nil {
            Logger.Warn().Err(err).Send()
        }
    }
}

func PatchSecret(path []string, value any) error {
	if SecretPath == "" {
		return errors.New("secret file disabled")
	}

	// empty config is OK
	b, _ := os.ReadFile(SecretPath)

	b, err := yaml.Patch(b, path, value)
	if err != nil {
		return err
	}

	if err := os.WriteFile(SecretPath, b, 0644); err == nil {
		secrets = [][]byte{b}
	}

	return err
}

func initSecret(secret string) {
	if secret == "" {
		secret = "go2rtc.secrets"
	}

	SecretPath = secret

	if SecretPath != "" {
		if !filepath.IsAbs(SecretPath) {
			if cwd, err := os.Getwd(); err == nil {
				SecretPath = filepath.Join(cwd, SecretPath)
			}
		}
		Info["secret_path"] = SecretPath
	}
}

func getNestedValue(m map[string]interface{}, path []string) interface{} {
	if len(path) == 0 || m == nil {
		return nil
	}
	
	key := path[0]
	value, exists := m[key]
	if !exists {
		return nil
	}
	
	if len(path) == 1 {
		return value
	}
	
	// Für verschachtelte Maps
	switch nextMap := value.(type) {
	case map[string]interface{}:
		return getNestedValue(nextMap, path[1:])
	case map[interface{}]interface{}:
		// Konvertiere map[interface{}]interface{} zu map[string]interface{}
		stringMap := make(map[string]interface{})
		for k, v := range nextMap {
			if keyStr, ok := k.(string); ok {
				stringMap[keyStr] = v
			}
		}
		return getNestedValue(stringMap, path[1:])
	default:
		return nil
	}
}

func stringify(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case int, int64, float64, bool:
		return fmt.Sprintf("%v", v)
	default:
		return ""
	}
}