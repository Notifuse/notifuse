package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestCert creates a self-signed certificate valid for 127.0.0.1 and
// localhost, returning it plus a pool that trusts it.
func generateTestCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

// overrideImplicitTLSPort points SMTPS detection at an ephemeral test port for
// the duration of the test (tests cannot bind the real port 465).
func overrideImplicitTLSPort(t *testing.T, port int) {
	t.Helper()
	prev := implicitTLSPort
	implicitTLSPort = port
	t.Cleanup(func() { implicitTLSPort = prev })
}

// trustTestCert makes the SMTP client trust the given certificate pool for the
// duration of the test, so a real handshake against a self-signed mock passes
// verification.
func trustTestCert(t *testing.T, pool *x509.CertPool) {
	t.Helper()
	prev := smtpTLSConfig
	smtpTLSConfig = func(host string) *tls.Config {
		return &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
			RootCAs:    pool,
		}
	}
	t.Cleanup(func() { smtpTLSConfig = prev })
}

// newMockSMTPSServer starts a mock SMTP server behind a TLS listener (implicit
// TLS / SMTPS): the TLS handshake happens before any SMTP exchange. It returns
// the server and a certificate pool trusting its self-signed certificate.
func newMockSMTPSServer(t *testing.T, authSuccess bool, authMode string, multilineBanner bool) (*mockSMTPServer, *x509.CertPool) {
	t.Helper()
	cert, pool := generateTestCert(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &mockSMTPServer{
		listener:        tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{cert}}),
		authSuccess:     authSuccess,
		authMode:        authMode,
		multilineBanner: multilineBanner,
		commands:        make([]string, 0),
		messages:        make([]capturedMessage, 0),
	}
	server.wg.Add(1)
	go server.serve()
	return server, pool
}

// newMockSMTPServerWithSTARTTLS starts a plaintext mock SMTP server that
// upgrades the connection to TLS when the client issues STARTTLS.
func newMockSMTPServerWithSTARTTLS(t *testing.T, authSuccess bool) (*mockSMTPServer, *x509.CertPool) {
	t.Helper()
	cert, pool := generateTestCert(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &mockSMTPServer{
		listener:    listener,
		authSuccess: authSuccess,
		tlsCert:     &cert,
		commands:    make([]string, 0),
		messages:    make([]capturedMessage, 0),
	}
	server.wg.Add(1)
	go server.serve()
	return server, pool
}

func TestSendRawEmail_ImplicitTLS_Success(t *testing.T) {
	server, pool := newMockSMTPSServer(t, true, "", false)
	defer server.Close()
	overrideImplicitTLSPort(t, server.Port())
	trustTestCert(t, pool)

	msg := []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nTest body")
	err := sendRawEmail("127.0.0.1", server.Port(), "user", "pass", true, "sender@example.com", []string{"recipient@example.com"}, msg)
	require.NoError(t, err)

	messages := server.GetMessages()
	require.Len(t, messages, 1)
	assert.Equal(t, "sender@example.com", messages[0].from)

	// The connection is encrypted from the first byte: STARTTLS must not be
	// issued, and a single EHLO (over TLS) negotiates the AUTH mechanisms.
	ehloCount := 0
	for _, cmd := range server.GetCommands() {
		upper := strings.ToUpper(cmd)
		assert.NotEqual(t, "STARTTLS", upper, "STARTTLS must not be sent on an implicit TLS connection")
		if strings.HasPrefix(upper, "EHLO") {
			ehloCount++
		}
	}
	assert.Equal(t, 1, ehloCount, "implicit TLS needs a single EHLO")
}

func TestSendRawEmail_ImplicitTLS_AuthLoginOnly(t *testing.T) {
	// Servers that advertise only AUTH LOGIN (e.g. Azure Communication
	// Services) must still be negotiated correctly when the EHLO happens over
	// an implicit TLS connection.
	server, pool := newMockSMTPSServer(t, true, "login_only", false)
	defer server.Close()
	overrideImplicitTLSPort(t, server.Port())
	trustTestCert(t, pool)

	msg := []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nTest body")
	err := sendRawEmail("127.0.0.1", server.Port(), "azureuser", "azurepass", true, "sender@example.com", []string{"recipient@example.com"}, msg)
	require.NoError(t, err)

	user, pass := server.GetLoginCredentials()
	assert.Equal(t, "azureuser", user)
	assert.Equal(t, "azurepass", pass)
}

func TestSendRawEmail_ImplicitTLS_MultilineBanner(t *testing.T) {
	// RFC 5321 multi-line 220 banners must still be read correctly when the
	// greeting arrives over an implicit TLS connection.
	server, pool := newMockSMTPSServer(t, true, "", true)
	defer server.Close()
	overrideImplicitTLSPort(t, server.Port())
	trustTestCert(t, pool)

	msg := []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nTest body")
	err := sendRawEmail("127.0.0.1", server.Port(), "user", "pass", true, "sender@example.com", []string{"recipient@example.com"}, msg)
	require.NoError(t, err)
	require.Len(t, server.GetMessages(), 1)
}

func TestSendRawEmail_ImplicitTLS_HandshakeFailure(t *testing.T) {
	// A plaintext server on the implicit TLS port sends its banner
	// immediately; the handshake must fail fast instead of hanging.
	server := newMockSMTPServer(t, true)
	defer server.Close()
	overrideImplicitTLSPort(t, server.Port())

	msg := []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nTest body")
	err := sendRawEmail("127.0.0.1", server.Port(), "", "", true, "sender@example.com", []string{"recipient@example.com"}, msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "implicit TLS handshake failed")
}

func TestSendRawEmail_ImplicitTLS_SilentServer(t *testing.T) {
	// A server that accepts the connection but never speaks must not block
	// the sender forever: the handshake is bounded by the dial timeout.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			c.Close()
		}
	})

	port := listener.Addr().(*net.TCPAddr).Port
	overrideImplicitTLSPort(t, port)
	t.Setenv("SMTP_DIAL_TIMEOUT", "500ms")

	start := time.Now()
	msg := []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nTest body")
	err = sendRawEmail("127.0.0.1", port, "", "", true, "sender@example.com", []string{"recipient@example.com"}, msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "implicit TLS handshake failed")
	assert.Less(t, time.Since(start), 5*time.Second, "handshake against a silent server must time out quickly")
}

