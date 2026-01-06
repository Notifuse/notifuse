package service

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	pkglogger "github.com/Notifuse/notifuse/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSMTPServer is a test SMTP server that captures commands and messages
type mockSMTPServer struct {
	listener        net.Listener
	mu              sync.Mutex
	commands        []string
	messages        []capturedMessage
	authSuccess     bool
	closed          bool
	wg              sync.WaitGroup
	mailFromCmd     string // captures the exact MAIL FROM command
	multilineBanner bool   // send multi-line 220 banner (RFC 5321 compliant)
}

type capturedMessage struct {
	from       string
	recipients []string
	data       []byte
}

func newMockSMTPServer(t *testing.T, authSuccess bool) *mockSMTPServer {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &mockSMTPServer{
		listener:    listener,
		authSuccess: authSuccess,
		commands:    make([]string, 0),
		messages:    make([]capturedMessage, 0),
	}

	server.wg.Add(1)
	go server.serve()
	return server
}

// newMockSMTPServerWithMultilineBanner creates a mock SMTP server that sends
// a multi-line 220 greeting banner (RFC 5321 Section 4.2 compliant).
// This tests the fix for issue #183.
func newMockSMTPServerWithMultilineBanner(t *testing.T, authSuccess bool) *mockSMTPServer {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &mockSMTPServer{
		listener:        listener,
		authSuccess:     authSuccess,
		commands:        make([]string, 0),
		messages:        make([]capturedMessage, 0),
		multilineBanner: true,
	}

	server.wg.Add(1)
	go server.serve()
	return server
}

func (s *mockSMTPServer) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return
			}
			continue
		}
		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

func (s *mockSMTPServer) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Send greeting (multi-line or single-line based on configuration)
	if s.multilineBanner {
		// RFC 5321 multi-line 220 banner (issue #183)
		// Realistic example based on enterprise SMTP relays and ISP servers
		conn.Write([]byte("220-mail.example.com ESMTP Postfix\r\n"))
		conn.Write([]byte("220-Authorized use only. All activity may be monitored.\r\n"))
		conn.Write([]byte("220 Service ready\r\n"))
	} else {
		conn.Write([]byte("220 localhost SMTP Mock Server\r\n"))
	}

	var from string
	var recipients []string
	var inData bool
	var dataBuffer strings.Builder

	for {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		line = strings.TrimSpace(line)
		s.mu.Lock()
		s.commands = append(s.commands, line)
		s.mu.Unlock()

		if inData {
			if line == "." {
				inData = false
				s.mu.Lock()
				s.messages = append(s.messages, capturedMessage{
					from:       from,
					recipients: recipients,
					data:       []byte(dataBuffer.String()),
				})
				s.mu.Unlock()
				conn.Write([]byte("250 OK message queued\r\n"))
				continue
			}
			dataBuffer.WriteString(line + "\r\n")
			continue
		}

		upperLine := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upperLine, "EHLO") || strings.HasPrefix(upperLine, "HELO"):
			conn.Write([]byte("250-localhost\r\n"))
			conn.Write([]byte("250-8BITMIME\r\n")) // Advertise 8BITMIME to test that we don't use it
			conn.Write([]byte("250-SMTPUTF8\r\n")) // Advertise SMTPUTF8 to test that we don't use it
			conn.Write([]byte("250-SIZE 10485760\r\n"))
			conn.Write([]byte("250 AUTH PLAIN LOGIN\r\n"))

		case strings.HasPrefix(upperLine, "AUTH"):
			if s.authSuccess {
				conn.Write([]byte("235 Authentication successful\r\n"))
			} else {
				conn.Write([]byte("535 Authentication failed\r\n"))
			}

		case strings.HasPrefix(upperLine, "MAIL FROM:"):
			s.mu.Lock()
			s.mailFromCmd = line // Capture the exact MAIL FROM command
			s.mu.Unlock()
			// Extract email from MAIL FROM:<email>
			start := strings.Index(line, "<")
			end := strings.Index(line, ">")
			if start != -1 && end != -1 && end > start {
				from = line[start+1 : end]
			}
			conn.Write([]byte("250 OK\r\n"))

		case strings.HasPrefix(upperLine, "RCPT TO:"):
			start := strings.Index(line, "<")
			end := strings.Index(line, ">")
			if start != -1 && end != -1 && end > start {
				recipients = append(recipients, line[start+1:end])
			}
			conn.Write([]byte("250 OK\r\n"))

		case strings.HasPrefix(upperLine, "DATA"):
			inData = true
			dataBuffer.Reset()
			conn.Write([]byte("354 Start mail input\r\n"))

		case strings.HasPrefix(upperLine, "QUIT"):
			conn.Write([]byte("221 Bye\r\n"))
			return

		case strings.HasPrefix(upperLine, "RSET"):
			from = ""
			recipients = nil
			conn.Write([]byte("250 OK\r\n"))

		case strings.HasPrefix(upperLine, "NOOP"):
			conn.Write([]byte("250 OK\r\n"))

		default:
			conn.Write([]byte("500 Command not recognized\r\n"))
		}
	}
}

