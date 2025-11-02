package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	svix "github.com/standard-webhooks/standard-webhooks/libraries/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSupabaseIntegrationE2E tests the complete Supabase integration flow end-to-end
// This test covers:
// - Complete installation that creates templates and transactional notifications
// - Webhook handling for all auth email types
// - Webhook handling for "before user created" that upserts contacts and subscribes to lists
// - Proper payload structure validation matching JSON schema specs
// - Webhook signature validation
func TestSupabaseIntegrationE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	// Create test user and workspace
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	// Add user to workspace as owner
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Set up authentication
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("Complete Installation Flow", func(t *testing.T) {
		testSupabaseInstallation(t, suite, client, workspace.ID)
	})

	t.Run("Auth Email Webhooks", func(t *testing.T) {
		testAuthEmailWebhooks(t, suite, workspace.ID)
	})

	t.Run("Before User Created Webhook", func(t *testing.T) {
		testBeforeUserCreatedWebhook(t, suite, workspace.ID)
	})

	t.Run("Webhook Payload Validation", func(t *testing.T) {
		testWebhookPayloadValidation(t, suite, workspace.ID)
	})

	t.Run("Webhook Signature Validation", func(t *testing.T) {
		testWebhookSignatureValidation(t, suite, workspace.ID)
	})

	t.Run("Reject Disposable Email", func(t *testing.T) {
		testRejectDisposableEmail(t, suite, workspace.ID)
	})
}

// testSupabaseInstallation tests the complete Supabase integration installation
func testSupabaseInstallation(t *testing.T, suite *testutil.IntegrationTestSuite, client *testutil.APIClient, workspaceID string) {
	// Create a list for testing the before-user-created hook
	testList, err := suite.DataFactory.CreateList(workspaceID)
	require.NoError(t, err)

	// Signature keys for webhook validation (standard-webhooks format)
	authEmailSignatureKey := generateWebhookSecret()
	userCreatedSignatureKey := generateWebhookSecret()

	// Create Supabase integration
	createReq := domain.CreateIntegrationRequest{
		WorkspaceID: workspaceID,
		Name:        "Test Supabase Integration",
		Type:        domain.IntegrationTypeSupabase,
		SupabaseSettings: &domain.SupabaseIntegrationSettings{
			AuthEmailHook: domain.SupabaseAuthEmailHookSettings{
				SignatureKey: authEmailSignatureKey,
			},
			BeforeUserCreatedHook: domain.SupabaseUserCreatedHookSettings{
				SignatureKey:    userCreatedSignatureKey,
				AddUserToLists:  []string{testList.ID},
				CustomJSONField: "custom_json_1",
			},
		},
	}

	resp, err := client.Post("/api/workspaces.createIntegration", createReq)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Contains(t, response, "integration_id")
	integrationID := response["integration_id"].(string)
	assert.NotEmpty(t, integrationID)

	// Store integration ID in context for other tests
	suite.T.Logf("Created Supabase integration with ID: %s", integrationID)

	// Verify workspace integration settings
	t.Run("Verify Integration Settings", func(t *testing.T) {
		getResp, err := client.Get("/api/workspaces.get", map[string]string{
			"id": workspaceID,
		})
		require.NoError(t, err)
		defer getResp.Body.Close()

		var getResponse map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&getResponse)
		require.NoError(t, err)

		workspaceData := getResponse["workspace"].(map[string]interface{})
		integrations := workspaceData["integrations"].([]interface{})
		assert.Len(t, integrations, 1)

		integration := integrations[0].(map[string]interface{})
		assert.Equal(t, integrationID, integration["id"])
		assert.Equal(t, "Test Supabase Integration", integration["name"])
		assert.Equal(t, "supabase", integration["type"])

		// Verify Supabase settings exist
		supabaseSettings := integration["supabase_settings"].(map[string]interface{})
		assert.NotNil(t, supabaseSettings)

		// Verify auth email hook settings
		authEmailHook := supabaseSettings["auth_email_hook"].(map[string]interface{})
		assert.NotEmpty(t, authEmailHook["encrypted_signature_key"])
		assert.NotEmpty(t, authEmailHook["signature_key"], "decrypted signature_key should be in response for now")

		// Verify before user created hook settings
		beforeUserCreatedHook := supabaseSettings["before_user_created_hook"].(map[string]interface{})
		assert.NotEmpty(t, beforeUserCreatedHook["encrypted_signature_key"])
		assert.NotEmpty(t, beforeUserCreatedHook["signature_key"], "decrypted signature_key should be in response for now")
		assert.Equal(t, "custom_json_1", beforeUserCreatedHook["custom_json_field"])

		addUserToLists := beforeUserCreatedHook["add_user_to_lists"].([]interface{})
		assert.Len(t, addUserToLists, 1)
		assert.Equal(t, testList.ID, addUserToLists[0])
	})

	// Verify templates were created
	t.Run("Verify Templates Created", func(t *testing.T) {
		// Get all templates for the workspace
		systemCtx := context.WithValue(context.Background(), domain.SystemCallKey, true)
		templates, err := suite.ServerManager.GetApp().GetTemplateRepository().GetTemplates(systemCtx, workspaceID, "")
		require.NoError(t, err)

		// Filter templates by integration_id
		var supabaseTemplates []domain.Template
		for _, template := range templates {
			if template.IntegrationID != nil && *template.IntegrationID == integrationID {
				supabaseTemplates = append(supabaseTemplates, *template)
			}
		}

		// Should have 6 templates
		assert.Len(t, supabaseTemplates, 6, "Expected 6 Supabase templates to be created")

		// Verify each template type exists
		expectedTemplates := map[string]string{
			"signup":       "Supabase Signup Confirmation",
			"magiclink":    "Supabase Magic Link",
			"recovery":     "Supabase Password Recovery",
			"email_change": "Supabase Email Change",
			"invite":       "Supabase User Invitation",
			"reauth":       "Supabase Reauthentication",
		}

		foundTemplates := make(map[string]bool)
		for _, template := range supabaseTemplates {
			// Check if template ID starts with expected prefix
			for actionType, expectedName := range expectedTemplates {
				if strings.HasPrefix(template.ID, "supabase_"+actionType+"_") {
					foundTemplates[actionType] = true
					assert.Equal(t, expectedName, template.Name)
					assert.Equal(t, "email", template.Channel)
					assert.Equal(t, "transactional", template.Category)
					assert.NotNil(t, template.Email)
					assert.NotNil(t, template.Email.VisualEditorTree)
					break
				}
			}
		}

		// Verify all expected templates were found
		for actionType := range expectedTemplates {
			assert.True(t, foundTemplates[actionType], "Template for action type '%s' not found", actionType)
		}
	})

	// Verify transactional notifications were created
	t.Run("Verify Transactional Notifications Created", func(t *testing.T) {
		// Get all transactional notifications for the workspace
		systemCtx := context.WithValue(context.Background(), domain.SystemCallKey, true)
		notifications, _, err := suite.ServerManager.GetApp().GetTransactionalNotificationRepository().List(systemCtx, workspaceID, map[string]interface{}{}, 1000, 0)
		require.NoError(t, err)

		// Filter notifications by integration_id
		var supabaseNotifications []domain.TransactionalNotification
		for _, notification := range notifications {
			if notification.IntegrationID != nil && *notification.IntegrationID == integrationID {
				supabaseNotifications = append(supabaseNotifications, *notification)
			}
		}

		// Should have 6 notifications
		assert.Len(t, supabaseNotifications, 6, "Expected 6 Supabase transactional notifications to be created")

		// Verify each notification type exists
		expectedNotifications := map[string]string{
			"signup":       "Supabase Signup Confirmation",
			"magiclink":    "Supabase Magic Link",
			"recovery":     "Supabase Password Recovery",
			"email_change": "Supabase Email Change",
			"invite":       "Supabase User Invitation",
			"reauth":       "Supabase Reauthentication",
		}

		foundNotifications := make(map[string]domain.TransactionalNotification)
		for _, notification := range supabaseNotifications {
			// Check if notification ID starts with expected prefix
			for actionType, expectedName := range expectedNotifications {
				if strings.HasPrefix(notification.ID, "supabase_"+actionType+"_") {
					foundNotifications[actionType] = notification
					assert.Equal(t, expectedName, notification.Name)
					assert.NotNil(t, notification.IntegrationID)
					assert.Equal(t, integrationID, *notification.IntegrationID)

					// Verify channels are configured
					assert.Contains(t, notification.Channels, domain.TransactionalChannelEmail)
					emailChannel := notification.Channels[domain.TransactionalChannelEmail]
					assert.NotEmpty(t, emailChannel.TemplateID)

					// Verify tracking is disabled
					assert.False(t, notification.TrackingSettings.EnableTracking, "Tracking should be disabled for Supabase notifications")
					break
				}
			}
		}

		// Verify all expected notifications were found
		for actionType := range expectedNotifications {
			assert.Contains(t, foundNotifications, actionType, "Notification for action type '%s' not found", actionType)
		}

		// Verify notification templates link to the created templates
		systemCtx = context.WithValue(context.Background(), domain.SystemCallKey, true)
		templates, err := suite.ServerManager.GetApp().GetTemplateRepository().GetTemplates(systemCtx, workspaceID, "")
		require.NoError(t, err)

		// Build a map of template IDs
		templateIDs := make(map[string]bool)
		for _, template := range templates {
			templateIDs[template.ID] = true
		}

		// Verify each notification references a valid template
		for actionType, notification := range foundNotifications {
			emailChannel := notification.Channels[domain.TransactionalChannelEmail]
			assert.True(t, templateIDs[emailChannel.TemplateID],
				"Notification '%s' references non-existent template '%s'",
				actionType, emailChannel.TemplateID)
		}
	})
}

