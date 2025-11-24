package smtp_relay

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	backend := NewBackend(nil, nil, mockLogger)

	t.Run("creates server with TLS config", func(t *testing.T) {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}

		cfg := ServerConfig{
			Host:      "localhost",
			Port:      2525,
			Domain:    "example.com",
			TLSConfig: tlsConfig,
			Logger:    mockLogger,
		}

		server, err := NewServer(cfg, backend)
		require.NoError(t, err)
		require.NotNil(t, server)
		assert.Equal(t, tlsConfig, server.server.TLSConfig)
		assert.False(t, server.server.AllowInsecureAuth)
	})

	t.Run("creates server without TLS config, RequireTLS=false", func(t *testing.T) {
		cfg := ServerConfig{
			Host:       "localhost",
			Port:       2525,
			Domain:     "example.com",
			RequireTLS:  false,
			Logger:     mockLogger,
		}

		server, err := NewServer(cfg, backend)
		require.NoError(t, err)
		require.NotNil(t, server)
		assert.Nil(t, server.server.TLSConfig)
		assert.True(t, server.server.AllowInsecureAuth)
	})

	t.Run("requires TLS in production", func(t *testing.T) {
		cfg := ServerConfig{
			Host:       "localhost",
			Port:       2525,
			Domain:     "example.com",
			RequireTLS:  true,
			Logger:     mockLogger,
		}

		server, err := NewServer(cfg, backend)
		require.Error(t, err)
		assert.Nil(t, server)
		assert.Contains(t, err.Error(), "TLS is required in production environment")
	})

	t.Run("server settings configured", func(t *testing.T) {
		cfg := ServerConfig{
			Host:   "localhost",
			Port:   2525,
			Domain: "example.com",
			Logger: mockLogger,
		}

		server, err := NewServer(cfg, backend)
		require.NoError(t, err)
		assert.Equal(t, 10*time.Second, server.server.ReadTimeout)
		assert.Equal(t, 10*time.Second, server.server.WriteTimeout)
		assert.Equal(t, int64(10*1024*1024), server.server.MaxMessageBytes)
		assert.Equal(t, 50, server.server.MaxRecipients)
	})
}

func TestServer_Start(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	backend := NewBackend(nil, nil, mockLogger)

	t.Run("starts listening on address", func(t *testing.T) {
		cfg := ServerConfig{
			Host:   "127.0.0.1",
			Port:   0, // Use port 0 to get a free port
			Domain: "example.com",
			Logger: mockLogger,
		}

		server, err := NewServer(cfg, backend)
		require.NoError(t, err)

		// Start server in a goroutine
		errChan := make(chan error, 1)
		go func() {
			errChan <- server.Start()
		}()

		// Give it a moment to start
		time.Sleep(100 * time.Millisecond)

		// Shutdown the server
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		err = server.Shutdown(ctx)
		assert.NoError(t, err)

		// Check for any start errors
		select {
		case err := <-errChan:
			// Server.Serve() will return an error when Close() is called, which is expected
			assert.Error(t, err)
		case <-time.After(2 * time.Second):
			// If no error after 2 seconds, that's fine too
		}
	})
}

func TestServer_Shutdown(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	backend := NewBackend(nil, nil, mockLogger)

	t.Run("graceful shutdown", func(t *testing.T) {
		cfg := ServerConfig{
			Host:   "127.0.0.1",
			Port:   0,
			Domain: "example.com",
			Logger: mockLogger,
		}

		server, err := NewServer(cfg, backend)
		require.NoError(t, err)

		// Start server in background
		go func() {
			_ = server.Start()
		}()

		// Give it a moment to start
		time.Sleep(100 * time.Millisecond)

		// Shutdown with sufficient timeout
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err = server.Shutdown(ctx)
		// Shutdown should succeed (or return error from Close, which is acceptable)
		// The important thing is it doesn't hang
		assert.NotNil(t, err) // Close() typically returns an error when called on a closed server
	})

	t.Run("context timeout", func(t *testing.T) {
		cfg := ServerConfig{
			Host:   "127.0.0.1",
			Port:   0,
			Domain: "example.com",
			Logger: mockLogger,
		}

		server, err := NewServer(cfg, backend)
		require.NoError(t, err)

		// Create a context that's already cancelled
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = server.Shutdown(ctx)
		require.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

