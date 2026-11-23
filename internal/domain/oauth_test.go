package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every number the spec states, against a literal.
func TestOAuthLimits(t *testing.T) {
	assert.Equal(t, time.Hour, OAuthAccessTokenLifetime)
	assert.Equal(t, 90*24*time.Hour, OAuthRefreshTokenLifetime)
	assert.Equal(t, 60*time.Second, OAuthCodeLifetime)
	assert.Equal(t, 10*time.Minute, OAuthRequestLifetime)
	assert.Equal(t, 60*time.Second, OAuthRefreshGracePeriod)
	assert.Equal(t, 5*time.Minute, OAuthLastUsedInterval)
	assert.Equal(t, 2048, OAuthClientIDMaxLength)
	assert.Equal(t, 200, OAuthClientNameMaxLength)
	assert.Equal(t, 20, OAuthMaxRedirectURIs)
	assert.Equal(t, 2048, OAuthRedirectURIMaxLength)
	assert.Equal(t, 5*time.Second, OAuthClientFetchTimeout)
	assert.Equal(t, 65536, OAuthClientMaxBodyBytes)
	assert.Equal(t, 5*time.Minute, OAuthClientMinCache)
	assert.Equal(t, 24*time.Hour, OAuthClientMaxCache)
	assert.Equal(t, 5*time.Minute, OAuthClientFailureCache)
	assert.Equal(t, 10000, OAuthGrantlessClientCap)
	assert.Equal(t, 24*time.Hour, OAuthGrantlessClientRetention)
	assert.Equal(t, 365*24*time.Hour, OAuthRevokedClientRetention)
	assert.Equal(t, "nfmcp_at_", OAuthAccessTokenPrefix)
	assert.Equal(t, "nfmcp_rt_", OAuthRefreshTokenPrefix)
	assert.Equal(t, "mcp", OAuthScope)
}

func TestOAuthKnownClients(t *testing.T) {
	assert.Equal(t, []string{"claude.ai", "chatgpt.com"}, OAuthKnownClientHosts)

	for _, clientID := range []string{
		"https://claude.ai/oauth/claude-code-client-metadata",
		"https://chatgpt.com/oauth/client-metadata.json",
		"https://CLAUDE.ai/x",
		"HTTPS://Claude.AI/any/path",
		"https://ChatGPT.COM/x?y=z",
	} {
		assert.True(t, IsKnownOAuthClient(clientID), clientID)
	}
	for _, clientID := range []string{
		"https://claude.a\u0130/x",
		"https://\u212alaude.ai/x",
		"https://claude.ai",
		"https://user@claude.ai/x",
		"https://claude.ai.evil.example/x",
		"https://evil-claude.ai/x",
		"https://www.claude.ai/x",
		"https://claude.ai:8443/x",
		"http://claude.ai/x",
		"https://example.com/claude.ai",
		"not a url",
	} {
		assert.False(t, IsKnownOAuthClient(clientID), clientID)
	}
}

func authorizeReason(t *testing.T, err error) string {
	t.Helper()
	var authorizeErr *OAuthAuthorizeError
	require.ErrorAs(t, err, &authorizeErr)
	return authorizeErr.Reason
}

func TestValidateOAuthClientID(t *testing.T) {
	for _, clientID := range []string{
		"https://claude.ai/oauth/claude-code-client-metadata",
		"https://app.example.com/oauth/client.json",
		"https://app.example.com:8443/client",
		"https://app.example.com/client?version=2",
		"https://app.example.com/" + strings.Repeat("a", 2048-len("https://app.example.com/")),
	} {
		assert.NoError(t, ValidateOAuthClientID(clientID), clientID)
	}

	// Not an https URL at all: the application does not support metadata documents.
	for _, clientID := range []string{"my-client", "", "http://app.example.com/client", "urn:example:client", "https:///client"} {
		assert.Equal(t, OAuthAuthorizeNotCIMD, authorizeReason(t, ValidateOAuthClientID(clientID)), clientID)
	}
	for name, clientID := range map[string]string{
		"a fragment":           "https://app.example.com/client#frag",
		"an empty fragment":    "https://app.example.com/client#",
		"user info":            "https://user:pass@app.example.com/client",
		"a user name":          "https://user@app.example.com/client",
		"a root path":          "https://app.example.com/",
		"no path":              "https://app.example.com",
		"a dot segment":        "https://app.example.com/./client",
		"a dot-dot segment":    "https://app.example.com/a/../client",
		"an encoded dot-dot":   "https://app.example.com/a/%2e%2e/client",
		"a trailing dot-dot":   "https://app.example.com/a/..",
		"over 2048 characters": "https://app.example.com/" + strings.Repeat("a", 2049-len("https://app.example.com/")),
	} {
		assert.Equal(t, OAuthAuthorizeInvalidClientID, authorizeReason(t, ValidateOAuthClientID(clientID)), name)
	}
}

