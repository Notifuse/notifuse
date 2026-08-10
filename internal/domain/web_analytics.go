package domain

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

//go:generate mockgen -destination mocks/mock_web_analytics_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain WebAnalyticsRepository
//go:generate mockgen -destination mocks/mock_web_analytics_service.go -package mocks github.com/Notifuse/notifuse/internal/domain WebAnalyticsService
//go:generate mockgen -destination mocks/mock_geoip_resolver.go -package mocks github.com/Notifuse/notifuse/internal/domain GeoIPResolver

const (
	// WebTrackMaxActions caps the number of actions accepted in a single beat.
	WebTrackMaxActions = 1000
	// WebTrackMaxPathLength caps URL paths sent by the SDK.
	WebTrackMaxPathLength = 2048
	// WebTrackMaxGoalNameLength caps goal names.
	WebTrackMaxGoalNameLength = 100
	// WebTrackMaxEmailLength matches contacts.email VARCHAR(255).
	WebTrackMaxEmailLength = 255
	// WebTrackMaxHMACLength bounds the hex-encoded SHA-256 credential.
	WebTrackMaxHMACLength = 64
	// WebTrackMaxIdentifyTokenLength bounds the encrypted nf_id parameter.
	WebTrackMaxIdentifyTokenLength = 512

	// Goal property bounds. These exist because actions[] is cumulative: the SDK
	// re-sends every action of the session on every beat, so an unbounded
	// properties map is carried forever and eventually pushes the serialized
	// body past webTrackMaxBodyBytes — after which EVERY later beat of that
	// session is rejected, permanently, with no client-side recovery. Bounding
	// here means an oversized map costs its own action and nothing else.
	WebTrackMaxGoalPropertyKeys        = 50
	WebTrackMaxGoalPropertyValueLength = 1024
	WebTrackMaxGoalPropertiesBytes     = 8 * 1024
	// WebTrackMaxDimensionValueLength caps custom dimension values (custom_1..custom_10).
	WebTrackMaxDimensionValueLength = 256
	// WebTrackTimeBounds is the accepted clock window for beat timestamps,
	// applied on both sides of the server clock.
	WebTrackTimeBounds = 24 * time.Hour

	// WebSessionIDMaxAge and WebSessionIDMaxFuture bound the timestamp embedded
	// in the UUIDv7 session id. The past bound is wider than WebTrackTimeBounds
	// because a session can keep beating for up to 24h after it started and the
	// SDK offline queue holds beats for up to 24h.
	//
	// The future bound is exactly WebTrackTimeBounds, and both halves of that
	// matter. It cannot be tighter: the SDK mints the id from the device clock,
	// so a visitor whose clock runs fast inherits the entire skew in the id, and
	// a tighter bound rejects every beat they will ever send — permanently,
	// because the SDK reads a 400 as unretryable and never rotates. It cannot be
	// much wider either: session_date is derived from the id, and the repository
	// creates missing partitions on demand, so this bound is what stops a client
	// minting partitions arbitrarily far ahead. Correcting the id against the
	// payload's sent_at is not an option — sent_at is client-supplied too, so it
	// bounds nothing against a hostile caller.
	WebSessionIDMaxAge    = 48 * time.Hour
	WebSessionIDMaxFuture = WebTrackTimeBounds

	// WebEntryTypeLanding marks the first page of a session.
	WebEntryTypeLanding = "landing"
	// WebEntryTypeNavigation marks subsequent pages.
	WebEntryTypeNavigation = "navigation"

	// WebActionTypePageview and WebActionTypeGoal discriminate payload actions.
	WebActionTypePageview = "pageview"
	WebActionTypeGoal     = "goal"

	// WebAnalyticsDefaultBounceThresholdSeconds is used when workspace settings
	// don't specify a bounce threshold.
	WebAnalyticsDefaultBounceThresholdSeconds = 10
)

