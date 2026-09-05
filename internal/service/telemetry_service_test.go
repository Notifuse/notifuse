package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	appconfig "github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
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

	// Every workspace is probed for restricted permissions; both of these hold
	// none, so rbac_custom stays false for both payloads.
	mockWorkspaceRepo.EXPECT().GetWorkspaceUsersWithEmail(gomock.Any(), "workspace1").
		Return([]*domain.UserWorkspaceWithEmail{}, nil)
	mockWorkspaceRepo.EXPECT().GetWorkspaceUsersWithEmail(gomock.Any(), "workspace2").
		Return([]*domain.UserWorkspaceWithEmail{}, nil)

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

// telemetryCapture wires a TelemetryService to a test server that keeps every
// payload it receives. The assertions that matter here are wire-level: a field
// can be set on TelemetryMetrics and never marshalled, and a json tag typo would
// be invisible to a struct comparison.
type telemetryCapture struct {
	service *TelemetryService

	mu       sync.Mutex
	payloads []map[string]interface{}
}

func newTelemetryCapture(t *testing.T, config TelemetryServiceConfig) *telemetryCapture {
	t.Helper()

	capture := &telemetryCapture{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		capture.mu.Lock()
		capture.payloads = append(capture.payloads, payload)
		capture.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	config.Enabled = true
	config.HTTPClient = &http.Client{
		Timeout: 5 * time.Second,
		Transport: &testTransport{
			testServerURL: server.URL,
			originalURL:   TelemetryEndpoint,
		},
	}
	if config.Logger == nil {
		config.Logger = logger.NewLoggerWithLevel("debug")
	}

	capture.service = NewTelemetryService(config)

	return capture
}

// onlyPayload returns the single payload sent, failing if a different number was.
func (c *telemetryCapture) onlyPayload(t *testing.T) map[string]interface{} {
	t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()
	require.Len(t, c.payloads, 1, "exactly one workspace payload should have been sent")

	return c.payloads[0]
}

func TestTelemetryService_InstanceLevelFlags(t *testing.T) {
	active := domain.Entitlements{
		Tier:          "agency",
		Org:           "ACME SAS",
		MaxWorkspaces: 15,
		Features:      []domain.Feature{domain.FeatureRBAC, domain.FeatureSESTenant},
		State:         domain.LicenseStateActive,
	}

	// A key whose renewal payment is still being retried keeps everything it
	// granted, so it is still a paying deployment and still reports its tier.
	grace := active
	grace.Tier = "studio"
	grace.State = domain.LicenseStateGrace

	// An expired key grants exactly what no key grants. Entitlements keeps the
	// tier so the console can say whose key ran out, but telemetry must not count
	// the deployment as licensed.
	expired := domain.CommunityEntitlements()
	expired.State = domain.LicenseStateExpired
	expired.Tier = "agency"

	unlicensed := domain.CommunityEntitlements()

	tests := []struct {
		name            string
		oidcEnabled     bool
		entitlements    *domain.Entitlements
		wantOIDC        bool
		wantLicenseTier string
	}{
		{
			name:            "single sign-on off and no licence service wired",
			oidcEnabled:     false,
			entitlements:    nil,
			wantOIDC:        false,
			wantLicenseTier: "",
		},
		{
			name:            "single sign-on resolved on",
			oidcEnabled:     true,
			entitlements:    &unlicensed,
			wantOIDC:        true,
			wantLicenseTier: "",
		},
		{
			name:            "an active licence reports its tier",
			oidcEnabled:     false,
			entitlements:    &active,
			wantOIDC:        false,
			wantLicenseTier: "agency",
		},
		{
			name:            "a licence inside its grace period still reports its tier",
			oidcEnabled:     false,
			entitlements:    &grace,
			wantOIDC:        false,
			wantLicenseTier: "studio",
		},
		{
			name:            "an expired licence reports no tier",
			oidcEnabled:     false,
			entitlements:    &expired,
			wantOIDC:        false,
			wantLicenseTier: "",
		},
		{
			name:            "an unlicensed installation reports no tier",
			oidcEnabled:     false,
			entitlements:    &unlicensed,
			wantOIDC:        false,
			wantLicenseTier: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			mockTelemetryRepo := mocks.NewMockTelemetryRepository(ctrl)

			mockWorkspaceRepo.EXPECT().List(gomock.Any()).
				Return([]*domain.Workspace{{ID: "workspace1"}}, nil)
			mockWorkspaceRepo.EXPECT().GetWorkspaceUsersWithEmail(gomock.Any(), "workspace1").
				Return([]*domain.UserWorkspaceWithEmail{}, nil)
			mockTelemetryRepo.EXPECT().GetWorkspaceMetrics(gomock.Any(), "workspace1").
				Return(&domain.TelemetryMetrics{}, nil)

			// A nil provider is the deployment with no licence service wired at
			// all. It must report the free tier rather than dereference.
			var entitlementProvider domain.EntitlementProvider
			if tt.entitlements != nil {
				provider := mocks.NewMockEntitlementProvider(ctrl)
				provider.EXPECT().Entitlements().Return(*tt.entitlements).AnyTimes()
				entitlementProvider = provider
			}

			capture := newTelemetryCapture(t, TelemetryServiceConfig{
				APIEndpoint:   "https://api.example.com",
				WorkspaceRepo: mockWorkspaceRepo,
				TelemetryRepo: mockTelemetryRepo,
				OIDCEnabled:   tt.oidcEnabled,
				Entitlements:  entitlementProvider,
			})

			require.NoError(t, capture.service.SendMetricsForAllWorkspaces(context.Background()))

			payload := capture.onlyPayload(t)
			assert.Equal(t, tt.wantOIDC, payload["oidc_enabled"],
				"oidc_enabled reports the resolved setting handed to the service")
			assert.Equal(t, tt.wantLicenseTier, payload["license_tier"])

			// The running build, on every payload. Read from the compiled
			// constant, so an operator's VERSION override cannot make a
			// deployment misreport which release it is on.
			assert.Equal(t, appconfig.VERSION, payload["version"])
			assert.NotEmpty(t, payload["version"], "every payload names a build")

			// Nothing that identifies the licensee travels with the tier.
			assert.NotContains(t, payload, "license_key")
			assert.NotContains(t, payload, "org")
			assert.NotContains(t, payload, "sub")
		})
	}
}

