package integration

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Suite 1: Workspace CRUD Operations
// Consolidates: Create, Get, List, Update, Delete flows
// ============================================================================

func TestWorkspaceCRUDSuite(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient
	tokenCache := testutil.NewTokenCache(client)

	// Root user email for authentication
	rootEmail := "test@example.com"
	rootToken := tokenCache.GetOrCreate(t, rootEmail)
	client.SetToken(rootToken)

	// =========================================================================
	// Create Flow Tests
	// =========================================================================
	t.Run("Create", func(t *testing.T) {
		t.Run("successful workspace creation", func(t *testing.T) {
			workspaceID := "testws" + uuid.New().String()[:8]
			createReq := domain.CreateWorkspaceRequest{
				ID:   workspaceID,
				Name: "Test Workspace",
				Settings: domain.WorkspaceSettings{
					Timezone:             "UTC",
					WebsiteURL:           "https://example.com",
					LogoURL:              "https://example.com/logo.png",
					EmailTrackingEnabled: true,
					DefaultLanguage:      "en",
					Languages:            []string{"en"},
				},
			}

			resp, err := client.Post("/api/workspaces.create", createReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusCreated, resp.StatusCode)

			var workspace domain.Workspace
			err = json.NewDecoder(resp.Body).Decode(&workspace)
			require.NoError(t, err)

			assert.Equal(t, workspaceID, workspace.ID)
			assert.Equal(t, "Test Workspace", workspace.Name)
			assert.Equal(t, "UTC", workspace.Settings.Timezone)
			assert.Equal(t, "https://example.com", workspace.Settings.WebsiteURL)
			assert.True(t, workspace.Settings.EmailTrackingEnabled)
			assert.False(t, workspace.CreatedAt.IsZero())
			assert.False(t, workspace.UpdatedAt.IsZero())

			// Verify workspace was created in database
			db := suite.DBManager.GetDB()
			var count int
			err = db.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id = $1", workspaceID).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 1, count)

			// Verify user was added as owner to the workspace
			err = db.QueryRow("SELECT COUNT(*) FROM user_workspaces WHERE workspace_id = $1 AND role = 'owner'", workspaceID).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 1, count)

			// Verify workspace database was created
			workspaceDB, err := suite.DBManager.GetWorkspaceDB(workspaceID)
			require.NoError(t, err)
			assert.NotNil(t, workspaceDB)

			// Test workspace database connectivity
			err = workspaceDB.Ping()
			require.NoError(t, err)
		})

		t.Run("duplicate workspace ID", func(t *testing.T) {
			workspaceID := "duplicate" + uuid.New().String()[:8]

			// Create first workspace
			createReq := domain.CreateWorkspaceRequest{
				ID:   workspaceID,
				Name: "First Workspace",
				Settings: domain.WorkspaceSettings{
					Timezone:        "UTC",
					DefaultLanguage: "en",
					Languages:       []string{"en"},
				},
			}

			resp1, err := client.Post("/api/workspaces.create", createReq)
			require.NoError(t, err)
			resp1.Body.Close()
			assert.Equal(t, http.StatusCreated, resp1.StatusCode)

			// Try to create second workspace with same ID
			createReq.Name = "Second Workspace"
			resp2, err := client.Post("/api/workspaces.create", createReq)
			require.NoError(t, err)
			defer resp2.Body.Close()

			assert.Equal(t, http.StatusConflict, resp2.StatusCode)

			var errorResp map[string]string
			err = json.NewDecoder(resp2.Body).Decode(&errorResp)
			require.NoError(t, err)
			assert.Contains(t, errorResp["error"], "already exists")
		})

		t.Run("invalid workspace data", func(t *testing.T) {
			// Missing required fields
			createReq := domain.CreateWorkspaceRequest{
				ID:   "", // Empty ID
				Name: "Test Workspace",
				Settings: domain.WorkspaceSettings{
					Timezone:        "UTC",
					DefaultLanguage: "en",
					Languages:       []string{"en"},
				},
			}

			resp, err := client.Post("/api/workspaces.create", createReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("workspace limit reached", func(t *testing.T) {
			// Depends on "successful workspace creation" having run first (Go runs
			// subtests sequentially within a parent t.Run). The seed data also
			// creates a test workspace, so at least 1 workspace exists.
			suite.Config.MaxWorkspaces = 1
			defer func() { suite.Config.MaxWorkspaces = 0 }()

			createReq := domain.CreateWorkspaceRequest{
				ID:   "limited" + uuid.New().String()[:8],
				Name: "Limited Workspace",
				Settings: domain.WorkspaceSettings{
					Timezone:        "UTC",
					DefaultLanguage: "en",
					Languages:       []string{"en"},
				},
			}

			resp, err := client.Post("/api/workspaces.create", createReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Should be forbidden because at least 1 workspace already exists (from earlier test)
			assert.Equal(t, http.StatusForbidden, resp.StatusCode)

			var errorResp map[string]string
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			require.NoError(t, err)
			assert.Contains(t, errorResp["error"], "workspace limit reached")
		})

		t.Run("unauthorized workspace creation", func(t *testing.T) {
			// Remove token
			client.SetToken("")

			createReq := domain.CreateWorkspaceRequest{
				ID:   "unauthorized" + uuid.New().String()[:8],
				Name: "Unauthorized Workspace",
				Settings: domain.WorkspaceSettings{
					Timezone:        "UTC",
					DefaultLanguage: "en",
					Languages:       []string{"en"},
				},
			}

			resp, err := client.Post("/api/workspaces.create", createReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

			// Restore token for other tests
			client.SetToken(rootToken)
		})
	})

	// =========================================================================
	// Get Flow Tests
	// =========================================================================
	t.Run("Get", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, rootToken, "Get Test Workspace")

		t.Run("successful workspace retrieval", func(t *testing.T) {
			resp, err := client.Get("/api/workspaces.get", map[string]string{
				"id": workspaceID,
			})
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)

			// Response should have workspace field
			assert.Contains(t, response, "workspace")
			workspaceData := response["workspace"].(map[string]interface{})
			assert.Equal(t, workspaceID, workspaceData["id"])
			assert.Equal(t, "Get Test Workspace", workspaceData["name"])
		})

		t.Run("workspace not found", func(t *testing.T) {
			resp, err := client.Get("/api/workspaces.get", map[string]string{
				"id": "nonexistent" + uuid.New().String()[:8],
			})
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})

		t.Run("missing workspace ID", func(t *testing.T) {
			resp, err := client.Get("/api/workspaces.get")
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})

	// =========================================================================
	// List Flow Tests
	// =========================================================================
	t.Run("List", func(t *testing.T) {
		t.Run("successful workspace listing", func(t *testing.T) {
			// Create a few workspaces
			workspaceID1 := createTestWorkspaceWithToken(t, client, rootToken, "List Test Workspace 1")
			workspaceID2 := createTestWorkspaceWithToken(t, client, rootToken, "List Test Workspace 2")

			resp, err := client.Get("/api/workspaces.list")
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var workspaces []domain.Workspace
			err = json.NewDecoder(resp.Body).Decode(&workspaces)
			require.NoError(t, err)

			// Should contain at least our created workspaces
			workspaceIDs := make(map[string]bool)
			for _, ws := range workspaces {
				workspaceIDs[ws.ID] = true
			}

			assert.True(t, workspaceIDs[workspaceID1], "Should contain first workspace")
			assert.True(t, workspaceIDs[workspaceID2], "Should contain second workspace")
		})

		t.Run("unauthorized workspace listing", func(t *testing.T) {
			client.SetToken("")

			resp, err := client.Get("/api/workspaces.list")
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

			// Restore token
			client.SetToken(rootToken)
		})
	})

	// =========================================================================
	// Update Flow Tests
	// =========================================================================
	t.Run("Update", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, rootToken, "Update Test Workspace")

		t.Run("successful workspace update", func(t *testing.T) {
			updateReq := domain.UpdateWorkspaceRequest{
				ID:   workspaceID,
				Name: "Updated Workspace Name",
				Settings: domain.WorkspaceSettings{
					Timezone:             "Europe/London",
					WebsiteURL:           "https://updated.example.com",
					LogoURL:              "https://updated.example.com/logo.png",
					EmailTrackingEnabled: false,
					DefaultLanguage:      "en",
					Languages:            []string{"en"},
				},
			}

			resp, err := client.Post("/api/workspaces.update", updateReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var workspace domain.Workspace
			err = json.NewDecoder(resp.Body).Decode(&workspace)
			require.NoError(t, err)

			assert.Equal(t, workspaceID, workspace.ID)
			assert.Equal(t, "Updated Workspace Name", workspace.Name)
			assert.Equal(t, "Europe/London", workspace.Settings.Timezone)
			assert.Equal(t, "https://updated.example.com", workspace.Settings.WebsiteURL)
			assert.False(t, workspace.Settings.EmailTrackingEnabled)

			// Verify update in database
			db := suite.DBManager.GetDB()
			var name string
			err = db.QueryRow("SELECT name FROM workspaces WHERE id = $1", workspaceID).Scan(&name)
			require.NoError(t, err)
			assert.Equal(t, "Updated Workspace Name", name)
		})

		t.Run("update nonexistent workspace", func(t *testing.T) {
			updateReq := domain.UpdateWorkspaceRequest{
				ID:   "nonexistent" + uuid.New().String()[:8],
				Name: "Updated Name",
				Settings: domain.WorkspaceSettings{
					Timezone:        "UTC",
					DefaultLanguage: "en",
					Languages:       []string{"en"},
				},
			}

			resp, err := client.Post("/api/workspaces.update", updateReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	})

	// =========================================================================
	// Delete Flow Tests
	// =========================================================================
	t.Run("Delete", func(t *testing.T) {
		t.Run("successful workspace deletion", func(t *testing.T) {
			workspaceID := createTestWorkspaceWithToken(t, client, rootToken, "Delete Test Workspace")

			// Verify workspace exists
			db := suite.DBManager.GetDB()
			var count int
			err := db.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id = $1", workspaceID).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 1, count)

			// The workspace database is created up front, so its later absence is a
			// meaningful assertion rather than a vacuous one.
			workspaceDBName := fmt.Sprintf("notifuse_test_ws_%s", strings.ReplaceAll(workspaceID, "-", "_"))
			err = db.QueryRow("SELECT COUNT(*) FROM pg_database WHERE datname = $1", workspaceDBName).Scan(&count)
			require.NoError(t, err)
			require.Equal(t, 1, count, "workspace database %s should exist before deletion", workspaceDBName)

			// Delete workspace
			deleteReq := domain.DeleteWorkspaceRequest{
				ID: workspaceID,
			}

			resp, err := client.Post("/api/workspaces.delete", deleteReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var response map[string]string
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)
			assert.Equal(t, "success", response["status"])

			// Verify workspace was deleted from database
			err = db.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id = $1", workspaceID).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 0, count)

			// Verify user_workspaces entries were cleaned up
			err = db.QueryRow("SELECT COUNT(*) FROM user_workspaces WHERE workspace_id = $1", workspaceID).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 0, count)

			// The workspace database itself must be gone, not merely unreferenced or
			// left behind marked invalid by an aborted drop.
			err = db.QueryRow("SELECT COUNT(*) FROM pg_database WHERE datname = $1", workspaceDBName).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 0, count, "workspace database %s should be dropped", workspaceDBName)
		})

		t.Run("delete nonexistent workspace", func(t *testing.T) {
			deleteReq := domain.DeleteWorkspaceRequest{
				ID: "nonexistent" + uuid.New().String()[:8],
			}

			resp, err := client.Post("/api/workspaces.delete", deleteReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	})
}

// ============================================================================
// Suite 2: Workspace Membership Operations
// Consolidates: Members, Invite, Acceptance, Removal flows
// ============================================================================

func TestWorkspaceMembershipSuite(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient
	tokenCache := testutil.NewTokenCache(client)
	db := suite.DBManager.GetDB()

	// Owner credentials
	ownerEmail := "test@example.com"
	ownerToken := tokenCache.GetOrCreate(t, ownerEmail)
	client.SetToken(ownerToken)

	// =========================================================================
	// Members Flow Tests
	// =========================================================================
	t.Run("Members", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "Members Test Workspace")

		t.Run("get workspace members", func(t *testing.T) {
			resp, err := client.Get("/api/workspaces.members", map[string]string{
				"id": workspaceID,
			})
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)

			assert.Contains(t, response, "members")
			members := response["members"].([]interface{})
			assert.Len(t, members, 1) // Should have the owner

			member := members[0].(map[string]interface{})
			assert.Equal(t, ownerEmail, member["email"])
			assert.Equal(t, "owner", member["role"])
		})

		t.Run("missing workspace ID", func(t *testing.T) {
			resp, err := client.Get("/api/workspaces.members")
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})

	// =========================================================================
	// Invite Member Flow Tests
	// =========================================================================
	t.Run("InviteMember", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "Invite Test Workspace")

		t.Run("invite existing user", func(t *testing.T) {
			// Create a user to invite
			existingUserEmail := "existing-user@example.com"
			_ = tokenCache.GetOrCreate(t, existingUserEmail)

			// Switch back to owner token
			client.SetToken(ownerToken)

			inviteReq := domain.InviteMemberRequest{
				WorkspaceID: workspaceID,
				Email:       existingUserEmail,
				Permissions: domain.NewFullPermissions(),
			}

			resp, err := client.Post("/api/workspaces.inviteMember", inviteReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)

			assert.Equal(t, "success", response["status"])
			// Existing user should be added directly
			assert.Equal(t, "User added to workspace", response["message"])

			// Verify user was added to workspace
			var count int
			err = db.QueryRow(`
				SELECT COUNT(*) FROM user_workspaces uw
				JOIN users u ON uw.user_id = u.id
				WHERE uw.workspace_id = $1 AND u.email = $2
			`, workspaceID, existingUserEmail).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 1, count)
		})

		t.Run("invite new user", func(t *testing.T) {
			newUserEmail := "new-user@example.com"

			inviteReq := domain.InviteMemberRequest{
				WorkspaceID: workspaceID,
				Email:       newUserEmail,
				Permissions: domain.NewFullPermissions(),
			}

			resp, err := client.Post("/api/workspaces.inviteMember", inviteReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)

			assert.Equal(t, "success", response["status"])
			assert.Equal(t, "Invitation sent", response["message"])
			assert.Contains(t, response, "invitation")
			assert.Contains(t, response, "token")

			// Verify invitation was created
			var count int
			err = db.QueryRow("SELECT COUNT(*) FROM workspace_invitations WHERE workspace_id = $1 AND email = $2", workspaceID, newUserEmail).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 1, count)
		})

		t.Run("invalid email", func(t *testing.T) {
			inviteReq := domain.InviteMemberRequest{
				WorkspaceID: workspaceID,
				Email:       "invalid-email",
				Permissions: domain.NewFullPermissions(),
			}

			resp, err := client.Post("/api/workspaces.inviteMember", inviteReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("invitation without permissions is rejected", func(t *testing.T) {
			// An API caller that omits the field would otherwise create a member who
			// can do nothing, stored as a row no permission backfill can ever repair.
			resp, err := client.Post("/api/workspaces.inviteMember", map[string]interface{}{
				"workspace_id": workspaceID,
				"email":        "no-permissions@example.com",
			})
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

			var count int
			require.NoError(t, db.QueryRow(
				"SELECT COUNT(*) FROM workspace_invitations WHERE workspace_id = $1 AND email = $2",
				workspaceID, "no-permissions@example.com",
			).Scan(&count))
			assert.Equal(t, 0, count)
		})
	})

	// =========================================================================
	// Invitation Acceptance Flow Tests
	// =========================================================================
	t.Run("InvitationAcceptance", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "Invite Accept Test WS")

		t.Run("complete invitation acceptance flow for new user", func(t *testing.T) {
			newUserEmail := "acceptance-test-user@example.com"

			// Step 1: Create invitation
			inviteReq := domain.InviteMemberRequest{
				WorkspaceID: workspaceID,
				Email:       newUserEmail,
				Permissions: domain.NewFullPermissions(),
			}

			resp, err := client.Post("/api/workspaces.inviteMember", inviteReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var inviteResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&inviteResponse)
			require.NoError(t, err)

			assert.Equal(t, "success", inviteResponse["status"])
			assert.Equal(t, "Invitation sent", inviteResponse["message"])
			assert.Contains(t, inviteResponse, "token")

			invitationToken := inviteResponse["token"].(string)
			assert.NotEmpty(t, invitationToken)

			// Step 2: Verify the invitation token
			client.SetToken("") // Clear auth token - verification doesn't require auth
			verifyReq := map[string]string{"token": invitationToken}

			verifyResp, err := client.Post("/api/workspaces.verifyInvitationToken", verifyReq)
			require.NoError(t, err)
			defer verifyResp.Body.Close()

			assert.Equal(t, http.StatusOK, verifyResp.StatusCode)

			var verifyResponse map[string]interface{}
			err = json.NewDecoder(verifyResp.Body).Decode(&verifyResponse)
			require.NoError(t, err)

			assert.Equal(t, "success", verifyResponse["status"])
			assert.Equal(t, true, verifyResponse["valid"])
			assert.Contains(t, verifyResponse, "invitation")
			assert.Contains(t, verifyResponse, "workspace")

			// Verify workspace details
			workspaceData := verifyResponse["workspace"].(map[string]interface{})
			assert.Equal(t, workspaceID, workspaceData["id"])
			assert.Equal(t, "Invite Accept Test WS", workspaceData["name"])

			// Verify invitation details
			invitationData := verifyResponse["invitation"].(map[string]interface{})
			assert.Equal(t, newUserEmail, invitationData["email"])
			assert.Equal(t, workspaceID, invitationData["workspace_id"])

			// Step 3: Accept the invitation
			acceptReq := map[string]string{"token": invitationToken}

			acceptResp, err := client.Post("/api/workspaces.acceptInvitation", acceptReq)
			require.NoError(t, err)
			defer acceptResp.Body.Close()

			assert.Equal(t, http.StatusOK, acceptResp.StatusCode)

			var acceptResponse map[string]interface{}
			err = json.NewDecoder(acceptResp.Body).Decode(&acceptResponse)
			require.NoError(t, err)

			assert.Equal(t, "success", acceptResponse["status"])
			assert.Equal(t, "Invitation accepted successfully", acceptResponse["message"])
			assert.Equal(t, workspaceID, acceptResponse["workspace_id"])
			assert.Equal(t, newUserEmail, acceptResponse["email"])
			assert.Contains(t, acceptResponse, "token")
			assert.Contains(t, acceptResponse, "user")
			assert.Contains(t, acceptResponse, "expires_at")

			// Verify user was created and added to workspace
			userAuthToken := acceptResponse["token"].(string)
			assert.NotEmpty(t, userAuthToken)

			userData := acceptResponse["user"].(map[string]interface{})
			assert.Equal(t, newUserEmail, userData["email"])
			newUserID := userData["id"].(string)
			assert.NotEmpty(t, newUserID)

			// Verify in database - user exists
			var count int
			err = db.QueryRow("SELECT COUNT(*) FROM users WHERE email = $1", newUserEmail).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 1, count, "User should be created in database")

			// Verify in database - user is member of workspace
			err = db.QueryRow("SELECT COUNT(*) FROM user_workspaces WHERE user_id = $1 AND workspace_id = $2", newUserID, workspaceID).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 1, count, "User should be added to workspace")

			// Verify invitation was deleted
			err = db.QueryRow("SELECT COUNT(*) FROM workspace_invitations WHERE workspace_id = $1 AND email = $2", workspaceID, newUserEmail).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 0, count, "Invitation should be deleted after acceptance")

			// Step 4: Verify the new user can access the workspace
			client.SetToken(userAuthToken)

			getResp, err := client.Get("/api/workspaces.get", map[string]string{
				"id": workspaceID,
			})
			require.NoError(t, err)
			defer getResp.Body.Close()

			assert.Equal(t, http.StatusOK, getResp.StatusCode)

			var getResponse map[string]interface{}
			err = json.NewDecoder(getResp.Body).Decode(&getResponse)
			require.NoError(t, err)

			workspaceResult := getResponse["workspace"].(map[string]interface{})
			assert.Equal(t, workspaceID, workspaceResult["id"])

			// Restore owner token for subsequent tests
			client.SetToken(ownerToken)
		})

		t.Run("accept invitation with invalid token", func(t *testing.T) {
			client.SetToken("") // No auth required for accept

			acceptReq := map[string]string{"token": "invalid-token"}

			resp, err := client.Post("/api/workspaces.acceptInvitation", acceptReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

			// Restore owner token
			client.SetToken(ownerToken)
		})

		t.Run("verify invitation with invalid token", func(t *testing.T) {
			client.SetToken("") // No auth required for verify

			verifyReq := map[string]string{"token": "invalid-token"}

			resp, err := client.Post("/api/workspaces.verifyInvitationToken", verifyReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

			// Restore owner token
			client.SetToken(ownerToken)
		})
	})

	// =========================================================================
	// Remove Member Flow Tests
	// =========================================================================
	t.Run("RemoveMember", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "Remove Member Test Workspace")

		// Create member user
		memberEmail := "workspace-member@example.com"
		_ = tokenCache.GetOrCreate(t, memberEmail)

		// Switch back to owner to add member
		client.SetToken(ownerToken)

		// Add member to workspace
		inviteReq := domain.InviteMemberRequest{
			WorkspaceID: workspaceID,
			Email:       memberEmail,
			Permissions: domain.NewFullPermissions(),
		}
		inviteResp, err := client.Post("/api/workspaces.inviteMember", inviteReq)
		require.NoError(t, err)
		inviteResp.Body.Close()

		// Get member user ID
		var memberUserID string
		err = db.QueryRow("SELECT id FROM users WHERE email = $1", memberEmail).Scan(&memberUserID)
		require.NoError(t, err)

		t.Run("successful member removal", func(t *testing.T) {
			removeReq := map[string]interface{}{
				"workspace_id": workspaceID,
				"user_id":      memberUserID,
			}

			resp, err := client.Post("/api/workspaces.removeMember", removeReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var response map[string]string
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)

			assert.Equal(t, "success", response["status"])
			assert.Equal(t, "Member removed successfully", response["message"])

			// Verify member was removed from database
			var count int
			err = db.QueryRow("SELECT COUNT(*) FROM user_workspaces WHERE workspace_id = $1 AND user_id = $2", workspaceID, memberUserID).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 0, count)
		})

		t.Run("remove non-member", func(t *testing.T) {
			// Create another user who is not a member
			nonMemberEmail := "non-member@example.com"
			_ = tokenCache.GetOrCreate(t, nonMemberEmail)

			var nonMemberUserID string
			err = db.QueryRow("SELECT id FROM users WHERE email = $1", nonMemberEmail).Scan(&nonMemberUserID)
			require.NoError(t, err)

			// Switch back to owner
			client.SetToken(ownerToken)

			removeReq := map[string]interface{}{
				"workspace_id": workspaceID,
				"user_id":      nonMemberUserID,
			}

			resp, err := client.Post("/api/workspaces.removeMember", removeReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		})
	})
}

