package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateFeedURL_ValidURLs(t *testing.T) {
	validURLs := []string{
		"https://api.example.com/feed",
		"https://api.example.com:8443/feed",
		"http://api.example.com/feed",
		"https://feed.company.io/data",
		"https://subdomain.example.org/api/v1/feed",
		"https://api.example.com",
		"https://api.example.com/",
		"https://external-api.company.com/broadcasts/data",
	}
	for _, url := range validURLs {
		t.Run(url, func(t *testing.T) {
			err := ValidateFeedURL(url)
			assert.NoError(t, err)
		})
	}
}

func TestValidateFeedURL_InvalidScheme(t *testing.T) {
	testCases := []struct {
		name string
		url  string
	}{
		{"ftp scheme", "ftp://api.example.com/feed"},
		{"file scheme", "file:///etc/passwd"},
		{"ssh scheme", "ssh://user@host.com/path"},
		{"javascript scheme", "javascript:alert(1)"},
		{"data scheme", "data:text/html,<h1>test</h1>"},
		{"mailto scheme", "mailto:test@example.com"},
		{"gopher scheme", "gopher://host.com/path"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFeedURL(tc.url)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "URL must use http or https scheme")
		})
	}
}

func TestValidateFeedURL_PrivateIPs(t *testing.T) {
	testCases := []struct {
		name string
		url  string
	}{
		// Loopback addresses (127.0.0.0/8)
		{"loopback 127.0.0.1", "https://127.0.0.1/feed"},
		{"loopback 127.0.0.255", "https://127.0.0.255/feed"},
		{"loopback 127.1.2.3", "https://127.1.2.3/feed"},

		// IPv6 loopback
		{"ipv6 loopback ::1", "https://[::1]/feed"},
		{"ipv6 loopback full", "https://[0:0:0:0:0:0:0:1]/feed"},

		// Class A private (10.0.0.0/8)
		{"class A 10.0.0.1", "https://10.0.0.1/feed"},
		{"class A 10.255.255.255", "https://10.255.255.255/feed"},
		{"class A 10.128.0.1", "https://10.128.0.1/feed"},

		// Class B private (172.16.0.0/12)
		{"class B 172.16.0.1", "https://172.16.0.1/feed"},
		{"class B 172.31.255.255", "https://172.31.255.255/feed"},
		{"class B 172.20.0.1", "https://172.20.0.1/feed"},

		// Class C private (192.168.0.0/16)
		{"class C 192.168.0.1", "https://192.168.0.1/feed"},
		{"class C 192.168.255.255", "https://192.168.255.255/feed"},
		{"class C 192.168.1.1", "https://192.168.1.1/feed"},

		// Link-local (169.254.0.0/16) - includes cloud metadata
		{"link-local 169.254.169.254", "https://169.254.169.254/feed"},
		{"link-local 169.254.0.1", "https://169.254.0.1/feed"},

		// Unspecified address
		{"unspecified 0.0.0.0", "https://0.0.0.0/feed"},

		// IPv6 link-local (fe80::/10)
		{"ipv6 link-local fe80::1", "https://[fe80::1]/feed"},

		// IPv6 unique local (fc00::/7)
		{"ipv6 unique local fc00::1", "https://[fc00::1]/feed"},
		{"ipv6 unique local fd00::1", "https://[fd00::1]/feed"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFeedURL(tc.url)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "private or restricted IP")
		})
	}
}

func TestValidateFeedURL_Localhost(t *testing.T) {
	testCases := []struct {
		name string
		url  string
	}{
		{"localhost", "https://localhost/feed"},
		{"LOCALHOST", "https://LOCALHOST/feed"},
		{"localhost with port", "https://localhost:8080/feed"},
		{"server.local", "https://server.local/feed"},
		{"internal.local", "https://internal.local/feed"},
		{"localhost.localdomain", "https://localhost.localdomain/feed"},
		{"subdomain.localhost", "https://subdomain.localhost/feed"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFeedURL(tc.url)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "localhost or local domain")
		})
	}
}

