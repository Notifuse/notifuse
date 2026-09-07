package service

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// AuditLogService is both the recorder (the write port the middleware and the
// purge worker call) and the read side the console and the API use.
//
// Recording is what the licence gates, and it is the only thing it gates: an
// unlicensed deployment writes no rows and is refused nothing. Listing,
// exporting and the retention settings never consult the licence — they work
// on every deployment, on whatever was recorded while a key covered
// audit_logs — and a change of licence state writes one marker row so a gap
// in the log is never silent.
//
// Record never fails the caller. The audit log must not be the reason a
// member's action is refused, so a row that cannot be written is logged as an
// error and dropped, and a panic anywhere in the recorder is recovered.
type AuditLogService struct {
	repo          domain.AuditLogRepository
	authService   domain.AuthService
	userRepo      domain.UserRepository
	workspaceRepo domain.WorkspaceRepository
	settingRepo   domain.SettingRepository
	entitlements  domain.EntitlementProvider
	isRootEmail   func(string) bool
	logger        logger.Logger
	now           func() time.Time

	mu           sync.Mutex
	lastLicensed *bool
}

// AuditLogServiceConfig wires the service. Entitlements may be nil in tests
// that predate licensing; a nil provider records everything, because "not
// wired" is not "not licensed".
type AuditLogServiceConfig struct {
	Repo          domain.AuditLogRepository
	AuthService   domain.AuthService
	UserRepo      domain.UserRepository
	WorkspaceRepo domain.WorkspaceRepository
	SettingRepo   domain.SettingRepository
	Entitlements  domain.EntitlementProvider
	IsRootEmail   func(string) bool
	Logger        logger.Logger
	Now           func() time.Time
}

// auditWriteTimeout bounds one insert. The write runs on a context detached
// from the request's, so a client that disconnects the moment its action
// succeeds still gets its row; the timeout is what keeps a stuck database
// from holding the request goroutine forever.
const auditWriteTimeout = 5 * time.Second

// auditExportFlushEvery is how many rows an export writes between flushes.
const auditExportFlushEvery = 500

// auditExportColumns is the CSV header, in this order.
var auditExportColumns = []string{
	"id", "occurred_at", "workspace_id", "action", "category", "outcome", "status_code",
	"actor_type", "actor_id", "actor_email", "actor_name", "actor_role", "auth_method",
	"target_type", "target_id", "target_name", "ip_address", "user_agent", "request_id",
	"changes", "metadata",
}

var errAuditExportCapReached = errors.New("audit export cap reached")

var _ domain.AuditLogRecorder = (*AuditLogService)(nil)
var _ domain.AuditLogService = (*AuditLogService)(nil)

// NewAuditLogService builds the service.
func NewAuditLogService(cfg AuditLogServiceConfig) *AuditLogService {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &AuditLogService{
		repo:          cfg.Repo,
		authService:   cfg.AuthService,
		userRepo:      cfg.UserRepo,
		workspaceRepo: cfg.WorkspaceRepo,
		settingRepo:   cfg.SettingRepo,
		entitlements:  cfg.Entitlements,
		isRootEmail:   cfg.IsRootEmail,
		logger:        cfg.Logger,
		now:           now,
	}
}

// isLicensedFor mirrors the gate helper in the other licensed services: a
// service constructed without a provider is unwired, not unlicensed.
func (s *AuditLogService) isLicensedFor(feature domain.Feature) bool {
	if s.entitlements == nil {
		return true
	}
	return s.entitlements.Entitlements().Has(feature)
}

// Recording reports whether rows are being written right now.
func (s *AuditLogService) Recording() bool {
	return s.isLicensedFor(domain.FeatureAuditLogs)
}

