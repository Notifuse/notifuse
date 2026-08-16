package http

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/botdetection"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// webTrackMaxBodyBytes bounds a beat: 1000 actions of modest paths fit well
// under 1MB; anything larger is hostile.
const webTrackMaxBodyBytes = 1 << 20

// WebAnalyticsHandler serves the public tracking endpoint and the embedded
// browser SDK. The console-facing RPCs (backfill control) are registered here
// too, behind auth.
type WebAnalyticsHandler struct {
	service      domain.WebAnalyticsService
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)

	sdkJS   []byte
	sdkHash string

	// Encodings and validators are precomputed: the bundle is fixed for the
	// process lifetime, so compressing per request would burn CPU on the
	// busiest public route for an identical result. The ETags are strong and
	// distinct per coding — one tag naming two byte sequences is what lets a
	// cache hand a client the representation it did not ask for.
	sdkGzip     []byte
	sdkETag     string
	sdkGzipETag string
}

// NewWebAnalyticsHandler creates the handler. sdkJS may be nil until the SDK
// build is embedded; the SDK routes are only registered when it is present.
func NewWebAnalyticsHandler(svc domain.WebAnalyticsService, getJWTSecret func() ([]byte, error), log logger.Logger, sdkJS []byte) *WebAnalyticsHandler {
	h := &WebAnalyticsHandler{service: svc, getJWTSecret: getJWTSecret, logger: log, sdkJS: sdkJS}
	if len(sdkJS) > 0 {
		sum := sha256.Sum256(sdkJS)
		h.sdkHash = hex.EncodeToString(sum[:])[:12]
		h.sdkETag = `"` + h.sdkHash + `"`
		h.sdkGzipETag = `"` + h.sdkHash + `-gzip"`
		h.sdkGzip = gzipBytes(sdkJS)
	}
	return h
}

// gzipBytes compresses b once, at construction. It returns nil when
// compression fails or does not shrink the input, in which case the SDK is
// served identity-only — a "compressed" body larger than the original is worse
// than none. Compressing here rather than per request also rules out the two
// classic streaming bugs: a forgotten Close leaves off the CRC32/ISIZE trailer
// and browsers reject the script outright, and an unknown length forces
// chunked encoding on a response whose size we know exactly.
func gzipBytes(b []byte) []byte {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil
	}
	if _, err := zw.Write(b); err != nil {
		return nil
	}
	if err := zw.Close(); err != nil {
		return nil
	}
	if buf.Len() >= len(b) {
		return nil
	}
	return buf.Bytes()
}

// SDKHash returns the content hash used in the immutable SDK URL (empty when
// no SDK is embedded).
func (h *WebAnalyticsHandler) SDKHash() string { return h.sdkHash }

// RegisterRoutes registers the public routes and the authenticated RPCs.
func (h *WebAnalyticsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/track", http.HandlerFunc(h.handleTrack))
	if len(h.sdkJS) > 0 {
		mux.Handle("/na.js", http.HandlerFunc(h.handleSDK))
		mux.Handle("/na."+h.sdkHash+".js", http.HandlerFunc(h.handleSDKImmutable))
	}
	if h.getJWTSecret != nil {
		requireAuth := middleware.NewAuthMiddleware(h.getJWTSecret).RequireAuth()
		mux.Handle("/api/webAnalytics.backfillStart", requireAuth(http.HandlerFunc(h.handleBackfillStart)))
		mux.Handle("/api/webAnalytics.backfillStatus", requireAuth(http.HandlerFunc(h.handleBackfillStatus)))
		mux.Handle("/api/webAnalytics.backfillCancel", requireAuth(http.HandlerFunc(h.handleBackfillCancel)))
	}
}

type webAnalyticsBackfillRequest struct {
	WorkspaceID string `json:"workspace_id"`
}

func (h *WebAnalyticsHandler) decodeBackfillRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return "", false
	}
	var req webAnalyticsBackfillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return "", false
	}
	if req.WorkspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return "", false
	}
	return req.WorkspaceID, true
}

func (h *WebAnalyticsHandler) writeBackfillError(w http.ResponseWriter, workspaceID string, err error) {
	if writePermissionError(w, err) {
		return
	}
	h.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Web analytics backfill request failed")
	WriteJSONError(w, err.Error(), http.StatusBadRequest)
}

func (h *WebAnalyticsHandler) handleBackfillStart(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.decodeBackfillRequest(w, r)
	if !ok {
		return
	}
	status, err := h.service.BackfillStart(r.Context(), workspaceID)
	if err != nil {
		h.writeBackfillError(w, workspaceID, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"backfill": status})
}

func (h *WebAnalyticsHandler) handleBackfillStatus(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.decodeBackfillRequest(w, r)
	if !ok {
		return
	}
	status, err := h.service.BackfillStatus(r.Context(), workspaceID)
	if err != nil {
		h.writeBackfillError(w, workspaceID, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"backfill": status})
}

func (h *WebAnalyticsHandler) handleBackfillCancel(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.decodeBackfillRequest(w, r)
	if !ok {
		return
	}
	if err := h.service.BackfillCancel(r.Context(), workspaceID); err != nil {
		h.writeBackfillError(w, workspaceID, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "success"})
}

