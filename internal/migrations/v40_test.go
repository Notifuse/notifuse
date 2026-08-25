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
	"github.com/Notifuse/notifuse/internal/domain"
)

// The statements v40 issues, in the order it issues them. They are pinned by
// regexp rather than by behaviour because the whole point of this migration is
// which SQL it does and does not emit: NOT VALID instead of a validating scan,
// guards instead of bare DDL, and nothing that touches a row it did not have to.
const (
	v40SubscriptionColumns = `(?s)ALTER TABLE\s+webhook_subscriptions\s+` +
		`ADD COLUMN IF NOT EXISTS source VARCHAR\(32\),\s+` +
		`ADD COLUMN IF NOT EXISTS consecutive_failures INT NOT NULL DEFAULT 0,\s+` +
		`ADD COLUMN IF NOT EXISTS disabled_reason TEXT`

	v40DeliveryColumn = `(?s)ALTER TABLE\s+webhook_deliveries\s+` +
		`ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ`

	v40ForeignKey = `(?s)IF NOT EXISTS \(\s*SELECT 1 FROM pg_constraint\s+` +
		`WHERE conname = 'webhook_deliveries_subscription_id_fkey'\s+` +
		`AND conrelid = to_regclass\('webhook_deliveries'\).*` +
		`ADD CONSTRAINT webhook_deliveries_subscription_id_fkey\s+` +
		`FOREIGN KEY \(subscription_id\) REFERENCES webhook_subscriptions\(id\)\s+` +
		`ON DELETE CASCADE NOT VALID`

	v40ResetDelivering = `(?s)UPDATE webhook_deliveries\s+` +
		`SET status = 'pending', claimed_at = NULL\s+` +
		`WHERE status = 'delivering'`

	v40ClaimedIndex = `(?s)CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_claimed\s+` +
		`ON webhook_deliveries\(claimed_at\) WHERE status = 'delivering'`
)

// v40StatementRecorder is the default regexp matcher plus a log of everything the
// migration issued. Expectations alone can only assert that the statements v40
// should run did run; the split between this migration and the next one lives
// entirely in the statements it must NOT run, and those are invisible to
// ExpectExec.
type v40StatementRecorder struct {
	issued []string
}

func (r *v40StatementRecorder) Match(expectedSQL, actualSQL string) error {
	// One statement can be offered to more than one expectation; record it once.
	if len(r.issued) == 0 || r.issued[len(r.issued)-1] != actualSQL {
		r.issued = append(r.issued, actualSQL)
	}
	return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
}

func (r *v40StatementRecorder) all() string {
	return strings.Join(r.issued, "\n")
}

func (r *v40StatementRecorder) indexOfStatementContaining(t *testing.T, needle string) int {
	t.Helper()
	for i, stmt := range r.issued {
		if strings.Contains(stmt, needle) {
			return i
		}
	}
	require.FailNowf(t, "statement not issued", "no statement containing %q in:\n%s", needle, r.all())
	return -1
}

func expectV40Workspace(mock sqlmock.Sqlmock) {
	mock.ExpectExec(v40SubscriptionColumns).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(v40DeliveryColumn).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(v40ForeignKey).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(v40ResetDelivering).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(v40ClaimedIndex).WillReturnResult(sqlmock.NewResult(0, 0))
}

func v40RecordedRun(t *testing.T) *v40StatementRecorder {
	t.Helper()

	rec := &v40StatementRecorder{}
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(rec))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	expectV40Workspace(mock)

	m := &V40Migration{}
	require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
	return rec
}

func TestV40Migration_Metadata(t *testing.T) {
	m := &V40Migration{}
	assert.Equal(t, 40.0, m.GetMajorVersion())
	// Both new columns live in workspace databases. A migration that also
	// claimed a system update would connect to and lock the system database for
	// nothing on every install.
	assert.False(t, m.HasSystemUpdate())
	assert.True(t, m.HasWorkspaceUpdate())
	assert.False(t, m.ShouldRestartServer())
}

func TestV40Migration_IsRegistered(t *testing.T) {
	migration, ok := GetRegisteredMigration(40.0)
	require.True(t, ok, "v40 must be registered so it runs on startup")
	assert.IsType(t, &V40Migration{}, migration)
}

