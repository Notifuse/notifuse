package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"

	"strings"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/internal/domain"
)

// The audit middleware sits inside the mux, around every route, and records
// the control-plane calls named in domain.AuditedActions once their response
// is written. It is the single place a request becomes an audit row: the auth
// middleware and the services only enrich the AuditRecord it puts in the
// context.
//
// It lives inside the mux rather than in App.Start's chain because the
// integration harness serves App.GetMux() directly; a middleware applied only
// in Start would be invisible to every integration test.

const (
	// RequestIDHeader is echoed on every response, generated when the client
	// sent none, and stored on every audit row so a row can be tied back to an
	// application log line.
	RequestIDHeader = "X-Request-ID"

	// auditBodyPeekLimit bounds how much of a JSON body the middleware reads to
	// find workspace_id and the target id. Bodies above it (an import) are
	// replayed untouched and simply lose target extraction.
	auditBodyPeekLimit = 1 << 20

	// auditErrorBodyLimit bounds how much of a 402/403 body is kept to name the
	// resource, permission or feature that was refused.
	auditErrorBodyLimit = 4 << 10

	// auditResponseBodyLimit bounds how much of a success body is kept when the
	// route's target id only exists in the response (a create that mints its
	// own id). Response bodies for those routes are one object; 64 KB is
	// generous.
	auditResponseBodyLimit = 64 << 10

	apiPrefix = "/api/"
)

// ClientIP returns the caller's address: the first X-Forwarded-For entry, then
// X-Real-IP, then the socket's remote address without its port. The header
// values are client-supplied unless a trusted proxy overwrites them, so a
// header that does not parse as an address is ignored rather than stored —
// otherwise "make my audit row too long to insert" would be one header away.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := parseIP(strings.Split(xff, ",")[0]); ip != "" {
			return ip
		}
	}
	if ip := parseIP(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	remote := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}
	if ip := parseIP(remote); ip != "" {
		return ip
	}
	return remote
}

// parseIP returns the canonical text of value when it is an IP address (with
// or without a port), and "" otherwise.
func parseIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	if parsed := net.ParseIP(value); parsed != nil {
		return parsed.String()
	}
	return ""
}

// auditRoute is what the catalogue says about one request.
type auditRoute struct {
	action      string
	category    string
	targetType  string
	targetKey   string
	responseKey string
}

// classifyAuditRequest answers whether a request is recorded, and as what.
// POST routes are looked up in AuditedActions; GET routes in AuditLoggedReads,
// which may further require export=true and no cursor. Everything else — reads,
// the data plane, OPTIONS, non-API paths — is not recorded.
func classifyAuditRequest(r *http.Request) (auditRoute, bool) {
	if !strings.HasPrefix(r.URL.Path, apiPrefix) {
		return auditRoute{}, false
	}
	route := strings.TrimPrefix(r.URL.Path, apiPrefix)
	switch r.Method {
	case http.MethodPost:
		spec, ok := domain.AuditedActions[route]
		if !ok {
			return auditRoute{}, false
		}
		return auditRoute{action: route, category: spec.Category, targetType: spec.TargetType, targetKey: spec.TargetKey, responseKey: spec.ResponseKey}, true
	case http.MethodGet:
		read, ok := domain.AuditLoggedReads[route]
		if !ok {
			return auditRoute{}, false
		}
		if read.RequireExport {
			query := r.URL.Query()
			if query.Get("export") != "true" || query.Get("cursor") != "" {
				return auditRoute{}, false
			}
		}
		return auditRoute{action: read.Action, category: read.Category}, true
	default:
		return auditRoute{}, false
	}
}

// NewAuditMiddleware returns the middleware. The recorder decides whether a
// row is actually written (the licence gate lives there); the middleware
// decides what the row says.
func NewAuditMiddleware(recorder domain.AuditLogRecorder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get(RequestIDHeader)
			if requestID == "" || len(requestID) > 64 {
				requestID = uuid.New().String()
			}
			w.Header().Set(RequestIDHeader, requestID)

			record := domain.NewAuditRecord(ClientIP(r), r.UserAgent(), requestID)
			r = r.WithContext(domain.WithAuditRecord(r.Context(), record))

			route, audited := classifyAuditRequest(r)
			if !audited {
				next.ServeHTTP(w, r)
				return
			}

			if r.Method == http.MethodPost {
				peekAuditBody(r, record, route)
			}

			writer := &auditResponseWriter{ResponseWriter: w, status: http.StatusOK, captureSuccess: route.responseKey != ""}
			next.ServeHTTP(writer, r)

			// A create that minted its own id names it only in the response. A
			// service that already named the target wins.
			if route.responseKey != "" && writer.status >= 200 && writer.status < 300 && !record.HasTargetID() {
				if id := lookupJSONPath(writer.successBody.Bytes(), route.responseKey); id != "" {
					record.SetTarget(route.targetType, id, "")
				}
			}

			// An anonymous 401 is an expired or missing credential, not an action:
			// recording it would fill the log with every stale console tab, and it
			// is also what keeps an unauthenticated scanner from writing rows. A
			// 401 a service marked as failed is different — a wrong code, a bad
			// root signature — and is exactly the kind of row the log is for.
			if writer.status == http.StatusUnauthorized && !record.Failed() {
				return
			}

			event, skipped := record.Finalize(writer.status)
			if skipped {
				return
			}
			event.Action = route.action
			event.Category = route.category
			if event.TargetType == "" {
				event.TargetType = route.targetType
			}
			if writer.status == http.StatusPaymentRequired || writer.status == http.StatusForbidden {
				annotateRefusal(&event, writer.status, writer.errorBody.Bytes())
			}
			recorder.Record(r.Context(), &event)
		})
	}
}

