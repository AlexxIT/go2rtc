package wyze

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCamerasFromDevicesBaseStation(t *testing.T) {
	// Wyze Cam Outdoor (WVOD1) behind its base station (WVODB1), plus a regular camera.
	const data = `[
		{"mac":"AABBCCDDEE01","enr":"camENR0123456789","nickname":"Outdoor","product_model":"WVOD1","product_type":"Camera",
		 "conn_state":1,"parent_device_mac":"AABBCCDDEE00","parent_device_enr":"baseENR012345678",
		 "device_params":{"p2p_id":"UID00000000000000001","ip":"192.168.200.10","dtls":0,"main_device_dtls":1}},
		{"mac":"AABBCCDDEE00","enr":"baseENR012345678","nickname":"Base","product_model":"WVODB1","product_type":"BaseStation",
		 "conn_state":1,"device_params":{"p2p_id":"UID00000000000000001","ip":"192.168.1.20","dtls":1}},
		{"mac":"AABBCCDDEE02","enr":"v3ENR01234567890","nickname":"Cam v3","product_model":"WYZE_CAKP2JFUS","product_type":"Camera",
		 "conn_state":1,"device_params":{"p2p_id":"UID00000000000000002","ip":"192.168.1.30","dtls":1}}
	]`

	var devices []deviceInfo
	require.NoError(t, json.Unmarshal([]byte(data), &devices))

	cameras := camerasFromDevices(devices)
	require.Len(t, cameras, 2)

	outdoor := cameras[0]
	require.Equal(t, "192.168.1.20", outdoor.IP) // the base station's LAN IP
	require.Equal(t, 1, outdoor.DTLS)            // main_device_dtls
	require.Equal(t, "AABBCCDDEE00", outdoor.ParentMAC)
	require.Equal(t, "baseENR012345678", outdoor.ParentENR)
	require.Equal(t, "camENR0123456789", outdoor.ENR)

	v3 := cameras[1]
	require.Equal(t, "192.168.1.30", v3.IP)
	require.Empty(t, v3.ParentMAC)
}