// ============================================================================
// Suite 3: Workspace Features (Integrations + API Keys)
// Consolidates: Integrations and API Key flows
// ============================================================================

func TestWorkspaceFeaturesSuite(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient
	tokenCache := testutil.NewTokenCache(client)

	// Owner credentials
	ownerEmail := "test@example.com"
	ownerToken := tokenCache.GetOrCreate(t, ownerEmail)
	client.SetToken(ownerToken)

	// =========================================================================
	// Integrations Flow Tests
	// =========================================================================
	t.Run("Integrations", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "Integration Test Workspace")

		t.Run("create email integration", func(t *testing.T) {
			createReq := domain.CreateIntegrationRequest{
				WorkspaceID: workspaceID,
				Name:        "Test Email Provider",
				Type:        domain.IntegrationTypeEmail,
				Provider: domain.EmailProvider{
					Kind: domain.EmailProviderKindMailgun,
					Mailgun: &domain.MailgunSettings{
						Domain: "test.example.com",
						APIKey: "test-api-key",
					},
					Senders: []domain.EmailSender{
						{
							ID:        "sender-1",
							Email:     "test@example.com",
							Name:      "Test Sender",
							IsDefault: true,
						},
					},
					RateLimitPerMinute: 25,
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

			// Verify integration was added to workspace
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
			assert.Equal(t, "Test Email Provider", integration["name"])
			assert.Equal(t, "email", integration["type"])
		})

		t.Run("invalid integration data", func(t *testing.T) {
			createReq := domain.CreateIntegrationRequest{
				WorkspaceID: workspaceID,
				Name:        "", // Empty name
				Type:        domain.IntegrationTypeEmail,
				Provider: domain.EmailProvider{
					Kind:               domain.EmailProviderKindMailgun,
					RateLimitPerMinute: 25,
				},
			}

			resp, err := client.Post("/api/workspaces.createIntegration", createReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("create supabase integration with templates and notifications", func(t *testing.T) {
			// Create Supabase integration
			createReq := domain.CreateIntegrationRequest{
				WorkspaceID: workspaceID,
				Name:        "Test Supabase Integration",
				Type:        domain.IntegrationTypeSupabase,
				SupabaseSettings: &domain.SupabaseIntegrationSettings{
					AuthEmailHook: domain.SupabaseAuthEmailHookSettings{
						SignatureKey: "v1,whsec_test_key_1234567890",
					},
					BeforeUserCreatedHook: domain.SupabaseUserCreatedHookSettings{
						SignatureKey:    "v1,whsec_test_key_0987654321",
						AddUserToLists:  []string{},
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

			// Verify integration was added to workspace
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

			// Find the Supabase integration
			var supabaseIntegration map[string]interface{}
			for _, integ := range integrations {
				integMap := integ.(map[string]interface{})
				if integMap["type"] == "supabase" {
					supabaseIntegration = integMap
					break
				}
			}

			require.NotNil(t, supabaseIntegration, "Supabase integration not found")
			assert.Equal(t, integrationID, supabaseIntegration["id"])
			assert.Equal(t, "Test Supabase Integration", supabaseIntegration["name"])
			assert.Equal(t, "supabase", supabaseIntegration["type"])

			// Verify templates were created
			templatesResp, err := client.Get("/api/templates.list", map[string]string{
				"workspace_id": workspaceID,
			})
			require.NoError(t, err)
			defer templatesResp.Body.Close()

			var templatesResponse map[string]interface{}
			err = json.NewDecoder(templatesResp.Body).Decode(&templatesResponse)
			require.NoError(t, err)

			templates := templatesResponse["templates"].([]interface{})

			// Count templates with this integration_id
			supabaseTemplateCount := 0
			expectedTemplateNames := []string{
				"Signup Confirmation",
				"Magic Link",
				"Password Recovery",
				"Email Change",
				"User Invitation",
				"Reauthentication",
			}
			foundTemplateNames := []string{}

			for _, tmpl := range templates {
				tmplMap := tmpl.(map[string]interface{})
				if integID, ok := tmplMap["integration_id"]; ok && integID == integrationID {
					supabaseTemplateCount++
					foundTemplateNames = append(foundTemplateNames, tmplMap["name"].(string))
				}
			}

			assert.Equal(t, 6, supabaseTemplateCount, "Expected 6 Supabase templates to be created")
			for _, expectedName := range expectedTemplateNames {
				assert.Contains(t, foundTemplateNames, expectedName, "Template %s not found", expectedName)
			}

			// Verify transactional notifications were created
			notificationsResp, err := client.Get("/api/transactional.list", map[string]string{
				"workspace_id": workspaceID,
			})
			require.NoError(t, err)
			defer notificationsResp.Body.Close()

			var notificationsResponse map[string]interface{}
			err = json.NewDecoder(notificationsResp.Body).Decode(&notificationsResponse)
			require.NoError(t, err)

			notifications := notificationsResponse["notifications"].([]interface{})

			// Count notifications with this integration_id
			supabaseNotificationCount := 0
			expectedNotificationNames := []string{
				"Signup Confirmation",
				"Magic Link",
				"Password Recovery",
				"Email Change",
				"User Invitation",
				"Reauthentication",
			}
			foundNotificationNames := []string{}

			for _, notif := range notifications {
				notifMap := notif.(map[string]interface{})
				if integID, ok := notifMap["integration_id"]; ok && integID == integrationID {
					supabaseNotificationCount++
					foundNotificationNames = append(foundNotificationNames, notifMap["name"].(string))
				}
			}

			assert.Equal(t, 6, supabaseNotificationCount, "Expected 6 Supabase transactional notifications to be created")
			for _, expectedName := range expectedNotificationNames {
				assert.Contains(t, foundNotificationNames, expectedName, "Notification %s not found", expectedName)
			}
		})
	})

	// =========================================================================
	// API Key Flow Tests
	// =========================================================================
	t.Run("APIKey", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "API Key Test Workspace")
		db := suite.DBManager.GetDB()

		// createAPIKey posts workspaces.createAPIKey as the owner and returns the
		// status alongside the decoded body. req is taken as interface{} so a case
		// can send a raw map: CreateAPIKeyRequest.Permissions is omitempty, so an
		// explicitly empty map cannot be expressed through the struct.
		createAPIKey := func(t *testing.T, req interface{}) (int, map[string]interface{}) {
			t.Helper()
			client.SetToken(ownerToken)
			resp, err := client.Post("/api/workspaces.createAPIKey", req)
			require.NoError(t, err)
			defer resp.Body.Close()

			var body map[string]interface{}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			return resp.StatusCode, body
		}

		// getAs issues a workspace-scoped GET with the given token and restores the
		// owner's token, so a subtest driving a key cannot leak it to the next one.
		getAs := func(t *testing.T, token, endpoint string) int {
			t.Helper()
			client.SetToken(token)
			defer client.SetToken(ownerToken)

			resp, err := client.Get(endpoint, map[string]string{"workspace_id": workspaceID})
			require.NoError(t, err)
			defer resp.Body.Close()
			return resp.StatusCode
		}

		// keyUserID resolves the users row behind an API key email. An API key is a
		// member row, so its user_id is what workspaces.setUserPermissions targets.
		keyUserID := func(t *testing.T, email string) string {
			t.Helper()
			var userID string
			require.NoError(t, db.QueryRow("SELECT id FROM users WHERE email = $1", email).Scan(&userID))
			return userID
		}

		// storedPermissions returns the membership row's permissions column as text,
		// so a case can tell SQL NULL apart from '{}'.
		storedPermissions := func(t *testing.T, userID string) sql.NullString {
			t.Helper()
			var raw sql.NullString
			require.NoError(t, db.QueryRow(
				`SELECT permissions::text FROM user_workspaces WHERE user_id = $1 AND workspace_id = $2`,
				userID, workspaceID,
			).Scan(&raw))
			return raw
		}

		t.Run("create API key as owner", func(t *testing.T) {
			createReq := domain.CreateAPIKeyRequest{
				WorkspaceID: workspaceID,
				EmailPrefix: "api",
			}

			resp, err := client.Post("/api/workspaces.createAPIKey", createReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)

			assert.Equal(t, "success", response["status"])
			assert.Contains(t, response, "token")
			assert.Contains(t, response, "email")

			token := response["token"].(string)
			email := response["email"].(string)
			assert.NotEmpty(t, token)
			assert.NotEmpty(t, email)
			assert.Contains(t, email, "api")
		})

		t.Run("missing email prefix", func(t *testing.T) {
			createReq := domain.CreateAPIKeyRequest{
				WorkspaceID: workspaceID,
				EmailPrefix: "", // Empty prefix
			}

			resp, err := client.Post("/api/workspaces.createAPIKey", createReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("invalid email prefix is rejected", func(t *testing.T) {
			status, _ := createAPIKey(t, domain.CreateAPIKeyRequest{
				WorkspaceID: workspaceID,
				EmailPrefix: "Bad Prefix", // uppercase and a space
			})

			assert.Equal(t, http.StatusBadRequest, status)
		})

		t.Run("duplicate email prefix conflicts", func(t *testing.T) {
			// users.email is unique across the deployment, so a prefix can be claimed once.
			status, _ := createAPIKey(t, domain.CreateAPIKeyRequest{
				WorkspaceID: workspaceID,
				EmailPrefix: "duplicate-prefix",
			})
			require.Equal(t, http.StatusOK, status)

			status, _ = createAPIKey(t, domain.CreateAPIKeyRequest{
				WorkspaceID: workspaceID,
				EmailPrefix: "duplicate-prefix",
			})

			assert.Equal(t, http.StatusConflict, status)
		})

		t.Run("key created without a permissions field keeps full access", func(t *testing.T) {
			status, body := createAPIKey(t, domain.CreateAPIKeyRequest{
				WorkspaceID: workspaceID,
				EmailPrefix: "legacy-shape",
			})
			require.Equal(t, http.StatusOK, status)

			keyToken := body["token"].(string)
			require.NotEmpty(t, keyToken)

			// The pre-39.0 request shape carries no permissions map, and must keep
			// producing the key it produced before the field existed.
			assert.Equal(t, http.StatusOK, getAs(t, keyToken, "/api/transactional.list"))
			assert.Equal(t, http.StatusOK, getAs(t, keyToken, "/api/broadcasts.list"))

			raw := storedPermissions(t, keyUserID(t, body["email"].(string)))
			require.True(t, raw.Valid)
			var stored domain.UserPermissions
			require.NoError(t, json.Unmarshal([]byte(raw.String), &stored))
			assert.Equal(t, domain.NewFullPermissions(), stored)
		})

		t.Run("key created with an empty permissions map is denied everywhere", func(t *testing.T) {
			// Sent as a raw map: the struct field is omitempty, so an explicit {}
			// would marshal away and be read back as "absent" — i.e. full access.
			status, body := createAPIKey(t, map[string]interface{}{
				"workspace_id": workspaceID,
				"email_prefix": "zero-scope",
				"permissions":  map[string]interface{}{},
			})
			require.Equal(t, http.StatusOK, status)

			keyToken := body["token"].(string)
			require.NotEmpty(t, keyToken)

			assert.Equal(t, http.StatusForbidden, getAs(t, keyToken, "/api/transactional.list"))
			assert.Equal(t, http.StatusForbidden, getAs(t, keyToken, "/api/broadcasts.list"))

			// Persisted as '{}' rather than SQL NULL: a NULL row is skipped by every
			// permission backfill guard and cannot be repaired from the console.
			raw := storedPermissions(t, keyUserID(t, body["email"].(string)))
			require.True(t, raw.Valid, "an explicitly empty map must not persist as SQL NULL")
			assert.Equal(t, "{}", raw.String)
		})

		t.Run("scoped key is allowed and denied by its permissions map", func(t *testing.T) {
			status, body := createAPIKey(t, domain.CreateAPIKeyRequest{
				WorkspaceID: workspaceID,
				EmailPrefix: "scoped",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
			})
			require.Equal(t, http.StatusOK, status)

			// The token createAPIKey just returned is the only way to drive a key:
			// /api/user.signin is the human magic-code flow and refuses api_key
			// identities, which the subtest below pins.
			keyToken := body["token"].(string)
			require.NotEmpty(t, keyToken)

			assert.Equal(t, http.StatusOK, getAs(t, keyToken, "/api/transactional.list"))
			assert.Equal(t, http.StatusForbidden, getAs(t, keyToken, "/api/broadcasts.list"))

			// Re-scoping an existing key: workspaces.setUserPermissions takes the
			// key's user_id like any other member's.
			client.SetToken(ownerToken)
			resp, err := client.Post("/api/workspaces.setUserPermissions", domain.SetUserPermissionsRequest{
				WorkspaceID: workspaceID,
				UserID:      keyUserID(t, body["email"].(string)),
				Permissions: domain.UserPermissions{
					domain.PermissionResourceBroadcasts: {Read: true, Write: true},
				},
			})
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			// Same token, opposite answers. The JWT carries no permissions claim, so
			// permissions are read from user_workspaces on every request and editing
			// a key never requires reissuing or rotating its token.
			assert.Equal(t, http.StatusOK, getAs(t, keyToken, "/api/broadcasts.list"))
			assert.Equal(t, http.StatusForbidden, getAs(t, keyToken, "/api/transactional.list"))
		})

		t.Run("API key identity cannot sign in", func(t *testing.T) {
			status, body := createAPIKey(t, domain.CreateAPIKeyRequest{
				WorkspaceID: workspaceID,
				EmailPrefix: "no-signin",
			})
			require.Equal(t, http.StatusOK, status)

			// An API key is a users row with a real, unique email. Without the type
			// check in SignIn, this mints a human session for it — and hands back the
			// magic code in the response body on a non-production instance.
			resp, err := client.Post("/api/user.signin", map[string]string{
				"email": body["email"].(string),
			})
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

			var signinBody map[string]interface{}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&signinBody))
			assert.NotContains(t, signinBody, "code")
		})
	})
}

// ============================================================================
// Helper Functions
// ============================================================================

// createTestWorkspaceWithToken creates a test workspace using a pre-obtained token.
// This avoids redundant authentication calls when the caller already has a valid token.
// ============================================================================
// Suite: Custom Field Labels (granular workspace:write permission) — issue #354
// Verifies that members with workspace:write (not just owners) can manage
// custom field labels via the dedicated /api/workspaces.setCustomFieldLabels
// endpoint, and that workspaces.update no longer clobbers labels.
// ============================================================================

func TestWorkspaceCustomFieldLabelsSuite(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient
	tokenCache := testutil.NewTokenCache(client)
	db := suite.DBManager.GetDB()

	ownerEmail := "test@example.com"
	ownerToken := tokenCache.GetOrCreate(t, ownerEmail)
	client.SetToken(ownerToken)

	// addMember creates a user and inserts a user_workspaces row with the given
	// granular permissions, returning the member's auth token.
	addMember := func(t *testing.T, email, workspaceID string, perms domain.UserPermissions) string {
		t.Helper()
		token := tokenCache.GetOrCreate(t, email)
		var userID string
		require.NoError(t, db.QueryRow("SELECT id FROM users WHERE email = $1", email).Scan(&userID))
		permsJSON, err := json.Marshal(perms)
		require.NoError(t, err)
		_, err = db.Exec(
			`INSERT INTO user_workspaces (user_id, workspace_id, role, permissions, created_at, updated_at)
			 VALUES ($1, $2, 'member', $3, NOW(), NOW())
			 ON CONFLICT (user_id, workspace_id) DO UPDATE SET permissions = EXCLUDED.permissions, updated_at = NOW()`,
			userID, workspaceID, string(permsJSON),
		)
		require.NoError(t, err)
		return token
	}

	// labelValue reads a single custom field label directly from the workspace settings JSON.
	labelValue := func(t *testing.T, workspaceID, key string) string {
		t.Helper()
		var label sql.NullString
		require.NoError(t, db.QueryRow(
			`SELECT settings->'custom_field_labels'->>$2 FROM workspaces WHERE id = $1`,
			workspaceID, key,
		).Scan(&label))
		return label.String
	}

	setLabels := func(token, workspaceID string, labels map[string]string) *http.Response {
		client.SetToken(token)
		resp, err := client.Post("/api/workspaces.setCustomFieldLabels", domain.SetCustomFieldLabelsRequest{
			WorkspaceID:       workspaceID,
			CustomFieldLabels: labels,
		})
		require.NoError(t, err)
		client.SetToken(ownerToken)
		return resp
	}

	t.Run("owner can set labels", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "CFL Owner WS")

		resp := setLabels(ownerToken, workspaceID, map[string]string{"custom_string_1": "Company Name"})
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "Company Name", labelValue(t, workspaceID, "custom_string_1"))
	})

	t.Run("member with workspace:write can set labels (issue #354)", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "CFL Writer WS")
		memberToken := addMember(t, "workspace-member@example.com", workspaceID, domain.UserPermissions{
			domain.PermissionResourceWorkspace: {Read: true, Write: true},
		})

		resp := setLabels(memberToken, workspaceID, map[string]string{"custom_string_1": "Set By Member"})
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "Set By Member", labelValue(t, workspaceID, "custom_string_1"))
	})

	t.Run("member with workspace read-only is denied", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "CFL ReadOnly WS")
		memberToken := addMember(t, "workspace-updater@example.com", workspaceID, domain.UserPermissions{
			domain.PermissionResourceWorkspace: {Read: true, Write: false},
		})

		resp := setLabels(memberToken, workspaceID, map[string]string{"custom_string_1": "Should Fail"})
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.Empty(t, labelValue(t, workspaceID, "custom_string_1"))
	})

	t.Run("member without workspace permission is denied", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "CFL NoPerm WS")
		// Has contacts access but not workspace.
		memberToken := addMember(t, "workspace-integrator@example.com", workspaceID, domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: true},
		})

		resp := setLabels(memberToken, workspaceID, map[string]string{"custom_string_1": "Should Fail"})
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.Empty(t, labelValue(t, workspaceID, "custom_string_1"))
	})

	t.Run("invalid label key is rejected", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "CFL Invalid WS")

		resp := setLabels(ownerToken, workspaceID, map[string]string{"custom_string_99": "Bad"})
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("non-member cannot set labels", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "CFL NonMember WS")
		strangerToken := tokenCache.GetOrCreate(t, "non-member@example.com") // not added to workspace

		resp := setLabels(strangerToken, workspaceID, map[string]string{"custom_string_1": "Should Fail"})
		defer resp.Body.Close()

		assert.GreaterOrEqual(t, resp.StatusCode, http.StatusBadRequest)
		assert.NotEqual(t, http.StatusOK, resp.StatusCode)
		assert.Empty(t, labelValue(t, workspaceID, "custom_string_1"))
	})

	t.Run("workspaces.update preserves custom field labels (sole-writer guard)", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "CFL SoleWriter WS")

		// Member sets a label via the dedicated endpoint.
		memberToken := addMember(t, "existing-user@example.com", workspaceID, domain.UserPermissions{
			domain.PermissionResourceWorkspace: {Read: true, Write: true},
		})
		resp := setLabels(memberToken, workspaceID, map[string]string{"custom_string_1": "Member Label"})
		resp.Body.Close()
		require.Equal(t, "Member Label", labelValue(t, workspaceID, "custom_string_1"))

		// Owner updates general settings WITHOUT custom_field_labels in the request.
		client.SetToken(ownerToken)
		updateResp, err := client.Post("/api/workspaces.update", domain.UpdateWorkspaceRequest{
			ID:   workspaceID,
			Name: "Renamed WS",
			Settings: domain.WorkspaceSettings{
				Timezone:        "Europe/London",
				DefaultLanguage: "en",
				Languages:       []string{"en"},
			},
		})
		require.NoError(t, err)
		defer updateResp.Body.Close()
		require.Equal(t, http.StatusOK, updateResp.StatusCode)

		// The label set by the member must be preserved, not wiped by the owner's update.
		assert.Equal(t, "Member Label", labelValue(t, workspaceID, "custom_string_1"))
	})
}

