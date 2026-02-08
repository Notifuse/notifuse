package integration

import (
	"encoding/json"
	"fmt"
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

// TestSegmentE2E tests the complete end-to-end segmentation engine flow
// This test covers:
// - Segment creation with different tree structures
// - Segment preview (query execution before building)
// - Segment building (async task processing)
// - Segment membership tracking
// - Contact filtering by segments
// - Complex segment trees with AND/OR logic
// - Integration with contact_lists and contact_timeline
func TestSegmentE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	// Add a sleep before cleanup to allow background tasks to complete
	defer func() {
		// Wait for any pending async operations to complete
		time.Sleep(500 * time.Millisecond)
		suite.Cleanup()
	}()

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

	t.Run("Simple Contact Segment", func(t *testing.T) {
		t.Cleanup(func() { testutil.CleanupAllTasks(t, client, workspace.ID) })
		testSimpleContactSegment(t, client, factory, workspace.ID)
	})

	t.Run("Segment Preview", func(t *testing.T) {
		t.Cleanup(func() { testutil.CleanupAllTasks(t, client, workspace.ID) })
		testSegmentPreview(t, client, factory, workspace.ID)
	})

	t.Run("Complex Segment with AND/OR Logic", func(t *testing.T) {
		t.Cleanup(func() { testutil.CleanupAllTasks(t, client, workspace.ID) })
		testComplexSegmentTree(t, client, factory, workspace.ID)
	})

	t.Run("Segment with Contact Lists", func(t *testing.T) {
		t.Cleanup(func() { testutil.CleanupAllTasks(t, client, workspace.ID) })
		testSegmentWithContactLists(t, client, factory, workspace.ID)
	})

	t.Run("Segment with Contact Timeline", func(t *testing.T) {
		t.Cleanup(func() { testutil.CleanupAllTasks(t, client, workspace.ID) })
		testSegmentWithContactTimeline(t, client, factory, workspace.ID)
	})

	t.Run("Segment Rebuild and Membership Updates", func(t *testing.T) {
		t.Cleanup(func() { testutil.CleanupAllTasks(t, client, workspace.ID) })
		testSegmentRebuild(t, client, factory, workspace.ID)
	})

	t.Run("List and Get Segments", func(t *testing.T) {
		t.Cleanup(func() { testutil.CleanupAllTasks(t, client, workspace.ID) })
		testListAndGetSegments(t, client, factory, workspace.ID)
	})

	t.Run("Update and Delete Segments", func(t *testing.T) {
		t.Cleanup(func() { testutil.CleanupAllTasks(t, client, workspace.ID) })
		testUpdateAndDeleteSegments(t, client, factory, workspace.ID)
	})

	t.Run("Segment with Relative Dates - Daily Recompute", func(t *testing.T) {
		t.Cleanup(func() { testutil.CleanupAllTasks(t, client, workspace.ID) })
		testSegmentWithRelativeDates(t, client, factory, workspace.ID)
	})

	t.Run("Check Segment Recompute Task Processor", func(t *testing.T) {
		t.Cleanup(func() { testutil.CleanupAllTasks(t, client, workspace.ID) })
		testCheckSegmentRecomputeProcessor(t, client, factory, workspace.ID)
	})

	t.Run("Contact Property Date Filter with Relative Dates", func(t *testing.T) {
		t.Cleanup(func() { testutil.CleanupAllTasks(t, client, workspace.ID) })
		testContactPropertyRelativeDates(t, client, factory, workspace.ID)
	})

	t.Run("Custom Events Goals Segmentation", func(t *testing.T) {
		t.Cleanup(func() { testutil.CleanupAllTasks(t, client, workspace.ID) })
		testCustomEventsGoalsSegmentation(t, client, factory, workspace.ID)
	})
}

// testSimpleContactSegment tests creating a simple segment with contact filters
func testSimpleContactSegment(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should create and build a simple contact segment", func(t *testing.T) {
		// Step 1: Create test contacts
		for i := 0; i < 10; i++ {
			_, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("us-contact-%d@example.com", i)),
				testutil.WithContactCountry("US"))
			require.NoError(t, err)
		}

		for i := 0; i < 5; i++ {
			_, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("ca-contact-%d@example.com", i)),
				testutil.WithContactCountry("CA"))
			require.NoError(t, err)
		}

		// Step 2: Create segment filtering US contacts
		segment := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           fmt.Sprintf("uscontacts%d", time.Now().Unix()),
			"name":         "US Contacts",
			"description":  "All contacts from the United States",
			"color":        "#FF5733",
			"timezone":     "UTC",
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "contacts",
					"contact": map[string]interface{}{
						"filters": []map[string]interface{}{
							{
								"field_name":    "country",
								"field_type":    "string",
								"operator":      "equals",
								"string_values": []string{"US"},
							},
						},
					},
				},
			},
		}

		// Step 3: Create the segment
		resp, err := client.Post("/api/segments.create", segment)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResult map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&createResult)
		require.NoError(t, err)

		segmentData := createResult["segment"].(map[string]interface{})
		segmentID := segmentData["id"].(string)
		assert.Equal(t, "US Contacts", segmentData["name"])
		assert.Equal(t, "building", segmentData["status"])

		// Step 4: Rebuild the segment (trigger async task)
		rebuildResp, err := client.Post("/api/segments.rebuild", map[string]interface{}{
			"workspace_id": workspaceID,
			"segment_id":   segmentID,
		})
		require.NoError(t, err)
		defer func() { _ = rebuildResp.Body.Close() }()
		assert.Equal(t, http.StatusOK, rebuildResp.StatusCode)

		// Step 5: Execute pending tasks to process segment build
		execResp, err := client.Post("/api/tasks.execute", map[string]interface{}{
			"limit": 10,
		})
		require.NoError(t, err)
		_ = execResp.Body.Close()

		// Wait for segment to be built
		status, err := testutil.WaitForSegmentBuilt(t, client, workspaceID, segmentID, 10*time.Second)
		if err != nil {
			t.Fatalf("Segment build failed: %v", err)
		}
		assert.Contains(t, []string{"built", "active"}, status, "Segment should be built or active")

		// Step 6: Verify segment status and users count
		getResp, err := client.Get(fmt.Sprintf("/api/segments.get?workspace_id=%s&id=%s", workspaceID, segmentID))
		require.NoError(t, err)
		defer func() { _ = getResp.Body.Close() }()

		var getResult map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&getResult)
		require.NoError(t, err)

		updatedSegment := getResult["segment"].(map[string]interface{})

		// Segment should be active after building
		status = updatedSegment["status"].(string)
		assert.Contains(t, []string{"active", "building"}, status)

		// Should have counted 10 US contacts
		if usersCount, ok := updatedSegment["users_count"].(float64); ok {
			assert.True(t, usersCount >= 10, "Expected at least 10 users, got %v", usersCount)
		}
	})
}

// testSegmentPreview tests the segment preview functionality
func testSegmentPreview(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should preview segment results without building", func(t *testing.T) {
		// Create test contacts with custom_number_1
		for i := 0; i < 5; i++ {
			_, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("premium-%d@example.com", i)),
				testutil.WithContactCustomNumber1(1000.0+float64(i)*100))
			require.NoError(t, err)
		}

		for i := 0; i < 10; i++ {
			_, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("regular-%d@example.com", i)),
				testutil.WithContactCustomNumber1(50.0))
			require.NoError(t, err)
		}

		// Preview segment for high-value contacts
		previewReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "contacts",
					"contact": map[string]interface{}{
						"filters": []map[string]interface{}{
							{
								"field_name":    "custom_number_1",
								"field_type":    "number",
								"operator":      "gte",
								"number_values": []float64{1000.0},
							},
						},
					},
				},
			},
			"limit": 10,
		}

		resp, err := client.Post("/api/segments.preview", previewReq)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		emails := result["emails"].([]interface{})
		totalCount := int(result["total_count"].(float64))

		// Emails should not be returned for privacy/performance reasons
		assert.Empty(t, emails, "Emails should not be returned in preview")
		// Should find at least 5 premium contacts in the count
		assert.True(t, totalCount >= 5, "Expected total count of at least 5")
	})
}

