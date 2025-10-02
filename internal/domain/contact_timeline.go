package domain

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

//go:generate mockgen -destination mocks/mock_contact_timeline_service.go -package mocks github.com/Notifuse/notifuse/internal/domain ContactTimelineService
//go:generate mockgen -destination mocks/mock_contact_timeline_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain ContactTimelineRepository

// ContactTimelineEntry represents a timeline entry for a contact
type ContactTimelineEntry struct {
	ID         string                 `json:"id"`
	Email      string                 `json:"email"`
	Operation  string                 `json:"operation"`   // 'insert', 'update', 'delete'
	EntityType string                 `json:"entity_type"` // 'contact', 'contact_list', 'message_history'
	Changes    map[string]interface{} `json:"changes"`
	EntityID   *string                `json:"entity_id,omitempty"`   // NULL for contact, list_id for contact_list, message_id for message_history
	EntityData map[string]interface{} `json:"entity_data,omitempty"` // Joined entity data (contact, list, or message details)
	CreatedAt  time.Time              `json:"created_at"`
}

// TimelineListRequest represents the request parameters for listing timeline entries
type TimelineListRequest struct {
	WorkspaceID string
	Email       string
	Limit       int
	Cursor      *string
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

	return r.Validate()
}

// ContactTimelineRepository defines methods for contact timeline persistence
type ContactTimelineRepository interface {
	// List retrieves timeline entries for a contact
	List(ctx context.Context, workspaceID string, email string, limit int, cursor *string) ([]*ContactTimelineEntry, *string, error)
	// DeleteForEmail deletes all timeline entries for a contact
	DeleteForEmail(ctx context.Context, workspaceID string, email string) error
}

// ContactTimelineService defines business logic for contact timeline
type ContactTimelineService interface {
	// List retrieves timeline entries for a contact with pagination
	List(ctx context.Context, workspaceID string, email string, limit int, cursor *string) ([]*ContactTimelineEntry, *string, error)
}
