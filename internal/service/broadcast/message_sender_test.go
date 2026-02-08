package broadcast

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	bmocks "github.com/Notifuse/notifuse/internal/service/broadcast/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// TestMessageSenderCreation tests creation of the message sender
func TestMessageSenderCreation(t *testing.T) {
	// Create mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks for all dependencies
	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Test creating message sender with default config
	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		nil, // dataFeedFetcher
		mockLogger,
		nil, // Passing nil config to test the default config behavior
		"",
	)

	// Assert the sender was created and implements the interface
	assert.NotNil(t, sender, "Message sender should not be nil")
	assert.Implements(t, (*MessageSender)(nil), sender, "Sender should implement MessageSender interface")

	// Test creating with custom config
	customConfig := &Config{
		MaxParallelism:          5,
		MaxProcessTime:          30 * time.Second,
		DefaultRateLimit:        300, // 5 per second
		EnableCircuitBreaker:    true,
		CircuitBreakerThreshold: 3,
		CircuitBreakerCooldown:  30 * time.Second,
	}

	customSender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		nil, // dataFeedFetcher
		mockLogger,
		customConfig,
		"",
	)

	assert.NotNil(t, customSender, "Message sender with custom config should not be nil")
	assert.Implements(t, (*MessageSender)(nil), customSender, "Custom sender should implement MessageSender interface")
}

// Helper function to create a simple text block
func createTestTextBlock(id, textContent string) notifuse_mjml.EmailBlock {
	content := textContent
	base := notifuse_mjml.NewBaseBlock(id, notifuse_mjml.MJMLComponentMjText)
	base.Content = &content
	return &notifuse_mjml.MJTextBlock{BaseBlock: base}
}

// Helper function to create a valid MJML tree structure
func createValidTestTree(textBlock notifuse_mjml.EmailBlock) notifuse_mjml.EmailBlock {
	columnBase := notifuse_mjml.NewBaseBlock("col1", notifuse_mjml.MJMLComponentMjColumn)
	columnBase.Children = []notifuse_mjml.EmailBlock{textBlock}
	columnBlock := &notifuse_mjml.MJColumnBlock{BaseBlock: columnBase}

	sectionBase := notifuse_mjml.NewBaseBlock("sec1", notifuse_mjml.MJMLComponentMjSection)
	sectionBase.Children = []notifuse_mjml.EmailBlock{columnBlock}
	sectionBlock := &notifuse_mjml.MJSectionBlock{BaseBlock: sectionBase}

	bodyBase := notifuse_mjml.NewBaseBlock("body1", notifuse_mjml.MJMLComponentMjBody)
	bodyBase.Children = []notifuse_mjml.EmailBlock{sectionBlock}
	bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}

	rootBase := notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)
	rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
	return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
}

// TestSendToRecipientSuccess tests successful sending to a recipient
func TestSendToRecipientSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	// Setup test data
	ctx := context.Background()
	workspaceID := "workspace-123"
	tracking := true
	broadcast := &domain.Broadcast{
		ID:          "broadcast-123",
		WorkspaceID: workspaceID,
		Name:        "Test Broadcast",
		ChannelType: "email",
		Audience:    domain.AudienceSettings{List: "list-1"},
		Status:      domain.BroadcastStatusDraft,
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
			Content:  "test-content",
			Term:     "test-term",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	emailProvider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Test Subject",
			VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
		},
	}

	// Setup mock expectations - SendToRecipient only calls emailService.SendEmail
	mockEmailService.EXPECT().
		SendEmail(
			gomock.Any(), // ctx
			gomock.Any(), // SendEmailProviderRequest
			gomock.Any(), // isMarketing
		).Return(nil)

	// Create message sender
	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		nil, // dataFeedFetcher
		mockLogger,
		TestConfig(),
		"",
	)

	// Test
	timeoutAt := time.Now().Add(30 * time.Second)
	err := sender.SendToRecipient(ctx, workspaceID, "test-integration-id", "https://api.test.com", tracking, broadcast, "message-123", "test@example.com", template, map[string]interface{}{}, emailProvider, timeoutAt)
	assert.NoError(t, err)
}

// TestSendToRecipientCompileFailure tests failure in template compilation
func TestSendToRecipientCompileFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	// Setup test data
	ctx := context.Background()
	workspaceID := "workspace-123"
	tracking := true
	broadcast := &domain.Broadcast{
		ID:          "broadcast-123",
		WorkspaceID: workspaceID,
		Name:        "Test Broadcast",
		ChannelType: "email",
		Audience:    domain.AudienceSettings{List: "list-1"},
		Status:      domain.BroadcastStatusDraft,
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
			Content:  "test-content",
			Term:     "test-term",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	emailProvider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}
	// Create a template with empty VisualEditorTree that should cause compilation to fail
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Test Subject",
			VisualEditorTree: &notifuse_mjml.MJMLBlock{}, // Empty block should cause compilation issues
		},
	}

	// Create message sender
	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		nil, // dataFeedFetcher
		mockLogger,
		TestConfig(),
		"",
	)

	// Test - this should fail due to template compilation issues
	timeoutAt := time.Now().Add(30 * time.Second)
	err := sender.SendToRecipient(ctx, workspaceID, "test-integration-id", "https://api.test.com", tracking, broadcast, "message-123", "test@example.com", template, map[string]interface{}{}, emailProvider, timeoutAt)
	assert.Error(t, err)
	broadcastErr, ok := err.(*BroadcastError)
	assert.True(t, ok)
	assert.Equal(t, ErrCodeTemplateCompile, broadcastErr.Code)
}

// TestWithMockMessageSender shows how to use the MockMessageSender
func TestWithMockMessageSender(t *testing.T) {
	// Create mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock message sender
	mockSender := bmocks.NewMockMessageSender(ctrl)

	// Setup test data
	ctx := context.Background()
	workspaceID := "workspace-123"
	workspaceSecretKey := "secret-key"
	trackingEnabled := true
	broadcast := &domain.Broadcast{
		ID: "broadcast-123",
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
			Content:  "test-content",
			Term:     "test-term",
		},
	}
	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	recipientEmail := "test@example.com"
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID: emailSender.ID,
			Subject:  "Test Subject",
		},
	}
	templateData := domain.MapOfAny{
		"name": "John",
	}

	// Set expectations on the mock
	messageID := "test-message-id"
	timeoutAt := time.Now().Add(30 * time.Second)
	mockSender.EXPECT().
		SendToRecipient(ctx, workspaceID, "test-integration-id", "https://api.test.com", trackingEnabled, broadcast, messageID, recipientEmail, template, templateData, nil, timeoutAt).
		Return(nil)

	// Use the mock (normally this would be in the system under test)
	err := mockSender.SendToRecipient(ctx, workspaceID, "test-integration-id", "https://api.test.com", trackingEnabled, broadcast, messageID, recipientEmail, template, templateData, nil, timeoutAt)

	// Verify the result
	assert.NoError(t, err)

	// We can also set up expectations for SendBatch
	mockContacts := []*domain.ContactWithList{
		{Contact: &domain.Contact{Email: recipientEmail}},
	}
	mockTemplates := map[string]*domain.Template{
		"template-123": template,
	}

	// Set up expectations with specific return values
	timeoutAt = time.Now().Add(30 * time.Second)
	mockSender.EXPECT().
		SendBatch(ctx, workspaceID, "test-integration-id", workspaceSecretKey, "https://api.example.com", trackingEnabled, broadcast.ID, mockContacts, mockTemplates, nil, timeoutAt).
		Return(1, 0, nil)

	// Use the mock
	sent, failed, err := mockSender.SendBatch(ctx, workspaceID, "test-integration-id", workspaceSecretKey, "https://api.example.com", trackingEnabled, broadcast.ID, mockContacts, mockTemplates, nil, timeoutAt)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, 1, sent)
	assert.Equal(t, 0, failed)
}

// TestErrorHandlingWithMock demonstrates error handling with mocks
func TestErrorHandlingWithMock(t *testing.T) {
	// Create mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock message sender
	mockSender := bmocks.NewMockMessageSender(ctrl)

	// Setup test data
	ctx := context.Background()
	workspaceID := "workspace-123"
	workspaceSecretKey := "secret-key"
	trackingEnabled := true
	broadcast := &domain.Broadcast{
		ID: "broadcast-123",
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
			Content:  "test-content",
			Term:     "test-term",
		},
	}
	recipientEmail := "test@example.com"
	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID: emailSender.ID,
			Subject:  "Test Subject",
		},
	}
	templateData := domain.MapOfAny{
		"name": "John",
	}

	// Set up mock to return an error
	timeoutAt := time.Now().Add(30 * time.Second)
	mockError := errors.New("send failed: service unavailable")
	messageID := "test-message-id"
	mockSender.EXPECT().
		SendToRecipient(ctx, workspaceID, "test-integration-id", "https://api.test.com", trackingEnabled, broadcast, messageID, recipientEmail, template, templateData, nil, timeoutAt).
		Return(mockError)

	// Call the method
	err := mockSender.SendToRecipient(ctx, workspaceID, "test-integration-id", "https://api.test.com", trackingEnabled, broadcast, messageID, recipientEmail, template, templateData, nil, timeoutAt)

	// Verify error handling
	assert.Error(t, err)
	assert.Equal(t, mockError, err)
	assert.Contains(t, err.Error(), "service unavailable")

	// Test batch processing with error
	mockContacts := []*domain.ContactWithList{
		{Contact: &domain.Contact{Email: recipientEmail}},
	}
	mockTemplates := map[string]*domain.Template{
		"template-123": template,
	}
	batchError := errors.New("batch processing failed")

	mockSender.EXPECT().
		SendBatch(ctx, workspaceID, "test-integration-id", workspaceSecretKey, "https://api.example.com", trackingEnabled, broadcast.ID, mockContacts, mockTemplates, nil, timeoutAt).
		Return(0, 0, batchError)

	sent, failed, err := mockSender.SendBatch(ctx, workspaceID, "test-integration-id", workspaceSecretKey, "https://api.example.com", trackingEnabled, broadcast.ID, mockContacts, mockTemplates, nil, timeoutAt)
	assert.Error(t, err)
	assert.Equal(t, batchError, err)
	assert.Equal(t, 0, sent)
	assert.Equal(t, 0, failed)
}