// WebSession is one row of the web_sessions table: the cumulative state of a
// visitor session, recomputed from every beat and upserted under a beat_seq
// guard.
type WebSession struct {
	SessionDate time.Time `json:"session_date"` // partition key, derived from the UUIDv7 id
	ID          string    `json:"id"`
	BeatSeq     int64     `json:"beat_seq"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	DurationMs           int64   `json:"duration_ms"` // SUM of per-page focus time
	PageviewCount        int     `json:"pageview_count"`
	MedianPageDurationMs int64   `json:"median_page_duration_ms"`
	MaxScroll            int     `json:"max_scroll"`
	GoalCount            int     `json:"goal_count"`
	GoalValue            float64 `json:"goal_value"`

	ExitPath       string `json:"exit_path"`
	LandingPage    string `json:"landing_page"`
	LandingDomain  string `json:"landing_domain"`
	LandingPath    string `json:"landing_path"`
	Referrer       string `json:"referrer"`
	ReferrerDomain string `json:"referrer_domain"`
	ReferrerPath   string `json:"referrer_path"`
	IsDirect       bool   `json:"is_direct"`

	UTMSource   string `json:"utm_source"`
	UTMMedium   string `json:"utm_medium"`
	UTMCampaign string `json:"utm_campaign"`
	UTMTerm     string `json:"utm_term"`
	UTMContent  string `json:"utm_content"`
	UTMID       string `json:"utm_id"`
	UTMIDFrom   string `json:"utm_id_from"`

	Channel      string `json:"channel"`
	ChannelGroup string `json:"channel_group"`

	Custom1  string `json:"custom_1"`
	Custom2  string `json:"custom_2"`
	Custom3  string `json:"custom_3"`
	Custom4  string `json:"custom_4"`
	Custom5  string `json:"custom_5"`
	Custom6  string `json:"custom_6"`
	Custom7  string `json:"custom_7"`
	Custom8  string `json:"custom_8"`
	Custom9  string `json:"custom_9"`
	Custom10 string `json:"custom_10"`

	ScreenWidth    int `json:"screen_width"`
	ScreenHeight   int `json:"screen_height"`
	ViewportWidth  int `json:"viewport_width"`
	ViewportHeight int `json:"viewport_height"`

	Device         string `json:"device"`
	Browser        string `json:"browser"`
	BrowserType    string `json:"browser_type"`
	OS             string `json:"os"`
	UserAgent      string `json:"user_agent"`
	ConnectionType string `json:"connection_type"`
	Language       string `json:"language"`
	Timezone       string `json:"timezone"`

	Country   string   `json:"country"`
	Region    string   `json:"region"`
	City      string   `json:"city"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`

	SDKVersion string `json:"sdk_version"`
	// ContactEmail is the verified contact this session belongs to, or nil when
	// anonymous. Sticky in the upsert: a later beat that does not know the
	// contact must never erase it.
	ContactEmail *string `json:"contact_email,omitempty"`
}

// WebPage is one row of the web_pages table (one pageview).
type WebPage struct {
	SessionDate time.Time `json:"session_date"` // the session's date, not the page's
	SessionID   string    `json:"session_id"`
	TabID       int64     `json:"tab_id"` // the writing tab; see the schema package
	PageNumber  int       `json:"page_number"`
	BeatSeq     int64     `json:"beat_seq"`

	Path         string    `json:"path"`
	EnteredAt    time.Time `json:"entered_at"`
	ExitedAt     time.Time `json:"exited_at"`
	DurationMs   int64     `json:"duration_ms"`
	MaxScroll    int       `json:"max_scroll"`
	IsLanding    bool      `json:"is_landing"`
	IsExit       bool      `json:"is_exit"`
	EntryType    string    `json:"entry_type"`
	ContactEmail *string   `json:"contact_email,omitempty"`
}

// WebGoal is one row of the web_goals table (one conversion event), carrying a
// denormalized snapshot of the session attribution so goal reports never join.
type WebGoal struct {
	SessionDate time.Time `json:"session_date"`
	SessionID   string    `json:"session_id"`
	TabID       int64     `json:"tab_id"`
	GoalName    string    `json:"goal_name"`
	// ClientTsMs is the goal's original client timestamp in epoch ms, before
	// clock-skew correction, so retried beats dedup onto the same row.
	ClientTsMs int64 `json:"client_ts_ms"`
	BeatSeq    int64 `json:"beat_seq"`

	GoalAt     time.Time         `json:"goal_at"` // skew-corrected
	GoalValue  float64           `json:"goal_value"`
	Path       string            `json:"path"`
	PageNumber int               `json:"page_number"`
	Properties map[string]string `json:"properties,omitempty"`

	Referrer       string `json:"referrer"`
	ReferrerDomain string `json:"referrer_domain"`
	ReferrerPath   string `json:"referrer_path"`
	IsDirect       bool   `json:"is_direct"`
	LandingPage    string `json:"landing_page"`
	LandingDomain  string `json:"landing_domain"`
	LandingPath    string `json:"landing_path"`

	UTMSource   string `json:"utm_source"`
	UTMMedium   string `json:"utm_medium"`
	UTMCampaign string `json:"utm_campaign"`
	UTMTerm     string `json:"utm_term"`
	UTMContent  string `json:"utm_content"`
	UTMID       string `json:"utm_id"`
	UTMIDFrom   string `json:"utm_id_from"`

	Channel      string `json:"channel"`
	ChannelGroup string `json:"channel_group"`

	Custom1  string `json:"custom_1"`
	Custom2  string `json:"custom_2"`
	Custom3  string `json:"custom_3"`
	Custom4  string `json:"custom_4"`
	Custom5  string `json:"custom_5"`
	Custom6  string `json:"custom_6"`
	Custom7  string `json:"custom_7"`
	Custom8  string `json:"custom_8"`
	Custom9  string `json:"custom_9"`
	Custom10 string `json:"custom_10"`

	ScreenWidth    int `json:"screen_width"`
	ScreenHeight   int `json:"screen_height"`
	ViewportWidth  int `json:"viewport_width"`
	ViewportHeight int `json:"viewport_height"`

	Device         string `json:"device"`
	Browser        string `json:"browser"`
	BrowserType    string `json:"browser_type"`
	OS             string `json:"os"`
	UserAgent      string `json:"user_agent"`
	ConnectionType string `json:"connection_type"`
	Language       string `json:"language"`
	Timezone       string `json:"timezone"`

	Country   string   `json:"country"`
	Region    string   `json:"region"`
	City      string   `json:"city"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`

	ContactEmail *string `json:"contact_email,omitempty"`
}

// WebSessionAttributes is the session-level context the SDK sends with every
// beat. Device/browser/os fields are optional legacy inputs: the server parses
// the raw user agent itself and its result wins.
type WebSessionAttributes struct {
	Referrer    string `json:"referrer,omitempty"`
	LandingPage string `json:"landing_page"`

	UTMSource   string `json:"utm_source,omitempty"`
	UTMMedium   string `json:"utm_medium,omitempty"`
	UTMCampaign string `json:"utm_campaign,omitempty"`
	UTMTerm     string `json:"utm_term,omitempty"`
	UTMContent  string `json:"utm_content,omitempty"`
	UTMID       string `json:"utm_id,omitempty"`
	UTMIDFrom   string `json:"utm_id_from,omitempty"`

	ScreenWidth    int `json:"screen_width,omitempty"`
	ScreenHeight   int `json:"screen_height,omitempty"`
	ViewportWidth  int `json:"viewport_width,omitempty"`
	ViewportHeight int `json:"viewport_height,omitempty"`

	Device         string `json:"device,omitempty"`
	Browser        string `json:"browser,omitempty"`
	BrowserType    string `json:"browser_type,omitempty"`
	OS             string `json:"os,omitempty"`
	UserAgent      string `json:"user_agent,omitempty"`
	ConnectionType string `json:"connection_type,omitempty"`
	Language       string `json:"language,omitempty"`
	Timezone       string `json:"timezone,omitempty"`
}

// WebTrackAction is a single action in a beat: a pageview or a goal,
// discriminated by Type. The SDK re-sends the full cumulative list on every
// beat, so the server can rebuild the whole session from any one payload.
type WebTrackAction struct {
	Type       string `json:"type"`
	Path       string `json:"path"`
	PageNumber int    `json:"page_number"`

	// Pageview fields
	Duration  int64 `json:"duration,omitempty"` // per-page focus time, ms
	Scroll    int   `json:"scroll,omitempty"`   // max scroll depth, 0-100
	EnteredAt int64 `json:"entered_at,omitempty"`
	ExitedAt  int64 `json:"exited_at,omitempty"`

	// Goal fields
	Name       string            `json:"name,omitempty"`
	Timestamp  int64             `json:"timestamp,omitempty"`
	Value      float64           `json:"value,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// Validate checks a single action.
func (a *WebTrackAction) Validate() error {
	if len(a.Path) > WebTrackMaxPathLength {
		return fmt.Errorf("action path exceeds %d characters", WebTrackMaxPathLength)
	}
	if a.PageNumber < 1 {
		return fmt.Errorf("action page_number must be >= 1")
	}
	switch a.Type {
	case WebActionTypePageview:
		if a.Duration < 0 {
			return fmt.Errorf("pageview duration must be >= 0")
		}
		if a.Scroll < 0 || a.Scroll > 100 {
			return fmt.Errorf("pageview scroll must be between 0 and 100")
		}
		if a.ExitedAt != 0 && a.EnteredAt != 0 && a.ExitedAt < a.EnteredAt {
			return fmt.Errorf("pageview exited_at must be >= entered_at")
		}
	case WebActionTypeGoal:
		if strings.TrimSpace(a.Name) == "" {
			return fmt.Errorf("goal name is required")
		}
		if len(a.Name) > WebTrackMaxGoalNameLength {
			return fmt.Errorf("goal name exceeds %d characters", WebTrackMaxGoalNameLength)
		}
		if a.Timestamp <= 0 {
			return fmt.Errorf("goal timestamp is required")
		}
		if a.Value < 0 {
			return fmt.Errorf("goal value must be >= 0")
		}
		if err := validateGoalProperties(a.Properties); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown action type: %q", a.Type)
	}
	return nil
}

// validateGoalProperties bounds a goal's properties three ways: key count, the
// length of any one value, and the total serialized size. All three are needed —
// many small keys, one huge value, and a merely large map each reach the same
// cumulative-payload wedge by a different route.
func validateGoalProperties(props map[string]string) error {
	if len(props) == 0 {
		return nil
	}
	if len(props) > WebTrackMaxGoalPropertyKeys {
		return fmt.Errorf("goal properties exceed %d keys", WebTrackMaxGoalPropertyKeys)
	}
	total := 0
	for key, value := range props {
		if len(value) > WebTrackMaxGoalPropertyValueLength {
			return fmt.Errorf("goal property %q exceeds %d characters", key, WebTrackMaxGoalPropertyValueLength)
		}
		total += len(key) + len(value)
		if total > WebTrackMaxGoalPropertiesBytes {
			return fmt.Errorf("goal properties exceed %d bytes", WebTrackMaxGoalPropertiesBytes)
		}
	}
	return nil
}

// WebTrackPayload is the body of POST /track. The wire format matches the
// Staminads SDK payload, plus the beat sequence number used for deterministic
// upsert ordering.
type WebTrackPayload struct {
	WorkspaceID string                `json:"workspace_id"`
	SessionID   string                `json:"session_id"`
	Actions     []WebTrackAction      `json:"actions"`
	Attributes  *WebSessionAttributes `json:"attributes,omitempty"`
	// CreatedAt is accepted for wire compatibility but ignored: the session
	// start is derived from the UUIDv7 session id, the single source of truth
	// that also decides the partition.
	CreatedAt  int64  `json:"created_at"` // epoch ms (ignored)
	UpdatedAt  int64  `json:"updated_at"` // epoch ms
	SDKVersion string `json:"sdk_version,omitempty"`
	// TabID identifies the writing tab. Tabs share a session id (localStorage)
	// but keep their own cumulative actions and their own seq (sessionStorage),
	// so they are disjoint writers. Absent (0) from an older SDK, which then
	// behaves exactly as it does today.
	TabID  int64  `json:"tab_id,omitempty"`
	SentAt *int64 `json:"sent_at,omitempty"` // stamped at each HTTP attempt

	// Identity credentials. /track is public and unauthenticated, so none of
	// these is believed until ResolveWebIdentity checks it against the
	// workspace secret. Either the pair (from identify()) or the token (from an
	// email-click link) may be present; never both in practice.
	ContactEmail     *string `json:"contact_email,omitempty"`
	ContactEmailHMAC *string `json:"contact_email_hmac,omitempty"`
	IdentifyToken    *string `json:"identify_token,omitempty"`

	Dimensions map[string]string `json:"dimensions,omitempty"` // custom_1..custom_10
	Seq        int64             `json:"seq"`                  // monotonic per-session beat counter
}

// Validate checks the payload against the server clock. It does not resolve
// the workspace or apply any enrichment.
func (p *WebTrackPayload) Validate(now time.Time) error {
	if strings.TrimSpace(p.WorkspaceID) == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if _, _, err := SessionDateFromUUIDv7(p.SessionID, now); err != nil {
		return fmt.Errorf("invalid session_id: %w", err)
	}
	if len(p.Actions) > WebTrackMaxActions {
		return fmt.Errorf("actions exceeds the maximum of %d", WebTrackMaxActions)
	}
	if p.Seq < 0 {
		return fmt.Errorf("seq must be >= 0")
	}
	// created_at is deliberately NOT validated: the session's start is taken
	// from the UUIDv7 id (see SessionDateFromUUIDv7), which already governs
	// partition placement. Trusting one source instead of two removes the case
	// where a session's stored start disagrees with its own partition, and the
	// case where a session still beating after 24h had every beat rejected
	// with a 400 the SDK never retries.
	if err := validateEpochMsWindow("updated_at", p.UpdatedAt, now); err != nil {
		return err
	}
	for key, value := range p.Dimensions {
		if !IsCustomDimensionKey(key) {
			// Unknown keys are ignored at build time; only bound their size so
			// hostile payloads can't inflate memory.
			continue
		}
		if len(value) > WebTrackMaxDimensionValueLength {
			return fmt.Errorf("dimension %s exceeds %d characters", key, WebTrackMaxDimensionValueLength)
		}
	}
	if len(p.Dimensions) > 50 {
		return fmt.Errorf("too many dimensions")
	}
	p.dropInvalidActions()
	return nil
}

// dropInvalidActions removes actions that fail their own validation, in place.
//
// A malformed action must never reject the whole beat. actions[] is cumulative
// — the SDK re-sends every action of the session on every beat — so one bad
// entry rejected wholesale becomes a 400 on every subsequent beat of that
// session, forever, and the SDK treats a 400 as permanent. The blast radius of
// a client-side arithmetic slip has to be the action, not the session.
//
// A beat left with no actions is not an error either: Track already treats an
// empty action list as a silent success.
func (p *WebTrackPayload) dropInvalidActions() {
	kept := p.Actions[:0]
	for i := range p.Actions {
		if p.Actions[i].Validate() == nil {
			kept = append(kept, p.Actions[i])
		}
	}
	p.Actions = kept
}

func validateEpochMsWindow(field string, epochMs int64, now time.Time) error {
	if epochMs <= 0 {
		return fmt.Errorf("%s is required", field)
	}
	ts := time.UnixMilli(epochMs)
	if ts.Before(now.Add(-WebTrackTimeBounds)) || ts.After(now.Add(WebTrackTimeBounds)) {
		return fmt.Errorf("%s is outside the accepted time window", field)
	}
	return nil
}

// IsCustomDimensionKey reports whether key is one of the ten custom dimension
// slots (custom_1..custom_10).
func IsCustomDimensionKey(key string) bool {
	if !strings.HasPrefix(key, "custom_") {
		return false
	}
	switch key {
	case "custom_1", "custom_2", "custom_3", "custom_4", "custom_5",
		"custom_6", "custom_7", "custom_8", "custom_9", "custom_10":
		return true
	}
	return false
}

// webIdentifyHMACPrefix domain-separates the analytics identity credential from
// every other HMAC computed over a bare email with the same workspace secret.
//
// ComputeEmailHMAC authorizes subscription changes (notification center,
// unsubscribe, one-click) and is printed into every email Notifuse sends.
// Without this prefix the two would be interchangeable: an unsubscribe HMAC
// scraped from a forwarded email would silently identify a visitor, and an
// analytics credential lifted out of page JS by any third-party script would
// let its holder change that contact's subscriptions.
const webIdentifyHMACPrefix = "wa_identify:"

// ComputeWebIdentifyHMAC is what a customer's server mints for identify().
func ComputeWebIdentifyHMAC(email string, secretKey string) string {
	return crypto.ComputeHMAC256([]byte(webIdentifyHMACPrefix+email), secretKey)
}

// webIdentifyTokenPayload is what an email-click link carries, encrypted.
type webIdentifyTokenPayload struct {
	Email     string `json:"e"`
	ExpiresAt int64  `json:"x"` // unix seconds
	Version   int    `json:"v"`
}

// BuildWebIdentifyToken mints the opaque nf_id parameter for a tracked link.
//
// AES-256-GCM keyed by the workspace secret, so it is authenticated AND
// confidential: the address never appears in a URL that would otherwise flow
// into the customer's own analytics, their server logs and any third-party
// Referer. Deliberately NOT crypto.EncryptTrackingToken, which uses a hardcoded
// obfuscation key and is therefore forgeable from the open-source repository.
func BuildWebIdentifyToken(email string, secretKey string, ttl time.Duration, now time.Time) (string, error) {
	if secretKey == "" {
		return "", fmt.Errorf("workspace secret key is required to mint an identify token")
	}
	body, err := json.Marshal(webIdentifyTokenPayload{
		Email:     email,
		ExpiresAt: now.Add(ttl).Unix(),
		Version:   1,
	})
	if err != nil {
		return "", fmt.Errorf("failed to encode identify token: %w", err)
	}
	return crypto.EncryptString(string(body), secretKey)
}

// ResolveWebIdentity returns the verified contact address a beat carries, or
// ok=false when it carries none that can be trusted.
//
// It only proves who the caller is; it does not prove the events are real, and
// it does not check that the address belongs to a contact. That gate lives in
// the service layer, which has database access.
//
// A malformed credential fails closed rather than falling through to the next
// one — otherwise an attacker could downgrade past whichever check they cannot
// satisfy. Bounds live here rather than in Validate because an over-long field
// must cost the identity, not the whole beat.
func ResolveWebIdentity(p *WebTrackPayload, secretKey string, now time.Time) (string, bool) {
	if p == nil || secretKey == "" {
		return "", false
	}

	if p.IdentifyToken != nil {
		if len(*p.IdentifyToken) > WebTrackMaxIdentifyTokenLength {
			return "", false
		}
		decrypted, err := crypto.DecryptFromHexString(*p.IdentifyToken, secretKey)
		if err != nil {
			return "", false
		}
		var token webIdentifyTokenPayload
		if err := json.Unmarshal([]byte(decrypted), &token); err != nil {
			return "", false
		}
		if token.ExpiresAt <= now.Unix() {
			return "", false
		}
		return normalizedIdentity(token.Email)
	}

	if p.ContactEmail == nil || p.ContactEmailHMAC == nil {
		return "", false
	}
	if len(*p.ContactEmail) > WebTrackMaxEmailLength || len(*p.ContactEmailHMAC) > WebTrackMaxHMACLength {
		return "", false
	}
	// Verify against the RAW address: the customer signed what they sent, not
	// what we would normalize it to.
	if !hmac.Equal([]byte(ComputeWebIdentifyHMAC(*p.ContactEmail, secretKey)), []byte(*p.ContactEmailHMAC)) {
		return "", false
	}
	return normalizedIdentity(*p.ContactEmail)
}

// normalizedIdentity lowercases and trims so the value matches contacts.email,
// which is stored normalized.
func normalizedIdentity(email string) (string, bool) {
	normalized := NormalizeEmail(email)
	if normalized == "" || len(normalized) > WebTrackMaxEmailLength {
		return "", false
	}
	return normalized, true
}

// SessionDateFromUUIDv7 derives the partition date and the session start time
// from the timestamp embedded in a UUIDv7 session id. It is a pure function of
// the id, so every beat and every replica routes a session to the same
// partition regardless of clock skew. Ids whose embedded timestamp falls
// outside [now-48h, now+10min] are rejected.
func SessionDateFromUUIDv7(sessionID string, now time.Time) (sessionDate time.Time, sessionStart time.Time, err error) {
	u, parseErr := uuid.Parse(sessionID)
	if parseErr != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("not a valid UUID: %w", parseErr)
	}
	if u.Version() != 7 {
		return time.Time{}, time.Time{}, fmt.Errorf("session id must be a UUIDv7 (got version %d)", u.Version())
	}
	// The first 48 bits of a UUIDv7 are the big-endian unix timestamp in ms.
	ms := int64(u[0])<<40 | int64(u[1])<<32 | int64(u[2])<<24 |
		int64(u[3])<<16 | int64(u[4])<<8 | int64(u[5])
	sessionStart = time.UnixMilli(ms).UTC()
	if sessionStart.Before(now.Add(-WebSessionIDMaxAge)) {
		return time.Time{}, time.Time{}, fmt.Errorf("session id timestamp is too old")
	}
	if sessionStart.After(now.Add(WebSessionIDMaxFuture)) {
		return time.Time{}, time.Time{}, fmt.Errorf("session id timestamp is in the future")
	}
	y, m, d := sessionStart.Date()
	sessionDate = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return sessionDate, sessionStart, nil
}

