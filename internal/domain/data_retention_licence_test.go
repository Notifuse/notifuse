package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetentionPolicyNeedsLicence(t *testing.T) {
	defaults := BuiltinRetentionDefaults()
	inForce := &DataRetentionSettings{Policy: &DataRetentionPolicy{
		Categories:           map[RetentionCategory]int{RetentionInactiveContacts: 365, RetentionMessageHistory: 400},
		ExemptListIDs:        []string{"vip"},
		ExemptSegmentIDs:     []string{"buyers"},
		EngagementEventNames: []string{"purchase", "login"},
	}}
	same := func() *DataRetentionPolicy { return inForce.Policy.Clone() }
	withPending := &DataRetentionSettings{Policy: inForce.Policy, Pending: &PendingRetentionChange{Policy: &DataRetentionPolicy{
		Categories:           map[RetentionCategory]int{RetentionInactiveContacts: 200, RetentionMessageHistory: 400},
		ExemptListIDs:        []string{"vip", "staff"},
		EngagementEventNames: []string{"purchase"},
	}}}

	cases := []struct {
		name   string
		stored *DataRetentionSettings
		policy func() *DataRetentionPolicy
		want   bool
	}{
		{"saving the policy in force again", inForce, same, false},
		{"switching a category off", inForce, func() *DataRetentionPolicy {
			p := same()
			p.Categories[RetentionMessageHistory] = 0
			return p
		}, false},
		{"removing an exemption", inForce, func() *DataRetentionPolicy {
			p := same()
			p.ExemptListIDs, p.ExemptSegmentIDs = nil, nil
			return p
		}, false},
		{"a built-in default (the technical logs' 7 days)", inForce, func() *DataRetentionPolicy {
			p := same()
			p.Categories[RetentionWebhookDeliveries] = 7
			return p
		}, false},
		{"switching a category on", inForce, func() *DataRetentionPolicy {
			p := same()
			p.Categories[RetentionWebAnalytics] = 90
			return p
		}, true},
		{"shortening a period", inForce, func() *DataRetentionPolicy {
			p := same()
			p.Categories[RetentionInactiveContacts] = 300
			return p
		}, true},
		{"lengthening a period", inForce, func() *DataRetentionPolicy {
			p := same()
			p.Categories[RetentionInactiveContacts] = 500
			return p
		}, true},
		{"adding an exempt list", inForce, func() *DataRetentionPolicy {
			p := same()
			p.ExemptListIDs = append(p.ExemptListIDs, "new")
			return p
		}, true},
		{"adding an exempt segment", inForce, func() *DataRetentionPolicy {
			p := same()
			p.ExemptSegmentIDs = append(p.ExemptSegmentIDs, "new")
			return p
		}, true},
		{"adding an engagement event name", inForce, func() *DataRetentionPolicy {
			p := same()
			p.EngagementEventNames = append(p.EngagementEventNames, "signup")
			return p
		}, true},
		{"removing an engagement event name", inForce, func() *DataRetentionPolicy {
			p := same()
			p.EngagementEventNames = []string{"purchase"}
			return p
		}, true},
		{"the event names in another order", inForce, func() *DataRetentionPolicy {
			p := same()
			p.EngagementEventNames = []string{"login", "purchase"}
			return p
		}, false},
		{"the change that waits, saved again", withPending, func() *DataRetentionPolicy { return withPending.Pending.Policy.Clone() }, false},
		{"going back to the policy in force while a change waits", withPending, same, false},
		{"a period that is neither in force nor waiting", withPending, func() *DataRetentionPolicy {
			p := same()
			p.Categories[RetentionInactiveContacts] = 250
			return p
		}, true},
		{"no policy yet, following the defaults", nil, func() *DataRetentionPolicy { return nil }, false},
		{"no policy yet, a first category", nil, func() *DataRetentionPolicy {
			return &DataRetentionPolicy{Categories: map[RetentionCategory]int{RetentionInactiveContacts: 365}}
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, RetentionPolicyNeedsLicence(tc.stored, tc.policy(), defaults))
		})
	}

	t.Run("a technical log back on its built-in 7 days after it was switched off", func(t *testing.T) {
		off := &DataRetentionSettings{Policy: &DataRetentionPolicy{Categories: map[RetentionCategory]int{RetentionWebhookDeliveries: 0}}}
		back := &DataRetentionPolicy{Categories: map[RetentionCategory]int{RetentionWebhookDeliveries: 7}}
		assert.False(t, RetentionPolicyNeedsLicence(off, back, defaults))
	})

	t.Run("a period root set as the deployment default", func(t *testing.T) {
		rootDefaults := BuiltinRetentionDefaults()
		rootDefaults[RetentionWebAnalytics] = 90
		policy := &DataRetentionPolicy{Categories: map[RetentionCategory]int{RetentionWebAnalytics: 90}}
		assert.False(t, RetentionPolicyNeedsLicence(nil, policy, rootDefaults))
	})
}

func TestRetentionDefaultsNeedLicence(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	full := func(overrides map[RetentionCategory]int) map[RetentionCategory]int {
		out := BuiltinRetentionDefaults()
		for k, v := range overrides {
			out[k] = v
		}
		return out
	}
	stored := &RetentionDefaults{
		Categories: map[RetentionCategory]int{RetentionMessageHistory: 365},
		Pending: &PendingRetentionDefaults{
			Categories:  full(map[RetentionCategory]int{RetentionMessageHistory: 200}),
			EffectiveAt: now.Add(48 * time.Hour),
		},
	}

	assert.False(t, RetentionDefaultsNeedLicence(nil, BuiltinRetentionDefaults(), now), "the built-in defaults")
	assert.False(t, RetentionDefaultsNeedLicence(nil, full(map[RetentionCategory]int{RetentionWebhookDeliveries: 0}), now), "a technical log switched off")
	assert.True(t, RetentionDefaultsNeedLicence(nil, full(map[RetentionCategory]int{RetentionMessageHistory: 365}), now), "a category switched on")
	assert.False(t, RetentionDefaultsNeedLicence(stored, full(map[RetentionCategory]int{RetentionMessageHistory: 365}), now), "the default in force")
	assert.False(t, RetentionDefaultsNeedLicence(stored, full(map[RetentionCategory]int{RetentionMessageHistory: 200}), now), "the default that waits")
	assert.True(t, RetentionDefaultsNeedLicence(stored, full(map[RetentionCategory]int{RetentionMessageHistory: 300}), now), "a new period")
	assert.False(t, RetentionDefaultsNeedLicence(stored, BuiltinRetentionDefaults(), now), "every category back to built-in")
}
