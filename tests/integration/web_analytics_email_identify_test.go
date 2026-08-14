//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestWebAnalyticsEmailIdentify walks one recipient from a real send to a
// contact-attributed web session, with nothing hand-built in between.
//
// The promise is that a link in a Notifuse email identifies whoever clicks it
// with no JavaScript written by the customer. Delivering it takes two codebases
// that never reference each other: the send path mints nf_id and the link
// rewriter appends it (Go), then the browser SDK reads it off the URL and
// replays it as identify_token on a beat (TypeScript). Every existing test owns
// one side. TestWebAnalyticsBrowserIdentify covers the browser half honestly —
// but it mints the token itself with BuildWebIdentifyToken and lands the page on
// a URL it wrote by hand, so the email half (mint site, allowlist gate, /r/
// encryption, redirect) has never been shown to produce a credential the ingest
// side actually accepts.
//
// So this test never calls BuildWebIdentifyToken and never writes a landing URL.
// It sends a transactional email through the SMTP provider to Mailpit, reads the
// delivered HTML, decrypts the /r/ token to recover the destination the browser
// would be sent to, and feeds THAT token to /track. A rename, a re-encoding, a
// changed parameter name or a mint site that stops firing all break here and
// nowhere else.
//
// Requires the integration environment (Postgres:5433 + Mailpit) from
// tests/compose.test.yaml.
func TestWebAnalyticsEmailIdentify(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory
	baseURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	require.NotNil(t, buffer, "ingest buffers every beat; nothing reaches Postgres without a flush")
	ctx := context.Background()

	// waPostBeat sends Origin: https://shop.example.com, and ingest silently
	// drops a beat whose origin is off the workspace allowlist. The link
	// rewriter reads that same list to decide whether a link may carry the
	// recipient's identity, so allowlisting exactly this host is what lets one
	// email and one beat exercise both gates at once.
	const trackedHost = "shop.example.com"
	// Deliberately NOT on the allowlist, and deliberately a link in the SAME
	// email as the tracked one: the negative below therefore cannot pass
	// because no token was minted. One was — the tracked link carries it — and
	// the per-link gate has to refuse to attach it to this destination.
	const foreignHost = "partner.example.net"

	user, err := factory.CreateUser()
	require.NoError(t, err)

	workspace, err := factory.CreateWorkspace(func(w *domain.Workspace) {
		// Click tracking on: this is the harder shape of the chain, because the
		// destination (nf_id included) gets encrypted into a /r/ token and the
		// parameter only exists again after the redirect handler decrypts it.
		w.Settings.EmailTrackingEnabled = true
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{
			Enabled: true,
			// The whole chain under test is off without this: it is the opt-in
			// for the one identity path the customer does not initiate, so a
			// workspace that merely enables web analytics mints nothing.
			IdentifyFromEmailLinks: true,
			AllowedDomains:         []string{trackedHost},
			Filters:                domain.DefaultWebFilters(),
		}
	})
	require.NoError(t, err)
	require.NotEmpty(t, workspace.Settings.SecretKey,
		"the workspace secret both mints the token and reads it back")

	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	// Points the workspace at Mailpit on localhost:1025 for both provider slots.
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	// Ingest refuses to remember an address that is not already a contact, so
	// without this the token would decrypt correctly and the identity still be
	// dropped. The transactional send upserts it too; creating it up front keeps
	// the reason explicit.
	contactEmail := fmt.Sprintf("email-identify-%s@example.com", uuid.New().String()[:8])
	_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(contactEmail))
	require.NoError(t, err)

	// Both authored links already carry a utm_* parameter, so applyUTMParameters
	// returns them untouched. That is what makes the assertions below exact: the
	// only difference between what the author wrote and what the redirect
	// carries is the identity parameter this test is about.
	trackedDest := "https://" + trackedHost + "/welcome?utm_source=newsletter"
	foreignDest := "https://" + foreignHost + "/deal?utm_source=newsletter"

	mjmlSource := fmt.Sprintf(`<mjml><mj-body><mj-section><mj-column>
		<mj-text>Your <a href="%s">order</a>, and a <a href="%s">partner offer</a>.</mj-text>
	</mj-column></mj-section></mj-body></mjml>`, trackedDest, foreignDest)

	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("WA email identify"),
		testutil.WithTemplateSubject(fmt.Sprintf("WA email identify %s", uuid.New().String()[:8])),
		testutil.WithCodeModeTemplate(mjmlSource))
	require.NoError(t, err)

	notification, err := factory.CreateTransactionalNotification(workspace.ID,
		testutil.WithTransactionalNotificationID("wa-email-identify"),
		testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
			domain.TransactionalChannelEmail: domain.ChannelTemplate{
				TemplateID: template.ID,
				Settings:   map[string]interface{}{},
			},
		}))
	require.NoError(t, err)

	require.NoError(t, testutil.ClearMailpitMessages(t))

	// The real send. No CC and no BCC: the mint site refuses to identify a
	// message with additional recipients, and a test that quietly added one
	// would assert the absence of a token while believing it asserted its
	// presence.
	sendResp, err := client.SendTransactionalNotification(map[string]interface{}{
		"id":       notification.ID,
		"contact":  map[string]interface{}{"email": contactEmail},
		"channels": []string{"email"},
	})
	require.NoError(t, err)
	sendBody, err := io.ReadAll(sendResp.Body)
	_ = sendResp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, sendResp.StatusCode, "transactional.send failed: %s", string(sendBody))

	var sendResult map[string]interface{}
	require.NoError(t, json.Unmarshal(sendBody, &sendResult))
	messageID, _ := sendResult["message_id"].(string)
	require.NotEmpty(t, messageID, "the send must report the message it created: %s", string(sendBody))

	email, err := testutil.WaitForMailpitMessageByRecipient(t, contactEmail, 30*time.Second)
	require.NoError(t, err, "the transactional email must be delivered to Mailpit")

	// ---------------------------------------------------------------------
	// Steps 2-4: what the delivered email actually contains.
	// ---------------------------------------------------------------------

	// Every href must have been rewritten. If an authored URL survives verbatim
	// the link rewriter never ran, and every assertion below would be describing
	// a pipeline that did nothing.
	assert.NotContains(t, email.HTML, `href="https://`+trackedHost,
		"the tracked link must be rewritten through the encrypted redirect")
	assert.NotContains(t, email.HTML, `href="https://`+foreignHost,
		"the off-allowlist link must still be rewritten for click tracking")

	linkRe := regexp.MustCompile(`href="([^"]*/r/([A-Za-z0-9_-]+))"`)
	matches := linkRe.FindAllStringSubmatch(email.HTML, -1)
	require.NotEmpty(t, matches, "the delivered HTML must carry /r/ redirect links")

	landingByHost := map[string]string{}
	clickURLByLanding := map[string]string{}
	var sentTs int64
	for _, m := range matches {
		decrypted, err := crypto.DecryptTrackingToken(m[2])
		require.NoError(t, err, "every /r/ token in the email must decrypt")
		parts := strings.SplitN(decrypted, "\n", 4)
		require.Len(t, parts, 4, "the redirect payload is messageID/workspaceID/ts/destination")
		assert.Equal(t, messageID, parts[0], "the redirect must name the message transactional.send reported")
		assert.Equal(t, workspace.ID, parts[1], "the redirect must name this workspace")
		ts, err := strconv.ParseInt(parts[2], 10, 64)
		require.NoError(t, err)
		if ts > sentTs {
			sentTs = ts
		}
		parsed, err := url.Parse(parts[3])
		require.NoError(t, err, "the encrypted destination must be a URL")
		landingByHost[parsed.Hostname()] = parts[3]
		// Clicked against the test server whatever endpoint host the email baked
		// in — cfg.APIEndpoint is empty in tests, so the href is a bare "/r/…".
		clickURLByLanding[parts[3]] = fmt.Sprintf("%s/r/%s", baseURL, m[2])
	}
	require.Len(t, landingByHost, 2,
		"both authored links must survive as distinct destinations, got %v", landingByHost)

	trackedLanding, ok := landingByHost[trackedHost]
	require.True(t, ok, "the allowlisted destination is missing from the email: %v", landingByHost)
	foreignLanding, ok := landingByHost[foreignHost]
	require.True(t, ok, "the off-allowlist destination is missing from the email: %v", landingByHost)

	trackedURL, err := url.Parse(trackedLanding)
	require.NoError(t, err)
	foreignURL, err := url.Parse(foreignLanding)
	require.NoError(t, err)
	emailToken := trackedURL.Query().Get(domain.WebIdentifyQueryParam)
	require.NotEmpty(t, emailToken,
		"the tracked link carries no %s, so no recipient can ever be identified by clicking it: landing=%s",
		domain.WebIdentifyQueryParam, trackedLanding)
	require.LessOrEqual(t, len(emailToken), domain.WebTrackMaxIdentifyTokenLength,
		"a token over the bound is dropped at the beat with no log and no error")

	// Exact, not "contains": the parameter must be appended to the authored URL
	// with every other byte intact. The token is hex, so QueryEscape is the
	// identity function on it and this equality is not a re-encoding accident.
	assert.Equal(t, trackedDest+"&"+domain.WebIdentifyQueryParam+"="+emailToken, trackedLanding,
		"the identity parameter must be appended to the authored URL, leaving the rest alone")

	// A cross-check with a precise failure message, not a substitute for the
	// beat below: it separates "the send minted the wrong credential" from "the
	// ingest side rejected a good one". Nothing here mints anything.
	resolved, ok := domain.ResolveWebIdentity(
		&domain.WebTrackPayload{IdentifyToken: &emailToken},
		workspace.Settings.SecretKey, time.Now().UTC())
	require.True(t, ok,
		"the token the email carries does not verify under the workspace secret")
	assert.Equal(t, contactEmail, resolved,
		"the token must name the recipient this email was addressed to")

	// The per-link gate. Same email, same minted token, different host.
	assert.Equal(t, foreignDest, foreignLanding,
		"a destination off the allowlist must reach the recipient byte-identical")
	assert.NotContains(t, foreignLanding, domain.WebIdentifyQueryParam,
		"a third-party link must never carry the recipient's identity")
	assert.NotContains(t, foreignLanding, emailToken,
		"not even under another parameter name")

	// With click tracking on, the credential exists only inside the encrypted
	// /r/ token. A token this long cannot appear in the body by chance, so
	// finding it there would mean a rewrite ordering change leaked it.
	assert.NotContains(t, email.HTML, emailToken,
		"the identity token must not sit in the delivered HTML in clear text")

	t.Logf("the email minted a %d-character %s for %s",
		len(emailToken), domain.WebIdentifyQueryParam, contactEmail)

	// ---------------------------------------------------------------------
	// Steps 5-9: the same token, replayed the way the SDK replays it.
	// ---------------------------------------------------------------------

	now := time.Now().UTC()

	// postBeat sends one beat exactly as the SDK builds it (see
	// session-state.ts buildPayload): the credential rides as identify_token,
	// and landing is the URL left in the address bar — for the tracked link,
	// that is what remains after init() strips nf_id, which the exact-equality
	// assertion above already pinned to the authored URL.
	//
	// Everything else is identical between the identified and the unidentified
	// run — same origin, same shape, same workspace, same existing contact — so
	// the only variable is whether the email put a token on the link that was
	// clicked. The unidentified run is deliberately given every other advantage,
	// including an allowlisted Origin its real landing host would not have,
	// precisely so a NULL contact_email cannot be credited to the domain gate
	// instead of to the per-link one this test is about.
	postBeat := func(t *testing.T, sessionID, identifyToken, landing, goalName string) {
		t.Helper()
		landingURL, err := url.Parse(landing)
		require.NoError(t, err)
		path := landingURL.Path
		payload := map[string]interface{}{
			"workspace_id": workspace.ID,
			"session_id":   sessionID,
			"tab_id":       1,
			"actions": []map[string]interface{}{
				waPageview(path, 1, 1500, 40, now),
				{
					"type": "goal", "name": goalName, "page_number": 1,
					"timestamp": now.Add(-1 * time.Minute).UnixMilli(),
					"value":     19.5, "path": path,
				},
			},
			"attributes": map[string]interface{}{
				"landing_page": landing,
				"user_agent":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36",
			},
			"created_at":  now.Add(-2 * time.Minute).UnixMilli(),
			"updated_at":  now.UnixMilli(),
			"sent_at":     now.UnixMilli(),
			"sdk_version": "1.0.0",
			"seq":         1,
		}
		if identifyToken != "" {
			payload["identify_token"] = identifyToken
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		decoded := waCloseAndDecode(t, waPostBeat(t, baseURL, body, nil))
		require.Equal(t, true, decoded["success"], "the beat was rejected: %v", decoded)
	}

	// flushUntil drives the ingest buffer until the row lands. The background
	// flusher may already own the workspace when FlushAll is called, which makes
	// a single flush a race; waiting weakens nothing, because a beat whose
	// identity was dropped never acquires one however long we wait.
	flushUntil := func(t *testing.T, what string, landed func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for {
			buffer.FlushAll(ctx)
			if landed() {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s never reached Postgres", what)
			}
			time.Sleep(250 * time.Millisecond)
		}
	}

	sessionRows := func(sessionID string) int {
		var n int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM web_sessions WHERE id = $1`, sessionID).Scan(&n))
		return n
	}

	t.Run("the token the email minted identifies the visit", func(t *testing.T) {
		sessionID := waUUIDv7At(now.Add(-2*time.Minute), 0xE1)
		postBeat(t, sessionID, emailToken, trackedDest, "email_link_signup")
		flushUntil(t, "the identified beat's session row", func() bool {
			return sessionRows(sessionID) == 1
		})

		var storedEmail *string
		require.NoError(t, wsDB.QueryRow(
			`SELECT contact_email FROM web_sessions WHERE id = $1`, sessionID).Scan(&storedEmail))
		require.NotNil(t, storedEmail,
			"the credential the EMAIL produced did not identify the visit; the two halves of nf_id disagree")
		assert.Equal(t, contactEmail, *storedEmail,
			"the session must name the recipient the email was addressed to")

		// The other two tables stamped from the same attribution snapshot. The
		// token is consumed during init, so the very first insert already knows
		// the contact and nothing has to re-stamp anything for these to match.
		var pageRows, identifiedPages int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*), count(*) FILTER (WHERE contact_email = $2) FROM web_pages WHERE session_id = $1`,
			sessionID, contactEmail).Scan(&pageRows, &identifiedPages))
		require.Equal(t, 1, pageRows, "the beat carried exactly one pageview")
		assert.Equal(t, pageRows, identifiedPages, "every page of an identified visit must name the contact")

		var goalRows, identifiedGoals int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*), count(*) FILTER (WHERE contact_email = $2) FROM web_goals WHERE session_id = $1`,
			sessionID, contactEmail).Scan(&goalRows, &identifiedGoals))
		require.Equal(t, 1, goalRows, "the beat carried exactly one goal")
		assert.Equal(t, goalRows, identifiedGoals, "the goal must carry the identity of the beat that brought it")

		// With the bridge on, the click-through conversion becomes the contact's
		// own timeline entry. This is the whole reason identity is worth having,
		// and the emit happens after the batch commits — hence the wait rather
		// than a bare read.
		bridged := func() int {
			var n int
			require.NoError(t, wsDB.QueryRow(
				`SELECT count(*) FROM custom_events WHERE email = $1 AND event_name = $2`,
				contactEmail, "email_link_signup").Scan(&n))
			return n
		}
		flushUntil(t, "the bridged custom_events row", func() bool { return bridged() > 0 })
		assert.Equal(t, 1, bridged(), "one conversion must bridge exactly once")

		var source string
		require.NoError(t, wsDB.QueryRow(
			`SELECT source FROM custom_events WHERE email = $1 AND event_name = $2`,
			contactEmail, "email_link_signup").Scan(&source))
		assert.Equal(t, "web_analytics", source, "the source distinguishes bridged goals from API ones")
	})

	t.Run("a link off the allowlist leaves the visit anonymous", func(t *testing.T) {
		// Replays whatever the foreign link actually carries, rather than a
		// hard-coded empty token. The two differ only when the per-link gate is
		// broken, and that is exactly the case this subtest exists to catch: with
		// "" written in, it keeps passing while every recipient hands the partner
		// site a working credential, because the beat it sends is one no such
		// visitor would ever send. Normally this is "" and nothing changes.
		foreignToken := foreignURL.Query().Get(domain.WebIdentifyQueryParam)
		sessionID := waUUIDv7At(now.Add(-2*time.Minute), 0xE2)
		postBeat(t, sessionID, foreignToken, foreignDest, "partner_link_signup")
		// The row has to exist before NULL means anything: a dropped beat would
		// produce no row at all, and "no identity" would be true for the wrong
		// reason.
		flushUntil(t, "the unidentified beat's session row", func() bool {
			return sessionRows(sessionID) == 1
		})

		var storedEmail *string
		require.NoError(t, wsDB.QueryRow(
			`SELECT contact_email FROM web_sessions WHERE id = $1`, sessionID).Scan(&storedEmail))
		assert.Nil(t, storedEmail,
			"a click on a link the allowlist excluded must not identify anyone")

		var namedPages, namedGoals int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM web_pages WHERE session_id = $1 AND contact_email IS NOT NULL`,
			sessionID).Scan(&namedPages))
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM web_goals WHERE session_id = $1 AND contact_email IS NOT NULL`,
			sessionID).Scan(&namedGoals))
		assert.Zero(t, namedPages, "no page of an anonymous visit may name a contact")
		assert.Zero(t, namedGoals, "no goal of an anonymous visit may name a contact")

		// And nothing reached the contact's timeline: an anonymous goal has
		// nothing to attach to.
		var bridged int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM custom_events WHERE event_name = $1`, "partner_link_signup").Scan(&bridged))
		assert.Zero(t, bridged, "an anonymous conversion must not become anyone's timeline entry")
	})

	t.Run("the click is recorded under the authored URL, not the per-recipient one", func(t *testing.T) {
		// Clicks within 7 seconds of the compile-time timestamp are treated as
		// bot prefetch and not recorded at all.
		if wait := time.Until(time.Unix(sentTs, 0).Add(8 * time.Second)); wait > 0 {
			t.Logf("waiting %s for the too-fast-click bot gate", wait.Round(time.Millisecond))
			time.Sleep(wait)
		}

		redirectClient := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		req, err := http.NewRequest(http.MethodGet, clickURLByLanding[trackedLanding], nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15")
		clickResp, err := redirectClient.Do(req)
		require.NoError(t, err)
		_ = clickResp.Body.Close()
		require.Equal(t, http.StatusSeeOther, clickResp.StatusCode)

		// Step 4 of the chain, asserted on the wire: this Location header is the
		// only thing that ever puts nf_id in front of a browser. If the redirect
		// dropped or rewrote it, every assertion above would still pass and no
		// real recipient would ever be identified.
		assert.Equal(t, trackedLanding, clickResp.Header.Get("Location"),
			"the redirect must hand the browser the destination the email encrypted, identity included")

		var clickedLinksJSON []byte
		require.NoError(t, wsDB.QueryRow(
			`SELECT clicked_links FROM message_history WHERE id = $1`, messageID).Scan(&clickedLinksJSON))
		require.NotEmpty(t, clickedLinksJSON, "the click was not recorded on the message at all")

		var clickedLinks map[string]clickedLinkEntry
		require.NoError(t, json.Unmarshal(clickedLinksJSON, &clickedLinks))
		require.Len(t, clickedLinks, 1, "one click on one link: %v", clickedLinks)

		// The key is a JSONB key that per-link reporting aggregates across every
		// recipient of a broadcast. The token is minted per recipient over a
		// fresh nonce, so leaving it in would turn one row per link into one row
		// per recipient — and would persist a bearer identity credential in the
		// workspace database.
		entry, ok := clickedLinks[trackedDest]
		require.True(t, ok,
			"the click must be recorded under the authored destination, got keys %v", clickedLinks)
		assert.Equal(t, 1, entry.Count)
		for key := range clickedLinks {
			assert.NotContains(t, key, domain.WebIdentifyQueryParam,
				"a recorded link must not carry the identity parameter")
			assert.NotContains(t, key, emailToken,
				"a recorded link must not carry the per-recipient credential")
		}
	})
}
