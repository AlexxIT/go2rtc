package store

import (
	"encoding/json"
	"github.com/rs/zerolog/log"
	"os"
)

const name = "go2rtc.json"

var store map[string]any

func load() {
	data, _ := os.ReadFile(name)
	if data != nil {
		if err := json.Unmarshal(data, &store); err != nil {
			// TODO: log
			log.Warn().Err(err).Msg("[app] read storage")
		}
	}

	if store == nil {
		store = make(map[string]any)
	}
}

func save() error {
	data, err := json.Marshal(store)
	if err != nil {
		return err
	}

	return os.WriteFile(name, data, 0644)
}

func GetRaw(key string) any {
	if store == nil {
		load()
	}

	return store[key]
}

func GetDict(key string) map[string]any {
	raw := GetRaw(key)
	if raw != nil {
		return raw.(map[string]any)
	}

	return make(map[string]any)
}

func Set(key string, v any) error {
	if store == nil {
		load()
	}

	store[key] = v

	return save()
}