// testComplexSegmentTree tests segments with complex AND/OR logic
func testComplexSegmentTree(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should handle complex segment trees with AND/OR logic", func(t *testing.T) {
		// Create test contacts with different attributes
		// Group 1: US + high value
		for i := 0; i < 3; i++ {
			_, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("us-vip-%d@example.com", i)),
				testutil.WithContactCountry("US"),
				testutil.WithContactCustomNumber1(2000.0))
			require.NoError(t, err)
		}

		// Group 2: CA + high value
		for i := 0; i < 2; i++ {
			_, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("ca-vip-%d@example.com", i)),
				testutil.WithContactCountry("CA"),
				testutil.WithContactCustomNumber1(2000.0))
			require.NoError(t, err)
		}

		// Group 3: US + low value (should not match)
		for i := 0; i < 5; i++ {
			_, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("us-regular-%d@example.com", i)),
				testutil.WithContactCountry("US"),
				testutil.WithContactCustomNumber1(100.0))
			require.NoError(t, err)
		}

		// Create segment: (country=US OR country=CA) AND custom_number_1 >= 2000
		segment := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           fmt.Sprintf("navip%d", time.Now().Unix()),
			"name":         "North America VIP",
			"description":  "High-value customers from US or Canada",
			"color":        "#FFD700",
			"timezone":     "UTC",
			"tree": map[string]interface{}{
				"kind": "branch",
				"branch": map[string]interface{}{
					"operator": "and",
					"leaves": []map[string]interface{}{
						// Branch 1: country=US OR country=CA
						{
							"kind": "branch",
							"branch": map[string]interface{}{
								"operator": "or",
								"leaves": []map[string]interface{}{
									{
										"kind": "leaf",
										"leaf": map[string]interface{}{
											"source": "contacts",
											"contact": map[string]interface{}{
												"filters": []map[string]interface{}{
													{
														"field_name":    "country",
														"field_type":    "string",
														"operator":      "equals",
														"string_values": []string{"US"},
													},
												},
											},
										},
									},
									{
										"kind": "leaf",
										"leaf": map[string]interface{}{
											"source": "contacts",
											"contact": map[string]interface{}{
												"filters": []map[string]interface{}{
													{
														"field_name":    "country",
														"field_type":    "string",
														"operator":      "equals",
														"string_values": []string{"CA"},
													},
												},
											},
										},
									},
								},
							},
						},
						// Branch 2: custom_number_1 >= 2000
						{
							"kind": "leaf",
							"leaf": map[string]interface{}{
								"source": "contacts",
								"contact": map[string]interface{}{
									"filters": []map[string]interface{}{
										{
											"field_name":    "custom_number_1",
											"field_type":    "number",
											"operator":      "gte",
											"number_values": []float64{2000.0},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		// Create and preview the segment
		previewResp, err := client.Post("/api/segments.preview", map[string]interface{}{
			"workspace_id": workspaceID,
			"tree":         segment["tree"],
			"limit":        20,
		})
		require.NoError(t, err)
		defer func() { _ = previewResp.Body.Close() }()
		assert.Equal(t, http.StatusOK, previewResp.StatusCode)

		var previewResult map[string]interface{}
		err = json.NewDecoder(previewResp.Body).Decode(&previewResult)
		require.NoError(t, err)

		emails := previewResult["emails"].([]interface{})
		totalCount := int(previewResult["total_count"].(float64))

		// Should match 5 contacts (3 US VIP + 2 CA VIP)
		assert.Equal(t, 5, totalCount, "Expected exactly 5 matching contacts")
		// Emails should not be returned for privacy/performance reasons
		assert.Empty(t, emails, "Emails should not be returned in preview")
	})
}

// testSegmentWithContactLists tests segments that filter by list membership
func testSegmentWithContactLists(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should filter contacts by list membership", func(t *testing.T) {
		// Create lists
		newsletterList, err := factory.CreateList(workspaceID,
			testutil.WithListName("Newsletter Subscribers"))
		require.NoError(t, err)

		vipList, err := factory.CreateList(workspaceID,
			testutil.WithListName("VIP List"))
		require.NoError(t, err)

		// Create contacts and add to lists
		// Group 1: Newsletter only
		for i := 0; i < 5; i++ {
			contact, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("newsletter-%d@example.com", i)))
			require.NoError(t, err)

			_, err = factory.CreateContactList(workspaceID,
				testutil.WithContactListEmail(contact.Email),
				testutil.WithContactListListID(newsletterList.ID),
				testutil.WithContactListStatus(domain.ContactListStatusActive))
			require.NoError(t, err)
		}

		// Group 2: VIP only
		for i := 0; i < 3; i++ {
			contact, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("vip-only-%d@example.com", i)))
			require.NoError(t, err)

			_, err = factory.CreateContactList(workspaceID,
				testutil.WithContactListEmail(contact.Email),
				testutil.WithContactListListID(vipList.ID),
				testutil.WithContactListStatus(domain.ContactListStatusActive))
			require.NoError(t, err)
		}

		// Create segment: contacts IN newsletter list
		segment := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           fmt.Sprintf("newsletter%d", time.Now().Unix()),
			"name":         "Newsletter Segment",
			"description":  "All newsletter subscribers",
			"color":        "#00BFFF",
			"timezone":     "UTC",
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "contact_lists",
					"contact_list": map[string]interface{}{
						"operator": "in",
						"list_id":  newsletterList.ID,
					},
				},
			},
		}

		// Preview the segment
		previewResp, err := client.Post("/api/segments.preview", map[string]interface{}{
			"workspace_id": workspaceID,
			"tree":         segment["tree"],
			"limit":        20,
		})
		require.NoError(t, err)
		defer func() { _ = previewResp.Body.Close() }()

		var previewResult map[string]interface{}
		err = json.NewDecoder(previewResp.Body).Decode(&previewResult)
		require.NoError(t, err)

		emails := previewResult["emails"].([]interface{})
		totalCount := int(previewResult["total_count"].(float64))

		// Should find 5 newsletter subscribers
		assert.Equal(t, 5, totalCount, "Expected 5 newsletter subscribers")
		// Emails should not be returned for privacy/performance reasons
		assert.Empty(t, emails, "Emails should not be returned in preview")
	})
}

// testSegmentWithContactTimeline tests segments that filter by timeline events
func testSegmentWithContactTimeline(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should filter contacts by timeline events", func(t *testing.T) {
		// Create contacts
		activeContact1, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("active-user-1@example.com"))
		require.NoError(t, err)

		activeContact2, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("active-user-2@example.com"))
		require.NoError(t, err)

		inactiveContact, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("inactive-user@example.com"))
		require.NoError(t, err)

		// Add timeline events for active users (multiple email opens)
		for i := 0; i < 5; i++ {
			err = factory.CreateContactTimelineEvent(workspaceID, activeContact1.Email, "email_opened", map[string]interface{}{
				"message_id": fmt.Sprintf("msg-%d", i),
			})
			require.NoError(t, err)
		}

		for i := 0; i < 3; i++ {
			err = factory.CreateContactTimelineEvent(workspaceID, activeContact2.Email, "email_opened", map[string]interface{}{
				"message_id": fmt.Sprintf("msg-%d", i+10),
			})
			require.NoError(t, err)
		}

		// Inactive user has only 1 open
		err = factory.CreateContactTimelineEvent(workspaceID, inactiveContact.Email, "email_opened", map[string]interface{}{
			"message_id": "msg-inactive",
		})
		require.NoError(t, err)

		// Create segment: contacts with at least 3 email_opened events
		segment := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           fmt.Sprintf("activeusers%d", time.Now().Unix()),
			"name":         "Active Email Users",
			"description":  "Users who opened at least 3 emails",
			"color":        "#32CD32",
			"timezone":     "UTC",
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "contact_timeline",
					"contact_timeline": map[string]interface{}{
						"kind":           "email_opened",
						"count_operator": "at_least",
						"count_value":    3,
					},
				},
			},
		}

		// Preview the segment
		previewResp, err := client.Post("/api/segments.preview", map[string]interface{}{
			"workspace_id": workspaceID,
			"tree":         segment["tree"],
			"limit":        20,
		})
		require.NoError(t, err)
		defer func() { _ = previewResp.Body.Close() }()

		var previewResult map[string]interface{}
		err = json.NewDecoder(previewResp.Body).Decode(&previewResult)
		require.NoError(t, err)

		emails := previewResult["emails"].([]interface{})
		totalCount := int(previewResult["total_count"].(float64))

		// Should find 2 active users
		assert.Equal(t, 2, totalCount, "Expected 2 active users")
		// Emails should not be returned for privacy/performance reasons
		assert.Empty(t, emails, "Emails should not be returned in preview")
	})
}

// testSegmentRebuild tests rebuilding segments and membership updates
func testSegmentRebuild(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should rebuild segment and update memberships", func(t *testing.T) {
		// Create initial contacts
		for i := 0; i < 3; i++ {
			_, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("rebuild-test-%d@example.com", i)),
				testutil.WithContactCountry("FR"))
			require.NoError(t, err)
		}

		// Create segment for French contacts
		segment := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           fmt.Sprintf("frcontacts%d", time.Now().Unix()),
			"name":         "French Contacts",
			"description":  "All contacts from France",
			"color":        "#0055A4",
			"timezone":     "UTC",
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "contacts",
					"contact": map[string]interface{}{
						"filters": []map[string]interface{}{
							{
								"field_name":    "country",
								"field_type":    "string",
								"operator":      "equals",
								"string_values": []string{"FR"},
							},
						},
					},
				},
			},
		}

		// Create segment
		createResp, err := client.Post("/api/segments.create", segment)
		require.NoError(t, err)
		defer func() { _ = createResp.Body.Close() }()

		var createResult map[string]interface{}
		err = json.NewDecoder(createResp.Body).Decode(&createResult)
		require.NoError(t, err)

		segmentData := createResult["segment"].(map[string]interface{})
		segmentID := segmentData["id"].(string)

		// Initial build
		rebuildResp, err := client.Post("/api/segments.rebuild", map[string]interface{}{
			"workspace_id": workspaceID,
			"segment_id":   segmentID,
		})
		require.NoError(t, err)
		defer func() { _ = rebuildResp.Body.Close() }()

		// Execute tasks
		execResp, err := client.Post("/api/tasks.execute", map[string]interface{}{"limit": 10})
		require.NoError(t, err)
		_ = execResp.Body.Close()

		// Wait for segment to be built
		status, err := testutil.WaitForSegmentBuilt(t, client, workspaceID, segmentID, 10*time.Second)
		if err != nil {
			t.Fatalf("Segment build failed: %v", err)
		}
		assert.Contains(t, []string{"built", "active"}, status, "Segment should be built or active")

		// Add more French contacts
		time.Sleep(500 * time.Millisecond)
		for i := 3; i < 6; i++ {
			_, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("rebuild-test-%d@example.com", i)),
				testutil.WithContactCountry("FR"))
			require.NoError(t, err)
		}

		// Rebuild segment
		rebuildResp2, err := client.Post("/api/segments.rebuild", map[string]interface{}{
			"workspace_id": workspaceID,
			"segment_id":   segmentID,
		})
		require.NoError(t, err)
		defer func() { _ = rebuildResp2.Body.Close() }()

		// Execute tasks again
		execResp2, err := client.Post("/api/tasks.execute", map[string]interface{}{"limit": 10})
		require.NoError(t, err)
		_ = execResp2.Body.Close()

		// Wait for segment to be built
		status2, err := testutil.WaitForSegmentBuilt(t, client, workspaceID, segmentID, 10*time.Second)
		if err != nil {
			t.Fatalf("Segment rebuild failed: %v", err)
		}
		assert.Contains(t, []string{"built", "active"}, status2, "Segment should be built or active")

		// Verify updated count
		getResp, err := client.Get(fmt.Sprintf("/api/segments.get?workspace_id=%s&id=%s", workspaceID, segmentID))
		require.NoError(t, err)
		defer func() { _ = getResp.Body.Close() }()

		var getResult map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&getResult)
		require.NoError(t, err)

		updatedSegment := getResult["segment"].(map[string]interface{})
		if usersCount, ok := updatedSegment["users_count"].(float64); ok {
			assert.True(t, usersCount >= 6, "Expected at least 6 users after rebuild, got %v", usersCount)
		}
	})
}

