package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageTableDefinitions_Idempotent(t *testing.T) {
	defs := UsageTableDefinitions()
	require.NotEmpty(t, defs)

	for _, stmt := range defs {
		assert.Contains(t, stmt, "IF NOT EXISTS", "every statement must be idempotent: %s", stmt)
	}

	joined := strings.Join(defs, "\n")
	assert.Contains(t, joined, "CREATE TABLE IF NOT EXISTS monthly_usage (")
	// The table is not partitioned, so it must never appear in the slice that
	// drives monthly partition creation.
	assert.NotContains(t, joined, "PARTITION")
	assert.NotContains(t, WebAnalyticsTableNames, "monthly_usage")
}

func TestUsageTableDefinitions_BillableTimelineIndexExcludesWebRows(t *testing.T) {
	defs := UsageTableDefinitions()

	var index string
	for _, stmt := range defs {
		if strings.Contains(stmt, "idx_contact_timeline_billable") {
			index = stmt
		}
	}
	require.NotEmpty(t, index, "the billable timeline index must exist")

	assert.Contains(t, index, "ON contact_timeline (created_at)")
	// These two literals are written by web_analytics_timeline_projection.go. The
	// index predicate and the meter's WHERE clause have to agree with it and with
	// each other, or the meter either bills pageviews twice or stops using the
	// index. Pinned here as well as in the repository test so a rename on the
	// projection side is caught from both directions.
	assert.Contains(t, index, "WHERE entity_type NOT IN ('web_page', 'web_session')")
}

func TestUsageTableDefinitions_StatementCount(t *testing.T) {
	defs := UsageTableDefinitions()
	require.Len(t, defs, 2, "monthly_usage table, billable contact_timeline index")
}
