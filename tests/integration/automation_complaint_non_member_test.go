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
		assert.Zero(t, timelineKindCount(t, db, email, "list.complained", list.ID),
			"a removed contact is not on the list, so the status on their row is no list event")
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
		assert.Zero(t, timelineKindCount(t, db, email, "list.bounced", list.ID),
			"a removed contact is not on the list, so the status on their row is no list event")
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
		assert.Zero(t, timelineKindCount(t, db, email, "list.unsubscribed", list.ID),
			"a removed contact is not on the list, so the status on their row is no list event")
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

// TestMessageStatusRecordsTheListRow drives apply_message_status_to_contact_lists, which
// track_message_history_changes runs, with direct writes to message_history, as
// SetStatusesIfNotSet makes them. It pins which list a complaint or hard bounce lands on,
// when a row is created, that an automation triggered by the email entry sees the new list
// status, and that a complaint racing an import of the same contact lands without a
// deadlock. The last subtest checks the same on a workspace upgraded by v42, which keeps the
// trigger that set the status before 42.
func TestMessageStatusRecordsTheListRow(t *testing.T) {
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
	// messageHistoryTriggers lists the triggers of message_history in the order PostgreSQL
	// fires the AFTER triggers of one event: by name, compared byte by byte.
	messageHistoryTriggers := func(t *testing.T) []string {
		t.Helper()
		r, err := db.QueryContext(bg, `SELECT tgname FROM pg_trigger
			WHERE tgrelid = 'message_history'::regclass AND NOT tgisinternal ORDER BY tgname COLLATE "C"`)
		require.NoError(t, err)
		defer func() { _ = r.Close() }()
		var names []string
		for r.Next() {
			var name string
			require.NoError(t, r.Scan(&name))
			names = append(names, name)
		}
		require.NoError(t, r.Err())
		return names
	}

	// The probe records, for each email.complained and email.bounced timeline entry, the
	// contact's status on the message's list when the entry was written. An automation
	// triggered by those entries evaluates its conditions at that moment: its trigger is an
	// AFTER INSERT trigger on contact_timeline, as this one is. Like the gate below, it goes
	// with the workspace database when the suite cleans up.
	exec(t, `CREATE TABLE status_seen_by_email_entries (email TEXT, kind TEXT, status TEXT)`)
	exec(t, `CREATE FUNCTION record_status_seen_by_email_entry() RETURNS TRIGGER AS $$
		BEGIN
			INSERT INTO status_seen_by_email_entries (email, kind, status)
			SELECT NEW.email, NEW.kind, (SELECT cl.status FROM message_history m
				JOIN contact_lists cl ON cl.email = m.contact_email AND cl.list_id = m.list_id
				WHERE m.id = NEW.entity_id);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`)
	exec(t, `CREATE TRIGGER record_status_seen_by_email_entry AFTER INSERT ON contact_timeline FOR EACH ROW
		WHEN (NEW.kind IN ('email.complained', 'email.bounced')) EXECUTE FUNCTION record_status_seen_by_email_entry()`)
	statusSeenByEmailEntries := func(t *testing.T, email string) []string {
		t.Helper()
		r, err := db.QueryContext(bg, `SELECT kind || ': ' || COALESCE(status, 'no row')
			FROM status_seen_by_email_entries WHERE email = $1 ORDER BY kind`, email)
		require.NoError(t, err)
		defer func() { _ = r.Close() }()
		var seen []string
		for r.Next() {
			var s string
			require.NoError(t, r.Scan(&s))
			seen = append(seen, s)
		}
		require.NoError(t, r.Err())
		return seen
	}

	// The gate holds an import between its two rows: status_trigger_test_gate takes an
	// advisory lock the test holds when it reaches the contact named by its second argument.
	exec(t, `CREATE OR REPLACE FUNCTION status_trigger_test_gate(email text, held text) RETURNS text AS $$
		BEGIN
			IF email = held THEN
				PERFORM pg_advisory_xact_lock(hashtext('status_trigger_test_gate'));
			END IF;
			RETURN email;
		END;
		$$ LANGUAGE plpgsql VOLATILE`)

	// An import writes a batch of rows in one statement, and each row's triggers run at the
	// end of it. The first inserts new rows; the second ends with the ON CONFLICT clause of
	// BulkAddContactsToLists (internal/repository/contact_list_postgres.go), copied here to
	// put the gate in, so it subscribes a contact who already has a row as an import does.
	// $1 and $2 are the two contacts, $3 the one the gate holds the import before, $4 the
	// list.
	const importNewRows = `INSERT INTO contact_lists (email, list_id, status)
		SELECT status_trigger_test_gate(t.email, $3), $4, 'active'
		FROM unnest(ARRAY[$1, $2]::text[]) WITH ORDINALITY AS t(email, n)
		ORDER BY t.n`
	const importUpsert = importNewRows + `
		ON CONFLICT (email, list_id) DO UPDATE
		SET status = EXCLUDED.status, updated_at = EXCLUDED.updated_at, deleted_at = NULL
		WHERE contact_lists.status NOT IN ('unsubscribed', 'bounced', 'complained')`

	// raceWithImport records a complaint or a hard bounce (status is "complained" or
	// "bounced") on messageID, a message to importing, while an import of importing and then
	// next is held between its two rows, so the import holds importing's row uncommitted
	// while the complaint or bounce runs. Before 42 the complaint or bounce locked the
	// contact's contact_segment_queue row (through the message's timeline entry) before its
	// contact_lists row, the reverse of every contact_lists write: the import then waited for
	// the queue row while the complaint waited for the list row, and PostgreSQL aborted one
	// of the two. Now the complaint or bounce waits for the import, and both land.
	raceWithImport := func(t *testing.T, importSQL, status, importing, next, messageID string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(bg, 30*time.Second)
		defer cancel()
		// The connections go back to the pool before the assertions below read from it
		var conns []*sql.Conn
		closeConns := func() {
			for _, c := range conns {
				_ = c.Close()
			}
			conns = nil
		}
		defer closeConns()
		conn := func() (*sql.Conn, int) {
			c, err := db.Conn(ctx)
			require.NoError(t, err)
			conns = append(conns, c)
			var pid int
			require.NoError(t, c.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
			return c, pid
		}
		gate, _ := conn()
		importer, importerPID := conn()
		reporter, reporterPID := conn()

		_, err := gate.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext('status_trigger_test_gate'))`)
		require.NoError(t, err)
		gateOpen := false
		openGate := func() {
			if !gateOpen {
				gateOpen = true
				_, err := gate.ExecContext(bg, `SELECT pg_advisory_unlock(hashtext('status_trigger_test_gate'))`)
				require.NoError(t, err)
			}
		}
		defer openGate()
		waitsOn := func(pid int, event string) {
			t.Helper()
			require.Eventually(t, func() bool {
				var waiting bool
				err := gate.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
					WHERE pid = $1 AND wait_event_type = 'Lock' AND wait_event = $2)`, pid, event).Scan(&waiting)
				return err == nil && waiting
			}, 10*time.Second, 20*time.Millisecond, "backend %d waiting on %s", pid, event)
		}

		importDone := make(chan error, 1)
		go func() {
			tx, err := importer.BeginTx(ctx, nil)
			if err != nil {
				importDone <- err
				return
			}
			if _, err := tx.ExecContext(ctx, importSQL, importing, next, next, list.ID); err != nil {
				_ = tx.Rollback()
				importDone <- err
				return
			}
			importDone <- tx.Commit()
		}()
		waitsOn(importerPID, "advisory")

		column := map[string]string{"complained": "complained_at", "bounced": "bounced_at"}[status]
		require.NotEmpty(t, column, "status is complained or bounced")
		reportDone := make(chan error, 1)
		go func() {
			_, err := reporter.ExecContext(ctx, `UPDATE message_history SET `+column+` = NOW() WHERE id = $1`, messageID)
			reportDone <- err
		}()
		// The complaint or bounce waits for the import's transaction, on the contact's row
		waitsOn(reporterPID, "transactionid")

		openGate()
		require.NoError(t, <-importDone, "the import")
		require.NoError(t, <-reportDone, "the "+status+" report")
		closeConns()

		assert.Equal(t, &listRow{status: status}, rowOf(t, importing, list.ID),
			"the status lands on the row the import committed")
		assert.Equal(t, &listRow{status: "active"}, rowOf(t, next, list.ID))
		kind := map[string]string{"complained": "email.complained", "bounced": "email.bounced"}[status]
		assert.Equal(t, []string{kind + ": " + status}, statusSeenByEmailEntries(t, importing))
	}
	// raceWithImportOfANewRow races a complaint or a bounce with an import of a contact who
	// has no row, raceWithImportOfAPendingRow a complaint with an import that subscribes a
	// contact whose row is pending, as a double opt-in list leaves it.
	raceWithImportOfANewRow := func(status string) func(t *testing.T) {
		return func(t *testing.T) {
			importing, next := newContact(t), newContact(t)
			raceWithImport(t, importNewRows, status, importing, next, message(t, importing, list.ID, nil))
		}
	}
	raceWithImportOfAPendingRow := func(t *testing.T) {
		importing, next := newContact(t), newContact(t)
		addRow(t, importing, list.ID, domain.ContactListStatusPending)
		raceWithImport(t, importUpsert, "complained", importing, next, message(t, importing, list.ID, nil))
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

	// A removed contact is not on the list: their row keeps the status for the email gate,
	// and a consumer that mirrors the list is not told of it.
	t.Run("existing rows take the status: a member's change is reported, a removed contact's is not", func(t *testing.T) {
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
		assert.Equal(t, 1, timelineKindCount(t, db, member, "list.complained", list.ID))

		assert.Empty(t, b24Deliveries(t, db, subscriptionID, removed), "no webhook event for a removed contact")
		assert.Zero(t, timelineKindCount(t, db, removed, "list.bounced", list.ID), "nor a timeline row")
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

	// MarkEmailsAsBounced bounces the address on every list after a hard bounce, the lists
	// it was removed from included. It must leave a complaint in place there too.
	t.Run("a hard bounce on every list keeps complaints, on a list the contact was removed from too", func(t *testing.T) {
		third, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		email := newContact(t)
		addRow(t, email, list.ID, domain.ContactListStatusComplained)
		addRow(t, email, other.ID, domain.ContactListStatusComplained)
		exec(t, `UPDATE contact_lists SET deleted_at = NOW() WHERE email = $1 AND list_id = $2`, email, other.ID)
		addRow(t, email, third.ID, domain.ContactListStatusActive)

		require.NoError(t, suite.ServerManager.GetApp().GetContactRepository().
			MarkEmailsAsBounced(bg, workspace.ID, []string{email}, time.Now().UTC()))

		assert.Equal(t, &listRow{status: "complained"}, rowOf(t, email, list.ID))
		assert.Equal(t, &listRow{status: "complained", removed: true}, rowOf(t, email, other.ID))
		assert.Equal(t, &listRow{status: "bounced"}, rowOf(t, email, third.ID))
	})

	// A separate trigger set the status before 42, after the timeline entries; see
	// schema.TrackMessageHistoryChangesFunction.
	t.Run("a new workspace sets the status from the timeline trigger, with no trigger of its own", func(t *testing.T) {
		assert.Equal(t, []string{"message_history_changes_trigger", "webhook_message_history"}, messageHistoryTriggers(t))
	})

	t.Run("an automation triggered by the email entry sees the new list status", func(t *testing.T) {
		complainer := newContact(t)
		addRow(t, complainer, list.ID, domain.ContactListStatusActive)
		complain(t, message(t, complainer, list.ID, nil))
		assert.Equal(t, []string{"email.complained: complained"}, statusSeenByEmailEntries(t, complainer))

		bouncer := newContact(t)
		bounce(t, message(t, bouncer, list.ID, nil))
		assert.Equal(t, []string{"email.bounced: bounced"}, statusSeenByEmailEntries(t, bouncer),
			"a contact with no row has one by then")
	})

	t.Run("a complaint racing an import of a contact with no row waits for it, and both land", raceWithImportOfANewRow("complained"))

	t.Run("a hard bounce racing an import of a contact with no row waits for it, and both land", raceWithImportOfANewRow("bounced"))

	t.Run("a complaint racing an import that subscribes a pending contact waits for it, and both land", raceWithImportOfAPendingRow)

	// Last: it swaps the installed functions for stand-ins, and attaches the trigger that
	// set the status before 42.
	t.Run("v42 makes a workspace with the pre-42 trigger record the status the same way, keeping its triggers", func(t *testing.T) {
		// Stand for the functions before 42: the timeline function set no status, and the
		// status function created no row. The shared function did not exist.
		exec(t, `CREATE OR REPLACE FUNCTION track_message_history_changes()
			RETURNS TRIGGER AS $$ BEGIN RETURN NEW; END; $$ LANGUAGE plpgsql`)
		exec(t, `DROP FUNCTION apply_message_status_to_contact_lists(VARCHAR, VARCHAR, TIMESTAMPTZ, TIMESTAMPTZ)`)
		exec(t, `CREATE OR REPLACE FUNCTION update_contact_lists_on_status_change()
			RETURNS TRIGGER AS $$ BEGIN RETURN NEW; END; $$ LANGUAGE plpgsql`)
		exec(t, `CREATE TRIGGER message_history_status_trigger AFTER UPDATE ON message_history
			FOR EACH ROW EXECUTE FUNCTION update_contact_lists_on_status_change()`)
		pre42 := []string{"message_history_changes_trigger", "message_history_status_trigger", "webhook_message_history"}
		require.Equal(t, pre42, messageHistoryTriggers(t))
		before := newContact(t)
		complain(t, message(t, before, list.ID, nil))
		require.Zero(t, rows(t, before), "the stand-ins are installed")

		// Each workspace migration runs in its own transaction, which v42's savepoints need
		tx, err := db.BeginTx(bg, nil)
		require.NoError(t, err)
		if mErr := (&migrations.V42Migration{}).UpdateWorkspace(bg, &config.Config{}, workspace, tx); mErr != nil {
			_ = tx.Rollback()
			t.Fatalf("v42 UpdateWorkspace failed: %v", mErr)
		}
		require.NoError(t, tx.Commit())

		// Dropping the trigger would lock message_history for the whole migration
		assert.Equal(t, pre42, messageHistoryTriggers(t), "v42 creates and drops no trigger")

		// Both functions apply the status now; the second application changes nothing
		after := newContact(t)
		complain(t, message(t, after, list.ID, newAutomation(t, list.ID)))
		assert.Equal(t, &listRow{status: "complained"}, rowOf(t, after, list.ID))
		assert.Equal(t, 1, timelineKindCount(t, db, after, "list.complained", list.ID), "the status is recorded once")
		require.Len(t, b24Deliveries(t, db, subscriptionID, after), 1, "and reported once")
		assert.Equal(t, []string{"email.complained: complained"}, statusSeenByEmailEntries(t, after))

		t.Run("a complaint racing an import of a contact with no row", raceWithImportOfANewRow("complained"))
		t.Run("a hard bounce racing an import of a contact with no row", raceWithImportOfANewRow("bounced"))
		t.Run("a complaint racing an import that subscribes a pending contact", raceWithImportOfAPendingRow)
	})
}
