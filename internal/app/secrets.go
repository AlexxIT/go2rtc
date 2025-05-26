package app

import (
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/yaml"
)

var (
	secrets   = make(map[string]*Secret)
	secretsMu sync.Mutex
)

type Secrets interface {
	Get(key string) any
	Set(key string, value any)
	Marshal(v any) ([]byte, error)
	Unmarshal(v any) error
	Save() error
}

type Secret struct {
	Secrets

	Name   string
	Values map[string]string
}

func NewSecret(name string, values interface{}) (*Secret, error) {
	secretsMu.Lock()
	defer secretsMu.Unlock()

	if s, exists := secrets[name]; exists {
		return s, nil
	}

	s := &Secret{Name: name, Values: make(map[string]string)}

	data, err := yaml.Encode(values, 2)
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(data, &s.Values); err != nil {
		return nil, err
	}

	secrets[name] = s

	return s, nil
}

func GetSecret(name string) *Secret {
	secretsMu.Lock()
	defer secretsMu.Unlock()
	return secrets[name]
}

func (s *Secret) Get(key string) any {
	secretsMu.Lock()
	defer secretsMu.Unlock()

	if s.Values == nil {
		return nil
	}

	return s.Values[key]
}

func (s *Secret) Set(key string, value string) {
	secretsMu.Lock()
	defer secretsMu.Unlock()

	if s.Values == nil {
		s.Values = make(map[string]string)
	}

	s.Values[key] = value
}

func (s *Secret) Marshal() (interface{}, error) {
	secretsMu.Lock()
	defer secretsMu.Unlock()

	if s.Values == nil {
		return make(map[string]any), nil
	}

	return s.Values, nil
}

func (s *Secret) Unmarshal(value any) error {
	secretsMu.Lock()
	defer secretsMu.Unlock()

	if s.Values == nil {
		s.Values = make(map[string]string)
	}

	data, err := yaml.Encode(value, 2)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(data, value); err != nil {
		return err
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
		Secrets map[string]map[string]string `yaml:"secrets"`
	}

	LoadConfig(&cfg)

	if cfg.Secrets == nil {
		return
	}

	secretsMu.Lock()
	defer secretsMu.Unlock()

	for name, values := range cfg.Secrets {
		secrets[name] = &Secret{
			Name:   name,
			Values: values,
		}
	}
}

func saveSecret(name string, secretValues map[string]string) error {
	return PatchConfig([]string{"secrets", name}, secretValues)
}
