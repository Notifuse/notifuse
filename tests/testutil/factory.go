package testutil

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

	err := tdf.messageHistoryRepo.Create(context.Background(), workspaceID, message)
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

	// Serialize metadata to JSON
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Insert timeline event directly into workspace database
	// The table has: email, operation, entity_type, kind, changes, entity_id, created_at
	query := `
		INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err = workspaceDB.ExecContext(context.Background(), query, email, "insert", "message_history", kind, metadataJSON, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to insert contact timeline event: %w", err)
	}

	return nil
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
		}),
	}

	// Append user-provided options
	smtpOpts = append(smtpOpts, opts...)

	return tdf.CreateIntegration(workspaceID, smtpOpts...)
}

// CreateMailhogSMTPIntegration creates an SMTP integration configured for Mailhog
func (tdf *TestDataFactory) CreateMailhogSMTPIntegration(workspaceID string, opts ...IntegrationOption) (*domain.Integration, error) {
	mailhogOpts := []IntegrationOption{
		WithIntegrationName("Mailhog SMTP"),
		WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Notifuse Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost", // Mailhog SMTP server
				Port:     1025,        // Mailhog SMTP port
				Username: "",          // Mailhog doesn't require auth
				Password: "",
				UseTLS:   false, // Mailhog doesn't use TLS by default
			},
		}),
	}

	// Append user-provided options
	mailhogOpts = append(mailhogOpts, opts...)

	return tdf.CreateIntegration(workspaceID, mailhogOpts...)
}

// SetupWorkspaceWithSMTPProvider creates a workspace with an SMTP email provider and sets it as the marketing provider
func (tdf *TestDataFactory) SetupWorkspaceWithSMTPProvider(workspaceID string, opts ...IntegrationOption) (*domain.Integration, error) {
	// Create Mailhog SMTP integration
	integration, err := tdf.CreateMailhogSMTPIntegration(workspaceID, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create SMTP integration: %w", err)
	}

	// Get workspace and update settings to use this integration as marketing provider
	workspace, err := tdf.workspaceRepo.GetByID(context.Background(), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	// Set the integration as the marketing email provider
	workspace.Settings.MarketingEmailProviderID = integration.ID

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

func WithContactLifetimeValue(value float64) ContactOption {
	return func(c *domain.Contact) {
		c.LifetimeValue = &domain.NullableFloat64{Float64: value, IsNull: false}
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

func createDefaultAudience() domain.AudienceSettings {
	return domain.AudienceSettings{
		ExcludeUnsubscribed: true,
		SkipDuplicateEmails: true,
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
		t.ErrorMessage = errorMsg
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
			SentCount:       0,
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
			SentCount:                 0,
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

// MarkTaskAsCompleted marks a task as completed
func (tdf *TestDataFactory) MarkTaskAsCompleted(workspaceID, taskID string) error {
	taskRepo := repository.NewTaskRepository(tdf.db)
	return taskRepo.MarkAsCompleted(context.Background(), workspaceID, taskID)
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
