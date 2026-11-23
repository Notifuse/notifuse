package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
)

type fakeOAuthMaintainer struct {
	mu     sync.Mutex
	calls  int
	result *domain.OAuthPurgeResult
	err    error
}

func (m *fakeOAuthMaintainer) RunMaintenance(context.Context) (*domain.OAuthPurgeResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return m.result, m.err
}

func (m *fakeOAuthMaintainer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func TestOAuthWorker_Defaults(t *testing.T) {
	worker := NewOAuthWorker(&fakeOAuthMaintainer{}, nil)
	assert.Equal(t, time.Hour, worker.interval, "every hour")
	assert.Equal(t, 2*time.Minute, worker.initialDelay)
}

func TestOAuthWorker_RunOnce(t *testing.T) {
	maintainer := &fakeOAuthMaintainer{result: &domain.OAuthPurgeResult{GrantsRevoked: 1, TokensPurged: 4}}
	worker := NewOAuthWorker(maintainer, auditQuietLogger(t))
	worker.RunOnce(context.Background())
	assert.Equal(t, 1, maintainer.count())

	maintainer.err = errors.New("boom")
	assert.NotPanics(t, func() { worker.RunOnce(context.Background()) })
	assert.Equal(t, 2, maintainer.count())

	maintainer.err, maintainer.result = nil, &domain.OAuthPurgeResult{}
	assert.NotPanics(t, func() { NewOAuthWorker(maintainer, nil).RunOnce(context.Background()) })
}

func TestOAuthWorker_Start(t *testing.T) {
	t.Run("waits the initial delay, runs, then follows the ticker", func(t *testing.T) {
		maintainer := &fakeOAuthMaintainer{result: &domain.OAuthPurgeResult{}}
		worker := NewOAuthWorker(maintainer, auditQuietLogger(t))
		worker.initialDelay = 5 * time.Millisecond
		worker.interval = 10 * time.Millisecond

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
		defer cancel()
		worker.Start(ctx)
		assert.GreaterOrEqual(t, maintainer.count(), 2)
	})

	t.Run("a cancelled context before the delay runs nothing", func(t *testing.T) {
		maintainer := &fakeOAuthMaintainer{}
		worker := NewOAuthWorker(maintainer, auditQuietLogger(t))
		worker.initialDelay = time.Hour
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		worker.Start(ctx)
		assert.Zero(t, maintainer.count())
	})
}

// The worker runs the service's job, which runs each rule of the repository:
// grants left without a credential, expired rows, and the client cache.
func TestOAuthWorker_RunsEveryRule(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockOAuthRepository(ctrl)
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	svc := NewOAuthService(OAuthServiceConfig{Repo: repo, Now: func() time.Time { return now }})

	repo.EXPECT().RevokeGrantsWithoutCredential(gomock.Any(), now).Return(int64(1), nil)
	repo.EXPECT().PurgeExpired(gomock.Any(), now).Return(int64(1), int64(1), int64(1), nil)
	repo.EXPECT().PurgeClients(gomock.Any(), now).Return(int64(1), nil)
	NewOAuthWorker(svc, auditQuietLogger(t)).RunOnce(context.Background())

	t.Run("a failed rule stops the run", func(t *testing.T) {
		repo.EXPECT().RevokeGrantsWithoutCredential(gomock.Any(), now).Return(int64(0), nil)
		repo.EXPECT().PurgeExpired(gomock.Any(), now).Return(int64(0), int64(0), int64(0), errors.New("boom"))
		require.NotPanics(t, func() { NewOAuthWorker(svc, auditQuietLogger(t)).RunOnce(context.Background()) })
	})
}
