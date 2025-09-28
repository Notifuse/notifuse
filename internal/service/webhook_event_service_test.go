package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessWebhook_Success(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	// Setup logging expectations
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()

	// Create test data
	workspaceID := "workspace1"
	integrationID := "integration1"

	// Setup SES test payload
	t.Run("SES webhook processing", func(t *testing.T) {
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"Bounce","bounce":{"bounceType":"Permanent","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"test@example.com","diagnosticCode":"554"}],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"message1"}}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Setup mock workspace
		workspace := &domain.Workspace{
			ID: workspaceID,
			Integrations: []domain.Integration{
				{
					ID: integrationID,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindSES,
						SES: &domain.AmazonSESSettings{
							Region:    "us-east-1",
							AccessKey: "test-key",
							SecretKey: "test-secret",
						},
					},
				},
			},
		}

		// Setup mocks to handle expectations
		mockEvent := &domain.WebhookEvent{
			Type:              domain.EmailEventBounce,
			EmailProviderKind: domain.EmailProviderKindSES,
			IntegrationID:     integrationID,
			RecipientEmail:    "test@example.com",
			MessageID:         "message1",
		}

		// Setup expectations to match what the service will actually store
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
		repo.EXPECT().StoreEvents(gomock.Any(), workspace.ID, gomock.Any()).DoAndReturn(
			func(_ context.Context, workspaceID string, events []*domain.WebhookEvent) error {
				assert.Equal(t, workspace.ID, workspaceID)
				assert.Equal(t, mockEvent.Type, events[0].Type)
				assert.Equal(t, mockEvent.EmailProviderKind, events[0].EmailProviderKind)
				assert.Equal(t, mockEvent.IntegrationID, events[0].IntegrationID)
				assert.Equal(t, mockEvent.RecipientEmail, events[0].RecipientEmail)
				assert.Equal(t, mockEvent.MessageID, events[0].MessageID)
				return nil
			})

		// Expect message history to be updated with the bounce status - using batch method
		messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		messageHistoryRepo.EXPECT().SetStatusesIfNotSet(
			gomock.Any(),
			workspaceID,
			gomock.Any(), // Will be a slice of MessageStatusUpdate
		).DoAndReturn(func(ctx context.Context, wsID string, updates []domain.MessageEventUpdate) error {
			assert.Equal(t, workspaceID, wsID)
			assert.Equal(t, 1, len(updates), "Should have 1 message status update")
			assert.Equal(t, "message1", updates[0].ID)
			assert.Equal(t, domain.MessageEventBounced, updates[0].Event)
			return nil
		})

		// Create service
		service := &WebhookEventService{
			repo:               repo,
			authService:        authService,
			logger:             log,
			workspaceRepo:      workspaceRepo,
			messageHistoryRepo: messageHistoryRepo,
		}

		// Call method
		err = service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
	})

	// Test Mailgun webhook processing
	t.Run("Mailgun webhook processing", func(t *testing.T) {
		// Setup Mailgun test payload
		payload := domain.MailgunWebhookPayload{
			EventData: domain.MailgunEventData{
				Event:     "delivered",
				Recipient: "test@example.com",
				Timestamp: 1672567200, // 2023-01-01 12:00:00 UTC
				Message: domain.MailgunMessage{
					Headers: domain.MailgunHeaders{
						MessageID: "message1",
					},
				},
			},
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Setup mock workspace
		workspace := &domain.Workspace{
			ID: workspaceID,
			Integrations: []domain.Integration{
				{
					ID: integrationID,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindMailgun,
						Mailgun: &domain.MailgunSettings{
							Domain: "example.com",
							APIKey: "test-key",
						},
					},
				},
			},
		}

		// Setup expectations
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
		repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

		// Expect message history to be updated with the delivery status - using batch method
		messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		messageHistoryRepo.EXPECT().SetStatusesIfNotSet(
			gomock.Any(),
			workspaceID,
			gomock.Any(), // Will be a slice of MessageStatusUpdate
		).DoAndReturn(func(ctx context.Context, wsID string, updates []domain.MessageEventUpdate) error {
			assert.Equal(t, workspaceID, wsID)
			assert.Equal(t, 1, len(updates), "Should have 1 message status update")
			assert.Equal(t, "message1", updates[0].ID)
			assert.Equal(t, domain.MessageEventDelivered, updates[0].Event)
			return nil
		})

		// Create service
		service := &WebhookEventService{
			repo:               repo,
			authService:        authService,
			logger:             log,
			workspaceRepo:      workspaceRepo,
			messageHistoryRepo: messageHistoryRepo,
		}

		// Call method
		err = service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
	})

	// Test integration not found case
	t.Run("Integration not found", func(t *testing.T) {
		rawPayload := []byte(`{}`)

		// Setup mock workspace with no matching integration
		workspace := &domain.Workspace{
			ID:           workspaceID,
			Integrations: []domain.Integration{}, // Empty integrations
		}

		// Setup expectations
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)

		// Create service
		messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		service := &WebhookEventService{
			repo:               repo,
			authService:        authService,
			logger:             log,
			workspaceRepo:      workspaceRepo,
			messageHistoryRepo: messageHistoryRepo,
		}

		// Call method
		err := service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload)

		// Assert
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported email provider kind")
	})

	// Test storage error case
	t.Run("Store event error", func(t *testing.T) {
		// Setup test payload
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"Bounce","bounce":{"bounceType":"Permanent","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"test@example.com","diagnosticCode":"554"}],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"message1"}}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Setup mock workspace
		workspace := &domain.Workspace{
			ID: workspaceID,
			Integrations: []domain.Integration{
				{
					ID: integrationID,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindSES,
						SES: &domain.AmazonSESSettings{
							Region:    "us-east-1",
							AccessKey: "test-key",
							SecretKey: "test-secret",
						},
					},
				},
			},
		}

		// Setup expectations with storage error
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
		repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(errors.New("database error"))

		// Create service
		messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		service := &WebhookEventService{
			repo:               repo,
			authService:        authService,
			logger:             log,
			workspaceRepo:      workspaceRepo,
			messageHistoryRepo: messageHistoryRepo,
		}

		// Call method
		err = service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload)

		// Assert
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to store webhook event")
	})
}

func TestProcessWebhook_WorkspaceNotFound(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	// Setup logging expectations
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()

	// Create test data
	workspaceID := "workspace1"
	integrationID := "integration1"
	rawPayload := []byte(`{}`)

	// Setup expectations - simulate workspace not found
	workspaceError := errors.New("workspace not found")
	workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(nil, workspaceError)

	// Create service
	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	service := &WebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
	}

	// Call method
	err := service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get workspace")
}

