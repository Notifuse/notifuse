package mailer

import (
	"bytes"
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"testing"
)

// captureOutput captures stdout for testing
func captureOutput(f func()) string {
	// Keep original stdout
	oldStdout := os.Stdout

	// Create a pipe to capture stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call the function that produces output
	f()

	// Close the write end and restore original stdout
	w.Close()
	os.Stdout = oldStdout

	// Read the captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)

	return buf.String()
}

// captureLog captures log output for testing
func captureLog(f func()) string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	f()
	log.SetOutput(os.Stderr) // Reset to default
	return buf.String()
}

// MockMailer is a mock implementation of the Mailer interface for testing
type MockMailer struct {
	shouldFail bool
}

func NewMockMailer(shouldFail bool) *MockMailer {
	return &MockMailer{
		shouldFail: shouldFail,
	}
}

func (m *MockMailer) SendWorkspaceInvitation(email, workspaceName, inviterName, token string) error {
	if m.shouldFail {
		return errors.New("mock mailer error")
	}
	return nil
}

func (m *MockMailer) SendMagicCode(email, code string) error {
	if m.shouldFail {
		return errors.New("mock mailer error")
	}
	return nil
}

// ValidatingMailer is a mock implementation that validates inputs
type ValidatingMailer struct {
	config *Config
}

func NewValidatingMailer(config *Config) *ValidatingMailer {
	return &ValidatingMailer{
		config: config,
	}
}

func (m *ValidatingMailer) SendWorkspaceInvitation(email, workspaceName, inviterName, token string) error {
	// Validate email
	if email == "" {
		return errors.New("email is required")
	}
	if !strings.Contains(email, "@") {
		return errors.New("invalid email format")
	}

	// Validate workspaceName
	if workspaceName == "" {
		return errors.New("workspace name is required")
	}

	// Validate inviterName
	if inviterName == "" {
		return errors.New("inviter name is required")
	}

	// Validate token
	if token == "" {
		return errors.New("token is required")
	}

	// If all validations pass, return success
	return nil
}

func TestMockMailer_SendWorkspaceInvitation(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mailer := NewMockMailer(false)
		err := mailer.SendWorkspaceInvitation("test@example.com", "Test Workspace", "Test Inviter", "test-token")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		mailer := NewMockMailer(true)
		err := mailer.SendWorkspaceInvitation("test@example.com", "Test Workspace", "Test Inviter", "test-token")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "mock mailer error" {
			t.Errorf("Expected 'mock mailer error', got '%s'", err.Error())
		}
	})
}

func TestValidatingMailer_SendWorkspaceInvitation(t *testing.T) {
	config := &Config{
		SMTPHost:     "smtp.example.com",
		SMTPPort:     587,
		SMTPUsername: "username",
		SMTPPassword: "password",
		FromEmail:    "noreply@example.com",
		FromName:     "Notifuse",
		APIEndpoint:  "https://example.com",
	}

	mailer := NewValidatingMailer(config)

	testCases := []struct {
		name          string
		email         string
		workspaceName string
		inviterName   string
		token         string
		expectedError string
	}{
		{
			name:          "valid input",
			email:         "test@example.com",
			workspaceName: "Test Workspace",
			inviterName:   "Test Inviter",
			token:         "test-token",
			expectedError: "",
		},
		{
			name:          "empty email",
			email:         "",
			workspaceName: "Test Workspace",
			inviterName:   "Test Inviter",
			token:         "test-token",
			expectedError: "email is required",
		},
		{
			name:          "invalid email format",
			email:         "invalid-email",
			workspaceName: "Test Workspace",
			inviterName:   "Test Inviter",
			token:         "test-token",
			expectedError: "invalid email format",
		},
		{
			name:          "empty workspace name",
			email:         "test@example.com",
			workspaceName: "",
			inviterName:   "Test Inviter",
			token:         "test-token",
			expectedError: "workspace name is required",
		},
		{
			name:          "empty inviter name",
			email:         "test@example.com",
			workspaceName: "Test Workspace",
			inviterName:   "",
			token:         "test-token",
			expectedError: "inviter name is required",
		},
		{
			name:          "empty token",
			email:         "test@example.com",
			workspaceName: "Test Workspace",
			inviterName:   "Test Inviter",
			token:         "",
			expectedError: "token is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := mailer.SendWorkspaceInvitation(tc.email, tc.workspaceName, tc.inviterName, tc.token)

			if tc.expectedError == "" {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error '%s', got nil", tc.expectedError)
				} else if err.Error() != tc.expectedError {
					t.Errorf("Expected error '%s', got '%s'", tc.expectedError, err.Error())
				}
			}
		})
	}
}

