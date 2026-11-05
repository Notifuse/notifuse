package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdateContactPreferencesRequest_TrackingConsent(t *testing.T) {
	base := func() UpdateContactPreferencesRequest {
		return UpdateContactPreferencesRequest{WorkspaceID: "ws", Email: "a@example.com", EmailHMAC: "h"}
	}

	onlyConsent := base()
	onlyConsent.TrackingOpensConsent = TrackingConsentDenied
	assert.NoError(t, onlyConsent.Validate(), "a consent alone is enough")
	assert.True(t, onlyConsent.HasTrackingConsent())
	assert.Equal(t, TrackingConsentSourceNotificationCenter, onlyConsent.TrackingConsentSource())

	emailLink := base()
	emailLink.TrackingClicksConsent = TrackingConsentDenied
	emailLink.ConsentSource = TrackingConsentSourceEmailLink
	assert.NoError(t, emailLink.Validate())
	assert.Equal(t, TrackingConsentSourceEmailLink, emailLink.TrackingConsentSource())

	badValue := base()
	badValue.TrackingOpensConsent = "unknown"
	assert.Error(t, badValue.Validate(), "the public path cannot set a consent back to unknown")

	badSource := base()
	badSource.TrackingOpensConsent = TrackingConsentGranted
	badSource.ConsentSource = TrackingConsentSourceConsole
	assert.Error(t, badSource.Validate(), "a public caller cannot claim another source")

	nothing := base()
	assert.Error(t, nothing.Validate())
	assert.False(t, nothing.HasTrackingConsent())
}

func TestNewContactTrackingState(t *testing.T) {
	policy := EmailTrackingPolicy{OpensEnabled: true, ClicksEnabled: false, UnknownConsent: UnknownConsentNoTrack}

	unknown := NewContactTrackingState(policy, &Contact{Email: "a@example.com"})
	assert.Equal(t, &ContactTrackingState{OpensEnabled: true, ClicksEnabled: false, OpensTracked: false, ClicksTracked: false}, unknown)

	granted := NewContactTrackingState(policy, &Contact{Email: "a@example.com", TrackingOpensConsent: &NullableString{String: TrackingConsentGranted}})
	assert.Equal(t, &ContactTrackingState{OpensEnabled: true, ClicksEnabled: false, OpensTracked: true, ClicksTracked: false}, granted)
}
