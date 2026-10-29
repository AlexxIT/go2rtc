package ui

import (
	"net/http"
	"os"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
)

func Init() {
	var cfg struct {
		Mod struct {
			BasePath string `yaml:"base_path"`
		} `yaml:"api"`
		UI struct {
			Dir string `yaml:"dir"`
		} `yaml:"ui"`
	}
	cfg.UI.Dir = "/ui"

	app.LoadConfig(&cfg)

	if cfg.UI.Dir == "" {
		return
	}

	log := app.GetLogger("ui")
	info, err := os.Stat(cfg.UI.Dir)
	if err != nil {
		log.Warn().Str("dir", cfg.UI.Dir).Msg("[ui] failed to stat directory")
		return
	}
	if !info.IsDir() {
		log.Warn().Str("dir", cfg.UI.Dir).Msg("[ui] path is not a directory")
		return
	}

	log.Info().Str("dir", cfg.UI.Dir).Msg("[ui] serving extra UI")

	prefix := cfg.Mod.BasePath + "/ui"
	fs := http.FileServer(http.Dir(cfg.UI.Dir))
	api.HandleFunc("ui/", http.StripPrefix(prefix+"/", fs).ServeHTTP)
}
