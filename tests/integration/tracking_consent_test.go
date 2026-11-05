package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/Notifuse/notifuse/pkg/trackingtoken"
	"github.com/Notifuse/notifuse/tests/testutil"
)

var (
	consentPixelRe    = regexp.MustCompile(`<img src="[^"]*(/t/[A-Za-z0-9_-]+)"`)
	consentRedirectRe = regexp.MustCompile(`href="[^"]*(/r/[A-Za-z0-9_-]+)"`)
)

// smtpToMailpit is the email provider every test here sends through.
func smtpToMailpit(senderName string) testutil.IntegrationOption {
	return testutil.WithIntegrationEmailProvider(domain.EmailProvider{
		Kind:               domain.EmailProviderKindSMTP,
		Senders:            []domain.EmailSender{domain.NewEmailSender("noreply@notifuse.test", senderName)},
		SMTP:               &domain.SMTPSettings{Host: "localhost", Port: 1025, UseTLS: false},
		RateLimitPerMinute: 2000,
	})
}

// withEmailTracking creates the workspace with tracking on and the given
// policy, as a workspace saved from the console holds it.
func withEmailTracking(settings domain.EmailTrackingSettings) testutil.WorkspaceOption {
	return func(w *domain.Workspace) {
		w.Settings.EmailTrackingEnabled = settings.OpensEnabled || settings.ClicksEnabled
		w.Settings.EmailTracking = &settings
	}
}

type storedConsent struct {
	opens, clicks, source sql.NullString
	updatedAt             sql.NullTime
}

