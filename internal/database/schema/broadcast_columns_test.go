package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A count that cannot be NULL and starts at 0, so an existing broadcast reads as never paused.
func TestBroadcastTestPausedSecondsColumn(t *testing.T) {
	assert.Equal(t, "test_paused_seconds INTEGER NOT NULL DEFAULT 0", BroadcastTestPausedSecondsColumn)
}
