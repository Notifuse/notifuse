package migrations

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

// The statements v41 issues, in order. Like v40's, they are pinned by regexp
// rather than by behaviour, because what makes this migration safe is precisely
// which SQL it does and does not emit: VALIDATE rather than a second ADD
// CONSTRAINT, a catalogue guard rather than bare DDL, and no trigger
// reattachment anywhere.
const (
	v41LockTimeout = `SET LOCAL lock_timeout = '5s'`

	v41OrphanSweep = `(?s)DELETE FROM webhook_deliveries d\s+` +
		`WHERE NOT EXISTS \(\s+` +
		`SELECT 1 FROM webhook_subscriptions s WHERE s\.id = d\.subscription_id\s+\)`

	v41ValidateConstraint = `(?s)IF EXISTS \(\s*SELECT 1 FROM pg_constraint\s+` +
		`WHERE conname = 'webhook_deliveries_subscription_id_fkey'\s+` +
		`AND conrelid = to_regclass\('webhook_deliveries'\)\s+` +
		`AND NOT convalidated.*` +
		`ALTER TABLE webhook_deliveries\s+` +
		`VALIDATE CONSTRAINT webhook_deliveries_subscription_id_fkey`
)

// The functions v41 converges. These are the names the workspace initializer
// attaches its triggers to, so a generator that stopped emitting one of them
// would leave that trigger on whichever body the workspace happened to inherit.
var v41TriggerFunctions = []string{
	"webhook_contacts_trigger",
	"webhook_contact_lists_trigger",
	"webhook_contact_segments_trigger",
	"webhook_message_history_trigger",
	"webhook_custom_events_trigger",
}

// v41StatementRecorder is the default regexp matcher plus a log of everything
// the migration issued. Expectations can only assert that the statements v41
// should run did run; half of what makes this migration safe lives in the
// statements it must NOT run, and those are invisible to ExpectExec.
type v41StatementRecorder struct {
	issued []string
}

func (r *v41StatementRecorder) Match(expectedSQL, actualSQL string) error {
	// One statement can be offered to more than one expectation; record it once.
	if len(r.issued) == 0 || r.issued[len(r.issued)-1] != actualSQL {
		r.issued = append(r.issued, actualSQL)
	}
	return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
}

func (r *v41StatementRecorder) all() string {
	return strings.Join(r.issued, "\n")
}

func (r *v41StatementRecorder) indexOfStatementContaining(t *testing.T, needle string) int {
	t.Helper()
	for i, stmt := range r.issued {
		if strings.Contains(stmt, needle) {
			return i
		}
	}
	require.FailNowf(t, "statement not issued", "no statement containing %q in:\n%s", needle, r.all())
	return -1
}

func expectV41Workspace(mock sqlmock.Sqlmock) {
	mock.ExpectExec(v41LockTimeout).WillReturnResult(sqlmock.NewResult(0, 0))
	for _, fn := range v41TriggerFunctions {
		mock.ExpectExec(`CREATE OR REPLACE FUNCTION ` + fn + `\(\)`).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec(v41OrphanSweep).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(v41ValidateConstraint).WillReturnResult(sqlmock.NewResult(0, 0))
}

func v41RecordedRun(t *testing.T) *v41StatementRecorder {
	t.Helper()

	rec := &v41StatementRecorder{}
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(rec))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	expectV41Workspace(mock)

	m := &V41Migration{}
	require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
	return rec
}

func TestV41Migration_Metadata(t *testing.T) {
	m := &V41Migration{}
	assert.Equal(t, 41.0, m.GetMajorVersion())
	// Every object this migration touches — webhook_deliveries,
	// webhook_subscriptions and the five trigger functions — lives in workspace
	// databases. Claiming a system update would connect to and lock the system
	// database for nothing on every install.
	assert.False(t, m.HasSystemUpdate())
	assert.True(t, m.HasWorkspaceUpdate())
	assert.False(t, m.ShouldRestartServer())
}

func TestV41Migration_IsRegistered(t *testing.T) {
	migration, ok := GetRegisteredMigration(41.0)
	require.True(t, ok, "v41 must be registered so it runs on startup")
	assert.IsType(t, &V41Migration{}, migration)
}

// v41 finishes what v40 started, so it must sort after it. The dispatcher runs
// migrations whose version is greater than the database's, in order, and the
// sweep has to happen against a table the NOT VALID constraint is already
// guarding or new orphans can appear between the two.
func TestV41Migration_FollowsV40(t *testing.T) {
	v40, ok := GetRegisteredMigration(40.0)
	require.True(t, ok)
	assert.Greater(t, (&V41Migration{}).GetMajorVersion(), v40.GetMajorVersion())
}

func TestV41Migration_UpdateSystem_TouchesNothing(t *testing.T) {
	rec := &v41StatementRecorder{}
	db, _, err := sqlmock.New(sqlmock.QueryMatcherOption(rec))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	m := &V41Migration{}
	require.NoError(t, m.UpdateSystem(context.Background(), &config.Config{}, db))
	assert.Empty(t, rec.issued, "v41 declares no system update, so it must issue no system statements")
}