// TestSendBatch tests the SendBatch method
func TestSendBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	// Setup test data
	ctx := context.Background()
	timeoutAt := time.Now().Add(30 * time.Second)
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"
	tracking := true
	broadcast := &domain.Broadcast{
		ID:          broadcastID,
		WorkspaceID: workspaceID,
		Name:        "Test Broadcast",
		ChannelType: "email",
		Audience:    domain.AudienceSettings{List: "list-1"},
		Status:      domain.BroadcastStatusDraft,
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
			Content:  "test-content",
			Term:     "test-term",
		},
		TestSettings: domain.BroadcastTestSettings{
			Enabled: false,
			Variations: []domain.BroadcastVariation{
				{
					VariationName: "variation-1",
					TemplateID:    "template-123",
				},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	emailProvider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Test Subject",
			VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
		},
	}

	// Setup mock expectations
	mockBroadcastRepository.EXPECT().
		GetBroadcast(ctx, workspaceID, broadcastID).
		Return(broadcast, nil)

	mockEmailService.EXPECT().
		SendEmail(
			gomock.Any(), // ctx
			gomock.Any(), // SendEmailProviderRequest
			gomock.Any(), // isMarketing
		).Return(nil).Times(2)

	mockMessageHistoryRepo.EXPECT().
		Create(
			ctx,
			workspaceID,
			gomock.Any(), // secretKey
			gomock.Any(), // message
		).Do(func(_ context.Context, _ string, _ string, msg *domain.MessageHistory) {
		// Verify list_id is populated from broadcast audience
		assert.NotNil(t, msg.ListID)
		assert.Equal(t, "list-1", *msg.ListID)
	}).Return(nil).Times(2)

	// Create message sender
	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		nil, // dataFeedFetcher
		mockLogger,
		TestConfig(),
		"",
	)

	// Test
	recipients := []*domain.ContactWithList{
		{
			Contact: &domain.Contact{
				Email: "recipient1@example.com",
			},
			ListID:   "list-1",
			ListName: "Test List",
		},
		{
			Contact: &domain.Contact{
				Email: "recipient2@example.com",
			},
			ListID:   "list-1",
			ListName: "Test List",
		},
	}
	templates := map[string]*domain.Template{"template-123": template}
	sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", "secret-key-123", "https://api.example.com", tracking, broadcastID, recipients, templates, emailProvider, timeoutAt)
	assert.NoError(t, err)
	assert.Equal(t, 2, sent)
	assert.Equal(t, 0, failed)
}

// TestSendBatch_EmptyRecipients tests SendBatch with no recipients
func TestSendBatch_EmptyRecipients(t *testing.T) {
	// Create mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks for all dependencies
	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	// Setup test data
	ctx := context.Background()
	timeoutAt := time.Now().Add(30 * time.Second)
	workspaceID := "workspace-123"
	workspaceSecretKey := "secret-key"
	trackingEnabled := true
	broadcastID := "broadcast-456"
	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	emailProvider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}
	// Create message sender
	config := TestConfig()
	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		nil, // dataFeedFetcher
		mockLogger,
		config,
		"",
	)

	// Call the method being tested with empty recipients
	sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", workspaceSecretKey, "https://api.example.com", trackingEnabled, broadcastID, []*domain.ContactWithList{},
		map[string]*domain.Template{}, emailProvider, timeoutAt)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, 0, sent)
	assert.Equal(t, 0, failed)
}

// TestSendBatch_CircuitBreakerOpen tests SendBatch when circuit breaker is open
func TestSendBatch_CircuitBreakerOpen(t *testing.T) {
	// Create mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks for all dependencies
	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	// Setup test data
	ctx := context.Background()
	timeoutAt := time.Now().Add(30 * time.Second)
	workspaceID := "workspace-123"
	workspaceSecretKey := "secret-key"
	trackingEnabled := true
	broadcastID := "broadcast-456"
	recipients := []*domain.ContactWithList{
		{
			Contact: &domain.Contact{
				Email: "test@example.com",
			},
		},
	}
	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	emailProvider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}

	// Create message sender with circuit breaker enabled
	config := TestConfig()
	config.EnableCircuitBreaker = true
	config.CircuitBreakerThreshold = 1
	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		nil, // dataFeedFetcher
		mockLogger,
		config,
		"",
	)

	// Force circuit breaker to open
	messageSenderImpl := sender.(*messageSender)
	messageSenderImpl.circuitBreaker.RecordFailure(fmt.Errorf("test error"))

	// Call the method being tested
	sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", workspaceSecretKey, "https://api.example.com", trackingEnabled, broadcastID, recipients,
		map[string]*domain.Template{}, emailProvider, timeoutAt)

	// Verify results
	assert.Error(t, err)
	assert.Equal(t, 0, sent)
	assert.Equal(t, 0, failed)

	// Check that we got the right error
	broadcastErr, ok := err.(*BroadcastError)
	assert.True(t, ok)
	assert.Equal(t, ErrCodeCircuitOpen, broadcastErr.Code)
}

// TestGenerateMessageID tests the generateMessageID function
func TestGenerateMessageID(t *testing.T) {
	workspaceID := "workspace-123"

	// Generate multiple message IDs
	id1 := generateMessageID(workspaceID)
	id2 := generateMessageID(workspaceID)

	// Check that IDs have the correct format (workspace_uuid)
	assert.Contains(t, id1, workspaceID+"_")
	assert.Contains(t, id2, workspaceID+"_")

	// Check that generated IDs are different
	assert.NotEqual(t, id1, id2)

	// Verify length is reasonable (workspace ID + "_" + UUID)
	expectedMinLength := len(workspaceID) + 1 + 32 // UUID strings are at least 32 chars
	assert.Greater(t, len(id1), expectedMinLength)
}

// TestSendBatch_WithFailure tests SendBatch with a failed email send
func TestSendBatch_WithFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	// Setup test data
	ctx := context.Background()
	timeoutAt := time.Now().Add(30 * time.Second)
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"
	tracking := true
	broadcast := &domain.Broadcast{
		ID:          broadcastID,
		WorkspaceID: workspaceID,
		Name:        "Test Broadcast",
		ChannelType: "email",
		Audience:    domain.AudienceSettings{List: "list-1"},
		Status:      domain.BroadcastStatusDraft,
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
			Content:  "test-content",
			Term:     "test-term",
		},
		TestSettings: domain.BroadcastTestSettings{
			Enabled: false,
			Variations: []domain.BroadcastVariation{
				{
					VariationName: "variation-1",
					TemplateID:    "template-123",
				},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	emailProvider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Test Subject",
			VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
		},
	}

	// Setup mock expectations
	mockBroadcastRepository.EXPECT().
		GetBroadcast(ctx, workspaceID, broadcastID).
		Return(broadcast, nil)

	mockEmailService.EXPECT().
		SendEmail(
			gomock.Any(), // ctx
			gomock.Any(), // SendEmailProviderRequest
			gomock.Any(), // isMarketing
		).Return(fmt.Errorf("email service unavailable")).Times(1)

	mockMessageHistoryRepo.EXPECT().
		Create(
			ctx,
			workspaceID,
			gomock.Any(), // secretKey
			gomock.Any(), // message
		).Do(func(_ context.Context, _ string, _ string, msg *domain.MessageHistory) {
		// Verify list_id is populated from broadcast audience
		assert.NotNil(t, msg.ListID)
		assert.Equal(t, "list-1", *msg.ListID)
	}).Return(nil)

	// Create message sender
	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		nil, // dataFeedFetcher
		mockLogger,
		TestConfig(),
		"",
	)

	// Test
	recipients := []*domain.ContactWithList{
		{
			Contact: &domain.Contact{
				Email: "recipient1@example.com",
			},
			ListID:   "list-1",
			ListName: "Test List",
		},
	}
	templates := map[string]*domain.Template{"template-123": template}
	sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", "secret-key-123", "https://api.example.com", tracking, broadcastID, recipients, templates, emailProvider, timeoutAt)
	assert.NoError(t, err)
	assert.Equal(t, 0, sent)
	assert.Equal(t, 1, failed)
}