func readStoredConsent(t *testing.T, db *sql.DB, email string) storedConsent {
	t.Helper()
	var c storedConsent
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT tracking_opens_consent, tracking_clicks_consent, tracking_consent_source, tracking_consent_updated_at
		 FROM contacts WHERE email = $1`, email).Scan(&c.opens, &c.clicks, &c.source, &c.updatedAt))
	return c
}

func postStatus(t *testing.T, client *testutil.APIClient, path string, body interface{}) (int, string) {
	t.Helper()
	resp, err := client.Post(path, body)
	require.NoError(t, err)
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode, string(raw)
}

// TestTrackingConsent_ContactAPI covers how the consent is written and read
// through the contacts API: values, explicit null, source and date, a repeated
// value that changes nothing, the list filters and the timeline entry.
func TestTrackingConsent_ContactAPI(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory
	ctx := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace(withEmailTracking(*domain.DefaultEmailTrackingSettings()))
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	tag := uuid.New().String()[:8]
	denier := fmt.Sprintf("denier-%s@example.com", tag)
	unknown := fmt.Sprintf("unknown-%s@example.com", tag)
	migrated := fmt.Sprintf("migrated-%s@example.com", tag)

	upsert := func(t *testing.T, contact map[string]interface{}) {
		t.Helper()
		status, body := postStatus(t, client, "/api/contacts.upsert", map[string]interface{}{
			"workspace_id": workspace.ID,
			"contact":      contact,
		})
		require.Equal(t, http.StatusOK, status, body)
	}

	t.Run("a console write records the value, the date and the source console", func(t *testing.T) {
		upsert(t, map[string]interface{}{"email": denier, "tracking_opens_consent": "denied", "tracking_clicks_consent": "granted"})

		c := readStoredConsent(t, db, denier)
		assert.Equal(t, "denied", c.opens.String)
		assert.Equal(t, "granted", c.clicks.String)
		assert.Equal(t, domain.TrackingConsentSourceConsole, c.source.String)
		assert.True(t, c.updatedAt.Valid)
	})

	t.Run("the same value again changes neither date nor source, and logs nothing", func(t *testing.T) {
		before := readStoredConsent(t, db, denier)
		time.Sleep(20 * time.Millisecond)
		upsert(t, map[string]interface{}{"email": denier, "tracking_opens_consent": "denied", "first_name": "Dee"})

		after := readStoredConsent(t, db, denier)
		assert.True(t, before.updatedAt.Time.Equal(after.updatedAt.Time))
		assert.Equal(t, before.source, after.source)

		var consentEntries int
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM contact_timeline WHERE email = $1 AND kind = 'contact.updated' AND changes ? 'tracking_opens_consent'`,
			denier).Scan(&consentEntries))
		assert.Zero(t, consentEntries, "the first write was the insert; the repeat changed nothing")
	})

	t.Run("a contact without consent stays unknown", func(t *testing.T) {
		upsert(t, map[string]interface{}{"email": unknown, "first_name": "Uma"})
		c := readStoredConsent(t, db, unknown)
		assert.False(t, c.opens.Valid)
		assert.False(t, c.clicks.Valid)
		assert.False(t, c.source.Valid)
		assert.False(t, c.updatedAt.Valid)
	})

	t.Run("an import carries its own date and source", func(t *testing.T) {
		resp, err := client.BatchImportContacts([]map[string]interface{}{{
			"email":                       migrated,
			"tracking_clicks_consent":     "denied",
			"tracking_consent_source":     "legacy_crm",
			"tracking_consent_updated_at": "2025-03-01T10:00:00Z",
		}}, nil)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))

		c := readStoredConsent(t, db, migrated)
		assert.Equal(t, "denied", c.clicks.String)
		assert.Equal(t, "legacy_crm", c.source.String)
		assert.True(t, c.updatedAt.Time.Equal(time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)))
	})

	t.Run("a change is logged on the timeline with the old and new values", func(t *testing.T) {
		upsert(t, map[string]interface{}{"email": unknown, "tracking_clicks_consent": "denied"})

		var changes []byte
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT changes FROM contact_timeline WHERE email = $1 AND kind = 'contact.updated' AND changes ? 'tracking_clicks_consent'
			 ORDER BY created_at DESC LIMIT 1`, unknown).Scan(&changes))
		var parsed map[string]map[string]interface{}
		require.NoError(t, json.Unmarshal(changes, &parsed))
		assert.Nil(t, parsed["tracking_clicks_consent"]["old"])
		assert.Equal(t, "denied", parsed["tracking_clicks_consent"]["new"])
		assert.Equal(t, domain.TrackingConsentSourceConsole, parsed["tracking_consent_source"]["new"])
		_, hasDate := parsed["tracking_consent_updated_at"]
		assert.False(t, hasDate, "the date moves with the consent and is not logged on its own")
	})

	t.Run("an invalid value is refused", func(t *testing.T) {
		status, _ := postStatus(t, client, "/api/contacts.upsert", map[string]interface{}{
			"workspace_id": workspace.ID,
			"contact":      map[string]interface{}{"email": unknown, "tracking_opens_consent": "yes"},
		})
		assert.Equal(t, http.StatusBadRequest, status)
	})

	t.Run("contacts.list filters on consent, unknown included", func(t *testing.T) {
		list := func(t *testing.T, params map[string]string) []string {
			t.Helper()
			params["workspace_id"] = workspace.ID
			params["limit"] = "100"
			resp, err := client.ListContacts(params)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			var result struct {
				Contacts []struct {
					Email string `json:"email"`
				} `json:"contacts"`
			}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
			emails := make([]string, 0, len(result.Contacts))
			for _, c := range result.Contacts {
				emails = append(emails, c.Email)
			}
			return emails
		}

		assert.ElementsMatch(t, []string{denier}, list(t, map[string]string{"tracking_opens_consent": "denied"}))
		assert.ElementsMatch(t, []string{unknown, migrated}, list(t, map[string]string{"tracking_opens_consent": "unknown"}))
		assert.ElementsMatch(t, []string{unknown, migrated}, list(t, map[string]string{"tracking_clicks_consent": "denied"}))
	})

	// importContacts posts an import and requires every row to be written: an
	// import answers 200 even when rows fail, which would leave the stored
	// values untouched and a "kept" assertion passing for the wrong reason.
	importContacts := func(t *testing.T, rows []map[string]interface{}) {
		t.Helper()
		status, body := postStatus(t, client, "/api/contacts.import", map[string]interface{}{
			"workspace_id": workspace.ID,
			"contacts":     rows,
		})
		require.Equal(t, http.StatusOK, status, body)
		var result domain.BatchImportContactsResponse
		require.NoError(t, json.Unmarshal([]byte(body), &result), body)
		require.Empty(t, result.Error, body)
		require.Len(t, result.Operations, len(rows), body)
		for _, op := range result.Operations {
			require.Empty(t, op.Error, body)
			require.NotEqual(t, domain.UpsertContactOperationError, op.Action, body)
		}
	}

	// Each case records its own choice first, so it runs on its own. They come
	// last: their contacts would otherwise show in the list filter case above.
	recordRefusal := func(t *testing.T, email string) storedConsent {
		t.Helper()
		importContacts(t, []map[string]interface{}{{
			"email":                       email,
			"tracking_opens_consent":      "denied",
			"tracking_clicks_consent":     "denied",
			"tracking_consent_source":     "legacy_crm",
			"tracking_consent_updated_at": "2025-03-01T10:00:00Z",
		}})
		return readStoredConsent(t, db, email)
	}

	assertUnchanged := func(t *testing.T, email string, before storedConsent) {
		t.Helper()
		after := readStoredConsent(t, db, email)
		assert.Equal(t, "denied", after.opens.String)
		assert.Equal(t, "denied", after.clicks.String)
		assert.Equal(t, before.source, after.source)
		assert.True(t, before.updatedAt.Time.Equal(after.updatedAt.Time), "the date stays")
	}

	// In an import, an empty or null consent is no value: the console's CSV
	// import sends null for an empty cell, and a spreadsheet holds blanks.
	t.Run("an import with an empty or null consent keeps the recorded one", func(t *testing.T) {
		email := fmt.Sprintf("import-blank-%s@example.com", tag)
		before := recordRefusal(t, email)

		for _, blank := range []interface{}{nil, "", "  "} {
			importContacts(t, []map[string]interface{}{{
				"email":                   email,
				"tracking_opens_consent":  blank,
				"tracking_clicks_consent": blank,
			}})
			assertUnchanged(t, email, before)
		}
	})

	// In an upsert, an empty string is no value either. An explicit null is a
	// value there: it sets the choice back to unknown.
	t.Run("an upsert with an empty consent keeps the recorded one, a null clears it", func(t *testing.T) {
		email := fmt.Sprintf("upsert-blank-%s@example.com", tag)
		before := recordRefusal(t, email)

		for _, blank := range []string{"", "  "} {
			upsert(t, map[string]interface{}{"email": email, "tracking_opens_consent": blank, "tracking_clicks_consent": blank})
			assertUnchanged(t, email, before)
		}

		var consentEntries int
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM contact_timeline WHERE email = $1 AND kind = 'contact.updated' AND changes ? 'tracking_opens_consent'`,
			email).Scan(&consentEntries))
		assert.Zero(t, consentEntries, "a blank consent logs no change")

		upsert(t, map[string]interface{}{"email": email, "tracking_opens_consent": nil})
		after := readStoredConsent(t, db, email)
		assert.False(t, after.opens.Valid, "null sets the choice back to unknown")
		assert.Equal(t, "denied", after.clicks.String)
	})
}

