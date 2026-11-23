package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/dkim"
	"github.com/Notifuse/notifuse/pkg/logger"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

const (
	dkimTryLock = `SELECT pg_try_advisory_lock\(\$1\)`
	dkimUnlock  = `SELECT pg_advisory_unlock\(\$1\)`
)

type dkimWorkerFixture struct {
	w        *DKIMVerificationWorker
	repo     *fakeDKIMRepo
	resolver *staticResolver
	mock     sqlmock.Sqlmock
	now      time.Time
}

func newDKIMWorkerFixture(t *testing.T, keys ...domain.DKIMKey) *dkimWorkerFixture {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	f := &dkimWorkerFixture{
		repo:     &fakeDKIMRepo{keys: keys},
		resolver: &staticResolver{records: map[string][]string{}},
		mock:     mock,
		now:      time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC),
	}
	f.w = NewDKIMVerificationWorker(f.repo, db, f.resolver, NewDKIMKeyStore(f.repo, dkimServiceSecret), logger.NewLoggerWithLevel("disabled"))
	f.w.now = func() time.Time { return f.now }
	return f
}

func (f *dkimWorkerFixture) expectLockedPass() {
	f.mock.ExpectQuery(dkimTryLock).WithArgs(dkimVerificationLockID).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	f.mock.ExpectExec(dkimUnlock).WithArgs(dkimVerificationLockID).WillReturnResult(sqlmock.NewResult(0, 0))
}

func workerKey(t *testing.T, domainName string, status domain.DKIMKeyStatus) domain.DKIMKey {
	t.Helper()
	_, pub := dkimServiceKey(t)
	return domain.DKIMKey{
		ID: "id-" + domainName, Revision: 1, Domain: domainName, Selector: dkimTestSelector,
		PublicKey: pub, EncryptedPrivateKey: sealedServiceKey(t), Status: status,
		CreatedAt: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
	}
}

func TestDueKeys(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { at := now.Add(-d); return &at }
	recent := now.Add(-24 * time.Hour)
	old := now.Add(-15 * 24 * time.Hour)

	cases := []struct {
		name string
		key  domain.DKIMKey
		due  bool
	}{
		{"never checked", domain.DKIMKey{Status: domain.DKIMKeyStatusPending, CreatedAt: recent}, true},
		{"pending, checked 58 minutes ago (an early tick)", domain.DKIMKey{Status: domain.DKIMKeyStatusPending, CreatedAt: recent, LastCheckedAt: ago(58 * time.Minute)}, true},
		{"pending, checked 30 minutes ago", domain.DKIMKey{Status: domain.DKIMKeyStatusPending, CreatedAt: recent, LastCheckedAt: ago(30 * time.Minute)}, false},
		{"mismatch, hourly", domain.DKIMKey{Status: domain.DKIMKeyStatusMismatch, CreatedAt: recent, LastCheckedAt: ago(time.Hour)}, true},
		{"pending for over 14 days, daily", domain.DKIMKey{Status: domain.DKIMKeyStatusPending, CreatedAt: old, LastCheckedAt: ago(2 * time.Hour)}, false},
		{"pending for over 14 days, a day later", domain.DKIMKey{Status: domain.DKIMKeyStatusPending, CreatedAt: old, LastCheckedAt: ago(24 * time.Hour)}, true},
		{"published and passing, daily", domain.DKIMKey{Status: domain.DKIMKeyStatusPublished, LastCheckedAt: ago(2 * time.Hour)}, false},
		{"published and passing, a day later", domain.DKIMKey{Status: domain.DKIMKeyStatusPublished, LastCheckedAt: ago(24 * time.Hour)}, true},
		{"published with a failure, hourly", domain.DKIMKey{Status: domain.DKIMKeyStatusPublished, ConsecutiveFailures: 1, LastCheckedAt: ago(time.Hour)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dueKeys([]domain.WorkspaceDKIMMetadata{{WorkspaceID: "ws", Keys: []domain.DKIMKey{tc.key}}}, now)
			assert.Equal(t, tc.due, len(got) == 1)
		})
	}

	t.Run("least recently checked first, never checked before all", func(t *testing.T) {
		got := dueKeys([]domain.WorkspaceDKIMMetadata{
			{WorkspaceID: "a", Keys: []domain.DKIMKey{{Domain: "two.io", Status: domain.DKIMKeyStatusPending, CreatedAt: recent, LastCheckedAt: ago(2 * time.Hour)}}},
			{WorkspaceID: "b", Keys: []domain.DKIMKey{
				{Domain: "three.io", Status: domain.DKIMKeyStatusPending, CreatedAt: recent, LastCheckedAt: ago(time.Hour)},
				{Domain: "one.io", Status: domain.DKIMKeyStatusPending, CreatedAt: recent, LastCheckedAt: ago(5 * time.Hour)},
				{Domain: "zero.io", Status: domain.DKIMKeyStatusPending, CreatedAt: recent},
			}},
		}, now)
		var order []string
		for _, d := range got {
			order = append(order, d.key.Domain)
		}
		assert.Equal(t, []string{"zero.io", "one.io", "two.io", "three.io"}, order)
	})
}

