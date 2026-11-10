package domain

import (
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/pkg/trackingtoken"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTemplateData_ViewInBrowserURL(t *testing.T) {
	req := trackingTemplateDataRequest(&Contact{Email: "a@example.com"})
	req.WebVersion = true

	data, err := BuildTemplateData(req)
	require.NoError(t, err)

	raw, ok := data["view_in_browser_url"].(string)
	require.True(t, ok)
	require.True(t, strings.HasPrefix(raw, "https://track.example.com/v/"), raw)

	tok, err := trackingtoken.Parse(strings.TrimPrefix(raw, "https://track.example.com/v/"), trackingtoken.PurposeView)
	require.NoError(t, err)
	assert.Equal(t, "msg-1", tok.MessageID)
	assert.Equal(t, "ws123", tok.WorkspaceID)
	assert.True(t, tok.Verify(TrackingSigningKey(req.WorkspaceSecretKey), trackingtoken.PurposeView))
}

func TestBuildTemplateData_ViewInBrowserURLTrimsTheEndpointSlash(t *testing.T) {
	req := trackingTemplateDataRequest(&Contact{Email: "a@example.com"})
	req.WebVersion = true
	req.TrackingSettings.Endpoint = "https://track.example.com/"

	data, err := BuildTemplateData(req)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(data["view_in_browser_url"].(string), "https://track.example.com/v/"))
}

func TestBuildTemplateData_ViewInBrowserURLOverridesCallerData(t *testing.T) {
	req := trackingTemplateDataRequest(&Contact{Email: "a@example.com"})
	req.WebVersion = true
	req.ProvidedData = MapOfAny{"view_in_browser_url": "https://caller.example.com/archive"}

	data, err := BuildTemplateData(req)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(data["view_in_browser_url"].(string), "https://track.example.com/v/"))
}

func TestBuildTemplateData_ViewInBrowserURLEmptyOutsideItsScope(t *testing.T) {
	data, err := BuildTemplateData(trackingTemplateDataRequest(&Contact{Email: "a@example.com"}))
	require.NoError(t, err)
	value, present := data["view_in_browser_url"]
	assert.True(t, present, "templates using the variable must render it empty, not fail")
	assert.Equal(t, "", value)
}

func TestBuildTemplateData_ViewInBrowserURLKeepsCallerDataOutsideItsScope(t *testing.T) {
	req := trackingTemplateDataRequest(&Contact{Email: "a@example.com"})
	req.ProvidedData = MapOfAny{"view_in_browser_url": "https://caller.example.com/archive"}

	data, err := BuildTemplateData(req)
	require.NoError(t, err)
	assert.Equal(t, "https://caller.example.com/archive", data["view_in_browser_url"])
}
