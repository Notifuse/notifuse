package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
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

	// Create a test HTTP server that keeps the payloads it is sent, so the
	// assertions below cover what actually goes over the wire rather than the
	// struct that was filled in — a field can be set on TelemetryMetrics and
	// still never be marshalled.
	var mu sync.Mutex
	var receivedPayloads []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		mu.Lock()
		receivedPayloads = append(receivedPayloads, payload)
		mu.Unlock()
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
	// workspace1 recorded a web session yesterday; workspace2 last recorded one
	// well outside the active window.
	mockTelemetryRepo.EXPECT().GetWorkspaceMetrics(gomock.Any(), "workspace1").Return(&domain.TelemetryMetrics{
		ContactsCount:      10,
		BroadcastsCount:    5,
		TransactionalCount: 3,
		MessagesCount:      25,
		ListsCount:         2,
		SegmentsCount:      4,
		UsersCount:         1,
		LastMessageAt:      "2023-01-01T00:00:00Z",
		LastWebSessionAt:   time.Now().UTC().AddDate(0, 0, -1).Format(time.RFC3339),
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
		LastWebSessionAt:   time.Now().UTC().AddDate(0, 0, -90).Format(time.RFC3339),
	}, nil)

	// Execute
	ctx := context.Background()
	err := service.SendMetricsForAllWorkspaces(ctx)

	// Verify - should succeed even with database errors
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, receivedPayloads, 2, "Should have sent metrics for 2 workspaces")

	// Workspaces are sent in list order, so the first payload is workspace1's.
	assert.Equal(t, true, receivedPayloads[0]["web_analytics"],
		"a session recorded yesterday counts as using web analytics")
	assert.Equal(t, false, receivedPayloads[1]["web_analytics"],
		"a session recorded 90 days ago does not")

	// The session date itself must never leave the installation.
	for i, payload := range receivedPayloads {
		assert.NotContains(t, payload, "last_web_session_at",
			"payload %d must carry the boolean only", i)
	}
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

func TestTelemetryService_SetNonEmailIntegrationFlags(t *testing.T) {
	service := NewTelemetryService(TelemetryServiceConfig{
		Enabled:     true,
		APIEndpoint: "https://api.example.com",
		Logger:      logger.NewLoggerWithLevel("debug"),
	})

	workspace := &domain.Workspace{
		ID: "test-workspace",
		Integrations: domain.Integrations{
			{ID: "llm-1", Type: domain.IntegrationTypeLLM,
				LLMProvider: &domain.LLMProvider{Kind: domain.LLMProviderKindAnthropic}},
			{ID: "llm-2", Type: domain.IntegrationTypeLLM,
				LLMProvider: &domain.LLMProvider{Kind: domain.LLMProviderKindGemini}},
			{ID: "firecrawl-1", Type: domain.IntegrationTypeFirecrawl},
		},
	}

	metrics := TelemetryMetrics{}
	service.setIntegrationFlagsFromWorkspace(workspace, &metrics)

	assert.True(t, metrics.Anthropic, "Anthropic LLM integration should be reported")
	assert.True(t, metrics.Gemini, "Gemini LLM integration should be reported")
	assert.True(t, metrics.Firecrawl, "Firecrawl integration should be reported")
	assert.False(t, metrics.OpenAI, "no OpenAI integration is configured")
	assert.False(t, metrics.Supabase, "no Supabase integration is configured")

	// An email flag must not be raised by a non-email integration.
	assert.False(t, metrics.SMTP)
	assert.False(t, metrics.Mailgun)
}

func TestTelemetryService_SetIntegrationFlags_SupabaseAndOpenAI(t *testing.T) {
	service := NewTelemetryService(TelemetryServiceConfig{
		Enabled: true, Logger: logger.NewLoggerWithLevel("debug"),
	})

	workspace := &domain.Workspace{
		ID: "test-workspace",
		Integrations: domain.Integrations{
			{ID: "supabase-1", Type: domain.IntegrationTypeSupabase},
			{ID: "llm-1", Type: domain.IntegrationTypeLLM,
				LLMProvider: &domain.LLMProvider{Kind: domain.LLMProviderKindOpenAI}},
		},
	}

	metrics := TelemetryMetrics{}
	service.setIntegrationFlagsFromWorkspace(workspace, &metrics)

	assert.True(t, metrics.Supabase)
	assert.True(t, metrics.OpenAI)
	assert.False(t, metrics.Anthropic)
	assert.False(t, metrics.Gemini)
}

