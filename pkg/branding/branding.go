// Package branding holds the identity an instance shows instead of Notifuse's: a name, a main
// logo URL, an icon URL, a background image URL for the sign-in pages and a switch that hides the
// Help menu.
//
// It validates and normalises those values and resolves what each surface shows. It does no I/O
// and knows nothing of licensing: whether a stored identity may be shown is decided by the caller,
// which hands Resolve an empty Branding when it may not.
package branding

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Notifuse/notifuse/pkg/safehttpclient"
)

const (
	// MaxNameCodePoints bounds the brand name. It counts code points, not bytes or graphemes.
	MaxNameCodePoints = 80

	// MaxURLBytes bounds each image URL, in UTF-8 bytes after trimming.
	MaxURLBytes = 2048

	// DefaultName is the name shown when no custom identity applies.
	DefaultName = "Notifuse"

	// DefaultLogoPath is the Notifuse logo the splash shows in default mode.
	DefaultLogoPath = "/console/logo.png"

	// DefaultFaviconPath is the Notifuse favicon used in default mode.
	DefaultFaviconPath = "/console/favicon-32x32.png"

	// NeutralIconPath is a local icon carrying no brand. A custom identity installs it as the
	// favicon at once, before the configured icon has loaded, and keeps it when that icon fails.
	NeutralIconPath = "/console/neutral-icon.png"
)

// API field names, used in validation errors so a form can put each error under its field.
const (
	FieldName          = "branding_name"
	FieldLogoURL       = "branding_logo_url"
	FieldIconURL       = "branding_icon_url"
	FieldHideHelpMenu  = "branding_hide_help_menu"
	FieldBackgroundURL = "branding_background_url"
)

// Branding is the stored identity. The zero value is the default identity.
type Branding struct {
	Name         string `json:"name"`
	LogoURL      string `json:"logo_url"`
	IconURL      string `json:"icon_url"`
	HideHelpMenu bool   `json:"hide_help_menu"`
	// BackgroundURL replaces the photo behind the sign-in, invitation and workspace pages. Like
	// HideHelpMenu it is independent of the identity: it neither makes the brand custom nor needs a
	// name. omitempty keeps the stored JSON, and so the Revision, of a brand without one as it was.
	BackgroundURL string `json:"background_url,omitempty"`
}

// FieldError is a validation failure tied to one API field.
type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ErrInvalidStored means a stored identity is not readable JSON. The API never writes such a
// value, so it comes from a hand edit or a corrupted row.
var ErrInvalidStored = errors.New("stored instance branding is not readable")

// Normalize trims surrounding white space from the three text fields. A field holding only white
// space becomes empty.
func (b Branding) Normalize() Branding {
	b.Name = strings.TrimSpace(b.Name)
	b.LogoURL = strings.TrimSpace(b.LogoURL)
	b.IconURL = strings.TrimSpace(b.IconURL)
	b.BackgroundURL = strings.TrimSpace(b.BackgroundURL)
	return b
}

// IsCustom reports whether the identity replaces Notifuse's: any of the name, logo or icon is set.
// HideHelpMenu and BackgroundURL alone do not make an identity custom.
func (b Branding) IsCustom() bool {
	n := b.Normalize()
	return n.Name != "" || n.LogoURL != "" || n.IconURL != ""
}

// Validate checks a normalised identity and returns the first *FieldError it finds, or nil.
func (b Branding) Validate() error {
	if err := ValidateName(b.Name); err != nil {
		return err
	}
	if err := ValidateURL(b.LogoURL); err != nil {
		return &FieldError{Field: FieldLogoURL, Message: err.Error()}
	}
	if err := ValidateURL(b.IconURL); err != nil {
		return &FieldError{Field: FieldIconURL, Message: err.Error()}
	}
	if err := ValidateURL(b.BackgroundURL); err != nil {
		return &FieldError{Field: FieldBackgroundURL, Message: err.Error()}
	}
	if b.Name == "" && (b.LogoURL != "" || b.IconURL != "") {
		return &FieldError{Field: FieldName, Message: "a brand name is required when a logo or an icon is set"}
	}
	return nil
}

// ValidateChanged validates only what a save changes against the stored identity. A stored value
// sent back unchanged is not checked again: it may predate a stricter rule (an address range added
// to the private list, for example), and a save of unrelated settings must not fail over it.
// Values are compared normalised.
func (b Branding) ValidateChanged(stored Branding) error {
	b, stored = b.Normalize(), stored.Normalize()
	if b == stored {
		return nil
	}
	if b.Name != stored.Name {
		if err := ValidateName(b.Name); err != nil {
			return err
		}
	}
	if b.LogoURL != stored.LogoURL {
		if err := ValidateURL(b.LogoURL); err != nil {
			return &FieldError{Field: FieldLogoURL, Message: err.Error()}
		}
	}
	if b.IconURL != stored.IconURL {
		if err := ValidateURL(b.IconURL); err != nil {
			return &FieldError{Field: FieldIconURL, Message: err.Error()}
		}
	}
	if b.BackgroundURL != stored.BackgroundURL {
		if err := ValidateURL(b.BackgroundURL); err != nil {
			return &FieldError{Field: FieldBackgroundURL, Message: err.Error()}
		}
	}
	if b.Name == "" && (b.LogoURL != "" || b.IconURL != "") {
		return &FieldError{Field: FieldName, Message: "a brand name is required when a logo or an icon is set"}
	}
	return nil
}

