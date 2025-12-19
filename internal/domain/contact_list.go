package domain

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/asaskevich/govalidator"
)

//go:generate mockgen -destination mocks/mock_contact_list_service.go -package mocks github.com/Notifuse/notifuse/internal/domain ContactListService
//go:generate mockgen -destination mocks/mock_contact_list_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain ContactListRepository

// ContactListStatus represents the status of a contact's subscription to a list
type ContactListStatus string

const (
	// ContactListStatusActive indicates an active subscription
	ContactListStatusActive ContactListStatus = "active"
	// ContactListStatusPending indicates a pending subscription (e.g., waiting for double opt-in)
	ContactListStatusPending ContactListStatus = "pending"
	// ContactListStatusUnsubscribed indicates an unsubscribed status
	ContactListStatusUnsubscribed ContactListStatus = "unsubscribed"
	// ContactListStatusBounced indicates the contact's email has bounced
	ContactListStatusBounced ContactListStatus = "bounced"
	// ContactListStatusComplained indicates the contact has complained (e.g., marked as spam)
	ContactListStatusComplained ContactListStatus = "complained"
)

// ContactList represents the relationship between a contact and a list
type ContactList struct {
	Email     string            `json:"email"`
	ListID    string            `json:"list_id"`
	ListName  string            `json:"list_name"`
	Status    ContactListStatus `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	DeletedAt *time.Time        `json:"deleted_at"`
}

// Validate performs validation on the contact list fields
func (cl *ContactList) Validate() error {
	// Check required fields
	if cl.Email == "" {
		return fmt.Errorf("email is required")
	}
	if !govalidator.IsEmail(cl.Email) {
		return fmt.Errorf("invalid email format: %s", cl.Email)
	}

	if cl.ListID == "" {
		return fmt.Errorf("list_id is required")
	}
	if len(cl.ListID) > 20 {
		return fmt.Errorf("list_id length must be between 1 and 20")
	}

	if cl.Status == "" {
		return fmt.Errorf("status is required")
	}

	// Validate status is one of the allowed values
	validStatus := false
	for _, status := range []ContactListStatus{
		ContactListStatusActive,
		ContactListStatusPending,
		ContactListStatusUnsubscribed,
		ContactListStatusBounced,
		ContactListStatusComplained,
	} {
		if cl.Status == status {
			validStatus = true
			break
		}
	}

	if !validStatus {
		return fmt.Errorf("invalid status: %s", cl.Status)
	}

	return nil
}

// For database scanning
type dbContactList struct {
	Email     string
	ListID    string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// ScanContactList scans a contact list from the database
func ScanContactList(scanner interface {
	Scan(dest ...interface{}) error
}) (*ContactList, error) {
	var dbcl dbContactList
	if err := scanner.Scan(
		&dbcl.Email,
		&dbcl.ListID,
		&dbcl.Status,
		&dbcl.CreatedAt,
		&dbcl.UpdatedAt,
		&dbcl.DeletedAt,
	); err != nil {
		return nil, err
	}

	cl := &ContactList{
		Email:     dbcl.Email,
		ListID:    dbcl.ListID,
		Status:    ContactListStatus(dbcl.Status),
		CreatedAt: dbcl.CreatedAt,
		UpdatedAt: dbcl.UpdatedAt,
		DeletedAt: dbcl.DeletedAt,
	}

	return cl, nil
}

// // Request/Response types
// type AddContactToListRequest struct {
// 	WorkspaceID string `json:"workspace_id"`
// 	Email       string `json:"email"`
// 	ListID      string `json:"list_id"`
// 	Status      string `json:"status"`
// }

// func (r *AddContactToListRequest) Validate() (contactList *ContactList, workspaceID string, err error) {
// 	if r.WorkspaceID == "" {
// 		return nil, "", fmt.Errorf("workspace_id is required")
// 	}

// 	if r.Email == "" {
// 		return nil, "", fmt.Errorf("email is required")
// 	}
// 	if !govalidator.IsEmail(r.Email) {
// 		return nil, "", fmt.Errorf("invalid email format: %s", r.Email)
// 	}

// 	if r.ListID == "" {
// 		return nil, "", fmt.Errorf("list_id is required")
// 	}

// 	if r.Status == "" {
// 		return nil, "", fmt.Errorf("status is required")
// 	}

// 	// Validate status is one of the allowed values
// 	validStatus := false
// 	for _, status := range []string{"active", "pending", "unsubscribed", "blacklisted"} {
// 		if r.Status == status {
// 			validStatus = true
// 			break
// 		}
// 	}

// 	if !validStatus {
// 		return nil, "", fmt.Errorf("invalid status: %s", r.Status)
// 	}

// 	return &ContactList{
// 		Email:  r.Email,
// 		ListID: r.ListID,
// 		Status: ContactListStatus(r.Status),
// 	}, r.WorkspaceID, nil
// }

type GetContactListRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Email       string `json:"email"`
	ListID      string `json:"list_id"`
}

func (r *GetContactListRequest) FromURLParams(queryParams url.Values) (err error) {
	r.WorkspaceID = queryParams.Get("workspace_id")
	r.Email = queryParams.Get("email")
	r.ListID = queryParams.Get("list_id")

	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if r.Email == "" {
		return fmt.Errorf("email is required")
	}
	if !govalidator.IsEmail(r.Email) {
		return fmt.Errorf("invalid email format: %s", r.Email)
	}

	if r.ListID == "" {
		return fmt.Errorf("list_id is required")
	}

	return nil
}

type GetContactsByListRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ListID      string `json:"list_id"`
}

func (r *GetContactsByListRequest) FromURLParams(queryParams url.Values) (err error) {
	r.WorkspaceID = queryParams.Get("workspace_id")
	r.ListID = queryParams.Get("list_id")

	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if r.ListID == "" {
		return fmt.Errorf("list_id is required")
	}

	if !govalidator.IsAlphanumeric(r.ListID) {
		return fmt.Errorf("list_id must be alphanumeric")
	}

	return nil
}

type GetListsByContactRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Email       string `json:"email"`
}

func (r *GetListsByContactRequest) FromURLParams(queryParams url.Values) (err error) {
	r.WorkspaceID = queryParams.Get("workspace_id")
	r.Email = queryParams.Get("email")

	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if r.Email == "" {
		return fmt.Errorf("email is required")
	}
	if !govalidator.IsEmail(r.Email) {
		return fmt.Errorf("invalid email format: %s", r.Email)
	}

	return nil
}

type UpdateContactListStatusRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Email       string `json:"email"`
	ListID      string `json:"list_id"`
	Status      string `json:"status"`
}

func (r *UpdateContactListStatusRequest) Validate() (workspaceID string, list *ContactList, err error) {
	if r.WorkspaceID == "" {
		return "", nil, fmt.Errorf("workspace_id is required")
	}

	if r.Email == "" {
		return "", nil, fmt.Errorf("email is required")
	}
	if !govalidator.IsEmail(r.Email) {
		return "", nil, fmt.Errorf("invalid email format: %s", r.Email)
	}

	if r.ListID == "" {
		return "", nil, fmt.Errorf("list_id is required")
	}

	if r.Status == "" {
		return "", nil, fmt.Errorf("status is required")
	}

	// Validate status is one of the allowed values
	validStatus := false
	for _, status := range []string{
		string(ContactListStatusActive),
		string(ContactListStatusPending),
		string(ContactListStatusUnsubscribed),
		string(ContactListStatusBounced),
		string(ContactListStatusComplained),
	} {
		if r.Status == status {
			validStatus = true
			break
		}
	}

	if !validStatus {
		return "", nil, fmt.Errorf("invalid status: %s", r.Status)
	}

	return r.WorkspaceID, &ContactList{
		Email:  r.Email,
		ListID: r.ListID,
		Status: ContactListStatus(r.Status),
	}, nil
}

type RemoveContactFromListRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Email       string `json:"email"`
	ListID      string `json:"list_id"`
}

func (r *RemoveContactFromListRequest) Validate() (err error) {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if r.Email == "" {
		return fmt.Errorf("email is required")
	}
	if !govalidator.IsEmail(r.Email) {
		return fmt.Errorf("invalid email format: %s", r.Email)
	}

	if r.ListID == "" {
		return fmt.Errorf("list_id is required")
	}

	return nil
}

// ContactListService provides operations for managing contact list relationships
type ContactListService interface {

	// GetContactListByIDs retrieves a contact list by email and list ID
	GetContactListByIDs(ctx context.Context, workspaceID string, email, listID string) (*ContactList, error)

	// GetContactsByListID retrieves all contacts for a list
	GetContactsByListID(ctx context.Context, workspaceID string, listID string) ([]*ContactList, error)

	// GetListsByEmail retrieves all lists for a contact
	GetListsByEmail(ctx context.Context, workspaceID string, email string) ([]*ContactList, error)

	// UpdateContactListStatus updates the status of a contact on a list
	UpdateContactListStatus(ctx context.Context, workspaceID string, email, listID string, status ContactListStatus) (*UpdateContactListStatusResult, error)

	// RemoveContactFromList removes a contact from a list
	RemoveContactFromList(ctx context.Context, workspaceID string, email, listID string) error
}

type ContactListRepository interface {
	// AddContactToList adds a contact to a list
	AddContactToList(ctx context.Context, workspaceID string, contactList *ContactList) error

	// BulkAddContactsToLists adds multiple contacts to multiple lists in a single operation
	BulkAddContactsToLists(ctx context.Context, workspaceID string, emails []string, listIDs []string, status ContactListStatus) error

	// GetContactListByIDs retrieves a contact list by email and list ID
	GetContactListByIDs(ctx context.Context, workspaceID string, email, listID string) (*ContactList, error)

	// GetContactsByListID retrieves all contacts for a list
	GetContactsByListID(ctx context.Context, workspaceID string, listID string) ([]*ContactList, error)

	// GetListsByEmail retrieves all lists for a contact
	GetListsByEmail(ctx context.Context, workspaceID string, email string) ([]*ContactList, error)

	// UpdateContactListStatus updates the status of a contact on a list
	UpdateContactListStatus(ctx context.Context, workspaceID string, email, listID string, status ContactListStatus) error

	// RemoveContactFromList removes a contact from a list
	RemoveContactFromList(ctx context.Context, workspaceID string, email, listID string) error

	// DeleteForEmail deletes all contact list relationships for a specific email
	DeleteForEmail(ctx context.Context, workspaceID, email string) error
}

// ErrContactListNotFound is returned when a contact list is not found
type ErrContactListNotFound struct {
	Message string
}

func (e *ErrContactListNotFound) Error() string {
	return e.Message
}

// UpdateContactListStatusResult represents the result of updating a contact list status
type UpdateContactListStatusResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Found   bool   `json:"found"` // Whether the contact was found in the list
}
