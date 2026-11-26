package xiaomi

import (
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss"
)

// The lookup contains configured source URLs only. A pair holds no credentials
// or device connection while both streams are idle.
type lensSource struct {
	pair      *miss.LensPair
	primary   string
	owner     string
	secondary bool
}

func lensSources(streams map[string][]string) map[string]lensSource {
	type urls struct{ primary, secondary string }
	pairs := make(map[string][]urls)
	counts := make(map[string]int)
	for name, list := range streams {
		for _, raw := range list {
			if !strings.HasPrefix(raw, "xiaomi:") {
				continue
			}
			counts[raw]++
			u, err := url.Parse(raw)
			if err != nil {
				continue
			}
			q, err := url.ParseQuery(u.RawQuery)
			owner := q.Get("reuse")
			if err != nil || owner == "" || owner == name || len(q["reuse"]) != 1 || len(q["channel"]) != 1 || q.Get("channel") != "2" {
				continue
			}
			primary := matchingPrimary(streams[owner], u, q)
			if primary != "" {
				pairs[owner] = append(pairs[owner], urls{primary, raw})
			}
		}
	}

	sources := make(map[string]lensSource)
	for owner, matches := range pairs {
		// A lens has one named stream. That stream fans out to its consumers.
		if len(matches) != 1 || counts[matches[0].primary] != 1 {
			continue
		}
		p := matches[0]
		src := lensSource{pair: new(miss.LensPair), primary: p.primary, owner: owner}
		sources[p.primary] = src
		src.secondary = true
		sources[p.secondary] = src
	}
	return sources
}

func matchingPrimary(list []string, second *url.URL, q url.Values) string {
	var raw string
	for _, candidate := range list {
		if strings.HasPrefix(candidate, "xiaomi:") {
			if raw != "" {
				return ""
			}
			raw = candidate
		}
	}
	if raw == "" || q.Get("model") != "xiaomi.camera.082ac1" || q.Get("did") == "" {
		return ""
	}
	p, err := url.Parse(raw)
	if err != nil || p.User == nil || second.User == nil || p.User.String() != second.User.String() || p.Host != second.Host {
		return ""
	}
	pq, err := url.ParseQuery(p.RawQuery)
	if err != nil || pq.Has("reuse") || (pq.Get("channel") != "" && pq.Get("channel") != "0") {
		return ""
	}
	// The secondary can omit the primary's media settings, or repeat them.
	for k, vs := range q {
		if k != "reuse" && k != "channel" && !slices.Equal(vs, pq[k]) {
			return ""
		}
	}
	return raw
}

var errReuse = errors.New("xiaomi: reuse requires one channel=2 stream and one matching primary xiaomi.camera.082ac1 stream")
