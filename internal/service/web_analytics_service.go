package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/cache"
	"github.com/Notifuse/notifuse/pkg/geoip"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
)

// workspaceSettingsCacheTTL matches Staminads' 60s workspace cache: beats
// arrive every few seconds per visitor and must not hammer the system DB.
const workspaceSettingsCacheTTL = 60 * time.Second

// Rate-limit namespaces for the identified ingest path. Sized well above a
// legitimate visitor's 10-30s heartbeat so a real session is never throttled.
const (
	webIdentifyEmailLimit = "wa_identify:email"
	webIdentifyIPLimit    = "wa_identify:ip"
)

// WebAnalyticsGeoLookup abstracts pkg/geoip for tests.
type WebAnalyticsGeoLookup interface {
	Lookup(ip string) (geoip.Result, error)
}

// WebAnalyticsService ingests tracking beats: resolve workspace → silent
// gates (enabled, allowed domains) → validate → enrich → buffer. It also
// exposes the console-facing attribution backfill controls.
// webAnalyticsWorkspace is what one cache entry holds. The secret key rides
// along because ResolveWebIdentity needs it on every identified beat and
// GetByID already decrypts it — fetching the workspace and then throwing the
// secret away would mean a second system-DB read per beat.
type webAnalyticsWorkspace struct {
	Settings  *domain.WebAnalyticsSettings
	SecretKey string
}

type WebAnalyticsService struct {
	workspaceRepo  domain.WorkspaceRepository
	contactRepo    domain.ContactRepository
	buffer         *WebAnalyticsBuffer
	geo            WebAnalyticsGeoLookup
	authService    domain.AuthService
	taskRepo       domain.TaskRepository
	logger         logger.Logger
	nowFn          func() time.Time
	workspaceCache *cache.InMemoryCache
	rateLimiter    *ratelimiter.RateLimiter
}

// ErrWebTrackInvalidPayload wraps payload validation failures so the handler
// can distinguish a malformed beat (400) from silently-dropped traffic (200).
type ErrWebTrackInvalidPayload struct{ Err error }

func (e *ErrWebTrackInvalidPayload) Error() string { return e.Err.Error() }
func (e *ErrWebTrackInvalidPayload) Unwrap() error { return e.Err }

// NewWebAnalyticsService creates the ingest service. authService and taskRepo
// back the console-facing backfill RPCs.
func NewWebAnalyticsService(
	workspaceRepo domain.WorkspaceRepository,
	contactRepo domain.ContactRepository,
	buffer *WebAnalyticsBuffer,
	geo WebAnalyticsGeoLookup,
	authService domain.AuthService,
	taskRepo domain.TaskRepository,
	rateLimiter *ratelimiter.RateLimiter,
	log logger.Logger,
) *WebAnalyticsService {
	return &WebAnalyticsService{
		workspaceRepo:  workspaceRepo,
		contactRepo:    contactRepo,
		buffer:         buffer,
		geo:            geo,
		authService:    authService,
		taskRepo:       taskRepo,
		logger:         log,
		rateLimiter:    rateLimiter,
		nowFn:          time.Now,
		workspaceCache: cache.NewInMemoryCache(5 * time.Minute),
	}
}

func backfillStatusFromTask(task *domain.Task) *domain.WebAnalyticsBackfillStatus {
	if task == nil {
		return nil
	}
	status := &domain.WebAnalyticsBackfillStatus{
		TaskID:   task.ID,
		Status:   string(task.Status),
		Progress: task.Progress,
	}
	if task.State != nil {
		status.State = task.State.WebAnalyticsBackfill
	}
	if task.ErrorMessage != nil {
		status.ErrorMessage = *task.ErrorMessage
	}
	return status
}

func (s *WebAnalyticsService) authorizeBackfill(ctx context.Context, workspaceID string, write bool) (context.Context, error) {
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return ctx, fmt.Errorf("failed to authenticate user: %w", err)
	}
	permission := domain.PermissionTypeRead
	if write {
		permission = domain.PermissionTypeWrite
	}
	if !userWorkspace.HasPermission(domain.PermissionResourceWebAnalytics, permission) {
		return ctx, domain.NewPermissionError(
			domain.PermissionResourceWebAnalytics,
			permission,
			fmt.Sprintf("Insufficient permissions: %s access to web_analytics required", permission),
		)
	}
	return ctx, nil
}

