package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// osExit is a variable to allow mocking os.Exit in tests
var osExit = os.Exit

// For testing purposes - allows us to mock the signal channel
var signalNotify = signal.Notify

// NewAppFunc defines the function signature for creating a new app
type NewAppFunc func(cfg *config.Config, opts ...app.AppOption) app.AppInterface

// runServer contains the core server logic, extracted for testability
func runServer(cfg *config.Config, appLogger logger.Logger) error {
	// Create app instance
	appInstance := app.NewApp(cfg, app.WithLogger(appLogger))

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

		// Create a context with timeout for graceful shutdown
		// Use 70 seconds to give 5 seconds buffer beyond app's internal timeout
		ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
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
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Log the loaded DB_PREFIX for debugging
	log.Printf("Configuration loaded - DB_PREFIX from config: '%s', DB_PREFIX from environment: '%s'", cfg.Database.Prefix, os.Getenv("DB_PREFIX"))

	// Initialize logger with configured log level
	appLogger := logger.NewLoggerWithLevel(cfg.LogLevel)
	appLogger.Info(fmt.Sprintf("Starting API server on %s:%d", cfg.Server.Host, cfg.Server.Port))

	// Run the server
	if err := runServer(cfg, appLogger); err != nil {
		osExit(1)
	}
}
