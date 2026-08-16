package service

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The weight totals are the demo's contract with itself: the generator draws by
// weight, so a row added or retuned without updating its neighbours silently
// reshapes every chart in the demo workspace.
func TestDemoWebAnalyticsCatalogWeights(t *testing.T) {
	t.Run("catalogue sizes", func(t *testing.T) {
		assert.Len(t, demoPages, 34)
		assert.Len(t, demoReferrers, 30)
		assert.Len(t, demoCampaigns, 21)
		assert.Len(t, demoDevices, 17)
		assert.Len(t, demoGeos, 13)
	})

	t.Run("weight totals", func(t *testing.T) {
		pageWeight := 0
		for _, page := range demoPages {
			assert.Positive(t, page.Weight, "page %s must carry a weight", page.Path)
			pageWeight += page.Weight
		}
		assert.Equal(t, 131, pageWeight)

		referrerWeight := 0
		for _, referrer := range demoReferrers {
			assert.Positive(t, referrer.Weight, "referrer %q must carry a weight", referrer.Domain)
			referrerWeight += referrer.Weight
		}
		assert.Equal(t, 109, referrerWeight)

		campaignWeight := 0
		for _, campaign := range demoCampaigns {
			assert.Positive(t, campaign.Weight, "campaign %s must carry a weight", campaign.Campaign)
			campaignWeight += campaign.Weight
		}
		assert.Equal(t, 63, campaignWeight)

		deviceWeight := 0
		for _, device := range demoDevices {
			assert.Positive(t, device.Weight, "device %s/%s must carry a weight", device.OS, device.Browser)
			deviceWeight += device.Weight
		}
		assert.Equal(t, 109, deviceWeight)

		geoWeight := 0
		for _, geo := range demoGeos {
			assert.Positive(t, geo.Weight, "geo %s must carry a weight", geo.City)
			geoWeight += geo.Weight
		}
		assert.Equal(t, 100, geoWeight)
	})

	t.Run("hour multipliers", func(t *testing.T) {
		total := 0.0
		for hour, multiplier := range demoHourMultipliers {
			assert.Positive(t, multiplier, "hour %d must stay positive", hour)
			total += multiplier
		}
		assert.InDelta(t, 22.45, total, 1e-9)
	})

	t.Run("day of week weights are indexed by time.Weekday", func(t *testing.T) {
		assert.Equal(t, 0.6, demoDayOfWeekWeights[time.Sunday])
		assert.Equal(t, 1.0, demoDayOfWeekWeights[time.Monday])
		assert.Equal(t, 1.1, demoDayOfWeekWeights[time.Tuesday])
		assert.Equal(t, 1.05, demoDayOfWeekWeights[time.Wednesday])
		assert.Equal(t, 1.0, demoDayOfWeekWeights[time.Thursday])
		assert.Equal(t, 0.9, demoDayOfWeekWeights[time.Friday])
		assert.Equal(t, 0.7, demoDayOfWeekWeights[time.Saturday])
	})

	t.Run("no-UTM weight dominates the campaign mix", func(t *testing.T) {
		assert.Equal(t, 60, demoNoUTMWeight)
	})
}

func TestDemoWebAnalyticsCatalogLaunchSubsets(t *testing.T) {
	t.Run("launch pages", func(t *testing.T) {
		paths := make([]string, 0, len(demoLaunchPages))
		weight := 0
		for _, page := range demoLaunchPages {
			paths = append(paths, page.Path)
			weight += page.Weight
		}
		assert.Equal(t, []string{"/iphone/", "/iphone-17-pro/", "/iphone-air/", "/iphone-17/"}, paths)
		assert.Equal(t, 33, weight)
	})

	t.Run("tech news referrers", func(t *testing.T) {
		weight := 0
		for _, referrer := range demoTechNewsReferrers {
			assert.Equal(t, "tech-news", referrer.Category)
			weight += referrer.Weight
		}
		assert.Len(t, demoTechNewsReferrers, 7)
		assert.Equal(t, 13, weight)
	})

	t.Run("launch campaigns", func(t *testing.T) {
		weight := 0
		for _, campaign := range demoLaunchCampaigns {
			assert.Equal(t, "iphone-launch-2024", campaign.Campaign)
			weight += campaign.Weight
		}
		assert.Equal(t, 16, weight)
	})
}

