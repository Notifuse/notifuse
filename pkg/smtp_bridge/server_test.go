package smtp_bridge

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateSelfSignedTLSConfig returns a *tls.Config with a self-signed cert
// for localhost — sufficient for tests that exercise TLS plumbing.
func generateSelfSignedTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	require.NoError(t, err)

	keyBytes, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
}

func TestNewServer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	backend := NewBackend(nil, nil, mockLogger)

	t.Run("mode_off", func(t *testing.T) {
		cfg := ServerConfig{
			Host:   "localhost",
			Port:   2525,
			Domain: "example.com",
			Mode:   ModeOff,
			Logger: mockLogger,
		}
		server, err := NewServer(cfg, backend)
		require.NoError(t, err)
		require.NotNil(t, server)
		assert.Nil(t, server.server.TLSConfig)
		assert.True(t, server.server.AllowInsecureAuth)
		assert.Nil(t, server.tlsListenerConfig)
	})

	t.Run("mode_starttls_with_cert", func(t *testing.T) {
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
		cfg := ServerConfig{
			Host:      "localhost",
			Port:      2525,
			Domain:    "example.com",
			Mode:      ModeSTARTTLS,
			TLSConfig: tlsCfg,
			Logger:    mockLogger,
		}
		server, err := NewServer(cfg, backend)
		require.NoError(t, err)
		assert.Equal(t, tlsCfg, server.server.TLSConfig)
		assert.False(t, server.server.AllowInsecureAuth)
		assert.Nil(t, server.tlsListenerConfig)
	})

	t.Run("mode_starttls_without_cert", func(t *testing.T) {
		cfg := ServerConfig{
			Host:   "localhost",
			Port:   2525,
			Domain: "example.com",
			Mode:   ModeSTARTTLS,
			Logger: mockLogger,
		}
		server, err := NewServer(cfg, backend)
		require.Error(t, err)
		assert.Nil(t, server)
		assert.Contains(t, err.Error(), "TLSConfig is required")
	})

	t.Run("mode_implicit_with_cert", func(t *testing.T) {
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
		cfg := ServerConfig{
			Host:      "localhost",
			Port:      2525,
			Domain:    "example.com",
			Mode:      ModeImplicit,
			TLSConfig: tlsCfg,
			Logger:    mockLogger,
		}
		server, err := NewServer(cfg, backend)
		require.NoError(t, err)
		assert.Nil(t, server.server.TLSConfig, "go-smtp TLSConfig should be nil so STARTTLS is not advertised")
		assert.False(t, server.server.AllowInsecureAuth)
		assert.Equal(t, tlsCfg, server.tlsListenerConfig)
	})

	t.Run("mode_implicit_without_cert", func(t *testing.T) {
		cfg := ServerConfig{
			Host:   "localhost",
			Port:   2525,
			Domain: "example.com",
			Mode:   ModeImplicit,
			Logger: mockLogger,
		}
		server, err := NewServer(cfg, backend)
		require.Error(t, err)
		assert.Nil(t, server)
	})

	t.Run("mode_unknown", func(t *testing.T) {
		cfg := ServerConfig{
			Host:   "localhost",
			Port:   2525,
			Domain: "example.com",
			Mode:   "bogus",
			Logger: mockLogger,
		}
		server, err := NewServer(cfg, backend)
		require.Error(t, err)
		assert.Nil(t, server)
	})

	t.Run("server settings configured", func(t *testing.T) {
		cfg := ServerConfig{
			Host:   "localhost",
			Port:   2525,
			Domain: "example.com",
			Mode:   ModeOff,
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
			Port:   0, // ephemeral
			Domain: "example.com",
			Mode:   ModeOff,
			Logger: mockLogger,
		}

		server, err := NewServer(cfg, backend)
		require.NoError(t, err)

		errChan := make(chan error, 1)
		go func() {
			errChan <- server.Start()
		}()

		time.Sleep(100 * time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)

		select {
		case <-errChan:
		case <-time.After(2 * time.Second):
		}
	})
}

func TestServer_Start_Implicit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	backend := NewBackend(nil, nil, mockLogger)
	tlsCfg := generateSelfSignedTLSConfig(t)

	// Listen on an ephemeral port via a throwaway listener to grab a free port
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := probe.Addr().(*net.TCPAddr).Port
	_ = probe.Close()

	cfg := ServerConfig{
		Host:      "127.0.0.1",
		Port:      port,
		Domain:    "localhost",
		Mode:      ModeImplicit,
		TLSConfig: tlsCfg,
		Logger:    mockLogger,
	}

	server, err := NewServer(cfg, backend)
	require.NoError(t, err)

	go func() { _ = server.Start() }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	// Give the listener a moment to bind
	time.Sleep(150 * time.Millisecond)

	// Dial with TLS directly — this is the hallmark of implicit TLS (SMTPS).
	conn, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port), &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "localhost",
	})
	require.NoError(t, err, "tls.Dial should succeed on implicit listener")
	defer conn.Close()

	// Read the SMTP banner
	buf := make([]byte, 512)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	n, err := conn.Read(buf)
	require.NoError(t, err)
	banner := string(buf[:n])
	assert.True(t, strings.HasPrefix(banner, "220 "), "expected 220 banner, got %q", banner)

	// Send EHLO and read the reply; must not advertise STARTTLS.
	_, err = conn.Write([]byte("EHLO test.local\r\n"))
	require.NoError(t, err)

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	n, err = conn.Read(buf)
	require.NoError(t, err)
	reply := string(buf[:n])
	assert.Contains(t, reply, "250", "EHLO reply should contain 250")
	assert.NotContains(t, reply, "STARTTLS", "implicit mode must not advertise STARTTLS")
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
			Mode:   ModeOff,
			Logger: mockLogger,
		}

		server, err := NewServer(cfg, backend)
		require.NoError(t, err)

		go func() { _ = server.Start() }()
		time.Sleep(100 * time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	t.Run("context timeout", func(t *testing.T) {
		cfg := ServerConfig{
			Host:   "127.0.0.1",
			Port:   0,
			Domain: "example.com",
			Mode:   ModeOff,
			Logger: mockLogger,
		}

		server, err := NewServer(cfg, backend)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = server.Shutdown(ctx)
		require.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}
