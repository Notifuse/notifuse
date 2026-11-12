package domain

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

//go:generate mockgen -destination mocks/mock_contact_timeline_service.go -package mocks github.com/Notifuse/notifuse/internal/domain ContactTimelineService
//go:generate mockgen -destination mocks/mock_contact_timeline_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain ContactTimelineRepository

// ContactTimelineEntry represents a timeline entry for a contact
type ContactTimelineEntry struct {
	ID          string                 `json:"id"`
	Email       string                 `json:"email"`
	Operation   string                 `json:"operation"`   // 'insert', 'update', 'delete'
	EntityType  string                 `json:"entity_type"` // 'contact', 'contact_list', 'message_history'
	Kind        string                 `json:"kind"`        // semantic dot-notation event kind (e.g., 'contact.created', 'email.clicked', 'email.updated')
	Changes     map[string]interface{} `json:"changes"`
	EntityID    *string                `json:"entity_id,omitempty"`   // NULL for contact, list_id for contact_list, message_id for message_history
	EntityData  map[string]interface{} `json:"entity_data,omitempty"` // Joined entity data (contact, list, or message details)
	CreatedAt   time.Time              `json:"created_at"`            // Can be set to historical data
	DBCreatedAt time.Time              `json:"db_created_at"`         // Timestamp when record was inserted into database
}

// Entity types of contact_timeline entries.
const (
	TimelineEntityContact             = "contact"
	TimelineEntityContactList         = "contact_list"
	TimelineEntityContactSegment      = "contact_segment"
	TimelineEntityMessageHistory      = "message_history"
	TimelineEntityCustomEvent         = "custom_event"
	TimelineEntityInboundWebhookEvent = "inbound_webhook_event"
	// TimelineEntityWebhookEvent is legacy: before v19 inbound webhook events were written
	// under this name, and v19 renamed the existing entries. It stays accepted.
	TimelineEntityWebhookEvent = "webhook_event"
	TimelineEntityAutomation   = "automation"
	TimelineEntityWebSession   = "web_session"
	TimelineEntityWebPage      = "web_page"
)

// TimelineEntityTypes lists every entity type timeline.list can filter on.
var TimelineEntityTypes = []string{
	TimelineEntityContact,
	TimelineEntityContactList,
	TimelineEntityContactSegment,
	TimelineEntityMessageHistory,
	TimelineEntityCustomEvent,
	TimelineEntityInboundWebhookEvent,
	TimelineEntityWebhookEvent,
	TimelineEntityAutomation,
	TimelineEntityWebSession,
	TimelineEntityWebPage,
}

// TimelineEntityTypeFilter restricts the entries a timeline query returns: Only keeps
// those entity types, Except drops them. Both empty returns every entry.
type TimelineEntityTypeFilter struct {
	Only   []string
	Except []string
}

// TimelineListRequest represents the request parameters for listing timeline entries
type TimelineListRequest struct {
	WorkspaceID string
	Email       string
	Limit       int
	Cursor      *string
	// EntityTypes keeps only these entity types; empty keeps all.
	EntityTypes []string
}

// TimelineListResponse represents the response for listing timeline entries
type TimelineListResponse struct {
	Timeline   []*ContactTimelineEntry `json:"timeline"`
	NextCursor *string                 `json:"next_cursor,omitempty"`
}

// Validate validates the timeline list request
func (r *TimelineListRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.Email == "" {
		return fmt.Errorf("email is required")
	}
	if r.Limit < 0 {
		return fmt.Errorf("limit must be non-negative")
	}
	if r.Limit > 100 {
		return fmt.Errorf("limit cannot exceed 100")
	}
	for _, t := range r.EntityTypes {
		if !slices.Contains(TimelineEntityTypes, t) {
			return fmt.Errorf("invalid entity_types: unknown entity type %q", t)
		}
	}
	return nil
}

// FromQuery parses query parameters into a TimelineListRequest
func (r *TimelineListRequest) FromQuery(query url.Values) error {
	r.WorkspaceID = query.Get("workspace_id")
	r.Email = query.Get("email")

	// Parse limit with default value
	r.Limit = 50 // Default
	if limitStr := query.Get("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil {
			return fmt.Errorf("invalid limit parameter: must be an integer")
		}
		r.Limit = parsedLimit
	}

	// Parse cursor
	if cursorStr := query.Get("cursor"); cursorStr != "" {
		r.Cursor = &cursorStr
	}

	r.EntityTypes = nil
	for _, t := range strings.Split(query.Get("entity_types"), ",") {
		if t = strings.TrimSpace(t); t != "" && !slices.Contains(r.EntityTypes, t) {
			r.EntityTypes = append(r.EntityTypes, t)
		}
	}

	return r.Validate()
}

// ContactTimelineRepository defines methods for contact timeline persistence
type ContactTimelineRepository interface {
	// Create inserts a new timeline entry
	Create(ctx context.Context, workspaceID string, entry *ContactTimelineEntry) error
	// List retrieves timeline entries for a contact, restricted by filter
	List(ctx context.Context, workspaceID string, email string, limit int, cursor *string, filter TimelineEntityTypeFilter) ([]*ContactTimelineEntry, *string, error)
	// DeleteForEmail deletes all timeline entries for a contact
	DeleteForEmail(ctx context.Context, workspaceID string, email string) error
}

// ContactTimelineService defines business logic for contact timeline
type ContactTimelineService interface {
	// List retrieves timeline entries for a contact with pagination. A non-empty entityTypes
	// keeps only those types; entries the caller may not read are dropped either way.
	List(ctx context.Context, workspaceID string, email string, limit int, cursor *string, entityTypes []string) ([]*ContactTimelineEntry, *string, error)
}
