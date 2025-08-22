package domain

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/asaskevich/govalidator"
)

//go:generate mockgen -destination mocks/mock_workspace_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain WorkspaceRepository
//go:generate mockgen -destination mocks/mock_workspace_service.go -package mocks github.com/Notifuse/notifuse/internal/domain WorkspaceServiceInterface

// IntegrationType defines the type of integration
type IntegrationType string

const (
	IntegrationTypeEmail IntegrationType = "email"
)

// Integrations is a slice of Integration with database serialization methods
type Integrations []Integration

// Value implements the driver.Valuer interface for database serialization
func (i Integrations) Value() (driver.Value, error) {
	if len(i) == 0 {
		return nil, nil
	}
	return json.Marshal(i)
}

// Scan implements the sql.Scanner interface for database deserialization
func (i *Integrations) Scan(value interface{}) error {
	if value == nil {
		*i = []Integration{}
		return nil
	}

	v, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(v)
	return json.Unmarshal(cloned, i)
}

// Integration represents a third-party service integration that's embedded in workspace settings
type Integration struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Type          IntegrationType `json:"type"`
	EmailProvider EmailProvider   `json:"email_provider"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// Validate validates the integration
func (i *Integration) Validate(passphrase string) error {
	if i.ID == "" {
		return fmt.Errorf("integration id is required")
	}

	if i.Name == "" {
		return fmt.Errorf("integration name is required")
	}

	if i.Type == "" {
		return fmt.Errorf("integration type is required")
	}

	// Validate provider config
	if err := i.EmailProvider.Validate(passphrase); err != nil {
		return fmt.Errorf("invalid provider configuration: %w", err)
	}

	return nil
}

// BeforeSave prepares an Integration for saving by encrypting secrets
func (i *Integration) BeforeSave(secretkey string) error {
	if err := i.EmailProvider.EncryptSecretKeys(secretkey); err != nil {
		return fmt.Errorf("failed to encrypt integration provider secrets: %w", err)
	}

	return nil
}

// AfterLoad processes an Integration after loading by decrypting secrets
func (i *Integration) AfterLoad(secretkey string) error {
	if err := i.EmailProvider.DecryptSecretKeys(secretkey); err != nil {
		return fmt.Errorf("failed to decrypt integration provider secrets: %w", err)
	}

	return nil
}

// Value implements the driver.Valuer interface for database serialization
func (b Integration) Value() (driver.Value, error) {
	return json.Marshal(b)
}

// Scan implements the sql.Scanner interface for database deserialization
func (b *Integration) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	v, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(v)
	return json.Unmarshal(cloned, b)
}

type TemplateBlock struct {
	ID      string                   `json:"id"`
	Name    string                   `json:"name"`
	Block   notifuse_mjml.EmailBlock `json:"block"`
	Created time.Time                `json:"created"`
	Updated time.Time                `json:"updated"`
}

// MarshalJSON implements custom JSON marshaling for TemplateBlock
func (tb TemplateBlock) MarshalJSON() ([]byte, error) {
	// Create a temporary struct with the same fields but Block as interface{}
	temp := struct {
		ID      string      `json:"id"`
		Name    string      `json:"name"`
		Block   interface{} `json:"block"`
		Created time.Time   `json:"created"`
		Updated time.Time   `json:"updated"`
	}{
		ID:      tb.ID,
		Name:    tb.Name,
		Block:   tb.Block,
		Created: tb.Created,
		Updated: tb.Updated,
	}
	return json.Marshal(temp)
}

// UnmarshalJSON implements custom JSON unmarshaling for TemplateBlock
func (tb *TemplateBlock) UnmarshalJSON(data []byte) error {
	// First unmarshal into a temporary struct
	temp := struct {
		ID      string          `json:"id"`
		Name    string          `json:"name"`
		Block   json.RawMessage `json:"block"`
		Created time.Time       `json:"created"`
		Updated time.Time       `json:"updated"`
	}{}

	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Set the simple fields
	tb.ID = temp.ID
	tb.Name = temp.Name
	tb.Created = temp.Created
	tb.Updated = temp.Updated

	// Unmarshal the Block using the existing EmailBlock unmarshaling logic
	if len(temp.Block) > 0 {
		block, err := notifuse_mjml.UnmarshalEmailBlock(temp.Block)
		if err != nil {
			return fmt.Errorf("failed to unmarshal template block: %w", err)
		}
		tb.Block = block
	}

	return nil
}

type SaveOperation string

const (
	SaveOperationCreate SaveOperation = "create"
	SaveOperationUpdate SaveOperation = "update"
)

// WorkspaceSettings contains configurable workspace settings
type WorkspaceSettings struct {
	WebsiteURL                   string              `json:"website_url,omitempty"`
	LogoURL                      string              `json:"logo_url,omitempty"`
	CoverURL                     string              `json:"cover_url,omitempty"`
	Timezone                     string              `json:"timezone"`
	FileManager                  FileManagerSettings `json:"file_manager,omitempty"`
	TransactionalEmailProviderID string              `json:"transactional_email_provider_id,omitempty"`
	MarketingEmailProviderID     string              `json:"marketing_email_provider_id,omitempty"`
	EncryptedSecretKey           string              `json:"encrypted_secret_key,omitempty"`
	EmailTrackingEnabled         bool                `json:"email_tracking_enabled"`
	TemplateBlocks               []TemplateBlock     `json:"template_blocks,omitempty"`

	// decoded secret key, not stored in the database
	SecretKey string `json:"-"`
}

// Validate validates workspace settings
func (ws *WorkspaceSettings) Validate(passphrase string) error {
	if ws.Timezone == "" {
		return fmt.Errorf("timezone is required")
	}

	if !IsValidTimezone(ws.Timezone) {
		return fmt.Errorf("invalid timezone: %s", ws.Timezone)
	}

	if ws.WebsiteURL != "" && !govalidator.IsURL(ws.WebsiteURL) {
		return fmt.Errorf("invalid website URL: %s", ws.WebsiteURL)
	}

	if ws.LogoURL != "" && !govalidator.IsURL(ws.LogoURL) {
		return fmt.Errorf("invalid logo URL: %s", ws.LogoURL)
	}

	if ws.CoverURL != "" && !govalidator.IsURL(ws.CoverURL) {
		return fmt.Errorf("invalid cover URL: %s", ws.CoverURL)
	}

	// FileManager is completely optional, but if any fields are set, validate them
	if err := ws.FileManager.Validate(passphrase); err != nil {
		return fmt.Errorf("invalid file manager settings: %w", err)
	}

	// Validate template blocks if any are present
	for i, templateBlock := range ws.TemplateBlocks {
		if templateBlock.Name == "" {
			return fmt.Errorf("template block at index %d: name is required", i)
		}
		if len(templateBlock.Name) > 255 {
			return fmt.Errorf("template block at index %d: name length must be between 1 and 255", i)
		}
		if templateBlock.Block == nil || templateBlock.Block.GetType() == "" {
			return fmt.Errorf("template block at index %d: block kind is required", i)
		}
	}

	return nil
}

// Value implements the driver.Valuer interface for database serialization
func (b WorkspaceSettings) Value() (driver.Value, error) {
	return json.Marshal(b)
}

// Scan implements the sql.Scanner interface for database deserialization
func (b *WorkspaceSettings) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	v, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(v)
	return json.Unmarshal(cloned, b)
}

type Workspace struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Settings     WorkspaceSettings `json:"settings"`
	Integrations Integrations      `json:"integrations"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// Validate performs validation on the workspace fields
