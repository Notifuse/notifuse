package service

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// WebAnalyticsBuffer holds the latest beat per session and debounces writes.
//
// Because every beat carries the full cumulative session state, skipping an
// intermediate beat loses recency only, never data. A session is therefore
// written when it is new (keeps Live fresh), when it gained goals, when its
// last write is older than SessionFlushInterval, or when it has been
// idle-dirty for IdleFlushAfter (guaranteeing the final beat always lands).
// This turns the ~20-60 heartbeat upserts of a session into ~5-10.
//
// Everything is in-process and bounded; PostgreSQL remains the only store. A
// crash loses at most the last few seconds of recency on in-flight sessions —
// the same failure profile as Staminads' 2s ClickHouse buffer.
type WebAnalyticsBufferConfig struct {
	FlushTick               time.Duration // scheduler cadence
	SessionFlushInterval    time.Duration // max staleness of a written session
	IdleFlushAfter          time.Duration // flush dirty sessions that stopped beating
	EvictAfter              time.Duration // forget clean sessions after this idle time
	MaxSessionsPerWorkspace int           // above this, the workspace force-flushes everything dirty
}

// DefaultWebAnalyticsBufferConfig returns the production tuning.
func DefaultWebAnalyticsBufferConfig() WebAnalyticsBufferConfig {
	return WebAnalyticsBufferConfig{
		FlushTick:               2 * time.Second,
		SessionFlushInterval:    60 * time.Second,
		IdleFlushAfter:          70 * time.Second,
		EvictAfter:              35 * time.Minute, // session timeout (30m) + slack
		MaxSessionsPerWorkspace: 20000,
	}
}

const webBufferMaxFlushAttempts = 2

type webBufferedSession struct {
	session *domain.WebSession
	pages   []*domain.WebPage
	goals   []*domain.WebGoal

	dirty            bool
	failedAttempts   int
	lastArrival      time.Time
	lastFlushedAt    time.Time
	flushedGoalCount int
	everFlushed      bool
}

type webWorkspaceBuffer struct {
	sessions map[string]*webBufferedSession
	flushing bool
}

// webBufferKey identifies one writer: a tab within a session.
func webBufferKey(sessionID string, tabID int64) string {
	return sessionID + "|" + strconv.FormatInt(tabID, 10)
}

// WebAnalyticsBuffer is safe for concurrent use.
type WebAnalyticsBuffer struct {
	repo   domain.WebAnalyticsRepository
	logger logger.Logger
	cfg    WebAnalyticsBufferConfig
	nowFn  func() time.Time

	mu         sync.Mutex
	workspaces map[string]*webWorkspaceBuffer
}

// NewWebAnalyticsBuffer creates the buffer. Zero-valued config fields fall
// back to the defaults, so tests can shrink only the knobs they need.
func NewWebAnalyticsBuffer(repo domain.WebAnalyticsRepository, log logger.Logger, cfg WebAnalyticsBufferConfig) *WebAnalyticsBuffer {
	defaults := DefaultWebAnalyticsBufferConfig()
	if cfg.FlushTick <= 0 {
		cfg.FlushTick = defaults.FlushTick
	}
	if cfg.SessionFlushInterval <= 0 {
		cfg.SessionFlushInterval = defaults.SessionFlushInterval
	}
	if cfg.IdleFlushAfter <= 0 {
		cfg.IdleFlushAfter = defaults.IdleFlushAfter
	}
	if cfg.EvictAfter <= 0 {
		cfg.EvictAfter = defaults.EvictAfter
	}
	if cfg.MaxSessionsPerWorkspace <= 0 {
		cfg.MaxSessionsPerWorkspace = defaults.MaxSessionsPerWorkspace
	}
	return &WebAnalyticsBuffer{
		repo:       repo,
		logger:     log,
		cfg:        cfg,
		nowFn:      time.Now,
		workspaces: map[string]*webWorkspaceBuffer{},
	}
}

// Add stores the beat's rows, collapsing onto any buffered older beat of the
// same session (highest beat_seq wins; ties keep the latest arrival).
func (b *WebAnalyticsBuffer) Add(workspaceID string, tabID int64, session *domain.WebSession, pages []*domain.WebPage, goals []*domain.WebGoal) {
	if session == nil {
		return
	}
	now := b.nowFn()

	b.mu.Lock()
	defer b.mu.Unlock()

	ws := b.workspaces[workspaceID]
	if ws == nil {
		ws = &webWorkspaceBuffer{sessions: map[string]*webBufferedSession{}}
		b.workspaces[workspaceID] = ws
	}

	// Buffer per WRITER, not per session. Tabs share a session id but keep
	// independent seq counters, so an entry keyed on the session alone would
	// hold whichever tab beat highest and then discard every beat from every
	// other tab — before the per-tab primary keys ever got a chance to apply.
	// The wholesale replacement below is likewise only correct per writer: a
	// beat carries that tab's complete cumulative state, and nobody else's.
	key := webBufferKey(session.ID, tabID)
	entry := ws.sessions[key]
	if entry == nil {
		entry = &webBufferedSession{}
		ws.sessions[key] = entry
	} else if entry.session != nil && session.BeatSeq < entry.session.BeatSeq {
		// Out-of-order arrival (offline queue replay) from this same tab: the
		// buffered state is newer, keep it. The repository guard would reject
		// the write anyway.
		return
	}

	entry.session = session
	entry.pages = pages
	entry.goals = goals
	entry.dirty = true
	entry.failedAttempts = 0
	entry.lastArrival = now
}

