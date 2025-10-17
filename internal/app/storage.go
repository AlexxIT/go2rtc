package app

import (
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/creds"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
)

func initStorage() {
	storage = &envStorage{data: make(map[string]string)}
	creds.SetStorage(storage)
}

func loadEnv(data []byte) {
	var cfg struct {
		Env map[string]string `yaml:"env"`
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return
	}

	storage.mu.Lock()
	for name, value := range cfg.Env {
		storage.data[name] = value
		creds.AddSecret(value)
	}
	storage.mu.Unlock()
}

var storage *envStorage

type envStorage struct {
	data map[string]string
	mu   sync.Mutex
}

func (s *envStorage) SetValue(name, value string) error {
	if err := PatchConfig([]string{"env", name}, value); err != nil {
		return err
	}

	s.mu.Lock()
	s.data[name] = value
	s.mu.Unlock()

	return nil
}

func (s *envStorage) GetValue(name string) (value string, ok bool) {
	s.mu.Lock()
	value, ok = s.data[name]
	s.mu.Unlock()
	return
}