func (w *Workspace) Validate(passphrase string) error {
	// Validate ID
	if w.ID == "" {
		return fmt.Errorf("invalid workspace: id is required")
	}
	if !govalidator.IsAlphanumeric(w.ID) {
		return fmt.Errorf("invalid workspace: id must be alphanumeric")
	}
	if len(w.ID) > 32 {
		return fmt.Errorf("invalid workspace: id length must be between 1 and 32")
	}

	// Validate Name
	if w.Name == "" {
		return fmt.Errorf("invalid workspace: name is required")
	}
	if len(w.Name) > 255 {
		return fmt.Errorf("invalid workspace: name length must be between 1 and 255")
	}

	// Validate Settings
	if err := w.Settings.Validate(passphrase); err != nil {
		return fmt.Errorf("invalid workspace settings: %w", err)
	}

	// initialize integrations if nil
	if w.Integrations == nil {
		w.Integrations = []Integration{}
	}

	// Validate integrations if any are defined
	for _, integration := range w.Integrations {
		if err := integration.Validate(passphrase); err != nil {
			return fmt.Errorf("invalid integration (%s): %w", integration.ID, err)
		}
	}

	return nil
}

func (w *Workspace) BeforeSave(globalSecretKey string) error {
	// Only process FileManager if there's a SecretKey to encrypt
	if w.Settings.FileManager.SecretKey != "" {
		if err := w.Settings.FileManager.EncryptSecretKey(globalSecretKey); err != nil {
			return fmt.Errorf("failed to encrypt secret key: %w", err)
		}
		// clear the secret key from the workspace settings
		w.Settings.FileManager.SecretKey = ""
	}

	if w.Settings.SecretKey == "" {
		return fmt.Errorf("workspace secret key is missing")
	}

	// Encrypt the secret key
	encryptedSecretKey, err := crypto.EncryptString(w.Settings.SecretKey, globalSecretKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret key: %w", err)
	}
	w.Settings.EncryptedSecretKey = encryptedSecretKey

	// Process all integrations
	for i := range w.Integrations {
		if err := w.Integrations[i].BeforeSave(globalSecretKey); err != nil {
			return fmt.Errorf("failed to process integration %s: %w", w.Integrations[i].ID, err)
		}
	}

	return nil
}