func TestDemoWebAnalyticsCatalogPages(t *testing.T) {
	categories := map[string]bool{"homepage": true, "category": true, "product": true, "shop": true}
	productLines := map[string]bool{
		"iphone": true, "mac": true, "ipad": true, "watch": true,
		"airpods": true, "tv-home": true, "vision": true, "": true,
	}

	seen := make(map[string]bool, len(demoPages))
	for _, page := range demoPages {
		t.Run(page.Path, func(t *testing.T) {
			assert.False(t, seen[page.Path], "duplicate page path")
			seen[page.Path] = true

			assert.True(t, categories[page.Category], "unknown category %q", page.Category)
			assert.True(t, productLines[page.ProductLine], "unknown product line %q", page.ProductLine)
			assert.True(t, strings.HasPrefix(page.Path, "/"), "path must be absolute")

			// Apple's buy links are the only paths without a trailing slash.
			if page.Category == "shop" {
				assert.False(t, strings.HasSuffix(page.Path, "/"), "shop redirects carry no trailing slash")
			} else {
				assert.True(t, strings.HasSuffix(page.Path, "/"), "path must end with a slash")
			}

			assert.Positive(t, page.DurationMultiplier, "duration multiplier must be positive")
		})
	}

	// A product page that resolves to no price can never convert, which is how
	// a comparison page mislabelled as a product silently kills conversions.
	t.Run("every product page resolves to a price", func(t *testing.T) {
		for _, page := range demoPages {
			if page.Category != "product" {
				continue
			}
			product, ok := demoPriceForPath(page.Path)
			require.True(t, ok, "product page %s resolves to no product", page.Path)
			assert.Equal(t, page.Product, product.Name)
			assert.Positive(t, product.MinPrice, "product %s needs a price", product.Name)
			assert.GreaterOrEqual(t, product.MaxPrice, product.MinPrice)
		}
	})

	t.Run("every product name exists in the Apple catalogue", func(t *testing.T) {
		known := make(map[string]bool, len(demoAppleProducts))
		for _, product := range demoAppleProducts {
			known[product.Name] = true
		}
		for _, page := range demoPages {
			if page.Product == "" {
				continue
			}
			assert.True(t, known[page.Product], "page %s sells unknown product %q", page.Path, page.Product)
		}
	})

	t.Run("pages that sell nothing resolve to no price", func(t *testing.T) {
		for _, page := range demoPages {
			if page.Product != "" {
				continue
			}
			_, ok := demoPriceForPath(page.Path)
			assert.False(t, ok, "page %s should not be priceable", page.Path)
		}
	})

	t.Run("unknown paths resolve to no price", func(t *testing.T) {
		_, ok := demoPriceForPath("/newsroom/")
		assert.False(t, ok)
	})
}

// The broadcast seeder and the web analytics seeder used to name their
// campaigns independently, which left every demo broadcast pointing at an empty
// traffic report — the two sets did not share a single name.
func TestDemoBroadcastCampaignsAreSeededAsTraffic(t *testing.T) {
	require.NotEmpty(t, demoBroadcastCampaigns)

	type scope struct{ campaign, content string }
	seeded := make(map[scope]demoCampaign, len(demoCampaigns))
	for _, campaign := range demoCampaigns {
		seeded[scope{campaign.Campaign, campaign.Content}] = campaign
	}

	for _, broadcast := range demoBroadcastCampaigns {
		require.NotEmpty(t, broadcast.Templates,
			"broadcast %q has no variation to seed traffic for", broadcast.Name)

		for _, templateID := range broadcast.Templates {
			// A send stamps utm_content with the variation's template id, so the
			// per-variation report filters on exactly this pair.
			campaign, ok := seeded[scope{broadcast.Campaign, templateID}]
			require.True(t, ok,
				"broadcast %q variation %q sends utm_campaign=%q utm_content=%q, which no demo session carries",
				broadcast.Name, templateID, broadcast.Campaign, templateID)
			// A source or medium that disagrees with the broadcast's would still
			// read as someone else's traffic in every breakdown beside it.
			assert.Equal(t, demoBroadcastUTMSource, campaign.Source)
			assert.Equal(t, "email", campaign.Medium)
		}
	}
}

