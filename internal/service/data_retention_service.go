package service

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/mailer"
)

// DataRetentionDefaultsSettingKey is the system setting holding the deployment defaults (D6).
const DataRetentionDefaultsSettingKey = "data_retention_defaults"

// Run budgets: a first run on a large workspace spreads over several days rather than
// flooding webhook subscribers with contact.deleted deliveries or holding the database.
const (
	retentionContactBatch   = 200
	retentionContactBudget  = 5000
	retentionDrainBatch     = 200
	retentionDrainBudget    = 5000
	retentionCategoryBatch  = 5000
	retentionCategoryBudget = 500000
	retentionRunsShown      = 20
)

// contactHistoryEraser erases what a contact deleted by the purge left behind (ContactService).
type contactHistoryEraser interface {
	EraseHistoryOfDeletedContact(ctx context.Context, workspaceID, email string) error
}

// auditRecorder writes one audit row (AuditLogService).
type auditRecorder interface {
	Record(ctx context.Context, log *domain.AuditLog)
}

// DataRetentionService configures and runs the data retention policies. The run holds no
// licence provider and must never be given one: a policy saved under a key keeps running
// after it lapses, and licensing never blocks a deletion (D3).
type DataRetentionService struct {
	workspaces domain.WorkspaceRepository
	settings   domain.SettingRepository
	repo       domain.DataRetentionRepository
	eraser     contactHistoryEraser
	auth       domain.AuthService
	mailer     mailer.Mailer
	audit      auditRecorder
	logger     logger.Logger
	now        func() time.Time
}

// NewDataRetentionService creates the service.
func NewDataRetentionService(workspaces domain.WorkspaceRepository, settings domain.SettingRepository, repo domain.DataRetentionRepository,
	eraser contactHistoryEraser, auth domain.AuthService, m mailer.Mailer, audit auditRecorder, log logger.Logger) *DataRetentionService {
	return &DataRetentionService{
		workspaces: workspaces, settings: settings, repo: repo, eraser: eraser, auth: auth,
		mailer: m, audit: audit, logger: log, now: time.Now,
	}
}

// SetClock replaces the service's clock: the integration tests move it past a change's date.
func (s *DataRetentionService) SetClock(now func() time.Time) {
	s.now = now
}

// DataRetentionView is what dataRetention.get returns.
type DataRetentionView struct {
	// Policy is the workspace's own policy in force (nil: the deployment defaults).
	Policy *domain.DataRetentionPolicy `json:"policy,omitempty"`
	// Pending is the change that waits, if any.
	Pending *domain.PendingRetentionChange `json:"pending,omitempty"`
	// InForce is every category's period applied today; PendingDays what applies once
	// every waiting change (the workspace's and the defaults') is due, at PendingAt.
	InForce     map[domain.RetentionCategory]int `json:"in_force"`
	PendingDays map[domain.RetentionCategory]int `json:"pending_days,omitempty"`
	PendingAt   *time.Time                       `json:"pending_at,omitempty"`
	Defaults    map[domain.RetentionCategory]int `json:"defaults"`
	Categories  []domain.RetentionCategorySpec   `json:"categories"`
	// BackfillReady: the engagement of the contacts collected before 45 has been read back.
	// Until then the inactive-contact category deletes nobody.
	BackfillReady bool                   `json:"backfill_ready"`
	Runs          []*domain.RetentionRun `json:"runs"`
}

// DataRetentionPreview is what a policy would remove if it applied now.
type DataRetentionPreview struct {
	Categories map[domain.RetentionCategory]domain.RetentionCategoryResult `json:"categories"`
	// Pending says whether saving this policy waits RetentionPendingDelay, and until when.
	Pending   bool       `json:"pending"`
	PendingAt *time.Time `json:"pending_at,omitempty"`
}

func (s *DataRetentionService) authenticate(ctx context.Context, workspaceID string, owner bool) (context.Context, *domain.User, error) {
	ctx, user, userWorkspace, err := s.auth.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return ctx, nil, fmt.Errorf("failed to authenticate user: %w", err)
	}
	if owner && userWorkspace.Role != "owner" {
		return ctx, nil, &domain.ErrUnauthorized{Message: "Only workspace owners can change the data retention policy"}
	}
	return ctx, user, nil
}

