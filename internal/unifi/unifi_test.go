package unifi

import (
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/unifi"
	"github.com/stretchr/testify/require"
)

func TestBackchannelCommand(t *testing.T) {
	session := &unifi.Session{URL: "rtp://192.168.1.1:7004", Codec: "opus", SamplingRate: 24000}

	cmd, err := backchannelCommand("/usr/bin/ffmpeg", session, &core.Codec{Name: core.CodecPCMA})
	require.NoError(t, err)
	require.Equal(t, "exec:/usr/bin/ffmpeg -hide_banner -v error -fflags nobuffer -f alaw -ar 8000 -ac 1 -i -"+
		" -c:a libopus -application:a lowdelay -ar:a 24000 -ac:a 1 -f rtp rtp://192.168.1.1:7004"+
		"#backchannel=1#audio=alaw/8000", cmd)

	cmd, err = backchannelCommand("ffmpeg", session, &core.Codec{Name: core.CodecPCMU})
	require.NoError(t, err)
	require.Contains(t, cmd, " -f mulaw -ar 8000 ")
	require.Contains(t, cmd, "#audio=mulaw/8000")
}

func TestTalkbackURL(t *testing.T) {
	for _, s := range []string{"rtp://192.168.1.1:7004", "rtp://192.168.1.1:7004/", "rtp://[fd00::1]:7004"} {
		u, err := talkbackURL(s)
		require.NoError(t, err, s)
		require.Equal(t, strings.TrimSuffix(s, "/"), u)
	}
}

func TestBackchannelCommandErrors(t *testing.T) {
	pcma := &core.Codec{Name: core.CodecPCMA}

	for _, test := range []struct {
		session unifi.Session
		codec   *core.Codec
		err     string
	}{
		{unifi.Session{URL: "rtp://192.168.1.1:7004", Codec: "aac"}, pcma, "unifi: unsupported talkback codec: aac"},
		{unifi.Session{URL: "rtp://192.168.1.1:7004", Codec: "opus"}, &core.Codec{Name: core.CodecG722}, "unifi: unsupported microphone codec: G722"},
		{unifi.Session{URL: "udp://192.168.1.1:7004", Codec: "opus"}, pcma, "unifi: unsupported talkback url"},
		{unifi.Session{URL: "rtp://192.168.1.1:7004 -y /tmp/file", Codec: "opus"}, pcma, "unifi: unsupported talkback url"},
		{unifi.Session{URL: "rtp://192.168.1.1:7004#backchannel=0", Codec: "opus"}, pcma, "unifi: unsupported talkback url"},
		{unifi.Session{URL: "rtp://192.168.1.1:7004?listen=1", Codec: "opus"}, pcma, "unifi: unsupported talkback url"},
		{unifi.Session{URL: "rtp://192.168.1.1:7004?localport=7004&pkt_size=64", Codec: "opus"}, pcma, "unifi: unsupported talkback url"},
		{unifi.Session{URL: "rtp://192.168.1.1:7004?", Codec: "opus"}, pcma, ""},
		{unifi.Session{URL: "rtp://user:pass@192.168.1.1:7004", Codec: "opus"}, pcma, "unifi: unsupported talkback url"},
		{unifi.Session{URL: "rtp://192.168.1.1:7004/stream", Codec: "opus"}, pcma, "unifi: unsupported talkback url"},
		{unifi.Session{URL: "rtp://192.168.1.1:7004#", Codec: "opus"}, pcma, ""},
		{unifi.Session{URL: "rtp://192.168.1.1", Codec: "opus"}, pcma, "unifi: unsupported talkback url"},
		{unifi.Session{URL: "rtp://192.168.1.1:abc", Codec: "opus"}, pcma, "unifi: unsupported talkback url"},
		{unifi.Session{URL: "rtp://192.168.1.1:70000", Codec: "opus"}, pcma, "unifi: unsupported talkback url"},
		{unifi.Session{URL: "rtp://:7004", Codec: "opus"}, pcma, "unifi: unsupported talkback url"},
		{unifi.Session{URL: "rtp:/tmp/file", Codec: "opus"}, pcma, "unifi: unsupported talkback url"},
	} {
		cmd, err := backchannelCommand("ffmpeg", &test.session, test.codec)
		if test.err == "" {
			// rebuilt from scheme, host and port, nothing else reaches FFmpeg
			require.NoError(t, err, test.session.URL)
			require.Contains(t, cmd, " -f rtp rtp://192.168.1.1:7004#backchannel=1#", test.session.URL)
			continue
		}
		require.EqualError(t, err, test.err, test.session.URL)
	}
}
