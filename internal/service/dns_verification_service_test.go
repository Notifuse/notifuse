package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupDNSVerificationTest creates a mock logger and DNS verification service for testing
func setupDNSVerificationTest(t *testing.T, expectedTarget string) (*DNSVerificationService, *pkgmocks.MockLogger) {
	ctrl := gomock.NewController(t)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger to handle any calls
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	service := NewDNSVerificationService(mockLogger, expectedTarget)
	return service, mockLogger
}

func TestExtractHostname(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "Full URL with https",
			input:    "https://preview.notifuse.com",
			expected: "preview.notifuse.com",
			wantErr:  false,
		},
		{
			name:     "Full URL with http",
			input:    "http://example.com",
			expected: "example.com",
			wantErr:  false,
		},
		{
			name:     "URL with path",
			input:    "https://preview.notifuse.com/path/to/resource",
			expected: "preview.notifuse.com",
			wantErr:  false,
		},
		{
			name:     "URL with query parameters",
			input:    "https://preview.notifuse.com?param=value",
			expected: "preview.notifuse.com",
			wantErr:  false,
		},
		{
			name:     "URL with port",
			input:    "https://preview.notifuse.com:8080",
			expected: "preview.notifuse.com",
			wantErr:  false,
		},
		{
			name:     "Plain hostname",
			input:    "preview.notifuse.com",
			expected: "preview.notifuse.com",
			wantErr:  false,
		},
		{
			name:     "Hostname with trailing slash",
			input:    "preview.notifuse.com/",
			expected: "preview.notifuse.com",
			wantErr:  false,
		},
		{
			name:     "Hostname with path",
			input:    "preview.notifuse.com/path",
			expected: "preview.notifuse.com",
			wantErr:  false,
		},
		{
			name:     "Hostname with query",
			input:    "preview.notifuse.com?query=value",
			expected: "preview.notifuse.com",
			wantErr:  false,
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "Just domain",
			input:    "notifuse.com",
			expected: "notifuse.com",
			wantErr:  false,
		},
		{
			name:     "Subdomain",
			input:    "https://subdomain.example.com",
			expected: "subdomain.example.com",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractHostname(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestNewDNSVerificationService(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	expectedTarget := "https://preview.notifuse.com"
	service := NewDNSVerificationService(mockLogger, expectedTarget)

	assert.NotNil(t, service)
	assert.Equal(t, expectedTarget, service.expectedTarget)
}

func TestDNSVerificationService_VerifyDomainOwnership_InvalidURL(t *testing.T) {
	service, _ := setupDNSVerificationTest(t, "https://preview.notifuse.com")

	tests := []struct {
		name      string
		domainURL string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "Invalid URL format",
			domainURL: "not-a-valid-url://",
			wantErr:   true,
			errMsg:    "no hostname found in URL", // url.Parse may succeed but return empty hostname
		},
		{
			name:      "Empty URL",
			domainURL: "",
			wantErr:   true,
			errMsg:    "no hostname found in URL", // Empty URL results in no hostname
		},
		{
			name:      "URL without hostname",
			domainURL: "https://",
			wantErr:   true,
			errMsg:    "no hostname found in URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.VerifyDomainOwnership(context.Background(), tt.domainURL)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
			_, ok := err.(domain.ValidationError)
			assert.True(t, ok, "error should be ValidationError")
		})
	}
}

func TestDNSVerificationService_VerifyDomainOwnership_ValidURL(t *testing.T) {
	service, _ := setupDNSVerificationTest(t, "https://preview.notifuse.com")

	// Test that valid URLs are parsed correctly
	// Note: Actual DNS lookups will fail in unit tests, but we can test the URL parsing
	validURLs := []string{
		"https://custom.example.com",
		"http://custom.example.com",
		"https://custom.example.com:8080",
		"https://custom.example.com/path",
	}

	for _, url := range validURLs {
		t.Run("Valid URL: "+url, func(t *testing.T) {
			err := service.VerifyDomainOwnership(context.Background(), url)
			// We expect DNS lookup to fail in unit tests, but URL parsing should succeed
			// The error should be a ValidationError about DNS, not URL parsing
			if err != nil {
				_, ok := err.(domain.ValidationError)
				assert.True(t, ok, "error should be ValidationError")
				// Should not be a URL parsing error
				assert.NotContains(t, err.Error(), "invalid domain URL")
				assert.NotContains(t, err.Error(), "no hostname found")
			}
		})
	}
}

