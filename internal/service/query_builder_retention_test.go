package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// The condition calls the workspace function and passes the days as the only argument: the
// policy's period is read when the query runs, never written into the SQL.
func TestQueryBuilder_RetentionCondition(t *testing.T) {
	tree := &domain.TreeNode{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{Source: domain.RetentionLeafSource,
		Retention: &domain.RetentionCondition{Operator: domain.RetentionConditionExpiresWithinDays, Days: 30}}}

	sql, args, err := NewQueryBuilder().BuildSQL(tree)
	require.NoError(t, err)
	assert.Contains(t, sql, "EXISTS (SELECT 1 FROM data_retention_state st")
	assert.Contains(t, sql, "<= NOW() + make_interval(days => $1)")
	assert.Equal(t, []interface{}{30}, args)
	assert.False(t, strings.Contains(sql, "1095") || strings.Contains(sql, " 365"), "no period literal: %s", sql)

	// Branch and filter steps compile it too.
	_, _, err = NewQueryBuilder().BuildDefensiveSQL(tree)
	require.NoError(t, err)

	// A trigger never does.
	_, _, err = NewQueryBuilder().BuildTriggerCondition(tree, "NEW.email")
	assert.Error(t, err)
}
