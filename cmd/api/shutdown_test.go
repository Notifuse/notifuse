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

// selfClosedApp closes its own server, as setup and the system settings do to reload the
// configuration: Start returns at once, and its Shutdown ends when shutdownEnds is closed.
type selfClosedApp struct {
	app.AppInterface
	shutdownEnds chan struct{}
}

func (a *selfClosedApp) Initialize() error { return nil }

func (a *selfClosedApp) Start() error { return http.ErrServerClosed }

func (a *selfClosedApp) ShutdownDone() <-chan struct{} { return a.shutdownEnds }

// runSelfClosed runs runServer on fake; a value sent on the returned signal channel arrives as a signal.
func runSelfClosed(t *testing.T, fake *selfClosedApp, logger *pkgmocks.MockLogger) (<-chan error, chan<- os.Signal) {
	originalNewApp := newApp
	originalSignalNotify := signalNotify
	t.Cleanup(func() {
		newApp = originalNewApp
		signalNotify = originalSignalNotify
	})
	newApp = func(cfg *config.Config, opts ...app.AppOption) app.AppInterface { return fake }
	signals := make(chan os.Signal, 1)
	var calls int32
	signalNotify = func(c chan<- os.Signal, sig ...os.Signal) {
		if atomic.AddInt32(&calls, 1) == 1 {
			go func() {
				for s := range signals {
					c <- s
				}
			}()
		}
	}
	t.Cleanup(func() { close(signals) })
	done := make(chan error, 1)
	go func() { done <- runServer(&config.Config{}, logger) }()
	return done, signals
}

func TestRunServer_ServerClosedForReloadWaitsForTheShutdownAndLogsNoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info("Server started successfully").AnyTimes()
	mockLogger.EXPECT().Info("Server stopped to load its new configuration - waiting for its shutdown to end").Times(1)
	mockLogger.EXPECT().Info("Exiting so that the process manager restarts the server with its new configuration").Times(1)
	mockLogger.EXPECT().Error(gomock.Any()).Times(0)
	mockLogger.EXPECT().Warn(gomock.Any()).Times(0)

	fake := &selfClosedApp{shutdownEnds: make(chan struct{})}
	done, _ := runSelfClosed(t, fake, mockLogger)

	// Still shutting down: the process must not exit yet
	select {
	case <-done:
		t.Fatal("runServer returned before the app's shutdown ended")
	case <-time.After(150 * time.Millisecond):
	}

	close(fake.shutdownEnds)
	select {
	case err := <-done:
		// Still an error: main exits with a failure code, which every restart policy restarts
		assert.ErrorIs(t, err, http.ErrServerClosed)
	case <-time.After(5 * time.Second):
		t.Fatal("runServer kept waiting once the app's shutdown had ended")
	}
}

func TestRunServer_ServerClosedForReloadExitsWhenTheShutdownOutlivesItsTimeout(t *testing.T) {
	originalShutdownTimeout := shutdownTimeout
	t.Cleanup(func() { shutdownTimeout = originalShutdownTimeout })
	shutdownTimeout = 50 * time.Millisecond

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn("Shutdown for the new configuration did not end in time - exiting").Times(1)

	// A shutdown that never ends
	done, _ := runSelfClosed(t, &selfClosedApp{shutdownEnds: make(chan struct{})}, mockLogger)

	select {
	case err := <-done:
		assert.ErrorIs(t, err, http.ErrServerClosed)
	case <-time.After(5 * time.Second):
		t.Fatal("runServer kept waiting for a shutdown past its timeout")
	}
}

func TestRunServer_ASignalDuringTheReloadShutdownExitsAtOnce(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn("Signal received while the shutdown for the new configuration runs - exiting now").Times(1)

	// A shutdown that never ends, then Ctrl+C
	done, signals := runSelfClosed(t, &selfClosedApp{shutdownEnds: make(chan struct{})}, mockLogger)
	time.Sleep(50 * time.Millisecond)
	signals <- os.Interrupt

	select {
	case err := <-done:
		assert.ErrorIs(t, err, http.ErrServerClosed)
	case <-time.After(5 * time.Second):
		t.Fatal("runServer ignored a signal while it waited for the reload's shutdown")
	}
}
