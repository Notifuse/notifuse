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
//
// Two designs were considered and rejected, and both get re-proposed easily:
//
//   - An external buffer (Redis) in front of Postgres. Beyond adding a second
//     datastore to a self-hosted product, it loses the property this one has:
//     unflushed state stays queryable by arbitrary GROUP BY, because it is in
//     the same process as the reader. PostgreSQL is the only store, by design.
//   - A raw-events table with rollups (the shape ClickHouse pushes you toward).
//     Unnecessary here: a beat carries the whole cumulative session, so the
//     final rows are computable from any single payload with no read-back.
//     That is also why there are no materialized views — the enrichment step
//     does what they did, synchronously.
//
// If exact aggregation ever outgrows the tables, the sanctioned next step is
// deterministic hash sampling above a row threshold, not approximation
// extensions: tdigest and TABLESAMPLE were declined for the operational burden
// they put on self-hosted installs.
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

	dirty          bool
	failedAttempts int
	// emittedGoals keys the goals already bridged to the contact timeline by
	// their identity, never by an index into entry.goals: a later beat can carry
	// FEWER goals than an earlier one (two tabs, an offline replay), so an
	// integer cursor would slice out of range or silently skip.
	emittedGoals map[string]struct{}
	// everIdentified records whether any beat of this writer has carried a
	// contact. Sticky, because contact_email is sticky in the database: a
	// visitor who logs out mid-visit keeps beating anonymously against rows that
	// are still identified, and their visit must keep converging.
	everIdentified bool
	// projectionAttempts counts consecutive failed projections for this writer,
	// bounding the retry below the same way failedAttempts bounds the flush.
	projectionAttempts int

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
	bridge *WebAnalyticsContactBridge
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

// SetContactBridge attaches the timeline bridge. Separate from the constructor
// so the bridge can be built after the buffer without a dependency cycle.
func (b *WebAnalyticsBuffer) SetContactBridge(bridge *WebAnalyticsContactBridge) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bridge = bridge
}

// Add stores the beat's rows, collapsing onto any buffered older beat of the
// same session (highest beat_seq wins; ties keep the latest arrival).
//
// No opt-in travels with the beat any more. Writing an identified visitor's
// goals and navigation to their contact timeline used to be gated by two
// workspace settings; calling identify() is now the opt-in, and Track only ever
// resolves an identity when a valid credential is present. An anonymous beat
// carries no contact_email and reaches no timeline by construction.
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
	if session.ContactEmail != nil && *session.ContactEmail != "" {
		entry.everIdentified = true
	}
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
// Stop drains the buffer. The final flush is the whole point of this method, not
// a courtesy: writes are debounced for up to IdleFlushAfter, so a shutdown that
// only stopped the ticker would silently discard every session dirtied in the
// last minute. It must also run while the database connections are still open.
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

// webFlushedEntry pairs a session id with the exact row pointer that was handed
// to the repository. They must travel together: FlushBatch sorts the slices it
// receives in place (for deadlock-free lock ordering), so parallel id/row slices
// would silently desync and the failure bookkeeping would retry and drop the
// wrong sessions.
type webFlushedEntry struct {
	id             string
	session        *domain.WebSession
	goals          []*domain.WebGoal
	everIdentified bool
}

func (b *WebAnalyticsBuffer) flush(ctx context.Context, force bool) {
	now := b.nowFn()

	type workspaceFlush struct {
		workspaceID string
		entries     []webFlushedEntry
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
			flushRun.entries = append(flushRun.entries, webFlushedEntry{id: id, session: entry.session, goals: entry.goals, everIdentified: entry.everIdentified})
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

		// Select the goals this writer has not bridged yet, while holding the
		// lock; emit them after releasing it. Emitting before the flush had
		// committed would advertise a conversion the write might still lose, and
		// emitting under b.mu — or inside the flush transaction — would hold
		// contact_segment_queue row locks across a batch spanning many contacts.
		var candidates []webBridgeCandidate
		if err == nil && b.bridge != nil {
			for _, sent := range run.entries {
				entry := ws.sessions[sent.id]
				if entry == nil {
					continue
				}
				for _, goal := range sent.goals {
					key := webBridgeGoalKey(goal)
					if _, seen := entry.emittedGoals[key]; seen {
						continue
					}
					candidates = append(candidates, webBridgeCandidate{entryID: sent.id, key: key, goal: goal})
				}
			}
		}

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

		if len(candidates) > 0 {
			goals := make([]*domain.WebGoal, 0, len(candidates))
			for _, c := range candidates {
				goals = append(goals, c.goal)
			}

			// Mark ONLY what was actually written. A goal that was anonymous at
			// this flush, whose contact does not exist yet, or whose insert
			// failed must stay unmarked so a later flush retries it — the SDK
			// re-sends its whole cumulative action list, so the goal is still
			// there. Marking on selection instead would silently drop every
			// conversion fired before the visitor identified, which is exactly
			// the case retroactive identification exists to serve.
			written := b.bridge.EmitGoals(ctx, run.workspaceID, goals)
			if len(written) > 0 {
				b.mu.Lock()
				for _, c := range candidates {
					if !written[c.goal] {
						continue
					}
					entry := ws.sessions[c.entryID]
					if entry == nil {
						continue // evicted mid-emit; the DB conflict guard covers a replay
					}
					if entry.emittedGoals == nil {
						entry.emittedGoals = make(map[string]struct{}, len(candidates))
					}
					entry.emittedGoals[c.key] = struct{}{}
				}
				b.mu.Unlock()
			}
		}

		if err == nil {
			b.projectNavigation(ctx, run.workspaceID, run.entries)
		}
	}
}

