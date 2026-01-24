package domain

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/asaskevich/govalidator"
)

// PermissionResource defines the different resources that can have permissions
type PermissionResource string

const (
	PermissionResourceContacts       PermissionResource = "contacts"
	PermissionResourceLists          PermissionResource = "lists"
	PermissionResourceTemplates      PermissionResource = "templates"
	PermissionResourceBroadcasts     PermissionResource = "broadcasts"
	PermissionResourceTransactional  PermissionResource = "transactional"
	PermissionResourceWorkspace      PermissionResource = "workspace"
	PermissionResourceMessageHistory PermissionResource = "message_history"
	PermissionResourceBlog           PermissionResource = "blog"
	PermissionResourceAutomations    PermissionResource = "automations"
	PermissionResourceLLM            PermissionResource = "llm"
)

// PermissionType defines the types of permissions (read/write)
type PermissionType string

const (
	PermissionTypeRead  PermissionType = "read"
	PermissionTypeWrite PermissionType = "write"
)

var FullPermissions = UserPermissions{
	PermissionResourceContacts:       ResourcePermissions{Read: true, Write: true},
	PermissionResourceLists:          ResourcePermissions{Read: true, Write: true},
	PermissionResourceTemplates:      ResourcePermissions{Read: true, Write: true},
	PermissionResourceBroadcasts:     ResourcePermissions{Read: true, Write: true},
	PermissionResourceTransactional:  ResourcePermissions{Read: true, Write: true},
	PermissionResourceWorkspace:      ResourcePermissions{Read: true, Write: true},
	PermissionResourceMessageHistory: ResourcePermissions{Read: true, Write: true},
	PermissionResourceBlog:           ResourcePermissions{Read: true, Write: true},
	PermissionResourceAutomations:    ResourcePermissions{Read: true, Write: true},
	PermissionResourceLLM:            ResourcePermissions{Read: true, Write: true},
}

// ResourcePermissions defines read/write permissions for a specific resource
type ResourcePermissions struct {
	Read  bool `json:"read"`
	Write bool `json:"write"`
}

// UserPermissions maps resources to their permission settings
type UserPermissions map[PermissionResource]ResourcePermissions

// Value implements the driver.Valuer interface for database serialization
func (up UserPermissions) Value() (driver.Value, error) {
	if len(up) == 0 {
		return nil, nil
	}
	return json.Marshal(up)
}

// Scan implements the sql.Scanner interface for database deserialization
func (up *UserPermissions) Scan(value interface{}) error {
	if value == nil {
		*up = make(UserPermissions)
		return nil
	}

	v, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(v)
	return json.Unmarshal(cloned, up)
}

//go:generate mockgen -destination mocks/mock_workspace_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain WorkspaceRepository
//go:generate mockgen -destination mocks/mock_workspace_service.go -package mocks github.com/Notifuse/notifuse/internal/domain WorkspaceServiceInterface

// IntegrationType defines the type of integration
type IntegrationType string

const (
	IntegrationTypeEmail     IntegrationType = "email"
	IntegrationTypeSupabase  IntegrationType = "supabase"
	IntegrationTypeLLM       IntegrationType = "llm"
	IntegrationTypeFirecrawl IntegrationType = "firecrawl"
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
	ID                string                       `json:"id"`
	Name              string                       `json:"name"`
	Type              IntegrationType              `json:"type"`
	EmailProvider     EmailProvider                `json:"email_provider,omitempty"`
	SupabaseSettings  *SupabaseIntegrationSettings `json:"supabase_settings,omitempty"`
	LLMProvider       *LLMProvider                 `json:"llm_provider,omitempty"`
	FirecrawlSettings *FirecrawlSettings           `json:"firecrawl_settings,omitempty"`
	CreatedAt         time.Time                    `json:"created_at"`
	UpdatedAt         time.Time                    `json:"updated_at"`
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

	// Validate based on integration type
	switch i.Type {
	case IntegrationTypeEmail:
		// Validate email provider config
		if err := i.EmailProvider.Validate(passphrase); err != nil {
			return fmt.Errorf("invalid provider configuration: %w", err)
		}
	case IntegrationTypeSupabase:
		// Validate Supabase settings
		if i.SupabaseSettings == nil {
			return fmt.Errorf("supabase settings are required for supabase integration")
		}
		if err := i.SupabaseSettings.Validate(passphrase); err != nil {
			return fmt.Errorf("invalid supabase settings: %w", err)
		}
	case IntegrationTypeLLM:
		// Validate LLM provider settings
		if i.LLMProvider == nil {
			return fmt.Errorf("llm provider settings are required for llm integration")
		}
		if err := i.LLMProvider.Validate(passphrase); err != nil {
			return fmt.Errorf("invalid llm provider settings: %w", err)
		}
	case IntegrationTypeFirecrawl:
		// Validate Firecrawl settings
		if i.FirecrawlSettings == nil {
			return fmt.Errorf("firecrawl settings are required for firecrawl integration")
		}
		if err := i.FirecrawlSettings.Validate(passphrase); err != nil {
			return fmt.Errorf("invalid firecrawl settings: %w", err)
		}
	default:
		return fmt.Errorf("unsupported integration type: %s", i.Type)
	}

	return nil
}

