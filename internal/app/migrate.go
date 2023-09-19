package app

import (
	"encoding/json"
	"os"

	"github.com/rs/zerolog/log"
)

func migrateStore() {
	const name = "go2rtc.json"

	data, _ := os.ReadFile(name)
	if data == nil {
		return
	}

	var store struct {
		Streams map[string]string `json:"streams"`
	}

	if err := json.Unmarshal(data, &store); err != nil {
		log.Warn().Err(err).Caller().Send()
		return
	}

	for id, url := range store.Streams {
		if err := PatchConfig(id, url, "streams"); err != nil {
			log.Warn().Err(err).Caller().Send()
			return
		}
	}

	_ = os.Remove(name)
}
