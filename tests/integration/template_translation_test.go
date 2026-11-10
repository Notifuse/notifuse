package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
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

// stalePreheaderSentinel is baked into a template's mj-preview block by makeEmailTemplateWithPreview
// to stand in for a "cloned default" inbox-preview value. A correctly-sent variant overrides it with
// its own subject_preview at compile time, so this sentinel must never appear in a delivered email.
const stalePreheaderSentinel = "STALE_PREHEADER_SHOULD_BE_OVERRIDDEN"

// makeEmailTemplateWithPreview is like makeEmailTemplate but also sets a subject_preview field and a
// tree whose mj-preview block carries the stale sentinel — so a send must override that block with
// the variant's own subject_preview for the preview text to be correct.
func makeEmailTemplateWithPreview(subject, subjectPreview, bodyContent string) *domain.EmailTemplate {
	preview := subjectPreview
	return &domain.EmailTemplate{
		Subject:          subject,
		SubjectPreview:   &preview,
		CompiledPreview:  fmt.Sprintf(`<mjml><mj-head></mj-head><mj-body><mj-section><mj-column><mj-text>%s</mj-text></mj-column></mj-section></mj-body></mjml>`, bodyContent),
		VisualEditorTree: testutil.CreateMJMLBlockWithContentAndPreview(bodyContent, stalePreheaderSentinel),
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
			enPreview := fmt.Sprintf("EN preheader %s", uid)
			frPreview := fmt.Sprintf("FR apercu %s", uid)
			esPreview := fmt.Sprintf("ES vistazo %s", uid)

			template, err := factory.CreateTemplate(workspace.ID,
				testutil.WithTemplateName("Broadcast Translation - Multi"),
				testutil.TemplateOption(func(tmpl *domain.Template) {
					tmpl.Email = makeEmailTemplateWithPreview(enSubject, enPreview, enBody)
				}),
				testutil.WithTemplateTranslations(map[string]domain.TemplateTranslation{
					"fr": {Email: makeEmailTemplateWithPreview(frSubject, frPreview, frBody)},
					"es": {Email: makeEmailTemplateWithPreview(esSubject, esPreview, esBody)},
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
			assert.Contains(t, enMsg.HTML, enPreview, "EN contact should get default EN preview text")
			assert.NotContains(t, enMsg.HTML, stalePreheaderSentinel, "stale preheader must be overridden")

			// Verify FR contact gets FR translation
			frMsg, err := testutil.WaitForMailpitMessageByRecipient(t, frEmail, 15*time.Second)
			require.NoError(t, err, "FR contact should receive email")
			t.Logf("FR email subject: %s", frMsg.Subject)
			assert.Contains(t, frMsg.Subject, frSubject, "FR contact should get FR subject")
			assert.Contains(t, frMsg.HTML, frBody, "FR contact should get FR body")
			assert.NotContains(t, frMsg.HTML, enBody, "FR contact should NOT get EN body")
			assert.Contains(t, frMsg.HTML, frPreview, "FR contact should get FR preview text")
			assert.NotContains(t, frMsg.HTML, enPreview, "FR contact should NOT get EN preview text")
			assert.NotContains(t, frMsg.HTML, stalePreheaderSentinel, "stale preheader must be overridden")

			// Verify ES contact gets ES translation
			esMsg, err := testutil.WaitForMailpitMessageByRecipient(t, esEmail, 15*time.Second)
			require.NoError(t, err, "ES contact should receive email")
			t.Logf("ES email subject: %s", esMsg.Subject)
			assert.Contains(t, esMsg.Subject, esSubject, "ES contact should get ES subject")
			assert.Contains(t, esMsg.HTML, esBody, "ES contact should get ES body")
			assert.NotContains(t, esMsg.HTML, enBody, "ES contact should NOT get EN body")
			assert.Contains(t, esMsg.HTML, esPreview, "ES contact should get ES preview text")
			assert.NotContains(t, esMsg.HTML, stalePreheaderSentinel, "stale preheader must be overridden")
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
			enPreview := fmt.Sprintf("EN preheader %s", uid)
			frPreview := fmt.Sprintf("FR apercu %s", uid)

			template, err := factory.CreateTemplate(workspace.ID,
				testutil.WithTemplateName("Auto Translation - FR Match"),
				testutil.TemplateOption(func(tmpl *domain.Template) {
					tmpl.Email = makeEmailTemplateWithPreview(enSubject, enPreview, enBody)
				}),
				testutil.WithTemplateTranslations(map[string]domain.TemplateTranslation{
					"fr": {Email: makeEmailTemplateWithPreview(frSubject, frPreview, frBody)},
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
			assert.Contains(t, msg.HTML, frPreview, "Preview should be the FR translation's own value")
			assert.NotContains(t, msg.HTML, enPreview, "Preview should NOT be the default EN value")
			assert.NotContains(t, msg.HTML, stalePreheaderSentinel, "stale preheader must be overridden")
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

// TestTemplateTranslationSubjectPreview is an end-to-end regression test: a translated
// email must render its OWN inbox preview text (mj-preview), not the default template's. The data
// factory inserts templates directly via the repository (bypassing the service that stamps
// mj-preview on save), so this exercises the render-time override wired into the send path.
func TestTemplateTranslationSubjectPreview(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace(
		testutil.WithWorkspaceDefaultLanguage("en", []string{"en", "fr"}),
	)
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	err = testutil.ClearMailpitMessages(t)
	require.NoError(t, err)

	uid := uuid.New().String()[:8]
	enSubject := fmt.Sprintf("Welcome EN - %s", uid)
	frSubject := fmt.Sprintf("Bienvenue FR - %s", uid)
	enPreview := fmt.Sprintf("EN preview text %s", uid)
	frPreview := fmt.Sprintf("FR texte apercu %s", uid)
	enBody := fmt.Sprintf("Hello EN %s", uid)
	frBody := fmt.Sprintf("Bonjour FR %s", uid)

	// Code-mode source WITHOUT an mj-preview block, so the only way the preview text can appear in
	// the rendered HTML is the send-path override injecting each variant's own SubjectPreview.
	codeEmail := func(subject, preview, body string) *domain.EmailTemplate {
		p := preview
		src := fmt.Sprintf(`<mjml><mj-body><mj-section><mj-column><mj-text>%s</mj-text></mj-column></mj-section></mj-body></mjml>`, body)
		return &domain.EmailTemplate{
			EditorMode:      domain.EditorModeCode,
			MjmlSource:      &src,
			Subject:         subject,
			SubjectPreview:  &p,
			CompiledPreview: src,
		}
	}

	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Preview Translation Test"),
		testutil.TemplateOption(func(tmpl *domain.Template) {
			tmpl.Email = codeEmail(enSubject, enPreview, enBody)
			tmpl.Translations = map[string]domain.TemplateTranslation{
				"fr": {Email: codeEmail(frSubject, frPreview, frBody)},
			}
		}),
	)
	require.NoError(t, err)

	notifID := fmt.Sprintf("preview-fr-%s", uid)
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

	contactEmail := fmt.Sprintf("preview-fr-%s@example.com", uid)
	_, err = factory.CreateContact(workspace.ID,
		testutil.WithContactEmail(contactEmail),
		testutil.WithContactLanguage("fr"),
	)
	require.NoError(t, err)

	resp, err := client.SendTransactionalNotification(map[string]interface{}{
		"id":       notification.ID,
		"contact":  map[string]interface{}{"email": contactEmail},
		"channels": []string{"email"},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Send should succeed")
	resp.Body.Close()

	msg, err := testutil.WaitForMailpitMessageByRecipient(t, contactEmail, 15*time.Second)
	require.NoError(t, err, "Should receive email in Mailpit")

	t.Logf("Received subject: %s", msg.Subject)
	assert.Contains(t, msg.Subject, frSubject, "Subject should be FR translation")
	assert.Contains(t, msg.HTML, frBody, "Body should contain FR translation content")
	// The crux of the fix: the FR translation's own preview text must render, not the default EN one.
	assert.Contains(t, msg.HTML, frPreview, "Preview text should be the FR translation's own value")
	assert.NotContains(t, msg.HTML, enPreview, "Preview text should NOT be the default EN value")
}

// mailpitReplyTo returns the Reply-To header of a message Mailpit received.
func mailpitReplyTo(t *testing.T, msg *testutil.MailpitMessage) string {
	t.Helper()
	raw, err := testutil.GetMailpitMessageRaw(t, msg.ID)
	require.NoError(t, err)
	parsed, err := mail.ReadMessage(strings.NewReader(raw))
	require.NoError(t, err)
	return parsed.Header.Get("Reply-To")
}

// TestTemplateTranslationSenderAndReplyTo checks that a translation saved the way the console
// saves it, with no sender and no reply-to of its own, sends from the sender picked on the
// template and with the template's reply-to, on every send path, and that a transactional send's
// own reply-to still wins over the template's.
func TestTemplateTranslationSenderAndReplyTo(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	ctx := context.Background()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace(
		testutil.WithWorkspaceDefaultLanguage("en", []string{"en", "fr"}),
	)
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))

	// The integration's default sender stays the factory's; the templates below pick a second,
	// non-default one, so a send that falls back to the default is told apart.
	newsletterSender := domain.NewEmailSender("newsletter@notifuse.test", "Newsletter")
	newsletterSender.IsDefault = false
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.IntegrationOption(func(integration *domain.Integration) {
			integration.EmailProvider.Senders = append(integration.EmailProvider.Senders, newsletterSender)
		}),
	)
	require.NoError(t, err)

	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	const templateReplyTo = "support@notifuse.test"

	// createTemplate stores a template on the newsletter sender with a reply-to, and a French
	// translation saved as the console saves it: no sender, no reply-to.
	createTemplate := func(t *testing.T, name, uid string) (*domain.Template, string) {
		t.Helper()
		frSubject := fmt.Sprintf("Bonjour FR - %s", uid)
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName(name),
			testutil.TemplateOption(func(tmpl *domain.Template) {
				tmpl.Email = makeEmailTemplate(fmt.Sprintf("Hello EN - %s", uid), "Hello in English")
				tmpl.Email.SenderID = newsletterSender.ID
				tmpl.Email.ReplyTo = templateReplyTo
			}),
			testutil.WithTemplateTranslations(map[string]domain.TemplateTranslation{
				"fr": {Email: makeEmailTemplate(frSubject, "Bonjour en Francais")},
			}),
		)
		require.NoError(t, err)
		require.Empty(t, template.Translations["fr"].Email.SenderID)
		require.Empty(t, template.Translations["fr"].Email.ReplyTo)
		return template, frSubject
	}

	assertTemplateSenderAndReplyTo := func(t *testing.T, msg *testutil.MailpitMessage, frSubject string) {
		t.Helper()
		assert.Contains(t, msg.Subject, frSubject, "the French translation is sent")
		assert.Equal(t, "newsletter@notifuse.test", msg.From.Address, "From is the template's sender")
		assert.Equal(t, "Newsletter", msg.From.Name)
		assert.Contains(t, mailpitReplyTo(t, msg), templateReplyTo, "Reply-To is the template's")
	}

	t.Run("Transactional", func(t *testing.T) {
		require.NoError(t, testutil.ClearMailpitMessages(t))
		uid := uuid.New().String()[:8]
		template, frSubject := createTemplate(t, "Sender Translation - Transactional", uid)

		notification, err := factory.CreateTransactionalNotification(workspace.ID,
			testutil.WithTransactionalNotificationID(fmt.Sprintf("trans-sender-%s", uid)),
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{
					TemplateID: template.ID,
					Settings:   map[string]interface{}{},
				},
			}),
		)
		require.NoError(t, err)

		send := func(t *testing.T, contactEmail string, emailOptions map[string]interface{}) *testutil.MailpitMessage {
			t.Helper()
			_, err := factory.CreateContact(workspace.ID,
				testutil.WithContactEmail(contactEmail),
				testutil.WithContactLanguage("fr"),
			)
			require.NoError(t, err)

			params := map[string]interface{}{
				"id":       notification.ID,
				"contact":  map[string]interface{}{"email": contactEmail},
				"channels": []string{"email"},
			}
			if emailOptions != nil {
				params["email_options"] = emailOptions
			}
			resp, err := client.SendTransactionalNotification(params)
			require.NoError(t, err)
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

			msg, err := testutil.WaitForMailpitMessageByRecipient(t, contactEmail, 15*time.Second)
			require.NoError(t, err)
			return msg
		}

		t.Run("translated contact gets the template's sender and reply-to", func(t *testing.T) {
			msg := send(t, fmt.Sprintf("trans-sender-fr-%s@example.com", uid), nil)
			assertTemplateSenderAndReplyTo(t, msg, frSubject)
		})

		t.Run("the send's own reply-to wins over the template's", func(t *testing.T) {
			msg := send(t, fmt.Sprintf("trans-replyto-fr-%s@example.com", uid),
				map[string]interface{}{"reply_to": "agent@notifuse.test"})
			assert.Equal(t, "newsletter@notifuse.test", msg.From.Address)
			replyTo := mailpitReplyTo(t, msg)
			assert.Contains(t, replyTo, "agent@notifuse.test")
			assert.NotContains(t, replyTo, templateReplyTo)
		})
	})

	t.Run("Broadcast", func(t *testing.T) {
		require.NoError(t, testutil.ClearMailpitMessages(t))
		uid := uuid.New().String()[:8]
		template, frSubject := createTemplate(t, "Sender Translation - Broadcast", uid)

		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName(fmt.Sprintf("Sender Translation List %s", uid)))
		require.NoError(t, err)

		contactEmail := fmt.Sprintf("bc-sender-fr-%s@example.com", uid)
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

		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName(fmt.Sprintf("Sender Translation Broadcast %s", uid)),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}),
		)
		require.NoError(t, err)

		broadcast.TestSettings.Variations[0].TemplateID = template.ID
		updateResp, err := client.UpdateBroadcast(map[string]interface{}{
			"workspace_id":  workspace.ID,
			"id":            broadcast.ID,
			"name":          broadcast.Name,
			"audience":      broadcast.Audience,
			"schedule":      broadcast.Schedule,
			"test_settings": broadcast.TestSettings,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, updateResp.StatusCode)
		updateResp.Body.Close()

		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcast.ID,
			"send_now":     true,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, scheduleResp.StatusCode)
		scheduleResp.Body.Close()

		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
			[]string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		msg, err := testutil.WaitForMailpitMessageByRecipient(t, contactEmail, 15*time.Second)
		require.NoError(t, err)
		assertTemplateSenderAndReplyTo(t, msg, frSubject)
	})

	t.Run("Automation", func(t *testing.T) {
		require.NoError(t, testutil.ClearMailpitMessages(t))
		uid := uuid.New().String()[:8]
		template, frSubject := createTemplate(t, "Sender Translation - Automation", uid)

		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName(fmt.Sprintf("Sender Translation Auto List %s", uid)))
		require.NoError(t, err)

		automationID := shortuuid.New()
		triggerNodeID := shortuuid.New()
		emailNodeID := shortuuid.New()

		resp, err := client.CreateAutomation(map[string]interface{}{
			"workspace_id": workspace.ID,
			"automation": map[string]interface{}{
				"id":           automationID,
				"workspace_id": workspace.ID,
				"name":         fmt.Sprintf("Sender Translation Auto %s", uid),
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
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))

		activateResp, err := client.ActivateAutomation(map[string]interface{}{
			"workspace_id":  workspace.ID,
			"automation_id": automationID,
		})
		require.NoError(t, err)
		body, _ = io.ReadAll(activateResp.Body)
		activateResp.Body.Close()
		require.Equal(t, http.StatusOK, activateResp.StatusCode, string(body))

		contactEmail := fmt.Sprintf("auto-sender-fr-%s@example.com", uid)
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

		require.NotNil(t, waitForEnrollmentViaAPI(t, client, automationID, contactEmail, 5*time.Second))
		require.NotNil(t, waitForAutomationComplete(t, factory, workspace.ID, automationID, contactEmail, 15*time.Second))

		msg, err := testutil.WaitForMailpitMessageByRecipient(t, contactEmail, 15*time.Second)
		require.NoError(t, err)
		assertTemplateSenderAndReplyTo(t, msg, frSubject)
	})
}
