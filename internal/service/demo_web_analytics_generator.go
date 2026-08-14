package service

import (
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/internal/domain"
)

// Demo web analytics generation.
//
// Everything here is a pure function of the options and the seed: no clock, no
// database. That is what makes a demo reset reproducible, and it is what lets
// the distributions be asserted in unit tests rather than eyeballed on a
// dashboard.
//
// Sessions are written with `channel`, `channel_group` and `custom_1` left
// empty and then classified by the workspace's own attribution rules, through
// the same call the ingest path makes. A demo that wrote pre-computed channels
// would make the Filters tab a museum piece: editing a rule and running a
// backfill would change nothing.

const (
	// The launch sits a few days back so the spike is inside every preset
	// from "Previous 7 days" up, and the comparison against the previous
	// period has something dramatic to show.
	demoLaunchDaysAgo    = 5
	demoLaunchDayFactor  = 2.5
	demoPostLaunchDays   = 3
	demoPostLaunchFactor = 1.5

	// Share of sessions belonging to a known contact. Staminads has no
	// identity at all; this is what makes the web-to-email story visible.
	demoIdentifiedShare = 0.12

	// Visitor tiers. An identified population where everyone returns as often
	// and buys as often as everyone else produces audiences that are all the
	// same size — which is the one thing a segmentation demo must not do. The
	// shares are of the contact list; the weights are relative visit frequency.
	demoVisitorAdvocateShare = 0.08
	demoVisitorRegularShare  = 0.22

	demoVisitorAdvocateWeight = 10
	demoVisitorRegularWeight  = 4
	demoVisitorCasualWeight   = 1

	demoVisitorAdvocateConversion = 3.0
	demoVisitorRegularConversion  = 1.5
	demoVisitorCasualConversion   = 0.8

	// How often an identified visitor browses the product line they care about
	// rather than whatever the weights would have given them. Below 1 so a
	// contact still wanders, which is what a real browsing history looks like.
	demoVisitorAffinityShare = 0.55

	// Signed-in visitors convert several times better than anonymous ones: a
	// saved address and payment method remove most of a checkout. It is also
	// what keeps the purchase pool viable now that every purchase in the demo
	// has to come from a real session rather than from invented order rows.
	demoIdentifiedGoalFactor = 4.0

	demoGoalAddToCartRate    = 0.04
	demoGoalCheckoutRate     = 0.40
	demoGoalPurchaseRate     = 0.50
	demoGoalMinDurationSec   = 30
	demoGoalMinScrollPercent = 20

	// The six catalogue duration multipliers compound to roughly this much on
	// average. Staminads lets them, which lands its median session at 208s and
	// its bounce rate at 4.2% — a site that does not exist. Dividing the
	// product out keeps the relative weights (an iPad session really is longer
	// than a phone one) while the absolute numbers stay believable.
	demoDurationMultiplierNorm = 2.35

	// Median engaged time on a page, and the spread around it. A log-normal
	// keeps the long tail real traffic has without the symmetric nonsense of a
	// normal distribution around a 22-second median.
	demoPageDurationMedianSec = 22.0
	demoPageDurationSigma     = 0.95
	demoPageDurationMaxSec    = 600 // the SDK's own per-page active-time cap

	// Share of single-pageview sessions that are a bounce rather than a short
	// read. Engaged time is genuinely bimodal — someone who lands on the wrong
	// page leaves in seconds, someone who lands on the right one stays — and a
	// single distribution puts the bounce rate wherever its tail happens to
	// fall, which is how Staminads ends up reporting 4%. Combined with the 38%
	// of sessions that are single-page, this targets a bounce rate near 30%.
	demoBounceShareOfSinglePage = 0.78
	demoBounceMaxSeconds        = 9

	demoSDKVersion = "1.0.0"
)

// demoPageviewBuckets is the pageview-count distribution. Staminads emits one
// pageview 76% of the time and two the rest — never three — which pins
// pages/session at 1.24, makes every exit path equal its landing path, and
// leaves the pages table a copy of the sessions table.
var demoPageviewBuckets = []struct {
	Min, Max, Weight int
}{
	{1, 1, 38},
	{2, 2, 22},
	{3, 3, 14},
	{4, 6, 17},
	{7, 12, 7},
	{13, 25, 2},
}

// demoIdentity is a contact the generator may attribute visits to, with the
// date they became one. The date is not decoration: a contact drawer that shows
// a purchase from before the contact existed, sorted below their own "Contact
// created" entry, is the kind of detail that makes a demo look generated.
type demoIdentity struct {
	Email      string
	KnownSince time.Time
}

type demoWebAnalyticsOptions struct {
	Sessions int
	Days     int
	Now      time.Time
	Seed     int64
	// Contacts available as visitor identities. May be empty.
	Identities []demoIdentity
	// The workspace's attribution rules, applied exactly as ingest applies them.
	Filters []domain.WebFilter
	// Site the demo traffic lands on, e.g. "https://www.apple.com".
	SiteURL string
}

type demoWebAnalyticsBatch struct {
	Sessions []*domain.WebSession
	Pages    []*domain.WebPage
	Goals    []*domain.WebGoal
}

