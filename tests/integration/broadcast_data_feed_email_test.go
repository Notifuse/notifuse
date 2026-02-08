package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

// TestBroadcastDataFeedEmailRendering tests that global_feed Liquid variables
// are correctly rendered in broadcast emails sent via Mailpit.
func TestBroadcastDataFeedEmailRendering(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	// Setup: Create user and workspace
	t.Log("Setting up user and workspace...")
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Setup SMTP provider (Mailpit on localhost:1025)
	t.Log("Setting up SMTP provider...")
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Notifuse Data Feed Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025,
				Username: "",
				Password: "",
				UseTLS:   false,
			},
			RateLimitPerMinute: 2000,
		}))
	require.NoError(t, err)

	// Start email queue worker to process enqueued emails
	t.Log("Starting email queue worker...")
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	err = suite.ServerManager.StartBackgroundWorkers(workerCtx)
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	// Login
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("global feed data is rendered in email subject and body", func(t *testing.T) {
		err := testutil.ClearMailpitMessages(t)
		require.NoError(t, err)

		// Setup mock feed server returning known data
		mockServer := NewMockFeedServer()
		defer mockServer.Close()
		mockServer.SetResponse(map[string]interface{}{
			"promo_code":    "WINTER2026",
			"discount_text": "30% off",
			"product_name":  "Premium Plan",
		})

		// Create list and contact
		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName("Data Feed Email Test List"))
		require.NoError(t, err)

		contactEmail := fmt.Sprintf("datafeed-%s@example.com", uuid.New().String()[:8])
		contact, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(contactEmail),
			testutil.WithContactName("John", "Doe"))
		require.NoError(t, err)
		t.Logf("Created contact: %s", contact.Email)

		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		// Create template with global_feed Liquid variables
		uniqueID := uuid.New().String()[:8]
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("Data Feed Template"),
			testutil.WithTemplateSubject(fmt.Sprintf("Deal: {{ global_feed.promo_code }} - %s", uniqueID)),
			testutil.WithTemplateEmailContent(
				"Hello {{ contact.first_name }}! Use code {{ global_feed.promo_code }} for {{ global_feed.discount_text }} on {{ global_feed.product_name }}."))
		require.NoError(t, err)

		// Create broadcast with global feed enabled and template_id already set
		// Factory writes directly to repo, bypassing SSRF validation on localhost URLs
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("Data Feed Email Test"),
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastGlobalFeed(&domain.GlobalFeedSettings{
				Enabled: true,
				URL:     mockServer.URL(),
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		// Schedule broadcast to send now
		t.Log("Scheduling broadcast...")
		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcast.ID,
			"send_now":     true,
		})
		require.NoError(t, err)
		scheduleResp.Body.Close()

		// Wait for broadcast completion
		t.Log("Waiting for broadcast completion...")
		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
			[]string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		// Fetch email from Mailpit
		t.Log("Fetching email from Mailpit...")
		msg, err := waitForEmailByRecipientAddr(t, contactEmail, 15*time.Second)
		require.NoError(t, err, "Should receive email in Mailpit")

		t.Logf("Email subject: %s", msg.Subject)
		t.Logf("Email HTML (first 300 chars): %s", truncStr(msg.HTML, 300))

		// Assert global feed data rendered in subject
		assert.Contains(t, msg.Subject, "WINTER2026", "Subject should contain rendered promo_code")
		assert.NotContains(t, msg.Subject, "{{", "Subject should NOT contain raw Liquid syntax")

		// Assert global feed data rendered in body
		assert.Contains(t, msg.HTML, "WINTER2026", "Body should contain rendered promo_code")
		assert.Contains(t, msg.HTML, "30% off", "Body should contain rendered discount_text")
		assert.Contains(t, msg.HTML, "Premium Plan", "Body should contain rendered product_name")
		assert.Contains(t, msg.HTML, "Hello John!", "Body should contain rendered contact name")
		assert.NotContains(t, msg.HTML, "{{ global_feed", "Body should NOT contain raw Liquid syntax")

		// Verify mock server received exactly 1 request (fetched once at scheduling)
		requests := mockServer.GetRequests()
		require.Len(t, requests, 1, "Global feed should be fetched exactly once")
		assert.Equal(t, "POST", requests[0].Method)
		assert.Contains(t, requests[0].Body, "broadcast", "Request should contain broadcast info")
		assert.Contains(t, requests[0].Body, "list", "Request should contain list info")
		assert.Contains(t, requests[0].Body, "workspace", "Request should contain workspace info")
	})

	t.Run("same global feed data rendered for multiple recipients", func(t *testing.T) {
		err := testutil.ClearMailpitMessages(t)
		require.NoError(t, err)

		// Setup mock feed server
		mockServer := NewMockFeedServer()
		defer mockServer.Close()
		mockServer.SetResponse(map[string]interface{}{
			"weekly_tip": "Use segments for better targeting",
			"edition":    "42",
		})

		// Create list and two contacts
		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName("Multi Recipient Feed List"))
		require.NoError(t, err)

		aliceEmail := fmt.Sprintf("alice-feed-%s@example.com", uuid.New().String()[:8])
		alice, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(aliceEmail),
			testutil.WithContactName("Alice", "Smith"))
		require.NoError(t, err)

		bobEmail := fmt.Sprintf("bob-feed-%s@example.com", uuid.New().String()[:8])
		bob, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(bobEmail),
			testutil.WithContactName("Bob", "Jones"))
		require.NoError(t, err)

		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(alice.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(bob.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		// Create template with global_feed variables
		uniqueID := uuid.New().String()[:8]
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("Multi Recipient Feed Template"),
			testutil.WithTemplateSubject(fmt.Sprintf("Tip #{{ global_feed.edition }} - %s", uniqueID)),
			testutil.WithTemplateEmailContent(
				"Hi {{ contact.first_name }}, tip: {{ global_feed.weekly_tip }}."))
		require.NoError(t, err)

		// Create broadcast with global feed and template_id already set
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("Multi Recipient Feed Test"),
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastGlobalFeed(&domain.GlobalFeedSettings{
				Enabled: true,
				URL:     mockServer.URL(),
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		t.Log("Scheduling broadcast...")
		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcast.ID,
			"send_now":     true,
		})
		require.NoError(t, err)
		scheduleResp.Body.Close()

		t.Log("Waiting for broadcast completion...")
		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
			[]string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		// Fetch both emails from Mailpit
		t.Log("Fetching emails from Mailpit...")
		aliceMsg, err := waitForEmailByRecipientAddr(t, aliceEmail, 15*time.Second)
		require.NoError(t, err, "Should receive email for Alice")

		bobMsg, err := waitForEmailByRecipientAddr(t, bobEmail, 15*time.Second)
		require.NoError(t, err, "Should receive email for Bob")

		// Assert Alice's email
		assert.Contains(t, aliceMsg.Subject, "Tip #42", "Alice's subject should contain edition")
		assert.Contains(t, aliceMsg.HTML, "Hi Alice", "Alice's body should contain her name")
		assert.Contains(t, aliceMsg.HTML, "Use segments for better targeting",
			"Alice's body should contain global feed tip")

		// Assert Bob's email — same global feed data, different contact name
		assert.Contains(t, bobMsg.Subject, "Tip #42", "Bob's subject should contain same edition")
		assert.Contains(t, bobMsg.HTML, "Hi Bob", "Bob's body should contain his name")
		assert.Contains(t, bobMsg.HTML, "Use segments for better targeting",
			"Bob's body should contain same global feed tip")

		// No raw Liquid syntax
		assert.NotContains(t, aliceMsg.HTML, "{{ global_feed", "No raw Liquid in Alice's email")
		assert.NotContains(t, bobMsg.HTML, "{{ global_feed", "No raw Liquid in Bob's email")

		// Global feed should be fetched exactly once (not per recipient)
		requests := mockServer.GetRequests()
		require.Len(t, requests, 1, "Global feed should be fetched exactly once, not per recipient")
	})

	t.Run("pre-populated global feed data renders without live fetch", func(t *testing.T) {
		err := testutil.ClearMailpitMessages(t)
		require.NoError(t, err)

		// Create list and contact
		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName("Pre-populated Feed List"))
		require.NoError(t, err)

		contactEmail := fmt.Sprintf("eve-feed-%s@example.com", uuid.New().String()[:8])
		contact, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(contactEmail),
			testutil.WithContactName("Eve", "Wilson"))
		require.NoError(t, err)

		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		// Create template referencing global_feed
		uniqueID := uuid.New().String()[:8]
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("Pre-populated Feed Template"),
			testutil.WithTemplateSubject(fmt.Sprintf("Flash Sale - %s", uniqueID)),
			testutil.WithTemplateEmailContent(
				"Hello {{ contact.first_name }}, check out {{ global_feed.product }} at {{ global_feed.sale_price }}!"))
		require.NoError(t, err)

		// Create broadcast with pre-populated global feed data (no live fetch)
		fetchedAt := time.Now().UTC()
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("Pre-populated Feed Test"),
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastGlobalFeed(&domain.GlobalFeedSettings{
				Enabled: false, // Disabled — no fetch at scheduling time
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastGlobalFeedData(
				map[string]interface{}{
					"product":     "Smart Watch",
					"sale_price":  "$199",
					"_success":    true,
					"_fetched_at": fetchedAt.Format(time.RFC3339),
				},
				&fetchedAt,
			),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		t.Log("Scheduling broadcast...")
		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcast.ID,
			"send_now":     true,
		})
		require.NoError(t, err)
		scheduleResp.Body.Close()

		t.Log("Waiting for broadcast completion...")
		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
			[]string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		// Fetch email from Mailpit
		t.Log("Fetching email from Mailpit...")
		msg, err := waitForEmailByRecipientAddr(t, contactEmail, 15*time.Second)
		require.NoError(t, err, "Should receive email in Mailpit")

		t.Logf("Email subject: %s", msg.Subject)
		t.Logf("Email HTML (first 300 chars): %s", truncStr(msg.HTML, 300))

		// Assert pre-populated data rendered correctly
		assert.Contains(t, msg.HTML, "Smart Watch", "Body should contain pre-populated product")
		assert.Contains(t, msg.HTML, "$199", "Body should contain pre-populated sale_price")
		assert.Contains(t, msg.HTML, "Hello Eve", "Body should contain rendered contact name")
		assert.NotContains(t, msg.HTML, "{{ global_feed", "Body should NOT contain raw Liquid syntax")
	})
}

