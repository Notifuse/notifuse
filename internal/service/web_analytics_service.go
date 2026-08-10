package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/cache"
	"github.com/Notifuse/notifuse/pkg/geoip"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// workspaceSettingsCacheTTL matches Staminads' 60s workspace cache: beats
// arrive every few seconds per visitor and must not hammer the system DB.
const workspaceSettingsCacheTTL = 60 * time.Second

// WebAnalyticsGeoLookup abstracts pkg/geoip for tests.
type WebAnalyticsGeoLookup interface {
	Lookup(ip string) (geoip.Result, error)
}

// WebAnalyticsService ingests tracking beats: resolve workspace → silent
// gates (enabled, allowed domains) → validate → enrich → buffer. It also
// exposes the console-facing attribution backfill controls.
type WebAnalyticsService struct {
	workspaceRepo  domain.WorkspaceRepository
	buffer         *WebAnalyticsBuffer
	geo            WebAnalyticsGeoLookup
	authService    domain.AuthService
	taskRepo       domain.TaskRepository
	logger         logger.Logger
	nowFn          func() time.Time
	workspaceCache *cache.InMemoryCache
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
	buffer *WebAnalyticsBuffer,
	geo WebAnalyticsGeoLookup,
	authService domain.AuthService,
	taskRepo domain.TaskRepository,
	log logger.Logger,
) *WebAnalyticsService {
	return &WebAnalyticsService{
		workspaceRepo:  workspaceRepo,
		buffer:         buffer,
		geo:            geo,
		authService:    authService,
		taskRepo:       taskRepo,
		logger:         log,
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

	settings := s.webAnalyticsSettings(ctx, payload.WorkspaceID)
	if settings == nil || !settings.Enabled {
		return nil // silent: unknown workspace or feature disabled
	}

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

	session, pages, goals, err := BuildWebRows(payload, settings, geoResult, receivedAt)
	if err != nil {
		return &ErrWebTrackInvalidPayload{Err: err}
	}

	s.buffer.Add(payload.WorkspaceID, session, pages, goals)
	return nil
}

// webAnalyticsSettings resolves the workspace's web analytics settings with a
// short TTL cache. Returns nil for unknown workspaces or absent settings.
func (s *WebAnalyticsService) webAnalyticsSettings(ctx context.Context, workspaceID string) *domain.WebAnalyticsSettings {
	if workspaceID == "" {
		return nil
	}
	cached, err := s.workspaceCache.GetOrSet("wa:"+workspaceID, workspaceSettingsCacheTTL, func() (interface{}, error) {
		workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
		if err != nil || workspace == nil {
			// Cache the miss too: unknown workspace ids must not turn into a
			// system-DB query per hostile beat.
			return (*domain.WebAnalyticsSettings)(nil), nil
		}
		return workspace.Settings.WebAnalytics, nil
	})
	if err != nil {
		return nil
	}
	settings, _ := cached.(*domain.WebAnalyticsSettings)
	return settings
}

// InvalidateWorkspaceCache drops the cached settings of one workspace (used
// after settings updates so changes apply within a beat, not a minute).
func (s *WebAnalyticsService) InvalidateWorkspaceCache(workspaceID string) {
	s.workspaceCache.Delete("wa:" + workspaceID)
}
