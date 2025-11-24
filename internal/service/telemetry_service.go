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
}

const (
	// TelemetryEndpoint is the hardcoded endpoint for sending telemetry data
	TelemetryEndpoint = "https://telemetry.notifuse.com"
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
	}

	// Send metrics to telemetry endpoint
	return t.sendMetrics(ctx, metrics)
}

// setIntegrationFlagsFromWorkspace sets boolean flags for each integration type from workspace integrations
func (t *TelemetryService) setIntegrationFlagsFromWorkspace(workspace *domain.Workspace, metrics *TelemetryMetrics) {
	// Iterate through workspace integrations and set flags based on email provider kind
	for _, integration := range workspace.Integrations {
		if integration.Type == domain.IntegrationTypeEmail {
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
		}
	}

	// Check if S3-compatible file storage is configured
	if t.isS3FileStorageConfigured(&workspace.Settings.FileManager) {
		metrics.S3 = true
	}
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