// Defaults reads the deployment defaults; nil when root never set any. Any other failure is
// an error: falling back to the built-in periods could delete what root chose to keep.
func (s *DataRetentionService) Defaults(ctx context.Context) (*domain.RetentionDefaults, error) {
	setting, err := s.settings.Get(ctx, DataRetentionDefaultsSettingKey)
	var notFound *domain.ErrSettingNotFound
	if errors.As(err, &notFound) || (err == nil && (setting == nil || setting.Value == "")) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the data retention defaults: %w", err)
	}
	var defaults domain.RetentionDefaults
	if err := json.Unmarshal([]byte(setting.Value), &defaults); err != nil {
		return nil, fmt.Errorf("unreadable data retention defaults: %w", err)
	}
	return &defaults, nil
}

// EffectiveDays returns a category's period in force for a workspace, for the hourly sweeps
// of webhook deliveries and failed emails. 0 when the defaults cannot be read: the sweep then
// keeps everything until they can, rather than deleting with periods root may have changed.
func (s *DataRetentionService) EffectiveDays(ctx context.Context, workspace *domain.Workspace, category domain.RetentionCategory) int {
	defaults, err := s.Defaults(ctx)
	if err != nil {
		s.logger.WithField("error", err.Error()).Error("Data retention defaults unreadable; the sweep keeps everything this time")
		return 0
	}
	inForce, _, _ := domain.RetentionInForce(workspace.Settings.DataRetention, defaults, s.now())
	return inForce.Days[category]
}

// planSave is what saving a policy does, shared by the save and its preview: a policy that
// deletes no more than the one in force replaces it at once and clears what waited; one that
// deletes more waits RetentionPendingDelay, unless the very same policy already waits, which
// keeps its date. newlyPending says the owners must be told.
func planSave(stored *domain.DataRetentionSettings, policy *domain.DataRetentionPolicy, defaults *domain.RetentionDefaults, now time.Time) (next *domain.DataRetentionSettings, newlyPending bool) {
	next = &domain.DataRetentionSettings{}
	if stored != nil {
		copied := *stored
		next = &copied
	}
	next.Due(now)
	current := domain.Resolve(next.Policy, defaults.ResolvedDefaults(now))
	requested := domain.Resolve(policy, defaults.ResolvedDefaults(now))
	switch {
	case !requested.DeletesMore(current):
		next.Policy, next.Pending = policy, nil
	case next.Pending == nil || !reflect.DeepEqual(next.Pending.Policy, policy):
		next.Pending = &domain.PendingRetentionChange{Policy: policy, EffectiveAt: now.Add(domain.RetentionPendingDelay), RequestedAt: now}
		newlyPending = true
	}
	return next, newlyPending
}

func normalizedPolicy(policy *domain.DataRetentionPolicy) (*domain.DataRetentionPolicy, error) {
	if policy == nil {
		policy = &domain.DataRetentionPolicy{}
	}
	if err := policy.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	policy = policy.Clone()
	policy.Normalize()
	return policy, nil
}

// Get returns the workspace's retention settings, what is in force and the last runs.
func (s *DataRetentionService) Get(ctx context.Context, workspaceID string) (*DataRetentionView, error) {
	ctx, _, err := s.authenticate(ctx, workspaceID, false)
	if err != nil {
		return nil, err
	}
	workspace, err := s.workspaces.GetByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	defaults, err := s.Defaults(ctx)
	if err != nil {
		return nil, err
	}
	inForce, pending, at := domain.RetentionInForce(workspace.Settings.DataRetention, defaults, now)
	view := &DataRetentionView{
		InForce:    inForce.Days,
		PendingAt:  at,
		Defaults:   defaults.ResolvedDefaults(now),
		Categories: domain.RetentionCategorySpecs,
	}
	if pending != nil {
		view.PendingDays = pending.Days
	}
	if settings := workspace.Settings.DataRetention; settings != nil {
		view.Policy = settings.Policy
		view.Pending = settings.Pending
		if view.Pending != nil && !now.Before(view.Pending.EffectiveAt) {
			view.Policy, view.Pending = view.Pending.Policy, nil
		}
	}
	if view.BackfillReady, err = s.repo.BackfillCompleted(ctx, workspaceID); err != nil {
		return nil, err
	}
	if view.Runs, err = s.repo.ListRuns(ctx, workspaceID, retentionRunsShown); err != nil {
		return nil, err
	}
	if view.Runs == nil {
		view.Runs = []*domain.RetentionRun{}
	}
	return view, nil
}

