package domain

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"time"

	// Import the notifuse_mjml package

	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/asaskevich/govalidator"
)

//go:generate mockgen -destination mocks/mock_template_service.go -package mocks github.com/Notifuse/notifuse/internal/domain TemplateService
//go:generate mockgen -destination mocks/mock_template_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain TemplateRepository

// templateIDPattern allows alphanumeric characters, underscores, and hyphens
var templateIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// validateTemplateID checks if a template ID is valid
func validateTemplateID(id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	if len(id) > 32 {
		return fmt.Errorf("id length must be between 1 and 32")
	}
	if !templateIDPattern.MatchString(id) {
		return fmt.Errorf("id must contain only letters, numbers, underscores, and hyphens")
	}
	return nil
}

// Channel constants for templates
const (
	ChannelEmail = "email"
	ChannelWeb   = "web"
)

type TemplateCategory string

const (
	TemplateCategoryMarketing     TemplateCategory = "marketing"
	TemplateCategoryTransactional TemplateCategory = "transactional"
	TemplateCategoryWelcome       TemplateCategory = "welcome"
	TemplateCategoryOptIn         TemplateCategory = "opt_in"
	TemplateCategoryUnsubscribe   TemplateCategory = "unsubscribe"
	TemplateCategoryBounce        TemplateCategory = "bounce"
	TemplateCategoryBlocklist     TemplateCategory = "blocklist"
	TemplateCategoryBlog          TemplateCategory = "blog"
	TemplateCategoryOther         TemplateCategory = "other"
)

func (t TemplateCategory) Validate() error {
	switch t {
	case TemplateCategoryMarketing, TemplateCategoryTransactional, TemplateCategoryWelcome, TemplateCategoryOptIn, TemplateCategoryUnsubscribe, TemplateCategoryBounce, TemplateCategoryBlocklist, TemplateCategoryBlog, TemplateCategoryOther:
		return nil
	}
	return fmt.Errorf("invalid template category: %s", t)
}

type Template struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Version         int64          `json:"version"`
	Channel         string         `json:"channel"` // email or web
	Email           *EmailTemplate `json:"email,omitempty"`
	Web             *WebTemplate   `json:"web,omitempty"`
	Category        string         `json:"category"`
	TemplateMacroID *string        `json:"template_macro_id,omitempty"`
	IntegrationID   *string        `json:"integration_id,omitempty"` // Set if template is managed by an integration (e.g., Supabase)
	TestData        MapOfAny       `json:"test_data,omitempty"`
	Settings        MapOfAny       `json:"settings,omitempty"` // Channels specific 3rd-party settings
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       *time.Time     `json:"deleted_at,omitempty"`
}

func (t *Template) Validate() error {
	// First validate the template itself
	if err := validateTemplateID(t.ID); err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}

	if t.Name == "" {
		return fmt.Errorf("invalid template: name is required")
	}
	if len(t.Name) > 32 {
		return fmt.Errorf("invalid template: name length must be between 1 and 32")
	}

	if t.Version <= 0 {
		return fmt.Errorf("invalid template: version must be positive")
	}

	if t.Channel == "" {
		return fmt.Errorf("invalid template: channel is required")
	}
	if len(t.Channel) > 20 {
		return fmt.Errorf("invalid template: channel length must be between 1 and 20")
	}

	// Validate channel is either email or web
	if t.Channel != ChannelEmail && t.Channel != ChannelWeb {
		return fmt.Errorf("invalid template: channel must be either '%s' or '%s'", ChannelEmail, ChannelWeb)
	}

	if t.Category == "" {
		return fmt.Errorf("invalid template: category is required")
	}
	if len(t.Category) > 20 {
		return fmt.Errorf("invalid template: category length must be between 1 and 20")
	}

	if t.TestData == nil {
		t.TestData = MapOfAny{}
	}

	// Channel-specific validation
	switch t.Channel {
	case ChannelEmail:
		// Email channel requires email field, web must be nil
		if t.Email == nil {
			return fmt.Errorf("invalid template: email is required for channel '%s'", ChannelEmail)
		}
		if t.Web != nil {
			return fmt.Errorf("invalid template: web must be nil for channel '%s'", ChannelEmail)
		}
		if err := t.Email.Validate(t.TestData); err != nil {
			return fmt.Errorf("invalid template: %w", err)
		}
	case ChannelWeb:
		// Web channel requires web field, email must be nil
		if t.Web == nil {
			return fmt.Errorf("invalid template: web is required for channel '%s'", ChannelWeb)
		}
		if t.Email != nil {
			return fmt.Errorf("invalid template: email must be nil for channel '%s'", ChannelWeb)
		}
		if err := t.Web.Validate(t.TestData); err != nil {
			return fmt.Errorf("invalid template: %w", err)
		}
	}

	return nil
}

