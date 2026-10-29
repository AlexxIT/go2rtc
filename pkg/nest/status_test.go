package nest

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func fakeResponse(code int, body string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Status:     http.StatusText(code),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestStatusErrorCarriesGoogleReason(t *testing.T) {
	err := newStatusError(fakeResponse(400, `{"error":{"code":400,"message":"The camera is not available for streaming.","status":"FAILED_PRECONDITION"}}`))

	var se *StatusError
	require.ErrorAs(t, err, &se)
	require.Equal(t, 400, se.Code)
	require.Contains(t, err.Error(), "nest: wrong status: Bad Request")
	require.Contains(t, err.Error(), "The camera is not available for streaming.")
}

// A camera that is switched off, missing, or forbidden will not come back
// within a retry window; only transport failures and server errors deserve one.
func TestRetryable(t *testing.T) {
	require.False(t, retryable(newStatusError(fakeResponse(400, ""))))
	require.False(t, retryable(newStatusError(fakeResponse(403, ""))))
	require.False(t, retryable(newStatusError(fakeResponse(404, ""))))
	require.True(t, retryable(newStatusError(fakeResponse(500, ""))))
	require.True(t, retryable(newStatusError(fakeResponse(503, ""))))
	require.True(t, retryable(errors.New("dial tcp: i/o timeout")))
}

// Google intermittently answers a GenerateWebRtcStream with 400
// INVALID_ARGUMENT "offerSdp contains an invalid value." for a camera that is
// streaming fine; the next dial, with a fresh offer, succeeds. That verdict is
// on the offer, not the device, so it must not start the off-camera hold-off.
func TestRejectedOfferIsRetryable(t *testing.T) {
	rejected := newStatusError(fakeResponse(400, `{"error":{"code":400,"message":"offerSdp contains an invalid value.","status":"INVALID_ARGUMENT"}}`))
	off := newStatusError(fakeResponse(400, `{"error":{"code":400,"message":"The camera is not available for streaming.","status":"FAILED_PRECONDITION"}}`))

	require.True(t, rejectedOffer(rejected))
	require.True(t, retryable(rejected))

	require.False(t, rejectedOffer(off))
	require.False(t, retryable(off))
	require.False(t, rejectedOffer(newStatusError(fakeResponse(500, `{"status":"INVALID_ARGUMENT"}`))))
	require.False(t, rejectedOffer(errors.New("INVALID_ARGUMENT")))
}
