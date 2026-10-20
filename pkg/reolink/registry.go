package reolink

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/rs/zerolog"
)

type cameraKey struct {
	endpoint   string
	credential [sha256.Size]byte
}

type Registry struct {
	mu      sync.Mutex
	cameras map[cameraKey]*Camera
	secret  [sha256.Size]byte
	open    profileOpener
	log     zerolog.Logger
}

func (r *Registry) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprint(state, "reolink.Registry")
}

func NewRegistry(log zerolog.Logger) *Registry {
	var secret [sha256.Size]byte
	if _, err := rand.Read(secret[:]); err != nil {
		panic(fmt.Errorf("reolink: initialize credential identity: %w", err))
	}
	return newRegistry(log, secret, openBaichuan)
}

func newRegistry(log zerolog.Logger, secret [sha256.Size]byte, open profileOpener) *Registry {
	return &Registry{
		cameras: make(map[cameraKey]*Camera), secret: secret,
		open: open, log: log,
	}
}

func (r *Registry) Dial(rawURL string) (*Producer, error) {
	s, err := parseURL(rawURL)
	if err != nil {
		return nil, err
	}
	key := r.cameraKey(s)
	s.username, s.password, s.identity = "", "", ""

	profile, err := r.acquire(key, s)
	if err != nil {
		return nil, err
	}
	if err = profile.waitReady(); err != nil {
		r.abort(profile)
		return nil, err
	}

	var talk *baichuan.TalkFormat
	if s.backchannel {
		format, supported, probeErr := profile.probeTalk()
		if probeErr != nil {
			r.abort(profile)
			return nil, probeErr
		}
		if supported {
			talk = &format
		}
	}
	if s.suppressVideo && s.suppressAudio && talk == nil {
		r.abort(profile)
		return nil, fmt.Errorf("reolink: backchannel-only source requires camera talkback")
	}
	return newProducer(s, profile, talk), nil
}

func (r *Registry) abort(profile *Profile) {
	r.release(profile)
	<-profile.done
	profile.closeReceivers()
}

func (r *Registry) cameraKey(s source) cameraKey {
	h := hmac.New(sha256.New, r.secret[:])
	writeIdentityField(h, s.username)
	writeIdentityField(h, s.password)
	writeIdentityField(h, s.identity)
	var credential [sha256.Size]byte
	copy(credential[:], h.Sum(nil))
	return cameraKey{endpoint: s.remote, credential: credential}
}

func writeIdentityField(w hash.Hash, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = w.Write(size[:])
	_, _ = w.Write([]byte(value))
}

func (r *Registry) acquire(key cameraKey, s source) (*Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	camera := r.cameras[key]
	if camera == nil {
		camera = &Camera{
			registry: r, key: key, remote: s.remote,
			profiles: make(map[*Profile]struct{}), epochs: make(map[ProfileKey]mediaEpoch),
		}
		r.cameras[key] = camera
	}
	profileKey := ProfileKey{Channel: s.channel, Stream: s.stream}
	camera.addSession(s.stream)
	camera.generation++
	profile := newProfile(camera, profileKey, camera.generation, s, r.open)
	camera.refs++
	camera.profiles[profile] = struct{}{}
	r.log.Debug().Str("camera", s.remote).Uint8("channel", s.channel).
		Str("profile", string(s.stream)).Uint64("generation", profile.generation).
		Msg("reolink profile started")
	go profile.run()
	return profile, nil
}

func (r *Registry) release(profile *Profile) {
	r.mu.Lock()
	defer r.mu.Unlock()
	profile.mu.Lock()
	if profile.released {
		profile.mu.Unlock()
		return
	}
	profile.released = true
	if profile.camera.refs == 0 {
		panic("reolink: camera reference accounting underflow")
	}
	profile.camera.refs--
	if _, ok := profile.camera.profiles[profile]; ok {
		delete(profile.camera.profiles, profile)
		if profile.state == profileStarting || profile.state == profileActive {
			profile.state = profileClosing
			profile.cancel()
		}
	}
	profile.mu.Unlock()
	r.removeCameraLocked(profile.camera)
}

func (r *Registry) removeCameraLocked(camera *Camera) {
	if camera.refs == 0 && len(camera.profiles) == 0 && camera.mainSessions == 0 && camera.otherSessions == 0 &&
		r.cameras[camera.key] == camera {
		delete(r.cameras, camera.key)
	}
}
