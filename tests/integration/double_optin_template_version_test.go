//go:build integration
// +build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// A list's confirmation email is built from the latest version of its template, as broadcasts,
// transactional notifications and automations are: the console saves every list with version 1,
// and its users edit the template expecting the change in the next confirmation. The message
// history records the version the email was built from, the latest one, not the 0 that asked
// for it.
func TestDoubleOptInConfirmationRecordsTheTemplateVersionSent(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	factory := suite.DataFactory
	client := suite.APIClient
	serverURL := suite.ServerManager.GetURL()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	marker := uuid.New().String()[:8]
	body := `<mjml><mj-body><mj-section><mj-column>` +
		`<mj-text><a href="{{ confirm_subscription_url }}">Confirm</a></mj-text>` +
		`</mj-column></mj-section></mj-body></mjml>`
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateSubject("Confirm v1 "+marker),
		testutil.WithCodeModeTemplate(body))
	require.NoError(t, err)
	require.Equal(t, int64(1), template.Version)
	// Edited after the list was set up: version 2.
	template.Email.Subject = "Confirm v2 " + marker
	require.NoError(t, factory.UpdateTemplate(workspace.ID, template))

	// The list, saved as the console saves it: version 1.
	listID := "doi" + strings.ReplaceAll(uuid.New().String(), "-", "")[:20]
	resp, err := client.CreateList(map[string]interface{}{
		"workspace_id":          workspace.ID,
		"id":                    listID,
		"name":                  "Newsletter",
		"is_double_optin":       true,
		"is_public":             true,
		"double_optin_template": map[string]interface{}{"id": template.ID, "version": 1},
	})
	require.NoError(t, err)
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equalf(t, http.StatusCreated, resp.StatusCode, "lists.create: %s", raw)

	email := fmt.Sprintf("doi-version-%s@example.com", marker)
	payload, err := json.Marshal(map[string]interface{}{
		"workspace_id": workspace.ID,
		"contact":      map[string]interface{}{"email": email},
		"list_ids":     []string{listID},
	})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, serverURL+"/subscribe", bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	subscribeResp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	require.NoError(t, err)
	raw, _ = io.ReadAll(subscribeResp.Body)
	_ = subscribeResp.Body.Close()
	require.Equalf(t, http.StatusOK, subscribeResp.StatusCode, "subscribe: %s", raw)

	// The email left with the latest version.
	message, err := testutil.WaitForMailpitMessageByRecipient(t, email, 10*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "Confirm v2 "+marker, message.Subject)

	// And the message history says so.
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	var templateID string
	var templateVersion int64
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT template_id, template_version FROM message_history WHERE contact_email = $1`, email,
	).Scan(&templateID, &templateVersion))
	assert.Equal(t, template.ID, templateID)
	assert.Equal(t, int64(2), templateVersion)

	// The list keeps the reference it was saved with.
	var stored domain.TemplateReference
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT double_optin_template FROM lists WHERE id = $1`, listID,
	).Scan(&stored))
	assert.Equal(t, domain.TemplateReference{ID: template.ID, Version: 1}, stored)
}
