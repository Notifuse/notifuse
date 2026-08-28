package domain

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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
	PermissionResourceWebAnalytics   PermissionResource = "web_analytics"

	PermissionResourceSegments             PermissionResource = "segments"
	PermissionResourceWebhookSubscriptions PermissionResource = "webhook_subscriptions"
	PermissionResourceWebhookEvents        PermissionResource = "webhook_events"
)

// PermissionType defines the types of permissions (read/write)
type PermissionType string

const (
	PermissionTypeRead  PermissionType = "read"
	PermissionTypeWrite PermissionType = "write"
)

// AllPermissionResources is the canonical list of permission resources.
// FullPermissions and UserPermissions.Validate both derive from it.
var AllPermissionResources = []PermissionResource{
	// Audience
	PermissionResourceContacts,
	PermissionResourceSegments,
	PermissionResourceLists,
	// Content
	PermissionResourceTemplates,
	PermissionResourceBlog,
	// Sending
	PermissionResourceBroadcasts,
	PermissionResourceTransactional,
	PermissionResourceAutomations,
	// Reporting
	PermissionResourceMessageHistory,
	PermissionResourceWebAnalytics,
	// Integrations
	PermissionResourceWebhookSubscriptions,
	PermissionResourceWebhookEvents,
	PermissionResourceLLM,
	// Workspace
	PermissionResourceWorkspace,
}

// knownPermissionResources is the lookup set behind UserPermissions.Validate
var knownPermissionResources = func() map[PermissionResource]struct{} {
	set := make(map[PermissionResource]struct{}, len(AllPermissionResources))
	for _, resource := range AllPermissionResources {
		set[resource] = struct{}{}
	}
	return set
}()

// NewFullPermissions returns a fresh map granting read and write on every resource.
// Callers must never share FullPermissions by reference: it is a package-level map
// and mutating it corrupts the global for the whole process.
func NewFullPermissions() UserPermissions {
	permissions := make(UserPermissions, len(AllPermissionResources))
	for _, resource := range AllPermissionResources {
		permissions[resource] = ResourcePermissions{Read: true, Write: true}
	}
	return permissions
}

var FullPermissions = NewFullPermissions()

// ResourcePermissions defines read/write permissions for a specific resource
type ResourcePermissions struct {
	Read  bool `json:"read"`
	Write bool `json:"write"`
}

// UserPermissions maps resources to their permission settings
type UserPermissions map[PermissionResource]ResourcePermissions

// Validate rejects unknown resource keys. A nil or empty map is valid: it means
// no permissions at all.
func (up UserPermissions) Validate() error {
	for resource := range up {
		if _, ok := knownPermissionResources[resource]; !ok {
			return fmt.Errorf("unknown permission resource: %s", resource)
		}
	}
	return nil
}