// Preview counts what a policy would remove if it applied now, category by category, and
// says whether saving it would wait. Never gated by the licence.
func (s *DataRetentionService) Preview(ctx context.Context, workspaceID string, policy *domain.DataRetentionPolicy) (*DataRetentionPreview, error) {
	ctx, _, err := s.authenticate(ctx, workspaceID, false)
	if err != nil {
		return nil, err
	}
	policy, err = normalizedPolicy(policy)
	if err != nil {
		return nil, err
	}
	workspace, err := s.workspaces.GetByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	defaults, err := s.Defaults(ctx)
	if err != nil {
		return nil, err
	}
	requested := domain.Resolve(policy, defaults.ResolvedDefaults(now))

	// Whether saving it waits, and until when, exactly as SetPolicy decides.
	preview := &DataRetentionPreview{Categories: map[domain.RetentionCategory]domain.RetentionCategoryResult{}}
	if next, _ := planSave(workspace.Settings.DataRetention, policy, defaults, now); next.Pending != nil {
		at := next.Pending.EffectiveAt
		preview.Pending, preview.PendingAt = true, &at
	}
	for _, spec := range domain.RetentionCategorySpecs {
		days := requested.Days[spec.Category]
		result := domain.RetentionCategoryResult{Days: days}
		if days == 0 {
			result.Skipped = "off"
			preview.Categories[spec.Category] = result
			continue
		}
		before := now.AddDate(0, 0, -days)
		switch spec.Category {
		case domain.RetentionInactiveContacts:
			ready, err := s.repo.BackfillCompleted(ctx, workspaceID)
			if err != nil {
				return nil, err
			}
			if !ready {
				result.Skipped = "backfill_pending"
				break
			}
			result.Affected, result.OptOutsKept, err = s.repo.CountExpiredContacts(ctx, workspaceID, domain.ExpiredContactsQuery{
				EngagedBefore: before, ExemptListIDs: requested.ExemptListIDs, ExemptSegmentIDs: requested.ExemptSegmentIDs,
				EngagementEventNames: requested.EngagementEventNames,
			})
			if err != nil {
				return nil, err
			}
		case domain.RetentionWebAnalytics:
			months, err := s.repo.WebAnalyticsMonthsBefore(ctx, workspaceID, before)
			if err != nil {
				return nil, err
			}
			result.Affected = int64(len(months))
		default:
			if result.Affected, err = s.repo.CountCategory(ctx, workspaceID, spec.Category, before); err != nil {
				return nil, err
			}
		}
		preview.Categories[spec.Category] = result
	}
	return preview, nil
}

// errRetentionConflict: the settings moved between the read and the write.
var errRetentionConflict = errors.New("the data retention settings changed meanwhile; read them again")

// SetPolicy saves a workspace's policy (owners only). The parts that delete less apply at
// once; if it deletes more than the policy in force, it waits RetentionPendingDelay, and the
// owners are told by email (D7). Saving the same waiting policy again keeps its date.
func (s *DataRetentionService) SetPolicy(ctx context.Context, workspaceID string, policy *domain.DataRetentionPolicy) (*domain.DataRetentionSettings, error) {
	ctx, user, err := s.authenticate(ctx, workspaceID, true)
	if err != nil {
		return nil, err
	}
	policy, err = normalizedPolicy(policy)
	if err != nil {
		return nil, err
	}
	workspace, err := s.workspaces.GetByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	defaults, err := s.Defaults(ctx)
	if err != nil {
		return nil, err
	}
	stored := workspace.Settings.DataRetention
	revision := 0
	if stored != nil {
		revision = stored.Revision
	}
	next, newlyPending := planSave(stored, policy, defaults, now)
	if newlyPending && user != nil {
		next.Pending.RequestedBy = user.Email
	}

	if audit := domain.AuditFromContext(ctx); audit != nil {
		audit.SetTarget(domain.AuditTargetWorkspace, workspaceID, workspace.Name)
		audit.SetChanges(domain.AuditDiff(stored, next))
	}
	written, err := s.workspaces.SetDataRetentionSettings(ctx, workspaceID, revision, next)
	if err != nil {
		return nil, err
	}
	if !written {
		return nil, &domain.ErrRetentionConflict{Message: errRetentionConflict.Error()}
	}
	if newlyPending {
		s.notifyOwners(ctx, workspace, next.Pending.EffectiveAt)
	}
	return next, nil
}

