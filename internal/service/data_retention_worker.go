package service

import (
	"context"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// optOutHashPurger is the one method the worker needs from the erasure repository.
type optOutHashPurger interface {
	PurgeOptOutHashes(ctx context.Context, workspaceID string, recordedBefore time.Time) (int64, error)
}

// workspaceLister is the one method the worker needs from the workspace repository.
type workspaceLister interface {
	List(ctx context.Context) ([]*domain.Workspace, error)
}

// retentionRunner runs every workspace's retention policy once (DataRetentionService).
type retentionRunner interface {
	RunAll(ctx context.Context)
	// LastRunAt is when the last run over every workspace ended; false when none has.
	LastRunAt(ctx context.Context) (time.Time, bool)
}

// dataRetentionCatchUp is how long after the last run a starting server runs at once (5
// minutes after boot) rather than waiting for 05:00 UTC: a day and a little margin.
const dataRetentionCatchUp = 25 * time.Hour

// DataRetentionWorker runs the daily data retention housekeeping: it deletes the opt-out
// hashes of deleted contacts once they are three years old (domain.OptOutHashExpiry), and
// those of lists deleted since, then runs every workspace's retention policy. It holds no
// licence provider and must never be given one: it only deletes, and licensing never blocks
// a deletion.
type DataRetentionWorker struct {
	workspaces   workspaceLister
	hashes       optOutHashPurger
	runner       retentionRunner
	logger       logger.Logger
	interval     time.Duration
	initialDelay time.Duration
	nowFn        func() time.Time
}

// dataRetentionRunHour is the hour, UTC, of the daily run: the time the settings page states.
const dataRetentionRunHour = 5

// untilNextRun is how long until the next 05:00 UTC.
func untilNextRun(now time.Time) time.Duration {
	now = now.UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), dataRetentionRunHour, 0, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now)
}

// NewDataRetentionWorker builds the worker: it runs every day at 05:00 UTC.
func NewDataRetentionWorker(workspaces workspaceLister, hashes optOutHashPurger, log logger.Logger) *DataRetentionWorker {
	return &DataRetentionWorker{
		workspaces:   workspaces,
		hashes:       hashes,
		logger:       log,
		interval:     24 * time.Hour,
		initialDelay: untilNextRun(time.Now()),
		nowFn:        time.Now,
	}
}

// firstDelay is how long the worker waits before its first run: until 05:00 UTC, or 5 minutes
// when the last run is more than a day old (the server was down or restarting at 05:00).
func (w *DataRetentionWorker) firstDelay(ctx context.Context) time.Duration {
	if w.runner == nil {
		return w.initialDelay
	}
	last, ok := w.runner.LastRunAt(ctx)
	if !ok || w.nowFn().Sub(last) > dataRetentionCatchUp {
		if catchUp := 5 * time.Minute; catchUp < w.initialDelay {
			return catchUp
		}
	}
	return w.initialDelay
}

// Start blocks until ctx is done. Call it in a goroutine.
func (w *DataRetentionWorker) Start(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(w.firstDelay(ctx)):
	}
	w.RunOnce(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.RunOnce(ctx)
		}
	}
}

// SetRunner supplies the retention policy runner.
func (w *DataRetentionWorker) SetRunner(runner retentionRunner) {
	w.runner = runner
}

// RunOnce purges every workspace once, going on past one that fails. Exported so tests
// and the integration suite can run it without waiting a day.
func (w *DataRetentionWorker) RunOnce(ctx context.Context) {
	w.purgeOptOutHashes(ctx)
	if w.runner != nil && ctx.Err() == nil {
		w.runner.RunAll(ctx)
	}
}

func (w *DataRetentionWorker) purgeOptOutHashes(ctx context.Context) {
	workspaces, err := w.workspaces.List(ctx)
	if err != nil {
		w.logger.WithField("error", err.Error()).Error("Data retention: failed to list workspaces")
		return
	}
	before := domain.OptOutHashExpiry(w.nowFn())
	for _, workspace := range workspaces {
		if ctx.Err() != nil {
			return
		}
		deleted, err := w.hashes.PurgeOptOutHashes(ctx, workspace.ID, before)
		if err != nil {
			w.logger.WithFields(map[string]interface{}{
				"workspace_id": workspace.ID,
				"error":        err.Error(),
			}).Error("Data retention: failed to purge opt-out hashes")
			continue
		}
		if deleted > 0 {
			w.logger.WithFields(map[string]interface{}{
				"workspace_id": workspace.ID,
				"deleted":      deleted,
			}).Info("Data retention: purged opt-out hashes")
		}
	}
}
