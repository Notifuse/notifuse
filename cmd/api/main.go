package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// gracefulShutdownTimeout bounds the graceful shutdown that a SIGTERM starts. A container
// runtime must wait longer than this before it kills the process: compose.yaml sets the api
// service's stop_grace_period accordingly.
const gracefulShutdownTimeout = 70 * time.Second

// osExit is a variable to allow mocking os.Exit in tests
var osExit = os.Exit

// For testing purposes - allows us to mock the signal channel
var signalNotify = signal.Notify

// NewAppFunc defines the function signature for creating a new app
type NewAppFunc func(cfg *config.Config, opts ...app.AppOption) app.AppInterface

// newApp creates the app runServer runs; tests replace it.
var newApp NewAppFunc = app.NewApp

// shutdownWaiter is what runServer needs to wait for a shutdown it did not start: the app
// closing its own server to load a new configuration (setup, system settings).
type shutdownWaiter interface {
	ShutdownDone() <-chan struct{}
}

// shutdownTimeout is the bound runServer applies: gracefulShutdownTimeout, unless a test
// sets it shorter. Past it runServer returns, even when the app's shutdown has not ended; it leaves 5
// seconds beyond the app's own 65-second timeout.
var shutdownTimeout = gracefulShutdownTimeout

// runServer contains the core server logic, extracted for testability
func runServer(cfg *config.Config, appLogger logger.Logger) error {
	// Create app instance
	appInstance := newApp(cfg, app.WithLogger(appLogger))

	// Initialize all components
	if err := appInstance.Initialize(); err != nil {
		appLogger.WithField("error", err.Error()).Fatal(err.Error())
		return err
	}

	// Set up graceful shutdown - single channel for all signals
	shutdown := make(chan os.Signal, 1)
	signalNotify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Start server in a goroutine
	serverError := make(chan error, 1)
	go func() {
		appLogger.Info("Server started successfully")
		serverError <- appInstance.Start()
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-serverError:
		if errors.Is(err, http.ErrServerClosed) {
			// The app closed its own server to load a new configuration (setup, system settings):
			// Start returns as soon as its Shutdown reaches the HTTP server, which then still waits
			// for the requests in flight and closes the databases. Exit once it has ended; the
			// handlers bound that shutdown under shutdownTimeout. A signal meanwhile exits at once.
			appLogger.Info("Server stopped to load its new configuration - waiting for its shutdown to end")
			if waiter, ok := appInstance.(shutdownWaiter); ok {
				select {
				case <-waiter.ShutdownDone():
				case sig := <-shutdown:
					appLogger.WithField("signal", sig.String()).Warn("Signal received while the shutdown for the new configuration runs - exiting now")
				case <-time.After(shutdownTimeout):
					appLogger.WithField("timeout", shutdownTimeout.String()).Warn("Shutdown for the new configuration did not end in time - exiting")
				}
			}
			// The error still ends the process with a failure code, so that a process manager
			// restarting only on failure starts it again.
			appLogger.Info("Exiting so that the process manager restarts the server with its new configuration")
			return err
		}
		if err != nil {
			appLogger.WithField("error", err.Error()).Error("Server error")
		}
		return err
	case sig := <-shutdown:
		appLogger.WithField("signal", sig.String()).Info("Shutdown signal received - starting graceful shutdown")
		appLogger.Info("Send signal again (Ctrl+C) to force immediate shutdown")

		// Configure app shutdown timeout for long-running tasks (55+ seconds)
		// Set to 65 seconds to allow tasks to complete or save progress
		appInstance.SetShutdownTimeout(65 * time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		// Log current active requests
		activeRequests := appInstance.GetActiveRequestCount()
		appLogger.WithField("active_requests", activeRequests).Info("Starting graceful shutdown")

		// Create a new channel for force shutdown (after first signal received)
		forceShutdown := make(chan os.Signal, 1)
		signalNotify(forceShutdown, os.Interrupt, syscall.SIGTERM)

		// Start graceful shutdown in a goroutine
		shutdownDone := make(chan error, 1)
		go func() {
			shutdownDone <- appInstance.Shutdown(ctx)
		}()

		// Wait for either graceful shutdown completion or forced shutdown signal
		select {
		case err := <-shutdownDone:
			if err != nil {
				appLogger.WithField("error", err.Error()).Error("Error during graceful shutdown")
				return err
			}
			appLogger.Info("Server shut down gracefully")
			return nil
		case <-ctx.Done():
			appLogger.WithField("timeout", shutdownTimeout.String()).Error("Graceful shutdown did not end in time - exiting")
			return ctx.Err()
		case forceSig := <-forceShutdown:
			appLogger.WithField("signal", forceSig.String()).Warn("Force shutdown signal received - terminating immediately")
			appLogger.Warn("Some requests may be interrupted!")

			// Cancel the graceful shutdown context to force immediate shutdown
			cancel()

			// Wait a brief moment for the shutdown to acknowledge the cancellation
			select {
			case err := <-shutdownDone:
				if err != nil {
					appLogger.WithField("error", err.Error()).Error("Error during forced shutdown")
				}
			case <-time.After(2 * time.Second):
				appLogger.Warn("Forced shutdown timeout - exiting immediately")
			}

			return fmt.Errorf("forced shutdown")
		}
	}
}

func main() {
	if err := config.CheckBlockedPlatforms(); err != nil {
		log.Fatalf("%v", err)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger with configured log level
	appLogger := logger.NewLoggerWithLevel(cfg.LogLevel)
	appLogger.Info(fmt.Sprintf("Starting API server on %s:%d", cfg.Server.Host, cfg.Server.Port))

	// Run the server
	if err := runServer(cfg, appLogger); err != nil {
		osExit(1)
	}
}
