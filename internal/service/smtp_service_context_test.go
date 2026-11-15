package service

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stallingSMTPServer answers a plain SMTP exchange and stops answering at one chosen step, until
// the test ends. It tells the test when the client reaches that step.
type stallingSMTPServer struct {
	listener net.Listener
	// stallAt is the step the server stops answering at: "greeting", "rcpt", "data-end" or
	// "quit". "quit" answers the end of the message with 250 first.
	stallAt  string
	reached  chan struct{}
	accepted chan struct{}
	release  chan struct{}
	once     sync.Once
	wg       sync.WaitGroup
}

func newStallingSMTPServer(t *testing.T, stallAt string, listener net.Listener) *stallingSMTPServer {
	t.Helper()
	if listener == nil {
		var err error
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
	}
	s := &stallingSMTPServer{
		listener: listener,
		stallAt:  stallAt,
		reached:  make(chan struct{}),
		accepted: make(chan struct{}, 1),
		release:  make(chan struct{}),
	}
	s.wg.Add(1)
	go s.serve()
	t.Cleanup(func() {
		close(s.release)
		listener.Close()
		s.wg.Wait()
	})
	return s
}

func (s *stallingSMTPServer) port() int { return s.listener.Addr().(*net.TCPAddr).Port }

func (s *stallingSMTPServer) stall(conn net.Conn) {
	s.once.Do(func() { close(s.reached) })
	select {
	case <-s.release:
	case <-time.After(10 * time.Second):
	}
	conn.Close()
}

func (s *stallingSMTPServer) serve() {
	defer s.wg.Done()
	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	s.accepted <- struct{}{}
	defer conn.Close()
	if tlsConn, ok := conn.(*tls.Conn); ok {
		if err := tlsConn.Handshake(); err != nil {
			return
		}
	}
	if s.stallAt == "greeting" {
		s.stall(conn)
		return
	}
	_, _ = conn.Write([]byte("220 localhost ESMTP\r\n"))
	reader := bufio.NewReader(conn)
	inData := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if inData {
			if line != "." {
				continue
			}
			inData = false
			if s.stallAt == "data-end" {
				s.stall(conn)
				return
			}
			_, _ = conn.Write([]byte("250 OK queued\r\n"))
			continue
		}
		switch {
		case strings.HasPrefix(line, "EHLO"):
			_, _ = conn.Write([]byte("250-localhost\r\n250 OK\r\n"))
		case strings.HasPrefix(line, "MAIL FROM"):
			_, _ = conn.Write([]byte("250 OK\r\n"))
		case strings.HasPrefix(line, "RCPT TO"):
			if s.stallAt == "rcpt" {
				s.stall(conn)
				return
			}
			_, _ = conn.Write([]byte("250 OK\r\n"))
		case line == "DATA":
			_, _ = conn.Write([]byte("354 go ahead\r\n"))
			inData = true
		case line == "QUIT":
			if s.stallAt == "quit" {
				s.stall(conn)
				return
			}
			_, _ = conn.Write([]byte("221 Bye\r\n"))
			return
		default:
			_, _ = conn.Write([]byte("250 OK\r\n"))
		}
	}
}

// sendWithin runs send and fails the test if it has not returned after limit, so a send that
// ignores its context fails the test instead of hanging it.
func sendWithin(t *testing.T, limit time.Duration, send func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- send() }()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		t.Fatalf("the send did not return within %s", limit)
		return nil
	}
}

func contextTestSettings(port int, useTLS bool) *domain.SMTPSettings {
	return &domain.SMTPSettings{Host: "127.0.0.1", Port: port, UseTLS: useTLS, AuthType: "basic"}
}

var contextTestMessage = []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nTest body")

