package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

func newBufferForTest(t *testing.T) (*WebAnalyticsBuffer, *mocks.MockWebAnalyticsRepository, *time.Time) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	repo := mocks.NewMockWebAnalyticsRepository(ctrl)

	buffer := NewWebAnalyticsBuffer(repo, logger.NewLogger(), WebAnalyticsBufferConfig{
		FlushTick:               time.Second,
		SessionFlushInterval:    60 * time.Second,
		IdleFlushAfter:          70 * time.Second,
		EvictAfter:              35 * time.Minute,
		MaxSessionsPerWorkspace: 100,
	})
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	buffer.nowFn = func() time.Time { return now }
	return buffer, repo, &now
}

// allowProjection lets a test ignore the projection call every successful flush
// now makes. Declared per test rather than in newBufferForTest, because gomock
// matches expectations in declaration order and a harness-level AnyTimes would
// swallow the calls the projection tests are actually asserting on.
func allowProjection(repo *mocks.MockWebAnalyticsRepository) {
	repo.EXPECT().ProjectContactNavigation(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()
}

func bufSession(id string, seq int64, goals int) (*domain.WebSession, []*domain.WebPage, []*domain.WebGoal) {
	session := &domain.WebSession{ID: id, BeatSeq: seq, SessionDate: time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)}
	pages := []*domain.WebPage{{SessionID: id, PageNumber: 1, BeatSeq: seq}}
	var goalRows []*domain.WebGoal
	for i := 0; i < goals; i++ {
		goalRows = append(goalRows, &domain.WebGoal{SessionID: id, GoalName: fmt.Sprintf("g%d", i), BeatSeq: seq})
	}
	return session, pages, goalRows
}

func TestWebAnalyticsBufferDebounce(t *testing.T) {
	ctx := context.Background()

	t.Run("first beat flushes on the next tick", func(t *testing.T) {
		buffer, repo, _ := newBufferForTest(t)
		allowProjection(repo)
		s, p, g := bufSession("s1", 1, 0)
		buffer.Add("ws1", 0, s, p, g)

		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Len(1), gomock.Len(1), gomock.Len(0)).Return(nil)
		buffer.flushDue(ctx)
		buffer.flushDue(ctx) // nothing dirty anymore: no second call expected
	})

	t.Run("subsequent beats debounce until the session interval elapses", func(t *testing.T) {
		buffer, repo, now := newBufferForTest(t)
		allowProjection(repo)

		s, p, g := bufSession("s1", 1, 0)
		buffer.Add("ws1", 0, s, p, g)
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		buffer.flushDue(ctx)

		// A heartbeat 10s later: dirty, but not due.
		*now = now.Add(10 * time.Second)
		s2, p2, g2 := bufSession("s1", 2, 0)
		buffer.Add("ws1", 0, s2, p2, g2)
		buffer.flushDue(ctx)

		// 60s after the first flush: due again, latest beat wins.
		*now = now.Add(50 * time.Second)
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sessions []*domain.WebSession, _ []*domain.WebPage, _ []*domain.WebGoal) error {
				require.Len(t, sessions, 1)
				assert.Equal(t, int64(2), sessions[0].BeatSeq)
				return nil
			})
		buffer.flushDue(ctx)
	})

	t.Run("a beat with new goals flushes immediately", func(t *testing.T) {
		buffer, repo, now := newBufferForTest(t)
		allowProjection(repo)

		s, p, g := bufSession("s1", 1, 0)
		buffer.Add("ws1", 0, s, p, g)
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		buffer.flushDue(ctx)

		*now = now.Add(5 * time.Second)
		s2, p2, g2 := bufSession("s1", 2, 1)
		buffer.Add("ws1", 0, s2, p2, g2)
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Len(1)).Return(nil)
		buffer.flushDue(ctx)
	})

	t.Run("idle-dirty sessions flush after the idle window (final beat lands)", func(t *testing.T) {
		buffer, repo, now := newBufferForTest(t)
		allowProjection(repo)

		s, p, g := bufSession("s1", 1, 0)
		buffer.Add("ws1", 0, s, p, g)
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		buffer.flushDue(ctx)

		*now = now.Add(3 * time.Second)
		s2, p2, g2 := bufSession("s1", 2, 0)
		buffer.Add("ws1", 0, s2, p2, g2) // the visitor's last beat
		buffer.flushDue(ctx)             // not due yet

		*now = now.Add(70 * time.Second)
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sessions []*domain.WebSession, _ []*domain.WebPage, _ []*domain.WebGoal) error {
				assert.Equal(t, int64(2), sessions[0].BeatSeq)
				return nil
			})
		buffer.flushDue(ctx)
	})

	t.Run("out-of-order beat with lower seq is ignored", func(t *testing.T) {
		buffer, repo, _ := newBufferForTest(t)
		allowProjection(repo)

		s5, p5, g5 := bufSession("s1", 5, 0)
		buffer.Add("ws1", 0, s5, p5, g5)
		s3, p3, g3 := bufSession("s1", 3, 0)
		buffer.Add("ws1", 0, s3, p3, g3)

		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sessions []*domain.WebSession, _ []*domain.WebPage, _ []*domain.WebGoal) error {
				assert.Equal(t, int64(5), sessions[0].BeatSeq)
				return nil
			})
		buffer.flushDue(ctx)
	})

	t.Run("workspaces are isolated", func(t *testing.T) {
		buffer, repo, _ := newBufferForTest(t)
		allowProjection(repo)
		s1, p1, g1 := bufSession("s1", 1, 0)
		s2, p2, g2 := bufSession("s2", 1, 0)
		buffer.Add("ws1", 0, s1, p1, g1)
		buffer.Add("ws2", 0, s2, p2, g2)

		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Len(1), gomock.Any(), gomock.Any()).Return(nil)
		repo.EXPECT().FlushBatch(gomock.Any(), "ws2", gomock.Len(1), gomock.Any(), gomock.Any()).Return(nil)
		buffer.flushDue(ctx)
	})
}