// ChatGPT's and Claude Code's documents, with the fields the spec cites.
const chatGPTMetadata = `{
	"client_id": "https://chatgpt.com/oauth/client-metadata.json",
	"client_name": "ChatGPT",
	"redirect_uris": ["https://chatgpt.com/connector_platform_oauth_redirect"],
	"token_endpoint_auth_methods_supported": ["none", "private_key_jwt"],
	"token_endpoint_auth_method": "private_key_jwt"
}`

const claudeCodeMetadata = `{
	"client_id": "https://claude.ai/oauth/claude-code-client-metadata",
	"client_name": "Claude Code",
	"redirect_uris": ["http://localhost/callback", "http://127.0.0.1/callback"],
	"token_endpoint_auth_method": "none"
}`

func TestParseOAuthClientMetadata_AuthenticationMethods(t *testing.T) {
	metadata, err := ParseOAuthClientMetadata("https://chatgpt.com/oauth/client-metadata.json", []byte(chatGPTMetadata))
	require.NoError(t, err, "ChatGPT: the plural lists none, the singular is ignored")
	assert.Equal(t, "ChatGPT", metadata.ClientName)
	assert.Equal(t, []string{"https://chatgpt.com/connector_platform_oauth_redirect"}, metadata.RedirectURIs)

	metadata, err = ParseOAuthClientMetadata("https://claude.ai/oauth/claude-code-client-metadata", []byte(claudeCodeMetadata))
	require.NoError(t, err, "Claude Code: the singular alone is none")
	assert.Equal(t, []string{"http://localhost/callback", "http://127.0.0.1/callback"}, metadata.RedirectURIs)

	base := `"client_id": "https://app.example.com/client", "client_name": "App", "redirect_uris": ["https://app.example.com/cb"]`
	parse := func(extra string) error {
		body := "{" + base
		if extra != "" {
			body += ", " + extra
		}
		_, err := ParseOAuthClientMetadata("https://app.example.com/client", []byte(body+"}"))
		return err
	}
	assert.NoError(t, parse(""), "neither field: none is assumed")
	assert.NoError(t, parse(`"token_endpoint_auth_methods_supported": ["private_key_jwt", "none"]`))
	assert.Error(t, parse(`"token_endpoint_auth_methods_supported": ["private_key_jwt"]`), "plural without none")
	assert.Error(t, parse(`"token_endpoint_auth_methods_supported": ["private_key_jwt"], "token_endpoint_auth_method": "none"`), "the plural wins over the singular")
	assert.Error(t, parse(`"token_endpoint_auth_methods_supported": []`), "an empty plural lists no none")
	assert.Error(t, parse(`"token_endpoint_auth_method": "private_key_jwt"`), "singular private_key_jwt alone")
	assert.Error(t, parse(`"token_endpoint_auth_method": "client_secret_basic"`))
	assert.Error(t, parse(`"token_endpoint_auth_methods_supported": "none"`), "the plural must be a list")
}

