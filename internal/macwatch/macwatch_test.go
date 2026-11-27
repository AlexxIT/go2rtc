package macwatch

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

func TestNormalizeMAC(t *testing.T) {
	tests := []struct {
		in  string
		out string
	}{
		{"aa:bb:cc:dd:ee:ff", "aa:bb:cc:dd:ee:ff"},
		{"AA:BB:CC:DD:EE:FF", "aa:bb:cc:dd:ee:ff"},
		{"0:11:22:33:44:55", "00:11:22:33:44:55"},  // macOS style
		{"AA-BB-CC-DD-EE-FF", "aa:bb:cc:dd:ee:ff"}, // Windows style
		{"aabbccddeeff", "aa:bb:cc:dd:ee:ff"},
		{"aabb.ccdd.eeff", "aa:bb:cc:dd:ee:ff"}, // Cisco style
		{"(incomplete)", ""},
		{"", ""},
		{"aa:bb:cc:dd:ee", ""},
		{"aa:bb:cc:dd:ee:ff:00", ""},
		{"00:00:00:00:00:00", "00:00:00:00:00:00"},
	}

	for _, test := range tests {
		require.Equal(t, test.out, NormalizeMAC(test.in), test.in)
	}
}

func TestParseArpAN(t *testing.T) {
	// real output of "arp -an" on macOS
	data := "? (192.168.31.1) at 44:df:65:b9:42:dc on en0 ifscope [ethernet]\n" +
		"? (192.168.31.22) at 0:e0:4c:68:0:80 on en0 ifscope [ethernet]\n" +
		"? (192.168.31.39) at c:37:96:46:bf:43 on en0 ifscope permanent [ethernet]\n" +
		"? (192.168.31.99) at (incomplete) on en0 ifscope [ethernet]\n" +
		"? (192.168.31.255) at ff:ff:ff:ff:ff:ff on en0 ifscope [ethernet]\n" +
		"? (224.0.0.251) at 1:0:5e:0:0:fb on en0 ifscope permanent [ethernet]\n" +
		"some garbage line\n"

	items := ParseArpAN(data)

	require.Equal(t, []Neighbor{
		{IP: "192.168.31.1", MAC: "44:df:65:b9:42:dc", Iface: "en0"},
		{IP: "192.168.31.22", MAC: "00:e0:4c:68:00:80", Iface: "en0"},
		{IP: "192.168.31.39", MAC: "0c:37:96:46:bf:43", Iface: "en0"},
	}, items)
}

func TestParseArpANLinux(t *testing.T) {
	// output of "arp -a" from net-tools on Linux
	data := "? (192.168.1.1) at a4:2b:b0:11:22:33 [ether] on eth0\n"

	items := ParseArpAN(data)

	require.Equal(t, []Neighbor{
		{IP: "192.168.1.1", MAC: "a4:2b:b0:11:22:33", Iface: "eth0"},
	}, items)
}

func TestParseProcNetARP(t *testing.T) {
	data := "IP address       HW type     Flags       HW address            Mask     Device\n" +
		"192.168.1.1      0x1         0x2         a4:2b:b0:11:22:33     *        eth0\n" +
		"192.168.1.99     0x1         0x0         00:00:00:00:00:00     *        eth0\n"

	items := ParseProcNetARP(data)

	require.Equal(t, []Neighbor{
		{IP: "192.168.1.1", MAC: "a4:2b:b0:11:22:33", Iface: "eth0"},
	}, items)
}

func TestParseArpWindows(t *testing.T) {
	data := "Interface: 192.168.1.10 --- 0x5\n" +
		"  Internet Address      Physical Address      Type\n" +
		"  192.168.1.1           a4-2b-b0-11-22-33     dynamic\n" +
		"  192.168.1.255         ff-ff-ff-ff-ff-ff     static\n"

	items := ParseArpWindows(data)

	require.Equal(t, []Neighbor{
		{IP: "192.168.1.1", MAC: "a4:2b:b0:11:22:33"},
	}, items)
}

