package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
)

func newOIDCTestHandler(t *testing.T, ctrl *gomock.Controller, apiEndpoint string) (*OIDCHandler, *mocks.MockOIDCServiceInterface, *ratelimiter.RateLimiter) {
	t.Helper()
	svc := mocks.NewMockOIDCServiceInterface(ctrl)
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("oidc:start", 10, time.Minute)
	rl.SetPolicy("oidc:callback", 10, time.Minute)
	rl.SetPolicy("oidc:exchange", 10, time.Minute)
	cfg := &config.Config{}
	cfg.APIEndpoint = apiEndpoint
	h := NewOIDCHandler(svc, cfg, rl, logger.NewLogger())
	return h, svc, rl
}

func cookieByName(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// --- Start ------------------------------------------------------------------

func TestOIDCHandler_Start_RedirectsToIdP(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h, svc, _ := newOIDCTestHandler(t, ctrl, "https://app.example.com")

	svc.EXPECT().IsEnabled().Return(true)
	svc.EXPECT().BuildAuthURL(gomock.Any()).Return(&domain.OIDCAuthRequest{
		AuthURL:   "https://idp.example.com/authorize?state=x",
		FlowState: domain.OIDCFlowState{State: "x", Nonce: "n", Verifier: "v"},
	}, nil)
	svc.EXPECT().SealFlowState(gomock.Any()).Return("sealed-blob", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/user.oidc.start", nil)
	rec := httptest.NewRecorder()
	h.Start(rec, req)

	res := rec.Result()
	assert.Equal(t, http.StatusFound, res.StatusCode)
	assert.Equal(t, "https://idp.example.com/authorize?state=x", res.Header.Get("Location"))
	c := cookieByName(res, "__Host-oidc_flow")
	require.NotNil(t, c, "flow cookie must be set")
	assert.Equal(t, "sealed-blob", c.Value)
	assert.True(t, c.HttpOnly)
	assert.True(t, c.Secure)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
	assert.Equal(t, "/", c.Path)
	assert.Equal(t, 300, c.MaxAge)
}

func TestOIDCHandler_Start_ProviderUnavailable_RedirectsWithReason(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h, svc, _ := newOIDCTestHandler(t, ctrl, "https://app.example.com")

	// OIDC is enabled (config flag) so the SSO button renders, but the provider is
	// unreachable/not-yet-initialized: BuildAuthURL returns ErrOIDCNotConfigured.
	svc.EXPECT().IsEnabled().Return(true)
	svc.EXPECT().BuildAuthURL(gomock.Any()).Return(nil, domain.ErrOIDCNotConfigured)

	rec := httptest.NewRecorder()
	h.Start(rec, httptest.NewRequest(http.MethodGet, "/api/user.oidc.start", nil))

	res := rec.Result()
	assert.Equal(t, http.StatusFound, res.StatusCode)
	assert.Contains(t, res.Header.Get("Location"), "oidc_error=provider_unavailable",
		"a clicked SSO button with a down provider must report 'provider_unavailable', not generic auth_failed")
}

func TestOIDCHandler_Start_Disabled_503(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h, svc, _ := newOIDCTestHandler(t, ctrl, "https://app.example.com")
	svc.EXPECT().IsEnabled().Return(false)

	rec := httptest.NewRecorder()
	h.Start(rec, httptest.NewRequest(http.MethodGet, "/api/user.oidc.start", nil))

	res := rec.Result()
	assert.Equal(t, http.StatusServiceUnavailable, res.StatusCode)
	assert.Nil(t, cookieByName(res, "__Host-oidc_flow"))
}

func TestOIDCHandler_Start_WrongMethod(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h, _, _ := newOIDCTestHandler(t, ctrl, "https://app.example.com")

	rec := httptest.NewRecorder()
	h.Start(rec, httptest.NewRequest(http.MethodPost, "/api/user.oidc.start", nil))

	res := rec.Result()
	assert.Equal(t, http.StatusFound, res.StatusCode)
	assert.Contains(t, res.Header.Get("Location"), "oidc_error=auth_failed")
}

func TestOIDCHandler_Start_RateLimited(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h, _, rl := newOIDCTestHandler(t, ctrl, "https://app.example.com")
	// Exhaust the policy for the default test client IP.
	for i := 0; i < 10; i++ {
		rl.Allow("oidc:start", "192.0.2.1")
	}
	rec := httptest.NewRecorder()
	h.Start(rec, httptest.NewRequest(http.MethodGet, "/api/user.oidc.start", nil))

	res := rec.Result()
	assert.Equal(t, http.StatusFound, res.StatusCode)
	assert.Contains(t, res.Header.Get("Location"), "oidc_error=rate_limited")
}

// --- Callback ---------------------------------------------------------------

func callbackReq(query string, withFlowCookie bool) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/user.oidc.callback?"+query, nil)
	if withFlowCookie {
		req.AddCookie(&http.Cookie{Name: "__Host-oidc_flow", Value: "sealed"})
	}
	return req
}

func TestOIDCHandler_Callback_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h, svc, _ := newOIDCTestHandler(t, ctrl, "https://app.example.com")

	svc.EXPECT().IsEnabled().Return(true)
	svc.EXPECT().OpenFlowState("sealed").Return(domain.OIDCFlowState{State: "st", Nonce: "n", Verifier: "v"}, nil)
	svc.EXPECT().HandleCallback(gomock.Any(), gomock.Any()).Return("ONETIME", nil)

	rec := httptest.NewRecorder()
	h.Callback(rec, callbackReq("code=abc&state=st", true))

	res := rec.Result()
	assert.Equal(t, http.StatusFound, res.StatusCode)
	loc := res.Header.Get("Location")
	assert.Contains(t, loc, "#oidc_code=ONETIME", "one-time code must be in the URL fragment")
	// Nothing sensitive in the query string.
	beforeHash := strings.SplitN(loc, "#", 2)[0]
	assert.NotContains(t, beforeHash, "code=")
	assert.NotContains(t, beforeHash, "token=")
	assert.NotContains(t, beforeHash, "oidc_error")
	// Flow cookie cleared.
	c := cookieByName(res, "__Host-oidc_flow")
	require.NotNil(t, c)
	assert.True(t, c.MaxAge < 0, "flow cookie must be cleared")
}

