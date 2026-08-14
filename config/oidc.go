package config

import (
	"fmt"
	"slices"
	"strings"
)

// OIDCConfig holds resolved OpenID Connect settings. Values are resolved with the
// same env-wins-over-DB precedence used for SMTP (see resolveOIDCConfig). The
// ClientSecret is held in memory decrypted but is encrypted at rest in the DB and
// must never be exposed to any client-facing path (serveConfigJS / settings GET).
//
// There is deliberately no "enforce SSO" / "disable magic code" switch here. SSO is
// additive: magic-code login always remains available, which is what keeps an
// unreachable IdP or a misconfigured issuer from locking every operator out of the
// instance — including out of the settings page where they would fix it. Anyone
// adding enforcement has to solve that first: an escape hatch that does not itself
// depend on the IdP, such as ROOT_EMAIL keeping password access unconditionally.
type OIDCConfig struct {
	Enabled         bool
	IssuerURL       string
	ClientID        string
	ClientSecret    string   // decrypted in-memory; encrypted at rest in DB
	RedirectURI     string   // derived from APIEndpoint when empty
	Scopes          []string // always contains "openid"
	ButtonLabel     string
	AutoCreateUsers bool
	AllowedDomains  []string // lower-cased; gates JIT provisioning

	// AllowUnverifiedEmail tolerates an id_token whose email_verified claim is
	// ABSENT (some IdPs — Microsoft Entra ID, Cloudflare Access — never emit it).
	// An explicit email_verified=false is always rejected regardless of this flag.
	// Env-only (OIDC_ALLOW_UNVERIFIED_EMAIL): no DB setting exists.
	AllowUnverifiedEmail bool
}

// oidcCallbackPath is the fixed callback route registered at the IdP. It must match
// the route registered by the OIDC HTTP handler.
const oidcCallbackPath = "/api/user.oidc.callback"

// DefaultOIDCScopes is used when neither env nor DB supplies scopes. Write paths
// (setup wizard, settings update) must persist it when the submitted value is
// empty: ParseScopes("") yields bare "openid", and a stored "openid" would
// permanently override this richer default at resolve time.
const DefaultOIDCScopes = "openid email profile"

// defaultOIDCButtonLabel is the fallback sign-in button text.
const defaultOIDCButtonLabel = "Sign in with SSO"

// Validate checks static OIDC fields and fails fast at boot when OIDC is enabled
// but misconfigured. It never dials the network — issuer reachability is a runtime
// concern handled by the service's lazy guarded-retry init.
func (c OIDCConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.IssuerURL == "" {
		return fmt.Errorf("OIDC is enabled but OIDC_ISSUER_URL is empty")
	}
	if !strings.HasPrefix(c.IssuerURL, "https://") {
		return fmt.Errorf("OIDC_ISSUER_URL must use https (got %q)", c.IssuerURL)
	}
	if c.ClientID == "" {
		return fmt.Errorf("OIDC is enabled but OIDC_CLIENT_ID is empty")
	}
	if c.ClientSecret == "" {
		return fmt.Errorf("OIDC is enabled but OIDC_CLIENT_SECRET is empty")
	}
	if c.RedirectURI == "" {
		return fmt.Errorf("OIDC redirect URI could not be derived (set OIDC_REDIRECT_URI or API_ENDPOINT)")
	}
	// An empty allowlist means "no domain is allowed", never "all domains" — the
	// resolver applies it as a positive match. Pairing it with auto-create would
	// therefore be inert rather than dangerous, but it reads as "let anyone in", so
	// refuse to boot instead of leaving an operator guessing which way it went.
	if c.AutoCreateUsers && len(c.AllowedDomains) == 0 {
		return fmt.Errorf("OIDC_AUTO_CREATE_USERS=true requires a non-empty OIDC_ALLOWED_DOMAINS allowlist")
	}
	return nil
}

// NormalizeScopesForStorage returns the canonical space-joined scope string that
// write paths (setup wizard, settings update) must persist. When raw contains no
// scope tokens at all (empty, whitespace, or separators only) the FULL default is
// substituted: persisting ParseScopes("") → bare "openid" would permanently
// override the richer default at resolve time and strip the email/profile scopes
// from authorize requests. A deliberate explicit "openid" is respected.
func NormalizeScopesForStorage(raw string) string {
	if len(ParseRootEmails(raw)) == 0 {
		raw = DefaultOIDCScopes
	}
	return strings.Join(ParseScopes(raw), " ")
}

