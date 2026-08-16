package service

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// TelemetryMetrics represents the metrics data sent to the telemetry endpoint
type TelemetryMetrics struct {
	WorkspaceIDSHA1    string `json:"workspace_id_sha1"`
	WorkspaceCreatedAt string `json:"workspace_created_at"`
	WorkspaceUpdatedAt string `json:"workspace_updated_at"`
	LastMessageAt      string `json:"last_message_at"`
	ContactsCount      int    `json:"contacts_count"`
	BroadcastsCount    int    `json:"broadcasts_count"`
	TransactionalCount int    `json:"transactional_count"`
	MessagesCount      int    `json:"messages_count"`
	ListsCount         int    `json:"lists_count"`
	SegmentsCount      int    `json:"segments_count"`
	UsersCount         int    `json:"users_count"`
	BlogPostsCount     int    `json:"blog_posts_count"`
	APIEndpoint        string `json:"api_endpoint"`

	// Integration flags - boolean for each email provider
	Mailgun   bool `json:"mailgun"`
	AmazonSES bool `json:"amazonses"`
	Mailjet   bool `json:"mailjet"`
	SparkPost bool `json:"sparkpost"`
	Postmark  bool `json:"postmark"`
	SMTP      bool `json:"smtp"`
	S3        bool `json:"s3"`

	// Non-email integrations. The LLM integration is reported per provider,
	// like email, because which model vendor a workspace connects is the
	// actionable part; the other two have nothing to sub-divide.
	//
	// SendGrid is deliberately absent: it was dropped from this payload in
	// October 2025 and stays dropped.
	Anthropic bool `json:"anthropic"`
	OpenAI    bool `json:"openai"`
	Gemini    bool `json:"gemini"`
	Supabase  bool `json:"supabase"`
	Firecrawl bool `json:"firecrawl"`

	// WebAnalytics reports whether the workspace is actually collecting web
	// analytics, not whether the feature is switched on: a workspace can enable
	// it and never install the snippet, and one that collected traffic for
	// months can clear its settings without the data going anywhere.
	//
	// Only the boolean is sent, never the date it derives from. The question
	// this payload asks is whether the feature is used, and a date answers more
	// than that.
	WebAnalytics bool `json:"web_analytics"`
}

const (
	// TelemetryEndpoint is the hardcoded endpoint for sending telemetry data
	TelemetryEndpoint = "https://telemetry.notifuse.com"

	// WebAnalyticsActiveDays is how recently a workspace must have recorded a
	// web analytics session to count as using the feature. Wide enough that a
	// low-traffic site does not flicker between reports, narrow enough that a
	// workspace which stopped months ago is not still counted as an adopter.
	WebAnalyticsActiveDays = 30
)

// TelemetryServiceConfig contains configuration for the telemetry service
type TelemetryServiceConfig struct {
	Enabled       bool
	APIEndpoint   string
	WorkspaceRepo domain.WorkspaceRepository
	TelemetryRepo domain.TelemetryRepository
	Logger        logger.Logger
	HTTPClient    *http.Client
}

// TelemetryService handles sending telemetry metrics
type TelemetryService struct {
	enabled       bool
	apiEndpoint   string
	workspaceRepo domain.WorkspaceRepository
	telemetryRepo domain.TelemetryRepository
	logger        logger.Logger
	httpClient    *http.Client
}

// NewTelemetryService creates a new telemetry service
func NewTelemetryService(config TelemetryServiceConfig) *TelemetryService {
	// Use a default HTTP client with 5 second timeout if none provided
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 5 * time.Second,
		}
	}

	return &TelemetryService{
		enabled:       config.Enabled,
		apiEndpoint:   config.APIEndpoint,
		workspaceRepo: config.WorkspaceRepo,
		telemetryRepo: config.TelemetryRepo,
		logger:        config.Logger,
		httpClient:    httpClient,
	}
}

// SendMetricsForAllWorkspaces collects and sends telemetry metrics for all workspaces
func (t *TelemetryService) SendMetricsForAllWorkspaces(ctx context.Context) error {
	if !t.enabled {
		return nil
	}

	// Get all workspaces
	workspaces, err := t.workspaceRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list workspaces: %w", err)
	}

	// Collect and send metrics for each workspace
	for _, workspace := range workspaces {
		_ = t.sendMetricsForWorkspace(ctx, workspace)
		// Continue with other workspaces on error
	}

	return nil
}