// testAuthEmailWebhooks tests webhook handling for all auth email types
func testAuthEmailWebhooks(t *testing.T, suite *testutil.IntegrationTestSuite, workspaceID string) {
	// First create a Supabase integration for webhook testing
	signatureKey := generateWebhookSecret()
	integrationID := setupSupabaseIntegrationForWebhooks(t, suite, workspaceID, signatureKey)

	// Setup SMTP provider for email sending
	_, err := suite.DataFactory.SetupWorkspaceWithSMTPProvider(workspaceID)
	require.NoError(t, err)

	testCases := []struct {
		name            string
		emailActionType string
		emailNew        string // For email_change only
		tokenNew        string // For email_change secure mode
		tokenHashNew    string // For email_change secure mode
	}{
		{
			name:            "Signup Confirmation",
			emailActionType: "signup",
		},
		{
			name:            "Magic Link",
			emailActionType: "magiclink",
		},
		{
			name:            "Password Recovery",
			emailActionType: "recovery",
		},
		{
			name:            "User Invitation",
			emailActionType: "invite",
		},
		{
			name:            "Email Change (Secure Mode)",
			emailActionType: "email_change",
			emailNew:        "newemail@example.com",
			tokenNew:        "token_new_" + uuid.New().String(),
			tokenHashNew:    "hash_new_" + uuid.New().String(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build webhook payload
			user := domain.SupabaseUser{
				ID:           uuid.New().String(),
				Aud:          "authenticated",
				Role:         "authenticated",
				Email:        "user@example.com",
				EmailNew:     tc.emailNew,
				Phone:        "+1234567890",
				AppMetadata:  map[string]interface{}{"provider": "email", "providers": []string{"email"}},
				UserMetadata: map[string]interface{}{"name": "Test User"},
				Identities:   []interface{}{},
				CreatedAt:    time.Now().UTC().Format(time.RFC3339),
				UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
				IsAnonymous:  false,
			}

			emailData := domain.SupabaseEmailData{
				Token:           "token_" + uuid.New().String(),
				TokenHash:       "hash_" + uuid.New().String(),
				TokenNew:        tc.tokenNew,
				TokenHashNew:    tc.tokenHashNew,
				RedirectTo:      "https://example.com/redirect",
				EmailActionType: tc.emailActionType,
				SiteURL:         "https://example.com",
			}

			webhook := domain.SupabaseAuthEmailWebhook{
				User:      user,
				EmailData: emailData,
			}

			payload, err := json.Marshal(webhook)
			require.NoError(t, err)

			// Generate webhook headers
			webhookID := "webhook_" + uuid.New().String()
			timestamp := time.Now().Unix()
			timestampStr := strconv.FormatInt(timestamp, 10)

			// Sign the payload
			wh, err := svix.NewWebhook(signatureKey)
			require.NoError(t, err)
			signature, err := wh.Sign(webhookID, time.Unix(timestamp, 0), payload)
			require.NoError(t, err)

			// Send webhook request
			url := fmt.Sprintf("/webhooks/supabase/auth-email?workspace_id=%s&integration_id=%s", workspaceID, integrationID)
			req, err := http.NewRequest(http.MethodPost, suite.ServerManager.GetURL()+url, bytes.NewReader(payload))
			require.NoError(t, err)

			req.Header.Set("webhook-id", webhookID)
			req.Header.Set("webhook-timestamp", timestampStr)
			req.Header.Set("webhook-signature", signature)
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Verify response
			assert.Equal(t, http.StatusOK, resp.StatusCode, "Webhook should be accepted with valid signature")

			var responseBody map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&responseBody)
			require.NoError(t, err)
			assert.True(t, responseBody["success"].(bool))

			// For email_change in secure mode, verify both emails would be sent
			// (We can't easily verify actual email sending in integration tests without a mock email service)
			// But we can verify the webhook was processed successfully
		})
	}
}

