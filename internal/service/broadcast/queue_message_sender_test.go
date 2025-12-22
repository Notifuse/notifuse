package broadcast

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewQueueMessageSender(t *testing.T) {
	t.Run("creates sender with all dependencies", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		sender := NewQueueMessageSender(
			mockQueueRepo,
			mockBroadcastRepo,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockLogger,
			nil,
			"https://api.example.com",
		)

		require.NotNil(t, sender)
		assert.Implements(t, (*MessageSender)(nil), sender)
	})

	t.Run("uses default config when nil provided", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		sender := NewQueueMessageSender(
			mockQueueRepo,
			mockBroadcastRepo,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockLogger,
			nil, // nil config
			"https://api.example.com",
		)

		require.NotNil(t, sender)
	})

	t.Run("initializes circuit breaker when enabled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		config := &Config{
			EnableCircuitBreaker:    true,
			CircuitBreakerThreshold: 5,
			CircuitBreakerCooldown:  30 * time.Second,
		}

		sender := NewQueueMessageSender(
			mockQueueRepo,
			mockBroadcastRepo,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockLogger,
			config,
			"https://api.example.com",
		)

		require.NotNil(t, sender)

		// Circuit breaker should be initialized
		qms := sender.(*queueMessageSender)
		assert.NotNil(t, qms.circuitBreaker)
	})
}

// Helper functions for creating test data
func createQueueTestTextBlock(id, textContent string) notifuse_mjml.EmailBlock {
	content := textContent
	base := notifuse_mjml.NewBaseBlock(id, notifuse_mjml.MJMLComponentMjText)
	base.Content = &content
	return &notifuse_mjml.MJTextBlock{BaseBlock: base}
}

func createQueueValidTestTree(textBlock notifuse_mjml.EmailBlock) notifuse_mjml.EmailBlock {
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

func TestQueueMessageSender_SendToRecipient(t *testing.T) {
	t.Run("successfully enqueues single email", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Setup logger expectations
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

		emailSender := domain.NewEmailSender("sender@example.com", "Test Sender")
		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{emailSender},
			SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587},
		}

		broadcast := &domain.Broadcast{
			ID:          "broadcast-1",
			WorkspaceID: "workspace-1",
			Name:        "Test Broadcast",
			UTMParameters: &domain.UTMParameters{
				Source:   "test",
				Medium:   "email",
				Campaign: "campaign-1",
			},
		}

		template := &domain.Template{
			ID: "template-1",
			Email: &domain.EmailTemplate{
				SenderID:         emailSender.ID,
				Subject:          "Test Subject",
				VisualEditorTree: createQueueValidTestTree(createQueueTestTextBlock("txt1", "Hello World")),
			},
		}

		// Expect enqueue call
		mockQueueRepo.EXPECT().Enqueue(
			gomock.Any(),
			"workspace-1",
			gomock.Any(),
		).Return(nil)

		sender := NewQueueMessageSender(
			mockQueueRepo,
			mockBroadcastRepo,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockLogger,
			nil,
			"https://api.example.com",
		)

		err := sender.SendToRecipient(
			context.Background(),
			"workspace-1",
			"integration-1",
			true,
			broadcast,
			"msg-1",
			"recipient@example.com",
			template,
			map[string]interface{}{"contact": map[string]interface{}{"email": "recipient@example.com"}},
			emailProvider,
			time.Now().Add(5*time.Minute),
		)

		assert.NoError(t, err)
	})

	t.Run("returns error when circuit breaker open", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

		config := &Config{
			EnableCircuitBreaker:    true,
			CircuitBreakerThreshold: 1,
			CircuitBreakerCooldown:  1 * time.Hour,
		}

		sender := NewQueueMessageSender(
			mockQueueRepo,
			mockBroadcastRepo,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockLogger,
			config,
			"https://api.example.com",
		)

		// Trip the circuit breaker
		qms := sender.(*queueMessageSender)
		qms.circuitBreaker.RecordFailure(errors.New("test error"))

		broadcast := &domain.Broadcast{
			ID:          "broadcast-1",
			WorkspaceID: "workspace-1",
		}

		err := sender.SendToRecipient(
			context.Background(),
			"workspace-1",
			"integration-1",
			true,
			broadcast,
			"msg-1",
			"recipient@example.com",
			&domain.Template{},
			nil,
			nil,
			time.Now().Add(5*time.Minute),
		)

		assert.Error(t, err)
		var broadcastErr *BroadcastError
		assert.True(t, errors.As(err, &broadcastErr))
		assert.Equal(t, ErrCodeCircuitOpen, broadcastErr.Code)
	})

	t.Run("returns error on enqueue failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		emailSender := domain.NewEmailSender("sender@example.com", "Test Sender")
		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{emailSender},
		}

		broadcast := &domain.Broadcast{
			ID:            "broadcast-1",
			WorkspaceID:   "workspace-1",
			UTMParameters: &domain.UTMParameters{},
		}

		template := &domain.Template{
			ID: "template-1",
			Email: &domain.EmailTemplate{
				SenderID:         emailSender.ID,
				Subject:          "Test Subject",
				VisualEditorTree: createQueueValidTestTree(createQueueTestTextBlock("txt1", "Hello")),
			},
		}

		mockQueueRepo.EXPECT().Enqueue(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(errors.New("database error"))

		sender := NewQueueMessageSender(
			mockQueueRepo,
			mockBroadcastRepo,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockLogger,
			nil,
			"https://api.example.com",
		)

		err := sender.SendToRecipient(
			context.Background(),
			"workspace-1",
			"integration-1",
			true,
			broadcast,
			"msg-1",
			"recipient@example.com",
			template,
			map[string]interface{}{},
			emailProvider,
			time.Now().Add(5*time.Minute),
		)

		assert.Error(t, err)
	})

	t.Run("returns error when no sender configured", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{}, // No senders
		}

		broadcast := &domain.Broadcast{
			ID:            "broadcast-1",
			WorkspaceID:   "workspace-1",
			UTMParameters: &domain.UTMParameters{},
		}

		template := &domain.Template{
			ID: "template-1",
			Email: &domain.EmailTemplate{
				SenderID:         "non-existent-sender",
				Subject:          "Test Subject",
				VisualEditorTree: createQueueValidTestTree(createQueueTestTextBlock("txt1", "Hello")),
			},
		}

		sender := NewQueueMessageSender(
			mockQueueRepo,
			mockBroadcastRepo,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockLogger,
			nil,
			"https://api.example.com",
		)

		err := sender.SendToRecipient(
			context.Background(),
			"workspace-1",
			"integration-1",
			true,
			broadcast,
			"msg-1",
			"recipient@example.com",
			template,
			map[string]interface{}{},
			emailProvider,
			time.Now().Add(5*time.Minute),
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no sender configured")
	})
}

