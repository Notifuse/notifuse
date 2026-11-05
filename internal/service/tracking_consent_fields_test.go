package service

import (
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryBuilder_TrackingConsentFields(t *testing.T) {
	qb := NewQueryBuilder()
	build := func(t *testing.T, filter *domain.DimensionFilter) (string, []interface{}) {
		t.Helper()
		sql, args, err := qb.BuildSQL(&domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{Source: "contacts", Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{filter}}},
		})
		require.NoError(t, err)
		return sql, args
	}

	sql, args := build(t, &domain.DimensionFilter{FieldName: "tracking_clicks_consent", FieldType: "string", Operator: "equals", StringValues: []string{domain.TrackingConsentDenied}})
	assert.Equal(t, "SELECT email FROM contacts WHERE (tracking_clicks_consent = $1)", sql)
	assert.Equal(t, []interface{}{domain.TrackingConsentDenied}, args)

	sql, _ = build(t, &domain.DimensionFilter{FieldName: "tracking_opens_consent", FieldType: "string", Operator: "is_not_set"})
	assert.Contains(t, sql, "tracking_opens_consent IS NULL", "unknown consent is a NULL column")

	sql, _ = build(t, &domain.DimensionFilter{FieldName: "tracking_consent_source", FieldType: "string", Operator: "equals", StringValues: []string{"import"}})
	assert.Contains(t, sql, "tracking_consent_source = $1")

	sql, _ = build(t, &domain.DimensionFilter{FieldName: "tracking_consent_updated_at", FieldType: "time", Operator: "is_set"})
	assert.Contains(t, sql, "tracking_consent_updated_at IS NOT NULL")
}

func TestAutomationTriggerGenerator_TrackingConsentUpdatedFields(t *testing.T) {
	gen := NewAutomationTriggerGenerator(NewQueryBuilder())
	result, err := gen.Generate(&domain.Automation{
		ID:         "consentchange",
		ListID:     "list1",
		RootNodeID: "node1",
		Trigger: &domain.TimelineTriggerConfig{
			EventKind:     "contact.updated",
			UpdatedFields: []string{"tracking_opens_consent", "tracking_clicks_consent"},
			Frequency:     domain.TriggerFrequencyEveryTime,
		},
	})
	require.NoError(t, err)
	assert.Contains(t, result.WHENClause, "NEW.changes ? 'tracking_opens_consent'")
	assert.Contains(t, result.WHENClause, "NEW.changes ? 'tracking_clicks_consent'")
}