// ============================================================================
// Suite: Blog Settings (granular blog:write permission)
// Verifies that members with blog:write (not just owners) can manage blog
// settings (enable flag + title/SEO/pagination/feed) via the dedicated
// /api/workspaces.setBlogSettings endpoint, and that workspaces.update no longer
// clobbers blog config.
// ============================================================================

func TestWorkspaceBlogSettingsSuite(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient
	tokenCache := testutil.NewTokenCache(client)
	db := suite.DBManager.GetDB()

	ownerEmail := "test@example.com"
	ownerToken := tokenCache.GetOrCreate(t, ownerEmail)
	client.SetToken(ownerToken)

	// addMember creates a user and inserts a user_workspaces row with the given
	// granular permissions, returning the member's auth token.
	addMember := func(t *testing.T, email, workspaceID string, perms domain.UserPermissions) string {
		t.Helper()
		token := tokenCache.GetOrCreate(t, email)
		var userID string
		require.NoError(t, db.QueryRow("SELECT id FROM users WHERE email = $1", email).Scan(&userID))
		permsJSON, err := json.Marshal(perms)
		require.NoError(t, err)
		_, err = db.Exec(
			`INSERT INTO user_workspaces (user_id, workspace_id, role, permissions, created_at, updated_at)
			 VALUES ($1, $2, 'member', $3, NOW(), NOW())
			 ON CONFLICT (user_id, workspace_id) DO UPDATE SET permissions = EXCLUDED.permissions, updated_at = NOW()`,
			userID, workspaceID, string(permsJSON),
		)
		require.NoError(t, err)
		return token
	}

	// blogEnabled reads the blog_enabled flag directly from the workspace settings JSON.
	blogEnabled := func(t *testing.T, workspaceID string) bool {
		t.Helper()
		var enabled sql.NullBool
		require.NoError(t, db.QueryRow(
			`SELECT (settings->>'blog_enabled')::boolean FROM workspaces WHERE id = $1`,
			workspaceID,
		).Scan(&enabled))
		return enabled.Bool
	}

	// blogTitle reads the blog title directly from the workspace settings JSON.
	blogTitle := func(t *testing.T, workspaceID string) string {
		t.Helper()
		var title sql.NullString
		require.NoError(t, db.QueryRow(
			`SELECT settings->'blog_settings'->>'title' FROM workspaces WHERE id = $1`,
			workspaceID,
		).Scan(&title))
		return title.String
	}

	setBlogSettings := func(token, workspaceID string, enabled bool, settings *domain.BlogSettings) *http.Response {
		client.SetToken(token)
		resp, err := client.Post("/api/workspaces.setBlogSettings", domain.SetBlogSettingsRequest{
			WorkspaceID:  workspaceID,
			BlogEnabled:  enabled,
			BlogSettings: settings,
		})
		require.NoError(t, err)
		client.SetToken(ownerToken)
		return resp
	}

	t.Run("owner can set blog settings", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "Blog Owner WS")

		resp := setBlogSettings(ownerToken, workspaceID, true, &domain.BlogSettings{Title: "Owner Blog"})
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, blogEnabled(t, workspaceID))
		assert.Equal(t, "Owner Blog", blogTitle(t, workspaceID))
	})

	t.Run("member with blog:write can set blog settings (regression)", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "Blog Writer WS")
		memberToken := addMember(t, "blog-manager@example.com", workspaceID, domain.UserPermissions{
			domain.PermissionResourceBlog: {Read: true, Write: true},
		})

		resp := setBlogSettings(memberToken, workspaceID, true, &domain.BlogSettings{Title: "Set By Blog Manager"})
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, blogEnabled(t, workspaceID))
		assert.Equal(t, "Set By Blog Manager", blogTitle(t, workspaceID))
	})

	t.Run("member with blog read-only is denied", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "Blog ReadOnly WS")
		memberToken := addMember(t, "blog-reader@example.com", workspaceID, domain.UserPermissions{
			domain.PermissionResourceBlog: {Read: true, Write: false},
		})

		resp := setBlogSettings(memberToken, workspaceID, true, &domain.BlogSettings{Title: "Should Fail"})
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.False(t, blogEnabled(t, workspaceID))
		assert.Empty(t, blogTitle(t, workspaceID))
	})

	t.Run("member without blog permission is denied", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "Blog NoPerm WS")
		// Has contacts access but not blog.
		memberToken := addMember(t, "blog-contacts@example.com", workspaceID, domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: true},
		})

		resp := setBlogSettings(memberToken, workspaceID, true, &domain.BlogSettings{Title: "Should Fail"})
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.False(t, blogEnabled(t, workspaceID))
	})

	t.Run("invalid blog settings are rejected", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "Blog Invalid WS")

		resp := setBlogSettings(ownerToken, workspaceID, true, &domain.BlogSettings{HomePageSize: 999})
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("non-member cannot set blog settings", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "Blog NonMember WS")
		strangerToken := tokenCache.GetOrCreate(t, "blog-stranger@example.com") // not added to workspace

		resp := setBlogSettings(strangerToken, workspaceID, true, &domain.BlogSettings{Title: "Should Fail"})
		defer resp.Body.Close()

		assert.GreaterOrEqual(t, resp.StatusCode, http.StatusBadRequest)
		assert.NotEqual(t, http.StatusOK, resp.StatusCode)
		assert.False(t, blogEnabled(t, workspaceID))
	})

	t.Run("workspaces.update preserves blog settings (sole-writer guard)", func(t *testing.T) {
		workspaceID := createTestWorkspaceWithToken(t, client, ownerToken, "Blog SoleWriter WS")

		// Member enables the blog and sets a title via the dedicated endpoint.
		memberToken := addMember(t, "blog-soleexisting@example.com", workspaceID, domain.UserPermissions{
			domain.PermissionResourceBlog: {Read: true, Write: true},
		})
		resp := setBlogSettings(memberToken, workspaceID, true, &domain.BlogSettings{Title: "Member Blog"})
		resp.Body.Close()
		require.True(t, blogEnabled(t, workspaceID))
		require.Equal(t, "Member Blog", blogTitle(t, workspaceID))

		// Owner updates general settings WITHOUT blog fields in the request.
		client.SetToken(ownerToken)
		updateResp, err := client.Post("/api/workspaces.update", domain.UpdateWorkspaceRequest{
			ID:   workspaceID,
			Name: "Renamed Blog WS",
			Settings: domain.WorkspaceSettings{
				Timezone:        "Europe/London",
				DefaultLanguage: "en",
				Languages:       []string{"en"},
			},
		})
		require.NoError(t, err)
		defer updateResp.Body.Close()
		require.Equal(t, http.StatusOK, updateResp.StatusCode)

		// The blog config set by the member must be preserved, not wiped by the owner's update.
		assert.True(t, blogEnabled(t, workspaceID))
		assert.Equal(t, "Member Blog", blogTitle(t, workspaceID))
	})
}

