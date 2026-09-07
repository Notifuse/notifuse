package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

func listAuditLogs(t *testing.T, client *testutil.APIClient, params map[string]string) domain.ListAuditLogsResponse {
	t.Helper()
	resp, err := client.Get("/api/auditLogs.list", deploymentSafe(params))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	var page domain.ListAuditLogsResponse
	require.NoError(t, json.Unmarshal(body, &page))
	return page
}

// deploymentSafe pins workspace_id to empty on deployment-scope calls: the
// harness client adds its default workspace id to any URL that lacks one, which
// would silently narrow the deployment view to that workspace.
func deploymentSafe(params map[string]string) map[string]string {
	if params["scope"] != domain.AuditScopeDeployment {
		return params
	}
	if _, ok := params["workspace_id"]; ok {
		return params
	}
	pinned := make(map[string]string, len(params)+1)
	for k, v := range params {
		pinned[k] = v
	}
	pinned["workspace_id"] = ""
	return pinned
}

func auditActions(page domain.ListAuditLogsResponse) []string {
	actions := make([]string, 0, len(page.Logs))
	for _, row := range page.Logs {
		actions = append(actions, row.Action)
	}
	return actions
}

func findAuditRow(page domain.ListAuditLogsResponse, action string) *domain.AuditLog {
	for _, row := range page.Logs {
		if row.Action == action {
			return row
		}
	}
	return nil
}