// demoVisitor is one identified person.
//
// Everything on it is settled once and reused for every visit they make, which
// is what turns a scatter of sessions into a journey: the same contact, on the
// same Mac, from the same city, coming back to the same product line week after
// week. Redrawing these per session — which is what the generator used to do —
// produced contacts who browsed from New York on macOS and from Tokyo on
// Android within the same hour, and a contact drawer that read as noise.
type demoVisitor struct {
	Email       string
	Geo         demoGeo
	Device      demoDevice
	ProductLine string

	// KnownSince is when they became a contact. No visit before it can be
	// attributed to them — they were a stranger to the site until then.
	KnownSince time.Time

	// VisitWeight is how often this person comes back, relative to the others.
	VisitWeight int
	// ConversionFactor scales their whole funnel, so the advocates and the
	// window shoppers do not end up in the same audiences.
	ConversionFactor float64
}

type demoWebAnalyticsGenerator struct {
	opts        demoWebAnalyticsOptions
	rng         *rand.Rand
	firstDay    time.Time // midnight UTC of the oldest generated day
	dailyCounts []int
	launchIndex int
	landingHost string

	// visitors, oldest contact first; visitorWeights[i] is the summed VisitWeight
	// of visitors[0..i], so an eligibility prefix has a weight total in O(1).
	visitors       []demoVisitor
	visitorWeights []int
	pagesByLine    map[string][]demoPage
	productLines   []demoProductLine
}

func newDemoWebAnalyticsGenerator(opts demoWebAnalyticsOptions) *demoWebAnalyticsGenerator {
	g := &demoWebAnalyticsGenerator{
		opts:         opts,
		rng:          rand.New(rand.NewSource(opts.Seed)),
		firstDay:     opts.Now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -(opts.Days - 1)),
		launchIndex:  opts.Days - 1 - demoLaunchDaysAgo,
		landingHost:  demoHostFromURL(opts.SiteURL),
		pagesByLine:  demoPagesByProductLine(),
		productLines: demoProductLines(),
	}
	g.dailyCounts = g.computeDailyCounts()
	g.buildVisitors()
	return g
}

// buildVisitors turns the contact list into people. Drawn from the seeded
// generator like everything else, so the same reset produces the same
// population and a screenshot of a contact's history still matches next month.
//
// The result is ordered oldest contact first, with a running weight total
// alongside it, so a visit only has to consider the contacts that already
// existed when it happened — see pickVisitor.
func (g *demoWebAnalyticsGenerator) buildVisitors() {
	g.visitors = make([]demoVisitor, 0, len(g.opts.Identities))
	for _, identity := range g.opts.Identities {
		if identity.Email == "" {
			continue
		}
		visitor := demoVisitor{
			Email:       identity.Email,
			KnownSince:  identity.KnownSince,
			Geo:         g.pickGeo(),
			Device:      g.pickDevice(),
			ProductLine: g.pickProductLine(),
		}
		switch draw := g.rng.Float64(); {
		case draw < demoVisitorAdvocateShare:
			visitor.VisitWeight = demoVisitorAdvocateWeight
			visitor.ConversionFactor = demoVisitorAdvocateConversion
		case draw < demoVisitorAdvocateShare+demoVisitorRegularShare:
			visitor.VisitWeight = demoVisitorRegularWeight
			visitor.ConversionFactor = demoVisitorRegularConversion
		default:
			visitor.VisitWeight = demoVisitorCasualWeight
			visitor.ConversionFactor = demoVisitorCasualConversion
		}
		g.visitors = append(g.visitors, visitor)
	}

	// Ties broken by address so the order is total, not merely sorted: two
	// contacts created in the same second would otherwise leave sort.Slice free
	// to order them either way, and the whole point of the fixed seed is that it
	// does not get to choose.
	sort.Slice(g.visitors, func(i, j int) bool {
		if g.visitors[i].KnownSince.Equal(g.visitors[j].KnownSince) {
			return g.visitors[i].Email < g.visitors[j].Email
		}
		return g.visitors[i].KnownSince.Before(g.visitors[j].KnownSince)
	})

	g.visitorWeights = make([]int, len(g.visitors))
	running := 0
	for i, visitor := range g.visitors {
		running += visitor.VisitWeight
		g.visitorWeights[i] = running
	}
}

// Days returns the number of day buckets, oldest first.
func (g *demoWebAnalyticsGenerator) Days() int { return len(g.dailyCounts) }

// DayStart returns midnight UTC of the given day bucket.
func (g *demoWebAnalyticsGenerator) DayStart(index int) time.Time {
	return g.firstDay.AddDate(0, 0, index)
}