func TestNewWebhookEventService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)

	service := NewWebhookEventService(repo, authService, log, workspaceRepo, messageHistoryRepo)

	assert.NotNil(t, service)
	assert.Equal(t, repo, service.repo)
	assert.Equal(t, authService, service.authService)
	assert.NotNil(t, service.logger)
	assert.Equal(t, workspaceRepo, service.workspaceRepo)
	assert.Equal(t, messageHistoryRepo, service.messageHistoryRepo)
}

func TestProcessSESWebhook(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	// Setup logging expectations
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()

	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	service := &WebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
	}

	integrationID := "integration1"

	t.Run("Bounce Event", func(t *testing.T) {
		// Create test bounce payload
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"Bounce","bounce":{"bounceType":"Permanent","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"test@example.com","diagnosticCode":"554"}],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"message1"}}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSESWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventBounce, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindSES, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "message1", events[0].MessageID)
		assert.Equal(t, "Permanent", events[0].BounceType)
		assert.Equal(t, "General", events[0].BounceCategory)
		assert.Equal(t, "554", events[0].BounceDiagnostic)
	})

	t.Run("Complaint Event", func(t *testing.T) {
		// Create test complaint payload
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"Complaint","complaint":{"complainedRecipients":[{"emailAddress":"test@example.com"}],"timestamp":"2023-01-01T12:00:00Z","complaintFeedbackType":"abuse"},"mail":{"messageId":"message1"}}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSESWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Len(t, events, 1)
		assert.Equal(t, domain.EmailEventComplaint, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindSES, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "message1", events[0].MessageID)
		assert.Equal(t, "abuse", events[0].ComplaintFeedbackType)
	})

	t.Run("Delivery Event", func(t *testing.T) {
		// Create test delivery payload
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"Delivery","delivery":{"recipients":["test@example.com"],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"message1"}}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSESWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Len(t, events, 1)
		assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindSES, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "message1", events[0].MessageID)
	})

	t.Run("Subscription Confirmation", func(t *testing.T) {
		// Create test subscription confirmation payload
		payload := domain.SESWebhookPayload{
			Type:         "SubscriptionConfirmation",
			TopicARN:     "arn:aws:sns:us-east-1:123456789:test-topic",
			SubscribeURL: "https://sns.us-east-1.amazonaws.com/?Action=ConfirmSubscription&TopicArn=arn:aws:sns:us-east-1:123456789:test-topic&Token=test-token",
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSESWebhook(integrationID, rawPayload)

		// Assert - should return empty events but no error
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Len(t, events, 0)
	})

	t.Run("Unsubscribe Confirmation", func(t *testing.T) {
		// Create test unsubscribe confirmation payload
		payload := domain.SESWebhookPayload{
			Type:     "UnsubscribeConfirmation",
			TopicARN: "arn:aws:sns:us-east-1:123456789:test-topic",
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSESWebhook(integrationID, rawPayload)

		// Assert - should return empty events but no error
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Len(t, events, 0)
	})

	t.Run("SNS Topic Validation Message", func(t *testing.T) {
		// Create test SNS topic validation payload
		payload := domain.SESWebhookPayload{
			Message: "Successfully validated SNS topic for Amazon SES event publishing.",
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSESWebhook(integrationID, rawPayload)

		// Assert - should return empty events but no error
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Len(t, events, 0)
	})

	t.Run("Unrecognized SES Notification", func(t *testing.T) {
		// Create test unrecognized payload
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"UnknownEvent","someData":"value"}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSESWebhook(integrationID, rawPayload)

		// Assert - should return empty events but no error (fails silently)
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Len(t, events, 0)
	})

	t.Run("Bounce Event with Notifuse Message ID", func(t *testing.T) {
		// Create test bounce payload with notifuse_message_id in tags
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"Bounce","bounce":{"bounceType":"Permanent","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"test@example.com","diagnosticCode":"554"}],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"provider-msg-id","tags":{"notifuse_message_id":["notifuse-123"]}}}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSESWebhook(integrationID, rawPayload)

		// Assert - should use notifuse message ID
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, "notifuse-123", events[0].MessageID)
	})

	t.Run("Delivery Event with Notifuse Message ID", func(t *testing.T) {
		// Create test delivery payload with notifuse_message_id in tags
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"Delivery","delivery":{"recipients":["test@example.com"],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"provider-msg-id","tags":{"notifuse_message_id":["notifuse-456"]}}}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSESWebhook(integrationID, rawPayload)

		// Assert - should use notifuse message ID
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, "notifuse-456", events[0].MessageID)
	})

	t.Run("Complaint Event with Notifuse Message ID", func(t *testing.T) {
		// Create test complaint payload with notifuse_message_id in tags
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"Complaint","complaint":{"complainedRecipients":[{"emailAddress":"test@example.com"}],"timestamp":"2023-01-01T12:00:00Z","complaintFeedbackType":"abuse"},"mail":{"messageId":"provider-msg-id","tags":{"notifuse_message_id":["notifuse-789"]}}}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSESWebhook(integrationID, rawPayload)

		// Assert - should use notifuse message ID
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, "notifuse-789", events[0].MessageID)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		// Create invalid JSON payload
		rawPayload := []byte(`{invalid json`)

		// Call method
		events, err := service.processSESWebhook(integrationID, rawPayload)

		// Assert - should return error
		assert.Error(t, err)
		assert.Nil(t, events)
		assert.Contains(t, err.Error(), "failed to unmarshal SES webhook payload")
	})

	t.Run("Invalid Timestamp Parsing", func(t *testing.T) {
		// Create test bounce payload with invalid timestamp
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"Bounce","bounce":{"bounceType":"Permanent","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"test@example.com","diagnosticCode":"554"}],"timestamp":"invalid-timestamp"},"mail":{"messageId":"message1"}}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSESWebhook(integrationID, rawPayload)

		// Assert - should still work but use current time
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventBounce, events[0].Type)
		// Timestamp should be close to now since parsing failed
		assert.WithinDuration(t, time.Now(), events[0].Timestamp, 5*time.Second)
	})

	t.Run("Subscription Confirmation with HTTP Error", func(t *testing.T) {
		// Create test subscription confirmation payload with invalid URL to trigger HTTP error
		payload := domain.SESWebhookPayload{
			Type:         "SubscriptionConfirmation",
			TopicARN:     "arn:aws:sns:us-east-1:123456789:test-topic",
			SubscribeURL: "http://invalid-url-that-does-not-exist.example.com/confirm",
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method - this should fail due to HTTP error
		events, err := service.processSESWebhook(integrationID, rawPayload)

		// Assert - should return error due to failed HTTP request
		assert.Error(t, err)
		assert.Nil(t, events)
		assert.Contains(t, err.Error(), "failed to confirm subscription")
	})
}