// TestRecipientFeedEmailRendering tests that recipient_feed Liquid variables
// are correctly rendered in broadcast emails with per-recipient data.
func TestRecipientFeedEmailRendering(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	// Setup: Create user and workspace
	t.Log("Setting up user and workspace...")
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Setup SMTP provider (Mailpit on localhost:1025)
	t.Log("Setting up SMTP provider...")
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Notifuse Recipient Feed Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025,
				Username: "",
				Password: "",
				UseTLS:   false,
			},
			RateLimitPerMinute: 2000,
		}))
	require.NoError(t, err)

	// Start email queue worker to process enqueued emails
	t.Log("Starting email queue worker...")
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	err = suite.ServerManager.StartBackgroundWorkers(workerCtx)
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	// Login
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("recipient feed data is rendered in email body", func(t *testing.T) {
		err := testutil.ClearMailpitMessages(t)
		require.NoError(t, err)

		// Setup mock feed server returning known data
		mockServer := NewMockFeedServer()
		defer mockServer.Close()
		mockServer.SetResponse(map[string]interface{}{
			"product":    "Premium Widget",
			"price":      "$99.99",
			"offer_code": "SAVE20",
		})

		// Create list and contact
		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName("Recipient Feed Email Test List"))
		require.NoError(t, err)

		contactEmail := fmt.Sprintf("recipientfeed-%s@example.com", uuid.New().String()[:8])
		contact, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(contactEmail),
			testutil.WithContactName("Sarah", "Connor"))
		require.NoError(t, err)
		t.Logf("Created contact: %s", contact.Email)

		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		// Create template with recipient_feed Liquid variables
		uniqueID := uuid.New().String()[:8]
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("Recipient Feed Template"),
			testutil.WithTemplateSubject(fmt.Sprintf("Your offer: {{ recipient_feed.offer_code }} - %s", uniqueID)),
			testutil.WithTemplateEmailContent(
				"Hello {{ contact.first_name }}! Get {{ recipient_feed.product }} for {{ recipient_feed.price }}."))
		require.NoError(t, err)

		// Create broadcast with recipient feed enabled
		// Factory writes directly to repo, bypassing SSRF/HTTPS validation on localhost URLs
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("Recipient Feed Email Test"),
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastRecipientFeed(&domain.RecipientFeedSettings{
				Enabled: true,
				URL:     mockServer.URL(),
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		// Schedule broadcast to send now
		t.Log("Scheduling broadcast...")
		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcast.ID,
			"send_now":     true,
		})
		require.NoError(t, err)
		scheduleResp.Body.Close()

		// Wait for broadcast completion
		t.Log("Waiting for broadcast completion...")
		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
			[]string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		// Fetch email from Mailpit
		t.Log("Fetching email from Mailpit...")
		msg, err := waitForEmailByRecipientAddr(t, contactEmail, 15*time.Second)
		require.NoError(t, err, "Should receive email in Mailpit")

		t.Logf("Email subject: %s", msg.Subject)
		t.Logf("Email HTML (first 300 chars): %s", truncStr(msg.HTML, 300))

		// Assert recipient feed data rendered in subject
		assert.Contains(t, msg.Subject, "SAVE20", "Subject should contain rendered offer_code")
		assert.NotContains(t, msg.Subject, "{{", "Subject should NOT contain raw Liquid syntax")

		// Assert recipient feed data rendered in body
		assert.Contains(t, msg.HTML, "Premium Widget", "Body should contain rendered product")
		assert.Contains(t, msg.HTML, "$99.99", "Body should contain rendered price")
		assert.Contains(t, msg.HTML, "Hello Sarah!", "Body should contain rendered contact name")
		assert.NotContains(t, msg.HTML, "{{ recipient_feed", "Body should NOT contain raw Liquid syntax")

		// Verify mock server received exactly 1 request (fetched once per recipient)
		requests := mockServer.GetRequests()
		require.Len(t, requests, 1, "Recipient feed should be fetched once per recipient")
		assert.Equal(t, "POST", requests[0].Method)

		// Verify request contains contact info
		contactData := requests[0].Body["contact"].(map[string]interface{})
		assert.Equal(t, contactEmail, contactData["email"], "Request should contain contact email")
	})

	t.Run("different recipients receive different personalized data", func(t *testing.T) {
		err := testutil.ClearMailpitMessages(t)
		require.NoError(t, err)

		// Setup mock server with per-recipient responses
		mockServer := NewMockFeedServer()
		defer mockServer.Close()

		aliceEmail := fmt.Sprintf("alice-feed-%s@example.com", uuid.New().String()[:8])
		bobEmail := fmt.Sprintf("bob-feed-%s@example.com", uuid.New().String()[:8])

		mockServer.SetRecipientResponses(map[string]map[string]interface{}{
			aliceEmail: {"product": "Widget A", "price": "$10", "discount": "10%"},
			bobEmail:   {"product": "Widget B", "price": "$25", "discount": "5%"},
		})

		// Create list and two contacts
		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName("Multi Recipient Feed List"))
		require.NoError(t, err)

		alice, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(aliceEmail),
			testutil.WithContactName("Alice", "Smith"))
		require.NoError(t, err)

		bob, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(bobEmail),
			testutil.WithContactName("Bob", "Jones"))
		require.NoError(t, err)

		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(alice.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(bob.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		// Create template with recipient_feed variables
		uniqueID := uuid.New().String()[:8]
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("Multi Recipient Feed Template"),
			testutil.WithTemplateSubject(fmt.Sprintf("Your personalized offer - %s", uniqueID)),
			testutil.WithTemplateEmailContent(
				"Hi {{ contact.first_name }}! Product: {{ recipient_feed.product }} - Price: {{ recipient_feed.price }}"))
		require.NoError(t, err)

		// Create broadcast with recipient feed
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("Multi Recipient Feed Test"),
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastRecipientFeed(&domain.RecipientFeedSettings{
				Enabled: true,
				URL:     mockServer.URL(),
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		t.Log("Scheduling broadcast...")
		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcast.ID,
			"send_now":     true,
		})
		require.NoError(t, err)
		scheduleResp.Body.Close()

		t.Log("Waiting for broadcast completion...")
		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
			[]string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		// Fetch and verify Alice's email
		t.Log("Fetching Alice's email from Mailpit...")
		aliceMsg, err := waitForEmailByRecipientAddr(t, aliceEmail, 15*time.Second)
		require.NoError(t, err, "Should receive email for Alice")

		t.Logf("Alice's email HTML (first 300 chars): %s", truncStr(aliceMsg.HTML, 300))

		assert.Contains(t, aliceMsg.HTML, "Widget A", "Alice should receive Widget A")
		assert.Contains(t, aliceMsg.HTML, "$10", "Alice should see $10 price")
		assert.Contains(t, aliceMsg.HTML, "Hi Alice!", "Alice should see her name")
		assert.NotContains(t, aliceMsg.HTML, "Widget B", "Alice should NOT see Bob's product")
		assert.NotContains(t, aliceMsg.HTML, "$25", "Alice should NOT see Bob's price")

		// Fetch and verify Bob's email
		t.Log("Fetching Bob's email from Mailpit...")
		bobMsg, err := waitForEmailByRecipientAddr(t, bobEmail, 15*time.Second)
		require.NoError(t, err, "Should receive email for Bob")

		t.Logf("Bob's email HTML (first 300 chars): %s", truncStr(bobMsg.HTML, 300))

		assert.Contains(t, bobMsg.HTML, "Widget B", "Bob should receive Widget B")
		assert.Contains(t, bobMsg.HTML, "$25", "Bob should see $25 price")
		assert.Contains(t, bobMsg.HTML, "Hi Bob!", "Bob should see his name")
		assert.NotContains(t, bobMsg.HTML, "Widget A", "Bob should NOT see Alice's product")
		assert.NotContains(t, bobMsg.HTML, "$10", "Bob should NOT see Alice's price")

		// Verify mock server received 2 requests (one per recipient)
		requests := mockServer.GetRequests()
		require.Len(t, requests, 2, "Should fetch feed once per recipient")

		// Verify each request contains the correct contact email
		emails := make([]string, 2)
		for i, req := range requests {
			contact := req.Body["contact"].(map[string]interface{})
			emails[i] = contact["email"].(string)
		}
		assert.Contains(t, emails, aliceEmail, "One request should be for Alice")
		assert.Contains(t, emails, bobEmail, "One request should be for Bob")
	})

	t.Run("recipient_feed retries on 5xx errors then succeeds", func(t *testing.T) {
		err := testutil.ClearMailpitMessages(t)
		require.NoError(t, err)

		// Setup mock server that fails 2 times then succeeds
		mockServer := NewMockFeedServer()
		defer mockServer.Close()
		mockServer.SetRetryBehavior(2) // Fail 2 times (500 error), succeed on 3rd attempt
		mockServer.SetResponse(map[string]interface{}{
			"product": "Success Widget",
			"price":   "$199",
		})

		// Create list and contact
		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName("Retry Test List"))
		require.NoError(t, err)

		contactEmail := fmt.Sprintf("retry-test-%s@example.com", uuid.New().String()[:8])
		contact, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(contactEmail),
			testutil.WithContactName("Retry", "Tester"))
		require.NoError(t, err)

		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		// Create template with recipient_feed variables
		uniqueID := uuid.New().String()[:8]
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("Retry Test Template"),
			testutil.WithTemplateSubject(fmt.Sprintf("Retry Success - %s", uniqueID)),
			testutil.WithTemplateEmailContent(
				"Product: {{ recipient_feed.product }} at {{ recipient_feed.price }}"))
		require.NoError(t, err)

		// Create broadcast with recipient feed
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("Retry Test Broadcast"),
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastRecipientFeed(&domain.RecipientFeedSettings{
				Enabled: true,
				URL:     mockServer.URL(),
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		t.Log("Scheduling broadcast (will retry on 5xx errors)...")
		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcast.ID,
			"send_now":     true,
		})
		require.NoError(t, err)
		scheduleResp.Body.Close()

		// Wait for broadcast completion - needs extra time for retries (5s delay x 2 retries = 10s+)
		t.Log("Waiting for broadcast completion (with retry delays)...")
		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
			[]string{"processed", "completed"}, 90*time.Second)
		require.NoError(t, err)

		// Fetch email from Mailpit
		t.Log("Fetching email from Mailpit...")
		msg, err := waitForEmailByRecipientAddr(t, contactEmail, 15*time.Second)
		require.NoError(t, err, "Should receive email after retries succeed")

		t.Logf("Email HTML (first 300 chars): %s", truncStr(msg.HTML, 300))

		// Verify the feed data was rendered (proves retry eventually succeeded)
		assert.Contains(t, msg.HTML, "Success Widget", "Body should contain product from successful retry")
		assert.Contains(t, msg.HTML, "$199", "Body should contain price from successful retry")

		// Verify mock server received exactly 3 requests (2 failures + 1 success)
		requests := mockServer.GetRequests()
		assert.Equal(t, 3, len(requests), "Should have 3 requests: 2 failures + 1 success")
	})

	t.Run("global_feed and recipient_feed both render correctly", func(t *testing.T) {
		err := testutil.ClearMailpitMessages(t)
		require.NoError(t, err)

		// Setup two mock servers - one for global, one for recipient
		globalServer := NewMockFeedServer()
		defer globalServer.Close()
		globalServer.SetResponse(map[string]interface{}{
			"promo_code":  "GLOBAL2026",
			"banner_text": "Winter Sale!",
		})

		recipientServer := NewMockFeedServer()
		defer recipientServer.Close()

		contactEmail := fmt.Sprintf("combined-feed-%s@example.com", uuid.New().String()[:8])
		recipientServer.SetRecipientResponses(map[string]map[string]interface{}{
			contactEmail: {
				"loyalty_points": "1500",
				"tier":           "Gold",
			},
		})

		// Create list and contact
		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName("Combined Feed List"))
		require.NoError(t, err)

		contact, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(contactEmail),
			testutil.WithContactName("Combined", "Tester"))
		require.NoError(t, err)

		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		// Create template using BOTH global_feed and recipient_feed variables
		uniqueID := uuid.New().String()[:8]
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("Combined Feed Template"),
			testutil.WithTemplateSubject(fmt.Sprintf("{{ global_feed.banner_text }} - %s", uniqueID)),
			testutil.WithTemplateEmailContent(
				"Use code {{ global_feed.promo_code }}! You have {{ recipient_feed.loyalty_points }} points ({{ recipient_feed.tier }} member)."))
		require.NoError(t, err)

		// Create broadcast with BOTH global_feed and recipient_feed enabled
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("Combined Feed Test"),
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastGlobalFeed(&domain.GlobalFeedSettings{
				Enabled: true,
				URL:     globalServer.URL(),
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastRecipientFeed(&domain.RecipientFeedSettings{
				Enabled: true,
				URL:     recipientServer.URL(),
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		t.Log("Scheduling broadcast with both feeds...")
		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcast.ID,
			"send_now":     true,
		})
		require.NoError(t, err)
		scheduleResp.Body.Close()

		t.Log("Waiting for broadcast completion...")
		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
			[]string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		// Fetch email from Mailpit
		t.Log("Fetching email from Mailpit...")
		msg, err := waitForEmailByRecipientAddr(t, contactEmail, 15*time.Second)
		require.NoError(t, err, "Should receive email with both feeds")

		t.Logf("Email subject: %s", msg.Subject)
		t.Logf("Email HTML (first 400 chars): %s", truncStr(msg.HTML, 400))

		// Verify global_feed data rendered
		assert.Contains(t, msg.Subject, "Winter Sale!", "Subject should contain global_feed banner_text")
		assert.Contains(t, msg.HTML, "GLOBAL2026", "Body should contain global_feed promo_code")

		// Verify recipient_feed data rendered
		assert.Contains(t, msg.HTML, "1500", "Body should contain recipient_feed loyalty_points")
		assert.Contains(t, msg.HTML, "Gold", "Body should contain recipient_feed tier")

		// Verify no raw Liquid syntax
		assert.NotContains(t, msg.HTML, "{{ global_feed", "Should NOT contain raw global_feed syntax")
		assert.NotContains(t, msg.HTML, "{{ recipient_feed", "Should NOT contain raw recipient_feed syntax")

		// Verify both servers received requests
		globalRequests := globalServer.GetRequests()
		recipientRequests := recipientServer.GetRequests()
		assert.GreaterOrEqual(t, len(globalRequests), 1, "Global feed server should receive at least 1 request")
		assert.Equal(t, 1, len(recipientRequests), "Recipient feed server should receive exactly 1 request")
	})

	t.Run("recipient_feed receives complete contact data in payload", func(t *testing.T) {
		err := testutil.ClearMailpitMessages(t)
		require.NoError(t, err)

		// Setup mock server to capture request payload
		mockServer := NewMockFeedServer()
		defer mockServer.Close()
		mockServer.SetResponse(map[string]interface{}{
			"verified": true,
		})

		// Create list
		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName("Payload Test List"))
		require.NoError(t, err)

		// Create contact with many fields populated
		contactEmail := fmt.Sprintf("payload-test-%s@example.com", uuid.New().String()[:8])
		contact, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(contactEmail),
			testutil.WithContactName("John", "Doe"),
			testutil.WithContactExternalID("ext-123-abc"),
			testutil.WithContactCountry("US"),
			testutil.WithContactPhone("+1-555-123-4567"),
			testutil.WithContactTimezone("America/New_York"),
			testutil.WithContactCustomString1("premium_user"),
			testutil.WithContactCustomNumber1(42.5))
		require.NoError(t, err)

		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		// Create simple template
		uniqueID := uuid.New().String()[:8]
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("Payload Test Template"),
			testutil.WithTemplateSubject(fmt.Sprintf("Payload Test - %s", uniqueID)),
			testutil.WithTemplateEmailContent("Verified: {{ recipient_feed.verified }}"))
		require.NoError(t, err)

		// Create broadcast with recipient feed
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("Payload Test Broadcast"),
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastRecipientFeed(&domain.RecipientFeedSettings{
				Enabled: true,
				URL:     mockServer.URL(),
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		t.Log("Scheduling broadcast...")
		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcast.ID,
			"send_now":     true,
		})
		require.NoError(t, err)
		scheduleResp.Body.Close()

		t.Log("Waiting for broadcast completion...")
		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
			[]string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		// Wait for email to ensure feed was fetched
		_, err = waitForEmailByRecipientAddr(t, contactEmail, 15*time.Second)
		require.NoError(t, err)

		// Verify the request payload sent to mock server
		requests := mockServer.GetRequests()
		require.Len(t, requests, 1, "Should have exactly 1 request")

		payload := requests[0].Body
		t.Logf("Request payload: %+v", payload)

		// Verify contact data in payload
		contactData, ok := payload["contact"].(map[string]interface{})
		require.True(t, ok, "Payload should contain contact object")

		assert.Equal(t, contactEmail, contactData["email"], "Should contain email")
		assert.Equal(t, "John", contactData["first_name"], "Should contain first_name")
		assert.Equal(t, "Doe", contactData["last_name"], "Should contain last_name")
		assert.Equal(t, "ext-123-abc", contactData["external_id"], "Should contain external_id")
		assert.Equal(t, "US", contactData["country"], "Should contain country")
		assert.Equal(t, "+1-555-123-4567", contactData["phone"], "Should contain phone")
		assert.Equal(t, "America/New_York", contactData["timezone"], "Should contain timezone")
		assert.Equal(t, "premium_user", contactData["custom_string_1"], "Should contain custom_string_1")
		assert.Equal(t, 42.5, contactData["custom_number_1"], "Should contain custom_number_1")

		// Verify broadcast context in payload
		broadcastData, ok := payload["broadcast"].(map[string]interface{})
		require.True(t, ok, "Payload should contain broadcast object")
		assert.Equal(t, broadcast.ID, broadcastData["id"], "Should contain broadcast ID")
		assert.Equal(t, "Payload Test Broadcast", broadcastData["name"], "Should contain broadcast name")

		// Verify list context in payload
		listData, ok := payload["list"].(map[string]interface{})
		require.True(t, ok, "Payload should contain list object")
		assert.Equal(t, list.ID, listData["id"], "Should contain list ID")
		assert.Equal(t, "Payload Test List", listData["name"], "Should contain list name")

		// Verify workspace context in payload
		workspaceData, ok := payload["workspace"].(map[string]interface{})
		require.True(t, ok, "Payload should contain workspace object")
		assert.Equal(t, workspace.ID, workspaceData["id"], "Should contain workspace ID")
	})
}

