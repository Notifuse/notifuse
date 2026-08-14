//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestDemoWebActivityOnTheContactTimeline covers the path the demo seeder takes,
// which is not the path any other test exercises.
//
// Every other web-analytics test drives the ingest buffer with beats stamped a
// few minutes ago. The demo writes months of settled history straight through
// the repository and then asks for it to be projected — so what is under test
// here is specifically that a visit from ten weeks ago produces a timeline entry
// dated ten weeks ago, that its conversions become custom events at their own
// timestamps, and that the abandonment segments the demo ships then select the
// contacts they are supposed to select.
//
// It matters because none of that is visible to the service-layer tests: those
// use mock repositories, so a partition that does not exist, a date that gets
// clamped to today, or a segment that compiles but matches nobody would all pass.
func TestDemoWebActivityOnTheContactTimeline(t *testing.T) {
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

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	repo := suite.ServerManager.GetApp().GetWebAnalyticsRepository()
	ctx := context.Background()
	now := time.Now().UTC()

	// Ten weeks back: inside the demo's 90-day timeline window, and far enough
	// that it needs a partition the current month does not provide.
	visitAt := now.AddDate(0, 0, -70)
	sessionDate := time.Date(visitAt.Year(), visitAt.Month(), visitAt.Day(), 0, 0, 0, 0, time.UTC)

	require.NoError(t, repo.EnsureMonthlyPartitions(ctx, workspace.ID, []time.Time{
		time.Date(visitAt.Year(), visitAt.Month(), 1, 0, 0, 0, 0, time.UTC),
		time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC),
	}))

	const abandoner = "abandoner@example.com"
	const buyer = "buyer@example.com"
	for _, email := range []string{abandoner, buyer} {
		_, err := suite.DataFactory.CreateContact(workspace.ID, func(c *domain.Contact) {
			c.Email = email
		})
		require.NoError(t, err, "contact %s", email)
	}

	// The demo's generator shape, hand-built: one identified visit of two pages,
	// with the funnel goals hanging off it.
	session := func(email string, salt byte, goalCount int, goalValue float64) *domain.WebSession {
		return &domain.WebSession{
			SessionDate: sessionDate,
			ID:          waUUIDv7At(visitAt, salt),
			CreatedAt:   visitAt,
			UpdatedAt:   visitAt.Add(4 * time.Minute),
			DurationMs:  240000, PageviewCount: 2, MaxScroll: 80,
			LandingPath: "/iphone-17-pro/", ExitPath: "/shop/",
			LandingDomain: "www.apple.com",
			LandingPage:   "https://www.apple.com/iphone-17-pro/",
			GoalCount:     goalCount, GoalValue: goalValue,
			Device: "desktop", Country: "US",
			ContactEmail: &email,
		}
	}

	pages := func(s *domain.WebSession, email string) []*domain.WebPage {
		return []*domain.WebPage{
			{
				SessionDate: sessionDate, SessionID: s.ID, TabID: 1, PageNumber: 1,
				Path: "/iphone-17-pro/", EnteredAt: visitAt, ExitedAt: visitAt.Add(3 * time.Minute),
				DurationMs: 180000, MaxScroll: 80, IsLanding: true, EntryType: "navigation",
				ContactEmail: &email,
			},
			{
				SessionDate: sessionDate, SessionID: s.ID, TabID: 1, PageNumber: 2,
				Path: "/shop/", EnteredAt: visitAt.Add(3 * time.Minute), ExitedAt: visitAt.Add(4 * time.Minute),
				DurationMs: 60000, MaxScroll: 40, IsExit: true, EntryType: "spa",
				ContactEmail: &email,
			},
		}
	}

	goal := func(s *domain.WebSession, email, name, goalType string, at time.Time, value float64) *domain.WebGoal {
		return &domain.WebGoal{
			SessionDate: sessionDate, SessionID: s.ID, TabID: 1,
			GoalName: name, GoalType: goalType,
			ClientTsMs: at.UnixMilli(), GoalAt: at, GoalValue: value,
			Path: "/iphone-17-pro/", LandingPath: "/iphone-17-pro/", PageNumber: 1,
			Device: "desktop", Country: "US",
			Properties:   map[string]string{"product": "iPhone 17 Pro"},
			ContactEmail: &email,
		}
	}

	abandonerSession := session(abandoner, 0xE1, 1, 1199)
	buyerSession := session(buyer, 0xE2, 3, 2398)

	abandonerGoals := []*domain.WebGoal{
		goal(abandonerSession, abandoner, "add_to_cart", domain.GoalTypeOther, visitAt.Add(2*time.Minute), 1199),
	}
	buyerGoals := []*domain.WebGoal{
		goal(buyerSession, buyer, "add_to_cart", domain.GoalTypeOther, visitAt.Add(2*time.Minute), 1199),
		goal(buyerSession, buyer, "checkout_start", domain.GoalTypeOther, visitAt.Add(2*time.Minute+10*time.Second), 0),
		goal(buyerSession, buyer, "purchase", domain.GoalTypePurchase, visitAt.Add(3*time.Minute), 1199),
	}

	sessions := []*domain.WebSession{abandonerSession, buyerSession}
	allPages := append(pages(abandonerSession, abandoner), pages(buyerSession, buyer)...)
	allGoals := append(append([]*domain.WebGoal{}, abandonerGoals...), buyerGoals...)

	require.NoError(t, repo.FlushBatch(ctx, workspace.ID, sessions, allPages, allGoals))
	require.NoError(t, repo.ProjectContactNavigation(ctx, workspace.ID, sessions))

	countRows := func(t *testing.T, email, kind string) int {
		t.Helper()
		var n int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM contact_timeline WHERE email = $1 AND kind = $2`, email, kind).Scan(&n))
		return n
	}

	t.Run("a visit from ten weeks ago reaches the timeline", func(t *testing.T) {
		assert.Equal(t, 1, countRows(t, abandoner, "web.session"))
		assert.Equal(t, 2, countRows(t, abandoner, "web.pageview"))
	})

	t.Run("the timeline entry keeps the date of the visit", func(t *testing.T) {
		// created_at is LEAST(entered_at, CURRENT_TIMESTAMP). A clamp that fired
		// on historical rows would stack the demo's whole history on today and
		// make every contact's drawer identical.
		var createdAt time.Time
		require.NoError(t, wsDB.QueryRow(`
			SELECT created_at FROM contact_timeline
			WHERE email = $1 AND kind = 'web.session'`, abandoner).Scan(&createdAt))
		assert.WithinDuration(t, visitAt, createdAt.UTC(), time.Second)
	})

	t.Run("a pageview carries the domain its path belongs to", func(t *testing.T) {
		// web_pages stores paths only, so without the visit's domain the console
		// cannot turn "/shop/" back into an address anyone can open — and it must
		// not guess one, since a workspace can track a site that is not the one
		// in its settings.
		var domain, path string
		require.NoError(t, wsDB.QueryRow(`
			SELECT changes->'landing_domain'->>'new', changes->'path'->>'new'
			FROM contact_timeline
			WHERE email = $1 AND kind = 'web.pageview' AND changes->'path'->>'new' = '/shop/'`,
			abandoner).Scan(&domain, &path))

		assert.Equal(t, "www.apple.com", domain)
		assert.Equal(t, "/shop/", path)
	})

	t.Run("the visit summary carries what the drawer renders", func(t *testing.T) {
		var landing, exit string
		var pageviews, duration int
		require.NoError(t, wsDB.QueryRow(`
			SELECT changes->'landing_path'->>'new', changes->'exit_path'->>'new',
			       (changes->'pageview_count'->>'new')::int, (changes->'duration_ms'->>'new')::int
			FROM contact_timeline WHERE email = $1 AND kind = 'web.session'`,
			abandoner).Scan(&landing, &exit, &pageviews, &duration))

		assert.Equal(t, "/iphone-17-pro/", landing)
		assert.Equal(t, "/shop/", exit)
		assert.Equal(t, 2, pageviews)
		assert.Equal(t, 240000, duration)
	})

	// The goals, written as the custom events the seeder produces. Spelled out
	// here rather than borrowed from the seeder's own builder: what this test is
	// for is the database's behaviour given a row of this shape, and a service
	// test already pins that the demo and the live bridge build the same one.
	eventRepo := suite.ServerManager.GetApp().GetCustomEventRepository()
	events := make([]*domain.CustomEvent, 0, len(allGoals))
	for _, g := range allGoals {
		goalType := g.GoalType
		occurredAt := time.UnixMilli(g.ClientTsMs).UTC()
		event := &domain.CustomEvent{
			ExternalID: fmt.Sprintf("%s:%d:%s:%d", g.SessionID, g.TabID, g.GoalName, g.ClientTsMs),
			Email:      *g.ContactEmail,
			EventName:  g.GoalName,
			Properties: map[string]interface{}{"session_id": g.SessionID, "path": g.Path},
			OccurredAt: occurredAt,
			Source:     "web_analytics",
			GoalName:   &g.GoalName,
			GoalType:   &goalType,
		}
		if g.GoalValue > 0 {
			value := float64(g.GoalValue)
			event.GoalValue = &value
		}
		events = append(events, event)
	}
	require.NoError(t, eventRepo.BatchInsertNew(ctx, workspace.ID, events))

	t.Run("a conversion becomes a timeline entry at its own timestamp", func(t *testing.T) {
		// The custom_events trigger builds the kind and takes created_at from
		// occurred_at, so a historical conversion has to land in the past too.
		var createdAt time.Time
		require.NoError(t, wsDB.QueryRow(`
			SELECT created_at FROM contact_timeline
			WHERE email = $1 AND kind = 'custom_event.add_to_cart'`, abandoner).Scan(&createdAt))
		assert.WithinDuration(t, visitAt.Add(2*time.Minute), createdAt.UTC(), time.Second)

		assert.Equal(t, 1, countRows(t, buyer, "custom_event.purchase"))
		assert.Equal(t, 0, countRows(t, abandoner, "custom_event.purchase"),
			"the abandoner never bought")
	})

	t.Run("re-seeding writes no second copy", func(t *testing.T) {
		// A demo reset re-runs the whole seed. The projection is keyed on a
		// derived UUID and the events on their external id, so both have to be
		// no-ops the second time round.
		require.NoError(t, repo.ProjectContactNavigation(ctx, workspace.ID, sessions))
		require.NoError(t, eventRepo.BatchInsertNew(ctx, workspace.ID, events))

		assert.Equal(t, 1, countRows(t, abandoner, "web.session"))
		assert.Equal(t, 2, countRows(t, abandoner, "web.pageview"))
		assert.Equal(t, 1, countRows(t, abandoner, "custom_event.add_to_cart"))
	})

	t.Run("the abandonment segments select the right contacts", func(t *testing.T) {
		// The shape createSampleSegments ships: the funnel step the visitor
		// reached, crossed with the absence of a purchase. Compiled and run for
		// real, because a tree that validates and compiles can still match
		// nobody — which is exactly how a showcase segment fails silently.
		abandonmentTree := func(eventName string) *domain.TreeNode {
			name := eventName
			return &domain.TreeNode{
				Kind: "branch",
				Branch: &domain.TreeNodeBranch{
					Operator: "and",
					Leaves: []*domain.TreeNode{
						{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{
							Source: "custom_events_goals",
							CustomEventsGoal: &domain.CustomEventsGoalCondition{
								GoalType: "*", EventName: &name,
								AggregateOperator: "count", Operator: "gte", Value: 1,
								TimeframeOperator: "in_the_last_days", TimeframeValues: []string{"90"},
							},
						}},
						{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{
							Source: "custom_events_goals",
							CustomEventsGoal: &domain.CustomEventsGoalCondition{
								GoalType:          domain.GoalTypePurchase,
								AggregateOperator: "count", Operator: "gte", Value: 1,
								TimeframeOperator: "in_the_last_days", TimeframeValues: []string{"90"},
								Negate: true,
							},
						}},
					},
				},
			}
		}

		matches := func(t *testing.T, tree *domain.TreeNode) []string {
			t.Helper()
			sql, args, err := service.NewQueryBuilder().BuildSQL(tree)
			require.NoError(t, err)

			rows, err := wsDB.Query(sql, args...)
			require.NoError(t, err, "sql: %s", sql)
			defer func() { _ = rows.Close() }()

			var emails []string
			for rows.Next() {
				var email string
				require.NoError(t, rows.Scan(&email))
				emails = append(emails, email)
			}
			require.NoError(t, rows.Err())
			return emails
		}

		for _, tc := range []struct{ name, eventName string }{
			{"cart abandoners", "add_to_cart"},
			{"checkout abandoners", "checkout_start"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				emails := matches(t, abandonmentTree(tc.eventName))
				assert.NotContains(t, emails, buyer, "a contact who bought has not abandoned")
				if tc.eventName == "add_to_cart" {
					assert.Contains(t, emails, abandoner)
				} else {
					// The abandoner never reached checkout, so this audience is
					// genuinely empty here — which is the point of having two.
					assert.NotContains(t, emails, abandoner)
				}
			})
		}
	})

	t.Run("every projected contact is queued for segment recomputation", func(t *testing.T) {
		// Without this the drawer shows the visit while the segments derived from
		// it stay stale until something unrelated touches the contact.
		var queued int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM contact_segment_queue WHERE email = ANY($1)`,
			pq.Array([]string{abandoner, buyer})).Scan(&queued))
		assert.Equal(t, 2, queued, "one row per contact, deduped by the queue's upsert")
	})

	t.Run("an anonymous visit of the same shape reaches nobody", func(t *testing.T) {
		// The seeder filters these out before calling the projection; the SQL
		// discards them too. Both guards are deliberate, so both are checked.
		anonAt := now.AddDate(0, 0, -60)
		anonDate := time.Date(anonAt.Year(), anonAt.Month(), anonAt.Day(), 0, 0, 0, 0, time.UTC)
		require.NoError(t, repo.EnsureMonthlyPartitions(ctx, workspace.ID, []time.Time{
			time.Date(anonAt.Year(), anonAt.Month(), 1, 0, 0, 0, 0, time.UTC),
		}))

		anon := &domain.WebSession{
			SessionDate: anonDate, ID: waUUIDv7At(anonAt, 0xE9),
			CreatedAt: anonAt, UpdatedAt: anonAt.Add(time.Minute),
			DurationMs: 60000, PageviewCount: 1, LandingPath: "/", ExitPath: "/",
		}
		require.NoError(t, repo.FlushBatch(ctx, workspace.ID,
			[]*domain.WebSession{anon},
			[]*domain.WebPage{{
				SessionDate: anonDate, SessionID: anon.ID, TabID: 1, PageNumber: 1,
				Path: "/", EnteredAt: anonAt, ExitedAt: anonAt.Add(time.Minute),
				DurationMs: 60000, IsLanding: true, IsExit: true, EntryType: "navigation",
			}}, nil))
		require.NoError(t, repo.ProjectContactNavigation(ctx, workspace.ID, []*domain.WebSession{anon}))

		var orphans int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM contact_timeline WHERE entity_id LIKE $1`,
			fmt.Sprintf("%s%%", anon.ID)).Scan(&orphans))
		assert.Zero(t, orphans)
	})
}
