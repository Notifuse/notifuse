//go:build integration

package integration

import (
	"context"
	"encoding/json"
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

// TestSendTimeSender sends real emails through Mailpit from a sender chosen on the broadcast
// variation or on the transactional call, rather than the template's, and checks that a sender
// the integration does not have is refused before anything is sent.
func TestSendTimeSender(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(context.Background()))

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))

	// The template sends from the default sender; "promos" is only ever chosen at send time.
	// domain.NewEmailSender marks every sender default, so the second is unmarked by hand.
	byDefault := domain.NewEmailSender("hello@notifuse.test", "Notifuse")
	promos := domain.NewEmailSender("promos@notifuse.test", "Notifuse Promos")
	promos.IsDefault = false
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind:               domain.EmailProviderKindSMTP,
			Senders:            []domain.EmailSender{byDefault, promos},
			SMTP:               &domain.SMTPSettings{Host: "localhost", Port: 1025, UseTLS: false},
			RateLimitPerMinute: 2000,
		}))
	require.NoError(t, err)

	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	newTemplate := func(t *testing.T, subject string) *domain.Template {
		tpl, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("Sender "+subject),
			testutil.WithTemplateSubject(subject),
			testutil.WithTemplateEmailContent("Hello"))
		require.NoError(t, err)
		return tpl
	}

	// newAudience creates a list holding the given number of fresh contacts.
	newAudience := func(t *testing.T, size int) (*domain.List, []string) {
		list, err := factory.CreateList(workspace.ID, testutil.WithListName("Sender list "+uuid.NewString()[:8]))
		require.NoError(t, err)
		emails := make([]string, 0, size)
		for i := 0; i < size; i++ {
			email := fmt.Sprintf("sender-%s@example.com", uuid.NewString()[:8])
			_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
			require.NoError(t, err)
			_, err = factory.CreateContactList(workspace.ID,
				testutil.WithContactListEmail(email),
				testutil.WithContactListListID(list.ID),
				testutil.WithContactListStatus(domain.ContactListStatusActive))
			require.NoError(t, err)
			emails = append(emails, email)
		}
		return list, emails
	}

	createBroadcast := func(t *testing.T, list *domain.List, testSettings map[string]interface{}) (*http.Response, string) {
		resp, err := client.CreateBroadcast(map[string]interface{}{
			"workspace_id":  workspace.ID,
			"name":          "Sender broadcast " + uuid.NewString()[:8],
			"audience":      map[string]interface{}{"list": list.ID, "exclude_unsubscribed": true},
			"test_settings": testSettings,
		})
		require.NoError(t, err)
		var body struct {
			Broadcast domain.Broadcast `json:"broadcast"`
		}
		if resp.StatusCode == http.StatusCreated {
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		}
		return resp, body.Broadcast.ID
	}

	sendNow := func(t *testing.T, broadcastID string) *http.Response {
		resp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcastID,
			"send_now":     true,
		})
		require.NoError(t, err)
		return resp
	}

	errorOf := func(t *testing.T, resp *http.Response) string {
		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		msg, _ := body["error"].(string)
		return msg
	}

	t.Run("a broadcast leaves from its variation's sender", func(t *testing.T) {
		require.NoError(t, testutil.ClearMailpitMessages(t))
		tpl := newTemplate(t, "Variation sender "+uuid.NewString()[:8])
		list, emails := newAudience(t, 1)

		resp, broadcastID := createBroadcast(t, list, map[string]interface{}{
			"enabled":    false,
			"variations": []map[string]interface{}{{"variation_name": "A", "template_id": tpl.ID, "sender_id": promos.ID}},
		})
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		resp = sendNow(t, broadcastID)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_, err := testutil.WaitForBroadcastStatusWithExecution(t, client, broadcastID, []string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		msg, err := testutil.WaitForMailpitMessageByRecipient(t, emails[0], 15*time.Second)
		require.NoError(t, err)
		assert.Equal(t, "promos@notifuse.test", msg.From.Address)
		assert.Equal(t, "Notifuse Promos", msg.From.Name)
	})

	t.Run("each A/B variation leaves from its own sender", func(t *testing.T) {
		require.NoError(t, testutil.ClearMailpitMessages(t))
		suffix := uuid.NewString()[:8]
		keeps := newTemplate(t, "Keeps the template's sender "+suffix)
		chooses := newTemplate(t, "Chooses promos "+suffix)
		// Recipients get a variation at random; with this many, both are all but certain to go out.
		list, emails := newAudience(t, 20)

		resp, broadcastID := createBroadcast(t, list, map[string]interface{}{
			"enabled":           true,
			"sample_percentage": 100,
			"auto_send_winner":  false,
			"variations": []map[string]interface{}{
				{"variation_name": "A", "template_id": keeps.ID},
				{"variation_name": "B", "template_id": chooses.ID, "sender_id": promos.ID},
			},
		})
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		resp = sendNow(t, broadcastID)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_, err := testutil.WaitForBroadcastStatusWithExecution(t, client, broadcastID,
			[]string{"test_completed", "winner_selected", "processed", "completed"}, 90*time.Second)
		require.NoError(t, err)

		fromByVariation := map[string]map[string]bool{}
		for _, email := range emails {
			msg, err := testutil.WaitForMailpitMessageByRecipient(t, email, 15*time.Second)
			require.NoError(t, err)
			if fromByVariation[msg.Subject] == nil {
				fromByVariation[msg.Subject] = map[string]bool{}
			}
			fromByVariation[msg.Subject][msg.From.Address] = true
		}
		for subject, froms := range fromByVariation {
			if subject == chooses.Email.Subject {
				assert.Equal(t, map[string]bool{"promos@notifuse.test": true}, froms, subject)
			} else {
				assert.Equal(t, map[string]bool{"hello@notifuse.test": true}, froms, subject)
			}
		}
		require.Contains(t, fromByVariation, chooses.Email.Subject, "the variation choosing a sender went out")
		require.Contains(t, fromByVariation, keeps.Email.Subject, "the variation keeping its template's sender went out")
	})

	t.Run("a sender the integration does not have is refused on create", func(t *testing.T) {
		tpl := newTemplate(t, "Unknown sender "+uuid.NewString()[:8])
		list, _ := newAudience(t, 1)

		resp, _ := createBroadcast(t, list, map[string]interface{}{
			"enabled":    false,
			"variations": []map[string]interface{}{{"variation_name": "A", "template_id": tpl.ID, "sender_id": "no-such-sender"}},
		})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Contains(t, errorOf(t, resp), "no-such-sender")
	})

	t.Run("a broadcast whose sender has left the integration is not scheduled", func(t *testing.T) {
		require.NoError(t, testutil.ClearMailpitMessages(t))
		tpl := newTemplate(t, "Sender since removed "+uuid.NewString()[:8])
		list, emails := newAudience(t, 1)
		// Stored directly, as it would be after the sender was removed from the integration.
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastAudience(domain.AudienceSettings{List: list.ID, ExcludeUnsubscribed: true}),
			func(b *domain.Broadcast) {
				b.TestSettings = domain.BroadcastTestSettings{Variations: []domain.BroadcastVariation{
					{VariationName: "A", TemplateID: tpl.ID, SenderID: "sender-since-removed"},
				}}
			})
		require.NoError(t, err)

		resp := sendNow(t, broadcast.ID)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Contains(t, errorOf(t, resp), "sender-since-removed")

		_, err = testutil.WaitForMailpitMessageByRecipient(t, emails[0], 3*time.Second)
		assert.Error(t, err, "nothing is sent")
	})

	notification, err := factory.CreateTransactionalNotification(workspace.ID,
		testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
			domain.TransactionalChannelEmail: domain.ChannelTemplate{
				TemplateID: newTemplate(t, "Transactional sender "+uuid.NewString()[:8]).ID,
				Settings:   map[string]interface{}{},
			},
		}))
	require.NoError(t, err)

	t.Run("a transactional email leaves from the sender the call chooses", func(t *testing.T) {
		require.NoError(t, testutil.ClearMailpitMessages(t))
		recipient := fmt.Sprintf("tx-sender-%s@example.com", uuid.NewString()[:8])

		resp, err := client.SendTransactionalNotification(map[string]interface{}{
			"id":            notification.ID,
			"contact":       map[string]interface{}{"email": recipient},
			"channels":      []string{"email"},
			"email_options": map[string]interface{}{"sender_id": promos.ID},
		})
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		msg, err := testutil.WaitForMailpitMessageByRecipient(t, recipient, 15*time.Second)
		require.NoError(t, err)
		assert.Equal(t, "promos@notifuse.test", msg.From.Address)
	})

	t.Run("an unknown transactional sender is a 400 and creates no contact", func(t *testing.T) {
		recipient := fmt.Sprintf("tx-unknown-%s@example.com", uuid.NewString()[:8])

		resp, err := client.SendTransactionalNotification(map[string]interface{}{
			"id":            notification.ID,
			"contact":       map[string]interface{}{"email": recipient},
			"channels":      []string{"email"},
			"email_options": map[string]interface{}{"sender_id": "no-such-sender"},
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Equal(t, "sender not found: no-such-sender", errorOf(t, resp))

		contactResp, err := client.GetContactByEmail(recipient)
		require.NoError(t, err)
		defer contactResp.Body.Close()
		assert.NotEqual(t, http.StatusOK, contactResp.StatusCode, "the contact was not created")
	})
}