// WebAnalyticsSettings is the per-workspace configuration for the web
// analytics feature, stored inside WorkspaceSettings (system DB JSONB).
type WebAnalyticsSettings struct {
	Enabled                bool              `json:"enabled"`
	AllowedDomains         []string          `json:"allowed_domains,omitempty"`
	BounceThresholdSeconds int               `json:"bounce_threshold_seconds,omitempty"`
	Filters                []WebFilter       `json:"filters,omitempty"`
	FiltersVersion         string            `json:"filters_version,omitempty"`
	CustomDimensionLabels  map[string]string `json:"custom_dimension_labels,omitempty"`

	GeoEnabled         bool `json:"geo_enabled"`
	GeoStoreCity       bool `json:"geo_store_city"`
	GeoStoreRegion     bool `json:"geo_store_region"`
	GeoCoordsPrecision int  `json:"geo_coordinates_precision"` // decimals kept on lat/lon, 0-2
}

// BounceThresholdMs returns the bounce threshold in milliseconds, applying the
// default when unset. Nil-receiver safe so callers can pass settings through
// without guards.
func (s *WebAnalyticsSettings) BounceThresholdMs() int {
	if s == nil || s.BounceThresholdSeconds <= 0 {
		return WebAnalyticsDefaultBounceThresholdSeconds * 1000
	}
	return s.BounceThresholdSeconds * 1000
}

