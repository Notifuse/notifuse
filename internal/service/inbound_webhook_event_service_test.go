package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	servicemocks "github.com/Notifuse/notifuse/internal/service/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/safehttpclient"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessWebhook_Success(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)

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
		messageID := "message1"
		mockEvent := &domain.InboundWebhookEvent{
			Type:           domain.EmailEventBounce,
			Source:         domain.WebhookSourceSES,
			IntegrationID:  integrationID,
			RecipientEmail: "test@example.com",
			MessageID:      &messageID,
		}

		// Setup expectations to match what the service will actually store
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
		repo.EXPECT().StoreEvents(gomock.Any(), workspace.ID, gomock.Any()).DoAndReturn(
			func(_ context.Context, workspaceID string, events []*domain.InboundWebhookEvent) error {
				assert.Equal(t, workspace.ID, workspaceID)
				assert.Equal(t, mockEvent.Type, events[0].Type)
				assert.Equal(t, mockEvent.Source, events[0].Source)
				assert.Equal(t, mockEvent.IntegrationID, events[0].IntegrationID)
				assert.Equal(t, mockEvent.RecipientEmail, events[0].RecipientEmail)
				assert.NotNil(t, events[0].MessageID)
				assert.Equal(t, *mockEvent.MessageID, *events[0].MessageID)
				return nil
			})

		// Expect message history to be updated with the bounce status - using batch method
		messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"message1"}).
			Return(map[string]string{"message1": "test@example.com"}, nil)
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

		// Hard bounce → contact_lists.status flipped to 'bounced'
		contactRepo.EXPECT().MarkEmailsAsBounced(gomock.Any(), workspaceID, []string{"test@example.com"}, gomock.Any()).Return(nil)

		// Create service
		service := &InboundWebhookEventService{
			repo:               repo,
			authService:        authService,
			logger:             log,
			workspaceRepo:      workspaceRepo,
			messageHistoryRepo: messageHistoryRepo,
			contactRepo:        contactRepo,
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
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"message1"}).
			Return(map[string]string{"message1": "test@example.com"}, nil)
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
		service := &InboundWebhookEventService{
			repo:               repo,
			authService:        authService,
			logger:             log,
			workspaceRepo:      workspaceRepo,
			messageHistoryRepo: messageHistoryRepo,
			contactRepo:        contactRepo,
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
		service := &InboundWebhookEventService{
			repo:               repo,
			authService:        authService,
			logger:             log,
			workspaceRepo:      workspaceRepo,
			messageHistoryRepo: messageHistoryRepo,
			contactRepo:        contactRepo,
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
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"message1"}).
			Return(map[string]string{"message1": "test@example.com"}, nil)
		service := &InboundWebhookEventService{
			repo:               repo,
			authService:        authService,
			logger:             log,
			workspaceRepo:      workspaceRepo,
			messageHistoryRepo: messageHistoryRepo,
			contactRepo:        contactRepo,
		}

		// Call method
		err = service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload)

		// Assert
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to store inbound webhook events")
	})
}

func TestProcessWebhook_WorkspaceNotFound(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)

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
	service := &InboundWebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
		contactRepo:        contactRepo,
	}

	// Call method
	err := service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get workspace")
}

func TestNewInboundWebhookEventService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)
	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	automationRepo := mocks.NewMockAutomationRepository(ctrl)
	unsubscriber := servicemocks.NewMockListUnsubscriber(ctrl)

	service := NewInboundWebhookEventService(repo, authService, log, workspaceRepo, messageHistoryRepo, contactRepo, automationRepo, unsubscriber)

	assert.NotNil(t, service)
	assert.Equal(t, repo, service.repo)
	assert.Equal(t, authService, service.authService)
	assert.NotNil(t, service.logger)
	assert.Equal(t, workspaceRepo, service.workspaceRepo)
	assert.Equal(t, messageHistoryRepo, service.messageHistoryRepo)
	assert.Equal(t, contactRepo, service.contactRepo)
	assert.Same(t, unsubscriber, service.listUnsubscriber)
}

func TestProcessSESWebhook(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)

	// Setup logging expectations
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()

	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	service := &InboundWebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
		contactRepo:        contactRepo,
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
		assert.Equal(t, domain.WebhookSourceSES, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "message1", *events[0].MessageID)
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
		assert.Equal(t, domain.WebhookSourceSES, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "message1", *events[0].MessageID)
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
		assert.Equal(t, domain.WebhookSourceSES, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "message1", *events[0].MessageID)
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
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "notifuse-123", *events[0].MessageID)
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
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "notifuse-456", *events[0].MessageID)
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
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "notifuse-789", *events[0].MessageID)
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

	t.Run("Bounce Event with X-Message-ID Header Fallback", func(t *testing.T) {
		// Create test bounce payload with X-Message-ID in headers but no tags
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"Bounce","bounce":{"bounceType":"Permanent","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"test@example.com","diagnosticCode":"554"}],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"provider-msg-id","headers":[{"name":"X-Message-ID","value":"notifuse-header-123"}]}}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		events, err := service.processSESWebhook(integrationID, rawPayload)

		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Len(t, events, 1)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "notifuse-header-123", *events[0].MessageID)
	})

	t.Run("Delivery Event with X-Message-ID Header Fallback", func(t *testing.T) {
		// Create test delivery payload with X-Message-ID in headers but no tags
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"Delivery","delivery":{"recipients":["test@example.com"],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"provider-msg-id","headers":[{"name":"X-Message-ID","value":"notifuse-header-456"}]}}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		events, err := service.processSESWebhook(integrationID, rawPayload)

		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Len(t, events, 1)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "notifuse-header-456", *events[0].MessageID)
	})

	t.Run("Complaint Event with X-Message-ID Header Fallback", func(t *testing.T) {
		// Create test complaint payload with X-Message-ID in headers but no tags
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"Complaint","complaint":{"complainedRecipients":[{"emailAddress":"test@example.com"}],"timestamp":"2023-01-01T12:00:00Z","complaintFeedbackType":"abuse"},"mail":{"messageId":"provider-msg-id","headers":[{"name":"X-Message-ID","value":"notifuse-header-789"}]}}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		events, err := service.processSESWebhook(integrationID, rawPayload)

		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Len(t, events, 1)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "notifuse-header-789", *events[0].MessageID)
	})

	t.Run("Tags take priority over X-Message-ID header fallback", func(t *testing.T) {
		// Create test delivery payload with both tags AND headers
		payload := domain.SESWebhookPayload{
			Message: `{"eventType":"Delivery","delivery":{"recipients":["test@example.com"],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"provider-msg-id","tags":{"notifuse_message_id":["tag-message-id"]},"headers":[{"name":"X-Message-ID","value":"header-message-id"}]}}`,
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		events, err := service.processSESWebhook(integrationID, rawPayload)

		assert.NoError(t, err)
		assert.NotNil(t, events)
		assert.Len(t, events, 1)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "tag-message-id", *events[0].MessageID) // Tags have priority
	})

	// A notification lists every recipient of the email it reports on, such as an address the
	// email was copied to
	t.Run("A notification listing several recipients gives one event per recipient", func(t *testing.T) {
		for name, c := range map[string]struct {
			message     string
			diagnostics []string
		}{
			"bounce": {
				message:     `{"eventType":"Bounce","bounce":{"bounceType":"Permanent","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"copy@example.com","diagnosticCode":"550 copy"},{"emailAddress":"test@example.com","diagnosticCode":"550 test"}],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"provider-msg-id","tags":{"notifuse_message_id":["msg-1"]}}}`,
				diagnostics: []string{"550 copy", "550 test"},
			},
			"complaint": {
				message:     `{"eventType":"Complaint","complaint":{"complainedRecipients":[{"emailAddress":"copy@example.com"},{"emailAddress":"test@example.com"}],"timestamp":"2023-01-01T12:00:00Z","complaintFeedbackType":"abuse"},"mail":{"messageId":"provider-msg-id","tags":{"notifuse_message_id":["msg-1"]}}}`,
				diagnostics: []string{"", ""},
			},
			"delivery": {
				message:     `{"eventType":"Delivery","delivery":{"recipients":["copy@example.com","test@example.com"],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"provider-msg-id","tags":{"notifuse_message_id":["msg-1"]}}}`,
				diagnostics: []string{"", ""},
			},
		} {
			t.Run(name, func(t *testing.T) {
				rawPayload, err := json.Marshal(domain.SESWebhookPayload{Message: c.message})
				require.NoError(t, err)

				events, err := service.processSESWebhook(integrationID, rawPayload)

				require.NoError(t, err)
				require.Len(t, events, 2)
				for i, email := range []string{"copy@example.com", "test@example.com"} {
					assert.Equal(t, email, events[i].RecipientEmail)
					require.NotNil(t, events[i].MessageID)
					assert.Equal(t, "msg-1", *events[i].MessageID)
					assert.Equal(t, c.diagnostics[i], events[i].BounceDiagnostic)
				}
				assert.NotEqual(t, events[0].ID, events[1].ID)
			})
		}
	})

	t.Run("A notification listing more recipients than an SES email can have gives events for the first 50", func(t *testing.T) {
		recipients := make([]map[string]string, 0, 60)
		for i := 0; i < 60; i++ {
			recipients = append(recipients, map[string]string{"emailAddress": fmt.Sprintf("r%d@example.com", i)})
		}
		listed, err := json.Marshal(recipients)
		require.NoError(t, err)
		rawPayload, err := json.Marshal(domain.SESWebhookPayload{
			Message: `{"eventType":"Complaint","complaint":{"complainedRecipients":` + string(listed) + `,"timestamp":"2023-01-01T12:00:00Z","complaintFeedbackType":"abuse"},"mail":{"messageId":"provider-msg-id","tags":{"notifuse_message_id":["msg-1"]}}}`,
		})
		require.NoError(t, err)

		events, err := service.processSESWebhook(integrationID, rawPayload)

		require.NoError(t, err)
		require.Len(t, events, 50)
		assert.Equal(t, "r0@example.com", events[0].RecipientEmail)
		assert.Equal(t, "r49@example.com", events[49].RecipientEmail)
		for _, event := range events {
			assert.Equal(t, string(rawPayload), event.RawPayload)
		}
	})

	// SES reports a bounced recipient as the DSN's Final-Recipient, which is the address a
	// forward or an alias led to rather than the one the email was sent to
	t.Run("A notification keeps the address SES reports, even for an email sent to one other address", func(t *testing.T) {
		rawPayload, err := json.Marshal(domain.SESWebhookPayload{
			Message: `{"eventType":"Bounce","bounce":{"bounceType":"Permanent","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"forwarded@elsewhere.example","diagnosticCode":"550 forwarded"}],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"provider-msg-id","destination":["test@example.com"],"tags":{"notifuse_message_id":["msg-1"]}}}`,
		})
		require.NoError(t, err)

		events, err := service.processSESWebhook(integrationID, rawPayload)

		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, "forwarded@elsewhere.example", events[0].RecipientEmail)
		assert.Equal(t, "550 forwarded", events[0].BounceDiagnostic)
	})

	t.Run("A notification about an email sent to several addresses reporting a forwarded address keeps it", func(t *testing.T) {
		rawPayload, err := json.Marshal(domain.SESWebhookPayload{
			Message: `{"eventType":"Bounce","bounce":{"bounceType":"Permanent","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"forwarded@elsewhere.example","diagnosticCode":"550"}],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"provider-msg-id","destination":["test@example.com","copy@example.com"],"tags":{"notifuse_message_id":["msg-1"]}}}`,
		})
		require.NoError(t, err)

		events, err := service.processSESWebhook(integrationID, rawPayload)

		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, "forwarded@elsewhere.example", events[0].RecipientEmail)
	})

	t.Run("A notification listing an empty address gives no event for it", func(t *testing.T) {
		rawPayload, err := json.Marshal(domain.SESWebhookPayload{
			Message: `{"eventType":"Delivery","delivery":{"recipients":["","test@example.com"],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"provider-msg-id","tags":{"notifuse_message_id":["msg-1"]}}}`,
		})
		require.NoError(t, err)

		events, err := service.processSESWebhook(integrationID, rawPayload)

		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
	})

	t.Run("A notification about an email sent to several addresses keeps the addresses SES reports", func(t *testing.T) {
		rawPayload, err := json.Marshal(domain.SESWebhookPayload{
			Message: `{"eventType":"Complaint","complaint":{"complainedRecipients":[{"emailAddress":"copy@example.com"}],"timestamp":"2023-01-01T12:00:00Z","complaintFeedbackType":"abuse"},"mail":{"messageId":"provider-msg-id","destination":["test@example.com","copy@example.com"],"tags":{"notifuse_message_id":["msg-1"]}}}`,
		})
		require.NoError(t, err)

		events, err := service.processSESWebhook(integrationID, rawPayload)

		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, "copy@example.com", events[0].RecipientEmail)
	})

	t.Run("A notification listing no recipient gives no event", func(t *testing.T) {
		rawPayload, err := json.Marshal(domain.SESWebhookPayload{
			Message: `{"eventType":"Bounce","bounce":{"bounceType":"Permanent","bounceSubType":"General","bouncedRecipients":[],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"provider-msg-id","tags":{"notifuse_message_id":["msg-1"]}}}`,
		})
		require.NoError(t, err)

		events, err := service.processSESWebhook(integrationID, rawPayload)

		require.NoError(t, err)
		assert.Empty(t, events)
	})

	t.Run("A notification listing the same recipient twice gives one event for it", func(t *testing.T) {
		rawPayload, err := json.Marshal(domain.SESWebhookPayload{
			Message: `{"eventType":"Bounce","bounce":{"bounceType":"Transient","bounceSubType":"MailboxFull","bouncedRecipients":[{"emailAddress":"test@example.com","diagnosticCode":"452 first"},{"emailAddress":"copy@example.com","diagnosticCode":"452 copy"},{"emailAddress":"Test@Example.com","diagnosticCode":"452 again"}],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"provider-msg-id","tags":{"notifuse_message_id":["msg-1"]}}}`,
		})
		require.NoError(t, err)

		events, err := service.processSESWebhook(integrationID, rawPayload)

		require.NoError(t, err)
		require.Len(t, events, 2)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.Equal(t, "452 first", events[0].BounceDiagnostic)
		assert.Equal(t, "copy@example.com", events[1].RecipientEmail)
	})
}

