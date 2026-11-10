package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
)

// Rate limit namespaces of the web version route. The address is the client-supplied
// first X-Forwarded-For entry unless a proxy overwrites it, so it cannot be the only
// bound: the per-message limit, applied once the token verifies, is the one an attacker
// cannot vary.
const (
	WebVersionIPPolicy      = "webversion:ip"
	WebVersionMessagePolicy = "webversion:message"
)

// webVersionCSP is fixed for every workspace. The email is author HTML with contact
// fields inserted unescaped, served on the console's origin, where the console keeps
// its session token: the page must run no script and must not share that origin.
// sandbox without allow-scripts and allow-same-origin does both; allow-popups keeps the
// target="_blank" links MJML writes on buttons and images working,
// allow-popups-to-escape-sandbox opens their destination as a normal page, and
// allow-downloads keeps links that download a file (a calendar invite, a PDF) working.
const webVersionCSP = "sandbox allow-popups allow-popups-to-escape-sandbox allow-downloads; default-src 'none'; img-src * data:; style-src 'unsafe-inline' *; font-src * data:; form-action 'none'; frame-ancestors 'none'; base-uri 'none'"

const webVersionNotFoundPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Email not available</title>` +
	`<style>body{font-family:system-ui,sans-serif;color:#374151;display:flex;min-height:100vh;margin:0;align-items:center;justify-content:center;text-align:center;padding:16px}</style></head>` +
	`<body><p>This email is no longer available.</p></body></html>`

const webVersionUnavailablePage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Email not available</title>` +
	`<style>body{font-family:system-ui,sans-serif;color:#374151;display:flex;min-height:100vh;margin:0;align-items:center;justify-content:center;text-align:center;padding:16px}</style></head>` +
	`<body><p>This email cannot be shown right now. Please try again in a moment.</p></body></html>`

// webVersionPreviewToken is the token of the sample link in template previews and test
// emails, which have no sent message behind them.
const webVersionPreviewToken = "preview"

const webVersionPreviewPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Email preview</title>` +
	`<style>body{font-family:system-ui,sans-serif;color:#374151;display:flex;min-height:100vh;margin:0;align-items:center;justify-content:center;text-align:center;padding:16px}</style></head>` +
	`<body><p>This link opens the web version of the email once it is sent to a contact.<br>It has none in a preview or a test email.</p></body></html>`

// WebVersionRenderer resolves a web version token and renders the email it points to.
type WebVersionRenderer interface {
	Resolve(ctx context.Context, token string) (service.WebVersionRef, error)
	Render(ctx context.Context, ref service.WebVersionRef) (string, error)
}

// WebVersionHandler serves GET /v/<token>, the web version of a broadcast or automation
// email.
type WebVersionHandler struct {
	renderer    WebVersionRenderer
	rateLimiter *ratelimiter.RateLimiter
}

// NewWebVersionHandler builds the handler. The rate limiter must carry the
// WebVersionIPPolicy and WebVersionMessagePolicy policies: without them every request
// is refused.
func NewWebVersionHandler(renderer WebVersionRenderer, rateLimiter *ratelimiter.RateLimiter) *WebVersionHandler {
	return &WebVersionHandler{renderer: renderer, rateLimiter: rateLimiter}
}

// RegisterRoutes registers the public web version route.
func (h *WebVersionHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/v/", http.HandlerFunc(h.handleWebVersion))
}

func (h *WebVersionHandler) handleWebVersion(w http.ResponseWriter, r *http.Request) {
	setWebVersionHeaders(w.Header())
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeWebVersionPage(w, r, http.StatusMethodNotAllowed, webVersionNotFoundPage)
		return
	}

	if !h.rateLimiter.Allow(WebVersionIPPolicy, getClientIP(r)) {
		w.Header().Set("Retry-After", "60")
		writeWebVersionPage(w, r, http.StatusTooManyRequests, webVersionUnavailablePage)
		return
	}

	token := strings.TrimPrefix(r.URL.Path, "/v/")
	if token == "" {
		writeWebVersionPage(w, r, http.StatusNotFound, webVersionNotFoundPage)
		return
	}
	if token == webVersionPreviewToken {
		writeWebVersionPage(w, r, http.StatusOK, webVersionPreviewPage)
		return
	}
	ref, err := h.renderer.Resolve(r.Context(), token)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	if !h.rateLimiter.Allow(WebVersionMessagePolicy, ref.WorkspaceID+"|"+ref.MessageID) {
		w.Header().Set("Retry-After", "60")
		writeWebVersionPage(w, r, http.StatusTooManyRequests, webVersionUnavailablePage)
		return
	}

	html, err := h.renderer.Render(r.Context(), ref)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeWebVersionPage(w, r, http.StatusOK, html)
}

func (h *WebVersionHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, service.ErrWebVersionNotFound) {
		writeWebVersionPage(w, r, http.StatusNotFound, webVersionNotFoundPage)
		return
	}
	w.Header().Set("Retry-After", "60")
	writeWebVersionPage(w, r, http.StatusServiceUnavailable, webVersionUnavailablePage)
}

// setWebVersionHeaders sets the headers every web version response carries, error
// pages included.
func setWebVersionHeaders(h http.Header) {
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", webVersionCSP)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "private, no-store")
	h.Set("X-Robots-Tag", "noindex, nofollow")
}

func writeWebVersionPage(w http.ResponseWriter, r *http.Request, status int, body string) {
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(body))
}