// computeDailyCounts spreads the session budget over the window: a linear
// growth trend so the year-over-year chart has a story, weekday seasonality so
// the heat map does, and a launch spike. The last day absorbs the rounding
// remainder so the totals land exactly on the requested budget.
func (g *demoWebAnalyticsGenerator) computeDailyCounts() []int {
	weights := make([]float64, g.opts.Days)
	total := 0.0

	for day := 0; day < g.opts.Days; day++ {
		date := g.firstDay.AddDate(0, 0, day)
		weight := demoDayOfWeekWeights[int(date.Weekday())]

		// 0.85 → 1.15 across the window.
		progress := float64(day) / math.Max(float64(g.opts.Days-1), 1)
		weight *= 0.85 + progress*0.30

		switch offset := day - g.launchIndex; {
		case offset == 0:
			weight *= demoLaunchDayFactor
		case offset > 0 && offset <= demoPostLaunchDays:
			weight *= demoPostLaunchFactor
		}

		weights[day] = weight
		total += weight
	}

	counts := make([]int, g.opts.Days)
	assigned := 0
	for day := 0; day < g.opts.Days-1; day++ {
		counts[day] = int(math.Round(float64(g.opts.Sessions) * weights[day] / total))
		assigned += counts[day]
	}
	counts[g.opts.Days-1] = g.opts.Sessions - assigned
	if counts[g.opts.Days-1] < 0 {
		counts[g.opts.Days-1] = 0
	}
	return counts
}

// launchPeriod reports how a day relates to the launch, which shifts both
// traffic volume and intent.
func (g *demoWebAnalyticsGenerator) launchPeriod(day int) string {
	switch offset := day - g.launchIndex; {
	case offset == 0:
		return "launch"
	case offset > 0 && offset <= demoPostLaunchDays:
		return "post"
	default:
		return "normal"
	}
}

// GenerateDay builds every row for one day bucket. Callers flush a day (or a
// month of days) at a time so peak memory stays bounded by the batch rather
// than by the whole run.
func (g *demoWebAnalyticsGenerator) GenerateDay(day int) demoWebAnalyticsBatch {
	count := g.dailyCounts[day]
	batch := demoWebAnalyticsBatch{
		Sessions: make([]*domain.WebSession, 0, count),
		Pages:    make([]*domain.WebPage, 0, count*3),
	}

	period := g.launchPeriod(day)
	midnight := g.DayStart(day)

	for i := 0; i < count; i++ {
		session, pages, goals := g.generateSession(midnight, period)
		if session == nil {
			continue
		}
		batch.Sessions = append(batch.Sessions, session)
		batch.Pages = append(batch.Pages, pages...)
		batch.Goals = append(batch.Goals, goals...)
	}
	return batch
}

func (g *demoWebAnalyticsGenerator) generateSession(
	midnight time.Time,
	period string,
) (*domain.WebSession, []*domain.WebPage, []*domain.WebGoal) {
	// Identity comes first, because an identified visit belongs to a person and
	// that person owns the device it is made on and the place it is made from.
	// Deciding those afterwards is what used to scatter one contact across
	// continents. It also settles identity genuinely before the attribution
	// rules run, which the previous ordering claimed but did not do.
	//
	// Eligibility is judged on the day rather than the exact start, because the
	// start is not known yet: it is sampled in the visitor's own timezone, which
	// the visitor has not been chosen to supply. A day's granularity is ample for
	// a signup date.
	visitor := g.pickVisitor(midnight)

	var geo demoGeo
	var device demoDevice
	if visitor != nil {
		geo, device = visitor.Geo, visitor.Device
	} else {
		geo, device = g.pickGeo(), g.pickDevice()
	}
	start := g.sessionStart(midnight, geo)

	// A session sampled into the future (today's remaining hours) has not
	// happened yet.
	if start.After(g.opts.Now) {
		return nil, nil, nil
	}

	landing := g.pickPageFor(visitor, period)
	campaign, referrer := g.pickAcquisition(period)

	attribution := g.buildAttribution(landing, campaign, referrer, device, geo)
	if visitor != nil {
		// Copied rather than aliased: the visitor outlives every session it
		// appears in, and thousands of rows sharing one *string is an aliasing
		// hazard for the sake of a few kilobytes.
		email := visitor.Email
		attribution.ContactEmail = &email
	}

	pageCount := g.pickPageviewCount()
	pages, sessionDuration, medianPageDuration, maxScroll := g.buildPages(
		landing, visitor, campaign, referrer, device, geo, start, pageCount,
	)

	// Custom slots the generator owns. custom_1 is deliberately left for the
	// product-category rules to fill.
	attribution.Custom[1] = "anonymous"
	if attribution.ContactEmail != nil {
		attribution.Custom[1] = "logged_in"
	}
	attribution.Custom[2] = "control"
	if g.rng.Float64() < 0.5 {
		attribution.Custom[2] = "variant-b"
	}

	attribution.applyFilterResult(
		domain.EvaluateWebFilters(g.opts.Filters, attribution.filterFields(attribution.LandingPath)),
	)

	sessionID := g.sessionID(start)
	sessionDate := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	exitPath := pages[len(pages)-1].Path

	session := &domain.WebSession{
		SessionDate: sessionDate,
		ID:          sessionID,
		BeatSeq:     int64(len(pages)),
		CreatedAt:   start,
		UpdatedAt:   pages[len(pages)-1].ExitedAt,

		DurationMs:           sessionDuration.Milliseconds(),
		PageviewCount:        len(pages),
		MedianPageDurationMs: medianPageDuration.Milliseconds(),
		MaxScroll:            maxScroll,

		ExitPath:    exitPath,
		LandingPage: attribution.LandingPage, LandingDomain: attribution.LandingDomain,
		LandingPath: attribution.LandingPath,
		Referrer:    attribution.Referrer, ReferrerDomain: attribution.ReferrerDomain,
		ReferrerPath: attribution.ReferrerPath, IsDirect: attribution.IsDirect,

		UTMSource: attribution.UTMSource, UTMMedium: attribution.UTMMedium,
		UTMCampaign: attribution.UTMCampaign, UTMTerm: attribution.UTMTerm,
		UTMContent: attribution.UTMContent, UTMID: attribution.UTMID,
		UTMIDFrom: attribution.UTMIDFrom,

		Channel: attribution.Channel, ChannelGroup: attribution.ChannelGroup,
		Custom1: attribution.Custom[0], Custom2: attribution.Custom[1], Custom3: attribution.Custom[2],
		Custom4: attribution.Custom[3], Custom5: attribution.Custom[4], Custom6: attribution.Custom[5],
		Custom7: attribution.Custom[6], Custom8: attribution.Custom[7], Custom9: attribution.Custom[8],
		Custom10: attribution.Custom[9],

		ScreenWidth: attribution.ScreenWidth, ScreenHeight: attribution.ScreenHeight,
		ViewportWidth: attribution.ViewportWidth, ViewportHeight: attribution.ViewportHeight,
		Device: attribution.Device, Browser: attribution.Browser,
		BrowserType: attribution.BrowserType, OS: attribution.OS,
		UserAgent: attribution.UserAgent, ConnectionType: attribution.ConnType,
		Language: attribution.Language, Timezone: attribution.Timezone,

		Country: geo.Country, Region: geo.Region, City: geo.City,
		Latitude: floatPtr(geo.Latitude), Longitude: floatPtr(geo.Longitude),

		SDKVersion:   demoSDKVersion,
		ContactEmail: attribution.ContactEmail,
	}

	for _, page := range pages {
		page.SessionDate = sessionDate
		page.SessionID = sessionID
		page.ContactEmail = attribution.ContactEmail
	}

	goals := g.buildGoals(session, attribution, visitor, landing, pages, sessionDuration, maxScroll, period)
	session.GoalCount = len(goals)
	for _, goal := range goals {
		session.GoalValue += goal.GoalValue
	}

	return session, pages, goals
}