func TestParseOAuthClientMetadata_Limits(t *testing.T) {
	const clientID = "https://app.example.com/client"
	document := func(name string, uris []string) []byte {
		body, err := json.Marshal(map[string]any{"client_id": clientID, "client_name": name, "redirect_uris": uris})
		require.NoError(t, err)
		return body
	}
	uris := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = "https://app.example.com/cb" + strings.Repeat("x", i)
		}
		return out
	}

	_, err := ParseOAuthClientMetadata(clientID, document(strings.Repeat("é", 200), uris(20)))
	assert.NoError(t, err, "200 characters and 20 redirect URIs are allowed")
	_, err = ParseOAuthClientMetadata(clientID, document(strings.Repeat("a", 201), uris(1)))
	assert.ErrorContains(t, err, "client_name is longer than 200 characters")
	_, err = ParseOAuthClientMetadata(clientID, document("App", uris(21)))
	assert.ErrorContains(t, err, "more than 20 redirect_uris")

	longURI := "https://app.example.com/" + strings.Repeat("a", 2048-len("https://app.example.com/"))
	_, err = ParseOAuthClientMetadata(clientID, document("App", []string{longURI}))
	assert.NoError(t, err, "a redirect URI of 2048 characters is allowed")
	_, err = ParseOAuthClientMetadata(clientID, document("App", []string{longURI + "a"}))
	assert.ErrorContains(t, err, "longer than 2048 characters")

	_, err = ParseOAuthClientMetadata(clientID, document("App", []string{"https://app.example.com/cb#frag"}))
	assert.ErrorContains(t, err, "fragment")
	_, err = ParseOAuthClientMetadata(clientID, document("", uris(1)))
	assert.ErrorContains(t, err, "client_name")
	_, err = ParseOAuthClientMetadata(clientID, document("App", nil))
	assert.ErrorContains(t, err, "redirect_uris")

	_, err = ParseOAuthClientMetadata("https://app.example.com/other", document("App", uris(1)))
	assert.ErrorContains(t, err, "does not equal the URL", "the document's client_id must equal the URL")
	_, err = ParseOAuthClientMetadata(clientID, []byte(`<html>`))
	assert.ErrorContains(t, err, "not a JSON object")
}

func TestMatchRedirectURI_Loopback(t *testing.T) {
	registered := []string{"http://localhost/callback", "http://127.0.0.1/callback", "http://[::1]/callback"}
	for _, requested := range []string{
		"http://localhost/callback",
		"http://localhost:54321/callback",
		"http://127.0.0.1:8787/callback",
		"http://127.0.0.1/callback",
		"http://[::1]:3000/callback",
	} {
		assert.True(t, MatchRedirectURI(requested, registered), requested)
	}
	for name, requested := range map[string]string{
		"another host":             "http://example.com:54321/callback",
		"another loopback name":    "http://127.0.0.2:54321/callback",
		"another path":             "http://localhost:54321/other",
		"a longer path":            "http://localhost:54321/callback/x",
		"a query the client lacks": "http://localhost:54321/callback?x=1",
		"a fragment":               "http://localhost:54321/callback#x",
		"https on a loopback host": "https://localhost:54321/callback",
	} {
		assert.False(t, MatchRedirectURI(requested, registered), name)
	}
	// The host is the registered one: localhost does not stand for 127.0.0.1.
	assert.False(t, MatchRedirectURI("http://127.0.0.1:54321/callback", []string{"http://localhost/callback"}))
	// The port rule is for http only: an https URI on a loopback host is exact.
	assert.False(t, MatchRedirectURI("https://localhost:9443/callback", []string{"https://localhost:8443/callback"}))
	assert.True(t, MatchRedirectURI("https://localhost:8443/callback", []string{"https://localhost:8443/callback"}))
	assert.ErrorContains(t, ValidateRedirectURISyntax("http://localhost:54321/callback#"), "fragment", "an empty fragment too")
	// A registered port is no more binding than a missing one.
	assert.True(t, MatchRedirectURI("http://localhost:9000/callback", []string{"http://localhost:8787/callback"}))
}

func TestMatchRedirectURI_HTTPSIsExact(t *testing.T) {
	registered := []string{"https://chatgpt.com/connector_platform_oauth_redirect", "https://claude.ai/api/mcp/auth_callback"}
	assert.True(t, MatchRedirectURI("https://claude.ai/api/mcp/auth_callback", registered))
	for name, requested := range map[string]string{
		"another port":     "https://claude.ai:8443/api/mcp/auth_callback",
		"the default port": "https://claude.ai:443/api/mcp/auth_callback",
		"another case":     "https://Claude.ai/api/mcp/auth_callback",
		"a trailing slash": "https://claude.ai/api/mcp/auth_callback/",
		"an added query":   "https://claude.ai/api/mcp/auth_callback?x=1",
		"a fragment":       "https://claude.ai/api/mcp/auth_callback#x",
		"an unknown URI":   "https://evil.example/cb",
	} {
		assert.False(t, MatchRedirectURI(requested, registered), name)
	}
}

