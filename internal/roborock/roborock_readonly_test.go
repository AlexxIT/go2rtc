package roborock

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/stretchr/testify/require"
)

func TestApiHandleReadOnly(t *testing.T) {
	prevReadOnly := api.ReadOnly
	t.Cleanup(func() {
		api.ReadOnly = prevReadOnly
	})

	api.ReadOnly = true

	req := httptest.NewRequest("POST", "/api/roborock", nil)
	w := httptest.NewRecorder()

	apiHandle(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}