func TestWebAnalyticsBufferFailureHandling(t *testing.T) {
	ctx := context.Background()

	t.Run("one retry, then the session is dropped", func(t *testing.T) {
		buffer, repo, _ := newBufferForTest(t)
		allowProjection(repo)
		s, p, g := bufSession("s1", 1, 0)
		buffer.Add("ws1", 0, s, p, g)

		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("db down"))
		buffer.flushDue(ctx)
		assert.Equal(t, 1, buffer.PendingSessions("ws1"), "kept for retry")

		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("db still down"))
		buffer.flushDue(ctx)
		assert.Equal(t, 0, buffer.PendingSessions("ws1"), "dropped after the retry budget")

		buffer.flushDue(ctx) // nothing left: no further FlushBatch expected
	})

	t.Run("a new beat gives a failed writer a fresh retry, whatever failed", func(t *testing.T) {
		// A writer whose own rows PostgreSQL refused too: the salvage budget
		// bounds what a flush spends on retrying it.
		for _, tc := range []struct {
			name string
			err  error
		}{
			{"the database was out of reach", errors.New("db down")},
			{"the database refused the writer's rows", errTestRowsRefused},
		} {
			t.Run(tc.name, func(t *testing.T) {
				buffer, repo, _ := newBufferForTest(t)
				allowProjection(repo)
				s, p, g := bufSession("s1", 1, 0)
				buffer.Add("ws1", 0, s, p, g)
				repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).Return(tc.err).Times(2)
				buffer.flushDue(ctx)

				s2, p2, g2 := bufSession("s1", 2, 0)
				buffer.Add("ws1", 0, s2, p2, g2)
				buffer.flushDue(ctx)
				assert.Equal(t, 1, buffer.PendingSessions("ws1"), "the beat started the count again, so that was a first failure")
			})
		}
	})

	t.Run("a newer beat during a failed flush resets the retry state", func(t *testing.T) {
		buffer, repo, _ := newBufferForTest(t)
		allowProjection(repo)
		s, p, g := bufSession("s1", 1, 0)
		buffer.Add("ws1", 0, s, p, g)

		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, _ []*domain.WebSession, _ []*domain.WebPage, _ []*domain.WebGoal) error {
				// Beat 2 arrives while the flush of beat 1 is failing.
				s2, p2, g2 := bufSession("s1", 2, 0)
				buffer.Add("ws1", 0, s2, p2, g2)
				return errors.New("db down")
			})
		buffer.flushDue(ctx)

		// The replacement entry is dirty and flushes fine.
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sessions []*domain.WebSession, _ []*domain.WebPage, _ []*domain.WebGoal) error {
				assert.Equal(t, int64(2), sessions[0].BeatSeq)
				return nil
			})
		buffer.flushDue(ctx)
		assert.Equal(t, 1, buffer.PendingSessions("ws1"))
	})
}

