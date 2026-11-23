package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"sort"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/dkim"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// dkimVerificationLockID is the session advisory lock one instance holds while it checks
// the DKIM records, so that replicas do not check the same keys at once. The other
// keys of this family: 4_242_000_001 rotates SECRET_KEY, 4_242_000_003 runs migrations.
const dkimVerificationLockID int64 = 4_242_000_002

const (
	// dkimRecheckSoon is the pace of a key that does not sign, and of a published key
	// whose record failed a check.
	dkimRecheckSoon = time.Hour
	// dkimRecheckDaily is the pace of a published key whose last check passed, and of a
	// key that has not been seen published for dkimSlowAfter.
	dkimRecheckDaily = 24 * time.Hour
	// dkimSlowAfter is how long a key may go unpublished before it is checked daily.
	dkimSlowAfter = 14 * 24 * time.Hour
	// dkimTickSlack lets a key due at the hour be checked at an hourly tick that comes a
	// little early.
	dkimTickSlack = 5 * time.Minute
	// dkimLongDNSError is how long a published key may go without a successful check
	// before its DNS errors are logged as a warning.
	dkimLongDNSError = 72 * time.Hour
)

// DKIMVerificationWorker checks the published records of the DKIM keys of every
// workspace: it publishes a key whose record appeared and stops one whose record went
// away (domain.EvaluateDKIMCheck). It reads the keys without their private half, and
// applies each result only when the key has not changed during the lookup.
type DKIMVerificationWorker struct {
	repo     domain.WorkspaceRepository
	db       *sql.DB
	resolver dkim.TXTResolver
	store    *DKIMKeyStore
	logger   logger.Logger

	interval     time.Duration
	initialDelay time.Duration
	parallel     int
	now          func() time.Time
}

// NewDKIMVerificationWorker checks every hour, starting three minutes after boot, with
// at most five lookups at once. db is the system database, for the advisory lock.
func NewDKIMVerificationWorker(repo domain.WorkspaceRepository, db *sql.DB, resolver dkim.TXTResolver, store *DKIMKeyStore, log logger.Logger) *DKIMVerificationWorker {
	if resolver == nil {
		resolver = dkim.DefaultResolver
	}
	return &DKIMVerificationWorker{
		repo:         repo,
		db:           db,
		resolver:     resolver,
		store:        store,
		logger:       log,
		interval:     time.Hour,
		initialDelay: 3 * time.Minute,
		parallel:     5,
		now:          func() time.Time { return time.Now().UTC() },
	}
}

// Start blocks until ctx is done. Call it in a goroutine.
func (w *DKIMVerificationWorker) Start(ctx context.Context) {
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

// RunOnce checks the keys that are due, when no other instance is doing it.
func (w *DKIMVerificationWorker) RunOnce(ctx context.Context) {
	conn, locked, err := w.lock(ctx)
	if err != nil {
		w.logger.WithField("error", err.Error()).Error("DKIM check: failed to take the lock")
		return
	}
	if !locked {
		return
	}
	defer w.unlock(conn)

	w.check(ctx)
}

// lock takes the session lock on a connection of its own, which is kept until unlock.
func (w *DKIMVerificationWorker) lock(ctx context.Context) (*sql.Conn, bool, error) {
	conn, err := w.db.Conn(ctx)
	if err != nil {
		return nil, false, err
	}
	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, dkimVerificationLockID).Scan(&locked); err != nil {
		_ = conn.Close()
		return nil, false, err
	}
	if !locked {
		_ = conn.Close()
		return nil, false, nil
	}
	return conn, true, nil
}

// unlock releases the lock on a context of its own, so a shutdown still releases it. A
// connection whose unlock failed may still hold the lock: it is closed instead of being
// given back to the pool.
func (w *DKIMVerificationWorker) unlock(conn *sql.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, dkimVerificationLockID); err != nil {
		w.logger.WithField("error", err.Error()).Warn("DKIM check: failed to release the lock; dropping the connection")
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	}
	_ = conn.Close()
}

type dueDKIMKey struct {
	workspaceID string
	key         domain.DKIMKey
}

