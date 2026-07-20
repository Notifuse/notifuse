package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setOIDCBaseEnv sets the minimal non-OIDC env needed for LoadWithOptions to reach
// the OIDC resolution path (env-only, since the test DB is unreachable). Returns a
// cleanup func that unsets everything it set plus any OIDC_* keys.
func setOIDCBaseEnv(t *testing.T, oidc map[string]string) func() {
	t.Helper()
	base := map[string]string{
		"SECRET_KEY":   "test-secret-key-1234567890123456",
		"DB_HOST":      "unreachable-db-host.invalid",
		"API_ENDPOINT": "https://app.example.com",
		"ENVIRONMENT":  "development",
	}
	for k, v := range base {
		_ = os.Setenv(k, v)
	}
	for k, v := range oidc {
		_ = os.Setenv(k, v)
	}
	return func() {
		for k := range base {
			_ = os.Unsetenv(k)
		}
		for k := range oidc {
			_ = os.Unsetenv(k)
		}
	}
}

func TestLoadWithOptions_OIDCFromEnv(t *testing.T) {
	cleanup := setOIDCBaseEnv(t, map[string]string{
		"OIDC_ENABLED":       "true",
		"OIDC_ISSUER_URL":    "https://idp.example.com",
		"OIDC_CLIENT_ID":     "client-abc",
		"OIDC_CLIENT_SECRET": "shh",
		"OIDC_SCOPES":        "email profile",
	})
	defer cleanup()

	cfg, err := LoadWithOptions(LoadOptions{})
	require.NoError(t, err)

	assert.True(t, cfg.OIDC.Enabled)
	assert.Equal(t, "https://idp.example.com", cfg.OIDC.IssuerURL)
	assert.Equal(t, "client-abc", cfg.OIDC.ClientID)
	assert.Equal(t, "shh", cfg.OIDC.ClientSecret)
	assert.Equal(t, []string{"openid", "email", "profile"}, cfg.OIDC.Scopes)
	// redirect derived from API_ENDPOINT
	assert.Equal(t, "https://app.example.com/api/user.oidc.callback", cfg.OIDC.RedirectURI)
	assert.Equal(t, "Sign in with SSO", cfg.OIDC.ButtonLabel, "default button label applied by resolver")
}

func TestLoadWithOptions_OIDCDisabledByDefault(t *testing.T) {
	cleanup := setOIDCBaseEnv(t, map[string]string{})
	defer cleanup()

	cfg, err := LoadWithOptions(LoadOptions{})
	require.NoError(t, err)
	assert.False(t, cfg.OIDC.Enabled, "OIDC is disabled when no env/DB enables it")
}

func TestLoadWithOptions_OIDCEnabledMissingIssuerFailsBoot(t *testing.T) {
	cleanup := setOIDCBaseEnv(t, map[string]string{
		"OIDC_ENABLED":   "true",
		"OIDC_CLIENT_ID": "client-abc",
		// no issuer, no secret → Validate must fail
	})
	defer cleanup()

	_, err := LoadWithOptions(LoadOptions{})
	require.Error(t, err, "enabling OIDC without an issuer must fail boot")
}

func TestLoadWithOptions_OIDCAutoCreateRequiresAllowlist(t *testing.T) {
	cleanup := setOIDCBaseEnv(t, map[string]string{
		"OIDC_ENABLED":           "true",
		"OIDC_ISSUER_URL":        "https://idp.example.com",
		"OIDC_CLIENT_ID":         "client-abc",
		"OIDC_CLIENT_SECRET":     "shh",
		"OIDC_AUTO_CREATE_USERS": "true",
		// no OIDC_ALLOWED_DOMAINS → must fail boot
	})
	defer cleanup()

	_, err := LoadWithOptions(LoadOptions{})
	require.Error(t, err, "JIT auto-create without an allowlist must fail boot")
}

