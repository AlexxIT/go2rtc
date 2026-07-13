package unifi

import (
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/unifi"
)

func Init() {
	unifi.SetLogger(app.GetLogger("unifi"))

	streams.HandleFunc(unifi.SchemeTalkback, func(source string) (core.Producer, error) {
		return unifi.DialTalkback(source)
	})

	for _, sources := range streams.GetAllSources() {
		for _, source := range sources {
			unifi.RegisterTalkbackSecrets(source)
		}
	}
}
