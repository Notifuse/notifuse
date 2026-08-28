package migrations

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

func TestV39Migration_Metadata(t *testing.T) {
	m := &V39Migration{}
	assert.Equal(t, 39.0, m.GetMajorVersion())
	// Both halves, against two different databases. The permission backfill can
	// only run against the system database, which is the one holding
	// user_workspaces and workspace_invitations; the webhook lifecycle work can
	// only run against a workspace database, which is the one holding
	// webhook_subscriptions, webhook_deliveries and the trigger functions.
	// Dropping either declaration silently skips that half — the dispatcher asks
	// the migration which databases it wants and connects to nothing else — while
	// still stamping the new version, so the work would never be retried.
	assert.True(t, m.HasSystemUpdate())
	assert.True(t, m.HasWorkspaceUpdate())
	assert.False(t, m.ShouldRestartServer())
}

func TestV39Migration_IsRegistered(t *testing.T) {
	migration, ok := GetRegisteredMigration(39.0)
	require.True(t, ok, "v39 must be registered so it runs on startup")
	assert.IsType(t, &V39Migration{}, migration)
}

// v39StatementRecorder is the default regexp matcher plus a log of everything the
// migration issued. Expectations alone can only assert that the statements v39
// should run did run; much of what makes this migration safe lives in the
// statements it must NOT run — a reattached trigger, a second ADD CONSTRAINT, a
// sweep with an extra predicate, a workspace table named from the system half —
// and those are invisible to ExpectExec.
type v39StatementRecorder struct {
	issued []string
}

func (r *v39StatementRecorder) Match(expectedSQL, actualSQL string) error {
	// Every call is recorded, with no de-duplication, because an ordered mock
	// calls this exactly once per statement: sqlmock.New sets ordered, and an
	// ordered mock takes the next unfulfilled expectation and matches against
	// that one alone — it never walks the list looking for a pattern that fits.
	// (Only an unordered mock offers a statement to several expectations.) So
	// the recorded slice is the statements the migration issued, in order, one
	// entry each, and a statement issued twice shows up twice rather than being
	// folded away by a de-duplication step.
	r.issued = append(r.issued, actualSQL)
	return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
}

func (r *v39StatementRecorder) all() string {
	return strings.Join(r.issued, "\n")
}

func (r *v39StatementRecorder) indexOfStatementContaining(t *testing.T, needle string) int {
	t.Helper()
	for i, stmt := range r.issued {
		if strings.Contains(stmt, needle) {
			return i
		}
	}
	require.FailNowf(t, "statement not issued", "no statement containing %q in:\n%s", needle, r.all())
	return -1
}

// ---------------------------------------------------------------------------
// UpdateSystem — the permission backfill, against the system database.
// ---------------------------------------------------------------------------

// The grants must keep four properties. The defaults literal has to sit on the
// LEFT of ||, since the right operand wins on duplicate keys and a stored grant
// must survive the merge. The object check has to be jsonb_typeof, since
// concatenating onto a JSON scalar yields an array that no longer scans into
// UserPermissions. The "already granted" check has to require all three
// resources, so a row holding one of them still receives the other two. And the
// empty object has to be excluded, or a re-run grants everything to the rows the
// normalisation statements just wrote — see TestV39PermissionBackfillMigration
// in tests/integration, which runs these statements against real Postgres.
const (
	v39GrantLiteral = `'\{"segments":\s+\{"read": true, "write": true\},\s+` +
		`"webhook_subscriptions":\s+\{"read": true, "write": true\},\s+` +
		`"webhook_events":\s+\{"read": true, "write": true\}\}'::jsonb\s+\|\|\s+permissions`
	v39GrantGuards = `jsonb_typeof\(permissions\)\s+=\s+'object'\s+` +
		`AND\s+permissions\s+<>\s+'\{\}'::jsonb\s+` +
		`AND\s+NOT\s+\(permissions\s+\?\s+'segments'\s+` +
		`AND\s+permissions\s+\?\s+'webhook_subscriptions'\s+` +
		`AND\s+permissions\s+\?\s+'webhook_events'\)`

	v39UserWorkspacesGrant = `(?s)UPDATE\s+user_workspaces.*` + v39GrantLiteral + `.*` + v39GrantGuards
	v39InvitationsGrant    = `(?s)UPDATE\s+workspace_invitations.*` + v39GrantLiteral + `.*` + v39GrantGuards

	v39UserWorkspacesNormalise = `(?s)UPDATE\s+user_workspaces\s+SET\s+permissions\s+=\s+'\{\}'::jsonb\s+WHERE\s+permissions\s+IS\s+NULL`
	v39InvitationsNormalise    = `(?s)UPDATE\s+workspace_invitations\s+SET\s+permissions\s+=\s+'\{\}'::jsonb\s+WHERE\s+permissions\s+IS\s+NULL`
)