// Init seeds the licence state from the settings row that remembers whether
// rows were being written the last time this server looked, so a licence that
// lapsed while the server was down is noticed at boot and gets its
// licence.recordingStopped marker then, not never. With no row (first boot on
// v41) the current state is recorded without a marker; with an unreadable row
// the state stays unknown and the first Record seeds it, again without a
// marker, because a transient database blip at boot must not invent one.
// Called once from app wiring.
func (s *AuditLogService) Init(ctx context.Context) {
	licensed := s.isLicensedFor(domain.FeatureAuditLogs)
	if s.settingRepo == nil {
		s.mu.Lock()
		s.lastLicensed = &licensed
		s.mu.Unlock()
		return
	}

	setting, err := s.settingRepo.Get(ctx, domain.AuditLogsRecordingSettingKey)
	var notFound *domain.ErrSettingNotFound
	switch {
	case err == nil && setting != nil:
		stored := strings.TrimSpace(setting.Value) == "true"
		s.mu.Lock()
		s.lastLicensed = &stored
		s.mu.Unlock()
		// Compare what the last run knew with what is true now; the marker,
		// when there is one, carries this boot's timestamp.
		s.noteLicenceTransition(ctx, licensed)
	case errors.As(err, &notFound):
		s.mu.Lock()
		s.lastLicensed = &licensed
		s.mu.Unlock()
		s.rememberRecordingState(ctx, licensed)
	default:
		// Unreadable: leave it unknown rather than guess.
	}
}

// rememberRecordingState persists the state the next boot compares against.
// Fail-open, like every other write here.
func (s *AuditLogService) rememberRecordingState(ctx context.Context, licensed bool) {
	if s.settingRepo == nil {
		return
	}
	value := "false"
	if licensed {
		value = "true"
	}
	if err := s.settingRepo.Set(ctx, domain.AuditLogsRecordingSettingKey, value); err != nil && s.logger != nil {
		s.logger.WithField("error", err.Error()).Error("Failed to remember the audit recording state")
	}
}

// Record writes one row when the licence covers audit logs, and nothing
// otherwise — except the marker row that says the licence state changed,
// which is written in both directions.
func (s *AuditLogService) Record(ctx context.Context, log *domain.AuditLog) {
	defer func() {
		if recovered := recover(); recovered != nil && s.logger != nil {
			s.logger.WithField("panic", fmt.Sprint(recovered)).Error("Audit recorder panicked; the row was dropped")
		}
	}()
	if log == nil || log.Action == "" {
		return
	}

	licensed := s.isLicensedFor(domain.FeatureAuditLogs)
	s.noteLicenceTransition(ctx, licensed)
	if !licensed {
		return
	}
	s.write(ctx, log)
}

// noteLicenceTransition writes licence.recordingStopped or
// licence.recordingResumed when the state differs from the last one seen.
// Per process: several replicas may each write one marker, which is the
// honest picture — each of them did stop or resume.
func (s *AuditLogService) noteLicenceTransition(ctx context.Context, licensed bool) {
	s.mu.Lock()
	previous := s.lastLicensed
	current := licensed
	s.lastLicensed = &current
	s.mu.Unlock()

	if previous == nil {
		// First observation in this process with no stored state: remember it,
		// mark nothing.
		s.rememberRecordingState(ctx, licensed)
		return
	}
	if *previous == licensed {
		return
	}
	action := domain.AuditActionRecordingStopped
	if licensed {
		action = domain.AuditActionRecordingResumed
	}
	s.write(ctx, &domain.AuditLog{
		Action:     action,
		Category:   domain.AuditCategoryLicence,
		Outcome:    domain.AuditOutcomeSuccess,
		ActorType:  domain.AuditActorSystem,
		TargetType: domain.AuditTargetAuditLog,
	})
	s.rememberRecordingState(ctx, licensed)
}

// write fills what the caller left out, redacts, and inserts on a detached
// context. Errors are logged, never returned.
func (s *AuditLogService) write(ctx context.Context, log *domain.AuditLog) {
	if log.ID == "" {
		log.ID = strings.ReplaceAll(uuid.New().String(), "-", "")
	}
	if log.OccurredAt.IsZero() {
		log.OccurredAt = s.now().UTC()
	}
	if log.Outcome == "" {
		log.Outcome = domain.AuditOutcomeSuccess
	}
	if log.ActorType == "" {
		log.ActorType = domain.AuditActorSystem
	}
	if log.Category == "" {
		log.Category = auditCategoryOf(log.Action)
	}
	s.completeActor(ctx, log)
	log.Changes = domain.RedactAuditChanges(log.Changes)
	log.Metadata = domain.RedactAuditMetadata(log.Metadata)

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), auditWriteTimeout)
	defer cancel()
	if err := s.repo.Insert(writeCtx, log); err != nil && s.logger != nil {
		s.logger.WithFields(map[string]interface{}{
			"action":       log.Action,
			"workspace_id": log.WorkspaceID,
			"request_id":   log.RequestID,
			"error":        err.Error(),
		}).Error("Failed to record audit log")
	}
}