func (s *mockSMTPServer) Port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

func (s *mockSMTPServer) Addr() string {
	return s.listener.Addr().String()
}

func (s *mockSMTPServer) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.listener.Close()
	s.wg.Wait()
}

func (s *mockSMTPServer) GetMailFromCommand() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mailFromCmd
}

func (s *mockSMTPServer) GetMessages() []capturedMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]capturedMessage, len(s.messages))
	copy(result, s.messages)
	return result
}

func (s *mockSMTPServer) GetCommands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]string, len(s.commands))
	copy(result, s.commands)
	return result
}

// noopLogger implements logger.Logger interface for testing
type noopLogger struct{}

func (l *noopLogger) Debug(msg string)                                          {}
func (l *noopLogger) Info(msg string)                                           {}
func (l *noopLogger) Warn(msg string)                                           {}
func (l *noopLogger) Error(msg string)                                          {}
func (l *noopLogger) Fatal(msg string)                                          {}
func (l *noopLogger) WithField(key string, value interface{}) pkglogger.Logger  { return l }
func (l *noopLogger) WithFields(fields map[string]interface{}) pkglogger.Logger { return l }

// ============================================================================
// Tests for sendRawEmail function - Core fix for issue #172
// ============================================================================

func TestSendRawEmail_NoExtensionsInMailFrom(t *testing.T) {
	// CRITICAL TEST: Verify MAIL FROM doesn't contain BODY=8BITMIME or SMTPUTF8
	// This is the core fix for issue #172

	server := newMockSMTPServer(t, true)
	defer server.Close()

	port := server.Port()
	msg := []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nTest body")

	err := sendRawEmail("127.0.0.1", port, "", "", false, "sender@example.com", []string{"recipient@example.com"}, msg)
	require.NoError(t, err)

	// Verify MAIL FROM command doesn't contain problematic extensions
	mailFromCmd := server.GetMailFromCommand()
	require.NotEmpty(t, mailFromCmd, "MAIL FROM command should be captured")

	assert.NotContains(t, mailFromCmd, "BODY=8BITMIME", "MAIL FROM should not contain BODY=8BITMIME")
	assert.NotContains(t, mailFromCmd, "SMTPUTF8", "MAIL FROM should not contain SMTPUTF8")
	assert.NotContains(t, mailFromCmd, "SIZE=", "MAIL FROM should not contain SIZE parameter")

	// Verify we got the message
	messages := server.GetMessages()
	require.Len(t, messages, 1)
	assert.Equal(t, "sender@example.com", messages[0].from)
}

func TestSendRawEmail_Success_NoTLS(t *testing.T) {
	server := newMockSMTPServer(t, true)
	defer server.Close()

	port := server.Port()
	msg := []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nTest body")

	err := sendRawEmail("127.0.0.1", port, "", "", false, "sender@example.com", []string{"recipient@example.com"}, msg)
	require.NoError(t, err)

	messages := server.GetMessages()
	require.Len(t, messages, 1)
	assert.Equal(t, "sender@example.com", messages[0].from)
	assert.Contains(t, messages[0].recipients, "recipient@example.com")
}

func TestSendRawEmail_WithAuth_NoTLS(t *testing.T) {
	server := newMockSMTPServer(t, true)
	defer server.Close()

	port := server.Port()
	msg := []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nTest body")

	err := sendRawEmail("127.0.0.1", port, "user", "pass", false, "sender@example.com", []string{"recipient@example.com"}, msg)
	require.NoError(t, err)

	messages := server.GetMessages()
	require.Len(t, messages, 1)
}

func TestSendRawEmail_AuthFailure(t *testing.T) {
	server := newMockSMTPServer(t, false) // Auth will fail
	defer server.Close()

	port := server.Port()
	msg := []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nTest body")

	err := sendRawEmail("127.0.0.1", port, "user", "pass", false, "sender@example.com", []string{"recipient@example.com"}, msg)
	require.Error(t, err)
}