func TestTelemetryService_SESTenantFlag(t *testing.T) {
	service := NewTelemetryService(TelemetryServiceConfig{
		Enabled: true, Logger: logger.NewLoggerWithLevel("debug"),
	})

	sesIntegration := func(id string, ses *domain.AmazonSESSettings) domain.Integration {
		return domain.Integration{
			ID:   id,
			Type: domain.IntegrationTypeEmail,
			EmailProvider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES:  ses,
			},
		}
	}

	tests := []struct {
		name         string
		integrations domain.Integrations
		want         bool
	}{
		{
			name:         "ses integration without tenant isolation",
			integrations: domain.Integrations{sesIntegration("ses-1", &domain.AmazonSESSettings{})},
			want:         false,
		},
		{
			name: "ses integration with tenant isolation",
			integrations: domain.Integrations{
				sesIntegration("ses-1", &domain.AmazonSESSettings{TenantIsolationEnabled: true}),
			},
			want: true,
		},
		{
			name: "isolation on one of several ses integrations",
			integrations: domain.Integrations{
				sesIntegration("ses-1", &domain.AmazonSESSettings{}),
				sesIntegration("ses-2", &domain.AmazonSESSettings{TenantIsolationEnabled: true}),
				sesIntegration("ses-3", &domain.AmazonSESSettings{}),
			},
			want: true,
		},
		{
			name: "isolation requested but not yet provisioned",
			integrations: domain.Integrations{
				sesIntegration("ses-1", &domain.AmazonSESSettings{
					TenantIsolationEnabled: true,
					ManagedTenantName:      "",
				}),
			},
			want: true,
		},
		{
			// ResolveTenant would answer true here. The flag deliberately does
			// not: a tenant the operator manages in their own AWS account is not
			// Notifuse tenant isolation.
			name: "operator-managed tenant name without isolation",
			integrations: domain.Integrations{
				sesIntegration("ses-1", &domain.AmazonSESSettings{TenantName: "acme"}),
			},
			want: false,
		},
		{
			// EmailProvider is a value but SES is a pointer, so an integration
			// whose settings never loaded arrives nil.
			name:         "ses integration whose settings failed to load",
			integrations: domain.Integrations{sesIntegration("ses-1", nil)},
			want:         false,
		},
		{
			name: "no ses integration at all",
			integrations: domain.Integrations{
				{ID: "mailgun-1", Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindMailgun}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := &domain.Workspace{ID: "test-workspace", Integrations: tt.integrations}

			metrics := TelemetryMetrics{}
			require.NotPanics(t, func() {
				service.setIntegrationFlagsFromWorkspace(workspace, &metrics)
			})

			assert.Equal(t, tt.want, metrics.SESTenant)
		})
	}
}