func expectV39Grants(mock sqlmock.Sqlmock) {
	mock.ExpectExec(v39UserWorkspacesGrant).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(v39InvitationsGrant).WillReturnResult(sqlmock.NewResult(0, 1))
}

// A complete, successful system half, with everything it issued recorded.
func v39RecordedSystemRun(t *testing.T) *v39StatementRecorder {
	t.Helper()

	rec := &v39StatementRecorder{}
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(rec))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	expectV39Grants(mock)
	mock.ExpectExec(v39UserWorkspacesNormalise).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(v39InvitationsNormalise).WillReturnResult(sqlmock.NewResult(0, 0))

	m := &V39Migration{}
	require.NoError(t, m.UpdateSystem(context.Background(), &config.Config{}, db))
	require.NoError(t, mock.ExpectationsWereMet())
	return rec
}

func TestV39Migration_UpdateSystem(t *testing.T) {
	m := &V39Migration{}
	ctx := context.Background()

	t.Run("grants the new resources then normalises the null rows", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Without the grants, HasPermission denies segments, webhook_subscriptions
		// and webhook_events for every non-owner member: their stored permission
		// map predates the resources.
		expectV39Grants(mock)
		mock.ExpectExec(v39UserWorkspacesNormalise).WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectExec(v39InvitationsNormalise).WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, m.UpdateSystem(ctx, &config.Config{}, db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("reports a failed user_workspaces grant", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec(v39UserWorkspacesGrant).WillReturnError(errors.New("boom"))

		err = m.UpdateSystem(ctx, &config.Config{}, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "add scoping permissions to user workspaces")
	})

	t.Run("reports a failed invitations grant", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec(v39UserWorkspacesGrant).WillReturnResult(sqlmock.NewResult(0, 3))
		mock.ExpectExec(v39InvitationsGrant).WillReturnError(errors.New("boom"))

		err = m.UpdateSystem(ctx, &config.Config{}, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "add scoping permissions to workspace invitations")
	})

	t.Run("reports a failed user_workspaces normalisation", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		expectV39Grants(mock)
		mock.ExpectExec(v39UserWorkspacesNormalise).WillReturnError(errors.New("boom"))

		err = m.UpdateSystem(ctx, &config.Config{}, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "normalise null permissions on user workspaces")
	})

	t.Run("reports a failed invitations normalisation", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		expectV39Grants(mock)
		mock.ExpectExec(v39UserWorkspacesNormalise).WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectExec(v39InvitationsNormalise).WillReturnError(errors.New("boom"))

		err = m.UpdateSystem(ctx, &config.Config{}, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "normalise null permissions on workspace invitations")
	})
}

// The normalisation statements must run after both grants. Run first, they would
// turn every SQL-NULL permissions column into '{}', which jsonb_typeof reports as
// 'object' — offering a zero-permission member to the grants. The '{}' exclusion
// pinned above refuses that row too, so this order is the second defence against
// the same escalation, not the only one.
//
// The expectations above already pin this order rather than merely tolerating
// it: sqlmock.New returns an ordered mock, which matches each statement against
// the next unfulfilled expectation and nothing else, so a normalisation issued
// before a grant fails there instead of finding its own pattern further down the
// list. Two things follow. MatchExpectationsInOrder(false) must not be added to
// those tests — it would unpin the order silently, leaving this test as the only
// check. And this test is not redundant with them: it says the requirement out
// loud instead of leaving it resting on a library default, and adds what the
// default cannot — that these four statements are the only four issued.
func TestV39Migration_UpdateSystem_StatementOrder(t *testing.T) {
	rec := v39RecordedSystemRun(t)

	require.Len(t, rec.issued, 4, "issued statements:\n%s", rec.all())
	assert.Regexp(t, v39UserWorkspacesGrant, rec.issued[0])
	assert.Regexp(t, v39InvitationsGrant, rec.issued[1])
	assert.Regexp(t, v39UserWorkspacesNormalise, rec.issued[2])
	assert.Regexp(t, v39InvitationsNormalise, rec.issued[3])
}