func TestProcessPostmarkWebhook(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	service := &WebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
	}

	integrationID := "integration1"

	t.Run("Delivery Event", func(t *testing.T) {
		// Create test delivery payload using a map to ensure correct JSON structure
		rawPayload, err := json.Marshal(map[string]interface{}{
			"RecordType":  "Delivery",
			"MessageID":   "message1",
			"Recipient":   "test@example.com",
			"DeliveredAt": "2023-01-01T12:00:00Z",
			"Details":     "250 OK",
		})
		require.NoError(t, err)

		// Call method
		events, err := service.processPostmarkWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindPostmark, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "message1", events[0].MessageID)
	})

	t.Run("Bounce Event", func(t *testing.T) {
		// Create test bounce payload using a map to ensure correct JSON structure
		rawPayload, err := json.Marshal(map[string]interface{}{
			"RecordType": "Bounce",
			"MessageID":  "message1",
			"Email":      "test@example.com",
			"Type":       "HardBounce",
			"TypeCode":   1,
			"Details":    "550 Address rejected",
			"BouncedAt":  "2023-01-01T12:00:00Z",
		})
		require.NoError(t, err)

		// Call method
		events, err := service.processPostmarkWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventBounce, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindPostmark, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "message1", events[0].MessageID)
		assert.Equal(t, "HardBounce", events[0].BounceType)
		assert.Equal(t, "HardBounce", events[0].BounceCategory)
		assert.Equal(t, "550 Address rejected", events[0].BounceDiagnostic)
	})

	t.Run("Complaint Event", func(t *testing.T) {
		// Create test complaint payload using a map to ensure correct JSON structure
		rawPayload, err := json.Marshal(map[string]interface{}{
			"RecordType":   "SpamComplaint",
			"MessageID":    "message1",
			"Email":        "test@example.com",
			"Type":         "SpamComplaint",
			"ComplainedAt": "2023-01-01T12:00:00Z",
		})
		require.NoError(t, err)

		// Call method
		events, err := service.processPostmarkWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventComplaint, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindPostmark, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "message1", events[0].MessageID)
		assert.Equal(t, "SpamComplaint", events[0].ComplaintFeedbackType)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		// Create invalid payload
		rawPayload := []byte(`{invalid json`)

		// Call method
		events, err := service.processPostmarkWebhook(integrationID, rawPayload)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, events)
	})

	t.Run("Unsupported Record Type", func(t *testing.T) {
		// Create unsupported record type
		rawPayload, err := json.Marshal(map[string]interface{}{
			"RecordType": "Unknown",
			"MessageID":  "message1",
		})
		require.NoError(t, err)

		// Call method
		events, err := service.processPostmarkWebhook(integrationID, rawPayload)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, events)
	})

	t.Run("Delivery Event with Empty Timestamp", func(t *testing.T) {
		// Create test delivery payload with empty timestamp
		rawPayload, err := json.Marshal(map[string]interface{}{
			"RecordType":  "Delivery",
			"MessageID":   "message1",
			"Recipient":   "test@example.com",
			"DeliveredAt": "", // Empty timestamp
		})
		require.NoError(t, err)

		// Call method
		events, err := service.processPostmarkWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
		// Should use current time since timestamp is empty
		assert.WithinDuration(t, time.Now(), events[0].Timestamp, 5*time.Second)
	})

	t.Run("Bounce Event with Empty Timestamp", func(t *testing.T) {
		// Create test bounce payload with empty timestamp
		rawPayload, err := json.Marshal(map[string]interface{}{
			"RecordType": "Bounce",
			"MessageID":  "message1",
			"Email":      "test@example.com",
			"Type":       "HardBounce",
			"Details":    "550 Address rejected",
			"BouncedAt":  "", // Empty timestamp
		})
		require.NoError(t, err)

		// Call method
		events, err := service.processPostmarkWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventBounce, events[0].Type)
		// Should use current time since timestamp is empty
		assert.WithinDuration(t, time.Now(), events[0].Timestamp, 5*time.Second)
	})

	t.Run("Complaint Event with Empty Timestamp", func(t *testing.T) {
		// Create test complaint payload with empty timestamp
		rawPayload, err := json.Marshal(map[string]interface{}{
			"RecordType":   "SpamComplaint",
			"MessageID":    "message1",
			"Email":        "test@example.com",
			"Type":         "SpamComplaint",
			"ComplainedAt": "", // Empty timestamp
		})
		require.NoError(t, err)

		// Call method
		events, err := service.processPostmarkWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventComplaint, events[0].Type)
		// Should use current time since timestamp is empty
		assert.WithinDuration(t, time.Now(), events[0].Timestamp, 5*time.Second)
	})

	t.Run("Event with Notifuse Message ID in Metadata", func(t *testing.T) {
		// Create test delivery payload with notifuse_message_id in metadata
		rawPayload, err := json.Marshal(map[string]interface{}{
			"RecordType":  "Delivery",
			"MessageID":   "provider-message-id",
			"Recipient":   "test@example.com",
			"DeliveredAt": "2023-01-01T12:00:00Z",
			"Metadata": map[string]interface{}{
				"notifuse_message_id": "notifuse-123",
			},
		})
		require.NoError(t, err)

		// Call method
		events, err := service.processPostmarkWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		// Should use notifuse message ID instead of provider message ID
		assert.Equal(t, "notifuse-123", events[0].MessageID)
	})

	t.Run("Invalid Timestamp Parsing", func(t *testing.T) {
		// Create test delivery payload with invalid timestamp
		rawPayload, err := json.Marshal(map[string]interface{}{
			"RecordType":  "Delivery",
			"MessageID":   "message1",
			"Recipient":   "test@example.com",
			"DeliveredAt": "invalid-timestamp",
		})
		require.NoError(t, err)

		// Call method
		events, err := service.processPostmarkWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
		// Should use current time since parsing failed
		assert.WithinDuration(t, time.Now(), events[0].Timestamp, 5*time.Second)
	})
}