func TestValidateFeedURL_MalformedURLs(t *testing.T) {
	testCases := []struct {
		name   string
		url    string
		errMsg string
	}{
		{"empty string", "", "URL is required"},
		{"not a url", "not-a-url", "URL must use http or https scheme"},
		{"missing scheme", "://missing-scheme.com", "invalid URL"},
		{"just scheme", "https://", "URL must have a host"},
		{"spaces in url", "https://exam ple.com/feed", "invalid URL"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFeedURL(tc.url)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.errMsg)
		})
	}
}

func TestValidateFeedURL_QueryParameters(t *testing.T) {
	testCases := []struct {
		name string
		url  string
	}{
		{"simple query", "https://api.example.com/feed?key=value"},
		{"multiple params", "https://api.example.com/feed?key=value&other=123"},
		{"query without value", "https://api.example.com/feed?key"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFeedURL(tc.url)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "URL must not contain query parameters")
		})
	}
}

func TestValidateFeedURL_Fragment(t *testing.T) {
	testCases := []struct {
		name string
		url  string
	}{
		{"simple fragment", "https://api.example.com/feed#section"},
		{"fragment with value", "https://api.example.com/feed#section=value"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFeedURL(tc.url)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "URL must not contain fragment")
		})
	}
}

func TestValidateFeedURL_SpecialCases(t *testing.T) {
	testCases := []struct {
		name   string
		url    string
		errMsg string
	}{
		// AWS/GCP/Azure metadata endpoints
		{"AWS metadata IP", "https://169.254.169.254/latest/meta-data/", "private or restricted IP"},
		{"GCP metadata domain", "https://metadata.google.internal/computeMetadata/", "internal domain"},
		{"Azure metadata domain", "https://metadata.azure.internal/metadata/", "internal domain"},

		// Docker/Kubernetes internal
		{"Docker bridge 172.17.0.1", "https://172.17.0.1/feed", "private or restricted IP"},
		{"Kubernetes 10.96.0.1", "https://10.96.0.1/feed", "private or restricted IP"},
		{"Kubernetes internal domain", "https://service.default.svc.cluster.local/feed", "localhost or local domain"},

		// Internal domains
		{"internal suffix", "https://api.internal/feed", "internal domain"},
		{"corp suffix", "https://api.corp/feed", "internal domain"},
		{"intranet suffix", "https://api.intranet/feed", "internal domain"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFeedURL(tc.url)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.errMsg)
		})
	}
}

func TestValidateFeedURL_ValidPublicIPs(t *testing.T) {
	// These should be allowed - public IPs
	validURLs := []string{
		"https://8.8.8.8/feed",                // Google DNS
		"https://1.1.1.1/feed",                // Cloudflare DNS
		"https://208.67.222.222/feed",         // OpenDNS
		"https://[2001:4860:4860::8888]/feed", // Google IPv6 DNS
		"https://172.15.255.255/feed",         // Just before private range
		"https://172.32.0.1/feed",             // Just after private range
		"https://192.167.255.255/feed",        // Just before 192.168.x.x
		"https://192.169.0.1/feed",            // Just after 192.168.x.x
		"https://169.253.255.255/feed",        // Just before link-local
		"https://169.255.0.1/feed",            // Just after link-local
	}
	for _, url := range validURLs {
		t.Run(url, func(t *testing.T) {
			err := ValidateFeedURL(url)
			assert.NoError(t, err)
		})
	}
}

func TestValidateIPAddress_EdgeCases(t *testing.T) {
	testCases := []struct {
		name      string
		url       string
		shouldErr bool
	}{
		// IPv4-mapped IPv6 addresses for private IPs
		{"ipv4-mapped localhost", "https://[::ffff:127.0.0.1]/feed", true},
		{"ipv4-mapped private", "https://[::ffff:192.168.1.1]/feed", true},
		{"ipv4-mapped public", "https://[::ffff:8.8.8.8]/feed", false},

		// Broadcast/multicast
		{"broadcast 255.255.255.255", "https://255.255.255.255/feed", true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFeedURL(tc.url)
			if tc.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