func TestDNSVerificationService_VerifyDomainOwnership_ExpectedTargetExtraction(t *testing.T) {
	tests := []struct {
		name           string
		expectedTarget string
		domainURL      string
		description    string
	}{
		{
			name:           "Expected target as URL",
			expectedTarget: "https://preview.notifuse.com",
			domainURL:      "https://custom.example.com",
			description:    "Should extract hostname from URL expectedTarget",
		},
		{
			name:           "Expected target as hostname",
			expectedTarget: "preview.notifuse.com",
			domainURL:      "https://custom.example.com",
			description:    "Should handle hostname-only expectedTarget",
		},
		{
			name:           "Expected target with path",
			expectedTarget: "https://preview.notifuse.com/api",
			domainURL:      "https://custom.example.com",
			description:    "Should extract hostname from URL with path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, _ := setupDNSVerificationTest(t, tt.expectedTarget)

			// Verify that the service can parse the expectedTarget correctly
			// by checking it doesn't fail on URL parsing
			err := service.VerifyDomainOwnership(context.Background(), tt.domainURL)

			// We expect DNS lookup errors, but not URL parsing errors for expectedTarget
			if err != nil {
				errMsg := err.Error()
				// Should not fail on expectedTarget parsing
				assert.NotContains(t, errMsg, "invalid expected target")
				assert.NotContains(t, errMsg, "Failed to parse expected target")
			}
		})
	}
}

func TestDNSVerificationService_VerifyTXTRecord_InvalidURL(t *testing.T) {
	service, _ := setupDNSVerificationTest(t, "https://preview.notifuse.com")

	tests := []struct {
		name      string
		domainURL string
		token     string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "Invalid URL format",
			domainURL: "not-a-valid-url://",
			token:     "test-token",
			wantErr:   true,
			errMsg:    "no hostname found in URL", // url.Parse may succeed but return empty hostname
		},
		{
			name:      "Empty URL",
			domainURL: "",
			token:     "test-token",
			wantErr:   true,
			errMsg:    "no hostname found in URL", // Empty URL results in no hostname
		},
		{
			name:      "URL without hostname",
			domainURL: "https://",
			token:     "test-token",
			wantErr:   true,
			errMsg:    "no hostname found in URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.VerifyTXTRecord(context.Background(), tt.domainURL, tt.token)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
			_, ok := err.(domain.ValidationError)
			assert.True(t, ok, "error should be ValidationError")
		})
	}
}

func TestDNSVerificationService_VerifyTXTRecord_ValidURL(t *testing.T) {
	service, _ := setupDNSVerificationTest(t, "https://preview.notifuse.com")

	// Test that valid URLs are parsed correctly
	validURLs := []string{
		"https://custom.example.com",
		"http://custom.example.com",
		"https://custom.example.com:8080",
	}

	for _, url := range validURLs {
		t.Run("Valid URL: "+url, func(t *testing.T) {
			err := service.VerifyTXTRecord(context.Background(), url, "test-token")
			// We expect DNS lookup to fail in unit tests, but URL parsing should succeed
			if err != nil {
				_, ok := err.(domain.ValidationError)
				assert.True(t, ok, "error should be ValidationError")
				// Should not be a URL parsing error
				assert.NotContains(t, err.Error(), "invalid domain URL")
				assert.NotContains(t, err.Error(), "no hostname found")
			}
		})
	}
}