func TestDKIMVerificationWorker_RunOnce(t *testing.T) {
	ctx := context.Background()

	t.Run("publishes a key whose record appeared, then releases the lock", func(t *testing.T) {
		key := workerKey(t, "acme.io", domain.DKIMKeyStatusPending)
		f := newDKIMWorkerFixture(t, key)
		f.resolver.set(dkim.RecordName(key.Selector, key.Domain), dkim.FormatDNSValue(key.PublicKey))
		f.expectLockedPass()

		f.w.RunOnce(ctx)

		assert.Equal(t, domain.DKIMKeyStatusPublished, f.repo.keys[0].Status)
		assert.Equal(t, 1, f.repo.touches, "a status change moves updated_at")
		assert.NoError(t, f.mock.ExpectationsWereMet())
	})

	t.Run("a check that only moves last_checked_at leaves updated_at alone", func(t *testing.T) {
		f := newDKIMWorkerFixture(t, workerKey(t, "acme.io", domain.DKIMKeyStatusPending))
		f.expectLockedPass()

		f.w.RunOnce(ctx)

		assert.Equal(t, domain.DKIMKeyStatusPending, f.repo.keys[0].Status)
		assert.NotNil(t, f.repo.keys[0].LastCheckedAt)
		assert.Equal(t, 1, f.repo.writes)
		assert.Zero(t, f.repo.touches)
	})

	t.Run("another instance holds the lock: nothing is read", func(t *testing.T) {
		f := newDKIMWorkerFixture(t, workerKey(t, "acme.io", domain.DKIMKeyStatusPending))
		f.mock.ExpectQuery(dkimTryLock).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))

		f.w.RunOnce(ctx)

		assert.Zero(t, f.repo.lists)
		assert.NoError(t, f.mock.ExpectationsWereMet())
	})

	t.Run("a key regenerated during the lookup is not overwritten", func(t *testing.T) {
		key := workerKey(t, "acme.io", domain.DKIMKeyStatusPending)
		f := newDKIMWorkerFixture(t, key)
		f.resolver.set(dkim.RecordName(key.Selector, key.Domain), dkim.FormatDNSValue(key.PublicKey))
		f.repo.beforeUpdate = func(r *fakeDKIMRepo) {
			r.mu.Lock()
			r.keys[0].ID = "regenerated"
			r.mu.Unlock()
		}
		f.expectLockedPass()

		f.w.RunOnce(ctx)

		assert.Equal(t, domain.DKIMKeyStatusPending, f.repo.keys[0].Status)
		assert.Zero(t, f.repo.writes)
	})

	t.Run("a deleted key is not written back", func(t *testing.T) {
		key := workerKey(t, "acme.io", domain.DKIMKeyStatusPending)
		f := newDKIMWorkerFixture(t, key)
		f.repo.beforeUpdate = func(r *fakeDKIMRepo) {
			r.mu.Lock()
			r.keys = nil
			r.mu.Unlock()
		}
		f.expectLockedPass()

		f.w.RunOnce(ctx)
		assert.Empty(t, f.repo.keys)
	})

	t.Run("a published key stops signing after three hourly failures, with one warning", func(t *testing.T) {
		key := workerKey(t, "acme.io", domain.DKIMKeyStatusPublished)
		verified := key.CreatedAt
		key.VerifiedAt = &verified
		f := newDKIMWorkerFixture(t, key)

		ctrl := gomock.NewController(t)
		log := pkgmocks.NewMockLogger(ctrl)
		log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().Warn(gomock.Any()).Times(1)
		f.w.logger = log

		for i := 0; i < 3; i++ {
			f.expectLockedPass()
			f.w.RunOnce(ctx)
			if i < 2 {
				assert.Equal(t, domain.DKIMKeyStatusPublished, f.repo.keys[0].Status, "pass %d", i)
			}
			f.now = f.now.Add(time.Hour)
		}
		assert.Equal(t, domain.DKIMKeyStatusPending, f.repo.keys[0].Status)
		assert.NotNil(t, f.repo.keys[0].DowngradedAt)
	})

	t.Run("lookups run at most five at a time", func(t *testing.T) {
		var keys []domain.DKIMKey
		for _, d := range []string{"a.io", "b.io", "c.io", "d.io", "e.io", "f.io", "g.io", "h.io"} {
			keys = append(keys, workerKey(t, d, domain.DKIMKeyStatusPending))
		}
		f := newDKIMWorkerFixture(t, keys...)
		counting := &countingResolver{release: make(chan struct{})}
		f.w.resolver = counting
		f.expectLockedPass()

		done := make(chan struct{})
		go func() { f.w.RunOnce(ctx); close(done) }()
		require.Eventually(t, func() bool { return counting.current() == 5 }, 2*time.Second, 5*time.Millisecond)
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 5, counting.max())
		close(counting.release)
		<-done
		assert.Equal(t, 8, f.repo.writes)
	})
}