func TestTelemetryService_HasCustomPermissions(t *testing.T) {
	member := func(role string, userType domain.UserType, permissions domain.UserPermissions) *domain.UserWorkspaceWithEmail {
		return &domain.UserWorkspaceWithEmail{
			UserWorkspace: domain.UserWorkspace{
				UserID:      "user-" + role,
				WorkspaceID: "workspace1",
				Role:        role,
				Permissions: permissions,
			},
			Type: userType,
		}
	}

	// One resource short of the full set. Written by removing an entry from the
	// canonical map rather than by listing resources, so it stays "not full" when
	// a resource is added to domain.AllPermissionResources.
	missingOneResource := domain.NewFullPermissions()
	delete(missingOneResource, domain.PermissionResourceContacts)

	readOnlyOnOneResource := domain.NewFullPermissions()
	readOnlyOnOneResource[domain.PermissionResourceContacts] = domain.ResourcePermissions{Read: true}

	tests := []struct {
		name    string
		members []*domain.UserWorkspaceWithEmail
		listErr error
		want    bool
	}{
		{
			name: "every member holds full permissions",
			members: []*domain.UserWorkspaceWithEmail{
				member("owner", domain.UserTypeUser, domain.NewFullPermissions()),
				member("member", domain.UserTypeUser, domain.NewFullPermissions()),
			},
			want: false,
		},
		{
			name: "a member is restricted to a single resource",
			members: []*domain.UserWorkspaceWithEmail{
				member("owner", domain.UserTypeUser, domain.NewFullPermissions()),
				member("member", domain.UserTypeUser, domain.UserPermissions{
					domain.PermissionResourceContacts: {Read: true, Write: true},
				}),
			},
			want: true,
		},
		{
			name: "a member is missing one resource",
			members: []*domain.UserWorkspaceWithEmail{
				member("member", domain.UserTypeUser, missingOneResource),
			},
			want: true,
		},
		{
			name: "a member has read but not write on one resource",
			members: []*domain.UserWorkspaceWithEmail{
				member("member", domain.UserTypeUser, readOnlyOnOneResource),
			},
			want: true,
		},
		{
			// API keys are ordinary membership rows with role "member",
			// distinguished only by users.type, and a scoped key is exactly the
			// restriction this signal measures.
			name: "a scoped api key counts",
			members: []*domain.UserWorkspaceWithEmail{
				member("owner", domain.UserTypeUser, domain.NewFullPermissions()),
				member("member", domain.UserTypeAPIKey, domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				}),
			},
			want: true,
		},
		{
			name: "an unscoped api key does not count",
			members: []*domain.UserWorkspaceWithEmail{
				member("member", domain.UserTypeAPIKey, domain.NewFullPermissions()),
			},
			want: false,
		},
		{
			// Owners bypass their stored map entirely in HasPermission, and v39
			// normalised NULL permissions to '{}'. Counting these would report a
			// restriction on nearly every installation.
			name: "an owner row holding an empty permission map",
			members: []*domain.UserWorkspaceWithEmail{
				member("owner", domain.UserTypeUser, domain.UserPermissions{}),
			},
			want: false,
		},
		{
			name: "an owner row holding no permission map at all",
			members: []*domain.UserWorkspaceWithEmail{
				member("owner", domain.UserTypeUser, nil),
			},
			want: false,
		},
		{
			name: "a member row holding no permission map at all",
			members: []*domain.UserWorkspaceWithEmail{
				member("member", domain.UserTypeUser, nil),
			},
			want: true,
		},
		{
			name:    "a workspace with no membership rows",
			members: []*domain.UserWorkspaceWithEmail{},
			want:    false,
		},
		{
			name:    "the membership read fails",
			listErr: assert.AnError,
			want:    false,
		},
		{
			name:    "a nil row is skipped rather than dereferenced",
			members: []*domain.UserWorkspaceWithEmail{nil},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			mockWorkspaceRepo.EXPECT().GetWorkspaceUsersWithEmail(gomock.Any(), "workspace1").
				Return(tt.members, tt.listErr)

			service := NewTelemetryService(TelemetryServiceConfig{
				Enabled:       true,
				WorkspaceRepo: mockWorkspaceRepo,
				Logger:        logger.NewLoggerWithLevel("debug"),
			})

			var got bool
			require.NotPanics(t, func() {
				got = service.hasCustomPermissions(context.Background(), "workspace1")
			})

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTelemetryService_LicensingFlagsReachTheWire(t *testing.T) {
	// The json tags are the contract with the receiving function; a struct-level
	// assertion would pass with any of them misspelled.
	tests := []struct {
		name            string
		integrations    domain.Integrations
		members         []*domain.UserWorkspaceWithEmail
		oidcEnabled     bool
		entitlements    domain.Entitlements
		wantSESTenant   bool
		wantRBACCustom  bool
		wantOIDC        bool
		wantLicenseTier string
	}{
		{
			name: "an unlicensed installation using none of the gated capabilities",
			integrations: domain.Integrations{
				{ID: "ses-1", Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindSES,
						SES:  &domain.AmazonSESSettings{},
					}},
			},
			members: []*domain.UserWorkspaceWithEmail{
				{UserWorkspace: domain.UserWorkspace{Role: "owner", Permissions: domain.NewFullPermissions()}},
			},
			oidcEnabled:     false,
			entitlements:    domain.CommunityEntitlements(),
			wantSESTenant:   false,
			wantRBACCustom:  false,
			wantOIDC:        false,
			wantLicenseTier: "",
		},
		{
			name: "a licensed installation using all of them",
			integrations: domain.Integrations{
				{ID: "ses-1", Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindSES,
						SES:  &domain.AmazonSESSettings{TenantIsolationEnabled: true},
					}},
			},
			members: []*domain.UserWorkspaceWithEmail{
				{UserWorkspace: domain.UserWorkspace{Role: "owner", Permissions: domain.NewFullPermissions()}},
				{UserWorkspace: domain.UserWorkspace{Role: "member", Permissions: domain.UserPermissions{
					domain.PermissionResourceContacts: {Read: true},
				}}},
			},
			oidcEnabled: true,
			entitlements: domain.Entitlements{
				Tier:          "enterprise",
				MaxWorkspaces: 15,
				Features:      []domain.Feature{domain.FeatureSSO},
				State:         domain.LicenseStateActive,
			},
			wantSESTenant:   true,
			wantRBACCustom:  true,
			wantOIDC:        true,
			wantLicenseTier: "enterprise",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			mockTelemetryRepo := mocks.NewMockTelemetryRepository(ctrl)
			mockEntitlements := mocks.NewMockEntitlementProvider(ctrl)

			mockWorkspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{
				{ID: "workspace1", Integrations: tt.integrations},
			}, nil)
			mockWorkspaceRepo.EXPECT().GetWorkspaceUsersWithEmail(gomock.Any(), "workspace1").
				Return(tt.members, nil)
			mockTelemetryRepo.EXPECT().GetWorkspaceMetrics(gomock.Any(), "workspace1").
				Return(&domain.TelemetryMetrics{UsersCount: len(tt.members)}, nil)
			mockEntitlements.EXPECT().Entitlements().Return(tt.entitlements).AnyTimes()

			capture := newTelemetryCapture(t, TelemetryServiceConfig{
				APIEndpoint:   "https://api.example.com",
				WorkspaceRepo: mockWorkspaceRepo,
				TelemetryRepo: mockTelemetryRepo,
				OIDCEnabled:   tt.oidcEnabled,
				Entitlements:  mockEntitlements,
			})

			require.NoError(t, capture.service.SendMetricsForAllWorkspaces(context.Background()))

			payload := capture.onlyPayload(t)
			assert.Equal(t, tt.wantSESTenant, payload["ses_tenant"])
			assert.Equal(t, tt.wantRBACCustom, payload["rbac_custom"])
			assert.Equal(t, tt.wantOIDC, payload["oidc_enabled"])
			assert.Equal(t, tt.wantLicenseTier, payload["license_tier"])
			assert.Equal(t, appconfig.VERSION, payload["version"])

			// The flags answer whether a capability is used, never by whom or on
			// what. Nothing naming a person, a tenant or a resource goes with them.
			for _, forbidden := range []string{"tenant_name", "managed_tenant_name", "permissions", "members", "emails"} {
				assert.NotContains(t, payload, forbidden)
			}
		})
	}
}

