package baichuan

import (
	"strings"
	"testing"
)

func TestParseDeviceInfo(t *testing.T) {
	body := []byte(`<body><VersionInfo><type> E1 Zoom </type><itemNo>E340</itemNo>` +
		`<hardwareVersion>IPC_NT14</hardwareVersion><firmwareVersion>v3.2.0</firmwareVersion>` +
		`<name>private camera name</name><serialNumber>private serial</serialNumber></VersionInfo></body>`)
	value, err := parseDeviceInfo(body)
	if err != nil {
		t.Fatal(err)
	}
	if value.Type != "E1 Zoom" || value.Model != "E340" || value.DisplayModel() != "E340" ||
		value.Hardware != "IPC_NT14" || value.Firmware != "v3.2.0" {
		t.Fatalf("unexpected device information: %+v", value)
	}
	if strings.Contains(value.Type+value.Model+value.Hardware+value.Firmware, "private") {
		t.Fatalf("private fields escaped parser: %+v", value)
	}
}

func TestParseDeviceInfoFallbackAndLimits(t *testing.T) {
	value, err := parseDeviceInfo([]byte(`<body><VersionInfo><type>TrackFlex</type></VersionInfo></body>`))
	if err != nil || value.DisplayModel() != "TrackFlex" {
		t.Fatalf("fallback = %+v, %v", value, err)
	}
	for _, body := range [][]byte{
		make([]byte, maxDeviceInfoBody+1),
		[]byte(`<body/>`),
		[]byte(`<body><VersionInfo><type></type></VersionInfo></body>`),
		[]byte(`<body><VersionInfo><type>` + strings.Repeat("x", maxDeviceInfoField+1) +
			`</type></VersionInfo></body>`),
	} {
		if _, err = parseDeviceInfo(body); err == nil {
			t.Fatalf("accepted invalid device information of length %d", len(body))
		}
	}
}