// Value implements the driver.Valuer interface for database serialization.
// Only a nil map becomes SQL NULL: an explicitly empty map persists as '{}' so
// it stays visible to the permission backfills, which skip NULL rows.
func (up UserPermissions) Value() (driver.Value, error) {
	if up == nil {
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
	IntegrationTypeZapier    IntegrationType = "zapier"
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
	ZapierSettings    *ZapierSettings              `json:"zapier_settings,omitempty"`
	// CredentialHints maps a credential to its last few characters, so an owner
	// can tell which key is configured without the key being served. Computed by
	// Redact at the API boundary and cleared by BeforeSave — never stored.
	CredentialHints map[string]string `json:"credential_hints,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
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
	case IntegrationTypeZapier:
		// Nothing a Zapier record holds is encrypted, so its Validate takes no passphrase.
		// The nil check is what closes the create path to service-layer callers that never
		// reach CreateIntegrationRequest.Validate: no create switch fills these settings, so
		// a zapier record arriving that way carries none and is rejected here.
		if i.ZapierSettings == nil {
			return fmt.Errorf("zapier settings are required for zapier integration")
		}
		if err := i.ZapierSettings.Validate(); err != nil {
			return fmt.Errorf("invalid zapier settings: %w", err)
		}
	default:
		return fmt.Errorf("unsupported integration type: %s", i.Type)
	}

	return nil
}

// BeforeSave prepares an Integration for saving by encrypting secrets
func (i *Integration) BeforeSave(secretkey string) error {
	// Display-only, recomputed on every read. Persisting it would leave a stale
	// hint behind after a rotation, and put a fragment of the secret in a column
	// that is not meant to hold one.
	i.CredentialHints = nil

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
	FeedSummaryOnly  bool         `json:"feed_summary_only,omitempty"`  // When true, RSS/JSON feeds emit excerpt instead of full HTML
	FeedMaxItems     int          `json:"feed_max_items,omitempty"`     // Items per RSS/JSON feed (default and cap: 20)
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

// GetFeedMaxItems returns the feed item cap, clamped to [1, 20].
func (bs *BlogSettings) GetFeedMaxItems() int {
	if bs == nil || bs.FeedMaxItems < 1 || bs.FeedMaxItems > 20 {
		return 20
	}
	return bs.FeedMaxItems
}

// Validate checks the bounded blog settings fields. The Get* accessors clamp the
// page sizes on read, so this is light hygiene that also guards non-console API
// callers against out-of-range values. URL/SEO fields are left lenient (the
// console constrains them); the bounds can be tightened later.
func (bs *BlogSettings) Validate() error {
	if bs == nil {
		return nil
	}
	if len(bs.Title) > 255 {
		return fmt.Errorf("blog title exceeds maximum length of 255 characters")
	}
	if bs.HomePageSize != 0 && (bs.HomePageSize < 1 || bs.HomePageSize > 100) {
		return fmt.Errorf("home_page_size must be between 1 and 100")
	}
	if bs.CategoryPageSize != 0 && (bs.CategoryPageSize < 1 || bs.CategoryPageSize > 100) {
		return fmt.Errorf("category_page_size must be between 1 and 100")
	}
	if bs.FeedMaxItems != 0 && (bs.FeedMaxItems < 1 || bs.FeedMaxItems > 20) {
		return fmt.Errorf("feed_max_items must be between 1 and 20")
	}
	return nil
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
	// TemplateBlocks live inside this settings blob rather than in their own table,
	// which is why block CRUD emits no webhook events while template CRUD does:
	// every webhook event in the product is produced by a row trigger writing to
	// webhook_deliveries, and there are no block rows to trigger on. Adding them
	// would mean inserting deliveries from the service layer — a second, parallel
	// mechanism — so it is a deliberate initiative, not an oversight to patch.
	TemplateBlocks    []TemplateBlock       `json:"template_blocks,omitempty"`
	CustomEndpointURL *string               `json:"custom_endpoint_url,omitempty"`
	CustomFieldLabels map[string]string     `json:"custom_field_labels,omitempty"`
	BlogEnabled       bool                  `json:"blog_enabled"`            // Enable blog feature at workspace level
	BlogSettings      *BlogSettings         `json:"blog_settings,omitempty"` // Blog styling and SEO settings
	WebAnalytics      *WebAnalyticsSettings `json:"web_analytics,omitempty"` // Web analytics configuration
	DefaultLanguage   string                `json:"default_language"`
	Languages         []string              `json:"languages"`

	// decoded secret key, not stored in the database
	SecretKey string `json:"-"`

	// omittedKeys names the settings an update body did not carry, so the service
	// can tell "the caller said nothing" from "the caller sent the zero value" —
	// every field above has a meaningful zero, and the difference is otherwise gone
	// by the time UpdateWorkspace runs.
	//
	// Written only by UpdateWorkspaceRequest.UnmarshalJSON, so settings decoded
	// anywhere else (a stored row, another endpoint) carry no record and are used
	// whole, exactly as before.
	omittedKeys map[string]struct{}
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

	// Validate default language is set
	if ws.DefaultLanguage == "" {
		return fmt.Errorf("default language is required")
	}

	// Validate language settings - languages list is mandatory
	if len(ws.Languages) == 0 {
		return fmt.Errorf("languages list is required and must contain at least one language")
	}

	seen := make(map[string]bool)
	for _, lang := range ws.Languages {
		if !IsValidLanguage(lang) {
			return fmt.Errorf("invalid language code: %s", lang)
		}
		if seen[lang] {
			return fmt.Errorf("duplicate language code: %s", lang)
		}
		seen[lang] = true
	}

	if !IsValidLanguage(ws.DefaultLanguage) {
		return fmt.Errorf("invalid default language code: %s", ws.DefaultLanguage)
	}

	found := false
	for _, lang := range ws.Languages {
		if lang == ws.DefaultLanguage {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("default language %s must be in the languages list", ws.DefaultLanguage)
	}

	if err := ws.WebAnalytics.Validate(); err != nil {
		return fmt.Errorf("invalid web analytics settings: %w", err)
	}

	return nil
}

// ResolveEndpoint returns the base endpoint used for tracking links, the
// notification center, and template URL composition: the configured Custom
// Endpoint URL when set, otherwise the provided default API endpoint.
//
// Note: this falls back to the API endpoint. The blog/public-site origin uses a
// different fallback (Custom Endpoint URL → Website URL); do not use this method
// for blog URL composition.
func (ws *WorkspaceSettings) ResolveEndpoint(apiEndpoint string) string {
	if ws.CustomEndpointURL != nil && *ws.CustomEndpointURL != "" {
		return *ws.CustomEndpointURL
	}
	return apiEndpoint
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
	provider, _, err := w.GetEmailProviderWithIntegrationID(isMarketing)
	return provider, err
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

	// A transactional-only provider (e.g. Mailjet, whose API rewrites unsubscribe
	// links) must never serve marketing sends. Enforced here — the resolution
	// point every send path goes through — so assignments made before the
	// restriction existed are blocked too, not just new ones at settings-save.
	if isMarketing && integration.EmailProvider.Kind.IsTransactionalOnly() {
		return nil, "", fmt.Errorf("%s cannot be used as a marketing email provider", integration.EmailProvider.Kind)
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
	Permissions UserPermissions `json:"permissions" db:"permissions"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

// UserWorkspaceWithEmail extends UserWorkspace to include user email
type UserWorkspaceWithEmail struct {
	UserWorkspace
	Email               string     `json:"email" db:"email"`
	Type                UserType   `json:"type" db:"type"`
	Language            string     `json:"language" db:"language"`
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

	// PatchIntegrationSESSettings merges patch into one integration's SES settings in a single
	// statement. Update() rewrites the whole row, so a read-modify-write there races with any
	// concurrent integration edit and silently loses one side's changes. Server-owned fields
	// are written through this instead.
	PatchIntegrationSESSettings(ctx context.Context, workspaceID string, integrationID string, patch map[string]interface{}) error
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
	CountWorkspaceMembersAndInvitations(ctx context.Context, workspaceID string) (int, error)
	CountWorkspaces(ctx context.Context) (int, error)

	// Database management
	GetConnection(ctx context.Context, workspaceID string) (*sql.DB, error)
	GetSystemConnection(ctx context.Context) (*sql.DB, error)
	CreateDatabase(ctx context.Context, workspaceID string) error
	DeleteDatabase(ctx context.Context, workspaceID string) error

	// Transaction management
	WithWorkspaceTransaction(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error
}

// ErrUserNotInWorkspace is returned (errors.Is-able) when a user has no membership
// row for a workspace. Callers (e.g. the platform-admin override in AuthService) use
// errors.Is to distinguish a plain "not a member" from a real database failure.
var ErrUserNotInWorkspace = errors.New("user is not a member of the workspace")

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

// ErrTeamMemberLimitReached is returned when a workspace has reached its team member limit
type ErrTeamMemberLimitReached struct {
	Limit   int
	Current int
}

func (e *ErrTeamMemberLimitReached) Error() string {
	return fmt.Sprintf("team member limit reached: workspace has %d members and invitations (limit: %d)", e.Current, e.Limit)
}

// ErrWorkspaceLimitReached is returned when the system has reached its workspace limit
type ErrWorkspaceLimitReached struct {
	Limit   int
	Current int
}

func (e *ErrWorkspaceLimitReached) Error() string {
	return fmt.Sprintf("workspace limit reached: %d workspaces exist (limit: %d)", e.Current, e.Limit)
}

// WorkspaceServiceInterface defines the interface for workspace operations
type WorkspaceServiceInterface interface {
	CreateWorkspace(ctx context.Context, id, name, websiteURL, logoURL, coverURL, timezone string, fileManager FileManagerSettings, defaultLanguage string, languages []string) (*Workspace, error)
	GetWorkspace(ctx context.Context, id string) (*Workspace, error)
	ListWorkspaces(ctx context.Context) ([]*Workspace, error)
	UpdateWorkspace(ctx context.Context, id, name string, settings WorkspaceSettings) (*Workspace, error)
	DeleteWorkspace(ctx context.Context, id string) error
	GetWorkspaceMembersWithEmail(ctx context.Context, id string) ([]*UserWorkspaceWithEmail, error)
	InviteMember(ctx context.Context, workspaceID, email string, permissions UserPermissions) (*WorkspaceInvitation, string, error)
	AddUserToWorkspace(ctx context.Context, workspaceID string, userID string, role string, permissions UserPermissions) error
	RemoveUserFromWorkspace(ctx context.Context, workspaceID string, userID string) error
	TransferOwnership(ctx context.Context, workspaceID string, newOwnerID string, currentOwnerID string) error
	CreateAPIKey(ctx context.Context, workspaceID string, emailPrefix string, permissions UserPermissions) (string, string, error)
	RemoveMember(ctx context.Context, workspaceID string, userIDToRemove string) error

	// Invitation management
	GetInvitationByID(ctx context.Context, invitationID string) (*WorkspaceInvitation, error)
	AcceptInvitation(ctx context.Context, invitationID, workspaceID, email string) (*AuthResponse, error)
	DeleteInvitation(ctx context.Context, invitationID string) error

	// Integration management
	CreateIntegration(ctx context.Context, req CreateIntegrationRequest) (string, error)
	UpdateIntegration(ctx context.Context, req UpdateIntegrationRequest) error
	DeleteIntegration(ctx context.Context, workspaceID, integrationID string) error

	// ConnectZapier mints an API key for a Zapier connection and records it as a zapier
	// integration in one call. It returns the key's token — shown once and unrecoverable
	// afterwards — the address the key answers to, and the id of the integration written.
	ConnectZapier(ctx context.Context, workspaceID string, label string) (token string, email string, integrationID string, err error)

	// Permission management
	SetUserPermissions(ctx context.Context, workspaceID, userID string, permissions UserPermissions) error

	// Custom field management
	SetCustomFieldLabels(ctx context.Context, workspaceID string, labels map[string]string) error

	// Blog management. A nil enabled means the caller said nothing about the flag,
	// and the stored one stands; the settings are replaced whole.
	// SetBlogSettings writes the two blog fields onto the workspace. Both arguments carry
	// their own answer to "did the caller mention this at all": a nil enabled leaves the
	// stored flag alone, and settingsSpecified false leaves the stored configuration alone.
	// The settings need the separate flag because nil already means something else — it is
	// how the configuration is deliberately cleared.
	SetBlogSettings(ctx context.Context, workspaceID string, enabled *bool, settings *BlogSettings, settingsSpecified bool) error

	// SetWebAnalyticsSettings replaces the workspace's web analytics settings
	// (gated by web_analytics:write; recomputes the filters version).
	SetWebAnalyticsSettings(ctx context.Context, workspaceID string, settings *WebAnalyticsSettings) error
}

// Request/Response types

// apiKeyEmailPrefixRegex constrains the local part of the generated api key email
var apiKeyEmailPrefixRegex = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

// CreateAPIKeyRequest defines the request structure for creating an API key
type CreateAPIKeyRequest struct {
	WorkspaceID string          `json:"workspace_id"`
	EmailPrefix string          `json:"email_prefix"`
	Permissions UserPermissions `json:"permissions,omitempty"` // absent or null means full access
}

// Validate validates the create API key request
func (r *CreateAPIKeyRequest) Validate() error {
	if r.WorkspaceID == "" {
		return errors.New("workspace ID is required")
	}
	if r.EmailPrefix == "" {
		return errors.New("email prefix is required")
	}
	if !apiKeyEmailPrefixRegex.MatchString(r.EmailPrefix) {
		return errors.New("email prefix must match ^[a-z0-9_-]{1,64}$")
	}
	if r.Permissions != nil {
		if err := r.Permissions.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// CreateIntegrationRequest defines the request structure for creating an integration.
//
// There is deliberately no zapier_settings field. A Zapier record holds one fact — the address
// of the API key minted for it — and the server derives that address itself, so nothing a
// client could send would fill it. Zapier connections are made through ConnectZapier instead,
// which is why Validate below rejects the type outright.
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
	case IntegrationTypeZapier:
		// Cosmetic, not a gate: the default below rejects zapier just as firmly once the
		// constant exists. This only trades "unsupported integration type" for a message
		// that names the endpoint the caller wanted.
		return fmt.Errorf("zapier integrations are created by workspaces.connectZapier")
	default:
		return fmt.Errorf("unsupported integration type: %s", r.Type)
	}

	return nil
}

// UpdateIntegrationRequest defines the request structure for updating an integration.
//
// There is deliberately no zapier_settings field, and here the absence is load-bearing rather
// than tidy. UpdateIntegration rebuilds the integration from id, name and type and then refills
// its settings from a switch on the stored type; a field here would let a rename arrive with a
// blank address and overwrite the minted key's. With no field, no payload can express that.
type UpdateIntegrationRequest struct {
	WorkspaceID       string                       `json:"workspace_id"`
	IntegrationID     string                       `json:"integration_id"`
	Name              string                       `json:"name"`
	Provider          EmailProvider                `json:"provider,omitempty"`           // For email integrations
	SupabaseSettings  *SupabaseIntegrationSettings `json:"supabase_settings,omitempty"`  // For Supabase integrations
	LLMProvider       *LLMProvider                 `json:"llm_provider,omitempty"`       // For LLM integrations
	FirecrawlSettings *FirecrawlSettings           `json:"firecrawl_settings,omitempty"` // For Firecrawl integrations

	// providerOmitted records that the body named no provider. The three settings
	// above are pointers, so nil already says that for them; Provider is a value,
	// and without this flag "the caller sent nothing" and "the caller sent an empty
	// provider" are the same bits by the time the service sees them.
	//
	// The polarity is deliberate: the zero value means PRESENT, so a request built
	// in Go — which has no body to read a key set from — keeps meaning exactly what
	// its fields say.
	providerOmitted bool
}

// UnmarshalJSON decodes the request and records whether the body named provider.
//
// A null provider counts as omitted. An email integration with no provider is not
// a state a caller can have meant — it stops sending — so a null there can only be
// a serializer writing out an empty optional.
func (r *UpdateIntegrationRequest) UnmarshalJSON(data []byte) error {
	type wire UpdateIntegrationRequest // sheds this method, so the decode does not recurse
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}

	*r = UpdateIntegrationRequest(decoded)
	raw, present := keys["provider"]
	r.providerOmitted = !present || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
	return nil
}

// ProviderSpecified reports whether the body that produced this request carried a
// provider. False means "leave the stored provider alone", never "clear it".
func (r *UpdateIntegrationRequest) ProviderSpecified() bool {
	return !r.providerOmitted
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

// preservableWorkspaceSettingKeys are the settings UpdateWorkspace copies from the
// request onto the stored workspace, and so the ones whose absence has to survive
// the decode. PreserveOmitted restores exactly these: a key added here wants a line
// there, and vice versa.
//
// timezone, default_language and languages are deliberately absent. Validate
// rejects a body that omits them, upstream of the service, so a preserve for them
// could never run. template_blocks is absent too: it is a pointer-shaped slice and
// UpdateWorkspace already skips a nil one.
var preservableWorkspaceSettingKeys = []string{
	"website_url",
	"logo_url",
	"cover_url",
	"file_manager",
	"transactional_email_provider_id",
	"marketing_email_provider_id",
	"email_tracking_enabled",
	"custom_endpoint_url",
}

// UnmarshalJSON decodes the request and records which settings the body left out.
//
// Presence here means the key is there, not that its value is non-null: the console
// clears the logo by sending null, so null has to keep meaning "clear it".
//
// The record is kept on the decoded Settings because that value is all the service
// is given — UpdateWorkspace takes settings, not the request.
func (r *UpdateWorkspaceRequest) UnmarshalJSON(data []byte) error {
	type wire UpdateWorkspaceRequest // sheds this method, so the decode does not recurse
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = UpdateWorkspaceRequest(decoded)

	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil {
		return err
	}
	var sent map[string]json.RawMessage
	if raw, ok := body["settings"]; ok {
		if err := json.Unmarshal(raw, &sent); err != nil {
			return err
		}
	}

	r.Settings.omittedKeys = nil
	for _, key := range preservableWorkspaceSettingKeys {
		if _, ok := sent[key]; ok {
			continue
		}
		if r.Settings.omittedKeys == nil {
			r.Settings.omittedKeys = make(map[string]struct{}, len(preservableWorkspaceSettingKeys))
		}
		r.Settings.omittedKeys[key] = struct{}{}
	}
	return nil
}

// PreserveOmitted restores, from the workspace as stored, every setting the update
// body did not name.
//
// Settings assembled in Go record nothing and are therefore applied whole, which is
// the only thing a caller with no body to omit keys from can mean.
func (ws *WorkspaceSettings) PreserveOmitted(stored WorkspaceSettings) {
	if len(ws.omittedKeys) == 0 {
		return
	}
	keep := func(key string, restore func()) {
		if _, omitted := ws.omittedKeys[key]; omitted {
			restore()
		}
	}

	keep("website_url", func() { ws.WebsiteURL = stored.WebsiteURL })
	keep("logo_url", func() { ws.LogoURL = stored.LogoURL })
	keep("cover_url", func() { ws.CoverURL = stored.CoverURL })
	keep("file_manager", func() { ws.FileManager = stored.FileManager })
	keep("transactional_email_provider_id", func() { ws.TransactionalEmailProviderID = stored.TransactionalEmailProviderID })
	keep("marketing_email_provider_id", func() { ws.MarketingEmailProviderID = stored.MarketingEmailProviderID })
	keep("email_tracking_enabled", func() { ws.EmailTrackingEnabled = stored.EmailTrackingEnabled })
	keep("custom_endpoint_url", func() { ws.CustomEndpointURL = stored.CustomEndpointURL })
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

// SetCustomFieldLabelsRequest defines the request structure for setting custom field labels
type SetCustomFieldLabelsRequest struct {
	WorkspaceID       string            `json:"workspace_id"`
	CustomFieldLabels map[string]string `json:"custom_field_labels"`
}

// Validate validates the set custom field labels request and returns the
// sanitized workspace ID and labels. An empty/nil label map is valid and
// clears all custom field labels.
func (r *SetCustomFieldLabelsRequest) Validate() (workspaceID string, labels map[string]string, err error) {
	if r.WorkspaceID == "" {
		return "", nil, fmt.Errorf("invalid set custom field labels request: workspace_id is required")
	}
	if !govalidator.IsAlphanumeric(r.WorkspaceID) {
		return "", nil, fmt.Errorf("invalid set custom field labels request: workspace_id must be alphanumeric")
	}
	if len(r.WorkspaceID) > 32 {
		return "", nil, fmt.Errorf("invalid set custom field labels request: workspace_id length must be between 1 and 32")
	}

	// Reuse the canonical label validation
	settings := &WorkspaceSettings{CustomFieldLabels: r.CustomFieldLabels}
	if err := settings.ValidateCustomFieldLabels(); err != nil {
		return "", nil, err
	}

	return r.WorkspaceID, r.CustomFieldLabels, nil
}

// SetBlogSettingsRequest defines the request structure for setting blog settings
// (the enable flag plus title/SEO/pagination/feed config) via the dedicated,
// blog:write gated endpoint.
type SetBlogSettingsRequest struct {
	WorkspaceID  string        `json:"workspace_id"`
	BlogEnabled  bool          `json:"blog_enabled"`
	BlogSettings *BlogSettings `json:"blog_settings"`

	// blogEnabledOmitted records that the body named no blog_enabled. Both of that
	// flag's values are meaningful, so without this the endpoint reads "the caller
	// said nothing" as "turn the blog off". The console works around it by
	// recomputing the flag from the workspace it holds; no other client can.
	//
	// Zero means PRESENT, so a request built in Go — which has no body to read a key
	// set from — keeps meaning exactly what its fields say.
	blogEnabledOmitted bool

	// blogSettingsOmitted records that the body named no blog_settings. The stored
	// configuration is replaced by whatever this request carries, so without this an
	// absent key erases the title, the SEO block, the pagination and the feed settings
	// — which is what a caller flipping blog_enabled on its own asks for by accident.
	//
	// Zero means PRESENT here too, for the same reason.
	blogSettingsOmitted bool
}

// UnmarshalJSON decodes the request and records whether the body named blog_enabled.
//
// A null counts as omitted: there is no bool a null could have meant, so it can only
// be a serializer writing out an absent optional.
func (r *SetBlogSettingsRequest) UnmarshalJSON(data []byte) error {
	type wire SetBlogSettingsRequest // sheds this method, so the decode does not recurse
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}

	*r = SetBlogSettingsRequest(decoded)
	raw, present := keys["blog_enabled"]
	r.blogEnabledOmitted = !present || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))

	// Presence alone for the settings, null included. Unlike a bool, an object has a null
	// that means something — it is how the configuration is cleared — so folding null into
	// "omitted" would take that away.
	_, settingsPresent := keys["blog_settings"]
	r.blogSettingsOmitted = !settingsPresent
	return nil
}

// enabledFlag returns the flag the body carried, or nil when it carried none. The
// copy keeps the caller from writing back into the request through the pointer.
func (r *SetBlogSettingsRequest) enabledFlag() *bool {
	if r.blogEnabledOmitted {
		return nil
	}
	enabled := r.BlogEnabled
	return &enabled
}

// Validate validates the set blog settings request and returns the sanitized
// workspace ID, the enabled flag, the (possibly nil) blog settings, and whether
// the body said anything about them at all.
//
// A nil enabled means the body did not name blog_enabled and the stored flag
// stands. The two settings results are separate answers: settingsSpecified false
// means the body named no blog_settings and the stored configuration stands,
// while a nil settings that WAS specified is the explicit null that clears it.
func (r *SetBlogSettingsRequest) Validate() (workspaceID string, enabled *bool, settings *BlogSettings, settingsSpecified bool, err error) {
	if r.WorkspaceID == "" {
		return "", nil, nil, false, fmt.Errorf("invalid set blog settings request: workspace_id is required")
	}
	if !govalidator.IsAlphanumeric(r.WorkspaceID) {
		return "", nil, nil, false, fmt.Errorf("invalid set blog settings request: workspace_id must be alphanumeric")
	}
	if len(r.WorkspaceID) > 32 {
		return "", nil, nil, false, fmt.Errorf("invalid set blog settings request: workspace_id length must be between 1 and 32")
	}

	if r.BlogSettings != nil {
		if err := r.BlogSettings.Validate(); err != nil {
			return "", nil, nil, false, err
		}
	}

	return r.WorkspaceID, r.enabledFlag(), r.BlogSettings, !r.blogSettingsOmitted, nil
}

// SetWebAnalyticsSettingsRequest defines the request structure for replacing a
// workspace's web analytics settings via the dedicated, web_analytics:write
// gated endpoint.
type SetWebAnalyticsSettingsRequest struct {
	WorkspaceID string                `json:"workspace_id"`
	Settings    *WebAnalyticsSettings `json:"settings"`
}

// Validate validates the request and returns the workspace ID and the
// (possibly nil) settings. Nil settings clear the stored configuration.
func (r *SetWebAnalyticsSettingsRequest) Validate() (workspaceID string, settings *WebAnalyticsSettings, err error) {
	if r.WorkspaceID == "" {
		return "", nil, fmt.Errorf("invalid set web analytics settings request: workspace_id is required")
	}
	if !govalidator.IsAlphanumeric(r.WorkspaceID) {
		return "", nil, fmt.Errorf("invalid set web analytics settings request: workspace_id must be alphanumeric")
	}
	if len(r.WorkspaceID) > 32 {
		return "", nil, fmt.Errorf("invalid set web analytics settings request: workspace_id length must be between 1 and 32")
	}
	if err := r.Settings.ValidateForSave(); err != nil {
		return "", nil, err
	}
	return r.WorkspaceID, r.Settings, nil
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

// Validate validates the set user permissions request. An empty map is allowed
// here: this is the deliberate path for zeroing an existing member's permissions.
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
	if err := r.Permissions.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate validates the invite member request. Unlike SetUserPermissionsRequest,
// an empty map is rejected here: an invitation with no permissions produces a
// member who can do nothing, which is never what the caller meant.
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

	if len(r.Permissions) == 0 {
		return fmt.Errorf("invalid invite member request: permissions is required and must grant at least one resource")
	}
	if err := r.Permissions.Validate(); err != nil {
		return fmt.Errorf("invalid invite member request: %w", err)
	}

	return nil
}

// TestEmailProviderRequest is the request for testing an email provider
// It includes the provider config, a recipient email, and the workspace ID
type TestEmailProviderRequest struct {
	Provider EmailProvider `json:"provider"`
	To       string        `json:"to"`
	// IntegrationID names the saved integration being tested, when there is one.
	// Credentials are not served to clients, so a client testing a saved
	// integration cannot send them back; blank ones are filled from this
	// integration. Absent when testing a provider not yet saved, where the client
	// still holds what it typed.
	IntegrationID string `json:"integration_id,omitempty"`
	WorkspaceID   string `json:"workspace_id"`
}

// TestEmailProviderResponse is the response for testing an email provider
// It can be extended to include more details if needed
type TestEmailProviderResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}