// handleTrack ingests one beat. Contract (Staminads parity): silently-dropped
// traffic still gets {success:true}; only malformed payloads get a 400; the
// endpoint never surfaces internal errors to the caller.
func (h *WebAnalyticsHandler) handleTrack(w http.ResponseWriter, r *http.Request) {
	// The collect endpoint runs without the global panic protection other
	// (authed) routes get from their middleware; a panic here must not kill
	// the connection with an empty reply browsers would retry.
	defer func() {
		if rec := recover(); rec != nil {
			h.logger.WithField("panic", rec).Error("Panic in /track handler")
			h.writeTrackResponse(w, r, http.StatusOK, true, "")
		}
	}()

	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Beats are sent as text/plain to avoid CORS preflights; decode JSON
	// regardless of Content-Type.
	r.Body = http.MaxBytesReader(w, r.Body, webTrackMaxBodyBytes)
	var payload domain.WebTrackPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		// An oversized body gets its own status. A generic 400 tells a client
		// nothing actionable, and this is the one failure it CAN recover from
		// by trimming its oldest actions or rotating the session — worth saying
		// so, because actions[] only grows and every later beat would fail too.
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			h.writeTrackResponse(w, r, http.StatusRequestEntityTooLarge, false, "payload too large")
			return
		}
		h.writeTrackResponse(w, r, http.StatusBadRequest, false, "invalid JSON payload")
		return
	}

	// Server-side bot filtering: accepted silently, never stored.
	if botdetection.IsBotUserAgent(r.UserAgent()) {
		h.writeTrackResponse(w, r, http.StatusOK, true, "")
		return
	}

	meta := domain.WebRequestMeta{
		Origin:     r.Header.Get("Origin"),
		Referer:    r.Header.Get("Referer"),
		UserAgent:  r.UserAgent(),
		ClientIP:   getClientIP(r),
		ReceivedAt: time.Now().UTC(),
	}

	err := h.service.Track(r.Context(), &payload, meta)
	var invalid *service.ErrWebTrackInvalidPayload
	switch {
	case err == nil:
		h.writeTrackResponse(w, r, http.StatusOK, true, "")
	case errors.As(err, &invalid):
		h.writeTrackResponse(w, r, http.StatusBadRequest, false, invalid.Error())
	default:
		// Internal failure: log it, keep the client oblivious so SDKs don't
		// queue retries for something they cannot fix.
		h.logger.WithField("error", err.Error()).Error("Failed to track web analytics beat")
		h.writeTrackResponse(w, r, http.StatusOK, true, "")
	}
}

// writeTrackResponse sets the per-route CORS headers (overriding whatever the
// global CORS middleware wrote — its origin may be pinned to the console while
// beats come from customer sites, and its Allow-Credentials would make a "*"
// origin invalid) and writes the JSON body.
func (h *WebAnalyticsHandler) writeTrackResponse(w http.ResponseWriter, r *http.Request, status int, success bool, errMsg string) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Del("Access-Control-Allow-Credentials")
	w.Header().Add("Vary", "Origin")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := map[string]interface{}{"success": success}
	if errMsg != "" {
		response["error"] = errMsg
	}
	_ = json.NewEncoder(w).Encode(response)
}

// handleSDK serves the embedded SDK under a stable URL with a short cache.
func (h *WebAnalyticsHandler) handleSDK(w http.ResponseWriter, r *http.Request) {
	h.serveSDK(w, r, "public, max-age=3600")
}

// handleSDKImmutable serves the hash-addressed URL with an immutable cache.
func (h *WebAnalyticsHandler) handleSDKImmutable(w http.ResponseWriter, r *http.Request) {
	h.serveSDK(w, r, "public, max-age=31536000, immutable")
}

// serveSDK writes the precomputed bundle in the encoding the client accepts.
//
// It deliberately does not use http.ServeContent. That would advertise
// Accept-Ranges: bytes unconditionally, exposing 206/416/412 responses this URL
// has never served, and it refuses to set Content-Length once the handler has
// set Content-Encoding. Leaving ranges unadvertised means net/http can never
// return a 206 here, so a slice of the compressed body cannot be spliced into a
// client's identity buffer — the per-coding ETags below close the same hole a
// second time.
func (h *WebAnalyticsHandler) serveSDK(w http.ResponseWriter, r *http.Request, cacheControl string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Negotiate before anything is written, so a HEAD reports the length of the
	// encoding it would actually have sent rather than the identity length.
	body, etag, encoding := h.sdkJS, h.sdkETag, ""
	if h.sdkGzip != nil && acceptsGzip(r.Header.Get("Accept-Encoding")) {
		body, etag, encoding = h.sdkGzip, h.sdkGzipETag, "gzip"
	}

	// Everything a 304 must still carry goes here, ahead of the conditional
	// return. The ACAO/credentials pair overrides the global CORS middleware,
	// whose origin may be pinned to the console while this script loads from
	// customer sites; an exit path that skipped this block would ship that
	// pinned origin plus Allow-Credentials: true to a third party. Vary is
	// added rather than set so a future Vary: Origin in the middleware is not
	// silently erased.
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Del("Access-Control-Allow-Credentials")
	w.Header().Add("Vary", "Accept-Encoding")
	w.Header().Set("ETag", etag)

	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// After the 304 check: net/http strips Content-Type and Content-Length from
	// a 304 but leaves Content-Encoding alone, and a 304 has no body to encode.
	if encoding != "" {
		w.Header().Set("Content-Encoding", encoding)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))

	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}