// CancelPending drops the change that waits (owners only).
func (s *DataRetentionService) CancelPending(ctx context.Context, workspaceID string) (*domain.DataRetentionSettings, error) {
	ctx, _, err := s.authenticate(ctx, workspaceID, true)
	if err != nil {
		return nil, err
	}
	workspace, err := s.workspaces.GetByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	stored := workspace.Settings.DataRetention
	if stored == nil || stored.Pending == nil {
		return stored, nil
	}
	next := *stored
	if next.Due(s.now()) {
		// Too late: it applied. The owner saves the policy they want instead.
		return nil, domain.NewValidationError("the change has already taken effect")
	}
	next.Pending = nil
	if audit := domain.AuditFromContext(ctx); audit != nil {
		audit.SetTarget(domain.AuditTargetWorkspace, workspaceID, workspace.Name)
		audit.SetChanges(domain.AuditDiff(stored, &next))
	}
	written, err := s.workspaces.SetDataRetentionSettings(ctx, workspaceID, stored.Revision, &next)
	if err != nil {
		return nil, err
	}
	if !written {
		return nil, &domain.ErrRetentionConflict{Message: errRetentionConflict.Error()}
	}
	return &next, nil
}

// SetDefaults saves the deployment defaults (root, checked by the caller). A default that
// deletes more waits RetentionPendingDelay, and the owners of every workspace it reaches
// are told (D6, D7).
func (s *DataRetentionService) SetDefaults(ctx context.Context, categories map[domain.RetentionCategory]int) (*domain.RetentionDefaults, error) {
	for category, days := range categories {
		if err := domain.ValidateRetentionDays(category, days); err != nil {
			return nil, domain.NewValidationError(err.Error())
		}
	}
	now := s.now()
	stored, err := s.Defaults(ctx)
	if err != nil {
		return nil, err
	}
	currentDays := stored.ResolvedDefaults(now)
	// What root asks for, for every category: one it leaves out goes back to its built-in
	// default. Stored whole, so the pending change replaces every category when it applies
	// rather than laying itself over the old values.
	requested := domain.BuiltinRetentionDefaults()
	for k, v := range categories {
		requested[k] = v
	}
	next := &domain.RetentionDefaults{Categories: currentDays}
	deletesMore := domain.Resolve(nil, requested).DeletesMore(domain.Resolve(nil, currentDays))
	if !deletesMore {
		next.Categories = requested
	} else {
		next.Pending = &domain.PendingRetentionDefaults{Categories: requested, EffectiveAt: now.Add(domain.RetentionPendingDelay)}
		if stored != nil && stored.Pending != nil && now.Before(stored.Pending.EffectiveAt) && reflect.DeepEqual(stored.Pending.Categories, requested) {
			next.Pending.EffectiveAt = stored.Pending.EffectiveAt
			deletesMore = false // already announced
		}
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return nil, err
	}
	if err := s.settings.Set(ctx, DataRetentionDefaultsSettingKey, string(raw)); err != nil {
		return nil, err
	}
	if deletesMore {
		s.notifyInheritingWorkspaces(ctx, stored, next, now)
	}
	return next, nil
}

// notifyInheritingWorkspaces emails the owners of the workspaces the new defaults reach.
func (s *DataRetentionService) notifyInheritingWorkspaces(ctx context.Context, before, after *domain.RetentionDefaults, now time.Time) {
	workspaces, err := s.workspaces.List(ctx)
	if err != nil {
		s.logger.WithField("error", err.Error()).Error("Failed to list workspaces for the data retention notice")
		return
	}
	for _, workspace := range workspaces {
		current, _, _ := domain.RetentionInForce(workspace.Settings.DataRetention, before, now)
		_, pending, at := domain.RetentionInForce(workspace.Settings.DataRetention, after, now)
		if pending != nil && at != nil && pending.DeletesMore(current) {
			s.notifyOwners(ctx, workspace, *at)
		}
	}
}

