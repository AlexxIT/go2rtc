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

func (l *Listener) NewSecret(name string, defaultValues interface{}) *app.Secret {
	return app.NewSecret(name, defaultValues)
}
