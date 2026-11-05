package domain

import "fmt"

// EmailTrackingSettings is the workspace's email tracking policy: which of
// opens and clicks it tracks, and what it does for a recipient whose consent
// is unknown.
type EmailTrackingSettings struct {
	OpensEnabled   bool   `json:"opens_enabled"`
	ClicksEnabled  bool   `json:"clicks_enabled"`
	UnknownConsent string `json:"unknown_consent"`
}

// DefaultEmailTrackingSettings is what a new workspace starts with.
func DefaultEmailTrackingSettings() *EmailTrackingSettings {
	return &EmailTrackingSettings{OpensEnabled: true, ClicksEnabled: true, UnknownConsent: UnknownConsentTrack}
}

// Validate checks the unknown-consent choice.
func (s *EmailTrackingSettings) Validate() error {
	switch s.UnknownConsent {
	case UnknownConsentTrack, UnknownConsentNoTrack:
		return nil
	default:
		return fmt.Errorf("invalid email_tracking.unknown_consent %q: must be %q or %q", s.UnknownConsent, UnknownConsentTrack, UnknownConsentNoTrack)
	}
}

// TrackingPolicy returns the workspace half of the tracking decision. A
// workspace that never saved EmailTracking keeps its single switch for both
// opens and clicks and tracks unknown contacts, as it always did.
func (ws WorkspaceSettings) TrackingPolicy() EmailTrackingPolicy {
	if ws.EmailTracking == nil {
		return EmailTrackingPolicy{
			OpensEnabled:   ws.EmailTrackingEnabled,
			ClicksEnabled:  ws.EmailTrackingEnabled,
			UnknownConsent: UnknownConsentTrack,
		}
	}
	return EmailTrackingPolicy{
		OpensEnabled:   ws.EmailTracking.OpensEnabled,
		ClicksEnabled:  ws.EmailTracking.ClicksEnabled,
		UnknownConsent: ws.EmailTracking.UnknownConsent,
	}
}

func (ws *WorkspaceSettings) validateEmailTracking() error {
	if ws.emailTrackingNull {
		return fmt.Errorf("email_tracking cannot be null")
	}
	if ws.EmailTracking != nil {
		return ws.EmailTracking.Validate()
	}
	return nil
}

func cloneEmailTracking(s *EmailTrackingSettings) *EmailTrackingSettings {
	if s == nil {
		return nil
	}
	c := *s
	return &c
}

func sameEmailTracking(a, b *EmailTrackingSettings) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// reconcileEmailTracking settles email_tracking and email_tracking_enabled
// against the stored settings, after the omitted keys were restored (an
// omitted email_tracking arrives here as nil).
//
//   - email_tracking sent and different from the stored value: it decides, and
//     email_tracking_enabled follows it.
//   - otherwise, email_tracking_enabled different from the stored value: an
//     older client flipped the single switch, so both opens and clicks follow
//     it and the unknown-consent choice is kept.
//   - otherwise the stored policy is kept as is.
//
// The result is always a fresh struct: the stored pointer may be shared with
// the audit snapshot.
func (ws *WorkspaceSettings) reconcileEmailTracking(stored WorkspaceSettings) {
	if ws.EmailTracking != nil && !sameEmailTracking(ws.EmailTracking, stored.EmailTracking) {
		ws.EmailTracking = cloneEmailTracking(ws.EmailTracking)
		ws.EmailTrackingEnabled = ws.EmailTracking.OpensEnabled || ws.EmailTracking.ClicksEnabled
		return
	}
	if ws.EmailTrackingEnabled != stored.EmailTrackingEnabled {
		unknown := UnknownConsentTrack
		if stored.EmailTracking != nil && stored.EmailTracking.UnknownConsent != "" {
			unknown = stored.EmailTracking.UnknownConsent
		}
		ws.EmailTracking = &EmailTrackingSettings{
			OpensEnabled:   ws.EmailTrackingEnabled,
			ClicksEnabled:  ws.EmailTrackingEnabled,
			UnknownConsent: unknown,
		}
		return
	}
	ws.EmailTracking = cloneEmailTracking(stored.EmailTracking)
}