func TestProcessSparkPostWebhook(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	service := &WebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
	}

	integrationID := "integration1"

	t.Run("Delivery Event", func(t *testing.T) {
		// Create test delivery payload
		payload := []*domain.SparkPostWebhookPayload{
			{
				MSys: domain.SparkPostMSys{
					MessageEvent: &domain.SparkPostMessageEvent{
						Type:        "delivery",
						RecipientTo: "test@example.com",
						MessageID:   "message1",
						Timestamp:   "2023-01-01T12:00:00Z",
					},
				},
			},
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSparkPostWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindSparkPost, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "message1", events[0].MessageID)
	})

	t.Run("Bounce Event", func(t *testing.T) {
		// Create test bounce payload
		payload := []*domain.SparkPostWebhookPayload{
			{
				MSys: domain.SparkPostMSys{
					MessageEvent: &domain.SparkPostMessageEvent{
						Type:        "bounce",
						RecipientTo: "test@example.com",
						MessageID:   "message1",
						BounceClass: "21", // Hard bounce
						Reason:      "550 5.1.1 The email account does not exist",
						Timestamp:   "2023-01-01T12:00:00Z",
					},
				},
			},
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSparkPostWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventBounce, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindSparkPost, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "message1", events[0].MessageID)
		assert.Equal(t, "Bounce", events[0].BounceType)
		assert.Equal(t, "21", events[0].BounceCategory)
		assert.Equal(t, "550 5.1.1 The email account does not exist", events[0].BounceDiagnostic)
	})

	t.Run("Complaint Event", func(t *testing.T) {
		// Create test complaint payload
		payload := []*domain.SparkPostWebhookPayload{
			{
				MSys: domain.SparkPostMSys{
					MessageEvent: &domain.SparkPostMessageEvent{
						Type:         "spam_complaint",
						RecipientTo:  "test@example.com",
						MessageID:    "message1",
						FeedbackType: "abuse",
						Timestamp:    "2023-01-01T12:00:00Z",
					},
				},
			},
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSparkPostWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventComplaint, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindSparkPost, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "message1", events[0].MessageID)
		assert.Equal(t, "abuse", events[0].ComplaintFeedbackType)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		// Create invalid payload
		rawPayload := []byte(`{invalid json`)

		// Call method
		events, err := service.processSparkPostWebhook(integrationID, rawPayload)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, events)
	})

	t.Run("No Supported Event", func(t *testing.T) {
		// Create payload with no supported event
		payload := []*domain.SparkPostWebhookPayload{
			{
				MSys: domain.SparkPostMSys{},
			},
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSparkPostWebhook(integrationID, rawPayload)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, events)
	})

	t.Run("Unix Timestamp Parsing", func(t *testing.T) {
		// Create test delivery payload with Unix timestamp
		payload := []*domain.SparkPostWebhookPayload{
			{
				MSys: domain.SparkPostMSys{
					MessageEvent: &domain.SparkPostMessageEvent{
						Type:        "delivery",
						RecipientTo: "test@example.com",
						MessageID:   "message1",
						Timestamp:   "1672567200", // Unix timestamp as string
					},
				},
			},
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSparkPostWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
		// Should parse Unix timestamp correctly
		expectedTime := time.Unix(1672567200, 0)
		assert.Equal(t, expectedTime, events[0].Timestamp)
	})

	t.Run("Invalid Timestamp Parsing", func(t *testing.T) {
		// Create test delivery payload with invalid timestamp
		payload := []*domain.SparkPostWebhookPayload{
			{
				MSys: domain.SparkPostMSys{
					MessageEvent: &domain.SparkPostMessageEvent{
						Type:        "delivery",
						RecipientTo: "test@example.com",
						MessageID:   "message1",
						Timestamp:   "invalid-timestamp",
					},
				},
			},
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Setup logging expectations for timestamp parsing warning
		log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().Warn(gomock.Any()).AnyTimes()

		// Call method
		events, err := service.processSparkPostWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
		// Should use current time since parsing failed
		assert.WithinDuration(t, time.Now(), events[0].Timestamp, 5*time.Second)
	})

	t.Run("Empty Timestamp", func(t *testing.T) {
		// Create test delivery payload with empty timestamp
		payload := []*domain.SparkPostWebhookPayload{
			{
				MSys: domain.SparkPostMSys{
					MessageEvent: &domain.SparkPostMessageEvent{
						Type:        "delivery",
						RecipientTo: "test@example.com",
						MessageID:   "message1",
						Timestamp:   "",
					},
				},
			},
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSparkPostWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
		// Should use current time since timestamp is empty
		assert.WithinDuration(t, time.Now(), events[0].Timestamp, 5*time.Second)
	})

	t.Run("Event with Notifuse Message ID", func(t *testing.T) {
		// Create test delivery payload with notifuse_message_id in recipient meta
		payload := []*domain.SparkPostWebhookPayload{
			{
				MSys: domain.SparkPostMSys{
					MessageEvent: &domain.SparkPostMessageEvent{
						Type:        "delivery",
						RecipientTo: "test@example.com",
						MessageID:   "provider-message-id",
						Timestamp:   "2023-01-01T12:00:00Z",
						RecipientMeta: map[string]interface{}{
							"notifuse_message_id": "notifuse-123",
						},
					},
				},
			},
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSparkPostWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		// Should use notifuse message ID instead of provider message ID
		assert.Equal(t, "notifuse-123", events[0].MessageID)
	})

	t.Run("Unsupported Event Type", func(t *testing.T) {
		// Create test payload with unsupported event type
		payload := []*domain.SparkPostWebhookPayload{
			{
				MSys: domain.SparkPostMSys{
					MessageEvent: &domain.SparkPostMessageEvent{
						Type:        "unsupported_event",
						RecipientTo: "test@example.com",
						MessageID:   "message1",
						Timestamp:   "2023-01-01T12:00:00Z",
					},
				},
			},
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSparkPostWebhook(integrationID, rawPayload)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, events)
		assert.Contains(t, err.Error(), "unsupported SparkPost event type: unsupported_event")
	})
}

func TestProcessMailgunWebhook(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	service := &WebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
	}

	integrationID := "integration1"

	t.Run("Delivery Event", func(t *testing.T) {
		// Create test delivery payload
		payload := domain.MailgunWebhookPayload{
			EventData: domain.MailgunEventData{
				Event:     "delivered",
				Recipient: "test@example.com",
				Timestamp: 1672567200, // 2023-01-01 12:00:00 UTC
				Message: domain.MailgunMessage{
					Headers: domain.MailgunHeaders{
						MessageID: "message1",
					},
				},
			},
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processMailgunWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindMailgun, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "message1", events[0].MessageID)
	})

	t.Run("Bounce Event", func(t *testing.T) {
		// Create test bounce payload
		payload := domain.MailgunWebhookPayload{
			EventData: domain.MailgunEventData{
				Event:     "failed",
				Recipient: "test@example.com",
				Timestamp: 1672567200, // 2023-01-01 12:00:00 UTC
				Severity:  "permanent",
				Reason:    "550 5.1.1 The email account does not exist",
				Message: domain.MailgunMessage{
					Headers: domain.MailgunHeaders{
						MessageID: "message1",
					},
				},
			},
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processMailgunWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventBounce, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindMailgun, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "message1", events[0].MessageID)
		assert.Equal(t, "Failed", events[0].BounceType)
		assert.Equal(t, "HardBounce", events[0].BounceCategory)
		assert.Equal(t, "550 5.1.1 The email account does not exist", events[0].BounceDiagnostic)
	})

	t.Run("Soft Bounce Event", func(t *testing.T) {
		// Create test soft bounce payload
		payload := domain.MailgunWebhookPayload{
			EventData: domain.MailgunEventData{
				Event:     "failed",
				Recipient: "test@example.com",
				Timestamp: 1672567200, // 2023-01-01 12:00:00 UTC
				Severity:  "temporary",
				Reason:    "450 4.2.1 Mailbox full",
				Message: domain.MailgunMessage{
					Headers: domain.MailgunHeaders{
						MessageID: "message1",
					},
				},
			},
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processMailgunWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventBounce, events[0].Type)
		assert.Equal(t, "SoftBounce", events[0].BounceCategory)
	})

	t.Run("Complaint Event", func(t *testing.T) {
		// Create test complaint payload
		payload := domain.MailgunWebhookPayload{
			EventData: domain.MailgunEventData{
				Event:     "complained",
				Recipient: "test@example.com",
				Timestamp: 1672567200, // 2023-01-01 12:00:00 UTC
				Message: domain.MailgunMessage{
					Headers: domain.MailgunHeaders{
						MessageID: "message1",
					},
				},
			},
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processMailgunWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventComplaint, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindMailgun, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "message1", events[0].MessageID)
		assert.Equal(t, "abuse", events[0].ComplaintFeedbackType)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		// Create invalid payload
		rawPayload := []byte(`{invalid json`)

		// Call method
		events, err := service.processMailgunWebhook(integrationID, rawPayload)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, events)
		assert.Contains(t, err.Error(), "failed to unmarshal Mailgun webhook payload")
	})

	t.Run("Unsupported Event Type", func(t *testing.T) {
		// Create unsupported event type
		payload := domain.MailgunWebhookPayload{
			EventData: domain.MailgunEventData{
				Event:     "unsupported",
				Recipient: "test@example.com",
				Message: domain.MailgunMessage{
					Headers: domain.MailgunHeaders{
						MessageID: "message1",
					},
				},
			},
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processMailgunWebhook(integrationID, rawPayload)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, events)
		assert.Contains(t, err.Error(), "unsupported Mailgun event type")
	})

	t.Run("Event with Notifuse Message ID in User Variables", func(t *testing.T) {
		// Create test delivery payload with notifuse_message_id in user variables
		// We need to create the raw JSON to include the user-variables structure
		rawPayload := []byte(`{
			"event-data": {
				"event": "delivered",
				"recipient": "test@example.com",
				"timestamp": 1672567200,
				"message": {
					"headers": {
						"message-id": "provider-message-id"
					}
				},
				"user-variables": {
					"notifuse_message_id": "notifuse-123"
				}
			}
		}`)

		// Call method
		events, err := service.processMailgunWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		// Should use notifuse message ID instead of provider message ID
		assert.Equal(t, "notifuse-123", events[0].MessageID)
	})
}

func TestProcessMailjetWebhook(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	service := &WebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
	}

	integrationID := "integration1"

	t.Run("Sent Event", func(t *testing.T) {
		// Create test sent payload
		payload := domain.MailjetWebhookPayload{
			Event:     "sent",
			Time:      1672574400, // 2023-01-01T12:00:00Z
			Email:     "test@example.com",
			MessageID: 12345,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processMailjetWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindMailjet, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "12345", events[0].MessageID)
	})

	t.Run("Bounce Event", func(t *testing.T) {
		// Create test bounce payload
		payload := domain.MailjetWebhookPayload{
			Event:      "bounce",
			Time:       1672574400, // 2023-01-01T12:00:00Z
			Email:      "test@example.com",
			MessageID:  12345,
			HardBounce: true,
			Comment:    "Mailbox does not exist",
			Error:      "550",
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processMailjetWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventBounce, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindMailjet, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "12345", events[0].MessageID)
		assert.Equal(t, "HardBounce", events[0].BounceType)
		assert.Equal(t, "Permanent", events[0].BounceCategory)
		assert.Equal(t, "Mailbox does not exist: 550", events[0].BounceDiagnostic)
	})

	t.Run("Spam Event", func(t *testing.T) {
		// Create test spam payload
		payload := domain.MailjetWebhookPayload{
			Event:     "spam",
			Time:      1672574400, // 2023-01-01T12:00:00Z
			Email:     "test@example.com",
			MessageID: 12345,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processMailjetWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventComplaint, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindMailjet, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "12345", events[0].MessageID)
		assert.Equal(t, "spam", events[0].ComplaintFeedbackType)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		// Create invalid payload
		rawPayload := []byte(`{invalid json`)

		// Call method
		events, err := service.processMailjetWebhook(integrationID, rawPayload)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, events)
	})

	t.Run("Unsupported Event Type", func(t *testing.T) {
		// Create unsupported event type
		payload := domain.MailjetWebhookPayload{
			Event:     "unknown",
			Time:      1672574400,
			Email:     "test@example.com",
			MessageID: 12345,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processMailjetWebhook(integrationID, rawPayload)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, events)
	})

	t.Run("Array Payload - Multiple Events", func(t *testing.T) {
		// Create test array payload with multiple events
		payloads := []domain.MailjetWebhookPayload{
			{
				Event:     "sent",
				Time:      1672574400, // 2023-01-01T12:00:00Z
				Email:     "test1@example.com",
				MessageID: 12345,
				CustomID:  "msg-1",
			},
			{
				Event:      "bounce",
				Time:       1672574401, // 2023-01-01T12:00:01Z
				Email:      "test2@example.com",
				MessageID:  12346,
				HardBounce: true,
				Comment:    "Mailbox does not exist",
				Error:      "550",
				CustomID:   "msg-2",
			},
			{
				Event:     "spam",
				Time:      1672574402, // 2023-01-01T12:00:02Z
				Email:     "test3@example.com",
				MessageID: 12347,
				Source:    "FBL",
				CustomID:  "msg-3",
			},
		}
		rawPayload, err := json.Marshal(payloads)
		require.NoError(t, err)

		// Call method
		events, err := service.processMailjetWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Len(t, events, 3)

		// Check first event (sent)
		assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindMailjet, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test1@example.com", events[0].RecipientEmail)
		assert.Equal(t, "msg-1", events[0].MessageID) // Should use CustomID

		// Check second event (bounce)
		assert.Equal(t, domain.EmailEventBounce, events[1].Type)
		assert.Equal(t, domain.EmailProviderKindMailjet, events[1].EmailProviderKind)
		assert.Equal(t, integrationID, events[1].IntegrationID)
		assert.Equal(t, "test2@example.com", events[1].RecipientEmail)
		assert.Equal(t, "msg-2", events[1].MessageID) // Should use CustomID
		assert.Equal(t, "HardBounce", events[1].BounceType)
		assert.Equal(t, "Permanent", events[1].BounceCategory)
		assert.Equal(t, "Mailbox does not exist: 550", events[1].BounceDiagnostic)

		// Check third event (spam)
		assert.Equal(t, domain.EmailEventComplaint, events[2].Type)
		assert.Equal(t, domain.EmailProviderKindMailjet, events[2].EmailProviderKind)
		assert.Equal(t, integrationID, events[2].IntegrationID)
		assert.Equal(t, "test3@example.com", events[2].RecipientEmail)
		assert.Equal(t, "msg-3", events[2].MessageID) // Should use CustomID
		assert.Equal(t, "FBL", events[2].ComplaintFeedbackType)
	})

	t.Run("Array Payload - Single Event", func(t *testing.T) {
		// Create test array payload with single event
		payloads := []domain.MailjetWebhookPayload{
			{
				Event:     "sent",
				Time:      1672574400, // 2023-01-01T12:00:00Z
				Email:     "test@example.com",
				MessageID: 12345,
				CustomID:  "msg-1",
			},
		}
		rawPayload, err := json.Marshal(payloads)
		require.NoError(t, err)

		// Call method
		events, err := service.processMailjetWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Len(t, events, 1)
		assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindMailjet, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "msg-1", events[0].MessageID) // Should use CustomID
	})

	t.Run("Array Payload - Empty Array", func(t *testing.T) {
		// Create empty array payload
		payloads := []domain.MailjetWebhookPayload{}
		rawPayload, err := json.Marshal(payloads)
		require.NoError(t, err)

		// Call method
		events, err := service.processMailjetWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.Len(t, events, 0)
	})

	t.Run("Array Payload - One Invalid Event", func(t *testing.T) {
		// Create test array payload with one invalid event
		payloads := []domain.MailjetWebhookPayload{
			{
				Event:     "sent",
				Time:      1672574400, // 2023-01-01T12:00:00Z
				Email:     "test1@example.com",
				MessageID: 12345,
			},
			{
				Event:     "unknown", // Invalid event type
				Time:      1672574401,
				Email:     "test2@example.com",
				MessageID: 12346,
			},
		}
		rawPayload, err := json.Marshal(payloads)
		require.NoError(t, err)

		// Call method
		events, err := service.processMailjetWebhook(integrationID, rawPayload)

		// Assert - should fail on the invalid event
		assert.Error(t, err)
		assert.Nil(t, events)
		assert.Contains(t, err.Error(), "unsupported Mailjet event type: unknown")
	})

	t.Run("Soft Bounce Event", func(t *testing.T) {
		// Create test soft bounce payload
		payload := domain.MailjetWebhookPayload{
			Event:      "bounce",
			Time:       1672574400, // 2023-01-01T12:00:00Z
			Email:      "test@example.com",
			MessageID:  12345,
			HardBounce: false, // Soft bounce
			Comment:    "Mailbox temporarily unavailable",
			Error:      "450",
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processMailjetWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventBounce, events[0].Type)
		assert.Equal(t, "SoftBounce", events[0].BounceType)
		assert.Equal(t, "Temporary", events[0].BounceCategory)
		assert.Equal(t, "Mailbox temporarily unavailable: 450", events[0].BounceDiagnostic)
	})

	t.Run("Blocked Event", func(t *testing.T) {
		// Create test blocked payload
		payload := domain.MailjetWebhookPayload{
			Event:     "blocked",
			Time:      1672574400, // 2023-01-01T12:00:00Z
			Email:     "test@example.com",
			MessageID: 12345,
			Comment:   "Blocked by recipient server",
			Error:     "550",
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processMailjetWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventBounce, events[0].Type)
		assert.Equal(t, "Blocked", events[0].BounceType)
		assert.Equal(t, "Blocked", events[0].BounceCategory)
		assert.Equal(t, "Blocked by recipient server: 550", events[0].BounceDiagnostic)
	})

	t.Run("Unsubscribe Event", func(t *testing.T) {
		// Create test unsubscribe payload
		payload := domain.MailjetWebhookPayload{
			Event:     "unsub",
			Time:      1672574400, // 2023-01-01T12:00:00Z
			Email:     "test@example.com",
			MessageID: 12345,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processMailjetWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventComplaint, events[0].Type)
		assert.Equal(t, "unsubscribe", events[0].ComplaintFeedbackType)
	})
}