func TestSendRawEmail_ConnectionError(t *testing.T) {
	// Try to connect to a port that's not listening
	msg := []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nTest body")

	err := sendRawEmail("127.0.0.1", 59999, "", "", false, "sender@example.com", []string{"recipient@example.com"}, msg)
	require.Error(t, err)
}

func TestSendRawEmail_MultipleRecipients(t *testing.T) {
	server := newMockSMTPServer(t, true)
	defer server.Close()

	port := server.Port()
	msg := []byte("From: sender@example.com\r\nTo: to@example.com\r\nCc: cc@example.com\r\n\r\nTest body")

	recipients := []string{"to@example.com", "cc@example.com", "bcc@example.com"}
	err := sendRawEmail("127.0.0.1", port, "", "", false, "sender@example.com", recipients, msg)
	require.NoError(t, err)

	messages := server.GetMessages()
	require.Len(t, messages, 1)
	assert.Len(t, messages[0].recipients, 3)
}

func TestSendRawEmail_MultilineBanner(t *testing.T) {
	// Test fix for issue #183: Multi-line 220 banner handling
	// RFC 5321 Section 4.2 allows multi-line greetings like:
	// 220-smtp.example.com ESMTP
	// 220-Additional info
	// 220 Service ready

	server := newMockSMTPServerWithMultilineBanner(t, true)
	defer server.Close()

	port := server.Port()
	msg := []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nTest body")

	err := sendRawEmail("127.0.0.1", port, "", "", false, "sender@example.com", []string{"recipient@example.com"}, msg)
	require.NoError(t, err, "Should handle multi-line 220 banner without error")

	messages := server.GetMessages()
	require.Len(t, messages, 1)
	assert.Equal(t, "sender@example.com", messages[0].from)
	assert.Contains(t, messages[0].recipients, "recipient@example.com")
}

func TestSendRawEmail_MultilineBannerWithAuth(t *testing.T) {
	// Test multi-line banner with authentication
	server := newMockSMTPServerWithMultilineBanner(t, true)
	defer server.Close()

	port := server.Port()
	msg := []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nTest body")

	err := sendRawEmail("127.0.0.1", port, "user", "pass", false, "sender@example.com", []string{"recipient@example.com"}, msg)
	require.NoError(t, err, "Should handle multi-line 220 banner with auth without error")

	messages := server.GetMessages()
	require.Len(t, messages, 1)
}

// ============================================================================
// Tests for SMTPService.SendEmail with real message composition
// ============================================================================

func TestSMTPService_SendEmail_Integration(t *testing.T) {
	server := newMockSMTPServer(t, true)
	defer server.Close()

	log := &noopLogger{}
	service := NewSMTPService(log)

	provider := &domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: &domain.SMTPSettings{
			Host:     "127.0.0.1",
			Port:     server.Port(),
			Username: "",
			Password: "",
			UseTLS:   false,
		},
	}

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		MessageID:     "message-123",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Test Subject",
		Content:       "<h1>Hello</h1><p>This is a test.</p>",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}

	err := service.SendEmail(context.Background(), request)
	require.NoError(t, err)

	// Verify message was sent
	messages := server.GetMessages()
	require.Len(t, messages, 1)
	assert.Equal(t, "sender@example.com", messages[0].from)

	// Verify no extensions in MAIL FROM (issue #172 fix)
	mailFromCmd := server.GetMailFromCommand()
	assert.NotContains(t, mailFromCmd, "BODY=8BITMIME")
	assert.NotContains(t, mailFromCmd, "SMTPUTF8")
}

func TestSMTPService_SendEmail_WithCCAndBCC(t *testing.T) {
	server := newMockSMTPServer(t, true)
	defer server.Close()

	log := &noopLogger{}
	service := NewSMTPService(log)

	provider := &domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: &domain.SMTPSettings{
			Host:     "127.0.0.1",
			Port:     server.Port(),
			Username: "",
			Password: "",
			UseTLS:   false,
		},
	}

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		MessageID:     "message-123",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "to@example.com",
		Subject:       "Test Subject",
		Content:       "<h1>Hello</h1>",
		Provider:      provider,
		EmailOptions: domain.EmailOptions{
			CC:  []string{"cc1@example.com", "cc2@example.com"},
			BCC: []string{"bcc@example.com"},
		},
	}

	err := service.SendEmail(context.Background(), request)
	require.NoError(t, err)

	messages := server.GetMessages()
	require.Len(t, messages, 1)
	// Should have 4 recipients: to + 2 CC + 1 BCC
	assert.Len(t, messages[0].recipients, 4)
}

