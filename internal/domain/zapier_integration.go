package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// ZapierSettings is everything a Zapier connection persists on the workspace.
type ZapierSettings struct {
	// Address of the API key minted when this integration was added. Display-only:
	// nothing in the backend reads it to make a decision.
	APIKeyEmail string `json:"api_key_email"`
}

// Validate validates the Zapier settings
func (z *ZapierSettings) Validate() error {
	if z.APIKeyEmail == "" {
		return fmt.Errorf("api key email is required for Zapier configuration")
	}
	if !strings.Contains(z.APIKeyEmail, "@") {
		return fmt.Errorf("api key email must be an email address")
	}
	return nil
}

// ZapierKeyPermissions is the grant a Zapier connection's API key is minted with, verb by verb:
//
//   - webhook_subscriptions read and write: every trigger registers and removes its own REST
//     hook when the Zap is turned on and off.
//   - contacts read and write: the two actions upsert contacts, and every trigger reads contacts
//     for the sample data Zapier shows while a Zap is being built.
//   - lists read and write: the list picker and the subscribe action.
//   - segments read only: the segment picker, and the member lookup behind both segment
//     triggers. Read is the whole of it — nothing in the app writes a segment — and the two
//     segment triggers are unusable without it.
//
// Nothing else is granted, so a leaked Zapier token cannot read message history or send.
//
// This is the key's *initial* grant, not an invariant: /api/workspaces.setUserPermissions is a
// live endpoint and Settings → Team edits the row freely, so an owner may widen or narrow the
// key afterwards and nothing here reasserts it.
//
// It returns a fresh map on every call for the reason NewFullPermissions carries: a shared
// package-level map has been mutated by a caller once already.
func ZapierKeyPermissions() UserPermissions {
	permissions := NewEmptyPermissions()
	permissions[PermissionResourceWebhookSubscriptions] = ResourcePermissions{Read: true, Write: true}
	permissions[PermissionResourceContacts] = ResourcePermissions{Read: true, Write: true}
	permissions[PermissionResourceLists] = ResourcePermissions{Read: true, Write: true}
	permissions[PermissionResourceSegments] = ResourcePermissions{Read: true, Write: false}
	// Layered last because each grant above replaces a whole ResourcePermissions value: were an
	// unenforced verb ever to land on one of these four resources, granting first would drop it.
	return GrantUnenforcedPermissions(permissions)
}

const (
	// zapierKeyPrefixBase opens every minted prefix, so a stray key is recognisable in
	// Settings → Team by its address alone.
	zapierKeyPrefixBase = "zapier"

	// zapierKeySlugMaxLen is what apiKeyEmailPrefixRegex leaves for the label: 64 characters,
	// less "zapier-" and less the "-" plus 8 hex characters that end the prefix.
	zapierKeySlugMaxLen = 64 - len(zapierKeyPrefixBase+"-") - len("-") - 8
)

// zapierKeyRandomHexRegex is what ZapierKeyPrefix requires of its caller's randomness. The
// length is fixed because zapierKeySlugMaxLen is budgeted against it.
var zapierKeyRandomHexRegex = regexp.MustCompile(`^[0-9a-f]{8}$`)

// ZapierKeyPrefix builds the local part of the address for a Zapier connection's API key:
// zapier-<slug of the label>-<randomHex>, or zapier-<randomHex> when the label slugifies to
// nothing or to "zapier" itself, so the default label does not mint zapier-zapier-….
//
// users.email is unique across the whole deployment while the address is derived from the API
// endpoint alone, so the random suffix is what lets two workspaces on one installation both
// connect Zapier under the same label.
//
// The result is checked against apiKeyEmailPrefixRegex rather than assumed to satisfy it:
// WorkspaceService.CreateAPIKey concatenates the prefix into the address without re-applying
// the check CreateAPIKeyRequest.Validate makes at the HTTP boundary, and nothing downstream
// validates an email address at all.
func ZapierKeyPrefix(label string, randomHex string) (string, error) {
	if !zapierKeyRandomHexRegex.MatchString(randomHex) {
		return "", fmt.Errorf("zapier key prefix: random suffix %q must match ^[0-9a-f]{8}$", randomHex)
	}

	prefix := zapierKeyPrefixBase + "-" + randomHex
	if slug := zapierLabelSlug(label); slug != "" && slug != zapierKeyPrefixBase {
		prefix = zapierKeyPrefixBase + "-" + slug + "-" + randomHex
	}

	if !apiKeyEmailPrefixRegex.MatchString(prefix) {
		return "", fmt.Errorf("zapier key prefix %q must match ^[a-z0-9_-]{1,64}$", prefix)
	}
	return prefix, nil
}

// zapierLabelSlug reduces a user-supplied label to [a-z0-9_-], collapsing every run of rejected
// characters into a single dash and truncating to the budget above. The output is ASCII by
// construction, so the truncation can slice bytes.
func zapierLabelSlug(label string) string {
	var slug strings.Builder
	pendingDash := false
	for _, r := range strings.ToLower(label) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			if pendingDash {
				slug.WriteByte('-')
				pendingDash = false
			}
			slug.WriteRune(r)
		default:
			// Held rather than written, so a leading or trailing run produces no dash at all.
			pendingDash = slug.Len() > 0
		}
	}

	if slug.Len() <= zapierKeySlugMaxLen {
		return slug.String()
	}
	return strings.TrimRight(slug.String()[:zapierKeySlugMaxLen], "-")
}