func TestSameAddress(t *testing.T) {
	for _, c := range []struct {
		name             string
		stored, reported string
		same             bool
	}{
		{"the same address", "jane@example.com", "jane@example.com", true},
		{"another letter case", "Jane@Example.com", "jane@EXAMPLE.com", true},
		{"an international domain reported in Punycode", "user@münchen.de", "user@xn--mnchen-3ya.de", true},
		{"an international domain reported in Unicode", "user@xn--mnchen-3ya.de", "user@münchen.de", true},
		{"an international domain in capitals reported in Punycode", "User@MÜNCHEN.de", "user@xn--mnchen-3ya.de", true},
		{"a non-ASCII local part reported as SES is sent it", "jürgen@example.com", "=?UTF-8?b?asO8cmdlbg==?=@example.com", true},
		{"a non-ASCII local part in capitals reported as SES is sent it", "Üser@münchen.de", "=?UTF-8?b?w5xzZXI=?=@xn--mnchen-3ya.de", true},
		{"a non-ASCII local part in capitals reported in lowercase", "Üser@münchen.de", "üser@xn--mnchen-3ya.de", true},
		{"a non-ASCII local part in capitals reported encoded in lowercase", "Üser@münchen.de", "=?UTF-8?B?w7xzZXI=?=@xn--mnchen-3ya.de", true},
		{"another non-ASCII local part", "Üser@münchen.de", "=?UTF-8?b?w7Z0aGVy?=@xn--mnchen-3ya.de", false},
		{"another domain", "user@münchen.de", "user@munchen.de", false},
		{"another local part", "jane@example.com", "john@example.com", false},
		{"no address", "jane@example.com", "", false},
		{"no domain", "jane@example.com", "jane", false},
		{"an international domain stored decomposed, reported composed", "user@mu\u0308nchen.de", "user@münchen.de", true},
		{"an international domain stored decomposed, reported in Punycode", "user@mu\u0308nchen.de", "user@xn--mnchen-3ya.de", true},
		{"an international domain stored decomposed, reported as SES is sent it", "user@mu\u0308nchen.de", "user@xn--munchen-gie.de", true},
		{"a domain DNS refuses, in another case", "jane@My_Domain.com", "jane@my_domain.COM", true},
		{"a non-ASCII local part encoded, on a domain DNS refuses", "jürgen@my_domain.com", "=?utf-8?q?j=C3=BCrgen?=@My_Domain.com", true},
		{"another local part on a domain DNS refuses", "jürgen@my_domain.com", "=?utf-8?q?j=C3=BCrg?=@my_domain.com", false},
		{"two domains DNS tells apart", "user@straße.de", "user@strasse.de", false},
		{"reported as SparkPost's sandbox sends it", "Jane@Example.com", "jane@example.com.sink.sparkpostmail.com", true},
		{"another address as SparkPost's sandbox sends it", "jane@example.com", "john@example.com.sink.sparkpostmail.com", false},
		{"an international domain in Punycode as SparkPost's sandbox sends it", "user@münchen.de", "user@xn--mnchen-3ya.de.SINK.sparkpostmail.com", true},
		{"a local part as SES is sent it, in another letter case, decodes to another", "jürgen@example.com", "=?UTF-8?b?AsO8cmdlbg==?=@example.com", false},
		{"reported with spaces around it", "jane@example.com", " jane@example.com\u00a0", true},
		{"a domain reported with a character DNS ignores", "user@example.com", "user@ex\u00adample.com", true},
		{"a domain longer than DNS holds is not looked up", "user@example.com", "user@" + strings.Repeat("\u00ad", 300) + "example.com", false},
		{"a local part encoded in two words", "jane@example.com", "=?utf-8?q?ja?= =?utf-8?q?ne?=@example.com", true},
		{"an address longer than any spelling of one is not decoded", "jane@example.com", "=?utf-8?q?ja?=" + strings.Repeat(" ", 3000) + "=?utf-8?q?ne?=@example.com", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.same, sameAddress(c.stored, c.reported))
		})
	}
}

// Encoding a domain to Punycode and decoding a crafted encoded local part take time growing
// with the square of their length, so an event naming an address far longer than any
// spelling of one must not hold up the request. The shorter bound on the domain alone is
// pinned in TestSameAddress, "a domain longer than DNS holds is not looked up".
func TestSameAddress_ALongAddressIsComparedQuickly(t *testing.T) {
	var label strings.Builder
	for i := 0; i < 20000; i++ {
		label.WriteRune(rune(0x4e00 + i))
	}

	for name, reported := range map[string]string{
		"a long domain":             "user@" + label.String(),
		"a long encoded local part": strings.Repeat("=?c?q?x", 64000) + "?=@example.com",
	} {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			assert.False(t, sameAddress("user@example.com", reported))
			assert.Less(t, time.Since(start), time.Second)
		})
	}
}

func TestExtractXMessageIDFromHeaders(t *testing.T) {
	tests := []struct {
		name     string
		headers  []domain.SESHeader
		expected string
	}{
		{
			name:     "empty headers",
			headers:  []domain.SESHeader{},
			expected: "",
		},
		{
			name: "X-Message-ID present",
			headers: []domain.SESHeader{
				{Name: "From", Value: "test@example.com"},
				{Name: "X-Message-ID", Value: "notifuse-123"},
				{Name: "Subject", Value: "Test"},
			},
			expected: "notifuse-123",
		},
		{
			name: "X-Message-ID with different case",
			headers: []domain.SESHeader{
				{Name: "x-message-id", Value: "notifuse-456"},
			},
			expected: "notifuse-456",
		},
		{
			name: "X-Message-ID not present",
			headers: []domain.SESHeader{
				{Name: "From", Value: "test@example.com"},
				{Name: "Subject", Value: "Test"},
			},
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractXMessageIDFromHeaders(tc.headers)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestProcessPostmarkWebhook(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)

	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	service := &InboundWebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
		contactRepo:        contactRepo,
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
		assert.Equal(t, domain.WebhookSourcePostmark, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "message1", *events[0].MessageID)
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
		assert.Equal(t, domain.WebhookSourcePostmark, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "message1", *events[0].MessageID)
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
		assert.Equal(t, domain.WebhookSourcePostmark, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "message1", *events[0].MessageID)
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
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "notifuse-123", *events[0].MessageID)
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

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)

	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	service := &InboundWebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
		contactRepo:        contactRepo,
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
		assert.Equal(t, domain.WebhookSourceSparkPost, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "message1", *events[0].MessageID)
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
		assert.Equal(t, domain.WebhookSourceSparkPost, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "message1", *events[0].MessageID)
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
		assert.Equal(t, domain.WebhookSourceSparkPost, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "message1", *events[0].MessageID)
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
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "notifuse-123", *events[0].MessageID)
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

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)

	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	service := &InboundWebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
		contactRepo:        contactRepo,
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
		assert.Equal(t, domain.WebhookSourceMailgun, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "message1", *events[0].MessageID)
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
		assert.Equal(t, domain.WebhookSourceMailgun, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "message1", *events[0].MessageID)
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
		assert.Equal(t, domain.WebhookSourceMailgun, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "message1", *events[0].MessageID)
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
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "notifuse-123", *events[0].MessageID)
	})
}

