// Package trackingtoken mints and reads the tokens of the email tracking routes:
// /r/<token> for clicks and /t/<token> for opens.
//
// A token is the plaintext below, sealed with crypto.EncryptTrackingToken. That outer
// layer uses a public key: it only keeps the content opaque to pixel blockers, and anyone
// can open or seal one. What makes a token trustworthy is the tag, an HMAC under a key
// derived from the workspace's secret key, carried as a suffix of the timestamp field:
//
//	click: <mid>\n<wid>\n<ts>.<tag>\n<url>
//	open:  <mid>\n<wid>\n<ts>.<tag>
//
// The fields keep the order of the unsigned tokens minted before 42.2, so an instance that
// still reads the old format finds the message, workspace and destination where they were.
// It only fails to parse the timestamp, which skips its bot check.
package trackingtoken

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

// Purpose tells a click token from an open token, so a tag minted for one is never
// accepted as the other.
type Purpose string

const (
	PurposeClick Purpose = "r"
	PurposeOpen  Purpose = "t"
)

// keyInfo is the HKDF info string. Changing it changes every workspace's key and makes
// every delivered signed link unverifiable.
const keyInfo = "notifuse-tracking-v2"

// tagBytes is the length of the truncated HMAC: 128 bits keeps links short.
const tagBytes = 16

// Workspace is what the tracking routes need to know about a workspace: the key its
// links are signed with, and the lowercase hostname of its custom endpoint ("" if none).
type Workspace struct {
	Key          []byte
	EndpointHost string
}

// WorkspaceStatus is what a lookup of a token's workspace found.
type WorkspaceStatus int

const (
	// WorkspaceUnknown: no such workspace, or one whose secret no key opens. Nothing
	// should be recorded for it. It is the zero value, so a status never set fails closed.
	WorkspaceUnknown WorkspaceStatus = iota
	// WorkspaceKnown: the workspace exists and its key is usable.
	WorkspaceKnown
	// WorkspaceUnavailable: the lookup failed transiently. Nothing verifies, but the
	// workspace may well exist, so events are still recorded.
	WorkspaceUnavailable
)

// validWorkspaceID matches the IDs a workspace database can be named after, as the
// connection manager checks them (hyphens read as underscores).
var validWorkspaceID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,63}$`)

// ValidWorkspaceID reports whether id can name a workspace. Anything else is unknown
// without a lookup: request-supplied IDs must not reach a cache unbounded.
func ValidWorkspaceID(id string) bool {
	return validWorkspaceID.MatchString(id)
}

// HostOf returns the lowercase hostname of an endpoint URL such as API_ENDPOINT or a
// workspace's custom endpoint, read as https when it has no scheme, or "" when it is
// empty or does not parse.
func HostOf(endpoint string) string {
	raw := strings.TrimSpace(endpoint)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// Token is a parsed, unverified tracking token.
type Token struct {
	MessageID   string
	WorkspaceID string
	// Timestamp is the send time in Unix seconds, or 0 when the field is missing or not
	// numeric. It only feeds the bot check.
	Timestamp int64
	// URL is the click destination, exactly as minted. Empty for open tokens.
	URL string
	// Tag is the signature, empty for tokens minted unsigned or before 42.2.
	Tag string

	// rawTimestamp is the timestamp field before the tag, as signed.
	rawTimestamp string
}

// DeriveKey derives a workspace's tracking key from its secret key. An empty secret is
// an error, never a key.
func DeriveKey(workspaceSecret string) ([]byte, error) {
	if workspaceSecret == "" {
		return nil, errors.New("trackingtoken: empty workspace secret")
	}
	return hkdf.Key(sha256.New, []byte(workspaceSecret), nil, keyInfo, 32)
}

// MintClick returns a click token for url. An empty key mints an unsigned token, which
// still records the click but never redirects without checking the destination.
func MintClick(key []byte, mid, wid string, ts int64, url string) (string, error) {
	return crypto.EncryptTrackingToken(mid + "\n" + wid + "\n" + timestampField(key, PurposeClick, mid, wid, ts, url) + "\n" + url)
}

// MintOpen returns an open token. An empty key mints an unsigned token.
func MintOpen(key []byte, mid, wid string, ts int64) (string, error) {
	return crypto.EncryptTrackingToken(mid + "\n" + wid + "\n" + timestampField(key, PurposeOpen, mid, wid, ts, ""))
}

func timestampField(key []byte, purpose Purpose, mid, wid string, ts int64, url string) string {
	field := strconv.FormatInt(ts, 10)
	if len(key) == 0 {
		return field
	}
	return field + "." + computeTag(key, purpose, mid, wid, field, url)
}

func computeTag(key []byte, purpose Purpose, mid, wid, ts, url string) string {
	mac := hmac.New(sha256.New, key)
	for i, part := range []string{string(purpose), mid, wid, ts, url} {
		if i > 0 {
			mac.Write([]byte{0})
		}
		mac.Write([]byte(part))
	}
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:tagBytes])
}

// Parse opens and splits a token without verifying it. It fails only for a token that
// does not open or has the wrong number of fields; a timestamp that is not numeric is
// read as 0.
func Parse(token string, purpose Purpose) (Token, error) {
	plaintext, err := crypto.DecryptTrackingToken(token)
	if err != nil {
		return Token{}, err
	}
	fields := 3
	if purpose == PurposeClick {
		fields = 4
	}
	parts := strings.SplitN(plaintext, "\n", fields)
	if len(parts) != fields {
		return Token{}, fmt.Errorf("trackingtoken: expected %d fields, got %d", fields, len(parts))
	}
	t := Token{MessageID: parts[0], WorkspaceID: parts[1]}
	ts, tag, _ := strings.Cut(parts[2], ".")
	t.Tag = tag
	if n, err := strconv.ParseInt(ts, 10, 64); err == nil {
		t.Timestamp = n
	}
	if purpose == PurposeClick {
		t.URL = parts[3]
	}
	t.rawTimestamp = ts
	return t, nil
}

// Verify reports whether the token carries a tag minted under key for purpose.
func (t Token) Verify(key []byte, purpose Purpose) bool {
	if t.Tag == "" || len(key) == 0 {
		return false
	}
	want := computeTag(key, purpose, t.MessageID, t.WorkspaceID, t.rawTimestamp, t.URL)
	return hmac.Equal([]byte(want), []byte(t.Tag))
}

// SchemeOf returns the lowercase scheme of raw, read leniently so that a URL net/url
// cannot parse still has one: WHATWG preprocessing (leading and trailing C0 controls and
// spaces trimmed, tab, CR and LF removed), then [a-zA-Z][a-zA-Z0-9+.-]* before the first
// ':'. It returns "" when there is none.
func SchemeOf(raw string) string {
	s := Preprocess(raw)
	colon := strings.IndexByte(s, ':')
	if colon <= 0 {
		return ""
	}
	for i := range colon {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case i > 0 && (c >= '0' && c <= '9' || c == '+' || c == '.' || c == '-'):
		default:
			return ""
		}
	}
	return strings.ToLower(s[:colon])
}

// Preprocess applies the WHATWG URL parser's preprocessing: leading and trailing C0
// control characters and spaces are trimmed, and tab, CR and LF are removed anywhere.
func Preprocess(raw string) string {
	s := strings.TrimFunc(raw, func(r rune) bool { return r <= 0x20 })
	if strings.ContainsAny(s, "\t\r\n") {
		s = strings.NewReplacer("\t", "", "\r", "", "\n", "").Replace(s)
	}
	return s
}
