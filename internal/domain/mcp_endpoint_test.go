package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalAPIEndpoint(t *testing.T) {
	for raw, want := range map[string]CanonicalEndpoint{
		"https://notifuse.example.com:443/": {URL: "https://notifuse.example.com", Origin: "https://notifuse.example.com", Host: "notifuse.example.com", Scheme: "https", Hostname: "notifuse.example.com"},
		"HTTPS://Notifuse.Example.COM":      {URL: "https://notifuse.example.com", Origin: "https://notifuse.example.com", Host: "notifuse.example.com", Scheme: "https", Hostname: "notifuse.example.com"},
		"http://localhost:8080":             {URL: "http://localhost:8080", Origin: "http://localhost:8080", Host: "localhost:8080", HasPort: true, Scheme: "http", Hostname: "localhost"},
		"http://[::1]:80/":                  {URL: "http://[::1]", Origin: "http://[::1]", Host: "[::1]", Scheme: "http", Hostname: "::1"},
		" https://example.com/notifuse/ ":   {URL: "https://example.com/notifuse", Origin: "https://example.com", Host: "example.com", Scheme: "https", Hostname: "example.com", Path: "/notifuse"},
	} {
		got, err := CanonicalAPIEndpoint(raw)
		require.NoError(t, err, raw)
		assert.Equal(t, want, got, raw)
	}
	for _, raw := range []string{"", "notifuse.example.com", "ftp://example.com", "https://"} {
		_, err := CanonicalAPIEndpoint(raw)
		assert.Error(t, err, raw)
	}
}

func TestCanonicalEndpoint_MatchesHost(t *testing.T) {
	endpoint, err := CanonicalAPIEndpoint("https://notifuse.example.com")
	require.NoError(t, err)
	assert.True(t, endpoint.MatchesHost("NOTIFUSE.example.com"))
	assert.True(t, endpoint.MatchesHost("notifuse.example.com:443"), "no port in the canonical URL: the request's is not compared")
	assert.False(t, endpoint.MatchesHost("other.example.com"))

	withPort, err := CanonicalAPIEndpoint("http://localhost:8080")
	require.NoError(t, err)
	assert.True(t, withPort.MatchesHost("localhost:8080"))
	assert.False(t, withPort.MatchesHost("localhost:9090"))
	assert.False(t, withPort.MatchesHost("localhost"))
}

func TestCanonicalEndpoint_MCPURLAndOAuthAvailability(t *testing.T) {
	endpoint, err := CanonicalAPIEndpoint("https://Notifuse.example.com/")
	require.NoError(t, err)
	assert.Equal(t, "https://notifuse.example.com/mcp", endpoint.MCPURL())
	assert.Equal(t, "", endpoint.OAuthUnavailableReason())

	for raw, reason := range map[string]string{
		"http://notifuse.example.com":    "API_ENDPOINT is not https",
		"http://10.0.0.5:8080":           "API_ENDPOINT is not https",
		"https://example.com/notifuse":   "API_ENDPOINT has a path",
		"http://localhost:8080":          "",
		"http://127.0.0.1:8080":          "",
		"http://[::1]:8080":              "",
		"http://localhost:8080/notifuse": "API_ENDPOINT has a path",
	} {
		endpoint, err := CanonicalAPIEndpoint(raw)
		require.NoError(t, err, raw)
		assert.Equal(t, reason, endpoint.OAuthUnavailableReason(), raw)
	}
}

func TestCanonicalResource(t *testing.T) {
	for raw, want := range map[string]string{
		"https://notifuse.example.com/mcp":      "https://notifuse.example.com/mcp",
		"HTTPS://Notifuse.Example.com:443/mcp/": "https://notifuse.example.com/mcp",
		"https://notifuse.example.com/MCP":      "https://notifuse.example.com/MCP",
	} {
		got, err := CanonicalResource(raw)
		require.NoError(t, err, raw)
		assert.Equal(t, want, got, raw)
	}
	for _, raw := range []string{
		"https://notifuse.example.com/mcp?x=1",
		"https://notifuse.example.com/mcp?",
		"https://notifuse.example.com/mcp#x",
		"https://user@notifuse.example.com/mcp",
		"mcp",
		"urn:notifuse",
	} {
		_, err := CanonicalResource(raw)
		assert.Error(t, err, raw)
	}
}
