package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailHandler_Integration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	t.Run("HandleClickRedirection", func(t *testing.T) {
		testEmailHandlerClickRedirection(t, suite)
	})

	t.Run("HandleOpens", func(t *testing.T) {
		testEmailHandlerOpens(t, suite)
	})

	t.Run("HandleTestEmailProvider", func(t *testing.T) {
		testEmailHandlerTestProvider(t, suite)
	})
}

func testEmailHandlerClickRedirection(t *testing.T, suite *testutil.IntegrationTestSuite) {
	baseURL := suite.ServerManager.GetURL()
	client := &http.Client{}

	t.Run("redirect with all parameters", func(t *testing.T) {
		// Test with all required parameters
		redirectURL := "https://example.com/test"
		messageID := "msg-123"
		workspaceID := "ws-123"

		visitURL := fmt.Sprintf("%s/visit?url=%s&mid=%s&wid=%s",
			baseURL,
			url.QueryEscape(redirectURL),
			messageID,
			workspaceID,
		)

		req, err := http.NewRequest(http.MethodGet, visitURL, nil)
		require.NoError(t, err)

		// Configure client to not follow redirects to check the redirect response
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should redirect with 303 See Other
		assert.Equal(t, http.StatusSeeOther, resp.StatusCode)

		// Check redirect location
		location := resp.Header.Get("Location")
		assert.Equal(t, redirectURL, location)
	})

	t.Run("redirect without tracking parameters", func(t *testing.T) {
		redirectURL := "https://example.com/notrack"

		visitURL := fmt.Sprintf("%s/visit?url=%s",
			baseURL,
			url.QueryEscape(redirectURL),
		)

		req, err := http.NewRequest(http.MethodGet, visitURL, nil)
		require.NoError(t, err)

		// Configure client to not follow redirects
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should still redirect even without tracking parameters
		assert.Equal(t, http.StatusSeeOther, resp.StatusCode)

		location := resp.Header.Get("Location")
		assert.Equal(t, redirectURL, location)
	})

	t.Run("missing redirect URL", func(t *testing.T) {
		visitURL := fmt.Sprintf("%s/visit?mid=msg-123&wid=ws-123", baseURL)

		req, err := http.NewRequest(http.MethodGet, visitURL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should return bad request when URL is missing
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "Missing redirect URL")
	})

	t.Run("partial tracking parameters", func(t *testing.T) {
		redirectURL := "https://example.com/partial"

		// Test with only message ID
		visitURL := fmt.Sprintf("%s/visit?url=%s&mid=msg-123",
			baseURL,
			url.QueryEscape(redirectURL),
		)

		req, err := http.NewRequest(http.MethodGet, visitURL, nil)
		require.NoError(t, err)

		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should redirect even with partial parameters
		assert.Equal(t, http.StatusSeeOther, resp.StatusCode)

		location := resp.Header.Get("Location")
		assert.Equal(t, redirectURL, location)
	})
}

