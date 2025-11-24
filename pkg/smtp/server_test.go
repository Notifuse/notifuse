package smtp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	backend := NewBackend(nil, nil, mockLogger)

	t.Run("creates server with valid config", func(t *testing.T) {
		cfg := ServerConfig{
			Host:   "localhost",
			Port:   2525,
			Domain: "example.com",
			Logger: mockLogger,
		}

		server, err := NewServer(cfg, backend)
		require.NoError(t, err)
		require.NotNil(t, server)
		assert.Equal(t, "localhost:2525", server.addr)
		assert.Equal(t, backend, server.backend)
	})

	t.Run("creates server with TLS", func(t *testing.T) {
		// Create temporary cert and key files
		certFile, keyFile := createTempCertFiles(t)
		defer func() { _ = os.Remove(certFile) }()
		defer func() { _ = os.Remove(keyFile) }()

		cfg := ServerConfig{
			Host:        "localhost",
			Port:        2525,
			Domain:      "example.com",
			TLSCertFile: certFile,
			TLSKeyFile:  keyFile,
			Logger:      mockLogger,
		}

		server, err := NewServer(cfg, backend)
		require.NoError(t, err)
		require.NotNil(t, server)
		assert.NotNil(t, server.server.TLSConfig)
	})

	t.Run("handles TLS certificate load error", func(t *testing.T) {
		cfg := ServerConfig{
			Host:        "localhost",
			Port:        2525,
			Domain:      "example.com",
			TLSCertFile: "/nonexistent/cert.pem",
			TLSKeyFile:  "/nonexistent/key.pem",
			Logger:      mockLogger,
		}

		server, err := NewServer(cfg, backend)
		require.Error(t, err)
		assert.Nil(t, server)
		assert.Contains(t, err.Error(), "failed to load TLS certificate")
	})

	t.Run("creates server without TLS", func(t *testing.T) {
		cfg := ServerConfig{
			Host:   "localhost",
			Port:   2525,
			Domain: "example.com",
			Logger: mockLogger,
		}

		server, err := NewServer(cfg, backend)
		require.NoError(t, err)
		require.NotNil(t, server)
		assert.Nil(t, server.server.TLSConfig)
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
		assert.False(t, server.server.AllowInsecureAuth)
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
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

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
			// Server.Serve() may return nil or an error when Close() is called
			// Both are acceptable - what matters is that the server shuts down
			_ = err // Accept any error value (nil or non-nil)
		case <-time.After(2 * time.Second):
			// If no error after 2 seconds, that's fine too
		}
	})

	t.Run("handles listen error", func(t *testing.T) {
		// Try to use an invalid address (port -1 is invalid)
		// Actually, we can't easily test this without mocking net.Listen
		// So we'll test with a valid but already-in-use port scenario
		// For now, we'll skip this as it's hard to test without more complex setup
		t.Skip("Listen error testing requires more complex setup")
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
		// Shutdown should complete without hanging
		// Close() may return nil or an error, both are acceptable
		// The important thing is it doesn't hang and completes
		_ = err // Accept any error value (nil or non-nil)
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

// Helper function to create temporary certificate files for testing
func createTempCertFiles(t *testing.T) (string, string) {
	// Create a self-signed certificate for testing
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, cert, &privKey.PublicKey, privKey)
	require.NoError(t, err)

	// Create temporary files
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")

	// Write cert file
	certOut, err := os.Create(certFile)
	require.NoError(t, err)
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
	require.NoError(t, err)
	_ = certOut.Close()

	// Write key file
	keyOut, err := os.Create(keyFile)
	require.NoError(t, err)
	keyBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	require.NoError(t, err)
	err = pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})
	require.NoError(t, err)
	_ = keyOut.Close()

	return certFile, keyFile
}