func TestTelemetryService_MembershipReadFailureStillProducesAPayload(t *testing.T) {
	// Every other signal in this file is best-effort, and this one has to be too:
	// a system database that hiccups must cost the rbac_custom flag, not the
	// whole day's telemetry.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTelemetryRepo := mocks.NewMockTelemetryRepository(ctrl)

	mockWorkspaceRepo.EXPECT().List(gomock.Any()).
		Return([]*domain.Workspace{{ID: "workspace1"}}, nil)
	mockWorkspaceRepo.EXPECT().GetWorkspaceUsersWithEmail(gomock.Any(), "workspace1").
		Return(nil, assert.AnError)
	mockTelemetryRepo.EXPECT().GetWorkspaceMetrics(gomock.Any(), "workspace1").
		Return(nil, assert.AnError)

	capture := newTelemetryCapture(t, TelemetryServiceConfig{
		APIEndpoint:   "https://api.example.com",
		WorkspaceRepo: mockWorkspaceRepo,
		TelemetryRepo: mockTelemetryRepo,
	})

	require.NoError(t, capture.service.SendMetricsForAllWorkspaces(context.Background()))

	payload := capture.onlyPayload(t)
	assert.Equal(t, false, payload["rbac_custom"], "an unreadable membership list reports no restriction")
	assert.Equal(t, appconfig.VERSION, payload["version"])
}