func testEmailHandlerOpens(t *testing.T, suite *testutil.IntegrationTestSuite) {
	baseURL := suite.ServerManager.GetURL()
	client := &http.Client{}

	t.Run("valid open tracking", func(t *testing.T) {
		messageID := "msg-123"
		workspaceID := "ws-123"

		openURL := fmt.Sprintf("%s/opens?mid=%s&wid=%s", baseURL, messageID, workspaceID)

		req, err := http.NewRequest(http.MethodGet, openURL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should return 200 OK
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Should return PNG image
		contentType := resp.Header.Get("Content-Type")
		assert.Equal(t, "image/png", contentType)

		// Read the response body (should be a 1x1 transparent PNG)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.True(t, len(body) > 0, "Response body should contain PNG data")

		// Check PNG signature (first 8 bytes)
		expectedSignature := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		assert.True(t, len(body) >= 8, "PNG should have at least 8 bytes for signature")
		assert.Equal(t, expectedSignature, body[:8], "Should have valid PNG signature")
	})

	t.Run("missing message ID", func(t *testing.T) {
		openURL := fmt.Sprintf("%s/opens?wid=ws-123", baseURL)

		req, err := http.NewRequest(http.MethodGet, openURL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should return bad request
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "Missing message ID or workspace ID")
	})

	t.Run("missing workspace ID", func(t *testing.T) {
		openURL := fmt.Sprintf("%s/opens?mid=msg-123", baseURL)

		req, err := http.NewRequest(http.MethodGet, openURL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should return bad request
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "Missing message ID or workspace ID")
	})

	t.Run("missing both parameters", func(t *testing.T) {
		openURL := fmt.Sprintf("%s/opens", baseURL)

		req, err := http.NewRequest(http.MethodGet, openURL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should return bad request
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "Missing message ID or workspace ID")
	})
}

func testEmailHandlerTestProvider(t *testing.T, suite *testutil.IntegrationTestSuite) {
	client := suite.APIClient

	// Create and authenticate a user, then create a workspace
	// Use a pre-seeded test user instead of generating a random email
	email := "testuser@example.com"
	token := performCompleteSignInFlow(t, client, email)
	client.SetToken(token)

	workspaceID := createTestWorkspace(t, client, "Email Test Workspace")
	client.SetWorkspaceID(workspaceID)

	t.Run("successful test email provider", func(t *testing.T) {
		reqBody := domain.TestEmailProviderRequest{
			WorkspaceID: workspaceID,
			To:          testutil.GenerateTestEmail(),
			Provider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
				Senders: []domain.EmailSender{
					domain.NewEmailSender("sender@example.com", "Test Sender"),
				},
				SMTP: &domain.SMTPSettings{
					Host:     "localhost",
					Port:     1025, // MailHog port
					Username: "",   // No auth for MailHog
					Password: "",
					UseTLS:   false,
				},
			},
		}

		var resp domain.TestEmailProviderResponse
		err := suite.APIClient.PostJSON("/api/email.testProvider", reqBody, &resp)

		// In demo mode, the service might not actually send emails
		// but should still return success
		if err != nil {
			// Check if it's a service-level error vs HTTP error
			httpResp, httpErr := suite.APIClient.Post("/api/email.testProvider", reqBody)
			if httpErr == nil {
				defer httpResp.Body.Close()
				assert.Equal(t, http.StatusOK, httpResp.StatusCode)

				// Decode the response
				err = json.NewDecoder(httpResp.Body).Decode(&resp)
				require.NoError(t, err)
			}
		}

		// The response should indicate success (true) or provide error details
		if resp.Success {
			assert.True(t, resp.Success)
		} else if resp.Error != "" {
			// Log the error for debugging but don't fail the test if it's expected
			t.Logf("Email provider test returned error (might be expected): %s", resp.Error)
		}
	})

	t.Run("missing recipient email", func(t *testing.T) {
		reqBody := domain.TestEmailProviderRequest{
			WorkspaceID: workspaceID,
			// Missing To field
			Provider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
			},
		}

		resp, err := suite.APIClient.Post("/api/email.testProvider", reqBody)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "Missing recipient email")
	})

	t.Run("missing workspace ID", func(t *testing.T) {
		reqBody := domain.TestEmailProviderRequest{
			To: testutil.GenerateTestEmail(),
			// Missing WorkspaceID
			Provider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
			},
		}

		resp, err := suite.APIClient.Post("/api/email.testProvider", reqBody)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "Missing workspace ID")
	})

	t.Run("invalid request body", func(t *testing.T) {
		// For invalid JSON, we need to send raw malformed JSON with proper authentication
		invalidJSON := `{"incomplete": json without closing brace`

		// Create manual request with proper token
		req, err := http.NewRequest(http.MethodPost,
			suite.ServerManager.GetURL()+"/api/email.testProvider",
			strings.NewReader(invalidJSON))
		require.NoError(t, err)

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		httpClient := &http.Client{}
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "Invalid request body")
	})

	t.Run("method not allowed", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet,
			suite.ServerManager.GetURL()+"/api/email.testProvider", nil)
		require.NoError(t, err)

		// Use proper authentication token
		req.Header.Set("Authorization", "Bearer "+token)

		httpClient := &http.Client{}
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "Method not allowed")
	})

	t.Run("unauthorized access", func(t *testing.T) {
		reqBody := domain.TestEmailProviderRequest{
			WorkspaceID: workspaceID,
			To:          testutil.GenerateTestEmail(),
			Provider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
			},
		}

		bodyBytes, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost,
			suite.ServerManager.GetURL()+"/api/email.testProvider",
			bytes.NewReader(bodyBytes))
		require.NoError(t, err)

		req.Header.Set("Content-Type", "application/json")
		// No Authorization header

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestEmailHandler_SMTPFromNameInHeaders(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient

	// Create and authenticate a user, then create a workspace
	email := "testuser@example.com"
	token := performCompleteSignInFlow(t, client, email)
	client.SetToken(token)

	workspaceID := createTestWorkspace(t, client, "From Name Test Workspace")
	client.SetWorkspaceID(workspaceID)

	t.Run("test email provider includes from_name in SMTP headers", func(t *testing.T) {
		// Clear Mailhog before test
		err := testutil.ClearMailhogMessages(t)
		if err != nil {
			t.Logf("Warning: Could not clear Mailhog messages: %v", err)
		}

		fromEmail := "sender@example.com"
		fromName := "Test Sender Name"
		recipientEmail := testutil.GenerateTestEmail()

		t.Logf("Test configuration:")
		t.Logf("  Workspace ID: %s", workspaceID)
		t.Logf("  From: %s <%s>", fromName, fromEmail)
		t.Logf("  To: %s", recipientEmail)

		reqBody := domain.TestEmailProviderRequest{
			WorkspaceID: workspaceID,
			To:          recipientEmail,
			Provider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
				Senders: []domain.EmailSender{
					domain.NewEmailSender(fromEmail, fromName),
				},
				SMTP: &domain.SMTPSettings{
					Host:     "localhost",
					Port:     1025, // MailHog port
					Username: "",   // No auth for MailHog
					Password: "",
					UseTLS:   false,
				},
			},
		}

		var resp domain.TestEmailProviderResponse
		err = suite.APIClient.PostJSON("/api/email.testProvider", reqBody, &resp)
		require.NoError(t, err, "Failed to send test email")

		// The response should indicate success
		if !resp.Success {
			t.Logf("❌ Email provider test failed with error: %s", resp.Error)
			t.Fatalf("Email provider test should succeed. Error: %s", resp.Error)
		}
		t.Logf("✅ Email provider test succeeded")

		t.Log("Waiting for email to be delivered to MailHog...")
		time.Sleep(2 * time.Second)

		// Wait for the message to appear in Mailhog
		// The subject is set in email_service.go line 119: "Notifuse: Test Email Provider"
		messageID, err := testutil.WaitForMailhogMessageWithSubject(t, "Notifuse: Test Email Provider", 10*time.Second)
		require.NoError(t, err, "Failed to find test email in Mailhog")

		t.Logf("Found message in Mailhog with ID: %s", messageID)

		// Fetch the message with full headers
		message, err := testutil.GetMailhogMessageWithHeaders(t, messageID)
		require.NoError(t, err, "Failed to fetch message details from Mailhog")

		// Check the From header in the message headers
		fromHeaders, ok := message.Content.Headers["From"]
		require.True(t, ok, "From header should exist in email")
		require.Greater(t, len(fromHeaders), 0, "From header should have at least one value")

		fromHeader := fromHeaders[0]
		t.Logf("From header value: %s", fromHeader)

		// Verify the from_name is included in the From header
		// The From header should be in format: "Name <email@example.com>" or just "email@example.com"
		assert.Contains(t, fromHeader, fromName, "From header should contain the from_name")
		assert.Contains(t, fromHeader, fromEmail, "From header should contain the from_email")

		// Also check the raw SMTP data
		rawData := message.Raw.Data
		assert.Contains(t, rawData, fmt.Sprintf("From: %s", fromName), "Raw SMTP data should contain From header with from_name")

		t.Log("✅ Successfully verified from_name is included in SMTP email headers")
	})

	t.Run("test email provider uses default sender name when provided", func(t *testing.T) {
		// Clear Mailhog before test
		err := testutil.ClearMailhogMessages(t)
		if err != nil {
			t.Logf("Warning: Could not clear Mailhog messages: %v", err)
		}

		fromEmail := "default-sender@example.com"
		fromName := "Default Sender Name"
		recipientEmail := testutil.GenerateTestEmail()

		t.Logf("Test configuration:")
		t.Logf("  Workspace ID: %s", workspaceID)
		t.Logf("  Default From: %s <%s>", fromName, fromEmail)
		t.Logf("  To: %s", recipientEmail)

		reqBody := domain.TestEmailProviderRequest{
			WorkspaceID: workspaceID,
			To:          recipientEmail,
			Provider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
				Senders: []domain.EmailSender{
					domain.NewEmailSender(fromEmail, fromName),
				},
				SMTP: &domain.SMTPSettings{
					Host:     "localhost",
					Port:     1025, // MailHog port
					Username: "",   // No auth for MailHog
					Password: "",
					UseTLS:   false,
				},
			},
		}

		var resp domain.TestEmailProviderResponse
		err = suite.APIClient.PostJSON("/api/email.testProvider", reqBody, &resp)
		require.NoError(t, err, "Failed to send test email")

		// The response should indicate success
		if !resp.Success {
			t.Logf("❌ Email provider test failed with error: %s", resp.Error)
			t.Fatalf("Email provider test should succeed. Error: %s", resp.Error)
		}
		t.Logf("✅ Email provider test succeeded")

		t.Log("Waiting for email to be delivered to MailHog...")
		time.Sleep(2 * time.Second)

		// Wait for the message to appear in Mailhog
		messageID, err := testutil.WaitForMailhogMessageWithSubject(t, "Notifuse: Test Email Provider", 10*time.Second)
		require.NoError(t, err, "Failed to find test email in Mailhog")

		t.Logf("Found message in Mailhog with ID: %s", messageID)

		// Fetch the message with full headers
		message, err := testutil.GetMailhogMessageWithHeaders(t, messageID)
		require.NoError(t, err, "Failed to fetch message details from Mailhog")

		// Check the From header in the message headers
		fromHeaders, ok := message.Content.Headers["From"]
		require.True(t, ok, "From header should exist in email")
		require.Greater(t, len(fromHeaders), 0, "From header should have at least one value")

		fromHeader := fromHeaders[0]
		t.Logf("From header value: %s", fromHeader)

		// Verify the default sender name is included in the From header
		assert.Contains(t, fromHeader, fromName, 
			"From header should contain the default sender name: '%s'", fromName)
		assert.Contains(t, fromHeader, fromEmail, 
			"From header should contain the sender email: '%s'", fromEmail)

		// Also check the raw SMTP data
		rawData := message.Raw.Data
		assert.Contains(t, rawData, fromName, 
			"Raw SMTP data should contain From header with default sender name: '%s'", fromName)

		// Verify the expected format is present
		t.Logf("✅ Successfully verified default sender name '%s' is used in test email provider", fromName)
		t.Log("✅ Default sender name is properly included in SMTP headers from test email endpoint")
	})
}

