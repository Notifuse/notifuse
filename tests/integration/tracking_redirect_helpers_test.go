package integration

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertTrackingInterstitial checks that resp is the confirmation page shown for a
// tracked link whose destination could not be verified: a 200 HTML page, never a
// redirect. With wantHost set, the page names it and offers a Continue link; with "",
// it offers no link at all.
func assertTrackingInterstitial(t *testing.T, resp *http.Response, wantHost string) {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "expected the interstitial, got %d: %s", resp.StatusCode, body)
	assert.Empty(t, resp.Header.Get("Location"))
	assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
	assert.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	assert.Contains(t, resp.Header.Get("Content-Security-Policy"), "default-src 'none'")
	page := string(body)
	if wantHost != "" {
		assert.Contains(t, page, wantHost)
		assert.Contains(t, page, `class="continue"`)
	} else {
		assert.False(t, strings.Contains(page, "<a "), "no link expected: %s", page)
	}
}

// assertTrackedRedirect checks that resp redirects with 303 to exactly want.
func assertTrackedRedirect(t *testing.T, resp *http.Response, want string) {
	t.Helper()
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, want, resp.Header.Get("Location"))
}
