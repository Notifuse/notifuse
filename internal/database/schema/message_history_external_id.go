package schema

// MessageHistoryExternalIDIndexName is the unique index on message_history.external_id. A
// write that would give a second message the same external_id fails with a unique violation
// that names this index, which is how the repository tells it from any other conflict.
const MessageHistoryExternalIDIndexName = "idx_message_history_external_id"

// MessageHistoryExternalIDIndex makes external_id, the key transactional.send deduplicates
// on, unique among the messages that carry one. A fresh workspace creates it with the table,
// and v45 creates it on the existing ones, from this one definition.
const MessageHistoryExternalIDIndex = `CREATE UNIQUE INDEX IF NOT EXISTS ` + MessageHistoryExternalIDIndexName + ` ON message_history(external_id) WHERE external_id IS NOT NULL`
