package tuya

import (
	"errors"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/require"
)

type publishToken struct {
	mqtt.Token
	complete bool
	err      error
	timeout  time.Duration
}

func (t *publishToken) Wait() bool { panic("publish must not wait without a timeout") }
func (t *publishToken) WaitTimeout(timeout time.Duration) bool {
	t.timeout = timeout
	return t.complete
}
func (t *publishToken) Error() error { return t.err }

type publishClient struct {
	mqtt.Client
	tokens       []*publishToken
	disconnected bool
}

func (c *publishClient) Publish(string, byte, bool, interface{}) mqtt.Token {
	token := c.tokens[0]
	c.tokens = c.tokens[1:]
	return token
}
func (c *publishClient) Disconnect(uint) { c.disconnected = true }

func TestPublishTimeoutAllowsNextAttempt(t *testing.T) {
	stalled := &publishToken{}
	completed := &publishToken{complete: true}
	client := &publishClient{tokens: []*publishToken{stalled, completed}}
	conn := NewTuyaMqttClient("test-camera")
	conn.client = client
	require.NotPanics(t, func() {
		require.ErrorContains(t, conn.SendCandidate("candidate:test"), "timeout")
		require.NoError(t, conn.SendCandidate("candidate:retry"))
	})
	require.Positive(t, stalled.timeout)
	require.LessOrEqual(t, stalled.timeout, 30*time.Second)
}

func TestPublishReturnsBrokerError(t *testing.T) {
	brokerErr := errors.New("connection lost")
	token := &publishToken{complete: true, err: brokerErr}
	conn := NewTuyaMqttClient("test-camera")
	conn.client = &publishClient{tokens: []*publishToken{token}}
	require.ErrorIs(t, conn.SendCandidate("candidate:test"), brokerErr)
}

func TestStopDisconnectsAfterPublishTimeout(t *testing.T) {
	client := &publishClient{tokens: []*publishToken{{}}}
	conn := NewTuyaMqttClient("test-camera")
	conn.client = client
	require.NotPanics(t, conn.Stop)
	require.True(t, client.disconnected)
	require.True(t, conn.closed)
}
