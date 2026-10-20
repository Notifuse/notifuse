package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMailjetService_RegisterWebhooks_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	testLogger := logger.NewLogger()

	// Create service with mocks
	service := NewMailjetService(mockHTTPClient, mockAuthService, testLogger)

	// Test data
	workspaceID := "workspace-123"
	integrationID := "integration-456"
	baseURL := "https://api.notifuse.com"

	providerConfig := &domain.EmailProvider{
		Kind: domain.EmailProviderKindMailjet,
		Mailjet: &domain.MailjetSettings{
			APIKey:    "test-api-key",
			SecretKey: "test-secret-key",
		},
	}

	// Test event types to register
	eventTypes := []domain.EmailEventType{
		domain.EmailEventDelivered,
		domain.EmailEventBounce,
	}

	// Expected webhook URL
	expectedWebhookURL := domain.GenerateWebhookCallbackURL(baseURL, domain.EmailProviderKindMailjet, workspaceID, integrationID)

	// Response for ListWebhooks - empty list
	emptyResponse := domain.MailjetWebhookResponse{
		Count: 0,
		Data:  []domain.MailjetWebhook{},
		Total: 0,
	}

	// Response for created webhook - for domain.MailjetEventSent
	createdWebhookSent := domain.MailjetWebhook{
		ID:        1001,
		EventType: string(domain.MailjetEventSent),
		Endpoint:  expectedWebhookURL,
		Status:    "active",
	}

	// Response for created webhook - for domain.MailjetEventBounce
	createdWebhookBounce := domain.MailjetWebhook{
		ID:        1002,
		EventType: string(domain.MailjetEventBounce),
		Endpoint:  expectedWebhookURL,
		Status:    "active",
	}

	// Response for created webhook - for domain.MailjetEventBlocked
	createdWebhookBlocked := domain.MailjetWebhook{
		ID:        1003,
		EventType: string(domain.MailjetEventBlocked),
		Endpoint:  expectedWebhookURL,
		Status:    "active",
	}

	ctx := context.Background()

	// Setup mock for ListWebhooks
	mockHTTPClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			// Check that it's a GET request to the eventcallback endpoint
			if req.Method == "GET" && req.URL.String() == "https://api.mailjet.com/v3/eventcallback" {
				return mockHTTPResponse(t, http.StatusOK, emptyResponse), nil
			}
			return mockHTTPResponse(t, http.StatusOK, emptyResponse), nil
		}).AnyTimes()

	// Setup mock for creating sent webhook (for EmailEventDelivered)
	mockHTTPClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			// Check that it's a POST request to create a webhook
			if req.Method == "POST" && req.URL.String() == "https://api.mailjet.com/v3/eventcallback" {
				return mockHTTPResponse(t, http.StatusCreated, createdWebhookSent), nil
			}
			return mockHTTPResponse(t, http.StatusCreated, createdWebhookSent), nil
		}).AnyTimes()

	// Setup mock for creating bounce webhook (for EmailEventBounce - first mapping)
	mockHTTPClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			// Check that it's a POST request to create a webhook
			if req.Method == "POST" && req.URL.String() == "https://api.mailjet.com/v3/eventcallback" {
				return mockHTTPResponse(t, http.StatusCreated, createdWebhookBounce), nil
			}
			return mockHTTPResponse(t, http.StatusCreated, createdWebhookBounce), nil
		}).AnyTimes()

	// Setup mock for creating blocked webhook (for EmailEventBounce - second mapping)
	mockHTTPClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			// Check that it's a POST request to create a webhook
			if req.Method == "POST" && req.URL.String() == "https://api.mailjet.com/v3/eventcallback" {
				return mockHTTPResponse(t, http.StatusCreated, createdWebhookBlocked), nil
			}
			return mockHTTPResponse(t, http.StatusCreated, createdWebhookBlocked), nil
		}).AnyTimes()

	// Call the service method
	status, err := service.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, providerConfig)

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, domain.EmailProviderKindMailjet, status.EmailProviderKind)
	assert.True(t, status.IsRegistered)

	// Check endpoints for the registered event types
	assert.NotEmpty(t, status.Endpoints)

	// Verify that we have endpoints for both event types
	var hasDeliveredEndpoint, hasBounceEndpoint bool
	for _, endpoint := range status.Endpoints {
		if endpoint.EventType == domain.EmailEventDelivered {
			hasDeliveredEndpoint = true
		}
		if endpoint.EventType == domain.EmailEventBounce {
			hasBounceEndpoint = true
		}
	}

	assert.True(t, hasDeliveredEndpoint, "Should have an endpoint for delivered events")
	assert.True(t, hasBounceEndpoint, "Should have an endpoint for bounce events")
}

// A complaint registration creates a spam and an unsub webhook at Mailjet. The webhook records
// an unsub event as an unsubscribe, so the status reports that webhook as an unsubscribe
// endpoint, and the spam one as a complaint endpoint.
func TestMailjetService_RegisterWebhooks_ReportsTheUnsubWebhookAsAnUnsubscribeEndpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	service := NewMailjetService(mockHTTPClient, mocks.NewMockAuthService(ctrl), logger.NewLogger())
	providerConfig := &domain.EmailProvider{
		Kind: domain.EmailProviderKindMailjet,
		Mailjet: &domain.MailjetSettings{
			APIKey:    "test-api-key",
			SecretKey: "test-secret-key",
		},
	}

	// The Mailjet event type of each webhook the registration creates, and the ID Mailjet gives it
	created := map[string]string{}
	mockHTTPClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			switch req.Method {
			case http.MethodGet:
				return mockHTTPResponse(t, http.StatusOK, domain.MailjetWebhookResponse{}), nil
			case http.MethodPost:
				var webhook domain.MailjetWebhook
				if err := json.NewDecoder(req.Body).Decode(&webhook); err != nil {
					t.Errorf("failed to decode the webhook creation request: %v", err)
				}
				webhook.ID = int64(301 + len(created))
				created[webhook.EventType] = strconv.FormatInt(webhook.ID, 10)
				return mockHTTPResponse(t, http.StatusCreated, webhook), nil
			}
			t.Errorf("unexpected request: %s %s", req.Method, req.URL)
			return nil, errors.New("unexpected request")
		}).
		AnyTimes()

	status, err := service.RegisterWebhooks(context.Background(), "workspace-123", "integration-456",
		"https://api.notifuse.com", []domain.EmailEventType{domain.EmailEventComplaint}, providerConfig)

	require.NoError(t, err)
	require.NotNil(t, status)
	require.Len(t, created, 2)
	require.Contains(t, created, string(domain.MailjetEventSpam))
	require.Contains(t, created, string(domain.MailjetEventUnsub))
	require.Len(t, status.Endpoints, 2)
	got := make(map[string]domain.EmailEventType, len(status.Endpoints))
	for _, endpoint := range status.Endpoints {
		got[endpoint.WebhookID] = endpoint.EventType
	}
	assert.Equal(t, map[string]domain.EmailEventType{
		created[string(domain.MailjetEventSpam)]:  domain.EmailEventComplaint,
		created[string(domain.MailjetEventUnsub)]: domain.EmailEventUnsubscribe,
	}, got)
}
