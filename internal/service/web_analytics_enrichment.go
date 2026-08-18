package service

import (
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/geoip"
)

// Enrichment is the pure part of the ingest pipeline: it turns one validated
// beat (which carries the whole cumulative session) into the final
// session/pages/goals rows. It replaces Staminads' three ClickHouse
// materialized views plus the server-side handler enrichment, with one decided
// divergence: session duration is the SUM of per-page focus time (the SDK's
// intent) instead of the view's max().

// webClockSkewThreshold: client clocks within this bound are trusted as-is.
const webClockSkewThreshold = 5 * time.Second

// webClockSkew computes the correction to apply to client timestamps, from
// the sent_at the SDK stamps at every HTTP attempt.
func webClockSkew(sentAt *int64, receivedAt time.Time) time.Duration {
	if sentAt == nil || *sentAt <= 0 {
		return 0
	}
	skew := receivedAt.Sub(time.UnixMilli(*sentAt))
	if skew > -webClockSkewThreshold && skew < webClockSkewThreshold {
		return 0
	}
	return skew
}

// correctedMs shifts a client timestamp by the measured skew. Apply it ONLY to
// values that are read as times — created_at, updated_at, goal_at. Never to a
// partition key or a dedup key: session_date is derived from the session id
// itself and web_goals dedups on the raw client_ts_ms, both deliberately. Skew
// is recomputed per beat from that beat's sent_at, so correcting a key would let
// the same session or goal land in different rows on different beats, splitting
// one visit in two with nothing to flag it.
func correctedMs(epochMs int64, skew time.Duration) time.Time {
	return time.UnixMilli(epochMs).Add(skew).UTC()
}

// webURLParts extracts the lowercase hostname and the path of a URL the way
// the browser's URL API does (root path is "/"). Unparseable input yields
// empty parts.
func webURLParts(raw string) (domain string, path string) {
	if strings.TrimSpace(raw) == "" {
		return "", ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", ""
	}
	path = u.Path
	if path == "" {
		path = "/"
	}
	return strings.ToLower(u.Hostname()), path
}

// webHostname returns just the lowercase hostname of a URL (used for the
// Origin/Referer allowed-domains check).
func webHostname(raw string) string {
	host, _ := webURLParts(raw)
	return host
}

// boundDimension caps a client-supplied dimension value and applies a default
// when the SDK omitted it.
func boundDimension(value, fallback string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return fallback
	}
	if len(v) > 200 {
		return v[:200]
	}
	return v
}

// applyWebGeo maps a raw lookup through the workspace privacy knobs.
func applyWebGeo(result geoip.Result, settings *domain.WebAnalyticsSettings) domain.WebGeoResult {
	if settings != nil && !settings.GeoEnabled {
		return domain.WebGeoResult{}
	}
	out := domain.WebGeoResult{Country: result.Country}
	storeRegion, storeCity := true, true
	if settings != nil {
		storeRegion, storeCity = settings.GeoStoreRegion, settings.GeoStoreCity
	}
	// Not settings.GeoCoordsPrecision directly: a coordinate is a place name in
	// another form, so it is capped by the finest name actually stored. See
	// EffectiveGeoCoordsPrecision.
	precision := settings.EffectiveGeoCoordsPrecision()
	if storeRegion {
		out.Region = result.Region
	}
	if storeCity {
		out.City = result.City
	}
	if result.Latitude != nil && result.Longitude != nil {
		lat := geoip.RoundCoord(*result.Latitude, precision)
		lon := geoip.RoundCoord(*result.Longitude, precision)
		out.Latitude = &lat
		out.Longitude = &lon
	}
	return out
}

// webAttribution is the session-level context shared by the session row and
// every goal snapshot after URL parsing, UA parsing, geo and filters ran.
type webAttribution struct {
	Referrer       string
	ReferrerDomain string
	ReferrerPath   string
	IsDirect       bool
	LandingPage    string
	LandingDomain  string
	LandingPath    string

	UTMSource, UTMMedium, UTMCampaign, UTMTerm, UTMContent, UTMID, UTMIDFrom string
	Channel, ChannelGroup                                                    string
	Custom                                                                   [10]string

	ScreenWidth, ScreenHeight, ViewportWidth, ViewportHeight int
	Device, Browser, BrowserType, OS, UserAgent, ConnType    string
	Language, Timezone                                       string

	Geo          domain.WebGeoResult
	ContactEmail *string
}

