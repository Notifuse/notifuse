package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func backfillRule(id string, priority int, conditions []domain.WebFilterCondition, operations []domain.WebFilterOperation) domain.WebFilter {
	return domain.WebFilter{
		ID: id, Name: id, Priority: priority, Enabled: true,
		Conditions: conditions, Operations: operations,
	}
}

func TestCompileWebFiltersToSetClause(t *testing.T) {
	t.Run("golden clause for a three-rule fixture", func(t *testing.T) {
		filters := []domain.WebFilter{
			backfillRule("high", 900,
				[]domain.WebFilterCondition{{Field: "utm_id_from", Operator: domain.WebFilterOpRegex, Value: "^(gclid|wbraid)$"}},
				[]domain.WebFilterOperation{
					{Dimension: "channel", Action: domain.WebFilterActionSetValue, Value: "google-ads"},
					{Dimension: "channel_group", Action: domain.WebFilterActionSetValue, Value: "search-paid"},
				}),
			backfillRule("mid", 600,
				[]domain.WebFilterCondition{{Field: "referrer_domain", Operator: domain.WebFilterOpContains, Value: "google"}},
				[]domain.WebFilterOperation{{Dimension: "channel", Action: domain.WebFilterActionSetValue, Value: "google-organic"}}),
			backfillRule("fallback", 10,
				nil,
				[]domain.WebFilterOperation{{Dimension: "channel", Action: domain.WebFilterActionSetDefaultValue, Value: "not-mapped"}}),
		}

		clause, err := CompileWebFiltersToSetClause(filters)
		require.NoError(t, err)

		// channel: set rules in priority order, then the default, then reset.
		assert.Contains(t, clause,
			"channel = CASE WHEN ((utm_id_from <> '' AND utm_id_from ~ '^(gclid|wbraid)$')) THEN 'google-ads'"+
				" WHEN ((referrer_domain <> '' AND referrer_domain LIKE '%google%')) THEN 'google-organic'"+
				" WHEN TRUE THEN 'not-mapped' ELSE '' END")
		// channel_group has one branch and the reset fallback.
		assert.Contains(t, clause, "channel_group = CASE WHEN ((utm_id_from <> '' AND utm_id_from ~ '^(gclid|wbraid)$')) THEN 'search-paid' ELSE '' END")
		// Untouched reset dimensions are cleared — but only the rule-owned ones.
		// custom_* carry tracker-supplied data and are left alone; see
		// TestCompileWebFiltersLeavesTrackerCustomDimensionsAlone.
		assert.NotContains(t, clause, "custom_1 = ''")
		assert.NotContains(t, clause, "custom_10 = ''")
		// Untouched passthrough dimensions don't appear at all.
		assert.NotContains(t, clause, "utm_source")
		assert.NotContains(t, clause, "is_direct =")
	})

	t.Run("priority order is preserved and defaults never outrank sets", func(t *testing.T) {
		filters := []domain.WebFilter{
			backfillRule("default-first", 950, nil,
				[]domain.WebFilterOperation{{Dimension: "channel", Action: domain.WebFilterActionSetDefaultValue, Value: "early-default"}}),
			backfillRule("low-set", 100, nil,
				[]domain.WebFilterOperation{{Dimension: "channel", Action: domain.WebFilterActionSetValue, Value: "real"}}),
		}
		clause, err := CompileWebFiltersToSetClause(filters)
		require.NoError(t, err)
		assert.Less(t, strings.Index(clause, "'real'"), strings.Index(clause, "'early-default'"),
			"set_value branches must precede set_default_value branches regardless of priority")
	})

	t.Run("literal escaping is airtight", func(t *testing.T) {
		filters := []domain.WebFilter{
			backfillRule("evil", 500,
				[]domain.WebFilterCondition{{Field: "utm_source", Operator: domain.WebFilterOpEquals, Value: `o'reilly\`}},
				[]domain.WebFilterOperation{{Dimension: "channel", Action: domain.WebFilterActionSetValue, Value: `x'; DROP TABLE web_sessions;--`}}),
		}
		clause, err := CompileWebFiltersToSetClause(filters)
		require.NoError(t, err)
		assert.NotContains(t, clause, `'x'; DROP`, "an unescaped quote would terminate the literal early")
		assert.Contains(t, clause, `'x''; DROP TABLE web_sessions;--'`, "the quote must be doubled inside the literal")
		// pq.QuoteLiteral escapes backslashes via the E'' form.
		assert.Contains(t, clause, `o''reilly`)
	})

	t.Run("LIKE wildcards in contains values are escaped", func(t *testing.T) {
		filters := []domain.WebFilter{
			backfillRule("wild", 500,
				[]domain.WebFilterCondition{{Field: "landing_path", Operator: domain.WebFilterOpContains, Value: "50%_off"}},
				[]domain.WebFilterOperation{{Dimension: "channel", Action: domain.WebFilterActionSetValue, Value: "promo"}}),
		}
		clause, err := CompileWebFiltersToSetClause(filters)
		require.NoError(t, err)
		assert.Contains(t, clause, `\%`)
		assert.Contains(t, clause, `\_`)
	})

	t.Run("is_direct conditions and assignments use boolean SQL", func(t *testing.T) {
		filters := []domain.WebFilter{
			backfillRule("direct", 700,
				[]domain.WebFilterCondition{{Field: "is_direct", Operator: domain.WebFilterOpEquals, Value: "true"}},
				[]domain.WebFilterOperation{
					{Dimension: "channel", Action: domain.WebFilterActionSetValue, Value: "direct"},
					{Dimension: "is_direct", Action: domain.WebFilterActionSetValue, Value: "false"},
				}),
		}
		clause, err := CompileWebFiltersToSetClause(filters)
		require.NoError(t, err)
		assert.Contains(t, clause, "CASE WHEN is_direct THEN 'true' ELSE 'false' END")
		assert.Contains(t, clause, "is_direct = CASE WHEN")
		assert.Contains(t, clause, "THEN FALSE ELSE is_direct END")
	})

	t.Run("disabled rules are skipped; empty rule set still resets channel dims", func(t *testing.T) {
		disabled := backfillRule("off", 900, nil,
			[]domain.WebFilterOperation{{Dimension: "channel", Action: domain.WebFilterActionSetValue, Value: "x"}})
		disabled.Enabled = false

		clause, err := CompileWebFiltersToSetClause([]domain.WebFilter{disabled})
		require.NoError(t, err)
		assert.NotContains(t, clause, "'x'")
		assert.Contains(t, clause, "channel = ''")
		assert.NotContains(t, clause, "custom_5 = ''",
			"disabling a rule must not blank a dimension the tracker also writes")
	})

	t.Run("empty-value semantics: equals never matches empty fields", func(t *testing.T) {
		filters := []domain.WebFilter{
			backfillRule("r", 500,
				[]domain.WebFilterCondition{{Field: "utm_medium", Operator: domain.WebFilterOpNotEquals, Value: "email"}},
				[]domain.WebFilterOperation{{Dimension: "channel", Action: domain.WebFilterActionSetValue, Value: "other"}}),
		}
		clause, err := CompileWebFiltersToSetClause(filters)
		require.NoError(t, err)
		assert.Contains(t, clause, "utm_medium <> '' AND utm_medium <> 'email'")
	})
}

func TestBackfillPartition(t *testing.T) {
	filters := []domain.WebFilter{
		backfillRule("r", 500, nil,
			[]domain.WebFilterOperation{{Dimension: "channel", Action: domain.WebFilterActionSetValue, Value: "x"}}),
	}

	t.Run("rejects invalid or foreign partitions", func(t *testing.T) {
		repo, _, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		_, err := repo.BackfillPartition(context.Background(), waTestWorkspace, "contacts; DROP", filters)
		assert.ErrorContains(t, err, "invalid partition name")

		_, err = repo.BackfillPartition(context.Background(), waTestWorkspace, "web_pages_y2026m08", filters)
		assert.ErrorContains(t, err, "only applies to web_sessions and web_goals")
	})

	t.Run("updates the partition and returns affected rows", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		mock.ExpectExec(`UPDATE "web_sessions_y2026m08" SET .*channel = CASE`).
			WillReturnResult(sqlmock.NewResult(0, 1234))

		rows, err := repo.BackfillPartition(context.Background(), waTestWorkspace, "web_sessions_y2026m08", filters)
		require.NoError(t, err)
		assert.Equal(t, int64(1234), rows)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// A backfill rewrites historical rows from the workspace's rules. That is right
// for channel and channel_group, which have no other source: they exist only
// because a rule said so, so "no rule matches" genuinely means "empty".
//
// custom_1..custom_10 are different. The tracker sets them from the beat's own
// dimensions (web_analytics_enrichment.go), so a workspace can populate them
// without any rule at all. Clearing them on backfill destroyed data the site had
// supplied — and a backfill is triggered by editing an unrelated attribution
// rule, so the loss looked unconnected to the action that caused it.
func TestCompileWebFiltersLeavesTrackerCustomDimensionsAlone(t *testing.T) {
	t.Run("an untargeted custom dimension is not assigned at all", func(t *testing.T) {
		filters := []domain.WebFilter{
			backfillRule("r", 900,
				[]domain.WebFilterCondition{{Field: "utm_source", Operator: domain.WebFilterOpEquals, Value: "google"}},
				[]domain.WebFilterOperation{{Dimension: "channel", Action: domain.WebFilterActionSetValue, Value: "search"}}),
		}

		clause, err := CompileWebFiltersToSetClause(filters)
		require.NoError(t, err)

		// Asserted in the same test as the channel reset, so the two behaviours
		// cannot drift apart: one dimension is rule-owned, the other is not.
		assert.Contains(t, clause, "channel = CASE", "channel is rule-owned and still rewritten")
		for i := 1; i <= 10; i++ {
			dim := fmt.Sprintf("custom_%d", i)
			assert.NotContains(t, clause, dim+" = ''",
				"%s holds tracker-supplied data; a backfill must not blank it", dim)
			assert.NotContains(t, clause, dim+" =",
				"%s is untargeted, so the clause should not mention it at all", dim)
		}
	})

	t.Run("a targeted custom dimension keeps its own value when no branch matches", func(t *testing.T) {
		filters := []domain.WebFilter{
			backfillRule("vip", 900,
				[]domain.WebFilterCondition{{Field: "utm_campaign", Operator: domain.WebFilterOpEquals, Value: "vip"}},
				[]domain.WebFilterOperation{{Dimension: "custom_3", Action: domain.WebFilterActionSetValue, Value: "vip"}}),
		}

		clause, err := CompileWebFiltersToSetClause(filters)
		require.NoError(t, err)

		assert.Contains(t, clause, "custom_3 = CASE")
		assert.Contains(t, clause, "ELSE custom_3 END",
			"a row the rule does not match must keep whatever the tracker set")
		assert.NotContains(t, clause, "custom_3 = CASE WHEN ((utm_campaign <> '' AND utm_campaign = 'vip')) THEN 'vip' ELSE '' END")
	})

	t.Run("unset_value still clears the rows it matches", func(t *testing.T) {
		// The escape hatch: a workspace that really does want a custom dimension
		// cleared says so with a rule, rather than getting it as a side effect of
		// editing an unrelated one.
		filters := []domain.WebFilter{
			backfillRule("wipe", 900,
				[]domain.WebFilterCondition{{Field: "utm_source", Operator: domain.WebFilterOpEquals, Value: "spam"}},
				[]domain.WebFilterOperation{{Dimension: "custom_4", Action: domain.WebFilterActionUnsetValue}}),
		}

		clause, err := CompileWebFiltersToSetClause(filters)
		require.NoError(t, err)

		assert.Contains(t, clause, "custom_4 = CASE")
		assert.Contains(t, clause, "THEN ''", "a matched row is still cleared")
		assert.Contains(t, clause, "ELSE custom_4 END", "an unmatched row is still left alone")
	})

	t.Run("a disabled rule clears channel but never a custom dimension", func(t *testing.T) {
		disabled := domain.WebFilter{
			ID: "off", Name: "off", Priority: 900, Enabled: false,
			Conditions: []domain.WebFilterCondition{{Field: "utm_source", Operator: domain.WebFilterOpEquals, Value: "x"}},
			Operations: []domain.WebFilterOperation{
				{Dimension: "channel", Action: domain.WebFilterActionSetValue, Value: "x"},
				{Dimension: "custom_5", Action: domain.WebFilterActionSetValue, Value: "x"},
			},
		}

		clause, err := CompileWebFiltersToSetClause([]domain.WebFilter{disabled})
		require.NoError(t, err)

		assert.Contains(t, clause, "channel = ''", "a rule-owned dimension with no live rule is empty")
		assert.NotContains(t, clause, "custom_5",
			"disabling a rule must not destroy what the tracker put in custom_5")
	})
}
