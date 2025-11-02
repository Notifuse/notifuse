package domain

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Notifuse/notifuse/pkg/crypto"
	svix "github.com/standard-webhooks/standard-webhooks/libraries/go"
)

// SupabaseEmailActionType defines the type of auth email being sent
// signup → Confirm signup (Dashboard: Confirm signup template)
// invite → Invite user (Dashboard: Invite user template)
// magiclink → Magic Link (Dashboard: Magic Link template)
// recovery → Reset Password (Dashboard: Reset Password template)
// email_change → Change Email Address (Dashboard: Change Email Address template)
// reauthentication → Reauthentication (system/internal OTP for sensitive actions)
// email → (No dedicated UI template; this is a generic/system email used internally — not exposed as a separate editable template in the Dashboard)

type SupabaseEmailActionType string

const (
	SupabaseEmailActionSignup      SupabaseEmailActionType = "signup"
	SupabaseEmailActionInvite      SupabaseEmailActionType = "invite"
	SupabaseEmailActionMagicLink   SupabaseEmailActionType = "magiclink"
	SupabaseEmailActionRecovery    SupabaseEmailActionType = "recovery"
	SupabaseEmailActionEmailChange SupabaseEmailActionType = "email_change"
	// SupabaseEmailActionEmail            SupabaseEmailActionType = "email"
	SupabaseEmailActionReauthentication SupabaseEmailActionType = "reauthentication"
)

// SupabaseIntegrationSettings contains all Supabase integration configuration
type SupabaseIntegrationSettings struct {
	AuthEmailHook         SupabaseAuthEmailHookSettings   `json:"auth_email_hook"`
	BeforeUserCreatedHook SupabaseUserCreatedHookSettings `json:"before_user_created_hook"`
}

// SupabaseAuthEmailHookSettings configures the Send Email Hook
// Hook activation is controlled in Supabase, not here
type SupabaseAuthEmailHookSettings struct {
	SignatureKey          string `json:"signature_key,omitempty"`           // Accepts plaintext key in requests (cleared before API responses)
	EncryptedSignatureKey string `json:"encrypted_signature_key,omitempty"` // Encrypted key (stored and returned in API responses)
}

// SupabaseUserCreatedHookSettings configures the Before User Created Hook
// Hook activation is controlled in Supabase, not here
type SupabaseUserCreatedHookSettings struct {
	SignatureKey          string   `json:"signature_key,omitempty"`           // Accepts plaintext key in requests (cleared before API responses)
	EncryptedSignatureKey string   `json:"encrypted_signature_key,omitempty"` // Encrypted key (stored and returned in API responses)
	AddUserToLists        []string `json:"add_user_to_lists,omitempty"`       // Optional lists to add contacts to
	CustomJSONField       string   `json:"custom_json_field,omitempty"`       // Which custom_json field to use (default: custom_json_1)
	RejectDisposableEmail bool     `json:"reject_disposable_email,omitempty"` // Reject user creation if email is disposable
}

// SupabaseTemplateMappings maps each email action type to a Notifuse template ID
type SupabaseTemplateMappings struct {
	Signup           string `json:"signup"`
	MagicLink        string `json:"magiclink"`
	Recovery         string `json:"recovery"`
	EmailChange      string `json:"email_change"` // Single template used for both current and new email addresses
	Invite           string `json:"invite"`
	Reauthentication string `json:"reauthentication"`
}

// SupabaseAuthEmailWebhook represents the webhook payload for Send Email Hook
type SupabaseAuthEmailWebhook struct {
	User      SupabaseUser      `json:"user"`
	EmailData SupabaseEmailData `json:"email_data"`
}

// SupabaseBeforeUserCreatedWebhook represents the webhook payload for Before User Created Hook
type SupabaseBeforeUserCreatedWebhook struct {
	Metadata SupabaseWebhookMetadata `json:"metadata"`
	User     SupabaseUser            `json:"user"`
}

// SupabaseUser represents a Supabase user object
type SupabaseUser struct {
	ID           string                 `json:"id"`
	Aud          string                 `json:"aud"`
	Role         string                 `json:"role"`
	Email        string                 `json:"email"`
	EmailNew     string                 `json:"email_new,omitempty"` // Present in email_change webhooks
	Phone        string                 `json:"phone"`
	AppMetadata  map[string]interface{} `json:"app_metadata"`
	UserMetadata map[string]interface{} `json:"user_metadata"`
	Identities   []interface{}          `json:"identities"`
	CreatedAt    string                 `json:"created_at"`
	UpdatedAt    string                 `json:"updated_at"`
	IsAnonymous  bool                   `json:"is_anonymous"`
}