// The harness serves App.GetHandler(), so what these tests see recorded is what
// production records. The harness licence carries audit_logs.
func TestAuditLogs(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	const rootEmail = "test@example.com" // RootEmail in the harness config

	t.Run("records the control plane end to end", func(t *testing.T) {
		suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
			return app.NewApp(cfg)
		})
		defer suite.Cleanup()
		client := suite.APIClient
		ctx := context.Background()

		rootToken := performCompleteSignInFlow(t, client, rootEmail)
		client.SetToken(rootToken)
		workspaceID := createTestWorkspaceWithToken(t, client, rootToken, "Audit WS")

		// A control-plane action with a target and a permission scope.
		resp, err := client.Post("/api/workspaces.inviteMember", domain.InviteMemberRequest{
			WorkspaceID: workspaceID, Email: "bob@example.com", Permissions: domain.NewFullPermissions(),
		})
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// The data plane: never recorded.
		resp, err = client.Post("/api/contacts.upsert", map[string]any{
			"workspace_id": workspaceID,
			"contact":      map[string]any{"email": "carol@example.com"},
		})
		require.NoError(t, err)
		resp.Body.Close()
		require.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp.StatusCode)

		page := listAuditLogs(t, client, map[string]string{"workspace_id": workspaceID})
		actions := auditActions(page)
		assert.Contains(t, actions, "workspaces.create")
		assert.Contains(t, actions, "workspaces.inviteMember")
		assert.NotContains(t, actions, "contacts.upsert")

		invite := findAuditRow(page, "workspaces.inviteMember")
		require.NotNil(t, invite)
		assert.Equal(t, domain.AuditOutcomeSuccess, invite.Outcome)
		assert.Equal(t, http.StatusOK, invite.StatusCode)
		assert.Equal(t, workspaceID, invite.WorkspaceID)
		assert.Equal(t, rootEmail, invite.ActorEmail)
		assert.Equal(t, domain.AuditRoleRoot, invite.ActorRole)
		assert.Equal(t, domain.AuditAuthSession, invite.AuthMethod)
		assert.Equal(t, domain.AuditTargetInvitation, invite.TargetType)
		assert.Equal(t, "bob@example.com", invite.TargetName)
		assert.NotEmpty(t, invite.TargetID)
		assert.Equal(t, "full", invite.Metadata["scope"])
		assert.NotEmpty(t, invite.IPAddress)
		assert.NotEmpty(t, invite.RequestID)

		created := findAuditRow(page, "workspaces.create")
		require.NotNil(t, created)
		assert.Equal(t, "Audit WS", created.TargetName)

		// Logins live in the deployment scope, root only.
		deployment := listAuditLogs(t, client, map[string]string{"scope": "deployment", "actions": "user.verify"})
		login := findAuditRow(deployment, "user.verify")
		require.NotNil(t, login, "the root sign-in must have been recorded")
		assert.Equal(t, rootEmail, login.ActorEmail)
		assert.Equal(t, domain.AuditAuthMagicCode, login.AuthMethod)
		assert.Empty(t, login.WorkspaceID)

		// A failed login is recorded with its reason, while the response gives
		// nothing away.
		resp, err = client.Post("/api/user.signin", domain.SignInInput{Email: "nobody@example.com"})
		require.NoError(t, err)
		resp.Body.Close()
		failed := listAuditLogs(t, client, map[string]string{"scope": "deployment", "actions": "user.signin", "outcome": domain.AuditOutcomeFailure})
		failure := findAuditRow(failed, "user.signin")
		require.NotNil(t, failure)
		assert.Equal(t, "unknown_email", failure.Metadata["reason"])
		assert.Equal(t, "nobody@example.com", failure.TargetID)
		assert.Equal(t, domain.AuditActorAnonymous, failure.ActorType)

		// Filters narrow, and the page carries a cursor shape.
		filtered := listAuditLogs(t, client, map[string]string{"workspace_id": workspaceID, "actions": "workspaces.inviteMember", "actor_email": "test@"})
		require.Len(t, filtered.Logs, 1)
		assert.False(t, filtered.HasMore)

		// Export streams CSV and is itself recorded.
		resp, err = client.Post("/api/auditLogs.export", domain.ExportAuditLogsRequest{
			AuditLogFilter: domain.AuditLogFilter{WorkspaceID: workspaceID},
			Format:         domain.AuditExportFormatCSV,
		})
		require.NoError(t, err)
		csvBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, string(csvBody))
		assert.Equal(t, "text/csv; charset=utf-8", resp.Header.Get("Content-Type"))
		assert.Contains(t, resp.Header.Get("Content-Disposition"), "audit-logs-"+workspaceID)
		lines := strings.Split(strings.TrimSpace(string(csvBody)), "\n")
		assert.True(t, strings.HasPrefix(lines[0], "id,occurred_at,workspace_id,action"))
		assert.GreaterOrEqual(t, len(lines), 3, "header plus the rows recorded so far")

		exported := listAuditLogs(t, client, map[string]string{"workspace_id": workspaceID, "actions": "auditLogs.export"})
		exportRow := findAuditRow(exported, "auditLogs.export")
		require.NotNil(t, exportRow)
		assert.Equal(t, "csv", exportRow.Metadata["format"])
		assert.Equal(t, domain.AuditTargetAuditLog, exportRow.TargetType)

		// A member reads only with the opt-in grant; a refused write is recorded
		// as denied with the resource it lacked.
		member, err := suite.DataFactory.CreateUser(testutil.WithUserEmail("member@example.com"))
		require.NoError(t, err)
		require.NoError(t, suite.DataFactory.AddUserToWorkspaceWithPermissions(member.ID, workspaceID, "member", domain.NewFullPermissions()))
		memberToken := performCompleteSignInFlow(t, client, "member@example.com")

		client.SetToken(memberToken)
		resp, err = client.Get("/api/auditLogs.list", map[string]string{"workspace_id": workspaceID})
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "full access does not imply the audit log")

		thirty := 30
		resp, err = client.Post("/api/workspaces.setAuditLogSettings", domain.SetAuditLogSettingsRequest{
			WorkspaceID: workspaceID, Settings: domain.AuditLogSettings{RetentionDays: &thirty},
		})
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "owners only")

		client.SetToken(rootToken)
		denied := listAuditLogs(t, client, map[string]string{"workspace_id": workspaceID, "outcome": domain.AuditOutcomeDenied})
		deniedRow := findAuditRow(denied, "workspaces.setAuditLogSettings")
		require.NotNil(t, deniedRow, "the refused write must be recorded")
		assert.Equal(t, "member@example.com", deniedRow.ActorEmail)
		assert.Equal(t, http.StatusForbidden, deniedRow.StatusCode)

		granted := domain.NewFullPermissions()
		granted[domain.PermissionResourceAuditLogs] = domain.ResourcePermissions{Read: true}
		resp, err = client.Post("/api/workspaces.setUserPermissions", map[string]any{
			"workspace_id": workspaceID, "user_id": member.ID, "permissions": granted,
		})
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		permissionRow := findAuditRow(listAuditLogs(t, client, map[string]string{"workspace_id": workspaceID, "actions": "workspaces.setUserPermissions"}), "workspaces.setUserPermissions")
		require.NotNil(t, permissionRow)
		assert.Equal(t, member.ID, permissionRow.TargetID)
		_, hasDiff := permissionRow.Changes["permissions"]
		assert.True(t, hasDiff, "the old and new permission maps are the change")

		// Permission changes revoke the member's sessions; sign in again.
		memberToken = performCompleteSignInFlow(t, client, "member@example.com")
		client.SetToken(memberToken)
		memberPage := listAuditLogs(t, client, map[string]string{"workspace_id": workspaceID})
		assert.NotEmpty(t, memberPage.Logs)
		resp, err = client.Get("/api/auditLogs.list", deploymentSafe(map[string]string{"scope": "deployment"}))
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "the deployment scope is root only")
		client.SetToken(rootToken)

		// The retention setting is owner-only and recorded with its diff.
		resp, err = client.Post("/api/workspaces.setAuditLogSettings", domain.SetAuditLogSettingsRequest{
			WorkspaceID: workspaceID, Settings: domain.AuditLogSettings{RetentionDays: &thirty},
		})
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Append-only: the application role cannot rewrite or delete history.
		db := suite.ServerManager.GetApp().GetDB()
		_, err = db.ExecContext(ctx, `UPDATE audit_logs SET action = 'tampered' WHERE workspace_id = $1`, workspaceID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "append-only")
		_, err = db.ExecContext(ctx, `DELETE FROM audit_logs WHERE workspace_id = $1`, workspaceID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "append-only")

		// Retention: a row older than the workspace's 30 days goes, the rest stays,
		// and the purge is itself recorded.
		oldID := strings.ReplaceAll(uuid.New().String(), "-", "")
		_, err = db.ExecContext(ctx, `INSERT INTO audit_logs (id, occurred_at, workspace_id, action, category, outcome, actor_type)
			VALUES ($1, NOW() - INTERVAL '400 days', $2, 'templates.update', 'templates', 'success', 'system')`, oldID, workspaceID)
		require.NoError(t, err)

		deleted, err := suite.ServerManager.GetApp().GetAuditLogService().PurgeExpired(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, deleted, int64(1))

		var remaining int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE id = $1`, oldID).Scan(&remaining))
		assert.Zero(t, remaining)
		purged := findAuditRow(listAuditLogs(t, client, map[string]string{"workspace_id": workspaceID, "actions": domain.AuditActionPurged}), domain.AuditActionPurged)
		require.NotNil(t, purged)
		assert.Equal(t, float64(30), purged.Metadata["retention_days"])
		assert.Equal(t, domain.AuditActorSystem, purged.ActorType)

		// A single row by id, scoped to its workspace.
		resp, err = client.Get("/api/auditLogs.get", map[string]string{"workspace_id": workspaceID, "id": invite.ID})
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		resp, err = client.Get("/api/auditLogs.get", map[string]string{"workspace_id": "other", "id": invite.ID})
		require.NoError(t, err)
		resp.Body.Close()
		assert.NotEqual(t, http.StatusOK, resp.StatusCode, "another workspace never sees the row")

		// The catalogue the console filters with.
		resp, err = client.Get("/api/auditLogs.actions")
		require.NoError(t, err)
		var catalogue struct {
			Actions []domain.AuditActionDescriptor `json:"actions"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&catalogue))
		resp.Body.Close()
		assert.NotEmpty(t, catalogue.Actions)

		// Every response carries a request id.
		resp, err = client.Get("/api/user.me")
		require.NoError(t, err)
		resp.Body.Close()
		assert.NotEmpty(t, resp.Header.Get("X-Request-ID"))
	})

	t.Run("an unlicensed server records nothing and still serves the page", func(t *testing.T) {
		suite := testutil.NewIntegrationTestSuite(t, unlicensed(nil))
		defer suite.Cleanup()
		client := suite.APIClient

		rootToken := performCompleteSignInFlow(t, client, rootEmail)
		client.SetToken(rootToken)
		workspaceID := createTestWorkspaceWithToken(t, client, rootToken, "Unlicensed WS")

		page := listAuditLogs(t, client, map[string]string{"workspace_id": workspaceID})
		assert.Empty(t, page.Logs, "nothing is written without a licence covering audit_logs")
		assert.False(t, page.HasMore)

		deployment := listAuditLogs(t, client, map[string]string{"scope": "deployment"})
		assert.Empty(t, deployment.Logs)

		resp, err := client.Post("/api/auditLogs.export", domain.ExportAuditLogsRequest{
			AuditLogFilter: domain.AuditLogFilter{WorkspaceID: workspaceID},
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "reading is never refused for licence reasons")
		assert.Len(t, strings.Split(strings.TrimSpace(string(body)), "\n"), 1, "the header only")

		thirty := 30
		resp, err = client.Post("/api/workspaces.setAuditLogSettings", domain.SetAuditLogSettingsRequest{
			WorkspaceID: workspaceID, Settings: domain.AuditLogSettings{RetentionDays: &thirty},
		})
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "the retention setting works in every licence state")
	})
}
