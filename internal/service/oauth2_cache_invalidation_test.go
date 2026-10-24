package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// Cache invalidation is the operator's escape hatch when a provider revokes
// or rotates credentials: if InvalidateCache silently kept serving the old
// token, mail would keep failing until expiry with no way to force a refresh.

func oauth2CountingServer(t *testing.T, hits *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"token-%d","token_type":"Bearer","expires_in":3600}`, n)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func googleSettings() *domain.SMTPSettings {
	return &domain.SMTPSettings{
		OAuth2Provider: "google", OAuth2ClientID: "client-1",
		OAuth2ClientSecret: "secret", OAuth2RefreshToken: "refresh",
	}
}

func TestOAuth2TokenService_InvalidateCache(t *testing.T) {
	var hits int32
	srv := oauth2CountingServer(t, &hits)
	svc := NewOAuth2TokenService(logger.NewLoggerWithLevel("disabled"))
	svc.SetTokenURLs("", srv.URL)
	settings := googleSettings()

	first, err := svc.GetAccessToken(settings)
	require.NoError(t, err)
	second, err := svc.GetAccessToken(settings)
	require.NoError(t, err)
	assert.Equal(t, first, second, "second call must come from the cache")
	assert.Equal(t, int32(1), atomic.LoadInt32(&hits))

	svc.InvalidateCache()

	third, err := svc.GetAccessToken(settings)
	require.NoError(t, err)
	assert.NotEqual(t, first, third, "after invalidation a fresh token must be fetched")
	assert.Equal(t, int32(2), atomic.LoadInt32(&hits))
}

func TestOAuth2TokenService_InvalidateCacheForSettings(t *testing.T) {
	var hits int32
	srv := oauth2CountingServer(t, &hits)
	svc := NewOAuth2TokenService(logger.NewLoggerWithLevel("disabled"))
	svc.SetTokenURLs("", srv.URL)

	settingsA := googleSettings()
	settingsB := googleSettings()
	settingsB.OAuth2ClientID = "client-2" // different cache key (provider:tenant:client)

	tokenA, err := svc.GetAccessToken(settingsA)
	require.NoError(t, err)
	tokenB, err := svc.GetAccessToken(settingsB)
	require.NoError(t, err)
	require.Equal(t, int32(2), atomic.LoadInt32(&hits))

	svc.InvalidateCacheForSettings(settingsA)

	againA, err := svc.GetAccessToken(settingsA)
	require.NoError(t, err)
	assert.NotEqual(t, tokenA, againA, "the invalidated settings must refetch")
	againB, err := svc.GetAccessToken(settingsB)
	require.NoError(t, err)
	assert.Equal(t, tokenB, againB, "other settings keep their cached token")
	assert.Equal(t, int32(3), atomic.LoadInt32(&hits), "exactly one refetch")
}

func TestOAuth2TokenService_SetTokenURLs_RoutesProviders(t *testing.T) {
	var msHits, googleHits int32
	msSrv := oauth2CountingServer(t, &msHits)
	googleSrv := oauth2CountingServer(t, &googleHits)

	svc := NewOAuth2TokenService(logger.NewLoggerWithLevel("disabled"))
	svc.SetTokenURLs(msSrv.URL, googleSrv.URL)

	_, err := svc.GetAccessToken(&domain.SMTPSettings{
		OAuth2Provider: "microsoft", OAuth2TenantID: "tenant-1",
		OAuth2ClientID: "client-1", OAuth2ClientSecret: "secret",
	})
	require.NoError(t, err)
	_, err = svc.GetAccessToken(googleSettings())
	require.NoError(t, err)

	assert.Equal(t, int32(1), atomic.LoadInt32(&msHits), "microsoft must use the microsoft URL")
	assert.Equal(t, int32(1), atomic.LoadInt32(&googleHits), "google must use the google URL")
}