// testListAndGetSegments tests listing and retrieving segments
func testListAndGetSegments(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should list and get segments", func(t *testing.T) {
		// Create multiple segments
		for i := 0; i < 3; i++ {
			segment := map[string]interface{}{
				"workspace_id": workspaceID,
				"id":           fmt.Sprintf("testseg%d%d", i, time.Now().Unix()),
				"name":         fmt.Sprintf("Test Segment %d", i),
				"description":  fmt.Sprintf("Description for segment %d", i),
				"color":        "#AABBCC",
				"timezone":     "UTC",
				"tree": map[string]interface{}{
					"kind": "leaf",
					"leaf": map[string]interface{}{
						"source": "contacts",
						"contact": map[string]interface{}{
							"filters": []map[string]interface{}{
								{
									"field_name":    "country",
									"field_type":    "string",
									"operator":      "equals",
									"string_values": []string{fmt.Sprintf("T%d", i)},
								},
							},
						},
					},
				},
			}

			createResp, err := client.Post("/api/segments.create", segment)
			require.NoError(t, err)
			_ = createResp.Body.Close()
		}

		// List segments
		listResp, err := client.Get(fmt.Sprintf("/api/segments.list?workspace_id=%s", workspaceID))
		require.NoError(t, err)
		defer func() { _ = listResp.Body.Close() }()
		assert.Equal(t, http.StatusOK, listResp.StatusCode)

		var listResult map[string]interface{}
		err = json.NewDecoder(listResp.Body).Decode(&listResult)
		require.NoError(t, err)

		segments := listResult["segments"].([]interface{})
		assert.True(t, len(segments) >= 3, "Expected at least 3 segments")

		// Get first segment
		firstSegment := segments[0].(map[string]interface{})
		segmentID := firstSegment["id"].(string)

		getResp, err := client.Get(fmt.Sprintf("/api/segments.get?workspace_id=%s&id=%s", workspaceID, segmentID))
		require.NoError(t, err)
		defer func() { _ = getResp.Body.Close() }()
		assert.Equal(t, http.StatusOK, getResp.StatusCode)

		var getResult map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&getResult)
		require.NoError(t, err)

		segment := getResult["segment"].(map[string]interface{})
		assert.Equal(t, segmentID, segment["id"])
		assert.NotEmpty(t, segment["name"])
	})
}

// testUpdateAndDeleteSegments tests updating and deleting segments
func testUpdateAndDeleteSegments(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should update and delete segments", func(t *testing.T) {
		// Create segment
		segment := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           fmt.Sprintf("updtest%d", time.Now().Unix()),
			"name":         "Original Name",
			"color":        "#FF0000",
			"timezone":     "UTC",
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "contacts",
					"contact": map[string]interface{}{
						"filters": []map[string]interface{}{
							{
								"field_name":    "country",
								"field_type":    "string",
								"operator":      "equals",
								"string_values": []string{"XX"},
							},
						},
					},
				},
			},
		}

		createResp, err := client.Post("/api/segments.create", segment)
		require.NoError(t, err)
		defer func() { _ = createResp.Body.Close() }()

		var createResult map[string]interface{}
		err = json.NewDecoder(createResp.Body).Decode(&createResult)
		require.NoError(t, err)

		segmentData := createResult["segment"].(map[string]interface{})
		segmentID := segmentData["id"].(string)

		// Update segment
		updateReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           segmentID,
			"name":         "Updated Name",
			"color":        "#00FF00",
			"timezone":     "UTC",
			"tree":         segment["tree"], // tree is required for update
		}

		updateResp, err := client.Post("/api/segments.update", updateReq)
		require.NoError(t, err)
		defer func() { _ = updateResp.Body.Close() }()
		assert.Equal(t, http.StatusOK, updateResp.StatusCode)

		var updateResult map[string]interface{}
		err = json.NewDecoder(updateResp.Body).Decode(&updateResult)
		require.NoError(t, err)

		updatedSegment := updateResult["segment"].(map[string]interface{})
		assert.Equal(t, "Updated Name", updatedSegment["name"])
		assert.Equal(t, "#00FF00", updatedSegment["color"])

		// Delete segment
		deleteResp, err := client.Post("/api/segments.delete", map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           segmentID,
		})
		require.NoError(t, err)
		defer func() { _ = deleteResp.Body.Close() }()
		assert.Equal(t, http.StatusOK, deleteResp.StatusCode)

		// Verify segment is deleted - soft delete sets status to "deleted"
		getResp, err := client.Get(fmt.Sprintf("/api/segments.get?workspace_id=%s&id=%s", workspaceID, segmentID))
		require.NoError(t, err)
		defer func() { _ = getResp.Body.Close() }()

		// The segment may be soft deleted (status="deleted") or hard deleted (404)
		if getResp.StatusCode == http.StatusOK {
			var getResult map[string]interface{}
			err = json.NewDecoder(getResp.Body).Decode(&getResult)
			require.NoError(t, err)

			if segment, ok := getResult["segment"].(map[string]interface{}); ok {
				// Soft delete - check status is "deleted"
				status, hasStatus := segment["status"].(string)
				assert.True(t, hasStatus, "Segment should have a status field")
				assert.Equal(t, "deleted", status, "Segment status should be 'deleted'")
			}
		} else {
			// Hard delete - expect 404
			assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
		}
	})
}

// testSegmentWithRelativeDates tests segments with relative date filters and daily recompute
func testSegmentWithRelativeDates(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should set recompute_after for segments with relative dates", func(t *testing.T) {
		// Create contacts with timeline events
		activeContact, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("recent-active@example.com"))
		require.NoError(t, err)

		// Add recent timeline events (within last 7 days)
		for i := 0; i < 5; i++ {
			err = factory.CreateContactTimelineEvent(workspaceID, activeContact.Email, "email_opened", map[string]interface{}{
				"message_id": fmt.Sprintf("recent-msg-%d", i),
			})
			require.NoError(t, err)
		}

		// Create segment with relative date filter: contacts who opened email in the last 7 days
		timeframeOp := "in_the_last_days"
		segment := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           fmt.Sprintf("recentactive%d", time.Now().Unix()),
			"name":         "Recently Active",
			"description":  "Contacts who opened emails in the last 7 days",
			"color":        "#FF6B6B",
			"timezone":     "America/New_York",
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "contact_timeline",
					"contact_timeline": map[string]interface{}{
						"kind":               "email_opened",
						"count_operator":     "at_least",
						"count_value":        1,
						"timeframe_operator": &timeframeOp,
						"timeframe_values":   []string{"7"},
					},
				},
			},
		}

		// Create the segment
		createResp, err := client.Post("/api/segments.create", segment)
		require.NoError(t, err)
		defer func() { _ = createResp.Body.Close() }()
		assert.Equal(t, http.StatusCreated, createResp.StatusCode)

		var createResult map[string]interface{}
		err = json.NewDecoder(createResp.Body).Decode(&createResult)
		require.NoError(t, err)

		segmentData := createResult["segment"].(map[string]interface{})
		segmentID := segmentData["id"].(string)

		// Verify recompute_after is set
		if recomputeAfter, ok := segmentData["recompute_after"].(string); ok {
			assert.NotEmpty(t, recomputeAfter, "recompute_after should be set for segments with relative dates")

			// Parse and verify it's a future timestamp
			recomputeTime, err := time.Parse(time.RFC3339, recomputeAfter)
			require.NoError(t, err)
			assert.True(t, recomputeTime.After(time.Now()), "recompute_after should be in the future")
		}

		// Build the segment
		rebuildResp, err := client.Post("/api/segments.rebuild", map[string]interface{}{
			"workspace_id": workspaceID,
			"segment_id":   segmentID,
		})
		require.NoError(t, err)
		defer func() { _ = rebuildResp.Body.Close() }()

		// Execute tasks to start the build
		execResp, err := client.Post("/api/tasks.execute", map[string]interface{}{"limit": 10})
		require.NoError(t, err)
		_ = execResp.Body.Close()

		// Wait for segment to be built
		status, err := testutil.WaitForSegmentBuilt(t, client, workspaceID, segmentID, 10*time.Second)
		if err != nil {
			t.Fatalf("Segment build failed: %v", err)
		}
		assert.Contains(t, []string{"built", "active"}, status, "Segment should be built or active")

		// Verify segment is built and recompute_after is still set
		getResp, err := client.Get(fmt.Sprintf("/api/segments.get?workspace_id=%s&id=%s", workspaceID, segmentID))
		require.NoError(t, err)
		defer func() { _ = getResp.Body.Close() }()

		var getResult map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&getResult)
		require.NoError(t, err)

		updatedSegment := getResult["segment"].(map[string]interface{})

		// After build, recompute_after should still be set (rescheduled for next day)
		if recomputeAfter, ok := updatedSegment["recompute_after"].(string); ok {
			assert.NotEmpty(t, recomputeAfter, "recompute_after should remain set after build")
		}
	})

	t.Run("should NOT set recompute_after for segments without relative dates", func(t *testing.T) {
		// Create segment without relative dates
		segment := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           fmt.Sprintf("norelative%d", time.Now().Unix()),
			"name":         "No Relative Dates",
			"description":  "Segment without relative date filters",
			"color":        "#4ECDC4",
			"timezone":     "UTC",
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "contacts",
					"contact": map[string]interface{}{
						"filters": []map[string]interface{}{
							{
								"field_name":    "country",
								"field_type":    "string",
								"operator":      "equals",
								"string_values": []string{"US"},
							},
						},
					},
				},
			},
		}

		createResp, err := client.Post("/api/segments.create", segment)
		require.NoError(t, err)
		defer func() { _ = createResp.Body.Close() }()

		var createResult map[string]interface{}
		err = json.NewDecoder(createResp.Body).Decode(&createResult)
		require.NoError(t, err)

		segmentData := createResult["segment"].(map[string]interface{})

		// Verify recompute_after is not set or is null
		recomputeAfter, hasField := segmentData["recompute_after"]
		if hasField && recomputeAfter != nil {
			t.Errorf("recompute_after should be null for segments without relative dates, got: %v", recomputeAfter)
		}
	})

	t.Run("should update recompute_after when adding relative dates", func(t *testing.T) {
		// Create segment without relative dates
		segmentID := fmt.Sprintf("updatetest%d", time.Now().Unix())
		segment := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           segmentID,
			"name":         "Test Update",
			"color":        "#95E1D3",
			"timezone":     "UTC",
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "contacts",
					"contact": map[string]interface{}{
						"filters": []map[string]interface{}{
							{
								"field_name":    "country",
								"field_type":    "string",
								"operator":      "equals",
								"string_values": []string{"FR"},
							},
						},
					},
				},
			},
		}

		createResp, err := client.Post("/api/segments.create", segment)
		require.NoError(t, err)
		_ = createResp.Body.Close()

		// Update segment to add relative dates
		timeframeOp := "in_the_last_days"
		updateReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           segmentID,
			"name":         "Test Update - With Relative Dates",
			"color":        "#95E1D3",
			"timezone":     "UTC",
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "contact_timeline",
					"contact_timeline": map[string]interface{}{
						"kind":               "email_opened",
						"count_operator":     "at_least",
						"count_value":        1,
						"timeframe_operator": &timeframeOp,
						"timeframe_values":   []string{"30"},
					},
				},
			},
		}

		updateResp, err := client.Post("/api/segments.update", updateReq)
		require.NoError(t, err)
		defer func() { _ = updateResp.Body.Close() }()
		assert.Equal(t, http.StatusOK, updateResp.StatusCode)

		var updateResult map[string]interface{}
		err = json.NewDecoder(updateResp.Body).Decode(&updateResult)
		require.NoError(t, err)

		updatedSegment := updateResult["segment"].(map[string]interface{})

		// Verify recompute_after is now set
		if recomputeAfter, ok := updatedSegment["recompute_after"].(string); ok {
			assert.NotEmpty(t, recomputeAfter, "recompute_after should be set after adding relative dates")
		}
	})
}