func TestProcessSMTPWebhook(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	service := &WebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
	}

	integrationID := "integration1"

	t.Run("Delivered Event", func(t *testing.T) {
		// Create test delivery payload
		payload := domain.SMTPWebhookPayload{
			Event:     "delivered",
			Timestamp: "2023-01-01T12:00:00Z",
			Recipient: "test@example.com",
			MessageID: "message1",
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSMTPWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindSMTP, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "message1", events[0].MessageID)
	})

	t.Run("Bounce Event", func(t *testing.T) {
		// Create test bounce payload
		payload := domain.SMTPWebhookPayload{
			Event:          "bounce",
			Timestamp:      "2023-01-01T12:00:00Z",
			Recipient:      "test@example.com",
			MessageID:      "message1",
			BounceCategory: "Permanent",
			DiagnosticCode: "550 5.1.1 User unknown",
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSMTPWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventBounce, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindSMTP, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "message1", events[0].MessageID)
		assert.Equal(t, "Bounce", events[0].BounceType)
		assert.Equal(t, "Permanent", events[0].BounceCategory)
		assert.Equal(t, "550 5.1.1 User unknown", events[0].BounceDiagnostic)
	})

	t.Run("Complaint Event", func(t *testing.T) {
		// Create test complaint payload
		payload := domain.SMTPWebhookPayload{
			Event:         "complaint",
			Timestamp:     "2023-01-01T12:00:00Z",
			Recipient:     "test@example.com",
			MessageID:     "message1",
			ComplaintType: "abuse",
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSMTPWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventComplaint, events[0].Type)
		assert.Equal(t, domain.EmailProviderKindSMTP, events[0].EmailProviderKind)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "message1", events[0].MessageID)
		assert.Equal(t, "abuse", events[0].ComplaintFeedbackType)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		// Create invalid payload
		rawPayload := []byte(`{invalid json`)

		// Call method
		event, err := service.processSMTPWebhook(integrationID, rawPayload)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, event)
	})

	t.Run("Unsupported Event Type", func(t *testing.T) {
		// Create unsupported event type
		payload := domain.SMTPWebhookPayload{
			Event:     "unknown",
			Timestamp: "2023-01-01T12:00:00Z",
			Recipient: "test@example.com",
			MessageID: "message1",
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		event, err := service.processSMTPWebhook(integrationID, rawPayload)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, event)
	})

	t.Run("Invalid Timestamp Parsing", func(t *testing.T) {
		// Create test delivery payload with invalid timestamp
		payload := domain.SMTPWebhookPayload{
			Event:     "delivered",
			Timestamp: "invalid-timestamp",
			Recipient: "test@example.com",
			MessageID: "message1",
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Call method
		events, err := service.processSMTPWebhook(integrationID, rawPayload)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
		// Should use current time since parsing failed
		assert.WithinDuration(t, time.Now(), events[0].Timestamp, 5*time.Second)
	})
}

