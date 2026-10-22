package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	svix "github.com/standard-webhooks/standard-webhooks/libraries/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// padJSON pads a JSON document with trailing whitespace to exactly size bytes,
// so the request has that size while the parsed content stays small.
func padJSON(t *testing.T, doc []byte, size int) []byte {
	t.Helper()
	require.LessOrEqual(t, len(doc), size)
	return append(doc, bytes.Repeat([]byte(" "), size-len(doc))...)
}

// postWebhookBody posts body to the webhook path, declaring its length or, when
// chunked is set, sending it without a Content-Length.
func postWebhookBody(t *testing.T, baseURL, path string, body []byte, chunked bool, headers map[string]string) (int, string) {
	t.Helper()
	var reader io.Reader = bytes.NewReader(body)
	if chunked {
		reader = io.MultiReader(reader) // hides the length from the client
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+path, reader)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if chunked {
		req.ContentLength = -1
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestWebhookRequestLimits(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	factory := suite.DataFactory
	baseURL := suite.ServerManager.GetURL()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	t.Run("email webhook", func(t *testing.T) {
		sendGrid, err := factory.CreateIntegration(workspace.ID,
			testutil.WithIntegrationName("Test SendGrid Integration"),
			testutil.WithIntegrationEmailProvider(domain.EmailProvider{
				Kind:     domain.EmailProviderKindSendGrid,
				Senders:  []domain.EmailSender{domain.NewEmailSender("test@example.com", "Test Sender")},
				SendGrid: &domain.SendGridSettings{APIKey: "test-api-key"},
			}))
		require.NoError(t, err)
		path := fmt.Sprintf("/webhooks/email?provider=sendgrid&workspace_id=%s&integration_id=%s", workspace.ID, sendGrid.ID)

		template, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)
		newMessage := func(t *testing.T) *domain.MessageHistory {
			t.Helper()
			email := fmt.Sprintf("limits-%s@example.com", uuid.New().String()[:8])
			_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
			require.NoError(t, err)
			message, err := factory.CreateMessageHistory(workspace.ID,
				testutil.WithMessageHistoryContactEmail(email),
				testutil.WithMessageTemplate(template.ID))
			require.NoError(t, err)
			return message
		}
		delivered := func(m *domain.MessageHistory) map[string]interface{} {
			return map[string]interface{}{
				"email":               m.ContactEmail,
				"timestamp":           time.Now().Unix(),
				"event":               "delivered",
				"sg_event_id":         uuid.New().String(),
				"sg_message_id":       uuid.New().String(),
				"notifuse_message_id": m.ID,
			}
		}
		deliveredAt := func(t *testing.T, m *domain.MessageHistory) sql.NullTime {
			t.Helper()
			var at sql.NullTime
			require.NoError(t, db.QueryRowContext(context.Background(),
				`SELECT delivered_at FROM message_history WHERE id = $1`, m.ID).Scan(&at))
			return at
		}

		t.Run("a body of exactly 10 MiB is processed", func(t *testing.T) {
			message := newMessage(t)
			batch, err := json.Marshal([]map[string]interface{}{delivered(message)})
			require.NoError(t, err)

			status, body := postWebhookBody(t, baseURL, path, padJSON(t, batch, 10<<20), false, nil)

			require.Equal(t, http.StatusOK, status, body)
			assert.True(t, deliveredAt(t, message).Valid)
		})

		for name, chunked := range map[string]bool{"with Content-Length": false, "chunked": true} {
			t.Run("a body over 10 MiB is refused, "+name, func(t *testing.T) {
				message := newMessage(t)
				batch, err := json.Marshal([]map[string]interface{}{delivered(message)})
				require.NoError(t, err)

				status, body := postWebhookBody(t, baseURL, path, padJSON(t, batch, (10<<20)+1), chunked, nil)

				assert.Equal(t, http.StatusRequestEntityTooLarge, status, body)
				assert.False(t, deliveredAt(t, message).Valid)
			})
		}

		t.Run("each event of a batch stores its own payload", func(t *testing.T) {
			first, second := newMessage(t), newMessage(t)
			batch, err := json.Marshal([]map[string]interface{}{delivered(first), delivered(second)})
			require.NoError(t, err)

			status, body := postWebhookBody(t, baseURL, path, batch, false, nil)
			require.Equal(t, http.StatusOK, status, body)

			for _, pair := range [][2]*domain.MessageHistory{{first, second}, {second, first}} {
				var raw string
				require.NoError(t, db.QueryRowContext(context.Background(),
					`SELECT raw_payload FROM inbound_webhook_events WHERE message_id = $1`, pair[0].ID).Scan(&raw))
				assert.Contains(t, raw, pair[0].ContactEmail)
				assert.NotContains(t, raw, pair[1].ContactEmail)
			}
		})
	})

	t.Run("Supabase hooks", func(t *testing.T) {
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		signatureKey := generateWebhookSecret()
		integrationID := setupSupabaseIntegrationWithUserCreatedHook(t, suite, workspace.ID, signatureKey, list.ID)
		userCreatedPath := fmt.Sprintf("/webhooks/supabase/before-user-created?workspace_id=%s&integration_id=%s", workspace.ID, integrationID)
		authEmailPath := fmt.Sprintf("/webhooks/supabase/auth-email?workspace_id=%s&integration_id=%s", workspace.ID, integrationID)

		signedUserCreated := func(t *testing.T, email string, size int) ([]byte, map[string]string) {
			t.Helper()
			payload, err := json.Marshal(domain.SupabaseBeforeUserCreatedWebhook{
				Metadata: domain.SupabaseWebhookMetadata{
					UUID:      uuid.New().String(),
					Time:      time.Now().UTC().Format(time.RFC3339),
					Name:      "before-user-created",
					IPAddress: "192.168.1.100",
				},
				User: domain.SupabaseUser{
					ID:           uuid.New().String(),
					Aud:          "authenticated",
					Role:         "authenticated",
					Email:        email,
					Phone:        "+15551234567",
					AppMetadata:  map[string]interface{}{"provider": "email"},
					UserMetadata: map[string]interface{}{},
					Identities:   []interface{}{},
					CreatedAt:    time.Now().UTC().Format(time.RFC3339),
					UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
				},
			})
			require.NoError(t, err)
			payload = padJSON(t, payload, size)

			webhookID := "webhook_" + uuid.New().String()
			timestamp := time.Now()
			wh, err := svix.NewWebhook(signatureKey)
			require.NoError(t, err)
			signature, err := wh.Sign(webhookID, timestamp, payload)
			require.NoError(t, err)
			return payload, map[string]string{
				"webhook-id":        webhookID,
				"webhook-timestamp": strconv.FormatInt(timestamp.Unix(), 10),
				"webhook-signature": signature,
			}
		}
		contactExists := func(t *testing.T, email string) bool {
			t.Helper()
			systemCtx := context.WithValue(context.Background(), domain.SystemCallKey, true)
			_, err := suite.ServerManager.GetApp().GetContactRepository().GetContactByEmail(systemCtx, workspace.ID, email)
			return err == nil
		}

		t.Run("a signed user of exactly 1 MiB is processed", func(t *testing.T) {
			email := fmt.Sprintf("supabase-limit-%s@example.com", uuid.New().String()[:8])
			payload, headers := signedUserCreated(t, email, 1<<20)

			status, body := postWebhookBody(t, baseURL, userCreatedPath, payload, false, headers)

			require.Equal(t, http.StatusNoContent, status, body)
			assert.True(t, contactExists(t, email))
		})

		for name, chunked := range map[string]bool{"with Content-Length": false, "chunked": true} {
			t.Run("a signed user over 1 MiB is refused, "+name, func(t *testing.T) {
				email := fmt.Sprintf("supabase-over-%s@example.com", uuid.New().String()[:8])
				payload, headers := signedUserCreated(t, email, (1<<20)+1)

				status, body := postWebhookBody(t, baseURL, userCreatedPath, payload, chunked, headers)

				assert.Equal(t, http.StatusRequestEntityTooLarge, status, body)
				assert.False(t, contactExists(t, email))
			})
		}

		t.Run("an auth email over 1 MiB is refused", func(t *testing.T) {
			status, body := postWebhookBody(t, baseURL, authEmailPath, bytes.Repeat([]byte(" "), (1<<20)+1), false, map[string]string{
				"webhook-id":        "webhook_" + uuid.New().String(),
				"webhook-timestamp": strconv.FormatInt(time.Now().Unix(), 10),
				"webhook-signature": "v1,signature",
			})

			assert.Equal(t, http.StatusRequestEntityTooLarge, status, body)
		})
	})
}