type consentRecipient struct {
	email      string
	consent    map[string]interface{}
	wantPixel  bool
	wantClicks bool
}

// TestTrackingConsent_BroadcastE2E sends a real broadcast to contacts with
// different consents, reads each delivered email from Mailpit, and checks what
// it carries, what message_history records, the tracked totals, and that a
// contact who refuses after the send has neither opens nor clicks recorded.
func TestTrackingConsent_BroadcastE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	baseURL := suite.ServerManager.GetURL()
	client := suite.APIClient
	factory := suite.DataFactory
	ctx := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace(withEmailTracking(*domain.DefaultEmailTrackingSettings()))
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID, smtpToMailpit("Consent E2E"))
	require.NoError(t, err)

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(workerCtx))

	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	require.NoError(t, testutil.ClearMailpitMessages(t))

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	list, err := factory.CreateList(workspace.ID, testutil.WithListName("Consent E2E List"))
	require.NoError(t, err)

	tag := uuid.New().String()[:8]
	recipients := map[string]*consentRecipient{
		"opens_denied":  {email: fmt.Sprintf("opens-denied-%s@example.com", tag), consent: map[string]interface{}{"tracking_opens_consent": "denied"}, wantClicks: true},
		"clicks_denied": {email: fmt.Sprintf("clicks-denied-%s@example.com", tag), consent: map[string]interface{}{"tracking_clicks_consent": "denied"}, wantPixel: true},
		"unknown":       {email: fmt.Sprintf("unknown-%s@example.com", tag), wantPixel: true, wantClicks: true},
		"granted":       {email: fmt.Sprintf("granted-%s@example.com", tag), consent: map[string]interface{}{"tracking_opens_consent": "granted", "tracking_clicks_consent": "granted"}, wantPixel: true, wantClicks: true},
	}
	for _, r := range recipients {
		contact := map[string]interface{}{"email": r.email}
		for k, v := range r.consent {
			contact[k] = v
		}
		status, body := postStatus(t, client, "/api/contacts.upsert", map[string]interface{}{"workspace_id": workspace.ID, "contact": contact})
		require.Equal(t, http.StatusOK, status, body)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(r.email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
	}

	mjml := `<mjml><mj-body><mj-section><mj-column>
		<mj-text>Read <a href="https://shop.example.com/new">what is new</a>.</mj-text>
	</mj-column></mj-section></mj-body></mjml>`
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Consent E2E Template"),
		testutil.WithTemplateSubject("Consent e2e "+tag),
		testutil.WithCodeModeTemplate(mjml))
	require.NoError(t, err)

	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName("Consent E2E Broadcast"),
		testutil.WithBroadcastTemplateID(template.ID),
		testutil.WithBroadcastAudience(domain.AudienceSettings{List: list.ID, ExcludeUnsubscribed: true}))
	require.NoError(t, err)

	resp, err := client.ScheduleBroadcast(map[string]interface{}{"workspace_id": workspace.ID, "id": broadcast.ID, "send_now": true})
	require.NoError(t, err)
	_ = resp.Body.Close()
	_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID, []string{"processed", "completed", "sent"}, 60*time.Second)
	require.NoError(t, err)

	html := map[string]string{}
	var sentTs int64
	for name, r := range recipients {
		email, err := waitForEmailByRecipientAddr(t, r.email, 30*time.Second)
		require.NoError(t, err, "the email to %s must arrive", name)
		html[name] = email.HTML
		if m := consentRedirectRe.FindStringSubmatch(email.HTML); m != nil {
			tok, err := trackingtoken.Parse(strings.TrimPrefix(m[1], "/r/"), trackingtoken.PurposeClick)
			require.NoError(t, err)
			sentTs = tok.Timestamp
		}
	}

	t.Run("each email carries what its recipient consented to", func(t *testing.T) {
		for name, r := range recipients {
			assert.Equal(t, r.wantPixel, consentPixelRe.MatchString(html[name]), "%s: open pixel", name)
			assert.Equal(t, r.wantClicks, consentRedirectRe.MatchString(html[name]), "%s: click redirect", name)
			if !r.wantClicks {
				assert.Contains(t, html[name], `href="https://shop.example.com/new?utm_content=`+template.ID,
					"%s: the link goes straight to its destination, UTM kept", name)
				assert.NotContains(t, html[name], "nf_id=", "%s: no web identity on an untracked link", name)
			}
		}
	})

	t.Run("message_history records what each email carried", func(t *testing.T) {
		for name, r := range recipients {
			var opens, clicks sql.NullBool
			require.NoError(t, db.QueryRowContext(ctx,
				`SELECT opens_tracked, clicks_tracked FROM message_history WHERE contact_email = $1 AND broadcast_id = $2`,
				r.email, broadcast.ID).Scan(&opens, &clicks))
			require.True(t, opens.Valid && clicks.Valid, "%s: both flags written", name)
			assert.Equal(t, r.wantPixel, opens.Bool, "%s: opens_tracked", name)
			assert.Equal(t, r.wantClicks, clicks.Bool, "%s: clicks_tracked", name)
		}
	})

	t.Run("broadcast stats count the tracked sends", func(t *testing.T) {
		resp, err := client.Get("/api/messages.broadcastStats", map[string]string{"workspace_id": workspace.ID, "broadcast_id": broadcast.ID})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var result struct {
			Stats domain.MessageHistoryStatusSum `json:"stats"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		assert.Equal(t, len(recipients), result.Stats.TotalSent)
		assert.Equal(t, 3, result.Stats.TotalOpensTracked, "every recipient but opens_denied")
		assert.Equal(t, 3, result.Stats.TotalClicksTracked, "every recipient but clicks_denied")
	})

	// Hits within a few seconds of the send are read as a link scanner.
	if wait := time.Until(time.Unix(sentTs, 0).Add(8 * time.Second)); wait > 0 {
		time.Sleep(wait)
	}

	hit := func(t *testing.T, path string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15")
		noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		resp, err := noRedirect.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	recorded := func(t *testing.T, email string) (opened, clicked bool) {
		t.Helper()
		var openedAt, clickedAt sql.NullTime
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT opened_at, clicked_at FROM message_history WHERE contact_email = $1 AND broadcast_id = $2`,
			email, broadcast.ID).Scan(&openedAt, &clickedAt))
		return openedAt.Valid, clickedAt.Valid
	}

	t.Run("a recipient who refuses after the send has nothing recorded", func(t *testing.T) {
		r := recipients["unknown"]

		// The opt-out link in the email, as the notification center sends it.
		resp, err := postPreferences(baseURL, domain.UpdateContactPreferencesRequest{
			WorkspaceID:           workspace.ID,
			Email:                 r.email,
			EmailHMAC:             domain.ComputeEmailHMAC(r.email, workspace.Settings.SecretKey),
			TrackingOpensConsent:  domain.TrackingConsentDenied,
			TrackingClicksConsent: domain.TrackingConsentDenied,
			ConsentSource:         domain.TrackingConsentSourceEmailLink,
		})
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, domain.TrackingConsentSourceEmailLink, readStoredConsent(t, db, r.email).source.String)

		pixel := consentPixelRe.FindStringSubmatch(html["unknown"])
		click := consentRedirectRe.FindStringSubmatch(html["unknown"])
		require.NotNil(t, pixel)
		require.NotNil(t, click)
		assert.Equal(t, http.StatusOK, hit(t, pixel[1]))
		assert.Equal(t, http.StatusSeeOther, hit(t, click[1]), "the click still reaches its destination")

		time.Sleep(300 * time.Millisecond)
		opened, clicked := recorded(t, r.email)
		assert.False(t, opened, "no open recorded after the refusal")
		assert.False(t, clicked, "no click recorded after the refusal")
	})

	t.Run("a consenting recipient is recorded", func(t *testing.T) {
		click := consentRedirectRe.FindStringSubmatch(html["granted"])
		require.NotNil(t, click)
		assert.Equal(t, http.StatusSeeOther, hit(t, click[1]))
		testutil.WaitForCondition(t, func() bool {
			opened, clicked := recorded(t, recipients["granted"].email)
			return opened && clicked
		}, 5*time.Second, "click and the open it implies are recorded")
	})

	t.Run("a click from a recipient who refused opens records no open", func(t *testing.T) {
		click := consentRedirectRe.FindStringSubmatch(html["opens_denied"])
		require.NotNil(t, click)
		assert.Equal(t, http.StatusSeeOther, hit(t, click[1]))
		testutil.WaitForCondition(t, func() bool {
			_, clicked := recorded(t, recipients["opens_denied"].email)
			return clicked
		}, 5*time.Second, "the click is recorded")
		opened, _ := recorded(t, recipients["opens_denied"].email)
		assert.False(t, opened)
	})
}