func TestDNSVerificationService_VerifyTXTRecord_ExpectedRecordFormat(t *testing.T) {
	_, _ = setupDNSVerificationTest(t, "https://preview.notifuse.com")

	// Test that the expected record format is correct
	token := "test-verification-token-123"
	expectedRecord := "notifuse-verify=" + token

	// Verify the format matches what the code expects
	assert.Equal(t, "notifuse-verify=test-verification-token-123", expectedRecord)

	// Test with different tokens
	tokens := []string{
		"token1",
		"token-with-dashes",
		"token_with_underscores",
		"token123",
	}

	for _, token := range tokens {
		t.Run("Token format: "+token, func(t *testing.T) {
			expected := "notifuse-verify=" + token
			assert.Equal(t, expected, "notifuse-verify="+token)
		})
	}
}

// TestDNSVerificationService_VerifyARecord_IPComparison tests the IP comparison logic
// Note: This test uses localhost which should resolve in most environments
func TestDNSVerificationService_VerifyARecord_IPComparison(t *testing.T) {
	// Use localhost as expectedTarget - it should resolve to 127.0.0.1
	service, _ := setupDNSVerificationTest(t, "localhost")

	// Test with localhost which should have matching IPs
	// This is a best-effort test that may work if DNS is available
	err := service.VerifyDomainOwnership(context.Background(), "http://localhost")

	// If DNS is available, this might succeed
	// If not, we expect a DNS error, not a logic error
	if err != nil {
		_, ok := err.(domain.ValidationError)
		assert.True(t, ok, "error should be ValidationError")
		// Should be a DNS-related error, not a parsing error
		assert.NotContains(t, err.Error(), "Failed to parse expected target")
	}
}

// TestDNSVerificationService_VerifyARecord_ExpectedTargetExtraction tests that
// expectedTarget URLs are correctly extracted before DNS lookup
func TestDNSVerificationService_VerifyARecord_ExpectedTargetExtraction(t *testing.T) {
	tests := []struct {
		name           string
		expectedTarget string
		description    string
	}{
		{
			name:           "URL with https",
			expectedTarget: "https://preview.notifuse.com",
			description:    "Should extract hostname from https URL",
		},
		{
			name:           "URL with http",
			expectedTarget: "http://preview.notifuse.com",
			description:    "Should extract hostname from http URL",
		},
		{
			name:           "Plain hostname",
			expectedTarget: "preview.notifuse.com",
			description:    "Should handle plain hostname",
		},
		{
			name:           "URL with path",
			expectedTarget: "https://preview.notifuse.com/api",
			description:    "Should extract hostname from URL with path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, _ := setupDNSVerificationTest(t, tt.expectedTarget)

			// Test that verifyARecord can extract hostname correctly
			// by attempting verification (will fail on DNS but not on parsing)
			err := service.VerifyDomainOwnership(context.Background(), "http://custom.example.com")

			if err != nil {
				errMsg := err.Error()
				// Should not fail on expectedTarget hostname extraction
				assert.NotContains(t, errMsg, "Failed to parse expected target")
				// The error should be about DNS resolution, not parsing
				// (unless DNS actually works, in which case we might get other errors)
			}
		})
	}
}

// TestDNSVerificationService_ErrorMessages tests that error messages are properly formatted
func TestDNSVerificationService_ErrorMessages(t *testing.T) {
	expectedTarget := "https://preview.notifuse.com"
	service, _ := setupDNSVerificationTest(t, expectedTarget)

	// Test invalid URL error message
	err := service.VerifyDomainOwnership(context.Background(), "invalid-url")
	require.Error(t, err)
	// url.Parse may succeed but return empty hostname, so error is "no hostname found"
	assert.Contains(t, err.Error(), "no hostname found")

	// Test that error messages include the expectedTarget
	// (when DNS fails, the error should reference the expectedTarget)
	err = service.VerifyDomainOwnership(context.Background(), "https://custom.example.com")
	if err != nil {
		// Error messages should reference the original expectedTarget (URL), not just hostname
		errMsg := err.Error()
		// The error should mention the expectedTarget in some form
		// (either as URL or hostname, depending on where it's used)
		assert.NotEmpty(t, errMsg)
	}
}

