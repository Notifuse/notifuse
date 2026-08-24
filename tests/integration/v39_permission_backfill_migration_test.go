package integration

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// The permission columns v39 has to cope with, written exactly as a pre-39
// install stores them.
const (
	// segments narrowed to read-only by an owner: the grant the backfill must not
	// widen back. contacts is along for the ride as an untouched bystander.
	v39NarrowedSegments = `{"contacts": {"read": true, "write": true}, "segments": {"read": true, "write": false}}`
	// The common case: a membership created before the three resources existed.
	v39NoNewResources = `{"contacts": {"read": true, "write": false}}`
	// A row that already holds all three, each deliberately narrowed. The guard
	// must skip it whole — a merge here would overwrite every one of them.
	v39AlreadyGranted = `{"segments": {"read": true, "write": false}, ` +
		`"webhook_subscriptions": {"read": false, "write": false}, ` +
		`"webhook_events": {"read": true, "write": false}}`
)

// TestV39PermissionBackfillMigration runs the compiled v39 against a real system
// database. The sqlmock unit tests pin the statements textually — the operand
// order around ||, the jsonb_typeof guard, the order the four statements are
// issued in — but sqlmock never executes SQL, so the property the whole design
// rests on goes unverified: Postgres' jsonb || is right-operand-wins, therefore
// `defaults || permissions` cannot widen a stored narrow grant.
//
// This exercises it against Postgres, on every row shape a pre-39 install can
// hold, for both tables the migration touches.
func TestV39PermissionBackfillMigration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	db := suite.DBManager.GetDB()
	factory := suite.DataFactory
	ctx := context.Background()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	inviter, err := factory.CreateUser()
	require.NoError(t, err)

	// seedMember inserts a membership whose permissions column is exactly the given
	// jsonb literal — or SQL NULL when raw is nil — and returns the member's user id.
	// role is "member", never "owner": an owner short-circuits HasPermission and so
	// has no stake in what the column holds.
	seedMember := func(t *testing.T, raw *string) string {
		t.Helper()
		user, err := factory.CreateUser()
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `
			INSERT INTO user_workspaces (user_id, workspace_id, role, permissions, created_at, updated_at)
			VALUES ($1, $2, 'member', $3::jsonb, NOW(), NOW())`,
			user.ID, workspace.ID, raw)
		require.NoError(t, err)
		return user.ID
	}

	// seedInvitation inserts a pending invitation with the same permissions shapes.
	// An invitation carries the permissions the member will be created with, so it
	// needs the identical backfill or the invitee lands short.
	seedInvitation := func(t *testing.T, raw *string) string {
		t.Helper()
		id := uuid.New().String()
		_, err := db.ExecContext(ctx, `
			INSERT INTO workspace_invitations (id, workspace_id, inviter_id, email, permissions, expires_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5::jsonb, NOW() + INTERVAL '7 days', NOW(), NOW())`,
			id, workspace.ID, inviter.ID, id+"@example.com", raw)
		require.NoError(t, err)
		return id
	}

	// memberPermissions returns the membership's permissions column both as raw text
	// (so a case can tell SQL NULL apart from '{}') and as the map the application
	// scans out of it.
	memberPermissions := func(t *testing.T, userID string) (sql.NullString, domain.UserPermissions) {
		t.Helper()
		var raw sql.NullString
		var perms domain.UserPermissions
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT permissions::text, permissions FROM user_workspaces WHERE user_id = $1 AND workspace_id = $2`,
			userID, workspace.ID).Scan(&raw, &perms))
		return raw, perms
	}

	invitationPermissions := func(t *testing.T, id string) (sql.NullString, domain.UserPermissions) {
		t.Helper()
		var raw sql.NullString
		var perms domain.UserPermissions
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT permissions::text, permissions FROM workspace_invitations WHERE id = $1`, id).
			Scan(&raw, &perms))
		return raw, perms
	}

	narrowed := v39NarrowedSegments
	missing := v39NoNewResources
	granted := v39AlreadyGranted
	empty := `{}`

	narrowedMember := seedMember(t, &narrowed)
	missingMember := seedMember(t, &missing)
	grantedMember := seedMember(t, &granted)
	emptyMember := seedMember(t, &empty)
	nullMember := seedMember(t, nil)

	narrowedInvitation := seedInvitation(t, &narrowed)
	missingInvitation := seedInvitation(t, &missing)
	nullInvitation := seedInvitation(t, nil)

	grantedBefore, _ := memberPermissions(t, grantedMember)

	require.NoError(t, (&migrations.V39Migration{}).UpdateSystem(ctx, &config.Config{}, db))

	full := domain.ResourcePermissions{Read: true, Write: true}
	readOnly := domain.ResourcePermissions{Read: true}

	t.Run("a narrowed grant survives the merge", func(t *testing.T) {
		_, perms := memberPermissions(t, narrowedMember)

		assert.Equal(t, readOnly, perms[domain.PermissionResourceSegments],
			"segments was deliberately narrowed to read-only; the backfill must not widen it")
		assert.Equal(t, full, perms[domain.PermissionResourceWebhookSubscriptions],
			"the two resources the row lacks must still be granted")
		assert.Equal(t, full, perms[domain.PermissionResourceWebhookEvents])
		assert.Equal(t, full, perms[domain.PermissionResourceContacts],
			"an unrelated resource must be carried through untouched")
	})

	t.Run("a row holding none of the three receives all three", func(t *testing.T) {
		_, perms := memberPermissions(t, missingMember)

		assert.Equal(t, full, perms[domain.PermissionResourceSegments])
		assert.Equal(t, full, perms[domain.PermissionResourceWebhookSubscriptions])
		assert.Equal(t, full, perms[domain.PermissionResourceWebhookEvents])
		assert.Equal(t, readOnly, perms[domain.PermissionResourceContacts],
			"a narrowed unrelated resource must not be widened either")
	})

	t.Run("a row already holding all three is left alone", func(t *testing.T) {
		after, perms := memberPermissions(t, grantedMember)

		assert.Equal(t, grantedBefore, after, "the guard must skip the row entirely")
		assert.Equal(t, readOnly, perms[domain.PermissionResourceSegments])
		assert.Equal(t, domain.ResourcePermissions{}, perms[domain.PermissionResourceWebhookSubscriptions])
		assert.Equal(t, readOnly, perms[domain.PermissionResourceWebhookEvents])
	})

	t.Run("a SQL NULL row is normalised to an empty object and granted nothing", func(t *testing.T) {
		// The row the statement order is arranged around: normalise before the grants
		// and jsonb_typeof reports 'object' for it, offering a zero-permission member
		// read+write on all three. Both the ordering and the '{}' exclusion in the
		// grants have to hold for this to come out empty.
		raw, perms := memberPermissions(t, nullMember)

		require.True(t, raw.Valid, "the NULL must be normalised so the console can edit the row")
		assert.JSONEq(t, `{}`, raw.String)
		assert.Empty(t, perms, "a row that had no permissions must gain none")

		member := &domain.UserWorkspace{Role: "member", Permissions: perms}
		assert.False(t, member.HasPermission(domain.PermissionResourceSegments, domain.PermissionTypeRead))
		assert.False(t, member.HasPermission(domain.PermissionResourceWebhookSubscriptions, domain.PermissionTypeWrite))
		assert.False(t, member.HasPermission(domain.PermissionResourceWebhookEvents, domain.PermissionTypeWrite))
	})

	t.Run("a row that is already an empty object is granted nothing", func(t *testing.T) {
		// The same rule as the normalised NULL row, and the reason a re-run is
		// harmless: after the first pass those two rows are indistinguishable.
		raw, perms := memberPermissions(t, emptyMember)

		assert.JSONEq(t, `{}`, raw.String)
		assert.Empty(t, perms, "a member with no permissions must not gain three resources on upgrade")
	})

	t.Run("invitations get the same treatment as memberships", func(t *testing.T) {
		_, narrowedPerms := invitationPermissions(t, narrowedInvitation)
		assert.Equal(t, readOnly, narrowedPerms[domain.PermissionResourceSegments],
			"an invitation's narrowed grant must survive too")
		assert.Equal(t, full, narrowedPerms[domain.PermissionResourceWebhookSubscriptions])
		assert.Equal(t, full, narrowedPerms[domain.PermissionResourceWebhookEvents])

		_, missingPerms := invitationPermissions(t, missingInvitation)
		assert.Equal(t, full, missingPerms[domain.PermissionResourceSegments])
		assert.Equal(t, full, missingPerms[domain.PermissionResourceWebhookSubscriptions])
		assert.Equal(t, full, missingPerms[domain.PermissionResourceWebhookEvents])

		nullRaw, nullPerms := invitationPermissions(t, nullInvitation)
		require.True(t, nullRaw.Valid)
		assert.JSONEq(t, `{}`, nullRaw.String)
		assert.Empty(t, nullPerms)
	})

	t.Run("re-running the migration changes nothing", func(t *testing.T) {
		// Reachable, not theoretical: Manager.RunMigrations stamps the new version
		// in a statement of its own, after the migration transaction has already
		// committed. A crash in that window replays UpdateSystem over a table where
		// the SQL-NULL rows are now '{}' — and a grant that only skipped rows
		// already holding the three resources would escalate every one of them.
		before := v39PermissionSnapshot(t, db, workspace.ID)

		require.NoError(t, (&migrations.V39Migration{}).UpdateSystem(ctx, &config.Config{}, db))

		assert.Equal(t, before, v39PermissionSnapshot(t, db, workspace.ID))
	})
}