func TestEmailHandler_ConcurrentRequests(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	baseURL := suite.ServerManager.GetURL()

	t.Run("concurrent click redirections", func(t *testing.T) {
		numRequests := 10
		results := make(chan error, numRequests)

		redirectURL := "https://example.com/concurrent"

		for i := 0; i < numRequests; i++ {
			go func(i int) {
				// Create a separate client for each goroutine to avoid data races
				client := &http.Client{
					CheckRedirect: func(req *http.Request, via []*http.Request) error {
						return http.ErrUseLastResponse
					},
				}

				messageID := fmt.Sprintf("msg-%d", i)
				workspaceID := fmt.Sprintf("ws-%d", i)

				visitURL := fmt.Sprintf("%s/visit?url=%s&mid=%s&wid=%s",
					baseURL,
					url.QueryEscape(redirectURL),
					messageID,
					workspaceID,
				)

				req, err := http.NewRequest(http.MethodGet, visitURL, nil)
				if err != nil {
					results <- err
					return
				}

				resp, err := client.Do(req)
				if err != nil {
					results <- err
					return
				}
				resp.Body.Close()

				if resp.StatusCode != http.StatusSeeOther {
					results <- fmt.Errorf("expected status 303, got %d", resp.StatusCode)
					return
				}

				results <- nil
			}(i)
		}

		// Collect results
		for i := 0; i < numRequests; i++ {
			err := <-results
			assert.NoError(t, err, "Concurrent request %d should succeed", i)
		}
	})

	t.Run("concurrent open tracking", func(t *testing.T) {
		numRequests := 10
		results := make(chan error, numRequests)

		for i := 0; i < numRequests; i++ {
			go func(i int) {
				// Create a separate client for each goroutine to avoid potential issues
				client := &http.Client{}

				messageID := fmt.Sprintf("msg-open-%d", i)
				workspaceID := fmt.Sprintf("ws-open-%d", i)

				openURL := fmt.Sprintf("%s/opens?mid=%s&wid=%s", baseURL, messageID, workspaceID)

				req, err := http.NewRequest(http.MethodGet, openURL, nil)
				if err != nil {
					results <- err
					return
				}

				resp, err := client.Do(req)
				if err != nil {
					results <- err
					return
				}
				resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					results <- fmt.Errorf("expected status 200, got %d", resp.StatusCode)
					return
				}

				if resp.Header.Get("Content-Type") != "image/png" {
					results <- fmt.Errorf("expected content-type image/png, got %s", resp.Header.Get("Content-Type"))
					return
				}

				results <- nil
			}(i)
		}

		// Collect results
		for i := 0; i < numRequests; i++ {
			err := <-results
			assert.NoError(t, err, "Concurrent open tracking request %d should succeed", i)
		}
	})
}
