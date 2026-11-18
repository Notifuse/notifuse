package main

import (
	"context"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// stuckShutdownApp starts at once and never finishes its shutdown, whatever its context says.
type stuckShutdownApp struct {
	app.AppInterface
	started chan struct{}
	release chan struct{}
}

func (a *stuckShutdownApp) Initialize() error { return nil }

func (a *stuckShutdownApp) Start() error {
	close(a.started)
	<-a.release
	return nil
}

func (a *stuckShutdownApp) Shutdown(ctx context.Context) error {
	<-a.release
	return nil
}

func (a *stuckShutdownApp) SetShutdownTimeout(time.Duration) {}

func (a *stuckShutdownApp) GetActiveRequestCount() int64 { return 0 }

func TestRunServer_ReturnsWhenTheShutdownOutlivesItsTimeout(t *testing.T) {
	originalNewApp := newApp
	originalSignalNotify := signalNotify
	originalShutdownTimeout := shutdownTimeout
	defer func() {
		newApp = originalNewApp
		signalNotify = originalSignalNotify
		shutdownTimeout = originalShutdownTimeout
	}()

	fakeApp := &stuckShutdownApp{started: make(chan struct{}), release: make(chan struct{})}
	defer close(fakeApp.release)
	newApp = func(cfg *config.Config, opts ...app.AppOption) app.AppInterface { return fakeApp }
	shutdownTimeout = 50 * time.Millisecond

	var calls int32
	signalNotify = func(c chan<- os.Signal, sig ...os.Signal) {
		if atomic.AddInt32(&calls, 1) == 1 {
			go func() {
				<-fakeApp.started
				c <- os.Interrupt
			}()
		}
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	done := make(chan error, 1)
	go func() { done <- runServer(&config.Config{}, mockLogger) }()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(5 * time.Second):
		t.Fatal("runServer kept waiting for a shutdown past its timeout")
	}
}

// selfClosedApp closes its own server, as setup and the system settings do to reload the configuration.
type selfClosedApp struct {
	app.AppInterface
}

func (a *selfClosedApp) Initialize() error { return nil }

func (a *selfClosedApp) Start() error { return http.ErrServerClosed }

func TestRunServer_ServerClosedForReloadIsNotLoggedAsAnError(t *testing.T) {
	originalNewApp := newApp
	originalSignalNotify := signalNotify
	defer func() {
		newApp = originalNewApp
		signalNotify = originalSignalNotify
	}()
	newApp = func(cfg *config.Config, opts ...app.AppOption) app.AppInterface { return &selfClosedApp{} }
	signalNotify = func(c chan<- os.Signal, sig ...os.Signal) {}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info("Server started successfully").AnyTimes()
	mockLogger.EXPECT().Info("Server stopped to load its new configuration - exiting so that the process manager restarts it").Times(1)
	mockLogger.EXPECT().Error(gomock.Any()).Times(0)

	err := runServer(&config.Config{}, mockLogger)

	// Still an error: main exits with a failure code, which every restart policy restarts
	assert.ErrorIs(t, err, http.ErrServerClosed)
}
