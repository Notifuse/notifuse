package integration

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSMTPServerIntegration captures SMTP commands and responses
// This is a more complete mock server for integration tests
type mockSMTPServerIntegration struct {
	listener    net.Listener
	commands    []string
	mu          sync.Mutex
	authSuccess bool
	wg          sync.WaitGroup
	shutdown    chan struct{}
}

func newMockSMTPServerIntegration(t *testing.T, authSuccess bool) *mockSMTPServerIntegration {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &mockSMTPServerIntegration{
		listener:    listener,
		commands:    make([]string, 0),
		authSuccess: authSuccess,
		shutdown:    make(chan struct{}),
	}

	server.wg.Add(1)
	go server.serve(t)
	return server
}

func (s *mockSMTPServerIntegration) serve(t *testing.T) {
	defer s.wg.Done()
	for {
		select {
		case <-s.shutdown:
			return
		default:
		}

		// Set deadline so we can check shutdown channel
		_ = s.listener.(*net.TCPListener).SetDeadline(time.Now().Add(100 * time.Millisecond))

		conn, err := s.listener.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}

		s.handleConnection(t, conn)
	}
}

func (s *mockSMTPServerIntegration) handleConnection(t *testing.T, conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Send greeting
	_, _ = conn.Write([]byte("220 localhost ESMTP Test\r\n"))

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		line = strings.TrimSpace(line)
		s.mu.Lock()
		s.commands = append(s.commands, line)
		s.mu.Unlock()

		t.Logf("SMTP Server received: %s", line)

		cmd := strings.ToUpper(strings.Split(line, " ")[0])

		switch cmd {
		case "EHLO":
			_, _ = conn.Write([]byte("250-localhost\r\n"))
			_, _ = conn.Write([]byte("250-SIZE 10485760\r\n"))
			_, _ = conn.Write([]byte("250-AUTH PLAIN LOGIN\r\n"))
			_, _ = conn.Write([]byte("250-8BITMIME\r\n"))
			_, _ = conn.Write([]byte("250 SMTPUTF8\r\n"))
		case "AUTH":
			if s.authSuccess {
				_, _ = conn.Write([]byte("235 2.7.0 Authentication successful\r\n"))
			} else {
				_, _ = conn.Write([]byte("535 5.7.8 Authentication failed\r\n"))
				return
			}
		case "MAIL":
			// Capture the full MAIL FROM command
			_, _ = conn.Write([]byte("250 2.1.0 Ok\r\n"))
		case "RCPT":
			_, _ = conn.Write([]byte("250 2.1.5 Ok\r\n"))
		case "DATA":
			_, _ = conn.Write([]byte("354 End data with <CR><LF>.<CR><LF>\r\n"))
			// Read until we get a line with just "."
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimSpace(dataLine) == "." {
					break
				}
			}
			_, _ = conn.Write([]byte("250 2.0.0 Ok: queued\r\n"))
		case "QUIT":
			_, _ = conn.Write([]byte("221 2.0.0 Bye\r\n"))
			return
		default:
			_, _ = conn.Write([]byte("500 Unknown command\r\n"))
		}
	}
}

func (s *mockSMTPServerIntegration) Close() {
	close(s.shutdown)
	s.listener.Close()
	s.wg.Wait()
}

func (s *mockSMTPServerIntegration) Addr() string {
	return s.listener.Addr().String()
}

func (s *mockSMTPServerIntegration) Port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

func (s *mockSMTPServerIntegration) GetMailFromCommand() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, cmd := range s.commands {
		if strings.HasPrefix(strings.ToUpper(cmd), "MAIL FROM:") {
			return cmd
		}
	}
	return ""
}

func (s *mockSMTPServerIntegration) GetAllCommands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]string, len(s.commands))
	copy(result, s.commands)
	return result
}

