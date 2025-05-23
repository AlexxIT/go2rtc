package app

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/yaml"
)

var secrets = make(map[string]*Secret)
var secretsMu sync.Mutex

var templateRegex = regexp.MustCompile(`\{\{\s*([^\}]+)\s*\}\}`)

type Secrets interface {
	Get(key string) any
	Set(key string, value any)
	Parse(template string) string
	Marshal(v any) ([]byte, error)
	Unmarshal(v any) error
	Save() error
}

type Secret struct {
	Secrets

	Name   string
	Values map[string]any
}

func NewSecret(name string, values interface{}) *Secret {
	secretsMu.Lock()
	defer secretsMu.Unlock()

	if s, exists := secrets[name]; exists {
		return s
	}

	s := &Secret{Name: name, Values: make(map[string]any)}

	switch v := values.(type) {
	case map[string]any:
		s.Values = v
	default:
		data, err := yaml.Encode(values, 2)
		if err == nil {
			var mapValues map[string]any
			if err := yaml.Unmarshal(data, &mapValues); err == nil {
				s.Values = mapValues
			}
		}
	}

	secrets[name] = s
	return s
}

func GetSecret(name string) *Secret {
	return secrets[name]
}

func (s *Secret) Get(key string) any {
	secretsMu.Lock()
	defer secretsMu.Unlock()

	return s.Values[key]
}

func (s *Secret) Set(key string, value any) {
	secretsMu.Lock()
	defer secretsMu.Unlock()

	if s.Values == nil {
		s.Values = make(map[string]any)
	}

	s.Values[key] = value
	secrets[s.Name] = s
}

func (s *Secret) Parse(template string) string {
	if !templateRegex.MatchString(template) {
		return template
	}

	secretsMu.Lock()
	defer secretsMu.Unlock()

	if _, exists := secrets[s.Name]; !exists {
		return template
	}

	result := templateRegex.ReplaceAllStringFunc(template, func(match string) string {
		varName := strings.TrimSpace(templateRegex.FindStringSubmatch(match)[1])
		pathParts := strings.Split(varName, ".")
		value := getNestedValue(s.Values, pathParts)

		if value != nil {
			return stringify(value)
		}

		return ""
	})

	return result
}

func (s *Secret) Marshal(v any) ([]byte, error) {
	secretsMu.Lock()
	defer secretsMu.Unlock()

	if s.Values == nil {
		return nil, fmt.Errorf("no values in secret %s", s.Name)
	}

	data, err := yaml.Encode(s.Values, 2)
	if err != nil {
		return nil, fmt.Errorf("error encoding secret values: %w", err)
	}

	return data, nil
}

func (s *Secret) Unmarshal(v any) error {
	secretsMu.Lock()
	defer secretsMu.Unlock()

	if s.Values == nil {
		return fmt.Errorf("no values in secret %s", s.Name)
	}

	data, err := yaml.Encode(s.Values, 2)
	if err != nil {
		return fmt.Errorf("error encoding secret values: %w", err)
	}

	if err := yaml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("error unmarshaling secret values: %w", err)
	}

	return nil
}

func (s *Secret) Save() error {
	secretsMu.Lock()
	defer secretsMu.Unlock()
	return saveSecret(s.Name, s.Values)
}

func initSecrets() {
	var cfg struct {
		Secrets map[string]map[string]any `yaml:"secrets"`
	}

	/*
		Example config:
			secrets:
				test_camera:
					username: test
					password: test
	*/

	LoadConfig(&cfg)

	if cfg.Secrets == nil {
		return
	}

	secretsMu.Lock()
	defer secretsMu.Unlock()

	for name, values := range cfg.Secrets {
		secrets[name] = &Secret{Name: name, Values: values}
	}
}

func saveSecret(name string, secret map[string]any) error {
	return PatchConfig([]string{"secrets", name}, secret)
}

func getNestedValue(m map[string]any, path []string) interface{} {
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

	// Check nested maps
	switch nextMap := value.(type) {
	case map[string]any:
		return getNestedValue(nextMap, path[1:])
	case map[interface{}]interface{}:
		stringMap := make(map[string]any)
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
