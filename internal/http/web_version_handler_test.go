package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeWebVersion resolves a token to the message msg-<token> and renders it, unless the
// token is mapped to an error.
type fakeWebVersion struct {
	resolveErrs map[string]error
	renderErr   error
	html        string
	resolved    []string
	hosts       []string
	rendered    int
}

func (f *fakeWebVersion) Resolve(_ context.Context, token, requestHost string) (service.WebVersionRef, error) {
	f.resolved = append(f.resolved, token)
	f.hosts = append(f.hosts, requestHost)
	if err, ok := f.resolveErrs[token]; ok {
		return service.WebVersionRef{}, err
	}
	return service.WebVersionRef{WorkspaceID: "ws1", MessageID: "msg-" + token}, nil
}

func (f *fakeWebVersion) Render(_ context.Context, _ service.WebVersionRef) (string, error) {
	f.rendered++
	if f.renderErr != nil {
		return "", f.renderErr
	}
	return f.html, nil
}

func newWebVersionTestHandler(t *testing.T, renderer *fakeWebVersion, ipLimit, messageLimit int) *http.ServeMux {
	t.Helper()
	rl := ratelimiter.NewRateLimiter()
	t.Cleanup(rl.Stop)
	rl.SetPolicy(WebVersionIPPolicy, ipLimit, time.Minute)
	rl.SetPolicy(WebVersionMessagePolicy, messageLimit, time.Minute)
	mux := http.NewServeMux()
	NewWebVersionHandler(renderer, rl).RegisterRoutes(mux)
	return mux
}

func serveWebVersion(mux *http.ServeMux, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func assertWebVersionSecurityHeaders(t *testing.T, h http.Header) {
	t.Helper()
	csp := h.Get("Content-Security-Policy")
	assert.Equal(t, "sandbox allow-popups allow-popups-to-escape-sandbox allow-downloads; default-src 'none'; img-src * data:; style-src 'unsafe-inline' *; font-src * data:; form-action 'none'; frame-ancestors 'none'; base-uri 'none'", csp)
	sandbox := strings.SplitN(csp, ";", 2)[0]
	assert.NotContains(t, sandbox, "allow-scripts", "a script in a contact field would run on the console origin")
	assert.NotContains(t, sandbox, "allow-same-origin", "the page must keep an opaque origin")
	assert.Equal(t, "text/html; charset=utf-8", h.Get("Content-Type"))
	assert.Equal(t, "nosniff", h.Get("X-Content-Type-Options"))
	assert.Equal(t, "no-referrer", h.Get("Referrer-Policy"))
	assert.Equal(t, "private, no-store", h.Get("Cache-Control"))
	assert.Equal(t, "noindex, nofollow", h.Get("X-Robots-Tag"))
}

func TestWebVersionHandler_ServesTheEmail(t *testing.T) {
	renderer := &fakeWebVersion{html: "<html>Hello Camille</html>"}
	mux := newWebVersionTestHandler(t, renderer, 100, 100)

	rec := serveWebVersion(mux, http.MethodGet, "/v/good", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "<html>Hello Camille</html>", rec.Body.String())
	assertWebVersionSecurityHeaders(t, rec.Header())
}

func TestWebVersionHandler_ResolvesForTheRequestedHost(t *testing.T) {
	// The service refuses a web version on another workspace's custom domain, so it
	// needs the host the reader asked for.
	renderer := &fakeWebVersion{html: "<html>ok</html>"}
	mux := newWebVersionTestHandler(t, renderer, 100, 100)

	req := httptest.NewRequest(http.MethodGet, "/v/good", nil)
	req.Host = "mail.brand-a.example:8443"
	mux.ServeHTTP(httptest.NewRecorder(), req)
	assert.Equal(t, []string{"mail.brand-a.example:8443"}, renderer.hosts)
}

func TestWebVersionHandler_IgnoresTheQueryString(t *testing.T) {
	// The link reaches the email with UTM parameters appended.
	renderer := &fakeWebVersion{html: "<html>ok</html>"}
	mux := newWebVersionTestHandler(t, renderer, 100, 100)

	rec := serveWebVersion(mux, http.MethodGet, "/v/good?utm_source=newsletter&utm_medium=email", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"good"}, renderer.resolved)
}

func TestWebVersionHandler_Head(t *testing.T) {
	renderer := &fakeWebVersion{html: "<html>ok</html>"}
	mux := newWebVersionTestHandler(t, renderer, 100, 100)

	rec := serveWebVersion(mux, http.MethodHead, "/v/good", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())
	assertWebVersionSecurityHeaders(t, rec.Header())
}