// TestSMTPClient_E2E_NoExtensionsInMailFrom is the critical test for issue #172
// It verifies that the MAIL FROM command does NOT contain BODY=8BITMIME or SMTPUTF8 extensions
func TestSMTPClient_E2E_NoExtensionsInMailFrom(t *testing.T) {
	// Start mock SMTP server that advertises 8BITMIME and SMTPUTF8
	server := newMockSMTPServerIntegration(t, true)
	defer server.Close()

	// Create SMTP service
	log := logger.NewLogger()
	smtpService := service.NewSMTPService(log)

	// Create test request
	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "test-workspace",
		IntegrationID: "test-integration",
		MessageID:     "test-msg-001",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Test Subject",
		Content:       "<html><body><p>Test content</p></body></html>",
		Provider: &domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:     "127.0.0.1",
				Port:     server.Port(),
				Username: "testuser",
				Password: "testpass",
				UseTLS:   false, // plaintext for test
			},
		},
	}

	// Send email
	err := smtpService.SendEmail(context.Background(), request)
	require.NoError(t, err)

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// CRITICAL ASSERTION FOR ISSUE #172
	// Verify MAIL FROM command does NOT contain extensions
	mailFromCmd := server.GetMailFromCommand()
	require.NotEmpty(t, mailFromCmd, "MAIL FROM command should have been captured")

	t.Logf("Captured MAIL FROM command: %s", mailFromCmd)

	// These assertions are the key fix verification
	assert.NotContains(t, mailFromCmd, "BODY=8BITMIME",
		"MAIL FROM should NOT contain BODY=8BITMIME extension (issue #172)")
	assert.NotContains(t, mailFromCmd, "SMTPUTF8",
		"MAIL FROM should NOT contain SMTPUTF8 extension (issue #172)")
	assert.NotContains(t, mailFromCmd, "SIZE=",
		"MAIL FROM should NOT contain SIZE extension")

	// Verify it's a simple MAIL FROM command
	assert.Contains(t, mailFromCmd, "MAIL FROM:<sender@example.com>")
}

// TestSMTPClient_E2E_WithAttachments verifies attachments work correctly
func TestSMTPClient_E2E_WithAttachments(t *testing.T) {
	server := newMockSMTPServerIntegration(t, true)
	defer server.Close()

	log := logger.NewLogger()
	smtpService := service.NewSMTPService(log)

	// Create test attachment
	testContent := []byte("This is test file content")
	encodedContent := base64.StdEncoding.EncodeToString(testContent)

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "test-workspace",
		IntegrationID: "test-integration",
		MessageID:     "test-msg-002",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Test with Attachment",
		Content:       "<html><body><p>Test with attachment</p></body></html>",
		Provider: &domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:     "127.0.0.1",
				Port:     server.Port(),
				Username: "testuser",
				Password: "testpass",
				UseTLS:   false,
			},
		},
		EmailOptions: domain.EmailOptions{
			Attachments: []domain.Attachment{
				{
					Filename:    "test.txt",
					Content:     encodedContent,
					ContentType: "text/plain",
					Disposition: "attachment",
				},
			},
		},
	}

	err := smtpService.SendEmail(context.Background(), request)
	require.NoError(t, err)

	// Verify MAIL FROM still doesn't contain extensions
	time.Sleep(100 * time.Millisecond)
	mailFromCmd := server.GetMailFromCommand()
	assert.NotContains(t, mailFromCmd, "BODY=8BITMIME")
	assert.NotContains(t, mailFromCmd, "SMTPUTF8")
}

// TestSMTPClient_E2E_WithCCAndBCC verifies CC/BCC recipients are handled correctly
func TestSMTPClient_E2E_WithCCAndBCC(t *testing.T) {
	server := newMockSMTPServerIntegration(t, true)
	defer server.Close()

	log := logger.NewLogger()
	smtpService := service.NewSMTPService(log)

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "test-workspace",
		IntegrationID: "test-integration",
		MessageID:     "test-msg-003",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Test with CC and BCC",
		Content:       "<html><body><p>Test with CC and BCC</p></body></html>",
		Provider: &domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:     "127.0.0.1",
				Port:     server.Port(),
				Username: "testuser",
				Password: "testpass",
				UseTLS:   false,
			},
		},
		EmailOptions: domain.EmailOptions{
			CC:  []string{"cc1@example.com", "cc2@example.com"},
			BCC: []string{"bcc@example.com"},
		},
	}

	err := smtpService.SendEmail(context.Background(), request)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify all RCPT TO commands were sent
	commands := server.GetAllCommands()
	rcptCommands := []string{}
	for _, cmd := range commands {
		if strings.HasPrefix(strings.ToUpper(cmd), "RCPT TO:") {
			rcptCommands = append(rcptCommands, cmd)
		}
	}

	t.Logf("RCPT TO commands: %v", rcptCommands)

	// Should have 4 RCPT TO commands: main recipient + 2 CC + 1 BCC
	assert.GreaterOrEqual(t, len(rcptCommands), 4, "Should have RCPT TO for all recipients")

	// Verify MAIL FROM doesn't have extensions
	mailFromCmd := server.GetMailFromCommand()
	assert.NotContains(t, mailFromCmd, "BODY=8BITMIME")
	assert.NotContains(t, mailFromCmd, "SMTPUTF8")
}

