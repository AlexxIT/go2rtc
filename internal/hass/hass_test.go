package hass

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImportHomeKitAddress(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".storage"), 0o755))

	entry := func(title, ip string) string {
		return `{"domain":"homekit_controller","title":"` + title + `","data":{
			"AccessoryIP":"` + ip + `","AccessoryPort":51826,"AccessoryPairingID":"AA:BB",
			"AccessoryLTPK":"aa","iOSPairingId":"id","iOSDeviceLTSK":"bb","iOSDeviceLTPK":"cc"}}`
	}
	config := `{"data":{"entries":[` + entry("v4", "192.168.1.50") + `,` + entry("v6", "fd00::60") + `]}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".storage/core.config_entries"), []byte(config), 0o644))

	require.NoError(t, importConfig(dir))
	require.Equal(t, "homekit://192.168.1.50:51826?client_id=id&client_private=bbcc&device_id=AA:BB&device_public=aa", entities["v4"])
	require.Equal(t, "homekit://[fd00::60]:51826?client_id=id&client_private=bbcc&device_id=AA:BB&device_public=aa", entities["v6"])
}
