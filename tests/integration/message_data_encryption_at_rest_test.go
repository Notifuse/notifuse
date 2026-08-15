//go:build integration
// +build integration

package integration

import (
	"fmt"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestMessageDataStaysEncryptedWhenTheSendFails asserts the encryption-at-rest
// claim for message_data on both send outcomes.
//
// Create() encrypts the blob through encryptMessageData, and the failure path
// does not go back through Create. It used to rewrite the whole row from the
// in-memory copy through a repository Update that took no secret key at all, so
// the plaintext template data landed back on top of the ciphertext; it now
// records the failure through SetStatusesIfNotSet, which never touches the
// column. This runs the same send twice against two workspaces, one wired to
// Mailpit and one to a dead port, so the only difference between the two rows is
// which write path produced them.
func TestMessageDataStaysEncryptedWhenTheSendFails(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	factory := suite.DataFactory
	client := suite.APIClient

	user, err := factory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))

	// A string that appears nowhere but the template data, so finding it in the
	// stored column means the blob went in as plaintext.
	canary := "PLAINTEXT-CANARY-" + uuid.New().String()

	// send wires a workspace to smtpPort, sends one transactional notification
	// carrying the canary, and returns the raw message_data column plus how the
	// send was recorded.
	send := func(t *testing.T, smtpPort int) (rawBlob string, failed bool, statusInfo string) {
		t.Helper()

		workspace, err := factory.CreateWorkspace()
		require.NoError(t, err)
		require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))

		_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
			testutil.WithIntegrationEmailProvider(domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
				Senders: []domain.EmailSender{
					domain.NewEmailSender("noreply@notifuse.test", "Encryption At Rest"),
				},
				SMTP: &domain.SMTPSettings{
					Host: "localhost", Port: smtpPort, UseTLS: false,
				},
				RateLimitPerMinute: 1000,
			}))
		require.NoError(t, err)
		client.SetWorkspaceID(workspace.ID)

		contactEmail := fmt.Sprintf("at-rest-%s@example.com", uuid.New().String()[:8])
		_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(contactEmail))
		require.NoError(t, err)

		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("at rest"),
			testutil.WithTemplateSubject("at rest "+uuid.New().String()[:8]),
			testutil.WithCodeModeTemplate(`<mjml><mj-body><mj-section><mj-column>
				<mj-text>Note: {{ order_note }}</mj-text>
			</mj-column></mj-section></mj-body></mjml>`))
		require.NoError(t, err)

		notification, err := factory.CreateTransactionalNotification(workspace.ID,
			testutil.WithTransactionalNotificationID(fmt.Sprintf("at-rest-%d", smtpPort)),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{
					TemplateID: template.ID,
					Settings:   map[string]interface{}{},
				},
			}))
		require.NoError(t, err)

		resp, err := client.SendTransactionalNotification(map[string]interface{}{
			"id":       notification.ID,
			"contact":  map[string]interface{}{"email": contactEmail},
			"channels": []string{"email"},
			"data":     map[string]interface{}{"order_note": canary},
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Logf("transactional.send on port %d → %d: %s", smtpPort, resp.StatusCode, string(body))

		wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
		require.NoError(t, err)
		require.NoError(t, wsDB.QueryRow(
			`SELECT message_data::text, failed_at IS NOT NULL, COALESCE(status_info, '')
			 FROM message_history WHERE contact_email = $1`,
			contactEmail).Scan(&rawBlob, &failed, &statusInfo))
		return rawBlob, failed, statusInfo
	}

	t.Run("a send that succeeds stores the blob encrypted", func(t *testing.T) {
		blob, failed, statusInfo := send(t, 1025) // Mailpit
		require.False(t, failed, "the control send failed, so it exercised the same path as the case below")
		assert.Empty(t, statusInfo, "a delivered send was annotated with a failure reason")

		assert.Contains(t, blob, "_encrypted", "Create() did not encrypt the blob")
		assert.NotContains(t, blob, canary, "the template data is readable in the column")
	})

	t.Run("a send that fails stores the blob encrypted", func(t *testing.T) {
		blob, failed, statusInfo := send(t, 9999) // nothing listening
		require.True(t, failed, "the send did not fail, so the failure path never ran and this asserts nothing")

		assert.Contains(t, blob, "_encrypted", "the failure path rewrote the blob without encrypting it")
		assert.NotContains(t, blob, canary, "the template data is readable in the column")

		// The narrower write must still record WHY it failed — an erasure of the
		// failure reason would pass every assertion above.
		assert.Contains(t, statusInfo, "connect",
			"the failure was recorded without the provider error: %q", statusInfo)
	})
}