func (s *DataRetentionService) notifyOwners(ctx context.Context, workspace *domain.Workspace, effectiveAt time.Time) {
	if s.mailer == nil {
		return
	}
	users, err := s.workspaces.GetWorkspaceUsersWithEmail(ctx, workspace.ID)
	if err != nil {
		s.logger.WithField("workspace_id", workspace.ID).WithField("error", err.Error()).Error("Failed to list owners for the data retention notice")
		return
	}
	for _, user := range users {
		if user.Role != "owner" || user.Email == "" {
			continue
		}
		if err := s.mailer.SendDataRetentionPending(user.Email, workspace.ID, workspace.Name, effectiveAt, user.Language); err != nil {
			s.logger.WithField("workspace_id", workspace.ID).WithField("error", err.Error()).Error("Failed to send the data retention notice")
		}
	}
}

// ListRuns returns the last runs.
func (s *DataRetentionService) ListRuns(ctx context.Context, workspaceID string, limit int) ([]*domain.RetentionRun, error) {
	ctx, _, err := s.authenticate(ctx, workspaceID, false)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	runs, err := s.repo.ListRuns(ctx, workspaceID, limit)
	if runs == nil && err == nil {
		runs = []*domain.RetentionRun{}
	}
	return runs, err
}

// ExportRuns writes the runs as CSV: one line per run and category. Counts only.
func (s *DataRetentionService) ExportRuns(ctx context.Context, workspaceID string, w io.Writer) error {
	runs, err := s.ListRuns(ctx, workspaceID, 1000)
	if err != nil {
		return err
	}
	out := csv.NewWriter(w)
	if err := out.Write([]string{"run_id", "started_at", "finished_at", "status", "category", "days", "affected", "opt_outs_kept", "remaining", "skipped", "error"}); err != nil {
		return err
	}
	for _, run := range runs {
		finished := ""
		if run.FinishedAt != nil {
			finished = run.FinishedAt.UTC().Format(time.RFC3339)
		}
		categories := make([]string, 0, len(run.Results))
		for c := range run.Results {
			categories = append(categories, string(c))
		}
		sort.Strings(categories)
		for _, c := range categories {
			r := run.Results[domain.RetentionCategory(c)]
			if err := out.Write([]string{run.ID, run.StartedAt.UTC().Format(time.RFC3339), finished, string(run.Status), c,
				strconv.Itoa(r.Days), strconv.FormatInt(r.Affected, 10), strconv.FormatInt(r.OptOutsKept, 10),
				strconv.FormatBool(r.Remaining), r.Skipped, r.Error}); err != nil {
				return err
			}
		}
	}
	out.Flush()
	return out.Error()
}

// RunAll runs every workspace once, and goes on past a workspace that fails.
func (s *DataRetentionService) RunAll(ctx context.Context) {
	workspaces, err := s.workspaces.List(ctx)
	if err != nil {
		s.logger.WithField("error", err.Error()).Error("Failed to list workspaces for data retention")
		return
	}
	defaults, err := s.Defaults(ctx)
	if err != nil {
		// Running with the built-in periods could delete what root chose to keep.
		s.logger.WithField("error", err.Error()).Error("Data retention defaults unreadable; no run today")
		return
	}
	for _, workspace := range workspaces {
		if ctx.Err() != nil {
			return
		}
		if err := s.RunWorkspace(ctx, workspace, defaults); err != nil {
			s.logger.WithField("workspace_id", workspace.ID).WithField("error", err.Error()).Error("Data retention run failed")
		}
	}
}