func TestNewTable(t *testing.T) {
	table := newTable([]Neighbor{
		{IP: "192.168.1.64", MAC: "aa:bb:cc:dd:ee:ff"},
		{IP: "192.168.1.71", MAC: "aa:bb:cc:dd:ee:ff"}, // stale entry
		{IP: "192.168.1.1", MAC: "a4:2b:b0:11:22:33"},
	})

	require.Equal(t, "aa:bb:cc:dd:ee:ff", table.ipToMAC["192.168.1.64"])
	require.Equal(t, []string{"192.168.1.64", "192.168.1.71"}, table.macToIP["aa:bb:cc:dd:ee:ff"])
	require.Equal(t, []string{"192.168.1.1"}, table.macToIP["a4:2b:b0:11:22:33"])
}

func TestReplaceIPHost(t *testing.T) {
	tests := []struct {
		raw    string
		newIP  string
		result string
		ok     bool
	}{
		{
			"rtsp://admin:pass@192.168.1.64:554/Streaming/Channels/101",
			"192.168.1.71",
			"rtsp://admin:pass@192.168.1.71:554/Streaming/Channels/101",
			true,
		},
		{
			"rtsp://192.168.1.64/Streaming/Channels/101",
			"192.168.1.71",
			"rtsp://192.168.1.71/Streaming/Channels/101",
			true,
		},
		{
			"rtsp://admin:p@ss@192.168.1.64:554/ch1?subtype=0#backchannel=0",
			"192.168.1.71",
			"rtsp://admin:p@ss@192.168.1.71:554/ch1?subtype=0#backchannel=0",
			true,
		},
		{
			"http://192.168.1.64/snapshot.jpg",
			"192.168.1.71",
			"http://192.168.1.71/snapshot.jpg",
			true,
		},
		{
			"rtsp://192.168.1.65:554/ch1", // another host
			"192.168.1.71",
			"",
			false,
		},
		{
			"ffmpeg:camera1#video=h264", // link to another stream
			"192.168.1.71",
			"",
			false,
		},
		{
			"rtsp://camera.local:554/ch1", // hostname
			"192.168.1.71",
			"",
			false,
		},
	}

	for _, test := range tests {
		result, ok := replaceIPHost(test.raw, "192.168.1.64", test.newIP)
		require.Equal(t, test.ok, ok, test.raw)
		require.Equal(t, test.result, result, test.raw)
	}
}

func TestSaveConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "go2rtc.yaml")

	config := "log:\n" +
		"  level: info\n" +
		"streams:\n" +
		"  camera1: rtsp://admin:pass@192.168.1.64:554/ch1\n" +
		"  camera2:\n" +
		"    - rtsp://admin:pass@192.168.1.64:554/ch1\n" +
		"    - rtsp://admin:pass@192.168.1.64:554/ch2\n" +
		"  camera3:\n" +
		"    url: rtsp://192.168.1.64/ch1\n" +
		"    name: Camera 3\n"

	require.NoError(t, os.WriteFile(path, []byte(config), 0644))

	oldPath := app.ConfigPath
	app.ConfigPath = path
	t.Cleanup(func() { app.ConfigPath = oldPath })

	require.NoError(t, saveConfig("camera1", "192.168.1.64", "192.168.1.71"))
	require.NoError(t, saveConfig("camera2", "192.168.1.64", "192.168.1.72"))
	require.NoError(t, saveConfig("camera3", "192.168.1.64", "192.168.1.73"))

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	expected := "log:\n" +
		"  level: info\n" +
		"streams:\n" +
		"  camera1: rtsp://admin:pass@192.168.1.71:554/ch1\n" +
		"  camera2:\n" +
		"    - rtsp://admin:pass@192.168.1.72:554/ch1\n" +
		"    - rtsp://admin:pass@192.168.1.72:554/ch2\n" +
		"  camera3:\n" +
		"    url: rtsp://192.168.1.73/ch1\n" +
		"    name: Camera 3\n"

	require.Equal(t, expected, string(data))

	// stream from another config file or from the command line
	require.Error(t, saveConfig("camera4", "192.168.1.64", "192.168.1.74"))
}

