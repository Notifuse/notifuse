package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelemetryService_SendMetricsForAllWorkspaces(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock repositories
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTelemetryRepo := mocks.NewMockTelemetryRepository(ctrl)

	// Create a test HTTP server
	var receivedRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRequests++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Temporarily override the TelemetryEndpoint constant for testing
	originalEndpoint := TelemetryEndpoint
	defer func() {
		// We can't actually change a const, but we can work around it
		// by creating a custom HTTP client that redirects to our test server
	}()

	// Create custom HTTP client that redirects to test server
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &testTransport{
			testServerURL: server.URL,
			originalURL:   originalEndpoint,
		},
	}

	// Create telemetry service
	config := TelemetryServiceConfig{
		Enabled:       true,
		APIEndpoint:   "https://api.example.com",
		WorkspaceRepo: mockWorkspaceRepo,
		TelemetryRepo: mockTelemetryRepo,
		Logger:        logger.NewLoggerWithLevel("debug"),
		HTTPClient:    httpClient,
	}

	service := NewTelemetryService(config)

	// Mock workspace list
	workspaces := []*domain.Workspace{
		{ID: "workspace1", Name: "Test Workspace 1"},
		{ID: "workspace2", Name: "Test Workspace 2"},
	}

	mockWorkspaceRepo.EXPECT().List(gomock.Any()).Return(workspaces, nil)

	// Mock telemetry repository calls
	mockTelemetryRepo.EXPECT().GetWorkspaceMetrics(gomock.Any(), "workspace1").Return(&domain.TelemetryMetrics{
		ContactsCount:      10,
		BroadcastsCount:    5,
		TransactionalCount: 3,
		MessagesCount:      25,
		ListsCount:         2,
		SegmentsCount:      4,
		UsersCount:         1,
		LastMessageAt:      "2023-01-01T00:00:00Z",
	}, nil)
	mockTelemetryRepo.EXPECT().GetWorkspaceMetrics(gomock.Any(), "workspace2").Return(&domain.TelemetryMetrics{
		ContactsCount:      15,
		BroadcastsCount:    8,
		TransactionalCount: 4,
		MessagesCount:      30,
		ListsCount:         3,
		SegmentsCount:      6,
		UsersCount:         2,
		LastMessageAt:      "2023-01-02T00:00:00Z",
	}, nil)

	// Execute
	ctx := context.Background()
	err := service.SendMetricsForAllWorkspaces(ctx)

	// Verify - should succeed even with database errors
	require.NoError(t, err)
	assert.Equal(t, 2, receivedRequests, "Should have sent metrics for 2 workspaces")
}

// testTransport is a custom HTTP transport for testing that redirects requests
type testTransport struct {
	testServerURL string
	originalURL   string
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.String() == t.originalURL {
		// Redirect to test server
		req.URL, _ = req.URL.Parse(t.testServerURL)
	}
	return http.DefaultTransport.RoundTrip(req)
}

func TestTelemetryService_DisabledService(t *testing.T) {
	// Create telemetry service with disabled configuration
	config := TelemetryServiceConfig{
		Enabled:     false,
		APIEndpoint: "https://api.example.com",
		Logger:      logger.NewLoggerWithLevel("debug"),
	}

	service := NewTelemetryService(config)

	// Execute
	ctx := context.Background()
	err := service.SendMetricsForAllWorkspaces(ctx)

	// Verify - should return without error and without making any calls
	require.NoError(t, err)
}

func TestTelemetryService_StartDailyScheduler(t *testing.T) {
	config := TelemetryServiceConfig{
		Enabled:     true,
		APIEndpoint: "https://api.example.com",
		Logger:      logger.NewLoggerWithLevel("debug"),
	}

	service := NewTelemetryService(config)

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the scheduler
	service.StartDailyScheduler(ctx)

	// The scheduler should start without error
	// We can't easily test the daily tick without waiting 24 hours,
	// but we can verify it doesn't panic or error on startup
	time.Sleep(100 * time.Millisecond) // Give it time to start

	// Cancel the context to stop the scheduler
	cancel()
	time.Sleep(100 * time.Millisecond) // Give it time to stop

	// Test passes if we reach here without panic
}

func TestTelemetryService_HardcodedEndpoint(t *testing.T) {
	// Verify that the hardcoded endpoint is used
	assert.Equal(t, "https://telemetry.notifuse.com", TelemetryEndpoint)
}

func TestTelemetryService_SetIntegrationFlags(t *testing.T) {
	config := TelemetryServiceConfig{
		Enabled:     true,
		APIEndpoint: "https://api.example.com",
		Logger:      logger.NewLoggerWithLevel("debug"),
	}

	service := NewTelemetryService(config)

	// Test workspace with various integrations
	workspace := &domain.Workspace{
		ID:   "test-workspace",
		Name: "Test Workspace",
		Integrations: domain.Integrations{
			{
				ID:   "mailgun-integration",
				Name: "Mailgun",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindMailgun,
				},
			},
			{
				ID:   "ses-integration",
				Name: "Amazon SES",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSES,
				},
			},
			{
				ID:   "smtp-integration",
				Name: "SMTP",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSMTP,
				},
			},
		},
	}

	// Test the integration flag setting
	metrics := TelemetryMetrics{}
	service.setIntegrationFlagsFromWorkspace(workspace, &metrics)

	// Verify that the correct flags are set
	assert.True(t, metrics.Mailgun, "Mailgun flag should be true")
	assert.True(t, metrics.AmazonSES, "AmazonSES flag should be true")
	assert.True(t, metrics.SMTP, "SMTP flag should be true")
	assert.False(t, metrics.Mailjet, "Mailjet flag should be false")
	assert.False(t, metrics.SparkPost, "SparkPost flag should be false")
	assert.False(t, metrics.Postmark, "Postmark flag should be false")

	// Test empty workspace
	emptyWorkspace := &domain.Workspace{
		ID:           "empty-workspace",
		Name:         "Empty Workspace",
		Integrations: domain.Integrations{},
	}

	emptyMetrics := TelemetryMetrics{}
	service.setIntegrationFlagsFromWorkspace(emptyWorkspace, &emptyMetrics)

	// Verify all flags are false
	assert.False(t, emptyMetrics.Mailgun, "All flags should be false for empty workspace")
	assert.False(t, emptyMetrics.AmazonSES, "All flags should be false for empty workspace")
	assert.False(t, emptyMetrics.SMTP, "All flags should be false for empty workspace")
	assert.False(t, emptyMetrics.Mailjet, "All flags should be false for empty workspace")
	assert.False(t, emptyMetrics.SparkPost, "All flags should be false for empty workspace")
	assert.False(t, emptyMetrics.Postmark, "All flags should be false for empty workspace")
}
