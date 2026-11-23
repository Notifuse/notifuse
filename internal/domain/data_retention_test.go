package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRetentionDays(t *testing.T) {
	for _, spec := range RetentionCategorySpecs {
		t.Run(string(spec.Category), func(t *testing.T) {
			assert.NoError(t, ValidateRetentionDays(spec.Category, 0), "0 switches it off")
			assert.NoError(t, ValidateRetentionDays(spec.Category, spec.MinDays))
			assert.NoError(t, ValidateRetentionDays(spec.Category, RetentionMaxDays))
			assert.Error(t, ValidateRetentionDays(spec.Category, spec.MinDays-1))
			assert.Error(t, ValidateRetentionDays(spec.Category, RetentionMaxDays+1))
			assert.Error(t, ValidateRetentionDays(spec.Category, -1))
			assert.LessOrEqual(t, spec.MinDays, spec.SuggestedDays)
		})
	}
	assert.Error(t, ValidateRetentionDays("everything", 30))
}

// The personal-data categories are opt-in (P4); only the two technical logs keep their 7 days.
func TestBuiltinRetentionDefaults(t *testing.T) {
	defaults := BuiltinRetentionDefaults()
	for _, spec := range RetentionCategorySpecs {
		switch spec.Category {
		case RetentionWebhookDeliveries, RetentionFailedEmails:
			assert.Equal(t, 7, defaults[spec.Category], spec.Category)
		default:
			assert.Zero(t, defaults[spec.Category], spec.Category)
		}
	}
	assert.Equal(t, RetentionInactiveContacts, RetentionCategorySpecs[0].Category, "contacts run first")
}

func TestDataRetentionPolicy_Validate(t *testing.T) {
	assert.NoError(t, (*DataRetentionPolicy)(nil).Validate())
	assert.NoError(t, (&DataRetentionPolicy{Categories: map[RetentionCategory]int{RetentionInactiveContacts: 1095}}).Validate())
	assert.Error(t, (&DataRetentionPolicy{Categories: map[RetentionCategory]int{RetentionInactiveContacts: 30}}).Validate())
	assert.Error(t, (&DataRetentionPolicy{Categories: map[RetentionCategory]int{"logs": 30}}).Validate())
	assert.Error(t, (&DataRetentionPolicy{ExemptListIDs: []string{" "}}).Validate())
	assert.Error(t, (&DataRetentionPolicy{ExemptSegmentIDs: []string{""}}).Validate())
	assert.Error(t, (&DataRetentionPolicy{EngagementEventNames: []string{""}}).Validate())
}

func TestResolve(t *testing.T) {
	defaults := BuiltinRetentionDefaults()
	defaults[RetentionMessageHistory] = 730

	t.Run("no policy follows the defaults", func(t *testing.T) {
		eff := Resolve(nil, defaults)
		assert.Equal(t, 730, eff.Days[RetentionMessageHistory])
		assert.Equal(t, 7, eff.Days[RetentionWebhookDeliveries])
		assert.Zero(t, eff.Days[RetentionInactiveContacts])
	})

	t.Run("a category the policy names overrides the default, 0 included", func(t *testing.T) {
		eff := Resolve(&DataRetentionPolicy{
			Categories:    map[RetentionCategory]int{RetentionMessageHistory: 0, RetentionInactiveContacts: 1095},
			ExemptListIDs: []string{"vip"},
		}, defaults)
		assert.Zero(t, eff.Days[RetentionMessageHistory])
		assert.Equal(t, 1095, eff.Days[RetentionInactiveContacts])
		assert.Equal(t, 7, eff.Days[RetentionFailedEmails])
		assert.Equal(t, []string{"vip"}, eff.ExemptListIDs)
	})
}

