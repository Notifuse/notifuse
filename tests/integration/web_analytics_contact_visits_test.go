//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// A contact's Web visits tab: the summary, the sessions page by page, and one session's
// pages and goals, never another visitor's.
func TestWebAnalyticsContactVisits(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	factory := suite.DataFactory
	client := suite.APIClient

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{Enabled: true, Filters: domain.DefaultWebFilters()}
	})
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	wsDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	const ann, bob = "ann@example.com", "bob@example.com"
	for _, email := range []string{ann, bob} {
		_, err := factory.CreateContact(workspace.ID, func(c *domain.Contact) { c.Email = email })
		require.NoError(t, err)
	}

	// The monthly partitions of every session below: the current month, and earlier ones.
	now := time.Now().UTC().Truncate(time.Second)
	previousMonth := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, time.UTC).AddDate(0, 0, -10)
	for _, month := range []time.Time{now, now.Add(-2 * time.Hour), previousMonth} {
		for _, table := range schema.WebAnalyticsTableNames {
			_, err := wsDB.Exec(schema.WebAnalyticsPartitionDDL(table, month))
			require.NoError(t, err)
		}
	}

	type session struct {
		id, date string
		at       time.Time
	}
	insertSession := func(salt byte, email string, at time.Time, durationMs int, channelGroup string, goals int, goalValue float64) session {
		t.Helper()
		s := session{id: waUUIDv7At(at, salt), date: at.Format(time.DateOnly), at: at}
		_, err := wsDB.Exec(`
			INSERT INTO web_sessions (session_date, id, beat_seq, created_at, updated_at, duration_ms, pageview_count,
				landing_path, channel, channel_group, goal_count, goal_value, contact_email)
			VALUES ($1, $2, 1, $3, $3, $4, 1, '/', $5, $5, $6, $7, $8)`,
			s.date, s.id, at, durationMs, channelGroup, goals, goalValue, email)
		require.NoError(t, err)
		return s
	}
	insertPage := func(s session, tab int, enteredAt time.Time, path string, email interface{}) {
		t.Helper()
		_, err := wsDB.Exec(`
			INSERT INTO web_pages (session_date, session_id, tab_id, page_number, path, entered_at, exited_at, duration_ms, contact_email)
			VALUES ($1, $2, $3, 1, $4, $5, $5, 1000, $6)`, s.date, s.id, tab, path, enteredAt, email)
		require.NoError(t, err)
	}
	insertGoal := func(s session, tab int, at time.Time, name string, value float64, email string) {
		t.Helper()
		_, err := wsDB.Exec(`
			INSERT INTO web_goals (session_date, session_id, tab_id, goal_name, client_ts_ms, beat_seq, goal_at, goal_value, path, contact_email)
			VALUES ($1, $2, $3, $4, $5, 1, $6, $7, '/signup', $8)`, s.date, s.id, tab, name, at.UnixMilli(), at, value, email)
		require.NoError(t, err)
	}

	latest := insertSession(0x31, ann, now.Add(-1*time.Hour), 60000, "email", 1, 20)
	middle := insertSession(0x32, ann, now.Add(-2*time.Hour), 10000, "organic", 0, 0)
	oldest := insertSession(0x33, ann, previousMonth, 30000, "email", 0, 0)
	insertSession(0x34, bob, now.Add(-30*time.Minute), 5000, "email", 0, 0)

	// latest has three tabs; tab 0 wrote its page before the visitor was identified.
	insertPage(latest, 0, latest.at, "/landing", nil)
	insertPage(latest, 1, latest.at.Add(time.Minute), "/pricing", ann)
	insertPage(latest, 2, latest.at.Add(2*time.Minute), "/signup", ann)
	insertPage(middle, 0, middle.at, "/pricing", ann)
	insertPage(oldest, 0, oldest.at, "/", ann)
	insertGoal(latest, 2, latest.at.Add(3*time.Minute), "signup", 20, ann)

	get := func(t *testing.T, endpoint string, params map[string]string, out interface{}) (int, string) {
		t.Helper()
		params["workspace_id"] = workspace.ID
		resp, err := client.Get(endpoint, params)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		var raw json.RawMessage
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))
		if resp.StatusCode == http.StatusOK && out != nil {
			require.NoError(t, json.Unmarshal(raw, out))
		}
		return resp.StatusCode, string(raw)
	}
	type visits struct {
		Summary    *domain.ContactVisitsSummary `json:"summary"`
		Sessions   []domain.ContactVisitSession `json:"sessions"`
		NextCursor string                       `json:"next_cursor"`
	}
	ids := func(sessions []domain.ContactVisitSession) []string {
		out := []string{}
		for _, s := range sessions {
			out = append(out, s.ID)
		}
		return out
	}

	require.NoError(t, client.Login(owner.Email, "password"))

	t.Run("summary over all of the contact's sessions, and their first page", func(t *testing.T) {
		var got visits
		status, raw := get(t, "/api/webAnalytics.contactVisits", map[string]string{"email": "Ann@Example.com", "limit": "2"}, &got)
		require.Equal(t, http.StatusOK, status, raw)
		require.NotNil(t, got.Summary)
		assert.Equal(t, 3, got.Summary.Sessions)
		assert.Equal(t, 1, got.Summary.GoalSessions)
		assert.Equal(t, 2, got.Summary.EmailSessions)
		assert.Equal(t, int64(30000), got.Summary.MedianDurationMs)
		require.NotNil(t, got.Summary.FirstSession)
		assert.Equal(t, oldest.id, got.Summary.FirstSession.ID)
		assert.Equal(t, []domain.ContactVisitGoal{{GoalName: "signup", Count: 1, Value: 20}}, got.Summary.Goals)
		assert.Equal(t, []domain.ContactVisitPage{{Path: "/pricing", Views: 2}, {Path: "/", Views: 1}, {Path: "/signup", Views: 1}}, got.Summary.TopPages)

		assert.Equal(t, []string{latest.id, middle.id}, ids(got.Sessions), "bob's session is not listed")
		assert.Equal(t, latest.date, got.Sessions[0].SessionDate)
		require.NotNil(t, got.Sessions[0].FirstGoalName)
		assert.Equal(t, "signup", *got.Sessions[0].FirstGoalName)
		assert.Nil(t, got.Sessions[1].FirstGoalName)
		require.NotEmpty(t, got.NextCursor)

		var next visits
		status, raw = get(t, "/api/webAnalytics.contactVisits", map[string]string{"email": ann, "limit": "2", "cursor": got.NextCursor}, &next)
		require.Equal(t, http.StatusOK, status, raw)
		assert.Nil(t, next.Summary, "no summary after the first page")
		assert.Equal(t, []string{oldest.id}, ids(next.Sessions), "the previous month's partition is read")
		assert.Empty(t, next.NextCursor)
	})

	t.Run("filters: sessions with a goal, sessions from email", func(t *testing.T) {
		var got visits
		_, raw := get(t, "/api/webAnalytics.contactVisits", map[string]string{"email": ann, "filter": "goal"}, &got)
		assert.Equal(t, []string{latest.id}, ids(got.Sessions), raw)
		assert.Equal(t, 3, got.Summary.Sessions, "the summary ignores the filter")

		_, raw = get(t, "/api/webAnalytics.contactVisits", map[string]string{"email": ann, "filter": "email"}, &got)
		assert.Equal(t, []string{latest.id, oldest.id}, ids(got.Sessions), raw)
	})

	t.Run("an email without sessions: zeros and empty lists", func(t *testing.T) {
		var got visits
		status, raw := get(t, "/api/webAnalytics.contactVisits", map[string]string{"email": "nobody@example.com"}, &got)
		require.Equal(t, http.StatusOK, status, raw)
		assert.JSONEq(t, `{"summary":{"sessions":0,"goal_sessions":0,"email_sessions":0,"median_duration_ms":0,"first_session":null,"goals":[],"top_pages":[]},"sessions":[]}`, raw)
	})

	t.Run("a session's pages and goals, every tab, in order", func(t *testing.T) {
		var got domain.ContactSessionPagesResponse
		status, raw := get(t, "/api/webAnalytics.contactSessionPages", map[string]string{
			"email": ann, "session_id": latest.id, "session_date": latest.date,
		}, &got)
		require.Equal(t, http.StatusOK, status, raw)
		paths := []string{}
		for _, p := range got.Pages {
			paths = append(paths, p.Path)
		}
		assert.Equal(t, []string{"/landing", "/pricing", "/signup"}, paths, "the page written before identification included")
		require.Len(t, got.Goals, 1)
		assert.Equal(t, "signup", got.Goals[0].GoalName)
		assert.Equal(t, int64(2), got.Goals[0].TabID)
	})

	t.Run("another contact's email returns nothing for that session", func(t *testing.T) {
		_, raw := get(t, "/api/webAnalytics.contactSessionPages", map[string]string{
			"email": bob, "session_id": latest.id, "session_date": latest.date,
		}, nil)
		assert.JSONEq(t, `{"pages":[],"goals":[]}`, raw)
	})

	t.Run("malformed inputs are a 400, not a database error", func(t *testing.T) {
		for _, tc := range []struct {
			route  string
			params map[string]string
		}{
			{"/api/webAnalytics.contactVisits", map[string]string{"email": "a\x00b@example.com"}},
			{"/api/webAnalytics.contactVisits", map[string]string{"email": "a\xffb@example.com"}},
			{"/api/webAnalytics.contactSessionPages", map[string]string{"email": "a\x00b@example.com", "session_id": latest.id, "session_date": latest.date}},
			{"/api/webAnalytics.contactSessionPages", map[string]string{"email": ann, "session_id": latest.id, "session_date": "0000-01-01"}},
		} {
			status, raw := get(t, tc.route, tc.params, nil)
			assert.Equal(t, http.StatusBadRequest, status, "%s %v: %s", tc.route, tc.params, raw)
		}
	})

	t.Run("a member who may not read web analytics is refused", func(t *testing.T) {
		member, err := factory.CreateUser()
		require.NoError(t, err)
		perms := domain.NewFullPermissions()
		perms[domain.PermissionResourceWebAnalytics] = domain.ResourcePermissions{}
		require.NoError(t, factory.AddUserToWorkspaceWithPermissions(member.ID, workspace.ID, "member", perms))
		require.NoError(t, client.Login(member.Email, "password"))
		defer func() { require.NoError(t, client.Login(owner.Email, "password")) }()

		status, _ := get(t, "/api/webAnalytics.contactVisits", map[string]string{"email": ann}, nil)
		assert.Equal(t, http.StatusForbidden, status)
	})

	t.Run("web analytics disabled is a 400", func(t *testing.T) {
		disabled, err := factory.CreateWorkspace()
		require.NoError(t, err)
		require.NoError(t, factory.AddUserToWorkspace(owner.ID, disabled.ID, "owner"))
		resp, err := client.Get("/api/webAnalytics.contactVisits", map[string]string{"workspace_id": disabled.ID, "email": ann})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Contains(t, body["error"], "web analytics is not enabled")
	})
}
