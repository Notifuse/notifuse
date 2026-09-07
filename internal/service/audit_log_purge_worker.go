package service

import (
	"context"
	"time"

	"github.com/Notifuse/notifuse/pkg/logger"
)

// auditPurger is the one method the worker needs from the audit service.
type auditPurger interface {
	PurgeExpired(ctx context.Context) (int64, error)
}

// AuditLogPurgeWorker runs the retention purge once a day, the way the web
// analytics maintenance worker runs its own housekeeping. It holds no licence
// provider and must never be given one: retention is a promise about storage,
// not about the licence, and the "Never" list in internal/domain/license.go
// forbids any code that deletes data from asking.
type AuditLogPurgeWorker struct {
	purger       auditPurger
	logger       logger.Logger
	interval     time.Duration
	initialDelay time.Duration
	nowFn        func() time.Time
}

// NewAuditLogPurgeWorker builds the worker with a 24-hour interval and a
// two-minute initial delay, so a boot is not slowed by a purge.
func NewAuditLogPurgeWorker(purger auditPurger, log logger.Logger) *AuditLogPurgeWorker {
	return &AuditLogPurgeWorker{
		purger:       purger,
		logger:       log,
		interval:     24 * time.Hour,
		initialDelay: 2 * time.Minute,
		nowFn:        time.Now,
	}
}

// Start blocks until ctx is done. Call it in a goroutine.
func (w *AuditLogPurgeWorker) Start(ctx context.Context) {
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
			return
		case <-ticker.C:
			w.RunOnce(ctx)
		}
	}
}

// RunOnce runs one purge and logs the outcome. Exported so tests and the
// integration suite can trigger it without waiting a day.
func (w *AuditLogPurgeWorker) RunOnce(ctx context.Context) {
	started := w.nowFn()
	deleted, err := w.purger.PurgeExpired(ctx)
	if err != nil {
		if w.logger != nil {
			w.logger.WithField("error", err.Error()).Error("Audit log purge failed")
		}
		return
	}
	if deleted > 0 && w.logger != nil {
		w.logger.WithFields(map[string]interface{}{
			"deleted":     deleted,
			"duration_ms": w.nowFn().Sub(started).Milliseconds(),
		}).Info("Audit log purge completed")
	}
}