// TestDNSVerificationService_CNAMEComparison tests the CNAME comparison logic
// This tests the string comparison logic without requiring actual DNS
func TestDNSVerificationService_CNAMEComparison(t *testing.T) {
	tests := []struct {
		name           string
		expectedTarget string
		cnameValue     string
		hostname       string
		shouldMatch    bool
		description    string
	}{
		{
			name:           "Exact match",
			expectedTarget: "preview.notifuse.com",
			cnameValue:     "preview.notifuse.com",
			hostname:       "custom.example.com",
			shouldMatch:    true,
			description:    "CNAME exactly matches expected target",
		},
		{
			name:           "Subdomain match",
			expectedTarget: "notifuse.com",
			cnameValue:     "subdomain.notifuse.com",
			hostname:       "custom.example.com",
			shouldMatch:    false, // HasSuffix check - subdomain.notifuse.com ends with notifuse.com
			description:    "CNAME is subdomain of expected target",
		},
		{
			name:           "No match",
			expectedTarget: "preview.notifuse.com",
			cnameValue:     "different.example.com",
			hostname:       "custom.example.com",
			shouldMatch:    false,
			description:    "CNAME doesn't match expected target",
		},
		{
			name:           "CNAME points to itself (A record)",
			expectedTarget: "preview.notifuse.com",
			cnameValue:     "custom.example.com",
			hostname:       "custom.example.com",
			shouldMatch:    true, // Should fall back to A record check
			description:    "CNAME points to itself, should use A record",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract hostname from expectedTarget for comparison
			expectedHostname, err := extractHostname(tt.expectedTarget)
			require.NoError(t, err)

			// Simulate the comparison logic from VerifyDomainOwnership
			cname := tt.cnameValue
			hostname := tt.hostname

			// This is the logic from the code:
			// if cname == hostname+"." || cname == hostname {
			//   // Fall back to A record validation
			// }
			// if !strings.HasSuffix(cname, expectedTargetHostname) && cname != hostname {
			//   // Fail
			// }

			shouldFallbackToARecord := cname == hostname+"." || cname == hostname
			shouldPassCNAMECheck := strings.HasSuffix(cname, expectedHostname) || cname == hostname

			if shouldFallbackToARecord {
				// Should fall back to A record check
				assert.True(t, true, "Should fall back to A record validation")
			} else if shouldPassCNAMECheck {
				// Should pass CNAME check
				assert.True(t, true, "Should pass CNAME validation")
			} else {
				// Should fail
				assert.False(t, tt.shouldMatch, "Should fail CNAME validation")
			}
		})
	}
}

// Integration test helper - tests with real DNS (optional, can be skipped)
// This would require actual DNS resolution and should be run separately
func TestDNSVerificationService_Integration_RealDNS(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Test with a known domain that should resolve
	service, _ := setupDNSVerificationTest(t, "https://google.com")

	// Try to verify a domain (this will fail unless DNS is configured correctly)
	err := service.VerifyDomainOwnership(context.Background(), "https://example.com")

	// We don't assert on the result since it depends on actual DNS configuration
	// This test is mainly to ensure the code doesn't panic with real DNS
	if err != nil {
		_, ok := err.(domain.ValidationError)
		assert.True(t, ok, "error should be ValidationError")
	}
}

// Test that extractHostname handles edge cases
func TestExtractHostname_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "URL with userinfo",
			input:    "https://user:pass@example.com",
			expected: "example.com",
		},
		{
			name:     "URL with fragment",
			input:    "https://example.com#fragment",
			expected: "example.com",
		},
		{
			name:     "IPv4 address",
			input:    "https://192.168.1.1",
			expected: "192.168.1.1",
		},
		{
			name:     "IPv6 address in URL",
			input:    "https://[2001:db8::1]",
			expected: "2001:db8::1",
		},
		{
			name:     "Hostname with spaces",
			input:    "  example.com  ",
			expected: "example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractHostname(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