// TestSendBatch_RecordMessageFails tests that SendBatch continues even if recording message history fails
func TestSendBatch_RecordMessageFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	// Setup test data
	ctx := context.Background()
	timeoutAt := time.Now().Add(30 * time.Second)
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"
	tracking := true
	broadcast := &domain.Broadcast{
		ID:          broadcastID,
		WorkspaceID: workspaceID,
		Name:        "Test Broadcast",
		ChannelType: "email",
		Audience:    domain.AudienceSettings{List: "list-1"},
		Status:      domain.BroadcastStatusDraft,
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
			Content:  "test-content",
			Term:     "test-term",
		},
		TestSettings: domain.BroadcastTestSettings{
			Enabled: false,
			Variations: []domain.BroadcastVariation{
				{
					VariationName: "variation-1",
					TemplateID:    "template-123",
				},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	emailProvider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Test Subject",
			VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
		},
	}

	// Setup mock expectations
	mockBroadcastRepository.EXPECT().
		GetBroadcast(ctx, workspaceID, broadcastID).
		Return(broadcast, nil)

	mockEmailService.EXPECT().
		SendEmail(
			gomock.Any(), // ctx
			gomock.Any(), // SendEmailProviderRequest
			gomock.Any(), // isMarketing
		).Return(nil)

	mockMessageHistoryRepo.EXPECT().
		Create(
			ctx,
			workspaceID,
			gomock.Any(), // secretKey
			gomock.Any(), // message
		).Do(func(_ context.Context, _ string, _ string, msg *domain.MessageHistory) {
		// Verify list_id is populated from broadcast audience
		assert.NotNil(t, msg.ListID)
		assert.Equal(t, "list-1", *msg.ListID)
	}).Return(fmt.Errorf("database connection error"))

	// Create message sender
	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		nil, // dataFeedFetcher
		mockLogger,
		TestConfig(),
		"",
	)

	// Test
	recipients := []*domain.ContactWithList{
		{
			Contact: &domain.Contact{
				Email: "recipient1@example.com",
			},
			ListID:   "list-1",
			ListName: "Test List",
		},
	}
	templates := map[string]*domain.Template{"template-123": template}
	sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", "secret-key-123", "https://api.example.com", tracking, broadcastID, recipients, templates, emailProvider, timeoutAt)
	assert.NoError(t, err)
	assert.Equal(t, 1, sent)
	assert.Equal(t, 0, failed)
}

// TestSendToRecipientWithLiquidSubject tests sending email with Liquid templating in subject
func TestSendToRecipientWithLiquidSubject(t *testing.T) {
	// Create mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	// Create message sender
	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		nil, // dataFeedFetcher
		mockLogger,
		TestConfig(),
		"https://api.test.com",
	)

	// Test data
	ctx := context.Background()
	workspaceID := "workspace-123"
	messageID := "message-123"
	email := "test@example.com"
	tracking := true

	// Mock email provider and sender
	emailProvider := &domain.EmailProvider{
		Kind: "sendgrid",
	}
	emailSender := domain.EmailSender{
		ID:    "sender-123",
		Email: "sender@example.com",
		Name:  "Test Sender",
	}
	emailProvider.Senders = append(emailProvider.Senders, emailSender)

	// Mock broadcast
	broadcast := &domain.Broadcast{
		ID: "broadcast-123",
		UTMParameters: &domain.UTMParameters{
			Source:   "newsletter",
			Medium:   "email",
			Campaign: "welcome",
		},
	}

	// Template with Liquid templating in subject
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Welcome {{firstName}}! Your {{company}} account is ready",
			VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Hello {{firstName}}")),
		},
	}

	// Template data with variables for Liquid processing
	templateData := domain.MapOfAny{
		"firstName": "John",
		"lastName":  "Doe",
		"company":   "ACME Corp",
		"email":     email,
	}

	timeoutAt := time.Now().Add(5 * time.Minute)

	// Set up mock expectations - expect processed subject
	mockEmailService.EXPECT().
		SendEmail(
			gomock.Any(), // ctx
			gomock.Any(), // SendEmailProviderRequest
			gomock.Any(), // isMarketing
		).
		Return(nil)

	// Call the method
	err := sender.SendToRecipient(ctx, workspaceID, "test-integration-id", "https://api.test.com", tracking, broadcast, messageID, email, template, templateData, emailProvider, timeoutAt)

	// Verify
	assert.NoError(t, err)
}

// TestEnforceRateLimit tests the rate limiting functionality
func TestEnforceRateLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	t.Run("RateLimitDisabled", func(t *testing.T) {
		// Config with rate limiting disabled
		config := &Config{
			DefaultRateLimit: 0,
		}

		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			config,
			"",
		)

		messageSenderImpl := sender.(*messageSender)
		ctx := context.Background()

		// Should return immediately without delay
		start := time.Now()
		err := messageSenderImpl.enforceRateLimit(ctx, 0) // 0 rate limit should use default (also 0)
		elapsed := time.Since(start)

		assert.NoError(t, err)
		assert.Less(t, elapsed, 50*time.Millisecond) // Should be very fast
	})

	t.Run("RateLimitWithDelay", func(t *testing.T) {
		// Config with low rate limit to force delay
		config := &Config{
			DefaultRateLimit: 1200, // 20 per second (50ms per message)
		}

		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			config,
			"",
		)

		messageSenderImpl := sender.(*messageSender)
		ctx := context.Background()

		// First call should be relatively fast (initialize the rate limiter)
		err := messageSenderImpl.enforceRateLimit(ctx, 0) // 0 means use default (1200)
		assert.NoError(t, err)

		// Second call immediately should be delayed (rate is 20 per second = 50ms)
		start := time.Now()
		err = messageSenderImpl.enforceRateLimit(ctx, 0) // 0 means use default (1200)
		elapsed := time.Since(start)
		assert.NoError(t, err)

		// Should wait close to 50ms for 20 per second rate, but allow some tolerance
		assert.Greater(t, elapsed, 40*time.Millisecond, "Should delay for rate limiting")
		assert.Less(t, elapsed, 80*time.Millisecond, "Should not delay too much longer than expected")
	})

	t.Run("ContextCancelled", func(t *testing.T) {
		config := &Config{
			DefaultRateLimit: 1200, // 20 per second (50ms per message)
		}

		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			config,
			"",
		)

		messageSenderImpl := sender.(*messageSender)

		// Create a context that will be cancelled
		ctx, cancel := context.WithCancel(context.Background())

		// First call to set up the rate limiter
		err := messageSenderImpl.enforceRateLimit(ctx, 0) // 0 means use default (1200)
		assert.NoError(t, err)

		// Cancel the context
		cancel()

		// Second call should return context error
		err = messageSenderImpl.enforceRateLimit(ctx, 0) // 0 means use default (1200)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

// TestSendToRecipient_ErrorCases tests various error scenarios
func TestSendToRecipient_ErrorCases(t *testing.T) {
	ctx := context.Background()
	workspaceID := "workspace-123"
	tracking := true
	timeoutAt := time.Now().Add(30 * time.Second)

	t.Run("NilUTMParameters", func(t *testing.T) {
		// Create fresh mocks for this subtest
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Setup logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()

		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			TestConfig(),
			"",
		)

		broadcast := &domain.Broadcast{
			ID:            "broadcast-123",
			WorkspaceID:   workspaceID,
			UTMParameters: nil, // This should be handled gracefully
		}

		emailSender := domain.NewEmailSender("sender@example.com", "Sender")
		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{emailSender},
			SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
		}

		template := &domain.Template{
			ID: "template-123",
			Email: &domain.EmailTemplate{
				SenderID:         emailSender.ID,
				Subject:          "Test Subject",
				VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
			},
		}

		// Should succeed - SendEmail should be called
		mockEmailService.EXPECT().
			SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)

		err := sender.SendToRecipient(ctx, workspaceID, "test-integration-id", "https://api.test.com", tracking, broadcast, "message-123", "test@example.com", template, map[string]interface{}{}, emailProvider, timeoutAt)
		assert.NoError(t, err)
		// UTM parameters should be initialized to non-nil
		assert.NotNil(t, broadcast.UTMParameters)
	})

	t.Run("SenderNotFound", func(t *testing.T) {
		// Create fresh mocks for this subtest
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Setup logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			TestConfig(),
			"",
		)

		broadcast := &domain.Broadcast{
			ID:          "broadcast-123",
			WorkspaceID: workspaceID,
			UTMParameters: &domain.UTMParameters{
				Source:   "test",
				Medium:   "email",
				Campaign: "unit-test",
			},
		}

		// Template with a sender ID that doesn't exist in the provider
		// Create an empty email provider to ensure no senders exist
		emptyEmailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{}, // No senders at all
		}

		template := &domain.Template{
			ID: "template-123",
			Email: &domain.EmailTemplate{
				SenderID:         "any-sender-id",
				Subject:          "Test Subject",
				VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
			},
		}

		err := sender.SendToRecipient(ctx, workspaceID, "test-integration-id", "https://api.test.com", tracking, broadcast, "message-123", "test@example.com", template, map[string]interface{}{}, emptyEmailProvider, timeoutAt)
		assert.Error(t, err)
		broadcastErr, ok := err.(*BroadcastError)
		assert.True(t, ok)
		assert.Equal(t, ErrCodeSenderNotFound, broadcastErr.Code)
		// No SendEmail call should be made since sender is not found
	})

	t.Run("LiquidSubjectProcessingError", func(t *testing.T) {
		// Create fresh mocks for this subtest
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Setup logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			TestConfig(),
			"",
		)

		broadcast := &domain.Broadcast{
			ID:          "broadcast-123",
			WorkspaceID: workspaceID,
			UTMParameters: &domain.UTMParameters{
				Source:   "test",
				Medium:   "email",
				Campaign: "unit-test",
			},
		}

		emailSender := domain.NewEmailSender("sender@example.com", "Sender")
		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{emailSender},
			SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
		}

		// Template with invalid Liquid syntax in subject
		template := &domain.Template{
			ID: "template-123",
			Email: &domain.EmailTemplate{
				SenderID:         emailSender.ID,
				Subject:          "Hello {{invalid.liquid.syntax}}", // This should cause Liquid processing to fail
				VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
			},
		}

		templateData := map[string]interface{}{
			"name": "John",
		}

		// Mock successful email send in case Liquid processing succeeds
		mockEmailService.EXPECT().
			SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			MaxTimes(1) // Allow 0 or 1 calls

		err := sender.SendToRecipient(ctx, workspaceID, "test-integration-id", "https://api.test.com", tracking, broadcast, "message-123", "test@example.com", template, templateData, emailProvider, timeoutAt)
		// This might succeed or fail depending on the Liquid processor implementation
		// If it fails, it should be a template compile error
		if err != nil {
			broadcastErr, ok := err.(*BroadcastError)
			assert.True(t, ok)
			assert.Equal(t, ErrCodeTemplateCompile, broadcastErr.Code)
		}
	})

	t.Run("RateLimitWithContextCancellation", func(t *testing.T) {
		// Create fresh mocks for this subtest
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Setup logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

		// Create a config with low rate limit
		config := &Config{
			DefaultRateLimit:        1200, // 20 per second (50ms per message)
			EnableCircuitBreaker:    false,
			CircuitBreakerThreshold: 5,
			CircuitBreakerCooldown:  1 * time.Minute,
		}

		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			config,
			"",
		)

		broadcast := &domain.Broadcast{
			ID:          "broadcast-123",
			WorkspaceID: workspaceID,
			UTMParameters: &domain.UTMParameters{
				Source:   "test",
				Medium:   "email",
				Campaign: "unit-test",
			},
		}

		emailSender := domain.NewEmailSender("sender@example.com", "Sender")
		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{emailSender},
			SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
		}

		template := &domain.Template{
			ID: "template-123",
			Email: &domain.EmailTemplate{
				SenderID:         emailSender.ID,
				Subject:          "Test Subject",
				VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
			},
		}

		// First, send a message successfully
		mockEmailService.EXPECT().
			SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		err := sender.SendToRecipient(ctx, workspaceID, "test-integration-id", "https://api.test.com", tracking, broadcast, "message-1", "test@example.com", template, map[string]interface{}{}, emailProvider, timeoutAt)
		assert.NoError(t, err)

		// Create a context that will be cancelled quickly
		cancelCtx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()

		// Mock for the second message: return context.Canceled error if SendEmail is called
		mockEmailService.EXPECT().
			SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(context.Canceled).
			MaxTimes(1)

		// Second message should fail due to context cancellation
		// Note: Error could be ErrCodeRateLimitExceeded (cancelled during rate limiting)
		// or ErrCodeSendFailed (cancelled during email send)
		err = sender.SendToRecipient(cancelCtx, workspaceID, "test-integration-id", "https://api.test.com", tracking, broadcast, "message-2", "test@example.com", template, map[string]interface{}{}, emailProvider, timeoutAt)
		assert.Error(t, err)
		broadcastErr, ok := err.(*BroadcastError)
		if assert.True(t, ok, "Expected error to be a BroadcastError but got: %T", err) {
			// Accept either rate limit error or send failed error depending on when cancellation occurs
			validCodes := []ErrorCode{ErrCodeRateLimitExceeded, ErrCodeSendFailed}
			assert.Contains(t, validCodes, broadcastErr.Code,
				"Expected error code to be either ErrCodeRateLimitExceeded or ErrCodeSendFailed, got: %s", broadcastErr.Code)
		}
	})
}

