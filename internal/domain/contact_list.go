package domain

import (
	"context"
	"fmt"
	"time"

	"github.com/asaskevich/govalidator"
)

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
	ContactID string            `json:"contact_id" valid:"required,uuid"`
	ListID    string            `json:"list_id" valid:"required,alphanum,stringlength(1|20)"`
	Status    ContactListStatus `json:"status" valid:"required,in(active|pending|unsubscribed|bounced|complained)"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// Validate performs validation on the contact list fields
func (cl *ContactList) Validate() error {
	if _, err := govalidator.ValidateStruct(cl); err != nil {
		return fmt.Errorf("invalid contact list: %w", err)
	}
	return nil
}

// For database scanning
type dbContactList struct {
	ContactID string
	ListID    string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ScanContactList scans a contact list from the database
func ScanContactList(scanner interface {
	Scan(dest ...interface{}) error
}) (*ContactList, error) {
	var dbcl dbContactList
	if err := scanner.Scan(
		&dbcl.ContactID,
		&dbcl.ListID,
		&dbcl.Status,
		&dbcl.CreatedAt,
		&dbcl.UpdatedAt,
	); err != nil {
		return nil, err
	}

	cl := &ContactList{
		ContactID: dbcl.ContactID,
		ListID:    dbcl.ListID,
		Status:    ContactListStatus(dbcl.Status),
		CreatedAt: dbcl.CreatedAt,
		UpdatedAt: dbcl.UpdatedAt,
	}

	return cl, nil
}

type ContactListRepository interface {
	// AddContactToList adds a contact to a list
	AddContactToList(ctx context.Context, contactList *ContactList) error

	// GetContactListByIDs retrieves a contact list by contact ID and list ID
	GetContactListByIDs(ctx context.Context, contactID, listID string) (*ContactList, error)

	// GetContactsByListID retrieves all contacts for a list
	GetContactsByListID(ctx context.Context, listID string) ([]*ContactList, error)

	// GetListsByContactID retrieves all lists for a contact
	GetListsByContactID(ctx context.Context, contactID string) ([]*ContactList, error)

	// UpdateContactListStatus updates the status of a contact on a list
	UpdateContactListStatus(ctx context.Context, contactID, listID string, status ContactListStatus) error

	// RemoveContactFromList removes a contact from a list
	RemoveContactFromList(ctx context.Context, contactID, listID string) error
}

// ErrContactListNotFound is returned when a contact list is not found
type ErrContactListNotFound struct {
	Message string
}

func (e *ErrContactListNotFound) Error() string {
	return e.Message
}