type TemplateReference struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}

func (t *TemplateReference) Validate() error {
	// Validate the template reference
	if err := validateTemplateID(t.ID); err != nil {
		return fmt.Errorf("invalid template reference: %w", err)
	}

	if t.Version < 0 {
		return fmt.Errorf("invalid template reference: version must be zero or positive")
	}

	return nil
}

// scan implements the sql.Scanner interface
func (t *TemplateReference) Scan(val interface{}) error {
	var data []byte

	if b, ok := val.([]byte); ok {
		// VERY IMPORTANT: we need to clone the bytes here
		// The sql driver will reuse the same bytes RAM slots for future queries
		// Thank you St Antoine De Padoue for helping me find this bug
		data = bytes.Clone(b)
	} else if s, ok := val.(string); ok {
		data = []byte(s)
	} else if val == nil {
		return nil
	}

	return json.Unmarshal(data, t)
}

// value implements the driver.Valuer interface
func (t TemplateReference) Value() (driver.Value, error) {
	return json.Marshal(t)
}

type EmailTemplate struct {
	SenderID         string                   `json:"sender_id,omitempty"`
	ReplyTo          string                   `json:"reply_to,omitempty"`
	Subject          string                   `json:"subject"`
	SubjectPreview   *string                  `json:"subject_preview,omitempty"`
	CompiledPreview  string                   `json:"compiled_preview"` // compiled html
	VisualEditorTree notifuse_mjml.EmailBlock `json:"visual_editor_tree"`
	Text             *string                  `json:"text,omitempty"`
}

func (e *EmailTemplate) Validate(testData MapOfAny) error {
	// Validate required fields
	if e.Subject == "" {
		return fmt.Errorf("invalid email template: subject is required")
	}
	if len(e.Subject) > 255 {
		return fmt.Errorf("invalid email template: subject length must be between 1 and 255")
	}
	if e.VisualEditorTree.GetType() != notifuse_mjml.MJMLComponentMjml {
		return fmt.Errorf("invalid email template: visual_editor_tree must have type 'mjml'")
	}
	if e.VisualEditorTree.GetChildren() == nil {
		return fmt.Errorf("invalid email template: visual_editor_tree root block must have children")
	}
	if e.CompiledPreview == "" {
		// Prepare template data JSON string
		var templateDataStr string
		if len(testData) > 0 {
			jsonDataBytes, err := json.Marshal(testData)
			if err != nil {
				return fmt.Errorf("failed to marshal test_data: %w", err)
			}
			templateDataStr = string(jsonDataBytes)
		}

		// Compile tree to MJML using our pkg/notifuse_mjml function
		var mjmlResult string
		if templateDataStr != "" {
			result, err := notifuse_mjml.ConvertJSONToMJMLWithData(e.VisualEditorTree, templateDataStr)
			if err != nil {
				return fmt.Errorf("failed to convert tree to MJML: %w", err)
			}
			mjmlResult = result
		} else {
			mjmlResult = notifuse_mjml.ConvertJSONToMJML(e.VisualEditorTree)
		}
		e.CompiledPreview = mjmlResult
	}

	// Validate optional fields
	if e.ReplyTo != "" && !govalidator.IsEmail(e.ReplyTo) {
		return fmt.Errorf("invalid email template: reply_to is not a valid email")
	}
	if e.SubjectPreview != nil && len(*e.SubjectPreview) > 255 {
		return fmt.Errorf("invalid email template: subject_preview length must be between 1 and 255")
	}

	return nil
}

