package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBounceThresholdPerEmail: the soft bounce threshold counts the emails that bounced
// since the address was last delivered to, not the provider's reports, and Postmark
// reports that say nothing of whether the address can be reached do not count.
func TestBounceThresholdPerEmail(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory
	bg := context.Background()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	contactListRepo := suite.ServerManager.GetApp().GetContactListRepository()

	sender := []domain.EmailSender{domain.NewEmailSender("test@example.com", "Test Sender")}
	webhookURL := func(t *testing.T, provider domain.EmailProvider) string {
		t.Helper()
		integration, err := factory.CreateIntegration(workspace.ID,
			testutil.WithIntegrationName("Threshold "+string(provider.Kind)),
			testutil.WithIntegrationEmailProvider(provider))
		require.NoError(t, err)
		return fmt.Sprintf("/webhooks/email?provider=%s&workspace_id=%s&integration_id=%s", provider.Kind, workspace.ID, integration.ID)
	}
	mailgunURL := webhookURL(t, domain.EmailProvider{Kind: domain.EmailProviderKindMailgun, Senders: sender,
		Mailgun: &domain.MailgunSettings{APIKey: "test-key", Domain: "example.com"}})
	postmarkURL := webhookURL(t, domain.EmailProvider{Kind: domain.EmailProviderKindPostmark, Senders: sender,
		Postmark: &domain.PostmarkSettings{ServerToken: "test-token"}})

	newContact := func(t *testing.T) string {
		t.Helper()
		email := fmt.Sprintf("threshold-%s@example.com", uuid.New().String()[:8])
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		return email
	}
	sendTo := func(t *testing.T, email string) string {
		t.Helper()
		return sentEmailID(t, factory, workspace.ID, template.ID, email)
	}
	statusOf := func(t *testing.T, email string) domain.ContactListStatus {
		t.Helper()
		row, err := contactListRepo.GetContactListByIDs(bg, workspace.ID, email, list.ID)
		require.NoError(t, err)
		return row.Status
	}
	storedFor := func(t *testing.T, messageID string) int {
		t.Helper()
		var stored int
		require.NoError(t, db.QueryRowContext(bg, `SELECT COUNT(*) FROM inbound_webhook_events WHERE message_id = $1`, messageID).Scan(&stored))
		return stored
	}
	// post sends the report and checks it was stored, so that a contact left active means
	// the report was read and did not count
	post := func(t *testing.T, url, messageID, body string) {
		t.Helper()
		before := storedFor(t, messageID)
		resp, err := client.PostRaw(url, body)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode, "webhook POST must return 200")
		require.Equal(t, before+1, storedFor(t, messageID), "the report must be stored")
	}
	at := time.Now().UTC().Truncate(time.Second)
	mailgunTemporaryFailure := func(messageID, email string) string {
		return fmt.Sprintf(`{"event-data":{"event":"failed","id":%q,"severity":"temporary","timestamp":%d,"recipient":%q,"reason":"greylisted","message":{"headers":{"message-id":"mg-%s"}},"user-variables":{"notifuse_message_id":%q}}}`,
			uuid.New().String(), at.Unix(), email, messageID, messageID)
	}
	postmarkBounce := func(messageID, email, postmarkType string) string {
		return postmarkBouncePayload(messageID, email, postmarkType, numericID(uuid.New().String()), at)
	}

	t.Run("Mailgun: each attempt to deliver one email counts as one bounce", func(t *testing.T) {
		email := newContact(t)
		retried := sendTo(t, email)
		for i := 0; i < domain.DefaultSoftBounceThreshold; i++ {
			post(t, mailgunURL, retried, mailgunTemporaryFailure(retried, email))
		}
		require.Equal(t, domain.ContactListStatusActive, statusOf(t, email))

		for i := 1; i < domain.DefaultSoftBounceThreshold-1; i++ {
			messageID := sendTo(t, email)
			post(t, mailgunURL, messageID, mailgunTemporaryFailure(messageID, email))
			require.Equalf(t, domain.ContactListStatusActive, statusOf(t, email), "after %d emails that bounced", i+1)
		}
		messageID := sendTo(t, email)
		post(t, mailgunURL, messageID, mailgunTemporaryFailure(messageID, email))
		assert.Equal(t, domain.ContactListStatusBounced, statusOf(t, email))
	})

	t.Run("Postmark: auto-replies do not count, soft bounces do", func(t *testing.T) {
		email := newContact(t)
		for i := 0; i < domain.DefaultSoftBounceThreshold; i++ {
			messageID := sendTo(t, email)
			post(t, postmarkURL, messageID, postmarkBounce(messageID, email, "AutoResponder"))
		}
		for i := 0; i < domain.DefaultSoftBounceThreshold-1; i++ {
			messageID := sendTo(t, email)
			post(t, postmarkURL, messageID, postmarkBounce(messageID, email, "SoftBounce"))
		}
		require.Equal(t, domain.ContactListStatusActive, statusOf(t, email))

		messageID := sendTo(t, email)
		post(t, postmarkURL, messageID, postmarkBounce(messageID, email, "SoftBounce"))
		assert.Equal(t, domain.ContactListStatusBounced, statusOf(t, email))
	})

	t.Run("Postmark: an ISP block counts as a soft bounce", func(t *testing.T) {
		email := newContact(t)
		for i := 1; i < domain.DefaultSoftBounceThreshold; i++ {
			messageID := sendTo(t, email)
			post(t, postmarkURL, messageID, postmarkBounce(messageID, email, "Blocked"))
			require.Equalf(t, domain.ContactListStatusActive, statusOf(t, email), "after %d blocks", i)
		}
		messageID := sendTo(t, email)
		post(t, postmarkURL, messageID, postmarkBounce(messageID, email, "Blocked"))
		assert.Equal(t, domain.ContactListStatusBounced, statusOf(t, email))
	})

	t.Run("Postmark: an invalid address is a hard bounce", func(t *testing.T) {
		email := newContact(t)
		messageID := sendTo(t, email)
		post(t, postmarkURL, messageID, postmarkBounce(messageID, email, "BadEmailAddress"))
		assert.Equal(t, domain.ContactListStatusBounced, statusOf(t, email))
	})
}
