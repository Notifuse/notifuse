package testutil

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/google/uuid"
)

// TestDataFactory creates test data entities using domain repositories
type TestDataFactory struct {
	db                            *sql.DB
	userRepo                      domain.UserRepository
	workspaceRepo                 domain.WorkspaceRepository
	contactRepo                   domain.ContactRepository
	listRepo                      domain.ListRepository
	templateRepo                  domain.TemplateRepository
	broadcastRepo                 domain.BroadcastRepository
	messageHistoryRepo            domain.MessageHistoryRepository
	contactListRepo               domain.ContactListRepository
	transactionalNotificationRepo domain.TransactionalNotificationRepository
}

// NewTestDataFactory creates a new test data factory with repository dependencies
func NewTestDataFactory(
	db *sql.DB,
	userRepo domain.UserRepository,
	workspaceRepo domain.WorkspaceRepository,
	contactRepo domain.ContactRepository,
	listRepo domain.ListRepository,
	templateRepo domain.TemplateRepository,
	broadcastRepo domain.BroadcastRepository,
	messageHistoryRepo domain.MessageHistoryRepository,
	contactListRepo domain.ContactListRepository,
	transactionalNotificationRepo domain.TransactionalNotificationRepository,
) *TestDataFactory {
	return &TestDataFactory{
		db:                            db,
		userRepo:                      userRepo,
		workspaceRepo:                 workspaceRepo,
		contactRepo:                   contactRepo,
		listRepo:                      listRepo,
		templateRepo:                  templateRepo,
		broadcastRepo:                 broadcastRepo,
		messageHistoryRepo:            messageHistoryRepo,
		contactListRepo:               contactListRepo,
		transactionalNotificationRepo: transactionalNotificationRepo,
	}
}

// CreateUser creates a test user using the user repository
func (tdf *TestDataFactory) CreateUser(opts ...UserOption) (*domain.User, error) {
	user := &domain.User{
		ID:        uuid.New().String(),
		Email:     fmt.Sprintf("user-%s@example.com", uuid.New().String()[:8]),
		Name:      "Test User",
		Type:      domain.UserTypeUser,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Apply options
	for _, opt := range opts {
		opt(user)
	}

	err := tdf.userRepo.CreateUser(context.Background(), user)
	if err != nil {
		return nil, fmt.Errorf("failed to create user via repository: %w", err)
	}

	return user, nil
}

// CreateWorkspace creates a test workspace using the workspace repository
func (tdf *TestDataFactory) CreateWorkspace(opts ...WorkspaceOption) (*domain.Workspace, error) {
	workspace := &domain.Workspace{
		ID:   fmt.Sprintf("test%s", uuid.New().String()[:8]), // Keep it under 20 chars
		Name: fmt.Sprintf("Test Workspace %s", uuid.New().String()[:8]),
		Settings: domain.WorkspaceSettings{
			Timezone:  "UTC",
			SecretKey: fmt.Sprintf("test-secret-key-%s", uuid.New().String()[:8]),
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Apply options
	for _, opt := range opts {
		opt(workspace)
	}

	err := tdf.workspaceRepo.Create(context.Background(), workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace via repository: %w", err)
	}

	return workspace, nil
}

// CreateContact creates a test contact using the contact repository
func (tdf *TestDataFactory) CreateContact(workspaceID string, opts ...ContactOption) (*domain.Contact, error) {
	contact := &domain.Contact{
		Email:     fmt.Sprintf("contact-%s@example.com", uuid.New().String()[:8]),
		FirstName: &domain.NullableString{String: "Test", IsNull: false},
		LastName:  &domain.NullableString{String: "Contact", IsNull: false},
		Timezone:  &domain.NullableString{String: "UTC", IsNull: false},
		Language:  &domain.NullableString{String: "en", IsNull: false},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Apply options
	for _, opt := range opts {
		opt(contact)
	}

	// Use UpsertContact since that's the method available in the repository
	_, err := tdf.contactRepo.UpsertContact(context.Background(), workspaceID, contact)
	if err != nil {
		return nil, fmt.Errorf("failed to create contact via repository: %w", err)
	}

	return contact, nil
}

// CreateList creates a test list using the list repository
func (tdf *TestDataFactory) CreateList(workspaceID string, opts ...ListOption) (*domain.List, error) {
	list := &domain.List{
		ID:            fmt.Sprintf("list%s", uuid.New().String()[:8]), // Keep it under 32 chars
		Name:          fmt.Sprintf("Test List %s", uuid.New().String()[:8]),
		IsDoubleOptin: false,
		IsPublic:      false,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	// Apply options
	for _, opt := range opts {
		opt(list)
	}

	err := tdf.listRepo.CreateList(context.Background(), workspaceID, list)
	if err != nil {
		return nil, fmt.Errorf("failed to create list via repository: %w", err)
	}

	return list, nil
}

// CreateTemplate creates a test template using the template repository
func (tdf *TestDataFactory) CreateTemplate(workspaceID string, opts ...TemplateOption) (*domain.Template, error) {
	template := &domain.Template{
		ID:        fmt.Sprintf("tmpl%s", uuid.New().String()[:8]), // Keep it under 32 chars
		Name:      fmt.Sprintf("Test Template %s", uuid.New().String()[:8]),
		Version:   1,
		Channel:   "email",
		Category:  "marketing",
		Email:     createDefaultEmailTemplate(),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Apply options
	for _, opt := range opts {
		opt(template)
	}

	err := tdf.templateRepo.CreateTemplate(context.Background(), workspaceID, template)
	if err != nil {
		return nil, fmt.Errorf("failed to create template via repository: %w", err)
	}

	return template, nil
}

// CreateBroadcast creates a test broadcast using the broadcast repository
func (tdf *TestDataFactory) CreateBroadcast(workspaceID string, opts ...BroadcastOption) (*domain.Broadcast, error) {
	broadcast := &domain.Broadcast{
		ID:           fmt.Sprintf("bc%s", uuid.New().String()[:8]), // Keep it under 32 chars
		WorkspaceID:  workspaceID,
		Name:         fmt.Sprintf("Test Broadcast %s", uuid.New().String()[:8]),
		Status:       domain.BroadcastStatusDraft,
		Audience:     createDefaultAudience(),
		Schedule:     createDefaultSchedule(),
		TestSettings: createDefaultTestSettings(),
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	// Apply options
	for _, opt := range opts {
		opt(broadcast)
	}

	err := tdf.broadcastRepo.CreateBroadcast(context.Background(), broadcast)
	if err != nil {
		return nil, fmt.Errorf("failed to create broadcast via repository: %w", err)
	}

	return broadcast, nil
}

// CreateSegment creates a test segment using direct DB insert
func (tdf *TestDataFactory) CreateSegment(workspaceID string) (*domain.Segment, error) {
	segmentID := fmt.Sprintf("seg%s", uuid.New().String()[:8])
	now := time.Now().UTC()
	sql := "SELECT email FROM contacts WHERE 1=1"

	segment := &domain.Segment{
		ID:    segmentID,
		Name:  fmt.Sprintf("Test Segment %s", uuid.New().String()[:8]),
		Color: "#FF5733",
		Tree: &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "email",
							FieldType:    "string",
							Operator:     "is_set",
							StringValues: []string{},
						},
					},
				},
			},
		},
		Timezone:      "UTC",
		Version:       1,
		Status:        string(domain.SegmentStatusActive),
		GeneratedSQL:  &sql,
		GeneratedArgs: domain.JSONArray{},
		DBCreatedAt:   now,
		DBUpdatedAt:   now,
		UsersCount:    0,
	}

	// Get workspace database connection
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace database: %w", err)
	}

	// Convert Tree to JSONB
	treeMap, err := segment.Tree.ToMapOfAny()
	if err != nil {
		return nil, fmt.Errorf("failed to convert tree to map: %w", err)
	}
	treeJSON, err := json.Marshal(treeMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tree: %w", err)
	}

	argsJSON, err := json.Marshal(segment.GeneratedArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal args: %w", err)
	}

	query := `
		INSERT INTO segments (
			id, name, color, tree, timezone, version, status,
			generated_sql, generated_args, recompute_after, db_created_at, db_updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err = workspaceDB.ExecContext(context.Background(), query,
		segment.ID, segment.Name, segment.Color, treeJSON,
		segment.Timezone, segment.Version, segment.Status,
		segment.GeneratedSQL, argsJSON, nil, segment.DBCreatedAt, segment.DBUpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create segment: %w", err)
	}

	return segment, nil
}

// CreateMessageHistory creates a test message history using the message history repository
func (tdf *TestDataFactory) CreateMessageHistory(workspaceID string, opts ...MessageHistoryOption) (*domain.MessageHistory, error) {
	now := time.Now().UTC()
	message := &domain.MessageHistory{
		ID:              uuid.New().String(),
		ContactEmail:    fmt.Sprintf("contact-%s@example.com", uuid.New().String()[:8]),
		TemplateID:      uuid.New().String(),
		TemplateVersion: 1,
		Channel:         "email",
		MessageData: domain.MessageData{
			Data: map[string]interface{}{
				"subject": "Test Message",
				"body":    "This is a test message",
			},
			Metadata: map[string]interface{}{
				"test": true,
			},
		},
		SentAt:    now,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Apply options
	for _, opt := range opts {
		opt(message)
	}

	// Get workspace to retrieve secret key for encryption
	workspace, err := tdf.workspaceRepo.GetByID(context.Background(), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	err = tdf.messageHistoryRepo.Create(context.Background(), workspaceID, workspace.Settings.SecretKey, message)
	if err != nil {
		return nil, fmt.Errorf("failed to create message history via repository: %w", err)
	}

	return message, nil
}

// CreateContactList creates a test contact list relationship using the repository
func (tdf *TestDataFactory) CreateContactList(workspaceID string, opts ...ContactListOption) (*domain.ContactList, error) {
	contactList := &domain.ContactList{
		Email:     fmt.Sprintf("contact-%s@example.com", uuid.New().String()[:8]),
		ListID:    uuid.New().String(),
		Status:    domain.ContactListStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Apply options
	for _, opt := range opts {
		opt(contactList)
	}

	err := tdf.contactListRepo.AddContactToList(context.Background(), workspaceID, contactList)
	if err != nil {
		return nil, fmt.Errorf("failed to create contact list via repository: %w", err)
	}

	return contactList, nil
}

// CreateContactTimelineEvent creates a timeline event for a contact
func (tdf *TestDataFactory) CreateContactTimelineEvent(workspaceID, email, kind string, metadata map[string]interface{}) error {
	// Get workspace database connection
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace database: %w", err)
	}

	// Extract entity_id and entity_type from metadata if present
	var entityID *string
	entityType := "message_history" // default
	if metadata != nil {
		if id, ok := metadata["entity_id"].(string); ok && id != "" {
			entityID = &id
		}
		if et, ok := metadata["entity_type"].(string); ok && et != "" {
			entityType = et
		}
	}

	// Serialize metadata to JSON
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Insert timeline event directly into workspace database
	// The table has: email, operation, entity_type, kind, changes, entity_id, created_at
	query := `
		INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, entity_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err = workspaceDB.ExecContext(context.Background(), query, email, "insert", entityType, kind, metadataJSON, entityID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to insert contact timeline event: %w", err)
	}

	return nil
}

// CreateCustomEvent creates a custom event which triggers the timeline event with proper format
// This is used for testing automations with custom_event triggers
func (tdf *TestDataFactory) CreateCustomEvent(workspaceID, email, eventName string, properties map[string]interface{}) error {
	// Get workspace database connection
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace database: %w", err)
	}

	// Serialize properties to JSON
	propsJSON := []byte("{}")
	if properties != nil {
		propsJSON, err = json.Marshal(properties)
		if err != nil {
			return fmt.Errorf("failed to marshal properties: %w", err)
		}
	}

	// Insert into custom_events table - this fires the trigger that creates the timeline entry
	// with kind = 'custom_event.<event_name>' and entity_type = 'custom_event'
	// Schema: event_name, external_id (PK), email, properties, occurred_at, source
	query := `
		INSERT INTO custom_events (event_name, external_id, email, properties, occurred_at, source)
		VALUES ($1, $2, $3, $4, $5, 'test')
	`

	externalID := fmt.Sprintf("test_%s", uuid.New().String()[:8])
	now := time.Now().UTC()
	_, err = workspaceDB.ExecContext(context.Background(), query, eventName, externalID, email, propsJSON, now)
	if err != nil {
		return fmt.Errorf("failed to insert custom event: %w", err)
	}

	return nil
}

