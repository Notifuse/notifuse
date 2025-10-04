package domain

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/asaskevich/govalidator"
)

//go:generate mockgen -destination mocks/mock_segment_service.go -package mocks github.com/Notifuse/notifuse/internal/domain SegmentService
//go:generate mockgen -destination mocks/mock_segment_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain SegmentRepository

// SegmentStatus represents the status of a segment
type SegmentStatus string

const (
	SegmentStatusActive   SegmentStatus = "active"
	SegmentStatusDeleted  SegmentStatus = "deleted"
	SegmentStatusBuilding SegmentStatus = "building"
)

// Validate checks if the segment status is valid
func (s SegmentStatus) Validate() error {
	switch s {
	case SegmentStatusActive, SegmentStatusDeleted, SegmentStatusBuilding:
		return nil
	}
	return fmt.Errorf("invalid segment status: %s", s)
}

// Segment represents a user segment for filtering contacts
type Segment struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Color         string    `json:"color"`
	Tree          *TreeNode `json:"tree"`
	Timezone      string    `json:"timezone"`
	Version       int64     `json:"version"`
	Status        string    `json:"status"`
	GeneratedSQL  *string   `json:"generated_sql,omitempty"`
	GeneratedArgs MapOfAny  `json:"generated_args,omitempty"`
	DBCreatedAt   time.Time `json:"db_created_at"`
	DBUpdatedAt   time.Time `json:"db_updated_at"`
	UsersCount    int       `json:"users_count"` // joined server-side
}

// ContactSegment represents the relationship between a contact and a segment
type ContactSegment struct {
	Email      string    `json:"email"`
	SegmentID  string    `json:"segment_id"`
	Version    int64     `json:"version"`
	MatchedAt  time.Time `json:"matched_at"`
	ComputedAt time.Time `json:"computed_at"`
}

// Validate performs validation on the segment fields
func (s *Segment) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("invalid segment: id is required")
	}
	// Allow lowercase letters, numbers, and underscores (snake_case)
	if !govalidator.Matches(s.ID, "^[a-z0-9_]+$") {
		return fmt.Errorf("invalid segment: id must contain only lowercase letters, numbers, and underscores")
	}
	if len(s.ID) > 32 {
		return fmt.Errorf("invalid segment: id length must be between 1 and 32")
	}

	if s.Name == "" {
		return fmt.Errorf("invalid segment: name is required")
	}
	if len(s.Name) > 255 {
		return fmt.Errorf("invalid segment: name length must be between 1 and 255")
	}

	if s.Color == "" {
		return fmt.Errorf("invalid segment: color is required")
	}
	if len(s.Color) > 50 {
		return fmt.Errorf("invalid segment: color length must be between 1 and 50")
	}

	if s.Timezone == "" {
		return fmt.Errorf("invalid segment: timezone is required")
	}
	if len(s.Timezone) > 100 {
		return fmt.Errorf("invalid segment: timezone length must be between 1 and 100")
	}

	if s.Version <= 0 {
		return fmt.Errorf("invalid segment: version must be positive")
	}

	// Validate status
	status := SegmentStatus(s.Status)
	if err := status.Validate(); err != nil {
		return fmt.Errorf("invalid segment: %w", err)
	}

	// Validate tree is not nil and valid
	if s.Tree == nil {
		return fmt.Errorf("invalid segment: tree is required")
	}

	if err := s.Tree.Validate(); err != nil {
		return fmt.Errorf("invalid segment tree: %w", err)
	}

	return nil
}

// For database scanning
type dbSegment struct {
	ID            string
	Name          string
	Color         string
	Tree          MapOfAny
	Timezone      string
	Version       int64
	Status        string
	GeneratedSQL  *string
	GeneratedArgs MapOfAny
	DBCreatedAt   time.Time
	DBUpdatedAt   time.Time
}

// ScanSegment scans a segment from the database
func ScanSegment(scanner interface {
	Scan(dest ...interface{}) error
}) (*Segment, error) {
	var dbs dbSegment
	var usersCount int
	if err := scanner.Scan(
		&dbs.ID,
		&dbs.Name,
		&dbs.Color,
		&dbs.Tree,
		&dbs.Timezone,
		&dbs.Version,
		&dbs.Status,
		&dbs.GeneratedSQL,
		&dbs.GeneratedArgs,
		&dbs.DBCreatedAt,
		&dbs.DBUpdatedAt,
		&usersCount,
	); err != nil {
		return nil, err
	}

	// Convert MapOfAny tree to TreeNode
	tree, err := TreeNodeFromMapOfAny(dbs.Tree)
	if err != nil {
		return nil, fmt.Errorf("failed to parse segment tree: %w", err)
	}

	s := &Segment{
		ID:            dbs.ID,
		Name:          dbs.Name,
		Color:         dbs.Color,
		Tree:          tree,
		Timezone:      dbs.Timezone,
		Version:       dbs.Version,
		Status:        dbs.Status,
		GeneratedSQL:  dbs.GeneratedSQL,
		GeneratedArgs: dbs.GeneratedArgs,
		DBCreatedAt:   dbs.DBCreatedAt,
		DBUpdatedAt:   dbs.DBUpdatedAt,
		UsersCount:    usersCount,
	}

	return s, nil
}

