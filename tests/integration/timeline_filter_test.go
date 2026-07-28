package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestTimelineDimensionFilterExecutes verifies that a segment condition which filters
// contact_timeline events by a field value produces SQL that actually runs against the
// real schema and matches the right contacts.
//
// The query builder previously emitted "ct.metadata->>'field'", but contact_timeline has
// no "metadata" column — the database triggers store event fields in the "changes" JSONB
// as {field: {old, new}}. The generated SQL therefore failed at execution. This test seeds
// production-shaped nested rows and runs the generated SQL end-to-end (no string
// assertions), covering both a numeric and a string filter, positive and negative.
func TestTimelineDimensionFilterExecutes(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	workspaceID := workspace.ID

	workspaceDB, err := factory.GetWorkspaceDB(workspaceID)
	require.NoError(t, err)
	ctx := context.Background()

	buyer, err := factory.CreateContact(workspaceID, testutil.WithContactEmail("buyer@tlfilter.com"))
	require.NoError(t, err)
	shopper, err := factory.CreateContact(workspaceID, testutil.WithContactEmail("shopper@tlfilter.com"))
	require.NoError(t, err)

	// Seed timeline events in the same nested {field: {new: ...}} shape the custom-event
	// trigger writes in production.
	seed := func(email string, changesJSON string) {
		_, err := workspaceDB.ExecContext(ctx,
			`INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, created_at)
			 VALUES ($1, 'insert', 'custom_event', 'custom_event.purchase', $2::jsonb, NOW())`,
			email, changesJSON)
		require.NoError(t, err)
	}
	seed(buyer.Email, `{"goal_type":{"new":"purchase"},"goal_value":{"new":150}}`)
	seed(shopper.Email, `{"goal_type":{"new":"purchase"},"goal_value":{"new":50}}`)

	qb := service.NewQueryBuilder()

	// matches runs the segment SQL generated for the given timeline filters and returns the
	// set of matched emails (restricted to our two seeded contacts).
	matches := func(t *testing.T, filters []*domain.DimensionFilter) map[string]bool {
		tree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "custom_event.purchase",
					CountOperator: "at_least",
					CountValue:    1,
					Filters:       filters,
				},
			},
		}
		sql, args, err := qb.BuildSQL(tree)
		require.NoError(t, err)

		rows, err := workspaceDB.QueryContext(ctx, sql, args...)
		require.NoError(t, err, "generated timeline-filter SQL must execute: %s", sql)
		defer rows.Close()

		found := map[string]bool{}
		for rows.Next() {
			var email string
			require.NoError(t, rows.Scan(&email))
			if email == buyer.Email || email == shopper.Email {
				found[email] = true
			}
		}
		require.NoError(t, rows.Err())
		return found
	}

	t.Run("numeric filter matches only the qualifying contact", func(t *testing.T) {
		got := matches(t, []*domain.DimensionFilter{
			{FieldName: "goal_value", FieldType: "number", Operator: "gte", NumberValues: []float64{100}},
		})
		require.True(t, got[buyer.Email], "buyer (goal_value 150) should match >= 100")
		require.False(t, got[shopper.Email], "shopper (goal_value 50) should not match >= 100")
	})

	t.Run("string filter matches on the nested new value", func(t *testing.T) {
		got := matches(t, []*domain.DimensionFilter{
			{FieldName: "goal_type", FieldType: "string", Operator: "equals", StringValues: []string{"purchase"}},
		})
		require.True(t, got[buyer.Email], "buyer goal_type=purchase should match")
		require.True(t, got[shopper.Email], "shopper goal_type=purchase should match")
	})

	t.Run("string filter excludes non-matching value", func(t *testing.T) {
		got := matches(t, []*domain.DimensionFilter{
			{FieldName: "goal_type", FieldType: "string", Operator: "equals", StringValues: []string{"refund"}},
		})
		require.False(t, got[buyer.Email], "no contact has goal_type=refund")
		require.False(t, got[shopper.Email], "no contact has goal_type=refund")
	})
}