// buildAttribution assembles the raw, unclassified session context: the values
// a browser would actually have sent.
func (g *demoWebAnalyticsGenerator) buildAttribution(
	landing demoPage,
	campaign *demoCampaign,
	referrer demoReferrer,
	device demoDevice,
	geo demoGeo,
) *webAttribution {
	attribution := &webAttribution{
		LandingPage:   g.opts.SiteURL + landing.Path,
		LandingDomain: g.landingHost,
		LandingPath:   landing.Path,

		ScreenWidth:  device.ScreenWidth,
		ScreenHeight: device.ScreenHeight,
		// Viewports are a fraction of the screen: browser chrome and, on
		// desktop, a window that is rarely maximised.
		ViewportWidth:  int(float64(device.ScreenWidth) * (0.90 + g.rng.Float64()*0.10)),
		ViewportHeight: int(float64(device.ScreenHeight) * (0.75 + g.rng.Float64()*0.20)),

		Device: device.Device, Browser: device.Browser, OS: device.OS,
		UserAgent: device.UserAgent,
		ConnType:  g.pickConnectionType(device.Device),
		Language:  geo.Language, Timezone: geo.Timezone,
	}

	if campaign != nil {
		attribution.UTMSource = campaign.Source
		attribution.UTMMedium = campaign.Medium
		attribution.UTMCampaign = campaign.Campaign
		attribution.UTMContent = campaign.Content
		attribution.UTMTerm = campaign.Term
		if campaign.ClickIDParam != "" {
			attribution.UTMIDFrom = campaign.ClickIDParam
			attribution.UTMID = g.clickID()
		}
	}

	if referrer.Domain == "" {
		attribution.IsDirect = true
	} else {
		path := referrer.Path
		if path == "" {
			path = "/"
		}
		attribution.Referrer = "https://www." + referrer.Domain + path
		attribution.ReferrerDomain = "www." + referrer.Domain
		attribution.ReferrerPath = path
	}

	return attribution
}