func TestTelemetryService_SetIntegrationFlags_NilLLMProviderDoesNotPanic(t *testing.T) {
	service := NewTelemetryService(TelemetryServiceConfig{
		Enabled: true, Logger: logger.NewLoggerWithLevel("debug"),
	})

	// LLMProvider is a pointer, so an integration row whose settings never
	// loaded reaches this code as nil. It must yield no flag, not a panic.
	workspace := &domain.Workspace{
		ID: "test-workspace",
		Integrations: domain.Integrations{
			{ID: "llm-broken", Type: domain.IntegrationTypeLLM, LLMProvider: nil},
			{ID: "firecrawl-1", Type: domain.IntegrationTypeFirecrawl},
		},
	}

	metrics := TelemetryMetrics{}
	require.NotPanics(t, func() {
		service.setIntegrationFlagsFromWorkspace(workspace, &metrics)
	})

	assert.False(t, metrics.Anthropic)
	assert.False(t, metrics.OpenAI)
	assert.False(t, metrics.Gemini)
	// The loop must carry on past the broken integration.
	assert.True(t, metrics.Firecrawl, "a nil LLMProvider must not abort the loop")
}

func TestTelemetryService_SendGridIsNotReported(t *testing.T) {
	service := NewTelemetryService(TelemetryServiceConfig{
		Enabled: true, Logger: logger.NewLoggerWithLevel("debug"),
	})

	// SendGrid is still a supported email provider but was deliberately
	// removed from the telemetry payload in October 2025. A SendGrid
	// integration must raise no flag at all, and must not be mistaken for
	// another provider.
	workspace := &domain.Workspace{
		ID: "test-workspace",
		Integrations: domain.Integrations{
			{ID: "sendgrid-1", Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSendGrid}},
		},
	}

	metrics := TelemetryMetrics{}
	service.setIntegrationFlagsFromWorkspace(workspace, &metrics)

	assert.Equal(t, TelemetryMetrics{}, metrics, "a SendGrid-only workspace reports no integration flag")
}

func TestIsWebAnalyticsActive(t *testing.T) {
	// Late in the UTC day, so a bug that measures the window from "now" rather
	// than from the start of the day shifts the boundary by 22 hours and fails.
	now := time.Date(2026, 8, 16, 22, 30, 0, 0, time.UTC)

	// Dates are written out rather than derived from WebAnalyticsActiveDays: a
	// case computed from the constant it is meant to protect moves with it, and
	// would keep passing if the window were widened to 60 days. session_date is
	// a DATE column, so every value the repository produces is midnight UTC.
	//
	// The window is the 30 days ending today, i.e. 2026-07-18 .. 2026-08-16.
	tests := []struct {
		name             string
		lastWebSessionAt string
		want             bool
	}{
		{"session today", "2026-08-16T00:00:00Z", true},
		{"session yesterday", "2026-08-15T00:00:00Z", true},
		{"session one day inside the window", "2026-07-19T00:00:00Z", true},
		{"session on the oldest day in the window", "2026-07-18T00:00:00Z", true},
		{"session one day older than the window", "2026-07-17T00:00:00Z", false},
		{"session long past the window", "2025-08-16T00:00:00Z", false},
		{"never recorded a session", "", false},
		{"unparseable date", "not-a-date", false},
		// A workspace whose clock or partition ran ahead still counts as active.
		{"session dated in the future", "2026-08-17T00:00:00Z", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isWebAnalyticsActive(tt.lastWebSessionAt, now))
		})
	}
}

func TestIsWebAnalyticsActive_UsesUTCDayRegardlessOfLocalZone(t *testing.T) {
	// 2026-08-17 01:00 +09:00 is still 2026-08-16 in UTC, and session_date is
	// stored in UTC — so the window must be measured there, not in whatever zone
	// the daily scheduler happens to fire in.
	tokyo := time.FixedZone("JST", 9*60*60)
	now := time.Date(2026, 8, 17, 1, 0, 0, 0, tokyo)

	// 01:00 +09:00 on the 17th is 16:00 UTC on the 16th, so the window is the 30
	// days ending 2026-08-16 — not the one ending 2026-08-17.
	oldestInWindow := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	justOutside := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)

	assert.True(t, isWebAnalyticsActive(oldestInWindow, now), "the 30th UTC day back is inside the window")
	assert.False(t, isWebAnalyticsActive(justOutside, now), "the 31st UTC day back is outside it")
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