// TestTrackingConsent_TransactionalE2E covers a transactional send: the
// workspace's choice for unknown consent, a consent sent with the
// notification, and the per-notification no_tracking mode.
func TestTrackingConsent_TransactionalE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace(withEmailTracking(domain.EmailTrackingSettings{
		OpensEnabled: true, ClicksEnabled: true, UnknownConsent: domain.UnknownConsentNoTrack,
	}))
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID, smtpToMailpit("Consent Transactional"))
	require.NoError(t, err)

	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	require.NoError(t, testutil.ClearMailpitMessages(t))

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	mjml := `<mjml><mj-body><mj-section><mj-column>
		<mj-text>Open <a href="https://app.example.com/orders">your orders</a>.</mj-text>
	</mj-column></mj-section></mj-body></mjml>`
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Consent Transactional Template"),
		testutil.WithCodeModeTemplate(mjml))
	require.NoError(t, err)

	newNotification := func(t *testing.T, mode string) *domain.TransactionalNotification {
		t.Helper()
		n, err := factory.CreateTransactionalNotification(workspace.ID,
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: {TemplateID: template.ID, Settings: map[string]interface{}{}},
			}),
			func(n *domain.TransactionalNotification) {
				n.TrackingSettings = notifuse_mjml.TrackingSettings{EnableTracking: true, TrackingMode: mode}
			})
		require.NoError(t, err)
		return n
	}
	send := func(t *testing.T, n *domain.TransactionalNotification, contact map[string]interface{}) (string, string) {
		t.Helper()
		resp, err := client.SendTransactionalNotification(map[string]interface{}{
			"id":       n.ID,
			"contact":  contact,
			"channels": []string{"email"},
		})
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))
		var result struct {
			MessageID string `json:"message_id"`
		}
		require.NoError(t, json.Unmarshal(raw, &result))
		email, err := waitForEmailByRecipientAddr(t, contact["email"].(string), 30*time.Second)
		require.NoError(t, err)
		return result.MessageID, email.HTML
	}
	flags := func(t *testing.T, messageID string) (bool, bool) {
		t.Helper()
		var opens, clicks sql.NullBool
		require.NoError(t, db.QueryRowContext(context.Background(),
			`SELECT opens_tracked, clicks_tracked FROM message_history WHERE id = $1`, messageID).Scan(&opens, &clicks))
		require.True(t, opens.Valid && clicks.Valid)
		return opens.Bool, clicks.Bool
	}

	tag := uuid.New().String()[:8]
	inherit := newNotification(t, "")

	t.Run("unknown consent under no_track: nothing tracked", func(t *testing.T) {
		messageID, html := send(t, inherit, map[string]interface{}{"email": fmt.Sprintf("tx-unknown-%s@example.com", tag)})
		assert.False(t, consentPixelRe.MatchString(html))
		assert.False(t, consentRedirectRe.MatchString(html))
		opens, clicks := flags(t, messageID)
		assert.False(t, opens)
		assert.False(t, clicks)
	})

	t.Run("a consent sent with the notification is recorded and applied", func(t *testing.T) {
		email := fmt.Sprintf("tx-granted-%s@example.com", tag)
		messageID, html := send(t, inherit, map[string]interface{}{
			"email": email, "tracking_opens_consent": "granted", "tracking_clicks_consent": "granted",
		})
		assert.True(t, consentPixelRe.MatchString(html))
		assert.True(t, consentRedirectRe.MatchString(html))
		opens, clicks := flags(t, messageID)
		assert.True(t, opens)
		assert.True(t, clicks)

		c := readStoredConsent(t, db, email)
		assert.Equal(t, "granted", c.opens.String)
		assert.Equal(t, domain.TrackingConsentSourceAPI, c.source.String)
	})

	t.Run("no_tracking keeps UTM tagging and tracks nothing, even for a consenting contact", func(t *testing.T) {
		email := fmt.Sprintf("tx-notrack-%s@example.com", tag)
		messageID, html := send(t, newNotification(t, notifuse_mjml.TrackingModeNoTracking), map[string]interface{}{
			"email": email, "tracking_opens_consent": "granted", "tracking_clicks_consent": "granted",
		})
		assert.False(t, consentPixelRe.MatchString(html))
		assert.False(t, consentRedirectRe.MatchString(html))
		assert.Contains(t, html, `href="https://app.example.com/orders?utm_content=`+template.ID)
		opens, clicks := flags(t, messageID)
		assert.False(t, opens)
		assert.False(t, clicks)
	})
}