func (w *Workspace) AfterLoad(globalSecretKey string) error {
	// Only decrypt if there's an EncryptedSecretKey present
	if w.Settings.FileManager.EncryptedSecretKey != "" {
		if err := w.Settings.FileManager.DecryptSecretKey(globalSecretKey); err != nil {
			return fmt.Errorf("failed to decrypt secret key: %w", err)
		}
	}

	// Decrypt the secret key
	decryptedSecretKey, err := crypto.DecryptFromHexString(w.Settings.EncryptedSecretKey, globalSecretKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt secret key: %w", err)
	}
	w.Settings.SecretKey = decryptedSecretKey

	// Process all integrations
	for i := range w.Integrations {
		if err := w.Integrations[i].AfterLoad(globalSecretKey); err != nil {
			return fmt.Errorf("failed to process integration %s: %w", w.Integrations[i].ID, err)
		}
	}

	return nil
}

// GetIntegrationByID finds an integration by ID in the workspace
func (w *Workspace) GetIntegrationByID(id string) *Integration {
	for i, integration := range w.Integrations {
		if integration.ID == id {
			return &w.Integrations[i]
		}
	}
	return nil
}

// GetIntegrationsByType returns all integrations of a specific type
func (w *Workspace) GetIntegrationsByType(integrationType IntegrationType) []*Integration {
	var results []*Integration
	for i, integration := range w.Integrations {
		if integration.Type == integrationType {
			results = append(results, &w.Integrations[i])
		}
	}
	return results
}

// AddIntegration adds a new integration to the workspace
func (w *Workspace) AddIntegration(integration Integration) {
	// Check if an integration with this ID already exists
	for i, existing := range w.Integrations {
		if existing.ID == integration.ID {
			// Replace the existing integration
			w.Integrations[i] = integration
			return
		}
	}
	// Add new integration
	w.Integrations = append(w.Integrations, integration)
}

// RemoveIntegration removes an integration by ID
func (w *Workspace) RemoveIntegration(id string) bool {
	for i, integration := range w.Integrations {
		if integration.ID == id {
			// Remove by slicing it out
			w.Integrations = append(w.Integrations[:i], w.Integrations[i+1:]...)
			return true
		}
	}
	return false
}

