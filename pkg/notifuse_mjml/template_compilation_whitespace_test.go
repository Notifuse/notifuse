package notifuse_mjml

import (
	"testing"

	"github.com/Notifuse/notifuse/pkg/trackingtoken"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTrackLinks_ReadsHrefsAsBrowsersDo covers hrefs written with whitespace
// around them or inside them. Browsers trim leading and trailing spaces and
// drop tabs and line breaks before reading a URL; TrackLinks read the raw
// value, so a trailing space got the UTM parameters after it (the recipient
// landed on /sale%20), a leading space kept the link untagged, and a leading
// space before mailto: got it wrapped in a tracking redirect.
func TestTrackLinks_ReadsHrefsAsBrowsersDo(t *testing.T) {
	utmOnly := TrackingSettings{UTMContent: "tpl"}

	cases := []struct {
		name string
		href string
		want string // the href written back
	}{
		{"trailing space", "https://shop.com/sale ", "https://shop.com/sale?utm_content=tpl"},
		{"leading space", " https://shop.com/sale", "https://shop.com/sale?utm_content=tpl"},
		{"line break inside", "https://shop.com/sa\nle", "https://shop.com/sale?utm_content=tpl"},
		{"leading tab, existing query", "\thttps://shop.com/x?a=1", "https://shop.com/x?a=1&amp;utm_content=tpl"},
		{"no whitespace is unchanged", "https://shop.com/x?a=1", "https://shop.com/x?a=1&amp;utm_content=tpl"},
		{"mailto with a leading space keeps its bytes", " mailto:hi@shop.com", " mailto:hi@shop.com"},
		{"tel with a leading space keeps its bytes", " tel:+33123456789", " tel:+33123456789"},
		{"anchor with a leading space keeps its bytes", " #top", " #top"},
		{"Liquid keeps its bytes", "https://shop.com/{{ contact.x }} ", "https://shop.com/{{ contact.x }} "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := TrackLinks(`<a href="`+tc.href+`">x</a>`, utmOnly)
			require.NoError(t, err)
			assert.Equal(t, `<a href="`+tc.want+`">x</a>`, got)
		})
	}
}

// TestTrackLinks_MintsThePreprocessedDestination checks what the signed /r/
// token carries, since the click route redirects to that string as it is.
func TestTrackLinks_MintsThePreprocessedDestination(t *testing.T) {
	key, err := trackingtoken.DeriveKey("workspace-secret")
	require.NoError(t, err)
	settings := TrackingSettings{
		EnableTracking:       true,
		Endpoint:             "https://track.example.com",
		WorkspaceID:          "ws-1",
		MessageID:            "msg-1",
		SigningKey:           key,
		UTMContent:           "tpl",
		IdentifyToken:        "tok-abc",
		IdentifyAllowedHosts: []string{"shop.com"},
	}

	for _, href := range []string{"https://shop.com/sale ", " https://shop.com/sale", "https://shop.com/sa\r\nle"} {
		result, err := TrackLinks(`<a href="`+href+`">x</a>`, settings)
		require.NoError(t, err)
		click := signingTestClickRegex.FindStringSubmatch(result)
		require.NotNil(t, click, "%q must be tracked: %s", href, result)
		tok, err := trackingtoken.Parse(click[1], trackingtoken.PurposeClick)
		require.NoError(t, err)
		assert.Equal(t, "https://shop.com/sale?utm_content=tpl&nf_id=tok-abc", tok.URL, "minted destination of %q", href)
	}

	for _, href := range []string{" mailto:hi@shop.com", " javascript:alert(1)"} {
		result, err := TrackLinks(`<a href="`+href+`">x</a>`, settings)
		require.NoError(t, err)
		assert.NotContains(t, result, "/r/", "%q must never be wrapped in a tracking redirect", href)
		assert.Contains(t, result, `href="`+href+`"`, "%q keeps its bytes", href)
	}
}
