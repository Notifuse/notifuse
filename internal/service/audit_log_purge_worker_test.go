package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

type fakeAuditPurger struct {
	calls   int
	deleted int64
	err     error
}

func (p *fakeAuditPurger) PurgeExpired(context.Context) (int64, error) {
	p.calls++
	return p.deleted, p.err
}

func auditQuietLogger(t *testing.T) *pkgmocks.MockLogger {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	return mockLogger
}

func TestAuditLogPurgeWorker_RunOnce(t *testing.T) {
	purger := &fakeAuditPurger{deleted: 3}
	worker := NewAuditLogPurgeWorker(purger, auditQuietLogger(t))
	worker.RunOnce(context.Background())
	assert.Equal(t, 1, purger.calls)

	purger.err = errors.New("boom")
	assert.NotPanics(t, func() { worker.RunOnce(context.Background()) })
	assert.Equal(t, 2, purger.calls)
}

func TestAuditLogPurgeWorker_Defaults(t *testing.T) {
	worker := NewAuditLogPurgeWorker(&fakeAuditPurger{}, nil)
	assert.Equal(t, 24*time.Hour, worker.interval)
	assert.Equal(t, 2*time.Minute, worker.initialDelay)
}

func TestAuditLogPurgeWorker_Start(t *testing.T) {
	t.Run("waits the initial delay, runs, then follows the ticker", func(t *testing.T) {
		purger := &fakeAuditPurger{}
		worker := NewAuditLogPurgeWorker(purger, auditQuietLogger(t))
		worker.initialDelay = 5 * time.Millisecond
		worker.interval = 10 * time.Millisecond

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
		defer cancel()
		worker.Start(ctx)
		assert.GreaterOrEqual(t, purger.calls, 2)
	})

	t.Run("a cancelled context before the delay runs nothing", func(t *testing.T) {
		purger := &fakeAuditPurger{}
		worker := NewAuditLogPurgeWorker(purger, auditQuietLogger(t))
		worker.initialDelay = time.Hour
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		worker.Start(ctx)
		assert.Zero(t, purger.calls)
	})
}

// The "Never" list on domain.EntitlementProvider says nothing that deletes
// data may consult the licence. The worker cannot: it has no field that could
// hold a provider.
func TestAuditLogPurgeWorker_HoldsNoEntitlementProvider(t *testing.T) {
	providerType := reflect.TypeOf((*domain.EntitlementProvider)(nil)).Elem()
	workerType := reflect.TypeOf(AuditLogPurgeWorker{})
	for i := 0; i < workerType.NumField(); i++ {
		field := workerType.Field(i)
		require.False(t, field.Type == providerType || field.Type.Implements(providerType),
			"field %s must not be an EntitlementProvider", field.Name)
	}
}
