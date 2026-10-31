package migrations

import (
	"database/sql"
	"regexp"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/Notifuse/notifuse/internal/database"
)

// expectMigrationLockGranted expects RunMigrations to read the version, finding none
// (so nothing is compared with the version read under the lock), then take the
// migration lock at once, in a lock transaction of its own: its BEGIN and settings, then
// the try that wins. The transaction's COMMIT, or ROLLBACK on an error, comes after the
// run's own statements.
func expectMigrationLockGranted(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").WillReturnError(sql.ErrNoRows)
	mock.ExpectBegin()
	for _, statement := range lockSettingsForTest() {
		mock.ExpectExec(statement).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_try_advisory_xact_lock($1)`)).WithArgs(database.MigrationLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"granted"}).AddRow(true))
}

func lockSettingsForTest() []string {
	settings := database.LockTransactionSettings()
	for i, s := range settings {
		settings[i] = regexp.QuoteMeta(s)
	}
	return settings
}