// Validate checks the settings. Nil-receiver safe (absent settings are valid).
func (s *WebAnalyticsSettings) Validate() error {
	if s == nil {
		return nil
	}
	if s.BounceThresholdSeconds < 0 {
		return fmt.Errorf("bounce_threshold_seconds must be >= 0")
	}
	if s.GeoCoordsPrecision < 0 || s.GeoCoordsPrecision > 2 {
		return fmt.Errorf("geo_coordinates_precision must be between 0 and 2")
	}
	for _, d := range s.AllowedDomains {
		if err := validateAllowedDomain(d); err != nil {
			return err
		}
	}
	for key := range s.CustomDimensionLabels {
		if !IsCustomDimensionKey(key) {
			return fmt.Errorf("custom_dimension_labels key %q must be custom_1..custom_10", key)
		}
	}
	for i := range s.Filters {
		if err := s.Filters[i].Validate(); err != nil {
			return fmt.Errorf("filter %d (%s): %w", i, s.Filters[i].Name, err)
		}
	}
	return nil
}

// validateAllowedDomain accepts bare hostnames and single leading wildcards
// ("example.com", "*.example.com").
func validateAllowedDomain(domain string) error {
	d := strings.TrimSpace(domain)
	if d == "" {
		return fmt.Errorf("allowed domain cannot be empty")
	}
	d = strings.TrimPrefix(d, "*.")
	if strings.ContainsAny(d, " */?#@") || strings.Contains(d, "://") {
		return fmt.Errorf("invalid allowed domain: %q", domain)
	}
	return nil
}

