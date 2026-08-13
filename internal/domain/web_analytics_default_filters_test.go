package domain

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultWebFilters(t *testing.T) {
	filters := DefaultWebFilters()

	t.Run("exactly 40 rules, all valid and enabled", func(t *testing.T) {
		require.Len(t, filters, 40)
		for _, f := range filters {
			assert.NoError(t, f.Validate(), f.Name)
			assert.True(t, f.Enabled, f.Name)
		}
	})

	t.Run("unique ids, shared version hash, sequential order", func(t *testing.T) {
		ids := map[string]bool{}
		for i, f := range filters {
			assert.False(t, ids[f.ID], "duplicate id for %s", f.Name)
			ids[f.ID] = true
			assert.Equal(t, filters[0].Version, f.Version)
			assert.Equal(t, i+1, f.Order)
		}
		assert.Len(t, filters[0].Version, 8)
		assert.Equal(t, ComputeWebFiltersVersion(filters), filters[0].Version)
	})

	t.Run("each workspace gets fresh ids but identical semantics modulo ids", func(t *testing.T) {
		other := DefaultWebFilters()
		assert.NotEqual(t, filters[0].ID, other[0].ID)
		assert.Equal(t, len(filters), len(other))
	})

	t.Run("all regex conditions compile under RE2", func(t *testing.T) {
		for _, f := range filters {
			for _, c := range f.Conditions {
				if c.Operator == WebFilterOpRegex {
					_, err := regexp.Compile(c.Value)
					assert.NoError(t, err, "%s: %s", f.Name, c.Value)
				}
			}
		}
	})

	channelFor := func(fields map[string]string) (string, string) {
		result := EvaluateWebFilters(filters, fields)
		channel, group := "", ""
		if v := result["channel"]; v != nil {
			channel = *v
		}
		if v := result["channel_group"]; v != nil {
			group = *v
		}
		return channel, group
	}

	t.Run("gclid click id maps to google-ads / search-paid", func(t *testing.T) {
		channel, group := channelFor(map[string]string{"utm_id_from": "gclid"})
		assert.Equal(t, "google-ads", channel)
		assert.Equal(t, "search-paid", group)
	})

	t.Run("google referrer maps to organic search", func(t *testing.T) {
		channel, group := channelFor(map[string]string{"referrer_domain": "www.google.com"})
		assert.Equal(t, "google-organic", channel)
		assert.Equal(t, "search-organic", group)
	})

	t.Run("direct traffic", func(t *testing.T) {
		channel, group := channelFor(map[string]string{"is_direct": "true"})
		assert.Equal(t, "direct", channel)
		assert.Equal(t, "direct", group)
	})

	t.Run("email utm_medium", func(t *testing.T) {
		channel, group := channelFor(map[string]string{"utm_medium": "email"})
		assert.Equal(t, "email", channel)
		assert.Equal(t, "email", group)
	})

	t.Run("unmatched traffic falls back to not-mapped", func(t *testing.T) {
		channel, group := channelFor(map[string]string{"referrer_domain": "some-blog.example"})
		assert.Equal(t, "not-mapped", channel)
		assert.Equal(t, "not-mapped", group)
	})

	t.Run("click id outranks utm and referrer signals", func(t *testing.T) {
		channel, group := channelFor(map[string]string{
			"utm_id_from":     "fbclid",
			"utm_source":      "google",
			"utm_medium":      "cpc",
			"referrer_domain": "www.google.com",
		})
		assert.Equal(t, "facebook-ads", channel)
		assert.Equal(t, "social-paid", group)
	})

	t.Run("android google app referrer outranks generic google", func(t *testing.T) {
		channel, _ := channelFor(map[string]string{"referrer_domain": "com.google.android.googlequicksearchbox"})
		assert.Equal(t, "google-organic", channel)
	})
}

func TestDefaultWebFiltersEmailBeatsDirect(t *testing.T) {
	// A mail client strips the referrer, so a newsletter click looks direct.
	// If the Direct rule wins, the email channel is permanently empty.
	fields := map[string]string{
		"utm_source": "newsletter",
		"utm_medium": "email",
		"is_direct":  "true",
	}

	result := EvaluateWebFilters(DefaultWebFilters(), fields)
	require.NotNil(t, result["channel_group"])
	assert.Equal(t, "email", *result["channel_group"])
	assert.Equal(t, "email", *result["channel"])
}

func TestDefaultWebFiltersDirectStaysDirect(t *testing.T) {
	// Without a campaign, a referrer-less visit really is direct.
	result := EvaluateWebFilters(DefaultWebFilters(), map[string]string{"is_direct": "true"})
	require.NotNil(t, result["channel_group"])
	assert.Equal(t, "direct", *result["channel_group"])
}

func TestDefaultWebFiltersDirectRequiresNoCampaign(t *testing.T) {
	group := func(fields map[string]string) string {
		fields["is_direct"] = "true"
		result := EvaluateWebFilters(DefaultWebFilters(), fields)
		if result["channel_group"] == nil {
			return ""
		}
		return *result["channel_group"]
	}

	t.Run("a bare referrer-less visit is direct", func(t *testing.T) {
		assert.Equal(t, "direct", group(map[string]string{}))
	})

	t.Run("campaign parameters take it out of direct even with no rule to catch it", func(t *testing.T) {
		// Mail clients, native apps, QR codes and display networks all strip
		// the referrer. Calling that traffic direct is a confident lie;
		// "not-mapped" is a prompt to write the missing rule.
		for _, fields := range []map[string]string{
			{"utm_source": "podcast", "utm_medium": "audio"},
			{"utm_medium": "print"},
			{"utm_campaign": "spring-launch"},
			{"utm_id_from": "some_unknown_click_id"},
		} {
			assert.Equal(t, "not-mapped", group(fields), "fields %v", fields)
		}
	})

	t.Run("a recognised campaign still wins over not-mapped", func(t *testing.T) {
		assert.Equal(t, "email", group(map[string]string{"utm_medium": "email"}))
		assert.Equal(t, "search-paid", group(map[string]string{"utm_id_from": "gclid"}))
	})

	t.Run("traffic deliberately tagged as direct stays direct", func(t *testing.T) {
		// The "Direct (UTM)" rule sits above, for anyone who tags it explicitly.
		assert.Equal(t, "direct", group(map[string]string{"utm_source": "direct", "utm_medium": "none"}))
	})
}
