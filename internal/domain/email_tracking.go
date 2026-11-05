package domain

import (
	"fmt"

	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

// A contact's consent to open or click tracking. A contact with no value
// recorded (the column is NULL) is "unknown", and the workspace decides what
// unknown means through EmailTrackingSettings.UnknownConsent.
const (
	TrackingConsentGranted = "granted"
	TrackingConsentDenied  = "denied"
)

// What the workspace does for a recipient whose consent is unknown.
const (
	UnknownConsentTrack   = "track"
	UnknownConsentNoTrack = "no_track"
)

// Where a contact's tracking consent was last changed. The server sets one of
// these for every write it makes on the contact's behalf; an API call or an
// import may bring its own, to carry over a history recorded elsewhere.
const (
	TrackingConsentSourceAPI                = "api"
	TrackingConsentSourceConsole            = "console"
	TrackingConsentSourceImport             = "import"
	TrackingConsentSourceNotificationCenter = "notification_center"
	TrackingConsentSourceEmailLink          = "email_link"
	TrackingConsentSourceSubscribeForm      = "subscribe_form"
)

// MaxTrackingConsentSourceLength bounds tracking_consent_source (VARCHAR(50)).
const MaxTrackingConsentSourceLength = 50

// ValidateTrackingConsentValue accepts granted, denied, or an empty value.
func ValidateTrackingConsentValue(value string) error {
	switch value {
	case "", TrackingConsentGranted, TrackingConsentDenied:
		return nil
	default:
		return fmt.Errorf("invalid tracking consent %q: must be %q or %q", value, TrackingConsentGranted, TrackingConsentDenied)
	}
}

// EmailTrackingPolicy is the workspace half of the tracking decision.
type EmailTrackingPolicy struct {
	OpensEnabled   bool
	ClicksEnabled  bool
	UnknownConsent string
}

// EmailTrackingDecision is what one email to one recipient carries.
type EmailTrackingDecision struct {
	// Opens: the open pixel is written and an open can be recorded.
	Opens bool
	// Clicks: links go through the click redirect and a click can be recorded.
	Clicks bool
	// Identify: the web analytics identity token may be minted for the links.
	Identify bool
	// UTM: UTM parameters are written onto the links.
	UTM bool
}

// ResolveEmailTracking decides what an email to contact tracks. contact may be
// nil (a test send to an address that is not a contact), which counts as
// unknown consent. mode is the send's tracking mode (notifuse_mjml.TrackingMode*).
//
// A consent never turns on what the workspace has turned off. The identity
// token does not depend on the workspace's click switch, as before: a workspace
// can run web analytics without click tracking. It does depend on the
// recipient's click consent, because it identifies the recipient on the site.
func ResolveEmailTracking(policy EmailTrackingPolicy, contact *Contact, mode string) EmailTrackingDecision {
	if mode == notifuse_mjml.TrackingModeDisabled {
		return EmailTrackingDecision{}
	}
	if mode == notifuse_mjml.TrackingModeNoTracking {
		return EmailTrackingDecision{UTM: true}
	}

	var opensConsent, clicksConsent *NullableString
	if contact != nil {
		opensConsent = contact.TrackingOpensConsent
		clicksConsent = contact.TrackingClicksConsent
	}
	opensAllowed := consentAllows(opensConsent, policy.UnknownConsent)
	clicksAllowed := consentAllows(clicksConsent, policy.UnknownConsent)

	return EmailTrackingDecision{
		Opens:    policy.OpensEnabled && opensAllowed,
		Clicks:   policy.ClicksEnabled && clicksAllowed,
		Identify: clicksAllowed,
		UTM:      true,
	}
}

// consentAllows reads one consent: granted allows, denied refuses, and unknown
// follows the workspace (an empty UnknownConsent tracks, as Notifuse always did).
func consentAllows(consent *NullableString, unknownConsent string) bool {
	switch TrackingConsentValue(consent) {
	case TrackingConsentGranted:
		return true
	case TrackingConsentDenied:
		return false
	default:
		return unknownConsent != UnknownConsentNoTrack
	}
}

// TrackingConsentValue returns the recorded consent, or "" when it is unknown.
// A nil pointer (a NULL column) and an explicit JSON null are both unknown.
func TrackingConsentValue(consent *NullableString) string {
	if consent == nil || consent.IsNull {
		return ""
	}
	return consent.String
}

// Apply writes the decision onto the settings a send compiles with. It leaves
// TrackingMode alone: a disabled send keeps its full veto in TrackLinks.
func (d EmailTrackingDecision) Apply(ts *notifuse_mjml.TrackingSettings) {
	ts.EnableTracking = d.Opens || d.Clicks
	ts.SuppressOpens = !d.Opens
	ts.SuppressClicks = !d.Clicks
	ts.SuppressIdentify = !d.Identify
}

// TrackedFlags returns the decision as the values message_history records.
func (d EmailTrackingDecision) TrackedFlags() (opens *bool, clicks *bool) {
	o, c := d.Opens, d.Clicks
	return &o, &c
}

// TrackedFlagsFromSettings derives the message_history flags from settings a
// send already resolved: what the compiled HTML actually carries.
func TrackedFlagsFromSettings(ts notifuse_mjml.TrackingSettings) (opens *bool, clicks *bool) {
	disabled := ts.TrackingMode == notifuse_mjml.TrackingModeDisabled
	o := ts.EnableTracking && !ts.SuppressOpens && !disabled
	c := ts.EnableTracking && !ts.SuppressClicks && !disabled
	return &o, &c
}
