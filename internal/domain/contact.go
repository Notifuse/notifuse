package domain

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/asaskevich/govalidator"
	"github.com/tidwall/gjson"
)

var (
	ErrContactNotFound = errors.New("contact not found")
)

//go:generate mockgen -destination mocks/mock_contact_service.go -package mocks github.com/Notifuse/notifuse/internal/domain ContactService
//go:generate mockgen -destination mocks/mock_contact_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain ContactRepository

// Contact represents a contact in the system
type Contact struct {
	// Required fields
	Email string `json:"email" valid:"required,email"`

	// Optional fields
	ExternalID   *NullableString `json:"external_id,omitempty" valid:"optional"`
	Timezone     *NullableString `json:"timezone,omitempty" valid:"optional,timezone"`
	Language     *NullableString `json:"language,omitempty" valid:"optional"`
	FirstName    *NullableString `json:"first_name,omitempty" valid:"optional"`
	LastName     *NullableString `json:"last_name,omitempty" valid:"optional"`
	FullName     *NullableString `json:"full_name,omitempty" valid:"optional"`
	Phone        *NullableString `json:"phone,omitempty" valid:"optional"`
	AddressLine1 *NullableString `json:"address_line_1,omitempty" valid:"optional"`
	AddressLine2 *NullableString `json:"address_line_2,omitempty" valid:"optional"`
	Country      *NullableString `json:"country,omitempty" valid:"optional"`
	Postcode     *NullableString `json:"postcode,omitempty" valid:"optional"`
	State        *NullableString `json:"state,omitempty" valid:"optional"`
	JobTitle     *NullableString `json:"job_title,omitempty" valid:"optional"`

	// Custom fields
	CustomString1 *NullableString `json:"custom_string_1,omitempty" valid:"optional"`
	CustomString2 *NullableString `json:"custom_string_2,omitempty" valid:"optional"`
	CustomString3 *NullableString `json:"custom_string_3,omitempty" valid:"optional"`
	CustomString4 *NullableString `json:"custom_string_4,omitempty" valid:"optional"`
	CustomString5 *NullableString `json:"custom_string_5,omitempty" valid:"optional"`

	CustomNumber1 *NullableFloat64 `json:"custom_number_1,omitempty" valid:"optional"`
	CustomNumber2 *NullableFloat64 `json:"custom_number_2,omitempty" valid:"optional"`
	CustomNumber3 *NullableFloat64 `json:"custom_number_3,omitempty" valid:"optional"`
	CustomNumber4 *NullableFloat64 `json:"custom_number_4,omitempty" valid:"optional"`
	CustomNumber5 *NullableFloat64 `json:"custom_number_5,omitempty" valid:"optional"`

	CustomDatetime1 *NullableTime `json:"custom_datetime_1,omitempty" valid:"optional"`
	CustomDatetime2 *NullableTime `json:"custom_datetime_2,omitempty" valid:"optional"`
	CustomDatetime3 *NullableTime `json:"custom_datetime_3,omitempty" valid:"optional"`
	CustomDatetime4 *NullableTime `json:"custom_datetime_4,omitempty" valid:"optional"`
	CustomDatetime5 *NullableTime `json:"custom_datetime_5,omitempty" valid:"optional"`

	CustomJSON1 *NullableJSON `json:"custom_json_1,omitempty" valid:"optional"`
	CustomJSON2 *NullableJSON `json:"custom_json_2,omitempty" valid:"optional"`
	CustomJSON3 *NullableJSON `json:"custom_json_3,omitempty" valid:"optional"`
	CustomJSON4 *NullableJSON `json:"custom_json_4,omitempty" valid:"optional"`
	CustomJSON5 *NullableJSON `json:"custom_json_5,omitempty" valid:"optional"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Database timestamps for internal tracking (not exposed via JSON)
	DBCreatedAt time.Time `json:"-"`
	DBUpdatedAt time.Time `json:"-"`

	// Join contact_lists
	ContactLists []*ContactList `json:"contact_lists"`
	// Join contact_segments
	ContactSegments []*ContactSegment `json:"contact_segments"`
	// Not persisted
	EmailHMAC string `json:"email_hmac,omitempty"`
}

// Validate ensures that the contact has all required fields
func (c *Contact) Validate() error {
	// Email is required
	if c.Email == "" {
		return fmt.Errorf("email is required")
	}
	// Email must be valid
	if !govalidator.IsEmail(c.Email) {
		return fmt.Errorf("invalid email format")
	}

	return nil
}

// VerifyEmailHMAC verifies if the provided HMAC for an email is valid
func VerifyEmailHMAC(email string, providedHMAC string, secretKey string) bool {
	// Use the crypto package to verify the HMAC
	computedHMAC := ComputeEmailHMAC(email, secretKey)
	return computedHMAC == providedHMAC
}

// ComputeEmailHMAC computes an HMAC for an email address using the workspace secret key
func ComputeEmailHMAC(email string, secretKey string) string {
	return crypto.ComputeHMAC256([]byte(email), secretKey)
}

// For database scanning
type dbContact struct {
	Email      string
	ExternalID sql.NullString
	Timezone   sql.NullString
	Language   sql.NullString

	FirstName    sql.NullString
	LastName     sql.NullString
	FullName     sql.NullString
	Phone        sql.NullString
	AddressLine1 sql.NullString
	AddressLine2 sql.NullString
	Country      sql.NullString
	Postcode     sql.NullString
	State        sql.NullString
	JobTitle     sql.NullString

	CustomString1 sql.NullString
	CustomString2 sql.NullString
	CustomString3 sql.NullString
	CustomString4 sql.NullString
	CustomString5 sql.NullString

	CustomNumber1 sql.NullFloat64
	CustomNumber2 sql.NullFloat64
	CustomNumber3 sql.NullFloat64
	CustomNumber4 sql.NullFloat64
	CustomNumber5 sql.NullFloat64

	CustomDatetime1 sql.NullTime
	CustomDatetime2 sql.NullTime
	CustomDatetime3 sql.NullTime
	CustomDatetime4 sql.NullTime
	CustomDatetime5 sql.NullTime

	CustomJSON1 []byte
	CustomJSON2 []byte
	CustomJSON3 []byte
	CustomJSON4 []byte
	CustomJSON5 []byte

	CreatedAt   time.Time
	UpdatedAt   time.Time
	DBCreatedAt time.Time
	DBUpdatedAt time.Time
}

// ScanContact scans a contact from the database
func ScanContact(scanner interface {
	Scan(dest ...interface{}) error
}) (*Contact, error) {
	var dbc dbContact

	// Try to scan with contact list fields first
	err := scanner.Scan(
		&dbc.Email,
		&dbc.ExternalID,
		&dbc.Timezone,
		&dbc.Language,
		&dbc.FirstName,
		&dbc.LastName,
		&dbc.FullName,
		&dbc.Phone,
		&dbc.AddressLine1,
		&dbc.AddressLine2,
		&dbc.Country,
		&dbc.Postcode,
		&dbc.State,
		&dbc.JobTitle,
		&dbc.CustomString1,
		&dbc.CustomString2,
		&dbc.CustomString3,
		&dbc.CustomString4,
		&dbc.CustomString5,
		&dbc.CustomNumber1,
		&dbc.CustomNumber2,
		&dbc.CustomNumber3,
		&dbc.CustomNumber4,
		&dbc.CustomNumber5,
		&dbc.CustomDatetime1,
		&dbc.CustomDatetime2,
		&dbc.CustomDatetime3,
		&dbc.CustomDatetime4,
		&dbc.CustomDatetime5,
		&dbc.CustomJSON1,
		&dbc.CustomJSON2,
		&dbc.CustomJSON3,
		&dbc.CustomJSON4,
		&dbc.CustomJSON5,
		&dbc.CreatedAt,
		&dbc.UpdatedAt,
		&dbc.DBCreatedAt,
		&dbc.DBUpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scan contact: %w", err)
	}

	// Convert to domain model
	c := &Contact{
		Email:       dbc.Email,
		CreatedAt:   dbc.CreatedAt,
		UpdatedAt:   dbc.UpdatedAt,
		DBCreatedAt: dbc.DBCreatedAt,
		DBUpdatedAt: dbc.DBUpdatedAt,
	}

	// Handle nullable fields
	if dbc.ExternalID.Valid {
		c.ExternalID = &NullableString{String: dbc.ExternalID.String, IsNull: false}
	}
	if dbc.Timezone.Valid {
		c.Timezone = &NullableString{String: dbc.Timezone.String, IsNull: false}
	}
	if dbc.Language.Valid {
		c.Language = &NullableString{String: dbc.Language.String, IsNull: false}
	}
	if dbc.FirstName.Valid {
		c.FirstName = &NullableString{String: dbc.FirstName.String, IsNull: false}
	}

	if dbc.LastName.Valid {
		c.LastName = &NullableString{String: dbc.LastName.String, IsNull: false}
	}

	if dbc.FullName.Valid {
		c.FullName = &NullableString{String: dbc.FullName.String, IsNull: false}
	}

	if dbc.Phone.Valid {
		c.Phone = &NullableString{String: dbc.Phone.String, IsNull: false}
	}

	if dbc.AddressLine1.Valid {
		c.AddressLine1 = &NullableString{String: dbc.AddressLine1.String, IsNull: false}
	}

	if dbc.AddressLine2.Valid {
		c.AddressLine2 = &NullableString{String: dbc.AddressLine2.String, IsNull: false}
	}

	if dbc.Country.Valid {
		c.Country = &NullableString{String: dbc.Country.String, IsNull: false}
	}

	if dbc.Postcode.Valid {
		c.Postcode = &NullableString{String: dbc.Postcode.String, IsNull: false}
	}

	if dbc.State.Valid {
		c.State = &NullableString{String: dbc.State.String, IsNull: false}
	}

	if dbc.JobTitle.Valid {
		c.JobTitle = &NullableString{String: dbc.JobTitle.String, IsNull: false}
	}

	if dbc.CustomString1.Valid {
		c.CustomString1 = &NullableString{String: dbc.CustomString1.String, IsNull: false}
	}

	if dbc.CustomString2.Valid {
		c.CustomString2 = &NullableString{String: dbc.CustomString2.String, IsNull: false}
	}

	if dbc.CustomString3.Valid {
		c.CustomString3 = &NullableString{String: dbc.CustomString3.String, IsNull: false}
	}

	if dbc.CustomString4.Valid {
		c.CustomString4 = &NullableString{String: dbc.CustomString4.String, IsNull: false}
	}

	if dbc.CustomString5.Valid {
		c.CustomString5 = &NullableString{String: dbc.CustomString5.String, IsNull: false}
	}

	if dbc.CustomNumber1.Valid {
		c.CustomNumber1 = &NullableFloat64{Float64: dbc.CustomNumber1.Float64, IsNull: false}
	}

	if dbc.CustomNumber2.Valid {
		c.CustomNumber2 = &NullableFloat64{Float64: dbc.CustomNumber2.Float64, IsNull: false}
	}

	if dbc.CustomNumber3.Valid {
		c.CustomNumber3 = &NullableFloat64{Float64: dbc.CustomNumber3.Float64, IsNull: false}
	}

	if dbc.CustomNumber4.Valid {
		c.CustomNumber4 = &NullableFloat64{Float64: dbc.CustomNumber4.Float64, IsNull: false}
	}

	if dbc.CustomNumber5.Valid {
		c.CustomNumber5 = &NullableFloat64{Float64: dbc.CustomNumber5.Float64, IsNull: false}
	}

	if dbc.CustomDatetime1.Valid {
		c.CustomDatetime1 = &NullableTime{Time: dbc.CustomDatetime1.Time, IsNull: false}
	}

	if dbc.CustomDatetime2.Valid {
		c.CustomDatetime2 = &NullableTime{Time: dbc.CustomDatetime2.Time, IsNull: false}
	}

	if dbc.CustomDatetime3.Valid {
		c.CustomDatetime3 = &NullableTime{Time: dbc.CustomDatetime3.Time, IsNull: false}
	}

	if dbc.CustomDatetime4.Valid {
		c.CustomDatetime4 = &NullableTime{Time: dbc.CustomDatetime4.Time, IsNull: false}
	}

	if dbc.CustomDatetime5.Valid {
		c.CustomDatetime5 = &NullableTime{Time: dbc.CustomDatetime5.Time, IsNull: false}
	}

	// Handle JSON fields
	if len(dbc.CustomJSON1) > 0 && string(dbc.CustomJSON1) != "null" {
		var data interface{}
		if err := json.Unmarshal(dbc.CustomJSON1, &data); err == nil {
			c.CustomJSON1 = &NullableJSON{Data: data, IsNull: false}
		}
	}
	if len(dbc.CustomJSON2) > 0 && string(dbc.CustomJSON2) != "null" {
		var data interface{}
		if err := json.Unmarshal(dbc.CustomJSON2, &data); err == nil {
			c.CustomJSON2 = &NullableJSON{Data: data, IsNull: false}
		}
	}
	if len(dbc.CustomJSON3) > 0 && string(dbc.CustomJSON3) != "null" {
		var data interface{}
		if err := json.Unmarshal(dbc.CustomJSON3, &data); err == nil {
			c.CustomJSON3 = &NullableJSON{Data: data, IsNull: false}
		}
	}
	if len(dbc.CustomJSON4) > 0 && string(dbc.CustomJSON4) != "null" {
		var data interface{}
		if err := json.Unmarshal(dbc.CustomJSON4, &data); err == nil {
			c.CustomJSON4 = &NullableJSON{Data: data, IsNull: false}
		}
	}
	if len(dbc.CustomJSON5) > 0 && string(dbc.CustomJSON5) != "null" {
		var data interface{}
		if err := json.Unmarshal(dbc.CustomJSON5, &data); err == nil {
			c.CustomJSON5 = &NullableJSON{Data: data, IsNull: false}
		}
	}

	return c, nil
}

// GetContactsRequest represents a request to get contacts with filters and pagination
type GetContactsRequest struct {
	// Required fields
	WorkspaceID string `json:"workspace_id" valid:"required,alphanum,stringlength(1|20)"`

	// Optional filters
	Email             string   `json:"email,omitempty" valid:"optional,email"`
	ExternalID        string   `json:"external_id,omitempty" valid:"optional"`
	FirstName         string   `json:"first_name,omitempty" valid:"optional"`
	LastName          string   `json:"last_name,omitempty" valid:"optional"`
	FullName          string   `json:"full_name,omitempty" valid:"optional"`
	Phone             string   `json:"phone,omitempty" valid:"optional"`
	Country           string   `json:"country,omitempty" valid:"optional"`
	Language          string   `json:"language,omitempty" valid:"optional"`
	ListID            string   `json:"list_id,omitempty" valid:"optional"`
	ContactListStatus string   `json:"contact_list_status,omitempty" valid:"optional"`
	Segments          []string `json:"segments,omitempty" valid:"optional"`

	// Join contact_lists
	WithContactLists bool `json:"with_contact_lists,omitempty" valid:"optional"`

	// Pagination
	Limit  int    `json:"limit,omitempty" valid:"optional,range(1|100)"`
	Cursor string `json:"cursor,omitempty" valid:"optional"`
}

// FromQueryParams populates the request from URL query parameters
func (r *GetContactsRequest) FromQueryParams(params url.Values) error {
	r.WorkspaceID = params.Get("workspace_id")
	r.Email = params.Get("email")
	r.ExternalID = params.Get("external_id")
	r.FirstName = params.Get("first_name")
	r.LastName = params.Get("last_name")
	r.FullName = params.Get("full_name")
	r.Phone = params.Get("phone")
	r.Country = params.Get("country")
	r.Language = params.Get("language")
	r.ListID = params.Get("list_id")
	r.ContactListStatus = params.Get("contact_list_status")
	r.Cursor = params.Get("cursor")

	// Parse segments array
	if segments, ok := params["segments[]"]; ok && len(segments) > 0 {
		r.Segments = segments
	}

	// Validate workspace ID
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	// Parse limit
	if limitStr := params.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return fmt.Errorf("invalid limit: %w", err)
		}
		if limit < 1 || limit > 100 {
			return fmt.Errorf("limit must be between 1 and 100")
		}
		r.Limit = limit
	}

	// Parse with_contact_lists
	if withContactListsStr := params.Get("with_contact_lists"); withContactListsStr != "" {
		withContactLists, err := strconv.ParseBool(withContactListsStr)
		if err != nil {
			return fmt.Errorf("invalid with_contact_lists: %w", err)
		}
		r.WithContactLists = withContactLists
	}

	// Validate email format if provided
	if r.Email != "" {
		if !govalidator.IsEmail(r.Email) {
			return fmt.Errorf("invalid email format")
		}
	}

	return nil
}

// GetContactsResponse represents the response from getting contacts
type GetContactsResponse struct {
	Contacts   []*Contact `json:"contacts"`
	NextCursor string     `json:"next_cursor,omitempty"`
}

// Validate ensures that the request has all required fields and valid values
func (r *GetContactsRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	// Set default limit if not provided
	if r.Limit == 0 {
		r.Limit = 20
	}

	// Enforce maximum limit
	if r.Limit > 100 {
		r.Limit = 100
	}

	return nil
}

// Request/Response types
type GetContactByEmailRequest struct {
	WorkspaceID string `json:"workspace_id" valid:"required"`
	Email       string `json:"email" valid:"required,email"`
}

type GetContactByExternalIDRequest struct {
	WorkspaceID string `json:"workspace_id" valid:"required"`
	ExternalID  string `json:"external_id" valid:"required"`
}

type DeleteContactRequest struct {
	WorkspaceID string `json:"workspace_id" valid:"required"`
	Email       string `json:"email" valid:"required,email"`
}

func (r *DeleteContactRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.Email == "" {
		return fmt.Errorf("email is required")
	}
	if !govalidator.IsEmail(r.Email) {
		return fmt.Errorf("invalid email format")
	}
	return nil
}

// Add the request type for batch importing contacts
type BatchImportContactsRequest struct {
	WorkspaceID      string          `json:"workspace_id" valid:"required"`
	Contacts         json.RawMessage `json:"contacts" valid:"required"`
	SubscribeToLists []string        `json:"subscribe_to_lists,omitempty"` // Optional: subscribe contacts to these lists
}

func (r *BatchImportContactsRequest) Validate() (contacts []*Contact, workspaceID string, err error) {
	if r.WorkspaceID == "" {
		return nil, "", fmt.Errorf("workspace_id is required")
	}

	// Parse the raw JSON bytes directly as an array
	jsonResult := gjson.ParseBytes(r.Contacts)
	if !jsonResult.IsArray() {
		return nil, "", fmt.Errorf("contacts must be an array")
	}

	contactsArray := jsonResult.Array()
	if len(contactsArray) == 0 {
		return nil, "", fmt.Errorf("contacts array is empty")
	}

	// Parse each contact
	// Note: We don't validate individual contacts here - that's done in the service layer
	// This allows for partial success (some contacts succeed, some fail validation)
	contacts = make([]*Contact, 0, len(contactsArray))
	for _, contactJson := range contactsArray {
		contact, err := FromJSON(contactJson)
		if err != nil {
			// For unparseable contacts, create an empty contact that will fail validation in the service
			contact = &Contact{}
		}
		contacts = append(contacts, contact)
	}
	return contacts, r.WorkspaceID, nil
}

type UpsertContactRequest struct {
	WorkspaceID string          `json:"workspace_id" valid:"required"`
	Contact     json.RawMessage `json:"contact" valid:"required"`
}

func (r *UpsertContactRequest) Validate() (contact *Contact, workspaceID string, err error) {
	if r.WorkspaceID == "" {
		return nil, "", fmt.Errorf("workspace_id is required")
	}
	jsonResult := gjson.ParseBytes(r.Contact)
	if !jsonResult.Exists() {
		return nil, "", fmt.Errorf("contact field is required")
	}
	contact, err = FromJSON(jsonResult)
	if err != nil {
		return nil, "", fmt.Errorf("invalid contact: %w", err)
	}
	return contact, r.WorkspaceID, nil
}

type BatchImportContactsResponse struct {
	Operations []*UpsertContactOperation `json:"operations"`
	Error      string                    `json:"error,omitempty"`
}

const (
	UpsertContactOperationCreate = "create"
	UpsertContactOperationUpdate = "update"
	UpsertContactOperationError  = "error"
)

type UpsertContactOperation struct {
	Email  string `json:"email"`
	Action string `json:"action"` // create or update or error
	Error  string `json:"error,omitempty"`
}

// ContactService provides operations for managing contacts
type ContactService interface {
	// GetContactByEmail retrieves a contact by email
	GetContactByEmail(ctx context.Context, workspaceID string, email string) (*Contact, error)

	// GetContactByExternalID retrieves a contact by external ID
	GetContactByExternalID(ctx context.Context, workspaceID string, externalID string) (*Contact, error)

	// GetContacts retrieves contacts with filters and pagination
	GetContacts(ctx context.Context, req *GetContactsRequest) (*GetContactsResponse, error)

	// DeleteContact deletes a contact by email
	DeleteContact(ctx context.Context, workspaceID string, email string) error

	// BatchImportContacts imports a batch of contacts (create or update)
	BatchImportContacts(ctx context.Context, workspaceID string, contacts []*Contact, listIDs []string) *BatchImportContactsResponse

	// UpsertContact creates a new contact or updates an existing one
	UpsertContact(ctx context.Context, workspaceID string, contact *Contact) UpsertContactOperation

	// CountContacts returns the total number of contacts in a workspace
	CountContacts(ctx context.Context, workspaceID string) (int, error)
}

// ContactRepository is the interface for contact operations
// BulkUpsertResult represents the result of a single contact upsert operation in a bulk operation
type BulkUpsertResult struct {
	Email string
	IsNew bool // true if inserted, false if updated
}

type ContactRepository interface {
	// GetContactByEmail retrieves a contact by email
	GetContactByEmail(ctx context.Context, workspaceID, email string) (*Contact, error)

	// GetContactByExternalID retrieves a contact by external ID
	GetContactByExternalID(ctx context.Context, workspaceID string, externalID string) (*Contact, error)

	// GetContacts retrieves contacts with filtering and pagination
	GetContacts(ctx context.Context, req *GetContactsRequest) (*GetContactsResponse, error)

	// DeleteContact deletes a contact
	DeleteContact(ctx context.Context, workspaceID string, email string) error

	// UpsertContact creates or updates a contact
	UpsertContact(ctx context.Context, workspaceID string, contact *Contact) (bool, error)

	// BulkUpsertContacts creates or updates multiple contacts in a single operation
	BulkUpsertContacts(ctx context.Context, workspaceID string, contacts []*Contact) ([]BulkUpsertResult, error)

	// GetContactsForBroadcast retrieves contacts based on broadcast audience settings
	// Uses cursor-based pagination: afterEmail is the last email from the previous batch (empty for first batch)
	GetContactsForBroadcast(ctx context.Context, workspaceID string, audience AudienceSettings, limit int, afterEmail string) ([]*ContactWithList, error)

	// CountContactsForBroadcast counts contacts based on broadcast audience settings
	CountContactsForBroadcast(ctx context.Context, workspaceID string, audience AudienceSettings) (int, error)

	// Count returns the total number of contacts in a workspace
	Count(ctx context.Context, workspaceID string) (int, error)

	// GetBatchForSegment retrieves a batch of email addresses for segment processing
	GetBatchForSegment(ctx context.Context, workspaceID string, offset int64, limit int) ([]string, error)
}

// FromJSON parses JSON data into a Contact struct
// The JSON data can be provided as a []byte or as a gjson.Result
func FromJSON(data interface{}) (*Contact, error) {
	var jsonResult gjson.Result

	switch v := data.(type) {
	case []byte:
		jsonResult = gjson.ParseBytes(v)
	case gjson.Result:
		jsonResult = v
	case string:
		jsonResult = gjson.Parse(v)
	default:
		return nil, fmt.Errorf("unsupported data type: %T", data)
	}

	// Extract required fields and trim all Unicode whitespace (including NBSP)
	email := trimUnicodeSpace(jsonResult.Get("email").String())
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}

	// Validate email format
	if !govalidator.IsEmail(email) {
		return nil, fmt.Errorf("invalid email format")
	}

	// Create the contact with required fields
	contact := &Contact{
		Email: email,
	}

	// Parse nullable string fields
	if err := parseNullableString(jsonResult, "external_id", &contact.ExternalID); err != nil {
		return nil, err
	}
	if err := parseNullableString(jsonResult, "timezone", &contact.Timezone); err != nil {
		return nil, err
	}
	if err := parseNullableString(jsonResult, "language", &contact.Language); err != nil {
		return nil, err
	}
	if err := parseNullableString(jsonResult, "first_name", &contact.FirstName); err != nil {
		return nil, err
	}
	if err := parseNullableString(jsonResult, "last_name", &contact.LastName); err != nil {
		return nil, err
	}
	if err := parseNullableString(jsonResult, "full_name", &contact.FullName); err != nil {
		return nil, err
	}
	if err := parseNullableString(jsonResult, "phone", &contact.Phone); err != nil {
		return nil, err
	}
	if err := parseNullableString(jsonResult, "address_line_1", &contact.AddressLine1); err != nil {
		return nil, err
	}
	if err := parseNullableString(jsonResult, "address_line_2", &contact.AddressLine2); err != nil {
		return nil, err
	}
	if err := parseNullableString(jsonResult, "country", &contact.Country); err != nil {
		return nil, err
	}
	if err := parseNullableString(jsonResult, "postcode", &contact.Postcode); err != nil {
		return nil, err
	}
	if err := parseNullableString(jsonResult, "state", &contact.State); err != nil {
		return nil, err
	}
	if err := parseNullableString(jsonResult, "job_title", &contact.JobTitle); err != nil {
		return nil, err
	}

	// Parse custom string fields
	if err := parseNullableString(jsonResult, "custom_string_1", &contact.CustomString1); err != nil {
		return nil, err
	}
	if err := parseNullableString(jsonResult, "custom_string_2", &contact.CustomString2); err != nil {
		return nil, err
	}
	if err := parseNullableString(jsonResult, "custom_string_3", &contact.CustomString3); err != nil {
		return nil, err
	}
	if err := parseNullableString(jsonResult, "custom_string_4", &contact.CustomString4); err != nil {
		return nil, err
	}
	if err := parseNullableString(jsonResult, "custom_string_5", &contact.CustomString5); err != nil {
		return nil, err
	}

	// Parse custom number fields
	if err := parseNullableFloat(jsonResult, "custom_number_1", &contact.CustomNumber1); err != nil {
		return nil, err
	}
	if err := parseNullableFloat(jsonResult, "custom_number_2", &contact.CustomNumber2); err != nil {
		return nil, err
	}
	if err := parseNullableFloat(jsonResult, "custom_number_3", &contact.CustomNumber3); err != nil {
		return nil, err
	}
	if err := parseNullableFloat(jsonResult, "custom_number_4", &contact.CustomNumber4); err != nil {
		return nil, err
	}
	if err := parseNullableFloat(jsonResult, "custom_number_5", &contact.CustomNumber5); err != nil {
		return nil, err
	}

	// Parse custom datetime fields
	if err := parseNullableTime(jsonResult, "custom_datetime_1", &contact.CustomDatetime1); err != nil {
		return nil, err
	}
	if err := parseNullableTime(jsonResult, "custom_datetime_2", &contact.CustomDatetime2); err != nil {
		return nil, err
	}
	if err := parseNullableTime(jsonResult, "custom_datetime_3", &contact.CustomDatetime3); err != nil {
		return nil, err
	}
	if err := parseNullableTime(jsonResult, "custom_datetime_4", &contact.CustomDatetime4); err != nil {
		return nil, err
	}
	if err := parseNullableTime(jsonResult, "custom_datetime_5", &contact.CustomDatetime5); err != nil {
		return nil, err
	}

	// Parse custom JSON fields if they exist
	for i := 1; i <= 5; i++ {
		field := fmt.Sprintf("custom_json_%d", i)
		if value := jsonResult.Get(field); value.Exists() {
			// Check if the value is explicitly null
			if value.Type == gjson.Null {
				// Set the field as null
				switch i {
				case 1:
					contact.CustomJSON1 = &NullableJSON{Data: nil, IsNull: true}
				case 2:
					contact.CustomJSON2 = &NullableJSON{Data: nil, IsNull: true}
				case 3:
					contact.CustomJSON3 = &NullableJSON{Data: nil, IsNull: true}
				case 4:
					contact.CustomJSON4 = &NullableJSON{Data: nil, IsNull: true}
				case 5:
					contact.CustomJSON5 = &NullableJSON{Data: nil, IsNull: true}
				}
				continue
			}

			// make sure the value is a valid JSON object or array
			if !value.IsObject() && !value.IsArray() {
				return nil, fmt.Errorf("invalid JSON value for custom_json_%d, got %s", i, value.Type)
			}

			// Set the custom JSON field
			switch i {
			case 1:
				contact.CustomJSON1 = &NullableJSON{Data: value.Value(), IsNull: false}
			case 2:
				contact.CustomJSON2 = &NullableJSON{Data: value.Value(), IsNull: false}
			case 3:
				contact.CustomJSON3 = &NullableJSON{Data: value.Value(), IsNull: false}
			case 4:
				contact.CustomJSON4 = &NullableJSON{Data: value.Value(), IsNull: false}
			case 5:
				contact.CustomJSON5 = &NullableJSON{Data: value.Value(), IsNull: false}
			}
		}
	}

	return contact, nil
}

// trimUnicodeSpace trims all Unicode whitespace characters from a string,
// including non-breaking space (NBSP, U+00A0) which is not trimmed by strings.TrimSpace.
// This is important for CSV imports where NBSP characters can prevent email delivery.
func trimUnicodeSpace(s string) string {
	return strings.TrimFunc(s, unicode.IsSpace)
}

// Helper functions for parsing nullable fields from JSON
func parseNullableString(result gjson.Result, field string, target **NullableString) error {
	if value := result.Get(field); value.Exists() {
		switch value.Type {
		case gjson.Null:
			*target = &NullableString{IsNull: true}
		case gjson.String:
			*target = &NullableString{String: trimUnicodeSpace(value.String()), IsNull: false}
		default:
			return fmt.Errorf("invalid type for %s: expected string, got %s", field, value.Type)
		}
	}
	return nil
}

func parseNullableFloat(result gjson.Result, field string, target **NullableFloat64) error {
	if value := result.Get(field); value.Exists() {
		switch value.Type {
		case gjson.Null:
			*target = &NullableFloat64{IsNull: true}
		case gjson.Number:
			*target = &NullableFloat64{Float64: value.Float(), IsNull: false}
		default:
			return fmt.Errorf("invalid type for %s: expected number, got %s", field, value.Type)
		}
	}
	return nil
}

func parseNullableTime(result gjson.Result, field string, target **NullableTime) error {
	if value := result.Get(field); value.Exists() {
		switch value.Type {
		case gjson.Null:
			*target = &NullableTime{IsNull: true}
		case gjson.String:
			t, err := time.Parse(time.RFC3339, value.String())
			if err != nil {
				return fmt.Errorf("invalid time format for %s: %v", field, err)
			}
			*target = &NullableTime{Time: t, IsNull: false}
		default:
			return fmt.Errorf("invalid type for %s: expected string, got %s", field, value.Type)
		}
	}
	return nil
}

// Merge updates non-nil fields from another contact
func (c *Contact) Merge(other *Contact) {
	if other == nil {
		return
	}

	// Required fields
	if other.Email != "" {
		c.Email = other.Email
	}

	// Optional fields
	if other.ExternalID != nil {
		c.ExternalID = other.ExternalID
	}
	if other.Timezone != nil {
		c.Timezone = other.Timezone
	}
	if other.Language != nil {
		c.Language = other.Language
	}
	if other.FirstName != nil {
		c.FirstName = other.FirstName
	}
	if other.LastName != nil {
		c.LastName = other.LastName
	}
	if other.FullName != nil {
		c.FullName = other.FullName
	}
	if other.Phone != nil {
		c.Phone = other.Phone
	}
	if other.AddressLine1 != nil {
		c.AddressLine1 = other.AddressLine1
	}
	if other.AddressLine2 != nil {
		c.AddressLine2 = other.AddressLine2
	}
	if other.Country != nil {
		c.Country = other.Country
	}
	if other.Postcode != nil {
		c.Postcode = other.Postcode
	}
	if other.State != nil {
		c.State = other.State
	}
	if other.JobTitle != nil {
		c.JobTitle = other.JobTitle
	}

	// Custom string fields
	if other.CustomString1 != nil {
		c.CustomString1 = other.CustomString1
	}
	if other.CustomString2 != nil {
		c.CustomString2 = other.CustomString2
	}
	if other.CustomString3 != nil {
		c.CustomString3 = other.CustomString3
	}
	if other.CustomString4 != nil {
		c.CustomString4 = other.CustomString4
	}
	if other.CustomString5 != nil {
		c.CustomString5 = other.CustomString5
	}

	// Custom number fields
	if other.CustomNumber1 != nil {
		c.CustomNumber1 = other.CustomNumber1
	}
	if other.CustomNumber2 != nil {
		c.CustomNumber2 = other.CustomNumber2
	}
	if other.CustomNumber3 != nil {
		c.CustomNumber3 = other.CustomNumber3
	}
	if other.CustomNumber4 != nil {
		c.CustomNumber4 = other.CustomNumber4
	}
	if other.CustomNumber5 != nil {
		c.CustomNumber5 = other.CustomNumber5
	}

	// Custom datetime fields
	if other.CustomDatetime1 != nil {
		c.CustomDatetime1 = other.CustomDatetime1
	}
	if other.CustomDatetime2 != nil {
		c.CustomDatetime2 = other.CustomDatetime2
	}
	if other.CustomDatetime3 != nil {
		c.CustomDatetime3 = other.CustomDatetime3
	}
	if other.CustomDatetime4 != nil {
		c.CustomDatetime4 = other.CustomDatetime4
	}
	if other.CustomDatetime5 != nil {
		c.CustomDatetime5 = other.CustomDatetime5
	}

	// Custom JSON fields
	if other.CustomJSON1 != nil {
		c.CustomJSON1 = other.CustomJSON1
	}
	if other.CustomJSON2 != nil {
		c.CustomJSON2 = other.CustomJSON2
	}
	if other.CustomJSON3 != nil {
		c.CustomJSON3 = other.CustomJSON3
	}
	if other.CustomJSON4 != nil {
		c.CustomJSON4 = other.CustomJSON4
	}
	if other.CustomJSON5 != nil {
		c.CustomJSON5 = other.CustomJSON5
	}

	// Update timestamps
	if !other.CreatedAt.IsZero() {
		c.CreatedAt = other.CreatedAt
	}
	if !other.UpdatedAt.IsZero() {
		c.UpdatedAt = other.UpdatedAt
	}

	// Update DB timestamps
	if !other.DBCreatedAt.IsZero() {
		c.DBCreatedAt = other.DBCreatedAt
	}
	if !other.DBUpdatedAt.IsZero() {
		c.DBUpdatedAt = other.DBUpdatedAt
	}
}

// MergeContactLists merges a new contact list into the contact's existing lists
func (c *Contact) MergeContactLists(list *ContactList) {
	// If this is the first list, initialize the slice
	if c.ContactLists == nil {
		c.ContactLists = []*ContactList{list}
		return
	}

	// Check if the list already exists
	for i, existingList := range c.ContactLists {
		if existingList.ListID == list.ListID {
			// Update the existing list
			c.ContactLists[i] = list
			return
		}
	}

	// If we get here, this is a new list, so append it
	c.ContactLists = append(c.ContactLists, list)
}

// ToMapOfAny converts the contact to a MapOfAny using JSON marshal/unmarshal
// This is useful for template rendering or API responses
func (c *Contact) ToMapOfAny() (MapOfAny, error) {
	// Marshal the contact to JSON
	jsonBytes, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal contact: %w", err)
	}

	// Unmarshal the JSON bytes into a MapOfAny
	var result MapOfAny
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal contact to map: %w", err)
	}

	return result, nil
}

// ContactWithList represents a contact with information about which list it belongs to
type ContactWithList struct {
	Contact  *Contact `json:"contact"`   // The contact
	ListID   string   `json:"list_id"`   // ID of the list that the contact belongs to
	ListName string   `json:"list_name"` // Name of the list that the contact belongs to
}
