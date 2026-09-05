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
// whether it wrote the response: 401 when the credential itself is dead, 402 when
// the deployment has not bought the capability, 403 when the credential is alive
// and the deployment is licensed but the caller lacks the grant.
//
// The revoked-key and licence mappings live here rather than one level up in
// writeServiceError because most handlers reach for this helper and nothing else —
// transactional notifications, custom events, broadcasts, automations, the contact
// timeline, message history, blog themes and web analytics all do. Mapping the
// revoked key only in writeServiceError left every one of those answering a dead
// credential with a 500, which is the case that matters most: those are the
// endpoints an API key actually calls. The licence refusals are placed at the same
// level for the same reason, and because writeServiceError delegates here on its
// first line, one branch covers both helpers — which is what lets the licence gates
// reach twenty-four handler files without a single handler being edited.
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
	// A licence refusal is answered before a permission denial because the two are
	// different questions and only the status code separates them for a client: 403
	// says the signed-in user lacks a grant, which no amount of money fixes, while
	// 402 says the user's permissions are fine and the deployment has not bought the
	// capability. The console renders one purchase component keyed on 402 alone,
	// rather than pattern-matching prose out of a 403 body.
	//
	// After the revoked-key check and never before it: authentication comes first,
	// and a dead credential must be told to reconnect, not told to go and buy
	// something it already paid for.
	var notLicensed *domain.ErrFeatureNotLicensed
	if errors.As(err, &notLicensed) {
		writeLicenseRequired(w, string(notLicensed.Feature), notLicensed.RequiredTier, notLicensed.Message)
		return true
	}
	// The workspace ceiling is a licence refusal too, and deliberately a different
	// error from ErrWorkspaceLimitReached: that one is the operator's own
	// PLAN_MAX_WORKSPACES and keeps its 403, because nothing is for sale there.
	//
	// It carries no required_tier. Which plan lifts the ceiling depends on how many
	// workspaces the deployment already holds — one sitting at eight needs the
	// fifteen-workspace plan, not the five — so naming the cheapest would advertise a
	// licence that would not actually let them create the next one.
	var quotaReached *domain.ErrWorkspaceQuotaReached
	if errors.As(err, &quotaReached) {
		writeLicenseRequired(w, licenseQuotaFeature, "", quotaReached.Error())
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

// The two values in a 402 body that are contracts rather than prose.
//
// licenseRequiredCode is what the console switches on. licenseDocsURL is the page
// the Additional Use Grant pins by URL and by version, which is also why no
// endpoint here serves the feature matrix: a second copy would drift from the text
// that is legally binding.
//
// The console renders a single component for every licence refusal, so these are a
// contract with it as much as with the AUG.
const (
	licenseRequiredCode = "license_required"
	licenseDocsURL      = "https://notifuse.com/licence-features"

	// licenseQuotaFeature stands in for a domain.Feature the workspace ceiling does
	// not have: the quota travels in the key's max_ws field, not as an entry in its
	// feature list. The body still needs a name for what was refused.
	licenseQuotaFeature = "workspaces"
)

// writeLicenseRequired writes the 402 that a purchase can fix.
//
// Machine-readable on purpose. A console that had to recognise licence refusals by
// their prose would break the day a message is reworded or translated, and the
// message here is the one thing in the body that is meant to be read by a person.
func writeLicenseRequired(w http.ResponseWriter, feature, requiredTier, message string) {
	body := map[string]string{
		"error":   licenseRequiredCode,
		"feature": feature,
		"message": message,
		"docs":    licenseDocsURL,
	}
	// Absent rather than empty when no single plan can be named: a console reading
	// the key unconditionally would otherwise offer "requires a  licence".
	if requiredTier != "" {
		body["required_tier"] = requiredTier
	}
	writeJSON(w, http.StatusPaymentRequired, body)
}

// writeServiceError maps the authorization and lookup errors a service can return
// to HTTP status codes, writing the response and reporting whether it handled the
// error. A dead credential answers 401, a licence refusal 402 and a permission
// denial 403 — all three through writePermissionError, so the two helpers can never
// disagree about which of them a given error is — an authorization
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