func TestDemoWebAnalyticsCatalogCampaigns(t *testing.T) {
	// The default attribution rules key paid channels off utm_id_from, so a
	// paid campaign without a click id would be reported as not-mapped.
	clickIDs := map[string]bool{"gclid": true, "fbclid": true, "msclkid": true, "": true}

	for _, campaign := range demoCampaigns {
		assert.NotEmpty(t, campaign.Source)
		assert.NotEmpty(t, campaign.Medium)
		assert.NotEmpty(t, campaign.Campaign)
		assert.True(t, clickIDs[campaign.ClickIDParam], "unknown click id param %q", campaign.ClickIDParam)
		assert.Positive(t, campaign.DurationMultiplier)

		switch {
		case campaign.Source == "google" && campaign.Medium == "cpc":
			assert.Equal(t, "gclid", campaign.ClickIDParam, "google cpc campaign %q needs a gclid", campaign.Campaign)
		case campaign.Source == "facebook":
			assert.Equal(t, "fbclid", campaign.ClickIDParam, "facebook campaign %q needs an fbclid", campaign.Campaign)
		default:
			assert.Empty(t, campaign.ClickIDParam, "campaign %q should carry no click id", campaign.Campaign)
		}
	}
}

// The SDK reports what ua-parser-js v2 parses, normalised by
// web_analytics_sdk/src/detection/device.ts: device type collapses to
// desktop/mobile/tablet, "Mac OS" becomes "macOS", iOS on a tablet becomes
// iPadOS. Browser names are ua-parser-js's own, which prefix mobile builds
// ("Mobile Safari", "Mobile Chrome") and spell Samsung's browser "Samsung
// Internet". Values Staminads stored that the SDK can never emit would leave
// the demo describing devices no real visitor reports.
func TestDemoWebAnalyticsCatalogDevices(t *testing.T) {
	deviceTypes := map[string]bool{"desktop": true, "mobile": true, "tablet": true}
	operatingSystems := map[string]bool{
		"Windows": true, "macOS": true, "iOS": true, "iPadOS": true,
		"Android": true, "Linux": true, "Chrome OS": true,
	}
	browsers := map[string]bool{
		"Safari": true, "Mobile Safari": true,
		"Chrome": true, "Mobile Chrome": true, "Chrome WebView": true,
		"Firefox": true, "Mobile Firefox": true,
		"Edge": true, "Opera": true, "Samsung Internet": true,
	}
	// ua-parser-js only reports the bare name on desktop builds.
	desktopOnlyBrowsers := map[string]bool{"Safari": true, "Chrome": true, "Firefox": true}

	for _, device := range demoDevices {
		t.Run(device.OS+"/"+device.Browser, func(t *testing.T) {
			assert.True(t, deviceTypes[device.Device], "unknown device type %q", device.Device)
			assert.True(t, operatingSystems[device.OS], "OS %q is not one the SDK emits", device.OS)
			assert.True(t, browsers[device.Browser], "browser %q is not one the SDK emits", device.Browser)

			if device.Device != "desktop" {
				assert.False(t, desktopOnlyBrowsers[device.Browser],
					"%s on %s should be reported as its mobile build", device.Browser, device.Device)
			}
			if device.OS == "iPadOS" {
				assert.Equal(t, "tablet", device.Device, "iPadOS is only reported for tablets")
			}

			assert.NotEmpty(t, device.UserAgent)
			assert.True(t, strings.HasPrefix(device.UserAgent, "Mozilla/5.0 "), "user agent must stay verbatim")
			assert.Positive(t, device.ScreenWidth)
			assert.Positive(t, device.ScreenHeight)
			assert.Positive(t, device.DurationMultiplier)
		})
	}
}

