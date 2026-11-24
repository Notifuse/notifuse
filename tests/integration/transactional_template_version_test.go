//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/trackingtoken"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// A transactional.send records in message_history the version of the template it sent,
// the latest one, as broadcasts, automations and test sends do. The web version still
// refuses a transactional message, now that it carries a version.
func TestTransactionalSendRecordsTemplateVersion(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory
	serverURL := suite.ServerManager.GetURL()
	ctx := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, serverURL))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	run := uuid.New().String()[:8]
	// The template shows the web version link, which is what turns a web version on for
	// a broadcast or an automation.
	template, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateSubject("tx-version-"+run),
		testutil.WithCodeModeTemplate(fmt.Sprintf(webVersionMJML, "your receipt.")))
	require.NoError(t, err)
	// A second version, so the one recorded is the latest and not the first.
	template.Email.Subject = "tx-version-" + run + " v2"
	require.NoError(t, factory.UpdateTemplate(workspace.ID, template))
	var latest int64
	require.NoError(t, workspaceDB.QueryRowContext(ctx,
		`SELECT MAX(version) FROM templates WHERE id = $1`, template.ID).Scan(&latest))
	require.Equal(t, int64(2), latest)

	notification, err := factory.CreateTransactionalNotification(workspace.ID,
		testutil.WithTransactionalNotificationID("txversion"+run),
		testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
			domain.TransactionalChannelEmail: domain.ChannelTemplate{TemplateID: template.ID},
		}),
	)
	require.NoError(t, err)

	recipient := fmt.Sprintf("tx-version-%s@example.com", run)
	resp, err := client.PostRaw("/api/transactional.send", fmt.Sprintf(`{
		"workspace_id": %q,
		"notification": {"id": %q, "contact": {"email": %q}, "channels": ["email"]}
	}`, workspace.ID, notification.ID, recipient))
	require.NoError(t, err)
	raw, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))
	var answer struct {
		MessageID string `json:"message_id"`
	}
	require.NoError(t, json.Unmarshal(raw, &answer), string(raw))
	require.NotEmpty(t, answer.MessageID)

	t.Run("message_history holds the version sent", func(t *testing.T) {
		var templateID string
		var version int64
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT template_id, template_version FROM message_history WHERE id = $1`, answer.MessageID).Scan(&templateID, &version))
		assert.Equal(t, template.ID, templateID)
		assert.Equal(t, latest, version)
	})

	t.Run("the web version still refuses a transactional message", func(t *testing.T) {
		stored, err := factory.GetWorkspace(workspace.ID)
		require.NoError(t, err)
		token, err := trackingtoken.MintView(domain.TrackingSigningKey(stored.Settings.SecretKey), answer.MessageID, workspace.ID,
			time.Now().Unix(), trackingtoken.HostOf(serverURL))
		require.NoError(t, err)
		resp, _ := getWebVersion(t, serverURL+"/v/"+token)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}