// dropSelfReferral blanks a referrer that points back at the very host the
// session landed on, and re-reads the visit as direct.
//
// The SDK stamps every new session with document.referrer, and it mints one
// whenever the inactivity window has lapsed — in place on a tab the visitor
// left open, or on their next internal click. Both hand it a referrer that is
// one of the site's OWN pages, so without this the visit's acquisition source
// is silently replaced by the site itself: a visitor who arrived from Google in
// the morning and came back to the tab at noon is re-credited to /compare/, and
// the row escapes the Direct rule (is_direct false) into not-mapped.
//
// It also guards the session row's merge. Attribution is sticky per column,
// first non-empty writer wins (webSessionStickyColumns), so a self-referral beat
// could otherwise overwrite the empty referrer of a genuinely direct session and
// flip is_direct with it.
//
// Exact host match, both sides already lowercased by webURLParts: docs.acme.com
// -> www.acme.com is a real referral between two hosts and must survive.
func (a *webAttribution) dropSelfReferral() {
	if a.ReferrerDomain == "" || a.ReferrerDomain != a.LandingDomain {
		return
	}
	a.Referrer = ""
	a.ReferrerDomain = ""
	a.ReferrerPath = ""
	a.IsDirect = true
}

// filterFields builds the source-field map the rules engine reads. path is
// context-dependent: the landing path for the session-level evaluation, the
// goal's own path for each goal.
func (a *webAttribution) filterFields(path string) map[string]string {
	isDirect := "false"
	if a.IsDirect {
		isDirect = "true"
	}
	return map[string]string{
		"utm_source":      a.UTMSource,
		"utm_medium":      a.UTMMedium,
		"utm_campaign":    a.UTMCampaign,
		"utm_term":        a.UTMTerm,
		"utm_content":     a.UTMContent,
		"utm_id":          a.UTMID,
		"utm_id_from":     a.UTMIDFrom,
		"referrer":        a.Referrer,
		"referrer_domain": a.ReferrerDomain,
		"referrer_path":   a.ReferrerPath,
		"is_direct":       isDirect,
		"landing_page":    a.LandingPage,
		"landing_domain":  a.LandingDomain,
		"landing_path":    a.LandingPath,
		"path":            path,
		"device":          a.Device,
		"browser":         a.Browser,
		"browser_type":    a.BrowserType,
		"os":              a.OS,
		"user_agent":      a.UserAgent,
		"connection_type": a.ConnType,
		"language":        a.Language,
		"timezone":        a.Timezone,
	}
}

// applyFilterResult writes the rule outcomes back onto the attribution.
// A nil value (unset_value) clears the dimension.
func (a *webAttribution) applyFilterResult(result domain.WebFilterResult) {
	str := func(v *string) string {
		if v == nil {
			return ""
		}
		return *v
	}
	for dimension, value := range result {
		switch dimension {
		case "channel":
			a.Channel = str(value)
		case "channel_group":
			a.ChannelGroup = str(value)
		case "utm_source":
			a.UTMSource = str(value)
		case "utm_medium":
			a.UTMMedium = str(value)
		case "utm_campaign":
			a.UTMCampaign = str(value)
		case "utm_term":
			a.UTMTerm = str(value)
		case "utm_content":
			a.UTMContent = str(value)
		case "referrer_domain":
			a.ReferrerDomain = str(value)
		case "is_direct":
			a.IsDirect = str(value) == "true"
		default:
			if strings.HasPrefix(dimension, "custom_") {
				if _, slot, ok := webCustomSlot(dimension); ok {
					a.Custom[slot-1] = str(value)
				}
			}
		}
	}
}