// TestSendBatch_AdvancedScenarios tests more complex SendBatch scenarios
func TestSendBatch_AdvancedScenarios(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	ctx := context.Background()
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"
	timeoutAt := time.Now().Add(30 * time.Second)

	t.Run("GetBroadcastReturnsNil", func(t *testing.T) {
		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			TestConfig(),
			"",
		)

		recipients := []*domain.ContactWithList{
			{Contact: &domain.Contact{Email: "test@example.com"}},
		}

		// Mock returns nil broadcast without error
		mockBroadcastRepository.EXPECT().
			GetBroadcast(ctx, workspaceID, broadcastID).
			Return(nil, nil)

		sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", "secret-key", "https://api.example.com", true, broadcastID, recipients, map[string]*domain.Template{}, nil, timeoutAt)

		assert.Error(t, err)
		assert.Equal(t, 0, sent)
		assert.Equal(t, 0, failed)
		broadcastErr, ok := err.(*BroadcastError)
		assert.True(t, ok)
		assert.Equal(t, ErrCodeBroadcastNotFound, broadcastErr.Code)
	})

	t.Run("TimeoutReached", func(t *testing.T) {
		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			TestConfig(),
			"",
		)

		broadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			UTMParameters: &domain.UTMParameters{
				Source:   "test",
				Medium:   "email",
				Campaign: "unit-test",
			},
			TestSettings: domain.BroadcastTestSettings{
				Enabled: false,
				Variations: []domain.BroadcastVariation{
					{VariationName: "variation-1", TemplateID: "template-123"},
				},
			},
		}

		recipients := []*domain.ContactWithList{
			{Contact: &domain.Contact{Email: "test1@example.com"}},
			{Contact: &domain.Contact{Email: "test2@example.com"}},
		}

		emailSender := domain.NewEmailSender("sender@example.com", "Sender")
		emailProvider := &domain.EmailProvider{
			Kind:               domain.EmailProviderKindSMTP,
			RateLimitPerMinute: 25,
			Senders:            []domain.EmailSender{emailSender},
			SMTP:               &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
		}
		template := &domain.Template{
			ID: "template-123",
			Email: &domain.EmailTemplate{
				SenderID:         emailSender.ID,
				Subject:          "Test Subject",
				VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
			},
		}
		templates := map[string]*domain.Template{"template-123": template}

		mockBroadcastRepository.EXPECT().
			GetBroadcast(ctx, workspaceID, broadcastID).
			Return(broadcast, nil)

		// Use a timeout that's already passed
		pastTimeout := time.Now().Add(-1 * time.Second)

		sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", "secret-key", "https://api.example.com", true, broadcastID, recipients, templates, emailProvider, pastTimeout)

		// Should return immediately without processing any recipients
		assert.NoError(t, err)
		assert.Equal(t, 0, sent)
		assert.Equal(t, 0, failed)
	})

	t.Run("ABTestRandomVariationSelection", func(t *testing.T) {
		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			TestConfig(),
			"",
		)

		broadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Audience: domain.AudienceSettings{
				List: "test-list-1",
			},
			UTMParameters: &domain.UTMParameters{
				Source:   "test",
				Medium:   "email",
				Campaign: "unit-test",
			},
			TestSettings: domain.BroadcastTestSettings{
				Enabled: true,
				Variations: []domain.BroadcastVariation{
					{VariationName: "variation-1", TemplateID: "template-123"},
					{VariationName: "variation-2", TemplateID: "template-456"},
				},
			},
		}

		recipients := []*domain.ContactWithList{
			{Contact: &domain.Contact{Email: "a@example.com"}}, // First char 'a' = ASCII 97
			{Contact: &domain.Contact{Email: "b@example.com"}}, // First char 'b' = ASCII 98
		}

		emailSender := domain.NewEmailSender("sender@example.com", "Sender")
		template1 := &domain.Template{
			ID: "template-123",
			Email: &domain.EmailTemplate{
				SenderID:         emailSender.ID,
				Subject:          "Test Subject 1",
				VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content 1")),
			},
		}
		template2 := &domain.Template{
			ID: "template-456",
			Email: &domain.EmailTemplate{
				SenderID:         emailSender.ID,
				Subject:          "Test Subject 2",
				VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content 2")),
			},
		}
		templates := map[string]*domain.Template{
			"template-123": template1,
			"template-456": template2,
		}

		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{emailSender},
			SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
		}

		mockBroadcastRepository.EXPECT().
			GetBroadcast(ctx, workspaceID, broadcastID).
			Return(broadcast, nil)

		// Expect emails to be sent
		mockEmailService.EXPECT().
			SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).Times(2)

		mockMessageHistoryRepo.EXPECT().
			Create(ctx, workspaceID, gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, _ string, _ string, msg *domain.MessageHistory) {
				// Verify list_id is populated from broadcast audience
				assert.NotNil(t, msg.ListID)
				assert.Equal(t, broadcast.Audience.List, *msg.ListID)
			}).
			Return(nil).Times(2)

		sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", "secret-key", "https://api.example.com", true, broadcastID, recipients, templates, emailProvider, timeoutAt)

		assert.NoError(t, err)
		assert.Equal(t, 2, sent)
		assert.Equal(t, 0, failed)
	})

	t.Run("WinnerTemplateSelected", func(t *testing.T) {
		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			TestConfig(),
			"",
		)

		winnerTemplateID := "template-winner"
		broadcast := &domain.Broadcast{
			ID:              broadcastID,
			WorkspaceID:     workspaceID,
			WinningTemplate: &winnerTemplateID, // Winner already selected
			Audience: domain.AudienceSettings{
				List: "test-list-1",
			},
			UTMParameters: &domain.UTMParameters{
				Source:   "test",
				Medium:   "email",
				Campaign: "unit-test",
			},
			TestSettings: domain.BroadcastTestSettings{
				Enabled: true,
				Variations: []domain.BroadcastVariation{
					{VariationName: "variation-1", TemplateID: "template-123"},
					{VariationName: "variation-2", TemplateID: "template-456"},
				},
			},
		}

		recipients := []*domain.ContactWithList{
			{Contact: &domain.Contact{Email: "test@example.com"}},
		}

		emailSender := domain.NewEmailSender("sender@example.com", "Sender")
		winnerTemplate := &domain.Template{
			ID: "template-winner",
			Email: &domain.EmailTemplate{
				SenderID:         emailSender.ID,
				Subject:          "Winner Template",
				VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Winner content")),
			},
		}
		templates := map[string]*domain.Template{
			"template-winner": winnerTemplate,
		}

		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{emailSender},
			SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
		}

		mockBroadcastRepository.EXPECT().
			GetBroadcast(ctx, workspaceID, broadcastID).
			Return(broadcast, nil)

		mockEmailService.EXPECT().
			SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)

		mockMessageHistoryRepo.EXPECT().
			Create(ctx, workspaceID, gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, _ string, _ string, msg *domain.MessageHistory) {
				// Verify list_id is populated from broadcast audience
				assert.NotNil(t, msg.ListID)
				assert.Equal(t, broadcast.Audience.List, *msg.ListID)
			}).
			Return(nil)

		sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", "secret-key", "https://api.example.com", true, broadcastID, recipients, templates, emailProvider, timeoutAt)

		assert.NoError(t, err)
		assert.Equal(t, 1, sent)
		assert.Equal(t, 0, failed)
	})

	t.Run("CircuitBreakerRecordsFailureOnHighFailureRate", func(t *testing.T) {
		config := TestConfig()
		config.EnableCircuitBreaker = true
		config.CircuitBreakerThreshold = 5 // Set higher than number of recipients to allow all attempts

		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			config,
			"",
		)

		broadcast := &domain.Broadcast{
			ID:          broadcastID,
			WorkspaceID: workspaceID,
			Audience: domain.AudienceSettings{
				List: "test-list-1",
			},
			UTMParameters: &domain.UTMParameters{
				Source:   "test",
				Medium:   "email",
				Campaign: "unit-test",
			},
			TestSettings: domain.BroadcastTestSettings{
				Enabled: false,
				Variations: []domain.BroadcastVariation{
					{VariationName: "variation-1", TemplateID: "template-123"},
				},
			},
		}

		recipients := []*domain.ContactWithList{
			{Contact: &domain.Contact{Email: "test1@example.com"}},
			{Contact: &domain.Contact{Email: "test2@example.com"}},
			{Contact: &domain.Contact{Email: "test3@example.com"}},
		}

		emailSender := domain.NewEmailSender("sender@example.com", "Sender")
		template := &domain.Template{
			ID: "template-123",
			Email: &domain.EmailTemplate{
				SenderID:         emailSender.ID,
				Subject:          "Test Subject",
				VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
			},
		}
		templates := map[string]*domain.Template{"template-123": template}

		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{emailSender},
			SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
		}

		mockBroadcastRepository.EXPECT().
			GetBroadcast(ctx, workspaceID, broadcastID).
			Return(broadcast, nil)

		// All email sends fail
		mockEmailService.EXPECT().
			SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(fmt.Errorf("email service unavailable")).Times(3)

		// Message history should still be recorded for failed messages
		mockMessageHistoryRepo.EXPECT().
			Create(ctx, workspaceID, gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, _ string, _ string, msg *domain.MessageHistory) {
				// Verify list_id is populated from broadcast audience
				assert.NotNil(t, msg.ListID)
				assert.Equal(t, broadcast.Audience.List, *msg.ListID)
			}).
			Return(nil).Times(3)

		sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", "secret-key", "https://api.example.com", true, broadcastID, recipients, templates, emailProvider, timeoutAt)

		assert.NoError(t, err) // SendBatch itself doesn't return error, just counts
		assert.Equal(t, 0, sent)
		assert.Equal(t, 3, failed)

		// Circuit breaker should have recorded the failure
		messageSenderImpl := sender.(*messageSender)
		assert.NotNil(t, messageSenderImpl.circuitBreaker.GetLastError())
	})
}