func TestSMTPService_SendEmail_WithAttachment(t *testing.T) {
	server := newMockSMTPServer(t, true)
	defer server.Close()

	log := &noopLogger{}
	service := NewSMTPService(log)

	provider := &domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: &domain.SMTPSettings{
			Host:     "127.0.0.1",
			Port:     server.Port(),
			Username: "",
			Password: "",
			UseTLS:   false,
		},
	}

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		MessageID:     "message-123",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Test with Attachment",
		Content:       "<h1>See attached</h1>",
		Provider:      provider,
		EmailOptions: domain.EmailOptions{
			Attachments: []domain.Attachment{
				{
					Filename:    "test.txt",
					Content:     "SGVsbG8gV29ybGQh", // "Hello World!" in base64
					ContentType: "text/plain",
					Disposition: "attachment",
				},
			},
		},
	}

	err := service.SendEmail(context.Background(), request)
	require.NoError(t, err)

	messages := server.GetMessages()
	require.Len(t, messages, 1)
	// Verify attachment is in the message data
	assert.Contains(t, string(messages[0].data), "test.txt")
}

func TestSMTPService_SendEmail_WithReplyTo(t *testing.T) {
	server := newMockSMTPServer(t, true)
	defer server.Close()

	log := &noopLogger{}
	service := NewSMTPService(log)

	provider := &domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: &domain.SMTPSettings{
			Host:     "127.0.0.1",
			Port:     server.Port(),
			Username: "",
			Password: "",
			UseTLS:   false,
		},
	}

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		MessageID:     "message-123",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Test Subject",
		Content:       "<h1>Hello</h1>",
		Provider:      provider,
		EmailOptions: domain.EmailOptions{
			ReplyTo: "reply@example.com",
		},
	}

	err := service.SendEmail(context.Background(), request)
	require.NoError(t, err)

	messages := server.GetMessages()
	require.Len(t, messages, 1)
	// Verify Reply-To header is in the message
	assert.Contains(t, string(messages[0].data), "Reply-To:")
}

func TestSMTPService_SendEmail_WithListUnsubscribe(t *testing.T) {
	server := newMockSMTPServer(t, true)
	defer server.Close()

	log := &noopLogger{}
	service := NewSMTPService(log)

	provider := &domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: &domain.SMTPSettings{
			Host:     "127.0.0.1",
			Port:     server.Port(),
			Username: "",
			Password: "",
			UseTLS:   false,
		},
	}

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		MessageID:     "message-123",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Test Subject",
		Content:       "<h1>Hello</h1>",
		Provider:      provider,
		EmailOptions: domain.EmailOptions{
			ListUnsubscribeURL: "https://example.com/unsubscribe",
		},
	}

	err := service.SendEmail(context.Background(), request)
	require.NoError(t, err)

	messages := server.GetMessages()
	require.Len(t, messages, 1)
	// Verify List-Unsubscribe headers are in the message
	assert.Contains(t, string(messages[0].data), "List-Unsubscribe:")
	assert.Contains(t, string(messages[0].data), "List-Unsubscribe-Post:")
}

func TestSMTPService_SendEmail_InlineAttachment(t *testing.T) {
	server := newMockSMTPServer(t, true)
	defer server.Close()

	log := &noopLogger{}
	service := NewSMTPService(log)

	provider := &domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: &domain.SMTPSettings{
			Host:     "127.0.0.1",
			Port:     server.Port(),
			Username: "",
			Password: "",
			UseTLS:   false,
		},
	}

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		MessageID:     "message-123",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Test with Inline Image",
		Content:       "<h1>See image</h1><img src=\"cid:logo.png\">",
		Provider:      provider,
		EmailOptions: domain.EmailOptions{
			Attachments: []domain.Attachment{
				{
					Filename:    "logo.png",
					Content:     "iVBORw0KGgo=", // minimal PNG header in base64
					ContentType: "image/png",
					Disposition: "inline",
				},
			},
		},
	}

	err := service.SendEmail(context.Background(), request)
	require.NoError(t, err)

	messages := server.GetMessages()
	require.Len(t, messages, 1)
	assert.Contains(t, string(messages[0].data), "logo.png")
}

// ============================================================================
// Validation tests
// ============================================================================