func TestQueueMessageSender_SendBatch(t *testing.T) {
	t.Run("successfully enqueues batch of emails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

		emailSender := domain.NewEmailSender("sender@example.com", "Test Sender")
		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{emailSender},
		}

		broadcast := &domain.Broadcast{
			ID:            "broadcast-1",
			WorkspaceID:   "workspace-1",
			Name:          "Test Broadcast",
			UTMParameters: &domain.UTMParameters{Source: "test", Medium: "email"},
		}

		template := &domain.Template{
			ID: "template-1",
			Email: &domain.EmailTemplate{
				SenderID:         emailSender.ID,
				Subject:          "Test Subject",
				VisualEditorTree: createQueueValidTestTree(createQueueTestTextBlock("txt1", "Hello")),
			},
		}

		recipients := []*domain.ContactWithList{
			{
				Contact: &domain.Contact{Email: "user1@example.com"},
				ListID:  "list-1",
			},
			{
				Contact: &domain.Contact{Email: "user2@example.com"},
				ListID:  "list-1",
			},
		}

		mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-1", "broadcast-1").
			Return(broadcast, nil)

		mockQueueRepo.EXPECT().Enqueue(gomock.Any(), "workspace-1", gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, entries []*domain.EmailQueueEntry) error {
				assert.Len(t, entries, 2)
				return nil
			})

		sender := NewQueueMessageSender(
			mockQueueRepo,
			mockBroadcastRepo,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockLogger,
			nil,
			"https://api.example.com",
		)

		sent, failed, err := sender.SendBatch(
			context.Background(),
			"workspace-1",
			"integration-1",
			"secret-key",
			"https://api.example.com",
			true,
			"broadcast-1",
			recipients,
			map[string]*domain.Template{"template-1": template},
			emailProvider,
			time.Now().Add(5*time.Minute),
		)

		assert.NoError(t, err)
		assert.Equal(t, 2, sent)
		assert.Equal(t, 0, failed)
	})

	t.Run("handles empty recipients", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		sender := NewQueueMessageSender(
			mockQueueRepo,
			mockBroadcastRepo,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockLogger,
			nil,
			"https://api.example.com",
		)

		sent, failed, err := sender.SendBatch(
			context.Background(),
			"workspace-1",
			"integration-1",
			"secret-key",
			"https://api.example.com",
			true,
			"broadcast-1",
			[]*domain.ContactWithList{}, // Empty
			nil,
			nil,
			time.Now().Add(5*time.Minute),
		)

		assert.NoError(t, err)
		assert.Equal(t, 0, sent)
		assert.Equal(t, 0, failed)
	})

	t.Run("returns error when circuit breaker open", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

		config := &Config{
			EnableCircuitBreaker:    true,
			CircuitBreakerThreshold: 1,
			CircuitBreakerCooldown:  1 * time.Hour,
		}

		sender := NewQueueMessageSender(
			mockQueueRepo,
			mockBroadcastRepo,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockLogger,
			config,
			"https://api.example.com",
		)

		// Trip the circuit breaker
		qms := sender.(*queueMessageSender)
		qms.circuitBreaker.RecordFailure(errors.New("test error"))

		recipients := []*domain.ContactWithList{
			{Contact: &domain.Contact{Email: "user1@example.com"}},
		}

		sent, failed, err := sender.SendBatch(
			context.Background(),
			"workspace-1",
			"integration-1",
			"secret-key",
			"https://api.example.com",
			true,
			"broadcast-1",
			recipients,
			nil,
			nil,
			time.Now().Add(5*time.Minute),
		)

		assert.Error(t, err)
		assert.Equal(t, 0, sent)
		assert.Equal(t, 1, failed)
	})

	t.Run("handles enqueue failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		emailSender := domain.NewEmailSender("sender@example.com", "Test Sender")
		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{emailSender},
		}

		broadcast := &domain.Broadcast{
			ID:            "broadcast-1",
			WorkspaceID:   "workspace-1",
			UTMParameters: &domain.UTMParameters{},
		}

		template := &domain.Template{
			ID: "template-1",
			Email: &domain.EmailTemplate{
				SenderID:         emailSender.ID,
				Subject:          "Test Subject",
				VisualEditorTree: createQueueValidTestTree(createQueueTestTextBlock("txt1", "Hello")),
			},
		}

		recipients := []*domain.ContactWithList{
			{Contact: &domain.Contact{Email: "user1@example.com"}},
		}

		mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-1", "broadcast-1").
			Return(broadcast, nil)

		mockQueueRepo.EXPECT().Enqueue(gomock.Any(), "workspace-1", gomock.Any()).
			Return(errors.New("database error"))

		sender := NewQueueMessageSender(
			mockQueueRepo,
			mockBroadcastRepo,
			mockMessageHistoryRepo,
			mockTemplateRepo,
			mockLogger,
			nil,
			"https://api.example.com",
		)

		sent, failed, err := sender.SendBatch(
			context.Background(),
			"workspace-1",
			"integration-1",
			"secret-key",
			"https://api.example.com",
			true,
			"broadcast-1",
			recipients,
			map[string]*domain.Template{"template-1": template},
			emailProvider,
			time.Now().Add(5*time.Minute),
		)

		assert.Error(t, err)
		assert.Equal(t, 0, sent)
		assert.Equal(t, 1, failed)
	})
}