func TestOIDCHandler_Callback_MissingFlowCookie_LoginCSRFGuard(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h, svc, _ := newOIDCTestHandler(t, ctrl, "https://app.example.com")
	// IsEnabled is reached; OpenFlowState / HandleCallback must NOT be (no cookie).
	svc.EXPECT().IsEnabled().Return(true)

	rec := httptest.NewRecorder()
	h.Callback(rec, callbackReq("code=abc&state=st", false)) // no flow cookie

	res := rec.Result()
	assert.Equal(t, http.StatusFound, res.StatusCode)
	loc := res.Header.Get("Location")
	assert.Contains(t, loc, "oidc_error=auth_failed")
	assert.NotContains(t, loc, "#oidc_code=", "no one-time code may be issued without a flow cookie")
}

func TestOIDCHandler_Callback_IdPError_NoLeak(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h, svc, _ := newOIDCTestHandler(t, ctrl, "https://app.example.com")
	svc.EXPECT().IsEnabled().Return(true)
	// HandleCallback must NOT be called when the IdP returned an error.

	rec := httptest.NewRecorder()
	h.Callback(rec, callbackReq("error=access_denied&error_description=secret-detail", true))

	res := rec.Result()
	loc := res.Header.Get("Location")
	assert.Contains(t, loc, "oidc_error=auth_failed")
	assert.NotContains(t, loc, "secret-detail")
}

func TestOIDCHandler_Callback_ErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		svcErr error
		reason string
	}{
		{"not provisioned", domain.ErrOIDCAccountNotProvisioned, "not_provisioned"},
		{"link conflict", domain.ErrOIDCIdentityConflict, "link_conflict"},
		{"email unverified", domain.ErrOIDCEmailNotVerified, "email_unverified"},
		// Its own reason so the console can say what happened; it used to fall through to
		// auth_failed, whose "use a magic code" advice cannot work for someone with no account.
		{"domain not allowed", domain.ErrOIDCDomainNotAllowed, "domain_not_allowed"},
		{"provider unavailable", domain.ErrOIDCNotConfigured, "provider_unavailable"},
		{"generic leaks nothing", errors.New("pq: password=topsecret dsn leak"), "auth_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			h, svc, _ := newOIDCTestHandler(t, ctrl, "https://app.example.com")
			svc.EXPECT().IsEnabled().Return(true)
			svc.EXPECT().OpenFlowState("sealed").Return(domain.OIDCFlowState{}, nil)
			svc.EXPECT().HandleCallback(gomock.Any(), gomock.Any()).Return("", tc.svcErr)

			rec := httptest.NewRecorder()
			h.Callback(rec, callbackReq("code=abc&state=st", true))

			res := rec.Result()
			assert.Equal(t, http.StatusFound, res.StatusCode, "never 500")
			loc := res.Header.Get("Location")
			assert.Contains(t, loc, "oidc_error="+tc.reason)
			assert.NotContains(t, loc, "topsecret", "raw error must never leak")
			// Flow cookie cleared even on error.
			c := cookieByName(res, "__Host-oidc_flow")
			require.NotNil(t, c)
			assert.True(t, c.MaxAge < 0)
		})
	}
}