// TestTrackingConsent_NotificationCenter covers the public preferences
// endpoints: the tracking state a contact sees, writing a consent, and a link
// whose contact no longer exists.
func TestTrackingConsent_NotificationCenter(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	baseURL := suite.ServerManager.GetURL()
	factory := suite.DataFactory

	workspace, err := factory.CreateWorkspace(withEmailTracking(domain.EmailTrackingSettings{
		OpensEnabled: false, ClicksEnabled: true, UnknownConsent: domain.UnknownConsentNoTrack,
	}))
	require.NoError(t, err)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	email := fmt.Sprintf("nc-%s@example.com", uuid.New().String()[:8])
	_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
	require.NoError(t, err)
	hmac := domain.ComputeEmailHMAC(email, workspace.Settings.SecretKey)

	getTracking := func(t *testing.T) domain.ContactTrackingState {
		t.Helper()
		resp, err := http.Get(fmt.Sprintf("%s/preferences?workspace_id=%s&email=%s&email_hmac=%s", baseURL, workspace.ID, email, hmac))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var result struct {
			Tracking *domain.ContactTrackingState `json:"tracking"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		require.NotNil(t, result.Tracking)
		return *result.Tracking
	}

	t.Run("an unknown contact sees the workspace policy and is not tracked", func(t *testing.T) {
		assert.Equal(t, domain.ContactTrackingState{ClicksEnabled: true}, getTracking(t))
	})

	t.Run("granting clicks from the notification center", func(t *testing.T) {
		resp, err := postPreferences(baseURL, domain.UpdateContactPreferencesRequest{
			WorkspaceID: workspace.ID, Email: email, EmailHMAC: hmac,
			TrackingClicksConsent: domain.TrackingConsentGranted,
		})
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		c := readStoredConsent(t, db, email)
		assert.Equal(t, "granted", c.clicks.String)
		assert.False(t, c.opens.Valid)
		assert.Equal(t, domain.TrackingConsentSourceNotificationCenter, c.source.String)
		assert.Equal(t, domain.ContactTrackingState{ClicksEnabled: true, ClicksTracked: true}, getTracking(t))
	})

	t.Run("a link whose contact is gone answers 404 and creates nothing", func(t *testing.T) {
		gone := fmt.Sprintf("gone-%s@example.com", uuid.New().String()[:8])
		resp, err := postPreferences(baseURL, domain.UpdateContactPreferencesRequest{
			WorkspaceID: workspace.ID, Email: gone, EmailHMAC: domain.ComputeEmailHMAC(gone, workspace.Settings.SecretKey),
			Language: "fr",
		})
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		var n int
		require.NoError(t, db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM contacts WHERE email = $1`, gone).Scan(&n))
		assert.Zero(t, n)
	})
}