// MatchesAllowedDomain reports whether hostname matches any configured allowed
// domain, with "*.example.com" matching both subdomains and the apex. An empty
// list allows every hostname (Staminads behavior).
func (s *WebAnalyticsSettings) MatchesAllowedDomain(hostname string) bool {
	if s == nil || len(s.AllowedDomains) == 0 {
		return true
	}
	host := strings.ToLower(strings.TrimSpace(hostname))
	if host == "" {
		return false
	}
	for _, d := range s.AllowedDomains {
		allowed := strings.ToLower(strings.TrimSpace(d))
		if wild, ok := strings.CutPrefix(allowed, "*."); ok {
			if host == wild || strings.HasSuffix(host, "."+wild) {
				return true
			}
			continue
		}
		if host == allowed {
			return true
		}
	}
	return false
}

// WebRequestMeta carries request-level context the enrichment pipeline needs.
// The client IP is used for the geo lookup only and is never persisted.
type WebRequestMeta struct {
	Origin     string
	Referer    string
	UserAgent  string
	ClientIP   string
	ReceivedAt time.Time
}

// WebGeoResult is the outcome of a GeoIP lookup.
type WebGeoResult struct {
	Country   string
	Region    string
	City      string
	Latitude  *float64
	Longitude *float64
}

// GeoIPResolver resolves an IP address to a coarse location. Implementations
// must be safe for concurrent use and cheap on repeated lookups.
type GeoIPResolver interface {
	Lookup(ip string) (*WebGeoResult, error)
}

