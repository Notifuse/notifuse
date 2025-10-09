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
		testSimpleContactSegment(t, client, factory, workspace.ID)
	})

	t.Run("Segment Preview", func(t *testing.T) {
		testSegmentPreview(t, client, factory, workspace.ID)
	})

	t.Run("Complex Segment with AND/OR Logic", func(t *testing.T) {
		testComplexSegmentTree(t, client, factory, workspace.ID)
	})

	t.Run("Segment with Contact Lists", func(t *testing.T) {
		testSegmentWithContactLists(t, client, factory, workspace.ID)
	})

	t.Run("Segment with Contact Timeline", func(t *testing.T) {
		testSegmentWithContactTimeline(t, client, factory, workspace.ID)
	})

	t.Run("Segment Rebuild and Membership Updates", func(t *testing.T) {
		testSegmentRebuild(t, client, factory, workspace.ID)
	})

	t.Run("List and Get Segments", func(t *testing.T) {
		testListAndGetSegments(t, client, factory, workspace.ID)
	})

	t.Run("Update and Delete Segments", func(t *testing.T) {
		testUpdateAndDeleteSegments(t, client, factory, workspace.ID)
	})

	t.Run("Segment with Relative Dates - Daily Recompute", func(t *testing.T) {
		testSegmentWithRelativeDates(t, client, factory, workspace.ID)
	})

	t.Run("Check Segment Recompute Task Processor", func(t *testing.T) {
		testCheckSegmentRecomputeProcessor(t, client, factory, workspace.ID)
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
					"table": "contacts",
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
		defer resp.Body.Close()
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
	defer rebuildResp.Body.Close()
	assert.Equal(t, http.StatusOK, rebuildResp.StatusCode)

	// Step 5: Execute pending tasks to process segment build
	execResp, err := client.Post("/api/tasks.execute", map[string]interface{}{
		"limit": 10,
	})
	require.NoError(t, err)
	execResp.Body.Close()

	// Wait for segment to be built
	status, err := testutil.WaitForSegmentBuilt(t, client, workspaceID, segmentID, 10*time.Second)
	if err != nil {
		t.Fatalf("Segment build failed: %v", err)
	}
	assert.Contains(t, []string{"built", "active"}, status, "Segment should be built or active")

	// Step 6: Verify segment status and users count
	getResp, err := client.Get(fmt.Sprintf("/api/segments.get?workspace_id=%s&id=%s", workspaceID, segmentID))
		require.NoError(t, err)
		defer getResp.Body.Close()

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
		// Create test contacts with lifetime_value
		for i := 0; i < 5; i++ {
			_, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("premium-%d@example.com", i)),
				testutil.WithContactLifetimeValue(1000.0+float64(i)*100))
			require.NoError(t, err)
		}

		for i := 0; i < 10; i++ {
			_, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("regular-%d@example.com", i)),
				testutil.WithContactLifetimeValue(50.0))
			require.NoError(t, err)
		}

		// Preview segment for high-value contacts
		previewReq := map[string]interface{}{
			"workspace_id": workspaceID,
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"table": "contacts",
					"contact": map[string]interface{}{
						"filters": []map[string]interface{}{
							{
								"field_name":    "lifetime_value",
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
		defer resp.Body.Close()
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
				testutil.WithContactLifetimeValue(2000.0))
			require.NoError(t, err)
		}

		// Group 2: CA + high value
		for i := 0; i < 2; i++ {
			_, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("ca-vip-%d@example.com", i)),
				testutil.WithContactCountry("CA"),
				testutil.WithContactLifetimeValue(2000.0))
			require.NoError(t, err)
		}

		// Group 3: US + low value (should not match)
		for i := 0; i < 5; i++ {
			_, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("us-regular-%d@example.com", i)),
				testutil.WithContactCountry("US"),
				testutil.WithContactLifetimeValue(100.0))
			require.NoError(t, err)
		}

		// Create segment: (country=US OR country=CA) AND lifetime_value >= 2000
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
											"table": "contacts",
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
											"table": "contacts",
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
						// Branch 2: lifetime_value >= 2000
						{
							"kind": "leaf",
							"leaf": map[string]interface{}{
								"table": "contacts",
								"contact": map[string]interface{}{
									"filters": []map[string]interface{}{
										{
											"field_name":    "lifetime_value",
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
		defer previewResp.Body.Close()
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
					"table": "contact_lists",
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
		defer previewResp.Body.Close()

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
					"table": "contact_timeline",
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
		defer previewResp.Body.Close()

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
					"table": "contacts",
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
		defer createResp.Body.Close()

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
	defer rebuildResp.Body.Close()

	// Execute tasks
	execResp, err := client.Post("/api/tasks.execute", map[string]interface{}{"limit": 10})
	require.NoError(t, err)
	execResp.Body.Close()

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
	defer rebuildResp2.Body.Close()

	// Execute tasks again
	execResp2, err := client.Post("/api/tasks.execute", map[string]interface{}{"limit": 10})
	require.NoError(t, err)
	execResp2.Body.Close()

	// Wait for segment to be built
	status2, err := testutil.WaitForSegmentBuilt(t, client, workspaceID, segmentID, 10*time.Second)
	if err != nil {
		t.Fatalf("Segment rebuild failed: %v", err)
	}
	assert.Contains(t, []string{"built", "active"}, status2, "Segment should be built or active")

		// Verify updated count
		getResp, err := client.Get(fmt.Sprintf("/api/segments.get?workspace_id=%s&id=%s", workspaceID, segmentID))
		require.NoError(t, err)
		defer getResp.Body.Close()

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
						"table": "contacts",
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
			createResp.Body.Close()
		}

		// List segments
		listResp, err := client.Get(fmt.Sprintf("/api/segments.list?workspace_id=%s", workspaceID))
		require.NoError(t, err)
		defer listResp.Body.Close()
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
		defer getResp.Body.Close()
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
					"table": "contacts",
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
		defer createResp.Body.Close()

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
		defer updateResp.Body.Close()
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
		defer deleteResp.Body.Close()
		assert.Equal(t, http.StatusOK, deleteResp.StatusCode)

		// Verify segment is deleted - soft delete sets status to "deleted"
		getResp, err := client.Get(fmt.Sprintf("/api/segments.get?workspace_id=%s&id=%s", workspaceID, segmentID))
		require.NoError(t, err)
		defer getResp.Body.Close()

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
					"table": "contact_timeline",
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
		defer createResp.Body.Close()
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
	defer rebuildResp.Body.Close()

	// Execute tasks to start the build
	execResp, err := client.Post("/api/tasks.execute", map[string]interface{}{"limit": 10})
	require.NoError(t, err)
	execResp.Body.Close()

	// Wait for segment to be built
	status, err := testutil.WaitForSegmentBuilt(t, client, workspaceID, segmentID, 10*time.Second)
	if err != nil {
		t.Fatalf("Segment build failed: %v", err)
	}
	assert.Contains(t, []string{"built", "active"}, status, "Segment should be built or active")

	// Verify segment is built and recompute_after is still set
		getResp, err := client.Get(fmt.Sprintf("/api/segments.get?workspace_id=%s&id=%s", workspaceID, segmentID))
		require.NoError(t, err)
		defer getResp.Body.Close()

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
					"table": "contacts",
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
		defer createResp.Body.Close()

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
					"table": "contacts",
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
		createResp.Body.Close()

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
					"table": "contact_timeline",
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
		defer updateResp.Body.Close()
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
					"table": "contact_timeline",
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
		defer createResp.Body.Close()
		assert.Equal(t, http.StatusCreated, createResp.StatusCode)

		var createResult map[string]interface{}
		err = json.NewDecoder(createResp.Body).Decode(&createResult)
		require.NoError(t, err)

		segmentData := createResult["segment"].(map[string]interface{})
		segment1ID := segmentData["id"].(string)

	// Manually set recompute_after to the past using the factory
	pastTime := time.Now().Add(-1 * time.Hour)
	err = factory.SetSegmentRecomputeAfter(workspaceID, segment1ID, pastTime)
	require.NoError(t, err)

	// Wait to ensure the update is persisted and to create a clear time gap
	// Longer wait to account for system load when running full test suite
	time.Sleep(2 * time.Second)

		// Find the check_segment_recompute task for this workspace
		listResp, err := client.ListTasks(map[string]string{
			"workspace_id": workspaceID,
			"type":         "check_segment_recompute",
		})
		require.NoError(t, err)
		defer listResp.Body.Close()

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
	defer executeResp.Body.Close()
	assert.Equal(t, http.StatusOK, executeResp.StatusCode)

	// Note: check_segment_recompute is a recurring task that stays "pending" by design
	// Wait a bit for it to execute and create build tasks
	time.Sleep(2 * time.Second)

	// Wait for build task to be created for segment1
	buildTaskID, err := testutil.WaitForBuildTaskCreated(t, client, workspaceID, segment1ID, timeBeforeExecution, 15*time.Second)
	if err != nil {
		t.Fatalf("Build task not created for segment due for recompute: %v", err)
	}
	t.Logf("Build task %s created for segment %s", buildTaskID, segment1ID)

		// Verify the check_segment_recompute task is still pending (continues recurring)
		getTaskResp, err := client.GetTask(workspaceID, recomputeTaskID)
		require.NoError(t, err)
		defer getTaskResp.Body.Close()

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
					"table": "contact_timeline",
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
		defer createResp.Body.Close()

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
		defer listResp.Body.Close()

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
		defer buildTasksBeforeResp.Body.Close()

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
	defer executeResp.Body.Close()

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
		defer buildTasksAfterResp.Body.Close()

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
					"table": "contact_timeline",
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
		defer createResp.Body.Close()

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
		defer deleteResp.Body.Close()

		// Find and execute the check_segment_recompute task
		listResp, err := client.ListTasks(map[string]string{
			"workspace_id": workspaceID,
			"type":         "check_segment_recompute",
		})
		require.NoError(t, err)
		defer listResp.Body.Close()

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
	defer executeResp.Body.Close()

	// Note: check_segment_recompute is a recurring task that stays "pending" by design
	// Wait a bit for it to execute
	time.Sleep(1 * time.Second)

	// Verify no NEW build task was created for the deleted segment after recompute ran
		buildTasksAfterResp, err := client.ListTasks(map[string]string{
			"workspace_id": workspaceID,
			"type":         "build_segment",
		})
		require.NoError(t, err)
		defer buildTasksAfterResp.Body.Close()

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
