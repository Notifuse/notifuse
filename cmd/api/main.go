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

	// Set up graceful shutdown
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
		appLogger.WithField("signal", sig.String()).Info("Shutdown signal received")

		// Create a context with timeout for graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Attempt graceful shutdown
		if err := appInstance.Shutdown(ctx); err != nil {
			appLogger.WithField("error", err.Error()).Error("Error during shutdown")
			return err
		}

		appLogger.Info("Server shut down gracefully")
		return nil
	}
}

func main() {
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
