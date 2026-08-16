package service

import (
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// Catalogues for the demo web analytics dataset, ported from the Staminads
// demo fixtures (apple-pages, referrers, devices, utm-campaigns, geo-languages,
// demo-filters). The generator draws from these tables by weight, so the shape
// of the demo — which pages are popular, which campaign wins the A/B test,
// which hours are busy — lives here and nowhere else.
//
// Weights are relative, not percentages. The catalogue tests pin every total,
// so adding a row without thinking about its neighbours fails loudly.

// demoPage is a page a session can land on. Product names the entry in
// demoAppleProducts the page sells, empty when the page sells nothing (a
// homepage, a category listing, a comparison page, a shop redirect).
type demoPage struct {
	Path               string  // e.g. "/iphone-17-pro/"
	Category           string  // "homepage" | "category" | "product" | "shop"
	ProductLine        string  // "iphone" | "mac" | "ipad" | "watch" | "airpods" | "tv-home" | "vision" | ""
	Product            string  // name in demoAppleProducts, "" when the page sells nothing
	DurationMultiplier float64 // scales the time spent on the page
	Weight             int
}

var demoPages = []demoPage{
	// Path, Category, ProductLine, Product, DurationMultiplier, Weight
	{"/", "homepage", "", "", 0.6, 8},

	// Category pages
	{"/mac/", "category", "mac", "", 1.0, 5},
	{"/ipad/", "category", "ipad", "", 1.0, 5},
	{"/iphone/", "category", "iphone", "", 1.0, 8},
	{"/watch/", "category", "watch", "", 1.0, 4},
	{"/airpods/", "category", "airpods", "", 1.0, 4},
	{"/tv-home/", "category", "tv-home", "", 1.0, 2},
	{"/apple-vision-pro/", "category", "vision", "", 1.0, 2},

	// Mac products
	{"/macbook-air/", "product", "mac", "MacBook Air", 1.4, 6},
	{"/macbook-pro/", "product", "mac", "MacBook Pro", 1.4, 5},
	{"/imac/", "product", "mac", "iMac", 1.4, 3},
	{"/mac-mini/", "product", "mac", "Mac mini", 1.4, 2},
	{"/mac-studio/", "product", "mac", "Mac Studio", 1.4, 1},
	{"/mac-pro/", "product", "mac", "Mac Pro", 1.4, 1},

	// iPhone products. The comparison page is a category page: it sells no
	// single model, so a "product" here would be a page that can never convert.
	{"/iphone-17-pro/", "product", "iphone", "iPhone 17 Pro", 1.4, 10},
	{"/iphone-air/", "product", "iphone", "iPhone Air", 1.4, 8},
	{"/iphone-17/", "product", "iphone", "iPhone 17", 1.4, 7},
	{"/iphone-16e/", "product", "iphone", "iPhone 16e", 1.4, 4},
	{"/iphone/compare/", "category", "iphone", "", 1.6, 3},

	// iPad products
	{"/ipad-pro/", "product", "ipad", "iPad Pro", 1.4, 4},
	{"/ipad-air/", "product", "ipad", "iPad Air", 1.4, 4},
	{"/ipad-mini/", "product", "ipad", "iPad mini", 1.4, 2},

	// Watch products
	{"/apple-watch-series-11/", "product", "watch", "Apple Watch Series 11", 1.4, 4},
	{"/apple-watch-ultra-3/", "product", "watch", "Apple Watch Ultra 3", 1.4, 2},
	{"/apple-watch-se-3/", "product", "watch", "Apple Watch SE 3", 1.4, 2},

	// AirPods products
	{"/airpods-pro/", "product", "airpods", "AirPods Pro", 1.4, 4},
	{"/airpods-4/", "product", "airpods", "AirPods 4", 1.4, 3},
	{"/airpods-max/", "product", "airpods", "AirPods Max", 1.4, 2},

	// TV & Home products
	{"/apple-tv-4k/", "product", "tv-home", "Apple TV 4K", 1.4, 2},
	{"/homepod-mini/", "product", "tv-home", "HomePod mini", 1.4, 2},

	// Shop redirects. Apple's own buy links carry no trailing slash, and they
	// hand off to the store rather than sell a model themselves.
	{"/us/shop/goto/buy_iphone", "shop", "iphone", "", 0.8, 5},
	{"/us/shop/goto/buy_mac", "shop", "mac", "", 0.8, 3},
	{"/us/shop/goto/buy_ipad", "shop", "ipad", "", 0.8, 2},
	{"/us/shop/goto/buy_watch", "shop", "watch", "", 0.8, 2},
}

// demoLaunchPages is what the iPhone 17 keynote spike lands on.
var demoLaunchPages = func() []demoPage {
	pages := make([]demoPage, 0, 4)
	for _, page := range demoPages {
		if strings.Contains(page.Path, "iphone-17") ||
			strings.Contains(page.Path, "iphone-air") ||
			page.Path == "/iphone/" {
			pages = append(pages, page)
		}
	}
	return pages
}()

// demoReferrer is where a session came from. An empty Domain is direct traffic.
type demoReferrer struct {
	Domain             string // "" means direct (no referrer)
	Path               string // "" means "/"
	Category           string // "direct"|"search"|"social"|"tech-news"|"retailer"|"other"|"internal"
	DurationMultiplier float64
	Weight             int
}

var demoReferrers = []demoReferrer{
	// Domain, Path, Category, DurationMultiplier, Weight
	{"", "", "direct", 1.3, 20},

	// Search engines
	{"google.com", "/search", "search", 1.2, 25},
	{"bing.com", "/search", "search", 1.1, 5},
	{"yahoo.com", "/search", "search", 1.0, 2},
	{"duckduckgo.com", "", "search", 1.3, 2},
	{"baidu.com", "", "search", 1.1, 2},

	// Social media
	{"facebook.com", "", "social", 0.8, 5},
	{"twitter.com", "", "social", 0.75, 3},
	{"instagram.com", "", "social", 0.7, 4},
	{"linkedin.com", "", "social", 1.4, 2},
	{"youtube.com", "/watch", "social", 1.1, 4},
	{"reddit.com", "/r/apple", "social", 1.2, 3},
	{"pinterest.com", "", "social", 0.9, 1},
	{"tiktok.com", "", "social", 0.7, 3},

	// Tech news
	{"macrumors.com", "", "tech-news", 1.6, 3},
	{"9to5mac.com", "", "tech-news", 1.6, 3},
	{"theverge.com", "", "tech-news", 1.5, 2},
	{"cnet.com", "", "tech-news", 1.4, 2},
	{"techcrunch.com", "", "tech-news", 1.5, 1},
	{"engadget.com", "", "tech-news", 1.4, 1},
	{"wired.com", "", "tech-news", 1.5, 1},

	// Retailers
	{"amazon.com", "", "retailer", 0.9, 2},
	{"bestbuy.com", "", "retailer", 1.0, 1},
	{"target.com", "", "retailer", 0.9, 1},
	{"walmart.com", "", "retailer", 0.8, 1},

	// Sources the default channel rules do not classify, so the demo has a
	// realistic "not-mapped" slice instead of a suspiciously perfect report.
	{"news.ycombinator.com", "", "other", 1.5, 2},
	{"slickdeals.net", "", "other", 0.6, 1},
	{"quora.com", "", "other", 1.1, 1},
	{"medium.com", "", "other", 1.3, 1},

	// Internal navigation
	{"apple.com", "", "internal", 1.2, 5},
}

// demoTechNewsReferrers carries the launch-week spike: the keynote coverage is
// what sends the extra traffic.
var demoTechNewsReferrers = func() []demoReferrer {
	referrers := make([]demoReferrer, 0, 7)
	for _, referrer := range demoReferrers {
		if referrer.Category == "tech-news" {
			referrers = append(referrers, referrer)
		}
	}
	return referrers
}()

// demoCampaign is a UTM-tagged acquisition source.
//
// ClickIDParam is a Notifuse addition. The ten highest-priority default
// attribution rules (priorities 900-870) key on utm_id_from, which Staminads
// never populated; without it the paid campaigns below would match no paid rule
// and the demo would report its ad spend as not-mapped.
type demoCampaign struct {
	Source, Medium, Campaign, Content, Term string
	ClickIDParam                            string // "gclid" | "fbclid" | "msclkid" | ""
	DurationMultiplier                      float64
	Weight                                  int
}

var demoCampaigns = func() []demoCampaign {
	campaigns := []demoCampaign{
		// Source, Medium, Campaign, Content, Term, ClickIDParam, DurationMultiplier, Weight

		// Holiday sale, with four creatives so the demo has an A/B result to read:
		// the video hero doubles engagement, the static banner halves it.
		{"google", "cpc", "holiday-sale", "video-hero-v2", "", "gclid", 2.2, 5},
		{"google", "cpc", "holiday-sale", "carousel-products", "", "gclid", 1.5, 4},
		{"google", "cpc", "holiday-sale", "static-banner-v1", "", "gclid", 0.7, 3},
		{"google", "cpc", "holiday-sale", "static-banner-v2", "", "gclid", 0.8, 3},

		// iPhone launch
		{"google", "cpc", "iphone-launch-2024", "hands-on-demo", "", "gclid", 2.5, 6},
		{"google", "cpc", "iphone-launch-2024", "spec-comparison", "", "gclid", 1.8, 4},
		{"google", "cpc", "iphone-launch-2024", "price-focus", "", "gclid", 0.6, 3},

		// Paid social
		{"facebook", "social", "holiday-sale", "lifestyle-video", "", "fbclid", 1.3, 4},
		{"instagram", "social", "holiday-sale", "story-carousel", "", "", 0.9, 4},
		{"instagram", "social", "iphone-launch-2024", "reel-unboxing", "", "", 1.4, 3},

		// Email is seeded from demoBroadcastCampaigns below, so the sessions carry
		// the same campaign names the sample broadcasts tag their links with.

		// Display
		{"display", "display", "retargeting", "", "", "", 1.1, 3},
		{"display", "display", "awareness", "", "", "", 0.7, 2},

		// Affiliate
		{"affiliate", "referral", "tech-reviewers", "", "", "", 1.4, 2},

		// Back to school
		{"google", "cpc", "back-to-school", "student-discount", "macbook student", "gclid", 1.3, 3},

		// WWDC
		{"twitter", "social", "wwdc-2024", "", "", "", 1.6, 2},

		// AirPods promo
		{"facebook", "social", "airpods-promo", "comparison-ad", "", "fbclid", 1.2, 2},
	}

	// One email row per sample broadcast variation, tagged exactly as that
	// variation's send tags its own links — campaign from the broadcast, content
	// from the variation's template — so neither a broadcast's report nor a
	// single variation's opens empty. Multipliers vary so the A/B test has a
	// winner to read; the weights total the 10 the three hand-written email rows
	// they replaced carried between them.
	multipliers := []float64{1.5, 1.4, 1.6, 1.9, 0.8}
	emailWeight := 2
	i := 0
	for _, broadcast := range demoBroadcastCampaigns {
		for _, templateID := range broadcast.Templates {
			campaigns = append(campaigns, demoCampaign{
				Source:             demoBroadcastUTMSource,
				Medium:             "email",
				Campaign:           broadcast.Campaign,
				Content:            templateID,
				DurationMultiplier: multipliers[i%len(multipliers)],
				Weight:             emailWeight,
			})
			i++
		}
	}

	return campaigns
}()

var demoLaunchCampaigns = func() []demoCampaign {
	campaigns := make([]demoCampaign, 0, 4)
	for _, campaign := range demoCampaigns {
		if campaign.Campaign == "iphone-launch-2024" {
			campaigns = append(campaigns, campaign)
		}
	}
	return campaigns
}()

// demoNoUTMWeight is the weight of "no campaign at all", drawn against the sum
// of demoCampaigns weights: most traffic is organic.
const demoNoUTMWeight = 60

// demoDevice is a browsing profile.
//
// OS, Browser and Device hold what the Notifuse SDK would report for the user
// agent below, not what Staminads stored: the SDK runs ua-parser-js v2 and then
// normalises (see web_analytics_sdk/src/detection/device.ts). That means
// "Mobile Safari" and "Mobile Chrome" on phones and tablets, "Samsung Internet"
// rather than "Samsung Browser", and iPads reported as iPadOS. Every value here
// was checked by parsing the user agent with the SDK's own parser version.
type demoDevice struct {
	OS, Browser, Device       string
	UserAgent                 string
	ScreenWidth, ScreenHeight int
	DurationMultiplier        float64
	Weight                    int
}

var demoDevices = []demoDevice{
	// macOS desktop
	{
		OS: "macOS", Browser: "Safari", Device: "desktop",
		UserAgent:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 15_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15",
		ScreenWidth: 1920, ScreenHeight: 1080,
		DurationMultiplier: 1.4, Weight: 15,
	},
	{
		OS: "macOS", Browser: "Safari", Device: "desktop",
		UserAgent:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15",
		ScreenWidth: 2560, ScreenHeight: 1440,
		DurationMultiplier: 1.4, Weight: 8,
	},
	{
		OS: "macOS", Browser: "Chrome", Device: "desktop",
		UserAgent:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 15_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		ScreenWidth: 1920, ScreenHeight: 1080,
		DurationMultiplier: 1.3, Weight: 8,
	},
	{
		OS: "macOS", Browser: "Firefox", Device: "desktop",
		UserAgent:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 15.0; rv:121.0) Gecko/20100101 Firefox/121.0",
		ScreenWidth: 1680, ScreenHeight: 1050,
		DurationMultiplier: 1.43, Weight: 3,
	},

	// iPhone
	{
		OS: "iOS", Browser: "Mobile Safari", Device: "mobile",
		UserAgent:   "Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1",
		ScreenWidth: 430, ScreenHeight: 932,
		DurationMultiplier: 1.4, Weight: 12,
	},
	{
		OS: "iOS", Browser: "Mobile Safari", Device: "mobile",
		UserAgent:   "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1",
		ScreenWidth: 430, ScreenHeight: 932,
		DurationMultiplier: 1.4, Weight: 10,
	},
	{
		OS: "iOS", Browser: "Mobile Safari", Device: "mobile",
		UserAgent:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.6 Mobile/15E148 Safari/604.1",
		ScreenWidth: 390, ScreenHeight: 844,
		DurationMultiplier: 1.4, Weight: 8,
	},
	{
		OS: "iOS", Browser: "Mobile Chrome", Device: "mobile",
		UserAgent:   "Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/120.0.6099.119 Mobile/15E148 Safari/604.1",
		ScreenWidth: 393, ScreenHeight: 852,
		DurationMultiplier: 1.0, Weight: 4,
	},

	// iPad
	{
		OS: "iPadOS", Browser: "Mobile Safari", Device: "tablet",
		UserAgent:   "Mozilla/5.0 (iPad; CPU OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1",
		ScreenWidth: 1024, ScreenHeight: 1366,
		DurationMultiplier: 2.52, Weight: 5,
	},
	{
		OS: "iPadOS", Browser: "Mobile Safari", Device: "tablet",
		UserAgent:   "Mozilla/5.0 (iPad; CPU OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1",
		ScreenWidth: 820, ScreenHeight: 1180,
		DurationMultiplier: 2.52, Weight: 4,
	},

	// Windows desktop
	{
		OS: "Windows", Browser: "Chrome", Device: "desktop",
		UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		ScreenWidth: 1920, ScreenHeight: 1080,
		DurationMultiplier: 1.3, Weight: 10,
	},
	{
		OS: "Windows", Browser: "Chrome", Device: "desktop",
		UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
		ScreenWidth: 1366, ScreenHeight: 768,
		DurationMultiplier: 1.3, Weight: 5,
	},
	{
		OS: "Windows", Browser: "Edge", Device: "desktop",
		UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
		ScreenWidth: 1920, ScreenHeight: 1080,
		DurationMultiplier: 1.17, Weight: 4,
	},
	{
		OS: "Windows", Browser: "Firefox", Device: "desktop",
		UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
		ScreenWidth: 1920, ScreenHeight: 1080,
		DurationMultiplier: 1.43, Weight: 3,
	},

	// Android
	{
		OS: "Android", Browser: "Mobile Chrome", Device: "mobile",
		UserAgent:   "Mozilla/5.0 (Linux; Android 14; SM-S928B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.6099.144 Mobile Safari/537.36",
		ScreenWidth: 412, ScreenHeight: 915,
		DurationMultiplier: 1.0, Weight: 5,
	},
	{
		OS: "Android", Browser: "Mobile Chrome", Device: "mobile",
		UserAgent:   "Mozilla/5.0 (Linux; Android 14; Pixel 8 Pro) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.6099.144 Mobile Safari/537.36",
		ScreenWidth: 412, ScreenHeight: 892,
		DurationMultiplier: 1.0, Weight: 3,
	},
	{
		OS: "Android", Browser: "Samsung Internet", Device: "mobile",
		UserAgent:   "Mozilla/5.0 (Linux; Android 13; SAMSUNG SM-S911B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/23.0 Chrome/115.0.0.0 Mobile Safari/537.36",
		ScreenWidth: 360, ScreenHeight: 780,
		DurationMultiplier: 0.95, Weight: 2,
	},
}