func createTestWorkspaceWithToken(t *testing.T, client *testutil.APIClient, token, name string) string {
	currentToken := client.GetToken()
	client.SetToken(token)
	defer client.SetToken(currentToken)

	workspaceID := "test" + uuid.New().String()[:8]
	createReq := domain.CreateWorkspaceRequest{
		ID:   workspaceID,
		Name: name,
		Settings: domain.WorkspaceSettings{
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
		},
	}

	resp, err := client.Post("/api/workspaces.create", createReq)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	return workspaceID
}

// createTestWorkspace creates a test workspace by authenticating as root user.
// This function is kept for backward compatibility with other test files.
// For new tests, prefer createTestWorkspaceWithToken to avoid redundant auth calls.
func createTestWorkspace(t *testing.T, client *testutil.APIClient, name string) string {
	// Save current token
	currentToken := client.GetToken()

	// Authenticate as root user to create workspace
	rootEmail := "test@example.com" // This matches the RootEmail in test config
	rootToken := performCompleteSignInFlow(t, client, rootEmail)
	client.SetToken(rootToken)

	workspaceID := "test" + uuid.New().String()[:8]
	createReq := domain.CreateWorkspaceRequest{
		ID:   workspaceID,
		Name: name,
		Settings: domain.WorkspaceSettings{
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
		},
	}

	resp, err := client.Post("/api/workspaces.create", createReq)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Restore original token
	client.SetToken(currentToken)

	return workspaceID
}

// createTestWorkspaceWithWebsite creates a workspace with a website_url configured,
// used to exercise the workspace.website_url template variable. (CustomEndpointURL is
// intentionally not settable here: the create endpoint doesn't persist it — it only
// goes through the DNS-verified settings-update flow.)
func createTestWorkspaceWithWebsite(t *testing.T, client *testutil.APIClient, name, websiteURL string) string {
	currentToken := client.GetToken()

	rootEmail := "test@example.com" // matches the RootEmail in test config
	rootToken := performCompleteSignInFlow(t, client, rootEmail)
	client.SetToken(rootToken)

	workspaceID := "test" + uuid.New().String()[:8]
	createReq := domain.CreateWorkspaceRequest{
		ID:   workspaceID,
		Name: name,
		Settings: domain.WorkspaceSettings{
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
			WebsiteURL:      websiteURL,
		},
	}

	resp, err := client.Post("/api/workspaces.create", createReq)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	client.SetToken(currentToken)

	return workspaceID
}