// buildPages walks the session's pageviews, returning them with the session
// aggregates derived from them rather than invented alongside them — a
// TimeScore that disagrees with the pages table is worse than no TimeScore.
func (g *demoWebAnalyticsGenerator) buildPages(
	landing demoPage,
	visitor *demoVisitor,
	campaign *demoCampaign,
	referrer demoReferrer,
	device demoDevice,
	geo demoGeo,
	start time.Time,
	pageCount int,
) (pages []*domain.WebPage, sessionDuration, medianPageDuration time.Duration, maxScroll int) {
	multiplier := landing.DurationMultiplier * referrer.DurationMultiplier *
		device.DurationMultiplier * geo.DurationMultiplier *
		demoHourMultipliers[start.In(mustLoadLocation(geo.Timezone)).Hour()]
	if campaign != nil {
		multiplier *= campaign.DurationMultiplier
	}
	multiplier /= demoDurationMultiplierNorm

	cursor := start
	durations := make([]time.Duration, 0, pageCount)
	path := landing.Path
	bounces := pageCount == 1 && g.rng.Float64() < demoBounceShareOfSinglePage

	for number := 1; number <= pageCount; number++ {
		duration := g.pageDuration(multiplier)
		if bounces {
			duration = time.Duration(1+g.rng.Intn(demoBounceMaxSeconds)) * time.Second
		}
		entered := cursor
		exited := entered.Add(duration)

		// Scroll only ever grows within a session: a visitor who reached 80%
		// of one page has seen that much of the visit.
		scroll := g.pageScroll(duration)
		if scroll > maxScroll {
			maxScroll = scroll
		}

		entryType := "navigation"
		if number > 1 {
			entryType = "spa"
			// Still the visitor's own taste: someone who came for the iPhone
			// mostly keeps browsing iPhones, which is what makes a pageview
			// history on the contact drawer read as a person shopping.
			path = g.pickPageFor(visitor, "normal").Path
		}

		pages = append(pages, &domain.WebPage{
			PageNumber: number,
			BeatSeq:    int64(number),
			Path:       path,
			EnteredAt:  entered,
			ExitedAt:   exited,
			DurationMs: duration.Milliseconds(),
			MaxScroll:  scroll,
			IsLanding:  number == 1,
			IsExit:     number == pageCount,
			EntryType:  entryType,
		})

		durations = append(durations, duration)
		sessionDuration += duration
		cursor = exited
	}

	return pages, sessionDuration, medianDuration(durations), maxScroll
}

// buildGoals runs the purchase funnel. Eligibility mirrors Staminads: only a
// product page, only an engaged visit. The channel term is an addition — a
// demo where paid and email traffic converts better than social gives the goal
// drawer's breakdowns something to say.
func (g *demoWebAnalyticsGenerator) buildGoals(
	session *domain.WebSession,
	attribution *webAttribution,
	visitor *demoVisitor,
	landing demoPage,
	pages []*domain.WebPage,
	sessionDuration time.Duration,
	maxScroll int,
	period string,
) []*domain.WebGoal {
	if landing.Category != "product" {
		return nil
	}
	if sessionDuration < demoGoalMinDurationSec*time.Second || maxScroll < demoGoalMinScrollPercent {
		return nil
	}
	product, ok := demoPriceForPath(landing.Path)
	if !ok {
		return nil
	}

	rate := demoGoalAddToCartRate * g.launchGoalFactor(period) *
		g.channelGoalFactor(attribution.ChannelGroup) * g.identityGoalFactor(visitor)
	engagement := math.Min(sessionDuration.Seconds()/300, 1.0)
	rate *= 1 + engagement*0.5 + float64(maxScroll)/100*0.3

	if g.rng.Float64() >= rate {
		return nil
	}

	price := demoPriceFor(product, g.rng)
	pageNumber := len(pages)
	goalPath := landing.Path

	cartAt := session.CreatedAt.Add(time.Duration(float64(sessionDuration) * (0.4 + g.rng.Float64()*0.2)))
	goals := []*domain.WebGoal{
		g.newGoal(session, "add_to_cart", domain.GoalTypeOther, cartAt, price, goalPath, pageNumber, product.Name),
	}
	if g.rng.Float64() >= demoGoalCheckoutRate {
		return goals
	}

	checkoutAt := cartAt.Add(time.Duration(5+g.rng.Intn(10)) * time.Second)
	goals = append(goals,
		g.newGoal(session, "checkout_start", domain.GoalTypeOther, checkoutAt, 0, goalPath, pageNumber, product.Name))
	if g.rng.Float64() >= demoGoalPurchaseRate {
		return goals
	}

	purchaseAt := checkoutAt.Add(time.Duration(30+g.rng.Intn(90)) * time.Second)
	goals = append(goals,
		g.newGoal(session, "purchase", domain.GoalTypePurchase, purchaseAt, price, goalPath, pageNumber, product.Name))
	return goals
}

// identityGoalFactor is the demo's argument for identifying visitors at all: a
// signed-in customer converts several times better than an anonymous one, and
// the per-person factor on top separates the advocates from the window
// shoppers. Without the second term every identified contact would end up in
// exactly the same audiences as every other.
func (g *demoWebAnalyticsGenerator) identityGoalFactor(visitor *demoVisitor) float64 {
	if visitor == nil {
		return 1
	}
	return demoIdentifiedGoalFactor * visitor.ConversionFactor
}

