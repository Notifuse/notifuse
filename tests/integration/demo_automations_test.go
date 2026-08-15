//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestDemoAutomationsSeedRealHistory covers the half of the demo automation seeder that only a real
// database can judge.
//
// The service-layer tests run on mocks, so they cannot see a column that does not exist, a
// constraint that rejects the batch, or a stats query that returns two rows where the console
// expects one. Each of those has a specific failure mode here:
//
//   - The cohort queries read `email` from custom_events and contact_lists. `contact_email` is the
//     column name on contact_automations, and getting the two confused is invisible everywhere else
//     — the seed swallows the error and every card ships empty.
//   - contact_automations carries UNIQUE(automation_id, contact_email, entered_at), and
//     automation_node_executions a NOT NULL foreign key onto it.
//   - The scheduler's claim query is the one thing standing between seeded history and a demo host
//     that suddenly starts sending mail. It is asserted here against a *live* automation, which is
//     the state that would actually be dangerous.
func TestDemoAutomationsSeedRealHistory(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	ctx := context.Background()

	// The lists the automations move contacts between. `vipclub` has no hyphen on purpose: list ids
	// are alphanumeric-only, and createSampleLists returns its error, so a hyphen would abort the
	// whole seed.
	for _, listID := range []string{"newsletter", "vipclub"} {
		_, err := suite.DataFactory.CreateList(workspace.ID, func(l *domain.List) {
			l.ID = listID
			l.Name = listID
		})
		require.NoError(t, err, "list %s", listID)
	}

	// A population that produces every journey shape: buyers, cart abandoners, and a few contacts
	// whose newsletter status makes the email node's subscription guard stop them.
	const population = 40
	statuses := []string{"active", "active", "active", "unsubscribed", "bounced", "complained"}
	for i := 0; i < population; i++ {
		email := fmt.Sprintf("demo%02d@example.com", i)
		_, err := suite.DataFactory.CreateContact(workspace.ID, func(c *domain.Contact) {
			c.Email = email
		})
		require.NoError(t, err, "contact %s", email)

		_, err = suite.DataFactory.CreateContactList(workspace.ID, func(cl *domain.ContactList) {
			cl.Email = email
			cl.ListID = "newsletter"
			cl.Status = domain.ContactListStatus(statuses[i%len(statuses)])
		})
		require.NoError(t, err, "subscription for %s", email)

		require.NoError(t, suite.DataFactory.CreateCustomEvent(workspace.ID, email, "add_to_cart", nil))
		if i%3 == 0 {
			require.NoError(t, suite.DataFactory.CreateCustomEvent(workspace.ID, email, "purchase", nil))
		}
	}

	// The factory stamps every event with now, but the win-back cohort is defined by a purchase
	// older than 90 days — the demo's own analytics history spans 400. Age half of them so the
	// sunset flow has anyone to enroll.
	_, err = wsDB.ExecContext(ctx, `
		UPDATE custom_events SET occurred_at = NOW() - INTERVAL '200 days'
		WHERE event_name = 'purchase' AND email IN (
			SELECT email FROM custom_events WHERE event_name = 'purchase' ORDER BY email LIMIT $1
		)`, population/6)
	require.NoError(t, err)

	// The real definitions, written through the factory so the rows exist for the foreign key.
	automations := service.DemoAutomations(workspace.ID)
	require.NotEmpty(t, automations)
	for _, automation := range automations {
		_, err := suite.DataFactory.CreateAutomation(workspace.ID,
			testutil.WithAutomationID(automation.ID),
			testutil.WithAutomationName(automation.Name),
			testutil.WithAutomationStatus(automation.Status),
			testutil.WithAutomationListID(automation.ListID),
			testutil.WithAutomationTrigger(automation.Trigger),
			testutil.WithAutomationRootNodeID(automation.RootNodeID),
			testutil.WithAutomationNodes(automation.Nodes),
		)
		require.NoError(t, err, "automation %s", automation.ID)
	}

	demoSvc := service.NewDemoService(
		suite.ServerManager.GetApp().GetLogger(),
		&config.Config{Environment: "demo"},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		suite.ServerManager.GetApp().GetWorkspaceRepository(),
		nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)

	require.NoError(t, demoSvc.SeedAutomationHistory(ctx, workspace.ID, automations))

	t.Run("every automation ends up with journeys behind its numbers", func(t *testing.T) {
		for _, automation := range automations {
			var journeys int
			require.NoError(t, wsDB.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM contact_automations WHERE automation_id = $1`, automation.ID,
			).Scan(&journeys))
			assert.Positive(t, journeys, "%s has no journey history, so its card reads zero", automation.ID)

			var enrolled int
			require.NoError(t, wsDB.QueryRowContext(ctx,
				`SELECT COALESCE((stats->>'enrolled')::int, -1) FROM automations WHERE id = $1`, automation.ID,
			).Scan(&enrolled))
			assert.Equal(t, journeys, enrolled,
				"%s advertises %d enrolled but has %d journeys", automation.ID, enrolled, journeys)
		}
	})

	// AutomationService.Create overwrites the status with draft, so the three flows that should come
	// to rest paused only get there because the history transaction writes it. A "Draft" badge on a
	// card carrying hundreds of finished journeys is the incoherence this guards against.
	t.Run("the flows come to rest in the status they were meant to", func(t *testing.T) {
		for _, automation := range automations {
			var status string
			require.NoError(t, wsDB.QueryRowContext(ctx,
				`SELECT status FROM automations WHERE id = $1`, automation.ID).Scan(&status))
			assert.Equal(t, string(automation.Status), status, "%s came to rest as %q", automation.ID, status)
		}
	})

	// The guard that matters most. A seeded row must be unclaimable on its own terms — not because
	// the automation happens to be paused, since Pause does not touch these rows and un-pausing would
	// release the whole backlog on the next tick.
	t.Run("the scheduler can claim nothing, even once every automation is live", func(t *testing.T) {
		_, err := wsDB.ExecContext(ctx, `UPDATE automations SET status = 'live'`)
		require.NoError(t, err)
		defer func() {
			_, err := wsDB.ExecContext(ctx, `UPDATE automations SET status = 'paused'`)
			require.NoError(t, err)
		}()

		var claimable int
		require.NoError(t, wsDB.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM contact_automations ca
			JOIN automations a ON ca.automation_id = a.id
			WHERE ca.status = 'active'
			  AND ca.scheduled_at <= $1
			  AND a.status = 'live'
			  AND a.deleted_at IS NULL
		`, time.Now().UTC()).Scan(&claimable))
		assert.Zero(t, claimable, "the scheduler would pick up seeded journeys and start sending")

		var scheduled int
		require.NoError(t, wsDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM contact_automations WHERE scheduled_at IS NOT NULL OR status = 'active'`,
		).Scan(&scheduled))
		assert.Zero(t, scheduled, "a seeded journey is still active or still scheduled")
	})

	// The console reads per-node stats through /api/analytics.query, which groups by (node_id,
	// node_type) while the client keys its map by node_id alone with no ordering. Two groups for one
	// node means one silently overwrites the other on screen.
	t.Run("each node reports exactly one stats group", func(t *testing.T) {
		for _, automation := range automations {
			rows, err := wsDB.QueryContext(ctx, `
				SELECT node_id, COUNT(DISTINCT node_type)
				FROM automation_node_executions
				WHERE automation_id = $1
				GROUP BY node_id
			`, automation.ID)
			require.NoError(t, err)
			defer rows.Close()

			for rows.Next() {
				var nodeID string
				var types int
				require.NoError(t, rows.Scan(&nodeID, &types))
				assert.Equal(t, 1, types, "%s node %s reports %d node_types", automation.ID, nodeID, types)
			}
			require.NoError(t, rows.Err())
		}
	})

	// Only the root node ever carries 'entered', because only the enroll function writes it. A
	// seeded funnel that put it on a delay node would show a number no real workspace can produce.
	t.Run("only the trigger node carries an enrollment row", func(t *testing.T) {
		rows, err := wsDB.QueryContext(ctx, `
			SELECT ne.automation_id, ne.node_id, ne.node_type
			FROM automation_node_executions ne
			WHERE ne.action = 'entered'
		`)
		require.NoError(t, err)
		defer rows.Close()

		roots := map[string]string{}
		for _, automation := range automations {
			roots[automation.ID] = automation.RootNodeID
		}

		seen := 0
		for rows.Next() {
			var automationID, nodeID, nodeType string
			require.NoError(t, rows.Scan(&automationID, &nodeID, &nodeType))
			assert.Equal(t, roots[automationID], nodeID, "an enrollment row sits on a non-root node")
			assert.Equal(t, "trigger", nodeType)
			seen++
		}
		require.NoError(t, rows.Err())
		assert.Positive(t, seen)

		var stray int
		require.NoError(t, wsDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM automation_node_executions WHERE action IN ('skipped', 'processing')`,
		).Scan(&stray))
		assert.Zero(t, stray, "seeded rows use an action the engine never leaves behind")
	})

	// Nothing executes add_to_list in demo, so without the seeder's own write the VIP list would be
	// empty while the post-purchase card claims journeys completed through that very node.
	t.Run("the audiences match what the cards claim", func(t *testing.T) {
		var vipMembers int
		require.NoError(t, wsDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM contact_lists WHERE list_id = 'vipclub' AND deleted_at IS NULL`,
		).Scan(&vipMembers))
		assert.Positive(t, vipMembers, "the VIP list is empty next to a flow that says it fills it")
	})

	// The drawer reads changes->'<key>'->>'new'. A flattened payload still renders — just blank.
	t.Run("the contact timeline carries the journeys", func(t *testing.T) {
		var starts, ends int
		require.NoError(t, wsDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM contact_timeline WHERE kind = 'automation.start'`).Scan(&starts))
		require.NoError(t, wsDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM contact_timeline WHERE kind = 'automation.end'`).Scan(&ends))
		assert.Positive(t, starts)
		assert.Equal(t, starts, ends)

		var enveloped int
		require.NoError(t, wsDB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM contact_timeline
			WHERE kind = 'automation.start'
			  AND changes->'automation_id'->>'new' IS NOT NULL
		`).Scan(&enveloped))
		assert.Equal(t, starts, enveloped, "changes is not in the {key:{new:...}} shape the drawer reads")

		// Dated when the journey ran, not when the seed ran: an entry stamped now would sort above
		// the cart event that caused it.
		var future int
		require.NoError(t, wsDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM contact_timeline WHERE entity_type = 'automation' AND created_at > NOW()`,
		).Scan(&future))
		assert.Zero(t, future, "a journey is recorded as finishing in the future")
	})

	// A cohort member who never did the thing that triggers the flow would be a contact a prospect
	// can click into and find nothing behind.
	t.Run("every enrolled contact has the event that would have triggered them", func(t *testing.T) {
		var orphans int
		require.NoError(t, wsDB.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM contact_automations ca
			WHERE ca.automation_id = 'cart-recovery'
			  AND NOT EXISTS (
			      SELECT 1 FROM custom_events ce
			      WHERE ce.email = ca.contact_email AND ce.event_name = 'add_to_cart' AND ce.deleted_at IS NULL
			  )
		`).Scan(&orphans))
		assert.Zero(t, orphans, "a cart-recovery journey belongs to a contact with no cart event")
	})
}
