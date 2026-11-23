package domain

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// MCPPath is the path of the MCP endpoint, under the canonical URL.
const MCPPath = "/mcp"

// CanonicalEndpoint is API_ENDPOINT reduced to what clients compare: a
// lowercase scheme and host, the port only when it is not the scheme's
// default, and no trailing slash. The /mcp handler, the OAuth issuer, the
// resource of every grant and the discovery documents all derive from it, so
// one function decides what "the same server" means.
type CanonicalEndpoint struct {
	// URL is the canonical URL, path included.
	URL string
	// Origin is scheme://host[:port].
	Origin string
	// Host is host[:port], as a Host header carries it.
	Host string
	// HasPort reports whether Host carries a port.
	HasPort bool
	// Scheme is the lowercase scheme.
	Scheme string
	// Hostname is the lowercase host without port or brackets.
	Hostname string
	// Path is the escaped path without its trailing slash; empty at the root.
	Path string
}

// CanonicalAPIEndpoint derives the canonical endpoint from API_ENDPOINT.
func CanonicalAPIEndpoint(apiEndpoint string) (CanonicalEndpoint, error) {
	parsed, err := url.Parse(strings.TrimSpace(apiEndpoint))
	if err != nil || parsed.Host == "" {
		return CanonicalEndpoint{}, fmt.Errorf("API_ENDPOINT is not an http(s) URL")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return CanonicalEndpoint{}, fmt.Errorf("API_ENDPOINT is not an http(s) URL")
	}
	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	host := hostname
	if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	}
	origin := scheme + "://" + host
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	return CanonicalEndpoint{
		URL:      origin + path,
		Origin:   origin,
		Host:     host,
		HasPort:  port != "",
		Scheme:   scheme,
		Hostname: hostname,
		Path:     path,
	}, nil
}

// MatchesHost compares a request's Host header with the canonical host. The
// port is compared when the canonical URL has one.
func (c CanonicalEndpoint) MatchesHost(requestHost string) bool {
	requestHost = strings.ToLower(requestHost)
	if c.HasPort {
		return requestHost == c.Host
	}
	if hostname, _, err := net.SplitHostPort(requestHost); err == nil {
		if strings.Contains(hostname, ":") {
			hostname = "[" + hostname + "]"
		}
		requestHost = hostname
	}
	return requestHost == c.Host
}

// MCPURL is the canonical URL of the MCP endpoint, which is also the
// resource every OAuth grant is bound to.
func (c CanonicalEndpoint) MCPURL() string {
	return c.URL + MCPPath
}

// OAuthUnavailableReason says why the authorization server cannot run on this
// endpoint, or "" when it can. It needs https, except on a loopback host, and
// no path: clients look for a path issuer's metadata at the host root, which a
// Notifuse served under a path prefix does not own.
func (c CanonicalEndpoint) OAuthUnavailableReason() string {
	if c.Scheme != "https" && !IsLoopbackHostname(c.Hostname) {
		return "API_ENDPOINT is not https"
	}
	if c.Path != "" {
		return "API_ENDPOINT has a path"
	}
	return ""
}

// IsLoopbackHostname reports whether a hostname, lowercase and without
// brackets, is one of the loopback names of RFC 8252 §7.3.
func IsLoopbackHostname(hostname string) bool {
	switch hostname {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// CanonicalResource reduces a resource indicator (RFC 8707) to the canonical
// form, so that it compares with CanonicalEndpoint.MCPURL as a string. A
// resource carrying a query, a fragment or user info is refused.
func CanonicalResource(resource string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(resource))
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.Contains(resource, "#") {
		return "", fmt.Errorf("resource is not an absolute URL without query or fragment")
	}
	endpoint, err := CanonicalAPIEndpoint(resource)
	if err != nil {
		return "", fmt.Errorf("resource is not an http(s) URL")
	}
	return endpoint.URL, nil
}
