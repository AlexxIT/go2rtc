package core

import "github.com/AlexxIT/go2rtc/internal/app"

type EventFunc func(msg any)

// Listener base struct for all classes with support feedback
type Listener struct {
	events []EventFunc
}

func (l *Listener) Listen(f EventFunc) {
	l.events = append(l.events, f)
}

func (l *Listener) Fire(msg any) {
	for _, f := range l.events {
		f(msg)
	}
}

func (l *Listener) ParseSource(url string) string {
	return app.ResolveSecrets(url)
}

func (l *Listener) SaveSource(path []string, value any) error {
	return app.PatchSecret(path, value)
}