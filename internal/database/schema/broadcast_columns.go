package schema

// BroadcastTestPausedSecondsColumn is the definition of broadcasts.test_paused_seconds, the
// time an A/B test waiting for its winner spent paused. A fresh workspace creates it with the
// table, and v44 adds it to the existing ones, from this one definition.
const BroadcastTestPausedSecondsColumn = "test_paused_seconds INTEGER NOT NULL DEFAULT 0"