func TestDemoWebAnalyticsCatalogGeos(t *testing.T) {
	flagDir := filepath.Join("..", "..", "console", "public", "icons", "flags")
	_, flagErr := os.Stat(flagDir)

	for _, geo := range demoGeos {
		t.Run(geo.City, func(t *testing.T) {
			assert.Len(t, geo.Country, 2, "country must be an ISO 3166-1 alpha-2 code")
			assert.Equal(t, strings.ToUpper(geo.Country), geo.Country, "country code must be uppercase")
			for _, r := range geo.Country {
				assert.True(t, r >= 'A' && r <= 'Z', "country code must be letters only")
			}

			assert.NotEmpty(t, geo.Region)
			assert.NotEmpty(t, geo.City)
			assert.NotEmpty(t, geo.Language)
			assert.Positive(t, geo.DurationMultiplier)

			// The generator converts session times into the visitor's local
			// hour, so an unknown zone name would panic at generation time.
			_, err := time.LoadLocation(geo.Timezone)
			assert.NoError(t, err, "timezone %q must load", geo.Timezone)

			assert.GreaterOrEqual(t, geo.Latitude, -90.0)
			assert.LessOrEqual(t, geo.Latitude, 90.0)
			assert.GreaterOrEqual(t, geo.Longitude, -180.0)
			assert.LessOrEqual(t, geo.Longitude, 180.0)

			// The console renders a flag per country in the geo reports.
			if flagErr != nil {
				t.Skip("console flag assets are not available")
			}
			flag := filepath.Join(flagDir, strings.ToLower(geo.Country)+".svg")
			_, err = os.Stat(flag)
			assert.NoError(t, err, "country %s has no flag asset", geo.Country)
		})
	}
}

func TestDemoWebAnalyticsCatalogReferrers(t *testing.T) {
	categories := map[string]bool{
		"direct": true, "search": true, "social": true, "tech-news": true,
		"retailer": true, "other": true, "internal": true,
	}

	direct := 0
	for _, referrer := range demoReferrers {
		assert.True(t, categories[referrer.Category], "unknown category %q", referrer.Category)
		assert.Positive(t, referrer.DurationMultiplier)

		if referrer.Domain == "" {
			direct++
			assert.Equal(t, "direct", referrer.Category, "only direct traffic carries no domain")
			assert.Empty(t, referrer.Path, "direct traffic carries no referrer path")
			continue
		}

		parsed, err := url.Parse("https://" + referrer.Domain)
		require.NoError(t, err, "referrer %q must parse as a host", referrer.Domain)
		assert.Equal(t, referrer.Domain, parsed.Host, "referrer %q must be a bare host", referrer.Domain)
		assert.Contains(t, referrer.Domain, ".", "referrer %q must be a fully qualified domain", referrer.Domain)

		if referrer.Path != "" {
			assert.True(t, strings.HasPrefix(referrer.Path, "/"), "referrer path %q must be absolute", referrer.Path)
		}
	}
	assert.Equal(t, 1, direct, "exactly one entry stands for direct traffic")
}

