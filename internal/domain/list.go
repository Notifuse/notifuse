package domain

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/asaskevich/govalidator"
)

//go:generate mockgen -destination mocks/mock_list_service.go -package mocks github.com/Notifuse/notifuse/internal/domain ListService
//go:generate mockgen -destination mocks/mock_list_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain ListRepository

// List represents a subscription list
type List struct {
	ID                  string             `json:"id"`
	Name                string             `json:"name"`
	IsDoubleOptin       bool               `json:"is_double_optin" db:"is_double_optin"`
	IsPublic            bool               `json:"is_public" db:"is_public"`
	Description         string             `json:"description,omitempty"`
	DoubleOptInTemplate *TemplateReference `json:"double_optin_template,omitempty"`
	CreatedAt           time.Time          `json:"created_at"`
	UpdatedAt           time.Time          `json:"updated_at"`
	DeletedAt           *time.Time         `json:"-" db:"deleted_at"`
}

// Validate performs validation on the list fields
func (l *List) Validate() error {
	if l.ID == "" {
		return fmt.Errorf("invalid list: id is required")
	}
	if !govalidator.IsAlphanumeric(l.ID) {
		return fmt.Errorf("invalid list: id must be alphanumeric")
	}
	if len(l.ID) > 32 {
		return fmt.Errorf("invalid list: id length must be between 1 and 32")
	}

	if l.Name == "" {
		return fmt.Errorf("invalid list: name is required")
	}
	if len(l.Name) > 255 {
		return fmt.Errorf("invalid list: name length must be between 1 and 255")
	}

	// Validate optional template references if they exist
	if l.DoubleOptInTemplate != nil {
		if err := l.DoubleOptInTemplate.Validate(); err != nil {
			return fmt.Errorf("invalid list: double opt-in template: %w", err)
		}
	}

	return nil
}

// For database scanning
type dbList struct {
	ID                  string
	Name                string
	IsDoubleOptin       bool
	IsPublic            bool
	Description         string
	DoubleOptInTemplate *TemplateReference
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           *time.Time
}

// ScanList scans a list from the database
func ScanList(scanner interface {
	Scan(dest ...interface{}) error
}) (*List, error) {
	var dbl dbList
	if err := scanner.Scan(
		&dbl.ID,
		&dbl.Name,
		&dbl.IsDoubleOptin,
		&dbl.IsPublic,
		&dbl.Description,
		&dbl.DoubleOptInTemplate,
		&dbl.CreatedAt,
		&dbl.UpdatedAt,
		&dbl.DeletedAt,
	); err != nil {
		return nil, err
	}

	l := &List{
		ID:                  dbl.ID,
		Name:                dbl.Name,
		IsDoubleOptin:       dbl.IsDoubleOptin,
		IsPublic:            dbl.IsPublic,
		Description:         dbl.Description,
		DoubleOptInTemplate: dbl.DoubleOptInTemplate,
		CreatedAt:           dbl.CreatedAt,
		UpdatedAt:           dbl.UpdatedAt,
		DeletedAt:           dbl.DeletedAt,
	}

	return l, nil
}

// Request/Response types
type CreateListRequest struct {
	WorkspaceID         string             `json:"workspace_id"`
	ID                  string             `json:"id"`
	Name                string             `json:"name"`
	IsDoubleOptin       bool               `json:"is_double_optin"`
	IsPublic            bool               `json:"is_public"`
	Description         string             `json:"description,omitempty"`
	DoubleOptInTemplate *TemplateReference `json:"double_optin_template,omitempty"`
}

