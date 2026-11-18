package tcp

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthValidate(t *testing.T) {
	auth := NewAuth(url.UserPassword("admin", "secret"))

	req := &Request{Header: map[string][]string{}}
	valid, empty := auth.Validate(req)
	require.False(t, valid)
	require.True(t, empty)

	req.Header.Set("Authorization", "Basic "+B64("admin", "wrong"))
	valid, empty = auth.Validate(req)
	require.False(t, valid)
	require.False(t, empty)

	req.Header.Set("Authorization", "Basic "+B64("admin", "secret"))
	valid, _ = auth.Validate(req)
	require.True(t, valid)
}