func TestV41Migration_UpdateWorkspace(t *testing.T) {
	m := &V41Migration{}
	ctx := context.Background()
	workspace := &domain.Workspace{ID: "ws1"}

	t.Run("sets the lock timeout, reinstalls the triggers, sweeps and validates", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		expectV41Workspace(mock)

		require.NoError(t, m.UpdateWorkspace(ctx, &config.Config{}, workspace, db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	upToLockTimeout := func(mock sqlmock.Sqlmock) {
		mock.ExpectExec(v41LockTimeout).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	upToTriggers := func(mock sqlmock.Sqlmock) {
		upToLockTimeout(mock)
		for _, fn := range v41TriggerFunctions {
			mock.ExpectExec(`CREATE OR REPLACE FUNCTION ` + fn + `\(\)`).WillReturnResult(sqlmock.NewResult(0, 0))
		}
	}

	failures := []struct {
		name     string
		upTo     func(sqlmock.Sqlmock)
		failing  string
		contains string
	}{
		{
			name:     "lock timeout",
			upTo:     func(sqlmock.Sqlmock) {},
			failing:  v41LockTimeout,
			contains: "set lock timeout",
		},
		{
			name:     "trigger reinstall",
			upTo:     upToLockTimeout,
			failing:  `CREATE OR REPLACE FUNCTION webhook_contacts_trigger\(\)`,
			contains: "reinstall webhook trigger functions",
		},
		{
			name:     "orphan sweep",
			upTo:     upToTriggers,
			failing:  v41OrphanSweep,
			contains: "sweep orphaned webhook deliveries",
		},
		{
			name: "constraint validation",
			upTo: func(mock sqlmock.Sqlmock) {
				upToTriggers(mock)
				mock.ExpectExec(v41OrphanSweep).WillReturnResult(sqlmock.NewResult(0, 0))
			},
			failing:  v41ValidateConstraint,
			contains: "validate webhook_deliveries subscription foreign key",
		},
	}

	// A migration runs in one transaction per database, so any failure rolls the
	// whole thing back to v40's state — but only if the error is returned rather
	// than swallowed, and only if it names the statement that failed. The lock
	// timeout makes failure a designed outcome here rather than a surprise, so an
	// unattributed error would be read on a real startup, under pressure.
	for _, tc := range failures {
		t.Run("reports a failed "+tc.name+" statement", func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			tc.upTo(mock)
			mock.ExpectExec(tc.failing).WillReturnError(errors.New("boom"))

			err = m.UpdateWorkspace(ctx, &config.Config{}, workspace, db)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.contains)
			assert.Contains(t, err.Error(), "boom", "the driver error must survive wrapping")
		})
	}
}

// The lock timeout is what lets this migration yield instead of queueing every
// customer write behind it, so it has to be in force before the first statement
// that takes a lock. SET LOCAL rather than SET, so it dies with the transaction
// instead of riding a pooled connection into request traffic.
func TestV41Migration_BoundsItsLockWaitBeforeDoingAnything(t *testing.T) {
	rec := v41RecordedRun(t)

	require.NotEmpty(t, rec.issued)
	assert.Equal(t, 0, rec.indexOfStatementContaining(t, "lock_timeout"),
		"the lock timeout must be set before any statement that takes a lock")
	assert.Contains(t, rec.issued[0], "SET LOCAL",
		"a session-level SET would outlive the migration on a pooled connection")
}

// The whole reason v40 and v41 are two migrations. VALIDATE CONSTRAINT takes a
// ShareUpdateExclusiveLock and permits concurrent INSERT, UPDATE and DELETE; a
// validating ADD CONSTRAINT takes a ShareRowExclusiveLock across a full scan.
// Because deliveries are enqueued by AFTER-row triggers inside customer write
// transactions, blocking INSERT on webhook_deliveries blocks every write to
// contacts, contact_lists, contact_segments and message_history.
func TestV41Migration_ValidatesRatherThanReaddingTheConstraint(t *testing.T) {
	rec := v41RecordedRun(t)
	stmt := rec.issued[rec.indexOfStatementContaining(t, "VALIDATE CONSTRAINT")]

	assert.Contains(t, stmt, "VALIDATE CONSTRAINT webhook_deliveries_subscription_id_fkey",
		"the name must match the one v40 and the workspace initializer both declare")

	// convalidated is the guard, not mere existence: a workspace created after
	// v40 shipped declares the constraint inline and already validated, and
	// re-validating it would re-scan the table for a result the catalogue
	// already holds. The same guard makes a re-run after a rollback a no-op.
	assert.Contains(t, stmt, "AND NOT convalidated")
	assert.Contains(t, stmt, "pg_constraint")

	assert.NotContains(t, rec.all(), "ADD CONSTRAINT",
		"adding the constraint is v40's job; a validating ADD here is a write outage")
	assert.NotContains(t, rec.all(), "DROP CONSTRAINT")
}

// Sweeping has to precede validating. VALIDATE CONSTRAINT raises on the first
// row that violates it, so validating first would abort the migration on every
// workspace that has an orphan — which is exactly the population this migration
// exists for.
func TestV41Migration_SweepsBeforeItValidates(t *testing.T) {
	rec := v41RecordedRun(t)
	assert.Less(t,
		rec.indexOfStatementContaining(t, "DELETE FROM webhook_deliveries"),
		rec.indexOfStatementContaining(t, "VALIDATE CONSTRAINT"),
	)
}

// The sweep deletes deliveries whose subscription is gone and nothing else. A
// stray extra predicate would either leave poison pills behind or, far worse,
// delete deliveries that are still routable — queued events a subscriber is
// waiting for, gone with no trace.
func TestV41Migration_SweepsOnlyGenuineOrphans(t *testing.T) {
	rec := v41RecordedRun(t)
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

// All five bodies come from internal/database/schema, so a workspace upgraded
// from v19 and a workspace created from the initializer converge on identical
// behaviour. The payload is a public contract; two installs emitting different
// shapes for the same data is what this reinstall exists to end.
func TestV41Migration_ReinstallsEveryTriggerFunctionFromTheSharedGenerator(t *testing.T) {
	rec := v41RecordedRun(t)

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
			"v41 must install the shared generator's text verbatim, not its own copy:\n%s", stmt)
	}
	assert.Equal(t, len(schema.WebhookTriggerFunctions()), installed,
		"every generated function must be installed, and nothing else")

	for _, fn := range v41TriggerFunctions {
		assert.Contains(t, rec.all(), "CREATE OR REPLACE FUNCTION "+fn+"()")
	}
}

// Reattaching a trigger means DROP TRIGGER + CREATE TRIGGER, which takes a
// ShareRowExclusiveLock on contacts, contact_lists, contact_segments,
// message_history or custom_events and holds it until the migration commits.
// That blocks every customer write to those tables for the length of the
// migration, and it buys nothing: an attached trigger picks up a replaced
// function body on its next invocation.
func TestV41Migration_NeverReattachesATrigger(t *testing.T) {
	rec := v41RecordedRun(t)

	assert.NotContains(t, rec.all(), "CREATE TRIGGER")
	assert.NotContains(t, rec.all(), "DROP TRIGGER")
	assert.NotContains(t, rec.all(), "ALTER TABLE contacts")
	assert.NotContains(t, rec.all(), "ALTER TABLE contact_lists")
	assert.NotContains(t, rec.all(), "ALTER TABLE contact_segments")
	assert.NotContains(t, rec.all(), "ALTER TABLE message_history")
	assert.NotContains(t, rec.all(), "ALTER TABLE custom_events")
}

// A migration transaction that rolls back — on the lock timeout, on a crash, on
// any later workspace failing — is retried on the next startup against a
// database that may already carry some of this. Every statement therefore has to
// be a no-op the second time round.
func TestV41Migration_EveryStatementSurvivesARerun(t *testing.T) {
	rec := v41RecordedRun(t)

	for _, stmt := range rec.issued {
		upper := strings.ToUpper(strings.TrimSpace(stmt))
		switch {
		case strings.HasPrefix(upper, "SET LOCAL"):
			// Scoped to the transaction; setting it again is free.
		case strings.Contains(stmt, "CREATE OR REPLACE FUNCTION"):
			// OR REPLACE is the guard: the second run rewrites the same body.
		case strings.HasPrefix(upper, "DELETE"):
			// Self-limiting: the first run leaves no row matching the predicate.
			assert.Contains(t, stmt, "WHERE NOT EXISTS (")
		case strings.HasPrefix(upper, "DO $$"):
			// ALTER TABLE ... VALIDATE CONSTRAINT has no IF NOT EXISTS form, so
			// the catalogue lookup is what makes the second run a no-op.
			assert.Contains(t, stmt, "AND NOT convalidated")
		default:
			require.FailNowf(t, "unguarded statement", "v41 issued a statement with no re-run guard:\n%s", stmt)
		}
	}
}

// The migration holds no state between calls, so the second run against a fresh
// connection issues exactly what the first did — which is what makes the
// per-statement guards above sufficient rather than merely necessary.
func TestV41Migration_RerunIssuesTheSameStatements(t *testing.T) {
	first := v41RecordedRun(t)
	second := v41RecordedRun(t)
	assert.Equal(t, first.issued, second.issued)
}

// VERSION gates which migrations the dispatcher will run at all: a migration
// numbered above the code version is registered and never dispatched, so the
// orphan sweep and the trigger convergence would sit in the binary doing
// nothing while the console reported a successful upgrade.
func TestV41Migration_IsReachableFromTheCodeVersion(t *testing.T) {
	matched := regexp.MustCompile(`^(\d+)\.`).FindStringSubmatch(config.VERSION)
	require.Len(t, matched, 2, "VERSION must start with a major number: %q", config.VERSION)

	// Parsed rather than compared as a string: "5" sorts after "41".
	major, err := strconv.Atoi(matched[1])
	require.NoError(t, err)
	assert.GreaterOrEqual(t, major, 41, "config.VERSION must be at least the highest migration version")
}