func TestOIDCConfig_Validate(t *testing.T) {
	base := OIDCConfig{
		Enabled:      true,
		IssuerURL:    "https://idp.example.com",
		ClientID:     "client-123",
		ClientSecret: "secret-xyz",
		RedirectURI:  "https://app.example.com/api/user.oidc.callback",
		Scopes:       []string{"openid", "email"},
	}

	tests := []struct {
		name    string
		mutate  func(c *OIDCConfig)
		wantErr bool
	}{
		{"disabled is always valid", func(c *OIDCConfig) { *c = OIDCConfig{Enabled: false} }, false},
		{"valid full config", func(c *OIDCConfig) {}, false},
		{"enabled missing issuer", func(c *OIDCConfig) { c.IssuerURL = "" }, true},
		{"issuer must be https", func(c *OIDCConfig) { c.IssuerURL = "http://idp.example.com" }, true},
		{"enabled missing client id", func(c *OIDCConfig) { c.ClientID = "" }, true},
		{"enabled missing client secret", func(c *OIDCConfig) { c.ClientSecret = "" }, true},
		{"empty redirect uri", func(c *OIDCConfig) { c.RedirectURI = "" }, true},
		{"auto-create without allowlist fails", func(c *OIDCConfig) { c.AutoCreateUsers = true; c.AllowedDomains = nil }, true},
		{"auto-create with allowlist ok", func(c *OIDCConfig) { c.AutoCreateUsers = true; c.AllowedDomains = []string{"corp.com"} }, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mutate(&c)
			err := c.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseScopes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty yields just openid", "", []string{"openid"}},
		{"prepends openid when missing", "email profile", []string{"openid", "email", "profile"}},
		{"does not duplicate openid", "openid email", []string{"openid", "email"}},
		{"openid not first is de-duped, openid stays once", "email openid profile", []string{"openid", "email", "profile"}},
		{"comma and semicolon separators", "email,profile;groups", []string{"openid", "email", "profile", "groups"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ParseScopes(tc.in))
		})
	}
}

func TestResolveOIDCConfig_EnvWins(t *testing.T) {
	env := EnvValues{
		OIDCEnabled:      "true",
		OIDCIssuerURL:    "https://env-idp.example.com",
		OIDCClientID:     "env-client",
		OIDCClientSecret: "env-secret",
		OIDCScopes:       "openid email",
		OIDCButtonLabel:  "Env SSO",
	}
	ss := &SystemSettings{
		OIDCEnabled:     true,
		OIDCIssuerURL:   "https://db-idp.example.com",
		OIDCClientID:    "db-client",
		OIDCButtonLabel: "DB SSO",
	}
	c := resolveOIDCConfig(env, ss, true, "https://app.example.com")

	assert.True(t, c.Enabled)
	assert.Equal(t, "https://env-idp.example.com", c.IssuerURL, "env issuer must win over DB")
	assert.Equal(t, "env-client", c.ClientID)
	assert.Equal(t, "env-secret", c.ClientSecret)
	assert.Equal(t, "Env SSO", c.ButtonLabel)
	assert.Equal(t, []string{"openid", "email"}, c.Scopes)
	// redirect derived from apiEndpoint since neither env nor DB set it
	assert.Equal(t, "https://app.example.com/api/user.oidc.callback", c.RedirectURI)
}

func TestResolveOIDCConfig_DBFallbackWhenInstalled(t *testing.T) {
	env := EnvValues{} // nothing from env
	ss := &SystemSettings{
		OIDCEnabled:         true,
		OIDCIssuerURL:       "https://db-idp.example.com",
		OIDCClientID:        "db-client",
		OIDCClientSecret:    "db-secret",
		OIDCScopes:          "openid email profile",
		OIDCButtonLabel:     "DB SSO",
		OIDCAutoCreateUsers: true,
		OIDCAllowedDomains:  "Corp.com, Sub.Corp.com",
	}
	c := resolveOIDCConfig(env, ss, true, "https://app.example.com/")

	assert.True(t, c.Enabled, "DB enables when env unset")
	assert.Equal(t, "https://db-idp.example.com", c.IssuerURL)
	assert.Equal(t, "db-secret", c.ClientSecret)
	assert.True(t, c.AutoCreateUsers)
	assert.Equal(t, []string{"corp.com", "sub.corp.com"}, c.AllowedDomains, "domains lower-cased")
}