func TestSendRawEmailWithSettings_HonoursItsContext(t *testing.T) {
	// The exchange's own bounds stay long, so only the context can end these sends early.
	t.Setenv("SMTP_DIAL_TIMEOUT", "30s")

	for _, step := range []string{"greeting", "rcpt", "data-end"} {
		t.Run("a server that stops answering at "+step+" is given up at the context's deadline", func(t *testing.T) {
			server := newStallingSMTPServer(t, step, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			start := time.Now()
			err := sendWithin(t, 5*time.Second, func() error {
				return sendRawEmailWithSettings(ctx, contextTestSettings(server.port(), false), "sender@example.com",
					[]string{"recipient@example.com"}, contextTestMessage, nil)
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, context.DeadlineExceeded)
			assert.GreaterOrEqual(t, time.Since(start), 150*time.Millisecond)
		})
	}

	t.Run("cancelling the context mid-exchange ends the send at once", func(t *testing.T) {
		server := newStallingSMTPServer(t, "rcpt", nil)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			<-server.reached
			cancel()
		}()
		err := sendWithin(t, 5*time.Second, func() error {
			return sendRawEmailWithSettings(ctx, contextTestSettings(server.port(), false), "sender@example.com",
				[]string{"recipient@example.com"}, contextTestMessage, nil)
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("a context already cancelled does not connect", func(t *testing.T) {
		server := newStallingSMTPServer(t, "greeting", nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := sendWithin(t, 5*time.Second, func() error {
			return sendRawEmailWithSettings(ctx, contextTestSettings(server.port(), false), "sender@example.com",
				[]string{"recipient@example.com"}, contextTestMessage, nil)
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		select {
		case <-server.accepted:
			t.Fatal("the send connected with a cancelled context")
		case <-time.After(100 * time.Millisecond):
		}
	})

	t.Run("a cancel after the server accepted the message still reports it sent", func(t *testing.T) {
		server := newStallingSMTPServer(t, "quit", nil)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			<-server.reached
			cancel()
		}()
		err := sendWithin(t, 5*time.Second, func() error {
			return sendRawEmailWithSettings(ctx, contextTestSettings(server.port(), false), "sender@example.com",
				[]string{"recipient@example.com"}, contextTestMessage, nil)
		})
		assert.NoError(t, err, "the 250 after the message is what makes it sent")
	})

	t.Run("a server that stops answering after an implicit TLS handshake is given up at the context's deadline", func(t *testing.T) {
		cert, pool := generateTestCert(t)
		trustTestCert(t, pool)
		inner, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		listener := tls.NewListener(inner, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
		server := newStallingSMTPServer(t, "greeting", listener)
		overrideImplicitTLSPort(t, server.port())

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		err = sendWithin(t, 5*time.Second, func() error {
			return sendRawEmailWithSettings(ctx, contextTestSettings(server.port(), true), "sender@example.com",
				[]string{"recipient@example.com"}, contextTestMessage, nil)
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func TestOAuth2TokenService_GetAccessToken_HonoursItsContext(t *testing.T) {
	release := make(chan struct{})
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer tokenServer.Close()
	defer close(release)

	svc := NewOAuth2TokenService(&noopLogger{})
	svc.SetTokenURLs(tokenServer.URL, tokenServer.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := sendWithin(t, 5*time.Second, func() error {
		_, err := svc.GetAccessToken(ctx, &domain.SMTPSettings{
			OAuth2Provider: "google", OAuth2ClientID: "id", OAuth2ClientSecret: "secret", OAuth2RefreshToken: "refresh",
		})
		return err
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "got %v", err)
}

// The SMTP send hands its context to the token provider.
func TestSendRawEmailWithSettings_PassesItsContextToTheTokenProvider(t *testing.T) {
	type ctxKey struct{}
	provider := &mockOAuth2TokenService{err: errors.New("no token")}
	server := newStallingSMTPServer(t, "quit", nil)
	ctx := context.WithValue(context.Background(), ctxKey{}, "send")
	settings := contextTestSettings(server.port(), false)
	settings.AuthType = "oauth2"
	err := sendWithin(t, 5*time.Second, func() error {
		return sendRawEmailWithSettings(ctx, settings, "sender@example.com", []string{"recipient@example.com"}, contextTestMessage, provider)
	})
	require.Error(t, err)
	require.NotNil(t, provider.lastCtx)
	assert.Equal(t, "send", provider.lastCtx.Value(ctxKey{}))
}
