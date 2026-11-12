//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// The template library shows, for each template, what still uses it.
func TestTemplatesUsage(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	factory := suite.DataFactory
	client := suite.APIClient
	ctx := context.Background()

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	tid := template.ID

	withVariations := func(status domain.BroadcastStatus, name string, templateIDs ...string) testutil.BroadcastOption {
		return func(b *domain.Broadcast) {
			b.Status = status
			b.Name = name
			b.TestSettings.Variations = nil
			for _, id := range templateIDs {
				b.TestSettings.Variations = append(b.TestSettings.Variations, domain.BroadcastVariation{VariationName: id, TemplateID: id})
			}
		}
	}
	// A draft A/B broadcast using the template in both variations counts once.
	draft, err := factory.CreateBroadcast(workspace.ID, withVariations(domain.BroadcastStatusDraft, "A draft", tid, tid))
	require.NoError(t, err)
	// A processed broadcast is done with its template.
	_, err = factory.CreateBroadcast(workspace.ID, withVariations(domain.BroadcastStatusProcessed, "B processed", tid))
	require.NoError(t, err)

	notification, err := factory.CreateTransactionalNotification(workspace.ID, func(n *domain.TransactionalNotification) {
		n.Name = "Password reset"
		n.Channels = domain.ChannelTemplates{domain.TransactionalChannelEmail: domain.ChannelTemplate{TemplateID: tid}}
	})
	require.NoError(t, err)
	deletedNotification, err := factory.CreateTransactionalNotification(workspace.ID, func(n *domain.TransactionalNotification) {
		n.Channels = domain.ChannelTemplates{domain.TransactionalChannelEmail: domain.ChannelTemplate{TemplateID: tid}}
	})
	require.NoError(t, err)
	_, err = workspaceDB.ExecContext(ctx, `UPDATE transactional_notifications SET deleted_at = NOW() WHERE id = $1`, deletedNotification.ID)
	require.NoError(t, err)

	// An automation using the template in an email node and in its trigger counts once.
	usingTemplate := func(name string) testutil.AutomationOption {
		return func(a *domain.Automation) {
			a.Name = name
			a.Status = domain.AutomationStatusLive
			id := tid
			a.Trigger.EventKind = "email.link_clicked"
			a.Trigger.TemplateID = &id
			a.Nodes = []*domain.AutomationNode{
				{ID: "n1", Type: domain.NodeTypeEmail, Config: map[string]interface{}{"template_id": tid}},
				{ID: "n2", Type: domain.NodeTypeDelay, Config: map[string]interface{}{"duration": 1, "unit": "days"}},
			}
		}
	}
	automation, err := factory.CreateAutomation(workspace.ID, usingTemplate("Onboarding"))
	require.NoError(t, err)
	deletedAutomation, err := factory.CreateAutomation(workspace.ID, usingTemplate("Old onboarding"))
	require.NoError(t, err)
	_, err = workspaceDB.ExecContext(ctx, `UPDATE automations SET deleted_at = NOW() WHERE id = $1`, deletedAutomation.ID)
	require.NoError(t, err)
	// An automation without nodes holds JSON null: it must not fail the query.
	empty, err := factory.CreateAutomation(workspace.ID)
	require.NoError(t, err)
	_, err = workspaceDB.ExecContext(ctx, `UPDATE automations SET nodes = 'null'::jsonb WHERE id = $1`, empty.ID)
	require.NoError(t, err)

	doubleOptIn := func(name string, enabled bool) testutil.ListOption {
		return func(l *domain.List) {
			l.Name = name
			l.IsDoubleOptin = enabled
			l.DoubleOptInTemplate = &domain.TemplateReference{ID: tid, Version: 1}
		}
	}
	list, err := factory.CreateList(workspace.ID, doubleOptIn("Newsletter", true))
	require.NoError(t, err)
	// The template kept on a list whose double opt-in is off does not count.
	_, err = factory.CreateList(workspace.ID, doubleOptIn("Single opt-in", false))
	require.NoError(t, err)
	deletedList, err := factory.CreateList(workspace.ID, doubleOptIn("Old newsletter", true))
	require.NoError(t, err)
	_, err = workspaceDB.ExecContext(ctx, `UPDATE lists SET deleted_at = NOW() WHERE id = $1`, deletedList.ID)
	require.NoError(t, err)

	usage := func(t *testing.T, params map[string]string) map[string]interface{} {
		t.Helper()
		params["workspace_id"] = workspace.ID
		resp, err := client.Get("/api/templates.usage", params)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var decoded map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&decoded))
		return decoded
	}

	require.NoError(t, client.Login(owner.Email, "password"))

	t.Run("every template: each live use counted once", func(t *testing.T) {
		got := usage(t, map[string]string{})
		counts, ok := got["usage"].(map[string]interface{})[tid].(map[string]interface{})
		require.True(t, ok, "%v", got)
		require.Equal(t, float64(1), counts["broadcasts"])
		require.Equal(t, float64(1), counts["transactional_notifications"])
		require.Equal(t, float64(1), counts["automations"])
		require.Equal(t, float64(1), counts["lists"])
		assert.NotContains(t, got, "references")
		assert.Equal(t, []interface{}{}, got["hidden_types"])
	})

	t.Run("one template: the objects using it", func(t *testing.T) {
		got := usage(t, map[string]string{"template_id": tid})
		assert.Equal(t, []interface{}{
			map[string]interface{}{"type": "broadcast", "id": draft.ID, "name": "A draft", "status": "draft"},
			map[string]interface{}{"type": "transactional_notification", "id": notification.ID, "name": "Password reset"},
			map[string]interface{}{"type": "automation", "id": automation.ID, "name": "Onboarding", "status": "live"},
			map[string]interface{}{"type": "list", "id": list.ID, "name": "Newsletter"},
		}, got["references"])
		counts := got["usage"].(map[string]interface{})[tid].(map[string]interface{})
		assert.Equal(t, float64(1), counts["automations"])
	})

	t.Run("an unused template has no entry and no reference", func(t *testing.T) {
		unused, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)
		got := usage(t, map[string]string{"template_id": unused.ID})
		assert.Equal(t, map[string]interface{}{}, got["usage"])
		assert.Equal(t, []interface{}{}, got["references"])
	})

	t.Run("a member who may not read automations does not see them", func(t *testing.T) {
		member, err := factory.CreateUser()
		require.NoError(t, err)
		perms := domain.NewFullPermissions()
		perms[domain.PermissionResourceAutomations] = domain.ResourcePermissions{}
		require.NoError(t, factory.AddUserToWorkspaceWithPermissions(member.ID, workspace.ID, "member", perms))
		require.NoError(t, client.Login(member.Email, "password"))
		defer func() { require.NoError(t, client.Login(owner.Email, "password")) }()

		got := usage(t, map[string]string{"template_id": tid})
		counts := got["usage"].(map[string]interface{})[tid].(map[string]interface{})
		assert.NotContains(t, counts, "automations")
		assert.Equal(t, float64(1), counts["broadcasts"])
		assert.Equal(t, []interface{}{"automations"}, got["hidden_types"])
		for _, ref := range got["references"].([]interface{}) {
			assert.NotEqual(t, "automation", ref.(map[string]interface{})["type"])
		}
	})
}
