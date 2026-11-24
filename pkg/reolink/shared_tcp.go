package reolink

import (
	"context"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
)

type sharedTCP struct {
	camera *Camera
	group  uint8
	config baichuan.Config
	used   map[ProfileKey]struct{}
	users  int
	ready  chan struct{}
	client *baichuan.Client
	err    error
}

// acquireTCP returns nil for sources that need a dedicated connection.
func (c *Camera) acquireTCP(ctx context.Context, s source) (*sharedTCP, error) {
	key := ProfileKey{Channel: s.channel, Stream: s.stream}
	group := s.sharedGroup
	for {
		c.registry.mu.Lock()
		shared := c.shared[group]
		if shared != nil && shared.client != nil {
			select {
			case <-shared.client.Done():
				delete(c.shared, group)
				shared = nil
			default:
			}
		}
		if shared == nil {
			shared = &sharedTCP{
				camera: c, group: group, config: s.config, used: map[ProfileKey]struct{}{key: {}},
				users: 1, ready: make(chan struct{}),
			}
			if c.shared == nil {
				c.shared = make(map[uint8]*sharedTCP)
			}
			c.shared[group] = shared
			c.registry.mu.Unlock()
			return shared.connect(ctx)
		}
		// Media headers have no preview generation. Keep used keys reserved
		// until this socket closes, even after a successful stop acknowledgement.
		if _, used := shared.used[key]; used || shared.config != s.config {
			c.registry.mu.Unlock()
			return nil, nil
		}
		if shared.client != nil {
			shared.used[key] = struct{}{}
			shared.users++
			c.registry.mu.Unlock()
			return shared, nil
		}
		c.registry.mu.Unlock()
		select {
		case <-shared.ready:
			if shared.err != nil {
				return nil, shared.err
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (s *sharedTCP) connect(ctx context.Context) (*sharedTCP, error) {
	c := s.camera
	client, err := loginBaichuan(ctx, c, s.config)
	c.registry.mu.Lock()
	s.client, s.err = client, err
	if err != nil && c.shared[s.group] == s {
		delete(c.shared, s.group)
	}
	close(s.ready)
	c.registry.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *sharedTCP) release() error {
	c := s.camera
	c.registry.mu.Lock()
	s.users--
	last := s.users == 0
	if last && c.shared[s.group] == s {
		delete(c.shared, s.group)
	}
	c.registry.mu.Unlock()
	if last {
		return s.client.Close()
	}
	return nil
}