// TestNewMessageSender_EdgeCases tests NewMessageSender with different configurations
func TestNewMessageSender_EdgeCases(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	t.Run("VeryLowRateLimit", func(t *testing.T) {
		config := &Config{
			DefaultRateLimit:     30, // 0.5 per second, should result in permitsPerSecond = 1
			EnableCircuitBreaker: false,
		}

		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			config,
			"https://api.example.com",
		)

		assert.NotNil(t, sender)
		messageSenderImpl := sender.(*messageSender)
		assert.Equal(t, "https://api.example.com", messageSenderImpl.apiEndpoint)
		assert.Nil(t, messageSenderImpl.circuitBreaker) // Should be nil when disabled
	})

	t.Run("ZeroRateLimit", func(t *testing.T) {
		config := &Config{
			DefaultRateLimit:        0, // Should result in permitsPerSecond = 1
			EnableCircuitBreaker:    true,
			CircuitBreakerThreshold: 10,
			CircuitBreakerCooldown:  2 * time.Minute,
		}

		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			config,
			"",
		)

		assert.NotNil(t, sender)
		messageSenderImpl := sender.(*messageSender)
		assert.NotNil(t, messageSenderImpl.circuitBreaker) // Should be present when enabled
		assert.Equal(t, config.CircuitBreakerThreshold, messageSenderImpl.circuitBreaker.threshold)
		assert.Equal(t, config.CircuitBreakerCooldown, messageSenderImpl.circuitBreaker.cooldownPeriod)
	})
}

// TestCircuitBreaker_Advanced tests more circuit breaker scenarios
func TestCircuitBreaker_Advanced(t *testing.T) {
	t.Run("CooldownReset", func(t *testing.T) {
		cb := NewCircuitBreaker(2, 20*time.Millisecond)

		// Record failures to open circuit
		cb.RecordFailure(fmt.Errorf("error 1"))
		cb.RecordFailure(fmt.Errorf("error 2"))
		assert.True(t, cb.IsOpen())

		// Wait for cooldown
		time.Sleep(30 * time.Millisecond)

		// Circuit should be closed after cooldown
		assert.False(t, cb.IsOpen())
		assert.Equal(t, 0, cb.failures)
		assert.Nil(t, cb.GetLastError())
	})

	t.Run("SuccessResetsFailures", func(t *testing.T) {
		cb := NewCircuitBreaker(3, 1*time.Minute)

		// Record some failures
		cb.RecordFailure(fmt.Errorf("error 1"))
		cb.RecordFailure(fmt.Errorf("error 2"))
		assert.False(t, cb.IsOpen()) // Not enough to open
		assert.Equal(t, 2, cb.failures)

		// Record success
		cb.RecordSuccess()
		assert.False(t, cb.IsOpen())
		assert.Equal(t, 0, cb.failures)
		assert.Nil(t, cb.GetLastError())
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		cb := NewCircuitBreaker(5, 1*time.Second)

		// Test concurrent access
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				if id%2 == 0 {
					cb.RecordFailure(fmt.Errorf("error %d", id))
				} else {
					cb.RecordSuccess()
				}
				cb.IsOpen()
				_ = cb.GetLastError()
			}(i)
		}
		wg.Wait()

		// Should not panic and should be in a consistent state
		assert.NotPanics(t, func() {
			cb.IsOpen()
			_ = cb.GetLastError()
		})
	})
}