func (r *CreateListRequest) Validate() (list *List, workspaceID string, err error) {
	if r.WorkspaceID == "" {
		return nil, "", fmt.Errorf("invalid create list request: workspace_id is required")
	}
	if r.ID == "" {
		return nil, "", fmt.Errorf("invalid create list request: id is required")
	}
	if !govalidator.IsAlphanumeric(r.ID) {
		return nil, "", fmt.Errorf("invalid create list request: id must be alphanumeric")
	}
	if len(r.ID) > 32 {
		return nil, "", fmt.Errorf("invalid create list request: id length must be between 1 and 32")
	}

	if r.Name == "" {
		return nil, "", fmt.Errorf("invalid create list request: name is required")
	}
	if len(r.Name) > 255 {
		return nil, "", fmt.Errorf("invalid create list request: name length must be between 1 and 255")
	}

	// Validate optional template references if they exist
	if r.DoubleOptInTemplate != nil {
		if err := r.DoubleOptInTemplate.Validate(); err != nil {
			return nil, "", fmt.Errorf("invalid create list request: double opt-in template: %w", err)
		}
	}

	if r.IsDoubleOptin && r.DoubleOptInTemplate == nil {
		return nil, "", fmt.Errorf("invalid create list request: double opt-in template is required when is_double_optin is true")
	}

	return &List{
		ID:                  r.ID,
		Name:                r.Name,
		IsDoubleOptin:       r.IsDoubleOptin,
		IsPublic:            r.IsPublic,
		Description:         r.Description,
		DoubleOptInTemplate: r.DoubleOptInTemplate,
	}, r.WorkspaceID, nil
}

type GetListsRequest struct {
	WorkspaceID string `json:"workspace_id"`
}

func (r *GetListsRequest) FromURLParams(queryParams url.Values) (err error) {
	r.WorkspaceID = queryParams.Get("workspace_id")

	if r.WorkspaceID == "" {
		return fmt.Errorf("invalid get lists request: workspace_id is required")
	}
	if len(r.WorkspaceID) > 20 {
		return fmt.Errorf("invalid get lists request: workspace_id length must be between 1 and 20")
	}

	return nil
}

type GetListRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

func (r *GetListRequest) FromURLParams(queryParams url.Values) (err error) {
	r.WorkspaceID = queryParams.Get("workspace_id")
	r.ID = queryParams.Get("id")

	if r.WorkspaceID == "" {
		return fmt.Errorf("invalid get list request: workspace_id is required")
	}

	if r.ID == "" {
		return fmt.Errorf("invalid get list request: id is required")
	}
	if !govalidator.IsAlphanumeric(r.ID) {
		return fmt.Errorf("invalid get list request: id must be alphanumeric")
	}
	if len(r.ID) > 32 {
		return fmt.Errorf("invalid get list request: id length must be between 1 and 32")
	}

	return nil
}

type UpdateListRequest struct {
	WorkspaceID         string             `json:"workspace_id"`
	ID                  string             `json:"id"`
	Name                string             `json:"name"`
	IsDoubleOptin       bool               `json:"is_double_optin"`
	IsPublic            bool               `json:"is_public"`
	Description         string             `json:"description,omitempty"`
	DoubleOptInTemplate *TemplateReference `json:"double_optin_template,omitempty"`
}

func (r *UpdateListRequest) Validate() (list *List, workspaceID string, err error) {
	if r.WorkspaceID == "" {
		return nil, "", fmt.Errorf("invalid update list request: workspace_id is required")
	}
	if r.ID == "" {
		return nil, "", fmt.Errorf("invalid update list request: id is required")
	}
	if !govalidator.IsAlphanumeric(r.ID) {
		return nil, "", fmt.Errorf("invalid update list request: id must be alphanumeric")
	}
	if len(r.ID) > 32 {
		return nil, "", fmt.Errorf("invalid update list request: id length must be between 1 and 32")
	}

	if r.Name == "" {
		return nil, "", fmt.Errorf("invalid update list request: name is required")
	}
	if len(r.Name) > 255 {
		return nil, "", fmt.Errorf("invalid update list request: name length must be between 1 and 255")
	}

	// Validate optional template references if they exist
	if r.DoubleOptInTemplate != nil {
		if err := r.DoubleOptInTemplate.Validate(); err != nil {
			return nil, "", fmt.Errorf("invalid update list request: double opt-in template: %w", err)
		}
	}

	if r.IsDoubleOptin && r.DoubleOptInTemplate == nil {
		return nil, "", fmt.Errorf("invalid update list request: double opt-in template is required when is_double_optin is true")
	}

	return &List{
		ID:                  r.ID,
		Name:                r.Name,
		IsDoubleOptin:       r.IsDoubleOptin,
		IsPublic:            r.IsPublic,
		Description:         r.Description,
		DoubleOptInTemplate: r.DoubleOptInTemplate,
	}, r.WorkspaceID, nil
}

type DeleteListRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