func TestSendRawEmail_ImplicitTLSPort_PlaintextWhenTLSDisabled(t *testing.T) {
	// Explicitly disabling TLS keeps the connection plaintext even on the
	// SMTPS port.
	server := newMockSMTPServer(t, true)
	defer server.Close()
	overrideImplicitTLSPort(t, server.Port())

	msg := []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nTest body")
	err := sendRawEmail("127.0.0.1", server.Port(), "user", "pass", false, "sender@example.com", []string{"recipient@example.com"}, msg)
	require.NoError(t, err)
	require.Len(t, server.GetMessages(), 1)

	for _, cmd := range server.GetCommands() {
		assert.NotEqual(t, "STARTTLS", strings.ToUpper(cmd))
	}
}

func TestSendRawEmail_STARTTLS_Success(t *testing.T) {
	// Real-handshake coverage of the STARTTLS path: plaintext greeting and
	// EHLO, upgrade, then a second EHLO whose AUTH capabilities supersede the
	// pre-TLS set.
	server, pool := newMockSMTPServerWithSTARTTLS(t, true)
	defer server.Close()
	trustTestCert(t, pool)

	msg := []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nTest body")
	err := sendRawEmail("127.0.0.1", server.Port(), "user", "pass", true, "sender@example.com", []string{"recipient@example.com"}, msg)
	require.NoError(t, err)
	require.Len(t, server.GetMessages(), 1)

	commands := server.GetCommands()
	starttlsIdx := indexOfCmd(commands, "STARTTLS")
	require.GreaterOrEqual(t, starttlsIdx, 0, "client must issue STARTTLS")

	ehloCount := 0
	lastEhloIdx := -1
	for i, cmd := range commands {
		if strings.HasPrefix(strings.ToUpper(cmd), "EHLO") {
			ehloCount++
			lastEhloIdx = i
		}
	}
	assert.Equal(t, 2, ehloCount, "EHLO must be sent before and after the TLS upgrade")
	assert.Greater(t, lastEhloIdx, starttlsIdx, "second EHLO must happen after STARTTLS")
}

func TestSetupService_TestSMTPConnection_ImplicitTLS(t *testing.T) {
	// The connection test must attempt a TLS-first handshake on the SMTPS
	// port: against a self-signed server it fails on certificate verification
	// (fast) instead of waiting for a plaintext greeting until the deadline.
	server, _ := newMockSMTPSServer(t, true, "", false)
	defer server.Close()
	overrideImplicitTLSPort(t, server.Port())

	setupService := NewSetupService(
		&SettingService{},
		&UserService{},
		nil,
		&noopLogger{},
		"test-secret-key",
		nil,
		nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := setupService.TestSMTPConnection(ctx, &SMTPTestConfig{
		Host:   "127.0.0.1",
		Port:   server.Port(),
		UseTLS: true,
	})
	require.Error(t, err)
	assert.NotErrorIs(t, err, context.DeadlineExceeded, "must fail on the handshake, not hang until the deadline")
	// Loose match: darwin's platform verifier says "certificate is not
	// trusted" while the pure-Go verifier says "certificate signed by unknown
	// authority".
	assert.Contains(t, strings.ToLower(err.Error()), "certificat")
}
