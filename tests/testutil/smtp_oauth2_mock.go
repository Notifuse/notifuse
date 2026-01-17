package testutil

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// MockOAuth2SMTPServer is an SMTP server that validates XOAUTH2 authentication
// Follows RFC 5321 (SMTP) and RFC 7628 (SASL XOAUTH2)
type MockOAuth2SMTPServer struct {
	listener         net.Listener
	validTokens      map[string]string // token -> username mapping
	receivedMessages []CapturedSMTPMessage
	authAttempts     []SMTPAuthAttempt
	failFirstAuth    bool   // For retry testing (fails first, succeeds on second)
	errorResponse    string // Custom base64-encoded JSON error
	mu               sync.Mutex
	wg               sync.WaitGroup
	closed           bool
	authAttemptCount int // Counter for retry testing
}

// SMTPAuthAttempt records details of an XOAUTH2 authentication attempt
type SMTPAuthAttempt struct {
	Username   string
	Token      string
	Success    bool
	RawXOAuth2 string // Base64-decoded XOAUTH2 string for validation
	Timestamp  time.Time
}

// CapturedSMTPMessage stores a captured email message
type CapturedSMTPMessage struct {
	From       string
	Recipients []string
	Data       []byte
	Timestamp  time.Time
}

// NewMockOAuth2SMTPServer creates a new mock SMTP server that validates XOAUTH2
func NewMockOAuth2SMTPServer(validTokens map[string]string) *MockOAuth2SMTPServer {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("Failed to create mock SMTP server: %v", err))
	}

	s := &MockOAuth2SMTPServer{
		listener:         listener,
		validTokens:      validTokens,
		receivedMessages: make([]CapturedSMTPMessage, 0),
		authAttempts:     make([]SMTPAuthAttempt, 0),
	}

	s.wg.Add(1)
	go s.serve()

	return s
}

// serve accepts and handles incoming connections
func (s *MockOAuth2SMTPServer) serve() {
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

// handleConnection handles an individual SMTP connection
func (s *MockOAuth2SMTPServer) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Send greeting
	conn.Write([]byte("220 localhost SMTP OAuth2 Mock Server\r\n"))

	var from string
	var recipients []string
	var inData bool
	var dataBuffer strings.Builder
	var authenticated bool

	for {
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		line = strings.TrimSpace(line)
		upperLine := strings.ToUpper(line)

		if inData {
			if line == "." {
				inData = false
				s.mu.Lock()
				s.receivedMessages = append(s.receivedMessages, CapturedSMTPMessage{
					From:       from,
					Recipients: recipients,
					Data:       []byte(dataBuffer.String()),
					Timestamp:  time.Now(),
				})
				s.mu.Unlock()
				conn.Write([]byte("250 2.0.0 OK message queued\r\n"))
				continue
			}
			dataBuffer.WriteString(line + "\r\n")
			continue
		}

		switch {
		case strings.HasPrefix(upperLine, "EHLO") || strings.HasPrefix(upperLine, "HELO"):
			conn.Write([]byte("250-localhost\r\n"))
			conn.Write([]byte("250-AUTH XOAUTH2\r\n"))
			conn.Write([]byte("250-SIZE 10485760\r\n"))
			conn.Write([]byte("250 OK\r\n"))

		case strings.HasPrefix(upperLine, "AUTH XOAUTH2"):
			// Extract the base64-encoded XOAUTH2 string
			parts := strings.SplitN(line, " ", 3)
			if len(parts) < 3 {
				s.recordAuthAttempt("", "", false, "")
				conn.Write([]byte("535 5.7.8 Invalid XOAUTH2 token format\r\n"))
				continue
			}

			encoded := parts[2]
			success, username, token, rawXOAuth2 := s.validateXOAuth2(encoded)

			// Record the attempt
			s.recordAuthAttempt(username, token, success, rawXOAuth2)

			if success {
				authenticated = true
				conn.Write([]byte("235 2.7.0 Authentication successful\r\n"))
			} else {
				// Return error response
				errorResp := s.getErrorResponse()
				conn.Write([]byte(fmt.Sprintf("535 5.7.8 %s\r\n", errorResp)))
			}

		case strings.HasPrefix(upperLine, "MAIL FROM:"):
			if !authenticated {
				conn.Write([]byte("530 5.7.0 Authentication required\r\n"))
				continue
			}
			// Extract email from MAIL FROM:<email>
			start := strings.Index(line, "<")
			end := strings.Index(line, ">")
			if start != -1 && end != -1 && end > start {
				from = line[start+1 : end]
			}
			conn.Write([]byte("250 2.1.0 OK\r\n"))

		case strings.HasPrefix(upperLine, "RCPT TO:"):
			if !authenticated {
				conn.Write([]byte("530 5.7.0 Authentication required\r\n"))
				continue
			}
			start := strings.Index(line, "<")
			end := strings.Index(line, ">")
			if start != -1 && end != -1 && end > start {
				recipients = append(recipients, line[start+1:end])
			}
			conn.Write([]byte("250 2.1.5 OK\r\n"))

		case strings.HasPrefix(upperLine, "DATA"):
			if !authenticated {
				conn.Write([]byte("530 5.7.0 Authentication required\r\n"))
				continue
			}
			inData = true
			dataBuffer.Reset()
			conn.Write([]byte("354 Start mail input; end with <CRLF>.<CRLF>\r\n"))

		case strings.HasPrefix(upperLine, "QUIT"):
			conn.Write([]byte("221 2.0.0 Bye\r\n"))
			return

		case strings.HasPrefix(upperLine, "RSET"):
			from = ""
			recipients = nil
			conn.Write([]byte("250 2.0.0 OK\r\n"))

		case strings.HasPrefix(upperLine, "NOOP"):
			conn.Write([]byte("250 2.0.0 OK\r\n"))

		default:
			conn.Write([]byte("500 5.5.1 Command not recognized\r\n"))
		}
	}
}

