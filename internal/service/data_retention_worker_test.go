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
	assert.Equal(t, 5*time.Minute, w.initialDelay)
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

type fakeRetentionRunner struct{ runs int }

func (f *fakeRetentionRunner) RunAll(context.Context) { f.runs++ }

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
