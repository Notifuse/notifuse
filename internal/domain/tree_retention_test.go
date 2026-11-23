package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func retentionLeaf(days int) *TreeNode {
	return &TreeNode{Kind: "leaf", Leaf: &TreeNodeLeaf{Source: RetentionLeafSource,
		Retention: &RetentionCondition{Operator: RetentionConditionExpiresWithinDays, Days: days}}}
}

func TestRetentionCondition(t *testing.T) {
	t.Run("JSON round trip", func(t *testing.T) {
		raw := `{"kind":"leaf","leaf":{"source":"retention","retention":{"operator":"expires_within_days","days":30}}}`
		var tree TreeNode
		require.NoError(t, json.Unmarshal([]byte(raw), &tree))
		require.NoError(t, tree.Validate())
		assert.Equal(t, 30, tree.Leaf.Retention.Days)
		back, err := json.Marshal(&tree)
		require.NoError(t, err)
		assert.JSONEq(t, raw, string(back))
	})

	t.Run("bounds and operator", func(t *testing.T) {
		assert.NoError(t, retentionLeaf(1).Validate())
		assert.NoError(t, retentionLeaf(365).Validate())
		assert.Error(t, retentionLeaf(0).Validate())
		assert.Error(t, retentionLeaf(366).Validate())
		wrong := retentionLeaf(30)
		wrong.Leaf.Retention.Operator = "expired"
		assert.Error(t, wrong.Validate())
		missing := &TreeNode{Kind: "leaf", Leaf: &TreeNodeLeaf{Source: RetentionLeafSource}}
		assert.Error(t, missing.Validate())
	})

	t.Run("has relative dates, alone or nested", func(t *testing.T) {
		assert.True(t, retentionLeaf(30).HasRelativeDates())
		nested := &TreeNode{Kind: "branch", Branch: &TreeNodeBranch{Operator: "and", Leaves: []*TreeNode{
			{Kind: "branch", Branch: &TreeNodeBranch{Operator: "or", Leaves: []*TreeNode{retentionLeaf(30)}}}}}}
		assert.True(t, nested.HasRelativeDates())
		assert.True(t, nested.HasRetentionLeaf())
		assert.False(t, (&TreeNode{Kind: "leaf", Leaf: &TreeNodeLeaf{Source: "contacts"}}).HasRetentionLeaf())
	})
}

// An automation trigger refuses the condition, nested or not, and says what to do instead.
func TestTimelineTriggerConfig_RefusesTheRetentionCondition(t *testing.T) {
	for name, conditions := range map[string]*TreeNode{
		"alone":  retentionLeaf(30),
		"nested": {Kind: "branch", Branch: &TreeNodeBranch{Operator: "and", Leaves: []*TreeNode{retentionLeaf(30)}}},
	} {
		t.Run(name, func(t *testing.T) {
			event := "email.clicked"
			config := &TimelineTriggerConfig{EventKind: event, Frequency: TriggerFrequencyOnce, Conditions: conditions}
			err := config.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "segment.joined")
		})
	}
}

func TestContactPolicyMirrorAt(t *testing.T) {
	now := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	week := now.Add(RetentionPendingDelay)
	settings := &DataRetentionSettings{
		Policy: &DataRetentionPolicy{Categories: map[RetentionCategory]int{RetentionInactiveContacts: 1095}, ExemptListIDs: []string{"vip"}},
		Pending: &PendingRetentionChange{EffectiveAt: week,
			Policy: &DataRetentionPolicy{Categories: map[RetentionCategory]int{RetentionInactiveContacts: 365}}},
	}
	mirror := ContactPolicyMirrorAt(settings, nil, now)
	assert.Equal(t, 1095, mirror.Days)
	assert.Equal(t, 365, mirror.PendingDays)
	require.NotNil(t, mirror.PendingAt)
	assert.Equal(t, week, *mirror.PendingAt)
	assert.Equal(t, []string{"vip"}, mirror.ExemptListIDs, "the exemption holds until the change applies")

	off := ContactPolicyMirrorAt(nil, nil, now)
	assert.Zero(t, off.Days)
	assert.Nil(t, off.PendingAt)
}

// A waiting change that only removes an exemption deletes more with the same period: it is
// mirrored as waiting, with its own exemptions.
func TestContactPolicyMirrorAt_WaitingExemptionRemoval(t *testing.T) {
	now := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	week := now.Add(RetentionPendingDelay)
	settings := &DataRetentionSettings{
		Policy:  &DataRetentionPolicy{Categories: map[RetentionCategory]int{RetentionInactiveContacts: 365}, ExemptListIDs: []string{"vip"}},
		Pending: &PendingRetentionChange{EffectiveAt: week, Policy: &DataRetentionPolicy{Categories: map[RetentionCategory]int{RetentionInactiveContacts: 365}}},
	}
	mirror := ContactPolicyMirrorAt(settings, nil, now)
	assert.Equal(t, 365, mirror.Days)
	assert.Equal(t, []string{"vip"}, mirror.ExemptListIDs)
	assert.Equal(t, 365, mirror.PendingDays)
	require.NotNil(t, mirror.PendingAt)
	assert.Empty(t, mirror.PendingExemptListIDs)
}