// ---------------------------------------------------------------------------
// UpdateWorkspace — the webhook lifecycle schema, against a workspace database.
// ---------------------------------------------------------------------------

// The statements the workspace half issues, in the order it issues them. They
// are pinned by regexp rather than by behaviour because the whole point of this
// half is which SQL it does and does not emit: a bounded lock wait before
// anything takes a lock, catalogue guards rather than bare DDL, a sweep before
// the constraint that would otherwise raise on what it left behind, and nothing
// that reattaches a trigger or touches a row it did not have to.
const (
	v39LockTimeout = `SET LOCAL lock_timeout = '5s'`

	v39SubscriptionColumns = `(?s)ALTER TABLE\s+webhook_subscriptions\s+` +
		`ADD COLUMN IF NOT EXISTS source VARCHAR\(32\),\s+` +
		`ADD COLUMN IF NOT EXISTS consecutive_failures INT NOT NULL DEFAULT 0,\s+` +
		`ADD COLUMN IF NOT EXISTS failing_since TIMESTAMPTZ,\s+` +
		`ADD COLUMN IF NOT EXISTS disabled_reason TEXT`

	v39DeliveryColumn = `(?s)ALTER TABLE\s+webhook_deliveries\s+` +
		`ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ`

	v39OrphanSweep = `(?s)DELETE FROM webhook_deliveries d\s+` +
		`WHERE NOT EXISTS \(\s+` +
		`SELECT 1 FROM webhook_subscriptions s WHERE s\.id = d\.subscription_id\s+\)`

	// Both branches, in one pattern: the constraint is added when it is absent
	// and validated when it is present but unvalidated.
	v39ForeignKey = `(?s)IF NOT EXISTS \(\s*SELECT 1 FROM pg_constraint\s+` +
		`WHERE conname = 'webhook_deliveries_subscription_id_fkey'\s+` +
		`AND conrelid = to_regclass\('webhook_deliveries'\).*` +
		`ADD CONSTRAINT webhook_deliveries_subscription_id_fkey\s+` +
		`FOREIGN KEY \(subscription_id\) REFERENCES webhook_subscriptions\(id\)\s+` +
		`ON DELETE CASCADE;.*` +
		`ELSIF EXISTS \(.*AND NOT convalidated.*` +
		`VALIDATE CONSTRAINT webhook_deliveries_subscription_id_fkey`

	v39ResetDelivering = `(?s)UPDATE webhook_deliveries\s+` +
		`SET status = 'pending', claimed_at = NULL\s+` +
		`WHERE status = 'delivering'`

	v39ClaimedIndex = `(?s)CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_claimed\s+` +
		`ON webhook_deliveries\(claimed_at\) WHERE status = 'delivering'`
)

// The trigger functions v39 converges. These are the names the workspace
// initializer attaches its triggers to, so a generator that stopped emitting one
// of them would leave that trigger running whichever body the workspace happened
// to inherit.
var v39TriggerFunctions = []string{
	"webhook_contacts_trigger",
	"webhook_contact_lists_trigger",
	"webhook_contact_segments_trigger",
	"webhook_message_history_trigger",
	"webhook_custom_events_trigger",
}

// v39Statement is one statement the workspace half is expected to issue: the
// pattern the mock matches it by, the fragment its failure must put in the
// returned error, and the rows it reports affected — which the migration
// ignores, and which the sweep and the reset are given non-zero values for so
// that a run which starts reading them fails here rather than in production.
type v39Statement struct {
	name    string
	pattern string
	errText string
	rows    int64
}

