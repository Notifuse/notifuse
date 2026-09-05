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

	appconfig "github.com/Notifuse/notifuse/config"
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

	// SESTenant reports whether any SES integration in this workspace has
	// Notifuse-managed tenant isolation switched on.
	//
	// It reads the operator's intent — the flag they set — rather than what AWS
	// has actually provisioned: a workspace that asked for isolation is an
	// adopter of the feature while provisioning is still pending, and
	// ManagedTenantName stays empty until it succeeds. It is deliberately not
	// ResolveTenant(), which also answers true for a tenant the operator manages
	// outside Notifuse and would therefore count somebody else's setup as ours.
	//
	// Only the boolean is sent. A tenant name is an AWS resource identifier
	// minted per integration, and this payload carries no identifiers.
	SESTenant bool `json:"ses_tenant"`

	// RBACCustom reports whether any member or API key of this workspace holds
	// permissions that are not the full set.
	//
	// It exists to measure the blast radius of the RBAC gate: the share of
	// installations that restrict anybody today, and would therefore feel a
	// licence requirement on writing restricted permissions. Nothing else in
	// this payload answers that, and it is the single largest unmeasured number
	// in the open-core plan — every other gate's reach is already known from the
	// workspace and integration counts above.
	//
	// Only the boolean is sent: not who is restricted, not which resources, not
	// how many. The question is whether an installation uses the feature at all,
	// and a breakdown would describe a named person's access.
	//
	// (A custom-domain flag was considered alongside this one and dropped:
	// custom domains are free, so their adoption measures no gate.)
	RBACCustom bool `json:"rbac_custom"`

	// The fields below describe the installation rather than the workspace, so
	// they repeat identically on every row this instance sends — the same way
	// api_endpoint already does. That is the shape of this payload, and a
	// duplicated column is cheaper to live with than a second endpoint whose rows
	// would have to be correlated back.

	// Version is the release this binary was built as, taken from the compiled
	// constant rather than from Config.Version. The env-overridable value would
	// let a deployment report a release it is not running, and "which version is
	// this failure on" is the only question this field is here to answer.
	Version string `json:"version"`

	// OIDCEnabled reports the RESOLVED single sign-on setting, never the raw
	// OIDC_ENABLED variable. config/oidc.go decides it env-wins-else-database, so
	// an installation that switched SSO on from Settings has it true with no
	// variable in sight — and reading the environment would under-count exactly
	// the installations this number is about.
	//
	// Whether SSO is on is the question. No issuer, client id or provider name is
	// sent: those name the identity provider, and through it the company.
	OIDCEnabled bool `json:"oidc_enabled"`

	// LicenseTier is the plan this deployment is licensed for, or an empty string
	// when it is not licensed. Display-only here as everywhere else — nothing
	// branches on it — and it carries no key, no licence id, no organisation and
	// no billing contact.
	LicenseTier string `json:"license_tier"`
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

	// OIDCEnabled is the resolved cfg.OIDC.Enabled, not the environment variable.
	// The service has no access to *config.Config, so the already-resolved answer
	// has to be handed to it; re-deriving it here would get the database case
	// wrong. See the OIDCEnabled field on TelemetryMetrics.
	OIDCEnabled bool

	// Entitlements supplies the licence tier.
	//
	// A nil provider reports an unlicensed deployment, because that is what a
	// process with no licence service running is — telemetry must never be the
	// thing that takes an instance down, and a hard requirement here would trade
	// a wrong number for a dead scheduler. But it is not silent: leaving it nil is
	// a warning at construction, so a wiring change that drops it shows up in the
	// first log line rather than as a permanently empty license_tier column that
	// looks exactly like a fleet nobody has paid for.
	Entitlements domain.EntitlementProvider
}