// RunWorkspace applies the policy in force to one workspace: categories in a fixed order,
// inactive contacts first so their history goes in the same run, each isolated from the
// others. It records a run report and an audit entry with counts only. It returns nil
// without a report when another server holds the workspace's lock.
func (s *DataRetentionService) RunWorkspace(ctx context.Context, workspace *domain.Workspace, defaults *domain.RetentionDefaults) error {
	release, ok, err := s.repo.TryLock(ctx, workspace.ID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	defer release()

	now := s.now()
	s.applyDueChange(ctx, workspace, now)
	inForce, _, _ := domain.RetentionInForce(workspace.Settings.DataRetention, defaults, now)

	run := &domain.RetentionRun{
		ID: uuid.New().String(), StartedAt: now, Status: domain.RetentionRunRunning,
		Policy: inForce.Days, Results: map[domain.RetentionCategory]domain.RetentionCategoryResult{},
	}
	if err := s.repo.InsertRun(ctx, workspace.ID, run); err != nil {
		return err
	}

	failed := false
	for _, spec := range domain.RetentionCategorySpecs {
		days := inForce.Days[spec.Category]
		result := domain.RetentionCategoryResult{Days: days}
		if days > 0 {
			before := now.AddDate(0, 0, -days)
			switch spec.Category {
			case domain.RetentionInactiveContacts:
				result = s.purgeContacts(ctx, workspace.ID, run.ID, inForce, before, result)
			case domain.RetentionWebAnalytics:
				result = s.dropWebAnalytics(ctx, workspace.ID, before, now, result)
			default:
				result = s.purgeCategory(ctx, workspace.ID, spec.Category, before, inForce.EngagementEventNames, result)
			}
		} else {
			result.Skipped = "off"
		}
		// Whatever the contact category did, the history of contacts already deleted is
		// finished: a run that stopped half-way left it on the queue.
		if spec.Category == domain.RetentionInactiveContacts {
			failures, remaining, err := s.drainErasureQueue(ctx, workspace.ID)
			if err != nil {
				result.Error = joinErrors(result.Error, err.Error())
			} else if failures > 0 {
				result.Error = joinErrors(result.Error, fmt.Sprintf("the history of %d deleted contacts could not be erased; retried next run", failures))
			}
			result.Remaining = result.Remaining || remaining
		}
		if result.Error != "" {
			failed = true
		}
		run.Results[spec.Category] = result
	}

	finished := s.now()
	run.FinishedAt = &finished
	run.Status = domain.RetentionRunCompleted
	if failed {
		run.Status = domain.RetentionRunPartial
	}
	if err := s.repo.FinishRun(ctx, workspace.ID, run); err != nil {
		s.logger.WithField("workspace_id", workspace.ID).WithField("error", err.Error()).Error("Failed to record the data retention run")
	}
	s.recordAudit(ctx, workspace, run)
	return nil
}

// applyDueChange writes a waiting change whose date has come as the policy in force, so the
// console shows it as such. A conflict is left to the next run: the run applies it anyway.
func (s *DataRetentionService) applyDueChange(ctx context.Context, workspace *domain.Workspace, now time.Time) {
	stored := workspace.Settings.DataRetention
	if stored == nil || stored.Pending == nil || now.Before(stored.Pending.EffectiveAt) {
		return
	}
	next := *stored
	next.Due(now)
	if _, err := s.workspaces.SetDataRetentionSettings(ctx, workspace.ID, stored.Revision, &next); err != nil {
		s.logger.WithField("workspace_id", workspace.ID).WithField("error", err.Error()).Warn("Failed to apply a due data retention change")
	}
}

func joinErrors(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

func (s *DataRetentionService) purgeContacts(ctx context.Context, workspaceID, runID string, policy domain.EffectiveRetention, before time.Time, result domain.RetentionCategoryResult) domain.RetentionCategoryResult {
	ready, err := s.repo.BackfillCompleted(ctx, workspaceID)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if !ready {
		result.Skipped = "backfill_pending"
		return result
	}
	// An exemption that cannot be trusted (a list or segment gone, a segment being rebuilt)
	// skips the category: purging without it could delete the very contacts it protects.
	missing, err := s.repo.MissingExemptions(ctx, workspaceID, policy.ExemptListIDs, policy.ExemptSegmentIDs)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if len(missing) > 0 {
		result.Skipped = "exemption_unavailable"
		return result
	}
	q := domain.ExpiredContactsQuery{
		EngagedBefore: before, ExemptListIDs: policy.ExemptListIDs, ExemptSegmentIDs: policy.ExemptSegmentIDs,
		EngagementEventNames: policy.EngagementEventNames, Limit: retentionContactBatch,
	}
	for result.Affected < retentionContactBudget {
		if ctx.Err() != nil {
			result.Remaining = true
			return result
		}
		emails, err := s.repo.ListExpiredContacts(ctx, workspaceID, q)
		if err != nil {
			result.Error = err.Error()
			return result
		}
		if len(emails) == 0 {
			return result
		}
		deleted, optOuts, err := s.repo.DeleteExpiredContacts(ctx, workspaceID, runID, q, emails)
		if errors.Is(err, domain.ErrRetentionExemptionUnavailable) {
			result.Skipped = "exemption_unavailable"
			return result
		}
		if err != nil {
			result.Error = err.Error()
			return result
		}
		result.Affected += int64(len(deleted))
		result.OptOutsKept += optOuts
		q.AfterEmail = emails[len(emails)-1]
		if len(emails) < retentionContactBatch {
			return result
		}
	}
	result.Remaining = true
	return result
}

// drainErasureQueue erases the history of the contacts the purge deleted, up to a budget. An
// address that exists again (a sync re-created it) is only dequeued: its rows are a live
// contact's. An address whose erasure fails goes to the back of the queue and the drain goes
// on; it is counted, and the next run tries it again.
func (s *DataRetentionService) drainErasureQueue(ctx context.Context, workspaceID string) (failures int, remaining bool, err error) {
	failed := map[string]struct{}{}
	done := 0
	for done < retentionDrainBudget {
		if ctx.Err() != nil {
			return failures, true, nil
		}
		emails, err := s.repo.ErasureQueue(ctx, workspaceID, retentionDrainBatch)
		if err != nil {
			return failures, false, err
		}
		progressed := false
		for _, email := range emails {
			if _, seen := failed[email]; seen {
				continue
			}
			progressed = true
			done++
			exists, err := s.repo.ContactExists(ctx, workspaceID, email)
			if err != nil {
				return failures, false, err
			}
			if !exists {
				if err := s.eraser.EraseHistoryOfDeletedContact(ctx, workspaceID, email); err != nil {
					s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Warn("Failed to erase the history of a deleted contact")
					failed[email] = struct{}{}
					failures++
					if err := s.repo.Requeue(ctx, workspaceID, email); err != nil {
						return failures, false, err
					}
					continue
				}
			}
			if err := s.repo.Dequeue(ctx, workspaceID, email); err != nil {
				return failures, false, err
			}
		}
		if !progressed {
			return failures, false, nil // empty, or only addresses that failed this run
		}
	}
	return failures, true, nil
}

func (s *DataRetentionService) purgeCategory(ctx context.Context, workspaceID string, category domain.RetentionCategory, before time.Time, names []string, result domain.RetentionCategoryResult) domain.RetentionCategoryResult {
	for result.Affected < retentionCategoryBudget {
		if ctx.Err() != nil {
			result.Remaining = true
			return result
		}
		n, err := s.repo.PurgeCategory(ctx, workspaceID, category, before, retentionCategoryBatch, names)
		if err != nil {
			result.Error = err.Error()
			return result
		}
		result.Affected += n
		if n < retentionCategoryBatch {
			return result
		}
	}
	result.Remaining = true
	return result
}

func (s *DataRetentionService) dropWebAnalytics(ctx context.Context, workspaceID string, before, now time.Time, result domain.RetentionCategoryResult) domain.RetentionCategoryResult {
	months, err := s.repo.WebAnalyticsMonthsBefore(ctx, workspaceID, before)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	for _, month := range months {
		if err := s.repo.DropWebAnalyticsMonth(ctx, workspaceID, month, now); err != nil {
			result.Error = err.Error()
			return result
		}
		result.Affected++
	}
	return result
}

func (s *DataRetentionService) recordAudit(ctx context.Context, workspace *domain.Workspace, run *domain.RetentionRun) {
	if s.audit == nil {
		return
	}
	counts := map[string]any{}
	for category, result := range run.Results {
		if result.Affected > 0 || result.Error != "" || result.Remaining {
			counts[string(category)] = map[string]any{
				"affected": result.Affected, "opt_outs_kept": result.OptOutsKept, "remaining": result.Remaining, "failed": result.Error != "",
			}
		}
	}
	if len(counts) == 0 {
		return // nothing happened: no row
	}
	outcome := domain.AuditOutcomeSuccess
	if run.Status != domain.RetentionRunCompleted {
		outcome = domain.AuditOutcomeFailure
	}
	s.audit.Record(ctx, &domain.AuditLog{
		WorkspaceID: workspace.ID,
		Action:      domain.AuditActionRetentionPurged,
		Category:    domain.AuditCategoryWorkspaces,
		Outcome:     outcome,
		ActorType:   domain.AuditActorSystem,
		TargetType:  domain.AuditTargetWorkspace,
		TargetID:    workspace.ID,
		TargetName:  workspace.Name,
		Metadata:    map[string]any{"run_id": run.ID, "categories": counts},
	})
}
