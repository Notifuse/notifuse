package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeWorkspaceLister struct {
	workspaces []*domain.Workspace
	err        error
}

func (f fakeWorkspaceLister) List(context.Context) ([]*domain.Workspace, error) {
	return f.workspaces, f.err
}

type fakeHashPurger struct {
	mu     sync.Mutex
	calls  []string
	before []time.Time
	fail   map[string]bool
}

func (f *fakeHashPurger) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeHashPurger) PurgeOptOutHashes(_ context.Context, workspaceID string, recordedBefore time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, workspaceID)
	f.before = append(f.before, recordedBefore)
	if f.fail[workspaceID] {
		return 0, errors.New("boom")
	}
	return 1, nil
}

func newTestDataRetentionWorker(t *testing.T, lister workspaceLister, purger optOutHashPurger) *DataRetentionWorker {
	t.Helper()
	ctrl := gomock.NewController(t)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	return NewDataRetentionWorker(lister, purger, log)
}

func TestDataRetentionWorker_Defaults(t *testing.T) {
	w := newTestDataRetentionWorker(t, fakeWorkspaceLister{}, &fakeHashPurger{})
	assert.Equal(t, 24*time.Hour, w.interval)
	assert.LessOrEqual(t, w.initialDelay, 24*time.Hour)
}

// The run starts every day at 05:00 UTC, the time the settings page states.
func TestDataRetentionWorker_RunsAtFiveUTC(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 11, 23, h, m, 0, 0, time.UTC) }
	assert.Equal(t, 2*time.Hour, untilNextRun(at(3, 0)))
	assert.Equal(t, 24*time.Hour, untilNextRun(at(5, 0)), "at 05:00 exactly, the next one is tomorrow")
	assert.Equal(t, 23*time.Hour+30*time.Minute, untilNextRun(at(5, 30)))
	paris := time.FixedZone("CET", 3600)
	assert.Equal(t, time.Hour, untilNextRun(time.Date(2026, 11, 23, 5, 0, 0, 0, paris)), "05:00 in Paris is 04:00 UTC")
}

func TestDataRetentionWorker_RunOnce(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

	t.Run("purges every workspace three years back, past one that fails", func(t *testing.T) {
		purger := &fakeHashPurger{fail: map[string]bool{"b": true}}
		w := newTestDataRetentionWorker(t, fakeWorkspaceLister{workspaces: []*domain.Workspace{{ID: "a"}, {ID: "b"}, {ID: "c"}}}, purger)
		w.nowFn = func() time.Time { return now }

		w.RunOnce(context.Background())

		assert.Equal(t, []string{"a", "b", "c"}, purger.calls)
		for _, before := range purger.before {
			assert.Equal(t, time.Date(2023, 11, 23, 12, 0, 0, 0, time.UTC), before, "three calendar years")
		}
	})

	t.Run("a failed listing purges nothing", func(t *testing.T) {
		purger := &fakeHashPurger{}
		w := newTestDataRetentionWorker(t, fakeWorkspaceLister{err: errors.New("down")}, purger)
		w.RunOnce(context.Background())
		assert.Empty(t, purger.calls)
	})

	t.Run("a cancelled context stops between workspaces", func(t *testing.T) {
		purger := &fakeHashPurger{}
		w := newTestDataRetentionWorker(t, fakeWorkspaceLister{workspaces: []*domain.Workspace{{ID: "a"}, {ID: "b"}}}, purger)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		w.RunOnce(ctx)
		assert.Empty(t, purger.calls)
	})
}

func TestDataRetentionWorker_StartStopsOnCancel(t *testing.T) {
	purger := &fakeHashPurger{}
	w := newTestDataRetentionWorker(t, fakeWorkspaceLister{workspaces: []*domain.Workspace{{ID: "a"}}}, purger)
	w.initialDelay = time.Millisecond
	w.interval = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Start(ctx); close(done) }()
	require.Eventually(t, func() bool { return purger.callCount() == 1 }, time.Second, 5*time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

type fakeRetentionRunner struct {
	runs    int
	lastRun *time.Time
}

func (f *fakeRetentionRunner) RunAll(context.Context) { f.runs++ }
func (f *fakeRetentionRunner) LastRunAt(context.Context) (time.Time, bool) {
	if f.lastRun == nil {
		return time.Time{}, false
	}
	return *f.lastRun, true
}

// A server that missed 05:00 UTC catches up 5 minutes after it starts.
func TestDataRetentionWorker_CatchesUpAMissedRun(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	recent, stale := now.Add(-7*time.Hour), now.Add(-30*time.Hour)
	for name, tc := range map[string]struct {
		last *time.Time
		want time.Duration
	}{
		"never ran":        {nil, 5 * time.Minute},
		"ran long ago":     {&stale, 5 * time.Minute},
		"ran this morning": {&recent, 17 * time.Hour},
	} {
		t.Run(name, func(t *testing.T) {
			w := newTestDataRetentionWorker(t, fakeWorkspaceLister{}, &fakeHashPurger{})
			w.nowFn = func() time.Time { return now }
			w.initialDelay = untilNextRun(now)
			w.SetRunner(&fakeRetentionRunner{lastRun: tc.last})
			assert.Equal(t, tc.want, w.firstDelay(context.Background()))
		})
	}
}

// The policies run after the hashes, in every licence state: the worker holds no licence.
func TestDataRetentionWorker_RunsThePolicies(t *testing.T) {
	purger := &fakeHashPurger{}
	runner := &fakeRetentionRunner{}
	w := newTestDataRetentionWorker(t, fakeWorkspaceLister{workspaces: []*domain.Workspace{{ID: "a"}}}, purger)
	w.SetRunner(runner)
	w.RunOnce(context.Background())
	assert.Equal(t, 1, purger.callCount())
	assert.Equal(t, 1, runner.runs)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w.RunOnce(ctx)
	assert.Equal(t, 1, runner.runs, "a cancelled context runs nothing")
}
