package service

import (
	"context"
	"errors"
	"fmt"
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
		buffer.flushDue(ctx)                    // not due yet

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
