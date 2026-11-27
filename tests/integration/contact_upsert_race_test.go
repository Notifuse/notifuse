//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// The lookup of UpsertContact locks nothing for an address that has no contact yet, so a
// second call creating the same contact can commit between that lookup and the insert. These
// tests stop the first call right there, create the contact from another call, and let the
// first one go on: it must update the contact the other call created instead of failing on
// contacts_pkey.
func TestContactUpsertDuringConcurrentCreateUpdatesTheNewContact(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	defer repository.SetContactUpsertAfterLookupHook(nil)

	client := suite.APIClient
	factory := suite.DataFactory
	ctx := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	run := strings.ToLower(uuid.New().String()[:8])

	post := func(t *testing.T, path, body string) (int, []byte) {
		t.Helper()
		resp, err := client.PostRaw(path, body)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, raw
	}

	// hookOnce runs act inside the first lookup that finds no contact for email, and counts
	// every lookup of email that reaches the hook.
	hookOnce := func(email string, act func()) *atomic.Int32 {
		var calls atomic.Int32
		var acted atomic.Bool
		repository.SetContactUpsertAfterLookupHook(func(_ context.Context, looked string) {
			if looked != email {
				return
			}
			calls.Add(1)
			// Not a sync.Once: the other call reaches this hook while act waits for it.
			if acted.CompareAndSwap(false, true) {
				act()
			}
		})
		return &calls
	}

	t.Run("contacts.upsert", func(t *testing.T) {
		email := fmt.Sprintf("upsert-race-%s@example.com", run)
		calls := hookOnce(email, func() {
			// The other call creates the contact and commits while the first one waits.
			_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email), testutil.WithContactCountry("FR"))
			require.NoError(t, err)
		})
		status, raw := post(t, "/api/contacts.upsert", fmt.Sprintf(
			`{"workspace_id": %q, "contact": {"email": %q, "first_name": "Ada"}}`, workspace.ID, email))
		repository.SetContactUpsertAfterLookupHook(nil)

		// The call's lookup and the other call's (the factory upserts too) found no contact; the
		// call's second run found the one the other call created and went to the update.
		require.Equal(t, int32(2), calls.Load(), "both lookups ran before the contact existed, and no third one did")
		require.Equalf(t, http.StatusOK, status, "contacts.upsert: %s", raw)
		var answer struct {
			Action string `json:"action"`
		}
		require.NoError(t, json.Unmarshal(raw, &answer), string(raw))
		assert.Equal(t, "update", answer.Action, "the contact existed when the call wrote it")

		var firstName, country string
		var contacts int
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT count(*) OVER (), coalesce(first_name, ''), coalesce(country, '') FROM contacts WHERE email = $1`, email).
			Scan(&contacts, &firstName, &country))
		assert.Equal(t, 1, contacts)
		assert.Equal(t, "Ada", firstName, "the call's field is written")
		assert.Equal(t, "FR", country, "the other call's field is kept")
	})

	t.Run("transactional.send with one external_id", func(t *testing.T) {
		mailpit, err := factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
		require.NoError(t, err)
		_ = mailpit
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("Upsert race"),
			testutil.WithTemplateSubject("upsert-race-"+run))
		require.NoError(t, err)
		notification, err := factory.CreateTransactionalNotification(workspace.ID,
			testutil.WithTransactionalNotificationID("upsertrace"+run),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{TemplateID: template.ID},
			}),
		)
		require.NoError(t, err)

		recipient := fmt.Sprintf("send-race-%s@example.com", run)
		body := fmt.Sprintf(`{
			"workspace_id": %q,
			"notification": {"id": %q, "external_id": %q, "contact": {"email": %q}, "channels": ["email"]}
		}`, workspace.ID, notification.ID, "order-race-"+run, recipient)
		messageID := func(t *testing.T, status int, raw []byte) string {
			t.Helper()
			require.Equalf(t, http.StatusOK, status, "transactional.send: %s", raw)
			var answer struct {
				MessageID string `json:"message_id"`
			}
			require.NoError(t, json.Unmarshal(raw, &answer), string(raw))
			return answer.MessageID
		}

		// The second call runs whole while the first one waits after its lookup: it creates
		// the contact, writes its message and sends.
		var secondStatus int
		var secondRaw []byte
		calls := hookOnce(recipient, func() {
			secondStatus, secondRaw = post(t, "/api/transactional.send", body)
		})
		firstStatus, firstRaw := post(t, "/api/transactional.send", body)
		repository.SetContactUpsertAfterLookupHook(nil)

		require.Equal(t, int32(2), calls.Load(), "both calls looked the contact up before either created it")
		second := messageID(t, secondStatus, secondRaw)
		first := messageID(t, firstStatus, firstRaw)
		assert.Equal(t, second, first, "both calls answer the one message")

		var total int
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT count(*) FROM message_history WHERE contact_email = $1`, recipient).Scan(&total))
		assert.Equal(t, 1, total, "one message written")
		_, err = testutil.WaitForMailpitMessageByRecipient(t, recipient, 15*time.Second)
		require.NoError(t, err)
		// Give a duplicate the time to arrive before counting.
		time.Sleep(2 * time.Second)
		count, err := testutil.CountMailpitMessagesBySubject(t, "upsert-race-"+run)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "one email delivered")
	})
}