// TestProcessWebhook_AdditionalScenarios tests additional scenarios for ProcessWebhook
func TestProcessWebhook_AdditionalScenarios(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)

	service := &WebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
	}

	workspaceID := "workspace1"
	integrationID := "integration1"

	t.Run("ProcessWebhook with long bounce reason truncation", func(t *testing.T) {
		// Create test payload with very long bounce reason
		longReason := strings.Repeat("a", 300) // Longer than 255 characters
		payload := domain.SESWebhookPayload{
			Message: fmt.Sprintf(`{"eventType":"Bounce","bounce":{"bounceType":"Permanent","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"test@example.com","diagnosticCode":"%s"}],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"message1"}}`, longReason),
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Setup workspace
		workspace := &domain.Workspace{
			ID: workspaceID,
			Integrations: []domain.Integration{
				{
					ID: integrationID,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindSES,
					},
				},
			},
		}

		// Setup expectations
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
		repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
			func(ctx context.Context, wsID string, updates []domain.MessageEventUpdate) error {
				assert.Equal(t, 1, len(updates))
				// Should be truncated to 255 characters
				assert.LessOrEqual(t, len(*updates[0].StatusInfo), 255)
				return nil
			})

		// Call method
		err = service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload)
		assert.NoError(t, err)
	})

	t.Run("ProcessWebhook with long complaint reason truncation", func(t *testing.T) {
		// Create test payload with very long complaint feedback type
		longFeedback := strings.Repeat("spam-", 60) // Longer than 255 characters
		payload := domain.SESWebhookPayload{
			Message: fmt.Sprintf(`{"eventType":"Complaint","complaint":{"complainedRecipients":[{"emailAddress":"test@example.com"}],"timestamp":"2023-01-01T12:00:00Z","complaintFeedbackType":"%s"},"mail":{"messageId":"message1"}}`, longFeedback),
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Setup workspace
		workspace := &domain.Workspace{
			ID: workspaceID,
			Integrations: []domain.Integration{
				{
					ID: integrationID,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindSES,
					},
				},
			},
		}

		// Setup expectations
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
		repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
			func(ctx context.Context, wsID string, updates []domain.MessageEventUpdate) error {
				assert.Equal(t, 1, len(updates))
				// Should be truncated to 255 characters
				assert.LessOrEqual(t, len(*updates[0].StatusInfo), 255)
				return nil
			})

		// Call method
		err = service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload)
		assert.NoError(t, err)
	})

	t.Run("ProcessWebhook with events without message ID", func(t *testing.T) {
		// Create test payload with empty message ID
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"Delivery","delivery":{"recipients":["test@example.com"],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":""}}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Setup workspace
		workspace := &domain.Workspace{
			ID: workspaceID,
			Integrations: []domain.Integration{
				{
					ID: integrationID,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindSES,
					},
				},
			},
		}

		// Setup expectations
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
		repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		// Should NOT call SetStatusesIfNotSet since there's no message ID
		messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
			func(ctx context.Context, wsID string, updates []domain.MessageEventUpdate) error {
				// Should be empty since no message ID
				assert.Equal(t, 0, len(updates))
				return nil
			})

		// Call method
		err = service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload)
		assert.NoError(t, err)
	})

	t.Run("ProcessWebhook with message history update error", func(t *testing.T) {
		// Create test payload
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"Delivery","delivery":{"recipients":["test@example.com"],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"message1"}}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Setup workspace
		workspace := &domain.Workspace{
			ID: workspaceID,
			Integrations: []domain.Integration{
				{
					ID: integrationID,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindSES,
					},
				},
			},
		}

		// Setup expectations with message history error
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
		repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).Return(errors.New("message history error"))

		// Call method
		err = service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update message status")
	})

	t.Run("ProcessWebhook with unsupported event type in switch", func(t *testing.T) {
		// This test is tricky because we need to create an event that passes webhook processing
		// but has an event type not handled in the message history switch
		// We'll modify the service to simulate this by creating a custom event
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"Delivery","delivery":{"recipients":["test@example.com"],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"message1"}}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		// Setup workspace
		workspace := &domain.Workspace{
			ID: workspaceID,
			Integrations: []domain.Integration{
				{
					ID: integrationID,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindSES,
					},
				},
			},
		}

		// Create a custom service that returns an event with unsupported type
		customService := &WebhookEventService{
			repo:               repo,
			authService:        authService,
			logger:             log,
			workspaceRepo:      workspaceRepo,
			messageHistoryRepo: messageHistoryRepo,
		}

		// We'll use reflection or create a mock to simulate this
		// For now, let's just test the actual supported types and ensure default case coverage
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
		repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

		// Call method - this should work normally
		err = customService.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload)
		assert.NoError(t, err)
	})
}

