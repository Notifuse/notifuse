package safehttpclient

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPrivateIP(t *testing.T) {
	testCases := []struct {
		name     string
		ip       string
		expected bool
	}{
		// Loopback (127.0.0.0/8)
		{"loopback 127.0.0.1", "127.0.0.1", true},
		{"loopback 127.255.255.255", "127.255.255.255", true},

		// Private class A (10.0.0.0/8)
		{"private 10.0.0.1", "10.0.0.1", true},
		{"private 10.255.255.255", "10.255.255.255", true},

		// Private class B (172.16.0.0/12)
		{"private 172.16.0.1", "172.16.0.1", true},
		{"private 172.31.255.255", "172.31.255.255", true},
		{"not private 172.15.255.255", "172.15.255.255", false},
		{"not private 172.32.0.1", "172.32.0.1", false},

		// Private class C (192.168.0.0/16)
		{"private 192.168.1.1", "192.168.1.1", true},
		{"private 192.168.255.255", "192.168.255.255", true},

		// Link-local (169.254.0.0/16) — cloud metadata
		{"link-local 169.254.0.1", "169.254.0.1", true},
		{"link-local 169.254.169.254", "169.254.169.254", true},

		// Carrier-grade NAT (100.64.0.0/10)
		{"cgnat 100.64.0.1", "100.64.0.1", true},
		{"cgnat 100.127.255.255", "100.127.255.255", true},
		{"not cgnat 100.128.0.1", "100.128.0.1", false},

		// Unspecified (0.0.0.0/8)
		{"unspecified 0.0.0.0", "0.0.0.0", true},

		// Public IPs
		{"public 8.8.8.8", "8.8.8.8", false},
		{"public 1.1.1.1", "1.1.1.1", false},
		{"public 93.184.216.34", "93.184.216.34", false},

		// IPv6 loopback
		{"ipv6 loopback", "::1", true},

		// IPv6 unique local (fc00::/7)
		{"ipv6 unique local", "fc00::1", true},
		{"ipv6 unique local fd", "fd00::1", true},

		// IPv6 link-local (fe80::/10)
		{"ipv6 link-local", "fe80::1", true},

		// IPv6 unspecified
		{"ipv6 unspecified", "::", true},

		// IPv6 multicast (ff00::/8)
		{"ipv6 multicast", "ff02::1", true},

		// IPv6 public
		{"ipv6 public", "2606:4700::", false},

		// IPv4-mapped IPv6 addresses
		{"ipv4-mapped loopback", "::ffff:127.0.0.1", true},
		{"ipv4-mapped private", "::ffff:10.0.0.1", true},
		{"ipv4-mapped link-local", "::ffff:169.254.169.254", true},
		{"ipv4-mapped public", "::ffff:8.8.8.8", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			require.NotNil(t, ip, "failed to parse IP: %s", tc.ip)
			assert.Equal(t, tc.expected, IsPrivateIP(ip))
		})
	}
}

func TestNew_ReturnsConfiguredClient(t *testing.T) {
	client := New()

	assert.Equal(t, 10*time.Second, client.Timeout)
	assert.NotNil(t, client.Transport)
	assert.NotNil(t, client.CheckRedirect)
}

func TestNew_BlocksLocalRequests(t *testing.T) {
	// httptest.NewServer binds to 127.0.0.1 — the safe client must reject it
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New()
	_, err := client.Get(server.URL)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private or reserved IP")
}

func TestNew_CheckRedirectBlocksSchemes(t *testing.T) {
	client := New()

	// Test the CheckRedirect function directly
	fileReq, _ := http.NewRequest("GET", "file:///etc/passwd", nil)
	err := client.CheckRedirect(fileReq, []*http.Request{{}})
	assert.ErrorIs(t, err, ErrBlockedScheme)

	ftpReq, _ := http.NewRequest("GET", "ftp://evil.com/file", nil)
	err = client.CheckRedirect(ftpReq, []*http.Request{{}})
	assert.ErrorIs(t, err, ErrBlockedScheme)

	// http and https should be allowed
	httpReq, _ := http.NewRequest("GET", "http://example.com", nil)
	err = client.CheckRedirect(httpReq, []*http.Request{{}})
	assert.NoError(t, err)

	httpsReq, _ := http.NewRequest("GET", "https://example.com", nil)
	err = client.CheckRedirect(httpsReq, []*http.Request{{}})
	assert.NoError(t, err)
}

func TestNew_CheckRedirectLimitsHops(t *testing.T) {
	client := New()

	// Simulate 10 prior redirects
	via := make([]*http.Request, 10)
	for i := range via {
		via[i] = &http.Request{}
	}

	req, _ := http.NewRequest("GET", "https://example.com", nil)
	err := client.CheckRedirect(req, via)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too many redirects")
}