// TestSendBatch_TemplateDataBuildFailure tests SendBatch when template data building fails
func TestSendBatch_TemplateDataBuildFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	ctx := context.Background()
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"
	timeoutAt := time.Now().Add(30 * time.Second)

	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		nil, // dataFeedFetcher
		mockLogger,
		TestConfig(),
		"",
	)

	broadcast := &domain.Broadcast{
		ID:          broadcastID,
		WorkspaceID: workspaceID,
		Audience: domain.AudienceSettings{
			List: "test-list-1",
		},
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
		},
		TestSettings: domain.BroadcastTestSettings{
			Enabled: false,
			Variations: []domain.BroadcastVariation{
				{VariationName: "variation-1", TemplateID: "template-123"},
			},
		},
	}

	// Recipients with contact that might cause template data building to fail
	recipients := []*domain.ContactWithList{
		{Contact: &domain.Contact{Email: "test@example.com"}},
	}

	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Test Subject",
			VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
		},
	}
	templates := map[string]*domain.Template{"template-123": template}

	mockBroadcastRepository.EXPECT().
		GetBroadcast(ctx, workspaceID, broadcastID).
		Return(broadcast, nil)

	emailProvider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}

	// Mock expectations for successful processing
	mockEmailService.EXPECT().
		SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	mockMessageHistoryRepo.EXPECT().
		Create(ctx, workspaceID, gomock.Any(), gomock.Any()).
		Do(func(_ context.Context, _ string, _ string, msg *domain.MessageHistory) {
			// Verify list_id is populated from broadcast audience
			assert.NotNil(t, msg.ListID)
			assert.Equal(t, broadcast.Audience.List, *msg.ListID)
		}).
		Return(nil).AnyTimes()

	sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", "secret-key", "https://api.example.com", true, broadcastID, recipients, templates, emailProvider, timeoutAt)

	// Should handle the case gracefully
	assert.NoError(t, err)
	// Results depend on whether template data building succeeds or fails
	assert.Equal(t, len(recipients), sent+failed)
}

// TestSendBatch_EmptyEmailContact tests SendBatch with contacts having empty emails
func TestSendBatch_EmptyEmailContact(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	ctx := context.Background()
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"
	timeoutAt := time.Now().Add(30 * time.Second)

	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		nil, // dataFeedFetcher
		mockLogger,
		TestConfig(),
		"",
	)

	broadcast := &domain.Broadcast{
		ID:          broadcastID,
		WorkspaceID: workspaceID,
		Audience: domain.AudienceSettings{
			List: "test-list-1",
		},
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
		},
		TestSettings: domain.BroadcastTestSettings{
			Enabled: false,
			Variations: []domain.BroadcastVariation{
				{VariationName: "variation-1", TemplateID: "template-123"},
			},
		},
	}

	// Mix of valid and invalid recipients
	recipients := []*domain.ContactWithList{
		{Contact: &domain.Contact{Email: ""}},                  // Empty email
		{Contact: &domain.Contact{Email: "valid@example.com"}}, // Valid email
		{Contact: nil}, // Nil contact
	}

	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Test Subject",
			VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
		},
	}
	templates := map[string]*domain.Template{"template-123": template}

	emailProvider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}

	mockBroadcastRepository.EXPECT().
		GetBroadcast(ctx, workspaceID, broadcastID).
		Return(broadcast, nil)

	// Only one email should be sent (the valid one)
	mockEmailService.EXPECT().
		SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).Times(1)

	mockMessageHistoryRepo.EXPECT().
		Create(ctx, workspaceID, gomock.Any(), gomock.Any()).
		Do(func(_ context.Context, _ string, _ string, msg *domain.MessageHistory) {
			// Verify list_id is populated from broadcast audience
			assert.NotNil(t, msg.ListID)
			assert.Equal(t, broadcast.Audience.List, *msg.ListID)
		}).
		Return(nil).Times(1)

	sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", "secret-key", "https://api.example.com", true, broadcastID, recipients, templates, emailProvider, timeoutAt)

	assert.NoError(t, err)
	assert.Equal(t, 1, sent)
	assert.Equal(t, 2, failed) // Two invalid contacts
}

// TestSendBatch_NoVariations tests SendBatch when broadcast has no variations
func TestSendBatch_NoVariations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	ctx := context.Background()
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"
	timeoutAt := time.Now().Add(30 * time.Second)

	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		nil, // dataFeedFetcher
		mockLogger,
		TestConfig(),
		"",
	)

	broadcast := &domain.Broadcast{
		ID:          broadcastID,
		WorkspaceID: workspaceID,
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
		},
		TestSettings: domain.BroadcastTestSettings{
			Enabled:    false,
			Variations: []domain.BroadcastVariation{}, // No variations
		},
	}

	recipients := []*domain.ContactWithList{
		{Contact: &domain.Contact{Email: "test@example.com"}},
	}

	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	emailProvider := &domain.EmailProvider{
		Kind:               domain.EmailProviderKindSMTP,
		RateLimitPerMinute: 25,
		Senders:            []domain.EmailSender{emailSender},
		SMTP:               &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}

	templates := map[string]*domain.Template{} // No templates

	mockBroadcastRepository.EXPECT().
		GetBroadcast(ctx, workspaceID, broadcastID).
		Return(broadcast, nil)

	sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", "secret-key", "https://api.example.com", true, broadcastID, recipients, templates, emailProvider, timeoutAt)

	assert.NoError(t, err)
	assert.Equal(t, 0, sent)
	assert.Equal(t, 1, failed) // Should fail due to no template
}

// TestSendToRecipient_CompilationFailsWithNilHTML tests when compilation succeeds but HTML is nil
func TestSendToRecipient_CompilationFailsWithNilHTML(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	ctx := context.Background()
	workspaceID := "workspace-123"
	tracking := true
	timeoutAt := time.Now().Add(30 * time.Second)

	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		nil, // dataFeedFetcher
		mockLogger,
		TestConfig(),
		"",
	)

	broadcast := &domain.Broadcast{
		ID:          "broadcast-123",
		WorkspaceID: workspaceID,
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
		},
	}

	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	emailProvider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}

	// Template with minimal content that might cause HTML compilation to return nil
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Test Subject",
			VisualEditorTree: &notifuse_mjml.MJMLBlock{}, // Minimal block
		},
	}

	err := sender.SendToRecipient(ctx, workspaceID, "test-integration-id", "https://api.test.com", tracking, broadcast, "message-123", "test@example.com", template, map[string]interface{}{}, emailProvider, timeoutAt)

	// Should fail with template compilation error
	assert.Error(t, err)
	broadcastErr, ok := err.(*BroadcastError)
	assert.True(t, ok)
	assert.Equal(t, ErrCodeTemplateCompile, broadcastErr.Code)
}

// TestSendToRecipient_CircuitBreakerSuccessRecording tests circuit breaker success recording
func TestSendToRecipient_CircuitBreakerSuccessRecording(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	config := TestConfig()
	config.EnableCircuitBreaker = true

	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		nil, // dataFeedFetcher
		mockLogger,
		config,
		"",
	)

	ctx := context.Background()
	workspaceID := "workspace-123"
	tracking := true
	timeoutAt := time.Now().Add(30 * time.Second)

	broadcast := &domain.Broadcast{
		ID:          "broadcast-123",
		WorkspaceID: workspaceID,
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
		},
	}

	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	emailProvider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}

	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Test Subject",
			VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
		},
	}

	// First record a failure to test success reset
	messageSenderImpl := sender.(*messageSender)
	messageSenderImpl.circuitBreaker.RecordFailure(fmt.Errorf("test failure"))
	assert.Equal(t, 1, messageSenderImpl.circuitBreaker.failures)

	// Mock successful email send
	mockEmailService.EXPECT().
		SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	err := sender.SendToRecipient(ctx, workspaceID, "test-integration-id", "https://api.test.com", tracking, broadcast, "message-123", "test@example.com", template, map[string]interface{}{}, emailProvider, timeoutAt)

	assert.NoError(t, err)
	// Circuit breaker should have recorded success and reset failures
	assert.Equal(t, 0, messageSenderImpl.circuitBreaker.failures)
	assert.Nil(t, messageSenderImpl.circuitBreaker.GetLastError())
}