func TestConsoleMailer_SendWorkspaceInvitation(t *testing.T) {
	// Setup test data
	email := "test@example.com"
	workspaceName := "Test Workspace"
	inviterName := "Test Inviter"
	token := "test-token-123"

	// Create the mailer
	mailer := NewConsoleMailer()

	// Capture output
	output := captureOutput(func() {
		err := mailer.SendWorkspaceInvitation(email, workspaceName, inviterName, token)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	// Verify the output contains the expected information
	expectedStrings := []string{
		"WORKSPACE INVITATION EMAIL",
		"To: " + email,
		"Subject: You've been invited to join " + workspaceName,
		inviterName + " has invited you to join the " + workspaceName,
		"Use the following token to join: " + token,
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain '%s', but it didn't. Output: %s", expected, output)
		}
	}
}

func TestConsoleMailer_SendMagicCode(t *testing.T) {
	// Create the mailer
	mailer := NewConsoleMailer()

	// Capture output
	output := captureOutput(func() {
		err := mailer.SendMagicCode("test@example.com", "123456")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	// Verify the output contains the expected information
	expectedStrings := []string{
		"AUTHENTICATION MAGIC CODE",
		"To: test@example.com",
		"Subject: Your authentication code",
		"123456",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain '%s', but it didn't. Output: %s", expected, output)
		}
	}
}

func TestSMTPMailer_SendWorkspaceInvitation(t *testing.T) {
	// Setup test data
	email := "test@example.com"
	workspaceName := "Test Workspace"
	inviterName := "Test Inviter"
	token := "test-token-123"
	baseURL := "https://notifuse.example.com"

	// Create the config and mailer
	config := &Config{
		SMTPHost:     "smtp.example.com",
		SMTPPort:     587,
		SMTPUsername: "username",
		SMTPPassword: "password",
		FromEmail:    "noreply@example.com",
		FromName:     "Notifuse",
		APIEndpoint:  baseURL,
	}

	// Create a test mode mailer
	mailer := NewTestSMTPMailer(config)

	// Capture log output
	logOutput := captureLog(func() {
		err := mailer.SendWorkspaceInvitation(email, workspaceName, inviterName, token)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	// Verify log output contains expected information
	expectedInviteURL := baseURL + "/accept-invitation?token=" + token
	expectedLogLines := []string{
		"Sending invitation email to: " + email,
		"From: " + config.FromName + " <" + config.FromEmail + ">",
		"Subject: You've been invited to join " + workspaceName,
		"Invitation URL: " + expectedInviteURL,
	}

	for _, expected := range expectedLogLines {
		if !strings.Contains(logOutput, expected) {
			t.Errorf("Expected log to contain '%s', but it didn't. Log: %s", expected, logOutput)
		}
	}
}

func TestSMTPMailer_WithEdgeCases(t *testing.T) {
	testCases := []struct {
		name          string
		email         string
		workspaceName string
		inviterName   string
		token         string
		baseURL       string
		expectError   bool
	}{
		{
			name:          "all fields empty",
			email:         "",
			workspaceName: "",
			inviterName:   "",
			token:         "",
			baseURL:       "",
			expectError:   true, // empty email should cause error
		},
		{
			name:          "special characters in workspace name",
			email:         "user@example.com",
			workspaceName: "Test & Workspace <script>alert('xss')</script>",
			inviterName:   "John Doe",
			token:         "valid-token",
			baseURL:       "https://example.com",
			expectError:   false,
		},
		{
			name:          "very long token",
			email:         "user@example.com",
			workspaceName: "Test Workspace",
			inviterName:   "John Doe",
			token:         strings.Repeat("x", 1000),
			baseURL:       "https://example.com",
			expectError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &Config{
				SMTPHost:     "smtp.example.com",
				SMTPPort:     587,
				SMTPUsername: "username",
				SMTPPassword: "password",
				FromEmail:    "noreply@example.com",
				FromName:     "Notifuse",
				APIEndpoint:  tc.baseURL,
			}

			// Use test mode mailer
			mailer := NewTestSMTPMailer(config)

			logOutput := captureLog(func() {
				err := mailer.SendWorkspaceInvitation(tc.email, tc.workspaceName, tc.inviterName, tc.token)
				if tc.expectError && err == nil {
					t.Error("Expected error but got nil")
				}
				if !tc.expectError && err != nil {
					t.Errorf("Did not expect error but got: %v", err)
				}
			})

			// For non-empty email cases, verify log contains info
			if tc.email != "" && !tc.expectError {
				if !strings.Contains(logOutput, "Sending invitation email to: "+tc.email) {
					t.Errorf("Expected log to contain email '%s', but it didn't. Log: %s", tc.email, logOutput)
				}
			}

			// For the special characters case, verify the log contains the workspace name
			if tc.name == "special characters in workspace name" && !tc.expectError {
				expectedSubject := "Subject: You've been invited to join " + tc.workspaceName
				if !strings.Contains(logOutput, expectedSubject) {
					t.Errorf("Expected log to contain workspace name with special characters, but it didn't. Log: %s", logOutput)
				}
			}
		})
	}
}

func TestNewSMTPMailer(t *testing.T) {
	// Setup test config
	config := &Config{
		SMTPHost:     "smtp.example.com",
		SMTPPort:     587,
		SMTPUsername: "username",
		SMTPPassword: "password",
		FromEmail:    "noreply@example.com",
		FromName:     "Notifuse",
		APIEndpoint:  "https://notifuse.example.com",
	}

	// Create new mailer
	mailer := NewSMTPMailer(config)

	// Verify the mailer has the correct config
	if mailer.config != config {
		t.Errorf("Expected config to be %v, got %v", config, mailer.config)
	}
}

func TestNewConsoleMailer(t *testing.T) {
	// Create new mailer
	mailer := NewConsoleMailer()

	// Verify it's not nil
	if mailer == nil {
		t.Errorf("Expected non-nil mailer")
	}
}

func TestMailerConfig(t *testing.T) {
	testCases := []struct {
		name     string
		config   *Config
		validate func(t *testing.T, config *Config)
	}{
		{
			name: "complete config",
			config: &Config{
				SMTPHost:     "smtp.example.com",
				SMTPPort:     587,
				SMTPUsername: "username",
				SMTPPassword: "password",
				FromEmail:    "noreply@example.com",
				FromName:     "Notifuse",
				APIEndpoint:  "https://notifuse.example.com",
			},
			validate: func(t *testing.T, config *Config) {
				if config.SMTPHost != "smtp.example.com" {
					t.Errorf("Expected SMTPHost to be 'smtp.example.com', got '%s'", config.SMTPHost)
				}
				if config.SMTPPort != 587 {
					t.Errorf("Expected SMTPPort to be 587, got %d", config.SMTPPort)
				}
			},
		},
		{
			name: "minimal config",
			config: &Config{
				SMTPHost:  "smtp.example.com",
				SMTPPort:  25, // Default SMTP port
				FromEmail: "noreply@example.com",
			},
			validate: func(t *testing.T, config *Config) {
				if config.SMTPUsername != "" {
					t.Errorf("Expected empty SMTPUsername, got '%s'", config.SMTPUsername)
				}
				if config.FromName != "" {
					t.Errorf("Expected empty FromName, got '%s'", config.FromName)
				}
			},
		},
		{
			name: "non-standard port",
			config: &Config{
				SMTPHost:  "smtp.example.com",
				SMTPPort:  2525, // Non-standard SMTP port
				FromEmail: "noreply@example.com",
			},
			validate: func(t *testing.T, config *Config) {
				if config.SMTPPort != 2525 {
					t.Errorf("Expected SMTPPort to be 2525, got %d", config.SMTPPort)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mailer := NewSMTPMailer(tc.config)

			// Verify the config was properly assigned
			if mailer.config != tc.config {
				t.Errorf("Expected config to be %v, got %v", tc.config, mailer.config)
			}

			// Run additional validation
			tc.validate(t, mailer.config)
		})
	}
}

func TestSMTPMailer_SendMagicCode(t *testing.T) {
	// Setup test data
	email := "test@example.com"
	code := "123456"
	baseURL := "https://notifuse.example.com"

	// Create the config and mailer
	config := &Config{
		SMTPHost:     "smtp.example.com",
		SMTPPort:     587,
		SMTPUsername: "username",
		SMTPPassword: "password",
		FromEmail:    "noreply@example.com",
		FromName:     "Notifuse",
		APIEndpoint:  baseURL,
	}

	// Create a test mode mailer
	mailer := NewTestSMTPMailer(config)

	// Capture log output
	logOutput := captureLog(func() {
		err := mailer.SendMagicCode(email, code)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	// Verify log output contains expected information
	expectedLogLines := []string{
		"Sending magic code to: " + email,
		"From: " + config.FromName + " <" + config.FromEmail + ">",
		"Subject: Your Notifuse authentication code",
		"Code: " + code,
	}

	for _, expected := range expectedLogLines {
		if !strings.Contains(logOutput, expected) {
			t.Errorf("Expected log to contain '%s', but it didn't. Log: %s", expected, logOutput)
		}
	}
}
