package domain

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
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
	// WebTrackMaxEmailLength matches contacts.email VARCHAR(255), and is counted
	// in CHARACTERS for that reason — Postgres counts VARCHAR in characters, not
	// bytes. Comparing it with Go's len() instead made an SMTPUTF8 address of 134
	// characters and 256 bytes storable as a contact yet unidentifiable: it was
	// accepted by Contact.Validate, sat happily in the column, minted a valid
	// token, and was then discarded at the beat without a word.
	WebTrackMaxEmailLength = 255
	// WebTrackMaxHMACLength bounds the hex-encoded SHA-256 credential.
	WebTrackMaxHMACLength = 64
	// The three terms WebTrackMaxIdentifyTokenLength is built from, each a
	// property of the encoding BuildWebIdentifyToken uses rather than a budget
	// anyone chose. TestWebIdentifyTokenBoundCoversEveryStorableAddress checks
	// all three against what the encoder actually emits.
	//
	// crypto.EncryptString hex-encodes a 12-byte GCM nonce, the ciphertext (one
	// byte per plaintext byte) and a 16-byte authentication tag.
	webIdentifyTokenCipherOverheadBytes = 12 + 16
	// The JSON around the address: {"e":"","x":9999999999,"v":1}. Counted rather
	// than measured with len(), which would make this constant typed and every
	// expression built from it typed with it. Unix seconds stay ten digits until
	// the year 2286.
	webIdentifyTokenJSONScaffoldBytes = 29
	// The most encoding/json can spend on one byte of the address: \uXXXX, for
	// a control character, one of the HTML-escaped "&<>", or a byte that is not
	// valid UTF-8 (each of those becomes \ufffd).
	webIdentifyTokenMaxJSONBytesPerByte = 6

	// WebTrackMaxIdentifyTokenLength bounds the encrypted nf_id parameter at
	// both ends: ResolveWebIdentity discards a longer token, and
	// BuildWebIdentifyToken measures what it produced against this same
	// constant and refuses the mint rather than putting a token on every link
	// of an email that the beat would then drop without a word.
	//
	// It is derived from WebTrackMaxEmailLength, not chosen. A token is the hex
	// of (nonce ‖ ciphertext ‖ tag) over a JSON payload carrying the address, so
	// its length grows with the address: a hand-picked 512 stopped at 199
	// characters while a contact may have 255, which made every longer address
	// one the platform accepts everywhere else and can never identify from an
	// email click — silently, since the mint refused and the send carried on.
	// Deriving the bound is what keeps those two facts in agreement.
	//
	// The worst case is budgeted rather than the typical one: encoding/json can
	// spend six characters on a single byte, and the bytes it does that to
	// ("&", "<", ">") are legal in an address the contact validator accepts. A
	// plain 255-character address mints 624 characters; one made of "&" mints
	// 3054.
	//
	// Still a safe bound on a public unauthenticated endpoint, because it is a
	// function of the longest address the contacts schema can hold and not of
	// client whim: a longer nf_id is dropped by this length comparison before
	// any hex decode or AES open, and what it does admit — under 1.6 kB of
	// ciphertext — is small beside what a beat already carries, where one action
	// path alone may be WebTrackMaxPathLength.
	//
	// Mirrored in three places outside this package: MAX_IDENTITY_TOKEN_BYTES in
	// the SDK, the identify_token maxLength in the OpenAPI schema, and the drift
	// test that compares this constant to that schema.
	WebTrackMaxIdentifyTokenLength = 2 * (webIdentifyTokenCipherOverheadBytes +
		webIdentifyTokenJSONScaffoldBytes +
		webIdentifyTokenMaxJSONBytesPerByte*WebTrackMaxEmailLength)

	// Goal property bounds. These exist because actions[] is cumulative: the SDK
	// re-sends every action of the session on every beat, so an unbounded
	// properties map is carried forever and eventually pushes the serialized
	// body past webTrackMaxBodyBytes — after which EVERY later beat of that
	// session is rejected, permanently, with no client-side recovery. Bounding
	// here means an oversized map costs its own action and nothing else.
	WebTrackMaxGoalPropertyKeys        = 50
	WebTrackMaxGoalPropertyValueLength = 1024
	WebTrackMaxGoalPropertiesBytes     = 8 * 1024

	// Upper bounds on the client's numbers. Sign and ordering were checked but
	// never magnitude, and each of these lands in a narrow Postgres column —
	// goal_value in REAL, the goal timestamp in TIMESTAMPTZ. flushOnce runs a
	// whole workspace batch in ONE transaction, so one out-of-range value
	// aborted every other visitor's rows with it, and after two failed attempts
	// the buffer deletes those sessions outright. Bounding here means the
	// offending action is dropped by dropInvalidActions and costs only itself.
	WebTrackMaxDurationMs = 24 * 60 * 60 * 1000 // a day of engaged time on one page
	WebTrackMaxGoalValue  = 1e12

	// webMaxEpochMs keeps a client timestamp inside what TIMESTAMPTZ can hold
	// without the year running away (roughly 5138). Deliberately not a window
	// check: a replayed offline beat legitimately carries an old timestamp.
	webMaxEpochMs = 100000000000000
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
//
// Keeping this table narrow and exposing goals through a view that joins back to
// web_sessions was the reviewed alternative. It lost because goals are sparse:
// duplicating a few dimensions onto the rare conversion row is cheap, while the
// join would drag every goal report across the much larger sessions table. The
// view remains the fallback if that sparsity assumption ever stops holding.
// Dimensions are plain text rather than ENUMs for the same kind of reason — the
// value sets (UTM sources, browsers, referrers) are open-ended, and each new
// value would otherwise need a schema change.
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

	// GoalType is asserted by the site, not verified by Notifuse — the same page
	// already chooses the goal's name and value. Read revenue reporting with that
	// in mind.
	GoalType string `json:"goal_type"`
}