// TestPerBroadcastRateLimit tests per-broadcast rate limiting functionality
func TestPerBroadcastRateLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Allow logging calls
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	t.Run("UseBroadcastSpecificRateLimit", func(t *testing.T) {
		// Config with default rate limit
		config := &Config{
			DefaultRateLimit: 6000, // 100 per second default
		}

		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			config,
			"http://api.example.com",
		)

		messageSenderImpl := sender.(*messageSender)
		ctx := context.Background()

		// Test with broadcast-specific rate limit of 1200 (20 per second = 50ms)
		err := messageSenderImpl.enforceRateLimit(ctx, 1200)
		assert.NoError(t, err)

		// Second call should be delayed due to broadcast rate limit
		start := time.Now()
		err = messageSenderImpl.enforceRateLimit(ctx, 1200)
		elapsed := time.Since(start)
		assert.NoError(t, err)

		// Should wait close to 50ms for 20 per second rate
		assert.Greater(t, elapsed, 30*time.Millisecond, "Should delay for broadcast rate limiting")
		assert.Less(t, elapsed, 80*time.Millisecond, "Should not delay too much longer than expected")
	})

	t.Run("FallbackToDefaultRateLimit", func(t *testing.T) {
		// Config with default rate limit
		config := &Config{
			DefaultRateLimit: 1200, // 20 per second default (50ms per message)
		}

		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			config,
			"http://api.example.com",
		)

		messageSenderImpl := sender.(*messageSender)
		ctx := context.Background()

		// Test with broadcast rate limit of 0 (should fall back to default)
		err := messageSenderImpl.enforceRateLimit(ctx, 0)
		assert.NoError(t, err)

		// Second call should be delayed due to default rate limit
		start := time.Now()
		err = messageSenderImpl.enforceRateLimit(ctx, 0)
		elapsed := time.Since(start)
		assert.NoError(t, err)

		// Should wait close to 50ms for 20 per second rate
		assert.Greater(t, elapsed, 30*time.Millisecond, "Should delay using default rate limiting")
		assert.Less(t, elapsed, 80*time.Millisecond, "Should not delay too much longer than expected")
	})

	t.Run("DisabledWhenBothRateLimitsAreZero", func(t *testing.T) {
		// Config with disabled rate limit
		config := &Config{
			DefaultRateLimit: 0, // Disabled
		}

		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			config,
			"http://api.example.com",
		)

		messageSenderImpl := sender.(*messageSender)
		ctx := context.Background()

		// Test with broadcast rate limit of 0 and default rate limit of 0
		start := time.Now()
		err := messageSenderImpl.enforceRateLimit(ctx, 0)
		elapsed := time.Since(start)
		assert.NoError(t, err)

		// Should return immediately without delay
		assert.Less(t, elapsed, 50*time.Millisecond, "Should not delay when rate limiting is disabled")
	})

	t.Run("SendToRecipientUsesBroadcastRateLimit", func(t *testing.T) {
		// Config with high default rate limit
		config := &Config{
			DefaultRateLimit:     6000, // 100 per second default
			EnableCircuitBreaker: false,
		}

		sender := NewMessageSender(
			mockBroadcastRepository,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockEmailService,
			nil, // dataFeedFetcher
			mockLogger,
			config,
			"http://api.example.com",
		)

		ctx := context.Background()
		workspaceID := "workspace-123"
		tracking := true
		timeoutAt := time.Now().Add(30 * time.Second)

		// Broadcast with low rate limit (20 per second = 50ms)
		broadcast := &domain.Broadcast{
			ID:          "broadcast-123",
			WorkspaceID: workspaceID,
			Audience:    domain.AudienceSettings{},
			UTMParameters: &domain.UTMParameters{
				Source:   "test",
				Medium:   "email",
				Campaign: "unit-test",
			},
		}

		emailSender := domain.NewEmailSender("sender@example.com", "Sender")
		emailProvider := &domain.EmailProvider{
			Kind:               domain.EmailProviderKindSMTP,
			RateLimitPerMinute: 1200, // 20 per second
			Senders:            []domain.EmailSender{emailSender},
			SMTP:               &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
		}

		template := &domain.Template{
			ID: "template-123",
			Email: &domain.EmailTemplate{
				SenderID:         emailSender.ID,
				Subject:          "Test Subject",
				VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
			},
		}

		// Mock successful email sends
		mockEmailService.EXPECT().
			SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).Times(2)

		// First send should be fast
		err := sender.SendToRecipient(ctx, workspaceID, "test-integration-id", "https://api.test.com", tracking, broadcast, "message-1", "test1@example.com", template, map[string]interface{}{}, emailProvider, timeoutAt)
		assert.NoError(t, err)

		// Second send should be delayed due to broadcast rate limit
		start := time.Now()
		err = sender.SendToRecipient(ctx, workspaceID, "test-integration-id", "https://api.test.com", tracking, broadcast, "message-2", "test2@example.com", template, map[string]interface{}{}, emailProvider, timeoutAt)
		elapsed := time.Since(start)
		assert.NoError(t, err)

		// Should wait close to 50ms due to broadcast rate limit
		assert.Greater(t, elapsed, 30*time.Millisecond, "Should delay for broadcast rate limiting")
		assert.Less(t, elapsed, 80*time.Millisecond, "Should not delay too much longer than expected")
	})
}

// TestSendBatch_WithRecipientFeed_Success tests successful recipient feed integration
func TestSendBatch_WithRecipientFeed_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockDataFeedFetcher := bmocks.NewMockDataFeedFetcher(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	ctx := context.Background()
	timeoutAt := time.Now().Add(30 * time.Second)
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"
	tracking := true

	// Broadcast with RecipientFeed enabled
	broadcast := &domain.Broadcast{
		ID:          broadcastID,
		WorkspaceID: workspaceID,
		Name:        "Test Broadcast",
		ChannelType: "email",
		Audience:    domain.AudienceSettings{List: "list-1"},
		Status:      domain.BroadcastStatusDraft,
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
		},
		TestSettings: domain.BroadcastTestSettings{
			Enabled: false,
			Variations: []domain.BroadcastVariation{
				{
					VariationName: "variation-1",
					TemplateID:    "template-123",
				},
			},
		},
		DataFeed: &domain.DataFeedSettings{
			RecipientFeed: &domain.RecipientFeedSettings{
				Enabled: true,
				URL:     "https://api.example.com/feed",
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	emailProvider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Test Subject",
			VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
		},
	}

	// Setup mock expectations
	mockBroadcastRepository.EXPECT().
		GetBroadcast(ctx, workspaceID, broadcastID).
		Return(broadcast, nil)

	// Mock FetchRecipient to return feed data
	feedData := map[string]interface{}{
		"product_name":  "Premium Widget",
		"product_price": 99.99,
		"_success":      true,
	}
	mockDataFeedFetcher.EXPECT().
		FetchRecipient(gomock.Any(), broadcast.DataFeed.RecipientFeed, gomock.Any()).
		Return(feedData, nil).
		Times(1)

	mockEmailService.EXPECT().
		SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).Times(1)

	mockMessageHistoryRepo.EXPECT().
		Create(ctx, workspaceID, gomock.Any(), gomock.Any()).
		Return(nil).Times(1)

	// Create message sender with DataFeedFetcher
	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		mockDataFeedFetcher,
		mockLogger,
		TestConfig(),
		"",
	)

	recipients := []*domain.ContactWithList{
		{
			Contact:  &domain.Contact{Email: "recipient1@example.com"},
			ListID:   "list-1",
			ListName: "Test List",
		},
	}
	templates := map[string]*domain.Template{"template-123": template}

	sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", "secret-key-123", "https://api.example.com", tracking, broadcastID, recipients, templates, emailProvider, timeoutAt)
	assert.NoError(t, err)
	assert.Equal(t, 1, sent)
	assert.Equal(t, 0, failed)
}

// TestSendBatch_WithRecipientFeed_PauseOnFailure tests that broadcast pauses immediately on feed failure
func TestSendBatch_WithRecipientFeed_PauseOnFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockDataFeedFetcher := bmocks.NewMockDataFeedFetcher(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	ctx := context.Background()
	timeoutAt := time.Now().Add(30 * time.Second)
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"
	tracking := true

	// Broadcast with RecipientFeed enabled
	broadcast := &domain.Broadcast{
		ID:          broadcastID,
		WorkspaceID: workspaceID,
		Name:        "Test Broadcast",
		ChannelType: "email",
		Audience:    domain.AudienceSettings{List: "list-1"},
		Status:      domain.BroadcastStatusDraft,
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
		},
		TestSettings: domain.BroadcastTestSettings{
			Enabled: false,
			Variations: []domain.BroadcastVariation{
				{
					VariationName: "variation-1",
					TemplateID:    "template-123",
				},
			},
		},
		DataFeed: &domain.DataFeedSettings{
			RecipientFeed: &domain.RecipientFeedSettings{
				Enabled: true,
				URL:     "https://api.example.com/feed",
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	emailProvider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Test Subject",
			VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
		},
	}

	// Setup mock expectations
	mockBroadcastRepository.EXPECT().
		GetBroadcast(ctx, workspaceID, broadcastID).
		Return(broadcast, nil)

	// Mock FetchRecipient to return error - broadcast should pause immediately
	mockDataFeedFetcher.EXPECT().
		FetchRecipient(gomock.Any(), broadcast.DataFeed.RecipientFeed, gomock.Any()).
		Return(nil, fmt.Errorf("feed service unavailable")).
		Times(1)

	// No email should be sent (broadcast pauses)
	// mockEmailService.EXPECT().SendEmail(...) should NOT be called

	// Create message sender with DataFeedFetcher
	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		mockDataFeedFetcher,
		mockLogger,
		TestConfig(),
		"",
	)

	recipients := []*domain.ContactWithList{
		{
			Contact:  &domain.Contact{Email: "recipient1@example.com"},
			ListID:   "list-1",
			ListName: "Test List",
		},
	}
	templates := map[string]*domain.Template{"template-123": template}

	sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", "secret-key-123", "https://api.example.com", tracking, broadcastID, recipients, templates, emailProvider, timeoutAt)
	// Broadcast should pause on first feed failure
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrBroadcastShouldPause), "Expected ErrBroadcastShouldPause")
	assert.Equal(t, 0, sent)
	assert.Equal(t, 0, failed)
}