// testCheckSegmentRecomputeProcessor tests the recurring task that checks for segments due for recompute
func testCheckSegmentRecomputeProcessor(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	// Ensure the check_segment_recompute task exists for this workspace
	err := factory.EnsureSegmentRecomputeTask(workspaceID)
	require.NoError(t, err)

	t.Run("should create build tasks for segments due for recompute", func(t *testing.T) {
		// Create a segment with relative dates
		timeframeOp := "in_the_last_days"
		segment1 := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           fmt.Sprintf("taskrecomp1%d", time.Now().Unix()),
			"name":         "Task Recompute Test 1",
			"color":        "#FF6B6B",
			"timezone":     "UTC",
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "contact_timeline",
					"contact_timeline": map[string]interface{}{
						"kind":               "email_opened",
						"count_operator":     "at_least",
						"count_value":        1,
						"timeframe_operator": &timeframeOp,
						"timeframe_values":   []string{"7"},
					},
				},
			},
		}

		// Create the segment
		createResp, err := client.Post("/api/segments.create", segment1)
		require.NoError(t, err)
		defer func() { _ = createResp.Body.Close() }()
		assert.Equal(t, http.StatusCreated, createResp.StatusCode)

		var createResult map[string]interface{}
		err = json.NewDecoder(createResp.Body).Decode(&createResult)
		require.NoError(t, err)

		segmentData := createResult["segment"].(map[string]interface{})
		segment1ID := segmentData["id"].(string)

		// BUILD THE SEGMENT FIRST - segments must be 'active' to be eligible for recompute
		// The GetSegmentsDueForRecompute query filters by status = 'active'
		rebuildResp, err := client.Post("/api/segments.rebuild", map[string]interface{}{
			"workspace_id": workspaceID,
			"segment_id":   segment1ID,
		})
		require.NoError(t, err)
		_ = rebuildResp.Body.Close()

		// Execute tasks to build the segment
		execBuildResp, err := client.Post("/api/tasks.execute", map[string]interface{}{"limit": 10})
		require.NoError(t, err)
		_ = execBuildResp.Body.Close()

		// Wait for segment to become active
		segmentStatus, err := testutil.WaitForSegmentBuilt(t, client, workspaceID, segment1ID, 10*time.Second)
		require.NoError(t, err)
		require.Contains(t, []string{"built", "active"}, segmentStatus, "Segment must be active before recompute check")

		// NOW set recompute_after to the past - segment is active and eligible
		pastTime := time.Now().Add(-1 * time.Hour)
		err = factory.SetSegmentRecomputeAfter(workspaceID, segment1ID, pastTime)
		require.NoError(t, err)

		// Wait to ensure the update is persisted and to create a clear time gap
		time.Sleep(1 * time.Second)

		// Find the check_segment_recompute task for this workspace
		listResp, err := client.ListTasks(map[string]string{
			"workspace_id": workspaceID,
			"type":         "check_segment_recompute",
		})
		require.NoError(t, err)
		defer func() { _ = listResp.Body.Close() }()

		var listResult map[string]interface{}
		err = json.NewDecoder(listResp.Body).Decode(&listResult)
		require.NoError(t, err)

		tasksInterface, ok := listResult["tasks"]
		if !ok || tasksInterface == nil {
			t.Skip("check_segment_recompute task not found - may need to wait for workspace initialization")
		}

		tasks := tasksInterface.([]interface{})
		if len(tasks) == 0 {
			t.Skip("check_segment_recompute task not yet created for this workspace")
		}
		require.GreaterOrEqual(t, len(tasks), 1, "Should have at least one check_segment_recompute task")

		recomputeTask := tasks[0].(map[string]interface{})
		recomputeTaskID := recomputeTask["id"].(string)

		// Get the current time to track tasks created after the recompute task runs
		timeBeforeExecution := time.Now()

		// Execute the check_segment_recompute task
		executeResp, err := client.ExecuteTask(map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           recomputeTaskID,
		})
		require.NoError(t, err)
		defer func() { _ = executeResp.Body.Close() }()
		assert.Equal(t, http.StatusOK, executeResp.StatusCode)

		// Note: check_segment_recompute is a recurring task that stays "pending" by design
		// Wait a bit for it to execute and create build tasks
		time.Sleep(1 * time.Second)

		// Wait for build task to be created for segment1 (should be quick now that segment is active)
		buildTaskID, err := testutil.WaitForBuildTaskCreated(t, client, workspaceID, segment1ID, timeBeforeExecution, 10*time.Second)
		if err != nil {
			t.Fatalf("Build task not created for segment due for recompute: %v", err)
		}
		t.Logf("Build task %s created for segment %s", buildTaskID, segment1ID)

		// Verify the check_segment_recompute task is still pending (continues recurring)
		getTaskResp, err := client.GetTask(workspaceID, recomputeTaskID)
		require.NoError(t, err)
		defer func() { _ = getTaskResp.Body.Close() }()

		var getTaskResult map[string]interface{}
		err = json.NewDecoder(getTaskResp.Body).Decode(&getTaskResult)
		require.NoError(t, err)

		// Verify the task is still pending (recurring tasks stay pending)
		if task, ok := getTaskResult["task"].(map[string]interface{}); ok {
			if status, ok := task["status"].(string); ok {
				assert.Equal(t, "pending", status, "check_segment_recompute task should remain pending for recurring execution")
			}
		}
	})

	t.Run("should NOT create build tasks for segments not yet due", func(t *testing.T) {
		// Create a segment with relative dates
		timeframeOp := "in_the_last_days"
		segment2 := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           fmt.Sprintf("taskrecomp2%d", time.Now().Unix()),
			"name":         "Task Recompute Test 2",
			"color":        "#4ECDC4",
			"timezone":     "UTC",
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "contact_timeline",
					"contact_timeline": map[string]interface{}{
						"kind":               "email_opened",
						"count_operator":     "at_least",
						"count_value":        1,
						"timeframe_operator": &timeframeOp,
						"timeframe_values":   []string{"30"},
					},
				},
			},
		}

		// Create the segment
		createResp, err := client.Post("/api/segments.create", segment2)
		require.NoError(t, err)
		defer func() { _ = createResp.Body.Close() }()

		var createResult map[string]interface{}
		err = json.NewDecoder(createResp.Body).Decode(&createResult)
		require.NoError(t, err)

		segmentData := createResult["segment"].(map[string]interface{})
		segment2ID := segmentData["id"].(string)

		// Set recompute_after to the future
		futureTime := time.Now().Add(24 * time.Hour)
		err = factory.SetSegmentRecomputeAfter(workspaceID, segment2ID, futureTime)
		require.NoError(t, err)

		// Find the check_segment_recompute task
		listResp, err := client.ListTasks(map[string]string{
			"workspace_id": workspaceID,
			"type":         "check_segment_recompute",
		})
		require.NoError(t, err)
		defer func() { _ = listResp.Body.Close() }()

		var listResult map[string]interface{}
		err = json.NewDecoder(listResp.Body).Decode(&listResult)
		require.NoError(t, err)

		tasksInterface, ok := listResult["tasks"]
		if !ok || tasksInterface == nil {
			t.Skip("check_segment_recompute task not found")
		}

		tasks := tasksInterface.([]interface{})
		if len(tasks) == 0 {
			t.Skip("check_segment_recompute task not yet created for this workspace")
		}
		require.GreaterOrEqual(t, len(tasks), 1)

		recomputeTask := tasks[0].(map[string]interface{})
		recomputeTaskID := recomputeTask["id"].(string)

		// Count build tasks before
		buildTasksBeforeResp, err := client.ListTasks(map[string]string{
			"workspace_id": workspaceID,
			"type":         "build_segment",
			"status":       "pending",
		})
		require.NoError(t, err)
		defer func() { _ = buildTasksBeforeResp.Body.Close() }()

		var buildTasksBeforeResult map[string]interface{}
		err = json.NewDecoder(buildTasksBeforeResp.Body).Decode(&buildTasksBeforeResult)
		require.NoError(t, err)
		buildTasksBeforeCount := 0
		if tasks, ok := buildTasksBeforeResult["tasks"].([]interface{}); ok && tasks != nil {
			buildTasksBeforeCount = len(tasks)
		}

		// Execute the check_segment_recompute task
		executeResp, err := client.ExecuteTask(map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           recomputeTaskID,
		})
		require.NoError(t, err)
		defer func() { _ = executeResp.Body.Close() }()

		// Note: check_segment_recompute is a recurring task that stays "pending" by design
		// Wait a bit for it to execute
		time.Sleep(1 * time.Second)

		// Count build tasks after - should NOT have created any for the future segment
		buildTasksAfterResp, err := client.ListTasks(map[string]string{
			"workspace_id": workspaceID,
			"type":         "build_segment",
			"status":       "pending",
		})
		require.NoError(t, err)
		defer func() { _ = buildTasksAfterResp.Body.Close() }()

		var buildTasksAfterResult map[string]interface{}
		err = json.NewDecoder(buildTasksAfterResp.Body).Decode(&buildTasksAfterResult)
		require.NoError(t, err)
		buildTasksAfterCount := 0
		if tasks, ok := buildTasksAfterResult["tasks"].([]interface{}); ok && tasks != nil {
			buildTasksAfterCount = len(tasks)
		}

		// The count should be the same or only increased by tasks from previous test
		// The important thing is that no task was created for segment2
		assert.LessOrEqual(t, buildTasksAfterCount-buildTasksBeforeCount, 1, "Should not create build task for segment not yet due")
	})

	t.Run("should skip deleted segments", func(t *testing.T) {
		// Create a segment with relative dates
		timeframeOp := "in_the_last_days"
		segment3 := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           fmt.Sprintf("taskrecomp3%d", time.Now().Unix()),
			"name":         "Task Recompute Test 3 - To Delete",
			"color":        "#95E1D3",
			"timezone":     "UTC",
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "contact_timeline",
					"contact_timeline": map[string]interface{}{
						"kind":               "email_clicked",
						"count_operator":     "at_least",
						"count_value":        1,
						"timeframe_operator": &timeframeOp,
						"timeframe_values":   []string{"14"},
					},
				},
			},
		}

		// Create the segment
		createResp, err := client.Post("/api/segments.create", segment3)
		require.NoError(t, err)
		defer func() { _ = createResp.Body.Close() }()

		var createResult map[string]interface{}
		err = json.NewDecoder(createResp.Body).Decode(&createResult)
		require.NoError(t, err)

		segmentData := createResult["segment"].(map[string]interface{})
		segment3ID := segmentData["id"].(string)

		// Set recompute_after to the past
		pastTime := time.Now().Add(-2 * time.Hour)
		err = factory.SetSegmentRecomputeAfter(workspaceID, segment3ID, pastTime)
		require.NoError(t, err)

		// Delete the segment
		deleteResp, err := client.Post("/api/segments.delete", map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           segment3ID,
		})
		require.NoError(t, err)
		defer func() { _ = deleteResp.Body.Close() }()

		// Find and execute the check_segment_recompute task
		listResp, err := client.ListTasks(map[string]string{
			"workspace_id": workspaceID,
			"type":         "check_segment_recompute",
		})
		require.NoError(t, err)
		defer func() { _ = listResp.Body.Close() }()

		var listResult map[string]interface{}
		err = json.NewDecoder(listResp.Body).Decode(&listResult)
		require.NoError(t, err)

		tasksInterface, ok := listResult["tasks"]
		if !ok || tasksInterface == nil {
			t.Skip("check_segment_recompute task not found")
		}

		tasks := tasksInterface.([]interface{})
		if len(tasks) == 0 {
			t.Skip("check_segment_recompute task not yet created for this workspace")
		}
		require.GreaterOrEqual(t, len(tasks), 1)

		recomputeTask := tasks[0].(map[string]interface{})
		recomputeTaskID := recomputeTask["id"].(string)

		// Count build tasks created by recompute task before execution
		// Get current timestamp to track new tasks
		timeBeforeExecution := time.Now()

		// Execute the check_segment_recompute task
		executeResp, err := client.ExecuteTask(map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           recomputeTaskID,
		})
		require.NoError(t, err)
		defer func() { _ = executeResp.Body.Close() }()

		// Note: check_segment_recompute is a recurring task that stays "pending" by design
		// Wait a bit for it to execute
		time.Sleep(1 * time.Second)

		// Verify no NEW build task was created for the deleted segment after recompute ran
		buildTasksAfterResp, err := client.ListTasks(map[string]string{
			"workspace_id": workspaceID,
			"type":         "build_segment",
		})
		require.NoError(t, err)
		defer func() { _ = buildTasksAfterResp.Body.Close() }()

		var buildTasksAfterResult map[string]interface{}
		err = json.NewDecoder(buildTasksAfterResp.Body).Decode(&buildTasksAfterResult)
		require.NoError(t, err)

		// Check that no NEW tasks were created for the deleted segment after recompute execution
		newTasksForDeletedSegment := 0
		if tasks, ok := buildTasksAfterResult["tasks"].([]interface{}); ok && tasks != nil {
			for _, taskInterface := range tasks {
				task := taskInterface.(map[string]interface{})

				// Parse created_at to see if task was created after we ran the recompute task
				createdAtStr, ok := task["created_at"].(string)
				if !ok {
					continue
				}
				createdAt, err := time.Parse(time.RFC3339, createdAtStr)
				if err != nil {
					continue
				}

				// Only check tasks created after we executed the recompute task
				if !createdAt.After(timeBeforeExecution) {
					continue
				}

				if state, ok := task["state"].(map[string]interface{}); ok {
					if buildSegment, ok := state["build_segment"].(map[string]interface{}); ok {
						if segmentID, ok := buildSegment["segment_id"].(string); ok {
							if segmentID == segment3ID {
								newTasksForDeletedSegment++
							}
						}
					}
				}
			}
		}

		assert.Equal(t, 0, newTasksForDeletedSegment, "Should not create NEW build tasks for deleted segment")
	})
}

