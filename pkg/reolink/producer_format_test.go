package reolink

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestProducerValueFormattingRedactsSource(t *testing.T) {
	const username, password, uid = "formatter-user-sentinel", "formatter-pass-sentinel", "ABC1234567890001"
	for _, endpoint := range []string{"camera/main?backchannel=0", uid + "/main?transport=uid&backchannel=0"} {
		s, err := parseURL("reolink://" + username + ":" + password + "@" + endpoint)
		if err != nil {
			t.Fatal(err)
		}
		// Match Dial's cleared identity fields while retaining Config's credentials.
		s.username, s.password, s.identity = "", "", ""
		camera := &Camera{remote: s.remote, epochs: make(map[ProfileKey]mediaEpoch)}
		p := newProducer(camera, ProfileKey{}, 1, s, nil)
		defer p.cancel()
		// This fresh owner has no worker, timer or previously acquired lock.
		// Reflection avoids a typed copy of a mutex-bearing value in the test.
		value := reflect.ValueOf(p).Elem().Interface()
		for name, subject := range map[string]any{
			"pointer": p, "value": value,
			"nested pointer": struct{ Owner any }{Owner: p},
			"nested value":   struct{ Owner any }{Owner: value},
			"slice value":    []any{value}, "map value": map[string]any{"owner": value},
		} {
			for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%o", "%O", "%b", "%f", "%F", "%e", "%E", "%g", "%G", "%c", "%U", "%t", "%p", "%T"} {
				text := fmt.Sprintf(verb, subject)
				for _, sentinel := range []string{username, password, uid} {
					for _, encoded := range []string{sentinel, fmt.Sprintf("%x", sentinel), fmt.Sprintf("%X", sentinel)} {
						if strings.Contains(text, encoded) {
							t.Errorf("formatted %s using %s exposes source sentinel", name, verb)
						}
					}
				}
			}
		}
	}
}
