package schema

// SegmentLastErrorColumns define segments.last_error, the last failure of a segment's
// computation (domain.SegmentLastError as JSON), and segments.last_error_at, when it was
// recorded. Both are NULL while nothing failed. A fresh workspace creates them with the table, and
// v44 adds them to the existing ones, from these definitions.
const (
	SegmentLastErrorColumn   = "last_error JSONB"
	SegmentLastErrorAtColumn = "last_error_at TIMESTAMP WITH TIME ZONE"
)