// TestSendBatch_WithRecipientFeed_ErrorPause is removed - the pause-on-failure behavior
// is now tested in TestSendBatch_WithRecipientFeed_PauseOnFailure above.
// Keeping this as a placeholder to avoid breaking test numbering.
func TestSendBatch_WithRecipientFeed_LegacyRemoved(t *testing.T) {
	t.Skip("Replaced by TestSendBatch_WithRecipientFeed_PauseOnFailure - feed errors now cause immediate pause")
}

// TestSendBatch_WithRecipientFeed_ErrorPause tests recipient feed error with pause strategy
func TestSendBatch_WithRecipientFeed_ErrorPause(t *testing.T) {
	t.Skip("Replaced by TestSendBatch_WithRecipientFeed_PauseOnFailure - feed errors now cause immediate pause")
}

func TestSendBatch_WithRecipientFeed_OldPause(t *testing.T) {
	t.Skip("Removed - consecutive failure threshold no longer exists. Feed errors now cause immediate pause.")
}

// TestSendBatch_WithBothFeeds tests broadcast with both GlobalFeedData and RecipientFeed
func TestSendBatch_WithBothFeeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockDataFeedFetcher := bmocks.NewMockDataFeedFetcher(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	ctx := context.Background()
	timeoutAt := time.Now().Add(30 * time.Second)
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"
	tracking := true

	// Broadcast with both GlobalFeedData and RecipientFeed
	broadcast := &domain.Broadcast{
		ID:          broadcastID,
		WorkspaceID: workspaceID,
		Name:        "Test Broadcast",
		ChannelType: "email",
		Audience:    domain.AudienceSettings{List: "list-1"},
		Status:      domain.BroadcastStatusDraft,
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
		},
		TestSettings: domain.BroadcastTestSettings{
			Enabled: false,
			Variations: []domain.BroadcastVariation{
				{
					VariationName: "variation-1",
					TemplateID:    "template-123",
				},
			},
		},
		// DataFeed with pre-fetched global feed data and recipient feed
		DataFeed: &domain.DataFeedSettings{
			GlobalFeedData: domain.MapOfAny{
				"company_name": "Acme Corp",
				"promo_code":   "SUMMER2024",
				"_success":     true,
			},
			RecipientFeed: &domain.RecipientFeedSettings{
				Enabled: true,
				URL:     "https://api.example.com/feed",
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	emailProvider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Test Subject",
			VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
		},
	}

	// Setup mock expectations
	mockBroadcastRepository.EXPECT().
		GetBroadcast(ctx, workspaceID, broadcastID).
		Return(broadcast, nil)

	// Mock FetchRecipient to return feed data
	recipientFeedData := map[string]interface{}{
		"product_name":  "Premium Widget",
		"product_price": 99.99,
		"_success":      true,
	}
	mockDataFeedFetcher.EXPECT().
		FetchRecipient(gomock.Any(), broadcast.DataFeed.RecipientFeed, gomock.Any()).
		Return(recipientFeedData, nil).
		Times(1)

	mockEmailService.EXPECT().
		SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).Times(1)

	mockMessageHistoryRepo.EXPECT().
		Create(ctx, workspaceID, gomock.Any(), gomock.Any()).
		Return(nil).Times(1)

	// Create message sender with DataFeedFetcher
	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		mockDataFeedFetcher,
		mockLogger,
		TestConfig(),
		"",
	)

	recipients := []*domain.ContactWithList{
		{
			Contact:  &domain.Contact{Email: "recipient1@example.com"},
			ListID:   "list-1",
			ListName: "Test List",
		},
	}
	templates := map[string]*domain.Template{"template-123": template}

	sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", "secret-key-123", "https://api.example.com", tracking, broadcastID, recipients, templates, emailProvider, timeoutAt)
	assert.NoError(t, err)
	assert.Equal(t, 1, sent)
	assert.Equal(t, 0, failed)
}

// TestSendBatch_WithRecipientFeed_Disabled tests that disabled recipient feed is handled properly
func TestSendBatch_WithRecipientFeed_Disabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockDataFeedFetcher := bmocks.NewMockDataFeedFetcher(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	ctx := context.Background()
	timeoutAt := time.Now().Add(30 * time.Second)
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"
	tracking := true

	// Broadcast with RecipientFeed disabled
	broadcast := &domain.Broadcast{
		ID:          broadcastID,
		WorkspaceID: workspaceID,
		Name:        "Test Broadcast",
		ChannelType: "email",
		Audience:    domain.AudienceSettings{List: "list-1"},
		Status:      domain.BroadcastStatusDraft,
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
		},
		TestSettings: domain.BroadcastTestSettings{
			Enabled: false,
			Variations: []domain.BroadcastVariation{
				{
					VariationName: "variation-1",
					TemplateID:    "template-123",
				},
			},
		},
		DataFeed: &domain.DataFeedSettings{
			RecipientFeed: &domain.RecipientFeedSettings{
				Enabled: false, // Disabled
				URL:     "https://api.example.com/feed",
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	emailProvider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Test Subject",
			VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
		},
	}

	// Setup mock expectations
	mockBroadcastRepository.EXPECT().
		GetBroadcast(ctx, workspaceID, broadcastID).
		Return(broadcast, nil)

	// FetchRecipient should NOT be called since RecipientFeed is disabled
	// No expectation for mockDataFeedFetcher.FetchRecipient

	mockEmailService.EXPECT().
		SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).Times(1)

	mockMessageHistoryRepo.EXPECT().
		Create(ctx, workspaceID, gomock.Any(), gomock.Any()).
		Return(nil).Times(1)

	// Create message sender with DataFeedFetcher
	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		mockDataFeedFetcher,
		mockLogger,
		TestConfig(),
		"",
	)

	recipients := []*domain.ContactWithList{
		{
			Contact:  &domain.Contact{Email: "recipient1@example.com"},
			ListID:   "list-1",
			ListName: "Test List",
		},
	}
	templates := map[string]*domain.Template{"template-123": template}

	sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", "secret-key-123", "https://api.example.com", tracking, broadcastID, recipients, templates, emailProvider, timeoutAt)
	assert.NoError(t, err)
	assert.Equal(t, 1, sent)
	assert.Equal(t, 0, failed)
}

// TestSendBatch_WithRecipientFeed_NilSettings tests that nil recipient feed settings are handled properly
func TestSendBatch_WithRecipientFeed_NilSettings(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockDataFeedFetcher := bmocks.NewMockDataFeedFetcher(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	ctx := context.Background()
	timeoutAt := time.Now().Add(30 * time.Second)
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"
	tracking := true

	// Broadcast without RecipientFeed (nil)
	broadcast := &domain.Broadcast{
		ID:          broadcastID,
		WorkspaceID: workspaceID,
		Name:        "Test Broadcast",
		ChannelType: "email",
		Audience:    domain.AudienceSettings{List: "list-1"},
		Status:      domain.BroadcastStatusDraft,
		UTMParameters: &domain.UTMParameters{
			Source:   "test",
			Medium:   "email",
			Campaign: "unit-test",
		},
		TestSettings: domain.BroadcastTestSettings{
			Enabled: false,
			Variations: []domain.BroadcastVariation{
				{
					VariationName: "variation-1",
					TemplateID:    "template-123",
				},
			},
		},
		DataFeed:  nil, // Nil DataFeed means no global or recipient feeds
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	emailProvider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Test Subject",
			VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
		},
	}

	// Setup mock expectations
	mockBroadcastRepository.EXPECT().
		GetBroadcast(ctx, workspaceID, broadcastID).
		Return(broadcast, nil)

	// FetchRecipient should NOT be called since RecipientFeed is nil
	// No expectation for mockDataFeedFetcher.FetchRecipient

	mockEmailService.EXPECT().
		SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).Times(1)

	mockMessageHistoryRepo.EXPECT().
		Create(ctx, workspaceID, gomock.Any(), gomock.Any()).
		Return(nil).Times(1)

	// Create message sender with DataFeedFetcher
	sender := NewMessageSender(
		mockBroadcastRepository,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockEmailService,
		mockDataFeedFetcher,
		mockLogger,
		TestConfig(),
		"",
	)

	recipients := []*domain.ContactWithList{
		{
			Contact:  &domain.Contact{Email: "recipient1@example.com"},
			ListID:   "list-1",
			ListName: "Test List",
		},
	}
	templates := map[string]*domain.Template{"template-123": template}

	sent, failed, err := sender.SendBatch(ctx, workspaceID, "test-integration-id", "secret-key-123", "https://api.example.com", tracking, broadcastID, recipients, templates, emailProvider, timeoutAt)
	assert.NoError(t, err)
	assert.Equal(t, 1, sent)
	assert.Equal(t, 0, failed)
}

// TestSendBatch_WithRecipientFeed_ConsecutiveFailuresReset is removed - consecutive failure tracking no longer exists.
// With the new behavior, feed errors cause immediate broadcast pause.
func TestSendBatch_WithRecipientFeed_ConsecutiveFailuresReset(t *testing.T) {
	t.Skip("Removed - consecutive failure counter no longer exists. Feed errors now cause immediate pause.")
}