// peekAuditBody reads the JSON body far enough to find workspace_id and the
// route's target key, then puts every byte back for the handler. A body above
// the limit is replayed whole but not decoded.
func peekAuditBody(r *http.Request, record *domain.AuditRecord, route auditRoute) {
	if r.Body == nil || r.Body == http.NoBody {
		return
	}
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if contentType != "" && !strings.Contains(contentType, "json") {
		return
	}

	limited := io.LimitReader(r.Body, auditBodyPeekLimit+1)
	buffered, err := io.ReadAll(limited)
	// Whatever was read is replayed ahead of whatever was not, so the handler
	// sees the body it would have seen.
	r.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(buffered), r.Body), r.Body}
	if err != nil || len(buffered) > auditBodyPeekLimit {
		return
	}

	if workspaceID := lookupJSONPath(buffered, "workspace_id"); workspaceID != "" {
		record.SetWorkspace(workspaceID)
	}
	if route.targetKey == "" {
		return
	}
	targetID := lookupJSONPath(buffered, route.targetKey)
	if targetID == "" {
		return
	}
	record.SetTarget(route.targetType, targetID, "")
	if route.targetType == domain.AuditTargetWorkspace {
		record.SetWorkspace(targetID)
	}
}

// lookupJSONPath reads a scalar at a dotted path ("automation.id") out of a
// JSON object, decoding one level at a time so a large body is never fully
// materialised. Anything that is not an object along the way, or not a
// scalar at the end, yields "".
func lookupJSONPath(raw []byte, path string) string {
	if len(raw) == 0 || path == "" {
		return ""
	}
	current := json.RawMessage(raw)
	for _, segment := range strings.Split(path, ".") {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(current, &object); err != nil {
			return ""
		}
		next, ok := object[segment]
		if !ok {
			return ""
		}
		current = next
	}
	return jsonScalar(current)
}

// jsonScalar renders a string or number JSON value as text; anything else is
// not a usable id.
func jsonScalar(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	var asNumber json.Number
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		return asNumber.String()
	}
	return ""
}

// annotateRefusal copies what the 402/403 body says was refused into the
// row's metadata: resource and permission for a 403 (see writePermissionError),
// feature and required_tier for a 402 (see writeLicenseRequired).
func annotateRefusal(event *domain.AuditLog, status int, body []byte) {
	if event.Metadata == nil {
		event.Metadata = make(map[string]any)
	}
	if status == http.StatusPaymentRequired {
		event.Metadata["licence_refused"] = true
	}
	if len(body) == 0 {
		return
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return
	}
	for _, key := range []string{"resource", "permission", "feature", "required_tier"} {
		if value, ok := decoded[key].(string); ok && value != "" {
			event.Metadata[key] = value
		}
	}
}

// auditResponseWriter captures the status and, for refusals, the start of
// the body — and, for the routes whose target id is minted by the server,
// the start of a success body. It forwards Flush so streaming handlers keep
// streaming.
type auditResponseWriter struct {
	http.ResponseWriter
	status         int
	wroteHeader    bool
	captureSuccess bool
	errorBody      bytes.Buffer
	successBody    bytes.Buffer
}

func (w *auditResponseWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.status = status
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *auditResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.status = http.StatusOK
	}
	if (w.status == http.StatusPaymentRequired || w.status == http.StatusForbidden) && w.errorBody.Len() < auditErrorBodyLimit {
		room := auditErrorBodyLimit - w.errorBody.Len()
		if room > len(b) {
			room = len(b)
		}
		w.errorBody.Write(b[:room])
	}
	if w.captureSuccess && w.status >= 200 && w.status < 300 && w.successBody.Len() < auditResponseBodyLimit {
		room := auditResponseBodyLimit - w.successBody.Len()
		if room > len(b) {
			room = len(b)
		}
		w.successBody.Write(b[:room])
	}
	return w.ResponseWriter.Write(b)
}

// Flush implements http.Flusher for the handlers that stream (LLM chat, the
// setup wizard, the SMTP test, the audit export).
func (w *auditResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		if !w.wroteHeader {
			w.wroteHeader = true
			w.status = http.StatusOK
		}
		flusher.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *auditResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

var _ http.Flusher = (*auditResponseWriter)(nil)
