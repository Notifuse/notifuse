package notifuse_mjml

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const consentTestHTML = `<html><body><a href="https://shop.example.com/x">Buy</a></body></html>`

func consentTestSettings() TrackingSettings {
	return TrackingSettings{
		EnableTracking: true,
		Endpoint:       "https://track.example.com",
		WorkspaceID:    "ws-1",
		MessageID:      "msg-1",
		UTMSource:      "newsletter",
	}
}

func TestTrackLinks_SuppressOpensKeepsClickTrackingWithoutPixel(t *testing.T) {
	settings := consentTestSettings()
	settings.SuppressOpens = true

	result, err := TrackLinks(consentTestHTML, settings)
	require.NoError(t, err)

	assert.Contains(t, result, `href="https://track.example.com/r/`)
	assert.NotContains(t, result, "https://track.example.com/t/")
}

func TestTrackLinks_SuppressClicksKeepsPixelAndUTMWithoutRedirect(t *testing.T) {
	settings := consentTestSettings()
	settings.SuppressClicks = true

	result, err := TrackLinks(consentTestHTML, settings)
	require.NoError(t, err)

	assert.NotContains(t, result, "https://track.example.com/r/")
	assert.Contains(t, result, `href="https://shop.example.com/x?utm_source=newsletter"`)
	assert.Contains(t, result, `src="https://track.example.com/t/`)
}

func TestTrackLinks_SuppressBothKeepsOnlyUTM(t *testing.T) {
	settings := consentTestSettings()
	settings.SuppressOpens = true
	settings.SuppressClicks = true

	result, err := TrackLinks(consentTestHTML, settings)
	require.NoError(t, err)

	assert.NotContains(t, result, "https://track.example.com/r/")
	assert.NotContains(t, result, "https://track.example.com/t/")
	assert.Contains(t, result, `href="https://shop.example.com/x?utm_source=newsletter"`)
}

func TestTrackLinks_SuppressBothWithoutUTMReturnsHTMLUnchanged(t *testing.T) {
	settings := consentTestSettings()
	settings.UTMSource = ""
	settings.SuppressOpens = true
	settings.SuppressClicks = true

	result, err := TrackLinks(consentTestHTML, settings)
	require.NoError(t, err)
	assert.Equal(t, consentTestHTML, result)
}

func TestTrackLinks_NoTrackingModeKeepsUTMOnly(t *testing.T) {
	// no_tracking reaches TrackLinks as EnableTracking=false (the send paths
	// resolve it), and unlike TrackingModeDisabled it must keep the UTMs.
	settings := consentTestSettings()
	settings.EnableTracking = false
	settings.TrackingMode = TrackingModeNoTracking

	result, err := TrackLinks(consentTestHTML, settings)
	require.NoError(t, err)

	assert.NotContains(t, result, "https://track.example.com/r/")
	assert.NotContains(t, result, "https://track.example.com/t/")
	assert.Contains(t, result, `href="https://shop.example.com/x?utm_source=newsletter"`)
}

func TestTrackLinks_DisabledModeStillLeavesEverythingUntouched(t *testing.T) {
	settings := consentTestSettings()
	settings.TrackingMode = TrackingModeDisabled

	result, err := TrackLinks(consentTestHTML, settings)
	require.NoError(t, err)
	assert.Equal(t, consentTestHTML, result)
}

func TestTrackLinks_LeavesTrackingPreferenceLinksUntouched(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		href     string
	}{
		{"optout", "https://track.example.com", "https://track.example.com/notification-center?email=a%40b.c&amp;action=tracking_optout&amp;wid=ws-1"},
		{"preferences", "https://track.example.com", "https://track.example.com/notification-center?action=tracking&amp;email=a%40b.c"},
		{"endpoint with trailing slash", "https://track.example.com/", "https://track.example.com/notification-center?action=tracking_optout"},
		{"host case differs", "https://Track.Example.com", "https://track.example.com/notification-center?action=tracking_optout"},
		{"endpoint with path prefix", "https://example.com/notifuse", "https://example.com/notifuse/notification-center?action=tracking_optout"},
		// Browsers open it, and the click route reads it, though net/url refuses the escape
		{"invalid percent escape", "https://track.example.com", "https://track.example.com/notification-center?action=tracking_optout#50%off"},
		{"surrounding spaces", "https://track.example.com", " https://track.example.com/notification-center?action=tracking "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := consentTestSettings()
			settings.Endpoint = tc.endpoint
			settings.IdentifyToken = "tok"
			settings.IdentifyAllowedHosts = []string{"track.example.com", "example.com"}

			html := `<html><body><a href="` + tc.href + `">Stop</a></body></html>`
			result, err := TrackLinks(html, settings)
			require.NoError(t, err)

			assert.Contains(t, result, `href="`+tc.href+`"`, "the link must be left byte for byte")
			assert.NotContains(t, result, "/r/")
			assert.NotContains(t, result, "utm_source")
			assert.NotContains(t, result, WebIdentifyQueryParam+"=")
		})
	}
}

func TestTrackLinks_StillTracksOtherNotificationCenterLinks(t *testing.T) {
	settings := consentTestSettings()
	html := `<html><body><a href="https://track.example.com/notification-center?action=unsubscribe&amp;lid=l1">Unsubscribe</a></body></html>`

	result, err := TrackLinks(html, settings)
	require.NoError(t, err)
	assert.Contains(t, result, `href="https://track.example.com/r/`)
}

func TestTrackLinks_TrackingActionOnAnotherHostIsTracked(t *testing.T) {
	settings := consentTestSettings()
	html := `<html><body><a href="https://other.example.com/notification-center?action=tracking_optout">Other</a></body></html>`

	result, err := TrackLinks(html, settings)
	require.NoError(t, err)
	assert.Contains(t, result, `href="https://track.example.com/r/`)
}

func TestTrackingSettings_PerRecipientDecisionsAreNotSerialized(t *testing.T) {
	settings := consentTestSettings()
	settings.SuppressOpens = true
	settings.SuppressClicks = true
	settings.SuppressIdentify = true

	data, err := json.Marshal(settings)
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(data), "suppress"), string(data))
}

func TestTrackingSettings_IsZeroSeesSuppressFlags(t *testing.T) {
	assert.False(t, TrackingSettings{SuppressOpens: true}.IsZero())
	assert.False(t, TrackingSettings{SuppressClicks: true}.IsZero())
	assert.False(t, TrackingSettings{SuppressIdentify: true}.IsZero())
}

func TestValidateTrackingMode_AcceptsNoTracking(t *testing.T) {
	assert.NoError(t, ValidateTrackingMode(TrackingModeNoTracking))
	assert.Error(t, ValidateTrackingMode("never"))
}
