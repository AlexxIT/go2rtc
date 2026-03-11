package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/stretchr/testify/require"
)

func TestConfigHandlerReadOnly(t *testing.T) {
	prevPath := app.ConfigPath
	prevReadOnly := ReadOnly
	t.Cleanup(func() {
		app.ConfigPath = prevPath
		ReadOnly = prevReadOnly
	})

	app.ConfigPath = filepath.Join(t.TempDir(), "config.yaml")
	ReadOnly = true

	for _, method := range []string{"POST", "PATCH"} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/config", strings.NewReader("log:\n  level: info\n"))
			w := httptest.NewRecorder()

			configHandler(w, req)

			require.Equal(t, http.StatusForbidden, w.Code)
			require.Contains(t, w.Body.String(), "read-only")
		})
	}
}
