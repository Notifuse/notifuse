//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// What POST /api/analytics.query refuses, and how: a query the caller has to change answers
// 400 with what to change, never 500 with the database's text. And what it no longer gets
// wrong: the limit and order of a filled series, LIKE wildcards, the bucket order.
func TestAnalyticsQuerySafety(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	factory := suite.DataFactory

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	workspaceRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	require.NoError(t, workspaceRepo.AddUserToWorkspace(context.Background(), &domain.UserWorkspace{
		UserID:      "550e8400-e29b-41d4-a716-446655440000", // testuser@example.com
		WorkspaceID: workspace.ID,
		Role:        "owner",
		Permissions: domain.FullPermissions,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}))
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)

	// Three broadcasts over four days: two messages of spring_sale and one of springXsale on
	// the 1st, four of promo_50% on the 3rd, nothing on the 2nd and the 4th.
	create := func(at time.Time, broadcastID string, count int) {
		t.Helper()
		for i := 0; i < count; i++ {
			_, err := factory.CreateMessageHistory(workspace.ID,
				testutil.WithMessageTemplate(template.ID),
				testutil.WithMessageBroadcast(broadcastID),
				func(m *domain.MessageHistory) {
					m.CreatedAt = at
					m.SentAt = at
					m.UpdatedAt = at
				})
			require.NoError(t, err)
		}
	}
	create(time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC), "spring_sale", 2)
	create(time.Date(2026, 8, 1, 13, 0, 0, 0, time.UTC), "springXsale", 1)
	create(time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC), "promo_50%", 4)

	require.NoError(t, suite.APIClient.Login("testuser@example.com", ""))

	// post sends the query as written, raw JSON, and returns the status and the body.
	post := func(t *testing.T, query string) (int, string) {
		t.Helper()
		resp, err := suite.APIClient.PostRaw("/api/analytics.query",
			fmt.Sprintf(`{"workspace_id":%q,"query":%s}`, workspace.ID, query))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, string(body)
	}
	rows := func(t *testing.T, query string) []map[string]interface{} {
		t.Helper()
		status, body := post(t, query)
		require.Equal(t, http.StatusOK, status, body)
		var decoded struct {
			Data []map[string]interface{} `json:"data"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &decoded))
		return decoded.Data
	}
	refused := func(t *testing.T, query, want string) {
		t.Helper()
		status, body := post(t, query)
		assert.Equal(t, http.StatusBadRequest, status, body)
		assert.Contains(t, body, want)
		assert.NotContains(t, body, "pq:")
	}
	// days renders a series as "day=count" cells.
	days := func(data []map[string]interface{}) []string {
		out := make([]string, len(data))
		for i, row := range data {
			out[i] = fmt.Sprintf("%s=%v", strings.TrimSuffix(row["created_at_day"].(string), "T00:00:00Z"), row["count"])
		}
		return out
	}
	const series = `"schema":"message_history","measures":["count"],
		"timeDimensions":[{"dimension":"created_at","granularity":"day","dateRange":["2026-08-01","2026-08-04"]}]`

	t.Run("a range with too many periods is refused before it is built", func(t *testing.T) {
		// 50001 hours: one past the cap.
		refused(t, `{"schema":"message_history","measures":["count"],
			"timeDimensions":[{"dimension":"created_at","granularity":"hour","dateRange":["2020-01-01T00:00:00Z","2025-09-14T08:00:00Z"]}]}`,
			"50001 periods")
		refused(t, `{"schema":"message_history","measures":["count"],
			"timeDimensions":[{"dimension":"created_at","granularity":"hour","dateRange":["0001-01-01","9999-12-31"]}]}`,
			"periods")
	})

	t.Run("the limit of a filled series keeps its real rows, in order", func(t *testing.T) {
		data := rows(t, `{`+series+`,"order":{"count":"desc"},"limit":2}`)
		assert.Equal(t, []string{"2026-08-03=4", "2026-08-01=3"}, days(data))

		// The offset pages through the filled days; equal ones keep their period order.
		data = rows(t, `{`+series+`,"order":{"count":"desc"},"limit":2,"offset":2}`)
		assert.Equal(t, []string{"2026-08-02=0", "2026-08-04=0"}, days(data))
	})

	t.Run("a filled series sorts by its bucket", func(t *testing.T) {
		data := rows(t, `{`+series+`,"order":{"created_at":"desc"}}`)
		assert.Equal(t, []string{"2026-08-04=0", "2026-08-03=4", "2026-08-02=0", "2026-08-01=3"}, days(data))

		data = rows(t, `{"schema":"message_history","measures":["count"],
			"timeDimensions":[{"dimension":"created_at","granularity":"day"}],
			"filters":[{"member":"created_at","operator":"inDateRange","values":["2026-08-01T00:00:00Z","2026-08-04T00:00:00Z"]}],
			"order":{"created_at_day":"desc"}}`)
		assert.Equal(t, []string{"2026-08-03=4", "2026-08-01=3"}, days(data))
	})

	t.Run("a grouped series with a limit keeps its top groups whole", func(t *testing.T) {
		data := rows(t, `{"schema":"message_history","measures":["count"],"dimensions":["broadcast_id"],
			"timeDimensions":[{"dimension":"created_at","granularity":"day","dateRange":["2026-08-01","2026-08-04"]}],
			"order":{"count":"desc"},"limit":3}`)
		cells := make([]string, len(data))
		for i, row := range data {
			cells[i] = fmt.Sprintf("%s %s=%v", row["broadcast_id"], strings.TrimSuffix(row["created_at_day"].(string), "T00:00:00Z"), row["count"])
		}
		// The rows of the three groups, ranked over the whole range; their filled days follow.
		require.Len(t, cells, 3)
		assert.Equal(t, []string{"promo_50% 2026-08-03=4", "spring_sale 2026-08-01=2", "springXsale 2026-08-01=1"}, cells)
	})

	t.Run("a range from a time of day to a bare end date holds that day, at any granularity", func(t *testing.T) {
		for _, granularity := range []string{"hour", "day", "week", "month", "year"} {
			data := rows(t, fmt.Sprintf(`{"schema":"message_history","measures":["count"],
				"timeDimensions":[{"dimension":"created_at","granularity":%q,"dateRange":["2026-08-01 08:30:00","2026-08-01"]}]}`, granularity))
			total := 0.0
			for _, row := range data {
				total += waNumber(t, row["count"])
			}
			assert.Equal(t, 3.0, total, granularity)
		}
	})

	t.Run("an empty or unknown order direction sorts ascending", func(t *testing.T) {
		for _, direction := range []string{"", "up"} {
			data := rows(t, fmt.Sprintf(`{"schema":"message_history","measures":["count"],"dimensions":["broadcast_id"],"order":{"count":%q}}`, direction))
			require.Len(t, data, 3)
			assert.Equal(t, "springXsale", data[0]["broadcast_id"], direction)
			assert.Equal(t, "promo_50%", data[2]["broadcast_id"], direction)
		}
	})

	t.Run("order by a dimension the query does not group by is refused", func(t *testing.T) {
		refused(t, `{"schema":"message_history","measures":["count"],"order":{"broadcast_id":"asc"}}`,
			`order by dimension \"broadcast_id\" needs it in dimensions`)
	})

	t.Run("several order keys sort in the order the query lists them", func(t *testing.T) {
		ids := func(data []map[string]interface{}) []string {
			out := make([]string, len(data))
			for i, row := range data {
				out[i] = row["broadcast_id"].(string)
			}
			return out
		}
		// Each second key alone would sort the other way round, so only the first key can
		// produce these orders.
		for i := 0; i < 5; i++ {
			assert.Equal(t, []string{"springXsale", "spring_sale", "promo_50%"}, ids(rows(t,
				`{"schema":"message_history","measures":["count"],"dimensions":["broadcast_id"],"order":{"count":"asc","broadcast_id":"asc"}}`)))
			assert.Equal(t, []string{"promo_50%", "spring_sale", "springXsale"}, ids(rows(t,
				`{"schema":"message_history","measures":["count"],"dimensions":["broadcast_id"],"order":{"count":"desc","broadcast_id":"desc"}}`)))
		}
	})

	t.Run("LIKE matches _ and % as written", func(t *testing.T) {
		count := func(operator, value string) float64 {
			data := rows(t, fmt.Sprintf(`{"schema":"message_history","measures":["count"],
				"filters":[{"member":"broadcast_id","operator":%q,"values":[%q]}]}`, operator, value))
			require.Len(t, data, 1)
			return waNumber(t, data[0]["count"])
		}
		assert.Equal(t, 2.0, count("contains", "spring_sale"), "springXsale is not spring_sale")
		assert.Equal(t, 4.0, count("endsWith", "50%"))
		assert.Equal(t, 4.0, count("contains", "%"), "only promo_50% holds a %")
		assert.Equal(t, 5.0, count("notContains", "_sale"), "springXsale holds no _sale")
	})

	t.Run("a measure in filters is refused, pointing to having", func(t *testing.T) {
		refused(t, `{"schema":"message_history","measures":["count"],"filters":[{"member":"count_sent","operator":"gt","values":["1"]}]}`,
			"put conditions on measures in having")
	})

	t.Run("queries the types cannot run are refused with a 400", func(t *testing.T) {
		refused(t, `{"schema":"message_history","measures":["count"],"timeDimensions":[{"dimension":"broadcast_id","granularity":"day"}]}`,
			`time dimension \"broadcast_id\" is a string dimension`)
		refused(t, `{"schema":"email_queue","measures":["count"],"filters":[{"member":"priority","operator":"contains","values":["5"]}]}`,
			`operator \"contains\" does not apply to number dimension \"priority\"`)
		refused(t, `{"schema":"message_history","measures":["count"],"timezone":"Local"}`,
			`timezone \"Local\" is not an IANA time zone name`)
		refused(t, `{"schema":"message_history","measures":["count"],"timeDimensions":[
			{"dimension":"created_at","granularity":"day","dateRange":["2026-08-01","2026-08-04"]},
			{"dimension":"sent_at","granularity":"hour"}]}`, "at most one time dimension")
		// Only the database can tell this one. Its own text stays in the server log.
		refused(t, `{"schema":"message_history","measures":["count"],"filters":[{"member":"created_at","operator":"equals","values":["soon"]}]}`,
			"does not fit its member's type")
	})

	t.Run("an empty timezone is UTC", func(t *testing.T) {
		data := rows(t, `{`+series+`,"timezone":""}`)
		assert.Equal(t, []string{"2026-08-01=3", "2026-08-02=0", "2026-08-03=4", "2026-08-04=0"}, days(data))
	})

	t.Run("the row bound and the body bound", func(t *testing.T) {
		refused(t, `{"schema":"message_history","measures":["count"],"limit":50001}`, "limit must be between 0 and 50000")

		values := strings.Repeat(`"x",`, 300000)
		status, _ := post(t, `{"schema":"message_history","measures":["count"],
			"filters":[{"member":"broadcast_id","operator":"in","values":[`+values+`"x"]}]}`)
		assert.Equal(t, http.StatusRequestEntityTooLarge, status)
	})
}