// ValidateName checks a trimmed brand name. An empty name is valid on its own.
func ValidateName(name string) error {
	if !utf8.ValidString(name) {
		return &FieldError{Field: FieldName, Message: "must be valid UTF-8"}
	}
	if utf8.RuneCountInString(name) > MaxNameCodePoints {
		return &FieldError{Field: FieldName, Message: fmt.Sprintf("must be at most %d characters", MaxNameCodePoints)}
	}
	for _, r := range name {
		if unicode.IsControl(r) || isHiddenFormatChar(r) {
			return &FieldError{Field: FieldName, Message: "must not contain control or invisible characters"}
		}
	}
	return nil
}

// isHiddenFormatChar reports the format characters that reorder or hide text: the bidirectional
// embeddings, overrides and isolates, and the zero-width space, word joiner and byte order mark.
// The zero-width joiner and non-joiner stay allowed, since some scripts and emoji need them.
func isHiddenFormatChar(r rune) bool {
	switch {
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return true
	case r == 0x200B, r == 0x200E, r == 0x200F, r == 0x2060, r == 0xFEFF, r == 0x061C:
		return true
	}
	return false
}

// ValidateURL checks a trimmed image URL. An empty URL is valid.
//
// The rules refuse the forms whose parsing differs between Go and browsers rather than trying to
// certify where a URL leads: no DNS lookup is made, and browsers, not the server, fetch the image.
func ValidateURL(raw string) error {
	if raw == "" {
		return nil
	}
	if len(raw) > MaxURLBytes {
		return fmt.Errorf("must be at most %d bytes", MaxURLBytes)
	}
	for _, r := range raw {
		switch {
		case unicode.IsControl(r):
			return errors.New("must not contain control characters")
		case unicode.IsSpace(r):
			return errors.New("must not contain spaces")
		case r == '\\':
			return errors.New("must not contain backslashes")
		}
	}
	if strings.Contains(raw, "#") {
		return errors.New("must not contain a fragment")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("is not a valid URL")
	}
	if !strings.EqualFold(u.Scheme, "https") || u.Opaque != "" || u.Host == "" {
		return errors.New("must be an absolute https:// URL")
	}
	if u.User != nil {
		return errors.New("must not contain a user name or password")
	}
	if strings.HasSuffix(u.Host, ":") {
		return errors.New("has an empty port")
	}
	if port := u.Port(); port != "" && port != "443" {
		return errors.New("must use the default HTTPS port")
	}

	return validateHost(u.Host, u.Hostname())
}

func validateHost(hostPort, host string) error {
	if strings.HasPrefix(hostPort, "[") {
		if strings.Contains(host, "%") {
			return errors.New("must not contain an IPv6 zone")
		}
		ip := net.ParseIP(host)
		if ip == nil || ip.To4() != nil || ip.String() != strings.ToLower(host) {
			return errors.New("must use a canonical IPv6 address")
		}
		if safehttpclient.IsPrivateIP(ip) {
			return errors.New("must not point to a private or reserved address")
		}
		return nil
	}

	for i := 0; i < len(host); i++ {
		if host[i] > unicode.MaxASCII {
			return errors.New("must use the ASCII (punycode) form of the domain")
		}
	}

	name := strings.ToLower(strings.TrimSuffix(host, "."))
	if name == "" {
		return errors.New("has no host")
	}
	labels := strings.Split(name, ".")
	last := labels[len(labels)-1]

	if isNumericLabel(last) {
		ip, ok := canonicalIPv4(labels)
		if !ok {
			return errors.New("must write an IPv4 address as four decimal numbers")
		}
		if safehttpclient.IsPrivateIP(ip) {
			return errors.New("must not point to a private or reserved address")
		}
		return nil
	}

	if len(labels) < 2 {
		return errors.New("must use a fully qualified domain name")
	}
	if name == "localhost" || strings.HasSuffix(name, ".localhost") ||
		strings.HasSuffix(name, ".local") || strings.HasSuffix(name, ".internal") {
		return errors.New("must use a public domain name")
	}
	for _, label := range labels {
		if !validLabel(label) {
			return errors.New("has an invalid domain name")
		}
	}
	return nil
}

