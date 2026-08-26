package integration

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestV39WebhookLifecycleMigration executes v39's workspace half against a real
// PostgreSQL. Its system half — the permission backfill — is covered separately
// by TestV39PermissionBackfillMigration, because the two run against different
// databases.
//
// Everything else covering this half is a string assertion: the unit tests build
// an ordered sqlmock from regexps, and sqlmock never parses SQL. That is enough
// to prove which statements are issued and in what order; it can prove nothing at
// all about whether the reinstalled PL/pgSQL compiles, whether a validating
// foreign key survives what the sweep left behind, or what any of the reinstalled
// trigger bodies do when a real row is written. Those failures land inside the
// transaction that migrates a customer's workspace, on their startup.
//
// A fresh workspace already carries the post-v39 schema, so this reverts one to
// its pre-v39 shape first and then migrates it forward — inside a transaction,
// the way the migration manager runs it.
func TestV39WebhookLifecycleMigration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	ctx := context.Background()
	factory := suite.DataFactory

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	// ---------------------------------------------------------------------
	// Put the workspace back to its pre-v39 shape.
	// ---------------------------------------------------------------------
	for _, stmt := range []string{
		`ALTER TABLE webhook_deliveries DROP CONSTRAINT IF EXISTS webhook_deliveries_subscription_id_fkey`,
		`DROP INDEX IF EXISTS idx_webhook_deliveries_claimed`,
		`ALTER TABLE webhook_deliveries DROP COLUMN IF EXISTS claimed_at`,
		`ALTER TABLE webhook_subscriptions
			DROP COLUMN IF EXISTS source,
			DROP COLUMN IF EXISTS consecutive_failures,
			DROP COLUMN IF EXISTS failing_since,
			DROP COLUMN IF EXISTS disabled_reason`,
	} {
		_, err := db.ExecContext(ctx, stmt)
		require.NoError(t, err, stmt)
	}

	// A subscription that survives, one delivery belonging to it left stranded
	// in 'delivering' by a worker that died, and one orphan whose subscription is
	// long gone. Orphans are guaranteed to exist on a real upgrade, because
	// webhook_deliveries had no foreign key until now.
	const liveSub = "sub-live-v39"
	_, err = db.ExecContext(ctx, `
		INSERT INTO webhook_subscriptions (id, name, url, secret, settings, enabled)
		VALUES ($1, 'Kept', 'https://example.com/hook', 'whsec_x', $2::jsonb, true)`,
		liveSub, `{"event_types":["contact.created"]}`)
	require.NoError(t, err)

	insertDelivery := func(id, subscriptionID, status string) {
		t.Helper()
		_, err := db.ExecContext(ctx, `
			INSERT INTO webhook_deliveries
				(id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
			VALUES ($1, $2, 'contact.created', '{}'::jsonb, $3, 0, 10, NOW())`,
			id, subscriptionID, status)
		require.NoError(t, err)
	}
	insertDelivery("d-stranded", liveSub, "delivering")
	insertDelivery("d-orphan", "sub-deleted-long-ago", "pending")

	// ---------------------------------------------------------------------
	// Migrate, in one transaction, exactly as the migration manager does.
	// ---------------------------------------------------------------------
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, (&migrations.V39Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, tx),
		"the migration must apply against a real PostgreSQL, not merely issue the right strings")
	require.NoError(t, tx.Commit())

	t.Run("the foreign key exists and is validated, not merely declared", func(t *testing.T) {
		var convalidated bool
		err := db.QueryRowContext(ctx, `
			SELECT convalidated FROM pg_constraint
			WHERE conname = 'webhook_deliveries_subscription_id_fkey'
				AND conrelid = to_regclass('webhook_deliveries')`).Scan(&convalidated)
		require.NoError(t, err, "the constraint must exist")
		// NOT VALID would let the orphans the sweep missed sit under a constraint
		// that never checked them, and the cascade is what keeps a deleted
		// subscription from leaving its queue behind.
		assert.True(t, convalidated)
	})

	t.Run("the orphan is gone and the live delivery is kept", func(t *testing.T) {
		var orphans int
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM webhook_deliveries WHERE id = 'd-orphan'`).Scan(&orphans))
		assert.Equal(t, 0, orphans, "validating the constraint over an orphan would have raised")

		var kept int
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM webhook_deliveries WHERE id = 'd-stranded'`).Scan(&kept))
		assert.Equal(t, 1, kept, "the sweep must delete orphans only")
	})

	t.Run("the stranded row rejoins circulation", func(t *testing.T) {
		var status string
		var claimedAt sql.NullTime
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT status, claimed_at FROM webhook_deliveries WHERE id = 'd-stranded'`).
			Scan(&status, &claimedAt))
		// A row left in 'delivering' matches no predicate: not the claim, which
		// deliberately excludes that status, and not the sweep, until a lease it
		// has no claimed_at to measure has expired.
		assert.Equal(t, "pending", status)
		assert.False(t, claimedAt.Valid)
	})

	t.Run("deleting a subscription now takes its queue with it", func(t *testing.T) {
		insertDelivery("d-cascade", liveSub, "pending")

		_, err := db.ExecContext(ctx, `DELETE FROM webhook_subscriptions WHERE id = $1`, liveSub)
		require.NoError(t, err)

		var left int
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM webhook_deliveries WHERE subscription_id = $1`, liveSub).Scan(&left))
		assert.Equal(t, 0, left, "ON DELETE CASCADE is what stops the next generation of orphans")
	})

	t.Run("re-running the migration changes nothing and raises nothing", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, (&migrations.V39Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, tx))
		require.NoError(t, tx.Commit())

		var convalidated bool
		require.NoError(t, db.QueryRowContext(ctx, `
			SELECT convalidated FROM pg_constraint
			WHERE conname = 'webhook_deliveries_subscription_id_fkey'
				AND conrelid = to_regclass('webhook_deliveries')`).Scan(&convalidated))
		assert.True(t, convalidated)
	})
}