// TimelineEventResult represents a timeline event returned from query
type TimelineEventResult struct {
	ID         string
	Email      string
	Operation  string
	EntityType string
	Kind       string
	EntityID   *string
	Changes    map[string]interface{}
	CreatedAt  time.Time
}

// GetContactTimelineEvents retrieves timeline events for a contact filtered by kind
func (tdf *TestDataFactory) GetContactTimelineEvents(workspaceID, email, kind string) ([]TimelineEventResult, error) {
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace database: %w", err)
	}

	query := `
		SELECT id, email, operation, entity_type, kind, entity_id, changes, created_at
		FROM contact_timeline
		WHERE email = $1 AND kind = $2
		ORDER BY created_at DESC
	`

	rows, err := workspaceDB.QueryContext(context.Background(), query, email, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to query timeline events: %w", err)
	}
	defer rows.Close()

	var results []TimelineEventResult
	for rows.Next() {
		var result TimelineEventResult
		var changesJSON []byte
		err := rows.Scan(&result.ID, &result.Email, &result.Operation, &result.EntityType,
			&result.Kind, &result.EntityID, &changesJSON, &result.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan timeline event: %w", err)
		}

		if len(changesJSON) > 0 {
			if err := json.Unmarshal(changesJSON, &result.Changes); err != nil {
				return nil, fmt.Errorf("failed to unmarshal changes: %w", err)
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// AddUserToWorkspace adds a user to a workspace with the specified role
func (tdf *TestDataFactory) AddUserToWorkspace(userID, workspaceID, role string) error {
	userWorkspace := &domain.UserWorkspace{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        role,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	err := tdf.workspaceRepo.AddUserToWorkspace(context.Background(), userWorkspace)
	if err != nil {
		return fmt.Errorf("failed to add user to workspace: %w", err)
	}

	return nil
}

// AddUserToWorkspaceWithPermissions adds a user to a workspace with the specified role and permissions
func (tdf *TestDataFactory) AddUserToWorkspaceWithPermissions(userID, workspaceID, role string, permissions domain.UserPermissions) error {
	userWorkspace := &domain.UserWorkspace{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        role,
		Permissions: permissions,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	err := tdf.workspaceRepo.AddUserToWorkspace(context.Background(), userWorkspace)
	if err != nil {
		return fmt.Errorf("failed to add user to workspace: %w", err)
	}

	return nil
}

// CreateIntegration creates a test integration using the workspace repository
func (tdf *TestDataFactory) CreateIntegration(workspaceID string, opts ...IntegrationOption) (*domain.Integration, error) {
	integration := &domain.Integration{
		ID:   fmt.Sprintf("integ%s", uuid.New().String()[:8]), // Keep it under 32 chars
		Name: fmt.Sprintf("Test Integration %s", uuid.New().String()[:8]),
		Type: domain.IntegrationTypeEmail,
		EmailProvider: domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("test@example.com", "Test Sender"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025,
				Username: "",
				Password: "",
				UseTLS:   false,
			},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Apply options
	for _, opt := range opts {
		opt(integration)
	}

	// Get workspace and add integration
	workspace, err := tdf.workspaceRepo.GetByID(context.Background(), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	workspace.AddIntegration(*integration)

	// Update workspace with the new integration
	err = tdf.workspaceRepo.Update(context.Background(), workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to update workspace with integration: %w", err)
	}

	return integration, nil
}

// CreateSMTPIntegration creates a test SMTP integration using the workspace repository
func (tdf *TestDataFactory) CreateSMTPIntegration(workspaceID string, opts ...IntegrationOption) (*domain.Integration, error) {
	smtpOpts := []IntegrationOption{
		WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("test@example.com", "Test Sender"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025,
				Username: "",
				Password: "",
				UseTLS:   false,
			},
			RateLimitPerMinute: 25,
		}),
	}

	// Append user-provided options
	smtpOpts = append(smtpOpts, opts...)

	return tdf.CreateIntegration(workspaceID, smtpOpts...)
}

// CreateSESIntegration creates a test SES integration for webhook testing
func (tdf *TestDataFactory) CreateSESIntegration(workspaceID string, opts ...IntegrationOption) (*domain.Integration, error) {
	sesOpts := []IntegrationOption{
		WithIntegrationName("Test SES Integration"),
		WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSES,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("test@example.com", "Test Sender"),
			},
			SES: &domain.AmazonSESSettings{
				Region:    "us-east-1",
				AccessKey: "test-key",
				SecretKey: "test-secret",
			},
			RateLimitPerMinute: 25,
		}),
	}

	// Append user-provided options
	sesOpts = append(sesOpts, opts...)

	return tdf.CreateIntegration(workspaceID, sesOpts...)
}

// CreateMailpitSMTPIntegration creates an SMTP integration configured for Mailpit
func (tdf *TestDataFactory) CreateMailpitSMTPIntegration(workspaceID string, opts ...IntegrationOption) (*domain.Integration, error) {
	mailpitOpts := []IntegrationOption{
		WithIntegrationName("Mailpit SMTP"),
		WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Notifuse Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost", // Mailpit SMTP server
				Port:     1025,        // Mailpit SMTP port
				Username: "",          // Mailpit doesn't require auth
				Password: "",
				UseTLS:   false, // Mailpit doesn't use TLS by default
			},
			RateLimitPerMinute: 25,
		}),
	}

	// Append user-provided options
	mailpitOpts = append(mailpitOpts, opts...)

	return tdf.CreateIntegration(workspaceID, mailpitOpts...)
}

// CreateFailingSMTPIntegration creates an SMTP integration that will fail to send emails
// Used for testing circuit breaker behavior
func (tdf *TestDataFactory) CreateFailingSMTPIntegration(workspaceID string, opts ...IntegrationOption) (*domain.Integration, error) {
	failingOpts := []IntegrationOption{
		WithIntegrationName("Failing SMTP"),
		WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Notifuse Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     9999, // Invalid port - no server listening
				Username: "",
				Password: "",
				UseTLS:   false,
			},
			RateLimitPerMinute: 6000, // High rate limit so we don't get throttled
		}),
	}

	// Append user-provided options
	failingOpts = append(failingOpts, opts...)

	return tdf.CreateIntegration(workspaceID, failingOpts...)
}