func (x *EmailTemplate) Scan(val interface{}) error {
	var data []byte

	if b, ok := val.([]byte); ok {
		// VERY IMPORTANT: we need to clone the bytes here
		// The sql driver will reuse the same bytes RAM slots for future queries
		// Thank you St Antoine De Padoue for helping me find this bug
		data = bytes.Clone(b)
	} else if s, ok := val.(string); ok {
		data = []byte(s)
	} else if val == nil {
		return nil
	}

	return x.UnmarshalJSON(data)
}

func (x EmailTemplate) Value() (driver.Value, error) {
	return x.MarshalJSON()
}

// MarshalJSON implements custom JSON marshaling for EmailTemplate
func (x EmailTemplate) MarshalJSON() ([]byte, error) {
	type Alias EmailTemplate
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(&x),
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for EmailTemplate
func (x *EmailTemplate) UnmarshalJSON(data []byte) error {
	type Alias EmailTemplate
	aux := &struct {
		VisualEditorTree json.RawMessage `json:"visual_editor_tree"`
		*Alias
	}{
		Alias: (*Alias)(x),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal EmailTemplate: %w", err)
	}

	// Handle the VisualEditorTree field specially
	if len(aux.VisualEditorTree) > 0 {
		block, err := notifuse_mjml.UnmarshalEmailBlock(aux.VisualEditorTree)
		if err != nil {
			return fmt.Errorf("failed to unmarshal VisualEditorTree: %w", err)
		}
		x.VisualEditorTree = block
	}

	return nil
}

type WebTemplate struct {
	Content   MapOfAny `json:"content,omitempty"`    // Tiptap JSON (source of truth)
	HTML      string   `json:"html,omitempty"`       // Pre-rendered HTML for display
	PlainText string   `json:"plain_text,omitempty"` // Extracted text for search indexing
}

func (w *WebTemplate) Validate(testData MapOfAny) error {
	// Validate that Content (Tiptap JSON) is present
	if len(w.Content) == 0 {
		return fmt.Errorf("invalid web template: content is required")
	}

	// HTML should be provided by frontend
	// No need to compile or validate structure here
	return nil
}

func (w *WebTemplate) Scan(val interface{}) error {
	var data []byte

	if b, ok := val.([]byte); ok {
		// VERY IMPORTANT: we need to clone the bytes here
		// The sql driver will reuse the same bytes RAM slots for future queries
		data = bytes.Clone(b)
	} else if s, ok := val.(string); ok {
		data = []byte(s)
	} else if val == nil {
		return nil
	}

	return w.UnmarshalJSON(data)
}

func (w WebTemplate) Value() (driver.Value, error) {
	return w.MarshalJSON()
}

// MarshalJSON implements custom JSON marshaling for WebTemplate
func (w WebTemplate) MarshalJSON() ([]byte, error) {
	type Alias WebTemplate
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(&w),
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for WebTemplate
func (w *WebTemplate) UnmarshalJSON(data []byte) error {
	type Alias WebTemplate
	aux := (*Alias)(w)

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal WebTemplate: %w", err)
	}

	return nil
}

//go:generate mockgen -destination mocks/mock_template_service.go -package mocks github.com/Notifuse/notifuse/internal/domain TemplateService
//go:generate mockgen -destination mocks/mock_template_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain TemplateRepository

// Request/Response types
type CreateTemplateRequest struct {
	WorkspaceID     string         `json:"workspace_id"`
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Channel         string         `json:"channel"`
	Email           *EmailTemplate `json:"email,omitempty"`
	Web             *WebTemplate   `json:"web,omitempty"`
	Category        string         `json:"category"`
	TemplateMacroID *string        `json:"template_macro_id,omitempty"`
	TestData        MapOfAny       `json:"test_data,omitempty"`
	Settings        MapOfAny       `json:"settings,omitempty"`
}

func (r *CreateTemplateRequest) Validate() (template *Template, workspaceID string, err error) {
	if r.WorkspaceID == "" {
		return nil, "", fmt.Errorf("invalid create template request: workspace_id is required")
	}
	if err := validateTemplateID(r.ID); err != nil {
		return nil, "", fmt.Errorf("invalid create template request: %w", err)
	}

	if r.Name == "" {
		return nil, "", fmt.Errorf("invalid create template request: name is required")
	}
	if len(r.Name) > 32 {
		return nil, "", fmt.Errorf("invalid create template request: name length must be between 1 and 32")
	}

	if r.Channel == "" {
		return nil, "", fmt.Errorf("invalid create template request: channel is required")
	}
	if len(r.Channel) > 20 {
		return nil, "", fmt.Errorf("invalid create template request: channel length must be between 1 and 20")
	}

	// Validate channel is either email or web
	if r.Channel != ChannelEmail && r.Channel != ChannelWeb {
		return nil, "", fmt.Errorf("invalid create template request: channel must be either '%s' or '%s'", ChannelEmail, ChannelWeb)
	}

	if r.Category == "" {
		return nil, "", fmt.Errorf("invalid create template request: category is required")
	}
	if len(r.Category) > 20 {
		return nil, "", fmt.Errorf("invalid create template request: category length must be between 1 and 20")
	}

	// Channel-specific validation
	switch r.Channel {
	case ChannelEmail:
		if r.Email == nil {
			return nil, "", fmt.Errorf("invalid create template request: email is required for channel '%s'", ChannelEmail)
		}
		if r.Web != nil {
			return nil, "", fmt.Errorf("invalid create template request: web must be nil for channel '%s'", ChannelEmail)
		}
		if err := r.Email.Validate(r.TestData); err != nil {
			return nil, "", fmt.Errorf("invalid create template request: %w", err)
		}
	case ChannelWeb:
		if r.Web == nil {
			return nil, "", fmt.Errorf("invalid create template request: web is required for channel '%s'", ChannelWeb)
		}
		if r.Email != nil {
			return nil, "", fmt.Errorf("invalid create template request: email must be nil for channel '%s'", ChannelWeb)
		}
		if err := r.Web.Validate(r.TestData); err != nil {
			return nil, "", fmt.Errorf("invalid create template request: %w", err)
		}
	}

	return &Template{
		ID:              r.ID,
		Name:            r.Name,
		Version:         1, // Start with version 1 for new templates
		Channel:         r.Channel,
		Email:           r.Email,
		Web:             r.Web,
		Category:        r.Category,
		TemplateMacroID: r.TemplateMacroID,
		TestData:        r.TestData,
		Settings:        r.Settings,
	}, r.WorkspaceID, nil
}

type GetTemplatesRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Category    string `json:"category,omitempty"`
	Channel     string `json:"channel,omitempty"`
}

