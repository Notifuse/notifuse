package integration

import (
	"encoding/json"
	"fmt"
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

// A 1x1 transparent PNG, base64 encoded — stands in for the per-recipient QR
// code described in issue #393.
const inlinePNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

// TestTransactionalInlineCIDMailpitE2E is the end-to-end proof for issue #393:
// a transactional email carrying an inline attachment with an explicit
// content_id is sent through the SMTP provider to Mailpit, and the delivered
// message is inspected to confirm the Content-ID round-trips onto the MIME
// part, the part is inline (not a dangling attachment), and the whole thing is
// assembled as multipart/related so clients embed it.
func TestTransactionalInlineCIDMailpitE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))

	// Default SMTP provider points at Mailpit (localhost:1025).
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	// The template body references the inline image by its content_id.
	mjmlSource := `<mjml><mj-body><mj-section><mj-column>
		<mj-text>Your check-in QR: <img src="cid:checkInQr" alt="QR"/></mj-text>
	</mj-column></mj-section></mj-body></mjml>`

	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Inline CID E2E"),
		testutil.WithTemplateSubject(fmt.Sprintf("Inline CID e2e %s", uuid.New().String()[:8])),
		testutil.WithCodeModeTemplate(mjmlSource))
	require.NoError(t, err)

	notification, err := factory.CreateTransactionalNotification(workspace.ID,
		testutil.WithTransactionalNotificationID("inline-cid-e2e"),
		testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
			domain.TransactionalChannelEmail: domain.ChannelTemplate{
				TemplateID: template.ID,
				Settings:   map[string]interface{}{},
			},
		}),
	)
	require.NoError(t, err)

	t.Run("explicit content_id round-trips onto the inline MIME part", func(t *testing.T) {
		require.NoError(t, testutil.ClearMailpitMessages(t))

		recipient := fmt.Sprintf("cid-%s@example.com", uuid.New().String()[:8])
		sendRequest := map[string]interface{}{
			"id": notification.ID,
			"contact": map[string]interface{}{
				"email": recipient,
			},
			"channels": []string{"email"},
			"email_options": map[string]interface{}{
				"attachments": []map[string]interface{}{
					{
						"filename":     "check-in-qr.png",
						"content":      inlinePNGBase64,
						"content_type": "image/png",
						"disposition":  "inline",
						"content_id":   "checkInQr",
					},
				},
			},
		}

		resp, err := client.SendTransactionalNotification(sendRequest)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode, "inline attachment with content_id must be accepted")

		email, err := testutil.WaitForMailpitMessageByRecipient(t, recipient, 15*time.Second)
		require.NoError(t, err, "the email must be delivered to Mailpit")

		// The image must be delivered as an inline part carrying the caller's
		// content_id — not as a regular (dangling) attachment.
		require.Len(t, email.Inline, 1, "exactly one inline part expected")
		require.Empty(t, email.Attachments, "the inline image must not appear as a regular attachment")

		inline := email.Inline[0]
		assert.Equal(t, "checkInQr", inline.ContentID,
			"the Content-ID must be the caller-provided content_id, not the filename")
		assert.Equal(t, "check-in-qr.png", inline.FileName, "the filename must be preserved independently of the content_id")
		assert.Contains(t, inline.ContentType, "image/png")

		// The message must be assembled as multipart/related so clients embed the
		// cid: reference instead of showing it as an attachment.
		raw, err := testutil.GetMailpitMessageRaw(t, email.ID)
		require.NoError(t, err)
		assert.Contains(t, raw, "multipart/related",
			"inline images must live under a multipart/related subtree")

		// The compiled HTML body keeps the cid: reference the template author wrote.
		assert.Contains(t, email.HTML, "cid:checkInQr",
			"the delivered HTML must reference the image by its content_id")

		t.Logf("✅ inline part delivered with Content-ID %q under multipart/related", inline.ContentID)
	})

	t.Run("inline attachment without content_id falls back to the filename", func(t *testing.T) {
		require.NoError(t, testutil.ClearMailpitMessages(t))

		recipient := fmt.Sprintf("cidfallback-%s@example.com", uuid.New().String()[:8])
		sendRequest := map[string]interface{}{
			"id": notification.ID,
			"contact": map[string]interface{}{
				"email": recipient,
			},
			"channels": []string{"email"},
			"email_options": map[string]interface{}{
				"attachments": []map[string]interface{}{
					{
						"filename":     "fallback-qr.png",
						"content":      inlinePNGBase64,
						"content_type": "image/png",
						"disposition":  "inline",
						// no content_id
					},
				},
			},
		}

		resp, err := client.SendTransactionalNotification(sendRequest)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		email, err := testutil.WaitForMailpitMessageByRecipient(t, recipient, 15*time.Second)
		require.NoError(t, err)

		require.Len(t, email.Inline, 1)
		assert.Equal(t, "fallback-qr.png", email.Inline[0].ContentID,
			"without an explicit content_id the Content-ID falls back to the filename")
	})

	t.Run("content_id on a non-inline attachment is rejected", func(t *testing.T) {
		recipient := fmt.Sprintf("cidreject-%s@example.com", uuid.New().String()[:8])
		sendRequest := map[string]interface{}{
			"id": notification.ID,
			"contact": map[string]interface{}{
				"email": recipient,
			},
			"channels": []string{"email"},
			"email_options": map[string]interface{}{
				"attachments": []map[string]interface{}{
					{
						"filename":     "doc.pdf",
						"content":      inlinePNGBase64,
						"content_type": "application/pdf",
						"disposition":  "attachment",
						"content_id":   "shouldFail",
					},
				},
			},
		}

		resp, err := client.SendTransactionalNotification(sendRequest)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"content_id is only valid for inline attachments")

		var result map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		errMsg, _ := result["error"].(string)
		assert.True(t, strings.Contains(errMsg, "content_id"),
			"error should mention content_id, got: %s", errMsg)
	})
}