// dueKeys lists the keys whose next check has come, those checked least recently (or
// never) first, so that no key waits behind others pass after pass.
func dueKeys(workspaces []domain.WorkspaceDKIMMetadata, now time.Time) []dueDKIMKey {
	var due []dueDKIMKey
	for _, ws := range workspaces {
		for _, k := range ws.Keys {
			if k.LastCheckedAt == nil || now.Sub(*k.LastCheckedAt) >= recheckInterval(k, now)-dkimTickSlack {
				due = append(due, dueDKIMKey{workspaceID: ws.WorkspaceID, key: k})
			}
		}
	}
	sort.SliceStable(due, func(i, j int) bool {
		a, b := due[i].key.LastCheckedAt, due[j].key.LastCheckedAt
		switch {
		case a == nil:
			return b != nil
		case b == nil:
			return false
		default:
			return a.Before(*b)
		}
	})
	return due
}

func recheckInterval(k domain.DKIMKey, now time.Time) time.Duration {
	if k.Status == domain.DKIMKeyStatusPublished {
		if k.ConsecutiveFailures > 0 {
			return dkimRecheckSoon
		}
		return dkimRecheckDaily
	}
	lastSeen := k.CreatedAt
	if k.VerifiedAt != nil && k.VerifiedAt.After(lastSeen) {
		lastSeen = *k.VerifiedAt
	}
	if now.Sub(lastSeen) > dkimSlowAfter {
		return dkimRecheckDaily
	}
	return dkimRecheckSoon
}

func (w *DKIMVerificationWorker) check(ctx context.Context) {
	workspaces, err := w.repo.ListWorkspacesDKIMMetadata(ctx)
	if err != nil {
		w.logger.WithField("error", err.Error()).Error("DKIM check: failed to list the keys")
		return
	}
	due := dueKeys(workspaces, w.now())
	if len(due) == 0 {
		return
	}

	sem := make(chan struct{}, w.parallel)
	var wg sync.WaitGroup
	for _, d := range due {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(d dueDKIMKey) {
			defer wg.Done()
			defer func() { <-sem }()
			w.checkKey(ctx, d)
		}(d)
	}
	wg.Wait()
}

func (w *DKIMVerificationWorker) checkKey(ctx context.Context, d dueDKIMKey) {
	result, detail := dkim.VerifyDNS(ctx, w.resolver, d.key.Selector, d.key.Domain, d.key.PublicKey)
	if ctx.Err() != nil {
		return
	}
	now := w.now()

	var transition *domain.DKIMTransition
	var stored domain.DKIMKey
	_, err := w.repo.UpdateDKIMKeys(ctx, d.workspaceID, func(keys []domain.DKIMKey, _ domain.Integrations) (domain.DKIMKeysMutation, error) {
		i := domain.FindDKIMKey(keys, d.key.Domain)
		if i < 0 || keys[i].ID != d.key.ID || keys[i].Revision != d.key.Revision {
			// Deleted, regenerated or checked meanwhile: this lookup is out of date.
			return domain.DKIMKeysMutation{}, nil
		}
		stored, transition = domain.EvaluateDKIMCheck(keys[i], domain.DKIMCheckResult(result), detail, now)
		updated := append([]domain.DKIMKey{}, keys...)
		updated[i] = stored
		return domain.DKIMKeysMutation{Keys: updated, Changed: true, TouchUpdatedAt: transition != nil}, nil
	})
	if err != nil {
		w.logger.WithFields(map[string]interface{}{
			"workspace_id": d.workspaceID,
			"domain":       d.key.Domain,
			"error":        err.Error(),
		}).Error("DKIM check: failed to record the result")
		return
	}

	if transition != nil {
		transition.WorkspaceID = d.workspaceID
		logDKIMTransition(w.logger, transition)
		if w.store != nil {
			w.store.Invalidate(d.workspaceID)
		}
	}
	if result == dkim.ResultDNSError && d.key.Status == domain.DKIMKeyStatusPublished {
		lastOK := d.key.CreatedAt
		if d.key.VerifiedAt != nil {
			lastOK = *d.key.VerifiedAt
		}
		if now.Sub(lastOK) > dkimLongDNSError {
			w.logger.WithFields(map[string]interface{}{
				"workspace_id": d.workspaceID,
				"domain":       d.key.Domain,
				"selector":     d.key.Selector,
				"since":        lastOK.Format(time.RFC3339),
				"error":        detail,
			}).Warn("DKIM check: the record of a signing key has not been readable for over 72 hours")
		}
	}
}