func TestV40Migration_UpdateSystem_TouchesNothing(t *testing.T) {
	rec := &v40StatementRecorder{}
	db, _, err := sqlmock.New(sqlmock.QueryMatcherOption(rec))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	m := &V40Migration{}
	require.NoError(t, m.UpdateSystem(context.Background(), &config.Config{}, db))
	assert.Empty(t, rec.issued, "v40 declares no system update, so it must issue no system statements")
}

func TestV40Migration_UpdateWorkspace(t *testing.T) {
	m := &V40Migration{}
	ctx := context.Background()
	workspace := &domain.Workspace{ID: "ws1"}

	t.Run("adds the columns, the foreign key, the reset and the index", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		expectV40Workspace(mock)

		require.NoError(t, m.UpdateWorkspace(ctx, &config.Config{}, workspace, db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	failures := []struct {
		name     string
		upTo     func(sqlmock.Sqlmock)
		failing  string
		contains string
	}{
		{
			name:     "subscription columns",
			upTo:     func(sqlmock.Sqlmock) {},
			failing:  v40SubscriptionColumns,
			contains: "add lifecycle columns to webhook_subscriptions",
		},
		{
			name: "delivery column",
			upTo: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(v40SubscriptionColumns).WillReturnResult(sqlmock.NewResult(0, 0))
			},
			failing:  v40DeliveryColumn,
			contains: "add claimed_at to webhook_deliveries",
		},
		{
			name: "foreign key",
			upTo: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(v40SubscriptionColumns).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(v40DeliveryColumn).WillReturnResult(sqlmock.NewResult(0, 0))
			},
			failing:  v40ForeignKey,
			contains: "add webhook_deliveries subscription foreign key",
		},
		{
			name: "delivering reset",
			upTo: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(v40SubscriptionColumns).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(v40DeliveryColumn).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(v40ForeignKey).WillReturnResult(sqlmock.NewResult(0, 0))
			},
			failing:  v40ResetDelivering,
			contains: "reset stranded delivering rows",
		},
		{
			name: "claimed index",
			upTo: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(v40SubscriptionColumns).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(v40DeliveryColumn).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(v40ForeignKey).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(v40ResetDelivering).WillReturnResult(sqlmock.NewResult(0, 0))
			},
			failing:  v40ClaimedIndex,
			contains: "create webhook_deliveries claimed index",
		},
	}

	// A migration runs in one transaction per database, so any failure rolls the
	// whole thing back — but only if the error is returned rather than swallowed,
	// and only if it names the statement that failed. An unattributed error in a
	// startup migration means reading the whole file to find out what broke.
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

func TestV40Migration_ForeignKeyIsAddedWithoutAValidatingScan(t *testing.T) {
	rec := v40RecordedRun(t)
	stmt := rec.issued[rec.indexOfStatementContaining(t, "ADD CONSTRAINT")]

	// NOT VALID is what makes this the instant half of the split. It returns
	// immediately, skips the scan, and still rejects every new orphan — so the
	// window before the validating migration runs cannot grow new ones. A
	// validating ADD CONSTRAINT here would take a ShareRowExclusiveLock across a
	// full scan of webhook_deliveries, and because deliveries are enqueued by
	// AFTER-row triggers inside customer write transactions, that blocks every
	// write to contacts, contact_lists, contact_segments and message_history.
	assert.Contains(t, stmt, "NOT VALID")
	assert.Contains(t, stmt, "ON DELETE CASCADE")

	// The expensive half belongs in the next migration. If either of these ever
	// appears here, a workspace with a large delivery table gets a write outage
	// inside a startup migration whose timeout crash-loops the pod.
	assert.NotContains(t, rec.all(), "VALIDATE CONSTRAINT")
	assert.NotContains(t, strings.ToUpper(rec.all()), "DELETE FROM")
}

