//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestWebAnalyticsIdentityStickiness locks down how a verified contact email
// attaches to a session, at the database level rather than by asserting on the
// generated SQL string.
//
// The rule is asymmetric on purpose and every half of it matters: a later beat
// that knows the contact must re-stamp the pages and goals that were already
// written anonymously (otherwise identifying someone mid-session loses the
// landing page and any earlier conversion), a later beat that does NOT know the
// contact must not erase it, and a beat carrying a DIFFERENT contact must win
// (a shared browser, a second login).
func TestWebAnalyticsIdentityStickiness(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{
			Enabled: true,
			Filters: domain.DefaultWebFilters(),
		}
	})
	require.NoError(t, err)

	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	now := time.Now().UTC()
	sessionDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	sessionID := waUUIDv7At(now.Add(-6*time.Minute), 0x81)
	ctx := context.Background()

	// beat writes one cumulative snapshot straight through the buffer, which is
	// the same path ingest uses. Driving it here rather than over HTTP keeps this
	// test about the upsert rules alone — nothing populates contact_email from a
	// payload until W2 lands.
	beat := func(t *testing.T, seq int64, email *string) {
		t.Helper()
		session := &domain.WebSession{
			SessionDate: sessionDate, ID: sessionID, BeatSeq: seq,
			CreatedAt: now.Add(-6 * time.Minute), UpdatedAt: now,
			PageviewCount: 2, DurationMs: 3000,
			LandingPage:  "https://shop.example.com/landing",
			ContactEmail: email,
		}
		pages := []*domain.WebPage{
			{SessionDate: sessionDate, SessionID: sessionID, PageNumber: 1, BeatSeq: seq,
				Path: "/landing", EnteredAt: now.Add(-6 * time.Minute), ExitedAt: now.Add(-4 * time.Minute),
				DurationMs: 1000, ContactEmail: email},
			{SessionDate: sessionDate, SessionID: sessionID, PageNumber: 2, BeatSeq: seq,
				Path: "/pricing", EnteredAt: now.Add(-4 * time.Minute), ExitedAt: now.Add(-1 * time.Minute),
				DurationMs: 2000, ContactEmail: email},
		}
		goals := []*domain.WebGoal{
			{SessionDate: sessionDate, SessionID: sessionID, GoalName: "signup", ClientTsMs: 4242,
				BeatSeq: seq, GoalAt: now.Add(-2 * time.Minute), GoalValue: 5, ContactEmail: email},
		}
		buffer.Add(workspace.ID, 0, session, pages, goals)
		buffer.FlushAll(ctx)
	}

	// Reads the identity off all three tables at once: the guarantee is that they
	// agree, so asserting only on web_sessions would miss a half-applied rule.
	identities := func(t *testing.T) (session *string, pages []*string, goal *string) {
		t.Helper()
		require.NoError(t, wsDB.QueryRow(
			`SELECT contact_email FROM web_sessions WHERE id = $1`, sessionID).Scan(&session))
		rows, err := wsDB.Query(
			`SELECT contact_email FROM web_pages WHERE session_id = $1 ORDER BY page_number`, sessionID)
		require.NoError(t, err)
		defer rows.Close()
		for rows.Next() {
			var e *string
			require.NoError(t, rows.Scan(&e))
			pages = append(pages, e)
		}
		require.NoError(t, rows.Err())
		require.NoError(t, wsDB.QueryRow(
			`SELECT contact_email FROM web_goals WHERE session_id = $1`, sessionID).Scan(&goal))
		return session, pages, goal
	}

	alice, bob := "alice@example.com", "bob@example.com"

	t.Run("an anonymous session stores no identity", func(t *testing.T) {
		beat(t, 1, nil)
		session, pages, goal := identities(t)
		assert.Nil(t, session)
		assert.Equal(t, []*string{nil, nil}, pages)
		assert.Nil(t, goal)
	})

	t.Run("identifying mid-session re-stamps the rows already written", func(t *testing.T) {
		// This is why the SDK re-sends the whole cumulative action list: the
		// landing page and the earlier goal were stored anonymously and must
		// become attributed, not stay orphaned.
		beat(t, 2, &alice)
		session, pages, goal := identities(t)
		require.NotNil(t, session)
		assert.Equal(t, alice, *session)
		for i, p := range pages {
			require.NotNil(t, p, "page %d kept no identity", i+1)
			assert.Equal(t, alice, *p)
		}
		require.NotNil(t, goal)
		assert.Equal(t, alice, *goal)
	})

	t.Run("a later anonymous beat does not erase the identity", func(t *testing.T) {
		// A logout, or simply a beat built before the SDK read its stored
		// identity. Losing the contact here would silently un-attribute a
		// session that had already been linked.
		beat(t, 3, nil)
		session, pages, goal := identities(t)
		require.NotNil(t, session)
		assert.Equal(t, alice, *session)
		for _, p := range pages {
			require.NotNil(t, p)
			assert.Equal(t, alice, *p)
		}
		require.NotNil(t, goal)
		assert.Equal(t, alice, *goal)
	})

	t.Run("a different contact wins", func(t *testing.T) {
		// Shared browser, or a second login in the same session. Sticky must
		// mean "never cleared", not "never changed".
		beat(t, 4, &bob)
		session, pages, goal := identities(t)
		require.NotNil(t, session)
		assert.Equal(t, bob, *session)
		for _, p := range pages {
			require.NotNil(t, p)
			assert.Equal(t, bob, *p)
		}
		require.NotNil(t, goal)
		assert.Equal(t, bob, *goal)
	})
}

