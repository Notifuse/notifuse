package domain

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

//go:generate mockgen -destination mocks/mock_custom_event_service.go -package mocks github.com/Notifuse/notifuse/internal/domain CustomEventService
//go:generate mockgen -destination mocks/mock_custom_event_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain CustomEventRepository

// Goal type constants
const (
	GoalTypePurchase     = "purchase"     // Transaction with revenue (goal_value REQUIRED)
	GoalTypeSubscription = "subscription" // Recurring revenue started (goal_value REQUIRED)
	GoalTypeLead         = "lead"         // Form/inquiry submission (goal_value optional)
	GoalTypeSignup       = "signup"       // Registration/account creation (goal_value optional)
	GoalTypeBooking      = "booking"      // Appointment/demo scheduled (goal_value optional)
	GoalTypeTrial        = "trial"        // Trial started (goal_value optional)
	GoalTypeOther        = "other"        // Custom goal (goal_value optional)
)

// ValidGoalTypes is the list of all valid goal types
var ValidGoalTypes = []string{
	GoalTypePurchase,
	GoalTypeSubscription,
	GoalTypeLead,
	GoalTypeSignup,
	GoalTypeBooking,
	GoalTypeTrial,
	GoalTypeOther,
}

// GoalTypesRequiringValue is the list of goal types that require goal_value
var GoalTypesRequiringValue = []string{
	GoalTypePurchase,
	GoalTypeSubscription,
}

// CustomEvent represents the current state of an external resource
// Note: ExternalID is the primary key and represents the unique identifier
// from the external system (e.g., "shopify_order_12345", "stripe_pi_abc123")
type CustomEvent struct {
	ExternalID    string                 `json:"external_id"` // Primary key: external system's unique ID
	Email         string                 `json:"email"`
	EventName     string                 `json:"event_name"`               // Generic: "shopify.order", "stripe.payment"
	Properties    map[string]interface{} `json:"properties"`               // Current state of the resource
	OccurredAt    time.Time              `json:"occurred_at"`              // When this version was created
	Source        string                 `json:"source"`                   // "api", "integration", "import"
	IntegrationID *string                `json:"integration_id,omitempty"` // Optional integration ID

	// Goal tracking fields
	GoalName  *string  `json:"goal_name,omitempty"`  // Optional goal name for categorization
	GoalType  *string  `json:"goal_type,omitempty"`  // purchase, subscription, lead, signup, booking, trial, other
	GoalValue *float64 `json:"goal_value,omitempty"` // Monetary value (required for purchase/subscription, can be negative for refunds)

	// Soft delete
	DeletedAt *time.Time `json:"deleted_at,omitempty"` // Soft delete timestamp

	// Timestamps
	CreatedAt time.Time `json:"created_at"` // When first inserted
	UpdatedAt time.Time `json:"updated_at"` // When last updated
}

// Validate validates the custom event
func (e *CustomEvent) Validate() error {
	if e.ExternalID == "" {
		return fmt.Errorf("external_id is required")
	}
	if len(e.ExternalID) > 255 {
		return fmt.Errorf("external_id must be 255 characters or less")
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

	// Validate goal fields
	if e.GoalType != nil {
		// Validate goal_type is in allowed list
		valid := false
		for _, t := range ValidGoalTypes {
			if *e.GoalType == t {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("goal_type must be one of: %v", ValidGoalTypes)
		}

		// Require goal_value for purchase and subscription
		requiresValue := false
		for _, t := range GoalTypesRequiringValue {
			if *e.GoalType == t {
				requiresValue = true
				break
			}
		}

		if requiresValue && e.GoalValue == nil {
			return fmt.Errorf("goal_value is required for goal_type '%s'", *e.GoalType)
		}
	}

	// Validate goal_name length if provided
	if e.GoalName != nil && len(*e.GoalName) > 100 {
		return fmt.Errorf("goal_name must be 100 characters or less")
	}

	// Note: Negative goal_value is allowed for refunds/chargebacks

	return nil
}

// IsDeleted returns true if the event has been soft-deleted
func (e *CustomEvent) IsDeleted() bool {
	return e.DeletedAt != nil
}

// UpsertCustomEventRequest represents the API request to create or update a custom event
// Soft-delete by setting deleted_at, restore by setting deleted_at to null
type UpsertCustomEventRequest struct {
	WorkspaceID   string                 `json:"workspace_id"`
	Email         string                 `json:"email"`
	EventName     string                 `json:"event_name"`
	ExternalID    string                 `json:"external_id"` // Required: unique external resource ID
	Properties    map[string]interface{} `json:"properties"`
	OccurredAt    *time.Time             `json:"occurred_at,omitempty"`    // Optional, defaults to now
	IntegrationID *string                `json:"integration_id,omitempty"` // Optional integration ID

	// Goal tracking fields
	GoalName  *string  `json:"goal_name,omitempty"`  // Optional goal name
	GoalType  *string  `json:"goal_type,omitempty"`  // purchase, subscription, lead, signup, booking, trial, other
	GoalValue *float64 `json:"goal_value,omitempty"` // Required for purchase/subscription, can be negative for refunds

	// Soft delete - set to a timestamp to delete, null to restore
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

func (r *UpsertCustomEventRequest) Validate() error {
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
	if len(r.ExternalID) > 255 {
		return fmt.Errorf("external_id must be 255 characters or less")
	}
	if r.Properties == nil {
		r.Properties = make(map[string]interface{})
	}

	// Validate goal fields
	if r.GoalType != nil {
		// Validate goal_type is in allowed list
		valid := false
		for _, t := range ValidGoalTypes {
			if *r.GoalType == t {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("goal_type must be one of: %v", ValidGoalTypes)
		}

		// Require goal_value for purchase and subscription
		requiresValue := false
		for _, t := range GoalTypesRequiringValue {
			if *r.GoalType == t {
				requiresValue = true
				break
			}
		}

		if requiresValue && r.GoalValue == nil {
			return fmt.Errorf("goal_value is required for goal_type '%s'", *r.GoalType)
		}
	}

	// Validate goal_name length if provided
	if r.GoalName != nil && len(*r.GoalName) > 100 {
		return fmt.Errorf("goal_name must be 100 characters or less")
	}

	return nil
}

// ImportCustomEventsRequest for bulk import
type ImportCustomEventsRequest struct {
	WorkspaceID string         `json:"workspace_id"`
	Events      []*CustomEvent `json:"events"`
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
	Upsert(ctx context.Context, workspaceID string, event *CustomEvent) error
	BatchUpsert(ctx context.Context, workspaceID string, events []*CustomEvent) error
	GetByID(ctx context.Context, workspaceID, eventName, externalID string) (*CustomEvent, error)
	ListByEmail(ctx context.Context, workspaceID, email string, limit int, offset int) ([]*CustomEvent, error)
	ListByEventName(ctx context.Context, workspaceID, eventName string, limit int, offset int) ([]*CustomEvent, error)
	DeleteForEmail(ctx context.Context, workspaceID, email string) error
}

// CustomEventService defines business logic
type CustomEventService interface {
	UpsertEvent(ctx context.Context, req *UpsertCustomEventRequest) (*CustomEvent, error)
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