func TestDemoWebAnalyticsCatalogProductFilters(t *testing.T) {
	filters := demoProductCategoryFilters()
	require.Len(t, filters, 35, "34 product rules plus the catch-all")

	t.Run("rules validate and write only custom_1", func(t *testing.T) {
		for _, filter := range filters {
			require.NoError(t, filter.Validate(), "rule %q must validate", filter.Name)
			require.NotEmpty(t, filter.Operations)
			assert.True(t, filter.Enabled, "rule %q must be enabled", filter.Name)

			for _, operation := range filter.Operations {
				assert.Equal(t, "custom_1", operation.Dimension,
					"rule %q writes %q instead of the product line dimension", filter.Name, operation.Dimension)
				assert.NotEmpty(t, operation.Value)
			}
		}
	})

	t.Run("ids are stable and unique", func(t *testing.T) {
		seen := make(map[string]bool, len(filters))
		for _, filter := range filters {
			assert.True(t, strings.HasPrefix(filter.ID, "demo-product-"), "id %q is not a demo product id", filter.ID)
			assert.False(t, seen[filter.ID], "duplicate rule id %q", filter.ID)
			seen[filter.ID] = true
		}

		// A demo reset must rewrite the same rules rather than pile up copies.
		for i, filter := range demoProductCategoryFilters() {
			assert.Equal(t, filters[i].ID, filter.ID, "rule ids must be reproducible")
		}
	})

	t.Run("conditions read landing_path with compilable regexes", func(t *testing.T) {
		for _, filter := range filters {
			for _, condition := range filter.Conditions {
				assert.Equal(t, "landing_path", condition.Field)
				assert.Equal(t, domain.WebFilterOpRegex, condition.Operator)
				_, err := regexp.Compile(condition.Value)
				assert.NoError(t, err, "regex %q must compile", condition.Value)
			}
		}
	})

	t.Run("priorities descend within a product line", func(t *testing.T) {
		lastPriority := map[string]int{}
		for _, filter := range filters {
			for _, operation := range filter.Operations {
				if operation.Action != domain.WebFilterActionSetValue {
					continue
				}
				if previous, seen := lastPriority[operation.Value]; seen {
					assert.Less(t, filter.Priority, previous,
						"rule %q must be less specific than the rule above it", filter.Name)
				}
				lastPriority[operation.Value] = filter.Priority
			}
		}
	})

	t.Run("a single catch-all writes Other", func(t *testing.T) {
		defaults := 0
		for _, filter := range filters {
			for _, operation := range filter.Operations {
				if operation.Action == domain.WebFilterActionSetDefaultValue {
					defaults++
					assert.Equal(t, "Other", operation.Value)
					assert.Empty(t, filter.Conditions, "the catch-all must always match")
				}
			}
		}
		assert.Equal(t, 1, defaults)
	})

	t.Run("the caller stamps the version after merging", func(t *testing.T) {
		for _, filter := range filters {
			assert.Empty(t, filter.Version, "rule %q must not carry its own version", filter.Name)
		}
	})
}

func TestDemoWebAnalyticsCatalogProductFilterEvaluation(t *testing.T) {
	filters := demoProductCategoryFilters()

	testCases := []struct {
		name        string
		landingPath string
		expected    string
	}{
		{"most specific iPhone page wins over the category rule", "/iphone-17-pro/", "iPhone"},
		{"iPhone category page", "/iphone/", "iPhone"},
		{"iPhone comparison page", "/iphone/compare/", "iPhone"},
		{"MacBook Air", "/macbook-air/", "Mac"},
		{"Mac category page is not shadowed by MacBook rules", "/mac/", "Mac"},
		{"iPad Air is not shadowed by the iPad category rule", "/ipad-air/", "iPad"},
		{"Apple Watch SE", "/apple-watch-se-3/", "Watch"},
		{"AirPods", "/airpods-4/", "AirPods"},
		{"Apple TV", "/apple-tv-4k/", "TV & Home"},
		{"Vision Pro", "/apple-vision-pro/", "Vision Pro"},
		{"shop redirect", "/us/shop/goto/buy_iphone", "iPhone"},
		{"homepage", "/", "Homepage"},
		{"unknown path falls back to the catch-all", "/newsroom/press-release/", "Other"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := domain.EvaluateWebFilters(filters, map[string]string{"landing_path": testCase.landingPath})
			value := result["custom_1"]
			require.NotNil(t, value, "landing path %q wrote no product line", testCase.landingPath)
			assert.Equal(t, testCase.expected, *value)
		})
	}

	// Every page the generator can emit must land on a real product line: a page
	// added without its rule would show up as "Other" in the demo reports.
	t.Run("every demo page maps to its own product line", func(t *testing.T) {
		expectedByLine := map[string]string{
			"iphone": "iPhone", "mac": "Mac", "ipad": "iPad", "watch": "Watch",
			"airpods": "AirPods", "tv-home": "TV & Home", "vision": "Vision Pro",
			"": "Homepage",
		}

		for _, page := range demoPages {
			result := domain.EvaluateWebFilters(filters, map[string]string{"landing_path": page.Path})
			value := result["custom_1"]
			require.NotNil(t, value, "page %s wrote no product line", page.Path)
			assert.Equal(t, expectedByLine[page.ProductLine], *value, "page %s", page.Path)
		}
	})
}