// latestBackfillTask returns the most recent backfill task, or nil.
func (s *WebAnalyticsService) latestBackfillTask(ctx context.Context, workspaceID string) (*domain.Task, error) {
	tasks, _, err := s.taskRepo.List(ctx, workspaceID, domain.TaskFilter{
		Type:  []string{domain.WebAnalyticsBackfillTaskType},
		Limit: 50,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list backfill tasks: %w", err)
	}
	var latest *domain.Task
	for _, task := range tasks {
		if latest == nil || task.CreatedAt.After(latest.CreatedAt) {
			latest = task
		}
	}
	return latest, nil
}

// BackfillStart launches an attribution backfill for the workspace.
func (s *WebAnalyticsService) BackfillStart(ctx context.Context, workspaceID string) (*domain.WebAnalyticsBackfillStatus, error) {
	ctx, err := s.authorizeBackfill(ctx, workspaceID, true)
	if err != nil {
		return nil, err
	}

	latest, err := s.latestBackfillTask(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if latest != nil && (latest.Status == domain.TaskStatusPending || latest.Status == domain.TaskStatusRunning || latest.Status == domain.TaskStatusPaused) {
		return backfillStatusFromTask(latest), fmt.Errorf("a backfill is already in progress")
	}

	now := s.nowFn().UTC()
	task := &domain.Task{
		WorkspaceID:   workspaceID,
		Type:          domain.WebAnalyticsBackfillTaskType,
		Status:        domain.TaskStatusPending,
		NextRunAfter:  &now,
		MaxRuntime:    50,
		MaxRetries:    3,
		RetryInterval: 60,
		State: &domain.TaskState{
			Message: "Attribution backfill queued",
		},
	}
	if err := s.taskRepo.Create(ctx, workspaceID, task); err != nil {
		return nil, fmt.Errorf("failed to create backfill task: %w", err)
	}
	return backfillStatusFromTask(task), nil
}

// BackfillStatus returns the latest backfill run (nil when none exists).
func (s *WebAnalyticsService) BackfillStatus(ctx context.Context, workspaceID string) (*domain.WebAnalyticsBackfillStatus, error) {
	ctx, err := s.authorizeBackfill(ctx, workspaceID, false)
	if err != nil {
		return nil, err
	}
	latest, err := s.latestBackfillTask(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return backfillStatusFromTask(latest), nil
}

// BackfillCancel aborts the in-flight backfill run.
func (s *WebAnalyticsService) BackfillCancel(ctx context.Context, workspaceID string) error {
	ctx, err := s.authorizeBackfill(ctx, workspaceID, true)
	if err != nil {
		return err
	}
	latest, err := s.latestBackfillTask(ctx, workspaceID)
	if err != nil {
		return err
	}
	if latest == nil || (latest.Status != domain.TaskStatusPending && latest.Status != domain.TaskStatusRunning && latest.Status != domain.TaskStatusPaused) {
		return fmt.Errorf("no backfill in progress")
	}
	latest.Status = domain.TaskStatusFailed
	cancelled := "cancelled by user"
	latest.ErrorMessage = &cancelled
	if err := s.taskRepo.Update(ctx, workspaceID, latest); err != nil {
		return fmt.Errorf("failed to cancel backfill: %w", err)
	}
	return nil
}

// Track processes one beat. Returns nil both on success and on silent drops
// (unknown/disabled workspace, disallowed origin, bot-ish traffic upstream);
// returns *ErrWebTrackInvalidPayload only for malformed payloads.
func (s *WebAnalyticsService) Track(ctx context.Context, payload *domain.WebTrackPayload, meta domain.WebRequestMeta) error {
	if payload == nil {
		return &ErrWebTrackInvalidPayload{Err: fmt.Errorf("empty payload")}
	}

	receivedAt := meta.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = s.nowFn()
	}

	resolved := s.webAnalyticsWorkspace(ctx, payload.WorkspaceID)
	if resolved == nil || resolved.Settings == nil || !resolved.Settings.Enabled {
		return nil // silent: unknown workspace or feature disabled
	}
	settings := resolved.Settings

	// Domain restriction against Origin, falling back to Referer. A rejection
	// is silent success (Staminads behavior): the SDK must not retry it.
	if len(settings.AllowedDomains) > 0 {
		hostname := webHostname(meta.Origin)
		if hostname == "" {
			hostname = webHostname(meta.Referer)
		}
		if !settings.MatchesAllowedDomain(hostname) {
			return nil
		}
	}

	if err := payload.Validate(receivedAt); err != nil {
		return &ErrWebTrackInvalidPayload{Err: err}
	}
	if len(payload.Actions) == 0 {
		return nil
	}

	// The SDK stopped sending a user agent inside attributes only when the
	// request header carries one; prefer the explicit attribute, then the
	// header, so enrichment always sees the best available signal.
	if payload.Attributes == nil {
		payload.Attributes = &domain.WebSessionAttributes{}
	}
	if payload.Attributes.UserAgent == "" {
		payload.Attributes.UserAgent = meta.UserAgent
	}

	var geoResult geoip.Result
	if s.geo != nil && (settings.GeoEnabled) && meta.ClientIP != "" {
		result, err := s.geo.Lookup(meta.ClientIP)
		if err != nil {
			s.logger.WithField("error", err.Error()).Error("GeoIP lookup failed")
		} else {
			geoResult = result
		}
	}

	contactEmail := s.resolveContactIdentity(ctx, payload, resolved.SecretKey, meta.ClientIP)

	session, pages, goals, err := BuildWebRows(payload, settings, geoResult, receivedAt, contactEmail)
	if err != nil {
		return &ErrWebTrackInvalidPayload{Err: err}
	}

	s.buffer.Add(payload.WorkspaceID, payload.TabID, session, pages, goals)
	return nil
}

// webAnalyticsSettings resolves the workspace's web analytics settings with a
// short TTL cache. Returns nil for unknown workspaces or absent settings.
func (s *WebAnalyticsService) webAnalyticsWorkspace(ctx context.Context, workspaceID string) *webAnalyticsWorkspace {
	if workspaceID == "" {
		return nil
	}
	cached, err := s.workspaceCache.GetOrSet("wa:"+workspaceID, workspaceSettingsCacheTTL, func() (interface{}, error) {
		workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
		if err != nil || workspace == nil {
			// Cache the miss too: unknown workspace ids must not turn into a
			// system-DB query per hostile beat.
			return (*webAnalyticsWorkspace)(nil), nil
		}
		return &webAnalyticsWorkspace{
			Settings:  workspace.Settings.WebAnalytics,
			SecretKey: workspace.Settings.SecretKey,
		}, nil
	})
	if err != nil {
		return nil
	}
	resolved, _ := cached.(*webAnalyticsWorkspace)
	return resolved
}

func (s *WebAnalyticsService) webAnalyticsSettings(ctx context.Context, workspaceID string) *domain.WebAnalyticsSettings {
	resolved := s.webAnalyticsWorkspace(ctx, workspaceID)
	if resolved == nil {
		return nil
	}
	return resolved.Settings
}

// resolveContactIdentity verifies the beat's credential and then requires the
// address to already be a contact.
//
// The signature proves who the caller is, never that the address belongs to
// anyone — so without this second gate a workspace's own signing key would let
// it store the email of people who are not contacts, and erasure would be
// unenforceable because a deleted contact's next beat would re-stamp it. Both
// problems disappear by refusing to remember an address we do not already hold.
//
// Every outcome is a silent drop of the IDENTITY only: a bad credential, an
// unknown address or a database hiccup must never cost the visitor their
// pageview.
func (s *WebAnalyticsService) resolveContactIdentity(ctx context.Context, payload *domain.WebTrackPayload, secretKey, clientIP string) *string {
	email, ok := domain.ResolveWebIdentity(payload, secretKey, s.nowFn())
	if !ok {
		return nil
	}
	if s.contactRepo == nil {
		return nil
	}

	// Throttle the IDENTIFIED path only, and before the contact lookup so an
	// abusive caller cannot spend database reads. Anonymous traffic is the
	// normal firehose and stays unthrottled. Exceeding the limit costs the
	// identity, never the beat — and never a 429, which the SDK would queue for
	// retry against something retrying cannot fix.
	if s.rateLimiter != nil {
		if !s.rateLimiter.Allow(webIdentifyEmailLimit, payload.WorkspaceID+"|"+email) {
			return nil
		}
		if clientIP != "" && !s.rateLimiter.Allow(webIdentifyIPLimit, clientIP) {
			return nil
		}
	}

	// An identified visitor beats every 10-30s, so this must not be a query per
	// beat. The TTL bounds how long a freshly created contact stays unrecognised
	// and how long a deleted one keeps resolving.
	key := "wa:contact:" + payload.WorkspaceID + ":" + email
	known, err := s.workspaceCache.GetOrSet(key, workspaceSettingsCacheTTL, func() (interface{}, error) {
		contact, err := s.contactRepo.GetContactByEmail(ctx, payload.WorkspaceID, email)
		return err == nil && contact != nil, nil
	})
	if err != nil {
		return nil
	}
	if exists, _ := known.(bool); !exists {
		return nil
	}
	return &email
}

// InvalidateWorkspaceCache drops the cached settings of one workspace (used
// after settings updates so changes apply within a beat, not a minute).
func (s *WebAnalyticsService) InvalidateWorkspaceCache(workspaceID string) {
	s.workspaceCache.Delete("wa:" + workspaceID)
}