// --- Exchange ---------------------------------------------------------------

func TestOIDCHandler_Exchange_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h, svc, _ := newOIDCTestHandler(t, ctrl, "https://app.example.com")

	resp := &domain.AuthResponse{Token: "minted-jwt", User: domain.User{ID: "u1", Email: "u1@corp.com"}}
	svc.EXPECT().ExchangeCode(gomock.Any(), "the-code").Return(resp, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/user.oidc.exchange", strings.NewReader(`{"code":"the-code"}`))
	rec := httptest.NewRecorder()
	h.Exchange(rec, req)

	res := rec.Result()
	assert.Equal(t, http.StatusOK, res.StatusCode)
	body := rec.Body.String()
	assert.Contains(t, body, `"token":"minted-jwt"`)
	assert.Contains(t, body, `"expires_at"`)
}

func TestOIDCHandler_Exchange_MissingCode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h, _, _ := newOIDCTestHandler(t, ctrl, "https://app.example.com")

	req := httptest.NewRequest(http.MethodPost, "/api/user.oidc.exchange", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.Exchange(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Result().StatusCode)
}

func TestOIDCHandler_Exchange_InvalidCode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h, svc, _ := newOIDCTestHandler(t, ctrl, "https://app.example.com")
	svc.EXPECT().ExchangeCode(gomock.Any(), "bad").Return(nil, errors.New("invalid or expired code"))

	req := httptest.NewRequest(http.MethodPost, "/api/user.oidc.exchange", strings.NewReader(`{"code":"bad"}`))
	rec := httptest.NewRecorder()
	h.Exchange(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Result().StatusCode)
}

func TestOIDCHandler_Exchange_WrongMethod(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h, _, _ := newOIDCTestHandler(t, ctrl, "https://app.example.com")
	rec := httptest.NewRecorder()
	h.Exchange(rec, httptest.NewRequest(http.MethodGet, "/api/user.oidc.exchange", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Result().StatusCode)
}

func TestOIDCHandler_Exchange_RateLimited(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h, _, rl := newOIDCTestHandler(t, ctrl, "https://app.example.com")
	for i := 0; i < 10; i++ {
		rl.Allow("oidc:exchange", "192.0.2.1")
	}
	req := httptest.NewRequest(http.MethodPost, "/api/user.oidc.exchange", strings.NewReader(`{"code":"x"}`))
	rec := httptest.NewRecorder()
	h.Exchange(rec, req)

	res := rec.Result()
	assert.Equal(t, http.StatusTooManyRequests, res.StatusCode)
	assert.NotEmpty(t, res.Header.Get("Retry-After"))
}

// --- dev-mode cookie naming -------------------------------------------------

func TestOIDCHandler_DevMode_NonHostCookieNames(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h, svc, _ := newOIDCTestHandler(t, ctrl, "http://localhost:3000") // not https
	svc.EXPECT().IsEnabled().Return(true)
	svc.EXPECT().BuildAuthURL(gomock.Any()).Return(&domain.OIDCAuthRequest{AuthURL: "http://idp/authorize"}, nil)
	svc.EXPECT().SealFlowState(gomock.Any()).Return("blob", nil)

	rec := httptest.NewRecorder()
	h.Start(rec, httptest.NewRequest(http.MethodGet, "/api/user.oidc.start", nil))

	res := rec.Result()
	assert.Nil(t, cookieByName(res, "__Host-oidc_flow"), "no __Host- prefix over http")
	c := cookieByName(res, "oidc_flow")
	require.NotNil(t, c)
	assert.False(t, c.Secure, "Secure must be false in dev http")
}