// Request/Response types
type CreateSegmentRequest struct {
	WorkspaceID string    `json:"workspace_id"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	Tree        *TreeNode `json:"tree"`
	Timezone    string    `json:"timezone"`
}

func (r *CreateSegmentRequest) Validate() (segment *Segment, workspaceID string, err error) {
	if r.WorkspaceID == "" {
		return nil, "", fmt.Errorf("invalid create segment request: workspace_id is required")
	}
	if r.ID == "" {
		return nil, "", fmt.Errorf("invalid create segment request: id is required")
	}
	if !govalidator.IsAlphanumeric(r.ID) {
		return nil, "", fmt.Errorf("invalid create segment request: id must be alphanumeric")
	}
	if len(r.ID) > 32 {
		return nil, "", fmt.Errorf("invalid create segment request: id length must be between 1 and 32")
	}

	if r.Name == "" {
		return nil, "", fmt.Errorf("invalid create segment request: name is required")
	}
	if len(r.Name) > 255 {
		return nil, "", fmt.Errorf("invalid create segment request: name length must be between 1 and 255")
	}

	if r.Color == "" {
		return nil, "", fmt.Errorf("invalid create segment request: color is required")
	}
	if len(r.Color) > 50 {
		return nil, "", fmt.Errorf("invalid create segment request: color length must be between 1 and 50")
	}

	if r.Timezone == "" {
		return nil, "", fmt.Errorf("invalid create segment request: timezone is required")
	}
	if len(r.Timezone) > 100 {
		return nil, "", fmt.Errorf("invalid create segment request: timezone length must be between 1 and 100")
	}

	if r.Tree == nil {
		return nil, "", fmt.Errorf("invalid create segment request: tree is required")
	}

	if err := r.Tree.Validate(); err != nil {
		return nil, "", fmt.Errorf("invalid create segment request: invalid tree: %w", err)
	}

	return &Segment{
		ID:       r.ID,
		Name:     r.Name,
		Color:    r.Color,
		Tree:     r.Tree,
		Timezone: r.Timezone,
		Version:  1,
		Status:   string(SegmentStatusBuilding),
	}, r.WorkspaceID, nil
}

type GetSegmentsRequest struct {
	WorkspaceID string `json:"workspace_id"`
	WithCount   bool   `json:"with_count"` // Whether to include contact counts (can be expensive)
}

func (r *GetSegmentsRequest) FromURLParams(values url.Values) error {
	r.WorkspaceID = values.Get("workspace_id")
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	// Parse with_count parameter (defaults to false)
	r.WithCount = values.Get("with_count") == "true" || values.Get("with_count") == "1"
	return nil
}

type GetSegmentRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

func (r *GetSegmentRequest) FromURLParams(values url.Values) error {
	r.WorkspaceID = values.Get("workspace_id")
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	r.ID = values.Get("id")
	if r.ID == "" {
		return fmt.Errorf("id is required")
	}

	return nil
}

func (r *GetSegmentRequest) Validate() (workspaceID string, id string, err error) {
	if r.WorkspaceID == "" {
		return "", "", fmt.Errorf("invalid get segment request: workspace_id is required")
	}

	if r.ID == "" {
		return "", "", fmt.Errorf("invalid get segment request: id is required")
	}
	if !govalidator.IsAlphanumeric(r.ID) {
		return "", "", fmt.Errorf("invalid get segment request: id must be alphanumeric")
	}
	if len(r.ID) > 32 {
		return "", "", fmt.Errorf("invalid get segment request: id length must be between 1 and 32")
	}

	return r.WorkspaceID, r.ID, nil
}

type UpdateSegmentRequest struct {
	WorkspaceID string    `json:"workspace_id"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	Tree        *TreeNode `json:"tree"`
	Timezone    string    `json:"timezone"`
}

func (r *UpdateSegmentRequest) Validate() (segment *Segment, workspaceID string, err error) {
	if r.WorkspaceID == "" {
		return nil, "", fmt.Errorf("invalid update segment request: workspace_id is required")
	}
	if r.ID == "" {
		return nil, "", fmt.Errorf("invalid update segment request: id is required")
	}
	if !govalidator.IsAlphanumeric(r.ID) {
		return nil, "", fmt.Errorf("invalid update segment request: id must be alphanumeric")
	}
	if len(r.ID) > 32 {
		return nil, "", fmt.Errorf("invalid update segment request: id length must be between 1 and 32")
	}

	if r.Name == "" {
		return nil, "", fmt.Errorf("invalid update segment request: name is required")
	}
	if len(r.Name) > 255 {
		return nil, "", fmt.Errorf("invalid update segment request: name length must be between 1 and 255")
	}

	if r.Color == "" {
		return nil, "", fmt.Errorf("invalid update segment request: color is required")
	}
	if len(r.Color) > 50 {
		return nil, "", fmt.Errorf("invalid update segment request: color length must be between 1 and 50")
	}

	if r.Timezone == "" {
		return nil, "", fmt.Errorf("invalid update segment request: timezone is required")
	}
	if len(r.Timezone) > 100 {
		return nil, "", fmt.Errorf("invalid update segment request: timezone length must be between 1 and 100")
	}

	if r.Tree == nil {
		return nil, "", fmt.Errorf("invalid update segment request: tree is required")
	}

	if err := r.Tree.Validate(); err != nil {
		return nil, "", fmt.Errorf("invalid update segment request: invalid tree: %w", err)
	}

	return &Segment{
		ID:       r.ID,
		Name:     r.Name,
		Color:    r.Color,
		Tree:     r.Tree,
		Timezone: r.Timezone,
	}, r.WorkspaceID, nil
}

type DeleteSegmentRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

func (r *DeleteSegmentRequest) Validate() (workspaceID string, id string, err error) {
	if r.WorkspaceID == "" {
		return "", "", fmt.Errorf("invalid delete segment request: workspace_id is required")
	}

	if r.ID == "" {
		return "", "", fmt.Errorf("invalid delete segment request: id is required")
	}
	if !govalidator.IsAlphanumeric(r.ID) {
		return "", "", fmt.Errorf("invalid delete segment request: id must be alphanumeric")
	}
	if len(r.ID) > 32 {
		return "", "", fmt.Errorf("invalid delete segment request: id length must be between 1 and 32")
	}

	return r.WorkspaceID, r.ID, nil
}

type PreviewSegmentResponse struct {
	Emails       []string      `json:"emails"`
	TotalCount   int           `json:"total_count"`
	Limit        int           `json:"limit"`
	GeneratedSQL string        `json:"generated_sql"`
	SQLArgs      []interface{} `json:"sql_args"`
}

// SegmentService provides operations for managing segments
type SegmentService interface {
	// CreateSegment creates a new segment
	CreateSegment(ctx context.Context, req *CreateSegmentRequest) (*Segment, error)

	// GetSegment retrieves a segment by ID
	GetSegment(ctx context.Context, req *GetSegmentRequest) (*Segment, error)

	// ListSegments retrieves all segments
	ListSegments(ctx context.Context, req *GetSegmentsRequest) ([]*Segment, error)

	// UpdateSegment updates an existing segment
	UpdateSegment(ctx context.Context, req *UpdateSegmentRequest) (*Segment, error)

	// DeleteSegment deletes a segment by ID
	DeleteSegment(ctx context.Context, req *DeleteSegmentRequest) error

	// RebuildSegment triggers a rebuild of a segment
	RebuildSegment(ctx context.Context, workspaceID, segmentID string) error

	// PreviewSegment previews the contacts that would match a segment tree
	PreviewSegment(ctx context.Context, workspaceID string, tree *TreeNode, limit int) (*PreviewSegmentResponse, error)

	// GetSegmentContacts retrieves the contacts belonging to a segment
	GetSegmentContacts(ctx context.Context, workspaceID, segmentID string, limit, offset int) ([]string, error)
}

type SegmentRepository interface {
	// CreateSegment creates a new segment in the database
	CreateSegment(ctx context.Context, workspaceID string, segment *Segment) error

	// GetSegmentByID retrieves a segment by its ID
	GetSegmentByID(ctx context.Context, workspaceID string, id string) (*Segment, error)

	// GetSegments retrieves all segments, optionally with contact counts
	GetSegments(ctx context.Context, workspaceID string, withCount bool) ([]*Segment, error)

	// UpdateSegment updates an existing segment
	UpdateSegment(ctx context.Context, workspaceID string, segment *Segment) error

	// DeleteSegment deletes a segment
	DeleteSegment(ctx context.Context, workspaceID string, id string) error

	// AddContactToSegment adds a contact to a segment membership
	AddContactToSegment(ctx context.Context, workspaceID string, email string, segmentID string, version int64) error

	// RemoveContactFromSegment removes a contact from a segment
	RemoveContactFromSegment(ctx context.Context, workspaceID string, email string, segmentID string) error

	// RemoveOldMemberships removes contact_segment records with old versions
	RemoveOldMemberships(ctx context.Context, workspaceID string, segmentID string, currentVersion int64) error

	// GetContactSegments retrieves all segments a contact belongs to
	GetContactSegments(ctx context.Context, workspaceID string, email string) ([]*Segment, error)

	// GetSegmentContactCount gets the count of contacts in a segment
	GetSegmentContactCount(ctx context.Context, workspaceID string, segmentID string) (int, error)

	// PreviewSegment executes a segment query and returns the count of matching contacts
	PreviewSegment(ctx context.Context, workspaceID string, sqlQuery string, args []interface{}) (int, error)
}

// ErrSegmentNotFound is returned when a segment is not found
type ErrSegmentNotFound struct {
	Message string
}

func (e *ErrSegmentNotFound) Error() string {
	return e.Message
}