func TestEffectiveRetention_DeletesMore(t *testing.T) {
	base := func() EffectiveRetention {
		return Resolve(&DataRetentionPolicy{
			Categories:           map[RetentionCategory]int{RetentionInactiveContacts: 730},
			ExemptListIDs:        []string{"vip"},
			ExemptSegmentIDs:     []string{"customers"},
			EngagementEventNames: []string{"purchase"},
		}, BuiltinRetentionDefaults())
	}
	cases := []struct {
		name   string
		change func(*EffectiveRetention)
		more   bool
	}{
		{"nothing", func(*EffectiveRetention) {}, false},
		{"switch a category on", func(e *EffectiveRetention) { e.Days[RetentionMessageHistory] = 365 }, true},
		{"shorten", func(e *EffectiveRetention) { e.Days[RetentionInactiveContacts] = 365 }, true},
		{"lengthen", func(e *EffectiveRetention) { e.Days[RetentionInactiveContacts] = 1095 }, false},
		{"switch off", func(e *EffectiveRetention) { e.Days[RetentionInactiveContacts] = 0 }, false},
		{"remove a list exemption", func(e *EffectiveRetention) { e.ExemptListIDs = nil }, true},
		{"remove a segment exemption", func(e *EffectiveRetention) { e.ExemptSegmentIDs = nil }, true},
		{"add an exemption", func(e *EffectiveRetention) { e.ExemptListIDs = append(e.ExemptListIDs, "partners") }, false},
		{"remove an engagement event", func(e *EffectiveRetention) { e.EngagementEventNames = nil }, true},
		{"add an engagement event", func(e *EffectiveRetention) { e.EngagementEventNames = append(e.EngagementEventNames, "login") }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := base()
			tc.change(&next)
			assert.Equal(t, tc.more, next.DeletesMore(base()))
		})
	}
}

func TestSafest(t *testing.T) {
	current := Resolve(&DataRetentionPolicy{
		Categories:    map[RetentionCategory]int{RetentionInactiveContacts: 365, RetentionContactTimeline: 400},
		ExemptListIDs: []string{"vip"},
	}, BuiltinRetentionDefaults())
	requested := Resolve(&DataRetentionPolicy{
		Categories:           map[RetentionCategory]int{RetentionInactiveContacts: 730, RetentionMessageHistory: 90, RetentionContactTimeline: 0},
		ExemptListIDs:        []string{"partners"},
		EngagementEventNames: []string{"purchase"},
	}, BuiltinRetentionDefaults())

	got := Safest(current, requested)
	assert.Equal(t, 730, got.Days[RetentionInactiveContacts], "a longer period applies at once")
	assert.Zero(t, got.Days[RetentionMessageHistory], "a category switched on waits")
	assert.Zero(t, got.Days[RetentionContactTimeline], "switching off applies at once")
	assert.Equal(t, 7, got.Days[RetentionWebhookDeliveries])
	assert.Equal(t, []string{"partners", "vip"}, got.ExemptListIDs, "an exemption added applies at once, one removed waits")
	assert.Equal(t, []string{"purchase"}, got.EngagementEventNames)
	assert.False(t, got.DeletesMore(current))
	assert.Equal(t, got, Safest(requested, current))
}

func TestRetentionDefaults(t *testing.T) {
	now := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	var none *RetentionDefaults
	assert.Equal(t, BuiltinRetentionDefaults(), none.ResolvedDefaults(now))

	d := &RetentionDefaults{
		Categories: map[RetentionCategory]int{RetentionMessageHistory: 730},
		Pending: &PendingRetentionDefaults{
			Categories:  map[RetentionCategory]int{RetentionMessageHistory: 365},
			EffectiveAt: now.Add(RetentionPendingDelay),
		},
	}
	assert.Equal(t, 730, d.ResolvedDefaults(now)[RetentionMessageHistory])
	pending, at := d.PendingDefaults(now)
	require.NotNil(t, at)
	assert.Equal(t, 365, pending[RetentionMessageHistory])
	assert.Equal(t, 7, pending[RetentionWebhookDeliveries])

	due := now.Add(RetentionPendingDelay)
	assert.Equal(t, 365, d.ResolvedDefaults(due)[RetentionMessageHistory], "due on its date")
	pending, at = d.PendingDefaults(due)
	assert.Nil(t, pending)
	assert.Nil(t, at)
}

func TestDataRetentionSettings_Due(t *testing.T) {
	now := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	target := &DataRetentionPolicy{Categories: map[RetentionCategory]int{RetentionInactiveContacts: 365}}
	s := &DataRetentionSettings{Pending: &PendingRetentionChange{Policy: target, EffectiveAt: now.Add(time.Hour)}}
	assert.False(t, s.Due(now))
	assert.NotNil(t, s.Pending)
	assert.True(t, s.Due(now.Add(time.Hour)))
	assert.Same(t, target, s.Policy)
	assert.Nil(t, s.Pending)
	assert.False(t, (*DataRetentionSettings)(nil).Due(now))
}

