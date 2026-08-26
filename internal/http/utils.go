package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
)

// WriteJSONError writes a JSON error response with the given message and status code.
// It sets the Content-Type header to application/json and automatically formats
// the response as {"error": "message"}.
func WriteJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// writeJSON writes a JSON response with the given status code and data.
// It sets the Content-Type header to application/json.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writePermissionError answers a caller that may not do what it asked, reporting
// whether it wrote the response: 401 when the credential itself is dead, 403 when
// it is alive but lacks the grant.
//
// The revoked-key mapping lives here rather than one level up in writeServiceError
// because most handlers reach for this helper and nothing else — transactional
// notifications, custom events, broadcasts, automations, the contact timeline,
// message history, blog themes and web analytics all do. Mapping it only in
// writeServiceError left every one of those answering a dead credential with a
// 500, which is the case that matters most: those are the endpoints an API key
// actually calls.
//
// errors.Is/errors.As rather than type assertions: services wrap errors on their
// way up — the authenticate step that sits one line above every permission check
// already does — and a bare assertion would silently degrade a wrapped denial
// into an opaque 500.
//
// The 403 body carries the resource and permission alongside the message, so a
// client can tell which grant is missing without parsing prose. Both fields are
// additive: the "error" key keeps the shape every existing caller already reads.
func writePermissionError(w http.ResponseWriter, err error) bool {
	// Authentication before authorization: a revoked key is not "you may not do
	// that", it is "this credential is dead", and only 401 says so. Clients act
	// on the difference — Zapier raises a reconnect prompt on 401 and treats
	// anything else as a fault of ours, so the 500 this used to be left every Zap
	// failing with an error its owner could not act on.
	//
	// One handler did worse than 500. transactional.send classifies its failures
	// by substring, and a revoked key arrives as "api key has been revoked: user
	// not found" — so it matched "not found" and answered 400 with that internal
	// string in the response body. Catching it here, before any caller sees the
	// error, is what keeps prose matching from reading authentication failures.
	if errors.Is(err, domain.ErrAPIKeyRevoked) {
		WriteJSONError(w, "API key has been revoked", http.StatusUnauthorized)
		return true
	}
	var permErr *domain.PermissionError
	if errors.As(err, &permErr) {
		writeJSON(w, http.StatusForbidden, map[string]interface{}{
			"error":      permErr.Error(),
			"resource":   permErr.Resource,
			"permission": permErr.Permission,
		})
		return true
	}
	return false
}

// writeServiceError maps the authorization and lookup errors a service can return
// to HTTP status codes, writing the response and reporting whether it handled the
// error. A dead credential answers 401 and a permission denial 403 — both through
// writePermissionError, so the two helpers can never disagree — an authorization
// failure (not a member / not an owner) answers 403 rather than a generic 500, and
// a missing row answers 404.
// It unwraps via errors.As/errors.Is, so it still matches when the service wrapped
// the error on its way up (e.g. "failed to authenticate user: %w").
//
// fallback is the message sent when the matched denial carries none of its own —
// an ErrUnauthorized built without a Message would otherwise answer 403 with an
// empty error string.
//
// Unrecognized errors are left untouched and reported as unhandled, so the caller
// keeps its own mapping (usually a method-specific 500).
func writeServiceError(w http.ResponseWriter, err error, fallback string) bool {
	if writePermissionError(w, err) {
		return true
	}
	var notFound *domain.ErrWorkspaceNotFound
	if errors.As(err, &notFound) {
		WriteJSONError(w, "Workspace not found", http.StatusNotFound)
		return true
	}
	// Deleting a row that is already gone has reached the state the caller asked
	// for, so it must not read as a server fault. The Zapier app deletes its
	// subscription on every Zap turn-off and the row can legitimately have been
	// removed by hand in the console first; a 500 there made turning a Zap off
	// fail permanently.
	if errors.Is(err, domain.ErrWebhookSubscriptionNotFound) {
		WriteJSONError(w, "Webhook subscription not found", http.StatusNotFound)
		return true
	}
	var listNotFound *domain.ErrListNotFound
	if errors.As(err, &listNotFound) {
		WriteJSONError(w, listNotFound.Error(), http.StatusNotFound)
		return true
	}
	// Mapped here rather than in each segment handler because a missing segment is
	// a class of error, not a property of one endpoint: every segment method wraps
	// its repository error in "failed to X segment: %w", so every one of them had
	// to see through the wrap, and four of the five did not. A fixed message rather
	// than the error's own, which names internal ids.
	var segmentNotFound *domain.ErrSegmentNotFound
	if errors.As(err, &segmentNotFound) {
		WriteJSONError(w, "Segment not found", http.StatusNotFound)
		return true
	}
	var unauthorized *domain.ErrUnauthorized
	if errors.As(err, &unauthorized) {
		message := unauthorized.Message
		if message == "" {
			message = fallback
		}
		WriteJSONError(w, message, http.StatusForbidden)
		return true
	}
	if errors.Is(err, domain.ErrUserNotInWorkspace) {
		WriteJSONError(w, "You do not have access to this workspace", http.StatusForbidden)
		return true
	}
	return false
}

// redactWorkspaceForCaller strips a workspace of what the caller must not see.
// Every endpoint that serialises a Workspace goes through it, so the rule lives
// in one place instead of being re-derived per handler.
//
// Two layers, because two very different callers ask for the same object.
// Workspace.Redact drops the decrypted integration credentials for everyone — no
// client has ever needed them. The S3 file-manager secret is the single credential
// Redact deliberately keeps, and it keeps it for exactly one reader: the console
// builds an S3 client in the browser from that field and talks to the bucket
// directly, so blanking it unconditionally would break the file manager rather
// than harden anything.
//
// That reader is always a console session. An API key authenticates the very same
// endpoints — user.me and workspaces.list both answer a bearer token, and neither
// consults a permission — so a Zap, an SDK or any integration platform received a
// live bucket credential it has no use for, in a body those platforms routinely
// log whole. Gating on the caller closes that without a second endpoint or a
// console change.
//
// Fails closed: a request that cannot prove it is a console session is treated as
// machine traffic.
func redactWorkspaceForCaller(ctx context.Context, workspace *domain.Workspace) {
	workspace.Redact()
	if !isConsoleSession(ctx) {
		workspace.RedactFileManagerSecret()
	}
}

// isConsoleSession reports whether the request authenticated as a user session
// rather than an API key.
//
// RequireAuth rejects a token that carries no type, so the value is present on
// every authenticated request; an absent one means the caller never went through
// that middleware and has proven nothing.
func isConsoleSession(ctx context.Context) bool {
	userType, _ := ctx.Value(domain.UserTypeKey).(string)
	return userType == string(domain.UserTypeUser)
}