// sendMetricsForWorkspace collects and sends telemetry metrics for a specific workspace
func (t *TelemetryService) sendMetricsForWorkspace(ctx context.Context, workspace *domain.Workspace) error {
	// Create SHA1 hash of workspace ID
	hasher := sha1.New()
	hasher.Write([]byte(workspace.ID))
	workspaceIDSHA1 := hex.EncodeToString(hasher.Sum(nil))

	// Collect metrics
	metrics := TelemetryMetrics{
		WorkspaceIDSHA1:    workspaceIDSHA1,
		WorkspaceCreatedAt: workspace.CreatedAt.Format(time.RFC3339),
		WorkspaceUpdatedAt: workspace.UpdatedAt.Format(time.RFC3339),
		APIEndpoint:        t.apiEndpoint,
	}

	// Set integration flags from workspace integrations
	t.setIntegrationFlagsFromWorkspace(workspace, &metrics)

	// Get telemetry metrics from repository
	if telemetryMetrics, err := t.telemetryRepo.GetWorkspaceMetrics(ctx, workspace.ID); err == nil {
		metrics.ContactsCount = telemetryMetrics.ContactsCount
		metrics.BroadcastsCount = telemetryMetrics.BroadcastsCount
		metrics.TransactionalCount = telemetryMetrics.TransactionalCount
		metrics.MessagesCount = telemetryMetrics.MessagesCount
		metrics.ListsCount = telemetryMetrics.ListsCount
		metrics.SegmentsCount = telemetryMetrics.SegmentsCount
		metrics.UsersCount = telemetryMetrics.UsersCount
		metrics.BlogPostsCount = telemetryMetrics.BlogPostsCount
		metrics.LastMessageAt = telemetryMetrics.LastMessageAt
		metrics.WebAnalytics = isWebAnalyticsActive(telemetryMetrics.LastWebSessionAt, time.Now())
	}

	// Send metrics to telemetry endpoint
	return t.sendMetrics(ctx, metrics)
}

// setIntegrationFlagsFromWorkspace sets boolean flags for each integration type from workspace integrations
func (t *TelemetryService) setIntegrationFlagsFromWorkspace(workspace *domain.Workspace, metrics *TelemetryMetrics) {
	// Iterate through workspace integrations and set a flag per configured
	// provider. EmailProviderKindSendGrid is intentionally unhandled: SendGrid
	// is still a supported provider, but it was removed from the telemetry
	// payload in October 2025 and is not reported.
	for _, integration := range workspace.Integrations {
		switch integration.Type {
		case domain.IntegrationTypeEmail:
			switch integration.EmailProvider.Kind {
			case domain.EmailProviderKindMailgun:
				metrics.Mailgun = true
			case domain.EmailProviderKindSES:
				metrics.AmazonSES = true
			case domain.EmailProviderKindMailjet:
				metrics.Mailjet = true
			case domain.EmailProviderKindPostmark:
				metrics.Postmark = true
			case domain.EmailProviderKindSMTP:
				metrics.SMTP = true
			case domain.EmailProviderKindSparkPost:
				metrics.SparkPost = true
			}

		case domain.IntegrationTypeLLM:
			// LLMProvider is a pointer where EmailProvider is a value, so an
			// integration whose settings failed to load nil-panics on .Kind
			// rather than falling through to no flag.
			if integration.LLMProvider == nil {
				continue
			}
			switch integration.LLMProvider.Kind {
			case domain.LLMProviderKindAnthropic:
				metrics.Anthropic = true
			case domain.LLMProviderKindOpenAI:
				metrics.OpenAI = true
			case domain.LLMProviderKindGemini:
				metrics.Gemini = true
			}

		case domain.IntegrationTypeSupabase:
			metrics.Supabase = true

		case domain.IntegrationTypeFirecrawl:
			metrics.Firecrawl = true
		}
	}

	// Check if S3-compatible file storage is configured
	if t.isS3FileStorageConfigured(&workspace.Settings.FileManager) {
		metrics.S3 = true
	}
}

// isWebAnalyticsActive reports whether the last recorded web analytics session
// is recent enough for the workspace to count as using the feature.
//
// lastWebSessionAt is a session_date: a UTC calendar day, not an instant. The
// cutoff is therefore taken from the start of the current UTC day, so the answer
// does not depend on what time of day the daily telemetry run happens to fire.
// An unparseable or absent date means no usage rather than an error, because a
// workspace database without the web analytics tables must still produce a
// payload.
func isWebAnalyticsActive(lastWebSessionAt string, now time.Time) bool {
	if lastWebSessionAt == "" {
		return false
	}

	sessionDate, err := time.Parse(time.RFC3339, lastWebSessionAt)
	if err != nil {
		return false
	}

	// The window is WebAnalyticsActiveDays calendar days counted inclusively:
	// today plus the days before it. Stepping a full WebAnalyticsActiveDays back
	// and then accepting that day too would make the window one day wider than
	// the field claims to mean.
	today := now.UTC().Truncate(24 * time.Hour)
	cutoff := today.AddDate(0, 0, -(WebAnalyticsActiveDays - 1))

	return !sessionDate.Before(cutoff)
}

// isS3FileStorageConfigured checks if S3-compatible file storage is configured in workspace settings
func (t *TelemetryService) isS3FileStorageConfigured(fileManager *domain.FileManagerSettings) bool {
	return fileManager.Endpoint != "" && fileManager.Bucket != "" && fileManager.AccessKey != ""
}

// sendMetrics sends the collected metrics to the telemetry endpoint
func (t *TelemetryService) sendMetrics(ctx context.Context, metrics TelemetryMetrics) error {
	// Marshal metrics to JSON
	jsonData, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal telemetry metrics: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", TelemetryEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create telemetry request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Notifuse-Telemetry/1.0")

	// Send request (will fail silently if endpoint is offline due to 5s timeout)
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil // Fail silently as requested
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response status
	if resp.StatusCode >= 400 {
		return nil // Fail silently as requested
	}

	return nil
}

// StartDailyScheduler starts a goroutine that sends telemetry metrics daily
func (t *TelemetryService) StartDailyScheduler(ctx context.Context) {
	if !t.enabled {
		return
	}

	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = t.SendMetricsForAllWorkspaces(ctx)
			}
		}
	}()
}