func TestProcessMailjetWebhook(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)

	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	service := &InboundWebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
		contactRepo:        contactRepo,
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
		assert.Equal(t, domain.WebhookSourceMailjet, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "12345", *events[0].MessageID)
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
		assert.Equal(t, domain.WebhookSourceMailjet, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "12345", *events[0].MessageID)
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
		assert.Equal(t, domain.WebhookSourceMailjet, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "12345", *events[0].MessageID)
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
		assert.Equal(t, domain.WebhookSourceMailjet, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test1@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "msg-1", *events[0].MessageID) // Should use CustomID

		// Check second event (bounce)
		assert.Equal(t, domain.EmailEventBounce, events[1].Type)
		assert.Equal(t, domain.WebhookSourceMailjet, events[1].Source)
		assert.Equal(t, integrationID, events[1].IntegrationID)
		assert.Equal(t, "test2@example.com", events[1].RecipientEmail)
		assert.NotNil(t, events[1].MessageID)
		assert.Equal(t, "msg-2", *events[1].MessageID) // Should use CustomID
		assert.Equal(t, "HardBounce", events[1].BounceType)
		assert.Equal(t, "Permanent", events[1].BounceCategory)
		assert.Equal(t, "Mailbox does not exist: 550", events[1].BounceDiagnostic)

		// Check third event (spam)
		assert.Equal(t, domain.EmailEventComplaint, events[2].Type)
		assert.Equal(t, domain.WebhookSourceMailjet, events[2].Source)
		assert.Equal(t, integrationID, events[2].IntegrationID)
		assert.Equal(t, "test3@example.com", events[2].RecipientEmail)
		assert.NotNil(t, events[2].MessageID)
		assert.Equal(t, "msg-3", *events[2].MessageID) // Should use CustomID
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
		assert.Equal(t, domain.WebhookSourceMailjet, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "msg-1", *events[0].MessageID) // Should use CustomID
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

	// Mailjet reports "unsub" when a recipient unsubscribes through Mailjet: an opt-out,
	// not a spam complaint
	t.Run("Unsubscribe Event", func(t *testing.T) {
		payload := domain.MailjetWebhookPayload{
			Event:     "unsub",
			Time:      1672574400, // 2023-01-01T12:00:00Z
			Email:     "test@example.com",
			MessageID: 12345,
			CustomID:  "notifuse-message-1",
		}
		rawPayload, err := json.Marshal(payload)
		require.NoError(t, err)

		events, err := service.processMailjetWebhook(integrationID, rawPayload)

		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, domain.EmailEventUnsubscribe, events[0].Type)
		assert.Empty(t, events[0].ComplaintFeedbackType, "an unsubscribe is no complaint")
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		require.NotNil(t, events[0].MessageID)
		assert.Equal(t, "notifuse-message-1", *events[0].MessageID, "the email is named by the CustomID Notifuse gave it")
		assert.Equal(t, time.Unix(1672574400, 0), events[0].Timestamp)
	})
}

func TestProcessSMTPWebhook(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)

	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	service := &InboundWebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
		contactRepo:        contactRepo,
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
		assert.Equal(t, domain.WebhookSourceSMTP, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "message1", *events[0].MessageID)
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
		assert.Equal(t, domain.WebhookSourceSMTP, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "message1", *events[0].MessageID)
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
		assert.Equal(t, domain.WebhookSourceSMTP, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "message1", *events[0].MessageID)
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

func TestProcessSendGridWebhook(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)
	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)

	service := &InboundWebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
		contactRepo:        contactRepo,
	}

	integrationID := "integration1"

	t.Run("Delivered Event", func(t *testing.T) {
		// SendGrid sends arrays of events
		rawPayload := []byte(`[{
			"email": "test@example.com",
			"timestamp": 1706097600,
			"event": "delivered",
			"sg_event_id": "abc123",
			"sg_message_id": "14c5d75ce93.dfd.64b469",
			"response": "250 2.0.0 OK",
			"notifuse_message_id": "msg_123"
		}]`)

		events, err := service.processSendGridWebhook(integrationID, rawPayload)

		assert.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
		assert.Equal(t, domain.WebhookSourceSendGrid, events[0].Source)
		assert.Equal(t, integrationID, events[0].IntegrationID)
		assert.Equal(t, "test@example.com", events[0].RecipientEmail)
		assert.NotNil(t, events[0].MessageID)
		assert.Equal(t, "msg_123", *events[0].MessageID) // Uses notifuse_message_id
	})

	t.Run("Delivered Event - Fallback to sg_message_id", func(t *testing.T) {
		// When notifuse_message_id is not present, fall back to sg_message_id
		rawPayload := []byte(`[{
			"email": "test@example.com",
			"timestamp": 1706097600,
			"event": "delivered",
			"sg_event_id": "abc123",
			"sg_message_id": "14c5d75ce93.dfd.64b469"
		}]`)

		events, err := service.processSendGridWebhook(integrationID, rawPayload)

		assert.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, "14c5d75ce93.dfd.64b469", *events[0].MessageID)
	})

	t.Run("Bounce Event - Hard Bounce", func(t *testing.T) {
		rawPayload := []byte(`[{
			"email": "bounce@example.com",
			"timestamp": 1706097700,
			"event": "bounce",
			"sg_event_id": "def456",
			"sg_message_id": "14c5d75ce93.dfd.64b469",
			"reason": "550 5.1.1 The email account does not exist.",
			"status": "5.1.1",
			"type": "bounce",
			"bounce_classification": "Invalid Address",
			"notifuse_message_id": "msg_456"
		}]`)

		events, err := service.processSendGridWebhook(integrationID, rawPayload)

		assert.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, domain.EmailEventBounce, events[0].Type)
		assert.Equal(t, domain.WebhookSourceSendGrid, events[0].Source)
		assert.Equal(t, "bounce@example.com", events[0].RecipientEmail)
		assert.Equal(t, "bounce", events[0].BounceType)
		assert.Equal(t, "Invalid Address", events[0].BounceCategory)
		assert.Equal(t, "5.1.1: 550 5.1.1 The email account does not exist.", events[0].BounceDiagnostic)
	})

	t.Run("Blocked Event - Soft Bounce", func(t *testing.T) {
		rawPayload := []byte(`[{
			"email": "blocked@example.com",
			"timestamp": 1706097800,
			"event": "blocked",
			"sg_event_id": "ghi789",
			"sg_message_id": "14c5d75ce93.dfd.64b469",
			"reason": "Temporarily rejected due to reputation",
			"status": "4.7.1",
			"bounce_classification": "Reputation",
			"notifuse_message_id": "msg_789"
		}]`)

		events, err := service.processSendGridWebhook(integrationID, rawPayload)

		assert.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, domain.EmailEventBounce, events[0].Type)
		assert.Equal(t, "blocked", events[0].BounceType)
		assert.Equal(t, "Reputation", events[0].BounceCategory)
		assert.Equal(t, "4.7.1: Temporarily rejected due to reputation", events[0].BounceDiagnostic)
	})

	t.Run("Dropped Event", func(t *testing.T) {
		rawPayload := []byte(`[{
			"email": "dropped@example.com",
			"timestamp": 1706097900,
			"event": "dropped",
			"sg_event_id": "jkl012",
			"sg_message_id": "14c5d75ce93.dfd.64b469",
			"reason": "Bounced Address",
			"notifuse_message_id": "msg_012"
		}]`)

		events, err := service.processSendGridWebhook(integrationID, rawPayload)

		assert.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, domain.EmailEventBounce, events[0].Type)
		assert.Equal(t, "dropped", events[0].BounceType)
		assert.Equal(t, "Dropped", events[0].BounceCategory)
		assert.Equal(t, "Bounced Address", events[0].BounceDiagnostic)
	})

	t.Run("Spam Report Event", func(t *testing.T) {
		rawPayload := []byte(`[{
			"email": "complaint@example.com",
			"timestamp": 1706099000,
			"event": "spamreport",
			"sg_event_id": "mno345",
			"sg_message_id": "14c5d75ce93.dfd.64b469",
			"notifuse_message_id": "msg_345"
		}]`)

		events, err := service.processSendGridWebhook(integrationID, rawPayload)

		assert.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, domain.EmailEventComplaint, events[0].Type)
		assert.Equal(t, domain.WebhookSourceSendGrid, events[0].Source)
		assert.Equal(t, "complaint@example.com", events[0].RecipientEmail)
		assert.Equal(t, "spam", events[0].ComplaintFeedbackType)
	})

	t.Run("Multiple Events in Single Payload", func(t *testing.T) {
		rawPayload := []byte(`[
			{
				"email": "delivered@example.com",
				"timestamp": 1706097600,
				"event": "delivered",
				"sg_event_id": "event1",
				"sg_message_id": "msg1",
				"notifuse_message_id": "notifuse_1"
			},
			{
				"email": "bounced@example.com",
				"timestamp": 1706097700,
				"event": "bounce",
				"sg_event_id": "event2",
				"sg_message_id": "msg2",
				"reason": "User unknown",
				"bounce_classification": "Invalid Address",
				"notifuse_message_id": "notifuse_2"
			},
			{
				"email": "complained@example.com",
				"timestamp": 1706097800,
				"event": "spamreport",
				"sg_event_id": "event3",
				"sg_message_id": "msg3",
				"notifuse_message_id": "notifuse_3"
			}
		]`)

		events, err := service.processSendGridWebhook(integrationID, rawPayload)

		assert.NoError(t, err)
		require.Len(t, events, 3)

		// First event: delivered
		assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
		assert.Equal(t, "delivered@example.com", events[0].RecipientEmail)

		// Second event: bounce
		assert.Equal(t, domain.EmailEventBounce, events[1].Type)
		assert.Equal(t, "bounced@example.com", events[1].RecipientEmail)

		// Third event: complaint
		assert.Equal(t, domain.EmailEventComplaint, events[2].Type)
		assert.Equal(t, "complained@example.com", events[2].RecipientEmail)
	})

	t.Run("Ignored Event Types", func(t *testing.T) {
		// processed, deferred, open, click should be skipped
		rawPayload := []byte(`[
			{
				"email": "test@example.com",
				"timestamp": 1706097600,
				"event": "processed",
				"sg_event_id": "event1",
				"sg_message_id": "msg1"
			},
			{
				"email": "test@example.com",
				"timestamp": 1706097700,
				"event": "deferred",
				"sg_event_id": "event2",
				"sg_message_id": "msg2"
			},
			{
				"email": "test@example.com",
				"timestamp": 1706097800,
				"event": "open",
				"sg_event_id": "event3",
				"sg_message_id": "msg3"
			},
			{
				"email": "test@example.com",
				"timestamp": 1706097900,
				"event": "click",
				"sg_event_id": "event4",
				"sg_message_id": "msg4"
			}
		]`)

		events, err := service.processSendGridWebhook(integrationID, rawPayload)

		assert.NoError(t, err)
		assert.Len(t, events, 0) // All events should be skipped
	})

	t.Run("Invalid JSON Payload", func(t *testing.T) {
		rawPayload := []byte(`{invalid json`)

		events, err := service.processSendGridWebhook(integrationID, rawPayload)

		assert.Error(t, err)
		assert.Nil(t, events)
		assert.Contains(t, err.Error(), "failed to unmarshal SendGrid webhook payload")
	})

	t.Run("Empty Array Payload", func(t *testing.T) {
		rawPayload := []byte(`[]`)

		events, err := service.processSendGridWebhook(integrationID, rawPayload)

		assert.NoError(t, err)
		assert.Len(t, events, 0)
	})

	t.Run("Bounce Without Status - Uses Reason Only", func(t *testing.T) {
		rawPayload := []byte(`[{
			"email": "bounce@example.com",
			"timestamp": 1706097700,
			"event": "bounce",
			"sg_event_id": "def456",
			"sg_message_id": "14c5d75ce93.dfd.64b469",
			"reason": "Mailbox does not exist",
			"bounce_classification": "Invalid Address"
		}]`)

		events, err := service.processSendGridWebhook(integrationID, rawPayload)

		assert.NoError(t, err)
		require.Len(t, events, 1)
		// When status is empty, only reason is used
		assert.Equal(t, "Mailbox does not exist", events[0].BounceDiagnostic)
	})
}

