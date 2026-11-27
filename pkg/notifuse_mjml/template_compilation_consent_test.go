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

func TestTrackLinks_LeavesOwnPageLinksUntouched(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		href     string
	}{
		{"tracking optout", "https://track.example.com", "https://track.example.com/notification-center?email=a%40b.c&amp;action=tracking_optout&amp;wid=ws-1"},
		{"tracking preferences", "https://track.example.com", "https://track.example.com/notification-center?action=tracking&amp;email=a%40b.c"},
		{"unsubscribe", "https://track.example.com", "https://track.example.com/notification-center?action=unsubscribe&amp;email=a%40b.c&amp;lid=l1&amp;mid=m1"},
		{"subscription confirmation", "https://track.example.com", "https://track.example.com/notification-center?action=confirm&amp;email=a%40b.c&amp;lid=l1"},
		{"unsubscribe confirmation", "https://track.example.com", "https://track.example.com/notification-center?action=confirm_unsubscribe&amp;lid=l1"},
		{"preferences, no action", "https://track.example.com", "https://track.example.com/notification-center?email=a%40b.c&amp;email_hmac=h&amp;wid=ws-1"},
		{"one-click unsubscribe URL", "https://track.example.com", "https://track.example.com/unsubscribe-oneclick?email=a%40b.c&amp;lids=l1"},
		{"web version", "https://track.example.com", "https://track.example.com/v/AbC-d_9"},
		{"web version preview", "https://track.example.com", "https://track.example.com/v/preview"},
		{"plain http", "http://track.example.com", "http://track.example.com/notification-center?action=unsubscribe"},
		{"page with trailing slash", "https://track.example.com", "https://track.example.com/notification-center/?action=unsubscribe"},
		{"endpoint with trailing slash", "https://track.example.com/", "https://track.example.com/notification-center?action=tracking_optout"},
		{"host case differs", "https://Track.Example.com", "https://track.example.com/notification-center?action=unsubscribe"},
		{"endpoint with port", "https://track.example.com:8443", "https://track.example.com:8443/v/AbC"},
		{"endpoint with path prefix", "https://example.com/notifuse", "https://example.com/notifuse/notification-center?action=unsubscribe"},
		{"web version under a path prefix", "https://example.com/notifuse", "https://example.com/notifuse/v/AbC"},
		// Browsers open it, though net/url refuses the escape
		{"invalid percent escape", "https://track.example.com", "https://track.example.com/notification-center?action=tracking_optout#50%off"},
		// Browsers strip the spaces before opening it
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

func TestIsOwnPageLink_ReadsLinksLeniently(t *testing.T) {
	settings := consentTestSettings()

	assert.True(t, settings.isOwnPageLink(" https://track.example.com/notification-center?action=tracking_optout"))
	assert.True(t, settings.isOwnPageLink("https://track.example.com/notification-center?action=unsubscribe&ref=50%off"))
	assert.True(t, settings.isOwnPageLink("HTTPS://track.example.com/v/AbC"))
	// Another scheme on the endpoint's host is not one of its pages
	assert.False(t, settings.isOwnPageLink("ftp://track.example.com/notification-center?action=unsubscribe"))
	// A percent sign in the host is not this endpoint's host
	assert.False(t, settings.isOwnPageLink("https://track.example.com%/notification-center?action=tracking_optout"))
}

func TestTrackLinks_TracksOtherPagesOfTheEndpointHost(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		href     string
	}{
		{"blog post on the custom domain", "https://track.example.com", "https://track.example.com/blog/post"},
		{"home page", "https://track.example.com", "https://track.example.com/"},
		{"longer path name", "https://track.example.com", "https://track.example.com/notification-centerx?action=unsubscribe"},
		{"page under the notification center path", "https://track.example.com", "https://track.example.com/notification-center/other"},
		{"post under the one-click path", "https://track.example.com", "https://track.example.com/unsubscribe-oneclick/post"},
		{"path starting with v", "https://track.example.com", "https://track.example.com/vote"},
		{"web version path without a slash", "https://track.example.com", "https://track.example.com/v"},
		{"web version path without a token", "https://track.example.com", "https://track.example.com/v/"},
		{"web version token followed by a path", "https://track.example.com", "https://track.example.com/v/AbC/more"},
		// A browser opens /blog/post: it is not the web version
		{"dot segments after the web version path", "https://track.example.com", "https://track.example.com/v/../blog/post"},
		{"escaped dot segment", "https://track.example.com", "https://track.example.com/v/%2e%2e"},
		{"outside the endpoint's path prefix", "https://example.com/notifuse", "https://example.com/notification-center?action=unsubscribe"},
		{"another port of the endpoint's host", "https://track.example.com", "https://track.example.com:8443/notification-center?action=unsubscribe"},
		{"another scheme on the endpoint's host", "https://track.example.com", "ftp://track.example.com/notification-center?action=unsubscribe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := consentTestSettings()
			settings.Endpoint = tc.endpoint
			endpoint := strings.TrimRight(tc.endpoint, "/")

			html := `<html><body><a href="` + tc.href + `">Page</a></body></html>`
			result, err := TrackLinks(html, settings)
			require.NoError(t, err)
			assert.Contains(t, result, `href="`+endpoint+`/r/`, "the link must go through the click redirect")
		})
	}
}

func TestTrackLinks_OwnPageLinksOnAnotherHostAreTracked(t *testing.T) {
	for _, href := range []string{
		"https://other.example.com/notification-center?action=tracking_optout",
		"https://other.example.com/notification-center?action=unsubscribe&amp;lid=l1",
		"https://other.example.com/unsubscribe-oneclick?lids=l1",
		"https://other.example.com/v/AbC",
	} {
		t.Run(href, func(t *testing.T) {
			settings := consentTestSettings()
			html := `<html><body><a href="` + href + `">Other</a></body></html>`

			result, err := TrackLinks(html, settings)
			require.NoError(t, err)
			assert.Contains(t, result, `href="https://track.example.com/r/`)
		})
	}
}

func TestTrackLinks_OwnPageLinksBesideTrackedLinks(t *testing.T) {
	// The footer of a newsletter: the content link is tracked and tagged, the
	// unsubscribe and preferences links are sent as written.
	settings := consentTestSettings()
	unsubscribe := "https://track.example.com/notification-center?action=unsubscribe&amp;lid=l1"
	preferences := "https://track.example.com/notification-center?email=a%40b.c"
	html := `<html><body><a href="https://shop.example.com/x">Buy</a>` +
		`<a href="` + unsubscribe + `">Unsubscribe</a><a href="` + preferences + `">Preferences</a></body></html>`

	result, err := TrackLinks(html, settings)
	require.NoError(t, err)
	assert.Contains(t, result, `href="`+unsubscribe+`"`)
	assert.Contains(t, result, `href="`+preferences+`"`)
	assert.Equal(t, 1, strings.Count(result, `href="https://track.example.com/r/`), "only the content link is tracked")
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