func TestWebAnalyticsBufferLifecycle(t *testing.T) {
	t.Run("FlushAll ignores debouncing", func(t *testing.T) {
		buffer, repo, now := newBufferForTest(t)
		allowProjection(repo)
		s, p, g := bufSession("s1", 1, 0)
		buffer.Add("ws1", 0, s, p, g)
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)
		buffer.flushDue(context.Background())

		*now = now.Add(time.Second)
		s2, p2, g2 := bufSession("s1", 2, 0)
		buffer.Add("ws1", 0, s2, p2, g2)
		buffer.FlushAll(context.Background())
	})

	t.Run("Start drains on context cancellation", func(t *testing.T) {
		buffer, repo, _ := newBufferForTest(t)
		allowProjection(repo)
		buffer.nowFn = time.Now
		s, p, g := bufSession("s1", 1, 0)
		buffer.Add("ws1", 0, s, p, g)

		flushed := make(chan struct{})
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, _ []*domain.WebSession, _ []*domain.WebPage, _ []*domain.WebGoal) error {
				close(flushed)
				return nil
			})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { buffer.Start(ctx); close(done) }()
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Start did not return after cancellation")
		}
		select {
		case <-flushed:
		case <-time.After(time.Second):
			t.Fatal("final flush never happened")
		}
	})

	t.Run("clean sessions are evicted after the idle horizon", func(t *testing.T) {
		buffer, repo, now := newBufferForTest(t)
		allowProjection(repo)
		s, p, g := bufSession("s1", 1, 0)
		buffer.Add("ws1", 0, s, p, g)
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		buffer.flushDue(context.Background())
		assert.Equal(t, 1, buffer.PendingSessions("ws1"))

		*now = now.Add(36 * time.Minute)
		buffer.flushDue(context.Background())
		assert.Equal(t, 0, buffer.PendingSessions("ws1"))
	})
}

func TestWebAnalyticsBufferConcurrency(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mocks.NewMockWebAnalyticsRepository(ctrl)
	repo.EXPECT().FlushBatch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	repo.EXPECT().ProjectContactNavigation(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	buffer := NewWebAnalyticsBuffer(repo, logger.NewLogger(), WebAnalyticsBufferConfig{})

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				s, p, g := bufSession(fmt.Sprintf("s-%d-%d", worker, i%20), int64(i), i%3)
				buffer.Add(fmt.Sprintf("ws%d", worker%3), 0, s, p, g)
				if i%50 == 0 {
					buffer.FlushAll(context.Background())
				}
			}
		}(worker)
	}
	wg.Wait()
	buffer.FlushAll(context.Background())
}

