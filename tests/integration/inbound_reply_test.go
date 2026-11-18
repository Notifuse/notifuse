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

// TestInboundReplyDetection exercises the inbound-reply ingestion path end-to-end
// against real Postgres (incl. the v33 trigger that surfaces replies on the contact
// timeline): reply→contact matching, classification (bounce/OOO), and dedup of
// replayed deliveries.
func TestInboundReplyDetection(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))

	// A Mailgun email integration (no signing key set → signature verification is
	// skipped; auth is the provider signature in production).
	integration, err := factory.CreateIntegration(workspace.ID, testutil.WithIntegrationEmailProvider(domain.EmailProvider{
		Kind:               domain.EmailProviderKindMailgun,
		RateLimitPerMinute: 100,
		Senders:            []domain.EmailSender{domain.NewEmailSender("hello@example.com", "Hello")},
		Mailgun:            &domain.MailgunSettings{Domain: "example.com", Region: "US"},
	}))
	require.NoError(t, err)

	baseURL := suite.ServerManager.GetURL()

	postReply := func(t *testing.T, form url.Values) *http.Response {
		t.Helper()
		u := fmt.Sprintf("%s/webhooks/email/inbound?workspace_id=%s&integration_id=%s",
			baseURL, workspace.ID, integration.ID)
		resp, err := http.Post(u, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	replyForm := func(sender, messageHeaders string) url.Values {
		f := url.Values{}
		f.Set("sender", sender)
		f.Set("from", "Replier <"+sender+">")
		f.Set("recipient", "hello@example.com")
		f.Set("subject", "Re: Welcome aboard")
		// A realistic (now) received time so the exit's "entered_at < received" bound holds
		// for journeys enrolled before the reply.
		f.Set("timestamp", fmt.Sprintf("%d", time.Now().Unix()))
		f.Set("message-headers", messageHeaders)
		return f
	}

	// countTimeline polls briefly for timeline entries of a given kind.
	countTimeline := func(t *testing.T, email, kind string) int {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			events, err := factory.GetContactTimelineEvents(workspace.ID, email, kind)
			require.NoError(t, err)
			if len(events) > 0 || time.Now().After(deadline) {
				return len(events)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	// storeSend records an outbound send with a known recipient-visible Message-ID, so a
	// reply that threads to it (via In-Reply-To) can be matched. Matching is now strictly
	// header-based — there is no sender-address fallback — so every reply test must first
	// store the send it replies to.
	storeSend := func(t *testing.T, email string, opts ...testutil.MessageHistoryOption) string {
		t.Helper()
		mid := fmt.Sprintf("send-%d@example.com", time.Now().UnixNano())
		opts = append([]testutil.MessageHistoryOption{
			testutil.WithMessageHistoryContactEmail(email),
			testutil.WithMessageHistorySMTPMessageID(mid),
			testutil.WithMessageHistoryTemplateID("tpl-reply"), // factory default is a 36-char UUID; column is VARCHAR(32)
		}, opts...)
		_, err := factory.CreateMessageHistory(workspace.ID, opts...)
		require.NoError(t, err)
		return mid
	}

	// replyHeaders builds a Mailgun message-headers JSON for a reply threading to
	// inReplyTo, with a unique reply Message-Id. extraPairs is a raw JSON fragment of
	// leading header pairs (e.g. `["Auto-Submitted","auto-replied"],`) or "".
	replyHeaders := func(inReplyTo, extraPairs string) string {
		return fmt.Sprintf(`[%s["In-Reply-To","<%s>"],["Message-Id","<reply-%d@x>"]]`,
			extraPairs, inReplyTo, time.Now().UnixNano())
	}

	wdb, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	// enroll creates an automation and an active journey for a contact in it. The journey
	// is entered an hour in the past so it sits clearly before any reply's received time
	// (the exit is bounded to journeys entered before the reply).
	enroll := func(t *testing.T, autoID string, exitOnReply bool, email string) {
		t.Helper()
		_, err := wdb.Exec(
			`INSERT INTO automations (id, workspace_id, name, status, exit_on_reply, trigger_config)
			 VALUES ($1, $2, $3, 'live', $4, '{}')`,
			autoID, workspace.ID, "auto "+autoID, exitOnReply)
		require.NoError(t, err)
		_, err = wdb.Exec(
			`INSERT INTO contact_automations (id, automation_id, contact_email, status, entered_at)
			 VALUES ($1, $2, $3, 'active', NOW() - INTERVAL '1 hour')`,
			"ca-"+autoID, autoID, email)
		require.NoError(t, err)
	}

	// waitJourneyExited polls until the contact's journey in autoID is exited (or times out).
	waitJourneyExited := func(t *testing.T, autoID, email string) *domain.ContactAutomation {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			ca, err := factory.GetContactAutomation(workspace.ID, autoID, email)
			require.NoError(t, err)
			if ca.Status == domain.ContactAutomationStatusExited || time.Now().After(deadline) {
				return ca
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	t.Run("KnownContactEmitsEmailReplied", func(t *testing.T) {
		email := fmt.Sprintf("replier-%d@example.com", time.Now().UnixNano())
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		mid := storeSend(t, email)

		resp := postReply(t, replyForm(email, replyHeaders(mid, "")))
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.GreaterOrEqual(t, countTimeline(t, email, "email.replied"), 1)
	})

	t.Run("OutOfOfficeRecordedNotReplied", func(t *testing.T) {
		email := fmt.Sprintf("ooo-%d@example.com", time.Now().UnixNano())
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		mid := storeSend(t, email)

		resp := postReply(t, replyForm(email, replyHeaders(mid, `["Auto-Submitted","auto-replied"],`)))
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.GreaterOrEqual(t, countTimeline(t, email, "email.auto_reply"), 1)
		events, err := factory.GetContactTimelineEvents(workspace.ID, email, "email.replied")
		require.NoError(t, err)
		assert.Len(t, events, 0, "an auto-reply must not be recorded as a genuine reply")
	})

	t.Run("BounceDropped", func(t *testing.T) {
		// Bounces are classified and dropped before matching, so no stored send is needed.
		email := fmt.Sprintf("bounce-%d@example.com", time.Now().UnixNano())
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		resp := postReply(t, replyForm(email, `[["Content-Type","multipart/report; report-type=delivery-status"],["Message-Id","<b-1@x>"]]`))
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, 0, countTimeline(t, email, "email.replied"))
	})

	t.Run("UnmatchedReplyIgnored", func(t *testing.T) {
		// With the sender-address fallback removed, a reply that doesn't thread to a
		// stored send is ignored — even from a known contact.
		email := fmt.Sprintf("nomatch-%d@example.com", time.Now().UnixNano())
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		resp := postReply(t, replyForm(email, replyHeaders("ghost-unknown@x", "")))
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, 0, countTimeline(t, email, "email.replied"))
	})

	t.Run("DuplicateReplyDeduped", func(t *testing.T) {
		email := fmt.Sprintf("dup-%d@example.com", time.Now().UnixNano())
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		mid := storeSend(t, email)

		// Fixed reply Message-Id so the replay is recognized as the same event.
		headers := fmt.Sprintf(`[["In-Reply-To","<%s>"],["Message-Id","<dup-same@x>"]]`, mid)
		resp1 := postReply(t, replyForm(email, headers))
		assert.Equal(t, http.StatusOK, resp1.StatusCode)
		require.GreaterOrEqual(t, countTimeline(t, email, "email.replied"), 1)

		// Same provider Message-Id again → deduped (no second timeline entry).
		resp2 := postReply(t, replyForm(email, headers))
		assert.Equal(t, http.StatusOK, resp2.StatusCode)
		time.Sleep(300 * time.Millisecond)
		events, err := factory.GetContactTimelineEvents(workspace.ID, email, "email.replied")
		require.NoError(t, err)
		assert.Len(t, events, 1, "a replayed reply (same Message-Id) must not create a second entry")
	})

	// With Message-ID matching, a reply exits ONLY the exact automation that sent the
	// replied-to email — not every flagged journey the contact happens to be in.
	t.Run("ReplyExitsOnlyTheRepliedToJourney", func(t *testing.T) {
		ts := time.Now().UnixNano()
		email := fmt.Sprintf("scoped-%d@example.com", ts)
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		autoA := fmt.Sprintf("scoped-a-%d", ts) // replied-to, exit_on_reply = TRUE
		autoC := fmt.Sprintf("scoped-c-%d", ts) // also flagged, but NOT replied to
		enroll(t, autoA, true, email)
		enroll(t, autoC, true, email)

		// The reply threads to a send from automation A.
		midA := storeSend(t, email, testutil.WithMessageHistoryAutomationID(autoA))
		resp := postReply(t, replyForm(email, replyHeaders(midA, "")))
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		caA := waitJourneyExited(t, autoA, email)
		assert.Equal(t, domain.ContactAutomationStatusExited, caA.Status,
			"the replied-to automation A must exit")
		if assert.NotNil(t, caA.ExitReason) {
			assert.Equal(t, "replied", *caA.ExitReason)
		}

		caC, err := factory.GetContactAutomation(workspace.ID, autoC, email)
		require.NoError(t, err)
		assert.Equal(t, domain.ContactAutomationStatusActive, caC.Status,
			"automation C (flagged but not replied to) must NOT exit — matching is scoped to the exact send")
	})

	// A reply whose subject only asks to stop is a reply like any other: it is recorded, ends
	// the journey, and leaves the contact's list subscription as it was.
	t.Run("StopReplyExitsAndKeepsListSubscription", func(t *testing.T) {
		ts := time.Now().UnixNano()
		email := fmt.Sprintf("stop-%d@example.com", ts)
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		autoS := fmt.Sprintf("stop-s-%d", ts)
		enroll(t, autoS, true, email)
		midS := storeSend(t, email, testutil.WithMessageHistoryAutomationID(autoS))

		form := replyForm(email, replyHeaders(midS, ""))
		form.Set("subject", "Re: STOP")
		resp := postReply(t, form)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		ca := waitJourneyExited(t, autoS, email)
		assert.Equal(t, domain.ContactAutomationStatusExited, ca.Status, "a STOP reply must end the journey")
		if assert.NotNil(t, ca.ExitReason) {
			assert.Equal(t, "replied", *ca.ExitReason)
		}
		assert.GreaterOrEqual(t, countTimeline(t, email, "email.replied"), 1)

		var status string
		require.NoError(t, wdb.QueryRow(
			`SELECT status FROM contact_lists WHERE email = $1 AND list_id = $2`, email, list.ID).Scan(&status))
		assert.Equal(t, string(domain.ContactListStatusActive), status, "a STOP reply changes no list subscription")
	})

	// Even a matched reply must not exit an automation that does not have Exit-on-reply.
	t.Run("ReplyToNonExitOnReplyAutomationDoesNotExit", func(t *testing.T) {
		ts := time.Now().UnixNano()
		email := fmt.Sprintf("noflag-%d@example.com", ts)
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		autoD := fmt.Sprintf("noflag-d-%d", ts) // exit_on_reply = FALSE
		enroll(t, autoD, false, email)

		midD := storeSend(t, email, testutil.WithMessageHistoryAutomationID(autoD))
		resp := postReply(t, replyForm(email, replyHeaders(midD, "")))
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// The reply is still recorded on the timeline, but the journey is not exited.
		require.GreaterOrEqual(t, countTimeline(t, email, "email.replied"), 1)
		caD, err := factory.GetContactAutomation(workspace.ID, autoD, email)
		require.NoError(t, err)
		assert.Equal(t, domain.ContactAutomationStatusActive, caD.Status,
			"a reply must not exit an automation without Exit-on-reply enabled")
	})

	// A replayed (deduped) reply must not re-exit a journey the contact RE-ENROLLED into
	// after the first reply. Instance #2 is entered before the replay's received time, so
	// only the dedup gate (not the entered_at bound) can protect it.
	t.Run("ReplayAfterReEnrollmentDoesNotReExit", func(t *testing.T) {
		ts := time.Now().UnixNano()
		email := fmt.Sprintf("replay-%d@example.com", ts)
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		autoA := fmt.Sprintf("replay-a-%d", ts)
		enroll(t, autoA, true, email) // instance #1
		midA := storeSend(t, email, testutil.WithMessageHistoryAutomationID(autoA))

		// Fixed reply Message-Id so the replay is recognized as the same event.
		headers := fmt.Sprintf(`[["In-Reply-To","<%s>"],["Message-Id","<replay-same-%d@x>"]]`, midA, ts)

		resp := postReply(t, replyForm(email, headers))
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, domain.ContactAutomationStatusExited, waitJourneyExited(t, autoA, email).Status)

		// Re-enroll: a fresh active instance #2, entered before the replay's received time.
		_, err = wdb.Exec(
			`INSERT INTO contact_automations (id, automation_id, contact_email, status, entered_at)
			 VALUES ($1, $2, $3, 'active', NOW() - INTERVAL '1 minute')`,
			"ca2-"+autoA, autoA, email)
		require.NoError(t, err)

		// Replay the identical reply → deduped → must NOT exit instance #2.
		resp = postReply(t, replyForm(email, headers))
		require.Equal(t, http.StatusOK, resp.StatusCode)
		time.Sleep(400 * time.Millisecond)

		ca2, err := factory.GetContactAutomation(workspace.ID, autoA, email) // latest instance
		require.NoError(t, err)
		assert.Equal(t, domain.ContactAutomationStatusActive, ca2.Status,
			"a deduped replay must not re-exit a re-enrolled journey instance")
	})

	// A reply that threads via References (no In-Reply-To) must still match the send and
	// exit — clients/mailing lists frequently thread this way.
	t.Run("ReferencesHeaderMatchesWhenNoInReplyTo", func(t *testing.T) {
		ts := time.Now().UnixNano()
		email := fmt.Sprintf("refs-%d@example.com", ts)
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		autoA := fmt.Sprintf("refs-a-%d", ts)
		enroll(t, autoA, true, email)
		midA := storeSend(t, email, testutil.WithMessageHistoryAutomationID(autoA))

		// No In-Reply-To; References lists an unknown id then OUR send's id (oldest→newest).
		headers := fmt.Sprintf(`[["References","<other-%d@x> <%s>"],["Message-Id","<refs-reply-%d@x>"]]`, ts, midA, ts)
		resp := postReply(t, replyForm(email, headers))
		require.Equal(t, http.StatusOK, resp.StatusCode)

		assert.GreaterOrEqual(t, countTimeline(t, email, "email.replied"), 1)
		assert.Equal(t, domain.ContactAutomationStatusExited, waitJourneyExited(t, autoA, email).Status,
			"a References-threaded reply must match the send and exit the journey")
	})
}