func TestResolveOIDCConfig_ExplicitEnvFalseLocksOutDB(t *testing.T) {
	env := EnvValues{OIDCEnabled: "false"}
	ss := &SystemSettings{OIDCEnabled: true}
	c := resolveOIDCConfig(env, ss, true, "https://app.example.com")
	assert.False(t, c.Enabled, "explicit env false must override DB true")
}

func TestResolveOIDCConfig_NotInstalledIgnoresDB(t *testing.T) {
	env := EnvValues{}
	ss := &SystemSettings{OIDCEnabled: true, OIDCIssuerURL: "https://db.example.com"}
	c := resolveOIDCConfig(env, ss, false, "https://app.example.com")
	assert.False(t, c.Enabled, "uninstalled boot must not read DB settings")
	assert.Empty(t, c.IssuerURL)
}

func TestResolveOIDCConfig_RedirectDefaultScopesAndLabel(t *testing.T) {
	env := EnvValues{OIDCEnabled: "true"}
	c := resolveOIDCConfig(env, nil, false, "https://app.example.com")
	assert.Equal(t, "https://app.example.com/api/user.oidc.callback", c.RedirectURI)
	assert.Equal(t, []string{"openid", "email", "profile"}, c.Scopes, "default scopes when none provided")
	assert.Equal(t, "Sign in with SSO", c.ButtonLabel, "default button label")
}

func TestResolveOIDCConfig_AllowUnverifiedEmail(t *testing.T) {
	ss := &SystemSettings{
		OIDCEnabled:      true,
		OIDCIssuerURL:    "https://db-idp.example.com",
		OIDCClientID:     "db-client",
		OIDCClientSecret: "db-secret",
	}

	c := resolveOIDCConfig(EnvValues{OIDCAllowUnverifiedEmail: true}, ss, true, "https://app.example.com")
	assert.True(t, c.AllowUnverifiedEmail)

	c = resolveOIDCConfig(EnvValues{}, ss, true, "https://app.example.com")
	assert.False(t, c.AllowUnverifiedEmail,
		"env-only flag: unset must stay false even when installed with DB-backed OIDC config")
}

func TestLoadWithOptions_OIDCAllowUnverifiedEmailSpellings(t *testing.T) {
	// The flag is parsed with GetBool semantics: operators writing TRUE or 1
	// (e.g. docker-compose YAML integer mapping) must not silently get false.
	for _, spelling := range []string{"true", "TRUE", "True", "1"} {
		cleanup := setOIDCBaseEnv(t, map[string]string{
			"OIDC_ALLOW_UNVERIFIED_EMAIL": spelling,
		})
		cfg, err := LoadWithOptions(LoadOptions{})
		cleanup()
		require.NoError(t, err)
		assert.True(t, cfg.OIDC.AllowUnverifiedEmail, "spelling %q must enable the flag", spelling)
	}

	cleanup := setOIDCBaseEnv(t, map[string]string{})
	cfg, err := LoadWithOptions(LoadOptions{})
	cleanup()
	require.NoError(t, err)
	assert.False(t, cfg.OIDC.AllowUnverifiedEmail, "unset must stay false")
}

func TestNormalizeScopesForStorage(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty gets the full default", "", "openid email profile"},
		{"whitespace only gets the full default", "   ", "openid email profile"},
		{"separators only get the full default", " , ; ", "openid email profile"},
		{"custom scopes force openid first", "email profile", "openid email profile"},
		{"explicit bare openid is respected", "openid", "openid"},
		{"explicit subset is respected", "openid email", "openid email"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, NormalizeScopesForStorage(tc.in))
		})
	}
}

func TestConfig_IsAllowedOIDCDomain(t *testing.T) {
	c := &Config{OIDC: OIDCConfig{AllowedDomains: []string{"corp.com", "sub.corp.com"}}}

	assert.True(t, c.IsAllowedOIDCDomain("alice@corp.com"))
	assert.True(t, c.IsAllowedOIDCDomain("Bob@Corp.com"), "case-insensitive domain match")
	assert.True(t, c.IsAllowedOIDCDomain("carol@sub.corp.com"))
	assert.False(t, c.IsAllowedOIDCDomain("dave@other.com"))
	assert.False(t, c.IsAllowedOIDCDomain("no-at-sign"))
	assert.False(t, c.IsAllowedOIDCDomain(""))
}
