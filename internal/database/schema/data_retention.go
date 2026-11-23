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

// DataRetentionMirrorStatements add the mirror of the inactive-contact policy (D13) to
// data_retention_state: the period in force with its exemptions and engagement event names,
// and the period that waits, with its own exemptions and event names and the date it applies.
// DataRetentionService writes it on every save, cancellation and run; the expiring-soon
// condition reads it.
func DataRetentionMirrorStatements() []string {
	return []string{
		`ALTER TABLE data_retention_state ADD COLUMN IF NOT EXISTS contacts_days INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE data_retention_state ADD COLUMN IF NOT EXISTS contacts_pending_days INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE data_retention_state ADD COLUMN IF NOT EXISTS contacts_pending_at TIMESTAMPTZ`,
		`ALTER TABLE data_retention_state ADD COLUMN IF NOT EXISTS exempt_list_ids TEXT[] NOT NULL DEFAULT '{}'`,
		`ALTER TABLE data_retention_state ADD COLUMN IF NOT EXISTS exempt_segment_ids TEXT[] NOT NULL DEFAULT '{}'`,
		`ALTER TABLE data_retention_state ADD COLUMN IF NOT EXISTS engagement_event_names TEXT[] NOT NULL DEFAULT '{}'`,
		`ALTER TABLE data_retention_state ADD COLUMN IF NOT EXISTS pending_exempt_list_ids TEXT[] NOT NULL DEFAULT '{}'`,
		`ALTER TABLE data_retention_state ADD COLUMN IF NOT EXISTS pending_exempt_segment_ids TEXT[] NOT NULL DEFAULT '{}'`,
		`ALTER TABLE data_retention_state ADD COLUMN IF NOT EXISTS pending_engagement_event_names TEXT[] NOT NULL DEFAULT '{}'`,
	}
}

// The expiry rule of the inactive-contact category, as SQL over a contact row (alias {c}), its
// engagement row (alias e) and the mirror row (alias st). It is the purge's own rule
// (DataRetentionRepository.ListExpiredContacts):
//   - the clock is the latest of collection, last engagement and last designated custom event;
//   - the period in force applies unless the contact is exempt; the period that waits applies
//     with its own exemptions and event names, never before its date;
//   - never within 7 days of the contact being stored;
//   - nothing expires before the engagement backfill has run, or while an exempt list is gone
//     or an exempt segment is not active (the purge then skips the category).
func retentionClock(c, names string) string {
	return `GREATEST(
			COALESCE(e.last_engaged_at, '-infinity'::timestamptz),
			LEAST(` + c + `.created_at, ` + c + `.db_created_at),
			COALESCE((SELECT MAX(LEAST(ce.occurred_at, NOW())) FROM custom_events ce
				WHERE ce.email = ` + c + `.email AND ce.event_name = ANY(st.` + names + `) AND ce.deleted_at IS NULL), '-infinity'::timestamptz))`
}

func retentionExempt(c, lists, segments string) string {
	return `(EXISTS (SELECT 1 FROM contact_lists cl WHERE cl.email = ` + c + `.email AND cl.list_id = ANY(st.` + lists + `) AND cl.deleted_at IS NULL)
			OR EXISTS (SELECT 1 FROM contact_segments cs WHERE cs.email = ` + c + `.email AND cs.segment_id = ANY(st.` + segments + `)))`
}

const retentionReady = `st.id = 1
		AND st.backfill_completed_at IS NOT NULL
		AND (SELECT COUNT(*) FROM lists l WHERE l.id = ANY(st.exempt_list_ids) AND l.deleted_at IS NULL) = cardinality(st.exempt_list_ids)
		AND (SELECT COUNT(*) FROM segments sg WHERE sg.id = ANY(st.exempt_segment_ids) AND sg.status = 'active') = cardinality(st.exempt_segment_ids)`

// retentionExpiries returns the FROM and WHERE parts shared by the condition and the function:
// x.cur and x.pend are the two expiries, NULL when the period does not apply to the contact.
func retentionExpiries(c string) string {
	return `FROM data_retention_state st
		LEFT JOIN contact_engagement e ON e.email = ` + c + `.email
		CROSS JOIN LATERAL (SELECT
			CASE WHEN st.contacts_days > 0 AND NOT ` + retentionExempt(c, "exempt_list_ids", "exempt_segment_ids") + `
				THEN ` + retentionClock(c, "engagement_event_names") + ` + make_interval(days => st.contacts_days) END AS cur,
			CASE WHEN st.contacts_pending_days > 0 AND st.contacts_pending_at IS NOT NULL
				AND NOT ` + retentionExempt(c, "pending_exempt_list_ids", "pending_exempt_segment_ids") + `
				THEN GREATEST(` + retentionClock(c, "pending_engagement_event_names") + ` + make_interval(days => st.contacts_pending_days), st.contacts_pending_at) END AS pend
		) x
		WHERE ` + retentionReady
}

// retentionExpiry is the expiry of the contact: the earlier of the two, never before the floor.
func retentionExpiry(c string) string {
	return `CASE WHEN LEAST(x.cur, x.pend) IS NULL THEN NULL
		ELSE GREATEST(LEAST(x.cur, x.pend), ` + c + `.db_created_at + INTERVAL '7 days') END`
}

// RetentionExpiresWithinCondition is the SQL of the expiring-soon segment condition for the
// contacts table referenced as contactRef, with the number of days in daysParam. The policy is
// read from the mirror when the query runs, never written into the SQL; the rule itself is,
// as for any condition, so a later change to it means regenerating the segments that use it.
// An EXISTS over the contact's own rows: one plan serves a whole build and a single contact.
func RetentionExpiresWithinCondition(contactRef, daysParam string) string {
	return `EXISTS (SELECT 1 ` + retentionExpiries(contactRef) + `
		AND LEAST(x.cur, x.pend) IS NOT NULL
		AND GREATEST(LEAST(x.cur, x.pend), ` + contactRef + `.db_created_at + INTERVAL '7 days') <= NOW() + make_interval(days => ` + daysParam + `))`
}

// ContactRetentionExpiresAtFunction returns contact_retention_expires_at(email): when the
// inactive-contact category will delete the contact, or NULL when it will not. The same rule as
// the condition; the agreement test compares it with the purge.
func ContactRetentionExpiresAtFunction() string {
	return `CREATE OR REPLACE FUNCTION contact_retention_expires_at(p_email TEXT)
		RETURNS TIMESTAMPTZ
		LANGUAGE sql STABLE AS $$
		SELECT ` + retentionExpiry("c") + `
		FROM contacts c
		JOIN LATERAL (SELECT x.cur, x.pend ` + retentionExpiries("c") + `) x ON TRUE
		WHERE c.email = p_email
		$$`
}
