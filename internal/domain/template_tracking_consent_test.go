package domain

import (
	"net/url"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func trackingTemplateDataRequest(contact *Contact) TemplateDataRequest {
	return TemplateDataRequest{
		WorkspaceID:        "ws123",
		WorkspaceSecretKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ContactWithList:    ContactWithList{Contact: contact},
		MessageID:          "msg-1",
		TrackingSettings:   notifuse_mjml.TrackingSettings{Endpoint: "https://track.example.com", EnableTracking: true},
	}
}

func TestBuildTemplateData_TrackingPreferenceURLs(t *testing.T) {
	data, err := BuildTemplateData(trackingTemplateDataRequest(&Contact{Email: "a@example.com"}))
	require.NoError(t, err)

	for key, action := range map[string]string{"tracking_preferences_url": "tracking", "tracking_optout_url": "tracking_optout"} {
		raw, ok := data[key].(string)
		require.True(t, ok, key)
		require.True(t, strings.HasPrefix(raw, "https://track.example.com/notification-center?"), raw)
		parsed, err := url.Parse(raw)
		require.NoError(t, err)
		q := parsed.Query()
		assert.Equal(t, action, q.Get("action"), key)
		assert.Equal(t, "a@example.com", q.Get("email"))
		assert.Equal(t, "ws123", q.Get("wid"))
		assert.Equal(t, ComputeEmailHMAC("a@example.com", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"), q.Get("email_hmac"))
	}
}

func TestBuildTemplateData_TrackingPreferenceURLsNeedAContact(t *testing.T) {
	data, err := BuildTemplateData(trackingTemplateDataRequest(nil))
	require.NoError(t, err)
	assert.NotContains(t, data, "tracking_preferences_url")
	assert.NotContains(t, data, "tracking_optout_url")
}

func TestBuildTemplateData_TrackingOpensURLFollowsTheDecision(t *testing.T) {
	tracked, err := BuildTemplateData(trackingTemplateDataRequest(&Contact{Email: "a@example.com"}))
	require.NoError(t, err)
	assert.Contains(t, tracked["tracking_opens_url"], "https://track.example.com/t/")

	req := trackingTemplateDataRequest(&Contact{Email: "a@example.com"})
	EmailTrackingDecision{Clicks: true, UTM: true}.Apply(&req.TrackingSettings)
	suppressed, err := BuildTemplateData(req)
	require.NoError(t, err)
	assert.Equal(t, "", suppressed["tracking_opens_url"])
}