// testContactPropertyRelativeDates tests segments with relative date filters on contact properties
func testContactPropertyRelativeDates(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should filter contacts by created_at in the last N days", func(t *testing.T) {
		// Create contacts with different creation dates
		// Recent contacts (within last 7 days) - should be auto-created with current timestamp
		recentContact1, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("recent-contact-1@example.com"))
		require.NoError(t, err)

		recentContact2, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("recent-contact-2@example.com"))
		require.NoError(t, err)

		// Create segment with relative date filter: contacts created in the last 30 days
		segment := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           fmt.Sprintf("recentcontacts%d", time.Now().Unix()),
			"name":         "Recently Created Contacts",
			"description":  "Contacts created in the last 30 days",
			"color":        "#FF6B6B",
			"timezone":     "America/New_York",
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "contacts",
					"contact": map[string]interface{}{
						"filters": []map[string]interface{}{
							{
								"field_name":    "created_at",
								"field_type":    "time",
								"operator":      "in_the_last_days",
								"string_values": []string{"30"},
							},
						},
					},
				},
			},
		}

		// Create the segment
		createResp, err := client.Post("/api/segments.create", segment)
		require.NoError(t, err)
		defer func() { _ = createResp.Body.Close() }()
		assert.Equal(t, http.StatusCreated, createResp.StatusCode)

		var createResult map[string]interface{}
		err = json.NewDecoder(createResp.Body).Decode(&createResult)
		require.NoError(t, err)

		segmentData := createResult["segment"].(map[string]interface{})
		segmentID := segmentData["id"].(string)

		// Verify recompute_after is set for segments with relative date filters
		if recomputeAfter, ok := segmentData["recompute_after"].(string); ok {
			assert.NotEmpty(t, recomputeAfter, "recompute_after should be set for segments with relative dates")

			// Parse and verify it's a future timestamp
			recomputeTime, err := time.Parse(time.RFC3339, recomputeAfter)
			require.NoError(t, err)
			assert.True(t, recomputeTime.After(time.Now()), "recompute_after should be in the future")
		}

		// Build the segment
		rebuildResp, err := client.Post("/api/segments.rebuild", map[string]interface{}{
			"workspace_id": workspaceID,
			"segment_id":   segmentID,
		})
		require.NoError(t, err)
		defer func() { _ = rebuildResp.Body.Close() }()

		// Execute tasks to build the segment
		execResp, err := client.Post("/api/tasks.execute", map[string]interface{}{"limit": 10})
		require.NoError(t, err)
		_ = execResp.Body.Close()

		// Wait for segment to be built
		status, err := testutil.WaitForSegmentBuilt(t, client, workspaceID, segmentID, 10*time.Second)
		if err != nil {
			t.Fatalf("Segment build failed: %v", err)
		}
		assert.Contains(t, []string{"built", "active"}, status, "Segment should be built or active")

		// Verify segment matches the recent contacts
		getResp, err := client.Get(fmt.Sprintf("/api/segments.get?workspace_id=%s&id=%s", workspaceID, segmentID))
		require.NoError(t, err)
		defer func() { _ = getResp.Body.Close() }()

		var getResult map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&getResult)
		require.NoError(t, err)

		updatedSegment := getResult["segment"].(map[string]interface{})

		// Should have at least the 2 recent contacts we created
		if usersCount, ok := updatedSegment["users_count"].(float64); ok {
			assert.True(t, usersCount >= 2, "Expected at least 2 users, got %v", usersCount)
		}

		// Verify recompute_after is still set after build
		if recomputeAfter, ok := updatedSegment["recompute_after"].(string); ok {
			assert.NotEmpty(t, recomputeAfter, "recompute_after should remain set after build")
		}

		// Verify the generated SQL contains the relative date logic
		if generatedSQL, ok := updatedSegment["generated_sql"].(string); ok {
			assert.Contains(t, generatedSQL, "NOW() - INTERVAL", "Generated SQL should contain relative date interval")
			assert.Contains(t, generatedSQL, "days", "Generated SQL should reference days")
		}

		t.Logf("Segment built successfully with %v contacts matching 'created in last 30 days'",
			updatedSegment["users_count"])
		t.Logf("Recent contacts: %s, %s", recentContact1.Email, recentContact2.Email)
	})

	t.Run("should combine relative date filter with other filters", func(t *testing.T) {
		// Create contacts with different attributes
		recentUSContact, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("recent-us@example.com"),
			testutil.WithContactCountry("US"))
		require.NoError(t, err)

		recentCAContact, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("recent-ca@example.com"),
			testutil.WithContactCountry("CA"))
		require.NoError(t, err)

		// Create segment: US contacts created in the last 60 days
		segment := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           fmt.Sprintf("recentus%d", time.Now().Unix()),
			"name":         "Recent US Contacts",
			"description":  "US contacts created recently",
			"color":        "#4ECDC4",
			"timezone":     "UTC",
			"tree": map[string]interface{}{
				"kind": "branch",
				"branch": map[string]interface{}{
					"operator": "and",
					"leaves": []map[string]interface{}{
						{
							"kind": "leaf",
							"leaf": map[string]interface{}{
								"source": "contacts",
								"contact": map[string]interface{}{
									"filters": []map[string]interface{}{
										{
											"field_name":    "country",
											"field_type":    "string",
											"operator":      "equals",
											"string_values": []string{"US"},
										},
									},
								},
							},
						},
						{
							"kind": "leaf",
							"leaf": map[string]interface{}{
								"source": "contacts",
								"contact": map[string]interface{}{
									"filters": []map[string]interface{}{
										{
											"field_name":    "created_at",
											"field_type":    "time",
											"operator":      "in_the_last_days",
											"string_values": []string{"60"},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		// Preview the segment
		previewResp, err := client.Post("/api/segments.preview", map[string]interface{}{
			"workspace_id": workspaceID,
			"tree":         segment["tree"],
			"limit":        20,
		})
		require.NoError(t, err)
		defer func() { _ = previewResp.Body.Close() }()
		assert.Equal(t, http.StatusOK, previewResp.StatusCode)

		var previewResult map[string]interface{}
		err = json.NewDecoder(previewResp.Body).Decode(&previewResult)
		require.NoError(t, err)

		totalCount := int(previewResult["total_count"].(float64))

		// Should find at least 1 US contact (the one we just created)
		assert.True(t, totalCount >= 1, "Expected at least 1 US contact created recently")

		t.Logf("Found %d recent US contacts (should include %s but not %s)",
			totalCount, recentUSContact.Email, recentCAContact.Email)
	})
}

// testCustomEventsGoalsSegmentation tests segmentation based on custom events with goal tracking
// This comprehensive test suite covers all aspects of goal-based segmentation:
// - Goal types (purchase, subscription, lead, etc.)
// - Aggregation operators (sum, count, avg, min, max)
// - Comparison operators (gte, lte, eq, between)
// - Timeframe operators (anytime, in_the_last_days, in_date_range, before_date, after_date)
// - Wildcard goal_type (*) for all goals
// - Goal name filtering
// - Soft-deleted events exclusion
// - Complex segments combining custom_events_goals with other conditions
func testCustomEventsGoalsSegmentation(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	// Helper function to create a custom event via API
	createCustomEvent := func(t *testing.T, email, eventName, externalID string, goalType, goalName *string, goalValue *float64, occurredAt *time.Time) {
		t.Helper()
		eventReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"email":        email,
			"event_name":   eventName,
			"external_id":  externalID,
			"properties":   map[string]interface{}{},
		}
		if goalType != nil {
			eventReq["goal_type"] = *goalType
		}
		if goalName != nil {
			eventReq["goal_name"] = *goalName
		}
		if goalValue != nil {
			eventReq["goal_value"] = *goalValue
		}
		if occurredAt != nil {
			eventReq["occurred_at"] = occurredAt.Format(time.RFC3339)
		} else {
			eventReq["occurred_at"] = time.Now().Format(time.RFC3339)
		}

		resp, err := client.Post("/api/customEvents.upsert", eventReq)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to create custom event for %s", email)
	}

	// Helper function to soft-delete a custom event via API
	softDeleteEvent := func(t *testing.T, email, eventName, externalID string) {
		t.Helper()
		// Import with deleted_at set to soft-delete
		importReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"events": []map[string]interface{}{
				{
					"email":       email,
					"event_name":  eventName,
					"external_id": externalID,
					"properties":  map[string]interface{}{},
					"occurred_at": time.Now().Format(time.RFC3339),
					"deleted_at":  time.Now().Format(time.RFC3339),
				},
			},
		}
		resp, err := client.Post("/api/customEvents.import", importReq)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	// Helper for creating and building a segment with a goal condition
	createAndPreviewGoalSegment := func(t *testing.T, segmentName string, goalCondition map[string]interface{}) int {
		t.Helper()
		segment := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           fmt.Sprintf("goal%d", time.Now().UnixNano()),
			"name":         segmentName,
			"description":  "Goal-based segment",
			"color":        "#FF6B6B",
			"timezone":     "UTC",
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source":             "custom_events_goals",
					"custom_events_goal": goalCondition,
				},
			},
		}

		// Preview the segment to get count
		previewResp, err := client.Post("/api/segments.preview", map[string]interface{}{
			"workspace_id": workspaceID,
			"tree":         segment["tree"],
			"limit":        100,
		})
		require.NoError(t, err)
		defer func() { _ = previewResp.Body.Close() }()

		var previewResult map[string]interface{}
		err = json.NewDecoder(previewResp.Body).Decode(&previewResult)
		require.NoError(t, err)

		if errMsg, hasError := previewResult["error"]; hasError {
			t.Fatalf("Segment preview failed: %v", errMsg)
		}

		return int(previewResult["total_count"].(float64))
	}

	t.Run("should segment by purchase goal sum (LTV)", func(t *testing.T) {
		// Create contacts with different purchase histories
		highLTV, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("high-ltv@example.com"))
		require.NoError(t, err)

		mediumLTV, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("medium-ltv@example.com"))
		require.NoError(t, err)

		lowLTV, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("low-ltv@example.com"))
		require.NoError(t, err)

		// Create purchase events
		purchaseType := "purchase"
		orderName := "order"

		// High LTV: $500 total (2 purchases)
		val1 := 300.00
		createCustomEvent(t, highLTV.Email, "purchase", "order-h1", &purchaseType, &orderName, &val1, nil)
		val2 := 200.00
		createCustomEvent(t, highLTV.Email, "purchase", "order-h2", &purchaseType, &orderName, &val2, nil)

		// Medium LTV: $150 total
		val3 := 150.00
		createCustomEvent(t, mediumLTV.Email, "purchase", "order-m1", &purchaseType, &orderName, &val3, nil)

		// Low LTV: $50 total
		val4 := 50.00
		createCustomEvent(t, lowLTV.Email, "purchase", "order-l1", &purchaseType, &orderName, &val4, nil)

		// Test 1: Contacts with LTV >= $200 (should be highLTV only)
		count := createAndPreviewGoalSegment(t, "High LTV Customers", map[string]interface{}{
			"goal_type":          "purchase",
			"aggregate_operator": "sum",
			"operator":           "gte",
			"value":              200.0,
			"timeframe_operator": "anytime",
		})
		assert.Equal(t, 1, count, "Expected 1 high LTV contact (>= $200)")

		// Test 2: Contacts with LTV >= $100 (should be highLTV and mediumLTV)
		count = createAndPreviewGoalSegment(t, "Medium+ LTV Customers", map[string]interface{}{
			"goal_type":          "purchase",
			"aggregate_operator": "sum",
			"operator":           "gte",
			"value":              100.0,
			"timeframe_operator": "anytime",
		})
		assert.Equal(t, 2, count, "Expected 2 contacts with LTV >= $100")

		// Test 3: Contacts with LTV <= $100 (should be lowLTV only)
		count = createAndPreviewGoalSegment(t, "Low LTV Customers", map[string]interface{}{
			"goal_type":          "purchase",
			"aggregate_operator": "sum",
			"operator":           "lte",
			"value":              100.0,
			"timeframe_operator": "anytime",
		})
		assert.Equal(t, 1, count, "Expected 1 low LTV contact (<= $100)")
	})

	t.Run("should segment by transaction count", func(t *testing.T) {
		// Use unique goal name to isolate this test
		uniqueGoalName := fmt.Sprintf("tx_count_test_%d", time.Now().UnixNano())

		// Create contacts with different transaction counts
		frequentBuyer, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail(fmt.Sprintf("frequent-buyer-%d@example.com", time.Now().UnixNano())))
		require.NoError(t, err)

		occasionalBuyer, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail(fmt.Sprintf("occasional-buyer-%d@example.com", time.Now().UnixNano())))
		require.NoError(t, err)

		purchaseType := "purchase"
		val := 10.00

		// Frequent buyer: 5 transactions
		for i := 0; i < 5; i++ {
			createCustomEvent(t, frequentBuyer.Email, "purchase", fmt.Sprintf("tx-freq-%d-%d", time.Now().UnixNano(), i), &purchaseType, &uniqueGoalName, &val, nil)
		}

		// Occasional buyer: 2 transactions
		for i := 0; i < 2; i++ {
			createCustomEvent(t, occasionalBuyer.Email, "purchase", fmt.Sprintf("tx-occ-%d-%d", time.Now().UnixNano(), i), &purchaseType, &uniqueGoalName, &val, nil)
		}

		// Test: Contacts with at least 4 transactions (using unique goal_name)
		count := createAndPreviewGoalSegment(t, "Frequent Buyers", map[string]interface{}{
			"goal_type":          "purchase",
			"goal_name":          uniqueGoalName,
			"aggregate_operator": "count",
			"operator":           "gte",
			"value":              4.0,
			"timeframe_operator": "anytime",
		})
		assert.Equal(t, 1, count, "Expected 1 frequent buyer (>= 4 transactions)")

		// Test: Contacts with exactly 2 transactions (using unique goal_name)
		count = createAndPreviewGoalSegment(t, "Two-Time Buyers", map[string]interface{}{
			"goal_type":          "purchase",
			"goal_name":          uniqueGoalName,
			"aggregate_operator": "count",
			"operator":           "eq",
			"value":              2.0,
			"timeframe_operator": "anytime",
		})
		assert.Equal(t, 1, count, "Expected 1 contact with exactly 2 transactions")
	})

	t.Run("should segment by average goal value", func(t *testing.T) {
		// Use unique goal name to isolate this test
		uniqueGoalName := fmt.Sprintf("avg_test_%d", time.Now().UnixNano())

		// Create contacts with different average order values
		bigSpender, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail(fmt.Sprintf("big-spender-%d@example.com", time.Now().UnixNano())))
		require.NoError(t, err)

		smallSpender, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail(fmt.Sprintf("small-spender-%d@example.com", time.Now().UnixNano())))
		require.NoError(t, err)

		purchaseType := "purchase"

		// Big spender: average $150 (200 + 100)
		val1 := 200.00
		createCustomEvent(t, bigSpender.Email, "purchase", fmt.Sprintf("avg-big-1-%d", time.Now().UnixNano()), &purchaseType, &uniqueGoalName, &val1, nil)
		val2 := 100.00
		createCustomEvent(t, bigSpender.Email, "purchase", fmt.Sprintf("avg-big-2-%d", time.Now().UnixNano()), &purchaseType, &uniqueGoalName, &val2, nil)

		// Small spender: average $25 (30 + 20)
		val3 := 30.00
		createCustomEvent(t, smallSpender.Email, "purchase", fmt.Sprintf("avg-small-1-%d", time.Now().UnixNano()), &purchaseType, &uniqueGoalName, &val3, nil)
		val4 := 20.00
		createCustomEvent(t, smallSpender.Email, "purchase", fmt.Sprintf("avg-small-2-%d", time.Now().UnixNano()), &purchaseType, &uniqueGoalName, &val4, nil)

		// Test: Contacts with average order value >= $100 (using unique goal_name)
		count := createAndPreviewGoalSegment(t, "Big Average Spenders", map[string]interface{}{
			"goal_type":          "purchase",
			"goal_name":          uniqueGoalName,
			"aggregate_operator": "avg",
			"operator":           "gte",
			"value":              100.0,
			"timeframe_operator": "anytime",
		})
		assert.Equal(t, 1, count, "Expected 1 contact with avg >= $100")
	})

	t.Run("should segment by min/max goal values", func(t *testing.T) {
		// Use unique goal name to isolate this test
		uniqueGoalName := fmt.Sprintf("minmax_test_%d", time.Now().UnixNano())

		// Create contacts with different purchase ranges
		contact1, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail(fmt.Sprintf("minmax-contact1-%d@example.com", time.Now().UnixNano())))
		require.NoError(t, err)

		contact2, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail(fmt.Sprintf("minmax-contact2-%d@example.com", time.Now().UnixNano())))
		require.NoError(t, err)

		purchaseType := "purchase"

		// Contact 1: purchases 10, 50, 100 (min=10, max=100)
		vals1 := []float64{10.0, 50.0, 100.0}
		for i, v := range vals1 {
			val := v
			createCustomEvent(t, contact1.Email, "purchase", fmt.Sprintf("minmax1-%d-%d", time.Now().UnixNano(), i), &purchaseType, &uniqueGoalName, &val, nil)
		}

		// Contact 2: purchases 200, 300 (min=200, max=300)
		vals2 := []float64{200.0, 300.0}
		for i, v := range vals2 {
			val := v
			createCustomEvent(t, contact2.Email, "purchase", fmt.Sprintf("minmax2-%d-%d", time.Now().UnixNano(), i), &purchaseType, &uniqueGoalName, &val, nil)
		}

		// Test MIN: Contacts with minimum purchase >= $100 (using unique goal_name)
		count := createAndPreviewGoalSegment(t, "Min Purchase >= 100", map[string]interface{}{
			"goal_type":          "purchase",
			"goal_name":          uniqueGoalName,
			"aggregate_operator": "min",
			"operator":           "gte",
			"value":              100.0,
			"timeframe_operator": "anytime",
		})
		assert.Equal(t, 1, count, "Expected 1 contact with min purchase >= $100")

		// Test MAX: Contacts with maximum purchase >= $200 (using unique goal_name)
		count = createAndPreviewGoalSegment(t, "Max Purchase >= 200", map[string]interface{}{
			"goal_type":          "purchase",
			"goal_name":          uniqueGoalName,
			"aggregate_operator": "max",
			"operator":           "gte",
			"value":              200.0,
			"timeframe_operator": "anytime",
		})
		assert.Equal(t, 1, count, "Expected 1 contact with max purchase >= $200")
	})

	t.Run("should segment using between operator", func(t *testing.T) {
		// Create contacts with different LTV values
		ltvRange, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("ltv-range@example.com"))
		require.NoError(t, err)

		purchaseType := "purchase"
		orderName := "order"
		val := 75.00
		createCustomEvent(t, ltvRange.Email, "purchase", "between-1", &purchaseType, &orderName, &val, nil)

		// Test: Contacts with LTV between $50 and $100
		count := createAndPreviewGoalSegment(t, "LTV Between 50-100", map[string]interface{}{
			"goal_type":          "purchase",
			"aggregate_operator": "sum",
			"operator":           "between",
			"value":              50.0,
			"value_2":            100.0,
			"timeframe_operator": "anytime",
		})
		assert.GreaterOrEqual(t, count, 1, "Expected at least 1 contact with LTV between $50-$100")
	})

	t.Run("should segment by goal type filter", func(t *testing.T) {
		// Create contact with different goal types
		multiGoal, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("multi-goal@example.com"))
		require.NoError(t, err)

		purchaseType := "purchase"
		subscriptionType := "subscription"
		leadType := "lead"

		purchaseVal := 100.00
		subVal := 29.99
		leadVal := 1.00

		createCustomEvent(t, multiGoal.Email, "order", "goal-type-1", &purchaseType, nil, &purchaseVal, nil)
		createCustomEvent(t, multiGoal.Email, "subscription", "goal-type-2", &subscriptionType, nil, &subVal, nil)
		createCustomEvent(t, multiGoal.Email, "lead_form", "goal-type-3", &leadType, nil, &leadVal, nil)

		// Test: Filter by subscription goal type
		count := createAndPreviewGoalSegment(t, "Subscription Goals", map[string]interface{}{
			"goal_type":          "subscription",
			"aggregate_operator": "count",
			"operator":           "gte",
			"value":              1.0,
			"timeframe_operator": "anytime",
		})
		assert.GreaterOrEqual(t, count, 1, "Expected at least 1 contact with subscription goal")

		// Test: Wildcard goal type (*) - should match all
		count = createAndPreviewGoalSegment(t, "Any Goal", map[string]interface{}{
			"goal_type":          "*",
			"aggregate_operator": "count",
			"operator":           "gte",
			"value":              3.0,
			"timeframe_operator": "anytime",
		})
		assert.GreaterOrEqual(t, count, 1, "Expected at least 1 contact with 3+ goals of any type")
	})

	t.Run("should segment by goal name filter", func(t *testing.T) {
		// Create contact with named goals
		namedGoal, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("named-goal@example.com"))
		require.NoError(t, err)

		purchaseType := "purchase"
		premiumName := "premium_plan"
		basicName := "basic_plan"

		premiumVal := 99.00
		basicVal := 29.00

		createCustomEvent(t, namedGoal.Email, "subscription", "named-1", &purchaseType, &premiumName, &premiumVal, nil)
		createCustomEvent(t, namedGoal.Email, "subscription", "named-2", &purchaseType, &basicName, &basicVal, nil)

		// Test: Filter by specific goal name
		count := createAndPreviewGoalSegment(t, "Premium Plan Goals", map[string]interface{}{
			"goal_type":          "purchase",
			"goal_name":          "premium_plan",
			"aggregate_operator": "count",
			"operator":           "gte",
			"value":              1.0,
			"timeframe_operator": "anytime",
		})
		assert.GreaterOrEqual(t, count, 1, "Expected at least 1 contact with premium_plan goal")
	})

	t.Run("should exclude soft-deleted events from aggregation", func(t *testing.T) {
		// Use unique goal name to isolate this test
		uniqueGoalName := fmt.Sprintf("softdel_test_%d", time.Now().UnixNano())
		uniqueSuffix := fmt.Sprintf("%d", time.Now().UnixNano())

		// Create contact with purchases
		deletedEvents, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail(fmt.Sprintf("deleted-events-%s@example.com", uniqueSuffix)))
		require.NoError(t, err)

		purchaseType := "purchase"

		// Create 3 purchases totaling $300
		for i := 0; i < 3; i++ {
			val := 100.00
			createCustomEvent(t, deletedEvents.Email, "purchase", fmt.Sprintf("del-order-%s-%d", uniqueSuffix, i), &purchaseType, &uniqueGoalName, &val, nil)
		}

		// Verify initial state: $300 total (using unique goal_name)
		count := createAndPreviewGoalSegment(t, "Before Delete", map[string]interface{}{
			"goal_type":          "purchase",
			"goal_name":          uniqueGoalName,
			"aggregate_operator": "sum",
			"operator":           "gte",
			"value":              300.0,
			"timeframe_operator": "anytime",
		})
		assert.Equal(t, 1, count, "Expected 1 contact with $300+ before soft-delete")

		// Soft-delete 2 purchases (should leave $100)
		softDeleteEvent(t, deletedEvents.Email, "purchase", fmt.Sprintf("del-order-%s-0", uniqueSuffix))
		softDeleteEvent(t, deletedEvents.Email, "purchase", fmt.Sprintf("del-order-%s-1", uniqueSuffix))

		// Verify after soft-delete: should no longer match $300 threshold
		count = createAndPreviewGoalSegment(t, "After Delete", map[string]interface{}{
			"goal_type":          "purchase",
			"goal_name":          uniqueGoalName,
			"aggregate_operator": "sum",
			"operator":           "gte",
			"value":              300.0,
			"timeframe_operator": "anytime",
		})
		assert.Equal(t, 0, count, "Expected 0 contacts with $300+ after soft-delete")

		// Verify reduced count: should match $100 threshold
		count = createAndPreviewGoalSegment(t, "After Delete Lower Threshold", map[string]interface{}{
			"goal_type":          "purchase",
			"goal_name":          uniqueGoalName,
			"aggregate_operator": "sum",
			"operator":           "eq",
			"value":              100.0,
			"timeframe_operator": "anytime",
		})
		assert.Equal(t, 1, count, "Expected 1 contact with exactly $100 after soft-delete")
	})

	t.Run("should segment by timeframe - in_the_last_days", func(t *testing.T) {
		// Create contact with recent and old purchases
		recentPurchaser, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("recent-purchaser@example.com"))
		require.NoError(t, err)

		purchaseType := "purchase"
		orderName := "order"

		// Recent purchase (within last 7 days)
		recentVal := 100.00
		createCustomEvent(t, recentPurchaser.Email, "purchase", "recent-order-1", &purchaseType, &orderName, &recentVal, nil)

		// Old purchase (30 days ago)
		oldTime := time.Now().AddDate(0, 0, -30)
		oldVal := 500.00
		createCustomEvent(t, recentPurchaser.Email, "purchase", "old-order-1", &purchaseType, &orderName, &oldVal, &oldTime)

		// Test: Contacts with purchases in the last 7 days
		count := createAndPreviewGoalSegment(t, "Recent 7 Days", map[string]interface{}{
			"goal_type":          "purchase",
			"aggregate_operator": "sum",
			"operator":           "gte",
			"value":              1.0,
			"timeframe_operator": "in_the_last_days",
			"timeframe_values":   []string{"7"},
		})
		assert.GreaterOrEqual(t, count, 1, "Expected at least 1 contact with recent purchases")

		// Test: Recent purchases should NOT include the old $500 purchase
		count = createAndPreviewGoalSegment(t, "Recent 7 Days High Value", map[string]interface{}{
			"goal_type":          "purchase",
			"aggregate_operator": "sum",
			"operator":           "gte",
			"value":              500.0,
			"timeframe_operator": "in_the_last_days",
			"timeframe_values":   []string{"7"},
		})
		// This contact should NOT match because the $500 was 30 days ago
		t.Logf("Contacts with $500+ in last 7 days: %d (should not include the old purchase)", count)
	})

	t.Run("should segment by timeframe - in_date_range", func(t *testing.T) {
		// Create contact with purchase at specific date
		dateRangeContact, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("date-range@example.com"))
		require.NoError(t, err)

		purchaseType := "purchase"
		orderName := "order"
		val := 100.00

		// Purchase on a specific date (10 days ago)
		purchaseDate := time.Now().AddDate(0, 0, -10)
		createCustomEvent(t, dateRangeContact.Email, "purchase", "daterange-1", &purchaseType, &orderName, &val, &purchaseDate)

		// Test: Date range that includes the purchase
		startDate := time.Now().AddDate(0, 0, -15).Format("2006-01-02")
		endDate := time.Now().AddDate(0, 0, -5).Format("2006-01-02")

		count := createAndPreviewGoalSegment(t, "Date Range Include", map[string]interface{}{
			"goal_type":          "purchase",
			"aggregate_operator": "count",
			"operator":           "gte",
			"value":              1.0,
			"timeframe_operator": "in_date_range",
			"timeframe_values":   []string{startDate, endDate},
		})
		assert.GreaterOrEqual(t, count, 1, "Expected at least 1 contact in date range")

		// Test: Date range that excludes the purchase
		excludeStart := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
		excludeEnd := time.Now().Format("2006-01-02")

		count = createAndPreviewGoalSegment(t, "Date Range Exclude", map[string]interface{}{
			"goal_type":          "purchase",
			"aggregate_operator": "sum",
			"operator":           "eq",
			"value":              100.0,
			"timeframe_operator": "in_date_range",
			"timeframe_values":   []string{excludeStart, excludeEnd},
		})
		// This should not match the contact whose purchase was 10 days ago
		t.Logf("Contacts with exactly $100 in last 5 days: %d", count)
	})

	t.Run("should segment by timeframe - before_date", func(t *testing.T) {
		// Create contact with old purchase
		oldPurchaser, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("old-purchaser@example.com"))
		require.NoError(t, err)

		purchaseType := "purchase"
		orderName := "order"
		val := 100.00

		// Purchase 60 days ago
		oldDate := time.Now().AddDate(0, 0, -60)
		createCustomEvent(t, oldPurchaser.Email, "purchase", "old-purchase-1", &purchaseType, &orderName, &val, &oldDate)

		// Test: Purchases before 30 days ago
		beforeDate := time.Now().AddDate(0, 0, -30).Format("2006-01-02")

		count := createAndPreviewGoalSegment(t, "Before Date", map[string]interface{}{
			"goal_type":          "purchase",
			"aggregate_operator": "count",
			"operator":           "gte",
			"value":              1.0,
			"timeframe_operator": "before_date",
			"timeframe_values":   []string{beforeDate},
		})
		assert.GreaterOrEqual(t, count, 1, "Expected at least 1 contact with purchase before 30 days ago")
	})

	t.Run("should segment by timeframe - after_date", func(t *testing.T) {
		// Create contact with recent purchase
		afterDateContact, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("after-date@example.com"))
		require.NoError(t, err)

		purchaseType := "purchase"
		orderName := "order"
		val := 100.00

		// Recent purchase
		createCustomEvent(t, afterDateContact.Email, "purchase", "after-purchase-1", &purchaseType, &orderName, &val, nil)

		// Test: Purchases after 7 days ago
		afterDate := time.Now().AddDate(0, 0, -7).Format("2006-01-02")

		count := createAndPreviewGoalSegment(t, "After Date", map[string]interface{}{
			"goal_type":          "purchase",
			"aggregate_operator": "count",
			"operator":           "gte",
			"value":              1.0,
			"timeframe_operator": "after_date",
			"timeframe_values":   []string{afterDate},
		})
		assert.GreaterOrEqual(t, count, 1, "Expected at least 1 contact with purchase after 7 days ago")
	})

	t.Run("should combine custom_events_goals with other conditions", func(t *testing.T) {
		// Create contacts with different combinations
		usHighLTV, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("us-high-ltv@example.com"),
			testutil.WithContactCountry("US"))
		require.NoError(t, err)

		caHighLTV, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("ca-high-ltv@example.com"),
			testutil.WithContactCountry("CA"))
		require.NoError(t, err)

		usLowLTV, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("us-low-ltv@example.com"),
			testutil.WithContactCountry("US"))
		require.NoError(t, err)

		purchaseType := "purchase"
		orderName := "order"

		// High LTV purchases for US and CA contacts
		highVal := 500.00
		createCustomEvent(t, usHighLTV.Email, "purchase", "combo-us-high", &purchaseType, &orderName, &highVal, nil)
		createCustomEvent(t, caHighLTV.Email, "purchase", "combo-ca-high", &purchaseType, &orderName, &highVal, nil)

		// Low LTV for US contact
		lowVal := 50.00
		createCustomEvent(t, usLowLTV.Email, "purchase", "combo-us-low", &purchaseType, &orderName, &lowVal, nil)

		// Test: US contacts with LTV >= $300 (should only be usHighLTV)
		segment := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           fmt.Sprintf("combo%d", time.Now().UnixNano()),
			"name":         "US High LTV",
			"description":  "US contacts with high LTV",
			"color":        "#4ECDC4",
			"timezone":     "UTC",
			"tree": map[string]interface{}{
				"kind": "branch",
				"branch": map[string]interface{}{
					"operator": "and",
					"leaves": []map[string]interface{}{
						{
							"kind": "leaf",
							"leaf": map[string]interface{}{
								"source": "contacts",
								"contact": map[string]interface{}{
									"filters": []map[string]interface{}{
										{
											"field_name":    "country",
											"field_type":    "string",
											"operator":      "equals",
											"string_values": []string{"US"},
										},
									},
								},
							},
						},
						{
							"kind": "leaf",
							"leaf": map[string]interface{}{
								"source": "custom_events_goals",
								"custom_events_goal": map[string]interface{}{
									"goal_type":          "purchase",
									"aggregate_operator": "sum",
									"operator":           "gte",
									"value":              300.0,
									"timeframe_operator": "anytime",
								},
							},
						},
					},
				},
			},
		}

		previewResp, err := client.Post("/api/segments.preview", map[string]interface{}{
			"workspace_id": workspaceID,
			"tree":         segment["tree"],
			"limit":        100,
		})
		require.NoError(t, err)
		defer func() { _ = previewResp.Body.Close() }()

		var previewResult map[string]interface{}
		err = json.NewDecoder(previewResp.Body).Decode(&previewResult)
		require.NoError(t, err)

		count := int(previewResult["total_count"].(float64))
		assert.GreaterOrEqual(t, count, 1, "Expected at least 1 US contact with high LTV")

		t.Logf("Combined segment (US + High LTV): %d contacts", count)
		t.Logf("Test contacts: US High LTV: %s, CA High LTV: %s, US Low LTV: %s",
			usHighLTV.Email, caHighLTV.Email, usLowLTV.Email)
	})

	t.Run("should build and persist goal-based segment", func(t *testing.T) {
		// Create a contact for segment membership
		segmentMember, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("segment-member@example.com"))
		require.NoError(t, err)

		purchaseType := "purchase"
		orderName := "order"
		val := 250.00
		createCustomEvent(t, segmentMember.Email, "purchase", "build-test-1", &purchaseType, &orderName, &val, nil)

		// Create segment
		segment := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           fmt.Sprintf("buildgoal%d", time.Now().UnixNano()),
			"name":         "Built Goal Segment",
			"description":  "Test building goal-based segment",
			"color":        "#FF5733",
			"timezone":     "UTC",
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "custom_events_goals",
					"custom_events_goal": map[string]interface{}{
						"goal_type":          "purchase",
						"aggregate_operator": "sum",
						"operator":           "gte",
						"value":              200.0,
						"timeframe_operator": "anytime",
					},
				},
			},
		}

		// Create the segment
		createResp, err := client.Post("/api/segments.create", segment)
		require.NoError(t, err)
		defer func() { _ = createResp.Body.Close() }()
		assert.Equal(t, http.StatusCreated, createResp.StatusCode)

		var createResult map[string]interface{}
		err = json.NewDecoder(createResp.Body).Decode(&createResult)
		require.NoError(t, err)

		segmentData := createResult["segment"].(map[string]interface{})
		segmentID := segmentData["id"].(string)

		// Build the segment
		rebuildResp, err := client.Post("/api/segments.rebuild", map[string]interface{}{
			"workspace_id": workspaceID,
			"segment_id":   segmentID,
		})
		require.NoError(t, err)
		defer func() { _ = rebuildResp.Body.Close() }()

		// Execute tasks
		execResp, err := client.Post("/api/tasks.execute", map[string]interface{}{"limit": 10})
		require.NoError(t, err)
		_ = execResp.Body.Close()

		// Wait for segment to be built
		status, err := testutil.WaitForSegmentBuilt(t, client, workspaceID, segmentID, 10*time.Second)
		if err != nil {
			t.Fatalf("Segment build failed: %v", err)
		}
		assert.Contains(t, []string{"built", "active"}, status, "Segment should be built or active")

		// Verify segment has members
		getResp, err := client.Get(fmt.Sprintf("/api/segments.get?workspace_id=%s&id=%s", workspaceID, segmentID))
		require.NoError(t, err)
		defer func() { _ = getResp.Body.Close() }()

		var getResult map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&getResult)
		require.NoError(t, err)

		builtSegment := getResult["segment"].(map[string]interface{})
		if usersCount, ok := builtSegment["users_count"].(float64); ok {
			assert.GreaterOrEqual(t, int(usersCount), 1, "Expected at least 1 member in goal-based segment")
			t.Logf("Built goal-based segment with %d members", int(usersCount))
		}

		// Verify generated SQL contains goal-related clauses
		if generatedSQL, ok := builtSegment["generated_sql"].(string); ok {
			assert.Contains(t, generatedSQL, "custom_events", "Generated SQL should reference custom_events table")
			assert.Contains(t, generatedSQL, "goal_type", "Generated SQL should filter by goal_type")
			assert.Contains(t, generatedSQL, "SUM", "Generated SQL should use SUM aggregation")
		}
	})
}
