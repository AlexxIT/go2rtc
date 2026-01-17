package tutk

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestXXTEADecrypt(t *testing.T) {
	buf := []byte("WERhJxb87WF3zgPa")
	key := []byte("GAgDiwVPg2E4GMke")
	XXTEADecrypt(buf, buf, key)
	require.Equal(t, "\xc4\xa6\x2c\xa1\x10\x64\x17\xa5\xda\x02\xe1\x62\xa5\xf0\x62\x71", string(buf))
}