// Start runs the flush scheduler until ctx is cancelled, then performs a
// final flush on a detached context so shutdown drains the buffer.
func (b *WebAnalyticsBuffer) Start(ctx context.Context) {
	ticker := time.NewTicker(b.cfg.FlushTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			b.FlushAll(flushCtx)
			cancel()
			return
		case <-ticker.C:
			b.flushDue(ctx)
		}
	}
}

// Stop drains everything synchronously; safe to call after Start returned.
func (b *WebAnalyticsBuffer) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	b.FlushAll(ctx)
}

// FlushAll writes every dirty session regardless of debouncing. Also the test
// hook that makes integration tests deterministic.
func (b *WebAnalyticsBuffer) FlushAll(ctx context.Context) {
	b.flush(ctx, true)
}

func (b *WebAnalyticsBuffer) flushDue(ctx context.Context) {
	b.flush(ctx, false)
}

func (b *WebAnalyticsBuffer) flush(ctx context.Context, force bool) {
	now := b.nowFn()

	// flushed pairs a session id with the exact row pointer that was handed to
	// the repository. They must travel together: FlushBatch sorts the slices
	// it receives in place (for deadlock-free lock ordering), so parallel
	// id/row slices would silently desync and the failure bookkeeping below
	// would retry and drop the wrong sessions.
	type flushed struct {
		id      string
		session *domain.WebSession
	}
	type workspaceFlush struct {
		workspaceID string
		entries     []flushed
		sessions    []*domain.WebSession
		pages       []*domain.WebPage
		goals       []*domain.WebGoal
	}
	var flushes []workspaceFlush

	b.mu.Lock()
	for workspaceID, ws := range b.workspaces {
		if ws.flushing {
			continue
		}
		forceWorkspace := force || len(ws.sessions) > b.cfg.MaxSessionsPerWorkspace

		var flushRun workspaceFlush
		for id, entry := range ws.sessions {
			if !entry.dirty {
				// Evict long-idle clean sessions so memory stays bounded.
				if now.Sub(entry.lastArrival) > b.cfg.EvictAfter {
					delete(ws.sessions, id)
				}
				continue
			}
			if !forceWorkspace && !b.isDue(entry, now) {
				continue
			}
			flushRun.entries = append(flushRun.entries, flushed{id: id, session: entry.session})
			flushRun.sessions = append(flushRun.sessions, entry.session)
			flushRun.pages = append(flushRun.pages, entry.pages...)
			flushRun.goals = append(flushRun.goals, entry.goals...)

			// Optimistically mark clean; a failure re-marks dirty below.
			entry.dirty = false
			entry.everFlushed = true
			entry.lastFlushedAt = now
			entry.flushedGoalCount = len(entry.goals)
		}
		if len(flushRun.sessions) == 0 {
			continue
		}
		flushRun.workspaceID = workspaceID
		ws.flushing = true
		flushes = append(flushes, flushRun)
	}
	b.mu.Unlock()

	for _, run := range flushes {
		err := b.repo.FlushBatch(ctx, run.workspaceID, run.sessions, run.pages, run.goals)

		b.mu.Lock()
		ws := b.workspaces[run.workspaceID]
		ws.flushing = false
		if err != nil {
			b.logger.WithField("workspace_id", run.workspaceID).
				WithField("sessions", len(run.sessions)).
				WithField("error", err.Error()).
				Error("Web analytics flush failed")
			for _, sent := range run.entries {
				id := sent.id
				entry := ws.sessions[id]
				if entry == nil {
					continue
				}
				// The write never landed: whatever entry is buffered now must
				// not be debounced as if it had been persisted.
				entry.everFlushed = false
				if entry.session != sent.session {
					// A newer beat replaced the entry mid-flush; it is dirty
					// with a fresh retry budget and flushes on the next tick.
					continue
				}
				entry.failedAttempts++
				if entry.failedAttempts >= webBufferMaxFlushAttempts {
					b.logger.WithField("workspace_id", run.workspaceID).
						WithField("session_id", id).
						Error("Dropping web analytics session after repeated flush failures")
					delete(ws.sessions, id)
					continue
				}
				entry.dirty = true
			}
		}
		b.mu.Unlock()
	}
}

func (b *WebAnalyticsBuffer) isDue(entry *webBufferedSession, now time.Time) bool {
	if !entry.everFlushed {
		return true
	}
	if len(entry.goals) > entry.flushedGoalCount {
		return true
	}
	if now.Sub(entry.lastFlushedAt) >= b.cfg.SessionFlushInterval {
		return true
	}
	if now.Sub(entry.lastArrival) >= b.cfg.IdleFlushAfter {
		return true
	}
	return false
}

// PendingSessions returns the number of buffered sessions for a workspace
// (test/observability helper).
func (b *WebAnalyticsBuffer) PendingSessions(workspaceID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	ws := b.workspaces[workspaceID]
	if ws == nil {
		return 0
	}
	return len(ws.sessions)
}