// testBeforeUserCreatedWebhook tests the before user created webhook
func testBeforeUserCreatedWebhook(t *testing.T, suite *testutil.IntegrationTestSuite, workspaceID string) {
	// Create a test list for subscription
	testList, err := suite.DataFactory.CreateList(workspaceID)
	require.NoError(t, err)

	// Create Supabase integration with user created hook
	signatureKey := generateWebhookSecret()
	integrationID := setupSupabaseIntegrationWithUserCreatedHook(t, suite, workspaceID, signatureKey, testList.ID)

	t.Run("Valid Payload with All Fields", func(t *testing.T) {
		// Build webhook payload matching the JSON schema
		userEmail := fmt.Sprintf("supabase-user-%s@example.com", uuid.New().String()[:8])
		userID := uuid.New().String()
		userPhone := "+15551234567"

		metadata := domain.SupabaseWebhookMetadata{
			UUID:      uuid.New().String(),
			Time:      time.Now().UTC().Format(time.RFC3339),
			Name:      "before-user-created",
			IPAddress: "192.168.1.100",
		}

		user := domain.SupabaseUser{
			ID:    userID,
			Aud:   "authenticated",
			Role:  "authenticated",
			Email: userEmail,
			Phone: userPhone,
			AppMetadata: map[string]interface{}{
				"provider":  "email",
				"providers": []string{"email"},
			},
			UserMetadata: map[string]interface{}{
				"first_name": "John",
				"last_name":  "Doe",
				"age":        30,
			},
			Identities:  []interface{}{},
			CreatedAt:   time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
			IsAnonymous: false,
		}

		webhook := domain.SupabaseBeforeUserCreatedWebhook{
			Metadata: metadata,
			User:     user,
		}

		payload, err := json.Marshal(webhook)
		require.NoError(t, err)

		// Verify payload structure matches JSON schema by unmarshaling it back
		var verifyWebhook domain.SupabaseBeforeUserCreatedWebhook
		err = json.Unmarshal(payload, &verifyWebhook)
		require.NoError(t, err)

		// Verify all required metadata fields
		assert.NotEmpty(t, verifyWebhook.Metadata.UUID)
		assert.NotEmpty(t, verifyWebhook.Metadata.Time)
		assert.NotEmpty(t, verifyWebhook.Metadata.IPAddress)
		assert.Equal(t, "before-user-created", verifyWebhook.Metadata.Name)

		// Verify all required user fields
		assert.NotEmpty(t, verifyWebhook.User.ID)
		assert.NotEmpty(t, verifyWebhook.User.Aud)
		assert.NotEmpty(t, verifyWebhook.User.Role)
		assert.NotEmpty(t, verifyWebhook.User.Email)
		assert.NotEmpty(t, verifyWebhook.User.Phone)
		assert.NotNil(t, verifyWebhook.User.AppMetadata)
		assert.NotNil(t, verifyWebhook.User.UserMetadata)
		assert.NotNil(t, verifyWebhook.User.Identities)
		assert.NotEmpty(t, verifyWebhook.User.CreatedAt)
		assert.NotEmpty(t, verifyWebhook.User.UpdatedAt)

		// Generate webhook headers
		webhookID := "webhook_" + uuid.New().String()
		timestamp := time.Now().Unix()
		timestampStr := strconv.FormatInt(timestamp, 10)

		// Sign the payload
		wh, err := svix.NewWebhook(signatureKey)
		require.NoError(t, err)
		signature, err := wh.Sign(webhookID, time.Unix(timestamp, 0), payload)
		require.NoError(t, err)

		// Send webhook request
		url := fmt.Sprintf("/webhooks/supabase/before-user-created?workspace_id=%s&integration_id=%s", workspaceID, integrationID)
		req, err := http.NewRequest(http.MethodPost, suite.ServerManager.GetURL()+url, bytes.NewReader(payload))
		require.NoError(t, err)

		req.Header.Set("webhook-id", webhookID)
		req.Header.Set("webhook-timestamp", timestampStr)
		req.Header.Set("webhook-signature", signature)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Verify response - should always return 204 No Content
		assert.Equal(t, http.StatusNoContent, resp.StatusCode, "Webhook should return 204 to not block user creation")

		// Verify contact was created/upserted
		systemCtx := context.WithValue(context.Background(), domain.SystemCallKey, true)
		contact, err := suite.ServerManager.GetApp().GetContactRepository().GetContactByEmail(systemCtx, workspaceID, userEmail)
		require.NoError(t, err, "Contact should be created from Supabase user")

		assert.Equal(t, userEmail, contact.Email)
		assert.NotNil(t, contact.ExternalID)
		assert.Equal(t, userID, contact.ExternalID.String)
		assert.NotNil(t, contact.Phone)
		assert.Equal(t, userPhone, contact.Phone.String)

		// Verify user metadata was mapped to custom_json_1
		assert.NotNil(t, contact.CustomJSON1)
		assert.False(t, contact.CustomJSON1.IsNull)
		metadata1, ok := contact.CustomJSON1.Data.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "John", metadata1["first_name"])
		assert.Equal(t, "Doe", metadata1["last_name"])
		assert.Equal(t, float64(30), metadata1["age"]) // JSON numbers are float64

		// Verify contact was subscribed to the list
		contactList, err := suite.ServerManager.GetApp().GetContactListRepository().GetContactListByIDs(systemCtx, workspaceID, userEmail, testList.ID)
		require.NoError(t, err, "Contact should be subscribed to the configured list")
		assert.Equal(t, domain.ContactListStatusActive, contactList.Status)
	})

	t.Run("Missing Required Fields Still Returns 204", func(t *testing.T) {
		// Build invalid webhook payload (missing phone, which is required in schema)
		metadata := domain.SupabaseWebhookMetadata{
			UUID:      uuid.New().String(),
			Time:      time.Now().UTC().Format(time.RFC3339),
			Name:      "before-user-created",
			IPAddress: "192.168.1.100",
		}

		user := domain.SupabaseUser{
			ID:           uuid.New().String(),
			Aud:          "authenticated",
			Role:         "authenticated",
			Email:        fmt.Sprintf("invalid-%s@example.com", uuid.New().String()[:8]),
			Phone:        "", // Empty phone
			AppMetadata:  map[string]interface{}{"provider": "email", "providers": []string{"email"}},
			UserMetadata: map[string]interface{}{},
			Identities:   []interface{}{},
			CreatedAt:    time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
			IsAnonymous:  false,
		}

		webhook := domain.SupabaseBeforeUserCreatedWebhook{
			Metadata: metadata,
			User:     user,
		}

		payload, err := json.Marshal(webhook)
		require.NoError(t, err)

		// Generate webhook headers
		webhookID := "webhook_" + uuid.New().String()
		timestamp := time.Now().Unix()
		timestampStr := strconv.FormatInt(timestamp, 10)

		// Sign the payload
		wh, err := svix.NewWebhook(signatureKey)
		require.NoError(t, err)
		signature, err := wh.Sign(webhookID, time.Unix(timestamp, 0), payload)
		require.NoError(t, err)

		// Send webhook request
		url := fmt.Sprintf("/webhooks/supabase/before-user-created?workspace_id=%s&integration_id=%s", workspaceID, integrationID)
		req, err := http.NewRequest(http.MethodPost, suite.ServerManager.GetURL()+url, bytes.NewReader(payload))
		require.NoError(t, err)

		req.Header.Set("webhook-id", webhookID)
		req.Header.Set("webhook-timestamp", timestampStr)
		req.Header.Set("webhook-signature", signature)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should still return 204 even with processing errors
		assert.Equal(t, http.StatusNoContent, resp.StatusCode, "Webhook should return 204 even on errors to not block user creation")
	})
}