// TestProcessWebhook_AdditionalScenarios tests additional scenarios for ProcessWebhook
func TestProcessWebhook_AdditionalScenarios(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)
	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)

	service := &InboundWebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
		contactRepo:        contactRepo,
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
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"message1"}).
			Return(map[string]string{"message1": "test@example.com"}, nil)
		repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
			func(ctx context.Context, wsID string, updates []domain.MessageEventUpdate) error {
				assert.Equal(t, 1, len(updates))
				// Should be truncated to 255 characters
				assert.LessOrEqual(t, len(*updates[0].StatusInfo), 255)
				return nil
			})
		// Hard bounce → contact_lists.status flipped to 'bounced'
		contactRepo.EXPECT().MarkEmailsAsBounced(gomock.Any(), workspaceID, []string{"test@example.com"}, gomock.Any()).Return(nil)

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
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"message1"}).
			Return(map[string]string{"message1": "test@example.com"}, nil)
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

	t.Run("ProcessWebhook with events without message ID: nothing is stored", func(t *testing.T) {
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

		// An event naming no email is not looked up, stored or applied: no expectation
		// beyond the workspace
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)

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
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"message1"}).
			Return(map[string]string{"message1": "test@example.com"}, nil)
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
		customService := &InboundWebhookEventService{
			repo:               repo,
			authService:        authService,
			logger:             log,
			workspaceRepo:      workspaceRepo,
			messageHistoryRepo: messageHistoryRepo,
			contactRepo:        contactRepo,
		}

		// We'll use reflection or create a mock to simulate this
		// For now, let's just test the actual supported types and ensure default case coverage
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"message1"}).
			Return(map[string]string{"message1": "test@example.com"}, nil)
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

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)
	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)

	// Create service
	service := &InboundWebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
		contactRepo:        contactRepo,
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
	messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{messageID}).
		Return(map[string]string{messageID: "test@example.com"}, nil)
	repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
		func(ctx context.Context, workspaceID string, events []*domain.InboundWebhookEvent) error {
			assert.Equal(t, workspace.ID, workspaceID)
			assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
			assert.NotNil(t, events[0].MessageID)
			assert.Equal(t, messageID, *events[0].MessageID)
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

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)
	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)

	// Create service
	service := &InboundWebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
		contactRepo:        contactRepo,
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
					BounceClass: "10", // Hard bounce - Invalid Recipient
				},
			},
		},
	}

	rawPayload, err := json.Marshal(sparkPostPayload)
	require.NoError(t, err)

	// Setup expectations
	workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
	messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{messageID1, messageID2}).
		Return(map[string]string{messageID1: "test1@example.com", messageID2: "test2@example.com"}, nil)
	repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
		func(ctx context.Context, workspaceID string, events []*domain.InboundWebhookEvent) error {
			assert.Equal(t, workspace.ID, workspaceID)
			assert.Equal(t, 2, len(events))
			assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
			assert.NotNil(t, events[0].MessageID)
			assert.Equal(t, messageID1, *events[0].MessageID)
			assert.Equal(t, domain.EmailEventBounce, events[1].Type)
			assert.NotNil(t, events[1].MessageID)
			assert.Equal(t, messageID2, *events[1].MessageID)
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

	// SparkPost class 10 is a hard bounce → contact_lists.status flipped to 'bounced'
	contactRepo.EXPECT().MarkEmailsAsBounced(gomock.Any(), workspaceID, []string{"test2@example.com"}, gomock.Any()).Return(nil)

	// Call the method
	err = service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload)

	// Verify
	assert.NoError(t, err)
}

// newClassificationTestService is a tiny helper for the SES classification tests
// below. Each call returns a fresh service + its mocks so each subtest is
// independent.
func newClassificationTestService(t *testing.T) (
	*InboundWebhookEventService,
	*mocks.MockInboundWebhookEventRepository,
	*mocks.MockWorkspaceRepository,
	*mocks.MockMessageHistoryRepository,
	*mocks.MockContactRepository,
	*gomock.Controller,
) {
	t.Helper()
	ctrl := gomock.NewController(t)

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Debug(gomock.Any()).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)

	service := &InboundWebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
		contactRepo:        contactRepo,
	}
	return service, repo, workspaceRepo, messageHistoryRepo, contactRepo, ctrl
}

func sesWorkspace(workspaceID, integrationID string) *domain.Workspace {
	return &domain.Workspace{
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
}

func TestProcessWebhook_SES_Permanent_HardBounce(t *testing.T) {
	service, repo, workspaceRepo, messageHistoryRepo, contactRepo, ctrl := newClassificationTestService(t)
	defer ctrl.Finish()

	workspaceID, integrationID := "ws1", "int1"
	payload := domain.SESWebhookPayload{
		Message: `{"eventType":"Bounce","bounce":{"bounceType":"Permanent","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"hard@example.com","diagnosticCode":"550 mailbox does not exist"}],"timestamp":"2026-05-12T10:00:00Z"},"mail":{"messageId":"msg-1"}}`,
	}
	rawPayload, err := json.Marshal(payload)
	require.NoError(t, err)

	workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(sesWorkspace(workspaceID, integrationID), nil)
	messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-1"}).
		Return(map[string]string{"msg-1": "hard@example.com"}, nil)
	repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	// Hard bounce: no count query, direct escalation.
	contactRepo.EXPECT().MarkEmailsAsBounced(gomock.Any(), workspaceID, []string{"hard@example.com"}, gomock.Any()).Return(nil)

	require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload))
}

func TestProcessWebhook_SES_TransientGeneral_BelowThreshold(t *testing.T) {
	service, repo, workspaceRepo, messageHistoryRepo, contactRepo, ctrl := newClassificationTestService(t)
	defer ctrl.Finish()

	workspaceID, integrationID := "ws1", "int1"
	payload := domain.SESWebhookPayload{
		Message: `{"eventType":"Bounce","bounce":{"bounceType":"Transient","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"soft@example.com","diagnosticCode":"smtp; 421 temporary"}],"timestamp":"2026-05-12T10:00:00Z"},"mail":{"messageId":"msg-2"}}`,
	}
	rawPayload, err := json.Marshal(payload)
	require.NoError(t, err)

	workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(sesWorkspace(workspaceID, integrationID), nil)
	messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-2"}).
		Return(map[string]string{"msg-2": "soft@example.com"}, nil)
	repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	// Soft bounce never produces a message_history update.
	messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, updates []domain.MessageEventUpdate) error {
			assert.Empty(t, updates, "soft bounces should not touch message_history")
			return nil
		})
	// Count query runs and returns 2 (under default threshold of 5).
	repo.EXPECT().
		CountConsecutiveSoftBounces(gomock.Any(), workspaceID, []string{"soft@example.com"}).
		Return(map[string]int{"soft@example.com": 2}, nil)
	// MarkEmailsAsBounced must NOT be called — no EXPECT.

	require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload))
	_ = contactRepo
}

func TestProcessWebhook_SES_TransientGeneral_AtThreshold(t *testing.T) {
	service, repo, workspaceRepo, messageHistoryRepo, contactRepo, ctrl := newClassificationTestService(t)
	defer ctrl.Finish()

	workspaceID, integrationID := "ws1", "int1"
	payload := domain.SESWebhookPayload{
		Message: `{"eventType":"Bounce","bounce":{"bounceType":"Transient","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"chronic@example.com","diagnosticCode":"smtp; 421 temporary"}],"timestamp":"2026-05-12T10:00:00Z"},"mail":{"messageId":"msg-3"}}`,
	}
	rawPayload, err := json.Marshal(payload)
	require.NoError(t, err)

	workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(sesWorkspace(workspaceID, integrationID), nil)
	messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-3"}).
		Return(map[string]string{"msg-3": "chronic@example.com"}, nil)
	repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	// Count query returns 5 (== default threshold) → escalate.
	repo.EXPECT().
		CountConsecutiveSoftBounces(gomock.Any(), workspaceID, []string{"chronic@example.com"}).
		Return(map[string]int{"chronic@example.com": domain.DefaultSoftBounceThreshold}, nil)
	contactRepo.EXPECT().MarkEmailsAsBounced(gomock.Any(), workspaceID, []string{"chronic@example.com"}, gomock.Any()).Return(nil)

	require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload))
}

func TestProcessWebhook_SES_TransientGeneral_RetryExhausted(t *testing.T) {
	service, repo, workspaceRepo, messageHistoryRepo, contactRepo, ctrl := newClassificationTestService(t)
	defer ctrl.Finish()

	workspaceID, integrationID := "ws1", "int1"
	// The exact diagnostic from the original issue (#323).
	payload := domain.SESWebhookPayload{
		Message: `{"eventType":"Bounce","bounce":{"bounceType":"Transient","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"expired@example.com","diagnosticCode":"smtp; 550 4.4.7 Message expired: unable to deliver in 840 minutes. <421 4.4.1 Failed to establish connection>"}],"timestamp":"2026-05-12T10:00:00Z"},"mail":{"messageId":"msg-4"}}`,
	}
	rawPayload, err := json.Marshal(payload)
	require.NoError(t, err)

	workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(sesWorkspace(workspaceID, integrationID), nil)
	messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-4"}).
		Return(map[string]string{"msg-4": "expired@example.com"}, nil)
	repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	// CountConsecutiveSoftBounces MUST NOT be called — diagnostic promotion bypasses the counter.
	// (No repo.EXPECT() for it.)
	contactRepo.EXPECT().MarkEmailsAsBounced(gomock.Any(), workspaceID, []string{"expired@example.com"}, gomock.Any()).Return(nil)

	require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload))
}

func TestProcessWebhook_SES_MessageTooLarge_NoOp(t *testing.T) {
	service, repo, workspaceRepo, messageHistoryRepo, contactRepo, ctrl := newClassificationTestService(t)
	defer ctrl.Finish()

	workspaceID, integrationID := "ws1", "int1"
	payload := domain.SESWebhookPayload{
		Message: `{"eventType":"Bounce","bounce":{"bounceType":"Transient","bounceSubType":"MessageTooLarge","bouncedRecipients":[{"emailAddress":"big@example.com","diagnosticCode":"552 5.3.4 message too big"}],"timestamp":"2026-05-12T10:00:00Z"},"mail":{"messageId":"msg-5"}}`,
	}
	rawPayload, err := json.Marshal(payload)
	require.NoError(t, err)

	workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(sesWorkspace(workspaceID, integrationID), nil)
	messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-5"}).
		Return(map[string]string{"msg-5": "big@example.com"}, nil)
	repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	// SoftIgnore: no count query, no escalation. (No EXPECT for either.)

	require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload))
	_ = contactRepo
}

