package domain

import (
	"time"

	"github.com/google/uuid"
)

// DefaultWebFilters returns the out-of-the-box channel attribution rule set
// (ported from Staminads' default-filters fixture, plus one: 40 rules). Each
// call generates fresh rule ids so every workspace owns unique identifiers, and
// stamps the computed version hash on every rule.
//
// Changing this set needs NO migration and no hash update: FiltersVersion is
// always recomputed, never stored as a literal, and existing workspaces keep the
// rules already in their settings — defaults are seeded only at workspace
// creation. So an edit here changes what NEW workspaces start with, nothing else.
//
// Every rule matching on utm_id_from must correspond to an id the browser SDK
// actually captures, and vice versa. The two lists live in different languages
// and different CI workflows; web_analytics_clickid_contract_test.go is what
// keeps them honest.
func DefaultWebFilters() []WebFilter {
	now := time.Now().UTC().Format(time.RFC3339)
	order := 0

	utmConditions := func(source, medium string) []WebFilterCondition {
		return []WebFilterCondition{
			{Field: "utm_source", Operator: WebFilterOpRegex, Value: "^" + source + "$"},
			{Field: "utm_medium", Operator: WebFilterOpRegex, Value: "^" + medium + "$"},
		}
	}
	clickID := func(value, operator string) []WebFilterCondition {
		return []WebFilterCondition{{Field: "utm_id_from", Operator: operator, Value: value}}
	}
	referrerContains := func(domain string) []WebFilterCondition {
		return []WebFilterCondition{{Field: "referrer_domain", Operator: WebFilterOpContains, Value: domain}}
	}
	referrerRegex := func(pattern string) []WebFilterCondition {
		return []WebFilterCondition{{Field: "referrer_domain", Operator: WebFilterOpRegex, Value: pattern}}
	}

	channelFilter := func(name string, conditions []WebFilterCondition, channelGroup, channel string, priority int) WebFilter {
		order++
		return WebFilter{
			ID:         uuid.NewString(),
			Name:       name,
			Priority:   priority,
			Order:      order,
			Tags:       []string{"channel"},
			Conditions: conditions,
			Operations: []WebFilterOperation{
				{Dimension: "channel_group", Action: WebFilterActionSetValue, Value: channelGroup},
				{Dimension: "channel", Action: WebFilterActionSetValue, Value: channel},
			},
			Enabled:   true,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	filters := []WebFilter{
		// Paid channels via ad click ids (priority 900-831).
		channelFilter("Google Ads (Click ID)", clickID("^(gclid|gbraid|wbraid)$", WebFilterOpRegex), "search-paid", "google-ads", 900),
		channelFilter("Google Display (Click ID)", clickID("dclid", WebFilterOpEquals), "display-banner", "google-ads", 895),
		channelFilter("Facebook Ads (Click ID)", clickID("fbclid", WebFilterOpEquals), "social-paid", "facebook-ads", 890),
		channelFilter("Microsoft Ads (Click ID)", clickID("msclkid", WebFilterOpEquals), "search-paid", "microsoft-ads", 880),
		channelFilter("TikTok Ads (Click ID)", clickID("ttclid", WebFilterOpEquals), "social-paid", "tiktok-ads", 870),
		channelFilter("Pinterest Ads (Click ID)", clickID("epik", WebFilterOpEquals), "social-paid", "pinterest-ads", 860),
		channelFilter("LinkedIn Ads (Click ID)", clickID("li_fat_id", WebFilterOpEquals), "social-paid", "linkedin-ads", 850),
		channelFilter("Twitter Ads (Click ID)", clickID("twclid", WebFilterOpEquals), "social-paid", "twitter-ads", 840),
		channelFilter("Snapchat Ads (Click ID)", clickID("ScCid", WebFilterOpEquals), "social-paid", "snapchat-ads", 835),
		channelFilter("Reddit Ads (Click ID)", clickID("rdt_cid", WebFilterOpEquals), "social-paid", "reddit-ads", 833),
		channelFilter("Quora Ads (Click ID)", clickID("qclid", WebFilterOpEquals), "social-paid", "quora-ads", 831),

		// Paid channels via UTM (priority 830-770).
		channelFilter("Google Ads (UTM)", utmConditions("google", "(cpc|ppc|paid)"), "search-paid", "google-ads", 830),
		channelFilter("Microsoft Ads (UTM)", utmConditions("(bing|microsoft)", "(cpc|ppc|paid)"), "search-paid", "microsoft-ads", 820),
		channelFilter("Facebook Ads (UTM)", utmConditions("facebook", "(cpc|paid|paidsocial)"), "social-paid", "facebook-ads", 810),
		channelFilter("Instagram Ads (UTM)", utmConditions("instagram", "(cpc|paid|paidsocial)"), "social-paid", "instagram-ads", 800),
		channelFilter("LinkedIn Ads (UTM)", utmConditions("linkedin", "(cpc|paid|paidsocial)"), "social-paid", "linkedin-ads", 790),
		channelFilter("TikTok Ads (UTM)", utmConditions("tiktok", "(cpc|paid|paidsocial)"), "social-paid", "tiktok-ads", 780),
		channelFilter("YouTube Ads (UTM)", utmConditions("youtube", "(cpc|cpv|paid)"), "video-paid", "youtube-ads", 770),

		// Paid via referrer: Google Ads display network (priority 760).
		channelFilter("Google Ads (Referrer)", referrerContains("googleadservices"), "display-banner", "google-ads", 760),

		// Direct traffic (priority 750-740).
		channelFilter("Direct (UTM)", utmConditions("direct", "none"), "direct", "direct", 750),
		// Direct means unattributed, not merely referrer-less. Mail clients,
		// native apps, PDF readers, QR codes and most display networks all
		// strip the referrer, so a session carrying campaign parameters is
		// tagged traffic whose rule is simply missing. Claiming it as direct
		// hides that; letting it fall through to "not-mapped" surfaces it in
		// the filters editor, where it can be classified once and for all.
		channelFilter("Direct Traffic", []WebFilterCondition{
			{Field: "is_direct", Operator: WebFilterOpEquals, Value: "true"},
			{Field: "utm_source", Operator: WebFilterOpIsEmpty},
			{Field: "utm_medium", Operator: WebFilterOpIsEmpty},
			{Field: "utm_campaign", Operator: WebFilterOpIsEmpty},
			{Field: "utm_id_from", Operator: WebFilterOpIsEmpty},
		}, "direct", "direct", 740),

		// Organic search engines (priority 705-650). The Android Google Search
		// app referrer must outrank the generic google match.
		channelFilter("Google Android App", referrerContains("com.google.android"), "search-organic", "google-organic", 705),
		channelFilter("Google Organic", referrerContains("google"), "search-organic", "google-organic", 700),
		channelFilter("Bing Organic", referrerContains("bing"), "search-organic", "bing-organic", 690),
		channelFilter("Yahoo Organic", referrerContains("yahoo"), "search-organic", "yahoo-organic", 680),
		channelFilter("DuckDuckGo Organic", referrerContains("duckduckgo"), "search-organic", "duckduckgo-organic", 670),
		channelFilter("Baidu Organic", referrerContains("baidu"), "search-organic", "baidu-organic", 660),
		channelFilter("Yandex Organic", referrerContains("yandex"), "search-organic", "yandex-organic", 650),

		// Social organic (priority 600-510).
		channelFilter("Facebook Organic", referrerContains("facebook"), "social-organic", "facebook-organic", 600),
		channelFilter("Instagram Organic", referrerContains("instagram"), "social-organic", "instagram-organic", 590),
		channelFilter("Twitter/X Organic", referrerRegex(`(twitter\.com|x\.com|t\.co)`), "social-organic", "twitter-organic", 580),
		channelFilter("LinkedIn Organic", referrerContains("linkedin"), "social-organic", "linkedin-organic", 570),
		channelFilter("YouTube Organic", referrerContains("youtube"), "social-organic", "youtube-organic", 560),
		channelFilter("TikTok Organic", referrerContains("tiktok"), "social-organic", "tiktok-organic", 550),
		channelFilter("Pinterest Organic", referrerContains("pinterest"), "social-organic", "pinterest-organic", 540),
		channelFilter("Reddit Organic", referrerContains("reddit"), "social-organic", "reddit-organic", 530),
		channelFilter("Snapchat Organic", referrerContains("snapchat"), "social-organic", "snapchat-organic", 520),
		channelFilter("Quora Organic", referrerContains("quora"), "social-organic", "quora-organic", 510),
	}

	// Email marketing (priority 745).
	//
	// It has to outrank "Direct Traffic" (740): mail clients strip the
	// referrer, so every newsletter click arrives with is_direct set and would
	// otherwise be attributed to Direct — silently emptying the email channel
	// of the one platform whose whole point is sending email.
	order++
	filters = append(filters, WebFilter{
		ID:       uuid.NewString(),
		Name:     "Email",
		Priority: 745,
		Order:    order,
		Tags:     []string{"channel"},
		Conditions: []WebFilterCondition{
			{Field: "utm_medium", Operator: WebFilterOpRegex, Value: "^email$"},
		},
		Operations: []WebFilterOperation{
			{Dimension: "channel_group", Action: WebFilterActionSetValue, Value: "email"},
			{Dimension: "channel", Action: WebFilterActionSetValue, Value: "email"},
		},
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	})

	// Fallback: everything unmatched lands in "not-mapped" (priority 10).
	order++
	filters = append(filters, WebFilter{
		ID:         uuid.NewString(),
		Name:       "Default Channel",
		Priority:   10,
		Order:      order,
		Tags:       []string{"default"},
		Conditions: []WebFilterCondition{}, // always matches
		Operations: []WebFilterOperation{
			{Dimension: "channel_group", Action: WebFilterActionSetDefaultValue, Value: "not-mapped"},
			{Dimension: "channel", Action: WebFilterActionSetDefaultValue, Value: "not-mapped"},
		},
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	})

	version := ComputeWebFiltersVersion(filters)
	for i := range filters {
		filters[i].Version = version
	}
	return filters
}