// validateXOAuth2 validates an XOAUTH2 authentication string
// Format: base64("user=" + email + "\x01auth=Bearer " + token + "\x01\x01")
func (s *MockOAuth2SMTPServer) validateXOAuth2(encoded string) (bool, string, string, string) {
	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return false, "", "", ""
	}

	rawXOAuth2 := string(decoded)

	// Parse XOAUTH2 format: user={email}\x01auth=Bearer {token}\x01\x01
	if !strings.HasPrefix(rawXOAuth2, "user=") {
		return false, "", "", rawXOAuth2
	}

	// Find the first \x01 delimiter
	userEnd := strings.Index(rawXOAuth2, "\x01")
	if userEnd == -1 {
		return false, "", "", rawXOAuth2
	}

	username := strings.TrimPrefix(rawXOAuth2[:userEnd], "user=")

	// Find auth=Bearer
	authStart := strings.Index(rawXOAuth2, "auth=Bearer ")
	if authStart == -1 {
		return false, username, "", rawXOAuth2
	}

	tokenStart := authStart + len("auth=Bearer ")
	tokenEnd := strings.Index(rawXOAuth2[tokenStart:], "\x01")
	if tokenEnd == -1 {
		return false, username, "", rawXOAuth2
	}

	token := rawXOAuth2[tokenStart : tokenStart+tokenEnd]

	// Check if we should fail the first attempt (for retry testing)
	s.mu.Lock()
	s.authAttemptCount++
	attemptNum := s.authAttemptCount
	failFirst := s.failFirstAuth
	s.mu.Unlock()

	if failFirst && attemptNum == 1 {
		return false, username, token, rawXOAuth2
	}

	// Validate token against valid tokens
	s.mu.Lock()
	expectedUsername, exists := s.validTokens[token]
	s.mu.Unlock()

	if !exists {
		return false, username, token, rawXOAuth2
	}

	if expectedUsername != username {
		return false, username, token, rawXOAuth2
	}

	return true, username, token, rawXOAuth2
}

// recordAuthAttempt records an authentication attempt
func (s *MockOAuth2SMTPServer) recordAuthAttempt(username, token string, success bool, rawXOAuth2 string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.authAttempts = append(s.authAttempts, SMTPAuthAttempt{
		Username:   username,
		Token:      token,
		Success:    success,
		RawXOAuth2: rawXOAuth2,
		Timestamp:  time.Now(),
	})
}

// getErrorResponse returns the error response for failed authentication
func (s *MockOAuth2SMTPServer) getErrorResponse() string {
	s.mu.Lock()
	customError := s.errorResponse
	s.mu.Unlock()

	if customError != "" {
		return customError
	}

	// Default XOAUTH2 error response (base64-encoded JSON)
	errorJSON := `{"status":"401","schemes":"Bearer","scope":"https://mail.google.com/"}`
	return base64.StdEncoding.EncodeToString([]byte(errorJSON))
}

// Host returns the host address
func (s *MockOAuth2SMTPServer) Host() string {
	return "127.0.0.1"
}

// Port returns the port the server is listening on
func (s *MockOAuth2SMTPServer) Port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

// Addr returns the full address (host:port)
func (s *MockOAuth2SMTPServer) Addr() string {
	return s.listener.Addr().String()
}

// Close shuts down the mock server
func (s *MockOAuth2SMTPServer) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.listener.Close()
	s.wg.Wait()
}

// GetMessages returns a copy of all received messages
func (s *MockOAuth2SMTPServer) GetMessages() []CapturedSMTPMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]CapturedSMTPMessage, len(s.receivedMessages))
	copy(result, s.receivedMessages)
	return result
}

// GetAuthAttempts returns a copy of all authentication attempts
func (s *MockOAuth2SMTPServer) GetAuthAttempts() []SMTPAuthAttempt {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]SMTPAuthAttempt, len(s.authAttempts))
	copy(result, s.authAttempts)
	return result
}

// SetFailFirstAuth configures the server to fail the first authentication attempt
// This is useful for testing retry logic
func (s *MockOAuth2SMTPServer) SetFailFirstAuth(fail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failFirstAuth = fail
}

// SetErrorResponse sets a custom error response for failed authentication
// The response should be a base64-encoded JSON string
func (s *MockOAuth2SMTPServer) SetErrorResponse(jsonError string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errorResponse = base64.StdEncoding.EncodeToString([]byte(jsonError))
}

// ClearMessages clears all received messages
func (s *MockOAuth2SMTPServer) ClearMessages() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.receivedMessages = make([]CapturedSMTPMessage, 0)
}

// ClearAuthAttempts clears all authentication attempts
func (s *MockOAuth2SMTPServer) ClearAuthAttempts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authAttempts = make([]SMTPAuthAttempt, 0)
	s.authAttemptCount = 0
}

// ResetRetryCounter resets the authentication attempt counter
func (s *MockOAuth2SMTPServer) ResetRetryCounter() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authAttemptCount = 0
}