// isNumericLabel reports whether a label is one browsers read as part of an IPv4 address: all
// digits, or a 0x-prefixed hexadecimal number.
func isNumericLabel(label string) bool {
	if label == "" {
		return false
	}
	if strings.HasPrefix(label, "0x") {
		return true
	}
	for i := 0; i < len(label); i++ {
		if label[i] < '0' || label[i] > '9' {
			return false
		}
	}
	return true
}

// canonicalIPv4 accepts exactly four decimal components from 0 to 255 without leading zeros. The
// shortened, integer, hexadecimal and octal forms are refused because Go and browsers read them
// differently.
func canonicalIPv4(labels []string) (net.IP, bool) {
	if len(labels) != 4 {
		return nil, false
	}
	for _, label := range labels {
		if label == "" || len(label) > 3 || (len(label) > 1 && label[0] == '0') {
			return nil, false
		}
		n, err := strconv.Atoi(label)
		if err != nil || n < 0 || n > 255 {
			return nil, false
		}
	}
	ip := net.ParseIP(strings.Join(labels, "."))
	return ip, ip != nil
}

func validLabel(label string) bool {
	if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for i := 0; i < len(label); i++ {
		c := label[i]
		if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') && c != '-' {
			return false
		}
	}
	return true
}

// Revision is a SHA-256 of the canonical JSON of the normalised identity, hex-encoded. Two
// identities that differ in any of the five fields have different revisions.
func (b Branding) Revision() string {
	data, _ := json.Marshal(b.Normalize()) // a struct of strings and a bool always marshals
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Encode serialises a normalised identity for storage.
func (b Branding) Encode() ([]byte, error) {
	return json.Marshal(b.Normalize())
}

// Decode reads a stored identity. Only the JSON is checked: a value saved under older or looser
// rules still reads, and Resolve decides what of it can be shown. A value that is not JSON wraps
// ErrInvalidStored. Unknown keys are ignored, so a value written by a newer version still reads.
func Decode(data []byte) (Branding, error) {
	var b Branding
	if err := json.Unmarshal(data, &b); err != nil {
		return Branding{}, fmt.Errorf("%w: %v", ErrInvalidStored, err)
	}
	return b.Normalize(), nil
}

// displayableURL keeps an image URL only if it is an absolute https:// URL. A stored value is read
// without the full validation, so this is the floor every shown URL meets.
func displayableURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Host == "" {
		return ""
	}
	return raw
}

// Resolved is what the console and the public pages show, worked out once so the bootstrap script
// and React read the same answer instead of each interpreting empty strings.
type Resolved struct {
	// Custom is true when the identity replaces Notifuse's.
	Custom bool `json:"custom"`
	// Name is the entered name in custom mode, DefaultName otherwise.
	Name string `json:"name"`
	// LogoURL and IconURL are the configured URLs, empty when not set or in default mode.
	LogoURL string `json:"logo_url"`
	IconURL string `json:"icon_url"`
	// HideHelpMenu hides the Help menu. It is independent of Custom.
	HideHelpMenu bool `json:"hide_help_menu"`
	// BackgroundURL is the image behind the sign-in, invitation and workspace pages, empty for the
	// Notifuse photo. It is independent of Custom.
	BackgroundURL string `json:"background_url"`
	// SplashLogoSrc is the image the loading screen shows: the Notifuse logo in default mode, the
	// configured logo in custom mode, or empty when the name is shown as text instead.
	SplashLogoSrc string `json:"splash_logo_src"`
	// FaviconSrc is the favicon installed before any configured icon has loaded.
	FaviconSrc string `json:"favicon_src"`
	// Revision identifies the stored identity, for the save-and-restart handshake.
	Revision string `json:"revision"`
}

// Resolve works out what to show for an identity. Pass an empty Branding to get the default
// identity; the revision is that of the identity passed. A URL that is not https:// is not shown,
// and a custom identity without a name shows DefaultName: both only happen with a stored value
// that bypassed the API.
func Resolve(b Branding) Resolved {
	revision := b.Revision()
	b = b.Normalize()
	b.LogoURL = displayableURL(b.LogoURL)
	b.IconURL = displayableURL(b.IconURL)
	r := Resolved{
		Custom:        b.IsCustom(),
		HideHelpMenu:  b.HideHelpMenu,
		BackgroundURL: displayableURL(b.BackgroundURL),
		Revision:      revision,
	}
	if !r.Custom {
		r.Name = DefaultName
		r.SplashLogoSrc = DefaultLogoPath
		r.FaviconSrc = DefaultFaviconPath
		return r
	}
	r.Name = b.Name
	if r.Name == "" {
		r.Name = DefaultName
	}
	r.LogoURL = b.LogoURL
	r.IconURL = b.IconURL
	r.SplashLogoSrc = b.LogoURL
	r.FaviconSrc = NeutralIconPath
	return r
}
