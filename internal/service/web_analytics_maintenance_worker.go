package service

import (
	"context"
	"time"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// WebAnalyticsMaintenanceWorker keeps the partitioned web analytics tables
// healthy across every workspace, once a day:
//
//   - ensures the current and next monthly partitions exist (ingestion also
//     self-heals on a missing partition, but pre-creating avoids the retry),
//   - moves the aggressive autovacuum profile forward: freshly-relevant months
//     keep it, last month's partitions are reset to defaults once cold,
//   - ANALYZEs newly created partitions so first queries plan sanely.
//
// Old data is never dropped. Expiring history is left to the operator, who
// can DROP the monthly partitions they no longer want.
type WebAnalyticsMaintenanceWorker struct {
	workspaceRepo    domain.WorkspaceRepository
	webAnalyticsRepo domain.WebAnalyticsRepository
	logger           logger.Logger

	interval     time.Duration
	initialDelay time.Duration
	lastRun      time.Time
	nowFn        func() time.Time
}

// NewWebAnalyticsMaintenanceWorker creates the worker.
func NewWebAnalyticsMaintenanceWorker(
	workspaceRepo domain.WorkspaceRepository,
	webAnalyticsRepo domain.WebAnalyticsRepository,
	log logger.Logger,
) *WebAnalyticsMaintenanceWorker {
	return &WebAnalyticsMaintenanceWorker{
		workspaceRepo:    workspaceRepo,
		webAnalyticsRepo: webAnalyticsRepo,
		logger:           log,
		interval:         24 * time.Hour,
		initialDelay:     2 * time.Minute,
		nowFn:            time.Now,
	}
}

// Start runs the worker until ctx is cancelled. One pass shortly after boot
// (so upgrades converge fast), then daily.
func (w *WebAnalyticsMaintenanceWorker) Start(ctx context.Context) {
	w.logger.Info("Web analytics maintenance worker started")

	select {
	case <-ctx.Done():
		return
	case <-time.After(w.initialDelay):
	}
	w.RunOnce(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Web analytics maintenance worker stopping...")
			return
		case <-ticker.C:
			w.RunOnce(ctx)
		}
	}
}

// RunOnce performs one maintenance pass over all workspaces. Exported for
// tests and manual triggering. Per-workspace failures are logged and skipped
// so one broken workspace database cannot stall the fleet.
func (w *WebAnalyticsMaintenanceWorker) RunOnce(ctx context.Context) {
	now := w.nowFn().UTC()
	w.lastRun = now

	workspaces, err := w.workspaceRepo.List(ctx)
	if err != nil {
		w.logger.WithField("error", err.Error()).Error("Web analytics maintenance: failed to list workspaces")
		return
	}

	currentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	nextMonth := currentMonth.AddDate(0, 1, 0)
	previousMonth := currentMonth.AddDate(0, -1, 0)

	for _, workspace := range workspaces {
		if ctx.Err() != nil {
			return
		}

		// Metered before the web analytics check, not after: the timeline meter
		// counts events that exist whether or not the tracking snippet is
		// installed, and a workspace whose analytics settings are cleared must
		// not silently stop being metered and freeze its usage history.
		w.recomputeUsage(ctx, workspace.ID, currentMonth, previousMonth)

		if workspace.Settings.WebAnalytics == nil {
			continue
		}

		if err := w.maintainWorkspace(ctx, workspace.ID, currentMonth, nextMonth, previousMonth); err != nil {
			w.logger.WithField("workspace_id", workspace.ID).WithField("error", err.Error()).
				Error("Web analytics maintenance failed for workspace")
		}
	}
}

// recomputeUsage refreshes the usage snapshot for the open month and the one
// before it.
//
// Two months, because a month keeps moving for a while after it ends: Validate
// rejects a session id whose embedded timestamp is older than
// WebSessionIDMaxAge (48h), so nothing can land in a month once it is 48h past,
// and the early passes of a new month are what settle the previous one's final
// value. Only the open month is written as live; a closed month's stored counts
// can never be lowered.
//
// Failures are logged per month and never returned: usage metering must not be
// able to stop partition maintenance for a workspace, which is what actually
// keeps ingestion working.
func (w *WebAnalyticsMaintenanceWorker) recomputeUsage(ctx context.Context, workspaceID string, currentMonth, previousMonth time.Time) {
	for _, period := range []struct {
		month time.Time
		live  bool
	}{
		{previousMonth, false},
		{currentMonth, true},
	} {
		if err := w.webAnalyticsRepo.RecomputeUsage(ctx, workspaceID, period.month, period.live); err != nil {
			w.logger.WithField("workspace_id", workspaceID).
				WithField("period_month", period.month.Format("2006-01")).
				WithField("error", err.Error()).
				Error("Web analytics maintenance: usage recompute failed")
		}
	}
}

func (w *WebAnalyticsMaintenanceWorker) maintainWorkspace(ctx context.Context, workspaceID string, currentMonth, nextMonth, previousMonth time.Time) error {
	// Snapshot existing partitions to know which ones this pass creates.
	existing := map[string]bool{}
	for _, table := range schema.WebAnalyticsTableNames {
		partitions, err := w.webAnalyticsRepo.ListPartitions(ctx, workspaceID, table)
		if err != nil {
			return err
		}
		for _, name := range partitions {
			existing[name] = true
		}
	}

	// Current + next partitions (EnsureMonthlyPartitions also applies the
	// aggressive autovacuum profile to current/future months).
	if err := w.webAnalyticsRepo.EnsureMonthlyPartitions(ctx, workspaceID, []time.Time{currentMonth, nextMonth}); err != nil {
		return err
	}

	// Reset the autovacuum profile on last month's partitions: they are cold
	// now, and the padding-friendly settings would just waste cycles.
	for _, table := range schema.WebAnalyticsTableNames {
		partition := schema.WebAnalyticsPartitionName(table, previousMonth)
		if !existing[partition] {
			continue
		}
		if err := w.webAnalyticsRepo.SetPartitionAutovacuum(ctx, workspaceID, partition, false); err != nil {
			w.logger.WithField("workspace_id", workspaceID).WithField("partition", partition).
				WithField("error", err.Error()).Error("Failed to reset partition autovacuum profile")
		}
	}

	// ANALYZE partitions created by this pass so their first queries plan on
	// real (empty-but-known) statistics.
	var created []string
	for _, table := range schema.WebAnalyticsTableNames {
		for _, month := range []time.Time{currentMonth, nextMonth} {
			partition := schema.WebAnalyticsPartitionName(table, month)
			if !existing[partition] {
				created = append(created, partition)
			}
		}
	}
	if len(created) > 0 {
		if err := w.webAnalyticsRepo.AnalyzePartitions(ctx, workspaceID, created); err != nil {
			w.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).
				Error("Failed to analyze new web analytics partitions")
		}
	}

	return nil
}
