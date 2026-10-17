//go:build integration
// +build integration

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAutomationComplaintFromContactNotOnList sends real automation emails through
// Mailpit to contacts with no active row on the automation's list (non-members, who
// receive them by design, and contacts removed from the list), then reports them through
// SNS-wrapped SES notifications that name the message the way SES does: by the
// X-Message-ID header of the delivered email.
//
// A complaint or hard bounce used to record nothing that stopped the next email. The
// trigger turning them into a list status only updated existing rows, so a non-member
// got none, and the automation email gate read through GetContactListByIDs, which hides a
// removed contact's row, so a removed contact's complaint was ignored.
//
// The other writes and reads of a removed contact's row are covered here too: a one-click
// unsubscribe, the public subscribe form, and a hard bounce on an email that stores no
// list.
func TestAutomationComplaintFromContactNotOnList(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	// Automation emails are sent by the automation scheduler and the email queue worker,
	// which the harness only runs on request.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	client := suite.APIClient
	factory := suite.DataFactory
	bg := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	// Absolute links, so the delivered List-Unsubscribe URL can be used as a mail client does
	require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, suite.ServerManager.GetURL()))

	// Emails go out through SMTP to Mailpit; the SES integration only receives the
	// notifications, which name the message, not the integration that sent it.
	ses, err := factory.CreateSESIntegration(workspace.ID)
	require.NoError(t, err)
	webhookURL := fmt.Sprintf("/webhooks/email?provider=ses&workspace_id=%s&integration_id=%s", workspace.ID, ses.ID)

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	contactRepo := suite.ServerManager.GetApp().GetContactRepository()

	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	require.Equal(t, "marketing", template.Category, "only marketing and blog emails check the list status")

	automationID := shortuuid.New()
	eventName := "complaint_not_on_list_" + uuid.New().String()[:8]
	createActiveAutomation(t, client, workspace.ID, automationID, list.ID, eventName, template.ID)

	post := func(t *testing.T, path string, body map[string]interface{}) {
		t.Helper()
		resp, err := client.Post(path, body)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", path, raw)
	}

	// newContact creates a contact with no row on the list or, when removed is set, one
	// added to it and then removed through the API.
	newContact := func(t *testing.T, name string, removed bool) string {
		t.Helper()
		email := fmt.Sprintf("not-on-list-%s-%s@example.com", name, uuid.New().String()[:8])
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		if removed {
			_, err = factory.CreateContactList(workspace.ID,
				testutil.WithContactListEmail(email),
				testutil.WithContactListListID(list.ID),
				testutil.WithContactListStatus(domain.ContactListStatusActive))
			require.NoError(t, err)
			post(t, "/api/contactLists.removeContact", map[string]interface{}{
				"workspace_id": workspace.ID, "email": email, "list_id": list.ID,
			})
		}
		return email
	}

	// deliver fires the automation's event and returns the email Mailpit received and the
	// Notifuse message ID it carries.
	deliver := func(t *testing.T, email string) (*testutil.MailpitMessage, string) {
		t.Helper()
		require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, eventName, map[string]interface{}{}))
		msg, err := waitForEmailAndGetMessage(t, email, 20*time.Second)
		require.NoError(t, err, "the automation email should reach Mailpit")
		messageID := deliveredHeader(t, msg, "X-Message-ID")

		// An SMTP send records its message_history row only once the server accepted it
		testutil.WaitForCondition(t, func() bool {
			var n int
			_ = db.QueryRowContext(bg, `SELECT COUNT(*) FROM message_history WHERE id = $1`, messageID).Scan(&n)
			return n == 1
		}, 10*time.Second, "waiting for the message_history row of "+messageID)
		return msg, messageID
	}

	type listRow struct {
		status  domain.ContactListStatus
		removed bool
	}
	rowOf := func(t *testing.T, email string) *listRow {
		t.Helper()
		var status string
		var deletedAt sql.NullTime
		err := db.QueryRowContext(bg,
			`SELECT status, deleted_at FROM contact_lists WHERE email = $1 AND list_id = $2`,
			email, list.ID).Scan(&status, &deletedAt)
		if err == sql.ErrNoRows {
			return nil
		}
		require.NoError(t, err)
		return &listRow{status: domain.ContactListStatus(status), removed: deletedAt.Valid}
	}

	// sendAgain fires the event once more and returns the new enrolment once it exits.
	sendAgain := func(t *testing.T, email string) *domain.ContactAutomation {
		t.Helper()
		require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, eventName, map[string]interface{}{}))
		// The first enrolment completed, so only the new one can report exited
		return waitForAutomationStatus(t, factory, workspace.ID, automationID, email, domain.ContactAutomationStatusExited, 15*time.Second)
	}

	// inBroadcastAudience reports whether a broadcast to the list would email the address,
	// with exclude_unsubscribed off: the widest audience a broadcast can have.
	inBroadcastAudience := func(t *testing.T, email string) bool {
		t.Helper()
		recipients, err := contactRepo.GetContactsForBroadcast(bg, workspace.ID,
			domain.AudienceSettings{List: list.ID, ExcludeUnsubscribed: false}, 1000, "")
		require.NoError(t, err)
		for _, r := range recipients {
			if r.Contact != nil && strings.EqualFold(r.Contact.Email, email) {
				return true
			}
		}
		return false
	}

	// expectSkipped checks the automation skips its next email, then adds the contact back
	// to the list through the API, as a workspace might. The status survives it, so the
	// list's broadcasts don't email them either; a contact with no status on the list
	// would come back an active member. Until then, the contact is in no broadcast
	// audience either way, being off the list.
	expectSkipped := func(t *testing.T, email string, reason domain.ContactListStatus) {
		t.Helper()
		ca := sendAgain(t, email)
		require.NotNil(t, ca.ExitReason)
		assert.Equal(t, string(reason), *ca.ExitReason, "the next email is skipped, not sent")
		assert.Equal(t, 1, deliveredCountTo(t, email), "only the first email was delivered")

		post(t, "/api/lists.subscribe", map[string]interface{}{
			"workspace_id": workspace.ID,
			"contact":      map[string]interface{}{"email": email},
			"list_ids":     []string{list.ID},
		})
		row := rowOf(t, email)
		require.NotNil(t, row)
		assert.Equal(t, reason, row.status, "adding the contact back to the list keeps the status")
		assert.False(t, inBroadcastAudience(t, email), "the list's broadcasts don't email them either")
	}

	complain := func(t *testing.T, messageID, email string) {
		t.Helper()
		postSESNotification(t, client, webhookURL, sesComplaintPayload(messageID, email))
	}
	hardBounce := func(t *testing.T, messageID, email string) {
		t.Helper()
		postSESNotification(t, client, webhookURL,
			sesBouncePayload(messageID, email, "Permanent", "General", "smtp; 550 5.1.1 user unknown"))
	}

	t.Run("contact not on the list: a complaint records complained there and stops the next email", func(t *testing.T) {
		email := newContact(t, "complaint", false)
		_, messageID := deliver(t, email)
		require.Nil(t, rowOf(t, email), "a non-member has no row before the complaint")

		complain(t, messageID, email)

		row := rowOf(t, email)
		require.NotNil(t, row, "the complaint records a row on the list the email was sent for")
		assert.Equal(t, domain.ContactListStatusComplained, row.status)
		assert.False(t, row.removed)
		assert.Equal(t, 1, b24TimelineInserts(t, db, email, "list.complained", list.ID),
			"the new row is on the timeline as the insert it is")
		expectSkipped(t, email, domain.ContactListStatusComplained)
	})

	t.Run("contact removed from the list: a complaint is kept on the removed row and stops the next email", func(t *testing.T) {
		email := newContact(t, "removed-complaint", true)
		_, messageID := deliver(t, email)

		complain(t, messageID, email)

		row := rowOf(t, email)
		require.NotNil(t, row)
		assert.Equal(t, domain.ContactListStatusComplained, row.status)
		assert.True(t, row.removed, "a complaint doesn't make a removed contact a member again")
		expectSkipped(t, email, domain.ContactListStatusComplained)
	})

	t.Run("contact not on the list: a hard bounce records bounced there and stops the next email", func(t *testing.T) {
		email := newContact(t, "bounce", false)
		_, messageID := deliver(t, email)

		hardBounce(t, messageID, email)

		row := rowOf(t, email)
		require.NotNil(t, row, "the bounce records a row on the list the email was sent for")
		assert.Equal(t, domain.ContactListStatusBounced, row.status)
		assert.False(t, row.removed)
		expectSkipped(t, email, domain.ContactListStatusBounced)
	})

	t.Run("contact removed from the list: a hard bounce is kept on the removed row and stops the next email", func(t *testing.T) {
		email := newContact(t, "removed-bounce", true)
		_, messageID := deliver(t, email)

		hardBounce(t, messageID, email)

		row := rowOf(t, email)
		require.NotNil(t, row)
		assert.Equal(t, domain.ContactListStatusBounced, row.status)
		assert.True(t, row.removed)
		expectSkipped(t, email, domain.ContactListStatusBounced)
	})

	// A hard bounce marks the address bounced on every list the contact has a row on,
	// whatever email it came from. MarkEmailsAsBounced skipped removed rows, which the
	// automation email gate reads, so the list's automation kept emailing the address.
	t.Run("contact removed from the list: a hard bounce on another email marks the removed row bounced too", func(t *testing.T) {
		email := newContact(t, "removed-other-bounce", true)
		// Stored the way a transactional email is, with no list, so the bounce reaches the
		// list rows through the address alone
		messageID := uuid.New().String()
		_, err := db.ExecContext(bg, `INSERT INTO message_history (id, contact_email, template_id,
				template_version, channel, message_data, sent_at)
			VALUES ($1, $2, 'tpl-other', 1, 'email', '{}'::jsonb, NOW())`, messageID, email)
		require.NoError(t, err)

		hardBounce(t, messageID, email)

		assert.Equal(t, &listRow{status: domain.ContactListStatusBounced, removed: true}, rowOf(t, email),
			"the removed row takes the bounce and stays removed")
		ca := sendAgain(t, email)
		require.NotNil(t, ca.ExitReason)
		assert.Equal(t, string(domain.ContactListStatusBounced), *ca.ExitReason, "the automation's email is skipped, not sent")
		assert.Zero(t, deliveredCountTo(t, email))
	})

	// Mail clients often send the one-click unsubscribe right after a spam report, and a
	// workspace may remove a contact who complained. Neither may erase the complaint.
	t.Run("member who complained, then was removed: still skipped, and a one-click unsubscribe keeps the complaint", func(t *testing.T) {
		email := newContact(t, "member-then-removed", false)
		_, err := factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		msg, messageID := deliver(t, email)

		complain(t, messageID, email)
		require.Equal(t, &listRow{status: domain.ContactListStatusComplained}, rowOf(t, email))

		post(t, "/api/contactLists.removeContact", map[string]interface{}{
			"workspace_id": workspace.ID, "email": email, "list_id": list.ID,
		})
		require.Equal(t, &listRow{status: domain.ContactListStatusComplained, removed: true}, rowOf(t, email))

		resp := requestListUnsubscribeURL(t, http.MethodPost, listUnsubscribeURL(t, msg),
			"application/x-www-form-urlencoded", strings.NewReader("List-Unsubscribe=One-Click"))
		require.Equal(t, http.StatusOK, resp.StatusCode)

		assert.Equal(t, &listRow{status: domain.ContactListStatusComplained, removed: true}, rowOf(t, email),
			"the unsubscribe neither replaces the complaint nor restores the contact")
		expectSkipped(t, email, domain.ContactListStatusComplained)
	})

	// The opt-out used to clear the removal too, putting the contact back on the list as
	// unsubscribed: in its "in list" segments, and in every broadcast that includes
	// unsubscribed contacts.
	t.Run("contact removed from the list: a one-click unsubscribe records the opt-out and keeps them removed", func(t *testing.T) {
		email := newContact(t, "removed-unsubscribe", true)
		msg, _ := deliver(t, email)
		require.Equal(t, &listRow{status: domain.ContactListStatusActive, removed: true}, rowOf(t, email))

		resp := requestListUnsubscribeURL(t, http.MethodPost, listUnsubscribeURL(t, msg),
			"application/x-www-form-urlencoded", strings.NewReader("List-Unsubscribe=One-Click"))
		require.Equal(t, http.StatusOK, resp.StatusCode)

		assert.Equal(t, &listRow{status: domain.ContactListStatusUnsubscribed, removed: true}, rowOf(t, email),
			"the opt-out is recorded, and the contact is not put back on the list")
		assert.False(t, inBroadcastAudience(t, email),
			"a broadcast that includes unsubscribed contacts doesn't email them")
		ca := sendAgain(t, email)
		require.NotNil(t, ca.ExitReason)
		assert.Equal(t, string(domain.ContactListStatusUnsubscribed), *ca.ExitReason, "the next email is skipped, not sent")
		assert.Equal(t, 1, deliveredCountTo(t, email), "only the first email was delivered")
	})

	// The subscribe flow read the row through GetContactListByIDs, which hides a removed
	// contact's row, so anyone could submit the address of a contact removed after a
	// complaint on the public form and have a confirmation email sent to it.
	t.Run("contact removed from a double opt-in list after a complaint: refused as a member is, with no confirmation email", func(t *testing.T) {
		subject := "confirm-" + uuid.New().String()[:8]
		confirmation, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateSubject(subject))
		require.NoError(t, err)
		doiList, err := factory.CreateList(workspace.ID, testutil.WithListPublic(true), testutil.WithListDoubleOptin(true),
			func(l *domain.List) {
				l.DoubleOptInTemplate = &domain.TemplateReference{ID: confirmation.ID, Version: confirmation.Version}
			})
		require.NoError(t, err)

		complained := newContact(t, "removed-complained-form", false)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(complained),
			testutil.WithContactListListID(doiList.ID),
			testutil.WithContactListStatus(domain.ContactListStatusComplained))
		require.NoError(t, err)
		post(t, "/api/contactLists.removeContact", map[string]interface{}{
			"workspace_id": workspace.ID, "email": complained, "list_id": doiList.ID,
		})
		// A contact with no row on the list, who does get the confirmation email
		control := newContact(t, "form-control", false)

		submitForm := func(t *testing.T, email string) {
			t.Helper()
			body, err := json.Marshal(map[string]interface{}{
				"workspace_id": workspace.ID,
				"contact":      map[string]interface{}{"email": email},
				"list_ids":     []string{doiList.ID},
			})
			require.NoError(t, err)
			resp, err := (&http.Client{Timeout: 10 * time.Second}).Post(
				suite.ServerManager.GetURL()+"/subscribe", "application/json", bytes.NewReader(body))
			require.NoError(t, err)
			raw, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, "/subscribe: %s", raw)
		}
		submitForm(t, complained)
		submitForm(t, control)

		// The form sends the confirmation before it answers, so once the control's email is
		// in, one sent to the complained address before it would be too
		_, err = waitForEmailAndGetMessage(t, control, 20*time.Second)
		require.NoError(t, err, "the form sends the confirmation email to a new subscriber")
		assert.Zero(t, deliveredCountTo(t, complained), "no confirmation email to an address that complained")

		var status string
		var deletedAt sql.NullTime
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT status, deleted_at FROM contact_lists WHERE email = $1 AND list_id = $2`,
			complained, doiList.ID).Scan(&status, &deletedAt))
		assert.Equal(t, string(domain.ContactListStatusComplained), status)
		assert.True(t, deletedAt.Valid, "the contact stays removed")

		// The API reports the refusal, as it does for a member who complained
		resp, err := client.Post("/api/lists.subscribe", map[string]interface{}{
			"workspace_id": workspace.ID,
			"contact":      map[string]interface{}{"email": complained},
			"list_ids":     []string{doiList.ID},
		})
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "lists.subscribe: %s", raw)
		var subscribed struct {
			ContactLists []struct {
				Status string `json:"status"`
			} `json:"contact_lists"`
		}
		require.NoError(t, json.Unmarshal(raw, &subscribed))
		require.Len(t, subscribed.ContactLists, 1)
		assert.Equal(t, string(domain.ContactListStatusComplained), subscribed.ContactLists[0].Status)
	})

	t.Run("contact erased after delivery: a complaint stores nothing", func(t *testing.T) {
		email := newContact(t, "erased", false)
		_, messageID := deliver(t, email)
		post(t, "/api/contacts.delete", map[string]interface{}{"workspace_id": workspace.ID, "email": email})

		complain(t, messageID, email)

		var n int
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT COUNT(*) FROM contact_lists WHERE email = $1 OR email = 'DELETED_EMAIL'`, email).Scan(&n))
		assert.Zero(t, n, "neither the erased address nor its redacted stand-in is stored")
	})
}

