package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceCreateFlow(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient

	// First authenticate as root user to create workspaces
	rootEmail := "test@example.com" // This matches the RootEmail in test config
	token := performCompleteSignInFlow(t, client, rootEmail)
	client.SetToken(token)

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
				Timezone: "UTC",
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
				Timezone: "UTC",
			},
		}

		resp, err := client.Post("/api/workspaces.create", createReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("unauthorized workspace creation", func(t *testing.T) {
		// Remove token
		client.SetToken("")

		createReq := domain.CreateWorkspaceRequest{
			ID:   "unauthorized" + uuid.New().String()[:8],
			Name: "Unauthorized Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone: "UTC",
			},
		}

		resp, err := client.Post("/api/workspaces.create", createReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		// Restore token for other tests
		client.SetToken(token)
	})
}

func TestWorkspaceGetFlow(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient

	// Create authenticated user and workspace (use root user)
	email := "test@example.com" // Root user can access workspaces they create
	token := performCompleteSignInFlow(t, client, email)
	client.SetToken(token)

	workspaceID := createTestWorkspace(t, client, "Get Test Workspace")

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
}

func TestWorkspaceListFlow(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient

	// Create authenticated user (use root user)
	email := "test@example.com" // Root user can list workspaces they create
	token := performCompleteSignInFlow(t, client, email)
	client.SetToken(token)

	t.Run("successful workspace listing", func(t *testing.T) {
		// Create a few workspaces
		workspaceID1 := createTestWorkspace(t, client, "List Test Workspace 1")
		workspaceID2 := createTestWorkspace(t, client, "List Test Workspace 2")

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
	})
}

func TestWorkspaceUpdateFlow(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient

	// Create authenticated user (use root user for owner operations)
	email := "test@example.com" // Root user can perform owner operations
	token := performCompleteSignInFlow(t, client, email)
	client.SetToken(token)

	workspaceID := createTestWorkspace(t, client, "Update Test Workspace")

	t.Run("successful workspace update", func(t *testing.T) {
		updateReq := domain.UpdateWorkspaceRequest{
			ID:   workspaceID,
			Name: "Updated Workspace Name",
			Settings: domain.WorkspaceSettings{
				Timezone:             "Europe/London",
				WebsiteURL:           "https://updated.example.com",
				LogoURL:              "https://updated.example.com/logo.png",
				EmailTrackingEnabled: false,
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
				Timezone: "UTC",
			},
		}

		resp, err := client.Post("/api/workspaces.update", updateReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestWorkspaceDeleteFlow(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient

	// Create authenticated user (use root user for owner operations)
	email := "test@example.com" // Root user can perform owner operations
	token := performCompleteSignInFlow(t, client, email)
	client.SetToken(token)

	t.Run("successful workspace deletion", func(t *testing.T) {
		workspaceID := createTestWorkspace(t, client, "Delete Test Workspace")

		// Verify workspace exists
		db := suite.DBManager.GetDB()
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id = $1", workspaceID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)

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
}

func TestWorkspaceMembersFlow(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient

	// Create authenticated user (owner) - use root user for owner operations
	ownerEmail := "test@example.com" // Root user can perform owner operations
	token := performCompleteSignInFlow(t, client, ownerEmail)
	client.SetToken(token)

	workspaceID := createTestWorkspace(t, client, "Members Test Workspace")

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
}

func TestWorkspaceInviteMemberFlow(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient

	// Create authenticated user (owner) - use root user for owner operations
	ownerEmail := "test@example.com" // Root user can perform owner operations
	token := performCompleteSignInFlow(t, client, ownerEmail)
	client.SetToken(token)

	workspaceID := createTestWorkspace(t, client, "Invite Test Workspace")

	t.Run("invite existing user", func(t *testing.T) {
		// Create a user to invite
		existingUserEmail := "existing-user@example.com"
		_ = performCompleteSignInFlow(t, client, existingUserEmail)

		// Switch back to owner token
		client.SetToken(token)

		inviteReq := domain.InviteMemberRequest{
			WorkspaceID: workspaceID,
			Email:       existingUserEmail,
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
		db := suite.DBManager.GetDB()
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
		db := suite.DBManager.GetDB()
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM workspace_invitations WHERE workspace_id = $1 AND email = $2", workspaceID, newUserEmail).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("invalid email", func(t *testing.T) {
		inviteReq := domain.InviteMemberRequest{
			WorkspaceID: workspaceID,
			Email:       "invalid-email",
		}

		resp, err := client.Post("/api/workspaces.inviteMember", inviteReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestWorkspaceIntegrationsFlow(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient

	// Create authenticated user (use root user for owner operations)
	email := "test@example.com" // Root user can perform owner operations
	token := performCompleteSignInFlow(t, client, email)
	client.SetToken(token)

	workspaceID := createTestWorkspace(t, client, "Integration Test Workspace")

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
}

func TestWorkspaceAPIKeyFlow(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient

	// Create authenticated user (owner) - use root user for owner operations
	email := "test@example.com" // Root user can perform owner operations
	token := performCompleteSignInFlow(t, client, email)
	client.SetToken(token)

	workspaceID := createTestWorkspace(t, client, "API Key Test Workspace")

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
}

func TestWorkspaceRemoveMemberFlow(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient

	// Create owner - use root user for owner operations
	ownerEmail := "test@example.com" // Root user can perform owner operations
	ownerToken := performCompleteSignInFlow(t, client, ownerEmail)
	client.SetToken(ownerToken)

	workspaceID := createTestWorkspace(t, client, "Remove Member Test Workspace")

	// Create member user
	memberEmail := "workspace-member@example.com"
	_ = performCompleteSignInFlow(t, client, memberEmail)

	// Switch back to owner to add member
	client.SetToken(ownerToken)

	// Add member to workspace
	inviteReq := domain.InviteMemberRequest{
		WorkspaceID: workspaceID,
		Email:       memberEmail,
	}
	inviteResp, err := client.Post("/api/workspaces.inviteMember", inviteReq)
	require.NoError(t, err)
	inviteResp.Body.Close()

	// Get member user ID
	db := suite.DBManager.GetDB()
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
		_ = performCompleteSignInFlow(t, client, nonMemberEmail)

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
}

// Helper function to create test workspace and return its ID
// This function creates a workspace as the root user
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
			Timezone: "UTC",
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
