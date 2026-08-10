package http

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
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
}

// NewWebAnalyticsHandler creates the handler. sdkJS may be nil until the SDK
// build is embedded; the SDK routes are only registered when it is present.
func NewWebAnalyticsHandler(svc domain.WebAnalyticsService, getJWTSecret func() ([]byte, error), log logger.Logger, sdkJS []byte) *WebAnalyticsHandler {
	h := &WebAnalyticsHandler{service: svc, getJWTSecret: getJWTSecret, logger: log, sdkJS: sdkJS}
	if len(sdkJS) > 0 {
		sum := sha256.Sum256(sdkJS)
		h.sdkHash = hex.EncodeToString(sum[:])[:12]
	}
	return h
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
	if _, ok := err.(*domain.PermissionError); ok {
		WriteJSONError(w, err.Error(), http.StatusForbidden)
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

func (h *WebAnalyticsHandler) serveSDK(w http.ResponseWriter, r *http.Request, cacheControl string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Del("Access-Control-Allow-Credentials")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(h.sdkJS)
}