func TestResolve(t *testing.T) {
	// stub, because internal/rtsp module is not inited in the tests
	streams.HandleFunc("rtsp", func(source string) (core.Producer, error) {
		return nil, nil
	})

	url := "rtsp://admin:pass@192.168.1.64:554/Streaming/Channels/101"

	stream, err := streams.New("macwatch_test", url)
	require.NoError(t, err)

	dir := t.TempDir()
	path := filepath.Join(dir, "go2rtc.yaml")

	config := "streams:\n  macwatch_test: " + url + "\n"
	require.NoError(t, os.WriteFile(path, []byte(config), 0644))

	oldPath := app.ConfigPath
	app.ConfigPath = path
	t.Cleanup(func() { app.ConfigPath = oldPath })

	cam := &Camera{
		Stream: "macwatch_test",
		MAC:    "aa:bb:cc:dd:ee:ff",
		Source: url,
		IP:     "192.168.1.64",
		port:   "554",
	}

	table := newTable([]Neighbor{
		{IP: "192.168.1.71", MAC: "aa:bb:cc:dd:ee:ff"},
		{IP: "192.168.1.1", MAC: "a4:2b:b0:11:22:33"},
	})

	resolve(cam, table)

	require.Equal(t, statusUpdated, cam.Status)
	require.Equal(t, "192.168.1.71", cam.IP)
	require.NotEmpty(t, cam.Updated)

	// in memory stream should be updated too
	require.Equal(t, []string{"rtsp://admin:pass@192.168.1.71:554/Streaming/Channels/101"}, stream.Sources())

	// and the config file
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "streams:\n  macwatch_test: rtsp://admin:pass@192.168.1.71:554/Streaming/Channels/101\n", string(data))
}

func TestResolveNotFound(t *testing.T) {
	cam := &Camera{
		Stream: "camera1",
		MAC:    "aa:bb:cc:dd:ee:ff",
		Source: "rtsp://192.168.1.64:554/ch1",
		IP:     "192.168.1.64",
		port:   "554",
	}

	resolve(cam, newTable(nil))

	require.Equal(t, statusNotFound, cam.Status)
}

func TestDiscoverCameras(t *testing.T) {
	streams.HandleFunc("rtsp", func(source string) (core.Producer, error) {
		return nil, nil
	})
	streams.HandleFunc("ffmpeg", func(source string) (core.Producer, error) {
		return nil, nil
	})

	for name, source := range map[string]string{
		"macwatch_d1": "rtsp://admin:pass@192.168.1.64:554/ch1",
		"macwatch_d2": "rtsp://camera.local:554/ch1",   // hostname
		"macwatch_d3": "rtsp://192.168.1.200:554/ch1",  // unknown MAC
		"macwatch_d4": "ffmpeg:macwatch_d1#video=h264", // link to another stream
	} {
		_, err := streams.New(name, source)
		require.NoError(t, err)
	}

	table := newTable([]Neighbor{
		{IP: "192.168.1.64", MAC: "aa:bb:cc:dd:ee:ff"},
		{IP: "192.168.1.1", MAC: "a4:2b:b0:11:22:33"},
	})

	cameras := discoverCameras(table)

	require.Equal(t, map[string]string{"macwatch_d1": "aa:bb:cc:dd:ee:ff"}, cameras)
}

func TestMissingCameras(t *testing.T) {
	targetsMu.Lock()
	targets = []*target{{Stream: "tracked1", MAC: "aa:bb:cc:dd:ee:ff"}}
	targetsMu.Unlock()

	t.Cleanup(func() {
		targetsMu.Lock()
		targets = nil
		targetsMu.Unlock()
	})

	cameras := map[string]string{
		"tracked1": "aa:bb:cc:dd:ee:ff",
		"new1":     "11:22:33:44:55:66",
		"new2":     "aa:bb:cc:dd:ee:00",
	}

	require.Equal(t, map[string]string{
		"new1": "11:22:33:44:55:66",
		"new2": "aa:bb:cc:dd:ee:00",
	}, missingCameras(cameras))
}

func TestSubnetHosts(t *testing.T) {
	_, n, err := net.ParseCIDR("192.168.1.0/24")
	require.NoError(t, err)

	hosts := subnetHosts(n)
	require.Len(t, hosts, 254)
	require.Equal(t, "192.168.1.1", hosts[0].String())
	require.Equal(t, "192.168.1.254", hosts[253].String())

	// no sweep for too big networks and IPv6
	require.Equal(t, 0, Sweep(nil))

	_, n, _ = net.ParseCIDR("10.0.0.0/8")
	require.Empty(t, subnetHosts(n))
	require.Equal(t, 0, Sweep(n))

	_, n, _ = net.ParseCIDR("192.168.1.64/32")
	require.Empty(t, subnetHosts(n))
}
