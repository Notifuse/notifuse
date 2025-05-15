package domain

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Notifuse/notifuse/pkg/mjml"
	"github.com/asaskevich/govalidator"
)

//go:generate mockgen -destination mocks/mock_transactional_notification_service.go -package mocks github.com/Notifuse/notifuse/internal/domain TransactionalNotificationService
//go:generate mockgen -destination mocks/mock_transactional_notification_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain TransactionalNotificationRepository

// TransactionalChannel represents supported notification channels
type TransactionalChannel string

const (
	// TransactionalChannelEmail for email notifications
	TransactionalChannelEmail TransactionalChannel = "email"
	// Add other channels in the future (sms, push, etc.)
)

// ChannelTemplate represents template configuration for a specific channel
type ChannelTemplate struct {
	TemplateID string   `json:"template_id"`
	Settings   MapOfAny `json:"settings,omitempty"`
}

// ChannelTemplates maps channels to their template configurations
type ChannelTemplates map[TransactionalChannel]ChannelTemplate

// Value implements the driver.Valuer interface for database storage
func (ct ChannelTemplates) Value() (driver.Value, error) {
	return json.Marshal(ct)
}

// Scan implements the sql.Scanner interface for database retrieval
func (ct *ChannelTemplates) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	v, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(v)
	return json.Unmarshal(cloned, ct)
}

// TransactionalNotification represents a transactional notification configuration
type TransactionalNotification struct {
	ID               string                `json:"id"` // Unique identifier for the notification, also used for API triggering
	Name             string                `json:"name"`
	Description      string                `json:"description"`
	Channels         ChannelTemplates      `json:"channels"`
	TrackingSettings mjml.TrackingSettings `json:"tracking_settings"`
	Metadata         MapOfAny              `json:"metadata,omitempty"`

	// System timestamps
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// TransactionalNotificationRepository defines methods for transactional notification persistence
type TransactionalNotificationRepository interface {
	// Create adds a new transactional notification
	Create(ctx context.Context, workspace string, notification *TransactionalNotification) error

	// Update updates an existing transactional notification
	Update(ctx context.Context, workspace string, notification *TransactionalNotification) error

	// Get retrieves a transactional notification by ID
	Get(ctx context.Context, workspace, id string) (*TransactionalNotification, error)

	// List retrieves all transactional notifications with optional filtering
	List(ctx context.Context, workspace string, filter map[string]interface{}, limit, offset int) ([]*TransactionalNotification, int, error)

	// Delete soft-deletes a transactional notification
	Delete(ctx context.Context, workspace, id string) error
}

// TransactionalNotificationCreateParams contains the parameters for creating a new transactional notification
type TransactionalNotificationCreateParams struct {
	ID              string           `json:"id" validate:"required"` // Unique identifier for API triggering
	Name            string           `json:"name" validate:"required"`
	Description     string           `json:"description"`
	Channels        ChannelTemplates `json:"channels" validate:"required,min=1"`
	TrackingEnabled bool             `json:"tracking_enabled"`
	Metadata        MapOfAny         `json:"metadata,omitempty"`
}

// TransactionalNotificationUpdateParams contains the parameters for updating an existing transactional notification
type TransactionalNotificationUpdateParams struct {
	Name            string           `json:"name,omitempty"`
	Description     string           `json:"description,omitempty"`
	Channels        ChannelTemplates `json:"channels,omitempty"`
	TrackingEnabled *bool            `json:"tracking_enabled,omitempty"`
	Metadata        MapOfAny         `json:"metadata,omitempty"`
}

// TransactionalNotificationSendParams contains the parameters for sending a transactional notification
type TransactionalNotificationSendParams struct {
	ID       string                 `json:"id" validate:"required"`      // ID of the notification to send
	Contact  *Contact               `json:"contact" validate:"required"` // Contact to send the notification to
	Channels []TransactionalChannel `json:"channels,omitempty"`          // Specific channels to send through (if empty, use all configured channels)
	Data     MapOfAny               `json:"data,omitempty"`              // Data to populate the template with
	Metadata MapOfAny               `json:"metadata,omitempty"`          // Additional metadata for tracking
	CC       []string               `json:"cc,omitempty"`                // CC email addresses
	BCC      []string               `json:"bcc,omitempty"`               // BCC email addresses
	ReplyTo  string                 `json:"reply_to,omitempty"`          // Reply-To email address
}

// TransactionalNotificationService defines the interface for transactional notification operations
type TransactionalNotificationService interface {
	// CreateNotification creates a new transactional notification
	CreateNotification(ctx context.Context, workspaceID string, params TransactionalNotificationCreateParams) (*TransactionalNotification, error)

	// UpdateNotification updates an existing transactional notification
	UpdateNotification(ctx context.Context, workspaceID, id string, params TransactionalNotificationUpdateParams) (*TransactionalNotification, error)

	// GetNotification retrieves a transactional notification by ID
	GetNotification(ctx context.Context, workspaceID, id string) (*TransactionalNotification, error)

	// ListNotifications retrieves all transactional notifications with optional filtering
	ListNotifications(ctx context.Context, workspaceID string, filter map[string]interface{}, limit, offset int) ([]*TransactionalNotification, int, error)

	// DeleteNotification soft-deletes a transactional notification
	DeleteNotification(ctx context.Context, workspaceID, id string) error

	// SendNotification sends a transactional notification to a contact
	SendNotification(ctx context.Context, workspaceID string, params TransactionalNotificationSendParams) (string, error)
}

// Request and response types for transactional notifications

// ListTransactionalRequest represents a request to list transactional notifications
type ListTransactionalRequest struct {
	WorkspaceID string                 `json:"workspace_id"`
	Search      string                 `json:"search,omitempty"`
	Limit       int                    `json:"limit,omitempty"`
	Offset      int                    `json:"offset,omitempty"`
	Filter      map[string]interface{} `json:"filter,omitempty"`
}

// FromURLParams populates the request from URL query parameters
func (req *ListTransactionalRequest) FromURLParams(values map[string][]string) error {
	req.WorkspaceID = getFirstValue(values, "workspace_id")
	if req.WorkspaceID == "" {
		return NewValidationError("workspace_id is required")
	}

	req.Search = getFirstValue(values, "search")

	if limitStr := getFirstValue(values, "limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			req.Limit = limit
		}
	}

	if offsetStr := getFirstValue(values, "offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			req.Offset = offset
		}
	}

	// Convert search to filter if provided
	if req.Filter == nil {
		req.Filter = make(map[string]interface{})
	}
	if req.Search != "" {
		req.Filter["search"] = req.Search
	}

	return nil
}