func TestProcessWebhook_BatchedMixedClassifications(t *testing.T) {
	service, repo, workspaceRepo, messageHistoryRepo, contactRepo, ctrl := newClassificationTestService(t)
	defer ctrl.Finish()

	workspaceID, integrationID := "ws1", "int1"

	// SparkPost is convenient: a single payload can carry multiple events with
	// distinct recipients and bounce classes.
	//  - hard@   → class 10 (Hard)
	//  - soft1@  → class 20 (SoftCount), will cross threshold via counter
	//  - soft2@  → class 20 (SoftCount), stays below threshold
	//  - ignore@ → class 25 (Admin/temporary → SoftCount per existing taxonomy; use a
	//              SoftIgnore equivalent below by switching to SES MessageTooLarge in a
	//              second integration. SparkPost has no SoftIgnore mapping, so this
	//              test exercises Hard + SoftCount only — sufficient to verify the
	//              count query receives only SoftCount emails and that hard ∪
	//              escalated_softCount lands in MarkEmailsAsBounced.)
	sp := []*domain.SparkPostWebhookPayload{
		{MSys: domain.SparkPostMSys{MessageEvent: &domain.SparkPostMessageEvent{
			Type: "bounce", RecipientTo: "hard@example.com", MessageID: "m-h",
			Timestamp: time.Now().Format(time.RFC3339), BounceClass: "10",
		}}},
		{MSys: domain.SparkPostMSys{MessageEvent: &domain.SparkPostMessageEvent{
			Type: "bounce", RecipientTo: "soft1@example.com", MessageID: "m-s1",
			Timestamp: time.Now().Format(time.RFC3339), BounceClass: "20",
		}}},
		{MSys: domain.SparkPostMSys{MessageEvent: &domain.SparkPostMessageEvent{
			Type: "bounce", RecipientTo: "soft2@example.com", MessageID: "m-s2",
			Timestamp: time.Now().Format(time.RFC3339), BounceClass: "20",
		}}},
	}
	rawPayload, err := json.Marshal(sp)
	require.NoError(t, err)

	workspace := &domain.Workspace{
		ID: workspaceID,
		Integrations: []domain.Integration{
			{ID: integrationID, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSparkPost}},
		},
	}
	workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
	messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"m-h", "m-s1", "m-s2"}).
		Return(map[string]string{"m-h": "hard@example.com", "m-s1": "soft1@example.com", "m-s2": "soft2@example.com"}, nil)
	repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

	// Verify the count query gets only the SoftCount emails (order-independent).
	repo.EXPECT().
		CountConsecutiveSoftBounces(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, emails []string) (map[string]int, error) {
			assert.ElementsMatch(t, []string{"soft1@example.com", "soft2@example.com"}, emails,
				"only SoftCount emails should hit the counter")
			return map[string]int{
				"soft1@example.com": domain.DefaultSoftBounceThreshold, // crosses → promoted
				"soft2@example.com": 1,                                 // below → stays soft
			}, nil
		})

	// Final escalation: hard@ (direct hard) ∪ soft1@ (counter-promoted).
	contactRepo.EXPECT().
		MarkEmailsAsBounced(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, emails []string, _ time.Time) error {
			assert.ElementsMatch(t, []string{"hard@example.com", "soft1@example.com"}, emails)
			return nil
		})

	require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, rawPayload))
}

// TestProcessWebhook_ActsOnlyOnEmailsSentToTheEventAddress: a provider event changes
// nothing, and is not stored, unless it names an email of the workspace sent to the address
// the event reports.
func TestProcessWebhook_ActsOnlyOnEmailsSentToTheEventAddress(t *testing.T) {
	workspaceID, integrationID := "ws1", "int1"

	// sesBounce builds an SES bounce carrying the Notifuse id in its tag, or only SES's own
	// id when notifuseID is empty.
	sesBounce := func(t *testing.T, notifuseID, email, bounceType string) []byte {
		t.Helper()
		tags := ""
		if notifuseID != "" {
			tags = fmt.Sprintf(`,"tags":{"notifuse_message_id":[%q]}`, notifuseID)
		}
		raw, err := json.Marshal(domain.SESWebhookPayload{
			Type: "Notification",
			Message: fmt.Sprintf(`{"eventType":"Bounce","bounce":{"bounceType":%q,"bounceSubType":"General","bouncedRecipients":[{"emailAddress":%q,"diagnosticCode":"smtp; 550 5.1.1 user unknown"}],"timestamp":"2026-10-21T10:00:00Z"},"mail":{"messageId":"0100018f-ses-own-id"%s}}`,
				bounceType, email, tags),
		})
		require.NoError(t, err)
		return raw
	}
	workspaceOf := func(kind domain.EmailProviderKind) *domain.Workspace {
		return &domain.Workspace{
			ID:           workspaceID,
			Integrations: []domain.Integration{{ID: integrationID, EmailProvider: domain.EmailProvider{Kind: kind}}},
		}
	}
	sendGridBatch := func(t *testing.T, events ...map[string]interface{}) []byte {
		t.Helper()
		raw, err := json.Marshal(events)
		require.NoError(t, err)
		return raw
	}
	sendGridEvent := func(kind, notifuseID, email string) map[string]interface{} {
		event := map[string]interface{}{
			"event":         kind,
			"email":         email,
			"timestamp":     time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC).Unix(),
			"sg_message_id": "sg-own-id." + kind,
		}
		if notifuseID != "" {
			event["notifuse_message_id"] = notifuseID
		}
		return event
	}

	// Each case leaves out the expectations of everything an event could change: gomock
	// fails the test on any call to StoreEvents, SetStatusesIfNotSet,
	// CountConsecutiveSoftBounces, MarkEmailsAsBounced or GetRecipientAndList it does not
	// expect.
	t.Run("a hard bounce naming no email of the workspace: nothing is stored or changed", func(t *testing.T) {
		service, _, workspaceRepo, messageHistoryRepo, _, ctrl := newClassificationTestService(t)
		defer ctrl.Finish()
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSES), nil)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-unknown"}).Return(map[string]string{}, nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID,
			sesBounce(t, "msg-unknown", "victim@example.com", "Permanent")))
	})

	t.Run("a hard bounce naming an email sent to another address: nothing is stored or changed", func(t *testing.T) {
		service, _, workspaceRepo, messageHistoryRepo, _, ctrl := newClassificationTestService(t)
		defer ctrl.Finish()
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSES), nil)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-1"}).
			Return(map[string]string{"msg-1": "recipient@example.com"}, nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID,
			sesBounce(t, "msg-1", "victim@example.com", "Permanent")))
	})

	t.Run("a soft bounce naming no email of the workspace is not counted", func(t *testing.T) {
		service, _, workspaceRepo, messageHistoryRepo, _, ctrl := newClassificationTestService(t)
		defer ctrl.Finish()
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSES), nil)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-unknown"}).Return(map[string]string{}, nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID,
			sesBounce(t, "msg-unknown", "victim@example.com", "Transient")))
	})

	t.Run("an event carrying only the provider's own id is ignored", func(t *testing.T) {
		service, _, workspaceRepo, messageHistoryRepo, _, ctrl := newClassificationTestService(t)
		defer ctrl.Finish()
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSES), nil)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"0100018f-ses-own-id"}).
			Return(map[string]string{}, nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID,
			sesBounce(t, "", "recipient@example.com", "Permanent")))
	})

	t.Run("events with no email id or no address: nothing is looked up or stored", func(t *testing.T) {
		service, _, workspaceRepo, _, _, ctrl := newClassificationTestService(t)
		defer ctrl.Finish()
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSendGrid), nil)
		noID := sendGridEvent("bounce", "", "recipient@example.com")
		delete(noID, "sg_message_id")

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID,
			sendGridBatch(t, noID, sendGridEvent("bounce", "msg-1", ""))))
	})

	t.Run("a hard bounce naming an email sent to its address in another case: recorded and applied with the address stored on the email", func(t *testing.T) {
		service, repo, workspaceRepo, messageHistoryRepo, contactRepo, ctrl := newClassificationTestService(t)
		defer ctrl.Finish()
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSES), nil)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-1"}).
			Return(map[string]string{"msg-1": "Recipient@Example.com"}, nil)
		repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, events []*domain.InboundWebhookEvent) error {
				require.Len(t, events, 1)
				assert.Equal(t, "Recipient@Example.com", events[0].RecipientEmail)
				assert.Equal(t, "msg-1", *events[0].MessageID)
				return nil
			})
		messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, updates []domain.MessageEventUpdate) error {
				require.Len(t, updates, 1)
				assert.Equal(t, "msg-1", updates[0].ID)
				assert.Equal(t, domain.MessageEventBounced, updates[0].Event)
				return nil
			})
		contactRepo.EXPECT().MarkEmailsAsBounced(gomock.Any(), workspaceID, []string{"Recipient@Example.com"}, gomock.Any()).Return(nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID,
			sesBounce(t, "msg-1", "recipient@example.com", "Permanent")))
	})

	t.Run("a soft bounce naming an email sent to its address is counted under the address stored on the email", func(t *testing.T) {
		service, repo, workspaceRepo, messageHistoryRepo, _, ctrl := newClassificationTestService(t)
		defer ctrl.Finish()
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSES), nil)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-1"}).
			Return(map[string]string{"msg-1": "Recipient@Example.com"}, nil)
		repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		repo.EXPECT().CountConsecutiveSoftBounces(gomock.Any(), workspaceID, []string{"Recipient@Example.com"}).
			Return(map[string]int{"Recipient@Example.com": 1}, nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID,
			sesBounce(t, "msg-1", "RECIPIENT@example.com", "Transient")))
	})

	t.Run("a complaint and a delivery naming an email sent to another address change no status", func(t *testing.T) {
		service, _, workspaceRepo, messageHistoryRepo, _, ctrl := newClassificationTestService(t)
		defer ctrl.Finish()
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSendGrid), nil)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-1"}).
			Return(map[string]string{"msg-1": "recipient@example.com"}, nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, sendGridBatch(t,
			sendGridEvent("spamreport", "msg-1", "someone-else@example.com"),
			sendGridEvent("delivered", "msg-1", "someone-else@example.com"))))
	})

	t.Run("a batch: one lookup, then only the events of emails sent to their address are stored and applied", func(t *testing.T) {
		service, repo, workspaceRepo, messageHistoryRepo, contactRepo, ctrl := newClassificationTestService(t)
		defer ctrl.Finish()
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSendGrid), nil)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-1", "msg-unknown", "msg-2"}).
			Return(map[string]string{"msg-1": "first@example.com", "msg-2": "second@example.com"}, nil)
		repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, events []*domain.InboundWebhookEvent) error {
				require.Len(t, events, 2)
				assert.Equal(t, domain.EmailEventDelivered, events[0].Type)
				assert.Equal(t, "msg-1", *events[0].MessageID)
				assert.Equal(t, domain.EmailEventBounce, events[1].Type)
				assert.Equal(t, "msg-2", *events[1].MessageID)
				return nil
			})
		messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, updates []domain.MessageEventUpdate) error {
				require.Len(t, updates, 2)
				assert.Equal(t, "msg-1", updates[0].ID)
				assert.Equal(t, domain.MessageEventDelivered, updates[0].Event)
				assert.Equal(t, "msg-2", updates[1].ID)
				assert.Equal(t, domain.MessageEventBounced, updates[1].Event)
				return nil
			})
		contactRepo.EXPECT().MarkEmailsAsBounced(gomock.Any(), workspaceID, []string{"second@example.com"}, gomock.Any()).Return(nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, sendGridBatch(t,
			sendGridEvent("delivered", "msg-1", "first@example.com"),
			sendGridEvent("bounce", "msg-unknown", "victim@example.com"),
			sendGridEvent("spamreport", "msg-1", "someone-else@example.com"),
			sendGridEvent("bounce", "msg-2", "second@example.com"))))
	})

	t.Run("an unsubscribe naming no email of the workspace is not stored", func(t *testing.T) {
		service, _, workspaceRepo, messageHistoryRepo, _, ctrl := newClassificationTestService(t)
		defer ctrl.Finish()
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindMailjet), nil)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-unknown"}).Return(map[string]string{}, nil)
		raw, err := json.Marshal(domain.MailjetWebhookPayload{
			Event: "unsub", Time: 1761040800, Email: "victim@example.com", MessageID: 1, CustomID: "msg-unknown",
		})
		require.NoError(t, err)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, raw))
	})

	// sesNotification wraps an SES notification of the email msg-1 names
	sesNotification := func(t *testing.T, message string) []byte {
		t.Helper()
		raw, err := json.Marshal(domain.SESWebhookPayload{Type: "Notification", Message: message})
		require.NoError(t, err)
		return raw
	}
	sesMail := `"mail":{"messageId":"0100018f-ses-own-id","tags":{"notifuse_message_id":["msg-1"]}}`

	t.Run("an SES bounce listing an address the email was copied to before its recipient: the recipient's bounce is recorded and applied", func(t *testing.T) {
		service, repo, workspaceRepo, messageHistoryRepo, contactRepo, ctrl := newClassificationTestService(t)
		defer ctrl.Finish()
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSES), nil)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-1"}).
			Return(map[string]string{"msg-1": "recipient@example.com"}, nil)
		repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, events []*domain.InboundWebhookEvent) error {
				require.Len(t, events, 1)
				assert.Equal(t, "recipient@example.com", events[0].RecipientEmail)
				assert.Equal(t, "smtp; 550 recipient", events[0].BounceDiagnostic)
				return nil
			})
		messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, updates []domain.MessageEventUpdate) error {
				require.Len(t, updates, 1)
				assert.Equal(t, "msg-1", updates[0].ID)
				assert.Equal(t, domain.MessageEventBounced, updates[0].Event)
				return nil
			})
		contactRepo.EXPECT().MarkEmailsAsBounced(gomock.Any(), workspaceID, []string{"recipient@example.com"}, gomock.Any()).Return(nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, sesNotification(t,
			`{"eventType":"Bounce","bounce":{"bounceType":"Permanent","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"copy@example.com","diagnosticCode":"smtp; 550 copy"},{"emailAddress":"recipient@example.com","diagnosticCode":"smtp; 550 recipient"}],"timestamp":"2026-10-21T10:00:00Z"},`+sesMail+`}`)))
	})

	for name, c := range map[string]struct {
		message string
		event   domain.MessageEvent
	}{
		"complaint": {
			message: `{"eventType":"Complaint","complaint":{"complainedRecipients":[{"emailAddress":"copy@example.com"},{"emailAddress":"recipient@example.com"}],"timestamp":"2026-10-21T10:00:00Z","complaintFeedbackType":"abuse"},` + sesMail + `}`,
			event:   domain.MessageEventComplained,
		},
		"delivery": {
			message: `{"eventType":"Delivery","delivery":{"recipients":["copy@example.com","recipient@example.com"],"timestamp":"2026-10-21T10:00:00Z"},` + sesMail + `}`,
			event:   domain.MessageEventDelivered,
		},
	} {
		t.Run("an SES "+name+" listing an address the email was copied to before its recipient: the recipient's "+name+" is recorded", func(t *testing.T) {
			service, repo, workspaceRepo, messageHistoryRepo, _, ctrl := newClassificationTestService(t)
			defer ctrl.Finish()
			workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSES), nil)
			messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-1"}).
				Return(map[string]string{"msg-1": "recipient@example.com"}, nil)
			repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
				func(_ context.Context, _ string, events []*domain.InboundWebhookEvent) error {
					require.Len(t, events, 1)
					assert.Equal(t, "recipient@example.com", events[0].RecipientEmail)
					return nil
				})
			messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
				func(_ context.Context, _ string, updates []domain.MessageEventUpdate) error {
					require.Len(t, updates, 1)
					assert.Equal(t, "msg-1", updates[0].ID)
					assert.Equal(t, c.event, updates[0].Event)
					return nil
				})

			require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, sesNotification(t, c.message)))
		})
	}

	t.Run("an SES bounce reporting the address an email sent to its recipient alone was forwarded to: nothing is stored or changed", func(t *testing.T) {
		service, _, workspaceRepo, messageHistoryRepo, _, ctrl := newClassificationTestService(t)
		defer ctrl.Finish()
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSES), nil)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-1"}).
			Return(map[string]string{"msg-1": "recipient@example.com"}, nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, sesNotification(t,
			`{"eventType":"Bounce","bounce":{"bounceType":"Permanent","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"forwarded@elsewhere.example","diagnosticCode":"smtp; 550 forwarded"}],"timestamp":"2026-10-21T10:00:00Z"},"mail":{"messageId":"0100018f-ses-own-id","destination":["recipient@example.com"],"tags":{"notifuse_message_id":["msg-1"]}}}`)))
	})

	t.Run("an SES complaint giving the international domain of the email's address in Punycode, as SES is sent it: recorded with the address stored on the email", func(t *testing.T) {
		service, repo, workspaceRepo, messageHistoryRepo, _, ctrl := newClassificationTestService(t)
		defer ctrl.Finish()
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSES), nil)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-1"}).
			Return(map[string]string{"msg-1": "user@münchen.de"}, nil)
		repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, events []*domain.InboundWebhookEvent) error {
				require.Len(t, events, 1)
				assert.Equal(t, "user@münchen.de", events[0].RecipientEmail)
				return nil
			})
		messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, updates []domain.MessageEventUpdate) error {
				require.Len(t, updates, 1)
				assert.Equal(t, domain.MessageEventComplained, updates[0].Event)
				return nil
			})

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, sesNotification(t,
			`{"eventType":"Complaint","complaint":{"complainedRecipients":[{"emailAddress":"user@xn--mnchen-3ya.de"}],"timestamp":"2026-10-21T10:00:00Z","complaintFeedbackType":"abuse"},`+sesMail+`}`)))
	})

	t.Run("the lookup fails: the error is returned and nothing is stored", func(t *testing.T) {
		service, _, workspaceRepo, messageHistoryRepo, _, ctrl := newClassificationTestService(t)
		defer ctrl.Finish()
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSES), nil)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-1"}).
			Return(nil, errors.New("connection reset"))

		err := service.ProcessWebhook(context.Background(), workspaceID, integrationID,
			sesBounce(t, "msg-1", "recipient@example.com", "Permanent"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection reset")
	})

	t.Run("ignored events are logged once, with their number and the first id, without addresses", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		log := pkgmocks.NewMockLogger(ctrl)
		fields := map[string]interface{}{}
		log.EXPECT().WithField(gomock.Any(), gomock.Any()).DoAndReturn(func(key string, value interface{}) logger.Logger {
			fields[key] = value
			return log
		}).AnyTimes()
		log.EXPECT().Info("Ignored provider events that name no email sent to their address").Times(1)
		service := &InboundWebhookEventService{
			logger:             log,
			workspaceRepo:      workspaceRepo,
			messageHistoryRepo: messageHistoryRepo,
		}
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSendGrid), nil)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{"msg-unknown", "msg-1"}).
			Return(map[string]string{"msg-1": "recipient@example.com"}, nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, sendGridBatch(t,
			sendGridEvent("bounce", "msg-unknown", "victim@example.com"),
			sendGridEvent("bounce", "msg-1", "someone-else@example.com"))))

		assert.Equal(t, map[string]interface{}{
			"workspace_id":     workspaceID,
			"integration_id":   integrationID,
			"ignored":          2,
			"first_message_id": "msg-unknown",
		}, fields)
	})

	t.Run("a long ignored id is logged cut to its first 255 bytes, on a character boundary", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		log := pkgmocks.NewMockLogger(ctrl)
		fields := map[string]interface{}{}
		log.EXPECT().WithField(gomock.Any(), gomock.Any()).DoAndReturn(func(key string, value interface{}) logger.Logger {
			fields[key] = value
			return log
		}).AnyTimes()
		log.EXPECT().Info("Ignored provider events that name no email sent to their address").Times(1)
		service := &InboundWebhookEventService{
			logger:             log,
			workspaceRepo:      workspaceRepo,
			messageHistoryRepo: messageHistoryRepo,
		}
		// Two bytes a character, so byte 255 falls inside the 128th
		longID := strings.Repeat("é", 200)
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSendGrid), nil)
		messageHistoryRepo.EXPECT().GetRecipients(gomock.Any(), workspaceID, []string{longID}).
			Return(map[string]string{}, nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, sendGridBatch(t,
			sendGridEvent("bounce", longID, "victim@example.com"))))

		assert.Equal(t, strings.Repeat("é", 127)+"…(truncated)", fields["first_message_id"])
	})
}