func (g *demoWebAnalyticsGenerator) newGoal(
	session *domain.WebSession,
	name string,
	goalType string,
	at time.Time,
	value float64,
	path string,
	pageNumber int,
	product string,
) *domain.WebGoal {
	return &domain.WebGoal{
		SessionDate: session.SessionDate,
		SessionID:   session.ID,
		GoalName:    name,
		GoalType:    goalType,
		ClientTsMs:  at.UnixMilli(),
		BeatSeq:     session.BeatSeq,
		GoalAt:      at,
		GoalValue:   value,
		Path:        path,
		PageNumber:  pageNumber,
		Properties:  map[string]string{"product": product},

		Referrer: session.Referrer, ReferrerDomain: session.ReferrerDomain,
		ReferrerPath: session.ReferrerPath, IsDirect: session.IsDirect,
		LandingPage: session.LandingPage, LandingDomain: session.LandingDomain,
		LandingPath: session.LandingPath,
		UTMSource:   session.UTMSource, UTMMedium: session.UTMMedium,
		UTMCampaign: session.UTMCampaign, UTMTerm: session.UTMTerm,
		UTMContent: session.UTMContent, UTMID: session.UTMID, UTMIDFrom: session.UTMIDFrom,
		Channel: session.Channel, ChannelGroup: session.ChannelGroup,
		Custom1: session.Custom1, Custom2: session.Custom2, Custom3: session.Custom3,
		Device: session.Device, Browser: session.Browser, BrowserType: session.BrowserType,
		OS: session.OS, ConnectionType: session.ConnectionType,
		Language: session.Language, Timezone: session.Timezone,
		Country: session.Country, Region: session.Region, City: session.City,
		ContactEmail: session.ContactEmail,
	}
}

func (g *demoWebAnalyticsGenerator) launchGoalFactor(period string) float64 {
	switch period {
	case "launch":
		return 1.5
	case "post":
		return 1.2
	default:
		return 1.0
	}
}

// channelGoalFactor makes intent visible: someone arriving from a campaign they
// opted into converts better than someone who clicked a social post.
func (g *demoWebAnalyticsGenerator) channelGoalFactor(channelGroup string) float64 {
	switch channelGroup {
	case "email":
		return 1.8
	case "search-paid":
		return 1.3
	case "search-organic":
		return 1.1
	case "social-paid", "social-organic":
		return 0.7
	default:
		return 1.0
	}
}

// ---------------------------------------------------------------- sampling

func (g *demoWebAnalyticsGenerator) sessionStart(midnight time.Time, geo demoGeo) time.Time {
	location := mustLoadLocation(geo.Timezone)
	// The hour curve describes a visitor's own day, so it is sampled in their
	// timezone and converted back. Sampling in UTC would put a European
	// evening peak in the middle of an American night.
	local := midnight.In(location)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	return day.
		Add(time.Duration(g.pickHour()) * time.Hour).
		Add(time.Duration(g.rng.Intn(60)) * time.Minute).
		Add(time.Duration(g.rng.Intn(60)) * time.Second).
		UTC()
}

func (g *demoWebAnalyticsGenerator) pickHour() int {
	total := 0.0
	for _, weight := range demoHourMultipliers {
		total += weight
	}
	target := g.rng.Float64() * total
	for hour, weight := range demoHourMultipliers {
		target -= weight
		if target <= 0 {
			return hour
		}
	}
	return 23
}

func (g *demoWebAnalyticsGenerator) pickPageviewCount() int {
	total := 0
	for _, bucket := range demoPageviewBuckets {
		total += bucket.Weight
	}
	target := g.rng.Intn(total)
	for _, bucket := range demoPageviewBuckets {
		target -= bucket.Weight
		if target < 0 {
			if bucket.Max == bucket.Min {
				return bucket.Min
			}
			return bucket.Min + g.rng.Intn(bucket.Max-bucket.Min+1)
		}
	}
	return 1
}

func (g *demoWebAnalyticsGenerator) pageDuration(multiplier float64) time.Duration {
	seconds := math.Exp(math.Log(demoPageDurationMedianSec)+g.rng.NormFloat64()*demoPageDurationSigma) * multiplier
	seconds = math.Max(1, math.Min(seconds, demoPageDurationMaxSec))
	// Whole milliseconds, which is the resolution the SDK reports — and what
	// keeps a session's duration exactly equal to the sum of its pages rather
	// than a few milliseconds above it, once every value has been truncated.
	return time.Duration(math.Round(seconds*1000)) * time.Millisecond
}

// pageScroll bands scroll depth by how long the visitor stayed, which is the
// correlation the heat map and the median-scroll metric exist to show.
func (g *demoWebAnalyticsGenerator) pageScroll(duration time.Duration) int {
	seconds := duration.Seconds()
	switch {
	case seconds < 15:
		return 5 + g.rng.Intn(21)
	case seconds < 60:
		return 20 + g.rng.Intn(31)
	case seconds < 180:
		return 40 + g.rng.Intn(36)
	case seconds < 600:
		return 60 + g.rng.Intn(31)
	default:
		return 80 + g.rng.Intn(21)
	}
}