// GetEmailProvider returns the email provider based on provider type
func (w *Workspace) GetEmailProvider(isMarketing bool) (*EmailProvider, error) {
	var integrationID string

	// Get integration ID from settings based on provider type
	if isMarketing {
		integrationID = w.Settings.MarketingEmailProviderID
	} else {
		integrationID = w.Settings.TransactionalEmailProviderID
	}

	// If no integration ID is configured, return nil
	if integrationID == "" {
		return nil, nil
	}

	// Find the integration by ID
	integration := w.GetIntegrationByID(integrationID)
	if integration == nil {
		return nil, fmt.Errorf("integration with ID %s not found", integrationID)
	}

	return &integration.EmailProvider, nil
}

// GetEmailProviderWithIntegrationID returns both the email provider and integration ID based on provider type
func (w *Workspace) GetEmailProviderWithIntegrationID(isMarketing bool) (*EmailProvider, string, error) {
	var integrationID string

	// Get integration ID from settings based on provider type
	if isMarketing {
		integrationID = w.Settings.MarketingEmailProviderID
	} else {
		integrationID = w.Settings.TransactionalEmailProviderID
	}

	// If no integration ID is configured, return nil
	if integrationID == "" {
		return nil, "", nil
	}

	// Find the integration by ID
	integration := w.GetIntegrationByID(integrationID)
	if integration == nil {
		return nil, "", fmt.Errorf("integration with ID %s not found", integrationID)
	}

	return &integration.EmailProvider, integrationID, nil
}

func (w *Workspace) MarshalJSON() ([]byte, error) {
	type Alias Workspace
	if w.Integrations == nil {
		w.Integrations = []Integration{}
	}
	return json.Marshal((*Alias)(w))
}

type FileManagerSettings struct {
	Endpoint           string  `json:"endpoint"`
	Bucket             string  `json:"bucket"`
	AccessKey          string  `json:"access_key"`
	EncryptedSecretKey string  `json:"encrypted_secret_key,omitempty"`
	Region             *string `json:"region,omitempty"`
	CDNEndpoint        *string `json:"cdn_endpoint,omitempty"`

	// decoded secret key, not stored in the database
	SecretKey string `json:"secret_key,omitempty"`
}

func (f *FileManagerSettings) DecryptSecretKey(passphrase string) error {
	secretKey, err := crypto.DecryptFromHexString(f.EncryptedSecretKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt secret key: %w", err)
	}
	f.SecretKey = secretKey
	return nil
}

func (f *FileManagerSettings) EncryptSecretKey(passphrase string) error {
	encryptedSecretKey, err := crypto.EncryptString(f.SecretKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret key: %w", err)
	}
	f.EncryptedSecretKey = encryptedSecretKey
	return nil
}

func (f *FileManagerSettings) Validate(passphrase string) error {
	// Check if any field is set to determine if we should validate
	isConfigured := f.Endpoint != "" || f.Bucket != "" || f.AccessKey != "" ||
		f.EncryptedSecretKey != "" || f.SecretKey != "" ||
		(f.Region != nil) || (f.CDNEndpoint != nil)

	// If no fields are set, consider it valid (optional config)
	if !isConfigured {
		return nil
	}

	// If any field is set, validate required fields are present
	if f.Endpoint == "" {
		return fmt.Errorf("endpoint is required when file manager is configured")
	}

	if !govalidator.IsURL(f.Endpoint) {
		return fmt.Errorf("invalid endpoint: %s", f.Endpoint)
	}

	if f.Bucket == "" {
		return fmt.Errorf("bucket is required when file manager is configured")
	}

	if f.AccessKey == "" {
		return fmt.Errorf("access key is required when file manager is configured")
	}

	// Region is optional, so we don't check if it's empty
	if f.CDNEndpoint != nil && !govalidator.IsURL(*f.CDNEndpoint) {
		return fmt.Errorf("invalid cdn endpoint: %s", *f.CDNEndpoint)
	}

	// only encrypt secret key if it's not empty
	if f.SecretKey != "" {
		if err := f.EncryptSecretKey(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt secret key: %w", err)
		}
	}

	return nil
}

