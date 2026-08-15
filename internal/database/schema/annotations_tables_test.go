package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnnotationsTableDefinitions_Idempotent(t *testing.T) {
	defs := AnnotationsTableDefinitions()
	require.NotEmpty(t, defs)

	for _, stmt := range defs {
		assert.Contains(t, stmt, "IF NOT EXISTS", "every statement must be idempotent: %s", stmt)
	}

	joined := strings.Join(defs, "\n")
	assert.Contains(t, joined, "CREATE TABLE IF NOT EXISTS annotations (")
	// The table is not partitioned, so it must never appear in the slice that
	// drives monthly partition creation.
	assert.NotContains(t, joined, "PARTITION")
	assert.NotContains(t, WebAnalyticsTableNames, "annotations")
}

func TestAnnotationsTableDefinitions_UniqueSourceIndexIsPartial(t *testing.T) {
	defs := AnnotationsTableDefinitions()

	var unique string
	for _, stmt := range defs {
		if strings.Contains(stmt, "CREATE UNIQUE INDEX") {
			unique = stmt
		}
	}
	require.NotEmpty(t, unique, "the (source, source_id) unique index must exist")

	assert.Contains(t, unique, "ON annotations (source, source_id)")
	// The predicate is what keeps manual rows (source_id NULL) unconstrained, and
	// it is also what every ON CONFLICT against this index has to repeat verbatim:
	// PostgreSQL will not infer a partial unique index as an arbiter and raises
	// 42P10 instead. Losing the predicate here silently breaks both.
	assert.Contains(t, unique, "WHERE source_id IS NOT NULL")
}

func TestAnnotationsTableDefinitions_StatementCount(t *testing.T) {
	defs := AnnotationsTableDefinitions()
	require.Len(t, defs, 3, "table, annotated_at index, partial unique source index")
}