// WebAnalyticsRepository persists web analytics rows into a workspace database.
type WebAnalyticsRepository interface {
	// FlushBatch upserts the given rows in one transaction. Row slices may be
	// empty. Implementations sort rows by primary key to keep concurrent
	// flushes deadlock-free, and auto-create missing monthly partitions once.
	FlushBatch(ctx context.Context, workspaceID string, sessions []*WebSession, pages []*WebPage, goals []*WebGoal) error

	// EnsureMonthlyPartitions creates the monthly partitions covering the given
	// months for all three tables (idempotent).
	EnsureMonthlyPartitions(ctx context.Context, workspaceID string, months []time.Time) error

	// ListPartitions returns partition names of the given parent table.
	ListPartitions(ctx context.Context, workspaceID string, table string) ([]string, error)

	// AnalyzePartitions runs ANALYZE on the given partitions.
	AnalyzePartitions(ctx context.Context, workspaceID string, partitions []string) error

	// SetPartitionAutovacuum applies (aggressive=true) or resets
	// (aggressive=false) the per-partition autovacuum storage parameters used
	// for hot, upsert-heavy current-month partitions.
	SetPartitionAutovacuum(ctx context.Context, workspaceID string, partition string, aggressive bool) error

	// BackfillPartition recompiles the attribution rules to SQL and rewrites
	// one partition of web_sessions or web_goals. Returns rows updated.
	BackfillPartition(ctx context.Context, workspaceID string, partition string, filters []WebFilter) (int64, error)
}