func (r *DeleteListRequest) Validate() (workspaceID string, err error) {
	if r.WorkspaceID == "" {
		return "", fmt.Errorf("invalid delete list request: workspace_id is required")
	}

	if r.ID == "" {
		return "", fmt.Errorf("invalid delete list request: id is required")
	}
	if !govalidator.IsAlphanumeric(r.ID) {
		return "", fmt.Errorf("invalid delete list request: id must be alphanumeric")
	}
	if len(r.ID) > 32 {
		return "", fmt.Errorf("invalid delete list request: id length must be between 1 and 32")
	}

	return r.WorkspaceID, nil
}

type ListStats struct {
	TotalActive       int `json:"total_active"`
	TotalPending      int `json:"total_pending"`
	TotalUnsubscribed int `json:"total_unsubscribed"`
	TotalBounced      int `json:"total_bounced"`
	TotalComplained   int `json:"total_complained"`
}

type SubscribeToListsRequest struct {
	WorkspaceID string   `json:"workspace_id"`
	Contact     Contact  `json:"contact"`
	ListIDs     []string `json:"list_ids"`
}

func (r *SubscribeToListsRequest) Validate() (err error) {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if err := r.Contact.Validate(); err != nil {
		return fmt.Errorf("invalid contact: %w", err)
	}

	if len(r.ListIDs) == 0 {
		return fmt.Errorf("list_ids is required")
	}

	return nil
}

type UnsubscribeFromListsRequest struct {
	WorkspaceID string   `json:"wid"`
	Email       string   `json:"email"`
	EmailHMAC   string   `json:"email_hmac"`
	ListIDs     []string `json:"lids"`
	MessageID   string   `json:"mid"`
}

// from url params
func (r *UnsubscribeFromListsRequest) FromURLParams(queryParams url.Values) (err error) {
	r.WorkspaceID = queryParams.Get("workspace_id")
	r.Email = queryParams.Get("email")
	r.EmailHMAC = queryParams.Get("email_hmac")
	r.ListIDs = queryParams["list_ids"]
	r.MessageID = queryParams.Get("mid")

	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if r.Email == "" {
		return fmt.Errorf("email is required")
	}

	if r.MessageID == "" {
		return fmt.Errorf("message_id is required")
	}

	if len(r.ListIDs) == 0 {
		return fmt.Errorf("list_ids is required")
	}

	return nil
}

// ListService provides operations for managing lists
type ListService interface {
	// SubscribeToLists subscribes a contact to a list
	SubscribeToLists(ctx context.Context, payload *SubscribeToListsRequest, hasBearerToken bool) error

	// UnsubscribeFromLists unsubscribes a contact from a list
	UnsubscribeFromLists(ctx context.Context, payload *UnsubscribeFromListsRequest, hasBearerToken bool) error

	// CreateList creates a new list
	CreateList(ctx context.Context, workspaceID string, list *List) error

	// GetListByID retrieves a list by ID
	GetListByID(ctx context.Context, workspaceID string, id string) (*List, error)

	// GetLists retrieves all lists
	GetLists(ctx context.Context, workspaceID string) ([]*List, error)

	// UpdateList updates an existing list
	UpdateList(ctx context.Context, workspaceID string, list *List) error

	// DeleteList deletes a list by ID
	DeleteList(ctx context.Context, workspaceID string, id string) error

	GetListStats(ctx context.Context, workspaceID string, id string) (*ListStats, error)
}

type ListRepository interface {
	// CreateList creates a new list in the database
	CreateList(ctx context.Context, workspaceID string, list *List) error

	// GetListByID retrieves a list by its ID
	GetListByID(ctx context.Context, workspaceID string, id string) (*List, error)

	// GetLists retrieves all lists
	GetLists(ctx context.Context, workspaceID string) ([]*List, error)

	// UpdateList updates an existing list
	UpdateList(ctx context.Context, workspaceID string, list *List) error

	// DeleteList deletes a list
	DeleteList(ctx context.Context, workspaceID string, id string) error

	GetListStats(ctx context.Context, workspaceID string, id string) (*ListStats, error)
}

// ErrListNotFound is returned when a list is not found
type ErrListNotFound struct {
	Message string
}

func (e *ErrListNotFound) Error() string {
	return e.Message
}
