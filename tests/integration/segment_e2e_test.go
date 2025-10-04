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

		// Step 5: Wait for segment to be built
		time.Sleep(2 * time.Second)

		// Execute pending tasks to process segment build
		execResp, err := client.Post("/api/tasks.execute", map[string]interface{}{
			"limit": 10,
		})
		require.NoError(t, err)
		defer execResp.Body.Close()

		// Step 6: Verify segment status and users count
		time.Sleep(1 * time.Second)
		getResp, err := client.Get(fmt.Sprintf("/api/segments.get?workspace_id=%s&id=%s", workspaceID, segmentID))
		require.NoError(t, err)
		defer getResp.Body.Close()

		var getResult map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&getResult)
		require.NoError(t, err)

		updatedSegment := getResult["segment"].(map[string]interface{})

		// Segment should be active after building
		status := updatedSegment["status"].(string)
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

		// Should find at least 5 premium contacts
		assert.True(t, len(emails) >= 5, "Expected at least 5 emails in preview")
		assert.True(t, totalCount >= 5, "Expected total count of at least 5")

		// Verify emails match the pattern
		for _, email := range emails {
			emailStr := email.(string)
			assert.Contains(t, emailStr, "premium-", "Preview should only return premium contacts")
		}
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
		assert.Equal(t, 5, len(emails), "Expected 5 emails in preview")

		// Verify only VIP emails are returned
		for _, email := range emails {
			emailStr := email.(string)
			assert.Contains(t, emailStr, "vip", "Should only return VIP contacts")
		}
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
		assert.Equal(t, 5, len(emails), "Expected 5 emails in preview")

		// Verify only newsletter contacts are returned
		for _, email := range emails {
			emailStr := email.(string)
			assert.Contains(t, emailStr, "newsletter-", "Should only return newsletter contacts")
		}
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
		assert.Equal(t, 2, len(emails), "Expected 2 emails in preview")

		// Verify correct contacts are returned
		emailMap := make(map[string]bool)
		for _, email := range emails {
			emailMap[email.(string)] = true
		}
		assert.True(t, emailMap["active-user-1@example.com"], "Should include active-user-1")
		assert.True(t, emailMap["active-user-2@example.com"], "Should include active-user-2")
		assert.False(t, emailMap["inactive-user@example.com"], "Should not include inactive user")
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
		time.Sleep(1 * time.Second)
		execResp, err := client.Post("/api/tasks.execute", map[string]interface{}{"limit": 10})
		require.NoError(t, err)
		defer execResp.Body.Close()

		// Add more French contacts
		time.Sleep(1 * time.Second)
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
		time.Sleep(1 * time.Second)
		execResp2, err := client.Post("/api/tasks.execute", map[string]interface{}{"limit": 10})
		require.NoError(t, err)
		defer execResp2.Body.Close()

		// Verify updated count
		time.Sleep(1 * time.Second)
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
