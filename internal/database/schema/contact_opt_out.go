package schema

// The opt-outs of a deleted contact, kept as hashes so that a contact who comes back is
// not mailed against an unsubscribe, a bounce or a complaint they left before.
//
// When a contact is deleted, their unsubscribed, bounced and complained rows on
// contact_lists are copied into contact_opt_out_hashes under a keyed hash of the address,
// with no other field. When the address is inserted into contacts again, a trigger puts
// those statuses back on the lists that still exist and deletes the hashes. The hashes are
// deleted after three years by the data retention worker.
//
// Two paths install these objects: the new-workspace path (internal/database/init.go) and
// the v45 migration on every existing workspace.

// OptOutRestoreGUC is the transaction-local setting the restore trigger turns on around its
// insert, so the contact_lists triggers record the restored rows under their own timeline
// kind and send no webhook: a re-import must not run what listens for list.unsubscribed.
const OptOutRestoreGUC = "notifuse.opt_out_restore"

// OptOutRestoredKind is the timeline kind of an opt-out put back on a returning contact.
const OptOutRestoredKind = "list.opt_out_restored"

// ContactOptOutRestoreTrigger puts a returning contact's opt-outs back. PostgreSQL fires the
// AFTER INSERT triggers of a table in the alphabetical order of their names, and this one must
// run before contact_changes_trigger: that one writes contact.created to the timeline, which
// enrolls the contact in the automations it starts, and their conditions on list statuses
// must see the restored opt-outs.
const ContactOptOutRestoreTrigger = "contact_before_changes_opt_out_restore_trigger"

// ContactChangesTrigger writes a contact's inserts and updates to the timeline
// (track_contact_changes). internal/database/init.go and the shipped migrations write the name
// as a literal, which the architecture repository's references look for: keep them equal.
const ContactChangesTrigger = "contact_changes_trigger"

// ContactOptOutStatements returns the tables, the key, the hash function and the restore
// trigger, in the order they must run. Every statement is idempotent.
//
// The key is 32 random bytes generated in SQL with gen_random_uuid(), core PostgreSQL since
// 13, so no extension is needed; a workspace built from scratch, migrated or repaired gets
// one the same way. It keeps the hashes of one workspace from matching another's. It sits in
// the same database as the hashes, so it does not protect against someone holding that
// database: a hash of an address stays personal data.
func ContactOptOutStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS contact_opt_out_hash_key (
			id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
			key BYTEA NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`INSERT INTO contact_opt_out_hash_key (id, key)
			VALUES (1, uuid_send(gen_random_uuid()) || uuid_send(gen_random_uuid()))
			ON CONFLICT (id) DO NOTHING`,
		`CREATE TABLE IF NOT EXISTS contact_opt_out_hashes (
			email_hash BYTEA NOT NULL,
			list_id VARCHAR(32) NOT NULL,
			status VARCHAR(20) NOT NULL CHECK (status IN ('unsubscribed', 'bounced', 'complained')),
			-- The contact had been removed from the list: they come back removed, so the
			-- opt-out still guards the list without making them a member again
			removed BOOLEAN NOT NULL DEFAULT false,
			opted_out_at TIMESTAMPTZ NOT NULL,
			recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source VARCHAR(20) NOT NULL CHECK (source IN ('delete', 'retention')),
			PRIMARY KEY (email_hash, list_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_opt_out_hashes_recorded_at ON contact_opt_out_hashes(recorded_at)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_opt_out_hashes_list_id ON contact_opt_out_hashes(list_id)`,
		ContactOptOutHashFunction(),
		RestoreContactOptOutsFunction(),
		// The name dev builds of 45 gave it, before it had to run first.
		`DROP TRIGGER IF EXISTS contact_opt_out_restore_trigger ON contacts`,
		`DROP TRIGGER IF EXISTS ` + ContactOptOutRestoreTrigger + ` ON contacts`,
		`CREATE TRIGGER ` + ContactOptOutRestoreTrigger + ` AFTER INSERT ON contacts FOR EACH ROW EXECUTE FUNCTION restore_contact_opt_outs()`,
	}
}

// ContactOptOutHashFunction returns contact_opt_out_hash(email): the keyed SHA-256 of the
// trimmed, lowercased address. It is computed only here, never in Go, because Go's
// strings.ToLower and PostgreSQL's lower() can differ. The "C" collation keeps the hash
// independent of the database's collation, so a pg_upgrade or a restore elsewhere cannot
// change it; with "C" only ASCII letters are lowered, which is enough since writes and
// lookups apply the same function. A missing key raises rather than return NULL, which
// would match nothing and silently forget every opt-out.
func ContactOptOutHashFunction() string {
	return `CREATE OR REPLACE FUNCTION contact_opt_out_hash(p_email TEXT)
		RETURNS BYTEA AS $$
		DECLARE
			k BYTEA;
		BEGIN
			SELECT key INTO k FROM contact_opt_out_hash_key WHERE id = 1;
			IF k IS NULL THEN
				RAISE EXCEPTION 'contact_opt_out_hash_key has no key';
			END IF;
			RETURN sha256(k || convert_to(lower(btrim(p_email) COLLATE "C"), 'UTF8'));
		END;
		$$ LANGUAGE plpgsql STABLE;`
}

// RestoreContactOptOutsFunction returns the AFTER INSERT trigger function on contacts that
// puts a returning contact's opt-outs back. Only a real insert fires it: an upsert that
// updates an existing contact does not.
//
// The restored rows are ordinary unsubscribed, bounced or complained rows, created now, on
// lists that still exist (a list is soft-deleted). A contact who had been removed from the
// list comes back removed: a member again, an unsubscribed contact would be in reach of the
// broadcasts that include unsubscribed contacts. A row already there wins (ON CONFLICT DO
// NOTHING). The hashes are deleted once restored. The insert runs with OptOutRestoreGUC on,
// and turns it off again before returning, so later writes in the same transaction are
// recorded as usual.
func RestoreContactOptOutsFunction() string {
	return `CREATE OR REPLACE FUNCTION restore_contact_opt_outs()
		RETURNS TRIGGER AS $$
		DECLARE
			h BYTEA;
		BEGIN
			-- Most workspaces keep no hash: skip the hash computation on every insert then
			IF NOT EXISTS (SELECT 1 FROM contact_opt_out_hashes) THEN
				RETURN NEW;
			END IF;
			h := contact_opt_out_hash(NEW.email);
			IF NOT EXISTS (SELECT 1 FROM contact_opt_out_hashes WHERE email_hash = h) THEN
				RETURN NEW;
			END IF;

			PERFORM set_config('` + OptOutRestoreGUC + `', 'on', true);
			INSERT INTO contact_lists (email, list_id, status, created_at, updated_at, deleted_at)
			SELECT NEW.email, o.list_id, o.status, NOW(), NOW(), CASE WHEN o.removed THEN NOW() END
			FROM contact_opt_out_hashes o
			JOIN lists l ON l.id = o.list_id AND l.deleted_at IS NULL
			WHERE o.email_hash = h
			ON CONFLICT (email, list_id) DO NOTHING;
			PERFORM set_config('` + OptOutRestoreGUC + `', 'off', true);

			DELETE FROM contact_opt_out_hashes WHERE email_hash = h;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;`
}