// testWebhookPayloadValidation tests payload structure validation
func testWebhookPayloadValidation(t *testing.T, suite *testutil.IntegrationTestSuite, workspaceID string) {
	signatureKey := generateWebhookSecret()
	integrationID := setupSupabaseIntegrationForWebhooks(t, suite, workspaceID, signatureKey)

	t.Run("Auth Email Hook - Missing Required Fields", func(t *testing.T) {
		// Invalid payload - missing email_data
		payload := []byte(`{
			"user": {
				"id": "test-id",
				"aud": "authenticated",
				"role": "authenticated",
				"email": "test@example.com",
				"phone": "+1234567890",
				"app_metadata": {"provider": "email", "providers": ["email"]},
				"user_metadata": {},
				"identities": [],
				"created_at": "2024-01-01T00:00:00Z",
				"updated_at": "2024-01-01T00:00:00Z",
				"is_anonymous": false
			}
		}`)

		// Generate valid signature for invalid payload
		webhookID := "webhook_" + uuid.New().String()
		timestamp := time.Now().Unix()
		timestampStr := strconv.FormatInt(timestamp, 10)

		wh, err := svix.NewWebhook(signatureKey)
		require.NoError(t, err)
		signature, err := wh.Sign(webhookID, time.Unix(timestamp, 0), payload)
		require.NoError(t, err)

		// Send webhook request
		url := fmt.Sprintf("/webhooks/supabase/auth-email?workspace_id=%s&integration_id=%s", workspaceID, integrationID)
		req, err := http.NewRequest(http.MethodPost, suite.ServerManager.GetURL()+url, bytes.NewReader(payload))
		require.NoError(t, err)

		req.Header.Set("webhook-id", webhookID)
		req.Header.Set("webhook-timestamp", timestampStr)
		req.Header.Set("webhook-signature", signature)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should return 400 for invalid payload structure
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Before User Created - Extra Fields Ignored", func(t *testing.T) {
		// Create integration with user created hook
		testList, err := suite.DataFactory.CreateList(workspaceID)
		require.NoError(t, err)
		userCreatedIntegrationID := setupSupabaseIntegrationWithUserCreatedHook(t, suite, workspaceID, signatureKey, testList.ID)

		// Payload with extra fields
		payload := []byte(fmt.Sprintf(`{
			"metadata": {
				"uuid": "%s",
				"time": "%s",
				"name": "before-user-created",
				"ip_address": "192.168.1.1",
				"extra_field": "should be ignored"
			},
			"user": {
				"id": "%s",
				"aud": "authenticated",
				"role": "authenticated",
				"email": "extra-fields-%s@example.com",
				"phone": "+1234567890",
				"app_metadata": {"provider": "email", "providers": ["email"]},
				"user_metadata": {"name": "Test"},
				"identities": [],
				"created_at": "%s",
				"updated_at": "%s",
				"is_anonymous": false,
				"extra_user_field": "should also be ignored"
			},
			"another_extra_field": "ignored"
		}`, uuid.New().String(), time.Now().UTC().Format(time.RFC3339), uuid.New().String(),
			uuid.New().String()[:8], time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)))

		// Generate valid signature
		webhookID := "webhook_" + uuid.New().String()
		timestamp := time.Now().Unix()
		timestampStr := strconv.FormatInt(timestamp, 10)

		wh, err := svix.NewWebhook(signatureKey)
		require.NoError(t, err)
		signature, err := wh.Sign(webhookID, time.Unix(timestamp, 0), []byte(payload))
		require.NoError(t, err)

		// Send webhook request
		url := fmt.Sprintf("/webhooks/supabase/before-user-created?workspace_id=%s&integration_id=%s", workspaceID, userCreatedIntegrationID)
		req, err := http.NewRequest(http.MethodPost, suite.ServerManager.GetURL()+url, bytes.NewReader(payload))
		require.NoError(t, err)

		req.Header.Set("webhook-id", webhookID)
		req.Header.Set("webhook-timestamp", timestampStr)
		req.Header.Set("webhook-signature", signature)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should process successfully and return 204
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})
}