// projectNavigation refreshes the contact timeline from the rows this flush just
// committed, for the writers whose workspace opted in and whose visitor is
// identified.
//
// There is no cursor and nothing to mark as done, unlike the goals bridge. A
// goal is an event — emitting it twice would mean two conversions — whereas a
// pageview row is a projection of state that is expected to be rewritten as the
// visit continues, and the derived primary key makes a repeat harmless. That is
// also why this does not need to know whether the session has ended: the last
// flush of a visit writes its final state, whenever that turns out to be.
//
// A failure is logged and dropped, and that is a real gap on the LAST flush of a
// visit: flush() marks the entry clean before the write, only a new beat
// re-dirties it, and the final flush is the one carrying the settled state. A
// visitor who never beats again after a failed projection gets no timeline rows
// at all. Earlier flushes self-repair — the next one re-projects everything from
// the database — and the analytics rows themselves are already persisted either
// way, so the loss is the timeline copy of one visit rather than the visit.
func (b *WebAnalyticsBuffer) projectNavigation(ctx context.Context, workspaceID string, entries []webFlushedEntry) {
	var sessions []*domain.WebSession
	for _, sent := range entries {
		if sent.session == nil {
			continue
		}
		// A writer that has never carried a contact cannot have a timeline to
		// write to, and the statements would select nothing. Skipping it here is
		// what stops a reporting-only workspace paying for the projection at all:
		// a force-flush at MaxSessionsPerWorkspace would otherwise open a
		// transaction per chunk, all of them matching zero rows, on the single
		// flush goroutine every other workspace is waiting on.
		//
		// The test is "EVER identified", not "identified by this beat" — see the
		// field's own comment.
		if !sent.everIdentified {
			continue
		}
		// Identity is NOT tested here. contact_email is sticky in the database —
		// a beat that does not know the contact never clears it — so a visitor
		// who logs out mid-visit keeps sending anonymous beats against rows that
		// are still identified. Skipping those would freeze their timeline on
		// whatever the last identified beat happened to say: a stale duration, an
		// exit page that is not the exit, and none of the pages that followed.
		// The projection filters on the persisted contact_email instead, which is
		// the value that is actually true.
		sessions = append(sessions, sent.session)
	}
	if len(sessions) == 0 {
		return
	}
	err := b.repo.ProjectContactNavigation(ctx, workspaceID, sessions)
	if err == nil {
		b.clearProjectionAttempts(workspaceID, entries)
		return
	}
	if err != nil {
		b.logger.WithField("workspace_id", workspaceID).
			WithField("sessions", len(sessions)).
			WithField("error", err.Error()).
			Error("Failed to record web navigation on the contact timeline")
		b.retryProjection(workspaceID, entries)
	}
}

// clearProjectionAttempts resets the retry budget after a projection lands.
func (b *WebAnalyticsBuffer) clearProjectionAttempts(workspaceID string, entries []webFlushedEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ws := b.workspaces[workspaceID]
	if ws == nil {
		return
	}
	for _, sent := range entries {
		if entry := ws.sessions[sent.id]; entry != nil {
			entry.projectionAttempts = 0
		}
	}
}

// retryProjection re-dirties the writers whose projection failed, so the flush
// scheduler comes back to them.
//
// Without this the LAST flush of a visit has no second chance: flush() marks an
// entry clean before the write and only a new beat re-dirties it, so a visitor
// who never beats again after a failed projection gets no timeline rows at all —
// and the last flush is the one carrying the settled state. Earlier flushes
// self-repair, because the next one re-projects everything from the database.
//
// Bounded by webBufferMaxFlushAttempts, mirroring the flush's own retry: a
// projection that fails for a structural reason rather than a transient one
// would otherwise keep its writer dirty forever, and a dirty writer is flushed
// again — re-running FlushBatch's upserts every tick for a session nobody is
// browsing any more.
func (b *WebAnalyticsBuffer) retryProjection(workspaceID string, entries []webFlushedEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ws := b.workspaces[workspaceID]
	if ws == nil {
		return
	}
	for _, sent := range entries {
		entry := ws.sessions[sent.id]
		if entry == nil || !sent.everIdentified {
			continue
		}
		entry.projectionAttempts++
		if entry.projectionAttempts >= webBufferMaxFlushAttempts {
			b.logger.WithField("workspace_id", workspaceID).
				WithField("session_id", sent.id).
				Error("Giving up on recording a visit's navigation after repeated failures")
			continue
		}
		entry.dirty = true
	}
}

// webBridgeCandidate pairs a goal with the writer it belongs to, so the cursor
// can be advanced after the emit rather than before it.
type webBridgeCandidate struct {
	entryID string
	key     string
	goal    *domain.WebGoal
}

// webBridgeGoalKey identifies one goal within a writer's stream. It mirrors the
// web_goals key minus the parts already fixed by the entry: session and tab.
func webBridgeGoalKey(goal *domain.WebGoal) string {
	return goal.GoalName + "|" + strconv.FormatInt(goal.ClientTsMs, 10)
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
