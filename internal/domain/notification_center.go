package domain

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
)

//go:generate mockgen -destination mocks/mock_notification_center_service.go -package mocks github.com/Notifuse/notifuse/internal/domain NotificationCenterService

type NotificationCenterService interface {
	// GetContactPreferences returns public lists and notifications for a contact
	GetContactPreferences(ctx context.Context, workspaceID string, email string, emailHMAC string) (*ContactPreferencesResponse, error)
	// UpdateContactPreferences updates a contact's language, timezone and/or
	// tracking consent. The contact must exist.
	UpdateContactPreferences(ctx context.Context, req *UpdateContactPreferencesRequest) error
}

type NotificationCenterRequest struct {
	Email       string `json:"email"`
	EmailHMAC   string `json:"email_hmac"`
	WorkspaceID string `json:"workspace_id"`
	Action      string `json:"action,omitempty"`     // Optional action (e.g., "confirm", "unsubscribe")
	ListID      string `json:"list_id,omitempty"`    // List ID for actions
	MessageID   string `json:"message_id,omitempty"` // Message ID for tracking
}

func (r *NotificationCenterRequest) Validate() error {
	if r.Email == "" {
		return errors.New("email is required")
	}
	if r.EmailHMAC == "" {
		return errors.New("email_hmac is required")
	}
	if r.WorkspaceID == "" {
		return errors.New("workspace_id is required")
	}
	return nil
}

func (r *NotificationCenterRequest) FromURLValues(values url.Values) error {
	r.Email = values.Get("email")
	r.EmailHMAC = values.Get("email_hmac")
	r.WorkspaceID = values.Get("workspace_id")
	r.Action = values.Get("action")
	r.ListID = values.Get("lid")
	r.MessageID = values.Get("mid")
	return r.Validate()
}

// ContactPreferencesResponse contains the response data for the notification center
type ContactPreferencesResponse struct {
	Contact      *Contact       `json:"contact"`
	PublicLists  []*List        `json:"public_lists"`
	ContactLists []*ContactList `json:"contact_lists"`
	LogoURL      string         `json:"logo_url"`
	WebsiteURL   string         `json:"website_url"`
	// Tracking tells the widget what the workspace tracks and what is tracked
	// for this contact now, which for an unknown consent depends on the
	// workspace. The recorded consent itself is on Contact.
	Tracking *ContactTrackingState `json:"tracking,omitempty"`
}

// ContactTrackingState is what the notification center's tracking dialog shows.
type ContactTrackingState struct {
	OpensEnabled  bool `json:"opens_enabled"`
	ClicksEnabled bool `json:"clicks_enabled"`
	OpensTracked  bool `json:"opens_tracked"`
	ClicksTracked bool `json:"clicks_tracked"`
}

// NewContactTrackingState reads the workspace policy and the contact's consent.
func NewContactTrackingState(policy EmailTrackingPolicy, contact *Contact) *ContactTrackingState {
	decision := ResolveEmailTracking(policy, contact, "")
	return &ContactTrackingState{
		OpensEnabled:  policy.OpensEnabled,
		ClicksEnabled: policy.ClicksEnabled,
		OpensTracked:  decision.Opens,
		ClicksTracked: decision.Clicks,
	}
}

// UpdateContactPreferencesRequest represents a request to update a contact's language/timezone
type UpdateContactPreferencesRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Email       string `json:"email"`
	EmailHMAC   string `json:"email_hmac"`
	Language    string `json:"language,omitempty"`
	Timezone    string `json:"timezone,omitempty"`

	// The contact's own tracking choice: granted or denied. A public request
	// cannot set a consent back to unknown.
	TrackingOpensConsent  string `json:"tracking_opens_consent,omitempty"`
	TrackingClicksConsent string `json:"tracking_clicks_consent,omitempty"`
	// ConsentSource is notification_center (the default) or email_link (the
	// one-click opt-out link of an email).
	ConsentSource string `json:"consent_source,omitempty"`
}

// HasTrackingConsent reports whether the request changes a consent.
func (r *UpdateContactPreferencesRequest) HasTrackingConsent() bool {
	return r.TrackingOpensConsent != "" || r.TrackingClicksConsent != ""
}

// TrackingConsentSource returns the source to record, defaulting to the
// notification center.
func (r *UpdateContactPreferencesRequest) TrackingConsentSource() string {
	if r.ConsentSource == "" {
		return TrackingConsentSourceNotificationCenter
	}
	return r.ConsentSource
}

var languageCodeRegex = regexp.MustCompile(`^[a-z]{2}$`)

func (r *UpdateContactPreferencesRequest) Validate() error {
	if r.WorkspaceID == "" {
		return errors.New("workspace_id is required")
	}
	if r.Email == "" {
		return errors.New("email is required")
	}
	if r.EmailHMAC == "" {
		return errors.New("email_hmac is required")
	}
	if r.Language == "" && r.Timezone == "" && !r.HasTrackingConsent() {
		return errors.New("at least one of language, timezone, tracking_opens_consent or tracking_clicks_consent must be provided")
	}
	if err := ValidateTrackingConsentValue(r.TrackingOpensConsent); err != nil {
		return fmt.Errorf("tracking_opens_consent: %w", err)
	}
	if err := ValidateTrackingConsentValue(r.TrackingClicksConsent); err != nil {
		return fmt.Errorf("tracking_clicks_consent: %w", err)
	}
	switch r.ConsentSource {
	case "", TrackingConsentSourceNotificationCenter, TrackingConsentSourceEmailLink:
	default:
		return fmt.Errorf("consent_source must be %q or %q", TrackingConsentSourceNotificationCenter, TrackingConsentSourceEmailLink)
	}
	if r.Language != "" && !languageCodeRegex.MatchString(r.Language) {
		return errors.New("language must be a 2-letter lowercase code")
	}
	if r.Timezone != "" && (len(r.Timezone) > 50 || len(r.Timezone) < 2) {
		return errors.New("timezone must be between 2 and 50 characters")
	}
	return nil
}