// pickPageFor chooses a page for a visit, honouring the visitor's taste when
// there is a visitor. The launch window outranks personal taste on purpose:
// everybody looks at the new phone the week it ships, which is the whole point
// of having a spike in the data.
func (g *demoWebAnalyticsGenerator) pickPageFor(visitor *demoVisitor, period string) demoPage {
	if page, ok := g.pickLaunchPage(period); ok {
		return page
	}
	if visitor != nil && g.rng.Float64() < demoVisitorAffinityShare {
		if pages := g.pagesByLine[visitor.ProductLine]; len(pages) > 0 {
			return pickWeighted(g.rng, pages, func(p demoPage) int { return p.Weight })
		}
	}
	return pickWeighted(g.rng, demoPages, func(p demoPage) int { return p.Weight })
}

func (g *demoWebAnalyticsGenerator) pickLaunchPage(period string) (demoPage, bool) {
	if len(demoLaunchPages) == 0 {
		return demoPage{}, false
	}
	switch period {
	case "launch":
		if g.rng.Float64() < 0.7 {
			return pickWeighted(g.rng, demoLaunchPages, func(p demoPage) int { return p.Weight }), true
		}
	case "post":
		if g.rng.Float64() < 0.5 {
			return pickWeighted(g.rng, demoLaunchPages, func(p demoPage) int { return p.Weight }), true
		}
	}
	return demoPage{}, false
}

// pickProductLine gives a visitor something to care about, weighted by how much
// traffic the line gets — so the demo's iPhone shoppers outnumber its Vision Pro
// shoppers the way the page weights say they should.
func (g *demoWebAnalyticsGenerator) pickProductLine() string {
	if len(g.productLines) == 0 {
		return ""
	}
	return pickWeighted(g.rng, g.productLines, func(l demoProductLine) int { return l.Weight }).Name
}

func (g *demoWebAnalyticsGenerator) pickDevice() demoDevice {
	return pickWeighted(g.rng, demoDevices, func(d demoDevice) int { return d.Weight })
}

func (g *demoWebAnalyticsGenerator) pickGeo() demoGeo {
	return pickWeighted(g.rng, demoGeos, func(geo demoGeo) int { return geo.Weight })
}

// pickAcquisition decides how the visitor arrived. A campaign implies its own
// referrer — someone who clicked a Google ad arrives from Google — so the two
// are chosen together rather than independently.
func (g *demoWebAnalyticsGenerator) pickAcquisition(period string) (*demoCampaign, demoReferrer) {
	campaignWeight := 0
	for _, campaign := range demoCampaigns {
		campaignWeight += campaign.Weight
	}

	if g.rng.Intn(campaignWeight+demoNoUTMWeight) < demoNoUTMWeight {
		return nil, g.pickReferrer(period)
	}

	var campaign demoCampaign
	if len(demoLaunchCampaigns) > 0 && (period == "launch" || period == "post") && g.rng.Float64() < 0.6 {
		campaign = pickWeighted(g.rng, demoLaunchCampaigns, func(c demoCampaign) int { return c.Weight })
	} else {
		campaign = pickWeighted(g.rng, demoCampaigns, func(c demoCampaign) int { return c.Weight })
	}
	return &campaign, demoReferrerForSource(campaign.Source)
}

func (g *demoWebAnalyticsGenerator) pickReferrer(period string) demoReferrer {
	if len(demoTechNewsReferrers) > 0 {
		switch period {
		case "launch":
			if g.rng.Float64() < 0.4 {
				return pickWeighted(g.rng, demoTechNewsReferrers, func(r demoReferrer) int { return r.Weight })
			}
		case "post":
			if g.rng.Float64() < 0.25 {
				return pickWeighted(g.rng, demoTechNewsReferrers, func(r demoReferrer) int { return r.Weight })
			}
		}
	}
	return pickWeighted(g.rng, demoReferrers, func(r demoReferrer) int { return r.Weight })
}

func (g *demoWebAnalyticsGenerator) pickConnectionType(device string) string {
	// Desktops report nothing useful; the Network Information API is a mobile
	// affair in practice.
	if device == "desktop" || g.rng.Float64() < 0.3 {
		return ""
	}
	switch draw := g.rng.Float64(); {
	case draw < 0.85:
		return "4g"
	case draw < 0.95:
		return "3g"
	case draw < 0.98:
		return "2g"
	default:
		return "slow-2g"
	}
}

// pickVisitor decides whether a visit on the given day belongs to a known
// contact, and to which one.
//
// Only contacts that already existed that day are eligible, so the identified
// share of traffic grows across the window the way a mailing list does — and no
// contact is ever shown browsing before they signed up.
//
// The draw is weighted by visit frequency rather than uniform: a uniform pick
// gives every contact the same history, and an audience where everybody looks
// alike cannot demonstrate segmentation.
func (g *demoWebAnalyticsGenerator) pickVisitor(day time.Time) *demoVisitor {
	if len(g.visitors) == 0 || g.rng.Float64() >= demoIdentifiedShare {
		return nil
	}

	// visitors are ordered by KnownSince, so the eligible ones are a prefix.
	eligible := sort.Search(len(g.visitors), func(i int) bool {
		return g.visitors[i].KnownSince.After(day)
	})
	if eligible == 0 {
		return nil // nobody had signed up yet
	}

	total := g.visitorWeights[eligible-1]
	if total <= 0 {
		return nil
	}
	target := g.rng.Intn(total)
	index := sort.Search(eligible, func(i int) bool { return g.visitorWeights[i] > target })
	if index >= eligible {
		index = eligible - 1
	}
	return &g.visitors[index]
}