func (r *GetTemplatesRequest) FromURLParams(queryParams url.Values) (err error) {
	r.WorkspaceID = queryParams.Get("workspace_id")
	r.Category = queryParams.Get("category")
	r.Channel = queryParams.Get("channel")

	if r.WorkspaceID == "" {
		return fmt.Errorf("invalid get templates request: workspace_id is required")
	}
	if len(r.WorkspaceID) > 20 {
		return fmt.Errorf("invalid get templates request: workspace_id length must be between 1 and 20")
	}

	if r.Category != "" {
		if len(r.Category) > 20 {
			return fmt.Errorf("invalid get templates request: category length must be between 1 and 20")
		}
	}

	if r.Channel != "" {
		if len(r.Channel) > 20 {
			return fmt.Errorf("invalid get templates request: channel length must be between 1 and 20")
		}
	}

	return nil
}

type GetTemplateRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
	Version     int64  `json:"version,omitempty"`
}

func (r *GetTemplateRequest) FromURLParams(queryParams url.Values) (err error) {
	r.WorkspaceID = queryParams.Get("workspace_id")
	r.ID = queryParams.Get("id")
	versionStr := queryParams.Get("version")

	if r.WorkspaceID == "" {
		return fmt.Errorf("invalid get template request: workspace_id is required")
	}

	if err := validateTemplateID(r.ID); err != nil {
		return fmt.Errorf("invalid get template request: %w", err)
	}

	if versionStr != "" {
		version, err := strconv.ParseInt(versionStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid get template request: version must be a valid integer")
		}
		r.Version = version
	}

	return nil
}

