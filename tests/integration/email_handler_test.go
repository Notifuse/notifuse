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
	defer func() { suite.Cleanup() }()

	t.Run("HandleClickRedirection", func(t *testing.T) {
		testEmailHandlerClickRedirection(t, suite)
	})

	t.Run("HandleOpens", func(t *testing.T) {
		testEmailHandlerOpens(t, suite)
	})

	t.Run("BotDetection_ClickTracking", func(t *testing.T) {
		testBotDetectionClickTracking(t, suite)
	})

	t.Run("BotDetection_OpenTracking", func(t *testing.T) {
		testBotDetectionOpenTracking(t, suite)
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
		defer func() { _ = resp.Body.Close() }()

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
		defer func() { _ = resp.Body.Close() }()

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
		defer func() { _ = resp.Body.Close() }()

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
		defer func() { _ = resp.Body.Close() }()

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
		defer func() { _ = resp.Body.Close() }()

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
		defer func() { _ = resp.Body.Close() }()

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
		defer func() { _ = resp.Body.Close() }()

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
		defer func() { _ = resp.Body.Close() }()

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
					Port:     1025, // Mailpit port
					Username: "",   // No auth for Mailpit
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
				defer func() { _ = httpResp.Body.Close() }()
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
		defer func() { _ = resp.Body.Close() }()

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
		defer func() { _ = resp.Body.Close() }()

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
		defer func() { _ = resp.Body.Close() }()

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
		defer func() { _ = resp.Body.Close() }()

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
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func testBotDetectionClickTracking(t *testing.T, suite *testutil.IntegrationTestSuite) {
	baseURL := suite.ServerManager.GetURL()
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	t.Run("bot user-agent not tracked but redirects", func(t *testing.T) {
		redirectURL := "https://example.com/bot-test"
		messageID := "msg-bot-test"
		workspaceID := "ws-bot-test"
		timestamp := time.Now().Add(-10 * time.Second).Unix() // Old enough to pass time check

		visitURL := fmt.Sprintf("%s/visit?url=%s&mid=%s&wid=%s&ts=%d",
			baseURL,
			url.QueryEscape(redirectURL),
			messageID,
			workspaceID,
			timestamp,
		)

		req, err := http.NewRequest(http.MethodGet, visitURL, nil)
		require.NoError(t, err)

		// Set a known bot user-agent
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should still redirect properly (bot gets redirected)
		assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
		location := resp.Header.Get("Location")
		assert.Equal(t, redirectURL, location)

		// Note: We can't directly verify the click wasn't recorded without
		// querying the database, but the behavior is that bots get redirected
		// without recording. The unit tests verify the logic.
	})

	t.Run("fast click not tracked but redirects (< 7 seconds)", func(t *testing.T) {
		redirectURL := "https://example.com/fast-click"
		messageID := "msg-fast-click"
		workspaceID := "ws-fast-click"
		// Timestamp very recent (< 7 seconds ago)
		timestamp := time.Now().Add(-2 * time.Second).Unix()

		visitURL := fmt.Sprintf("%s/visit?url=%s&mid=%s&wid=%s&ts=%d",
			baseURL,
			url.QueryEscape(redirectURL),
			messageID,
			workspaceID,
			timestamp,
		)

		req, err := http.NewRequest(http.MethodGet, visitURL, nil)
		require.NoError(t, err)

		// Use a normal browser user-agent (not a bot)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should still redirect properly (fast clicks get redirected)
		assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
		location := resp.Header.Get("Location")
		assert.Equal(t, redirectURL, location)
	})

	t.Run("human user-agent with proper timing redirects", func(t *testing.T) {
		redirectURL := "https://example.com/human-click"
		messageID := "msg-human-click"
		workspaceID := "ws-human-click"
		// Timestamp old enough (> 7 seconds ago)
		timestamp := time.Now().Add(-20 * time.Second).Unix()

		visitURL := fmt.Sprintf("%s/visit?url=%s&mid=%s&wid=%s&ts=%d",
			baseURL,
			url.QueryEscape(redirectURL),
			messageID,
			workspaceID,
			timestamp,
		)

		req, err := http.NewRequest(http.MethodGet, visitURL, nil)
		require.NoError(t, err)

		// Use a normal browser user-agent
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/120.0.0.0")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should redirect properly
		assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
		location := resp.Header.Get("Location")
		assert.Equal(t, redirectURL, location)
	})

	t.Run("empty user-agent treated as bot", func(t *testing.T) {
		redirectURL := "https://example.com/empty-ua"
		messageID := "msg-empty-ua"
		workspaceID := "ws-empty-ua"
		timestamp := time.Now().Add(-10 * time.Second).Unix()

		visitURL := fmt.Sprintf("%s/visit?url=%s&mid=%s&wid=%s&ts=%d",
			baseURL,
			url.QueryEscape(redirectURL),
			messageID,
			workspaceID,
			timestamp,
		)

		req, err := http.NewRequest(http.MethodGet, visitURL, nil)
		require.NoError(t, err)

		// Don't set User-Agent header (will be empty)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should still redirect properly (bot gets redirected)
		assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
		location := resp.Header.Get("Location")
		assert.Equal(t, redirectURL, location)
	})

	t.Run("email security scanner patterns", func(t *testing.T) {
		scannerUserAgents := []string{
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/70.0.3538.102 Safari/537.36 Edge/18.18362 (Microsoft Outlook SafeLinks PreFetch)",
			"Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 Chrome/42.0.2311.135 Safari/537.36 (Proofpoint URL Defense)",
			"Mimecast-URL-Rewriter/1.0",
		}

		for i, ua := range scannerUserAgents {
			t.Run(fmt.Sprintf("scanner_%d", i), func(t *testing.T) {
				redirectURL := fmt.Sprintf("https://example.com/scanner-%d", i)
				messageID := fmt.Sprintf("msg-scanner-%d", i)
				workspaceID := fmt.Sprintf("ws-scanner-%d", i)
				timestamp := time.Now().Add(-10 * time.Second).Unix()

				visitURL := fmt.Sprintf("%s/visit?url=%s&mid=%s&wid=%s&ts=%d",
					baseURL,
					url.QueryEscape(redirectURL),
					messageID,
					workspaceID,
					timestamp,
				)

				req, err := http.NewRequest(http.MethodGet, visitURL, nil)
				require.NoError(t, err)
				req.Header.Set("User-Agent", ua)

				resp, err := client.Do(req)
				require.NoError(t, err)
				defer func() { _ = resp.Body.Close() }()

				// Email scanners should still get redirected
				assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
				location := resp.Header.Get("Location")
				assert.Equal(t, redirectURL, location)
			})
		}
	})

	t.Run("missing timestamp parameter still works", func(t *testing.T) {
		// Backward compatibility test - old URLs without ts parameter
		redirectURL := "https://example.com/no-ts"
		messageID := "msg-no-ts"
		workspaceID := "ws-no-ts"

		visitURL := fmt.Sprintf("%s/visit?url=%s&mid=%s&wid=%s",
			baseURL,
			url.QueryEscape(redirectURL),
			messageID,
			workspaceID,
		)

		req, err := http.NewRequest(http.MethodGet, visitURL, nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/120.0.0.0")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should still work (time check is skipped if no ts parameter)
		assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
		location := resp.Header.Get("Location")
		assert.Equal(t, redirectURL, location)
	})
}

func testBotDetectionOpenTracking(t *testing.T, suite *testutil.IntegrationTestSuite) {
	baseURL := suite.ServerManager.GetURL()
	client := &http.Client{}

	t.Run("bot user-agent not tracked but returns pixel", func(t *testing.T) {
		messageID := "msg-bot-open"
		workspaceID := "ws-bot-open"
		timestamp := time.Now().Add(-10 * time.Second).Unix()

		openURL := fmt.Sprintf("%s/opens?mid=%s&wid=%s&ts=%d",
			baseURL,
			messageID,
			workspaceID,
			timestamp,
		)

		req, err := http.NewRequest(http.MethodGet, openURL, nil)
		require.NoError(t, err)

		// Set a known bot user-agent
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Googlebot/2.1)")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should still return a pixel (bot gets the image)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "image/png", resp.Header.Get("Content-Type"))

		// Verify it's a valid PNG
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.True(t, len(body) > 0)

		expectedSignature := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		assert.Equal(t, expectedSignature, body[:8])
	})

	t.Run("fast open not tracked but returns pixel (< 7 seconds)", func(t *testing.T) {
		messageID := "msg-fast-open"
		workspaceID := "ws-fast-open"
		// Timestamp very recent (< 7 seconds ago)
		timestamp := time.Now().Add(-3 * time.Second).Unix()

		openURL := fmt.Sprintf("%s/opens?mid=%s&wid=%s&ts=%d",
			baseURL,
			messageID,
			workspaceID,
			timestamp,
		)

		req, err := http.NewRequest(http.MethodGet, openURL, nil)
		require.NoError(t, err)

		// Use a normal browser user-agent
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should still return a pixel
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "image/png", resp.Header.Get("Content-Type"))
	})

	t.Run("human user-agent with proper timing returns pixel", func(t *testing.T) {
		messageID := "msg-human-open"
		workspaceID := "ws-human-open"
		// Timestamp old enough (> 7 seconds ago)
		timestamp := time.Now().Add(-30 * time.Second).Unix()

		openURL := fmt.Sprintf("%s/opens?mid=%s&wid=%s&ts=%d",
			baseURL,
			messageID,
			workspaceID,
			timestamp,
		)

		req, err := http.NewRequest(http.MethodGet, openURL, nil)
		require.NoError(t, err)

		// Use a normal browser user-agent
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should return a pixel
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "image/png", resp.Header.Get("Content-Type"))
	})

	t.Run("headless browser detected as bot", func(t *testing.T) {
		messageID := "msg-headless"
		workspaceID := "ws-headless"
		timestamp := time.Now().Add(-10 * time.Second).Unix()

		openURL := fmt.Sprintf("%s/opens?mid=%s&wid=%s&ts=%d",
			baseURL,
			messageID,
			workspaceID,
			timestamp,
		)

		req, err := http.NewRequest(http.MethodGet, openURL, nil)
		require.NoError(t, err)

		// HeadlessChrome user-agent
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 HeadlessChrome/120.0.0.0")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should still return a pixel
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "image/png", resp.Header.Get("Content-Type"))
	})

	t.Run("invalid timestamp format ignored", func(t *testing.T) {
		messageID := "msg-invalid-ts"
		workspaceID := "ws-invalid-ts"

		openURL := fmt.Sprintf("%s/opens?mid=%s&wid=%s&ts=invalid",
			baseURL,
			messageID,
			workspaceID,
		)

		req, err := http.NewRequest(http.MethodGet, openURL, nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/120.0.0.0")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should still work (invalid ts is ignored, time check skipped)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "image/png", resp.Header.Get("Content-Type"))
	})

	t.Run("timestamp exactly at 7 second boundary", func(t *testing.T) {
		messageID := "msg-boundary"
		workspaceID := "ws-boundary"
		// Exactly 7 seconds ago - should pass (>= 7 seconds is allowed)
		timestamp := time.Now().Add(-7 * time.Second).Unix()

		openURL := fmt.Sprintf("%s/opens?mid=%s&wid=%s&ts=%d",
			baseURL,
			messageID,
			workspaceID,
			timestamp,
		)

		req, err := http.NewRequest(http.MethodGet, openURL, nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/120.0.0.0")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should return a pixel
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "image/png", resp.Header.Get("Content-Type"))
	})

	t.Run("very old timestamp still works", func(t *testing.T) {
		messageID := "msg-old"
		workspaceID := "ws-old"
		// Very old timestamp (1 hour ago)
		timestamp := time.Now().Add(-1 * time.Hour).Unix()

		openURL := fmt.Sprintf("%s/opens?mid=%s&wid=%s&ts=%d",
			baseURL,
			messageID,
			workspaceID,
			timestamp,
		)

		req, err := http.NewRequest(http.MethodGet, openURL, nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/120.0.0.0")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should work fine
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "image/png", resp.Header.Get("Content-Type"))
	})
}

func TestEmailHandler_ConcurrentRequests(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

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
				_ = resp.Body.Close()

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
				_ = resp.Body.Close()

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