// ParseScopes splits a space/comma/semicolon-separated scope string (reusing the
// ROOT_EMAIL splitter for consistency), de-dupes, preserves order, and guarantees
// "openid" is present and first.
func ParseScopes(raw string) []string {
	parsed := ParseRootEmails(raw) // same separator/dedup semantics
	out := make([]string, 0, len(parsed)+1)
	out = append(out, "openid")
	for _, s := range parsed {
		if s == "openid" {
			continue
		}
		out = append(out, s)
	}
	return out
}

// normalizeDomains lower-cases each domain entry.
func normalizeDomains(domains []string) []string {
	if len(domains) == 0 {
		return nil
	}
	out := make([]string, 0, len(domains))
	for _, d := range domains {
		out = append(out, strings.ToLower(d))
	}
	return out
}

// resolveOIDCConfig resolves the effective OIDC configuration with env-wins-else-DB
// precedence, mirroring the SMTP-bridge resolution semantics. DB settings are only
// consulted when the instance is installed. The redirect URI is derived from the
// final (post-overlay, post-trim) apiEndpoint when neither env nor DB supplies one.
func resolveOIDCConfig(env EnvValues, ss *SystemSettings, isInstalled bool, apiEndpoint string) OIDCConfig {
	hasDB := isInstalled && ss != nil

	c := OIDCConfig{
		IssuerURL:    env.OIDCIssuerURL,
		ClientID:     env.OIDCClientID,
		ClientSecret: env.OIDCClientSecret,
		RedirectURI:  env.OIDCRedirectURI,
		ButtonLabel:  env.OIDCButtonLabel,
	}

	// Enabled: explicit env wins; "" → DB (only when installed).
	switch env.OIDCEnabled {
	case "true":
		c.Enabled = true
	case "false":
		c.Enabled = false
	default:
		if hasDB {
			c.Enabled = ss.OIDCEnabled
		}
	}

	// AutoCreateUsers: same tri-state.
	switch env.OIDCAutoCreateUsers {
	case "true":
		c.AutoCreateUsers = true
	case "false":
		c.AutoCreateUsers = false
	default:
		if hasDB {
			c.AutoCreateUsers = ss.OIDCAutoCreateUsers
		}
	}

	// AllowUnverifiedEmail is env-only (no DB setting): parsed at the config edge
	// with viper GetBool semantics (true/1/T/TRUE...), unset simply means false.
	c.AllowUnverifiedEmail = env.OIDCAllowUnverifiedEmail

	// String fields: env value, else DB.
	if hasDB {
		if c.IssuerURL == "" {
			c.IssuerURL = ss.OIDCIssuerURL
		}
		if c.ClientID == "" {
			c.ClientID = ss.OIDCClientID
		}
		if c.ClientSecret == "" {
			c.ClientSecret = ss.OIDCClientSecret
		}
		if c.RedirectURI == "" {
			c.RedirectURI = ss.OIDCRedirectURI
		}
		if c.ButtonLabel == "" {
			c.ButtonLabel = ss.OIDCButtonLabel
		}
	}

	// Scopes: env raw, else DB raw, else default; always force-include openid.
	rawScopes := env.OIDCScopes
	if rawScopes == "" && hasDB {
		rawScopes = ss.OIDCScopes
	}
	if rawScopes == "" {
		rawScopes = DefaultOIDCScopes
	}
	c.Scopes = ParseScopes(rawScopes)

	// Allowed domains: env raw, else DB raw; lower-cased.
	rawDomains := env.OIDCAllowedDomains
	if rawDomains == "" && hasDB {
		rawDomains = ss.OIDCAllowedDomains
	}
	c.AllowedDomains = normalizeDomains(ParseRootEmails(rawDomains))

	if c.ButtonLabel == "" {
		c.ButtonLabel = defaultOIDCButtonLabel
	}

	// Derive the redirect URI from the final apiEndpoint when empty.
	if c.RedirectURI == "" && apiEndpoint != "" {
		c.RedirectURI = strings.TrimRight(apiEndpoint, "/") + oidcCallbackPath
	}

	return c
}

// IsAllowedOIDCDomain reports whether the email's domain is in the configured OIDC
// allowlist (case-insensitive). An empty email or one without a domain never matches.
func (c *Config) IsAllowedOIDCDomain(email string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return false
	}
	domain := strings.ToLower(email[at+1:])
	return slices.Contains(c.OIDC.AllowedDomains, domain)
}