func TestQueueMessageSender_SelectTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	sender := NewQueueMessageSender(
		mockQueueRepo,
		mockBroadcastRepo,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockLogger,
		nil,
		"https://api.example.com",
	)

	qms := sender.(*queueMessageSender)
	broadcast := &domain.Broadcast{ID: "broadcast-1"}

	t.Run("returns nil for empty templates", func(t *testing.T) {
		result := qms.selectTemplate(map[string]*domain.Template{}, broadcast)
		assert.Nil(t, result)
	})

	t.Run("returns single template when only one", func(t *testing.T) {
		template := &domain.Template{ID: "template-1"}
		templates := map[string]*domain.Template{
			"template-1": template,
		}

		result := qms.selectTemplate(templates, broadcast)
		assert.NotNil(t, result)
		assert.Equal(t, "template-1", result.ID)
	})

	t.Run("randomly selects for A/B testing", func(t *testing.T) {
		template1 := &domain.Template{ID: "template-1"}
		template2 := &domain.Template{ID: "template-2"}
		templates := map[string]*domain.Template{
			"template-1": template1,
			"template-2": template2,
		}

		// Run multiple times to verify randomness
		selections := make(map[string]int)
		for i := 0; i < 20; i++ {
			result := qms.selectTemplate(templates, broadcast)
			require.NotNil(t, result)
			selections[result.ID]++
		}

		// Both templates should be selected at least once
		assert.Greater(t, selections["template-1"]+selections["template-2"], 0)
	})
}