func TestDKIMVerificationWorker_Unlock(t *testing.T) {
	t.Run("a released lock gives the connection back to the pool", func(t *testing.T) {
		f := newDKIMWorkerFixture(t)
		db := f.w.db
		f.expectLockedPass()
		f.w.RunOnce(context.Background())
		assert.Equal(t, 1, db.Stats().Idle)
	})

	t.Run("a failed release drops the connection, which may still hold the lock", func(t *testing.T) {
		f := newDKIMWorkerFixture(t)
		db := f.w.db
		f.mock.ExpectQuery(dkimTryLock).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
		f.mock.ExpectExec(dkimUnlock).WillReturnError(errors.New("connection reset"))
		f.w.RunOnce(context.Background())
		assert.Zero(t, db.Stats().Idle)
		assert.Zero(t, db.Stats().OpenConnections)
	})

	t.Run("a shutdown during the pass still releases the lock", func(t *testing.T) {
		f := newDKIMWorkerFixture(t, workerKey(t, "acme.io", domain.DKIMKeyStatusPending))
		ctx, cancel := context.WithCancel(context.Background())
		f.w.resolver = cancellingResolver{cancel: cancel}
		f.expectLockedPass()
		f.w.RunOnce(ctx)
		assert.NoError(t, f.mock.ExpectationsWereMet())
		assert.Zero(t, f.repo.writes, "nothing written after the cancel")
	})
}

type failingListRepo struct {
	fakeDKIMRepo
}

func (r *failingListRepo) ListWorkspacesDKIMMetadata(ctx context.Context) ([]domain.WorkspaceDKIMMetadata, error) {
	return []domain.WorkspaceDKIMMetadata{}, errors.New("workspace ws-bad: failed to decode the DKIM keys")
}

func TestDKIMVerificationWorker_LogsAnUnreadableWorkspaceOnce(t *testing.T) {
	f := newDKIMWorkerFixture(t)
	f.w.repo = &failingListRepo{}
	ctrl := gomock.NewController(t)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).Times(1)
	f.w.logger = log

	for i := 0; i < 3; i++ {
		f.expectLockedPass()
		f.w.RunOnce(context.Background())
	}
	assert.NoError(t, f.mock.ExpectationsWereMet())
}

type countingResolver struct {
	mu       sync.Mutex
	inFlight int
	peak     int
	release  chan struct{}
}

func (c *countingResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	c.mu.Lock()
	c.inFlight++
	if c.inFlight > c.peak {
		c.peak = c.inFlight
	}
	c.mu.Unlock()
	<-c.release
	c.mu.Lock()
	c.inFlight--
	c.mu.Unlock()
	return nil, &dnsNotFound{}
}

func (c *countingResolver) current() int { c.mu.Lock(); defer c.mu.Unlock(); return c.inFlight }
func (c *countingResolver) max() int     { c.mu.Lock(); defer c.mu.Unlock(); return c.peak }

type dnsNotFound struct{}

func (*dnsNotFound) Error() string { return "not found" }

type cancellingResolver struct{ cancel context.CancelFunc }

func (r cancellingResolver) LookupTXT(ctx context.Context, _ string) ([]string, error) {
	r.cancel()
	return nil, ctx.Err()
}
