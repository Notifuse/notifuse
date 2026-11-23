//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// transactional.send deduplicates on external_id: calls arriving together with one key send
// once, a send that failed at the provider keeps its key, and an empty key is no key.
func TestTransactionalSendExternalID(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory
	ctx := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	mailpit, err := factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	// Mailpit is shared with other suites: each case counts the emails by its own subject.
	run := uuid.New().String()[:8]
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("External ID"),
		testutil.WithTemplateSubject("extid-"+run+" {{ subject_tag }}"))
	require.NoError(t, err)
	notification, err := factory.CreateTransactionalNotification(workspace.ID,
		testutil.WithTransactionalNotificationID("extid"+run),
		testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
			domain.TransactionalChannelEmail: domain.ChannelTemplate{TemplateID: template.ID},
		}),
	)
	require.NoError(t, err)

	send := func(t *testing.T, recipient, externalID, subjectTag string) (int, string) {
		t.Helper()
		body := fmt.Sprintf(`{
			"workspace_id": %q,
			"notification": {
				"id": %q,
				"external_id": %q,
				"contact": {"email": %q},
				"channels": ["email"],
				"data": {"subject_tag": %q}
			}
		}`, workspace.ID, notification.ID, externalID, recipient, subjectTag)
		resp, err := client.PostRaw("/api/transactional.send", body)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		var answer struct {
			MessageID string `json:"message_id"`
		}
		if resp.StatusCode != http.StatusOK {
			t.Logf("transactional.send answered %d: %s", resp.StatusCode, raw)
			return resp.StatusCode, ""
		}
		require.NoError(t, json.Unmarshal(raw, &answer), string(raw))
		return resp.StatusCode, answer.MessageID
	}
	historyRows := func(t *testing.T, recipient string) (total int, keyed int) {
		t.Helper()
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT count(*), count(external_id) FROM message_history WHERE contact_email = $1`, recipient).Scan(&total, &keyed))
		return total, keyed
	}

	t.Run("calls arriving together with one external_id send once", func(t *testing.T) {
		recipient := fmt.Sprintf("extid-concurrent-%s@example.com", run)
		externalID := "order-concurrent-" + run
		tag := "concurrent"
		const calls = 8
		// The contact exists already: concurrent upserts of a contact that does not exist yet
		// race on its insert, a separate fault this test does not cover.
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(recipient))
		require.NoError(t, err)

		var wg sync.WaitGroup
		start := make(chan struct{})
		statuses := make([]int, calls)
		ids := make([]string, calls)
		for i := 0; i < calls; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				statuses[i], ids[i] = send(t, recipient, externalID, tag)
			}(i)
		}
		close(start)
		wg.Wait()

		for i := 0; i < calls; i++ {
			require.Equal(t, http.StatusOK, statuses[i], "call %d", i)
			assert.Equal(t, ids[0], ids[i], "every call answers the one message sent")
		}

		total, keyed := historyRows(t, recipient)
		assert.Equal(t, 1, total, "one message written")
		assert.Equal(t, 1, keyed)

		_, err = testutil.WaitForMailpitMessageByRecipient(t, recipient, 15*time.Second)
		require.NoError(t, err)
		// Give a duplicate the time to arrive before counting.
		time.Sleep(2 * time.Second)
		count, err := testutil.CountMailpitMessagesBySubject(t, "extid-"+run+" "+tag)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "one email delivered")

		// A later replay is answered with the same message.
		status, id := send(t, recipient, externalID, tag)
		require.Equal(t, http.StatusOK, status)
		assert.Equal(t, ids[0], id)
	})

	// A key is never released: the failed message keeps it, and a replay answers that message
	// without sending. A client retries a failed send under a new key.
	t.Run("a send that failed at the provider keeps its external_id", func(t *testing.T) {
		recipient := fmt.Sprintf("extid-failed-%s@example.com", run)
		externalID := "order-failed-" + run
		tag := "failed"

		failing, err := factory.CreateFailingSMTPIntegration(workspace.ID)
		require.NoError(t, err)
		setTransactionalProvider := func(integrationID string) {
			ws, err := factory.GetWorkspace(workspace.ID)
			require.NoError(t, err)
			ws.Settings.TransactionalEmailProviderID = integrationID
			require.NoError(t, factory.SaveWorkspace(ws))
		}

		setTransactionalProvider(failing.ID)
		status, _ := send(t, recipient, externalID, tag)
		require.Equal(t, http.StatusInternalServerError, status, "the provider refused the send")

		var failedID string
		var failedAt sql.NullTime
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT id, failed_at FROM message_history WHERE external_id = $1`, externalID).Scan(&failedID, &failedAt))
		assert.True(t, failedAt.Valid, "the failure is recorded on the message")

		setTransactionalProvider(mailpit.ID)
		status, id := send(t, recipient, externalID, tag)
		require.Equal(t, http.StatusOK, status)
		assert.Equal(t, failedID, id, "the replay answers the failed message")

		total, keyed := historyRows(t, recipient)
		assert.Equal(t, 1, total, "the replay writes no message")
		assert.Equal(t, 1, keyed, "the failed message keeps its external_id")

		// Give a send the time to arrive before counting.
		time.Sleep(2 * time.Second)
		count, err := testutil.CountMailpitMessagesBySubject(t, "extid-"+run+" "+tag)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "the replay sends nothing")

		// A new key sends.
		status, retryID := send(t, recipient, externalID+"-2", tag)
		require.Equal(t, http.StatusOK, status)
		assert.NotEqual(t, failedID, retryID)
		_, err = testutil.WaitForMailpitMessageByRecipient(t, recipient, 15*time.Second)
		require.NoError(t, err, "the send under a new key goes out")
	})

	// "" is no key: two sends with it are two messages, both sent, stored without a key.
	t.Run("two sends with an empty external_id both send", func(t *testing.T) {
		recipient := fmt.Sprintf("extid-empty-%s@example.com", run)
		tag := "empty"

		status, first := send(t, recipient, "", tag)
		require.Equal(t, http.StatusOK, status)
		status, second := send(t, recipient, "", tag)
		require.Equal(t, http.StatusOK, status)
		assert.NotEqual(t, first, second)

		total, keyed := historyRows(t, recipient)
		assert.Equal(t, 2, total, "two messages written")
		assert.Equal(t, 0, keyed, "neither carries a key")

		deadline := time.Now().Add(15 * time.Second)
		count := 0
		for time.Now().Before(deadline) {
			count, err = testutil.CountMailpitMessagesBySubject(t, "extid-"+run+" "+tag)
			require.NoError(t, err)
			if count >= 2 {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		assert.Equal(t, 2, count, "both emails delivered")
	})
}