// WebSessionAttributes is the session-level context the SDK sends with every
// beat. Device, browser and OS are taken from the client as sent — the SDK
// parses them in the browser, where Client Hints are available, and nothing
// re-parses UserAgent server-side. Modern browsers freeze the UA string and
// expose the real OS and version only through an API the server never sees, so
// the client's answer is the better one. Being client input, the values are
// bounded during enrichment rather than trusted verbatim.
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
	Name string `json:"name,omitempty"`
	// GoalType is the goal's own type, NOT the action discriminator above. It
	// cannot be called `type`: that key already says "goal" vs "pageview".
	GoalType   string            `json:"goal_type,omitempty"`
	Timestamp  int64             `json:"timestamp,omitempty"`
	Value      float64           `json:"value,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// UnmarshalJSON accepts fractional milliseconds for every ms-valued field.
//
// The SDK accumulates focus time from performance.now(), which is fractional,
// so a real browser beat carries "duration":1473.3999999761581. encoding/json
// refuses a fractional number for an int64 field, and that error rejects the
// WHOLE payload — not the single action — so one ordinary pageview would sink
// the entire session with a 400. Hand-built test payloads all use round
// integers, which is why only a real browser can surface this.
//
// Rounding is the right resolution rather than widening the fields: storage is
// integer milliseconds (web_pages.duration_ms is INTEGER), so the fraction has
// nowhere to go. Doing it here keeps every consumer on int64, including the
// goal timestamp that is baked into the dedup ExternalID — where rounding an
// already-integer value is a no-op, so dedup is unaffected.
func (a *WebTrackAction) UnmarshalJSON(data []byte) error {
	// The alias sheds this method, so the embedded decode does not recurse.
	// Shallower fields win over the embedded ones on a tag conflict, so the
	// float64 shadows below are what actually receive the ms values.
	type alias WebTrackAction
	aux := struct {
		Duration  *float64 `json:"duration,omitempty"`
		EnteredAt *float64 `json:"entered_at,omitempty"`
		ExitedAt  *float64 `json:"exited_at,omitempty"`
		Timestamp *float64 `json:"timestamp,omitempty"`
		*alias
	}{alias: (*alias)(a)}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// A NaN or ±Inf would round to an unusable int64, so leave those at zero
	// and let Validate reject the action on its own terms rather than storing
	// a garbage timestamp.
	round := func(v *float64) int64 {
		if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
			return 0
		}
		return int64(math.Round(*v))
	}
	a.Duration = round(aux.Duration)
	a.EnteredAt = round(aux.EnteredAt)
	a.ExitedAt = round(aux.ExitedAt)
	a.Timestamp = round(aux.Timestamp)
	return nil
}

// Validate checks a single action.
// normalizeWebGoalType maps whatever a page sent onto a type the rest of the
// product understands, and never fails.
//
// The SDK requires a valid type and throws without one, so this only ever sees
// input from a stale cached bundle or a hand-rolled client. Rejecting those would
// mean silently losing their conversions — dropInvalidActions removes a failing
// action without telling anyone — so an unrecognised value becomes "other" and
// the conversion is still recorded, just less precisely.
func normalizeWebGoalType(goalType string) string {
	normalized := strings.ToLower(strings.TrimSpace(goalType))
	for _, valid := range ValidGoalTypes {
		if normalized == valid {
			return normalized
		}
	}
	return GoalTypeOther
}

func (a *WebTrackAction) Validate() error {
	if len(a.Path) > WebTrackMaxPathLength {
		return fmt.Errorf("action path exceeds %d characters", WebTrackMaxPathLength)
	}
	if a.PageNumber < 1 || a.PageNumber > WebTrackMaxActions {
		return fmt.Errorf("action page_number must be between 1 and %d", WebTrackMaxActions)
	}
	switch a.Type {
	case WebActionTypePageview:
		if a.Duration < 0 || a.Duration > WebTrackMaxDurationMs {
			return fmt.Errorf("pageview duration must be between 0 and %d ms", WebTrackMaxDurationMs)
		}
		if a.Scroll < 0 || a.Scroll > 100 {
			return fmt.Errorf("pageview scroll must be between 0 and 100")
		}
		for _, stamp := range []int64{a.EnteredAt, a.ExitedAt} {
			if stamp < 0 || stamp > webMaxEpochMs {
				return fmt.Errorf("pageview timestamp is out of range")
			}
		}
		if a.ExitedAt != 0 && a.EnteredAt != 0 && a.ExitedAt < a.EnteredAt {
			return fmt.Errorf("pageview exited_at must be >= entered_at")
		}
	case WebActionTypeGoal:
		a.GoalType = normalizeWebGoalType(a.GoalType)
		if strings.TrimSpace(a.Name) == "" {
			return fmt.Errorf("goal name is required")
		}
		if len(a.Name) > WebTrackMaxGoalNameLength {
			return fmt.Errorf("goal name exceeds %d characters", WebTrackMaxGoalNameLength)
		}
		if a.Timestamp <= 0 || a.Timestamp > webMaxEpochMs {
			return fmt.Errorf("goal timestamp is required and must be a plausible epoch millisecond")
		}
		if a.Value < 0 || a.Value > WebTrackMaxGoalValue {
			return fmt.Errorf("goal value must be between 0 and %g", WebTrackMaxGoalValue)
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

// WebIdentifyQueryParam names the URL parameter a tracked link carries the
// token in. The literal is owned by notifuse_mjml, which runs the link-rewriting
// pass and cannot import this package (that import already goes the other way);
// re-exporting it here keeps one definition for both sides.
const WebIdentifyQueryParam = notifuse_mjml.WebIdentifyQueryParam

// WebIdentifyTokenTTL is how long a minted nf_id stays usable, counted from the
// mint — which is not, on every path, the moment the mail is delivered.
//
// The length is deliberately much shorter than what the encryption itself would
// allow: a forwarded email hands a third party a bearer identity for the
// original recipient, so the window is sized to the realistic click window of a
// campaign rather than to the credential's natural lifetime.
//
// Where the clock starts is worth stating because not every send path mints at
// delivery. The transactional send and the broadcast direct sender compile the
// message and hand it to the provider in one go, so for them the two moments
// coincide. The queue paths — a queued broadcast's buildQueueEntry, the
// automation email executor — bake the token into the HTML they store in
// email_queue, so the window opens when that row is written. An entry that then
// waits spends part of its window before the recipient ever sees the mail: a
// paused broadcast keeps status 'paused' until it is resumed or deleted, a large
// queue drains behind the provider's rate limit, a failing entry retries. One
// that waits longer than this TTL is delivered with an nf_id that is already
// dead.
//
// What that costs, when it happens, is invisible from every side: the click
// resolves to nothing, the beat still returns 200 and the visit is recorded
// anonymously, and the SDK goes on re-sending the stored token on later beats
// because an opaque token gives it no expiry to inspect. Nothing logs it.
// Starting the clock at delivery instead would mean rewriting the links in the
// worker, which is a larger change than a constant.
const WebIdentifyTokenTTL = 7 * 24 * time.Hour

// BuildWebIdentifyToken mints the opaque nf_id parameter for a tracked link.
//
// AES-256-GCM keyed by the workspace secret, so it is authenticated AND
// confidential: the address never appears in a URL that would otherwise flow
// into the customer's own analytics, their server logs and any third-party
// Referer. Deliberately NOT crypto.EncryptTrackingToken, which uses a hardcoded
// obfuscation key and is therefore forgeable from the open-source repository.
//
// The minted token is measured against WebTrackMaxIdentifyTokenLength — the
// very bound ResolveWebIdentity applies before it will decrypt an nf_id, and
// the same constant on both ends on purpose. That bound is now derived from
// WebTrackMaxEmailLength, so no address a contact can actually have reaches it;
// what is left for this check to catch is an address longer than the schema
// stores, or a payload that grew without the constant following it. Either way
// the failure belongs here, where the address is still in hand and the caller
// can log it: a token over the bound would otherwise mint happily, ride in
// every link of the email, and be dropped at the beat with no log and no error,
// so the identity silently never happens. Measuring the produced token rather
// than deriving a maximum address length keeps the two ends tied to one
// constant, instead of to arithmetic that would drift the moment the payload or
// the encoding changes.
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
	token, err := crypto.EncryptString(string(body), secretKey)
	if err != nil {
		return "", err
	}
	if len(token) > WebTrackMaxIdentifyTokenLength {
		return "", fmt.Errorf("identify token for a %d-character address is %d characters, over the %d ResolveWebIdentity accepts",
			len(email), len(token), WebTrackMaxIdentifyTokenLength)
	}
	return token, nil
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
	// Characters, not bytes: the bound mirrors a VARCHAR(255) column. The hmac
	// beside it stays byte-counted because it is hex, where the two agree.
	if utf8.RuneCountInString(*p.ContactEmail) > WebTrackMaxEmailLength ||
		len(*p.ContactEmailHMAC) > WebTrackMaxHMACLength {
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
	// Re-checked after normalization because lowercasing can change the length,
	// and in characters for the same reason as the raw gate above.
	if normalized == "" || utf8.RuneCountInString(normalized) > WebTrackMaxEmailLength {
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

	// IdentifyFromEmailLinks lets Notifuse mint an identity credential into the
	// links of tracked emails, so a recipient who clicks one is identified on
	// landing without a line of customer code.
	//
	// Off by default, and separate from everything below, because it is the one
	// identity path the CUSTOMER does not initiate. identify(email, hmac) is
	// their own server's decision, made with their own secret; this one is ours,
	// made on their behalf for every recipient of every tracked broadcast. Left
	// ungated it would tie a contact to their browsing — timeline entries, goals,
	// automation enrolments — for any workspace that merely turned web analytics
	// on and left link tracking enabled.
	IdentifyFromEmailLinks bool `json:"identify_from_email_links"`

	// There is deliberately no opt-in flag for writing to contact timelines.
	// Calling identify() with an HMAC IS the opt-in: minting that credential
	// requires the workspace secret, so the customer's own server has already
	// decided this visitor may be tied to a contact. A workspace that does not
	// want web activity on its timelines does not call identify().
	//
	// Two flags used to gate this — contact_bridge_enabled for goals and
	// record_contact_navigation for sessions and pageviews — and both were
	// removed once identify() became the consent checkpoint. A workspace can
	// still use web analytics anonymously for reporting alone; it simply never
	// identifies anyone.

	GeoEnabled         bool `json:"geo_enabled"`
	GeoStoreCity       bool `json:"geo_store_city"`
	GeoStoreRegion     bool `json:"geo_store_region"`
	GeoCoordsPrecision int  `json:"geo_coordinates_precision"` // decimals kept on lat/lon, 0-2
}

// CanIdentifyFromEmailLinks reports whether Notifuse may mint an identity into
// the links of a tracked email.
//
// One predicate for every send path — broadcast, transactional and automation —
// because three copies of it is exactly how the gate came to be enforced on one
// path and not the other two. Nil-receiver safe: a workspace with no web
// analytics settings identifies nobody.
func (s *WebAnalyticsSettings) CanIdentifyFromEmailLinks() bool {
	return s != nil && s.Enabled && s.IdentifyFromEmailLinks && len(s.AllowedDomains) > 0
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

// EffectiveGeoCoordsPrecision is how many decimals of latitude and longitude may
// actually be stored, given the place-name toggles.
//
// A coordinate is a place name expressed differently, so it cannot be more
// precise than the finest name the workspace agreed to store:
//
//	city stored  -> up to 2 decimals (~1 km)
//	region only  -> up to 1 decimal  (~11 km)
//	neither      -> 0 decimals       (~111 km, country-scale)
//
// A ceiling, never a floor: the configured value still wins when it is coarser.
//
// Clamped rather than gated. Dropping the coordinates entirely would empty the
// Live map, and an empty Live map reads as "nobody is online" — a more confusing
// failure than a coarse pin.
//
// Nil-receiver safe: absent settings mean everything on, matching applyWebGeo.
func (s *WebAnalyticsSettings) EffectiveGeoCoordsPrecision() int {
	const maxPrecision = 2
	if s == nil {
		return maxPrecision
	}

	configured := s.GeoCoordsPrecision
	if configured < 0 {
		configured = 0
	}
	if configured > maxPrecision {
		configured = maxPrecision
	}

	ceiling := 0
	switch {
	case s.GeoStoreCity:
		ceiling = 2
	case s.GeoStoreRegion:
		ceiling = 1
	}

	return min(configured, ceiling)
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

// ValidateForSave adds the rules that only apply when an operator is saving the
// web analytics settings themselves.
//
// Deliberately not folded into Validate: that one also runs on a plain
// workspace update, so a workspace enabled before this rule existed would find
// itself unable to change its name or timezone until someone filled in a domain
// list it never needed. Here the operator is already editing this very screen.
func (s *WebAnalyticsSettings) ValidateForSave() error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s == nil {
		return nil
	}
	// An empty list accepts beats from any origin, and silently withholds the
	// identity token from every tracked email link. That is a defensible state
	// to have been left in, but not one to switch collection on into.
	if s.Enabled && len(s.AllowedDomains) == 0 {
		return fmt.Errorf("allowed_domains must list at least one domain to enable web analytics")
	}
	return nil
}

// validateAllowedDomain accepts bare hostnames and single leading wildcards
// ("example.com", "*.example.com").
//
// A wildcard over a single label ("*.com", "*.io") is refused. This list is not
// only the beat-origin gate: it is also the allowlist deciding which link hosts
// receive a recipient's identity token, and "*.com" there hands every recipient
// a bearer identity to every .com link the email happens to contain.
//
// Two things this rule does NOT do, so nothing downstream should assume them.
// It only runs when settings are saved, so a workspace that stored "*.com"
// before it existed keeps that value — matching still has to cope with one.
// And a label count cannot tell a public suffix from a registrable domain
// without a public-suffix list, so "*.co.uk" still passes here.
func validateAllowedDomain(domain string) error {
	d := strings.TrimSpace(domain)
	if d == "" {
		return fmt.Errorf("allowed domain cannot be empty")
	}
	if wild, ok := strings.CutPrefix(d, "*."); ok {
		if !strings.Contains(wild, ".") {
			return fmt.Errorf("invalid allowed domain %q: a wildcard must cover your own domain, such as %q", domain, "*.example.com")
		}
		d = wild
	}
	if strings.ContainsAny(d, " */?#@") || strings.Contains(d, "://") {
		return fmt.Errorf("invalid allowed domain: %q", domain)
	}
	// An entry carrying a port can never match. Both readings of this list
	// compare against url.Hostname(), which has the port stripped off, so
	// "example.com:443" looks like a correct entry, matches no origin and no
	// link host, and disables itself in silence. Refusing it is how the admin
	// finds out — it protects nothing already stored, where the port is dealt
	// with at match time by MatchesAllowedHost.
	//
	// A bare IP literal is exempt: url.Hostname() yields "::1" for an IPv6
	// origin, brackets already stripped, so that is the form such an entry has
	// to be written in. Anything else containing a colon, including the
	// bracketed "[::1]", cannot equal a hostname.
	if net.ParseIP(d) == nil && strings.Contains(d, ":") {
		return fmt.Errorf("invalid allowed domain %q: enter the hostname on its own, without a port (%q, not %q)",
			domain, "example.com", "example.com:443")
	}
	return nil
}

// MatchesAllowedDomain reports whether hostname matches any configured allowed
// domain, with "*.example.com" matching both subdomains and the apex.
//
// This is the beat-origin gate, and here an empty list allows every hostname
// (Staminads behavior): a workspace that turned tracking on without naming a
// domain still collects its traffic. That fail-open is the difference from
// notifuse_mjml.MatchesAllowedHost, which reads the same list to decide whether
// a link may carry a recipient's identity and therefore treats an empty list as
// "no host at all". Same list, opposite defaults, because one drops analytics
// and the other releases a bearer credential.
//
// Everything else is that shared implementation, entry by entry, so a value the
// link rewriter refuses to trust is one no origin can beat under either. That
// includes the wildcard over a bare suffix ("*.com") which the shared matcher
// skips: validateAllowedDomain now refuses to store one, and an entry the
// product will not accept must not quietly keep admitting traffic here. A
// workspace still holding one from before that validation has to replace it
// with its own domain ("*.example.com") for those origins to be accepted again.
func (s *WebAnalyticsSettings) MatchesAllowedDomain(hostname string) bool {
	if s == nil || len(s.AllowedDomains) == 0 {
		return true
	}
	return notifuse_mjml.MatchesAllowedHost(hostname, s.AllowedDomains)
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

	// AnonymizeContact clears the stored identity for one address across all
	// three tables, so deleting a contact actually erases the link to their
	// browsing. The rows themselves stay: they are anonymous analytics once the
	// address is gone, and deleting them would silently rewrite historical
	// traffic totals.
	AnonymizeContact(ctx context.Context, workspaceID string, email string) error

	// ProjectContactNavigation refreshes the contact timeline from the given
	// sessions' persisted rows: one entry per pageview, one per session, for
	// identified visitors only. Idempotent — calling it again with the same
	// sessions is how a visit's final state ends up recorded. Must run after the
	// flush has committed and outside its transaction.
	ProjectContactNavigation(ctx context.Context, workspaceID string, sessions []*WebSession) error

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

	// RecomputeUsage recounts one UTC month of metered usage — pageviews and
	// billable timeline entries — and stores the snapshot in monthly_usage.
	//
	// live must be true only for the month still being written to. A closed
	// month's stored counts are never lowered, so retention dropping a
	// web_pages partition, or a contact deletion removing timeline rows, cannot
	// rewrite a month that has already been reported.
	RecomputeUsage(ctx context.Context, workspaceID string, month time.Time, live bool) error

	// GetUsage returns the stored snapshots for the given UTC months in
	// ascending order. Months with no snapshot are omitted rather than returned
	// as zero, so a caller can tell "nothing metered yet" from "metered zero".
	GetUsage(ctx context.Context, workspaceID string, months []time.Time) ([]*MonthlyUsage, error)
}

// MonthlyUsage is one UTC month of metered usage for a workspace: the counts a
// plan quota is measured against.
//
// Both counters are recomputed snapshots rather than running totals — see
// schema.UsageTableDefinitions. Pageviews are COUNT(*) over web_pages, one row
// per page a visitor opened, which is the unit the pricing page publishes.
// TimelineEntries excludes the rows the web analytics projection writes, so a
// pageview is never metered as an event as well.
type MonthlyUsage struct {
	// PeriodMonth is the first day of the UTC month, at midnight UTC.
	PeriodMonth     time.Time `json:"period_month"`
	Pageviews       int64     `json:"pageviews"`
	TimelineEntries int64     `json:"timeline_entries"`
	ComputedAt      time.Time `json:"computed_at"`
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