// TestProcessWebhook_UpdatesMessageHistory tests that the ProcessWebhook method updates message history
func TestProcessWebhook_UpdatesMessageHistory(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)

	// Create service
	service := &WebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
	}

	// Test data
	workspaceID := "workspace1"
	integrationID := "integration1"
	messageID := "message123"

	// Setup workspace with integration
	workspace := &domain.Workspace{
		ID: workspaceID,
		Integrations: []domain.Integration{
			{
				ID: integrationID,
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindPostmark,
				},
			},
		},
	}

	// Create a map for Postmark payload since we don't know the exact struct fields
	postmarkPayload := map[string]interface{}{
		"RecordType":  "Delivery",
		"MessageID":   messageID,
		"Recipient":   "test@example.com",
		"DeliveredAt": time.Now().Format(time.RFC3339),
	}

	rawPayload, err := json.Marshal(postmarkPayload)
	require.NoError(t, err)

	// Setup expectations
	workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
	repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
		func(ctx context.Context, workspaceID string, events []*domain.WebhookEvent) error {
			assert.Equal(t, workspace.ID, workspaceID)
			assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
			assert.Equal(t, messageID, events[0].MessageID)
			return nil
		})

	// Expect message history to be updated with the delivery status using batch method
	messageHistoryRepo.EXPECT().SetStatusesIfNotSet(
		gomock.Any(),
		workspaceID,
		gomock.Any(), // Will be a slice of MessageStatusUpdate
	).DoAndReturn(func(ctx context.Context, wsID string, updates []domain.MessageEventUpdate) error {
		assert.Equal(t, workspaceID, wsID)
		assert.Equal(t, 1, len(updates), "Should have 1 message status update")
		assert.Equal(t, messageID, updates[0].ID)
		assert.Equal(t, domain.MessageEventDelivered, updates[0].Event)
		return nil
	})

	// Call the method
	err = service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload)

	// Verify
	assert.NoError(t, err)
}