// WebAnalyticsBackfillTaskType is the task-system type of attribution
// backfill runs.
const WebAnalyticsBackfillTaskType = "web_analytics_backfill"

// WebAnalyticsBackfillStatus is the console-facing view of a backfill run.
type WebAnalyticsBackfillStatus struct {
	TaskID       string                     `json:"task_id"`
	Status       string                     `json:"status"` // pending | running | completed | failed
	Progress     float64                    `json:"progress"`
	State        *WebAnalyticsBackfillState `json:"state,omitempty"`
	ErrorMessage string                     `json:"error_message,omitempty"`
}

// WebAnalyticsService is consumed by the public /track handler (Track) and
// the authenticated console RPCs (Backfill*).
type WebAnalyticsService interface {
	// Track validates, enriches and buffers one beat. It must never return
	// data-dependent errors for silently-rejected traffic (disabled feature,
	// disallowed origin): those are dropped while reporting success, matching
	// Staminads.
	Track(ctx context.Context, payload *WebTrackPayload, meta WebRequestMeta) error

	// BackfillStart launches an attribution backfill task for the workspace
	// (web_analytics:write). Fails if a run is already pending or running.
	BackfillStart(ctx context.Context, workspaceID string) (*WebAnalyticsBackfillStatus, error)

	// BackfillStatus returns the latest backfill run, or nil when none exists.
	BackfillStatus(ctx context.Context, workspaceID string) (*WebAnalyticsBackfillStatus, error)

	// BackfillCancel aborts the in-flight backfill run (web_analytics:write).
	BackfillCancel(ctx context.Context, workspaceID string) error
}
