package notifuse_mjml

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/pkg/trackingtoken"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	signingTestClickRegex = regexp.MustCompile(`href="https://track\.example\.com/r/([^"]+)"`)
	signingTestOpenRegex  = regexp.MustCompile(`src="https://track\.example\.com/t/([^"]+)"`)
)

func TestTrackLinks_SignsLinksAndPixelWithSigningKey(t *testing.T) {
	key, err := trackingtoken.DeriveKey("workspace-secret")
	require.NoError(t, err)

	result, err := TrackLinks(`<html><body><a href="https://shop.example.com/x">Buy</a></body></html>`, TrackingSettings{
		EnableTracking: true,
		Endpoint:       "https://track.example.com",
		WorkspaceID:    "ws-1",
		MessageID:      "msg-1",
		SigningKey:     key,
	})
	require.NoError(t, err)

	click := signingTestClickRegex.FindStringSubmatch(result)
	require.NotNil(t, click, result)
	tok, err := trackingtoken.Parse(click[1], trackingtoken.PurposeClick)
	require.NoError(t, err)
	assert.Equal(t, "msg-1", tok.MessageID)
	assert.Equal(t, "ws-1", tok.WorkspaceID)
	assert.Equal(t, "https://shop.example.com/x", tok.URL)
	assert.True(t, tok.Verify(key, trackingtoken.PurposeClick))

	pixel := signingTestOpenRegex.FindStringSubmatch(result)
	require.NotNil(t, pixel, result)
	openTok, err := trackingtoken.Parse(pixel[1], trackingtoken.PurposeOpen)
	require.NoError(t, err)
	assert.Equal(t, "msg-1", openTok.MessageID)
	assert.True(t, openTok.Verify(key, trackingtoken.PurposeOpen))
}

func TestTrackLinks_WithoutSigningKeyMintsUnsignedTokens(t *testing.T) {
	result, err := TrackLinks(`<html><body><a href="https://shop.example.com/x">Buy</a></body></html>`, TrackingSettings{
		EnableTracking: true,
		Endpoint:       "https://track.example.com",
		WorkspaceID:    "ws-1",
		MessageID:      "msg-1",
	})
	require.NoError(t, err)
	assert.NotContains(t, result, "/visit")
	assert.NotContains(t, result, "/opens")

	click := signingTestClickRegex.FindStringSubmatch(result)
	require.NotNil(t, click, result)
	tok, err := trackingtoken.Parse(click[1], trackingtoken.PurposeClick)
	require.NoError(t, err)
	assert.Empty(t, tok.Tag)
	assert.Equal(t, "https://shop.example.com/x", tok.URL)

	pixel := signingTestOpenRegex.FindStringSubmatch(result)
	require.NotNil(t, pixel, result)
	openTok, err := trackingtoken.Parse(pixel[1], trackingtoken.PurposeOpen)
	require.NoError(t, err)
	assert.Empty(t, openTok.Tag)
}

func TestGenerateOpenTrackingURL(t *testing.T) {
	key, err := trackingtoken.DeriveKey("workspace-secret")
	require.NoError(t, err)
	u := GenerateOpenTrackingURL(key, "ws-1", "msg-1", "https://track.example.com", 1730000000)
	require.True(t, strings.HasPrefix(u, "https://track.example.com/t/"), u)
	tok, err := trackingtoken.Parse(strings.TrimPrefix(u, "https://track.example.com/t/"), trackingtoken.PurposeOpen)
	require.NoError(t, err)
	assert.Equal(t, int64(1730000000), tok.Timestamp)
	assert.True(t, tok.Verify(key, trackingtoken.PurposeOpen))
}

func TestTrackingSettings_SigningKeyNeverInJSON(t *testing.T) {
	var settings TrackingSettings
	require.NoError(t, json.Unmarshal([]byte(`{"enable_tracking":true,"signing_key":"AAAA","SigningKey":"AAAA"}`), &settings))
	assert.Empty(t, settings.SigningKey, "a request body must never carry a signing key")

	settings.SigningKey = []byte("secret-key-bytes")
	encoded, err := json.Marshal(settings)
	require.NoError(t, err)
	assert.NotContains(t, strings.ToLower(string(encoded)), "signing")

	value, err := settings.Value()
	require.NoError(t, err)
	assert.NotContains(t, strings.ToLower(string(value.([]byte))), "signing")
}