// For database scanning
type dbWorkspace struct {
	ID           string
	Name         string
	Settings     []byte
	Integrations []byte
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ScanWorkspace scans a workspace from the database
func ScanWorkspace(scanner interface {
	Scan(dest ...interface{}) error
}) (*Workspace, error) {
	var dbw dbWorkspace
	if err := scanner.Scan(
		&dbw.ID,
		&dbw.Name,
		&dbw.Settings,
		&dbw.Integrations,
		&dbw.CreatedAt,
		&dbw.UpdatedAt,
	); err != nil {
		return nil, err
	}

	w := &Workspace{
		ID:        dbw.ID,
		Name:      dbw.Name,
		CreatedAt: dbw.CreatedAt,
		UpdatedAt: dbw.UpdatedAt,
	}

	if err := json.Unmarshal(dbw.Settings, &w.Settings); err != nil {
		return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	// Unmarshal integrations if present
	if dbw.Integrations != nil && len(dbw.Integrations) > 0 {
		if err := json.Unmarshal(dbw.Integrations, &w.Integrations); err != nil {
			return nil, fmt.Errorf("failed to unmarshal integrations: %w", err)
		}
	}

	return w, nil
}

// UserWorkspace represents the relationship between a user and a workspace
type UserWorkspace struct {
	UserID      string    `json:"user_id" db:"user_id"`
	WorkspaceID string    `json:"workspace_id" db:"workspace_id"`
	Role        string    `json:"role" db:"role"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// UserWorkspaceWithEmail extends UserWorkspace to include user email
type UserWorkspaceWithEmail struct {
	UserWorkspace
	Email string   `json:"email" db:"email"`
	Type  UserType `json:"type" db:"type"`
}

// Validate performs validation on the user workspace fields
func (uw *UserWorkspace) Validate() error {
	if uw.UserID == "" {
		return fmt.Errorf("invalid user workspace: user_id is required")
	}
	if uw.WorkspaceID == "" {
		return fmt.Errorf("invalid user workspace: workspace_id is required")
	}
	if uw.Role == "" {
		return fmt.Errorf("invalid user workspace: role is required")
	}
	if uw.Role != "owner" && uw.Role != "member" {
		return fmt.Errorf("invalid user workspace: role must be either 'owner' or 'member'")
	}

	return nil
}

// WorkspaceInvitation represents an invitation to a workspace
type WorkspaceInvitation struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	InviterID   string    `json:"inviter_id"`
	Email       string    `json:"email"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type WorkspaceRepository interface {
	Create(ctx context.Context, workspace *Workspace) error
	GetByID(ctx context.Context, id string) (*Workspace, error)
	List(ctx context.Context) ([]*Workspace, error)
	Update(ctx context.Context, workspace *Workspace) error
	Delete(ctx context.Context, id string) error

	// User workspace management
	AddUserToWorkspace(ctx context.Context, userWorkspace *UserWorkspace) error
	RemoveUserFromWorkspace(ctx context.Context, userID string, workspaceID string) error
	GetUserWorkspaces(ctx context.Context, userID string) ([]*UserWorkspace, error)
	GetWorkspaceUsersWithEmail(ctx context.Context, workspaceID string) ([]*UserWorkspaceWithEmail, error)
	GetUserWorkspace(ctx context.Context, userID string, workspaceID string) (*UserWorkspace, error)

	// Workspace invitation management
	CreateInvitation(ctx context.Context, invitation *WorkspaceInvitation) error
	GetInvitationByID(ctx context.Context, id string) (*WorkspaceInvitation, error)
	GetInvitationByEmail(ctx context.Context, workspaceID, email string) (*WorkspaceInvitation, error)
	IsUserWorkspaceMember(ctx context.Context, userID, workspaceID string) (bool, error)

	// Database management
	GetConnection(ctx context.Context, workspaceID string) (*sql.DB, error)
	GetSystemConnection(ctx context.Context) (*sql.DB, error)
	CreateDatabase(ctx context.Context, workspaceID string) error
	DeleteDatabase(ctx context.Context, workspaceID string) error

	// Transaction management
	WithWorkspaceTransaction(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error
}

// ErrUnauthorized is returned when a user is not authorized to perform an action
type ErrUnauthorized struct {
	Message string
}

func (e *ErrUnauthorized) Error() string {
	return e.Message
}

// ErrWorkspaceNotFound is returned when a workspace is not found
type ErrWorkspaceNotFound struct {
	WorkspaceID string
}

func (e *ErrWorkspaceNotFound) Error() string {
	return fmt.Sprintf("workspace not found: %s", e.WorkspaceID)
}

// WorkspaceServiceInterface defines the interface for workspace operations
type WorkspaceServiceInterface interface {
	CreateWorkspace(ctx context.Context, id, name, websiteURL, logoURL, coverURL, timezone string, fileManager FileManagerSettings) (*Workspace, error)
	GetWorkspace(ctx context.Context, id string) (*Workspace, error)
	ListWorkspaces(ctx context.Context) ([]*Workspace, error)
	UpdateWorkspace(ctx context.Context, id, name string, settings WorkspaceSettings) (*Workspace, error)
	DeleteWorkspace(ctx context.Context, id string) error
	GetWorkspaceMembersWithEmail(ctx context.Context, id string) ([]*UserWorkspaceWithEmail, error)
	InviteMember(ctx context.Context, workspaceID, email string) (*WorkspaceInvitation, string, error)
	AddUserToWorkspace(ctx context.Context, workspaceID string, userID string, role string) error
	RemoveUserFromWorkspace(ctx context.Context, workspaceID string, userID string) error
	TransferOwnership(ctx context.Context, workspaceID string, newOwnerID string, currentOwnerID string) error
	CreateAPIKey(ctx context.Context, workspaceID string, emailPrefix string) (string, string, error)
	RemoveMember(ctx context.Context, workspaceID string, userIDToRemove string) error

	// Integration management
	CreateIntegration(ctx context.Context, workspaceID, name string, integrationType IntegrationType, provider EmailProvider) (string, error)
	UpdateIntegration(ctx context.Context, workspaceID, integrationID, name string, provider EmailProvider) error
	DeleteIntegration(ctx context.Context, workspaceID, integrationID string) error
}

// Request/Response types

// CreateAPIKeyRequest defines the request structure for creating an API key
type CreateAPIKeyRequest struct {
	WorkspaceID string `json:"workspace_id"`
	EmailPrefix string `json:"email_prefix"`
}

// Validate validates the create API key request
func (r *CreateAPIKeyRequest) Validate() error {
	if r.WorkspaceID == "" {
		return errors.New("workspace ID is required")
	}
	if r.EmailPrefix == "" {
		return errors.New("email prefix is required")
	}
	return nil
}

// CreateIntegrationRequest defines the request structure for creating an integration
type CreateIntegrationRequest struct {
	WorkspaceID string          `json:"workspace_id"`
	Name        string          `json:"name"`
	Type        IntegrationType `json:"type"`
	Provider    EmailProvider   `json:"provider"`
}

func (r *CreateIntegrationRequest) Validate(passphrase string) error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace ID is required")
	}

	if r.Name == "" {
		return fmt.Errorf("integration name is required")
	}

	if r.Type == "" {
		return fmt.Errorf("integration type is required")
	}

	// Validate provider configuration
	if err := r.Provider.Validate(passphrase); err != nil {
		return fmt.Errorf("invalid provider configuration: %w", err)
	}

	return nil
}

// UpdateIntegrationRequest defines the request structure for updating an integration
type UpdateIntegrationRequest struct {
	WorkspaceID   string        `json:"workspace_id"`
	IntegrationID string        `json:"integration_id"`
	Name          string        `json:"name"`
	Provider      EmailProvider `json:"provider"`
}

func (r *UpdateIntegrationRequest) Validate(passphrase string) error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace ID is required")
	}

	if r.IntegrationID == "" {
		return fmt.Errorf("integration ID is required")
	}

	if r.Name == "" {
		return fmt.Errorf("integration name is required")
	}

	// Validate provider configuration
	if err := r.Provider.Validate(passphrase); err != nil {
		return fmt.Errorf("invalid provider configuration: %w", err)
	}

	return nil
}