// TestV39PermissionBackfillSkipsJSONScalars covers the row shape that makes the
// guard jsonb_typeof(permissions) = 'object' rather than the obvious
// IS NOT NULL. Concatenating an object onto a JSON scalar does not fail in
// Postgres, it silently produces an ARRAY — 'null'::jsonb || '{"a":1}'::jsonb is
// [null, {"a": 1}] — which no longer scans into UserPermissions at all, locking
// the member out of the workspace instead of widening their access.
func TestV39PermissionBackfillSkipsJSONScalars(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	db := suite.DBManager.GetDB()
	ctx := context.Background()

	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)
	user, err := suite.DataFactory.CreateUser()
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO user_workspaces (user_id, workspace_id, role, permissions, created_at, updated_at)
		VALUES ($1, $2, 'member', 'null'::jsonb, NOW(), NOW())`, user.ID, workspace.ID)
	require.NoError(t, err)

	// The premise: unguarded, the merge really does turn this row into an array.
	var mergedKind string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT jsonb_typeof('{"segments": {"read": true}}'::jsonb || 'null'::jsonb)`).Scan(&mergedKind))
	require.Equal(t, "array", mergedKind)

	require.NoError(t, (&migrations.V39Migration{}).UpdateSystem(ctx, &config.Config{}, db))

	var kind string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT jsonb_typeof(permissions) FROM user_workspaces WHERE user_id = $1 AND workspace_id = $2`,
		user.ID, workspace.ID).Scan(&kind))
	assert.NotEqual(t, "array", kind, "a scalar column must be skipped, not concatenated into an array")

	// The column still scans, which is what "not locked out" actually means.
	var perms domain.UserPermissions
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT permissions FROM user_workspaces WHERE user_id = $1 AND workspace_id = $2`,
		user.ID, workspace.ID).Scan(&perms))
	assert.Empty(t, perms)
}

// v39PermissionSnapshot reads every permissions column v39 touches for a
// workspace, keyed by row, as raw text so '{}' and SQL NULL stay distinguishable.
func v39PermissionSnapshot(t *testing.T, db *sql.DB, workspaceID string) map[string]sql.NullString {
	t.Helper()

	snapshot := map[string]sql.NullString{}
	for _, q := range []struct{ label, query string }{
		{"member", `SELECT user_id, permissions::text FROM user_workspaces WHERE workspace_id = $1`},
		{"invitation", `SELECT id, permissions::text FROM workspace_invitations WHERE workspace_id = $1`},
	} {
		rows, err := db.QueryContext(context.Background(), q.query, workspaceID)
		require.NoError(t, err)

		for rows.Next() {
			var key string
			var raw sql.NullString
			require.NoError(t, rows.Scan(&key, &raw))
			snapshot[q.label+":"+key] = raw
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
	}
	return snapshot
}