// GetTransactionalRequest represents a request to get a transactional notification
type GetTransactionalRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

// FromURLParams populates the request from URL query parameters
func (req *GetTransactionalRequest) FromURLParams(values map[string][]string) error {
	req.WorkspaceID = getFirstValue(values, "workspace_id")
	if req.WorkspaceID == "" {
		return NewValidationError("workspace_id is required")
	}

	req.ID = getFirstValue(values, "id")
	if req.ID == "" {
		return NewValidationError("id is required")
	}

	return nil
}

// CreateTransactionalRequest represents a request to create a transactional notification
type CreateTransactionalRequest struct {
	WorkspaceID  string                                `json:"workspace_id"`
	Notification TransactionalNotificationCreateParams `json:"notification"`
}

// Validate validates the create request
func (req *CreateTransactionalRequest) Validate() error {
	if req.WorkspaceID == "" {
		return NewValidationError("workspace_id is required")
	}

	if req.Notification.ID == "" {
		return NewValidationError("notification.id is required")
	}

	if req.Notification.Name == "" {
		return NewValidationError("notification.name is required")
	}

	if len(req.Notification.Channels) == 0 {
		return NewValidationError("notification must have at least one channel")
	}

	return nil
}

// UpdateTransactionalRequest represents a request to update a transactional notification
type UpdateTransactionalRequest struct {
	WorkspaceID string                                `json:"workspace_id"`
	ID          string                                `json:"id"`
	Updates     TransactionalNotificationUpdateParams `json:"updates"`
}

// Validate validates the update request
func (req *UpdateTransactionalRequest) Validate() error {
	if req.WorkspaceID == "" {
		return NewValidationError("workspace_id is required")
	}

	if req.ID == "" {
		return NewValidationError("id is required")
	}

	// At least one field must be updated
	if req.Updates.Name == "" &&
		req.Updates.Description == "" &&
		req.Updates.Channels == nil &&
		req.Updates.Metadata == nil {
		return NewValidationError("at least one field must be updated")
	}

	return nil
}

// DeleteTransactionalRequest represents a request to delete a transactional notification
type DeleteTransactionalRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

// Validate validates the delete request
func (req *DeleteTransactionalRequest) Validate() error {
	if req.WorkspaceID == "" {
		return NewValidationError("workspace_id is required")
	}

	if req.ID == "" {
		return NewValidationError("id is required")
	}

	return nil
}

// SendTransactionalRequest represents a request to send a transactional notification
type SendTransactionalRequest struct {
	WorkspaceID  string                              `json:"workspace_id"`
	Notification TransactionalNotificationSendParams `json:"notification"`
}

// Validate validates the send request
func (req *SendTransactionalRequest) Validate() error {
	if req.WorkspaceID == "" {
		return NewValidationError("workspace_id is required")
	}

	if req.Notification.ID == "" {
		return NewValidationError("notification.id is required")
	}

	if req.Notification.Contact == nil {
		return NewValidationError("notification.contact is required")
	}

	if req.Notification.Contact.Validate() != nil {
		return NewValidationError("notification.contact is invalid")
	}

	if len(req.Notification.Channels) == 0 {
		return NewValidationError("notification must have at least one channel")
	}

	// validate optional cc and bcc
	for _, cc := range req.Notification.CC {
		if !govalidator.IsEmail(cc) {
			return NewValidationError(fmt.Sprintf("cc '%s' must be a valid email address", cc))
		}
	}

	for _, bcc := range req.Notification.BCC {
		if !govalidator.IsEmail(bcc) {
			return NewValidationError(fmt.Sprintf("bcc '%s' must be a valid email address", bcc))
		}
	}

	// validate reply_to if provided
	if req.Notification.ReplyTo != "" && !govalidator.IsEmail(req.Notification.ReplyTo) {
		return NewValidationError(fmt.Sprintf("replyTo '%s' must be a valid email address", req.Notification.ReplyTo))
	}

	return nil
}

// Helper function to get the first value from a map of string slices
func getFirstValue(values map[string][]string, key string) string {
	if vals, ok := values[key]; ok && len(vals) > 0 {
		return vals[0]
	}
	return ""
}
