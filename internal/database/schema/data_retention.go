package schema

// The data retention engine (Lot 1): the report of every run, the work table of the batch
// erasure, and the message history cursor. Shared by a new workspace and the v45 migration.

// RetentionPurgeGUC is the transaction-local setting every purge statement turns on. While it
// is on, deleting a contact's segment memberships writes no segment.left timeline row or
// webhook (the contact is being erased, not leaving a segment), and deleting the contact
// sends a contact.deleted webhook carrying the address only (D8): subscribers get the signal
// to propagate the deletion, not a copy of the data being erased.
const RetentionPurgeGUC = "notifuse.retention_purge"

// RetentionPurgeOn is the condition a trigger tests.
const RetentionPurgeOn = `current_setting('` + RetentionPurgeGUC + `', true) = 'on'`

// DataRetentionStatements returns the engine's tables, idempotent.
func DataRetentionStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS data_retention_runs (
			id UUID PRIMARY KEY,
			started_at TIMESTAMPTZ NOT NULL,
			finished_at TIMESTAMPTZ,
			status VARCHAR(20) NOT NULL,
			policy JSONB NOT NULL DEFAULT '{}'::jsonb,
			results JSONB NOT NULL DEFAULT '{}'::jsonb,
			error TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_data_retention_runs_started_at ON data_retention_runs(started_at DESC)`,
		// The contacts an inactivity purge has deleted and whose history is still being
		// erased. A crash mid-batch leaves the rest here, to finish from: no join on contacts
		// could find that history again.
		`CREATE TABLE IF NOT EXISTS data_retention_erasure_queue (
			email VARCHAR(255) PRIMARY KEY,
			run_id UUID NOT NULL,
			enqueued_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		// Where the message history redaction stopped, so the next run does not walk the
		// rows already redacted again.
		`ALTER TABLE data_retention_state ADD COLUMN IF NOT EXISTS message_history_cursor_at TIMESTAMPTZ`,
		`ALTER TABLE data_retention_state ADD COLUMN IF NOT EXISTS message_history_cursor_id VARCHAR(255)`,
	}
}

// TrackContactSegmentChangesFunction returns the timeline function of contact_segments:
// segment.joined and segment.left. A membership deleted by a retention purge writes nothing.
func TrackContactSegmentChangesFunction() string {
	return `CREATE OR REPLACE FUNCTION track_contact_segment_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
			op VARCHAR(20);
			kind_value VARCHAR(50);
		BEGIN
			IF TG_OP = 'INSERT' THEN
				op := 'insert';
				kind_value := 'segment.joined';
				changes_json := jsonb_build_object('segment_id', jsonb_build_object('new', NEW.segment_id), 'version', jsonb_build_object('new', NEW.version), 'matched_at', jsonb_build_object('new', NEW.matched_at));
			ELSIF TG_OP = 'DELETE' THEN
				-- A contact erased by a retention purge does not leave a segment.
				IF ` + RetentionPurgeOn + ` THEN
					RETURN OLD;
				END IF;
				op := 'delete';
				kind_value := 'segment.left';
				changes_json := jsonb_build_object('segment_id', jsonb_build_object('old', OLD.segment_id), 'version', jsonb_build_object('old', OLD.version));
				INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
				VALUES (OLD.email, op, 'contact_segment', kind_value, OLD.segment_id, changes_json, CURRENT_TIMESTAMP);
				RETURN OLD;
			END IF;
			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
			VALUES (NEW.email, op, 'contact_segment', kind_value, NEW.segment_id, changes_json, CURRENT_TIMESTAMP);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;`
}
