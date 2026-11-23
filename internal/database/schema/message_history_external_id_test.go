package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Unique, and partial: the many messages without an external_id never conflict.
func TestMessageHistoryExternalIDIndex(t *testing.T) {
	assert.Equal(t, "idx_message_history_external_id", MessageHistoryExternalIDIndexName)
	assert.Equal(t,
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_message_history_external_id ON message_history(external_id) WHERE external_id IS NOT NULL",
		MessageHistoryExternalIDIndex)
}