// The whole sequence, in issue order. Driving both the happy path and the
// failure paths off one list is what keeps the two from drifting: a statement
// added to the migration without being added here fails the ordered mock, and it
// cannot be added here without also acquiring a failure-attribution case.
func v39StatementSequence() []v39Statement {
	seq := []v39Statement{{
		name:    "lock timeout",
		pattern: v39LockTimeout,
		errText: "set lock timeout",
	}}

	for _, fn := range v39TriggerFunctions {
		seq = append(seq, v39Statement{
			name:    "trigger reinstall of " + fn,
			pattern: `CREATE OR REPLACE FUNCTION ` + fn + `\(\)`,
			errText: "reinstall webhook trigger functions",
		})
	}

	return append(seq,
		v39Statement{
			name:    "subscription columns",
			pattern: v39SubscriptionColumns,
			errText: "add lifecycle columns to webhook_subscriptions",
		},
		v39Statement{
			name:    "delivery column",
			pattern: v39DeliveryColumn,
			errText: "add claimed_at to webhook_deliveries",
		},
		v39Statement{
			name:    "orphan sweep",
			pattern: v39OrphanSweep,
			errText: "sweep orphaned webhook deliveries",
			rows:    3,
		},
		v39Statement{
			name:    "foreign key",
			pattern: v39ForeignKey,
			errText: "add webhook_deliveries subscription foreign key",
		},
		v39Statement{
			name:    "delivering reset",
			pattern: v39ResetDelivering,
			errText: "reset stranded delivering rows",
			rows:    2,
		},
		v39Statement{
			name:    "claimed index",
			pattern: v39ClaimedIndex,
			errText: "create webhook_deliveries claimed index",
		},
	)
}

func expectV39Workspace(mock sqlmock.Sqlmock) {
	for _, stmt := range v39StatementSequence() {
		mock.ExpectExec(stmt.pattern).WillReturnResult(sqlmock.NewResult(0, stmt.rows))
	}
}

// A complete, successful workspace half, with everything it issued recorded.
func v39RecordedWorkspaceRun(t *testing.T) *v39StatementRecorder {
	t.Helper()

	rec := &v39StatementRecorder{}
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(rec))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	expectV39Workspace(mock)

	m := &V39Migration{}
	require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
	return rec
}