// completeActor fills the email and name for rows that only carry the JWT's
// user id — deployment-level routes never resolve a workspace membership, so
// nothing upstream named the actor — and marks platform administrators.
func (s *AuditLogService) completeActor(ctx context.Context, log *domain.AuditLog) {
	if log.ActorID != "" && log.ActorEmail == "" && s.userRepo != nil {
		if user, err := s.userRepo.GetUserByID(ctx, log.ActorID); err == nil && user != nil {
			log.ActorEmail = user.Email
			log.ActorName = user.Name
			if user.Type == domain.UserTypeAPIKey {
				log.ActorType = domain.AuditActorAPIKey
				if log.AuthMethod == "" {
					log.AuthMethod = domain.AuditAuthAPIKey
				}
			}
		}
	}
	if log.ActorEmail != "" && log.ActorRole != domain.AuditRoleRoot && s.isRootEmail != nil && s.isRootEmail(log.ActorEmail) {
		log.ActorRole = domain.AuditRoleRoot
	}
}

// auditCategoryOf derives the category for a row whose caller did not name
// one, from the catalogue.
func auditCategoryOf(action string) string {
	if spec, ok := domain.AuditedActions[action]; ok {
		return spec.Category
	}
	if spec, ok := domain.AuditSystemActions[action]; ok {
		return spec.Category
	}
	for _, read := range domain.AuditLoggedReads {
		if read.Action == action {
			return read.Category
		}
	}
	return domain.AuditCategorySystem
}

// authorize is the read-side check. Workspace scope: a member of that
// workspace holding audit_logs read, or its owner. Deployment scope: a
// platform administrator. It never consults the licence.
func (s *AuditLogService) authorize(ctx context.Context, scope, workspaceID string) (context.Context, error) {
	if scope == domain.AuditScopeDeployment {
		user, err := s.authService.AuthenticateUserFromContext(ctx)
		if err != nil {
			return ctx, fmt.Errorf("failed to authenticate user: %w", err)
		}
		if s.isRootEmail == nil || !s.isRootEmail(user.Email) {
			return ctx, &domain.ErrUnauthorized{Message: "root user access required"}
		}
		return ctx, nil
	}

	ctx, _, membership, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return ctx, fmt.Errorf("failed to authenticate user: %w", err)
	}
	if !membership.HasPermission(domain.PermissionResourceAuditLogs, domain.PermissionTypeRead) {
		return ctx, domain.NewPermissionError(
			domain.PermissionResourceAuditLogs,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to audit_logs required",
		)
	}
	return ctx, nil
}

// List returns one page.
func (s *AuditLogService) List(ctx context.Context, filter domain.AuditLogFilter) (*domain.ListAuditLogsResponse, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	ctx, err := s.authorize(ctx, filter.Scope, filter.WorkspaceID)
	if err != nil {
		return nil, err
	}
	logs, next, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs: %w", err)
	}
	return &domain.ListAuditLogsResponse{Logs: logs, NextCursor: next, HasMore: next != ""}, nil
}

// Get returns one row. In workspace scope a row of another workspace is a
// miss, not a leak.
func (s *AuditLogService) Get(ctx context.Context, scope, workspaceID, id string) (*domain.AuditLog, error) {
	filter := domain.AuditLogFilter{Scope: scope, WorkspaceID: workspaceID}
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	ctx, err := s.authorize(ctx, filter.Scope, filter.WorkspaceID)
	if err != nil {
		return nil, err
	}
	log, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if filter.Scope == domain.AuditScopeWorkspace && log.WorkspaceID != filter.WorkspaceID {
		return nil, fmt.Errorf("%w: %s", domain.ErrAuditLogNotFound, id)
	}
	return log, nil
}

