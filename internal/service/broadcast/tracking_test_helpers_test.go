package broadcast

import "github.com/Notifuse/notifuse/internal/domain"

// testTrackingPolicy is the workspace policy a single tracking switch stood
// for before opens and clicks were split: both on or both off, unknown
// contacts tracked.
func testTrackingPolicy(enabled bool) domain.EmailTrackingPolicy {
	return domain.EmailTrackingPolicy{OpensEnabled: enabled, ClicksEnabled: enabled, UnknownConsent: domain.UnknownConsentTrack}
}

// testTrackingDecision is what that policy decides for a contact with no
// recorded consent.
func testTrackingDecision(enabled bool) domain.EmailTrackingDecision {
	return domain.ResolveEmailTracking(testTrackingPolicy(enabled), nil, "")
}