// TestWebAnalyticsBufferProjectsNavigation covers what the buffer hands to the
// contact-timeline projection.
//
// The projection reads the analytics tables rather than the beat, so what
// matters here is only WHICH sessions it is pointed at and WHEN — the opt-in
// must be honoured per workspace, an anonymous visit has no timeline to write
// to, and a failed flush must not project rows whose source never landed.
func TestWebAnalyticsBufferProjectsNavigation(t *testing.T) {
	ctx := context.Background()
	identified := func(session *domain.WebSession, email string) *domain.WebSession {
		session.ContactEmail = &email
		return session
	}

	t.Run("hands over the sessions that have ever been identified", func(t *testing.T) {
		// A writer that has never carried a contact has no timeline to write to,
		// and skipping it is what keeps a reporting-only workspace from paying
		// for the projection at all.
		buffer, repo, _ := newBufferForTest(t)
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		var projected []string
		repo.EXPECT().ProjectContactNavigation(gomock.Any(), "ws1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sessions []*domain.WebSession) error {
				for _, s := range sessions {
					projected = append(projected, s.ID)
				}
				return nil
			}).Times(1)

		known, pages, goals := bufSession("s-known", 1, 0)
		buffer.Add("ws1", 0, identified(known, "her@example.com"), pages, goals)
		anon, pages, goals := bufSession("s-anon", 1, 0)
		buffer.Add("ws1", 0, anon, pages, goals)

		buffer.FlushAll(ctx)
		assert.Equal(t, []string{"s-known"}, projected)
	})

	t.Run("keeps projecting a visitor who stops identifying mid-visit", func(t *testing.T) {
		// The reason the test above is "EVER identified" rather than "identified
		// by this beat". contact_email is sticky in the database, so a visitor who
		// logs out keeps beating anonymously against rows that are still theirs —
		// and dropping them here would freeze that visit's timeline on whatever
		// the last identified beat happened to say.
		buffer, repo, _ := newBufferForTest(t)
		repo.EXPECT().FlushBatch(gomock.Any(), "ws5", gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		repo.EXPECT().ProjectContactNavigation(gomock.Any(), "ws5", gomock.Any()).Return(nil).Times(2)

		session, pages, goals := bufSession("s-loggedout", 1, 0)
		buffer.Add("ws5", 0, identified(session, "her@example.com"), pages, goals)
		buffer.FlushAll(ctx)

		// Same writer, no identity on the beat any more.
		session, pages, goals = bufSession("s-loggedout", 2, 0)
		buffer.Add("ws5", 0, session, pages, goals)
		buffer.FlushAll(ctx)
	})

	t.Run("does not project when the flush failed", func(t *testing.T) {
		buffer, repo, _ := newBufferForTest(t)
		repo.EXPECT().FlushBatch(gomock.Any(), "ws3", gomock.Any(), gomock.Any(), gomock.Any()).
			Return(errors.New("write failed")).AnyTimes()
		// Projecting here would advertise pageviews the database does not hold —
		// and the projection reads those very rows, so it would write a timeline
		// entry from stale or missing state.
		repo.EXPECT().ProjectContactNavigation(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		session, pages, goals := bufSession("s-failed", 1, 0)
		buffer.Add("ws3", 0, identified(session, "her@example.com"), pages, goals)
		buffer.FlushAll(ctx)
	})

	t.Run("a failed projection is retried, and bounded", func(t *testing.T) {
		// The last flush of a visit has no second chance otherwise: the entry is
		// marked clean before the write and only a new beat re-dirties it, so a
		// visitor who never beats again gets no timeline rows at all — and that
		// flush is the one carrying the settled state.
		buffer, repo, _ := newBufferForTest(t)
		repo.EXPECT().FlushBatch(gomock.Any(), "ws6", gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		// Exactly two: the first attempt and one retry. A third would mean the
		// bound is not holding, and a permanently failing projection would keep
		// its writer dirty — which also re-runs FlushBatch every tick.
		repo.EXPECT().ProjectContactNavigation(gomock.Any(), "ws6", gomock.Any()).
			Return(errors.New("deadlock detected")).Times(2)

		session, pages, goals := bufSession("s-retry", 1, 0)
		buffer.Add("ws6", 0, identified(session, "her@example.com"), pages, goals)

		buffer.FlushAll(ctx) // attempt 1 fails, entry re-dirtied
		buffer.FlushAll(ctx) // attempt 2 fails, budget spent
		buffer.FlushAll(ctx) // nothing left to retry
	})

	t.Run("re-projects the same session on every flush so its final state lands", func(t *testing.T) {
		// There is no cursor and no end-of-session signal: the buffer cannot tell
		// a visit is over (any flush marks the entry clean, and only a new beat
		// marks it dirty again). Convergence is what replaces that, so a second
		// flush of the same session must project it again.
		buffer, repo, _ := newBufferForTest(t)
		repo.EXPECT().FlushBatch(gomock.Any(), "ws4", gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		repo.EXPECT().ProjectContactNavigation(gomock.Any(), "ws4", gomock.Any()).Return(nil).Times(2)

		for seq := int64(1); seq <= 2; seq++ {
			session, pages, goals := bufSession("s-converging", seq, 0)
			buffer.Add("ws4", 0, identified(session, "her@example.com"), pages, goals)
			buffer.FlushAll(ctx)
		}
	})
}

// errTestRowsRefused is FlushBatch's error for a batch holding rows PostgreSQL
// refuses, as the repository reports it.
var errTestRowsRefused = fmt.Errorf("%w: failed to upsert web_sessions: pq: invalid byte sequence for encoding \"UTF8\": 0xff",
	domain.ErrWebAnalyticsRowsRefused)

// batchRecorder stands in for PostgreSQL in the salvage tests: it refuses every
// batch its refuses func objects to, commits every other one, and records the
// session ids of each batch it is handed, in call order.
type batchRecorder struct {
	t       *testing.T
	refuses func(sessions []*domain.WebSession) bool
	batches [][]string
	written map[string]int // session id -> batches committed with it
}

func newBatchRecorder(t *testing.T, refuses func(sessions []*domain.WebSession) bool) *batchRecorder {
	return &batchRecorder{t: t, refuses: refuses, written: map[string]int{}}
}

// refusing objects to any batch holding one of the given sessions.
func refusing(ids ...string) func(sessions []*domain.WebSession) bool {
	return func(sessions []*domain.WebSession) bool {
		for _, s := range sessions {
			for _, id := range ids {
				if s.ID == id {
					return true
				}
			}
		}
		return false
	}
}

func (r *batchRecorder) flush(_ context.Context, _ string, sessions []*domain.WebSession, pages []*domain.WebPage, goals []*domain.WebGoal) error {
	ids := make([]string, 0, len(sessions))
	in := map[string]bool{}
	for _, s := range sessions {
		ids = append(ids, s.ID)
		in[s.ID] = true
	}
	// A writer's rows travel together, whichever part of a split they land in:
	// bufSession gives every writer a page, and no row comes without its session.
	assert.Len(r.t, pages, len(sessions))
	for _, p := range pages {
		assert.True(r.t, in[p.SessionID], "page of %s handed over without its session", p.SessionID)
	}
	for _, g := range goals {
		assert.True(r.t, in[g.SessionID], "goal of %s handed over without its session", g.SessionID)
	}
	sort.Strings(ids)
	r.batches = append(r.batches, ids)
	if r.refuses(sessions) {
		return errTestRowsRefused
	}
	for _, id := range ids {
		r.written[id]++
	}
	return nil
}

func addBufferedWriters(buffer *WebAnalyticsBuffer, workspaceID string, ids ...string) {
	for _, id := range ids {
		s, p, g := bufSession(id, 1, 0)
		buffer.Add(workspaceID, 0, s, p, g)
	}
}

// spreadRefusedWriters names n writers and has a share of them refused, spread
// across the id space, as ids anyone can pick spread them.
func spreadRefusedWriters(n int) (writers, clean, refused []string) {
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("s-%02d", i)
		writers = append(writers, id)
		if i%4 == 2 {
			refused = append(refused, id)
		} else {
			clean = append(clean, id)
		}
	}
	return writers, clean, refused
}

// TestWebAnalyticsBufferSalvagesARefusedBatch covers what happens when
// PostgreSQL refuses rows of a workspace's batch: the flush writes the whole
// batch in one transaction, so one writer's refused value would fail every
// other writer's rows with it, and after the retry drop them all. The buffer
// splits such a batch instead, so that only the writers whose own rows are
// refused fail.
func TestWebAnalyticsBufferSalvagesARefusedBatch(t *testing.T) {
	ctx := context.Background()
	clean := []string{"s-1", "s-2", "s-3", "s-4", "s-5", "s-6", "s-7"}

	t.Run("a writer PostgreSQL refuses costs only itself", func(t *testing.T) {
		buffer, repo, _ := newBufferForTest(t)
		allowProjection(repo)
		db := newBatchRecorder(t, refusing("s-bad"))
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(db.flush).AnyTimes()
		addBufferedWriters(buffer, "ws1", append(append([]string{}, clean...), "s-bad")...)

		buffer.FlushAll(ctx)
		require.NotEmpty(t, db.batches)
		assert.Len(t, db.batches[0], len(clean)+1, "the whole run is tried first, as one transaction")
		for _, id := range clean {
			assert.Equal(t, 1, db.written[id], "%s is written, once", id)
		}
		assert.Zero(t, db.written["s-bad"])
		assert.Equal(t, len(clean)+1, buffer.PendingSessions("ws1"), "the refused writer keeps its retry")

		calls := len(db.batches)
		buffer.FlushAll(ctx)
		require.Len(t, db.batches, calls+1, "the retry is that writer alone, which leaves nothing to split")
		assert.Equal(t, []string{"s-bad"}, db.batches[calls])
		assert.Equal(t, len(clean), buffer.PendingSessions("ws1"), "and it is dropped once the retry fails too")

		buffer.FlushAll(ctx)
		assert.Len(t, db.batches, calls+1, "nothing is left to write")
		for _, id := range clean {
			assert.Equal(t, 1, db.written[id])
		}
	})

	t.Run("rows refused only in each other's company are all written", func(t *testing.T) {
		// Writers PostgreSQL refuses only together, as it does the same key
		// written twice in one statement. Apart, each is fine.
		buffer, repo, _ := newBufferForTest(t)
		allowProjection(repo)
		db := newBatchRecorder(t, func(sessions []*domain.WebSession) bool {
			return refusing("s-1")(sessions) && refusing("s-7")(sessions)
		})
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(db.flush).AnyTimes()
		addBufferedWriters(buffer, "ws1", clean...)

		buffer.FlushAll(ctx)
		for _, id := range clean {
			assert.Equal(t, 1, db.written[id], id)
		}
		calls := len(db.batches)
		buffer.FlushAll(ctx)
		assert.Len(t, db.batches, calls, "every writer landed, so none is retried")
	})

	t.Run("a failure that is not a refusal is not split", func(t *testing.T) {
		// Every part of a batch that failed for a reason of the server's own —
		// the connection, a timeout — would fail the same way.
		buffer, repo, _ := newBufferForTest(t)
		allowProjection(repo)
		calls := 0
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, _ []*domain.WebSession, _ []*domain.WebPage, _ []*domain.WebGoal) error {
				calls++
				return errors.New("dial tcp: connection refused")
			}).AnyTimes()
		addBufferedWriters(buffer, "ws1", clean...)

		buffer.FlushAll(ctx)
		assert.Equal(t, 1, calls)
		assert.Equal(t, len(clean), buffer.PendingSessions("ws1"), "kept for their retry")
		buffer.FlushAll(ctx)
		assert.Equal(t, 2, calls)
		assert.Zero(t, buffer.PendingSessions("ws1"), "and dropped once it fails too, as before")
	})

	t.Run("the salvage spends no more flushes than its budget, and the writers it has not reached land later", func(t *testing.T) {
		// Refused writers spread across the id space, as ids anyone can pick
		// spread them: every part of the first splits holds one, so the first
		// flushes run out of budget before a single part lands. The writers the
		// split has not reached must not be charged for that, or they would be
		// dropped with the refused ones.
		buffer, repo, _ := newBufferForTest(t)
		allowProjection(repo)
		const budget = 2
		buffer.cfg.MaxSalvageFlushes = budget
		writers, cleanWriters, refused := spreadRefusedWriters(16)
		db := newBatchRecorder(t, refusing(refused...))
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(db.flush).AnyTimes()
		addBufferedWriters(buffer, "ws1", writers...)
		ws := buffer.workspaces["ws1"]

		calls := 0
		flush := func() {
			buffer.FlushAll(ctx)
			assert.LessOrEqual(t, len(db.batches)-calls, 1+budget, "a flush spends the run's own call and the budget, no more")
			calls = len(db.batches)
		}
		flush()
		assert.Len(t, db.batches, 1+budget, "the run itself, then the budget")
		flush()
		require.Empty(t, db.written, "precondition: two flushes, and still no part has landed")
		for _, id := range cleanWriters {
			entry := ws.sessions[webBufferKey(id, 0)]
			require.NotNil(t, entry, "%s survives both flushes", id)
			assert.Zero(t, entry.failedAttempts, "%s is not charged for what the budget left unresolved", id)
			assert.True(t, entry.dirty, id)
		}

		dirty := func() int {
			n := 0
			for _, entry := range ws.sessions {
				if entry.dirty {
					n++
				}
			}
			return n
		}
		for i := 0; i < 100 && dirty() > 0; i++ {
			flush()
		}
		require.Zero(t, dirty(), "the split goes on from where each flush left it, until it is done")
		for _, id := range cleanWriters {
			assert.Equal(t, 1, db.written[id], "%s lands, once", id)
		}
		for _, id := range refused {
			assert.Zero(t, db.written[id], id)
			assert.NotContains(t, ws.sessions, webBufferKey(id, 0), "%s is dropped once its retry is refused too", id)
		}
	})

	t.Run("a writer the salvage keeps deferring is dropped in the end", func(t *testing.T) {
		// More refused writers than the salvage can single out in the flushes
		// the bound allows: without the bound, the writers the split has not
		// reached yet would stay buffered and dirty for as long as it took.
		buffer, repo, _ := newBufferForTest(t)
		allowProjection(repo)
		buffer.cfg.MaxSalvageFlushes = 1
		db := newBatchRecorder(t, func([]*domain.WebSession) bool { return true })
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(db.flush).AnyTimes()
		bound := webBufferMaxDeferredFlushes + webBufferMaxFlushAttempts
		var writers []string
		for i := 0; i < 4*bound; i++ {
			writers = append(writers, fmt.Sprintf("s-%03d", i))
		}
		addBufferedWriters(buffer, "ws1", writers...)

		buffer.FlushAll(ctx)
		ws := buffer.workspaces["ws1"]
		require.Len(t, ws.sessions, len(writers), "a deferral drops nobody")
		for key, entry := range ws.sessions {
			assert.Zero(t, entry.failedAttempts, "%s is not charged for a deferral", key)
		}

		for i := 1; i < bound; i++ {
			buffer.FlushAll(ctx)
		}
		assert.Zero(t, buffer.PendingSessions("ws1"),
			"a writer is charged once it has been deferred for the bound, and dropped as any failing writer")
		assert.Empty(t, db.written)
	})

	t.Run("shutdown goes on with the split until the writers it deferred are written", func(t *testing.T) {
		// No tick comes after the shutdown's flush to write them.
		for _, tc := range []struct {
			name     string
			shutDown func(*WebAnalyticsBuffer)
		}{
			{"Stop", (*WebAnalyticsBuffer).Stop},
			{"Start, once its context ends", func(buffer *WebAnalyticsBuffer) {
				ended, cancel := context.WithCancel(ctx)
				cancel()
				buffer.Start(ended)
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				buffer, repo, _ := newBufferForTest(t)
				allowProjection(repo)
				buffer.cfg.MaxSalvageFlushes = 1
				writers, cleanWriters, refused := spreadRefusedWriters(16)
				db := newBatchRecorder(t, refusing(refused...))
				repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(db.flush).AnyTimes()
				addBufferedWriters(buffer, "ws1", writers...)

				tc.shutDown(buffer)
				for _, id := range cleanWriters {
					assert.Equal(t, 1, db.written[id], "%s is written before the shutdown ends", id)
				}
				for _, id := range refused {
					assert.Zero(t, db.written[id], id)
				}
			})
		}
	})

	t.Run("a failure that is not a refusal ends the salvage", func(t *testing.T) {
		buffer, repo, _ := newBufferForTest(t)
		allowProjection(repo)
		calls := 0
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, _ []*domain.WebSession, _ []*domain.WebPage, _ []*domain.WebGoal) error {
				calls++
				if calls == 1 {
					return errTestRowsRefused
				}
				return errors.New("read tcp: connection reset by peer")
			}).AnyTimes()
		addBufferedWriters(buffer, "ws1", clean...)

		buffer.FlushAll(ctx)
		assert.Equal(t, 2, calls, "the server went away: no other part is tried")
		assert.Equal(t, len(clean), buffer.PendingSessions("ws1"))
	})

	t.Run("a cancelled flush ends the salvage", func(t *testing.T) {
		buffer, repo, _ := newBufferForTest(t)
		allowProjection(repo)
		flushCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		calls := 0
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, _ []*domain.WebSession, _ []*domain.WebPage, _ []*domain.WebGoal) error {
				calls++
				cancel()
				return errTestRowsRefused
			}).AnyTimes()
		addBufferedWriters(buffer, "ws1", clean...)

		buffer.FlushAll(flushCtx)
		assert.Equal(t, 1, calls)
		assert.Equal(t, len(clean), buffer.PendingSessions("ws1"))
	})

	t.Run("a writer replaced by a newer beat during the salvage is not charged", func(t *testing.T) {
		buffer, repo, _ := newBufferForTest(t)
		allowProjection(repo)
		db := newBatchRecorder(t, func(sessions []*domain.WebSession) bool {
			for _, s := range sessions {
				if s.ID == "s-bad" && s.BeatSeq == 1 {
					return true
				}
			}
			return false
		})
		repo.EXPECT().FlushBatch(gomock.Any(), "ws1", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string, sessions []*domain.WebSession, pages []*domain.WebPage, goals []*domain.WebGoal) error {
				if len(db.batches) == 1 {
					// The visitor beats again while the salvage runs, without
					// what was refused.
					s, p, g := bufSession("s-bad", 2, 0)
					buffer.Add("ws1", 0, s, p, g)
				}
				return db.flush(ctx, workspaceID, sessions, pages, goals)
			}).AnyTimes()
		addBufferedWriters(buffer, "ws1", append(append([]string{}, clean...), "s-bad")...)

		buffer.FlushAll(ctx)
		entry := buffer.workspaces["ws1"].sessions[webBufferKey("s-bad", 0)]
		require.NotNil(t, entry)
		assert.Zero(t, entry.failedAttempts, "the failure was the old beat's")
		assert.True(t, entry.dirty)

		buffer.FlushAll(ctx)
		assert.Equal(t, 1, db.written["s-bad"], "the newer beat is written")
	})

	t.Run("goals are bridged and navigation projected only for the writers that landed", func(t *testing.T) {
		h := newBridgeForTest(t)
		buffer, repo, _ := newBufferForTest(t)
		buffer.SetContactBridge(h.bridge)
		db := newBatchRecorder(t, refusing("s-bad"))
		repo.EXPECT().FlushBatch(gomock.Any(), bridgeTestWorkspace, gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(db.flush).AnyTimes()
		var projected []string
		repo.EXPECT().ProjectContactNavigation(gomock.Any(), bridgeTestWorkspace, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sessions []*domain.WebSession) error {
				for _, s := range sessions {
					projected = append(projected, s.ID)
				}
				return nil
			}).AnyTimes()
		email := "her@example.com"
		h.contactExists(email)
		h.expectInsert(1, nil)

		for _, id := range []string{"s-good", "s-bad"} {
			session, pages, _ := bufSession(id, 1, 0)
			session.ContactEmail = &email
			goal := h.goalAt("signup", email, -time.Minute)
			goal.SessionID = id
			buffer.Add(bridgeTestWorkspace, 0, session, pages, []*domain.WebGoal{goal})
		}

		buffer.FlushAll(ctx)
		buffer.FlushAll(ctx) // the refused writer's retry, which fails too
		require.Len(t, h.inserted(), 1)
		assert.Equal(t, "s-good", h.inserted()[0].Properties["session_id"])
		assert.Equal(t, []string{"s-good"}, projected)
	})
}
