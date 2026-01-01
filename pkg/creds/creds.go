package creds

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Storage interface {
	SetValue(name, value string) error
	GetValue(name string) (string, bool)
}

var storage Storage

func SetStorage(s Storage) {
	storage = s
}

func SetValue(name, value string) error {
	if storage == nil {
		return errors.New("credentials: storage not initialized")
	}
	if err := storage.SetValue(name, value); err != nil {
		return err
	}
	AddSecret(value)
	return nil
}

func GetValue(name string) (value string, ok bool) {
	value, ok = getValue(name)
	AddSecret(value)
	return
}

func getValue(name string) (string, bool) {
	if storage != nil {
		if value, ok := storage.GetValue(name); ok {
			return value, true
		}
	}

	if dir, ok := os.LookupEnv("CREDENTIALS_DIRECTORY"); ok {
		if value, _ := os.ReadFile(filepath.Join(dir, name)); value != nil {
			return strings.TrimSpace(string(value)), true
		}
	}

	return os.LookupEnv(name)
}

// ReplaceVars - support format ${CAMERA_PASSWORD} and ${RTSP_USER:admin}
func ReplaceVars(data []byte) []byte {
	re := regexp.MustCompile(`\${([^}{]+)}`)
	return re.ReplaceAllFunc(data, func(match []byte) []byte {
		key := string(match[2 : len(match)-1])

		var def string
		var defok bool

		if i := strings.IndexByte(key, ':'); i > 0 {
			key, def = key[:i], key[i+1:]
			defok = true
		}

		if value, ok := GetValue(key); ok {
			return []byte(value)
		}

		if defok {
			return []byte(def)
		}

		return match
	})
}