func TestDataRetentionPolicy_NormalizeAndJSON(t *testing.T) {
	p := &DataRetentionPolicy{
		Categories:    map[RetentionCategory]int{RetentionInactiveContacts: 1095},
		ExemptListIDs: []string{"b", " a", "b"},
	}
	p.Normalize()
	assert.Equal(t, []string{"a", "b"}, p.ExemptListIDs)
	assert.Nil(t, p.ExemptSegmentIDs)

	raw, err := json.Marshal(p)
	require.NoError(t, err)
	assert.JSONEq(t, `{"categories":{"inactive_contacts":1095},"exempt_list_ids":["a","b"]}`, string(raw))
	var back DataRetentionPolicy
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, p, &back)

	clone := p.Clone()
	clone.Categories[RetentionInactiveContacts] = 0
	clone.ExemptListIDs[0] = "z"
	assert.Equal(t, 1095, p.Categories[RetentionInactiveContacts], "a deep copy")
	assert.Equal(t, "a", p.ExemptListIDs[0])
}

func TestRetentionInForce(t *testing.T) {
	now := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	week := now.Add(RetentionPendingDelay)
	policy := func(days map[RetentionCategory]int, exempt ...string) *DataRetentionPolicy {
		return &DataRetentionPolicy{Categories: days, ExemptListIDs: exempt}
	}

	t.Run("nothing set: the built-in defaults, nothing waits", func(t *testing.T) {
		inForce, pending, at := RetentionInForce(nil, nil, now)
		assert.Equal(t, 7, inForce.Days[RetentionWebhookDeliveries])
		assert.Zero(t, inForce.Days[RetentionInactiveContacts])
		assert.Nil(t, pending)
		assert.Nil(t, at)
	})

	t.Run("a waiting change: what deletes less applies, what deletes more waits", func(t *testing.T) {
		settings := &DataRetentionSettings{
			Policy: policy(map[RetentionCategory]int{RetentionInactiveContacts: 365, RetentionContactTimeline: 365}, "vip"),
			Pending: &PendingRetentionChange{EffectiveAt: week,
				Policy: policy(map[RetentionCategory]int{RetentionInactiveContacts: 730, RetentionMessageHistory: 90}, "partners")},
		}
		inForce, pending, at := RetentionInForce(settings, nil, now)
		assert.Equal(t, 730, inForce.Days[RetentionInactiveContacts], "lengthened at once")
		assert.Zero(t, inForce.Days[RetentionContactTimeline], "switched off at once")
		assert.Zero(t, inForce.Days[RetentionMessageHistory], "switched on later")
		assert.Equal(t, []string{"partners", "vip"}, inForce.ExemptListIDs)
		require.NotNil(t, pending)
		assert.Equal(t, 90, pending.Days[RetentionMessageHistory])
		assert.Equal(t, []string{"partners"}, pending.ExemptListIDs)
		assert.Equal(t, week, *at)

		inForce, pending, _ = RetentionInForce(settings, nil, week)
		assert.Equal(t, 90, inForce.Days[RetentionMessageHistory], "due on its date")
		assert.Equal(t, []string{"partners"}, inForce.ExemptListIDs)
		assert.Nil(t, pending)
	})

	t.Run("a waiting change of the defaults reaches a workspace that inherits them", func(t *testing.T) {
		defaults := &RetentionDefaults{
			Categories: map[RetentionCategory]int{RetentionMessageHistory: 730},
			Pending:    &PendingRetentionDefaults{Categories: map[RetentionCategory]int{RetentionMessageHistory: 365}, EffectiveAt: week},
		}
		inForce, pending, at := RetentionInForce(nil, defaults, now)
		assert.Equal(t, 730, inForce.Days[RetentionMessageHistory])
		require.NotNil(t, pending)
		assert.Equal(t, 365, pending.Days[RetentionMessageHistory])
		assert.Equal(t, week, *at)

		// A workspace that set its own period is not affected.
		own := &DataRetentionSettings{Policy: policy(map[RetentionCategory]int{RetentionMessageHistory: 1000})}
		inForce, pending, _ = RetentionInForce(own, defaults, now)
		assert.Equal(t, 1000, inForce.Days[RetentionMessageHistory])
		require.NotNil(t, pending)
		assert.Equal(t, 1000, pending.Days[RetentionMessageHistory])
	})
}
