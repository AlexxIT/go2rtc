package app

import (
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/secrets"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
)

var (
	secretsMap = make(map[string]*Secret)
	secretsMu  sync.Mutex
)

// SecretsManager implements secrets.SecretsManager interface
type SecretsManager struct{}

func (m *SecretsManager) NewSecret(name string, values interface{}) (secrets.Secret, error) {
	secretsMu.Lock()
	defer secretsMu.Unlock()

	if s, exists := secretsMap[name]; exists {
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

	secretsMap[name] = s

	return s, nil
}

func (m *SecretsManager) GetSecret(name string) secrets.Secret {
	secretsMu.Lock()
	defer secretsMu.Unlock()
	return secretsMap[name]
}

// Secret implements secrets.Secret interface
type Secret struct {
	Name   string
	Values map[string]string
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
	return PatchConfig([]string{"secrets", s.Name}, s.Values)
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
		secretsMap[name] = &Secret{
			Name:   name,
			Values: values,
		}
	}

	// Register
	secrets.SetManager(&SecretsManager{})
}
