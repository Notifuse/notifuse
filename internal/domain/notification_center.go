package domain

import (
	"context"
	"errors"
	"net/url"
	"regexp"
)

//go:generate mockgen -destination mocks/mock_notification_center_service.go -package mocks github.com/Notifuse/notifuse/internal/domain NotificationCenterService

type NotificationCenterService interface {
	// GetContactPreferences returns public lists and notifications for a contact
	GetContactPreferences(ctx context.Context, workspaceID string, email string, emailHMAC string) (*ContactPreferencesResponse, error)
	// UpdateContactPreferences updates a contact's language and/or timezone
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
}

// UpdateContactPreferencesRequest represents a request to update a contact's language/timezone
type UpdateContactPreferencesRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Email       string `json:"email"`
	EmailHMAC   string `json:"email_hmac"`
	Language    string `json:"language,omitempty"`
	Timezone    string `json:"timezone,omitempty"`
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
	if r.Language == "" && r.Timezone == "" {
		return errors.New("at least one of language or timezone must be provided")
	}
	if r.Language != "" && !languageCodeRegex.MatchString(r.Language) {
		return errors.New("language must be a 2-letter lowercase code")
	}
	if r.Timezone != "" && (len(r.Timezone) > 50 || len(r.Timezone) < 2) {
		return errors.New("timezone must be between 2 and 50 characters")
	}
	return nil
}
