package xiaomi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/stretchr/testify/require"
)

func TestApiXiaomiReadOnly(t *testing.T) {
	prevReadOnly := api.ReadOnly
	t.Cleanup(func() {
		api.ReadOnly = prevReadOnly
	})

	api.ReadOnly = true

	req := httptest.NewRequest("POST", "/api/xiaomi", nil)
	w := httptest.NewRecorder()

	apiXiaomi(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}
