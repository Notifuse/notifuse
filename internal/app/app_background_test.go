package app

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

func newBackgroundTestApp(t *testing.T) (*App, *sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectClose()

	cfg := createTestConfig()
	cfg.Server.Port = 19080 + (time.Now().Nanosecond() % 1000)
	app, ok := NewApp(cfg, WithLogger(mockLogger), WithMockDB(db)).(*App)
	require.True(t, ok)
	return app, db, mock
}

// stopsOnShutdown is background work that ends when the shutdown context does, then
// still uses the database, which must not be closed yet.
func stopsOnShutdown(db *sql.DB, done chan<- error) func(ctx context.Context) {
	return func(ctx context.Context) {
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond)
		done <- db.PingContext(context.Background())
	}
}

func TestApp_StartBackground(t *testing.T) {
	t.Run("runs with the shutdown context, registered until it returns", func(t *testing.T) {
		app, _, _ := newBackgroundTestApp(t)
		release := make(chan struct{})
		var got context.Context
		app.startBackground("test", func(ctx context.Context) {
			got = ctx
			<-release
		})

		waited := make(chan struct{})
		go func() { app.backgroundWg.Wait(); close(waited) }()
		select {
		case <-waited:
			t.Fatal("the work was not registered")
		case <-time.After(50 * time.Millisecond):
		}
		close(release)
		select {
		case <-waited:
		case <-time.After(5 * time.Second):
			t.Fatal("the work never finished")
		}
		assert.Equal(t, app.shutdownCtx, got)
	})

	t.Run("without a server, Shutdown stops it and waits for it before closing the database", func(t *testing.T) {
		app, db, mock := newBackgroundTestApp(t)
		done := make(chan error, 1)
		app.startBackground("test", stopsOnShutdown(db, done))

		require.NoError(t, app.Shutdown(context.Background()))
		select {
		case err := <-done:
			assert.NoError(t, err, "the database was closed while the work still ran")
		default:
			t.Fatal("Shutdown returned before the work ended")
		}
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("with a server, Shutdown stops it and waits for it before closing the database", func(t *testing.T) {
		app, db, mock := newBackgroundTestApp(t)
		app.SetShutdownTimeout(2 * time.Second)
		errCh := make(chan error, 1)
		go func() { errCh <- app.Start() }()
		startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.True(t, app.WaitForServerStart(startCtx))

		done := make(chan error, 1)
		app.startBackground("test", stopsOnShutdown(db, done))

		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelShutdown()
		require.NoError(t, app.Shutdown(shutdownCtx))
		select {
		case err := <-done:
			assert.NoError(t, err, "the database was closed while the work still ran")
		default:
			t.Fatal("Shutdown returned before the work ended")
		}
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("work ignoring the shutdown does not hold it past the caller's deadline", func(t *testing.T) {
		app, _, _ := newBackgroundTestApp(t)
		stuck := make(chan struct{})
		defer close(stuck)
		app.startBackground("stuck", func(context.Context) { <-stuck })

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		start := time.Now()
		require.NoError(t, app.Shutdown(ctx))
		assert.Less(t, time.Since(start), 2*time.Second)
	})

	t.Run("nor, with no deadline, past the shutdown timeout", func(t *testing.T) {
		app, _, _ := newBackgroundTestApp(t)
		app.SetShutdownTimeout(200 * time.Millisecond)
		stuck := make(chan struct{})
		defer close(stuck)
		app.startBackground("stuck", func(context.Context) { <-stuck })

		start := time.Now()
		require.NoError(t, app.Shutdown(context.Background()))
		elapsed := time.Since(start)
		assert.GreaterOrEqual(t, elapsed, 200*time.Millisecond)
		assert.Less(t, elapsed, 2*time.Second)
	})
}