// testWebhookSignatureValidation tests webhook signature validation
func testWebhookSignatureValidation(t *testing.T, suite *testutil.IntegrationTestSuite, workspaceID string) {
	signatureKey := generateWebhookSecret()
	integrationID := setupSupabaseIntegrationForWebhooks(t, suite, workspaceID, signatureKey)

	// Valid payload
	webhook := domain.SupabaseAuthEmailWebhook{
		User: domain.SupabaseUser{
			ID:           uuid.New().String(),
			Aud:          "authenticated",
			Role:         "authenticated",
			Email:        "test@example.com",
			Phone:        "+1234567890",
			AppMetadata:  map[string]interface{}{"provider": "email", "providers": []string{"email"}},
			UserMetadata: map[string]interface{}{},
			Identities:   []interface{}{},
			CreatedAt:    time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
			IsAnonymous:  false,
		},
		EmailData: domain.SupabaseEmailData{
			Token:           "test-token",
			TokenHash:       "test-hash",
			RedirectTo:      "https://example.com",
			EmailActionType: "signup",
			SiteURL:         "https://example.com",
		},
	}

	payload, err := json.Marshal(webhook)
	require.NoError(t, err)

	t.Run("Valid Signature", func(t *testing.T) {
		webhookID := "webhook_" + uuid.New().String()
		timestamp := time.Now().Unix()
		timestampStr := strconv.FormatInt(timestamp, 10)

		wh, err := svix.NewWebhook(signatureKey)
		require.NoError(t, err)
		signature, err := wh.Sign(webhookID, time.Unix(timestamp, 0), payload)
		require.NoError(t, err)

		url := fmt.Sprintf("/webhooks/supabase/auth-email?workspace_id=%s&integration_id=%s", workspaceID, integrationID)
		req, err := http.NewRequest(http.MethodPost, suite.ServerManager.GetURL()+url, bytes.NewReader(payload))
		require.NoError(t, err)

		req.Header.Set("webhook-id", webhookID)
		req.Header.Set("webhook-timestamp", timestampStr)
		req.Header.Set("webhook-signature", signature)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Invalid Signature", func(t *testing.T) {
		webhookID := "webhook_" + uuid.New().String()
		timestamp := time.Now().Unix()
		timestampStr := strconv.FormatInt(timestamp, 10)

		// Use invalid signature
		invalidSignature := "v1,invalid_signature_" + base64.StdEncoding.EncodeToString([]byte("invalid"))

		url := fmt.Sprintf("/webhooks/supabase/auth-email?workspace_id=%s&integration_id=%s", workspaceID, integrationID)
		req, err := http.NewRequest(http.MethodPost, suite.ServerManager.GetURL()+url, bytes.NewReader(payload))
		require.NoError(t, err)

		req.Header.Set("webhook-id", webhookID)
		req.Header.Set("webhook-timestamp", timestampStr)
		req.Header.Set("webhook-signature", invalidSignature)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Missing webhook-id Header", func(t *testing.T) {
		timestamp := time.Now().Unix()
		timestampStr := strconv.FormatInt(timestamp, 10)

		url := fmt.Sprintf("/webhooks/supabase/auth-email?workspace_id=%s&integration_id=%s", workspaceID, integrationID)
		req, err := http.NewRequest(http.MethodPost, suite.ServerManager.GetURL()+url, bytes.NewReader(payload))
		require.NoError(t, err)

		// Missing webhook-id header
		req.Header.Set("webhook-timestamp", timestampStr)
		req.Header.Set("webhook-signature", "v1,signature")
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Missing webhook-timestamp Header", func(t *testing.T) {
		webhookID := "webhook_" + uuid.New().String()

		url := fmt.Sprintf("/webhooks/supabase/auth-email?workspace_id=%s&integration_id=%s", workspaceID, integrationID)
		req, err := http.NewRequest(http.MethodPost, suite.ServerManager.GetURL()+url, bytes.NewReader(payload))
		require.NoError(t, err)

		req.Header.Set("webhook-id", webhookID)
		// Missing webhook-timestamp header
		req.Header.Set("webhook-signature", "v1,signature")
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Missing webhook-signature Header", func(t *testing.T) {
		webhookID := "webhook_" + uuid.New().String()
		timestamp := time.Now().Unix()
		timestampStr := strconv.FormatInt(timestamp, 10)

		url := fmt.Sprintf("/webhooks/supabase/auth-email?workspace_id=%s&integration_id=%s", workspaceID, integrationID)
		req, err := http.NewRequest(http.MethodPost, suite.ServerManager.GetURL()+url, bytes.NewReader(payload))
		require.NoError(t, err)

		req.Header.Set("webhook-id", webhookID)
		req.Header.Set("webhook-timestamp", timestampStr)
		// Missing webhook-signature header
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Expired Timestamp", func(t *testing.T) {
		webhookID := "webhook_" + uuid.New().String()
		// Use a timestamp from 10 minutes ago (standard-webhooks default tolerance is 5 minutes)
		oldTimestamp := time.Now().Add(-10 * time.Minute).Unix()
		timestampStr := strconv.FormatInt(oldTimestamp, 10)

		wh, err := svix.NewWebhook(signatureKey)
		require.NoError(t, err)
		signature, err := wh.Sign(webhookID, time.Unix(oldTimestamp, 0), payload)
		require.NoError(t, err)

		url := fmt.Sprintf("/webhooks/supabase/auth-email?workspace_id=%s&integration_id=%s", workspaceID, integrationID)
		req, err := http.NewRequest(http.MethodPost, suite.ServerManager.GetURL()+url, bytes.NewReader(payload))
		require.NoError(t, err)

		req.Header.Set("webhook-id", webhookID)
		req.Header.Set("webhook-timestamp", timestampStr)
		req.Header.Set("webhook-signature", signature)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should reject expired timestamp
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// Helper functions

// setupSupabaseIntegrationForWebhooks creates a basic Supabase integration for webhook testing
func setupSupabaseIntegrationForWebhooks(t *testing.T, suite *testutil.IntegrationTestSuite, workspaceID, signatureKey string) string {
	systemCtx := context.WithValue(context.Background(), domain.SystemCallKey, true)

	// Get workspace
	workspace, err := suite.ServerManager.GetApp().GetWorkspaceRepository().GetByID(systemCtx, workspaceID)
	require.NoError(t, err)

	// Create integration
	integrationID := uuid.New().String()
	integration := domain.Integration{
		ID:        integrationID,
		Name:      "Test Supabase Webhook Integration",
		Type:      domain.IntegrationTypeSupabase,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		SupabaseSettings: &domain.SupabaseIntegrationSettings{
			AuthEmailHook: domain.SupabaseAuthEmailHookSettings{
				SignatureKey: signatureKey,
			},
			BeforeUserCreatedHook: domain.SupabaseUserCreatedHookSettings{
				SignatureKey: signatureKey,
			},
		},
	}

	workspace.AddIntegration(integration)
	err = suite.ServerManager.GetApp().GetWorkspaceRepository().Update(systemCtx, workspace)
	require.NoError(t, err)

	// Create default templates and notifications
	// Create SupabaseService instance directly since it's not exposed through the app interface
	supabaseService := service.NewSupabaseService(
		suite.ServerManager.GetApp().GetWorkspaceRepository(),
		nil, // emailService not needed for template/notification creation
		nil, // contactService not needed for template/notification creation
		nil, // listRepo not needed for template/notification creation
		nil, // contactListRepo not needed for template/notification creation
		suite.ServerManager.GetApp().GetTemplateRepository(),
		nil, // templateService not needed for template/notification creation
		suite.ServerManager.GetApp().GetTransactionalNotificationRepository(),
		nil, // transactionalService not needed for template/notification creation
		suite.ServerManager.GetApp().GetLogger(),
	)

	mappings, err := supabaseService.CreateDefaultSupabaseTemplates(systemCtx, workspaceID, integrationID)
	require.NoError(t, err)

	err = supabaseService.CreateDefaultSupabaseNotifications(systemCtx, workspaceID, integrationID, mappings)
	require.NoError(t, err)

	return integrationID
}

// setupSupabaseIntegrationWithUserCreatedHook creates a Supabase integration with user created hook configured
func setupSupabaseIntegrationWithUserCreatedHook(t *testing.T, suite *testutil.IntegrationTestSuite, workspaceID, signatureKey, listID string) string {
	systemCtx := context.WithValue(context.Background(), domain.SystemCallKey, true)

	// Get workspace
	workspace, err := suite.ServerManager.GetApp().GetWorkspaceRepository().GetByID(systemCtx, workspaceID)
	require.NoError(t, err)

	// Create integration
	integrationID := uuid.New().String()
	integration := domain.Integration{
		ID:        integrationID,
		Name:      "Test Supabase User Created Integration",
		Type:      domain.IntegrationTypeSupabase,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		SupabaseSettings: &domain.SupabaseIntegrationSettings{
			AuthEmailHook: domain.SupabaseAuthEmailHookSettings{
				SignatureKey: signatureKey,
			},
			BeforeUserCreatedHook: domain.SupabaseUserCreatedHookSettings{
				SignatureKey:    signatureKey,
				AddUserToLists:  []string{listID},
				CustomJSONField: "custom_json_1",
			},
		},
	}

	workspace.AddIntegration(integration)
	err = suite.ServerManager.GetApp().GetWorkspaceRepository().Update(systemCtx, workspace)
	require.NoError(t, err)

	// Create default templates and notifications
	// Create SupabaseService instance directly since it's not exposed through the app interface
	supabaseService := service.NewSupabaseService(
		suite.ServerManager.GetApp().GetWorkspaceRepository(),
		nil, // emailService not needed for template/notification creation
		nil, // contactService not needed for template/notification creation
		nil, // listRepo not needed for template/notification creation
		nil, // contactListRepo not needed for template/notification creation
		suite.ServerManager.GetApp().GetTemplateRepository(),
		nil, // templateService not needed for template/notification creation
		suite.ServerManager.GetApp().GetTransactionalNotificationRepository(),
		nil, // transactionalService not needed for template/notification creation
		suite.ServerManager.GetApp().GetLogger(),
	)

	mappings, err := supabaseService.CreateDefaultSupabaseTemplates(systemCtx, workspaceID, integrationID)
	require.NoError(t, err)

	err = supabaseService.CreateDefaultSupabaseNotifications(systemCtx, workspaceID, integrationID, mappings)
	require.NoError(t, err)

	return integrationID
}

// testRejectDisposableEmail tests the RejectDisposableEmail feature
func testRejectDisposableEmail(t *testing.T, suite *testutil.IntegrationTestSuite, workspaceID string) {
	// Create a test list for subscription
	testList, err := suite.DataFactory.CreateList(workspaceID)
	require.NoError(t, err)

	signatureKey := generateWebhookSecret()

	t.Run("Disposable Email Rejected When Enabled", func(t *testing.T) {
		// Create integration with RejectDisposableEmail enabled
		systemCtx := context.WithValue(context.Background(), domain.SystemCallKey, true)
		workspace, err := suite.ServerManager.GetApp().GetWorkspaceRepository().GetByID(systemCtx, workspaceID)
		require.NoError(t, err)

		integrationID := uuid.New().String()
		integration := domain.Integration{
			ID:        integrationID,
			Name:      "Test Reject Disposable Email",
			Type:      domain.IntegrationTypeSupabase,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
			SupabaseSettings: &domain.SupabaseIntegrationSettings{
				AuthEmailHook: domain.SupabaseAuthEmailHookSettings{
					SignatureKey: signatureKey,
				},
				BeforeUserCreatedHook: domain.SupabaseUserCreatedHookSettings{
					SignatureKey:          signatureKey,
					AddUserToLists:        []string{testList.ID},
					CustomJSONField:       "custom_json_1",
					RejectDisposableEmail: true, // Enable rejection
				},
			},
		}

		workspace.AddIntegration(integration)
		err = suite.ServerManager.GetApp().GetWorkspaceRepository().Update(systemCtx, workspace)
		require.NoError(t, err)

		// Build webhook payload with disposable email
		disposableEmail := "test@disposable-email.ml"
		metadata := domain.SupabaseWebhookMetadata{
			UUID:      uuid.New().String(),
			Time:      time.Now().UTC().Format(time.RFC3339),
			Name:      "before-user-created",
			IPAddress: "192.168.1.100",
		}

		user := domain.SupabaseUser{
			ID:           uuid.New().String(),
			Aud:          "authenticated",
			Role:         "authenticated",
			Email:        disposableEmail,
			Phone:        "+15551234567",
			AppMetadata:  map[string]interface{}{"provider": "email", "providers": []string{"email"}},
			UserMetadata: map[string]interface{}{"name": "Test User"},
			Identities:   []interface{}{},
			CreatedAt:    time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
			IsAnonymous:  false,
		}

		webhook := domain.SupabaseBeforeUserCreatedWebhook{
			Metadata: metadata,
			User:     user,
		}

		payload, err := json.Marshal(webhook)
		require.NoError(t, err)

		// Generate webhook headers
		webhookID := "webhook_" + uuid.New().String()
		timestamp := time.Now().Unix()
		timestampStr := strconv.FormatInt(timestamp, 10)

		// Sign the payload
		wh, err := svix.NewWebhook(signatureKey)
		require.NoError(t, err)
		signature, err := wh.Sign(webhookID, time.Unix(timestamp, 0), payload)
		require.NoError(t, err)

		// Send webhook request
		url := fmt.Sprintf("/webhooks/supabase/before-user-created?workspace_id=%s&integration_id=%s", workspaceID, integrationID)
		req, err := http.NewRequest(http.MethodPost, suite.ServerManager.GetURL()+url, bytes.NewReader(payload))
		require.NoError(t, err)

		req.Header.Set("webhook-id", webhookID)
		req.Header.Set("webhook-timestamp", timestampStr)
		req.Header.Set("webhook-signature", signature)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should return 400 Bad Request when disposable email is rejected
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Disposable email should be rejected with 400")

		var responseBody map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&responseBody)
		require.NoError(t, err)

		// Check error object structure
		assert.Contains(t, responseBody, "error")
		errorObj := responseBody["error"].(map[string]interface{})
		assert.Equal(t, float64(400), errorObj["http_code"])
		assert.Contains(t, errorObj["message"], "disposable email")

		// Verify contact was NOT created
		contact, err := suite.ServerManager.GetApp().GetContactRepository().GetContactByEmail(systemCtx, workspaceID, disposableEmail)
		assert.Error(t, err, "Contact should not be created for disposable email")
		assert.Nil(t, contact)
	})

	t.Run("Non-Disposable Email Accepted When Enabled", func(t *testing.T) {
		// Create integration with RejectDisposableEmail enabled
		systemCtx := context.WithValue(context.Background(), domain.SystemCallKey, true)
		workspace, err := suite.ServerManager.GetApp().GetWorkspaceRepository().GetByID(systemCtx, workspaceID)
		require.NoError(t, err)

		integrationID := uuid.New().String()
		integration := domain.Integration{
			ID:        integrationID,
			Name:      "Test Accept Valid Email",
			Type:      domain.IntegrationTypeSupabase,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
			SupabaseSettings: &domain.SupabaseIntegrationSettings{
				AuthEmailHook: domain.SupabaseAuthEmailHookSettings{
					SignatureKey: signatureKey,
				},
				BeforeUserCreatedHook: domain.SupabaseUserCreatedHookSettings{
					SignatureKey:          signatureKey,
					AddUserToLists:        []string{testList.ID},
					CustomJSONField:       "custom_json_1",
					RejectDisposableEmail: true, // Enable rejection
				},
			},
		}

		workspace.AddIntegration(integration)
		err = suite.ServerManager.GetApp().GetWorkspaceRepository().Update(systemCtx, workspace)
		require.NoError(t, err)

		// Build webhook payload with valid email
		validEmail := fmt.Sprintf("valid-user-%s@example.com", uuid.New().String()[:8])
		userID := uuid.New().String()
		metadata := domain.SupabaseWebhookMetadata{
			UUID:      uuid.New().String(),
			Time:      time.Now().UTC().Format(time.RFC3339),
			Name:      "before-user-created",
			IPAddress: "192.168.1.100",
		}

		user := domain.SupabaseUser{
			ID:           userID,
			Aud:          "authenticated",
			Role:         "authenticated",
			Email:        validEmail,
			Phone:        "+15551234567",
			AppMetadata:  map[string]interface{}{"provider": "email", "providers": []string{"email"}},
			UserMetadata: map[string]interface{}{"name": "Valid User"},
			Identities:   []interface{}{},
			CreatedAt:    time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
			IsAnonymous:  false,
		}

		webhook := domain.SupabaseBeforeUserCreatedWebhook{
			Metadata: metadata,
			User:     user,
		}

		payload, err := json.Marshal(webhook)
		require.NoError(t, err)

		// Generate webhook headers
		webhookID := "webhook_" + uuid.New().String()
		timestamp := time.Now().Unix()
		timestampStr := strconv.FormatInt(timestamp, 10)

		// Sign the payload
		wh, err := svix.NewWebhook(signatureKey)
		require.NoError(t, err)
		signature, err := wh.Sign(webhookID, time.Unix(timestamp, 0), payload)
		require.NoError(t, err)

		// Send webhook request
		url := fmt.Sprintf("/webhooks/supabase/before-user-created?workspace_id=%s&integration_id=%s", workspaceID, integrationID)
		req, err := http.NewRequest(http.MethodPost, suite.ServerManager.GetURL()+url, bytes.NewReader(payload))
		require.NoError(t, err)

		req.Header.Set("webhook-id", webhookID)
		req.Header.Set("webhook-timestamp", timestampStr)
		req.Header.Set("webhook-signature", signature)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should return 204 No Content for valid email
		assert.Equal(t, http.StatusNoContent, resp.StatusCode, "Valid email should be accepted")

		// Verify contact WAS created
		contact, err := suite.ServerManager.GetApp().GetContactRepository().GetContactByEmail(systemCtx, workspaceID, validEmail)
		require.NoError(t, err, "Contact should be created for valid email")
		assert.Equal(t, validEmail, contact.Email)
		assert.NotNil(t, contact.ExternalID)
		assert.Equal(t, userID, contact.ExternalID.String)
	})

	t.Run("Disposable Email Accepted When Disabled", func(t *testing.T) {
		// Create integration with RejectDisposableEmail disabled
		systemCtx := context.WithValue(context.Background(), domain.SystemCallKey, true)
		workspace, err := suite.ServerManager.GetApp().GetWorkspaceRepository().GetByID(systemCtx, workspaceID)
		require.NoError(t, err)

		integrationID := uuid.New().String()
		integration := domain.Integration{
			ID:        integrationID,
			Name:      "Test Disabled Rejection",
			Type:      domain.IntegrationTypeSupabase,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
			SupabaseSettings: &domain.SupabaseIntegrationSettings{
				AuthEmailHook: domain.SupabaseAuthEmailHookSettings{
					SignatureKey: signatureKey,
				},
				BeforeUserCreatedHook: domain.SupabaseUserCreatedHookSettings{
					SignatureKey:          signatureKey,
					AddUserToLists:        []string{testList.ID},
					CustomJSONField:       "custom_json_1",
					RejectDisposableEmail: false, // Disable rejection
				},
			},
		}

		workspace.AddIntegration(integration)
		err = suite.ServerManager.GetApp().GetWorkspaceRepository().Update(systemCtx, workspace)
		require.NoError(t, err)

		// Build webhook payload with disposable email
		disposableEmail := fmt.Sprintf("test-%s@disposable-email.ml", uuid.New().String()[:8])
		userID := uuid.New().String()
		metadata := domain.SupabaseWebhookMetadata{
			UUID:      uuid.New().String(),
			Time:      time.Now().UTC().Format(time.RFC3339),
			Name:      "before-user-created",
			IPAddress: "192.168.1.100",
		}

		user := domain.SupabaseUser{
			ID:           userID,
			Aud:          "authenticated",
			Role:         "authenticated",
			Email:        disposableEmail,
			Phone:        "+15551234567",
			AppMetadata:  map[string]interface{}{"provider": "email", "providers": []string{"email"}},
			UserMetadata: map[string]interface{}{"name": "Test User"},
			Identities:   []interface{}{},
			CreatedAt:    time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
			IsAnonymous:  false,
		}

		webhook := domain.SupabaseBeforeUserCreatedWebhook{
			Metadata: metadata,
			User:     user,
		}

		payload, err := json.Marshal(webhook)
		require.NoError(t, err)

		// Generate webhook headers
		webhookID := "webhook_" + uuid.New().String()
		timestamp := time.Now().Unix()
		timestampStr := strconv.FormatInt(timestamp, 10)

		// Sign the payload
		wh, err := svix.NewWebhook(signatureKey)
		require.NoError(t, err)
		signature, err := wh.Sign(webhookID, time.Unix(timestamp, 0), payload)
		require.NoError(t, err)

		// Send webhook request
		url := fmt.Sprintf("/webhooks/supabase/before-user-created?workspace_id=%s&integration_id=%s", workspaceID, integrationID)
		req, err := http.NewRequest(http.MethodPost, suite.ServerManager.GetURL()+url, bytes.NewReader(payload))
		require.NoError(t, err)

		req.Header.Set("webhook-id", webhookID)
		req.Header.Set("webhook-timestamp", timestampStr)
		req.Header.Set("webhook-signature", signature)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should return 204 No Content even for disposable email when disabled
		assert.Equal(t, http.StatusNoContent, resp.StatusCode, "Disposable email should be accepted when rejection is disabled")

		// Verify contact WAS created
		contact, err := suite.ServerManager.GetApp().GetContactRepository().GetContactByEmail(systemCtx, workspaceID, disposableEmail)
		require.NoError(t, err, "Contact should be created when rejection is disabled")
		assert.Equal(t, disposableEmail, contact.Email)
		assert.NotNil(t, contact.ExternalID)
		assert.Equal(t, userID, contact.ExternalID.String)
	})
}

// generateWebhookSecret generates a webhook secret in the format expected by standard-webhooks library
func generateWebhookSecret() string {
	// Generate 32 random bytes
	bytes := make([]byte, 32)
	for i := range bytes {
		bytes[i] = byte(uuid.New().ID() % 256)
	}
	// Encode to base64 and add whsec_ prefix
	encoded := base64.StdEncoding.EncodeToString(bytes)
	return "whsec_" + encoded
}