// DeleteIntegrationRequest defines the request structure for deleting an integration
type DeleteIntegrationRequest struct {
	WorkspaceID   string `json:"workspace_id"`
	IntegrationID string `json:"integration_id"`
}

func (r *DeleteIntegrationRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace ID is required")
	}

	if r.IntegrationID == "" {
		return fmt.Errorf("integration ID is required")
	}

	return nil
}

type CreateWorkspaceRequest struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Settings WorkspaceSettings `json:"settings"`
}

func (r *CreateWorkspaceRequest) Validate(passphrase string) error {
	// Validate ID
	if r.ID == "" {
		return fmt.Errorf("invalid create workspace request: id is required")
	}
	if !govalidator.IsAlphanumeric(r.ID) {
		return fmt.Errorf("invalid create workspace request: id must be alphanumeric")
	}
	if len(r.ID) > 32 {
		return fmt.Errorf("invalid create workspace request: id length must be between 1 and 32")
	}

	// Validate Name
	if r.Name == "" {
		return fmt.Errorf("invalid create workspace request: name is required")
	}
	if len(r.Name) > 32 {
		return fmt.Errorf("invalid create workspace request: name length must be between 1 and 32")
	}

	// Validate Settings
	if err := r.Settings.Validate(passphrase); err != nil {
		return fmt.Errorf("invalid create workspace request: %w", err)
	}

	return nil
}

