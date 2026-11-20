package rtsp

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/AlexxIT/go2rtc/pkg/onvif"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
)

// keyframePorts - ONVIF ports for keyframe request after PLAY, nil - disabled
var keyframePorts []int

type keyframeTarget struct {
	client *onvif.Client
	token  string
	mu     sync.Mutex
}

var keyframeTargets = map[string]*keyframeTarget{}
var keyframeMu sync.Mutex

// listenKeyframe - after successful PLAY ask the camera for a keyframe, so the session
// starts at once and doesn't wait for the next keyframe (can be a few seconds)
func listenKeyframe(conn *rtsp.Conn, rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}

	// find ONVIF profile before PLAY, so the request will be sent without delay
	go getKeyframeTarget(u)

	var play atomic.Bool
	conn.Listen(func(msg any) {
		switch msg := msg.(type) {
		case *tcp.Request:
			play.Store(msg.Method == rtsp.MethodPlay)
		case *tcp.Response:
			if play.Swap(false) && msg.StatusCode == http.StatusOK {
				go requestKeyframe(u)
			}
		}
	})
}

func requestKeyframe(u *url.URL) {
	client, token := getKeyframeTarget(u)
	if client == nil {
		return
	}

	if err := client.SetSynchronizationPoint(token); err != nil {
		log.Debug().Err(err).Msgf("[rtsp] keyframe request host=%s", u.Host)

		// search the profile again next time, camera settings may have changed
		keyframeMu.Lock()
		delete(keyframeTargets, u.Host+u.Path)
		keyframeMu.Unlock()
		return
	}

	log.Trace().Msgf("[rtsp] keyframe request host=%s token=%s", u.Host, token)
}

// getKeyframeTarget - return ONVIF client and profile with the same stream path as RTSP source
func getKeyframeTarget(u *url.URL) (*onvif.Client, string) {
	key := u.Host + u.Path

	keyframeMu.Lock()
	target := keyframeTargets[key]
	if target == nil {
		target = &keyframeTarget{}
		keyframeTargets[key] = target
	}
	keyframeMu.Unlock()

	// only one search for each source, other requests will wait for it
	target.mu.Lock()
	defer target.mu.Unlock()

	if target.client == nil {
		target.client, target.token = findProfile(u)
	}

	return target.client, target.token
}

func findProfile(u *url.URL) (*onvif.Client, string) {
	for _, port := range keyframePorts {
		host := net.JoinHostPort(u.Hostname(), strconv.Itoa(port))
		client, err := onvif.NewClient((&url.URL{Scheme: "onvif", User: u.User, Host: host}).String())
		if err != nil {
			continue
		}

		tokens, err := client.GetProfilesTokens()
		if err != nil {
			continue
		}

		for _, token := range tokens {
			b, err := client.GetStreamUri(token)
			if err != nil {
				continue
			}
			if uri, err := url.Parse(onvif.FindTagValue(b, "Uri")); err == nil && uri.Path == u.Path {
				return client, token
			}
		}
	}

	log.Debug().Msgf("[rtsp] can't find ONVIF profile for keyframe request host=%s", u.Host)

	return nil, ""
}