type UpdateTemplateRequest struct {
	WorkspaceID     string         `json:"workspace_id"`
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Channel         string         `json:"channel"`
	Email           *EmailTemplate `json:"email,omitempty"`
	Web             *WebTemplate   `json:"web,omitempty"`
	Category        string         `json:"category"`
	TemplateMacroID *string        `json:"template_macro_id,omitempty"`
	TestData        MapOfAny       `json:"test_data,omitempty"`
	Settings        MapOfAny       `json:"settings,omitempty"`
}

func (r *UpdateTemplateRequest) Validate() (template *Template, workspaceID string, err error) {
	if r.WorkspaceID == "" {
		return nil, "", fmt.Errorf("invalid update template request: workspace_id is required")
	}
	if err := validateTemplateID(r.ID); err != nil {
		return nil, "", fmt.Errorf("invalid update template request: %w", err)
	}

	if r.Name == "" {
		return nil, "", fmt.Errorf("invalid update template request: name is required")
	}
	if len(r.Name) > 32 {
		return nil, "", fmt.Errorf("invalid update template request: name length must be between 1 and 32")
	}

	if r.Channel == "" {
		return nil, "", fmt.Errorf("invalid update template request: channel is required")
	}
	if len(r.Channel) > 20 {
		return nil, "", fmt.Errorf("invalid update template request: channel length must be between 1 and 20")
	}

	// Validate channel is either email or web
	if r.Channel != ChannelEmail && r.Channel != ChannelWeb {
		return nil, "", fmt.Errorf("invalid update template request: channel must be either '%s' or '%s'", ChannelEmail, ChannelWeb)
	}

	if r.Category == "" {
		return nil, "", fmt.Errorf("invalid update template request: category is required")
	}
	if len(r.Category) > 20 {
		return nil, "", fmt.Errorf("invalid update template request: category length must be between 1 and 20")
	}

	// Channel-specific validation
	switch r.Channel {
	case ChannelEmail:
		if r.Email == nil {
			return nil, "", fmt.Errorf("invalid update template request: email is required for channel '%s'", ChannelEmail)
		}
		if r.Web != nil {
			return nil, "", fmt.Errorf("invalid update template request: web must be nil for channel '%s'", ChannelEmail)
		}
		if err := r.Email.Validate(r.TestData); err != nil {
			return nil, "", fmt.Errorf("invalid update template request: %w", err)
		}
	case ChannelWeb:
		if r.Web == nil {
			return nil, "", fmt.Errorf("invalid update template request: web is required for channel '%s'", ChannelWeb)
		}
		if r.Email != nil {
			return nil, "", fmt.Errorf("invalid update template request: email must be nil for channel '%s'", ChannelWeb)
		}
		if err := r.Web.Validate(r.TestData); err != nil {
			return nil, "", fmt.Errorf("invalid update template request: %w", err)
		}
	}

	return &Template{
		ID:              r.ID,
		Name:            r.Name,
		Channel:         r.Channel,
		Email:           r.Email,
		Web:             r.Web,
		Category:        r.Category,
		TemplateMacroID: r.TemplateMacroID,
		TestData:        r.TestData,
		Settings:        r.Settings,
	}, r.WorkspaceID, nil
}

type DeleteTemplateRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

func (r *DeleteTemplateRequest) Validate() (workspaceID string, id string, err error) {
	if r.WorkspaceID == "" {
		return "", "", fmt.Errorf("invalid delete template request: workspace_id is required")
	}

	if err := validateTemplateID(r.ID); err != nil {
		return "", "", fmt.Errorf("invalid delete template request: %w", err)
	}

	return r.WorkspaceID, r.ID, nil
}

// --- Compile Request/Response ---

// Use types from notifuse_mjml package
type CompileTemplateRequest = notifuse_mjml.CompileTemplateRequest
type CompileTemplateResponse = notifuse_mjml.CompileTemplateResponse

// TemplateService provides operations for managing templates
type TemplateService interface {
	// CreateTemplate creates a new template
	CreateTemplate(ctx context.Context, workspaceID string, template *Template) error

	// GetTemplateByID retrieves a template by ID and optional version
	GetTemplateByID(ctx context.Context, workspaceID string, id string, version int64) (*Template, error)

	// GetTemplates retrieves all templates
	GetTemplates(ctx context.Context, workspaceID string, category string, channel string) ([]*Template, error)

	// UpdateTemplate updates an existing template
	UpdateTemplate(ctx context.Context, workspaceID string, template *Template) error

	// DeleteTemplate deletes a template by ID
	DeleteTemplate(ctx context.Context, workspaceID string, id string) error

	// CompileTemplate compiles a visual editor tree to MJML and HTML
	CompileTemplate(ctx context.Context, payload CompileTemplateRequest) (*CompileTemplateResponse, error) // Use notifuse_mjml.EmailBlock
}

