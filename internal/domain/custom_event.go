package domain

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

//go:generate mockgen -destination mocks/mock_custom_event_service.go -package mocks github.com/Notifuse/notifuse/internal/domain CustomEventService
//go:generate mockgen -destination mocks/mock_custom_event_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain CustomEventRepository

// CustomEvent represents the current state of an external resource
// Note: ExternalID is the primary key and represents the unique identifier
// from the external system (e.g., "shopify_order_12345", "stripe_pi_abc123")
type CustomEvent struct {
	ExternalID    string                 `json:"external_id"`              // Primary key: external system's unique ID
	Email         string                 `json:"email"`
	EventName     string                 `json:"event_name"`               // Generic: "shopify.order", "stripe.payment"
	Properties    map[string]interface{} `json:"properties"`               // Current state of the resource
	OccurredAt    time.Time              `json:"occurred_at"`              // When this version was created
	Source        string                 `json:"source"`                   // "api", "integration", "import"
	IntegrationID *string                `json:"integration_id,omitempty"` // Optional integration ID
	CreatedAt     time.Time              `json:"created_at"`               // When first inserted
	UpdatedAt     time.Time              `json:"updated_at"`               // When last updated
}

// Validate validates the custom event
func (e *CustomEvent) Validate() error {
	if e.ExternalID == "" {
		return fmt.Errorf("external_id is required")
	}
	if e.Email == "" {
		return fmt.Errorf("email is required")
	}
	if e.EventName == "" {
		return fmt.Errorf("event_name is required")
	}
	if len(e.EventName) > 100 {
		return fmt.Errorf("event_name must be 100 characters or less")
	}
	// Validate event name format
	if !isValidEventName(e.EventName) {
		return fmt.Errorf("event_name must contain only lowercase letters, numbers, underscores, dots, and slashes")
	}
	if e.OccurredAt.IsZero() {
		return fmt.Errorf("occurred_at is required")
	}
	if e.Properties == nil {
		e.Properties = make(map[string]interface{})
	}
	return nil
}

// CreateCustomEventRequest represents the API request to create a custom event
type CreateCustomEventRequest struct {
	WorkspaceID   string                 `json:"workspace_id"`
	Email         string                 `json:"email"`
	EventName     string                 `json:"event_name"`
	ExternalID    string                 `json:"external_id"`              // Required: unique external resource ID
	Properties    map[string]interface{} `json:"properties"`
	OccurredAt    *time.Time             `json:"occurred_at,omitempty"`    // Optional, defaults to now
	IntegrationID *string                `json:"integration_id,omitempty"` // Optional integration ID
}

func (r *CreateCustomEventRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.Email == "" {
		return fmt.Errorf("email is required")
	}
	if r.EventName == "" {
		return fmt.Errorf("event_name is required")
	}
	if r.ExternalID == "" {
		return fmt.Errorf("external_id is required")
	}
	if r.Properties == nil {
		r.Properties = make(map[string]interface{})
	}
	return nil
}

// ImportCustomEventsRequest for bulk import
type ImportCustomEventsRequest struct {
	WorkspaceID string          `json:"workspace_id"`
	Events      []*CustomEvent  `json:"events"`
}

func (r *ImportCustomEventsRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if len(r.Events) == 0 {
		return fmt.Errorf("events array cannot be empty")
	}
	if len(r.Events) > 50 {
		return fmt.Errorf("cannot import more than 50 events at once")
	}
	return nil
}

// ListCustomEventsRequest represents query parameters for listing custom events
type ListCustomEventsRequest struct {
	WorkspaceID string
	Email       string
	EventName   *string // Optional filter by event name
	Limit       int
	Offset      int
}

func (r *ListCustomEventsRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.Email == "" && r.EventName == nil {
		return fmt.Errorf("either email or event_name is required")
	}
	if r.Limit <= 0 {
		r.Limit = 50 // Default
	}
	if r.Limit > 100 {
		r.Limit = 100 // Max
	}
	if r.Offset < 0 {
		r.Offset = 0
	}
	return nil
}

// CustomEventRepository defines persistence methods
type CustomEventRepository interface {
	Create(ctx context.Context, workspaceID string, event *CustomEvent) error
	BatchCreate(ctx context.Context, workspaceID string, events []*CustomEvent) error
	GetByID(ctx context.Context, workspaceID, eventName, externalID string) (*CustomEvent, error)
	ListByEmail(ctx context.Context, workspaceID, email string, limit int, offset int) ([]*CustomEvent, error)
	ListByEventName(ctx context.Context, workspaceID, eventName string, limit int, offset int) ([]*CustomEvent, error)
	DeleteForEmail(ctx context.Context, workspaceID, email string) error
}

// CustomEventService defines business logic
type CustomEventService interface {
	CreateEvent(ctx context.Context, req *CreateCustomEventRequest) (*CustomEvent, error)
	ImportEvents(ctx context.Context, req *ImportCustomEventsRequest) ([]string, error)
	GetEvent(ctx context.Context, workspaceID, eventName, externalID string) (*CustomEvent, error)
	ListEvents(ctx context.Context, req *ListCustomEventsRequest) ([]*CustomEvent, error)
}

// Helper function to validate event name format
func isValidEventName(name string) bool {
	// Event names can use various formats:
	// - Webhook topics: "orders/fulfilled", "customers/create"
	// - Dotted: "payment.succeeded", "subscription.created"
	// - Underscores: "trial_started", "feature_activated"
	// Allow lowercase letters, numbers, underscores, dots, and slashes
	pattern := regexp.MustCompile(`^[a-z0-9_./-]+$`)
	return pattern.MatchString(name)
}