// TestNewTelemetryService_WarnsWhenEntitlementsAreUnwired covers the one
// remaining way license_tier can be wrong without anybody noticing.
//
// The provider is deliberately not required — telemetry never takes a process
// down, and a scheduler that refuses to start because the licence service is
// missing would be a worse trade than a wrong column. But a config struct field
// that silently defaults is exactly the shape of an omission nobody sees, so the
// omission is announced instead.
func TestNewTelemetryService_WarnsWhenEntitlementsAreUnwired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("an unwired provider is announced once at construction", func(t *testing.T) {
		var warnings []string
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().Warn(gomock.Any()).Do(func(message string) {
			warnings = append(warnings, message)
		}).AnyTimes()

		service := NewTelemetryService(TelemetryServiceConfig{
			APIEndpoint: "https://api.example.com",
			Logger:      mockLogger,
		})

		require.NotNil(t, service)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "license_tier")
		assert.Equal(t, "", service.licenseTier(), "an unwired provider reports the free tier")
	})

	t.Run("a wired provider says nothing", func(t *testing.T) {
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().Warn(gomock.Any()).Times(0)

		mockEntitlements := mocks.NewMockEntitlementProvider(ctrl)
		mockEntitlements.EXPECT().Entitlements().Return(domain.Entitlements{
			Tier:  "studio",
			State: domain.LicenseStateActive,
		}).AnyTimes()

		service := NewTelemetryService(TelemetryServiceConfig{
			APIEndpoint:  "https://api.example.com",
			Logger:       mockLogger,
			Entitlements: mockEntitlements,
		})

		require.NotNil(t, service)
		assert.Equal(t, "studio", service.licenseTier())
	})
}