// TestWebAnalyticsIdentityRequiresKnownContact covers the ingest gate: a
// signature proves who the caller is, never that the address belongs to anyone.
//
// Storing an unknown-but-signed address would mean Notifuse retains the email of
// people who are not contacts, and would leave erasure unenforceable — a deleted
// contact's next beat would simply re-stamp the address. Requiring an existing
// contact makes both problems disappear rather than needing a suppression list.
func TestWebAnalyticsIdentityRequiresKnownContact(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{
			Enabled: true,
			Filters: domain.DefaultWebFilters(),
		}
	})
	require.NoError(t, err)
	require.NotEmpty(t, workspace.Settings.SecretKey, "the workspace secret is what signs an identity")

	baseURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	now := time.Now().UTC()

	identifiedBeat := func(t *testing.T, sessionID, email string, seq int64) {
		t.Helper()
		body, err := json.Marshal(map[string]interface{}{
			"workspace_id":       workspace.ID,
			"session_id":         sessionID,
			"contact_email":      email,
			"contact_email_hmac": domain.ComputeWebIdentifyHMAC(email, workspace.Settings.SecretKey),
			"actions":            []map[string]interface{}{waPageview("/pricing", 1, 1000, 20, now)},
			"attributes": map[string]interface{}{
				"landing_page": "https://shop.example.com/pricing",
				"user_agent":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36",
			},
			"created_at":  now.Add(-2 * time.Minute).UnixMilli(),
			"updated_at":  now.UnixMilli(),
			"sent_at":     now.UnixMilli(),
			"sdk_version": "1.0.0",
			"seq":         seq,
		})
		require.NoError(t, err)
		decoded := waCloseAndDecode(t, waPostBeat(t, baseURL, body, nil))
		require.Equal(t, true, decoded["success"], "an identity must never cost the pageview: %v", decoded)
		buffer.FlushAll(context.Background())
	}

	stored := func(t *testing.T, sessionID string) *string {
		t.Helper()
		var email *string
		require.NoError(t, wsDB.QueryRow(
			`SELECT contact_email FROM web_sessions WHERE id = $1`, sessionID).Scan(&email))
		return email
	}

	t.Run("a signed address that is not a contact is dropped", func(t *testing.T) {
		sessionID := waUUIDv7At(now.Add(-3*time.Minute), 0x91)
		identifiedBeat(t, sessionID, "stranger@example.com", 1)

		assert.Nil(t, stored(t, sessionID), "an unknown address must not be retained")
		var contacts int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM contacts WHERE email = $1`, "stranger@example.com").Scan(&contacts))
		assert.Equal(t, 0, contacts, "ingest must never create a contact")
	})

	t.Run("the same beat identifies once the contact exists", func(t *testing.T) {
		_, err := suite.DataFactory.CreateContact(workspace.ID, func(c *domain.Contact) {
			c.Email = "known@example.com"
		})
		require.NoError(t, err)

		sessionID := waUUIDv7At(now.Add(-3*time.Minute), 0x92)
		identifiedBeat(t, sessionID, "known@example.com", 1)

		got := stored(t, sessionID)
		require.NotNil(t, got)
		assert.Equal(t, "known@example.com", *got)
	})

	t.Run("a forged signature is ignored but the pageview still lands", func(t *testing.T) {
		sessionID := waUUIDv7At(now.Add(-3*time.Minute), 0x93)
		body, err := json.Marshal(map[string]interface{}{
			"workspace_id":       workspace.ID,
			"session_id":         sessionID,
			"contact_email":      "known@example.com",
			"contact_email_hmac": "0000000000000000000000000000000000000000000000000000000000000000",
			"actions":            []map[string]interface{}{waPageview("/pricing", 1, 1000, 20, now)},
			"attributes":         map[string]interface{}{"landing_page": "https://shop.example.com/pricing", "user_agent": "Mozilla/5.0 Chrome/126.0.0.0"},
			"created_at":         now.Add(-2 * time.Minute).UnixMilli(),
			"updated_at":         now.UnixMilli(),
			"sent_at":            now.UnixMilli(),
			"sdk_version":        "1.0.0",
			"seq":                1,
		})
		require.NoError(t, err)
		decoded := waCloseAndDecode(t, waPostBeat(t, baseURL, body, nil))
		require.Equal(t, true, decoded["success"])
		buffer.FlushAll(context.Background())

		assert.Nil(t, stored(t, sessionID), "a bad signature drops the identity, not the beat")
		var pages int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM web_pages WHERE session_id = $1`, sessionID).Scan(&pages))
		assert.Equal(t, 1, pages)
	})
}