// TemplateRepository provides database operations for templates
type TemplateRepository interface {
	// CreateTemplate creates a new template in the database
	CreateTemplate(ctx context.Context, workspaceID string, template *Template) error

	// GetTemplateByID retrieves a template by its ID and optional version
	GetTemplateByID(ctx context.Context, workspaceID string, id string, version int64) (*Template, error)

	// GetTemplateLatestVersion retrieves the latest version of a template
	GetTemplateLatestVersion(ctx context.Context, workspaceID string, id string) (int64, error)

	// GetTemplates retrieves all templates
	GetTemplates(ctx context.Context, workspaceID string, category string, channel string) ([]*Template, error)

	// UpdateTemplate updates an existing template, creating a new version
	UpdateTemplate(ctx context.Context, workspaceID string, template *Template) error

	// DeleteTemplate deletes a template
	DeleteTemplate(ctx context.Context, workspaceID string, id string) error
}

// ErrTemplateNotFound is returned when a template is not found
type ErrTemplateNotFound struct {
	Message string
}

func (e *ErrTemplateNotFound) Error() string {
	return e.Message
}

// TemplateDataRequest groups parameters for building template data
type TemplateDataRequest struct {
	WorkspaceID        string                         `json:"workspace_id"`
	WorkspaceSecretKey string                         `json:"workspace_secret_key"`
	ContactWithList    ContactWithList                `json:"contact_with_list"`
	MessageID          string                         `json:"message_id"`
	ProvidedData       MapOfAny                       `json:"provided_data,omitempty"`
	TrackingSettings   notifuse_mjml.TrackingSettings `json:"tracking_settings"`
	Broadcast          *Broadcast                     `json:"broadcast,omitempty"`
}

// Validate ensures that the template data request has all required fields
func (r *TemplateDataRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.WorkspaceSecretKey == "" {
		return fmt.Errorf("workspace_secret_key is required")
	}
	if r.MessageID == "" {
		return fmt.Errorf("message_id is required")
	}
	return nil
}

