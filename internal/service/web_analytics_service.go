package service

import (
	"context"
	"fmt"
	"strings"
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
	// webIdentifyCreateLimit bounds how fast one workspace's contact list can
	// grow through /track. The per-email and per-IP limits above cannot: they
	// bound how often ONE address beats, and a caller minting a fresh address per
	// request never trips either.
	webIdentifyCreateLimit = "wa_identify:create"
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

	contactEmail := s.resolveContactIdentity(ctx, payload, resolved.SecretKey, meta.ClientIP, webContactSeed{
		Country:  geoResult.Country,
		Language: payload.Attributes.Language,
		Timezone: payload.Attributes.Timezone,
	})

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

// Column widths of the contacts fields a beat may seed. Mirrored here because
// the alternative is discovering them from a failed INSERT on a public endpoint.
const (
	// contacts.country VARCHAR(100). Language needs no width constant: the
	// supported-language allowlist bounds it far inside its own column.
	webContactCountryMaxLength = 100
)

// fits reports whether a seed value is present and short enough to store.
//
// len() counts bytes and the columns count characters, so this is conservative
// in the only direction that matters: it can reject a value that would have fit,
// never accept one that would abort the INSERT.
func fits(value string, max int) bool {
	return value != "" && len(value) <= max
}

// webSupportedLanguage maps a browser language tag onto a code the product
// actually supports, or "" when none matches.
//
// The full tag is tried first and the primary subtag only as a fallback,
// because SupportedLanguages carries both forms for the languages that need
// them — "pt" and "pt-BR", "zh" and "zh-TW". Cutting at the hyphen
// unconditionally turned a Brazilian visitor into "pt", which no pt-BR template
// translation will ever match. The allowlist also replaces a hand-rolled
// "two or three lowercase letters" test that happily stored "xx".
func webSupportedLanguage(tag string) string {
	tag = strings.TrimSpace(tag)
	if domain.IsValidLanguage(tag) {
		return tag
	}
	if primary, _, found := strings.Cut(tag, "-"); found && domain.IsValidLanguage(primary) {
		return primary
	}
	return ""
}

// webContactSeed is the context a beat can contribute to a contact it creates.
// Only what the visitor's own browser and the geo lookup already told us —
// nothing is inferred, and an unusable value is dropped rather than guessed.
type webContactSeed struct {
	Country  string
	Language string
	Timezone string
}

// resolveContactIdentity verifies the beat's credential and resolves it to a
// contact, creating one when the address is not held yet.
//
// Creating is safe precisely because of what the credential is: an HMAC over the
// workspace secret, or a token Notifuse itself minted. Only the customer's own
// server can produce one, so an address arriving here has already been vouched
// for by the same authority that an API contact.create call carries. What the
// signature does NOT prove is that the visitor is that person, which is why the
// credential is domain-separated from the notification-center HMAC and why the
// created contact joins no list.
//
// The cost of this decision, stated where it is taken: erasure no longer holds
// by construction. A deleted contact whose browser still holds the credential is
// re-created by its next beat, and only the customer can stop that by no longer
// calling identify() for the address.
//
// Every outcome is a silent drop of the IDENTITY only: a bad credential, a
// throttle or a database hiccup must never cost the visitor their pageview.
func (s *WebAnalyticsService) resolveContactIdentity(ctx context.Context, payload *domain.WebTrackPayload, secretKey, clientIP string, seed webContactSeed) *string {
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
	// and how long a deleted one keeps resolving. Shares the helper with the
	// bridge so both treat a transient lookup failure the same way: costly for
	// this beat, never remembered.
	if webContactExists(ctx, s.workspaceCache, s.contactRepo, "wa:contact:", payload.WorkspaceID, email) {
		return &email
	}
	if !s.createContact(ctx, payload.WorkspaceID, email, seed) {
		return nil
	}
	return &email
}

// createContact adds the verified address, reporting whether it may now be used
// as an identity. Returns false for every uncertain outcome, so a session is
// only ever stamped with an address the database actually holds.
//
// Creating a contact here is not silent, and that was accepted rather than
// overlooked: the contacts INSERT fires the change trigger, which writes a
// contact.created row, which is a valid automation trigger kind. A workspace with
// a welcome automation will therefore mail someone who did nothing but browse a
// page that called identify(). Suppressing it would take a source marker on
// contacts and a redefinition of the trigger; the judgement is that a signed
// identify() is as deliberate an act as an API contact.create, and firing the
// onboarding is usually the intent.
func (s *WebAnalyticsService) createContact(ctx context.Context, workspaceID, email string, seed webContactSeed) bool {
	// Keyed per workspace, not per address: the limits upstream bound how often
	// ONE address beats, which a caller minting a fresh address per request never
	// trips. This is the only thing standing between a leaked workspace secret and
	// an unbounded contact list.
	if s.rateLimiter != nil && !s.rateLimiter.Allow(webIdentifyCreateLimit, workspaceID) {
		s.logger.WithField("workspace_id", workspaceID).
			Warn("Web analytics identify: contact creation throttled, identity dropped for this beat")
		return false
	}

	// Each seed field is dropped rather than truncated when it does not fit its
	// column, and that matters more than it looks: these values come from a
	// public endpoint, nothing upstream bounds them (WebTrackPayload.Validate
	// bounds paths, goal names and dimensions, but not the browser's reported
	// language), and an over-long value makes the INSERT fail. Because the
	// contact is then never created, the same visitor's every subsequent beat
	// retries and fails identically — a permanent, silent identity outage for
	// that one address. A missing country is a far smaller loss than that.
	contact := &domain.Contact{Email: email}
	if fits(seed.Country, webContactCountryMaxLength) {
		contact.Country = &domain.NullableString{String: seed.Country}
	}
	// navigator.language is a BCP-47 tag ("en-US"); everything that consumes
	// contacts.language treats it as a bare ISO 639-1 code — Template
	// translations are looked up by exact map key, and the console renders the
	// field from a fixed language list. Storing the full tag would silently send
	// the default template instead of the English one and show an empty
	// language on the contact.
	if language := webSupportedLanguage(seed.Language); language != "" {
		contact.Language = &domain.NullableString{String: language}
	}
	// Nothing downstream validates a contact's timezone — Contact.Validate checks
	// only the email — so this is the only gate, not a redundant one. It bounds
	// the length for free, since every accepted name is far inside the column.
	//
	// The allowlist, not time.LoadLocation: that one accepts "Local", which is
	// not a zone at all but the process's own, and would land on the contact as
	// a name nothing else in the product recognises. It also reads zoneinfo off
	// disk, where this is a slice scan.
	if domain.IsValidTimezone(seed.Timezone) {
		contact.Timezone = &domain.NullableString{String: seed.Timezone}
	}
	now := s.nowFn().UTC()
	contact.CreatedAt = now
	contact.UpdatedAt = now

	if err := contact.Validate(); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).
			Warn("Web analytics identify: refusing to create an invalid contact")
		return false
	}

	// The boolean is deliberately discarded: false means a concurrent beat won the
	// race, and the address is held either way — which is all the caller asked.
	if _, err := s.contactRepo.CreateContactIfAbsent(ctx, workspaceID, contact); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).
			Error("Web analytics identify: failed to create the contact")
		return false
	}
	s.workspaceCache.Set("wa:contact:"+workspaceID+":"+email, true, webBridgeContactCacheTTL)
	return true
}

// InvalidateWorkspaceCache drops the cached settings of one workspace (used
// after settings updates so changes apply within a beat, not a minute).
func (s *WebAnalyticsService) InvalidateWorkspaceCache(workspaceID string) {
	s.workspaceCache.Delete("wa:" + workspaceID)
}
