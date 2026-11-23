package service

import (
	"context"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// oauthMaintainer is the one method the worker needs from the OAuth service.
type oauthMaintainer interface {
	RunMaintenance(ctx context.Context) (*domain.OAuthPurgeResult, error)
}

// OAuthWorker runs the periodic job of the OAuth authorization server every
// hour, the way AuditLogPurgeWorker runs the audit purge: it revokes the
// grants with neither a valid refresh token nor a live code, purges expired
// requests, codes and tokens, and purges the client records the cache rules
// let go. It fetches nothing and consults no licence.
type OAuthWorker struct {
	maintainer   oauthMaintainer
	logger       logger.Logger
	interval     time.Duration
	initialDelay time.Duration
	nowFn        func() time.Time
}

// NewOAuthWorker builds the worker with a one-hour interval and a
// two-minute initial delay, so a boot is not slowed by it.
func NewOAuthWorker(maintainer oauthMaintainer, log logger.Logger) *OAuthWorker {
	return &OAuthWorker{
		maintainer:   maintainer,
		logger:       log,
		interval:     time.Hour,
		initialDelay: 2 * time.Minute,
		nowFn:        time.Now,
	}
}

// Start blocks until ctx is done. Call it in a goroutine.
func (w *OAuthWorker) Start(ctx context.Context) {
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

// RunOnce runs the job once and logs the outcome. Exported so tests and the
// integration suite can trigger it without waiting an hour.
func (w *OAuthWorker) RunOnce(ctx context.Context) {
	started := w.nowFn()
	result, err := w.maintainer.RunMaintenance(ctx)
	if err != nil {
		if w.logger != nil {
			w.logger.WithField("error", err.Error()).Error("OAuth maintenance failed")
		}
		return
	}
	if result == nil || w.logger == nil {
		return
	}
	if result.GrantsRevoked+result.RequestsPurged+result.CodesPurged+result.TokensPurged+result.ClientsPurged == 0 {
		return
	}
	w.logger.WithFields(map[string]interface{}{
		"grants_revoked":  result.GrantsRevoked,
		"requests_purged": result.RequestsPurged,
		"codes_purged":    result.CodesPurged,
		"tokens_purged":   result.TokensPurged,
		"clients_purged":  result.ClientsPurged,
		"duration_ms":     w.nowFn().Sub(started).Milliseconds(),
	}).Info("OAuth maintenance completed")
}
