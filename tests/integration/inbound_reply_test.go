package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInboundReplyDetection exercises the inbound-reply layer of issue #346
// end-to-end: a Mailgun Routes-style inbound POST for a known contact must
// produce an "email.replied" entry on the contact timeline (the event that
// automations trigger on), while a reply from an unknown sender produces none.
func TestInboundReplyDetection(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))

	// A Mailgun email integration is required for the inbound endpoint to accept
	// the reply (the service dispatches on the integration's provider kind).
	integration, err := factory.CreateIntegration(workspace.ID, testutil.WithIntegrationEmailProvider(domain.EmailProvider{
		Kind: domain.EmailProviderKindMailgun,
		Senders: []domain.EmailSender{
			domain.NewEmailSender("hello@example.com", "Hello"),
		},
		Mailgun: &domain.MailgunSettings{
			Domain: "example.com",
			Region: "US",
		},
	}))
	require.NoError(t, err)

	inboundURL := suite.ServerManager.GetURL() + fmt.Sprintf(
		"/webhooks/email/inbound?workspace_id=%s&integration_id=%s", workspace.ID, integration.ID)

	postReply := func(t *testing.T, form url.Values) *http.Response {
		t.Helper()
		resp, err := http.Post(inboundURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	// waitForRepliedEvent polls the contact timeline for an "email.replied"
	// entry (the timeline write happens in the same transaction as the inbound
	// event insert, but we poll briefly to avoid any ordering flakiness).
	waitForRepliedEvent := func(t *testing.T, email string) int {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			events, err := factory.GetContactTimelineEvents(workspace.ID, email, "email.replied")
			require.NoError(t, err)
			if len(events) > 0 || time.Now().After(deadline) {
				return len(events)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	t.Run("KnownContactEmitsEmailReplied", func(t *testing.T) {
		contactEmail := fmt.Sprintf("replier-%d@example.com", time.Now().UnixNano())
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(contactEmail))
		require.NoError(t, err)

		form := url.Values{}
		form.Set("sender", contactEmail)
		form.Set("from", "Replier <"+contactEmail+">")
		form.Set("subject", "Re: Welcome aboard")
		form.Set("Message-Id", "<reply-1@mail.example.com>")
		form.Set("In-Reply-To", "<orig-1@notifuse>")
		form.Set("timestamp", "1700000000")

		resp := postReply(t, form)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		count := waitForRepliedEvent(t, contactEmail)
		assert.GreaterOrEqual(t, count, 1, "expected an email.replied timeline entry for the known contact")
	})

	t.Run("UnknownSenderIsIgnored", func(t *testing.T) {
		unknownEmail := fmt.Sprintf("stranger-%d@example.com", time.Now().UnixNano())

		form := url.Values{}
		form.Set("sender", unknownEmail)
		form.Set("subject", "Re: Welcome aboard")

		resp := postReply(t, form)
		// Still 200 so the provider does not retry.
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		events, err := factory.GetContactTimelineEvents(workspace.ID, unknownEmail, "email.replied")
		require.NoError(t, err)
		assert.Len(t, events, 0, "unknown senders must not produce a timeline entry")
	})
}
