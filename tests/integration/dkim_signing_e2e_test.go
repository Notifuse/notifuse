package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	msgdkim "github.com/emersion/go-msgauth/dkim"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// dnsZone is the DNS the app sees: a test publishes a key's record by writing it here.
type dnsZone struct {
	mu      sync.Mutex
	records map[string][]string
}

func (z *dnsZone) publish(name, value string) {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.records[strings.TrimSuffix(name, ".")] = []string{value}
}

func (z *dnsZone) LookupTXT(_ context.Context, name string) ([]string, error) {
	return z.lookup(name)
}

func (z *dnsZone) lookup(name string) ([]string, error) {
	z.mu.Lock()
	defer z.mu.Unlock()
	records, ok := z.records[strings.TrimSuffix(name, ".")]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
	return records, nil
}

func decodeBody(t *testing.T, resp *http.Response, into interface{}) {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, into), string(body))
}

// TestDKIMSigningE2E creates a domain's key through the API, publishes its record,
// verifies it, and checks the DKIM signature of what Mailpit receives from a
// transactional send and a broadcast.
func TestDKIMSigningE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	zone := &dnsZone{records: map[string][]string{}}
	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg, app.WithDKIMTXTResolver(zone))
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID, testutil.WithIntegrationEmailProvider(domain.EmailProvider{
		Kind:               domain.EmailProviderKindSMTP,
		Senders:            []domain.EmailSender{domain.NewEmailSender("noreply@notifuse.test", "Notifuse DKIM Test")},
		SMTP:               &domain.SMTPSettings{Host: "localhost", Port: 1025},
		RateLimitPerMinute: 2000,
	}))
	require.NoError(t, err)

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(workerCtx))

	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	// A copy of the settings read before any key exists, as a settings save holds it.
	staleCopy, err := factory.GetWorkspace(workspace.ID)
	require.NoError(t, err)

	// The request bodies are written out, as an API client sends them.
	resp, err := client.Post("/api/workspaces.createDKIMDomain", json.RawMessage(fmt.Sprintf(`{"workspace_id":%q,"domain":"Notifuse.TEST"}`, workspace.ID)))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created struct {
		Domain domain.DKIMDomainView `json:"domain"`
	}
	decodeBody(t, resp, &created)
	assert.Equal(t, "notifuse.test", created.Domain.Domain)
	assert.Equal(t, domain.DKIMKeyStatusPending, created.Domain.Status)
	record := created.Domain.DNSRecord

	t.Run("a settings save from an older copy keeps the key", func(t *testing.T) {
		staleCopy.Settings.Timezone = "Europe/Paris"
		require.NoError(t, factory.SaveWorkspace(staleCopy))
		stored, err := factory.GetWorkspace(workspace.ID)
		require.NoError(t, err)
		require.Len(t, stored.Settings.DKIMKeys, 1)
		assert.Equal(t, "Europe/Paris", stored.Settings.Timezone)
	})

	t.Run("the workspace and the list never carry a private key", func(t *testing.T) {
		for _, path := range []string{"/api/workspaces.get", "/api/workspaces.listDKIMDomains"} {
			resp, err := client.Get(path, map[string]string{"workspace_id": workspace.ID, "id": workspace.ID})
			require.NoError(t, err)
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
			assert.NotContains(t, string(body), "private_key", path)
			assert.NotContains(t, string(body), "PRIVATE KEY", path)
		}
	})

	sendTransactional := func(t *testing.T) string {
		t.Helper()
		require.NoError(t, testutil.ClearMailpitMessages(t))
		uid := uuid.New().String()[:8]
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("DKIM "+uid),
			testutil.WithTemplateSubject("Événement d'été "+uid),
			testutil.WithTemplateEmailContent("Bonjour Zoé, à bientôt. "+uid))
		require.NoError(t, err)
		notification, err := factory.CreateTransactionalNotification(workspace.ID,
			testutil.WithTransactionalNotificationID("dkim-"+uid),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{TemplateID: template.ID, Settings: map[string]interface{}{}},
			}))
		require.NoError(t, err)
		to := fmt.Sprintf("dkim-%s@example.com", uid)
		resp, err := client.SendTransactionalNotification(map[string]interface{}{
			"id":       notification.ID,
			"contact":  map[string]interface{}{"email": to},
			"channels": []string{"email"},
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()
		msg, err := testutil.WaitForMailpitMessageByRecipient(t, to, 15*time.Second)
		require.NoError(t, err)
		raw, err := testutil.GetMailpitMessageRaw(t, msg.ID)
		require.NoError(t, err)
		return raw
	}

	verifyRaw := func(t *testing.T, raw string) *msgdkim.Verification {
		t.Helper()
		verifications, err := msgdkim.VerifyWithOptions(bytes.NewReader([]byte(raw)), &msgdkim.VerifyOptions{LookupTXT: zone.lookup})
		require.NoError(t, err)
		require.Len(t, verifications, 1)
		return verifications[0]
	}

	t.Run("a pending key does not sign", func(t *testing.T) {
		raw := sendTransactional(t)
		assert.NotContains(t, raw, "DKIM-Signature")
	})

	t.Run("a verified key signs transactional emails", func(t *testing.T) {
		zone.publish(record.Name, record.Value)
		resp, err := client.Post("/api/workspaces.verifyDKIMDomain", json.RawMessage(fmt.Sprintf(`{"workspace_id":%q,"domain":"notifuse.test"}`, workspace.ID)))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var verified domain.DKIMVerifyResult
		decodeBody(t, resp, &verified)
		require.Equal(t, domain.DKIMCheckResultOK, verified.Check.Result)
		require.Equal(t, domain.DKIMKeyStatusPublished, verified.Domain.Status)

		raw := sendTransactional(t)
		v := verifyRaw(t, raw)
		assert.NoError(t, v.Err)
		assert.Equal(t, "notifuse.test", v.Domain)
		for i := 0; i < len(raw); i++ {
			require.Less(t, raw[i], byte(0x80), "byte %d of the received message is not 7-bit", i)
		}
	})

	t.Run("a broadcast through the queue is signed, its unsubscribe headers included", func(t *testing.T) {
		require.NoError(t, testutil.ClearMailpitMessages(t))
		list, err := factory.CreateList(workspace.ID, testutil.WithListName("DKIM list"))
		require.NoError(t, err)
		to := fmt.Sprintf("dkim-broadcast-%s@example.com", uuid.New().String()[:8])
		_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(to))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(to),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("DKIM broadcast"),
			testutil.WithTemplateSubject("DKIM broadcast"),
			testutil.WithTemplateEmailContent("Bonjour, à bientôt."))
		require.NoError(t, err)
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("DKIM broadcast"),
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastAudience(domain.AudienceSettings{List: list.ID, ExcludeUnsubscribed: true}))
		require.NoError(t, err)

		resp, err := client.ScheduleBroadcast(map[string]interface{}{"workspace_id": workspace.ID, "id": broadcast.ID, "send_now": true})
		require.NoError(t, err)
		scheduleBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, string(scheduleBody))
		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID, []string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		msg, err := testutil.WaitForMailpitMessageByRecipient(t, to, 15*time.Second)
		require.NoError(t, err)
		raw, err := testutil.GetMailpitMessageRaw(t, msg.ID)
		require.NoError(t, err)
		v := verifyRaw(t, raw)
		assert.NoError(t, v.Err)
		require.Contains(t, raw, "List-Unsubscribe:")
		signed := strings.ToLower(strings.Join(v.HeaderKeys, ":"))
		assert.Contains(t, signed, "list-unsubscribe:list-unsubscribe:list-unsubscribe-post:list-unsubscribe-post")
	})

	t.Run("a domain a sender uses cannot be deleted", func(t *testing.T) {
		resp, err := client.Post("/api/workspaces.deleteDKIMDomain", json.RawMessage(fmt.Sprintf(`{"workspace_id":%q,"domain":"notifuse.test"}`, workspace.ID)))
		require.NoError(t, err)
		var refusal struct {
			Error  string                  `json:"error"`
			UsedBy []domain.DKIMDomainUser `json:"used_by"`
		}
		require.Equal(t, http.StatusConflict, resp.StatusCode)
		decodeBody(t, resp, &refusal)
		require.Len(t, refusal.UsedBy, 1)
	})

	t.Run("a save from a copy holding a deleted key does not bring it back", func(t *testing.T) {
		resp, err := client.Post("/api/workspaces.createDKIMDomain", json.RawMessage(fmt.Sprintf(`{"workspace_id":%q,"domain":"spare.test"}`, workspace.ID)))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()

		copyWithBoth, err := factory.GetWorkspace(workspace.ID)
		require.NoError(t, err)
		require.Len(t, copyWithBoth.Settings.DKIMKeys, 2)

		resp, err = client.Post("/api/workspaces.deleteDKIMDomain", json.RawMessage(fmt.Sprintf(`{"workspace_id":%q,"domain":"spare.test"}`, workspace.ID)))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()

		require.NoError(t, factory.SaveWorkspace(copyWithBoth))
		stored, err := factory.GetWorkspace(workspace.ID)
		require.NoError(t, err)
		require.Len(t, stored.Settings.DKIMKeys, 1)
		assert.Equal(t, "notifuse.test", stored.Settings.DKIMKeys[0].Domain)
	})
}