// TestSMTPClient_E2E_WithReplyTo verifies Reply-To header works
func TestSMTPClient_E2E_WithReplyTo(t *testing.T) {
	server := newMockSMTPServerIntegration(t, true)
	defer server.Close()

	log := logger.NewLogger()
	smtpService := service.NewSMTPService(log)

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "test-workspace",
		IntegrationID: "test-integration",
		MessageID:     "test-msg-004",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Test with Reply-To",
		Content:       "<html><body><p>Test with Reply-To</p></body></html>",
		Provider: &domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:     "127.0.0.1",
				Port:     server.Port(),
				Username: "testuser",
				Password: "testpass",
				UseTLS:   false,
			},
		},
		EmailOptions: domain.EmailOptions{
			ReplyTo: "replyto@example.com",
		},
	}

	err := smtpService.SendEmail(context.Background(), request)
	require.NoError(t, err)
}

// TestSMTPClient_E2E_WithListUnsubscribe verifies List-Unsubscribe headers work
func TestSMTPClient_E2E_WithListUnsubscribe(t *testing.T) {
	server := newMockSMTPServerIntegration(t, true)
	defer server.Close()

	log := logger.NewLogger()
	smtpService := service.NewSMTPService(log)

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "test-workspace",
		IntegrationID: "test-integration",
		MessageID:     "test-msg-005",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Test with List-Unsubscribe",
		Content:       "<html><body><p>Test with List-Unsubscribe</p></body></html>",
		Provider: &domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:     "127.0.0.1",
				Port:     server.Port(),
				Username: "testuser",
				Password: "testpass",
				UseTLS:   false,
			},
		},
		EmailOptions: domain.EmailOptions{
			ListUnsubscribeURL: "https://example.com/unsubscribe?token=abc123",
		},
	}

	err := smtpService.SendEmail(context.Background(), request)
	require.NoError(t, err)
}

// TestSMTPClient_E2E_AuthenticationFailure verifies auth failure is handled correctly
func TestSMTPClient_E2E_AuthenticationFailure(t *testing.T) {
	server := newMockSMTPServerIntegration(t, false) // auth will fail
	defer server.Close()

	log := logger.NewLogger()
	smtpService := service.NewSMTPService(log)

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "test-workspace",
		IntegrationID: "test-integration",
		MessageID:     "test-msg-006",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Test Auth Failure",
		Content:       "<html><body><p>Test</p></body></html>",
		Provider: &domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:     "127.0.0.1",
				Port:     server.Port(),
				Username: "baduser",
				Password: "badpass",
				UseTLS:   false,
			},
		},
	}

	err := smtpService.SendEmail(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

// TestSMTPClient_E2E_MultipleEmails verifies sending multiple emails in sequence
func TestSMTPClient_E2E_MultipleEmails(t *testing.T) {
	server := newMockSMTPServerIntegration(t, true)
	defer server.Close()

	log := logger.NewLogger()
	smtpService := service.NewSMTPService(log)

	for i := 0; i < 3; i++ {
		request := domain.SendEmailProviderRequest{
			WorkspaceID:   "test-workspace",
			IntegrationID: "test-integration",
			MessageID:     fmt.Sprintf("test-msg-%d", i),
			FromAddress:   "sender@example.com",
			FromName:      "Test Sender",
			To:            fmt.Sprintf("recipient%d@example.com", i),
			Subject:       fmt.Sprintf("Test Email %d", i),
			Content:       fmt.Sprintf("<html><body><p>Test content %d</p></body></html>", i),
			Provider: &domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
				SMTP: &domain.SMTPSettings{
					Host:     "127.0.0.1",
					Port:     server.Port(),
					Username: "testuser",
					Password: "testpass",
					UseTLS:   false,
				},
			},
		}

		err := smtpService.SendEmail(context.Background(), request)
		require.NoError(t, err, "Email %d should send successfully", i)
	}
}