// demoGeo is a visitor location. Timezone drives the local hour of the session,
// so it has to be a zone the Go time database knows.
type demoGeo struct {
	Country, Region, City string
	Latitude, Longitude   float64
	Timezone, Language    string
	DurationMultiplier    float64
	Weight                int
}

var demoGeos = []demoGeo{
	// Country, Region, City, Latitude, Longitude, Timezone, Language, DurationMultiplier, Weight
	{"US", "New York", "New York", 40.71, -74.01, "America/New_York", "en-US", 1.0, 25},
	{"US", "California", "San Francisco", 37.77, -122.42, "America/Los_Angeles", "en-US", 1.0, 15},
	{"GB", "England", "London", 51.51, -0.13, "Europe/London", "en-GB", 1.1, 10},
	{"DE", "Berlin", "Berlin", 52.52, 13.41, "Europe/Berlin", "de-DE", 1.3, 8},
	{"FR", "Île-de-France", "Paris", 48.86, 2.35, "Europe/Paris", "fr-FR", 1.2, 7},
	{"JP", "Tokyo", "Tokyo", 35.68, 139.69, "Asia/Tokyo", "ja-JP", 1.5, 7},
	{"CN", "Shanghai", "Shanghai", 31.23, 121.47, "Asia/Shanghai", "zh-CN", 1.1, 6},
	{"AU", "New South Wales", "Sydney", -33.87, 151.21, "Australia/Sydney", "en-AU", 1.15, 5},
	{"CA", "Ontario", "Toronto", 43.65, -79.38, "America/Toronto", "en-CA", 1.05, 4},
	{"ES", "Community of Madrid", "Madrid", 40.42, -3.7, "Europe/Madrid", "es-ES", 1.1, 4},
	{"BR", "São Paulo", "São Paulo", -23.55, -46.63, "America/Sao_Paulo", "pt-BR", 0.8, 3},
	{"IN", "Maharashtra", "Mumbai", 19.08, 72.88, "Asia/Kolkata", "en-IN", 0.7, 3},
	{"KR", "Seoul", "Seoul", 37.57, 126.98, "Asia/Seoul", "ko-KR", 1.35, 3},
}