// SupabaseEmailData contains email-specific data for auth hooks
type SupabaseEmailData struct {
	Token           string `json:"token"`
	TokenHash       string `json:"token_hash"`
	RedirectTo      string `json:"redirect_to"`
	EmailActionType string `json:"email_action_type"`
	SiteURL         string `json:"site_url"`
	TokenNew        string `json:"token_new,omitempty"`      // For email_change in secure mode
	TokenHashNew    string `json:"token_hash_new,omitempty"` // For email_change in secure mode
}

// SupabaseWebhookMetadata contains metadata about the webhook request
type SupabaseWebhookMetadata struct {
	UUID      string `json:"uuid"`       // Request ID
	Time      string `json:"time"`       // ISO 8601 timestamp
	Name      string `json:"name"`       // Hook name
	IPAddress string `json:"ip_address"` // User's IP address
}

// Validate validates the Supabase integration settings
func (s *SupabaseIntegrationSettings) Validate(passphrase string) error {
	// Validate auth email hook
	if err := s.AuthEmailHook.Validate(); err != nil {
		return fmt.Errorf("invalid auth email hook settings: %w", err)
	}

	// Validate before user created hook
	if err := s.BeforeUserCreatedHook.Validate(); err != nil {
		return fmt.Errorf("invalid before user created hook settings: %w", err)
	}

	return nil
}

// Validate validates the auth email hook settings
func (s *SupabaseAuthEmailHookSettings) Validate() error {
	// No validation needed - signature key validation happens at webhook processing time
	return nil
}

// Validate validates the user created hook settings
func (s *SupabaseUserCreatedHookSettings) Validate() error {
	// All fields are optional - signature key, target list, and custom JSON field
	// Validation happens at webhook processing time if signature key is missing
	return nil
}

// GetTemplateID returns the template ID for a given email action type
// For email_change, the same template is used for both current and new email addresses
// (matching Supabase's behavior where there's only one customizable email_change template)
func (m *SupabaseTemplateMappings) GetTemplateID(actionType SupabaseEmailActionType) (string, error) {
	switch actionType {
	case SupabaseEmailActionSignup:
		return m.Signup, nil
	case SupabaseEmailActionMagicLink:
		return m.MagicLink, nil
	case SupabaseEmailActionRecovery:
		return m.Recovery, nil
	case SupabaseEmailActionEmailChange:
		return m.EmailChange, nil
	case SupabaseEmailActionInvite:
		return m.Invite, nil
	case SupabaseEmailActionReauthentication:
		return m.Reauthentication, nil
	default:
		return "", fmt.Errorf("unsupported email action type: %s", actionType)
	}
}

// ValidateSupabaseWebhookSignature validates a Supabase webhook signature
// Supabase uses the standard-webhooks format with webhook-id, webhook-timestamp, and webhook-signature headers
func ValidateSupabaseWebhookSignature(payload []byte, signatureHeader, timestampHeader, idHeader, secret string) error {
	// Create a new webhook verifier with the secret
	wh, err := svix.NewWebhook(secret)
	if err != nil {
		return fmt.Errorf("failed to create webhook verifier: %w", err)
	}

	// Build headers map as expected by the standard-webhooks library
	// Use canonical header names (capitalized)
	headers := http.Header{}
	headers.Set("Webhook-Id", idHeader)
	headers.Set("Webhook-Timestamp", timestampHeader)
	headers.Set("Webhook-Signature", signatureHeader)

	// Verify the webhook signature
	// The Verify method returns an error if validation fails
	err = wh.Verify(payload, headers)
	if err != nil {
		return fmt.Errorf("signature validation failed: %w", err)
	}

	return nil
}

// EncryptSignatureKeys encrypts both signature keys
func (s *SupabaseIntegrationSettings) EncryptSignatureKeys(passphrase string) error {
	if s.AuthEmailHook.SignatureKey != "" {
		if err := s.AuthEmailHook.EncryptSignatureKey(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt auth email hook signature key: %w", err)
		}
	}

	if s.BeforeUserCreatedHook.SignatureKey != "" {
		if err := s.BeforeUserCreatedHook.EncryptSignatureKey(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt before user created hook signature key: %w", err)
		}
	}

	return nil
}

// DecryptSignatureKeys decrypts both signature keys
func (s *SupabaseIntegrationSettings) DecryptSignatureKeys(passphrase string) error {
	if s.AuthEmailHook.EncryptedSignatureKey != "" {
		if err := s.AuthEmailHook.DecryptSignatureKey(passphrase); err != nil {
			return fmt.Errorf("failed to decrypt auth email hook signature key: %w", err)
		}
	}

	if s.BeforeUserCreatedHook.EncryptedSignatureKey != "" {
		if err := s.BeforeUserCreatedHook.DecryptSignatureKey(passphrase); err != nil {
			return fmt.Errorf("failed to decrypt before user created hook signature key: %w", err)
		}
	}

	return nil
}