// TestProcessWebhook_MailjetUnsubscribe posts Mailjet "unsub" events through ProcessWebhook.
// An unsubscribe of the email's own recipient marks the email unsubscribed, after the list
// service recorded the opt-out on the list found for the email, when there is one. It never
// becomes a complaint.
func TestProcessWebhook_MailjetUnsubscribe(t *testing.T) {
	workspaceID, integrationID := "ws1", "int1"
	listID := "list-1"
	unsubscribedAt := time.Unix(1672574400, 0)
	mailjetWorkspace := &domain.Workspace{
		ID: workspaceID,
		Integrations: []domain.Integration{
			{ID: integrationID, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindMailjet}},
		},
	}
	unsub := func(messageID, email string) domain.MailjetWebhookPayload {
		return domain.MailjetWebhookPayload{
			Event: "unsub", Time: unsubscribedAt.Unix(), Email: email, MessageID: 12345, CustomID: messageID,
		}
	}
	sentFor := func(email string, listID *string) *domain.MessageHistory {
		return &domain.MessageHistory{ContactEmail: email, ListID: listID}
	}
	marked := func(messageIDs ...string) []domain.MessageEventUpdate {
		updates := make([]domain.MessageEventUpdate, 0, len(messageIDs))
		for _, id := range messageIDs {
			updates = append(updates, domain.MessageEventUpdate{ID: id, Event: domain.MessageEventUnsubscribed, Timestamp: unsubscribedAt})
		}
		return updates
	}

	type fixture struct {
		service      *InboundWebhookEventService
		repo         *mocks.MockInboundWebhookEventRepository
		history      *mocks.MockMessageHistoryRepository
		unsubscriber *servicemocks.MockListUnsubscriber
	}
	// setup returns a service whose controller finishes with the subtest. gomock fails the
	// subtest on any call it did not expect, a call to the list service included.
	setup := func(t *testing.T) fixture {
		service, repo, workspaceRepo, history, _, ctrl := newClassificationTestService(t)
		f := fixture{service: service, repo: repo, history: history, unsubscriber: servicemocks.NewMockListUnsubscriber(ctrl)}
		service.listUnsubscriber = f.unsubscriber
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(mailjetWorkspace, nil)
		return f
	}
	// expectNoComplaint expects the status update of the delivery's deliveries, bounces and
	// complaints, which every delivery makes, to hold nothing
	expectNoComplaint := func(t *testing.T, f fixture) *gomock.Call {
		return f.history.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, updates []domain.MessageEventUpdate) error {
				assert.Empty(t, updates, "an unsubscribe is no complaint")
				return nil
			})
	}
	// sentTo expects the check of the emails the delivery names, which finds them sent to
	// these addresses
	sentTo := func(f fixture, recipients map[string]string) {
		f.history.EXPECT().GetRecipients(gomock.Any(), workspaceID, gomock.Any()).Return(recipients, nil)
	}
	process := func(t *testing.T, f fixture, events ...domain.MailjetWebhookPayload) error {
		t.Helper()
		raw, err := json.Marshal(events)
		require.NoError(t, err)
		return f.service.ProcessWebhook(context.Background(), workspaceID, integrationID, raw)
	}

	t.Run("an email sent for a list: the opt-out is recorded on that list, then the email is marked unsubscribed", func(t *testing.T) {
		f := setup(t)
		sentTo(f, map[string]string{"msg-1": "jane@example.com"})
		f.repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, events []*domain.InboundWebhookEvent) error {
				require.Len(t, events, 1)
				assert.Equal(t, domain.EmailEventUnsubscribe, events[0].Type)
				return nil
			})
		gomock.InOrder(
			expectNoComplaint(t, f),
			f.history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-1").Return(sentFor("jane@example.com", &listID), nil),
			f.unsubscriber.EXPECT().UnsubscribeFromMessageList(gomock.Any(), workspaceID, "jane@example.com", listID, "msg-1").Return(nil),
			f.history.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, marked("msg-1")).Return(nil),
		)

		require.NoError(t, process(t, f, unsub("msg-1", "jane@example.com")))
	})

	t.Run("the address as stored is unsubscribed, whatever its case in the event", func(t *testing.T) {
		f := setup(t)
		sentTo(f, map[string]string{"msg-1": "jane@example.com"})
		f.repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		expectNoComplaint(t, f)
		f.history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-1").Return(sentFor("jane@example.com", &listID), nil)
		f.unsubscriber.EXPECT().UnsubscribeFromMessageList(gomock.Any(), workspaceID, "jane@example.com", listID, "msg-1").Return(nil)
		f.history.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, marked("msg-1")).Return(nil)

		require.NoError(t, process(t, f, unsub("msg-1", "Jane@Example.com")))
	})

	// Transactional email, which Mailjet still sends, is sent for no list
	emptyList := ""
	for name, message := range map[string]*domain.MessageHistory{
		"an email sent for no list, such as a transactional one": sentFor("jane@example.com", nil),
		"an email stored with an empty list":                     sentFor("jane@example.com", &emptyList),
	} {
		message := message
		t.Run(name+": the email is marked unsubscribed, and no list is touched", func(t *testing.T) {
			f := setup(t)
			sentTo(f, map[string]string{"msg-1": "jane@example.com"})
			f.repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
			// gomock fails the test on a call to the list service
			gomock.InOrder(
				expectNoComplaint(t, f),
				f.history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-1").Return(message, nil),
				f.history.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, marked("msg-1")).Return(nil),
			)

			require.NoError(t, process(t, f, unsub("msg-1", "jane@example.com")))
		})
	}

	// Mailjet's webhooks cover the whole Mailjet account, and an email can be copied to
	// someone: such an unsubscribe is not even stored
	t.Run("an email Notifuse did not send, and an email to another address such as a copy: nothing is stored or recorded", func(t *testing.T) {
		f := setup(t)
		// gomock fails the test on any call past the check
		sentTo(f, map[string]string{"msg-2": "john@example.com"})

		require.NoError(t, process(t, f, unsub("msg-1", "jane@example.com"), unsub("msg-2", "jane@example.com")))
	})

	// The email is read again to record the opt-out, after the check: a contact erased in
	// between leaves it with the address redacted
	for name, message := range map[string]*domain.MessageHistory{
		"the email is gone":                             nil,
		"the email names another address":               sentFor("john@example.com", &listID),
		"its contact is erased":                         sentFor("DELETED_EMAIL", &listID),
		"the contact of an email for no list is erased": sentFor("DELETED_EMAIL", nil),
	} {
		message := message
		t.Run(name+" by the time the opt-out is recorded: nothing is recorded", func(t *testing.T) {
			f := setup(t)
			sentTo(f, map[string]string{"msg-1": "jane@example.com"})
			f.repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
			// gomock fails the test on a call to the list service, or on a second status update
			expectNoComplaint(t, f)
			f.history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-1").Return(message, nil)

			require.NoError(t, process(t, f, unsub("msg-1", "jane@example.com")))
		})
	}

	t.Run("one delivery: the emails of their own recipients are marked in one update", func(t *testing.T) {
		f := setup(t)
		sentTo(f, map[string]string{"msg-1": "msg-1@example.com", "msg-2": "john@example.com", "msg-3": "msg-3@example.com"})
		f.repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, events []*domain.InboundWebhookEvent) error {
				require.Len(t, events, 2, "the unsubscribe of another address than the email's is left out")
				return nil
			})
		expectNoComplaint(t, f)
		f.history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-1").Return(sentFor("msg-1@example.com", &listID), nil)
		f.history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-3").Return(sentFor("msg-3@example.com", nil), nil)
		f.unsubscriber.EXPECT().UnsubscribeFromMessageList(gomock.Any(), workspaceID, "msg-1@example.com", listID, "msg-1").Return(nil)
		f.history.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, marked("msg-1", "msg-3")).Return(nil)

		require.NoError(t, process(t, f,
			unsub("msg-1", "msg-1@example.com"), unsub("msg-2", "msg-2@example.com"), unsub("msg-3", "msg-3@example.com")))
	})

	// People who report spam often unsubscribe too, and Mailjet can post both in one delivery
	t.Run("a complaint and an unsubscribe in one delivery: the complaint first, the opt-out after", func(t *testing.T) {
		f := setup(t)
		spam := domain.MailjetWebhookPayload{
			Event: "spam", Time: unsubscribedAt.Unix(), Email: "john@example.com", MessageID: 12345, CustomID: "msg-1", Source: "JMRPP",
		}
		sentTo(f, map[string]string{"msg-1": "john@example.com", "msg-2": "jane@example.com"})
		f.repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		gomock.InOrder(
			f.history.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, updates []domain.MessageEventUpdate) error {
					require.Len(t, updates, 1)
					assert.Equal(t, "msg-1", updates[0].ID)
					assert.Equal(t, domain.MessageEventComplained, updates[0].Event)
					return nil
				}),
			f.history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-2").Return(sentFor("jane@example.com", &listID), nil),
			f.unsubscriber.EXPECT().UnsubscribeFromMessageList(gomock.Any(), workspaceID, "jane@example.com", listID, "msg-2").Return(nil),
			f.history.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, marked("msg-2")).Return(nil),
		)

		require.NoError(t, process(t, f, spam, unsub("msg-2", "jane@example.com")))
	})

	// An error answers the delivery with 400, and Mailjet posts it again
	t.Run("error - the email lookup fails", func(t *testing.T) {
		f := setup(t)
		sentTo(f, map[string]string{"msg-1": "jane@example.com"})
		f.repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		expectNoComplaint(t, f)
		f.history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-1").Return(nil, errors.New("connection refused"))

		err := process(t, f, unsub("msg-1", "jane@example.com"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to record unsubscribes")
		assert.Contains(t, err.Error(), "connection refused")
	})

	t.Run("error - the list service fails: the email is not marked", func(t *testing.T) {
		f := setup(t)
		sentTo(f, map[string]string{"msg-1": "jane@example.com"})
		f.repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		// gomock fails the test on a second status update
		expectNoComplaint(t, f)
		f.history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-1").Return(sentFor("jane@example.com", &listID), nil)
		f.unsubscriber.EXPECT().UnsubscribeFromMessageList(gomock.Any(), workspaceID, "jane@example.com", listID, "msg-1").
			Return(errors.New("connection refused"))

		err := process(t, f, unsub("msg-1", "jane@example.com"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to record unsubscribes")
		assert.Contains(t, err.Error(), "msg-1")
		assert.Contains(t, err.Error(), "connection refused")
	})

	// The opt-outs recorded before the failure are committed: their emails are marked now,
	// not on Mailjet's retry. The emails after it wait for the retry.
	t.Run("error - a later email's list opt-out fails: the emails before it are marked", func(t *testing.T) {
		f := setup(t)
		sentTo(f, map[string]string{"msg-1": "jane@example.com", "msg-2": "john@example.com", "msg-3": "jim@example.com"})
		f.repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		gomock.InOrder(
			expectNoComplaint(t, f),
			f.history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-1").Return(sentFor("jane@example.com", &listID), nil),
			f.unsubscriber.EXPECT().UnsubscribeFromMessageList(gomock.Any(), workspaceID, "jane@example.com", listID, "msg-1").Return(nil),
			f.history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-2").Return(sentFor("john@example.com", &listID), nil),
			f.unsubscriber.EXPECT().UnsubscribeFromMessageList(gomock.Any(), workspaceID, "john@example.com", listID, "msg-2").
				Return(errors.New("connection refused")),
			f.history.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, marked("msg-1")).Return(nil),
		)

		err := process(t, f, unsub("msg-1", "jane@example.com"), unsub("msg-2", "john@example.com"), unsub("msg-3", "jim@example.com"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to record unsubscribes")
		assert.Contains(t, err.Error(), "msg-2")
		assert.Contains(t, err.Error(), "connection refused")
	})

	t.Run("error - a later email's lookup fails: the emails before it are marked", func(t *testing.T) {
		f := setup(t)
		sentTo(f, map[string]string{"msg-1": "jane@example.com", "msg-2": "john@example.com"})
		f.repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		gomock.InOrder(
			expectNoComplaint(t, f),
			f.history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-1").Return(sentFor("jane@example.com", nil), nil),
			f.history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-2").Return(nil, errors.New("connection refused")),
			f.history.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, marked("msg-1")).Return(nil),
		)

		err := process(t, f, unsub("msg-1", "jane@example.com"), unsub("msg-2", "john@example.com"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to record unsubscribes")
		assert.Contains(t, err.Error(), "connection refused")
	})

	t.Run("error - a later email fails and marking the ones before it fails too: both errors are returned", func(t *testing.T) {
		f := setup(t)
		sentTo(f, map[string]string{"msg-1": "jane@example.com", "msg-2": "john@example.com"})
		f.repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		optOutErr := errors.New("list update: connection refused")
		markErr := errors.New("status update: connection refused")
		gomock.InOrder(
			expectNoComplaint(t, f),
			f.history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-1").Return(sentFor("jane@example.com", &listID), nil),
			f.unsubscriber.EXPECT().UnsubscribeFromMessageList(gomock.Any(), workspaceID, "jane@example.com", listID, "msg-1").Return(nil),
			f.history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-2").Return(sentFor("john@example.com", &listID), nil),
			f.unsubscriber.EXPECT().UnsubscribeFromMessageList(gomock.Any(), workspaceID, "john@example.com", listID, "msg-2").Return(optOutErr),
			f.history.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, marked("msg-1")).Return(markErr),
		)

		err := process(t, f, unsub("msg-1", "jane@example.com"), unsub("msg-2", "john@example.com"))
		require.Error(t, err)
		assert.ErrorIs(t, err, optOutErr)
		assert.ErrorIs(t, err, markErr)
		assert.Contains(t, err.Error(), "failed to record unsubscribes")
	})

	t.Run("error - marking the email fails", func(t *testing.T) {
		f := setup(t)
		sentTo(f, map[string]string{"msg-1": "jane@example.com"})
		f.repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		markErr := errors.New("connection refused")
		gomock.InOrder(
			expectNoComplaint(t, f),
			f.history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-1").Return(sentFor("jane@example.com", &listID), nil),
			f.unsubscriber.EXPECT().UnsubscribeFromMessageList(gomock.Any(), workspaceID, "jane@example.com", listID, "msg-1").Return(nil),
			f.history.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, marked("msg-1")).Return(markErr),
		)

		err := process(t, f, unsub("msg-1", "jane@example.com"))
		require.Error(t, err)
		assert.ErrorIs(t, err, markErr)
		assert.Contains(t, err.Error(), "failed to record unsubscribes")
	})
}

// A provider event may carry no message ID; Mailjet's always carries one, the CustomID or
// Mailjet's own.
func TestRecordUnsubscribes_SkipsEventsWithoutMessageID(t *testing.T) {
	service, _, _, _, _, ctrl := newClassificationTestService(t)
	defer ctrl.Finish()
	service.listUnsubscriber = servicemocks.NewMockListUnsubscriber(ctrl)
	empty := ""

	// gomock fails the test on any call to the message history repository or the list service
	err := service.recordUnsubscribes(context.Background(), "ws1", []*domain.InboundWebhookEvent{
		{Type: domain.EmailEventUnsubscribe, RecipientEmail: "jane@example.com"},
		{Type: domain.EmailEventUnsubscribe, RecipientEmail: "jane@example.com", MessageID: &empty},
	})
	require.NoError(t, err)
}

// TestListEvents tests the ListEvents method of WebhookEventService
func TestListEvents(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)
	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)

	// Setup logging expectations
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()

	// Create service
	service := &InboundWebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
		contactRepo:        contactRepo,
	}

	// Create test data
	workspaceID := "workspace1"
	user := &domain.User{ID: "user1"}
	// A member holding the read grant, so these behaviour tests pass the gate on the
	// permission rather than on the owner short-circuit.
	member := &domain.UserWorkspace{
		UserID:      user.ID,
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceWebhookEvents: {Read: true},
		},
	}
	now := time.Now().UTC()

	t.Run("Success case", func(t *testing.T) {
		// Create test params and expected result
		params := domain.InboundWebhookEventListParams{
			Limit:          10,
			WorkspaceID:    workspaceID,
			EventType:      domain.EmailEventBounce,
			RecipientEmail: "test@example.com",
		}

		msg1 := "message1"
		msg2 := "message2"
		expectedEvents := []*domain.InboundWebhookEvent{
			{
				ID:               "event1",
				Type:             domain.EmailEventBounce,
				Source:           domain.WebhookSourceSES,
				IntegrationID:    "integration1",
				RecipientEmail:   "test@example.com",
				MessageID:        &msg1,
				Timestamp:        now,
				BounceType:       "Permanent",
				BounceCategory:   "General",
				BounceDiagnostic: "550 User unknown",
				CreatedAt:        now,
			},
			{
				ID:               "event2",
				Type:             domain.EmailEventBounce,
				Source:           domain.WebhookSourceMailjet,
				IntegrationID:    "integration2",
				RecipientEmail:   "test@example.com",
				MessageID:        &msg2,
				Timestamp:        now,
				BounceType:       "HardBounce",
				BounceCategory:   "Permanent",
				BounceDiagnostic: "550 User unknown",
				CreatedAt:        now,
			},
		}

		expectedResult := &domain.InboundWebhookEventListResult{
			Events:     expectedEvents,
			NextCursor: "next-cursor",
			HasMore:    true,
		}

		// Setup mocks for authentication and repository
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(
			context.Background(), user, member, nil)
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
		params := domain.InboundWebhookEventListParams{
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
		params := domain.InboundWebhookEventListParams{
			Limit:       -1, // Invalid limit
			WorkspaceID: workspaceID,
		}

		// Setup mock for successful authentication but failed validation
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(
			context.Background(), user, member, nil)

		// Call method
		result, err := service.ListEvents(context.Background(), workspaceID, params)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid parameters")
	})

	t.Run("Repository error", func(t *testing.T) {
		params := domain.InboundWebhookEventListParams{
			Limit:       10,
			WorkspaceID: workspaceID,
		}

		// Setup mocks for successful authentication but repository error
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(
			context.Background(), user, member, nil)

		repoErr := errors.New("database error")
		repo.EXPECT().ListEvents(gomock.Any(), workspaceID, params).Return(nil, repoErr)

		// Call method
		result, err := service.ListEvents(context.Background(), workspaceID, params)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to list inbound webhook events")
	})

	t.Run("Empty result", func(t *testing.T) {
		params := domain.InboundWebhookEventListParams{
			Limit:       10,
			WorkspaceID: workspaceID,
		}

		// Setup mocks for successful authentication with empty result
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(
			context.Background(), user, member, nil)

		emptyResult := &domain.InboundWebhookEventListResult{
			Events:     []*domain.InboundWebhookEvent{},
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

// An SNS SubscribeURL arrives in the body of a public, unauthenticated endpoint, so
// it is hostile input. The confirmation fetch must refuse anything that is not an
// https AWS SNS host, and must not issue the request at all — reaching a loopback
// address and discarding the response is still a probe of the internal network.
//
// This covers the host check only. The SSRF-safe client behind it is a second layer
// against a host that passes that check and still resolves somewhere private, and no
// test here exercises it: that needs a controlled DNS answer, not a URL. Removing the
// safe client leaves this suite green, so do not read a pass as covering both.
func TestProcessSESWebhook_RefusesHostileSubscribeURL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()

	var hits int32
	victim := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer victim.Close()

	service := &InboundWebhookEventService{logger: log, httpClient: safehttpclient.New()}

	for _, target := range []string{
		victim.URL, // a loopback address is what an SSRF actually aims at
		"http://sns.us-east-1.amazonaws.com/?Action=ConfirmSubscription",   // right host, plaintext
		"https://sns.us-east-1.amazonaws.com.evil.example/?Action=Confirm", // suffix-matching trap
		"https://evil.example.com/?Action=ConfirmSubscription",
	} {
		raw, err := json.Marshal(domain.SESWebhookPayload{
			Type:         "SubscriptionConfirmation",
			TopicARN:     "arn:aws:sns:us-east-1:123456789:test-topic",
			SubscribeURL: target,
		})
		require.NoError(t, err)

		_, err = service.processSESWebhook("integration1", raw)
		require.Error(t, err, "SubscribeURL %q must be refused", target)
	}

	assert.Zero(t, atomic.LoadInt32(&hits),
		"a refused SubscribeURL must not be fetched at all, not merely ignored")
}

// TestInboundWebhookEventService_PermissionEnforcement pins the webhook_events
// gate on the one authenticated read this service exposes. The caller is a member
// granted the OPPOSITE permission, so the case fails both if the check is missing
// and if it asks for the wrong verb; the nil-membership case fails if the guard
// ever skips the check instead of denying. No repository expectation is set, so
// gomock also fails if anything past the gate runs.
func TestInboundWebhookEventService_PermissionEnforcement(t *testing.T) {
	const workspaceID = "ws-1"

	newService := func(t *testing.T, userWorkspace *domain.UserWorkspace) *InboundWebhookEventService {
		t.Helper()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		log := pkgmocks.NewMockLogger(ctrl)
		log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().Info(gomock.Any()).AnyTimes()
		log.EXPECT().Error(gomock.Any()).AnyTimes()

		authService := mocks.NewMockAuthService(ctrl)
		authService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			DoAndReturn(func(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
				return ctx, &domain.User{ID: "u1"}, userWorkspace, nil
			})

		return &InboundWebhookEventService{
			repo:        mocks.NewMockInboundWebhookEventRepository(ctrl),
			authService: authService,
			logger:      log,
		}
	}

	params := domain.InboundWebhookEventListParams{Limit: 10, WorkspaceID: workspaceID}

	t.Run("a member granted only write cannot read", func(t *testing.T) {
		svc := newService(t, &domain.UserWorkspace{
			UserID:      "u1",
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceWebhookEvents: {Write: true},
			},
		})

		result, err := svc.ListEvents(context.Background(), workspaceID, params)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.IsType(t, &domain.PermissionError{}, err)

		var permErr *domain.PermissionError
		require.True(t, errors.As(err, &permErr))
		assert.Equal(t, domain.PermissionResourceWebhookEvents, permErr.Resource)
		assert.Equal(t, domain.PermissionTypeRead, permErr.Permission)
	})

	t.Run("a nil membership is denied, not waved through", func(t *testing.T) {
		svc := newService(t, nil)

		result, err := svc.ListEvents(context.Background(), workspaceID, params)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.IsType(t, &domain.PermissionError{}, err)
	})
}

// Providers that batch events must give each event its own element of the
// batch as raw payload, never the whole batch: storing the batch on every event
// grows with the square of its size, and puts every recipient of the batch on
// each event's row.
func TestBatchedProviderEventsKeepTheirOwnPayload(t *testing.T) {
	service := &InboundWebhookEventService{}
	integrationID := "integration1"

	sendGridFirst := `{"email":"first@example.com","timestamp":1706097600,"event":"delivered","sg_message_id":"sg1","notifuse_message_id":"msg_1"}`
	sendGridSecond := `{"email":"second@example.com","timestamp":1706097600,"event":"bounce","type":"bounce","reason":"unknown user","sg_message_id":"sg2"}`
	sparkPostFirst := `{"msys":{"message_event":{"type":"delivery","rcpt_to":"first@example.com","message_id":"sp1","timestamp":"2023-01-01T12:00:00Z","rcpt_meta":{"notifuse_message_id":"msg_1"}}}}`
	sparkPostSecond := `{"msys":{"message_event":{"type":"bounce","rcpt_to":"second@example.com","message_id":"sp2","timestamp":"2023-01-01T12:00:00Z","bounce_class":"10"}}}`
	mailjetFirst := `{"event":"sent","time":1672574400,"MessageID":1,"email":"first@example.com"}`
	mailjetSecond := `{"event":"bounce","time":1672574400,"MessageID":2,"email":"second@example.com","hard_bounce":true}`

	cases := []struct {
		name    string
		process func(string, []byte) ([]*domain.InboundWebhookEvent, error)
		first   string
		second  string
	}{
		{"SendGrid", service.processSendGridWebhook, sendGridFirst, sendGridSecond},
		{"SparkPost", service.processSparkPostWebhook, sparkPostFirst, sparkPostSecond},
		{"Mailjet grouped events", service.processMailjetWebhook, mailjetFirst, mailjetSecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			batch := []byte("[\n  " + tc.first + ",\n  " + tc.second + "\n]")

			events, err := tc.process(integrationID, batch)

			require.NoError(t, err)
			require.Len(t, events, 2)
			assert.Equal(t, "first@example.com", events[0].RecipientEmail)
			assert.JSONEq(t, tc.first, events[0].RawPayload)
			assert.NotContains(t, events[0].RawPayload, "second@example.com")
			assert.Equal(t, "second@example.com", events[1].RecipientEmail)
			assert.JSONEq(t, tc.second, events[1].RawPayload)
			assert.NotContains(t, events[1].RawPayload, "first@example.com")
		})
	}

	t.Run("a single Mailjet event keeps the whole body", func(t *testing.T) {
		events, err := service.processMailjetWebhook(integrationID, []byte(mailjetFirst))

		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, mailjetFirst, events[0].RawPayload)
	})

	t.Run("a null SparkPost element fails the batch without a panic", func(t *testing.T) {
		events, err := service.processSparkPostWebhook(integrationID, []byte("["+sparkPostFirst+",null]"))

		require.Error(t, err)
		assert.Nil(t, events)
	})

	t.Run("a malformed element fails the batch", func(t *testing.T) {
		for name, process := range map[string]func(string, []byte) ([]*domain.InboundWebhookEvent, error){
			"SendGrid":  service.processSendGridWebhook,
			"SparkPost": service.processSparkPostWebhook,
			"Mailjet":   service.processMailjetWebhook,
		} {
			t.Run(name, func(t *testing.T) {
				events, err := process(integrationID, []byte(`[{"timestamp":"not a number","time":"not a number","msys":"not an object"}]`))

				require.Error(t, err)
				assert.Nil(t, events)
			})
		}
	})
}