func TestWebVersionHandler_NotFoundIsTheSameWhateverTheCause(t *testing.T) {
	renderer := &fakeWebVersion{
		resolveErrs: map[string]error{"forged": service.ErrWebVersionNotFound},
		html:        "<html>ok</html>",
	}
	mux := newWebVersionTestHandler(t, renderer, 100, 100)

	forged := serveWebVersion(mux, http.MethodGet, "/v/forged", nil)
	empty := serveWebVersion(mux, http.MethodGet, "/v/", nil)
	renderer.renderErr = service.ErrWebVersionNotFound
	erased := serveWebVersion(mux, http.MethodGet, "/v/erased", nil)

	for name, rec := range map[string]*httptest.ResponseRecorder{"forged": forged, "no token": empty, "erased": erased} {
		assert.Equal(t, http.StatusNotFound, rec.Code, name)
		assert.Equal(t, forged.Body.String(), rec.Body.String(), name)
		assertWebVersionSecurityHeaders(t, rec.Header())
	}
	assert.Contains(t, forged.Body.String(), "no longer available")
}

func TestWebVersionHandler_Unavailable(t *testing.T) {
	renderer := &fakeWebVersion{renderErr: service.ErrWebVersionUnavailable}
	mux := newWebVersionTestHandler(t, renderer, 100, 100)

	rec := serveWebVersion(mux, http.MethodGet, "/v/good", nil)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "60", rec.Header().Get("Retry-After"))
	assertWebVersionSecurityHeaders(t, rec.Header())
}

func TestWebVersionHandler_MethodNotAllowed(t *testing.T) {
	renderer := &fakeWebVersion{html: "<html>ok</html>"}
	mux := newWebVersionTestHandler(t, renderer, 100, 100)

	rec := serveWebVersion(mux, http.MethodPost, "/v/good", nil)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, "GET, HEAD", rec.Header().Get("Allow"))
	assert.Empty(t, renderer.resolved)
	assertWebVersionSecurityHeaders(t, rec.Header())
}

func TestWebVersionHandler_PreviewLink(t *testing.T) {
	// A template preview and a test email link to /v/preview: there is no sent message
	// behind them, and the page says so instead of reading as a broken link.
	renderer := &fakeWebVersion{html: "<html>ok</html>"}
	mux := newWebVersionTestHandler(t, renderer, 100, 100)

	rec := serveWebVersion(mux, http.MethodGet, "/v/preview", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "web version of the email")
	assert.Empty(t, renderer.resolved, "the preview page opens no token")
	assertWebVersionSecurityHeaders(t, rec.Header())
}

func TestWebVersionHandler_LimitsEachMessageWhateverTheForwardedAddress(t *testing.T) {
	renderer := &fakeWebVersion{html: "<html>ok</html>"}
	mux := newWebVersionTestHandler(t, renderer, 100, 2)

	for i := 0; i < 2; i++ {
		rec := serveWebVersion(mux, http.MethodGet, "/v/good", map[string]string{"X-Forwarded-For": "203.0.113." + strconv.Itoa(i)})
		require.Equal(t, http.StatusOK, rec.Code)
	}
	rec := serveWebVersion(mux, http.MethodGet, "/v/good", map[string]string{"X-Forwarded-For": "203.0.113.99"})
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "60", rec.Header().Get("Retry-After"))
	assertWebVersionSecurityHeaders(t, rec.Header())
	assert.Equal(t, 2, renderer.rendered, "a limited request compiles nothing")

	other := serveWebVersion(mux, http.MethodGet, "/v/other", map[string]string{"X-Forwarded-For": "203.0.113.99"})
	assert.Equal(t, http.StatusOK, other.Code, "the limit is per message")
}

func TestWebVersionHandler_LimitsEachAddress(t *testing.T) {
	renderer := &fakeWebVersion{html: "<html>ok</html>"}
	mux := newWebVersionTestHandler(t, renderer, 1, 100)

	first := serveWebVersion(mux, http.MethodGet, "/v/good", map[string]string{"X-Forwarded-For": "203.0.113.7"})
	require.Equal(t, http.StatusOK, first.Code)
	rec := serveWebVersion(mux, http.MethodGet, "/v/other", map[string]string{"X-Forwarded-For": "203.0.113.7"})
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Len(t, renderer.resolved, 1, "a limited address opens no token")
}

func TestWebVersionHandler_LimitIsNotSpentByInvalidTokens(t *testing.T) {
	// The per-message limit applies after verification, so forged tokens cannot exhaust
	// a real message's budget.
	renderer := &fakeWebVersion{
		resolveErrs: map[string]error{"forged": service.ErrWebVersionNotFound},
		html:        "<html>ok</html>",
	}
	mux := newWebVersionTestHandler(t, renderer, 100, 1)

	for i := 0; i < 3; i++ {
		serveWebVersion(mux, http.MethodGet, "/v/forged", nil)
	}
	rec := serveWebVersion(mux, http.MethodGet, "/v/good", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}