// waitForEmailByRecipientAddr polls Mailpit until an email for the given recipient is found
func waitForEmailByRecipientAddr(t *testing.T, recipientEmail string, timeout time.Duration) (*testutil.MailpitMessage, error) {
	deadline := time.Now().Add(timeout)
	httpClient := &http.Client{Timeout: 5 * time.Second}
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		resp, err := httpClient.Get("http://localhost:8025/api/v1/messages")
		if err != nil {
			t.Logf("Failed to connect to Mailpit: %v", err)
			time.Sleep(pollInterval)
			continue
		}

		var apiResp testutil.MailpitMessagesResponse
		err = json.NewDecoder(resp.Body).Decode(&apiResp)
		resp.Body.Close()

		if err != nil {
			t.Logf("Failed to decode Mailpit response: %v", err)
			time.Sleep(pollInterval)
			continue
		}

		for _, msg := range apiResp.Messages {
			for _, to := range msg.To {
				if strings.EqualFold(recipientEmail, to.Address) {
					t.Logf("Found email for recipient: %s (message ID: %s)", recipientEmail, msg.ID)
					return testutil.GetMailpitMessage(t, msg.ID)
				}
			}
		}
		time.Sleep(pollInterval)
	}
	return nil, fmt.Errorf("timeout waiting for email to %s after %v", recipientEmail, timeout)
}

// truncStr truncates a string to maxLen characters
func truncStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