func TestSMTPService_SendEmail_MissingSMTPSettings(t *testing.T) {
	log := &noopLogger{}
	service := NewSMTPService(log)

	provider := &domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: nil, // Missing SMTP settings
	}

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		MessageID:     "message-123",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Test Subject",
		Content:       "<h1>Hello</h1>",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}

	err := service.SendEmail(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SMTP settings required")
}

func TestSMTPService_SendEmail_EmptyMessageID(t *testing.T) {
	log := &noopLogger{}
	service := NewSMTPService(log)

	provider := &domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: &domain.SMTPSettings{
			Host:     "127.0.0.1",
			Port:     587,
			Username: "user",
			Password: "pass",
			UseTLS:   true,
		},
	}

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		MessageID:     "", // Empty message ID
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Test Subject",
		Content:       "<h1>Hello</h1>",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}

	err := service.SendEmail(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message ID is required")
}

func TestSMTPService_SendEmail_EmptySubject(t *testing.T) {
	log := &noopLogger{}
	service := NewSMTPService(log)

	provider := &domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: &domain.SMTPSettings{
			Host:     "127.0.0.1",
			Port:     587,
			Username: "user",
			Password: "pass",
			UseTLS:   true,
		},
	}

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		MessageID:     "message-123",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "", // Empty subject
		Content:       "<h1>Hello</h1>",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}

	err := service.SendEmail(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subject is required")
}

func TestSMTPService_SendEmail_EmptyContent(t *testing.T) {
	log := &noopLogger{}
	service := NewSMTPService(log)

	provider := &domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: &domain.SMTPSettings{
			Host:     "127.0.0.1",
			Port:     587,
			Username: "user",
			Password: "pass",
			UseTLS:   true,
		},
	}

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		MessageID:     "message-123",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Test Subject",
		Content:       "", // Empty content
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}

	err := service.SendEmail(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content is required")
}

func TestSMTPService_SendEmail_InvalidBase64Attachment(t *testing.T) {
	server := newMockSMTPServer(t, true)
	defer server.Close()

	log := &noopLogger{}
	service := NewSMTPService(log)

	provider := &domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: &domain.SMTPSettings{
			Host:     "127.0.0.1",
			Port:     server.Port(),
			Username: "",
			Password: "",
			UseTLS:   false,
		},
	}

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		MessageID:     "message-123",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Test Subject",
		Content:       "<h1>Hello</h1>",
		Provider:      provider,
		EmailOptions: domain.EmailOptions{
			Attachments: []domain.Attachment{
				{
					Filename:    "test.pdf",
					Content:     "not-valid-base64!@#$",
					ContentType: "application/pdf",
					Disposition: "attachment",
				},
			},
		},
	}

	err := service.SendEmail(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode content")
}

func TestSMTPService_SendEmail_ConnectionError(t *testing.T) {
	log := &noopLogger{}
	service := NewSMTPService(log)

	// Use a port that's not listening
	provider := &domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: &domain.SMTPSettings{
			Host:     "127.0.0.1",
			Port:     59999, // Unlikely to be in use
			Username: "",
			Password: "",
			UseTLS:   false,
		},
	}

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		MessageID:     "message-123",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Test Subject",
		Content:       "<h1>Hello</h1>",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}

	err := service.SendEmail(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send email")
}

func TestNewSMTPService(t *testing.T) {
	log := &noopLogger{}
	service := NewSMTPService(log)

	require.NotNil(t, service)
	require.Equal(t, log, service.logger)
}

func TestSMTPService_SendEmail_EmptyCCAndBCCFiltering(t *testing.T) {
	server := newMockSMTPServer(t, true)
	defer server.Close()

	log := &noopLogger{}
	service := NewSMTPService(log)

	provider := &domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: &domain.SMTPSettings{
			Host:     "127.0.0.1",
			Port:     server.Port(),
			Username: "",
			Password: "",
			UseTLS:   false,
		},
	}

	// CC and BCC with some empty strings that should be filtered out
	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		MessageID:     "message-123",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "to@example.com",
		Subject:       "Test Subject",
		Content:       "<h1>Hello</h1>",
		Provider:      provider,
		EmailOptions: domain.EmailOptions{
			CC:  []string{"", "cc@example.com", ""},
			BCC: []string{"bcc@example.com", ""},
		},
	}

	err := service.SendEmail(context.Background(), request)
	require.NoError(t, err)

	messages := server.GetMessages()
	require.Len(t, messages, 1)
	// Should have 3 recipients: to + 1 valid CC + 1 valid BCC (empty strings filtered)
	assert.Len(t, messages[0].recipients, 3)
}