// SetupWorkspaceWithSMTPProvider creates a workspace with an SMTP email provider and sets it as the marketing and transactional provider
func (tdf *TestDataFactory) SetupWorkspaceWithSMTPProvider(workspaceID string, opts ...IntegrationOption) (*domain.Integration, error) {
	// Create Mailpit SMTP integration
	integration, err := tdf.CreateMailpitSMTPIntegration(workspaceID, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create SMTP integration: %w", err)
	}

	// Get workspace and update settings to use this integration as marketing and transactional provider
	workspace, err := tdf.workspaceRepo.GetByID(context.Background(), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	// Set the integration as the marketing email provider
	workspace.Settings.MarketingEmailProviderID = integration.ID

	// Set the integration as the transactional email provider
	workspace.Settings.TransactionalEmailProviderID = integration.ID

	// Update workspace
	err = tdf.workspaceRepo.Update(context.Background(), workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to update workspace settings: %w", err)
	}

	return integration, nil
}

// Option types for customizing test data
type UserOption func(*domain.User)
type WorkspaceOption func(*domain.Workspace)
type ContactOption func(*domain.Contact)
type ListOption func(*domain.List)
type TemplateOption func(*domain.Template)
type BroadcastOption func(*domain.Broadcast)
type MessageHistoryOption func(*domain.MessageHistory)
type ContactListOption func(*domain.ContactList)
type IntegrationOption func(*domain.Integration)

// User options
func WithUserEmail(email string) UserOption {
	return func(u *domain.User) {
		u.Email = email
	}
}

func WithUserName(name string) UserOption {
	return func(u *domain.User) {
		u.Name = name
	}
}

func WithUserType(userType domain.UserType) UserOption {
	return func(u *domain.User) {
		u.Type = userType
	}
}

// Workspace options
func WithWorkspaceName(name string) WorkspaceOption {
	return func(w *domain.Workspace) {
		w.Name = name
	}
}

func WithWorkspaceSettings(settings domain.WorkspaceSettings) WorkspaceOption {
	return func(w *domain.Workspace) {
		w.Settings = settings
	}
}

func WithCustomDomain(customDomain string) WorkspaceOption {
	return func(w *domain.Workspace) {
		w.Settings.CustomEndpointURL = &customDomain
	}
}

func WithBlogEnabled(enabled bool) WorkspaceOption {
	return func(w *domain.Workspace) {
		w.Settings.BlogEnabled = enabled
	}
}

// Contact options
func WithContactEmail(email string) ContactOption {
	return func(c *domain.Contact) {
		c.Email = email
	}
}

func WithContactName(firstName, lastName string) ContactOption {
	return func(c *domain.Contact) {
		c.FirstName = &domain.NullableString{String: firstName, IsNull: false}
		c.LastName = &domain.NullableString{String: lastName, IsNull: false}
	}
}

func WithContactExternalID(externalID string) ContactOption {
	return func(c *domain.Contact) {
		c.ExternalID = &domain.NullableString{String: externalID, IsNull: false}
	}
}

func WithContactCountry(country string) ContactOption {
	return func(c *domain.Contact) {
		c.Country = &domain.NullableString{String: country, IsNull: false}
	}
}

func WithContactCustomNumber1(value float64) ContactOption {
	return func(c *domain.Contact) {
		c.CustomNumber1 = &domain.NullableFloat64{Float64: value, IsNull: false}
	}
}

func WithContactPhone(phone string) ContactOption {
	return func(c *domain.Contact) {
		c.Phone = &domain.NullableString{String: phone, IsNull: false}
	}
}

func WithContactTimezone(timezone string) ContactOption {
	return func(c *domain.Contact) {
		c.Timezone = &domain.NullableString{String: timezone, IsNull: false}
	}
}

func WithContactCustomString1(value string) ContactOption {
	return func(c *domain.Contact) {
		c.CustomString1 = &domain.NullableString{String: value, IsNull: false}
	}
}

// List options
func WithListName(name string) ListOption {
	return func(l *domain.List) {
		l.Name = name
	}
}

func WithListDoubleOptin(enabled bool) ListOption {
	return func(l *domain.List) {
		l.IsDoubleOptin = enabled
	}
}

func WithListPublic(enabled bool) ListOption {
	return func(l *domain.List) {
		l.IsPublic = enabled
	}
}

// Template options
func WithTemplateName(name string) TemplateOption {
	return func(t *domain.Template) {
		t.Name = name
	}
}

func WithTemplateCategory(category string) TemplateOption {
	return func(t *domain.Template) {
		t.Category = category
	}
}

func WithTemplateSubject(subject string) TemplateOption {
	return func(t *domain.Template) {
		if t.Email != nil {
			t.Email.Subject = subject
		}
	}
}

// WithTemplateEmailContent sets the text content in the email template's mj-text block
// This is useful for testing Liquid template variable substitution
func WithTemplateEmailContent(content string) TemplateOption {
	return func(t *domain.Template) {
		if t.Email != nil {
			t.Email.VisualEditorTree = createMJMLBlockWithContent(content)
		}
	}
}

// Broadcast options
func WithBroadcastName(name string) BroadcastOption {
	return func(b *domain.Broadcast) {
		b.Name = name
	}
}

func WithBroadcastStatus(status domain.BroadcastStatus) BroadcastOption {
	return func(b *domain.Broadcast) {
		b.Status = status
	}
}

func WithBroadcastABTesting(templateIDs []string) BroadcastOption {
	return func(b *domain.Broadcast) {
		if len(templateIDs) >= 2 {
			b.TestSettings.Enabled = true
			b.TestSettings.SamplePercentage = 50
			b.TestSettings.AutoSendWinner = true
			b.TestSettings.AutoSendWinnerMetric = "open_rate"
			b.TestSettings.TestDurationHours = 24
			// Create variations for the templates
			variations := make([]domain.BroadcastVariation, len(templateIDs))
			for i, templateID := range templateIDs {
				variations[i] = domain.BroadcastVariation{
					VariationName: fmt.Sprintf("Version %c", 'A'+i),
					TemplateID:    templateID,
				}
			}
			b.TestSettings.Variations = variations
		}
	}
}

func WithBroadcastAudience(audience domain.AudienceSettings) BroadcastOption {
	return func(b *domain.Broadcast) {
		b.Audience = audience
	}
}

// WithBroadcastGlobalFeed sets the global feed settings for a broadcast
func WithBroadcastGlobalFeed(settings *domain.GlobalFeedSettings) BroadcastOption {
	return func(b *domain.Broadcast) {
		if b.DataFeed == nil {
			b.DataFeed = &domain.DataFeedSettings{}
		}
		b.DataFeed.GlobalFeed = settings
	}
}

// WithBroadcastRecipientFeed sets the recipient feed settings for a broadcast
func WithBroadcastRecipientFeed(settings *domain.RecipientFeedSettings) BroadcastOption {
	return func(b *domain.Broadcast) {
		if b.DataFeed == nil {
			b.DataFeed = &domain.DataFeedSettings{}
		}
		b.DataFeed.RecipientFeed = settings
	}
}

// WithBroadcastGlobalFeedData sets pre-populated global feed data for a broadcast
func WithBroadcastGlobalFeedData(data map[string]interface{}, fetchedAt *time.Time) BroadcastOption {
	return func(b *domain.Broadcast) {
		if b.DataFeed == nil {
			b.DataFeed = &domain.DataFeedSettings{}
		}
		b.DataFeed.GlobalFeedData = data
		b.DataFeed.GlobalFeedFetchedAt = fetchedAt
	}
}

// WithBroadcastTemplateID sets the template ID on the default (first) variation
func WithBroadcastTemplateID(templateID string) BroadcastOption {
	return func(b *domain.Broadcast) {
		if len(b.TestSettings.Variations) > 0 {
			b.TestSettings.Variations[0].TemplateID = templateID
		}
	}
}

// Message history options
func WithMessageHistoryContactEmail(email string) MessageHistoryOption {
	return func(m *domain.MessageHistory) {
		m.ContactEmail = email
	}
}

func WithMessageHistoryTemplateID(templateID string) MessageHistoryOption {
	return func(m *domain.MessageHistory) {
		m.TemplateID = templateID
	}
}

func WithMessageHistoryTemplateVersion(version int64) MessageHistoryOption {
	return func(m *domain.MessageHistory) {
		m.TemplateVersion = version
	}
}

func WithMessageHistoryChannel(channel string) MessageHistoryOption {
	return func(m *domain.MessageHistory) {
		m.Channel = channel
	}
}

// ContactList options
func WithContactListEmail(email string) ContactListOption {
	return func(cl *domain.ContactList) {
		cl.Email = email
	}
}

func WithContactListListID(listID string) ContactListOption {
	return func(cl *domain.ContactList) {
		cl.ListID = listID
	}
}

func WithContactListStatus(status domain.ContactListStatus) ContactListOption {
	return func(cl *domain.ContactList) {
		cl.Status = status
	}
}

// Convenience aliases for cleaner test code
func WithMessageContact(email string) MessageHistoryOption {
	return WithMessageHistoryContactEmail(email)
}

func WithMessageTemplate(templateID string) MessageHistoryOption {
	return WithMessageHistoryTemplateID(templateID)
}

func WithMessageChannel(channel string) MessageHistoryOption {
	return WithMessageHistoryChannel(channel)
}

func WithMessageBroadcast(broadcastID string) MessageHistoryOption {
	return func(m *domain.MessageHistory) {
		m.BroadcastID = &broadcastID
	}
}

func WithMessageSentAt(sentAt time.Time) MessageHistoryOption {
	return func(m *domain.MessageHistory) {
		m.SentAt = sentAt
	}
}

func WithMessageDelivered(delivered bool) MessageHistoryOption {
	return func(m *domain.MessageHistory) {
		if delivered {
			now := time.Now().UTC()
			m.DeliveredAt = &now
		} else {
			m.DeliveredAt = nil
		}
	}
}

func WithMessageOpened(opened bool) MessageHistoryOption {
	return func(m *domain.MessageHistory) {
		if opened {
			now := time.Now().UTC()
			m.OpenedAt = &now
			// If opened, also mark as delivered
			if m.DeliveredAt == nil {
				m.DeliveredAt = &now
			}
		} else {
			m.OpenedAt = nil
		}
	}
}

func WithMessageClicked(clicked bool) MessageHistoryOption {
	return func(m *domain.MessageHistory) {
		if clicked {
			now := time.Now().UTC()
			m.ClickedAt = &now
			// If clicked, also mark as opened and delivered
			if m.OpenedAt == nil {
				m.OpenedAt = &now
			}
			if m.DeliveredAt == nil {
				m.DeliveredAt = &now
			}
		} else {
			m.ClickedAt = nil
		}
	}
}

func WithMessageFailed(failed bool) MessageHistoryOption {
	return func(m *domain.MessageHistory) {
		if failed {
			now := time.Now().UTC()
			m.FailedAt = &now
		} else {
			m.FailedAt = nil
		}
	}
}

func WithMessageBounced(bounced bool) MessageHistoryOption {
	return func(m *domain.MessageHistory) {
		if bounced {
			now := time.Now().UTC()
			m.BouncedAt = &now
		} else {
			m.BouncedAt = nil
		}
	}
}

func WithMessageListID(listID string) MessageHistoryOption {
	return func(m *domain.MessageHistory) {
		if listID != "" {
			m.ListID = &listID
		} else {
			m.ListID = nil
		}
	}
}

func WithMessageID(id string) MessageHistoryOption {
	return func(m *domain.MessageHistory) {
		m.ID = id
	}
}

// Integration options
func WithIntegrationName(name string) IntegrationOption {
	return func(integration *domain.Integration) {
		integration.Name = name
	}
}

func WithIntegrationType(integrationType domain.IntegrationType) IntegrationOption {
	return func(integration *domain.Integration) {
		integration.Type = integrationType
	}
}

func WithIntegrationEmailProvider(emailProvider domain.EmailProvider) IntegrationOption {
	return func(integration *domain.Integration) {
		integration.EmailProvider = emailProvider
	}
}

// Helper functions to create default structures
func createDefaultEmailTemplate() *domain.EmailTemplate {
	return &domain.EmailTemplate{
		Subject:          "Test Email Subject",
		CompiledPreview:  `<mjml><mj-head></mj-head><mj-body><mj-section><mj-column><mj-text>Hello Test!</mj-text></mj-column></mj-section></mj-body></mjml>`,
		VisualEditorTree: createDefaultMJMLBlock(),
	}
}

func createDefaultMJMLBlock() notifuse_mjml.EmailBlock {
	// Create a simple MJML structure using BaseBlock with proper JSON structure
	// Create a map structure instead of using specific block types to avoid marshaling issues
	textBlockMap := map[string]interface{}{
		"id":      "text-1",
		"type":    "mj-text",
		"content": "Hello Test!",
		"attributes": map[string]interface{}{
			"color":    "#000000",
			"fontSize": "14px",
		},
		"children": []interface{}{},
	}

	columnBlockMap := map[string]interface{}{
		"id":       "column-1",
		"type":     "mj-column",
		"children": []interface{}{textBlockMap},
		"attributes": map[string]interface{}{
			"width": "100%",
		},
	}

	sectionBlockMap := map[string]interface{}{
		"id":       "section-1",
		"type":     "mj-section",
		"children": []interface{}{columnBlockMap},
		"attributes": map[string]interface{}{
			"backgroundColor": "#ffffff",
			"padding":         "20px 0",
		},
	}

	bodyBlockMap := map[string]interface{}{
		"id":       "body-1",
		"type":     "mj-body",
		"children": []interface{}{sectionBlockMap},
		"attributes": map[string]interface{}{
			"backgroundColor": "#f4f4f4",
		},
	}

	mjmlBlockMap := map[string]interface{}{
		"id":         "mjml-1",
		"type":       "mjml",
		"children":   []interface{}{bodyBlockMap},
		"attributes": map[string]interface{}{},
	}

	// Convert to JSON and back to create a proper EmailBlock structure
	jsonData, err := json.Marshal(mjmlBlockMap)
	if err != nil {
		panic(err)
	}

	block, err := notifuse_mjml.UnmarshalEmailBlock(jsonData)
	if err != nil {
		panic(err)
	}

	return block
}

// createMJMLBlockWithContent creates an MJML block with custom text content
// This allows testing Liquid template variables in the email body
func createMJMLBlockWithContent(content string) notifuse_mjml.EmailBlock {
	textBlockMap := map[string]interface{}{
		"id":      "text-1",
		"type":    "mj-text",
		"content": content,
		"attributes": map[string]interface{}{
			"color":    "#000000",
			"fontSize": "14px",
		},
		"children": []interface{}{},
	}

	columnBlockMap := map[string]interface{}{
		"id":       "column-1",
		"type":     "mj-column",
		"children": []interface{}{textBlockMap},
		"attributes": map[string]interface{}{
			"width": "100%",
		},
	}

	sectionBlockMap := map[string]interface{}{
		"id":       "section-1",
		"type":     "mj-section",
		"children": []interface{}{columnBlockMap},
		"attributes": map[string]interface{}{
			"backgroundColor": "#ffffff",
			"padding":         "20px 0",
		},
	}

	bodyBlockMap := map[string]interface{}{
		"id":       "body-1",
		"type":     "mj-body",
		"children": []interface{}{sectionBlockMap},
		"attributes": map[string]interface{}{
			"backgroundColor": "#f4f4f4",
		},
	}

	mjmlBlockMap := map[string]interface{}{
		"id":         "mjml-1",
		"type":       "mjml",
		"children":   []interface{}{bodyBlockMap},
		"attributes": map[string]interface{}{},
	}

	jsonData, err := json.Marshal(mjmlBlockMap)
	if err != nil {
		panic(err)
	}

	block, err := notifuse_mjml.UnmarshalEmailBlock(jsonData)
	if err != nil {
		panic(err)
	}

	return block
}

func createDefaultAudience() domain.AudienceSettings {
	return domain.AudienceSettings{
		ExcludeUnsubscribed: true,
	}
}

func createDefaultSchedule() domain.ScheduleSettings {
	return domain.ScheduleSettings{
		IsScheduled: false,
	}
}

func createDefaultTestSettings() domain.BroadcastTestSettings {
	return domain.BroadcastTestSettings{
		Enabled:          false,
		SamplePercentage: 100,
		Variations: []domain.BroadcastVariation{
			{
				VariationName: "Default",
				TemplateID:    "",
			},
		},
	}
}

// TaskOption defines options for creating tasks
type TaskOption func(*domain.Task)

// WithTaskType sets the task type
func WithTaskType(taskType string) TaskOption {
	return func(t *domain.Task) {
		t.Type = taskType
	}
}

// WithTaskStatus sets the task status
func WithTaskStatus(status domain.TaskStatus) TaskOption {
	return func(t *domain.Task) {
		t.Status = status
	}
}

// WithTaskProgress sets the task progress
func WithTaskProgress(progress float64) TaskOption {
	return func(t *domain.Task) {
		t.Progress = progress
	}
}

// WithTaskState sets the task state
func WithTaskState(state *domain.TaskState) TaskOption {
	return func(t *domain.Task) {
		t.State = state
	}
}

// WithTaskBroadcastID sets the broadcast ID for the task
func WithTaskBroadcastID(broadcastID string) TaskOption {
	return func(t *domain.Task) {
		t.BroadcastID = &broadcastID
	}
}

// WithTaskMaxRetries sets the max retries for the task
func WithTaskMaxRetries(maxRetries int) TaskOption {
	return func(t *domain.Task) {
		t.MaxRetries = maxRetries
	}
}

// WithTaskRetryInterval sets the retry interval for the task
func WithTaskRetryInterval(retryInterval int) TaskOption {
	return func(t *domain.Task) {
		t.RetryInterval = retryInterval
	}
}

// WithTaskMaxRuntime sets the max runtime for the task
func WithTaskMaxRuntime(maxRuntime int) TaskOption {
	return func(t *domain.Task) {
		t.MaxRuntime = maxRuntime
	}
}

// WithTaskNextRunAfter sets when the task should run next
func WithTaskNextRunAfter(nextRunAfter time.Time) TaskOption {
	return func(t *domain.Task) {
		t.NextRunAfter = &nextRunAfter
	}
}

// WithTaskErrorMessage sets the error message for the task
func WithTaskErrorMessage(errorMsg string) TaskOption {
	return func(t *domain.Task) {
		t.ErrorMessage = &errorMsg
	}
}

// CreateTask creates a test task with optional configuration
func (tdf *TestDataFactory) CreateTask(workspaceID string, opts ...TaskOption) (*domain.Task, error) {
	// Create default task
	task := &domain.Task{
		ID:            uuid.New().String(),
		WorkspaceID:   workspaceID,
		Type:          "test_task",
		Status:        domain.TaskStatusPending,
		Progress:      0.0,
		State:         &domain.TaskState{},
		MaxRuntime:    50, // 50 seconds
		MaxRetries:    3,
		RetryInterval: 300, // 5 minutes
		RetryCount:    0,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	// Apply options
	for _, opt := range opts {
		opt(task)
	}

	// Create task in database using domain service
	taskRepo := repository.NewTaskRepository(tdf.db)
	err := taskRepo.Create(context.Background(), workspaceID, task)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	return task, nil
}

// CreateSendBroadcastTask creates a task specifically for sending broadcasts
func (tdf *TestDataFactory) CreateSendBroadcastTask(workspaceID, broadcastID string, opts ...TaskOption) (*domain.Task, error) {
	// Create send broadcast state
	state := &domain.TaskState{
		SendBroadcast: &domain.SendBroadcastState{
			BroadcastID:     broadcastID,
			TotalRecipients: 100,
			EnqueuedCount:   0,
			FailedCount:     0,
			ChannelType:     "email",
			RecipientOffset: 0,
			Phase:           "single",
		},
	}

	// Default options for send broadcast task
	defaultOpts := []TaskOption{
		WithTaskType("send_broadcast"),
		WithTaskState(state),
		WithTaskBroadcastID(broadcastID),
		WithTaskMaxRuntime(50), // 50 seconds for broadcast tasks
	}

	// Combine default options with provided options
	allOpts := append(defaultOpts, opts...)

	return tdf.CreateTask(workspaceID, allOpts...)
}

// CreateTaskWithABTesting creates a task for A/B testing broadcasts
func (tdf *TestDataFactory) CreateTaskWithABTesting(workspaceID, broadcastID string, opts ...TaskOption) (*domain.Task, error) {
	// Create A/B testing state
	state := &domain.TaskState{
		SendBroadcast: &domain.SendBroadcastState{
			BroadcastID:               broadcastID,
			TotalRecipients:           1000,
			EnqueuedCount:             0,
			FailedCount:               0,
			ChannelType:               "email",
			RecipientOffset:           0,
			Phase:                     "test",
			TestPhaseCompleted:        false,
			TestPhaseRecipientCount:   100, // 10% for A/B testing
			WinnerPhaseRecipientCount: 900, // 90% for winner
		},
	}

	// Default options for A/B testing task
	defaultOpts := []TaskOption{
		WithTaskType("send_broadcast"),
		WithTaskState(state),
		WithTaskBroadcastID(broadcastID),
		WithTaskMaxRuntime(50), // 50 seconds for A/B testing tasks
	}

	// Combine default options with provided options
	allOpts := append(defaultOpts, opts...)

	return tdf.CreateTask(workspaceID, allOpts...)
}

// UpdateTaskState updates a task's state and progress
func (tdf *TestDataFactory) UpdateTaskState(workspaceID, taskID string, progress float64, state *domain.TaskState) error {
	taskRepo := repository.NewTaskRepository(tdf.db)
	return taskRepo.SaveState(context.Background(), workspaceID, taskID, progress, state)
}

// MarkTaskAsRunning marks a task as running with a timeout
func (tdf *TestDataFactory) MarkTaskAsRunning(workspaceID, taskID string) error {
	taskRepo := repository.NewTaskRepository(tdf.db)
	timeoutAfter := time.Now().Add(5 * time.Minute)
	return taskRepo.MarkAsRunning(context.Background(), workspaceID, taskID, timeoutAfter)
}

// MarkTaskAsCompleted marks a task as completed with the final state
func (tdf *TestDataFactory) MarkTaskAsCompleted(workspaceID, taskID string, state *domain.TaskState) error {
	taskRepo := repository.NewTaskRepository(tdf.db)
	return taskRepo.MarkAsCompleted(context.Background(), workspaceID, taskID, state)
}

// MarkTaskAsFailed marks a task as failed with an error message
func (tdf *TestDataFactory) MarkTaskAsFailed(workspaceID, taskID string, errorMsg string) error {
	taskRepo := repository.NewTaskRepository(tdf.db)
	return taskRepo.MarkAsFailed(context.Background(), workspaceID, taskID, errorMsg)
}

// MarkTaskAsPaused marks a task as paused with next run time
func (tdf *TestDataFactory) MarkTaskAsPaused(workspaceID, taskID string, nextRunAfter time.Time, progress float64, state *domain.TaskState) error {
	taskRepo := repository.NewTaskRepository(tdf.db)
	return taskRepo.MarkAsPaused(context.Background(), workspaceID, taskID, nextRunAfter, progress, state)
}

// UpdateTaskMaxRuntime updates a task's max_runtime value for testing timeout behavior
func (tdf *TestDataFactory) UpdateTaskMaxRuntime(workspaceID, taskID string, maxRuntime int) error {
	query := `UPDATE tasks SET max_runtime = $1 WHERE workspace_id = $2 AND id = $3`
	_, err := tdf.db.ExecContext(context.Background(), query, maxRuntime, workspaceID, taskID)
	return err
}

// CreateTransactionalNotification creates a test transactional notification using the repository
func (tdf *TestDataFactory) CreateTransactionalNotification(workspaceID string, opts ...TransactionalNotificationOption) (*domain.TransactionalNotification, error) {
	channels := domain.ChannelTemplates{
		domain.TransactionalChannelEmail: domain.ChannelTemplate{
			TemplateID: fmt.Sprintf("tmpl%s", uuid.New().String()[:8]),
			Settings:   map[string]interface{}{},
		},
	}

	notification := &domain.TransactionalNotification{
		ID:          fmt.Sprintf("txn%s", uuid.New().String()[:8]),
		Name:        fmt.Sprintf("Test Transactional %s", uuid.New().String()[:8]),
		Description: "Test transactional notification",
		Channels:    channels,
		TrackingSettings: notifuse_mjml.TrackingSettings{
			EnableTracking: true,
		},
		Metadata:  map[string]interface{}{},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Apply options
	for _, opt := range opts {
		opt(notification)
	}

	err := tdf.transactionalNotificationRepo.Create(context.Background(), workspaceID, notification)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactional notification via repository: %w", err)
	}

	return notification, nil
}

type TransactionalNotificationOption func(*domain.TransactionalNotification)

// TransactionalNotification option functions
func WithTransactionalNotificationName(name string) TransactionalNotificationOption {
	return func(tn *domain.TransactionalNotification) {
		tn.Name = name
	}
}

func WithTransactionalNotificationID(id string) TransactionalNotificationOption {
	return func(tn *domain.TransactionalNotification) {
		tn.ID = id
	}
}

func WithTransactionalNotificationDescription(description string) TransactionalNotificationOption {
	return func(tn *domain.TransactionalNotification) {
		tn.Description = description
	}
}

func WithTransactionalNotificationChannels(channels domain.ChannelTemplates) TransactionalNotificationOption {
	return func(tn *domain.TransactionalNotification) {
		tn.Channels = channels
	}
}

func WithTransactionalNotificationMetadata(metadata map[string]interface{}) TransactionalNotificationOption {
	return func(tn *domain.TransactionalNotification) {
		tn.Metadata = metadata
	}
}

// WithNotificationTemplateID sets the template ID for email channel
func WithNotificationTemplateID(templateID string) TransactionalNotificationOption {
	return func(n *domain.TransactionalNotification) {
		if config, exists := n.Channels[domain.TransactionalChannelEmail]; exists {
			config.TemplateID = templateID
			n.Channels[domain.TransactionalChannelEmail] = config
		}
	}
}

// WithNotificationID is an alias for WithTransactionalNotificationID for consistency
func WithNotificationID(id string) TransactionalNotificationOption {
	return WithTransactionalNotificationID(id)
}

// CreateAPIKey creates an API key user for a workspace
func (tdf *TestDataFactory) CreateAPIKey(workspaceID string, opts ...UserOption) (*domain.User, error) {
	apiUser := &domain.User{
		ID:        uuid.New().String(),
		Email:     fmt.Sprintf("api-%s@example.com", uuid.New().String()[:8]),
		Name:      "API Key User",
		Type:      domain.UserTypeAPIKey,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Apply options
	for _, opt := range opts {
		opt(apiUser)
	}

	err := tdf.userRepo.CreateUser(context.Background(), apiUser)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key user: %w", err)
	}

	// Add API user to workspace
	userWorkspace := &domain.UserWorkspace{
		UserID:      apiUser.ID,
		WorkspaceID: workspaceID,
		Role:        "member",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	err = tdf.workspaceRepo.AddUserToWorkspace(context.Background(), userWorkspace)
	if err != nil {
		return nil, fmt.Errorf("failed to add API user to workspace: %w", err)
	}

	return apiUser, nil
}

// CleanupWorkspace removes a workspace and its database from the connection pool
func (tdf *TestDataFactory) CleanupWorkspace(workspaceID string) error {
	pool := GetGlobalTestPool()
	return pool.CleanupWorkspace(workspaceID)
}

// GetConnectionCount returns the current number of active connections in the pool
func (tdf *TestDataFactory) GetConnectionCount() int {
	pool := GetGlobalTestPool()
	return pool.GetConnectionCount()
}

// SetSegmentRecomputeAfter sets the recompute_after timestamp for a segment (for testing)
func (tdf *TestDataFactory) SetSegmentRecomputeAfter(workspaceID, segmentID string, recomputeAfter time.Time) error {
	// Get workspace database connection
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace DB: %w", err)
	}

	// Update the segment's recompute_after field
	query := `UPDATE segments SET recompute_after = $1, db_updated_at = CURRENT_TIMESTAMP WHERE id = $2`
	_, err = workspaceDB.ExecContext(context.Background(), query, recomputeAfter, segmentID)
	if err != nil {
		return fmt.Errorf("failed to set recompute_after: %w", err)
	}

	return nil
}

// EnsureSegmentRecomputeTask ensures the check_segment_recompute task exists for a workspace (for testing)
func (tdf *TestDataFactory) EnsureSegmentRecomputeTask(workspaceID string) error {
	ctx := context.Background()

	// Check if task already exists
	existing, err := tdf.db.QueryContext(ctx,
		`SELECT id FROM tasks WHERE workspace_id = $1 AND type = 'check_segment_recompute' LIMIT 1`,
		workspaceID)
	if err != nil {
		return fmt.Errorf("failed to check for existing task: %w", err)
	}
	defer existing.Close()

	if existing.Next() {
		// Task already exists
		return nil
	}

	// Create the task
	taskID := uuid.New().String()
	state := map[string]interface{}{
		"message": "Check segments for daily recompute",
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	now := time.Now().UTC()
	query := `
		INSERT INTO tasks (
			id, workspace_id, type, status, progress, state,
			created_at, updated_at, next_run_after,
			max_runtime, max_retries, retry_count, retry_interval
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		)
	`
	_, err = tdf.db.ExecContext(ctx, query,
		taskID,
		workspaceID,
		"check_segment_recompute",
		"pending",
		0.0,
		stateJSON,
		now,
		now,
		now, // next_run_after - run immediately
		50,  // max_runtime in seconds
		3,   // max_retries
		0,   // retry_count
		60,  // retry_interval in seconds
	)
	if err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	return nil
}

// Blog Factory Methods

// CreateBlogCategory creates a test blog category using the blog repository
func (tdf *TestDataFactory) CreateBlogCategory(workspaceID string, opts ...BlogCategoryOption) (*domain.BlogCategory, error) {
	category := &domain.BlogCategory{
		ID:   uuid.New().String(),
		Slug: fmt.Sprintf("category-%s", uuid.New().String()[:8]),
		Settings: domain.BlogCategorySettings{
			Name:        fmt.Sprintf("Test Category %s", uuid.New().String()[:8]),
			Description: "Test category description",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Apply options
	for _, opt := range opts {
		opt(category)
	}

	// Create blog category repository
	categoryRepo := repository.NewBlogCategoryRepository(tdf.workspaceRepo)

	// Create context with workspace ID
	ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, workspaceID)

	// Create category
	err := categoryRepo.CreateCategory(ctx, category)
	if err != nil {
		return nil, fmt.Errorf("failed to create blog category: %w", err)
	}

	return category, nil
}

// CreateBlogPost creates a test blog post using the blog repository
func (tdf *TestDataFactory) CreateBlogPost(workspaceID, categoryID string, opts ...BlogPostOption) (*domain.BlogPost, error) {
	// Create a default template for the post if needed
	template, err := tdf.CreateTemplate(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create template for blog post: %w", err)
	}

	post := &domain.BlogPost{
		ID:         uuid.New().String(),
		CategoryID: categoryID,
		Slug:       fmt.Sprintf("post-%s", uuid.New().String()[:8]),
		Settings: domain.BlogPostSettings{
			Title: fmt.Sprintf("Test Post %s", uuid.New().String()[:8]),
			Template: domain.BlogPostTemplateReference{
				TemplateID:      template.ID,
				TemplateVersion: 1,
			},
			Excerpt:            "This is a test post excerpt",
			FeaturedImageURL:   "",
			Authors:            []domain.BlogAuthor{{Name: "Test Author"}},
			ReadingTimeMinutes: 5,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Apply options
	for _, opt := range opts {
		opt(post)
	}

	// Create blog post repository
	postRepo := repository.NewBlogPostRepository(tdf.workspaceRepo)

	// Create context with workspace ID
	ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, workspaceID)

	// Create post
	err = postRepo.CreatePost(ctx, post)
	if err != nil {
		return nil, fmt.Errorf("failed to create blog post: %w", err)
	}

	return post, nil
}

// CreateBlogTheme creates a test blog theme using the blog repository
func (tdf *TestDataFactory) CreateBlogTheme(workspaceID string, opts ...BlogThemeOption) (*domain.BlogTheme, error) {
	theme := &domain.BlogTheme{
		Version: 1,
		Files: domain.BlogThemeFiles{
			HomeLiquid:     "<html><body>Home</body></html>",
			CategoryLiquid: "<html><body>Category</body></html>",
			PostLiquid:     "<html><body>Post</body></html>",
			HeaderLiquid:   "<header>Header</header>",
			FooterLiquid:   "<footer>Footer</footer>",
			SharedLiquid:   "{% comment %}Shared{% endcomment %}",
			StylesCSS:      "body { margin: 0; }",
			ScriptsJS:      "console.log('test');",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Apply options
	for _, opt := range opts {
		opt(theme)
	}

	// Create blog theme repository
	themeRepo := repository.NewBlogThemeRepository(tdf.workspaceRepo)

	// Create context with workspace ID
	ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, workspaceID)

	// Create theme
	err := themeRepo.CreateTheme(ctx, theme)
	if err != nil {
		return nil, fmt.Errorf("failed to create blog theme: %w", err)
	}

	return theme, nil
}

// Blog option types
type BlogCategoryOption func(*domain.BlogCategory)
type BlogPostOption func(*domain.BlogPost)
type BlogThemeOption func(*domain.BlogTheme)

// Blog category options
func WithCategoryName(name string) BlogCategoryOption {
	return func(c *domain.BlogCategory) {
		c.Settings.Name = name
	}
}

func WithCategorySlug(slug string) BlogCategoryOption {
	return func(c *domain.BlogCategory) {
		c.Slug = slug
	}
}

func WithCategoryDescription(description string) BlogCategoryOption {
	return func(c *domain.BlogCategory) {
		c.Settings.Description = description
	}
}

// Blog post options
func WithPostTitle(title string) BlogPostOption {
	return func(p *domain.BlogPost) {
		p.Settings.Title = title
	}
}

func WithPostSlug(slug string) BlogPostOption {
	return func(p *domain.BlogPost) {
		p.Slug = slug
	}
}

func WithPostExcerpt(excerpt string) BlogPostOption {
	return func(p *domain.BlogPost) {
		p.Settings.Excerpt = excerpt
	}
}

func WithPostPublished(published bool) BlogPostOption {
	return func(p *domain.BlogPost) {
		if published {
			now := time.Now().UTC()
			p.PublishedAt = &now
		} else {
			p.PublishedAt = nil
		}
	}
}

func WithPostAuthors(authors []domain.BlogAuthor) BlogPostOption {
	return func(p *domain.BlogPost) {
		p.Settings.Authors = authors
	}
}

func WithPostTemplate(templateID string, version int) BlogPostOption {
	return func(p *domain.BlogPost) {
		p.Settings.Template = domain.BlogPostTemplateReference{
			TemplateID:      templateID,
			TemplateVersion: version,
		}
	}
}

// Blog theme options
func WithThemeVersion(version int) BlogThemeOption {
	return func(t *domain.BlogTheme) {
		t.Version = version
	}
}

func WithThemeFiles(files domain.BlogThemeFiles) BlogThemeOption {
	return func(t *domain.BlogTheme) {
		t.Files = files
	}
}

func WithThemePublished(published bool) BlogThemeOption {
	return func(t *domain.BlogTheme) {
		if published {
			now := time.Now().UTC()
			t.PublishedAt = &now
		} else {
			t.PublishedAt = nil
		}
	}
}

// GetWorkspaceDB returns a database connection for the specified workspace
// This is useful for tests that need direct database access to simulate edge cases
func (tdf *TestDataFactory) GetWorkspaceDB(workspaceID string) (*sql.DB, error) {
	return tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
}

// ========================================
// Automation Factory Methods
// ========================================

// AutomationOption defines options for creating automations
type AutomationOption func(*domain.Automation)

// CreateAutomation creates a test automation
func (tdf *TestDataFactory) CreateAutomation(workspaceID string, opts ...AutomationOption) (*domain.Automation, error) {
	automation := &domain.Automation{
		ID:          uuid.New().String(),
		WorkspaceID: workspaceID,
		Name:        fmt.Sprintf("Test Automation %s", uuid.New().String()[:8]),
		Status:      domain.AutomationStatusDraft,
		ListID:      "", // Optional - set via options
		Trigger: &domain.TimelineTriggerConfig{
			EventKind: "contact.created",
			Frequency: domain.TriggerFrequencyOnce,
		},
		RootNodeID: "",                         // Set after creating nodes
		Nodes:      []*domain.AutomationNode{}, // Initialize empty
		Stats: &domain.AutomationStats{
			Enrolled:  0,
			Completed: 0,
			Exited:    0,
			Failed:    0,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Apply options
	for _, opt := range opts {
		opt(automation)
	}

	// Insert into database
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace DB: %w", err)
	}

	triggerJSON, err := json.Marshal(automation.Trigger)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal trigger: %w", err)
	}

	nodesJSON, err := json.Marshal(automation.Nodes)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal nodes: %w", err)
	}

	statsJSON, err := json.Marshal(automation.Stats)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal stats: %w", err)
	}

	query := `
		INSERT INTO automations (id, workspace_id, name, status, list_id, trigger_config, trigger_sql, root_node_id, nodes, stats, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err = workspaceDB.ExecContext(context.Background(), query,
		automation.ID, workspaceID, automation.Name, automation.Status,
		automation.ListID, triggerJSON, automation.TriggerSQL, automation.RootNodeID,
		nodesJSON, statsJSON, automation.CreatedAt, automation.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create automation: %w", err)
	}

	return automation, nil
}

// Automation options
func WithAutomationName(name string) AutomationOption {
	return func(a *domain.Automation) {
		a.Name = name
	}
}

func WithAutomationStatus(status domain.AutomationStatus) AutomationOption {
	return func(a *domain.Automation) {
		a.Status = status
	}
}

func WithAutomationListID(listID string) AutomationOption {
	return func(a *domain.Automation) {
		a.ListID = listID
	}
}

func WithAutomationTrigger(trigger *domain.TimelineTriggerConfig) AutomationOption {
	return func(a *domain.Automation) {
		a.Trigger = trigger
	}
}

func WithAutomationRootNodeID(nodeID string) AutomationOption {
	return func(a *domain.Automation) {
		a.RootNodeID = nodeID
	}
}

func WithAutomationID(id string) AutomationOption {
	return func(a *domain.Automation) {
		a.ID = id
	}
}

func WithAutomationNodes(nodes []*domain.AutomationNode) AutomationOption {
	return func(a *domain.Automation) {
		a.Nodes = nodes
	}
}

// AutomationNodeOption defines options for creating automation nodes
type AutomationNodeOption func(*domain.AutomationNode)

// CreateAutomationNode creates a test automation node by appending to the automation's embedded nodes array
func (tdf *TestDataFactory) CreateAutomationNode(workspaceID string, opts ...AutomationNodeOption) (*domain.AutomationNode, error) {
	node := &domain.AutomationNode{
		ID:           uuid.New().String(),
		AutomationID: "", // Must be set via options
		Type:         domain.NodeTypeTrigger,
		Config:       map[string]interface{}{},
		NextNodeID:   nil,
		Position:     domain.NodePosition{X: 0, Y: 0},
		CreatedAt:    time.Now().UTC(),
	}

	// Apply options
	for _, opt := range opts {
		opt(node)
	}

	if node.AutomationID == "" {
		return nil, fmt.Errorf("automation_id is required")
	}

	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace DB: %w", err)
	}

	// Get current automation to get existing nodes
	var nodesJSON []byte
	err = workspaceDB.QueryRowContext(context.Background(),
		`SELECT nodes FROM automations WHERE id = $1`,
		node.AutomationID).Scan(&nodesJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to get automation: %w", err)
	}

	var nodes []*domain.AutomationNode
	if err := json.Unmarshal(nodesJSON, &nodes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal nodes: %w", err)
	}

	// Append new node
	nodes = append(nodes, node)

	// Update automation with new nodes
	newNodesJSON, err := json.Marshal(nodes)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal nodes: %w", err)
	}

	_, err = workspaceDB.ExecContext(context.Background(),
		`UPDATE automations SET nodes = $1, updated_at = $2 WHERE id = $3`,
		newNodesJSON, time.Now().UTC(), node.AutomationID)
	if err != nil {
		return nil, fmt.Errorf("failed to update automation nodes: %w", err)
	}

	return node, nil
}

// Node options
func WithNodeID(id string) AutomationNodeOption {
	return func(n *domain.AutomationNode) {
		n.ID = id
	}
}

func WithNodeAutomationID(automationID string) AutomationNodeOption {
	return func(n *domain.AutomationNode) {
		n.AutomationID = automationID
	}
}

func WithNodeType(nodeType domain.NodeType) AutomationNodeOption {
	return func(n *domain.AutomationNode) {
		n.Type = nodeType
	}
}

func WithNodeConfig(config map[string]interface{}) AutomationNodeOption {
	return func(n *domain.AutomationNode) {
		n.Config = config
	}
}

func WithNodeNextNodeID(nextNodeID string) AutomationNodeOption {
	return func(n *domain.AutomationNode) {
		n.NextNodeID = &nextNodeID
	}
}

func WithNodePosition(x, y float64) AutomationNodeOption {
	return func(n *domain.AutomationNode) {
		n.Position = domain.NodePosition{X: x, Y: y}
	}
}

// UpdateAutomationNodeNextNodeID updates a node's next_node_id in the automation's embedded nodes array
func (tdf *TestDataFactory) UpdateAutomationNodeNextNodeID(workspaceID, automationID, nodeID, nextNodeID string) error {
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace DB: %w", err)
	}

	// Get current nodes
	var nodesJSON []byte
	err = workspaceDB.QueryRowContext(context.Background(),
		`SELECT nodes FROM automations WHERE id = $1`,
		automationID).Scan(&nodesJSON)
	if err != nil {
		return fmt.Errorf("failed to get automation: %w", err)
	}

	var nodes []*domain.AutomationNode
	if err := json.Unmarshal(nodesJSON, &nodes); err != nil {
		return fmt.Errorf("failed to unmarshal nodes: %w", err)
	}

	// Find and update the node
	found := false
	for _, node := range nodes {
		if node.ID == nodeID {
			node.NextNodeID = &nextNodeID
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("node %s not found in automation %s", nodeID, automationID)
	}

	// Update automation with modified nodes
	newNodesJSON, err := json.Marshal(nodes)
	if err != nil {
		return fmt.Errorf("failed to marshal nodes: %w", err)
	}

	_, err = workspaceDB.ExecContext(context.Background(),
		`UPDATE automations SET nodes = $1, updated_at = $2 WHERE id = $3`,
		newNodesJSON, time.Now().UTC(), automationID)
	if err != nil {
		return fmt.Errorf("failed to update automation nodes: %w", err)
	}

	return nil
}

// UpdateAutomationRootNode updates an automation's root_node_id
func (tdf *TestDataFactory) UpdateAutomationRootNode(workspaceID, automationID, rootNodeID string) error {
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace DB: %w", err)
	}

	query := `UPDATE automations SET root_node_id = $1, updated_at = $2 WHERE id = $3`
	_, err = workspaceDB.ExecContext(context.Background(), query, rootNodeID, time.Now().UTC(), automationID)
	if err != nil {
		return fmt.Errorf("failed to update automation root node: %w", err)
	}

	return nil
}

// ActivateAutomation activates an automation (creates DB trigger)
func (tdf *TestDataFactory) ActivateAutomation(workspaceID, automationID string) error {
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace DB: %w", err)
	}

	// Get automation to build trigger
	var triggerJSON []byte
	var rootNodeID string
	var frequency string

	err = workspaceDB.QueryRowContext(context.Background(),
		`SELECT trigger_config, root_node_id FROM automations WHERE id = $1`,
		automationID,
	).Scan(&triggerJSON, &rootNodeID)
	if err != nil {
		return fmt.Errorf("failed to get automation: %w", err)
	}

	var trigger domain.TimelineTriggerConfig
	if err := json.Unmarshal(triggerJSON, &trigger); err != nil {
		return fmt.Errorf("failed to unmarshal trigger: %w", err)
	}
	frequency = string(trigger.Frequency)

	// Build event kind filter
	eventKindFilter := fmt.Sprintf("NEW.kind = '%s'", trigger.EventKind)

	// Create trigger function (remove hyphens from UUID for valid PostgreSQL identifier)
	// Note: list_id is NOT passed to automation_enroll_contact - it's only for unsubscribe URLs
	safeID := strings.ReplaceAll(automationID, "-", "")
	functionName := fmt.Sprintf("automation_trigger_%s", safeID)
	functionSQL := fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s()
		RETURNS TRIGGER AS $$
		BEGIN
			PERFORM automation_enroll_contact(
				'%s',
				NEW.email,
				'%s',
				'%s'
			);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql
	`, functionName, automationID, rootNodeID, frequency)

	_, err = workspaceDB.ExecContext(context.Background(), functionSQL)
	if err != nil {
		return fmt.Errorf("failed to create trigger function: %w", err)
	}

	// Create trigger
	triggerSQL := fmt.Sprintf(`
		CREATE TRIGGER %s
		AFTER INSERT ON contact_timeline
		FOR EACH ROW
		WHEN (%s)
		EXECUTE FUNCTION %s()
	`, functionName, eventKindFilter, functionName)

	_, err = workspaceDB.ExecContext(context.Background(), triggerSQL)
	if err != nil {
		return fmt.Errorf("failed to create trigger: %w", err)
	}

	// Update automation status to live
	_, err = workspaceDB.ExecContext(context.Background(),
		`UPDATE automations SET status = 'live', updated_at = $1 WHERE id = $2`,
		time.Now().UTC(), automationID,
	)
	if err != nil {
		return fmt.Errorf("failed to update automation status: %w", err)
	}

	return nil
}

// DeactivateAutomation deactivates an automation (drops DB trigger)
func (tdf *TestDataFactory) DeactivateAutomation(workspaceID, automationID string) error {
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace DB: %w", err)
	}

	triggerName := fmt.Sprintf("automation_trigger_%s", automationID)

	// Drop trigger
	_, err = workspaceDB.ExecContext(context.Background(),
		fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON contact_timeline", triggerName))
	if err != nil {
		return fmt.Errorf("failed to drop trigger: %w", err)
	}

	// Drop function
	_, err = workspaceDB.ExecContext(context.Background(),
		fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", triggerName))
	if err != nil {
		return fmt.Errorf("failed to drop function: %w", err)
	}

	// Update status
	_, err = workspaceDB.ExecContext(context.Background(),
		`UPDATE automations SET status = 'paused', updated_at = $1 WHERE id = $2`,
		time.Now().UTC(), automationID,
	)
	if err != nil {
		return fmt.Errorf("failed to update automation status: %w", err)
	}

	return nil
}

// GetContactAutomation retrieves a contact automation record
func (tdf *TestDataFactory) GetContactAutomation(workspaceID, automationID, email string) (*domain.ContactAutomation, error) {
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace DB: %w", err)
	}

	var ca domain.ContactAutomation
	var contextJSON []byte
	var scheduledAt, lastRetryAt sql.NullTime
	var lastError sql.NullString

	err = workspaceDB.QueryRowContext(context.Background(), `
		SELECT id, automation_id, contact_email, current_node_id, status,
		       entered_at, scheduled_at, context, retry_count, last_error, last_retry_at, max_retries
		FROM contact_automations
		WHERE automation_id = $1 AND contact_email = $2
		ORDER BY entered_at DESC
		LIMIT 1
	`, automationID, email).Scan(
		&ca.ID, &ca.AutomationID, &ca.ContactEmail, &ca.CurrentNodeID, &ca.Status,
		&ca.EnteredAt, &scheduledAt, &contextJSON, &ca.RetryCount, &lastError, &lastRetryAt, &ca.MaxRetries,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get contact automation: %w", err)
	}

	if scheduledAt.Valid {
		ca.ScheduledAt = &scheduledAt.Time
	}
	if lastRetryAt.Valid {
		ca.LastRetryAt = &lastRetryAt.Time
	}
	if lastError.Valid {
		ca.LastError = &lastError.String
	}
	if len(contextJSON) > 0 {
		json.Unmarshal(contextJSON, &ca.Context)
	}

	return &ca, nil
}

// GetAllContactAutomations retrieves all contact automation records for an automation
func (tdf *TestDataFactory) GetAllContactAutomations(workspaceID, automationID string) ([]*domain.ContactAutomation, error) {
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace DB: %w", err)
	}

	rows, err := workspaceDB.QueryContext(context.Background(), `
		SELECT id, automation_id, contact_email, current_node_id, status,
		       entered_at, scheduled_at, context, retry_count, last_error, last_retry_at, max_retries
		FROM contact_automations
		WHERE automation_id = $1
		ORDER BY entered_at DESC
	`, automationID)
	if err != nil {
		return nil, fmt.Errorf("failed to query contact automations: %w", err)
	}
	defer rows.Close()

	var results []*domain.ContactAutomation
	for rows.Next() {
		var ca domain.ContactAutomation
		var contextJSON []byte
		var scheduledAt, lastRetryAt sql.NullTime
		var lastError sql.NullString

		err := rows.Scan(
			&ca.ID, &ca.AutomationID, &ca.ContactEmail, &ca.CurrentNodeID, &ca.Status,
			&ca.EnteredAt, &scheduledAt, &contextJSON, &ca.RetryCount, &lastError, &lastRetryAt, &ca.MaxRetries,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan contact automation: %w", err)
		}

		if scheduledAt.Valid {
			ca.ScheduledAt = &scheduledAt.Time
		}
		if lastRetryAt.Valid {
			ca.LastRetryAt = &lastRetryAt.Time
		}
		if lastError.Valid {
			ca.LastError = &lastError.String
		}
		if len(contextJSON) > 0 {
			json.Unmarshal(contextJSON, &ca.Context)
		}

		results = append(results, &ca)
	}

	return results, nil
}

// GetAutomationStats retrieves an automation's stats
func (tdf *TestDataFactory) GetAutomationStats(workspaceID, automationID string) (*domain.AutomationStats, error) {
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace DB: %w", err)
	}

	var statsJSON []byte
	err = workspaceDB.QueryRowContext(context.Background(),
		`SELECT stats FROM automations WHERE id = $1`, automationID,
	).Scan(&statsJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to get automation stats: %w", err)
	}

	var stats domain.AutomationStats
	if err := json.Unmarshal(statsJSON, &stats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal stats: %w", err)
	}

	return &stats, nil
}

// GetNodeExecutions retrieves node executions for a contact automation
func (tdf *TestDataFactory) GetNodeExecutions(workspaceID, contactAutomationID string) ([]*domain.NodeExecution, error) {
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace DB: %w", err)
	}

	rows, err := workspaceDB.QueryContext(context.Background(), `
		SELECT id, contact_automation_id, node_id, node_type, action,
		       entered_at, completed_at, duration_ms, output, error
		FROM automation_node_executions
		WHERE contact_automation_id = $1
		ORDER BY entered_at ASC
	`, contactAutomationID)
	if err != nil {
		return nil, fmt.Errorf("failed to query node executions: %w", err)
	}
	defer rows.Close()

	var results []*domain.NodeExecution
	for rows.Next() {
		var ne domain.NodeExecution
		var completedAt sql.NullTime
		var durationMs sql.NullInt64
		var outputJSON []byte
		var errorMsg sql.NullString

		err := rows.Scan(
			&ne.ID, &ne.ContactAutomationID, &ne.NodeID, &ne.NodeType, &ne.Action,
			&ne.EnteredAt, &completedAt, &durationMs, &outputJSON, &errorMsg,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan node execution: %w", err)
		}

		if completedAt.Valid {
			ne.CompletedAt = &completedAt.Time
		}
		if durationMs.Valid {
			ne.DurationMs = &durationMs.Int64
		}
		if errorMsg.Valid {
			ne.Error = &errorMsg.String
		}
		if len(outputJSON) > 0 {
			json.Unmarshal(outputJSON, &ne.Output)
		}

		results = append(results, &ne)
	}

	return results, nil
}

