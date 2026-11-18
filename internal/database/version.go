package database

import (
	"database/sql"
	"fmt"
)

// MinServerVersionNum is the oldest PostgreSQL Notifuse runs on, as server_version_num: 17.0,
// the version the tests run on and the floor AlloyDB Omni sets (tests/compose.test.yaml).
//
// The conditions compiled into automation triggers and segment queries test a conversion with
// pg_input_is_valid before making it, so that a value such as 2026-02-30 gives no match instead
// of raising, which inside a trigger would abort the contact, list or message write that fired it.
// The function exists since PostgreSQL 16; on an older server those conditions fail one by one (an
// automation refuses to activate, blaming its conditions). Rather than run half-working, or on a
// version nothing tests, the server is refused at startup, with the reason.
const MinServerVersionNum = 170000

// RequireServerVersion returns an error naming the required and the running version when the
// server behind db is older than minVersionNum.
func RequireServerVersion(db *sql.DB, minVersionNum int) error {
	var num int
	var version string
	if err := db.QueryRow(`SELECT current_setting('server_version_num')::int, current_setting('server_version')`).Scan(&num, &version); err != nil {
		return fmt.Errorf("failed to read the PostgreSQL version: %w", err)
	}
	if num < minVersionNum {
		return fmt.Errorf("PostgreSQL %d or later is required; this server runs %s", minVersionNum/10000, version)
	}
	return nil
}