// BeforeSave prepares an Integration for saving by encrypting secrets
func (i *Integration) BeforeSave(secretkey string) error {
	// Encrypt based on integration type
	switch i.Type {
	case IntegrationTypeEmail:
		if err := i.EmailProvider.EncryptSecretKeys(secretkey); err != nil {
			return fmt.Errorf("failed to encrypt integration provider secrets: %w", err)
		}
	case IntegrationTypeSupabase:
		if i.SupabaseSettings != nil {
			if err := i.SupabaseSettings.EncryptSignatureKeys(secretkey); err != nil {
				return fmt.Errorf("failed to encrypt supabase signature keys: %w", err)
			}
		}
	case IntegrationTypeLLM:
		if i.LLMProvider != nil {
			if err := i.LLMProvider.EncryptSecretKeys(secretkey); err != nil {
				return fmt.Errorf("failed to encrypt llm provider secrets: %w", err)
			}
		}
	case IntegrationTypeFirecrawl:
		if i.FirecrawlSettings != nil {
			if err := i.FirecrawlSettings.EncryptSecretKeys(secretkey); err != nil {
				return fmt.Errorf("failed to encrypt firecrawl secret keys: %w", err)
			}
		}
	}

	return nil
}

// AfterLoad processes an Integration after loading by decrypting secrets
func (i *Integration) AfterLoad(secretkey string) error {
	// Decrypt based on integration type
	switch i.Type {
	case IntegrationTypeEmail:
		if err := i.EmailProvider.DecryptSecretKeys(secretkey); err != nil {
			return fmt.Errorf("failed to decrypt integration provider secrets: %w", err)
		}
	case IntegrationTypeSupabase:
		if i.SupabaseSettings != nil {
			if err := i.SupabaseSettings.DecryptSignatureKeys(secretkey); err != nil {
				return fmt.Errorf("failed to decrypt supabase signature keys: %w", err)
			}
		}
	case IntegrationTypeLLM:
		if i.LLMProvider != nil {
			if err := i.LLMProvider.DecryptSecretKeys(secretkey); err != nil {
				return fmt.Errorf("failed to decrypt llm provider secrets: %w", err)
			}
		}
	case IntegrationTypeFirecrawl:
		if i.FirecrawlSettings != nil {
			if err := i.FirecrawlSettings.DecryptSecretKeys(secretkey); err != nil {
				return fmt.Errorf("failed to decrypt firecrawl secret keys: %w", err)
			}
		}
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

// BlogSettings contains blog title and SEO configuration
type BlogSettings struct {
	Title            string       `json:"title,omitempty"`
	LogoURL          *string      `json:"logo_url,omitempty"`
	IconURL          *string      `json:"icon_url,omitempty"`
	SEO              *SEOSettings `json:"seo,omitempty"`
	HomePageSize     int          `json:"home_page_size,omitempty"`     // Posts per page on home (default: 20)
	CategoryPageSize int          `json:"category_page_size,omitempty"` // Posts per page on category (default: 20)
}

// GetHomePageSize returns the home page size with validation and default
func (bs *BlogSettings) GetHomePageSize() int {
	if bs == nil || bs.HomePageSize < 1 || bs.HomePageSize > 100 {
		return 20 // default
	}
	return bs.HomePageSize
}

// GetCategoryPageSize returns the category page size with validation and default
func (bs *BlogSettings) GetCategoryPageSize() int {
	if bs == nil || bs.CategoryPageSize < 1 || bs.CategoryPageSize > 100 {
		return 20 // default
	}
	return bs.CategoryPageSize
}

// Value implements the driver.Valuer interface for database serialization
func (b BlogSettings) Value() (driver.Value, error) {
	return json.Marshal(b)
}

// Scan implements the sql.Scanner interface for database deserialization
func (b *BlogSettings) Scan(value interface{}) error {
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
	CustomEndpointURL            *string             `json:"custom_endpoint_url,omitempty"`
	CustomFieldLabels            map[string]string   `json:"custom_field_labels,omitempty"`
	BlogEnabled                  bool                `json:"blog_enabled"`            // Enable blog feature at workspace level
	BlogSettings                 *BlogSettings       `json:"blog_settings,omitempty"` // Blog styling and SEO settings

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

	// Validate custom endpoint URL if provided
	if ws.CustomEndpointURL != nil && *ws.CustomEndpointURL != "" {
		customURL := *ws.CustomEndpointURL
		if !govalidator.IsURL(customURL) {
			return fmt.Errorf("invalid custom endpoint URL: %s", customURL)
		}
		// Ensure it uses http or https scheme
		if !strings.HasPrefix(customURL, "http://") && !strings.HasPrefix(customURL, "https://") {
			return fmt.Errorf("custom endpoint URL must use http or https scheme: %s", customURL)
		}
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

	// Validate custom field labels if any are present
	if err := ws.ValidateCustomFieldLabels(); err != nil {
		return fmt.Errorf("invalid custom field labels: %w", err)
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

// ValidateCustomFieldLabels validates custom field label mappings
func (ws *WorkspaceSettings) ValidateCustomFieldLabels() error {
	if len(ws.CustomFieldLabels) == 0 {
		return nil
	}

	// Define valid custom field names
	validFields := make(map[string]bool)
	for i := 1; i <= 5; i++ {
		validFields[fmt.Sprintf("custom_string_%d", i)] = true
		validFields[fmt.Sprintf("custom_number_%d", i)] = true
		validFields[fmt.Sprintf("custom_datetime_%d", i)] = true
		validFields[fmt.Sprintf("custom_json_%d", i)] = true
	}

	// Validate each custom field label
	for fieldKey, label := range ws.CustomFieldLabels {
		// Check if the field key is valid
		if !validFields[fieldKey] {
			return fmt.Errorf("invalid custom field key: %s", fieldKey)
		}

		// Check if the label is empty
		if label == "" {
			return fmt.Errorf("custom field label for '%s' cannot be empty", fieldKey)
		}

		// Check if the label is too long
		if len(label) > 100 {
			return fmt.Errorf("custom field label for '%s' exceeds maximum length of 100 characters", fieldKey)
		}
	}

	return nil
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
	Provider           string  `json:"provider,omitempty"`
	Endpoint           string  `json:"endpoint"`
	Bucket             string  `json:"bucket"`
	AccessKey          string  `json:"access_key"`
	EncryptedSecretKey string  `json:"encrypted_secret_key,omitempty"`
	Region             *string `json:"region,omitempty"`
	CDNEndpoint        *string `json:"cdn_endpoint,omitempty"`
	ForcePathStyle     bool    `json:"force_path_style"`

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
	if len(dbw.Integrations) > 0 {
		if err := json.Unmarshal(dbw.Integrations, &w.Integrations); err != nil {
			return nil, fmt.Errorf("failed to unmarshal integrations: %w", err)
		}
	}

	return w, nil
}

// UserWorkspace represents the relationship between a user and a workspace
type UserWorkspace struct {
	UserID      string          `json:"user_id" db:"user_id"`
	WorkspaceID string          `json:"workspace_id" db:"workspace_id"`
	Role        string          `json:"role" db:"role"`
	Permissions UserPermissions `json:"permissions,omitempty" db:"permissions"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

// UserWorkspaceWithEmail extends UserWorkspace to include user email
type UserWorkspaceWithEmail struct {
	UserWorkspace
	Email               string     `json:"email" db:"email"`
	Type                UserType   `json:"type" db:"type"`
	InvitationExpiresAt *time.Time `json:"invitation_expires_at" db:"invitation_expires_at"`
	InvitationID        string     `json:"invitation_id,omitempty" db:"invitation_id"`
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

// HasPermission checks if the user has a specific permission for a resource
func (uw *UserWorkspace) HasPermission(resource PermissionResource, permissionType PermissionType) bool {
	if uw.Role == "owner" {
		return true // Owners have all permissions
	}

	if uw.Permissions == nil {
		return false
	}

	resourcePerms, exists := uw.Permissions[resource]
	if !exists {
		return false
	}

	switch permissionType {
	case PermissionTypeRead:
		return resourcePerms.Read
	case PermissionTypeWrite:
		return resourcePerms.Write
	default:
		return false
	}
}

// SetPermissions replaces all permissions for the user
func (uw *UserWorkspace) SetPermissions(permissions UserPermissions) {
	uw.Permissions = permissions
}

// WorkspaceInvitation represents an invitation to a workspace
type WorkspaceInvitation struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	InviterID   string          `json:"inviter_id"`
	Email       string          `json:"email"`
	Permissions UserPermissions `json:"permissions,omitempty"`
	ExpiresAt   time.Time       `json:"expires_at"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type WorkspaceRepository interface {
	Create(ctx context.Context, workspace *Workspace) error
	GetByID(ctx context.Context, id string) (*Workspace, error)
	GetWorkspaceByCustomDomain(ctx context.Context, hostname string) (*Workspace, error)
	List(ctx context.Context) ([]*Workspace, error)
	Update(ctx context.Context, workspace *Workspace) error
	Delete(ctx context.Context, id string) error

	// User workspace management
	AddUserToWorkspace(ctx context.Context, userWorkspace *UserWorkspace) error
	RemoveUserFromWorkspace(ctx context.Context, userID string, workspaceID string) error
	GetUserWorkspaces(ctx context.Context, userID string) ([]*UserWorkspace, error)
	GetWorkspaceUsersWithEmail(ctx context.Context, workspaceID string) ([]*UserWorkspaceWithEmail, error)
	GetUserWorkspace(ctx context.Context, userID string, workspaceID string) (*UserWorkspace, error)

	// User permission management
	UpdateUserWorkspacePermissions(ctx context.Context, userWorkspace *UserWorkspace) error

	// Workspace invitation management
	CreateInvitation(ctx context.Context, invitation *WorkspaceInvitation) error
	GetInvitationByID(ctx context.Context, id string) (*WorkspaceInvitation, error)
	GetInvitationByEmail(ctx context.Context, workspaceID, email string) (*WorkspaceInvitation, error)
	GetWorkspaceInvitations(ctx context.Context, workspaceID string) ([]*WorkspaceInvitation, error)
	DeleteInvitation(ctx context.Context, id string) error
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
	InviteMember(ctx context.Context, workspaceID, email string, permissions UserPermissions) (*WorkspaceInvitation, string, error)
	AddUserToWorkspace(ctx context.Context, workspaceID string, userID string, role string, permissions UserPermissions) error
	RemoveUserFromWorkspace(ctx context.Context, workspaceID string, userID string) error
	TransferOwnership(ctx context.Context, workspaceID string, newOwnerID string, currentOwnerID string) error
	CreateAPIKey(ctx context.Context, workspaceID string, emailPrefix string) (string, string, error)
	RemoveMember(ctx context.Context, workspaceID string, userIDToRemove string) error

	// Invitation management
	GetInvitationByID(ctx context.Context, invitationID string) (*WorkspaceInvitation, error)
	AcceptInvitation(ctx context.Context, invitationID, workspaceID, email string) (*AuthResponse, error)
	DeleteInvitation(ctx context.Context, invitationID string) error

	// Integration management
	CreateIntegration(ctx context.Context, req CreateIntegrationRequest) (string, error)
	UpdateIntegration(ctx context.Context, req UpdateIntegrationRequest) error
	DeleteIntegration(ctx context.Context, workspaceID, integrationID string) error

	// Permission management
	SetUserPermissions(ctx context.Context, workspaceID, userID string, permissions UserPermissions) error
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
	WorkspaceID       string                       `json:"workspace_id"`
	Name              string                       `json:"name"`
	Type              IntegrationType              `json:"type"`
	Provider          EmailProvider                `json:"provider,omitempty"`           // For email integrations
	SupabaseSettings  *SupabaseIntegrationSettings `json:"supabase_settings,omitempty"`  // For Supabase integrations
	LLMProvider       *LLMProvider                 `json:"llm_provider,omitempty"`       // For LLM integrations
	FirecrawlSettings *FirecrawlSettings           `json:"firecrawl_settings,omitempty"` // For Firecrawl integrations
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

	// Validate based on integration type
	switch r.Type {
	case IntegrationTypeEmail:
		if err := r.Provider.Validate(passphrase); err != nil {
			return fmt.Errorf("invalid provider configuration: %w", err)
		}
	case IntegrationTypeSupabase:
		if r.SupabaseSettings == nil {
			return fmt.Errorf("supabase settings are required for supabase integration")
		}
		if err := r.SupabaseSettings.Validate(passphrase); err != nil {
			return fmt.Errorf("invalid supabase settings: %w", err)
		}
	case IntegrationTypeLLM:
		if r.LLMProvider == nil {
			return fmt.Errorf("llm provider settings are required for llm integration")
		}
		if err := r.LLMProvider.Validate(passphrase); err != nil {
			return fmt.Errorf("invalid llm provider configuration: %w", err)
		}
	case IntegrationTypeFirecrawl:
		if r.FirecrawlSettings == nil {
			return fmt.Errorf("firecrawl settings are required for firecrawl integration")
		}
		if err := r.FirecrawlSettings.Validate(passphrase); err != nil {
			return fmt.Errorf("invalid firecrawl settings: %w", err)
		}
	default:
		return fmt.Errorf("unsupported integration type: %s", r.Type)
	}

	return nil
}

// UpdateIntegrationRequest defines the request structure for updating an integration
type UpdateIntegrationRequest struct {
	WorkspaceID       string                       `json:"workspace_id"`
	IntegrationID     string                       `json:"integration_id"`
	Name              string                       `json:"name"`
	Provider          EmailProvider                `json:"provider,omitempty"`           // For email integrations
	SupabaseSettings  *SupabaseIntegrationSettings `json:"supabase_settings,omitempty"`  // For Supabase integrations
	LLMProvider       *LLMProvider                 `json:"llm_provider,omitempty"`       // For LLM integrations
	FirecrawlSettings *FirecrawlSettings           `json:"firecrawl_settings,omitempty"` // For Firecrawl integrations
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

	// Validate provider/settings configuration based on what's provided
	// Note: We don't validate the type here since it cannot be changed in updates
	if r.Provider.Kind != "" {
		if err := r.Provider.Validate(passphrase); err != nil {
			return fmt.Errorf("invalid provider configuration: %w", err)
		}
	} else if r.SupabaseSettings != nil {
		if err := r.SupabaseSettings.Validate(passphrase); err != nil {
			return fmt.Errorf("invalid supabase settings: %w", err)
		}
	} else if r.LLMProvider != nil {
		if err := r.LLMProvider.Validate(passphrase); err != nil {
			return fmt.Errorf("invalid llm provider configuration: %w", err)
		}
	} else if r.FirecrawlSettings != nil {
		if err := r.FirecrawlSettings.Validate(passphrase); err != nil {
			return fmt.Errorf("invalid firecrawl settings: %w", err)
		}
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
	WorkspaceID string          `json:"workspace_id"`
	Email       string          `json:"email"`
	Permissions UserPermissions `json:"permissions,omitempty"`
}

// SetUserPermissionsRequest defines the request structure for setting user permissions
type SetUserPermissionsRequest struct {
	WorkspaceID string          `json:"workspace_id"`
	UserID      string          `json:"user_id"`
	Permissions UserPermissions `json:"permissions"`
}

// Validate validates the set user permissions request
func (r *SetUserPermissionsRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if !govalidator.IsAlphanumeric(r.WorkspaceID) {
		return fmt.Errorf("workspace_id must be alphanumeric")
	}
	if len(r.WorkspaceID) > 32 {
		return fmt.Errorf("workspace_id length must be between 1 and 32")
	}
	if r.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if r.Permissions == nil {
		return fmt.Errorf("permissions is required")
	}
	return nil
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