// demoHourMultipliers weights sessions by the visitor's local hour: a morning
// rise, a lunch dip, an evening peak.
var demoHourMultipliers = [24]float64{
	0.6, 0.55, 0.5, 0.45, 0.5, 0.6, // 00:00-05:00
	0.8, 0.9, 1.0, 1.2, 1.2, 1.1, // 06:00-11:00, 09:00 peak
	0.95, 0.9, 1.0, 1.0, 1.05, 1.1, // 12:00-17:00, lunch dip
	1.15, 1.4, 1.4, 1.3, 1.0, 0.8, // 18:00-23:00, 19:00 peak
}

// demoDayOfWeekWeights is indexed by time.Weekday: weekends are quieter.
var demoDayOfWeekWeights = [7]float64{
	0.6,  // Sunday
	1.0,  // Monday
	1.1,  // Tuesday
	1.05, // Wednesday
	1.0,  // Thursday
	0.9,  // Friday
	0.7,  // Saturday
}

// demoPricesByPath resolves a landing path to the product it sells. Pages that
// name no product — homepages, categories, shop redirects — are absent, which
// is what stops the generator from booking a conversion against them.
var demoPricesByPath = func() map[string]demoProduct {
	byName := make(map[string]demoProduct, len(demoAppleProducts))
	for _, product := range demoAppleProducts {
		byName[product.Name] = product
	}

	prices := make(map[string]demoProduct, len(demoPages))
	for _, page := range demoPages {
		if page.Product == "" {
			continue
		}
		if product, ok := byName[page.Product]; ok {
			prices[page.Path] = product
		}
	}
	return prices
}()