func TestV40Migration_LeavesExistingRowsAloneExceptTheStrandedClaims(t *testing.T) {
	rec := v40RecordedRun(t)

	// Exactly one statement writes to existing rows, and it is the reset. Every
	// other statement is DDL, so an upgrade cannot change a single subscription
	// or delivery the customer already had.
	var writes []string
	for _, stmt := range rec.issued {
		upper := strings.ToUpper(strings.TrimSpace(stmt))
		if strings.HasPrefix(upper, "UPDATE") || strings.HasPrefix(upper, "DELETE") || strings.HasPrefix(upper, "INSERT") {
			writes = append(writes, stmt)
		}
	}
	require.Len(t, writes, 1, "v40 should write to exactly one set of rows:\n%s", rec.all())

	// 'delivering' is a status no build before this one ever wrote, so every row
	// carrying it was claimed by a worker that is not coming back. Left alone it
	// matches neither the pending predicate nor the reclaim sweep and is stranded
	// for the whole retention window.
	assert.Contains(t, writes[0], "WHERE status = 'delivering'")
	assert.Contains(t, writes[0], "SET status = 'pending'")
	assert.Contains(t, writes[0], "claimed_at = NULL")
}

func TestV40Migration_EveryStatementSurvivesARerun(t *testing.T) {
	// A migration transaction that rolls back is retried on the next startup
	// against a database that may already carry some of these objects, so every
	// statement has to be guarded. ADD CONSTRAINT is the trap: unlike ADD COLUMN
	// and CREATE INDEX it has no IF NOT EXISTS form, so it needs the catalogue
	// lookup instead.
	rec := v40RecordedRun(t)

	for _, stmt := range rec.issued {
		trimmed := strings.TrimSpace(stmt)
		switch {
		case strings.Contains(stmt, "ADD COLUMN"):
			assert.Equal(t, strings.Count(stmt, "ADD COLUMN"), strings.Count(stmt, "ADD COLUMN IF NOT EXISTS"),
				"every added column needs IF NOT EXISTS:\n%s", stmt)
		case strings.Contains(stmt, "ADD CONSTRAINT"):
			assert.Contains(t, stmt, "pg_constraint", "the constraint needs a catalogue guard:\n%s", stmt)
		case strings.HasPrefix(strings.ToUpper(trimmed), "CREATE INDEX"):
			assert.Contains(t, stmt, "IF NOT EXISTS", "the index needs IF NOT EXISTS:\n%s", stmt)
		case strings.HasPrefix(strings.ToUpper(trimmed), "UPDATE"):
			// Self-limiting: after the first run no row matches the predicate.
			assert.Contains(t, stmt, "WHERE status = 'delivering'")
		default:
			require.FailNowf(t, "unguarded statement", "v40 issued a statement with no re-run guard:\n%s", stmt)
		}
	}
}

func TestV40Migration_IndexFollowsTheColumnItIndexes(t *testing.T) {
	// The index is on claimed_at, so it cannot precede the ALTER TABLE that adds
	// claimed_at. Both statements are guarded, which means a wrong order would
	// not fail on a database that already has the column — only on the upgrades
	// this migration exists for.
	rec := v40RecordedRun(t)
	assert.Less(t,
		rec.indexOfStatementContaining(t, "ADD COLUMN IF NOT EXISTS claimed_at"),
		rec.indexOfStatementContaining(t, "idx_webhook_deliveries_claimed"),
	)
}

func TestV40Migration_ColumnDefaultsPreserveExistingBehaviour(t *testing.T) {
	rec := v40RecordedRun(t)
	subscriptions := rec.issued[rec.indexOfStatementContaining(t, "webhook_subscriptions")]
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

	// disabled_reason and claimed_at are nullable with no default: NULL means
	// "never auto-disabled" and "not claimed", which is true of every existing
	// row and is exactly what the code reads them as.
	assert.Contains(t, subscriptions, "disabled_reason TEXT")
	assert.NotContains(t, subscriptions, "disabled_reason TEXT NOT NULL")
	assert.Contains(t, deliveries, "claimed_at TIMESTAMPTZ")
	assert.NotContains(t, deliveries, "claimed_at TIMESTAMPTZ NOT NULL")
	assert.NotContains(t, deliveries, "claimed_at TIMESTAMPTZ DEFAULT")
}