func TestMatchRedirectURI_Schemes(t *testing.T) {
	for _, requested := range []string{
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"http://app.example.com/cb",
		"com.example.app:/callback",
		"ftp://app.example.com/cb",
		"//app.example.com/cb",
		"",
	} {
		// Registered verbatim, still refused.
		assert.False(t, MatchRedirectURI(requested, []string{requested}), requested)
		assert.Error(t, ValidateRedirectURISyntax(requested), requested)
	}
	assert.NoError(t, ValidateRedirectURISyntax("https://app.example.com/cb"))
	assert.NoError(t, ValidateRedirectURISyntax("http://localhost:8080/cb"))
	assert.ErrorContains(t, ValidateRedirectURISyntax("https://app.example.com/"+strings.Repeat("a", 2048)), "longer than 2048")
	// The redirect warning on the page.
	assert.True(t, IsLoopbackRedirectURI("http://127.0.0.1:1234/cb"))
	assert.False(t, IsLoopbackRedirectURI("https://localhost/cb"))
}

// RFC 7636 Appendix B.
func TestPKCE_RFC7636Vector(t *testing.T) {
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	assert.NoError(t, ValidateCodeChallenge(challenge, "S256"))
	assert.True(t, VerifyCodeVerifier(verifier, challenge))
	assert.False(t, VerifyCodeVerifier(verifier[:len(verifier)-1]+"j", challenge))
	assert.False(t, VerifyCodeVerifier(challenge, challenge), "plain: the challenge is not its own verifier")
}

func TestPKCE_MethodAndSyntax(t *testing.T) {
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	assert.ErrorContains(t, ValidateCodeChallenge(challenge, ""), "S256", "a missing method is refused")
	assert.ErrorContains(t, ValidateCodeChallenge(challenge, "plain"), "S256")
	assert.ErrorContains(t, ValidateCodeChallenge(challenge, "s256"), "S256")

	unreserved := "ABCXYZabcxyz0189-._~"
	assert.NoError(t, ValidateCodeChallenge(strings.Repeat("a", 43), "S256"), "43 characters")
	assert.NoError(t, ValidateCodeChallenge(strings.Repeat("a", 128), "S256"), "128 characters")
	assert.NoError(t, ValidateCodeChallenge(unreserved+strings.Repeat("a", 43-len(unreserved)), "S256"))
	for name, bad := range map[string]string{
		"42 characters":  strings.Repeat("a", 42),
		"129 characters": strings.Repeat("a", 129),
		"a plus":         strings.Repeat("a", 42) + "+",
		"a slash":        strings.Repeat("a", 42) + "/",
		"padding":        strings.Repeat("a", 42) + "=",
		"a space":        strings.Repeat("a", 42) + " ",
		"non-ASCII":      strings.Repeat("a", 42) + "é",
		"empty":          "",
	} {
		assert.Error(t, ValidateCodeChallenge(bad, "S256"), name)
	}

	// The verifier has the same syntax: one outside it never verifies, even
	// against its own hash.
	shortVerifier := strings.Repeat("a", 42)
	assert.False(t, VerifyCodeVerifier(shortVerifier, s256(shortVerifier)))
	longVerifier := strings.Repeat("a", 129)
	assert.False(t, VerifyCodeVerifier(longVerifier, s256(longVerifier)))
	badChar := strings.Repeat("a", 42) + "+"
	assert.False(t, VerifyCodeVerifier(badChar, s256(badChar)))
	good := strings.Repeat("a", 43)
	assert.True(t, VerifyCodeVerifier(good, s256(good)))
}

func s256(verifier string) string {
	return PKCEChallengeS256(verifier)
}

func TestHashOAuthSecret(t *testing.T) {
	// SHA-256 of "abc", hex.
	assert.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", HashOAuthSecret("abc"))
}