// BuildTemplateData creates a template data map with flexible options
func BuildTemplateData(req TemplateDataRequest) (MapOfAny, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid template data request: %w", err)
	}

	templateData := MapOfAny{}

	// Add provided data as default data if available
	if req.ProvidedData != nil {
		for key, value := range req.ProvidedData {
			templateData[key] = value
		}
	}

	var emailHMAC string

	if req.ContactWithList.Contact != nil {

		// Use all contact data
		contactData, err := req.ContactWithList.Contact.ToMapOfAny()
		if err != nil {
			return nil, fmt.Errorf("failed to convert contact to template data: %w", err)
		}

		// generate hmac for notification center auth
		emailHMAC = ComputeEmailHMAC(req.ContactWithList.Contact.Email, req.WorkspaceSecretKey)

		templateData["contact"] = contactData

		notificationCenterParams := url.Values{}
		notificationCenterParams.Set("email", req.ContactWithList.Contact.Email)
		notificationCenterParams.Set("email_hmac", emailHMAC)
		notificationCenterParams.Set("wid", req.WorkspaceID)

		notificationCenterURL := fmt.Sprintf("%s/notification-center?%s",
			req.TrackingSettings.Endpoint, notificationCenterParams.Encode())
		templateData["notification_center_url"] = notificationCenterURL

	} else {
		// Create empty contact object if none provided
		templateData["contact"] = MapOfAny{}
	}

	// Add broadcast data if available
	if req.Broadcast != nil {
		templateData["broadcast"] = MapOfAny{
			"id":   req.Broadcast.ID,
			"name": req.Broadcast.Name,
		}

		// Add UTM parameters from broadcast if available
		if req.TrackingSettings.UTMSource != "" {
			templateData["utm_source"] = req.TrackingSettings.UTMSource
		}
		if req.TrackingSettings.UTMMedium != "" {
			templateData["utm_medium"] = req.TrackingSettings.UTMMedium
		}
		if req.TrackingSettings.UTMCampaign != "" {
			templateData["utm_campaign"] = req.TrackingSettings.UTMCampaign
		}
		if req.TrackingSettings.UTMTerm != "" {
			templateData["utm_term"] = req.TrackingSettings.UTMTerm
		}
		if req.TrackingSettings.UTMContent != "" {
			templateData["utm_content"] = req.TrackingSettings.UTMContent
		}
	}

	// Add list data and unsubscribe link if available (requires contact for email-based unsubscribe URLs)
	if req.ContactWithList.ListID != "" && req.WorkspaceID != "" && req.ContactWithList.Contact != nil {

		templateData["list"] = MapOfAny{
			"id":   req.ContactWithList.ListID,
			"name": req.ContactWithList.ListName,
		}

		// Create unsubscribe link
		// Build unsubscribe URL query params
		unsubscribeParams := url.Values{}
		unsubscribeParams.Set("action", "unsubscribe")
		unsubscribeParams.Set("lid", req.ContactWithList.ListID)
		unsubscribeParams.Set("lname", req.ContactWithList.ListName)
		unsubscribeParams.Set("wid", req.WorkspaceID)
		unsubscribeParams.Set("mid", req.MessageID)
		unsubscribeParams.Set("email", req.ContactWithList.Contact.Email)
		unsubscribeParams.Set("email_hmac", emailHMAC)

		unsubscribeURL := fmt.Sprintf("%s/notification-center?%s",
			req.TrackingSettings.Endpoint, unsubscribeParams.Encode())
		templateData["unsubscribe_url"] = unsubscribeURL

		// Build oneclick unsubscribe URL query params
		oneclickParams := url.Values{}
		oneclickParams.Set("email", req.ContactWithList.Contact.Email)
		oneclickParams.Set("lids", req.ContactWithList.ListID)
		oneclickParams.Set("wid", req.WorkspaceID)
		oneclickParams.Set("mid", req.MessageID)

		oneclickUnsubscribeURL := fmt.Sprintf("%s/unsubscribe-oneclick?%s",
			req.TrackingSettings.Endpoint, oneclickParams.Encode())
		templateData["oneclick_unsubscribe_url"] = oneclickUnsubscribeURL

		// Build confirmation URL query params for double opt-in
		confirmParams := url.Values{}
		confirmParams.Set("action", "confirm")
		confirmParams.Set("lid", req.ContactWithList.ListID)
		confirmParams.Set("lname", req.ContactWithList.ListName)
		confirmParams.Set("wid", req.WorkspaceID)
		confirmParams.Set("mid", req.MessageID)
		confirmParams.Set("email", req.ContactWithList.Contact.Email)
		confirmParams.Set("email_hmac", emailHMAC)

		confirmURL := fmt.Sprintf("%s/notification-center?%s",
			req.TrackingSettings.Endpoint, confirmParams.Encode())
		templateData["confirm_subscription_url"] = confirmURL
	}

	// Add global feed data if broadcast has pre-fetched data
	if req.Broadcast != nil && req.Broadcast.DataFeed != nil && req.Broadcast.DataFeed.GlobalFeedData != nil {
		templateData["global_feed"] = req.Broadcast.DataFeed.GlobalFeedData
	}

	// Add tracking data
	templateData["message_id"] = req.MessageID

	// Add tracking pixel if API endpoint is provided

	// Format: {apiEndpoint}/api/pixel?id={messageID}&t=o&w={workspaceID}
	messageID := url.QueryEscape(req.MessageID)
	workspaceID := url.QueryEscape(req.WorkspaceID)

	// Tracking pixel for opens (with timestamp for bot detection)
	sentTimestamp := time.Now().Unix()
	trackingPixelURL := fmt.Sprintf("%s/opens?mid=%s&wid=%s&ts=%d",
		req.TrackingSettings.Endpoint, messageID, workspaceID, sentTimestamp)

	templateData["tracking_opens_url"] = trackingPixelURL

	return templateData, nil
}