// Export streams the selection as CSV or NDJSON, newest first, flushing
// every auditExportFlushEvery rows and stopping at
// domain.AuditLogExportMaxRows. The export is itself recorded, through the
// request's audit record: the handler's route is audited, this only adds
// what the row should say.
func (s *AuditLogService) Export(ctx context.Context, req domain.ExportAuditLogsRequest, w io.Writer, flush func()) (int64, bool, error) {
	if err := req.Validate(); err != nil {
		return 0, false, err
	}
	// Taken before authorize: the context it hands back is derived from this
	// one in production, but a test double may hand back a fresh one.
	record := domain.AuditFromContext(ctx)
	ctx, err := s.authorize(ctx, req.Scope, req.WorkspaceID)
	if err != nil {
		return 0, false, err
	}
	if flush == nil {
		flush = func() {}
	}

	// What the export row should say, set before the first byte: a stream
	// that breaks half-way still leaves a row that names the format and the
	// filter, with the handler marking it failed.
	target := req.WorkspaceID
	if req.Scope == domain.AuditScopeDeployment && target == "" {
		target = domain.AuditScopeDeployment
	}
	record.SetTarget(domain.AuditTargetAuditLog, target, "")
	record.AddMetadata("format", req.Format)
	record.AddMetadata("scope", req.Scope)
	if len(req.Actions) > 0 {
		record.AddMetadata("actions", req.Actions)
	}
	if req.From != nil {
		record.AddMetadata("from", req.From.UTC().Format(time.RFC3339))
	}
	if req.To != nil {
		record.AddMetadata("to", req.To.UTC().Format(time.RFC3339))
	}

	var (
		rows      int64
		truncated bool
		csvWriter *csv.Writer
		jsonEnc   *json.Encoder
	)
	if req.Format == domain.AuditExportFormatCSV {
		csvWriter = csv.NewWriter(w)
		if err := csvWriter.Write(auditExportColumns); err != nil {
			return 0, false, err
		}
	} else {
		jsonEnc = json.NewEncoder(w)
	}

	err = s.repo.Stream(ctx, req.AuditLogFilter, func(log *domain.AuditLog) error {
		if rows >= domain.AuditLogExportMaxRows {
			truncated = true
			return errAuditExportCapReached
		}
		if csvWriter != nil {
			if err := csvWriter.Write(auditCSVRow(log)); err != nil {
				return err
			}
		} else if err := jsonEnc.Encode(log); err != nil {
			return err
		}
		rows++
		if rows%auditExportFlushEvery == 0 {
			if csvWriter != nil {
				csvWriter.Flush()
			}
			flush()
		}
		return nil
	})
	if err != nil && !errors.Is(err, errAuditExportCapReached) {
		return rows, truncated, err
	}
	if csvWriter != nil {
		if truncated {
			_ = csvWriter.Write([]string{"", "", "", "export.truncated", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", fmt.Sprintf(`{"max_rows":%d}`, domain.AuditLogExportMaxRows)})
		}
		csvWriter.Flush()
		if err := csvWriter.Error(); err != nil {
			return rows, truncated, err
		}
	} else if truncated {
		_ = jsonEnc.Encode(map[string]any{"action": "export.truncated", "max_rows": domain.AuditLogExportMaxRows})
	}
	flush()

	record.AddMetadata("rows", rows)
	record.AddMetadata("truncated", truncated)
	return rows, truncated, nil
}

func auditCSVRow(log *domain.AuditLog) []string {
	status := ""
	if log.StatusCode != 0 {
		status = strconv.Itoa(log.StatusCode)
	}
	changes := ""
	if len(log.Changes) > 0 {
		if encoded, err := json.Marshal(log.Changes); err == nil {
			changes = string(encoded)
		}
	}
	metadata := ""
	if len(log.Metadata) > 0 {
		if encoded, err := json.Marshal(log.Metadata); err == nil {
			metadata = string(encoded)
		}
	}
	return []string{
		log.ID, log.OccurredAt.UTC().Format(time.RFC3339Nano), log.WorkspaceID, log.Action, log.Category, log.Outcome, status,
		log.ActorType, log.ActorID, log.ActorEmail, log.ActorName, log.ActorRole, log.AuthMethod,
		log.TargetType, log.TargetID, log.TargetName, log.IPAddress, log.UserAgent, log.RequestID,
		changes, metadata,
	}
}

// Actions is the catalogue, for the console's filter and the docs.
func (s *AuditLogService) Actions() []domain.AuditActionDescriptor {
	return domain.AuditActionCatalogue()
}

// DeploymentRetentionDays reads the deployment default from the system
// settings; absent or unreadable means domain.DefaultAuditLogRetentionDays.
func (s *AuditLogService) DeploymentRetentionDays(ctx context.Context) int {
	if s.settingRepo == nil {
		return domain.DefaultAuditLogRetentionDays
	}
	setting, err := s.settingRepo.Get(ctx, domain.AuditLogsRetentionSettingKey)
	if err != nil || setting == nil {
		return domain.DefaultAuditLogRetentionDays
	}
	days, err := strconv.Atoi(strings.TrimSpace(setting.Value))
	if err != nil || domain.ValidateAuditRetentionDays(days) != nil {
		return domain.DefaultAuditLogRetentionDays
	}
	return days
}

// PurgeExpired deletes rows past their retention: each workspace's own
// setting (or the deployment default), then the deployment-level rows and the
// rows of workspaces that no longer exist under the deployment default. A
// retention of 0 keeps forever. Nothing here consults the licence — deleting
// data must never depend on it — and every purge that removed something is
// itself recorded, in every licence state: rows disappearing from an
// unlicensed deployment's log is exactly the kind of gap the marker rows
// exist to explain, so the purge row bypasses the recording gate the way
// the licence markers do.
func (s *AuditLogService) PurgeExpired(ctx context.Context) (int64, error) {
	defaultDays := s.DeploymentRetentionDays(ctx)
	workspaces, err := s.workspaceRepo.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list workspaces for audit purge: %w", err)
	}
	now := s.now().UTC()

	var total int64
	keep := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		keep = append(keep, workspace.ID)
		days := workspace.Settings.AuditLogs.EffectiveRetentionDays(defaultDays)
		if days <= 0 {
			continue
		}
		before := now.AddDate(0, 0, -days)
		workspaceID := workspace.ID
		deleted, err := s.repo.Purge(ctx, &workspaceID, before)
		if err != nil {
			if s.logger != nil {
				s.logger.WithFields(map[string]interface{}{"workspace_id": workspaceID, "error": err.Error()}).Error("Failed to purge audit logs")
			}
			continue
		}
		if deleted > 0 {
			total += deleted
			s.write(ctx, s.purgedEvent(workspaceID, deleted, days, "workspace"))
		}
	}

	if defaultDays > 0 {
		before := now.AddDate(0, 0, -defaultDays)
		deleted, err := s.repo.Purge(ctx, nil, before)
		if err != nil {
			if s.logger != nil {
				s.logger.WithField("error", err.Error()).Error("Failed to purge deployment audit logs")
			}
		} else if deleted > 0 {
			total += deleted
			s.write(ctx, s.purgedEvent("", deleted, defaultDays, "deployment"))
		}

		// Only with a successful workspace listing in hand: an empty keep list
		// would delete every workspace row.
		deleted, err = s.repo.PurgeOrphans(ctx, keep, before)
		if err != nil {
			if s.logger != nil {
				s.logger.WithField("error", err.Error()).Error("Failed to purge orphaned audit logs")
			}
		} else if deleted > 0 {
			total += deleted
			s.write(ctx, s.purgedEvent("", deleted, defaultDays, "orphans"))
		}
	}
	return total, nil
}

func (s *AuditLogService) purgedEvent(workspaceID string, deleted int64, days int, scope string) *domain.AuditLog {
	return &domain.AuditLog{
		WorkspaceID: workspaceID,
		Action:      domain.AuditActionPurged,
		Category:    domain.AuditCategoryAudit,
		Outcome:     domain.AuditOutcomeSuccess,
		ActorType:   domain.AuditActorSystem,
		TargetType:  domain.AuditTargetAuditLog,
		Metadata: map[string]any{
			"deleted":        deleted,
			"retention_days": days,
			"scope":          scope,
		},
	}
}
