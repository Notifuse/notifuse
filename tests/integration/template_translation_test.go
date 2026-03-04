package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeEmailTemplate creates an EmailTemplate with the given subject and body content.
// Used to build translation entries with unique, verifiable content.
func makeEmailTemplate(subject, bodyContent string) *domain.EmailTemplate {
	return &domain.EmailTemplate{
		Subject:          subject,
		CompiledPreview:  fmt.Sprintf(`<mjml><mj-head></mj-head><mj-body><mj-section><mj-column><mj-text>%s</mj-text></mj-column></mj-section></mj-body></mjml>`, bodyContent),
		VisualEditorTree: testutil.CreateMJMLBlockWithContent(bodyContent),
	}
}

// TestTemplateTranslationIntegration tests that template translations are correctly resolved
// across all three sending flows: transactional, broadcast, and automation.
// Each subtest verifies a specific branch of the ResolveEmailContent fallback chain:
//  1. translations nil / contact language empty → default template
//  2. contact language == workspace default language → default template
//  3. translation exists for contact language → that translation
//  4. no translation for contact language → default template
func TestTemplateTranslationIntegration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	// Start background workers (needed for automation scheduler)
	ctx := context.Background()
	err := suite.ServerManager.StartBackgroundWorkers(ctx)
	require.NoError(t, err)

	client := suite.APIClient
	factory := suite.DataFactory

	// Create user and workspace with language settings
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace(
		testutil.WithWorkspaceDefaultLanguage("en", []string{"en", "fr", "es"}),
	)
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Setup SMTP provider (does GetByID→Update which preserves language settings)
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	// Login and set workspace
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// ========================================================================
	// Transactional Tests (4 subtests)
	// ========================================================================

	t.Run("Transactional", func(t *testing.T) {
		t.Run("sends translated variant when contact language matches", func(t *testing.T) {
			err := testutil.ClearMailpitMessages(t)
			require.NoError(t, err)

			uid := uuid.New().String()[:8]
			enSubject := fmt.Sprintf("Welcome EN - %s", uid)
			frSubject := fmt.Sprintf("Bienvenue FR - %s", uid)
			enBody := fmt.Sprintf("Hello in English - %s", uid)
			frBody := fmt.Sprintf("Bonjour en Francais - %s", uid)

			template, err := factory.CreateTemplate(workspace.ID,
				testutil.WithTemplateName("Translation Test - FR Match"),
				testutil.WithTemplateSubject(enSubject),
				testutil.WithTemplateEmailContent(enBody),
				testutil.WithTemplateTranslations(map[string]domain.TemplateTranslation{
					"fr": {Email: makeEmailTemplate(frSubject, frBody)},
				}),
			)
			require.NoError(t, err)

			notifID := fmt.Sprintf("trans-fr-match-%s", uid)
			notification, err := factory.CreateTransactionalNotification(workspace.ID,
				testutil.WithTransactionalNotificationID(notifID),
				testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
					domain.TransactionalChannelEmail: domain.ChannelTemplate{
						TemplateID: template.ID,
						Settings:   map[string]interface{}{},
					},
				}),
			)
			require.NoError(t, err)

			contactEmail := fmt.Sprintf("trans-fr-%s@example.com", uid)
			_, err = factory.CreateContact(workspace.ID,
				testutil.WithContactEmail(contactEmail),
				testutil.WithContactLanguage("fr"),
			)
			require.NoError(t, err)

			resp, err := client.SendTransactionalNotification(map[string]interface{}{
				"id": notification.ID,
				"contact": map[string]interface{}{
					"email": contactEmail,
				},
				"channels": []string{"email"},
			})
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode, "Send should succeed")
			resp.Body.Close()

			msg, err := testutil.WaitForMailpitMessageByRecipient(t, contactEmail, 15*time.Second)
			require.NoError(t, err, "Should receive email in Mailpit")

			t.Logf("Received email subject: %s", msg.Subject)
			assert.Contains(t, msg.Subject, frSubject, "Subject should be FR translation")
			assert.NotContains(t, msg.Subject, "Welcome EN", "Subject should NOT be default EN")
			assert.Contains(t, msg.HTML, frBody, "Body should contain FR translation content")
			assert.NotContains(t, msg.HTML, enBody, "Body should NOT contain default EN content")
		})

		t.Run("falls back to default when no translation exists", func(t *testing.T) {
			err := testutil.ClearMailpitMessages(t)
			require.NoError(t, err)

			uid := uuid.New().String()[:8]
			enSubject := fmt.Sprintf("Welcome EN - %s", uid)
			frSubject := fmt.Sprintf("Bienvenue FR - %s", uid)
			enBody := fmt.Sprintf("Hello in English - %s", uid)
			frBody := fmt.Sprintf("Bonjour en Francais - %s", uid)

			template, err := factory.CreateTemplate(workspace.ID,
				testutil.WithTemplateName("Translation Test - DE Fallback"),
				testutil.WithTemplateSubject(enSubject),
				testutil.WithTemplateEmailContent(enBody),
				testutil.WithTemplateTranslations(map[string]domain.TemplateTranslation{
					"fr": {Email: makeEmailTemplate(frSubject, frBody)},
				}),
			)
			require.NoError(t, err)

			notifID := fmt.Sprintf("trans-de-fall-%s", uid)
			notification, err := factory.CreateTransactionalNotification(workspace.ID,
				testutil.WithTransactionalNotificationID(notifID),
				testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
					domain.TransactionalChannelEmail: domain.ChannelTemplate{
						TemplateID: template.ID,
						Settings:   map[string]interface{}{},
					},
				}),
			)
			require.NoError(t, err)

			contactEmail := fmt.Sprintf("trans-de-%s@example.com", uid)
			_, err = factory.CreateContact(workspace.ID,
				testutil.WithContactEmail(contactEmail),
				testutil.WithContactLanguage("de"),
			)
			require.NoError(t, err)

			resp, err := client.SendTransactionalNotification(map[string]interface{}{
				"id": notification.ID,
				"contact": map[string]interface{}{
					"email": contactEmail,
				},
				"channels": []string{"email"},
			})
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			resp.Body.Close()

			msg, err := testutil.WaitForMailpitMessageByRecipient(t, contactEmail, 15*time.Second)
			require.NoError(t, err, "Should receive email in Mailpit")

			t.Logf("Received email subject: %s", msg.Subject)
			assert.Contains(t, msg.Subject, enSubject, "Subject should be default EN (no DE translation)")
			assert.NotContains(t, msg.Subject, "Bienvenue", "Subject should NOT be FR")
			assert.Contains(t, msg.HTML, enBody, "Body should contain default EN content")
			assert.NotContains(t, msg.HTML, frBody, "Body should NOT contain FR content")
		})

		t.Run("uses default when contact language equals workspace default", func(t *testing.T) {
			err := testutil.ClearMailpitMessages(t)
			require.NoError(t, err)

			uid := uuid.New().String()[:8]
			enSubject := fmt.Sprintf("Welcome EN - %s", uid)
			frSubject := fmt.Sprintf("Bienvenue FR - %s", uid)
			enBody := fmt.Sprintf("Hello in English - %s", uid)
			frBody := fmt.Sprintf("Bonjour en Francais - %s", uid)

			template, err := factory.CreateTemplate(workspace.ID,
				testutil.WithTemplateName("Translation Test - EN Match Default"),
				testutil.WithTemplateSubject(enSubject),
				testutil.WithTemplateEmailContent(enBody),
				testutil.WithTemplateTranslations(map[string]domain.TemplateTranslation{
					"fr": {Email: makeEmailTemplate(frSubject, frBody)},
				}),
			)
			require.NoError(t, err)

			notifID := fmt.Sprintf("trans-en-def-%s", uid)
			notification, err := factory.CreateTransactionalNotification(workspace.ID,
				testutil.WithTransactionalNotificationID(notifID),
				testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
					domain.TransactionalChannelEmail: domain.ChannelTemplate{
						TemplateID: template.ID,
						Settings:   map[string]interface{}{},
					},
				}),
			)
			require.NoError(t, err)

			// Contact language == workspace default language ("en")
			contactEmail := fmt.Sprintf("trans-en-def-%s@example.com", uid)
			_, err = factory.CreateContact(workspace.ID,
				testutil.WithContactEmail(contactEmail),
				testutil.WithContactLanguage("en"),
			)
			require.NoError(t, err)

			resp, err := client.SendTransactionalNotification(map[string]interface{}{
				"id": notification.ID,
				"contact": map[string]interface{}{
					"email": contactEmail,
				},
				"channels": []string{"email"},
			})
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			resp.Body.Close()

			msg, err := testutil.WaitForMailpitMessageByRecipient(t, contactEmail, 15*time.Second)
			require.NoError(t, err, "Should receive email in Mailpit")

			t.Logf("Received email subject: %s", msg.Subject)
			assert.Contains(t, msg.Subject, enSubject, "Subject should be default EN (contact lang == workspace default)")
			assert.NotContains(t, msg.Subject, "Bienvenue", "Subject should NOT be FR")
			assert.Contains(t, msg.HTML, enBody, "Body should contain default EN content")
		})

		t.Run("uses default when contact has no language", func(t *testing.T) {
			err := testutil.ClearMailpitMessages(t)
			require.NoError(t, err)

			uid := uuid.New().String()[:8]
			enSubject := fmt.Sprintf("Welcome EN - %s", uid)
			frSubject := fmt.Sprintf("Bienvenue FR - %s", uid)
			enBody := fmt.Sprintf("Hello in English - %s", uid)
			frBody := fmt.Sprintf("Bonjour en Francais - %s", uid)

			template, err := factory.CreateTemplate(workspace.ID,
				testutil.WithTemplateName("Translation Test - No Lang"),
				testutil.WithTemplateSubject(enSubject),
				testutil.WithTemplateEmailContent(enBody),
				testutil.WithTemplateTranslations(map[string]domain.TemplateTranslation{
					"fr": {Email: makeEmailTemplate(frSubject, frBody)},
				}),
			)
			require.NoError(t, err)

			notifID := fmt.Sprintf("trans-nolang-%s", uid)
			notification, err := factory.CreateTransactionalNotification(workspace.ID,
				testutil.WithTransactionalNotificationID(notifID),
				testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
					domain.TransactionalChannelEmail: domain.ChannelTemplate{
						TemplateID: template.ID,
						Settings:   map[string]interface{}{},
					},
				}),
			)
			require.NoError(t, err)

			// Contact with no language set (nil)
			contactEmail := fmt.Sprintf("trans-nolang-%s@example.com", uid)
			_, err = factory.CreateContact(workspace.ID,
				testutil.WithContactEmail(contactEmail),
				testutil.WithContactLanguageNil(),
			)
			require.NoError(t, err)

			resp, err := client.SendTransactionalNotification(map[string]interface{}{
				"id": notification.ID,
				"contact": map[string]interface{}{
					"email": contactEmail,
				},
				"channels": []string{"email"},
			})
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			resp.Body.Close()

			msg, err := testutil.WaitForMailpitMessageByRecipient(t, contactEmail, 15*time.Second)
			require.NoError(t, err, "Should receive email in Mailpit")

			t.Logf("Received email subject: %s", msg.Subject)
			assert.Contains(t, msg.Subject, enSubject, "Subject should be default EN (no contact language)")
			assert.NotContains(t, msg.Subject, "Bienvenue", "Subject should NOT be FR")
			assert.Contains(t, msg.HTML, enBody, "Body should contain default EN content")
			assert.NotContains(t, msg.HTML, frBody, "Body should NOT contain FR content")
		})
	})

	// ========================================================================
	// Broadcast Tests (2 subtests)
	// ========================================================================

	t.Run("Broadcast", func(t *testing.T) {
		t.Run("sends correct translation per contact language", func(t *testing.T) {
			err := testutil.ClearMailpitMessages(t)
			require.NoError(t, err)

			uid := uuid.New().String()[:8]
			enSubject := fmt.Sprintf("Welcome EN - %s", uid)
			frSubject := fmt.Sprintf("Bienvenue FR - %s", uid)
			esSubject := fmt.Sprintf("Bienvenida ES - %s", uid)
			enBody := fmt.Sprintf("Hello in English - %s", uid)
			frBody := fmt.Sprintf("Bonjour en Francais - %s", uid)
			esBody := fmt.Sprintf("Hola en Espanol - %s", uid)

			template, err := factory.CreateTemplate(workspace.ID,
				testutil.WithTemplateName("Broadcast Translation - Multi"),
				testutil.WithTemplateSubject(enSubject),
				testutil.WithTemplateEmailContent(enBody),
				testutil.WithTemplateTranslations(map[string]domain.TemplateTranslation{
					"fr": {Email: makeEmailTemplate(frSubject, frBody)},
					"es": {Email: makeEmailTemplate(esSubject, esBody)},
				}),
			)
			require.NoError(t, err)

			list, err := factory.CreateList(workspace.ID,
				testutil.WithListName(fmt.Sprintf("Translation List %s", uid)))
			require.NoError(t, err)

			// Create three contacts: EN (default), FR, ES
			enEmail := fmt.Sprintf("bc-en-%s@example.com", uid)
			frEmail := fmt.Sprintf("bc-fr-%s@example.com", uid)
			esEmail := fmt.Sprintf("bc-es-%s@example.com", uid)

			_, err = factory.CreateContact(workspace.ID,
				testutil.WithContactEmail(enEmail),
				testutil.WithContactLanguage("en"),
			)
			require.NoError(t, err)
			_, err = factory.CreateContact(workspace.ID,
				testutil.WithContactEmail(frEmail),
				testutil.WithContactLanguage("fr"),
			)
			require.NoError(t, err)
			_, err = factory.CreateContact(workspace.ID,
				testutil.WithContactEmail(esEmail),
				testutil.WithContactLanguage("es"),
			)
			require.NoError(t, err)

			// Add contacts to list
			for _, email := range []string{enEmail, frEmail, esEmail} {
				_, err = factory.CreateContactList(workspace.ID,
					testutil.WithContactListEmail(email),
					testutil.WithContactListListID(list.ID),
					testutil.WithContactListStatus(domain.ContactListStatusActive),
				)
				require.NoError(t, err)
			}

			// Create broadcast
			broadcast, err := factory.CreateBroadcast(workspace.ID,
				testutil.WithBroadcastName(fmt.Sprintf("Translation Broadcast %s", uid)),
				testutil.WithBroadcastAudience(domain.AudienceSettings{
					List:                list.ID,
					ExcludeUnsubscribed: true,
				}),
			)
			require.NoError(t, err)

			// Set template on broadcast
			broadcast.TestSettings.Variations[0].TemplateID = template.ID
			updateReq := map[string]interface{}{
				"workspace_id":  workspace.ID,
				"id":            broadcast.ID,
				"name":          broadcast.Name,
				"audience":      broadcast.Audience,
				"schedule":      broadcast.Schedule,
				"test_settings": broadcast.TestSettings,
			}
			updateResp, err := client.UpdateBroadcast(updateReq)
			require.NoError(t, err)
			updateResp.Body.Close()

			// Schedule to send now
			scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
				"workspace_id": workspace.ID,
				"id":           broadcast.ID,
				"send_now":     true,
			})
			require.NoError(t, err)
			scheduleResp.Body.Close()

			// Wait for completion
			_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
				[]string{"processed", "completed"}, 60*time.Second)
			require.NoError(t, err)

			// Verify EN contact gets default template
			enMsg, err := testutil.WaitForMailpitMessageByRecipient(t, enEmail, 15*time.Second)
			require.NoError(t, err, "EN contact should receive email")
			t.Logf("EN email subject: %s", enMsg.Subject)
			assert.Contains(t, enMsg.Subject, enSubject, "EN contact should get default EN subject")
			assert.Contains(t, enMsg.HTML, enBody, "EN contact should get default EN body")
			assert.NotContains(t, enMsg.HTML, frBody, "EN contact should NOT get FR body")
			assert.NotContains(t, enMsg.HTML, esBody, "EN contact should NOT get ES body")

			// Verify FR contact gets FR translation
			frMsg, err := testutil.WaitForMailpitMessageByRecipient(t, frEmail, 15*time.Second)
			require.NoError(t, err, "FR contact should receive email")
			t.Logf("FR email subject: %s", frMsg.Subject)
			assert.Contains(t, frMsg.Subject, frSubject, "FR contact should get FR subject")
			assert.Contains(t, frMsg.HTML, frBody, "FR contact should get FR body")
			assert.NotContains(t, frMsg.HTML, enBody, "FR contact should NOT get EN body")

			// Verify ES contact gets ES translation
			esMsg, err := testutil.WaitForMailpitMessageByRecipient(t, esEmail, 15*time.Second)
			require.NoError(t, err, "ES contact should receive email")
			t.Logf("ES email subject: %s", esMsg.Subject)
			assert.Contains(t, esMsg.Subject, esSubject, "ES contact should get ES subject")
			assert.Contains(t, esMsg.HTML, esBody, "ES contact should get ES body")
			assert.NotContains(t, esMsg.HTML, enBody, "ES contact should NOT get EN body")
		})

		t.Run("falls back for contacts without matching translation", func(t *testing.T) {
			err := testutil.ClearMailpitMessages(t)
			require.NoError(t, err)

			uid := uuid.New().String()[:8]
			enSubject := fmt.Sprintf("Welcome EN - %s", uid)
			frSubject := fmt.Sprintf("Bienvenue FR - %s", uid)
			enBody := fmt.Sprintf("Hello in English - %s", uid)
			frBody := fmt.Sprintf("Bonjour en Francais - %s", uid)

			template, err := factory.CreateTemplate(workspace.ID,
				testutil.WithTemplateName("Broadcast Translation - Fallback"),
				testutil.WithTemplateSubject(enSubject),
				testutil.WithTemplateEmailContent(enBody),
				testutil.WithTemplateTranslations(map[string]domain.TemplateTranslation{
					"fr": {Email: makeEmailTemplate(frSubject, frBody)},
				}),
			)
			require.NoError(t, err)

			list, err := factory.CreateList(workspace.ID,
				testutil.WithListName(fmt.Sprintf("Fallback List %s", uid)))
			require.NoError(t, err)

			// FR contact (has translation) and DE contact (no translation)
			frEmail := fmt.Sprintf("bc-fr-fb-%s@example.com", uid)
			deEmail := fmt.Sprintf("bc-de-fb-%s@example.com", uid)

			_, err = factory.CreateContact(workspace.ID,
				testutil.WithContactEmail(frEmail),
				testutil.WithContactLanguage("fr"),
			)
			require.NoError(t, err)
			_, err = factory.CreateContact(workspace.ID,
				testutil.WithContactEmail(deEmail),
				testutil.WithContactLanguage("de"),
			)
			require.NoError(t, err)

			for _, email := range []string{frEmail, deEmail} {
				_, err = factory.CreateContactList(workspace.ID,
					testutil.WithContactListEmail(email),
					testutil.WithContactListListID(list.ID),
					testutil.WithContactListStatus(domain.ContactListStatusActive),
				)
				require.NoError(t, err)
			}

			broadcast, err := factory.CreateBroadcast(workspace.ID,
				testutil.WithBroadcastName(fmt.Sprintf("Fallback Broadcast %s", uid)),
				testutil.WithBroadcastAudience(domain.AudienceSettings{
					List:                list.ID,
					ExcludeUnsubscribed: true,
				}),
			)
			require.NoError(t, err)

			broadcast.TestSettings.Variations[0].TemplateID = template.ID
			updateReq := map[string]interface{}{
				"workspace_id":  workspace.ID,
				"id":            broadcast.ID,
				"name":          broadcast.Name,
				"audience":      broadcast.Audience,
				"schedule":      broadcast.Schedule,
				"test_settings": broadcast.TestSettings,
			}
			updateResp, err := client.UpdateBroadcast(updateReq)
			require.NoError(t, err)
			updateResp.Body.Close()

			scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
				"workspace_id": workspace.ID,
				"id":           broadcast.ID,
				"send_now":     true,
			})
			require.NoError(t, err)
			scheduleResp.Body.Close()

			_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
				[]string{"processed", "completed"}, 60*time.Second)
			require.NoError(t, err)

			// FR contact should get FR translation
			frMsg, err := testutil.WaitForMailpitMessageByRecipient(t, frEmail, 15*time.Second)
			require.NoError(t, err, "FR contact should receive email")
			t.Logf("FR email subject: %s", frMsg.Subject)
			assert.Contains(t, frMsg.Subject, frSubject, "FR contact should get FR subject")
			assert.Contains(t, frMsg.HTML, frBody, "FR contact should get FR body")

			// DE contact should fall back to default EN
			deMsg, err := testutil.WaitForMailpitMessageByRecipient(t, deEmail, 15*time.Second)
			require.NoError(t, err, "DE contact should receive email")
			t.Logf("DE email subject: %s", deMsg.Subject)
			assert.Contains(t, deMsg.Subject, enSubject, "DE contact should get default EN subject (no DE translation)")
			assert.Contains(t, deMsg.HTML, enBody, "DE contact should get default EN body")
			assert.NotContains(t, deMsg.HTML, frBody, "DE contact should NOT get FR body")
		})
	})

	// ========================================================================
	// Automation Tests (2 subtests)
	// ========================================================================

	t.Run("Automation", func(t *testing.T) {
		t.Run("email node sends translated variant", func(t *testing.T) {
			err := testutil.ClearMailpitMessages(t)
			require.NoError(t, err)

			uid := uuid.New().String()[:8]
			enSubject := fmt.Sprintf("Welcome EN - %s", uid)
			frSubject := fmt.Sprintf("Bienvenue FR - %s", uid)
			enBody := fmt.Sprintf("Hello in English - %s", uid)
			frBody := fmt.Sprintf("Bonjour en Francais - %s", uid)

			template, err := factory.CreateTemplate(workspace.ID,
				testutil.WithTemplateName("Auto Translation - FR Match"),
				testutil.WithTemplateSubject(enSubject),
				testutil.WithTemplateEmailContent(enBody),
				testutil.WithTemplateTranslations(map[string]domain.TemplateTranslation{
					"fr": {Email: makeEmailTemplate(frSubject, frBody)},
				}),
			)
			require.NoError(t, err)

			list, err := factory.CreateList(workspace.ID,
				testutil.WithListName(fmt.Sprintf("Auto Trans List %s", uid)))
			require.NoError(t, err)

			// Create automation: trigger → email node
			automationID := shortuuid.New()
			triggerNodeID := shortuuid.New()
			emailNodeID := shortuuid.New()

			createReq := map[string]interface{}{
				"workspace_id": workspace.ID,
				"automation": map[string]interface{}{
					"id":           automationID,
					"workspace_id": workspace.ID,
					"name":         fmt.Sprintf("Translation Auto %s", uid),
					"status":       "draft",
					"list_id":      list.ID,
					"trigger": map[string]interface{}{
						"event_kind": "list.subscribed",
						"list_id":    list.ID,
						"frequency":  "once",
					},
					"root_node_id": triggerNodeID,
					"nodes": []map[string]interface{}{
						{
							"id":            triggerNodeID,
							"automation_id": automationID,
							"type":          "trigger",
							"config":        map[string]interface{}{},
							"next_node_id":  emailNodeID,
							"position":      map[string]interface{}{"x": 0, "y": 0},
						},
						{
							"id":            emailNodeID,
							"automation_id": automationID,
							"type":          "email",
							"config":        map[string]interface{}{"template_id": template.ID},
							"position":      map[string]interface{}{"x": 0, "y": 100},
						},
					},
					"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
				},
			}

			resp, err := client.CreateAutomation(createReq)
			require.NoError(t, err)
			if resp.StatusCode != http.StatusCreated {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				t.Fatalf("CreateAutomation: Expected 201, got %d: %s", resp.StatusCode, string(body))
			}
			resp.Body.Close()

			// Activate automation
			activateResp, err := client.ActivateAutomation(map[string]interface{}{
				"workspace_id":  workspace.ID,
				"automation_id": automationID,
			})
			require.NoError(t, err)
			if activateResp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(activateResp.Body)
				activateResp.Body.Close()
				t.Fatalf("ActivateAutomation: Expected 200, got %d: %s", activateResp.StatusCode, string(body))
			}
			activateResp.Body.Close()

			// Create FR contact and subscribe to list (triggers enrollment)
			contactEmail := fmt.Sprintf("auto-fr-%s@example.com", uid)
			_, err = factory.CreateContact(workspace.ID,
				testutil.WithContactEmail(contactEmail),
				testutil.WithContactLanguage("fr"),
			)
			require.NoError(t, err)

			_, err = factory.CreateContactList(workspace.ID,
				testutil.WithContactListEmail(contactEmail),
				testutil.WithContactListListID(list.ID),
				testutil.WithContactListStatus(domain.ContactListStatusActive),
			)
			require.NoError(t, err)

			// Wait for enrollment
			ca := waitForEnrollmentViaAPI(t, client, automationID, contactEmail, 5*time.Second)
			require.NotNil(t, ca, "Contact should be enrolled")

			// Wait for automation to complete
			completedCA := waitForAutomationComplete(t, factory, workspace.ID, automationID, contactEmail, 15*time.Second)
			require.NotNil(t, completedCA, "Automation should complete")

			// Verify email
			msg, err := testutil.WaitForMailpitMessageByRecipient(t, contactEmail, 15*time.Second)
			require.NoError(t, err, "Should receive email in Mailpit")

			t.Logf("Automation email subject: %s", msg.Subject)
			assert.Contains(t, msg.Subject, frSubject, "Subject should be FR translation")
			assert.NotContains(t, msg.Subject, "Welcome EN", "Subject should NOT be default EN")
			assert.Contains(t, msg.HTML, frBody, "Body should contain FR translation content")
			assert.NotContains(t, msg.HTML, enBody, "Body should NOT contain default EN content")
		})

		t.Run("email node falls back to default", func(t *testing.T) {
			err := testutil.ClearMailpitMessages(t)
			require.NoError(t, err)

			uid := uuid.New().String()[:8]
			enSubject := fmt.Sprintf("Welcome EN - %s", uid)
			frSubject := fmt.Sprintf("Bienvenue FR - %s", uid)
			enBody := fmt.Sprintf("Hello in English - %s", uid)
			frBody := fmt.Sprintf("Bonjour en Francais - %s", uid)

			template, err := factory.CreateTemplate(workspace.ID,
				testutil.WithTemplateName("Auto Translation - DE Fallback"),
				testutil.WithTemplateSubject(enSubject),
				testutil.WithTemplateEmailContent(enBody),
				testutil.WithTemplateTranslations(map[string]domain.TemplateTranslation{
					"fr": {Email: makeEmailTemplate(frSubject, frBody)},
				}),
			)
			require.NoError(t, err)

			list, err := factory.CreateList(workspace.ID,
				testutil.WithListName(fmt.Sprintf("Auto Fallback List %s", uid)))
			require.NoError(t, err)

			automationID := shortuuid.New()
			triggerNodeID := shortuuid.New()
			emailNodeID := shortuuid.New()

			createReq := map[string]interface{}{
				"workspace_id": workspace.ID,
				"automation": map[string]interface{}{
					"id":           automationID,
					"workspace_id": workspace.ID,
					"name":         fmt.Sprintf("Fallback Auto %s", uid),
					"status":       "draft",
					"list_id":      list.ID,
					"trigger": map[string]interface{}{
						"event_kind": "list.subscribed",
						"list_id":    list.ID,
						"frequency":  "once",
					},
					"root_node_id": triggerNodeID,
					"nodes": []map[string]interface{}{
						{
							"id":            triggerNodeID,
							"automation_id": automationID,
							"type":          "trigger",
							"config":        map[string]interface{}{},
							"next_node_id":  emailNodeID,
							"position":      map[string]interface{}{"x": 0, "y": 0},
						},
						{
							"id":            emailNodeID,
							"automation_id": automationID,
							"type":          "email",
							"config":        map[string]interface{}{"template_id": template.ID},
							"position":      map[string]interface{}{"x": 0, "y": 100},
						},
					},
					"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
				},
			}

			resp, err := client.CreateAutomation(createReq)
			require.NoError(t, err)
			if resp.StatusCode != http.StatusCreated {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				t.Fatalf("CreateAutomation: Expected 201, got %d: %s", resp.StatusCode, string(body))
			}
			resp.Body.Close()

			activateResp, err := client.ActivateAutomation(map[string]interface{}{
				"workspace_id":  workspace.ID,
				"automation_id": automationID,
			})
			require.NoError(t, err)
			if activateResp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(activateResp.Body)
				activateResp.Body.Close()
				t.Fatalf("ActivateAutomation: Expected 200, got %d: %s", activateResp.StatusCode, string(body))
			}
			activateResp.Body.Close()

			// Create DE contact (no translation exists for DE)
			contactEmail := fmt.Sprintf("auto-de-%s@example.com", uid)
			_, err = factory.CreateContact(workspace.ID,
				testutil.WithContactEmail(contactEmail),
				testutil.WithContactLanguage("de"),
			)
			require.NoError(t, err)

			_, err = factory.CreateContactList(workspace.ID,
				testutil.WithContactListEmail(contactEmail),
				testutil.WithContactListListID(list.ID),
				testutil.WithContactListStatus(domain.ContactListStatusActive),
			)
			require.NoError(t, err)

			ca := waitForEnrollmentViaAPI(t, client, automationID, contactEmail, 5*time.Second)
			require.NotNil(t, ca, "Contact should be enrolled")

			completedCA := waitForAutomationComplete(t, factory, workspace.ID, automationID, contactEmail, 15*time.Second)
			require.NotNil(t, completedCA, "Automation should complete")

			msg, err := testutil.WaitForMailpitMessageByRecipient(t, contactEmail, 15*time.Second)
			require.NoError(t, err, "Should receive email in Mailpit")

			t.Logf("Automation email subject: %s", msg.Subject)
			assert.Contains(t, msg.Subject, enSubject, "Subject should be default EN (no DE translation)")
			assert.NotContains(t, msg.Subject, "Bienvenue", "Subject should NOT be FR")
			assert.Contains(t, msg.HTML, enBody, "Body should contain default EN content")
			assert.NotContains(t, msg.HTML, frBody, "Body should NOT contain FR content")
		})
	})
}