type GetWorkspaceRequest struct {
	ID string `json:"id"`
}

type UpdateWorkspaceRequest struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Settings WorkspaceSettings `json:"settings"`
}

func (r *UpdateWorkspaceRequest) Validate(passphrase string) error {
	// Validate ID
	if r.ID == "" {
		return fmt.Errorf("invalid update workspace request: id is required")
	}
	if !govalidator.IsAlphanumeric(r.ID) {
		return fmt.Errorf("invalid update workspace request: id must be alphanumeric")
	}
	if len(r.ID) > 32 {
		return fmt.Errorf("invalid update workspace request: id length must be between 1 and 32")
	}

	// Validate Name
	if r.Name == "" {
		return fmt.Errorf("invalid update workspace request: name is required")
	}
	if len(r.Name) > 32 {
		return fmt.Errorf("invalid update workspace request: name length must be between 1 and 32")
	}

	// Validate Settings
	if err := r.Settings.Validate(passphrase); err != nil {
		return fmt.Errorf("invalid update workspace request: %w", err)
	}

	return nil
}

type DeleteWorkspaceRequest struct {
	ID string `json:"id"`
}

func (r *DeleteWorkspaceRequest) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("invalid delete workspace request: id is required")
	}
	if !govalidator.IsAlphanumeric(r.ID) {
		return fmt.Errorf("invalid delete workspace request: id must be alphanumeric")
	}
	if len(r.ID) > 32 {
		return fmt.Errorf("invalid delete workspace request: id length must be between 1 and 32")
	}

	return nil
}

type InviteMemberRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Email       string `json:"email"`
}

func (r *InviteMemberRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("invalid invite member request: workspace_id is required")
	}
	if !govalidator.IsAlphanumeric(r.WorkspaceID) {
		return fmt.Errorf("invalid invite member request: workspace_id must be alphanumeric")
	}
	if len(r.WorkspaceID) > 32 {
		return fmt.Errorf("invalid invite member request: workspace_id length must be between 1 and 32")
	}

	if r.Email == "" {
		return fmt.Errorf("invalid invite member request: email is required")
	}
	if !govalidator.IsEmail(r.Email) {
		return fmt.Errorf("invalid invite member request: email is not valid")
	}

	return nil
}

// TestEmailProviderRequest is the request for testing an email provider
// It includes the provider config, a recipient email, and the workspace ID
type TestEmailProviderRequest struct {
	Provider    EmailProvider `json:"provider"`
	To          string        `json:"to"`
	WorkspaceID string        `json:"workspace_id"`
}

// TestEmailProviderResponse is the response for testing an email provider
// It can be extended to include more details if needed
type TestEmailProviderResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}
