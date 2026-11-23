package schema

// The engagement clock of the data retention plan: when each contact last did something
// themselves (a click, a reply, a double opt-in confirmation, their own subscription, an
// identified visit). A contact with no row has done nothing since they were collected.
// Something the workspace or Notifuse does to the contact never writes a row.
//
// data_retention_state holds one row. Its backfill_completed_at says the engagement of the
// contacts collected before this table existed has been read back from history: the
// inactive-contact category cannot be switched on until it is set. A missing or NULL
// marker reads as not ready, never as ready.

// ContactEngagementStatements returns the engagement table and the state table, shared by
// a new workspace and the v45 migration. Every statement is idempotent.
func ContactEngagementStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS contact_engagement (
			email VARCHAR(255) PRIMARY KEY,
			last_engaged_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_engagement_last_engaged_at ON contact_engagement(last_engaged_at)`,
		`CREATE TABLE IF NOT EXISTS data_retention_state (
			id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
			backfill_completed_at TIMESTAMPTZ
		)`,
	}
}

// NewWorkspaceDataRetentionMarker marks a workspace created from scratch as ready: it has no
// history to read back. A repaired workspace, built by the same initializer, is in the same
// case.
const NewWorkspaceDataRetentionMarker = `INSERT INTO data_retention_state (id, backfill_completed_at)
	VALUES (1, NOW()) ON CONFLICT (id) DO NOTHING`

// MigratedWorkspaceDataRetentionMarker creates the marker of a workspace upgraded to 45 as
// not ready: its backfill task sets it. A marker already there is kept, so running the
// migration again never resets a finished backfill.
const MigratedWorkspaceDataRetentionMarker = `INSERT INTO data_retention_state (id, backfill_completed_at)
	VALUES (1, NULL) ON CONFLICT (id) DO NOTHING`