// EncryptSignatureKey encrypts the signature key for auth email hook
func (s *SupabaseAuthEmailHookSettings) EncryptSignatureKey(passphrase string) error {
	if s.SignatureKey == "" {
		return nil
	}

	encrypted, err := EncryptString(s.SignatureKey, passphrase)
	if err != nil {
		return err
	}

	s.EncryptedSignatureKey = encrypted
	s.SignatureKey = "" // Clear plaintext
	return nil
}

// DecryptSignatureKey decrypts the signature key for auth email hook
func (s *SupabaseAuthEmailHookSettings) DecryptSignatureKey(passphrase string) error {
	if s.EncryptedSignatureKey == "" {
		return nil
	}

	decrypted, err := DecryptString(s.EncryptedSignatureKey, passphrase)
	if err != nil {
		return err
	}

	s.SignatureKey = decrypted
	return nil
}

// EncryptSignatureKey encrypts the signature key for user created hook
func (s *SupabaseUserCreatedHookSettings) EncryptSignatureKey(passphrase string) error {
	if s.SignatureKey == "" {
		return nil
	}

	encrypted, err := EncryptString(s.SignatureKey, passphrase)
	if err != nil {
		return err
	}

	s.EncryptedSignatureKey = encrypted
	s.SignatureKey = "" // Clear plaintext
	return nil
}

// DecryptSignatureKey decrypts the signature key for user created hook
func (s *SupabaseUserCreatedHookSettings) DecryptSignatureKey(passphrase string) error {
	if s.EncryptedSignatureKey == "" {
		return nil
	}

	decrypted, err := DecryptString(s.EncryptedSignatureKey, passphrase)
	if err != nil {
		return err
	}

	s.SignatureKey = decrypted
	return nil
}

// EncryptString encrypts a string using the same encryption as other sensitive data
func EncryptString(plaintext, passphrase string) (string, error) {
	return crypto.EncryptString(plaintext, passphrase)
}

// DecryptString decrypts an encrypted string
func DecryptString(encrypted, passphrase string) (string, error) {
	return crypto.DecryptFromHexString(encrypted, passphrase)
}

// ToContact converts a Supabase user to a Notifuse Contact
// customJSONField specifies which custom_json field to use (e.g., "custom_json_1", "custom_json_2", etc.)
// If empty, user_metadata will not be mapped
func (u *SupabaseUser) ToContact(customJSONField string) (*Contact, error) {
	if u.Email == "" {
		return nil, fmt.Errorf("email is required")
	}

	contact := &Contact{
		Email: u.Email,
	}

	// Map external ID
	if u.ID != "" {
		contact.ExternalID = &NullableString{String: u.ID, IsNull: false}
	}

	// Map phone
	if u.Phone != "" {
		contact.Phone = &NullableString{String: u.Phone, IsNull: false}
	}

	// Map user_metadata to the specified custom_json field (if customJSONField is specified)
	if len(u.UserMetadata) > 0 && customJSONField != "" {
		metadata := &NullableJSON{Data: u.UserMetadata, IsNull: false}

		switch customJSONField {
		case "custom_json_1":
			contact.CustomJSON1 = metadata
		case "custom_json_2":
			contact.CustomJSON2 = metadata
		case "custom_json_3":
			contact.CustomJSON3 = metadata
		case "custom_json_4":
			contact.CustomJSON4 = metadata
		case "custom_json_5":
			contact.CustomJSON5 = metadata
		}
		// If customJSONField doesn't match any valid field, user_metadata is not mapped
	}

	// Map created_at if it's not a zero value
	if u.CreatedAt != "" {
		if createdAt, err := parseSupabaseTimestamp(u.CreatedAt); err == nil {
			contact.CreatedAt = createdAt
		}
	}

	// Map updated_at if it's not a zero value
	if u.UpdatedAt != "" {
		if updatedAt, err := parseSupabaseTimestamp(u.UpdatedAt); err == nil {
			contact.UpdatedAt = updatedAt
		}
	}

	return contact, nil
}

// parseSupabaseTimestamp parses a Supabase timestamp string and returns zero time if it's a zero value
func parseSupabaseTimestamp(timestamp string) (time.Time, error) {
	// Parse the timestamp
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	// Check if it's a zero value (0001-01-01)
	if t.Year() == 1 {
		return time.Time{}, fmt.Errorf("timestamp is zero value")
	}

	return t, nil
}