// TestV39WebhookTriggerFiltersAgainstRealPostgres fires real writes at the
// reinstalled trigger bodies.
//
// The list and segment filters are read out of a free-form JSONB column, which
// means the shapes they have to survive are not the shapes the API produces —
// they are every shape anything has ever written. The unit tests assert the
// generator emits jsonb_typeof rather than a key-exists test; only PostgreSQL
// can say what happens when a subscription actually carries `"list_ids": null`
// or `"list_ids": "newsletter"` and a contact is added to a list. And what
// happens is not confined to webhooks: these triggers are AFTER-row triggers on
// the customer's own tables, so a raise here fails their INSERT.
func TestV39WebhookTriggerFiltersAgainstRealPostgres(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	ctx := context.Background()
	factory := suite.DataFactory

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	// Two lists and two segments, so "watching one" is distinguishable from
	// "watching everything".
	for _, l := range []struct{ id, name string }{{"list-watched", "Watched"}, {"list-other", "Other"}} {
		_, err := db.ExecContext(ctx, `
			INSERT INTO lists (id, name, is_double_optin, is_public, created_at, updated_at)
			VALUES ($1, $2, false, true, NOW(), NOW())`, l.id, l.name)
		require.NoError(t, err, l.id)
	}
	for _, s := range []struct{ id, name string }{{"seg-watched", "Watched"}, {"seg-other", "Other"}} {
		_, err := db.ExecContext(ctx, `
			INSERT INTO segments (id, name, color, tree, timezone, version, status)
			VALUES ($1, $2, '#fff', '{}'::jsonb, 'UTC', 1, 'active')`, s.id, s.name)
		require.NoError(t, err, s.id)
	}

	// The shapes a subscription's settings can carry. Only a populated array is
	// a filter; everything else has to mean "no filter", because that is what
	// every subscription written before these fields existed carries and a
	// subscription must never be silenced by a shape nobody anticipated.
	cases := []struct {
		name           string
		settings       string
		expectDelivery bool
	}{
		{"filter absent", `{"event_types":["list.subscribed","segment.left"]}`, true},
		{"filter is an empty array", `{"event_types":["list.subscribed","segment.left"],"list_ids":[],"segment_ids":[]}`, true},
		{"filter is JSON null", `{"event_types":["list.subscribed","segment.left"],"list_ids":null,"segment_ids":null}`, true},
		{"filter is a bare string", `{"event_types":["list.subscribed","segment.left"],"list_ids":"list-watched","segment_ids":"seg-watched"}`, true},
		{"filter is an object", `{"event_types":["list.subscribed","segment.left"],"list_ids":{"id":"list-watched"},"segment_ids":{"id":"seg-watched"}}`, true},
		{"filter names the id being written", `{"event_types":["list.subscribed","segment.left"],"list_ids":["list-watched"],"segment_ids":["seg-watched"]}`, true},
		{"filter names a different id", `{"event_types":["list.subscribed","segment.left"],"list_ids":["list-other"],"segment_ids":["seg-other"]}`, false},
	}

	for i, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			subID := fmt.Sprintf("sub-shape-%02d", i)
			email := fmt.Sprintf("shape%02d@example.com", i)

			_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
			require.NoError(t, err)

			_, err = db.ExecContext(ctx, `
				INSERT INTO webhook_subscriptions (id, name, url, secret, settings, enabled)
				VALUES ($1, $2, 'https://example.com/hook', 'whsec_x', $3::jsonb, true)`,
				subID, tc.name, tc.settings)
			require.NoError(t, err)

			// A real INSERT into the customer's own table. If the guard raises,
			// this is what fails — the contact never joins the list, and the
			// webhook is the least of it.
			_, err = db.ExecContext(ctx, `
				INSERT INTO contact_lists (email, list_id, status, created_at, updated_at)
				VALUES ($1, 'list-watched', 'active', NOW(), NOW())`, email)
			require.NoError(t, err, "the list trigger must not fail the customer's write")

			// And a real DELETE, which is the segment trigger's other branch: NEW
			// is unassigned there, so every read has to come through COALESCE.
			_, err = db.ExecContext(ctx, `
				INSERT INTO contact_segments (email, segment_id, version, matched_at)
				VALUES ($1, 'seg-watched', 1, NOW())`, email)
			require.NoError(t, err)
			_, err = db.ExecContext(ctx, `
				DELETE FROM contact_segments WHERE email = $1 AND segment_id = 'seg-watched'`, email)
			require.NoError(t, err, "the segment trigger must not fail the customer's write")

			countFor := func(eventType string) int {
				var n int
				require.NoError(t, db.QueryRowContext(ctx, `
					SELECT COUNT(*) FROM webhook_deliveries
					WHERE subscription_id = $1 AND event_type = $2`, subID, eventType).Scan(&n))
				return n
			}

			if tc.expectDelivery {
				assert.Equal(t, 1, countFor("list.subscribed"),
					"only a populated array may narrow the fan-out")
				assert.Equal(t, 1, countFor("segment.left"),
					"only a populated array may narrow the fan-out")
			} else {
				assert.Equal(t, 0, countFor("list.subscribed"))
				assert.Equal(t, 0, countFor("segment.left"))
			}
		})
	}

	// The custom_events guards live in the same reinstalled file and read the
	// same free-form column. They used to test for the key rather than the type,
	// so a goal_types of JSON null reached jsonb_array_length — which raises on a
	// scalar and rolls back the customer's events.track write, not just the
	// webhook.
	t.Run("a JSON null custom event filter does not fail the customer's write", func(t *testing.T) {
		email := "customevent@example.com"
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		_, err = db.ExecContext(ctx, `
			INSERT INTO webhook_subscriptions (id, name, url, secret, settings, enabled)
			VALUES ('sub-null-filters', 'Null filters', 'https://example.com/hook', 'whsec_x', $1::jsonb, true)`,
			`{"event_types":["custom_event.created"],"custom_event_filters":{"goal_types":null,"event_names":null}}`)
		require.NoError(t, err)

		_, err = db.ExecContext(ctx, `
			INSERT INTO custom_events (event_name, external_id, email, properties, occurred_at)
			VALUES ('purchase', 'ext-null-filter', $1, '{}'::jsonb, NOW())`, email)
		require.NoError(t, err,
			"a null filter must read as no filter, not raise inside the customer's transaction")

		var n int
		require.NoError(t, db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM webhook_deliveries
			WHERE subscription_id = 'sub-null-filters' AND event_type = 'custom_event.created'`).Scan(&n))
		assert.Equal(t, 1, n, "no filter means every event")
	})
}