func (g *demoWebAnalyticsGenerator) clickID() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	id := make([]byte, 24)
	for i := range id {
		id[i] = alphabet[g.rng.Intn(len(alphabet))]
	}
	return string(id)
}

// sessionID builds a UUIDv7 whose embedded timestamp is the session start, so
// the id agrees with the partition the row is written to — the same invariant
// SessionDateFromUUIDv7 enforces at ingest.
func (g *demoWebAnalyticsGenerator) sessionID(start time.Time) string {
	ms := start.UnixMilli()
	var raw [16]byte
	raw[0], raw[1], raw[2] = byte(ms>>40), byte(ms>>32), byte(ms>>24)
	raw[3], raw[4], raw[5] = byte(ms>>16), byte(ms>>8), byte(ms)
	_, _ = g.rng.Read(raw[6:])
	raw[6] = (raw[6] & 0x0F) | 0x70 // version 7
	raw[8] = (raw[8] & 0x3F) | 0x80 // RFC 4122 variant
	return uuid.UUID(raw).String()
}

// ----------------------------------------------------------------- helpers

// demoProductLine is one shoppable line of the catalogue, with the traffic
// weight of every page that belongs to it.
type demoProductLine struct {
	Name   string
	Weight int
}

// demoPagesByProductLine groups the catalogue so a visitor with a taste for one
// line can be given pages from it. Pages with no line — the homepage, the shop
// redirect — belong to nobody and are left out.
func demoPagesByProductLine() map[string][]demoPage {
	byLine := map[string][]demoPage{}
	for _, page := range demoPages {
		if page.ProductLine == "" {
			continue
		}
		byLine[page.ProductLine] = append(byLine[page.ProductLine], page)
	}
	return byLine
}

// demoProductLines lists the lines with their summed page weights, sorted by
// name. The sort is not cosmetic: it is drawn from with a seeded RNG, and Go
// randomises map iteration, so an unsorted slice would make the demo different
// on every reset despite the fixed seed.
func demoProductLines() []demoProductLine {
	totals := map[string]int{}
	for _, page := range demoPages {
		if page.ProductLine == "" {
			continue
		}
		totals[page.ProductLine] += page.Weight
	}

	lines := make([]demoProductLine, 0, len(totals))
	for name, weight := range totals {
		lines = append(lines, demoProductLine{Name: name, Weight: weight})
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].Name < lines[j].Name })
	return lines
}

func pickWeighted[T any](rng *rand.Rand, items []T, weight func(T) int) T {
	total := 0
	for _, item := range items {
		total += weight(item)
	}
	var zero T
	if total <= 0 || len(items) == 0 {
		return zero
	}
	target := rng.Intn(total)
	for _, item := range items {
		target -= weight(item)
		if target < 0 {
			return item
		}
	}
	return items[len(items)-1]
}

func medianDuration(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

func floatPtr(v float64) *float64 { return &v }

var demoLocationCache = map[string]*time.Location{}

// mustLoadLocation falls back to UTC rather than failing a demo reset over a
// timezone name; the catalogue test is what guarantees they all resolve.
func mustLoadLocation(name string) *time.Location {
	if location, ok := demoLocationCache[name]; ok {
		return location
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		location = time.UTC
	}
	demoLocationCache[name] = location
	return location
}

func demoHostFromURL(siteURL string) string {
	host := siteURL
	for _, prefix := range []string{"https://", "http://"} {
		if len(host) >= len(prefix) && host[:len(prefix)] == prefix {
			host = host[len(prefix):]
			break
		}
	}
	if index := indexByte(host, '/'); index >= 0 {
		host = host[:index]
	}
	return host
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// demoReferrerForSource maps a campaign's source to the referrer a click on it
// would actually carry. Sampling the two independently is what produces demo
// data where a Google Ads session arrives from Instagram.
func demoReferrerForSource(source string) demoReferrer {
	switch source {
	case "google":
		return demoReferrer{Domain: "google.com", Path: "/search", Category: "search", DurationMultiplier: 1.2}
	case "facebook":
		return demoReferrer{Domain: "facebook.com", Category: "social", DurationMultiplier: 0.8}
	case "instagram":
		return demoReferrer{Domain: "instagram.com", Category: "social", DurationMultiplier: 0.7}
	case "twitter":
		return demoReferrer{Domain: "twitter.com", Category: "social", DurationMultiplier: 0.75}
	case "email":
		// A click from an email client carries no referrer, which is exactly
		// why the email campaign rules key on the UTM rather than the source.
		return demoReferrer{Category: "direct", DurationMultiplier: 1.5}
	default:
		return demoReferrer{Category: "direct", DurationMultiplier: 1.0}
	}
}