func TestV39Migration_UpdateWorkspace(t *testing.T) {
	m := &V39Migration{}
	ctx := context.Background()
	workspace := &domain.Workspace{ID: "ws1"}

	t.Run("issues every statement in order", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		expectV39Workspace(mock)

		require.NoError(t, m.UpdateWorkspace(ctx, &config.Config{}, workspace, db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	// A migration runs in one transaction per database, so any failure rolls the
	// whole thing back — but only if the error is returned rather than swallowed,
	// and only if it names the statement that failed. The lock timeout makes
	// failure a designed outcome here rather than a surprise, so an unattributed
	// error would be read on a real startup, under pressure.
	sequence := v39StatementSequence()
	for i, failing := range sequence {
		t.Run("reports a failed "+failing.name+" statement", func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			for _, preceding := range sequence[:i] {
				mock.ExpectExec(preceding.pattern).WillReturnResult(sqlmock.NewResult(0, preceding.rows))
			}
			mock.ExpectExec(failing.pattern).WillReturnError(errors.New("boom"))

			err = m.UpdateWorkspace(ctx, &config.Config{}, workspace, db)
			require.Error(t, err)
			assert.Contains(t, err.Error(), failing.errText)
			assert.Contains(t, err.Error(), "boom", "the driver error must survive wrapping")
		})
	}
}

// The lock timeout is what lets this migration yield instead of queueing every
// customer write behind it, so it has to be in force before the first statement
// that takes a lock. SET LOCAL rather than SET, so it dies with the transaction
// instead of riding a pooled connection into request traffic.
func TestV39Migration_BoundsItsLockWaitBeforeDoingAnything(t *testing.T) {
	rec := v39RecordedWorkspaceRun(t)

	require.NotEmpty(t, rec.issued)
	assert.Equal(t, 0, rec.indexOfStatementContaining(t, "lock_timeout"),
		"the lock timeout must be set before any statement that takes a lock")
	assert.Contains(t, rec.issued[0], "SET LOCAL",
		"a session-level SET would outlive the migration on a pooled connection")
}

// The constraint is added validating, not NOT VALID. Deferring validation is a
// real technique, but it buys nothing inside a single transaction: the ADD
// COLUMN earlier in this migration already holds an AccessExclusiveLock on
// webhook_deliveries until commit, so the weaker ShareUpdateExclusiveLock a
// separate VALIDATE would take is moot, and the deferral would cost a second
// pass over the table for the same end state.
//
// Both catalogue branches have to be there. Absent is the workspace upgrading
// into this release; present but unvalidated is not reachable from a released
// build but is cheap to reconcile and would otherwise leave the constraint
// unvalidated forever; present and valid is the workspace created from the
// initializer, which declares the constraint inline, and re-validating it would
// re-scan the table for a result the catalogue already holds.
func TestV39Migration_AddsAValidatedConstraintWhateverStateItFinds(t *testing.T) {
	rec := v39RecordedWorkspaceRun(t)
	stmt := rec.issued[rec.indexOfStatementContaining(t, "ADD CONSTRAINT")]

	assert.Contains(t, stmt, "ADD CONSTRAINT webhook_deliveries_subscription_id_fkey",
		"the name must match the one the workspace initializer declares inline")
	assert.Contains(t, stmt, "ON DELETE CASCADE",
		"without the cascade, deleting a subscription strands its queued deliveries again")
	assert.NotContains(t, stmt, "NOT VALID",
		"a constraint left unvalidated inside a transaction that already holds the table's strongest lock")

	assert.Contains(t, stmt, "VALIDATE CONSTRAINT webhook_deliveries_subscription_id_fkey")
	assert.Contains(t, stmt, "AND NOT convalidated",
		"existence alone would re-scan the table on a workspace that declared the constraint inline")
	assert.Contains(t, stmt, "pg_constraint", "the constraint needs a catalogue guard")

	// Dropping and re-adding would be the other way to reach a validated
	// constraint, and it would leave a window inside the transaction where
	// nothing enforces it.
	assert.NotContains(t, rec.all(), "DROP CONSTRAINT")
}

// Sweeping has to precede the constraint work. Validation raises on the first
// row that violates the constraint, so doing it first would abort the migration
// on every workspace that has an orphan — which is exactly the population this
// migration exists for, since webhook_deliveries had no foreign key until now.
func TestV39Migration_SweepsBeforeItTouchesTheConstraint(t *testing.T) {
	rec := v39RecordedWorkspaceRun(t)
	assert.Less(t,
		rec.indexOfStatementContaining(t, "DELETE FROM webhook_deliveries"),
		rec.indexOfStatementContaining(t, "ADD CONSTRAINT"),
	)
}

// The sweep deletes deliveries whose subscription is gone and nothing else. A
// stray extra predicate would either leave poison pills behind or, far worse,
// delete deliveries that are still routable — queued events a subscriber is
// waiting for, gone with no trace.
func TestV39Migration_SweepsOnlyGenuineOrphans(t *testing.T) {
	rec := v39RecordedWorkspaceRun(t)
	stmt := rec.issued[rec.indexOfStatementContaining(t, "DELETE FROM webhook_deliveries")]

	assert.Contains(t, stmt, "WHERE NOT EXISTS (")
	assert.Contains(t, stmt, "SELECT 1 FROM webhook_subscriptions s WHERE s.id = d.subscription_id")

	// One WHERE, one predicate. Anything else in it — a status, an age, an OR —
	// is either a delivery destroyed or an orphan retained.
	assert.Equal(t, 1, strings.Count(stmt, "WHERE NOT EXISTS"))
	assert.Equal(t, 2, strings.Count(stmt, "WHERE"), "the outer WHERE and the correlated one, and no third")
	assert.NotContains(t, strings.ToUpper(stmt), " OR ")
	assert.NotContains(t, stmt, "status")
	assert.NotContains(t, stmt, "created_at")

	// Deletes rows, never the table.
	assert.NotContains(t, strings.ToUpper(rec.all()), "TRUNCATE")
	assert.NotContains(t, strings.ToUpper(rec.all()), "DROP TABLE")
}

// Every body comes from internal/database/schema, so a workspace upgraded from
// v19 and a workspace created from the initializer converge on identical
// behaviour. The payload is a public contract; two installs emitting different
// shapes for the same data is what this reinstall exists to end.
func TestV39Migration_ReinstallsEveryTriggerFunctionFromTheSharedGenerator(t *testing.T) {
	rec := v39RecordedWorkspaceRun(t)

	// The generator's set and the names below have to stay the same set, or the
	// per-name assertions at the end of this test would silently stop covering a
	// function the migration is supposed to converge.
	require.Len(t, schema.WebhookTriggerFunctions(), len(v39TriggerFunctions))

	generated := make(map[string]bool, len(schema.WebhookTriggerFunctions()))
	for _, fn := range schema.WebhookTriggerFunctions() {
		generated[fn] = true
	}

	installed := 0
	for _, stmt := range rec.issued {
		if !strings.Contains(stmt, "CREATE OR REPLACE FUNCTION") {
			continue
		}
		installed++
		assert.Truef(t, generated[stmt],
			"v39 must install the shared generator's text verbatim, not its own copy:\n%s", stmt)
	}
	assert.Equal(t, len(schema.WebhookTriggerFunctions()), installed,
		"every generated function must be installed, and nothing else")

	for _, fn := range v39TriggerFunctions {
		assert.Contains(t, rec.all(), "CREATE OR REPLACE FUNCTION "+fn+"()")
	}
}

// The functions are reinstalled first, while the migration has done no real work
// yet: a second replica running this same migration collides on the first CREATE
// OR REPLACE and aborts on the lock timeout, and the loser of that race throws
// away nothing.
func TestV39Migration_ReinstallsTheTriggersBeforeAnyTableWork(t *testing.T) {
	rec := v39RecordedWorkspaceRun(t)
	assert.Less(t,
		rec.indexOfStatementContaining(t, "CREATE OR REPLACE FUNCTION"),
		rec.indexOfStatementContaining(t, "ADD COLUMN IF NOT EXISTS source"),
	)
}

// Reattaching a trigger means DROP TRIGGER + CREATE TRIGGER, which takes a
// ShareRowExclusiveLock on contacts, contact_lists, contact_segments,
// message_history or custom_events and holds it until the migration commits.
// That blocks every customer write to those tables for the length of the
// migration, and it buys nothing: an attached trigger picks up a replaced
// function body on its next invocation.
func TestV39Migration_NeverReattachesATrigger(t *testing.T) {
	rec := v39RecordedWorkspaceRun(t)

	assert.NotContains(t, rec.all(), "CREATE TRIGGER")
	assert.NotContains(t, rec.all(), "DROP TRIGGER")
	assert.NotContains(t, rec.all(), "ALTER TABLE contacts")
	assert.NotContains(t, rec.all(), "ALTER TABLE contact_lists")
	assert.NotContains(t, rec.all(), "ALTER TABLE contact_segments")
	assert.NotContains(t, rec.all(), "ALTER TABLE message_history")
	assert.NotContains(t, rec.all(), "ALTER TABLE custom_events")
}

func TestV39Migration_WritesOnlyToOrphanedAndStrandedRows(t *testing.T) {
	rec := v39RecordedWorkspaceRun(t)

	// Exactly two statements write to existing rows: the orphan sweep and the
	// reset. Everything else is DDL, so an upgrade cannot change a single
	// subscription, or a delivery whose subscription still exists.
	var writes []string
	for _, stmt := range rec.issued {
		upper := strings.ToUpper(strings.TrimSpace(stmt))
		if strings.HasPrefix(upper, "UPDATE") || strings.HasPrefix(upper, "DELETE") || strings.HasPrefix(upper, "INSERT") {
			writes = append(writes, stmt)
		}
	}
	require.Len(t, writes, 2, "the workspace half should write to exactly two sets of rows:\n%s", rec.all())

	// Rows whose subscription is gone: unroutable, and before this release the
	// worker skipped them without writing, so each one kept matching the pending
	// predicate for the whole retention window.
	assert.Contains(t, writes[0], "DELETE FROM webhook_deliveries d")
	assert.Contains(t, writes[0], "WHERE NOT EXISTS (")

	// 'delivering' is a status no build before this one ever wrote, so every row
	// carrying it was claimed by a worker that is not coming back. Left alone it
	// matches neither the pending predicate nor the reclaim sweep and is stranded
	// for the whole retention window.
	assert.Contains(t, writes[1], "WHERE status = 'delivering'")
	assert.Contains(t, writes[1], "SET status = 'pending'")
	assert.Contains(t, writes[1], "claimed_at = NULL")
}

func TestV39Migration_EveryWorkspaceStatementSurvivesARerun(t *testing.T) {
	// A migration transaction that rolls back — on the lock timeout, on a crash,
	// on any later workspace failing — is retried on the next startup against a
	// database that may already carry some of these objects, so every statement
	// has to be a no-op the second time round. ADD CONSTRAINT is the trap: unlike
	// ADD COLUMN and CREATE INDEX it has no IF NOT EXISTS form, so it needs the
	// catalogue lookup instead.
	rec := v39RecordedWorkspaceRun(t)

	for _, stmt := range rec.issued {
		upper := strings.ToUpper(strings.TrimSpace(stmt))
		switch {
		case strings.HasPrefix(upper, "SET LOCAL"):
			// Scoped to the transaction; setting it again is free.
		case strings.Contains(stmt, "CREATE OR REPLACE FUNCTION"):
			// OR REPLACE is the guard: the second run rewrites the same body.
		case strings.Contains(stmt, "ADD COLUMN"):
			assert.Equal(t, strings.Count(stmt, "ADD COLUMN"), strings.Count(stmt, "ADD COLUMN IF NOT EXISTS"),
				"every added column needs IF NOT EXISTS:\n%s", stmt)
		case strings.HasPrefix(upper, "DELETE"):
			// Self-limiting: the first run leaves no row matching the predicate.
			assert.Contains(t, stmt, "WHERE NOT EXISTS (")
		case strings.HasPrefix(upper, "DO $$"):
			// Neither ADD CONSTRAINT nor VALIDATE CONSTRAINT has an IF NOT EXISTS
			// form, so the catalogue lookup is what makes the second run a no-op.
			assert.Contains(t, stmt, "pg_constraint", "the constraint needs a catalogue guard:\n%s", stmt)
			assert.Contains(t, stmt, "AND NOT convalidated")
		case strings.HasPrefix(upper, "CREATE INDEX"):
			assert.Contains(t, stmt, "IF NOT EXISTS", "the index needs IF NOT EXISTS:\n%s", stmt)
		case strings.HasPrefix(upper, "UPDATE"):
			// Self-limiting: after the first run no row matches the predicate.
			assert.Contains(t, stmt, "WHERE status = 'delivering'")
		default:
			require.FailNowf(t, "unguarded statement", "v39 issued a workspace statement with no re-run guard:\n%s", stmt)
		}
	}
}

// The migration holds no state between calls, so the second run against a fresh
// connection issues exactly what the first did — which is what makes the
// per-statement guards above sufficient rather than merely necessary.
func TestV39Migration_RerunIssuesTheSameStatements(t *testing.T) {
	first := v39RecordedWorkspaceRun(t)
	second := v39RecordedWorkspaceRun(t)
	assert.Equal(t, first.issued, second.issued)
}

// The index is on claimed_at, so it cannot precede the ALTER TABLE that adds
// claimed_at. Both statements are guarded, which means a wrong order would not
// fail on a database that already has the column — only on the upgrades this
// migration exists for.
func TestV39Migration_IndexFollowsTheColumnItIndexes(t *testing.T) {
	rec := v39RecordedWorkspaceRun(t)
	assert.Less(t,
		rec.indexOfStatementContaining(t, "ADD COLUMN IF NOT EXISTS claimed_at"),
		rec.indexOfStatementContaining(t, "idx_webhook_deliveries_claimed"),
	)
}

func TestV39Migration_ColumnDefaultsPreserveExistingBehaviour(t *testing.T) {
	rec := v39RecordedWorkspaceRun(t)
	// Located by the column, not by the table: the trigger function bodies read
	// webhook_subscriptions too, and they are issued first.
	subscriptions := rec.issued[rec.indexOfStatementContaining(t, "ADD COLUMN IF NOT EXISTS source")]
	deliveries := rec.issued[rec.indexOfStatementContaining(t, "ADD COLUMN IF NOT EXISTS claimed_at")]

	// source stays nullable: NULL is the user-created case, which is what every
	// pre-existing subscription is. Defaulting it to anything else would
	// mis-attribute every row the migration touches.
	assert.Contains(t, subscriptions, "source VARCHAR(32),")
	assert.NotContains(t, subscriptions, "source VARCHAR(32) NOT NULL")
	assert.NotContains(t, subscriptions, "source VARCHAR(32) DEFAULT")

	// consecutive_failures is NOT NULL DEFAULT 0 so the threshold comparison
	// never has to coalesce. PostgreSQL stores a non-volatile default in the
	// catalogue rather than rewriting the table, so this stays instant.
	assert.Contains(t, subscriptions, "consecutive_failures INT NOT NULL DEFAULT 0")

	// failing_since is nullable with no default, and that is load-bearing: NULL
	// means "not currently failing", so an upgraded workspace starts its first
	// run of failures from the first failure after the upgrade instead of
	// inheriting a window that already looks hours old and retiring a healthy
	// subscription on its next hiccup.
	assert.Contains(t, subscriptions, "failing_since TIMESTAMPTZ")
	assert.NotContains(t, subscriptions, "failing_since TIMESTAMPTZ NOT NULL")
	assert.NotContains(t, subscriptions, "failing_since TIMESTAMPTZ DEFAULT")

	// disabled_reason and claimed_at are nullable with no default: NULL means
	// "never auto-disabled" and "not claimed", which is true of every existing
	// row and is exactly what the code reads them as.
	assert.Contains(t, subscriptions, "disabled_reason TEXT")
	assert.NotContains(t, subscriptions, "disabled_reason TEXT NOT NULL")
	assert.Contains(t, deliveries, "claimed_at TIMESTAMPTZ")
	assert.NotContains(t, deliveries, "claimed_at TIMESTAMPTZ NOT NULL")
	assert.NotContains(t, deliveries, "claimed_at TIMESTAMPTZ DEFAULT")
}

// ---------------------------------------------------------------------------
// The two halves together.
// ---------------------------------------------------------------------------

// The halves run against different databases and share nothing but this file.
// user_workspaces and workspace_invitations exist only in the system database;
// webhook_deliveries and the trigger functions exist only in a workspace
// database. So a statement issued from the wrong half does not quietly do
// nothing — it raises "relation does not exist", and because the manager runs
// each database in one transaction, it rolls back a migration that was otherwise
// complete and leaves the version stamp unwritten.
//
// The markers are chosen to survive the fact that the permission resources are
// named after the workspace tables they gate: the grant literal contains the
// string "webhook_subscriptions", which is why the workspace-side markers below
// key off webhook_deliveries and DDL verbs instead.
func TestV39Migration_EachHalfStaysInItsOwnDatabase(t *testing.T) {
	system := v39RecordedSystemRun(t)
	workspace := v39RecordedWorkspaceRun(t)

	for _, workspaceOnly := range []string{
		"webhook_deliveries",
		"ALTER TABLE",
		"CREATE INDEX",
		"CREATE OR REPLACE FUNCTION",
		"lock_timeout",
		"pg_constraint",
	} {
		assert.NotContains(t, system.all(), workspaceOnly,
			"the system half reached for a workspace object:\n%s", system.all())
	}

	for _, systemOnly := range []string{
		"user_workspaces",
		"workspace_invitations",
		"permissions",
	} {
		assert.NotContains(t, workspace.all(), systemOnly,
			"the workspace half reached for a system object:\n%s", workspace.all())
	}
}

// VERSION gates which migrations the dispatcher will run at all: a migration
// numbered above the code version is registered and never dispatched, so the
// permission backfill, the columns, the orphan sweep and the trigger convergence
// would sit in the binary doing nothing while the console reported a successful
// upgrade.
//
// Both sides are read from their source of truth — the registry the dispatcher
// itself walks, and config.VERSION — rather than from a literal repeated here.
// A guard that names the version it protects stops seeing the mismatch the
// moment the next vN.go lands without a VERSION bump, which is the only moment
// it exists for.
func TestRegisteredMigrations_AreReachableFromTheCodeVersion(t *testing.T) {
	codeVersion, err := GetCurrentCodeVersion()
	require.NoError(t, err)

	registered := GetRegisteredMigrations()
	require.NotEmpty(t, registered, "no migrations registered: the vN.go init() registrations did not run")

	// GetRegisteredMigrations sorts ascending, so the last entry is the highest.
	// Compared numerically, not as strings: "5" sorts after "40".
	highest := registered[len(registered)-1].GetMajorVersion()
	assert.LessOrEqual(t, highest, codeVersion,
		"config.VERSION is %q but the highest registered migration is v%.0f: the dispatcher only runs migrations above the DB version and up to the code version, so that migration never runs",
		config.VERSION, highest)
}