func TestQueueMessageSender_BuildQueueEntry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	sender := NewQueueMessageSender(
		mockQueueRepo,
		mockBroadcastRepo,
		mockMessageHistoryRepo,
		mockTemplateRepo,
		mockLogger,
		nil,
		"https://api.example.com",
	)

	qms := sender.(*queueMessageSender)

	t.Run("builds entry with all required fields", func(t *testing.T) {
		emailSender := domain.NewEmailSender("sender@example.com", "Test Sender")
		emailProvider := &domain.EmailProvider{
			Kind:               domain.EmailProviderKindSMTP,
			Senders:            []domain.EmailSender{emailSender},
			RateLimitPerMinute: 100,
		}

		broadcast := &domain.Broadcast{
			ID:          "broadcast-1",
			WorkspaceID: "workspace-1",
			UTMParameters: &domain.UTMParameters{
				Source:   "newsletter",
				Medium:   "email",
				Campaign: "weekly",
			},
		}

		template := &domain.Template{
			ID: "template-1",
			Email: &domain.EmailTemplate{
				SenderID:         emailSender.ID,
				Subject:          "Hello {{ contact.name }}",
				VisualEditorTree: createQueueValidTestTree(createQueueTestTextBlock("txt1", "Hello")),
			},
		}

		data := map[string]interface{}{
			"contact": map[string]interface{}{
				"name":  "John",
				"email": "john@example.com",
			},
		}

		entry, err := qms.buildQueueEntry(
			context.Background(),
			"workspace-1",
			"integration-1",
			true,
			broadcast,
			"msg-123",
			"john@example.com",
			template,
			data,
			emailProvider,
		)

		require.NoError(t, err)
		require.NotNil(t, entry)

		assert.NotEmpty(t, entry.ID)
		assert.Equal(t, domain.EmailQueueStatusPending, entry.Status)
		assert.Equal(t, domain.EmailQueuePriorityMarketing, entry.Priority)
		assert.Equal(t, domain.EmailQueueSourceBroadcast, entry.SourceType)
		assert.Equal(t, "broadcast-1", entry.SourceID)
		assert.Equal(t, "integration-1", entry.IntegrationID)
		assert.Equal(t, domain.EmailProviderKindSMTP, entry.ProviderKind)
		assert.Equal(t, "john@example.com", entry.ContactEmail)
		assert.Equal(t, "msg-123", entry.MessageID)
		assert.Equal(t, "template-1", entry.TemplateID)
		assert.Equal(t, "sender@example.com", entry.Payload.FromAddress)
		assert.Equal(t, "Test Sender", entry.Payload.FromName)
		assert.Contains(t, entry.Payload.Subject, "Hello")
		assert.NotEmpty(t, entry.Payload.HTMLContent)
		assert.Equal(t, 100, entry.Payload.RateLimitPerMinute)
		assert.Equal(t, 3, entry.MaxAttempts)
	})

	t.Run("extracts List-Unsubscribe URL from data", func(t *testing.T) {
		emailSender := domain.NewEmailSender("sender@example.com", "Test Sender")
		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{emailSender},
		}

		broadcast := &domain.Broadcast{
			ID:            "broadcast-1",
			UTMParameters: &domain.UTMParameters{},
		}

		template := &domain.Template{
			ID: "template-1",
			Email: &domain.EmailTemplate{
				SenderID:         emailSender.ID,
				Subject:          "Test",
				VisualEditorTree: createQueueValidTestTree(createQueueTestTextBlock("txt1", "Hello")),
			},
		}

		data := map[string]interface{}{
			"oneclick_unsubscribe_url": "https://example.com/unsubscribe?token=abc123",
		}

		entry, err := qms.buildQueueEntry(
			context.Background(),
			"workspace-1",
			"integration-1",
			true,
			broadcast,
			"msg-123",
			"test@example.com",
			template,
			data,
			emailProvider,
		)

		require.NoError(t, err)
		assert.Equal(t, "https://example.com/unsubscribe?token=abc123", entry.Payload.EmailOptions.ListUnsubscribeURL)
	})

	t.Run("returns error when no sender configured", func(t *testing.T) {
		emailProvider := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{}, // No senders
		}

		broadcast := &domain.Broadcast{
			ID:            "broadcast-1",
			UTMParameters: &domain.UTMParameters{},
		}

		template := &domain.Template{
			ID: "template-1",
			Email: &domain.EmailTemplate{
				SenderID: "non-existent",
				Subject:  "Test",
			},
		}

		_, err := qms.buildQueueEntry(
			context.Background(),
			"workspace-1",
			"integration-1",
			true,
			broadcast,
			"msg-123",
			"test@example.com",
			template,
			nil,
			emailProvider,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no sender configured")
	})
}