// demoPriceForPath returns the product a page sells and its price range.
func demoPriceForPath(path string) (demoProduct, bool) {
	product, ok := demoPricesByPath[path]
	return product, ok
}

// demoProductCategoryFilters returns the attribution rules that derive custom_1
// (the product line) from landing_path, ported from the Staminads demo filters
// where the same dimension was called stm_1.
//
// Rules are ordered most specific first — "^/iphone-17-pro/" at 450 outranks
// "^/iphone/" at 400 — because the first writer of a dimension at the highest
// priority wins. Ids are derived from the rule name rather than generated, so
// resetting the demo rewrites the same rules instead of accumulating duplicates.
//
// Version is deliberately left empty: these rules are appended to a workspace's
// existing set, and the caller hashes the merged result.
func demoProductCategoryFilters() []domain.WebFilter {
	now := time.Now().UTC().Format(time.RFC3339)

	// Orders start above the default channel rule set so the merged list keeps
	// channel rules first in the console.
	order := 100

	slug := func(name string) string {
		var builder strings.Builder
		dashed := false
		for _, r := range strings.ToLower(name) {
			switch {
			case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
				builder.WriteRune(r)
				dashed = false
			case builder.Len() > 0 && !dashed:
				builder.WriteByte('-')
				dashed = true
			}
		}
		return strings.TrimSuffix(builder.String(), "-")
	}

	rule := func(name, pathPattern, productLine string, priority int) domain.WebFilter {
		order++
		return domain.WebFilter{
			ID:       "demo-product-" + slug(name),
			Name:     name,
			Priority: priority,
			Order:    order,
			Tags:     []string{"product category"},
			Conditions: []domain.WebFilterCondition{
				{Field: "landing_path", Operator: domain.WebFilterOpRegex, Value: pathPattern},
			},
			Operations: []domain.WebFilterOperation{
				{Dimension: "custom_1", Action: domain.WebFilterActionSetValue, Value: productLine},
			},
			Enabled:   true,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	filters := []domain.WebFilter{
		// iPhone (450-390)
		rule("iPhone 17 Pro", "^/iphone-17-pro/", "iPhone", 450),
		rule("iPhone Air", "^/iphone-air/", "iPhone", 440),
		rule("iPhone 17", "^/iphone-17/", "iPhone", 430),
		rule("iPhone 16e", "^/iphone-16e/", "iPhone", 420),
		rule("iPhone Compare", "^/iphone/compare/", "iPhone", 410),
		rule("iPhone General", "^/iphone/", "iPhone", 400),
		rule("Buy iPhone", "/buy_iphone", "iPhone", 390),

		// Mac (380-310)
		rule("MacBook Air", "^/macbook-air/", "Mac", 380),
		rule("MacBook Pro", "^/macbook-pro/", "Mac", 370),
		rule("iMac", "^/imac/", "Mac", 360),
		rule("Mac mini", "^/mac-mini/", "Mac", 350),
		rule("Mac Studio", "^/mac-studio/", "Mac", 340),
		rule("Mac Pro", "^/mac-pro/", "Mac", 330),
		rule("Mac General", "^/mac/", "Mac", 320),
		rule("Buy Mac", "/buy_mac", "Mac", 310),

		// iPad (300-260)
		rule("iPad Pro", "^/ipad-pro/", "iPad", 300),
		rule("iPad Air", "^/ipad-air/", "iPad", 290),
		rule("iPad mini", "^/ipad-mini/", "iPad", 280),
		rule("iPad General", "^/ipad/", "iPad", 270),
		rule("Buy iPad", "/buy_ipad", "iPad", 260),

		// Watch (250-210)
		rule("Apple Watch Series 11", "^/apple-watch-series-11/", "Watch", 250),
		rule("Apple Watch Ultra 3", "^/apple-watch-ultra-3/", "Watch", 240),
		rule("Apple Watch SE 3", "^/apple-watch-se-3/", "Watch", 230),
		rule("Watch General", "^/watch/", "Watch", 220),
		rule("Buy Watch", "/buy_watch", "Watch", 210),

		// AirPods (200-170)
		rule("AirPods Pro", "^/airpods-pro/", "AirPods", 200),
		rule("AirPods 4", "^/airpods-4/", "AirPods", 190),
		rule("AirPods Max", "^/airpods-max/", "AirPods", 180),
		rule("AirPods General", "^/airpods/", "AirPods", 170),

		// TV & Home (160-140)
		rule("Apple TV 4K", "^/apple-tv-4k/", "TV & Home", 160),
		rule("HomePod mini", "^/homepod-mini/", "TV & Home", 150),
		rule("TV & Home General", "^/tv-home/", "TV & Home", 140),

		// Vision (130)
		rule("Vision Pro", "^/apple-vision-pro/", "Vision Pro", 130),

		// Homepage (100)
		rule("Homepage", "^/$", "Homepage", 100),
	}

	order++
	filters = append(filters, domain.WebFilter{
		ID:         "demo-product-default",
		Name:       "Default Product",
		Priority:   5,
		Order:      order,
		Tags:       []string{"default"},
		Conditions: []domain.WebFilterCondition{}, // always matches
		Operations: []domain.WebFilterOperation{
			{Dimension: "custom_1", Action: domain.WebFilterActionSetDefaultValue, Value: "Other"},
		},
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	})

	return filters
}
