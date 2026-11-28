package xiaomi

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfirmURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "no confirm url",
			url:  "https://account.xiaomi.com/fe/service/verifyPhone",
		},
		{
			name: "absolute confirm url",
			url:  "https://account.xiaomi.com/fe/service/verifyPhone?confirmUrl=https%3A%2F%2Fsts.api.io.mi.com%2Fsts%3Ffoo%3Dbar",
			want: "https://sts.api.io.mi.com/sts?foo=bar",
		},
		{
			name: "relative confirm url",
			url:  "https://account.xiaomi.com/fe/service/verifyPhone?confirmUrl=%2Fpass%2FserviceLoginAuth2%2Fend",
			want: "https://account.xiaomi.com/pass/serviceLoginAuth2/end",
		},
		{
			name: "skip url",
			url:  "https://account.xiaomi.com/pass2/confirmPhone?skipUrl=https%3A%2F%2Faccount.xiaomi.com%2Fpass2%2Fuserinfo%2Fstart",
			want: "https://account.xiaomi.com/pass2/userinfo/start",
		},
		{
			name: "confirm url has priority",
			url:  "https://account.xiaomi.com/fe/service/verifyPhone?confirmUrl=https%3A%2F%2Fa.example%2Fx&skipUrl=https%3A%2F%2Fb.example%2Fy",
			want: "https://a.example/x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tt.url, nil)
			require.NoError(t, err)

			require.Equal(t, tt.want, confirmURL(&http.Response{Request: req}))
		})
	}

	require.Empty(t, confirmURL(nil))
	require.Empty(t, confirmURL(&http.Response{}))
}

func TestNewCloudHasCookieJar(t *testing.T) {
	cloud := NewCloud("xiaomiio")
	require.NotNil(t, cloud.client.Jar)
}