// TelemetryService handles sending telemetry metrics
type TelemetryService struct {
	enabled       bool
	apiEndpoint   string
	workspaceRepo domain.WorkspaceRepository
	telemetryRepo domain.TelemetryRepository
	logger        logger.Logger
	httpClient    *http.Client
	oidcEnabled   bool
	entitlements  domain.EntitlementProvider
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

	if config.Entitlements == nil && config.Logger != nil {
		config.Logger.Warn("telemetry: no entitlement provider wired; license_tier will be reported empty for every workspace")
	}

	return &TelemetryService{
		enabled:       config.Enabled,
		apiEndpoint:   config.APIEndpoint,
		workspaceRepo: config.WorkspaceRepo,
		telemetryRepo: config.TelemetryRepo,
		logger:        config.Logger,
		httpClient:    httpClient,
		oidcEnabled:   config.OIDCEnabled,
		entitlements:  config.Entitlements,
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
		Version:            appconfig.VERSION,
		OIDCEnabled:        t.oidcEnabled,
		LicenseTier:        t.licenseTier(),
	}

	// Set integration flags from workspace integrations
	t.setIntegrationFlagsFromWorkspace(workspace, &metrics)

	// Whether anybody in this workspace is restricted. Read separately because it
	// lives in the system database's membership rows rather than in the workspace
	// row or the workspace database, and false on any failure like every other
	// signal here.
	metrics.RBACCustom = t.hasCustomPermissions(ctx, workspace.ID)

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
				// SES is a pointer where EmailProvider is a value, so an
				// integration whose settings failed to load arrives nil here
				// rather than zeroed — the same trap the LLM branch below
				// documents. The flag is OR-ed across integrations because a
				// workspace can hold several SES integrations and isolating one
				// of them is adopting the feature.
				if ses := integration.EmailProvider.SES; ses != nil && ses.TenantIsolationEnabled {
					metrics.SESTenant = true
				}
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

// licenseTier reports the plan this deployment is licensed for, or an empty
// string when it is not licensed.
//
// Grace counts as licensed and expired does not. An expired key grants exactly
// what no key grants — that is the whole point of there being no intermediate
// frozen state — so counting a lapsed deployment as a paying one would inflate
// the only number this field exists to produce. Entitlements.Licensed is the
// domain's own predicate for that distinction and this must not re-derive it.
//
// A nil provider reports the free tier rather than dereferencing. Telemetry runs
// on a background ticker, and licence handling never takes a process down.
func (t *TelemetryService) licenseTier() string {
	if t.entitlements == nil {
		return ""
	}

	entitlements := t.entitlements.Entitlements()
	if !entitlements.Licensed() {
		return ""
	}

	return entitlements.Tier
}

// hasCustomPermissions reports whether any member or API key of the workspace
// holds permissions other than the full set.
//
// It reads the system database's membership rows through
// GetWorkspaceUsersWithEmail, which is already on the repository interface this
// service holds — one extra read per workspace per day, against the same table
// users_count is drawn from, and no new query, interface method or mock.
//
// Owner rows are skipped. UserWorkspace.HasPermission returns true for an owner
// before it ever looks at the stored map, so an owner's persisted permissions are
// not load-bearing, and v39 normalised NULL permissions to '{}' — leaving plenty
// of owner rows holding a map that is not full and grants everything anyway.
// Counting those would report custom permissions on nearly every installation and
// make the number worthless.
//
// API keys do count. CreateAPIKey stores an ordinary membership row with role
// "member", full permissions unless a scope was requested, so a scoped key is
// precisely the restriction this measures.
//
// Any failure reports false, in keeping with the rest of this file: an
// installation whose system database hiccups must still produce a payload.
func (t *TelemetryService) hasCustomPermissions(ctx context.Context, workspaceID string) bool {
	if t.workspaceRepo == nil {
		return false
	}

	members, err := t.workspaceRepo.GetWorkspaceUsersWithEmail(ctx, workspaceID)
	if err != nil {
		return false
	}

	for _, member := range members {
		if member == nil || member.Role == "owner" {
			continue
		}
		if !grantsFullPermissions(member.Permissions) {
			return true
		}
	}

	return false
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