// TestStatusChangeTriggerRecordsTheListRow drives update_contact_lists_on_status_change
// with direct writes to message_history, as SetStatusesIfNotSet makes them, to pin which
// list a complaint or hard bounce lands on and when a row is created. The last subtest
// checks that v42 installs the function on a workspace that has an older one.
func TestStatusChangeTriggerRecordsTheListRow(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		cfg.Webhook.AllowPrivateDeliveryHosts = true
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory
	bg := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	other, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	subscriptionID := createZapierSubscription(t, suite, workspace.ID, "Status trigger",
		"https://hooks.example.com/status-trigger", []string{"list.complained", "list.bounced"}, []string{list.ID}, nil)

	exec := func(t *testing.T, query string, args ...interface{}) {
		t.Helper()
		_, err := db.ExecContext(bg, query, args...)
		require.NoError(t, err)
	}
	newContact := func(t *testing.T) string {
		t.Helper()
		email := fmt.Sprintf("status-trigger-%s@example.com", uuid.New().String()[:8])
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		return email
	}
	addRow := func(t *testing.T, email, listID string, status domain.ContactListStatus) {
		t.Helper()
		_, err := factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(listID),
			testutil.WithContactListStatus(status))
		require.NoError(t, err)
	}
	newAutomation := func(t *testing.T, listID string) string {
		t.Helper()
		id := shortuuid.New()
		exec(t, `INSERT INTO automations (id, workspace_id, name, status, list_id, trigger_config)
			VALUES ($1, $2, 'Status trigger', 'draft', $3, '{}'::jsonb)`, id, workspace.ID, listID)
		return id
	}
	// message stores a sent email as the queue worker does: a broadcast records its list,
	// and so does an automation email since 42; one sent before recorded only the
	// automation.
	message := func(t *testing.T, email string, listID, automationID interface{}) string {
		t.Helper()
		id := uuid.New().String()
		exec(t, `INSERT INTO message_history (id, contact_email, automation_id, list_id, template_id,
				template_version, channel, message_data, sent_at)
			VALUES ($1, $2, $3, $4, 'tpl-status', 1, 'email', '{}'::jsonb, NOW())`,
			id, email, automationID, listID)
		return id
	}
	complain := func(t *testing.T, messageID string) {
		t.Helper()
		exec(t, `UPDATE message_history SET complained_at = NOW() WHERE id = $1 AND complained_at IS NULL`, messageID)
	}
	bounce := func(t *testing.T, messageID string) {
		t.Helper()
		exec(t, `UPDATE message_history SET bounced_at = NOW() WHERE id = $1 AND bounced_at IS NULL`, messageID)
	}
	type listRow struct {
		status  string
		removed bool
	}
	rowOf := func(t *testing.T, email, listID string) *listRow {
		t.Helper()
		var r listRow
		var deletedAt sql.NullTime
		err := db.QueryRowContext(bg, `SELECT status, deleted_at FROM contact_lists WHERE email = $1 AND list_id = $2`,
			email, listID).Scan(&r.status, &deletedAt)
		if err == sql.ErrNoRows {
			return nil
		}
		require.NoError(t, err)
		r.removed = deletedAt.Valid
		return &r
	}
	rows := func(t *testing.T, email string) int {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRowContext(bg, `SELECT COUNT(*) FROM contact_lists WHERE email = $1`, email).Scan(&n))
		return n
	}

	t.Run("message stored with a list, no row: a complained row is created, with its timeline row and webhook", func(t *testing.T) {
		email := newContact(t)
		complain(t, message(t, email, list.ID, nil))

		assert.Equal(t, &listRow{status: "complained"}, rowOf(t, email, list.ID))
		assert.Equal(t, 1, rows(t, email))
		assert.Equal(t, 1, b24TimelineInserts(t, db, email, "list.complained", list.ID))
		deliveries := b24Deliveries(t, db, subscriptionID, email)
		require.Len(t, deliveries, 1)
		assert.Equal(t, "list.complained", deliveries[0].eventType)
		prev, present := deliveries[0].payload["previous_status"]
		assert.True(t, present)
		assert.Nil(t, prev, "a new row has no previous status")
	})

	// The automation's list may have changed since an email sent before 42, so the
	// complaint could land on a list the email was never sent for.
	t.Run("automation message sent before 42, stored without a list: nothing is recorded, on the automation's list or any", func(t *testing.T) {
		member := newContact(t)
		addRow(t, member, list.ID, domain.ContactListStatusActive)
		nonMember := newContact(t)
		automationID := newAutomation(t, list.ID)

		complain(t, message(t, member, nil, automationID))
		bounce(t, message(t, nonMember, nil, automationID))

		assert.Equal(t, &listRow{status: "active"}, rowOf(t, member, list.ID))
		assert.Equal(t, 1, rows(t, member))
		assert.Zero(t, rows(t, nonMember))
	})

	t.Run("an automation email records the list stored on it, not the automation's current one", func(t *testing.T) {
		email := newContact(t)
		complain(t, message(t, email, list.ID, newAutomation(t, other.ID)))

		assert.Equal(t, &listRow{status: "complained"}, rowOf(t, email, list.ID))
		assert.Nil(t, rowOf(t, email, other.ID))
	})

	t.Run("existing rows are updated as before: an active member, a removed contact", func(t *testing.T) {
		member := newContact(t)
		addRow(t, member, list.ID, domain.ContactListStatusActive)
		removed := newContact(t)
		addRow(t, removed, list.ID, domain.ContactListStatusActive)
		exec(t, `UPDATE contact_lists SET deleted_at = NOW() WHERE email = $1 AND list_id = $2`, removed, list.ID)

		complain(t, message(t, member, list.ID, nil))
		bounce(t, message(t, removed, list.ID, nil))

		assert.Equal(t, &listRow{status: "complained"}, rowOf(t, member, list.ID))
		assert.Equal(t, &listRow{status: "bounced", removed: true}, rowOf(t, removed, list.ID),
			"the removed row takes the bounce and stays removed")
		deliveries := b24Deliveries(t, db, subscriptionID, member)
		require.Len(t, deliveries, 1)
		assert.Equal(t, "active", deliveries[0].payload["previous_status"])
	})

	t.Run("a bounce never replaces a complaint, a complaint replaces a bounce", func(t *testing.T) {
		complainedFirst := newContact(t)
		messageID := message(t, complainedFirst, list.ID, nil)
		complain(t, messageID)
		bounce(t, messageID)
		assert.Equal(t, &listRow{status: "complained"}, rowOf(t, complainedFirst, list.ID))

		bouncedFirst := newContact(t)
		messageID = message(t, bouncedFirst, list.ID, nil)
		bounce(t, messageID)
		assert.Equal(t, &listRow{status: "bounced"}, rowOf(t, bouncedFirst, list.ID))
		complain(t, messageID)
		assert.Equal(t, &listRow{status: "complained"}, rowOf(t, bouncedFirst, list.ID))

		both := newContact(t)
		messageID = message(t, both, list.ID, nil)
		exec(t, `UPDATE message_history SET complained_at = NOW(), bounced_at = NOW() WHERE id = $1`, messageID)
		assert.Equal(t, &listRow{status: "complained"}, rowOf(t, both, list.ID))
	})

	t.Run("nothing is created for an address that is not a contact, on a deleted list, or for no list", func(t *testing.T) {
		stranger := fmt.Sprintf("not-a-contact-%s@example.com", uuid.New().String()[:8])
		complain(t, message(t, stranger, list.ID, nil))
		assert.Zero(t, rows(t, stranger))

		deleted, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		exec(t, `UPDATE lists SET deleted_at = NOW() WHERE id = $1`, deleted.ID)
		onDeleted := newContact(t)
		complain(t, message(t, onDeleted, deleted.ID, nil))
		assert.Zero(t, rows(t, onDeleted))

		noList := newContact(t)
		complain(t, message(t, noList, nil, newAutomation(t, "")))
		complain(t, message(t, noList, nil, nil))
		assert.Zero(t, rows(t, noList))
	})

	t.Run("an open or a delivery records nothing", func(t *testing.T) {
		email := newContact(t)
		messageID := message(t, email, list.ID, nil)
		exec(t, `UPDATE message_history SET opened_at = NOW(), delivered_at = NOW() WHERE id = $1`, messageID)
		assert.Zero(t, rows(t, email))
	})

	// Last: it swaps the installed function for a stand-in.
	t.Run("v42 installs the function on an existing workspace", func(t *testing.T) {
		// Stands for the pre-42 body, which never created a row
		exec(t, `CREATE OR REPLACE FUNCTION update_contact_lists_on_status_change()
			RETURNS TRIGGER AS $$ BEGIN RETURN NEW; END; $$ LANGUAGE plpgsql`)
		before := newContact(t)
		complain(t, message(t, before, list.ID, nil))
		require.Zero(t, rows(t, before), "the stand-in is installed")

		// Each workspace migration runs in its own transaction, which v42's savepoints need
		tx, err := db.BeginTx(bg, nil)
		require.NoError(t, err)
		if mErr := (&migrations.V42Migration{}).UpdateWorkspace(bg, &config.Config{}, workspace, tx); mErr != nil {
			_ = tx.Rollback()
			t.Fatalf("v42 UpdateWorkspace failed: %v", mErr)
		}
		require.NoError(t, tx.Commit())

		after := newContact(t)
		complain(t, message(t, after, list.ID, newAutomation(t, list.ID)))
		assert.Equal(t, &listRow{status: "complained"}, rowOf(t, after, list.ID))
	})
}