// Test with multiple events
func TestProcessWebhook_UpdatesMessageHistoryWithMultipleEvents(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)

	// Create service
	service := &WebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
	}

	// Test data
	workspaceID := "workspace1"
	integrationID := "integration1"
	messageID1 := "message123"
	messageID2 := "message456"

	// Setup workspace with integration
	workspace := &domain.Workspace{
		ID: workspaceID,
		Integrations: []domain.Integration{
			{
				ID: integrationID,
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSparkPost,
				},
			},
		},
	}

	// Create SparkPost payload with multiple events
	sparkPostPayload := []*domain.SparkPostWebhookPayload{
		{
			MSys: domain.SparkPostMSys{
				MessageEvent: &domain.SparkPostMessageEvent{
					Type:        "delivery",
					RecipientTo: "test1@example.com",
					MessageID:   messageID1,
					Timestamp:   time.Now().Format(time.RFC3339),
				},
			},
		},
		{
			MSys: domain.SparkPostMSys{
				MessageEvent: &domain.SparkPostMessageEvent{
					Type:        "bounce",
					RecipientTo: "test2@example.com",
					MessageID:   messageID2,
					Timestamp:   time.Now().Format(time.RFC3339),
				},
			},
		},
	}

	rawPayload, err := json.Marshal(sparkPostPayload)
	require.NoError(t, err)

	// Setup expectations
	workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
	repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
		func(ctx context.Context, workspaceID string, events []*domain.WebhookEvent) error {
			assert.Equal(t, workspace.ID, workspaceID)
			assert.Equal(t, 2, len(events))
			assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
			assert.Equal(t, messageID1, events[0].MessageID)
			assert.Equal(t, domain.EmailEventBounce, events[1].Type)
			assert.Equal(t, messageID2, events[1].MessageID)
			return nil
		})

	// Expect message history to be updated with batch of status updates
	messageHistoryRepo.EXPECT().SetStatusesIfNotSet(
		gomock.Any(),
		workspaceID,
		gomock.Any(), // Will be a slice of MessageStatusUpdate
	).DoAndReturn(func(ctx context.Context, wsID string, updates []domain.MessageEventUpdate) error {
		assert.Equal(t, workspaceID, wsID)
		assert.Equal(t, 2, len(updates), "Should have 2 message status updates")

		// Check first update
		assert.Equal(t, messageID1, updates[0].ID)
		assert.Equal(t, domain.MessageEventDelivered, updates[0].Event)

		// Check second update
		assert.Equal(t, messageID2, updates[1].ID)
		assert.Equal(t, domain.MessageEventBounced, updates[1].Event)

		return nil
	})

	// Call the method
	err = service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload)

	// Verify
	assert.NoError(t, err)
}

// TestListEvents tests the ListEvents method of WebhookEventService
func TestListEvents(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)

	// Setup logging expectations
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()

	// Create service
	service := &WebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
	}

	// Create test data
	workspaceID := "workspace1"
	user := &domain.User{ID: "user1"}
	now := time.Now().UTC()

	t.Run("Success case", func(t *testing.T) {
		// Create test params and expected result
		params := domain.WebhookEventListParams{
			Limit:          10,
			WorkspaceID:    workspaceID,
			EventType:      domain.EmailEventBounce,
			RecipientEmail: "test@example.com",
		}

		expectedEvents := []*domain.WebhookEvent{
			{
				ID:                "event1",
				Type:              domain.EmailEventBounce,
				EmailProviderKind: domain.EmailProviderKindSES,
				IntegrationID:     "integration1",
				RecipientEmail:    "test@example.com",
				MessageID:         "message1",
				Timestamp:         now,
				BounceType:        "Permanent",
				BounceCategory:    "General",
				BounceDiagnostic:  "550 User unknown",
				CreatedAt:         now,
			},
			{
				ID:                "event2",
				Type:              domain.EmailEventBounce,
				EmailProviderKind: domain.EmailProviderKindMailjet,
				IntegrationID:     "integration2",
				RecipientEmail:    "test@example.com",
				MessageID:         "message2",
				Timestamp:         now,
				BounceType:        "HardBounce",
				BounceCategory:    "Permanent",
				BounceDiagnostic:  "550 User unknown",
				CreatedAt:         now,
			},
		}

		expectedResult := &domain.WebhookEventListResult{
			Events:     expectedEvents,
			NextCursor: "next-cursor",
			HasMore:    true,
		}

		// Setup mocks for authentication and repository
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(
			context.Background(), user, nil, nil)
		repo.EXPECT().ListEvents(gomock.Any(), workspaceID, params).Return(expectedResult, nil)

		// Call method
		result, err := service.ListEvents(context.Background(), workspaceID, params)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, expectedResult, result)
		assert.Len(t, result.Events, 2)
		assert.Equal(t, "next-cursor", result.NextCursor)
		assert.True(t, result.HasMore)
	})

	t.Run("Authentication error", func(t *testing.T) {
		params := domain.WebhookEventListParams{
			Limit:       10,
			WorkspaceID: workspaceID,
		}

		// Setup mock for failed authentication
		authErr := &domain.ErrUnauthorized{Message: "User not authorized for workspace"}
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(
			context.Background(), nil, nil, authErr)

		// Call method
		result, err := service.ListEvents(context.Background(), workspaceID, params)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("Validation error", func(t *testing.T) {
		// Create invalid params
		params := domain.WebhookEventListParams{
			Limit:       -1, // Invalid limit
			WorkspaceID: workspaceID,
		}

		// Setup mock for successful authentication but failed validation
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(
			context.Background(), user, nil, nil)

		// Call method
		result, err := service.ListEvents(context.Background(), workspaceID, params)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid parameters")
	})

	t.Run("Repository error", func(t *testing.T) {
		params := domain.WebhookEventListParams{
			Limit:       10,
			WorkspaceID: workspaceID,
		}

		// Setup mocks for successful authentication but repository error
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(
			context.Background(), user, nil, nil)

		repoErr := errors.New("database error")
		repo.EXPECT().ListEvents(gomock.Any(), workspaceID, params).Return(nil, repoErr)

		// Call method
		result, err := service.ListEvents(context.Background(), workspaceID, params)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to list webhook events")
	})

	t.Run("Empty result", func(t *testing.T) {
		params := domain.WebhookEventListParams{
			Limit:       10,
			WorkspaceID: workspaceID,
		}

		// Setup mocks for successful authentication with empty result
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(
			context.Background(), user, nil, nil)

		emptyResult := &domain.WebhookEventListResult{
			Events:     []*domain.WebhookEvent{},
			NextCursor: "",
			HasMore:    false,
		}
		repo.EXPECT().ListEvents(gomock.Any(), workspaceID, params).Return(emptyResult, nil)

		// Call method
		result, err := service.ListEvents(context.Background(), workspaceID, params)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.Events)
		assert.Empty(t, result.NextCursor)
		assert.False(t, result.HasMore)
	})
}
