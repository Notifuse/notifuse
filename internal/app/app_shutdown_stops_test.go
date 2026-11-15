package app

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/internal/service/queue"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runAll runs its functions side by side: each of these two waits for the other to start.
func TestRunAll_RunsSideBySide(t *testing.T) {
	aStarted, bStarted := make(chan struct{}), make(chan struct{})
	done := runAll(
		func() { close(aStarted); <-bStarted },
		func() { close(bStarted); <-aStarted },
	)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the functions did not run side by side")
	}
	select {
	case <-runAll():
	case <-time.After(time.Second):
		t.Fatal("with nothing to run, the channel is closed at once")
	}
}

// Shutdown stops the email queue worker and waits for the task runs started outside the
// scheduler before the databases close.
func TestAppShutdown_StopsTheQueueAndWaitsForImmediateRuns(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockDB, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	dbMock.ExpectClose()
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Debug(gomock.Any()).AnyTimes()
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()

	a := NewApp(&config.Config{}, WithLogger(log), WithMockDB(mockDB)).(*App)

	workspaces := mocks.NewMockWorkspaceRepository(ctrl)
	workspaces.EXPECT().List(gomock.Any()).Return(nil, nil).AnyTimes()
	a.emailQueueWorker = queue.NewEmailQueueWorker(mocks.NewMockEmailQueueRepository(ctrl), workspaces,
		mocks.NewMockEmailServiceInterface(ctrl), mocks.NewMockMessageHistoryRepository(ctrl), nil, log)
	require.NoError(t, a.emailQueueWorker.Start(a.shutdownCtx))

	a.taskService = service.NewTaskService(mocks.NewMockTaskRepository(ctrl), mocks.NewMockSettingRepository(ctrl), log, nil, "")
	a.taskService.SetShutdownContext(a.shutdownCtx)
	runCtx, runDone, ok := a.taskService.BeginRun(context.Background())
	require.True(t, ok)
	ended := make(chan time.Time, 1)
	go func() {
		<-runCtx.Done() // the shutdown cancels the run
		time.Sleep(50 * time.Millisecond)
		ended <- time.Now()
		runDone()
	}()

	require.NoError(t, a.Shutdown(context.Background()))
	returned := time.Now()

	assert.False(t, a.emailQueueWorker.IsRunning())
	select {
	case at := <-ended:
		assert.False(t, returned.Before(at), "Shutdown returned before the run ended")
	default:
		t.Fatal("Shutdown returned before the run ended")
	}
}
