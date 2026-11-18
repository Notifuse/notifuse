package domain

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

// InboundReply is the provider-agnostic representation of a reply email received
// through an ESP's inbound-parse webhook. Each provider's ReplyParser normalizes
// its payload into this shape so the matching/classification pipeline is
// provider-independent.
type InboundReply struct {
	FromEmail  string    // normalized, lowercased sender (the contact who replied)
	ToEmail    string    // address it was delivered to (informational)
	Subject    string    //
	MessageID  string    // the reply's own Message-ID, angle-brackets stripped (dedup key)
	InReplyTo  string    // first Message-ID from In-Reply-To → matching key
	References []string   // additional thread Message-IDs → matching fallback
	ReceivedAt time.Time //

	// Classification inputs (raw header values; empty string when absent).
	AutoSubmitted string            // Auto-Submitted header (RFC 3834)
	Precedence    string            // Precedence header (bulk / auto_reply / …)
	ContentType   string            // top-level Content-Type (for DSN detection)
	RawHeaders    map[string]string // any other headers a parser chooses to surface
}

// InboundRequest is the neutral HTTP payload handed to a ReplyParser. The HTTP
// handler populates Form for multipart/urlencoded bodies and Body for JSON, and
// keeps both so a parser can verify a provider signature over whichever it needs.
type InboundRequest struct {
	Header      http.Header
	ContentType string
	Query       url.Values
	Form        url.Values
	Body        []byte
}

// ReplyParser normalizes one ESP's inbound-parse payload into an InboundReply and
// authenticates the request using the provider's native signature scheme (the
// per-integration endpoint token is checked separately, in the HTTP handler).
type ReplyParser interface {
	Source() WebhookSource
	Verify(req *InboundRequest, integration *Integration) error
	Parse(req *InboundRequest) (*InboundReply, error)
}

// ReplyClass categorizes an inbound message so only a genuine human reply triggers
// a stop-on-reply exit.
type ReplyClass int

const (
	// ReplyGenuine is a real human reply — the only class that triggers an exit.
	ReplyGenuine ReplyClass = iota
	// ReplyAutoResponder is an out-of-office / vacation / auto-reply — recorded but
	// never exits a journey (an OOO must not permanently stop a sequence).
	ReplyAutoResponder
	// ReplyBounce is a delivery-status notification / non-delivery report — dropped.
	ReplyBounce
)

// String returns a stable label for logging and the timeline kind decision.
func (c ReplyClass) String() string {
	switch c {
	case ReplyGenuine:
		return "genuine"
	case ReplyAutoResponder:
		return "auto_responder"
	case ReplyBounce:
		return "bounce"
	default:
		return "unknown"
	}
}

// Classify categorizes an inbound reply. Order matters: bounces/DSNs first (so an
// NDR is never mistaken for a reply), then auto-responders, otherwise a genuine
// reply. A reply whose subject asks to stop ("STOP", "unsubscribe") is a genuine
// reply: it ends an Exit on reply journey like any other, and changes no list
// subscription. A nil reply is treated as droppable.
func Classify(reply *InboundReply) ReplyClass {
	if reply == nil {
		return ReplyBounce
	}
	switch {
	case isBounce(reply):
		return ReplyBounce
	case isAutoResponder(reply):
		return ReplyAutoResponder
	default:
		return ReplyGenuine
	}
}

// isBounce detects delivery-status notifications by Content-Type (RFC 3462) or a
// daemon sender address.
func isBounce(r *InboundReply) bool {
	ct := strings.ToLower(r.ContentType)
	if strings.Contains(ct, "multipart/report") || strings.Contains(ct, "delivery-status") {
		return true
	}
	switch strings.ToLower(localPart(r.FromEmail)) {
	case "mailer-daemon", "postmaster":
		return true
	}
	return false
}

// isAutoResponder detects out-of-office / vacation / auto-reply messages via the
// standard auto-submission headers (RFC 3834) and common vendor headers.
func isAutoResponder(r *InboundReply) bool {
	if as := strings.ToLower(strings.TrimSpace(r.AutoSubmitted)); as != "" && as != "no" {
		// "auto-generated", "auto-replied", "auto-notified" → automated.
		return true
	}
	switch strings.ToLower(strings.TrimSpace(r.Precedence)) {
	case "bulk", "auto_reply", "junk":
		return true
	}
	return headerPresent(r, "X-Autoreply") || headerPresent(r, "X-Autorespond")
}

// localPart returns the part of an email address before the last "@".
func localPart(email string) string {
	if at := strings.LastIndex(email, "@"); at >= 0 {
		return email[:at]
	}
	return email
}

// headerPresent reports whether a header (case-insensitive) is present in
// RawHeaders, regardless of value.
func headerPresent(r *InboundReply, name string) bool {
	for k := range r.RawHeaders {
		if strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}
