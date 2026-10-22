package http

import (
	"net/http"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

const (
	// UsageTimestampHeader carries the Unix seconds the request was signed at.
	UsageTimestampHeader = "X-Notifuse-Timestamp"
	// UsageSignatureHeader carries `v1,{base64(HMAC-SHA256)}`.
	UsageSignatureHeader = "X-Notifuse-Signature"
)

// UsageHandler answers signed reads of this installation's metered usage.
//
// The caller is the control plane, which pulls rather than being pushed to. That
// direction is the whole design: a self-hosted installation never initiates an
// outbound call, because there is no code here that makes one — only this
// endpoint, which answers when asked with a valid signature.
//
// The route is registered on the bare mux with no JWT middleware, because the
// caller is a machine with no user session. It is authenticated by signature
// instead, and the signing key is derived from SECRET_KEY (see
// domain.UsageSigningKey), so nothing new has to be generated or distributed.
// It is reachable from the internet like every other route on this mux, so the
// signature is what protects it — never its network position.
type UsageHandler struct {
	usageService domain.UsageService
	secretKey    string
	logger       logger.Logger
	nowFn        func() time.Time
}

// NewUsageHandler creates the usage handler.
func NewUsageHandler(usageService domain.UsageService, secretKey string, log logger.Logger) *UsageHandler {
	return &UsageHandler{
		usageService: usageService,
		secretKey:    secretKey,
		logger:       log,
		nowFn:        time.Now,
	}
}

// RegisterRoutes registers the signed usage read.
func (h *UsageHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/api/usage.get", http.HandlerFunc(h.handleGet))
}

func (h *UsageHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// An installation with no SECRET_KEY cannot verify anything, so it must refuse
	// rather than derive a key from the empty string and accept whatever matches
	// it. Config makes SECRET_KEY mandatory, so this is defence in depth.
	if h.secretKey == "" {
		WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	timestamp, err := domain.ParseUsageTimestamp(r.Header.Get(UsageTimestampHeader))
	if err != nil {
		WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	key := domain.UsageSigningKey(h.secretKey)
	if err := domain.VerifyUsageSignature(key, timestamp, r.URL.Path,
		r.Header.Get(UsageSignatureHeader), h.nowFn()); err != nil {
		WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// The open month and the one before it — the two the meter keeps refreshed,
	// and the two the two-consecutive-months rule needs. Derived here rather than
	// taken from query parameters: there is nothing for a caller to choose, and a
	// month range from the request would be a way to ask this endpoint for
	// arbitrary work.
	now := h.nowFn().UTC()
	currentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	months := []time.Time{currentMonth.AddDate(0, -1, 0), currentMonth}

	report, err := h.usageService.GetUsage(r.Context(), months)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to read usage")
		// 500, never 403: there is no permission check here, and being over a
		// quota is not a permission state. A caller that gets an error must read
		// it as "no usage reported", not as "this installation denied me".
		WriteJSONError(w, "Failed to read usage", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, report)
}