// UpdateContactAutomationScheduledAt updates scheduled_at for a contact automation (for testing delays)
func (tdf *TestDataFactory) UpdateContactAutomationScheduledAt(workspaceID, contactAutomationID string, scheduledAt time.Time) error {
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace DB: %w", err)
	}

	_, err = workspaceDB.ExecContext(context.Background(),
		`UPDATE contact_automations SET scheduled_at = $1 WHERE id = $2`,
		scheduledAt, contactAutomationID,
	)
	if err != nil {
		return fmt.Errorf("failed to update scheduled_at: %w", err)
	}

	return nil
}

// GetTriggerLogEntry checks if a trigger log entry exists for deduplication
func (tdf *TestDataFactory) GetTriggerLogEntry(workspaceID, automationID, email string) (bool, error) {
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get workspace DB: %w", err)
	}

	var exists bool
	err = workspaceDB.QueryRowContext(context.Background(), `
		SELECT EXISTS(SELECT 1 FROM automation_trigger_log WHERE automation_id = $1 AND contact_email = $2)
	`, automationID, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check trigger log: %w", err)
	}

	return exists, nil
}

// CountContactAutomations counts contact automation records for an automation
func (tdf *TestDataFactory) CountContactAutomations(workspaceID, automationID string) (int, error) {
	workspaceDB, err := tdf.workspaceRepo.GetConnection(context.Background(), workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace DB: %w", err)
	}

	var count int
	err = workspaceDB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM contact_automations WHERE automation_id = $1`,
		automationID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count contact automations: %w", err)
	}

	return count, nil
}

// ========================================
// OAuth2 SMTP Factory Methods
// ========================================

// SMTPOAuth2Config holds configuration for OAuth2 SMTP integration
type SMTPOAuth2Config struct {
	Host         string // SMTP server host
	Port         int    // SMTP server port
	Provider     string // "microsoft" or "google"
	TenantID     string // Microsoft only - Azure tenant ID
	ClientID     string // OAuth2 client ID
	ClientSecret string // OAuth2 client secret
	RefreshToken string // Google only - refresh token
	Username     string // Email address for XOAUTH2
	SenderEmail  string // Sender email address (defaults to Username if empty)
	SenderName   string // Sender display name
}

// WithSMTPOAuth2 creates an integration option for OAuth2 SMTP configuration
func WithSMTPOAuth2(config SMTPOAuth2Config) IntegrationOption {
	return func(integration *domain.Integration) {
		// Use username as sender email if not specified
		senderEmail := config.SenderEmail
		if senderEmail == "" {
			senderEmail = config.Username
		}

		// Default sender name
		senderName := config.SenderName
		if senderName == "" {
			senderName = "OAuth2 Test Sender"
		}

		integration.EmailProvider = domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender(senderEmail, senderName),
			},
			SMTP: &domain.SMTPSettings{
				Host:               config.Host,
				Port:               config.Port,
				Username:           config.Username,
				AuthType:           "oauth2",
				OAuth2Provider:     config.Provider,
				OAuth2TenantID:     config.TenantID,
				OAuth2ClientID:     config.ClientID,
				OAuth2ClientSecret: config.ClientSecret,
				OAuth2RefreshToken: config.RefreshToken,
				UseTLS:             false, // Test servers typically don't use TLS
			},
			RateLimitPerMinute: 100,
		}
	}
}

// CreateMicrosoftOAuth2SMTPIntegration creates an SMTP integration configured for Microsoft OAuth2
func (tdf *TestDataFactory) CreateMicrosoftOAuth2SMTPIntegration(
	workspaceID string,
	host string,
	port int,
	tenantID, clientID, clientSecret, username string,
	opts ...IntegrationOption,
) (*domain.Integration, error) {
	oauth2Opts := []IntegrationOption{
		WithIntegrationName("Microsoft OAuth2 SMTP"),
		WithSMTPOAuth2(SMTPOAuth2Config{
			Host:         host,
			Port:         port,
			Provider:     "microsoft",
			TenantID:     tenantID,
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Username:     username,
		}),
	}

	// Append user-provided options
	oauth2Opts = append(oauth2Opts, opts...)

	return tdf.CreateIntegration(workspaceID, oauth2Opts...)
}

// CreateGoogleOAuth2SMTPIntegration creates an SMTP integration configured for Google OAuth2
func (tdf *TestDataFactory) CreateGoogleOAuth2SMTPIntegration(
	workspaceID string,
	host string,
	port int,
	clientID, clientSecret, refreshToken, username string,
	opts ...IntegrationOption,
) (*domain.Integration, error) {
	oauth2Opts := []IntegrationOption{
		WithIntegrationName("Google OAuth2 SMTP"),
		WithSMTPOAuth2(SMTPOAuth2Config{
			Host:         host,
			Port:         port,
			Provider:     "google",
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RefreshToken: refreshToken,
			Username:     username,
		}),
	}

	// Append user-provided options
	oauth2Opts = append(oauth2Opts, opts...)

	return tdf.CreateIntegration(workspaceID, oauth2Opts...)
}
