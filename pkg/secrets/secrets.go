package secrets

import (
	"errors"
	"sync"
)

type SecretsManager interface {
	NewSecret(name string, defaultValues interface{}) (Secret, error)
	GetSecret(name string) Secret
}

type Secret interface {
	Get(key string) any
	Set(key string, value string)
	Marshal() (interface{}, error)
	Unmarshal(value any) error
	Save() error
}

var manager SecretsManager
var once sync.Once

func SetManager(m SecretsManager) {
	once.Do(func() {
		manager = m
	})
}

// NewSecret creates or retrieves a secret
func NewSecret(name string, defaultValues interface{}) (Secret, error) {
	if manager == nil {
		return nil, errors.New("secrets manager not initialized")
	}
	return manager.NewSecret(name, defaultValues)
}

// GetSecret retrieves an existing secret
func GetSecret(name string) Secret {
	if manager == nil {
		return nil
	}
	return manager.GetSecret(name)
}