// The grouped series the console's web analytics assistant asks for (page_path by day over a
// long range, limit 400, by page count descending) over more than 50,000 grouped rows: it
// reads the series of the top groups only, so it answers its top rows instead of a 400.
func TestAnalyticsGroupedSeriesPastMaxRows(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{Enabled: true, BounceThresholdSeconds: 10}
	})
	require.NoError(t, err)
	workspaceRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	require.NoError(t, workspaceRepo.AddUserToWorkspace(context.Background(), &domain.UserWorkspace{
		UserID:      "550e8400-e29b-41d4-a716-446655440000", // testuser@example.com
		WorkspaceID: workspace.ID,
		Role:        "owner",
		Permissions: domain.FullPermissions,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}))

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	for _, month := range []time.Month{time.May, time.June, time.July} {
		_, err := wsDB.Exec(schema.WebAnalyticsPartitionDDL("web_pages", time.Date(2026, month, 1, 0, 0, 0, 0, time.UTC)))
		require.NoError(t, err)
	}

	// 600 paths viewed once a day for 90 days: 54,000 grouped rows. Two of them get a busy day.
	_, err = wsDB.Exec(`
		INSERT INTO web_pages (session_date, session_id, page_number, path, entered_at, exited_at)
		SELECT day::date, gen_random_uuid(), 1, '/p' || lpad(n::text, 3, '0'), day + interval '12 hours', day + interval '12 hours'
		FROM generate_series(1, 600) AS n, generate_series('2026-05-01'::timestamptz, '2026-07-29'::timestamptz, interval '1 day') AS day`)
	require.NoError(t, err)
	_, err = wsDB.Exec(`
		INSERT INTO web_pages (session_date, session_id, page_number, path, entered_at, exited_at)
		SELECT DATE '2026-06-10', gen_random_uuid(), 1, '/p123', TIMESTAMPTZ '2026-06-10 09:00:00+00', TIMESTAMPTZ '2026-06-10 09:00:00+00' FROM generate_series(1, 9)
		UNION ALL
		SELECT DATE '2026-07-02', gen_random_uuid(), 1, '/p456', TIMESTAMPTZ '2026-07-02 09:00:00+00', TIMESTAMPTZ '2026-07-02 09:00:00+00' FROM generate_series(1, 4)`)
	require.NoError(t, err)
	var grouped int
	require.NoError(t, wsDB.QueryRow(`SELECT COUNT(*) FROM (SELECT path, date_trunc('day', entered_at) FROM web_pages GROUP BY 1, 2) AS g`).Scan(&grouped))
	require.Greater(t, grouped, 50000)

	require.NoError(t, suite.APIClient.Login("testuser@example.com", ""))
	resp, err := suite.APIClient.PostRaw("/api/analytics.query", fmt.Sprintf(`{"workspace_id":%q,"query":{
		"schema":"web_pages","measures":["page_count"],"dimensions":["page_path"],
		"timeDimensions":[{"dimension":"entered_at","granularity":"day","dateRange":["2026-05-01","2026-07-29"]}],
		"order":{"page_count":"desc"},"limit":400,"timezone":"UTC"}}`, workspace.ID))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	var decoded struct {
		Data []map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &decoded))
	require.Len(t, decoded.Data, 400)
	assert.Equal(t, "/p123", decoded.Data[0]["page_path"])
	assert.Equal(t, "2026-06-10T00:00:00Z", decoded.Data[0]["entered_at_day"])
	assert.Equal(t, 10.0, waNumber(t, decoded.Data[0]["page_count"]))
	assert.Equal(t, "/p456", decoded.Data[1]["page_path"])
	assert.Equal(t, 5.0, waNumber(t, decoded.Data[1]["page_count"]))
	// Every other row is a real day of one view: none is a zero the filling made up.
	for i, row := range decoded.Data[2:] {
		assert.Equal(t, 1.0, waNumber(t, row["page_count"]), "row %d", i+2)
	}
}
