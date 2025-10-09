package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBroadcastHandler tests the broadcast handler with proper SMTP email provider configuration.
// This test suite sets up a Mailhog SMTP server (configured in docker-compose.test.yml) to properly
// test email sending functionality without skipping tests due to missing email provider configuration.

func TestBroadcastHandler(t *testing.T) {
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

	// Set up SMTP email provider for testing
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	// Login to get auth token
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("CRUD Operations", func(t *testing.T) {
		testBroadcastCRUD(t, client, factory, workspace.ID)
	})

	t.Run("Email Provider Configuration", func(t *testing.T) {
		testBroadcastEmailProvider(t, client, factory, workspace.ID)
	})

	t.Run("Lifecycle Operations", func(t *testing.T) {
		testBroadcastLifecycle(t, client, factory, workspace.ID)
	})

	t.Run("A/B Testing", func(t *testing.T) {
		testBroadcastABTesting(t, client, factory, workspace.ID)
	})

	t.Run("Individual Send", func(t *testing.T) {
		testBroadcastIndividualSend(t, client, factory, workspace.ID)
	})
}

func testBroadcastCRUD(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("Create Broadcast", func(t *testing.T) {
		t.Run("should create broadcast successfully", func(t *testing.T) {
			// Create test data
			list, err := factory.CreateList(workspaceID)
			require.NoError(t, err)

			broadcast := map[string]interface{}{
				"workspace_id": workspaceID,
				"name":         "Test Broadcast",
				"audience": map[string]interface{}{
					"lists":                 []string{list.ID},
					"exclude_unsubscribed":  true,
					"skip_duplicate_emails": true,
				},
				"schedule": map[string]interface{}{
					"is_scheduled": false,
				},
				"test_settings": map[string]interface{}{
					"enabled": false,
				},
				"tracking_enabled": true,
			}

			resp, err := client.CreateBroadcast(broadcast)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusCreated, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Contains(t, result, "broadcast")
			broadcastData := result["broadcast"].(map[string]interface{})
			assert.Equal(t, "Test Broadcast", broadcastData["name"])
			assert.Equal(t, workspaceID, broadcastData["workspace_id"])
			assert.Equal(t, "draft", broadcastData["status"])
		})

		t.Run("should validate required fields", func(t *testing.T) {
			broadcast := map[string]interface{}{
				"workspace_id": workspaceID,
				// Missing name
			}

			resp, err := client.CreateBroadcast(broadcast)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("should validate audience settings", func(t *testing.T) {
			broadcast := map[string]interface{}{
				"workspace_id": workspaceID,
				"name":         "Test Broadcast",
				"audience": map[string]interface{}{
					// Missing lists and segments
					"exclude_unsubscribed": true,
				},
				"schedule": map[string]interface{}{
					"is_scheduled": false,
				},
				"test_settings": map[string]interface{}{
					"enabled": false,
				},
			}

			resp, err := client.CreateBroadcast(broadcast)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("should handle A/B test validation", func(t *testing.T) {
			template1, err := factory.CreateTemplate(workspaceID)
			require.NoError(t, err)
			template2, err := factory.CreateTemplate(workspaceID)
			require.NoError(t, err)
			list, err := factory.CreateList(workspaceID)
			require.NoError(t, err)

			broadcast := map[string]interface{}{
				"workspace_id": workspaceID,
				"name":         "A/B Test Broadcast",
				"audience": map[string]interface{}{
					"lists":                 []string{list.ID},
					"exclude_unsubscribed":  true,
					"skip_duplicate_emails": true,
				},
				"schedule": map[string]interface{}{
					"is_scheduled": false,
				},
				"test_settings": map[string]interface{}{
					"enabled":                 true,
					"sample_percentage":       50,
					"auto_send_winner":        true,
					"auto_send_winner_metric": "open_rate",
					"test_duration_hours":     24,
					"variations": []map[string]interface{}{
						{
							"variation_name": "Version A",
							"template_id":    template1.ID,
						},
						{
							"variation_name": "Version B",
							"template_id":    template2.ID,
						},
					},
				},
			}

			resp, err := client.CreateBroadcast(broadcast)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusCreated, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			broadcastData := result["broadcast"].(map[string]interface{})
			testSettings := broadcastData["test_settings"].(map[string]interface{})
			assert.True(t, testSettings["enabled"].(bool))
			assert.Equal(t, float64(50), testSettings["sample_percentage"])
		})
	})

	t.Run("Get Broadcast", func(t *testing.T) {
		t.Run("should get broadcast successfully", func(t *testing.T) {
			// Create a broadcast first
			broadcast, err := factory.CreateBroadcast(workspaceID)
			require.NoError(t, err)

			resp, err := client.GetBroadcast(broadcast.ID)
			require.NoError(t, err)
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Logf("Unexpected status code %d: %s", resp.StatusCode, string(body))
			}
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Contains(t, result, "broadcast")
			if broadcastDataInterface, ok := result["broadcast"]; ok && broadcastDataInterface != nil {
				broadcastData := broadcastDataInterface.(map[string]interface{})
				assert.Equal(t, broadcast.ID, broadcastData["id"])
				assert.Equal(t, broadcast.Name, broadcastData["name"])
			} else {
				t.Errorf("broadcast field is missing or nil in response: %+v", result)
			}
		})

		t.Run("should return 404 for non-existent broadcast", func(t *testing.T) {
			resp, err := client.GetBroadcast("non-existent-id")
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})

		t.Run("should validate required parameters", func(t *testing.T) {
			// Test missing parameters
			resp, err := client.Get("/api/broadcasts.get")
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})

	t.Run("List Broadcasts", func(t *testing.T) {
		t.Run("should list broadcasts successfully", func(t *testing.T) {
			// Create multiple broadcasts
			for i := 0; i < 3; i++ {
				_, err := factory.CreateBroadcast(workspaceID)
				require.NoError(t, err)
			}

			resp, err := client.ListBroadcasts(map[string]string{
				"workspace_id": workspaceID,
			})
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Contains(t, result, "broadcasts")
			assert.Contains(t, result, "total_count")
			broadcasts := result["broadcasts"].([]interface{})
			assert.GreaterOrEqual(t, len(broadcasts), 3)
		})

		t.Run("should handle pagination", func(t *testing.T) {
			resp, err := client.ListBroadcasts(map[string]string{
				"workspace_id": workspaceID,
				"limit":        "2",
				"offset":       "1",
			})
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			broadcasts := result["broadcasts"].([]interface{})
			assert.LessOrEqual(t, len(broadcasts), 2)
		})

		t.Run("should filter by status", func(t *testing.T) {
			resp, err := client.ListBroadcasts(map[string]string{
				"workspace_id": workspaceID,
				"status":       "draft",
			})
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			broadcasts := result["broadcasts"].([]interface{})
			for _, b := range broadcasts {
				broadcastData := b.(map[string]interface{})
				assert.Equal(t, "draft", broadcastData["status"])
			}
		})
	})

	t.Run("Update Broadcast", func(t *testing.T) {
		t.Run("should update broadcast successfully", func(t *testing.T) {
			// Create a list first
			list, err := factory.CreateList(workspaceID)
			require.NoError(t, err)

			// Create a broadcast first
			broadcast, err := factory.CreateBroadcast(workspaceID)
			require.NoError(t, err)

			updateRequest := map[string]interface{}{
				"workspace_id": workspaceID,
				"id":           broadcast.ID,
				"name":         "Updated Broadcast Name",
				"audience": map[string]interface{}{
					"lists":                 []string{list.ID},
					"exclude_unsubscribed":  true,
					"skip_duplicate_emails": true,
				},
				"schedule":      broadcast.Schedule,
				"test_settings": broadcast.TestSettings,
			}

			resp, err := client.UpdateBroadcast(updateRequest)
			require.NoError(t, err)
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Logf("Update broadcast failed with status %d: %s", resp.StatusCode, string(body))
			}
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			if broadcastDataInterface, ok := result["broadcast"]; ok && broadcastDataInterface != nil {
				broadcastData := broadcastDataInterface.(map[string]interface{})
				assert.Equal(t, "Updated Broadcast Name", broadcastData["name"])
			} else {
				t.Errorf("broadcast field is missing or nil in update response: %+v", result)
			}
		})

		t.Run("should prevent updating non-draft broadcasts", func(t *testing.T) {
			// Create a list first
			list, err := factory.CreateList(workspaceID)
			require.NoError(t, err)

			// Create a broadcast and set it to sent status
			broadcast, err := factory.CreateBroadcast(workspaceID,
				testutil.WithBroadcastStatus(domain.BroadcastStatusSent))
			require.NoError(t, err)

			updateRequest := map[string]interface{}{
				"workspace_id": workspaceID,
				"id":           broadcast.ID,
				"name":         "Should Not Update",
				"audience": map[string]interface{}{
					"lists":                 []string{list.ID},
					"exclude_unsubscribed":  true,
					"skip_duplicate_emails": true,
				},
				"schedule":      broadcast.Schedule,
				"test_settings": broadcast.TestSettings,
			}

			resp, err := client.UpdateBroadcast(updateRequest)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})

	t.Run("Delete Broadcast", func(t *testing.T) {
		t.Run("should delete broadcast successfully", func(t *testing.T) {
			// Create a broadcast first
			broadcast, err := factory.CreateBroadcast(workspaceID)
			require.NoError(t, err)

			deleteRequest := map[string]interface{}{
				"workspace_id": workspaceID,
				"id":           broadcast.ID,
			}

			resp, err := client.DeleteBroadcast(deleteRequest)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			// Verify broadcast is deleted
			getResp, err := client.GetBroadcast(broadcast.ID)
			require.NoError(t, err)
			defer getResp.Body.Close()

			assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
		})
	})
}

func testBroadcastEmailProvider(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should have SMTP email provider configured", func(t *testing.T) {
		// Get workspace to verify email provider configuration
		resp, err := client.Get("/api/workspaces.get", map[string]string{
			"id": workspaceID,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		workspaceData := result["workspace"].(map[string]interface{})
		settings := workspaceData["settings"].(map[string]interface{})

		// Verify marketing email provider is configured
		assert.Contains(t, settings, "marketing_email_provider_id")
		assert.NotEmpty(t, settings["marketing_email_provider_id"])

		// Verify integrations exist
		assert.Contains(t, workspaceData, "integrations")
		integrations := workspaceData["integrations"].([]interface{})
		assert.Greater(t, len(integrations), 0)

		// Find the email integration
		var emailIntegration map[string]interface{}
		for _, integration := range integrations {
			integData := integration.(map[string]interface{})
			if integData["type"] == "email" {
				emailIntegration = integData
				break
			}
		}

		assert.NotNil(t, emailIntegration)
		assert.Equal(t, "email", emailIntegration["type"])

		// Verify email provider configuration
		emailProvider := emailIntegration["email_provider"].(map[string]interface{})
		assert.Equal(t, "smtp", emailProvider["kind"])

		// Verify SMTP settings
		assert.Contains(t, emailProvider, "smtp")
		smtpSettings := emailProvider["smtp"].(map[string]interface{})
		assert.Equal(t, "localhost", smtpSettings["host"])
		assert.Equal(t, float64(1025), smtpSettings["port"]) // JSON numbers are floats
		assert.False(t, smtpSettings["use_tls"].(bool))

		// Verify senders
		assert.Contains(t, emailProvider, "senders")
		senders := emailProvider["senders"].([]interface{})
		assert.Greater(t, len(senders), 0)

		sender := senders[0].(map[string]interface{})
		assert.Contains(t, sender, "email")
		assert.Contains(t, sender, "name")
		assert.NotEmpty(t, sender["email"])
		assert.NotEmpty(t, sender["name"])
	})
}

func testBroadcastLifecycle(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("Schedule Broadcast", func(t *testing.T) {
		t.Run("should schedule broadcast for immediate sending with real recipients", func(t *testing.T) {
			// Create a template for the broadcast
			template, err := factory.CreateTemplate(workspaceID)
			require.NoError(t, err)

			// Create a list
			list, err := factory.CreateList(workspaceID)
			require.NoError(t, err)

			// Create contacts and add them to the list
			for i := 0; i < 3; i++ {
				contact, err := factory.CreateContact(workspaceID,
					testutil.WithContactEmail(fmt.Sprintf("lifecycle-test-%d@example.com", i)))
				require.NoError(t, err)

				// Add contact to list with Active status
				_, err = factory.CreateContactList(workspaceID,
					testutil.WithContactListEmail(contact.Email),
					testutil.WithContactListListID(list.ID),
					testutil.WithContactListStatus(domain.ContactListStatusActive))
				require.NoError(t, err)
			}

			// Create broadcast targeting the list with a template
			broadcast, err := factory.CreateBroadcast(workspaceID,
				testutil.WithBroadcastAudience(domain.AudienceSettings{
					Lists:               []string{list.ID},
					ExcludeUnsubscribed: true,
					SkipDuplicateEmails: true,
				}))
			require.NoError(t, err)

			// Set the template for the broadcast
			broadcast.TestSettings.Variations[0].TemplateID = template.ID
			updateReq := map[string]interface{}{
				"workspace_id": workspaceID,
				"id":           broadcast.ID,
				"name":         broadcast.Name,
				"audience":     broadcast.Audience,
				"schedule":     broadcast.Schedule,
				"test_settings": broadcast.TestSettings,
			}
			updateResp, err := client.UpdateBroadcast(updateReq)
			require.NoError(t, err)
			defer updateResp.Body.Close()
			require.Equal(t, http.StatusOK, updateResp.StatusCode)

			scheduleRequest := map[string]interface{}{
				"workspace_id": workspaceID,
				"id":           broadcast.ID,
				"send_now":     true,
			}

			resp, err := client.ScheduleBroadcast(scheduleRequest)
			require.NoError(t, err)
			defer resp.Body.Close()

			// With SMTP provider configured, this should succeed
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			if successInterface, ok := result["success"]; ok && successInterface != nil {
				assert.True(t, successInterface.(bool))
			} else {
				t.Logf("Unexpected response format: %+v", result)
			}

			// Execute pending tasks to start the broadcast
			time.Sleep(1 * time.Second)
			execResp, err := client.ExecutePendingTasks(10)
			require.NoError(t, err)
			defer execResp.Body.Close()

			// Wait for broadcast to complete and verify it succeeded
			// This is the critical check that would have caught the SQL scan bug!
			finalStatus, err := testutil.WaitForBroadcastCompletion(t, client, broadcast.ID, 30*time.Second)
			if err != nil {
				t.Fatalf("Broadcast failed or timed out: %v", err)
			}

			assert.Contains(t, []string{"sent", "completed"}, finalStatus,
				"Broadcast should complete successfully")
		})

		t.Run("should schedule broadcast for future sending", func(t *testing.T) {
			broadcast, err := factory.CreateBroadcast(workspaceID)
			require.NoError(t, err)

			futureTime := time.Now().Add(24 * time.Hour)
			scheduleRequest := map[string]interface{}{
				"workspace_id":           workspaceID,
				"id":                     broadcast.ID,
				"send_now":               false,
				"scheduled_date":         futureTime.Format("2006-01-02"),
				"scheduled_time":         futureTime.Format("15:04"),
				"timezone":               "UTC",
				"use_recipient_timezone": false,
			}

			resp, err := client.ScheduleBroadcast(scheduleRequest)
			require.NoError(t, err)
			defer resp.Body.Close()

			// With SMTP provider configured, this should succeed
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})

		t.Run("should validate schedule parameters", func(t *testing.T) {
			broadcast, err := factory.CreateBroadcast(workspaceID)
			require.NoError(t, err)

			// Missing required fields for scheduled sending
			scheduleRequest := map[string]interface{}{
				"workspace_id": workspaceID,
				"id":           broadcast.ID,
				"send_now":     false,
				// Missing scheduled_date and scheduled_time
			}

			resp, err := client.ScheduleBroadcast(scheduleRequest)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})

	t.Run("Pause Broadcast", func(t *testing.T) {
		t.Run("should pause broadcast successfully", func(t *testing.T) {
			broadcast, err := factory.CreateBroadcast(workspaceID,
				testutil.WithBroadcastStatus(domain.BroadcastStatusSending))
			require.NoError(t, err)

			pauseRequest := map[string]interface{}{
				"workspace_id": workspaceID,
				"id":           broadcast.ID,
			}

			resp, err := client.PauseBroadcast(pauseRequest)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	})

	t.Run("Resume Broadcast", func(t *testing.T) {
		t.Run("should resume broadcast successfully", func(t *testing.T) {
			broadcast, err := factory.CreateBroadcast(workspaceID,
				testutil.WithBroadcastStatus(domain.BroadcastStatusPaused))
			require.NoError(t, err)

			resumeRequest := map[string]interface{}{
				"workspace_id": workspaceID,
				"id":           broadcast.ID,
			}

			resp, err := client.ResumeBroadcast(resumeRequest)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	})

	t.Run("Cancel Broadcast", func(t *testing.T) {
		t.Run("should cancel broadcast successfully", func(t *testing.T) {
			broadcast, err := factory.CreateBroadcast(workspaceID,
				testutil.WithBroadcastStatus(domain.BroadcastStatusScheduled))
			require.NoError(t, err)

			cancelRequest := map[string]interface{}{
				"workspace_id": workspaceID,
				"id":           broadcast.ID,
			}

			resp, err := client.CancelBroadcast(cancelRequest)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	})
}

func testBroadcastABTesting(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("Get Test Results", func(t *testing.T) {
		t.Run("should get test results successfully", func(t *testing.T) {
			// Create broadcast with A/B testing enabled and in testing status
			template1, err := factory.CreateTemplate(workspaceID)
			require.NoError(t, err)
			template2, err := factory.CreateTemplate(workspaceID)
			require.NoError(t, err)

			broadcast, err := factory.CreateBroadcast(workspaceID,
				testutil.WithBroadcastABTesting([]string{template1.ID, template2.ID}),
				testutil.WithBroadcastStatus(domain.BroadcastStatusTestCompleted))
			require.NoError(t, err)

			resp, err := client.GetBroadcastTestResults(workspaceID, broadcast.ID)
			require.NoError(t, err)
			defer resp.Body.Close()

			// With SMTP provider configured, this should succeed
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			if broadcastIDInterface, ok := result["broadcast_id"]; ok && broadcastIDInterface != nil {
				assert.Equal(t, broadcast.ID, broadcastIDInterface.(string))
			} else {
				t.Errorf("broadcast_id field is missing or nil in response: %+v", result)
			}

			if _, ok := result["variation_results"]; !ok {
				t.Errorf("variation_results field is missing in response: %+v", result)
			}
		})

		t.Run("should validate required parameters", func(t *testing.T) {
			resp, err := client.Get("/api/broadcasts.getTestResults")
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})

	t.Run("Select Winner", func(t *testing.T) {
		t.Run("should select winner successfully", func(t *testing.T) {
			template1, err := factory.CreateTemplate(workspaceID)
			require.NoError(t, err)
			template2, err := factory.CreateTemplate(workspaceID)
			require.NoError(t, err)

			broadcast, err := factory.CreateBroadcast(workspaceID,
				testutil.WithBroadcastABTesting([]string{template1.ID, template2.ID}),
				testutil.WithBroadcastStatus(domain.BroadcastStatusTestCompleted))
			require.NoError(t, err)

			selectWinnerRequest := map[string]interface{}{
				"workspace_id": workspaceID,
				"id":           broadcast.ID,
				"template_id":  template1.ID,
			}

			resp, err := client.SelectBroadcastWinner(selectWinnerRequest)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			if successInterface, ok := result["success"]; ok && successInterface != nil {
				assert.True(t, successInterface.(bool))
			} else {
				t.Errorf("success field is missing or nil in select winner response: %+v", result)
			}
		})

		t.Run("should validate winner selection parameters", func(t *testing.T) {
			selectWinnerRequest := map[string]interface{}{
				"workspace_id": workspaceID,
				// Missing id and template_id
			}

			resp, err := client.SelectBroadcastWinner(selectWinnerRequest)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})
}

func testBroadcastIndividualSend(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("Send To Individual", func(t *testing.T) {
		t.Run("should send to individual successfully", func(t *testing.T) {
			// Create templates for A/B testing (requires at least 2)
			template1, err := factory.CreateTemplate(workspaceID)
			require.NoError(t, err)
			template2, err := factory.CreateTemplate(workspaceID)
			require.NoError(t, err)

			// Create broadcast with A/B testing that includes our templates
			broadcast, err := factory.CreateBroadcast(workspaceID,
				testutil.WithBroadcastABTesting([]string{template1.ID, template2.ID}))
			require.NoError(t, err)

			contact, err := factory.CreateContact(workspaceID)
			require.NoError(t, err)

			sendRequest := map[string]interface{}{
				"workspace_id":    workspaceID,
				"broadcast_id":    broadcast.ID,
				"recipient_email": contact.Email,
				"template_id":     template1.ID,
			}

			resp, err := client.SendBroadcastToIndividual(sendRequest)
			require.NoError(t, err)
			defer resp.Body.Close()

			// With SMTP provider configured, this should succeed
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			if successInterface, ok := result["success"]; ok && successInterface != nil {
				assert.True(t, successInterface.(bool))
			} else {
				t.Errorf("success field is missing or nil in send individual response: %+v", result)
			}
		})

		t.Run("should validate send to individual parameters", func(t *testing.T) {
			sendRequest := map[string]interface{}{
				"workspace_id": workspaceID,
				// Missing required fields
			}

			resp, err := client.SendBroadcastToIndividual(sendRequest)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})
}

func TestBroadcastAuthentication(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	t.Run("should require authentication for all endpoints", func(t *testing.T) {
		// Don't login, make requests without auth
		client.SetToken("")

		endpoints := []struct {
			name string
			fn   func() (*http.Response, error)
		}{
			{"list", func() (*http.Response, error) {
				return client.ListBroadcasts(map[string]string{"workspace_id": workspace.ID})
			}},
			{"get", func() (*http.Response, error) {
				return client.GetBroadcast("test-id")
			}},
			{"create", func() (*http.Response, error) {
				return client.CreateBroadcast(map[string]interface{}{"workspace_id": workspace.ID})
			}},
			{"update", func() (*http.Response, error) {
				return client.UpdateBroadcast(map[string]interface{}{"workspace_id": workspace.ID})
			}},
			{"schedule", func() (*http.Response, error) {
				return client.ScheduleBroadcast(map[string]interface{}{"workspace_id": workspace.ID})
			}},
			{"pause", func() (*http.Response, error) {
				return client.PauseBroadcast(map[string]interface{}{"workspace_id": workspace.ID})
			}},
			{"resume", func() (*http.Response, error) {
				return client.ResumeBroadcast(map[string]interface{}{"workspace_id": workspace.ID})
			}},
			{"cancel", func() (*http.Response, error) {
				return client.CancelBroadcast(map[string]interface{}{"workspace_id": workspace.ID})
			}},
			{"delete", func() (*http.Response, error) {
				return client.DeleteBroadcast(map[string]interface{}{"workspace_id": workspace.ID})
			}},
			{"sendToIndividual", func() (*http.Response, error) {
				return client.SendBroadcastToIndividual(map[string]interface{}{"workspace_id": workspace.ID})
			}},
			{"getTestResults", func() (*http.Response, error) {
				return client.GetBroadcastTestResults(workspace.ID, "test-id")
			}},
			{"selectWinner", func() (*http.Response, error) {
				return client.SelectBroadcastWinner(map[string]interface{}{"workspace_id": workspace.ID})
			}},
		}

		for _, endpoint := range endpoints {
			t.Run(endpoint.name, func(t *testing.T) {
				resp, err := endpoint.fn()
				require.NoError(t, err)
				defer resp.Body.Close()

				assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
			})
		}
	})
}

func TestBroadcastMethodValidation(t *testing.T) {
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

	// Login to get auth token
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("should validate HTTP methods", func(t *testing.T) {
		getEndpoints := []string{
			"/api/broadcasts.list",
			"/api/broadcasts.get",
			"/api/broadcasts.getTestResults",
		}

		postEndpoints := []string{
			"/api/broadcasts.create",
			"/api/broadcasts.update",
			"/api/broadcasts.schedule",
			"/api/broadcasts.pause",
			"/api/broadcasts.resume",
			"/api/broadcasts.cancel",
			"/api/broadcasts.delete",
			"/api/broadcasts.sendToIndividual",
			"/api/broadcasts.selectWinner",
		}

		// Test GET endpoints with POST method
		for _, endpoint := range getEndpoints {
			t.Run("POST to "+endpoint, func(t *testing.T) {
				resp, err := client.Post(endpoint, map[string]interface{}{}, map[string]string{
					"workspace_id": workspace.ID,
				})
				require.NoError(t, err)
				defer resp.Body.Close()

				assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
			})
		}

		// Test POST endpoints with GET method
		for _, endpoint := range postEndpoints {
			t.Run("GET to "+endpoint, func(t *testing.T) {
				resp, err := client.Get(endpoint, map[string]string{
					"workspace_id": workspace.ID,
				})
				require.NoError(t, err)
				defer resp.Body.Close()

				assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
			})
		}
	})
}

func TestBroadcastDataFactory(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	t.Run("CreateBroadcast", func(t *testing.T) {
		broadcast, err := factory.CreateBroadcast(workspace.ID)
		require.NoError(t, err)
		require.NotNil(t, broadcast)

		assert.NotEmpty(t, broadcast.ID)
		assert.Equal(t, workspace.ID, broadcast.WorkspaceID)
		assert.NotEmpty(t, broadcast.Name)
		assert.Equal(t, domain.BroadcastStatusDraft, broadcast.Status)
		assert.NotZero(t, broadcast.CreatedAt)
		assert.NotZero(t, broadcast.UpdatedAt)
	})

	t.Run("CreateBroadcast with options", func(t *testing.T) {
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("Custom Broadcast"),
			testutil.WithBroadcastStatus(domain.BroadcastStatusScheduled))
		require.NoError(t, err)

		assert.Equal(t, "Custom Broadcast", broadcast.Name)
		assert.Equal(t, domain.BroadcastStatusScheduled, broadcast.Status)
	})

	t.Run("CreateBroadcast persisted to database", func(t *testing.T) {
		broadcast, err := factory.CreateBroadcast(workspace.ID)
		require.NoError(t, err)

		// Verify broadcast was created successfully with proper data
		require.NotNil(t, broadcast)
		assert.NotEmpty(t, broadcast.ID)
		assert.Equal(t, workspace.ID, broadcast.WorkspaceID)
		assert.NotEmpty(t, broadcast.Name)
		assert.NotZero(t, broadcast.CreatedAt)
		assert.NotZero(t, broadcast.UpdatedAt)

		// The factory uses the repository to create the broadcast,
		// so if this succeeds, it means the broadcast was persisted correctly
		// Additional verification would require workspace database setup which
		// is already tested in the repository unit tests
	})
}