func webCustomSlot(key string) (string, int, bool) {
	if !domain.IsCustomDimensionKey(key) {
		return "", 0, false
	}
	slot, err := strconv.Atoi(strings.TrimPrefix(key, "custom_"))
	if err != nil || slot < 1 || slot > 10 {
		return "", 0, false
	}
	return key, slot, true
}

// BuildWebRows turns one validated beat into its final rows. geo is the raw
// lookup result (zero value when disabled/unavailable).
func BuildWebRows(payload *domain.WebTrackPayload, settings *domain.WebAnalyticsSettings, geo geoip.Result, receivedAt time.Time, contactEmail *string) (*domain.WebSession, []*domain.WebPage, []*domain.WebGoal, error) {
	sessionDate, sessionStart, err := domain.SessionDateFromUUIDv7(payload.SessionID, receivedAt)
	if err != nil {
		return nil, nil, nil, err
	}

	skew := webClockSkew(payload.SentAt, receivedAt)
	attrs := payload.Attributes
	if attrs == nil {
		attrs = &domain.WebSessionAttributes{}
	}

	attribution := webAttribution{
		Referrer:    attrs.Referrer,
		LandingPage: attrs.LandingPage,
		IsDirect:    strings.TrimSpace(attrs.Referrer) == "",
		UTMSource:   attrs.UTMSource, UTMMedium: attrs.UTMMedium, UTMCampaign: attrs.UTMCampaign,
		UTMTerm: attrs.UTMTerm, UTMContent: attrs.UTMContent, UTMID: attrs.UTMID, UTMIDFrom: attrs.UTMIDFrom,
		ScreenWidth: attrs.ScreenWidth, ScreenHeight: attrs.ScreenHeight,
		ViewportWidth: attrs.ViewportWidth, ViewportHeight: attrs.ViewportHeight,
		UserAgent: attrs.UserAgent, ConnType: attrs.ConnectionType,
		Language: attrs.Language, Timezone: attrs.Timezone,
		Geo:          applyWebGeo(geo, settings),
		ContactEmail: contactEmail,
	}
	attribution.ReferrerDomain, attribution.ReferrerPath = webURLParts(attrs.Referrer)
	attribution.LandingDomain, attribution.LandingPath = webURLParts(attrs.LandingPage)
	// Before the rules run, so they see the corrected is_direct and an empty
	// referrer_domain rather than classifying the site as its own channel.
	attribution.dropSelfReferral()

	// Device, browser and OS come from the SDK, which parses the user agent in
	// the browser with Client Hints available. That is strictly more accurate
	// than re-parsing the UA string server-side — modern browsers freeze the UA
	// and expose the real OS/version only through the Client Hints API, which
	// the server never sees. Values are bounded because they are client input.
	attribution.Device = boundDimension(attrs.Device, "desktop")
	attribution.Browser = boundDimension(attrs.Browser, "Unknown")
	attribution.OS = boundDimension(attrs.OS, "Unknown")
	attribution.BrowserType = boundDimension(attrs.BrowserType, "")

	// Custom dimensions from the payload; filter rules may overwrite below.
	for i := 1; i <= 10; i++ {
		if v, ok := payload.Dimensions["custom_"+strconv.Itoa(i)]; ok {
			attribution.Custom[i-1] = v
		}
	}

	var filters []domain.WebFilter
	if settings != nil {
		filters = settings.Filters
	}
	// Session-level rule evaluation uses the landing path as the page context;
	// goals re-evaluate below with their own path (Staminads evaluated per
	// event, and its session row kept an arbitrary event's outcome — using the
	// landing context makes the session outcome deterministic instead).
	attribution.applyFilterResult(domain.EvaluateWebFilters(filters, attribution.filterFields(attribution.LandingPath)))

	// Split actions.
	var pageviews, goalActions []domain.WebTrackAction
	for _, action := range payload.Actions {
		switch action.Type {
		case domain.WebActionTypePageview:
			pageviews = append(pageviews, action)
		case domain.WebActionTypeGoal:
			goalActions = append(goalActions, action)
		}
	}

	// The session start comes from the id, not the payload: the two would
	// otherwise be independent claims about the same fact, free to disagree
	// (a session whose start falls outside its own partition).
	createdAt := correctedMs(sessionStart.UnixMilli(), skew)
	updatedAt := correctedMs(payload.UpdatedAt, skew)

	session := &domain.WebSession{
		SessionDate: sessionDate,
		ID:          strings.ToLower(payload.SessionID),
		BeatSeq:     payload.Seq,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,

		Referrer: attribution.Referrer, ReferrerDomain: attribution.ReferrerDomain, ReferrerPath: attribution.ReferrerPath,
		IsDirect:    attribution.IsDirect,
		LandingPage: attribution.LandingPage, LandingDomain: attribution.LandingDomain, LandingPath: attribution.LandingPath,
		UTMSource: attribution.UTMSource, UTMMedium: attribution.UTMMedium, UTMCampaign: attribution.UTMCampaign,
		UTMTerm: attribution.UTMTerm, UTMContent: attribution.UTMContent, UTMID: attribution.UTMID, UTMIDFrom: attribution.UTMIDFrom,
		Channel: attribution.Channel, ChannelGroup: attribution.ChannelGroup,
		Custom1: attribution.Custom[0], Custom2: attribution.Custom[1], Custom3: attribution.Custom[2], Custom4: attribution.Custom[3], Custom5: attribution.Custom[4],
		Custom6: attribution.Custom[5], Custom7: attribution.Custom[6], Custom8: attribution.Custom[7], Custom9: attribution.Custom[8], Custom10: attribution.Custom[9],
		ScreenWidth: attribution.ScreenWidth, ScreenHeight: attribution.ScreenHeight,
		ViewportWidth: attribution.ViewportWidth, ViewportHeight: attribution.ViewportHeight,
		Device: attribution.Device, Browser: attribution.Browser, BrowserType: attribution.BrowserType, OS: attribution.OS,
		UserAgent: attribution.UserAgent, ConnectionType: attribution.ConnType,
		Language: attribution.Language, Timezone: attribution.Timezone,
		Country: attribution.Geo.Country, Region: attribution.Geo.Region, City: attribution.Geo.City,
		Latitude: attribution.Geo.Latitude, Longitude: attribution.Geo.Longitude,
		SDKVersion:   payload.SDKVersion,
		ContactEmail: attribution.ContactEmail,
	}

	// Page rows + session aggregates.
	maxPageNumber := 0
	for _, pv := range pageviews {
		if pv.PageNumber > maxPageNumber {
			maxPageNumber = pv.PageNumber
		}
	}

	var pages []*domain.WebPage
	var totalDuration int64
	var positiveDurations []int64
	maxScroll := 0
	exitPath := ""
	for _, pv := range pageviews {
		entryType := domain.WebEntryTypeNavigation
		if pv.PageNumber == 1 {
			entryType = domain.WebEntryTypeLanding
		}
		page := &domain.WebPage{
			SessionDate: sessionDate,
			SessionID:   session.ID,
			PageNumber:  pv.PageNumber,
			BeatSeq:     payload.Seq,
			Path:        pv.Path,
			EnteredAt:   correctedMs(pv.EnteredAt, skew),
			ExitedAt:    correctedMs(pv.ExitedAt, skew),
			DurationMs:  pv.Duration,
			MaxScroll:   pv.Scroll,
			// Provisional: these are this TAB's first and last page. The rollup
			// after the insert recomputes them across all of the session's tabs,
			// otherwise a three-tab visitor registers three entries and three
			// exits and inflates Entries, Exits and Exit Rate.
			IsLanding:    pv.PageNumber == 1,
			IsExit:       pv.PageNumber == maxPageNumber,
			EntryType:    entryType,
			TabID:        payload.TabID,
			ContactEmail: attribution.ContactEmail,
		}
		pages = append(pages, page)

		totalDuration += pv.Duration
		if pv.Duration > 0 {
			positiveDurations = append(positiveDurations, pv.Duration)
		}
		if pv.Scroll > maxScroll {
			maxScroll = pv.Scroll
		}
		if pv.PageNumber == maxPageNumber {
			exitPath = pv.Path
		}
	}

	session.PageviewCount = len(pageviews)
	session.DurationMs = totalDuration
	session.MedianPageDurationMs = medianInt64(positiveDurations)
	session.MaxScroll = maxScroll
	session.ExitPath = exitPath

	// Goal rows: per-goal rule evaluation with the goal's own path, and a
	// fresh copy of the attribution snapshot.
	var goals []*domain.WebGoal
	var goalValueSum float64
	for _, ga := range goalActions {
		goalAttribution := attribution
		goalAttribution.applyFilterResult(domain.EvaluateWebFilters(filters, attribution.filterFields(ga.Path)))

		goal := &domain.WebGoal{
			SessionDate: sessionDate,
			SessionID:   session.ID,
			GoalName:    ga.Name,
			ClientTsMs:  ga.Timestamp, // original, uncorrected: stable dedup across retries
			BeatSeq:     payload.Seq,
			GoalAt:      correctedMs(ga.Timestamp, skew),
			GoalValue:   ga.Value,
			GoalType:    ga.GoalType,
			Path:        ga.Path,
			PageNumber:  ga.PageNumber,
			Properties:  ga.Properties,

			Referrer: goalAttribution.Referrer, ReferrerDomain: goalAttribution.ReferrerDomain, ReferrerPath: goalAttribution.ReferrerPath,
			IsDirect:    goalAttribution.IsDirect,
			LandingPage: goalAttribution.LandingPage, LandingDomain: goalAttribution.LandingDomain, LandingPath: goalAttribution.LandingPath,
			UTMSource: goalAttribution.UTMSource, UTMMedium: goalAttribution.UTMMedium, UTMCampaign: goalAttribution.UTMCampaign,
			UTMTerm: goalAttribution.UTMTerm, UTMContent: goalAttribution.UTMContent, UTMID: goalAttribution.UTMID, UTMIDFrom: goalAttribution.UTMIDFrom,
			Channel: goalAttribution.Channel, ChannelGroup: goalAttribution.ChannelGroup,
			Custom1: goalAttribution.Custom[0], Custom2: goalAttribution.Custom[1], Custom3: goalAttribution.Custom[2], Custom4: goalAttribution.Custom[3], Custom5: goalAttribution.Custom[4],
			Custom6: goalAttribution.Custom[5], Custom7: goalAttribution.Custom[6], Custom8: goalAttribution.Custom[7], Custom9: goalAttribution.Custom[8], Custom10: goalAttribution.Custom[9],
			ScreenWidth: goalAttribution.ScreenWidth, ScreenHeight: goalAttribution.ScreenHeight,
			ViewportWidth: goalAttribution.ViewportWidth, ViewportHeight: goalAttribution.ViewportHeight,
			Device: goalAttribution.Device, Browser: goalAttribution.Browser, BrowserType: goalAttribution.BrowserType, OS: goalAttribution.OS,
			UserAgent: goalAttribution.UserAgent, ConnectionType: goalAttribution.ConnType,
			Language: goalAttribution.Language, Timezone: goalAttribution.Timezone,
			Country: goalAttribution.Geo.Country, Region: goalAttribution.Geo.Region, City: goalAttribution.Geo.City,
			Latitude: goalAttribution.Geo.Latitude, Longitude: goalAttribution.Geo.Longitude,
			TabID:        payload.TabID,
			ContactEmail: goalAttribution.ContactEmail,
		}
		goals = append(goals, goal)
		goalValueSum += ga.Value
	}

	session.GoalCount = len(goals)
	session.GoalValue = goalValueSum

	return session, pages, goals, nil
}

// normalizedContactEmail lowercases and trims a verified address so it matches
// contacts.email, which is stored normalized. Callers must verify the signature
// against the RAW address first — the customer signed what they sent, not what
// we normalize it to.
func normalizedContactEmail(email *string) *string {
	if email == nil {
		return nil
	}
	normalized := domain.NormalizeEmail(*email)
	if normalized == "" {
		return nil
	}
	return &normalized
}

func medianInt64(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]int64, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return int64(math.Round(float64(sorted[mid-1]+sorted[mid]) / 2))
}
