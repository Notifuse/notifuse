package safehttpclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// ErrPrivateIP is returned when a request resolves to a private/reserved IP address.
var ErrPrivateIP = errors.New("request to private or reserved IP address is not allowed")

// ErrBlockedScheme is returned when a redirect uses a non-http/https scheme.
var ErrBlockedScheme = errors.New("only http and https schemes are allowed")

var privateNetworks []*net.IPNet

func init() {
	cidrs := []string{
		"127.0.0.0/8",     // loopback
		"10.0.0.0/8",      // private class A
		"172.16.0.0/12",   // private class B
		"192.168.0.0/16",  // private class C
		"169.254.0.0/16",  // link-local (includes cloud metadata 169.254.169.254)
		"0.0.0.0/8",       // "this host on this network" (RFC 1122)
		"100.64.0.0/10",   // carrier-grade NAT (RFC 6598)
		"192.0.0.0/24",    // IETF protocol assignments (RFC 6890)
		"192.0.2.0/24",    // TEST-NET-1 documentation (RFC 5737)
		"198.18.0.0/15",   // benchmarking (RFC 2544)
		"198.51.100.0/24", // TEST-NET-2 documentation (RFC 5737)
		"203.0.113.0/24",  // TEST-NET-3 documentation (RFC 5737)
		"224.0.0.0/4",     // IPv4 multicast
		"240.0.0.0/4",     // reserved + broadcast 255.255.255.255 (RFC 1112)
		"::1/128",         // IPv6 loopback
		"fc00::/7",        // IPv6 unique local
		"fe80::/10",       // IPv6 link-local
		"::/128",          // IPv6 unspecified
		"ff00::/8",        // IPv6 multicast
		"2001:db8::/32",   // IPv6 documentation (RFC 3849)
		"2002::/16",       // 6to4 — can embed a private IPv4 (RFC 3056; deprecated by RFC 7526)
		"64:ff9b::/96",    // NAT64 well-known prefix — can embed a private IPv4 (RFC 6052)
	}
	for _, cidr := range cidrs {
		_, network, _ := net.ParseCIDR(cidr)
		privateNetworks = append(privateNetworks, network)
	}
}

// IsPrivateIP checks whether a given net.IP falls within private/reserved ranges.
func IsPrivateIP(ip net.IP) bool {
	if ip.IsUnspecified() {
		return true
	}
	// Normalize IPv4-mapped IPv6 addresses (e.g., ::ffff:127.0.0.1) to IPv4
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	for _, network := range privateNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// dialer bounds the connection set-up of DialContext.
var dialer = &net.Dialer{
	Timeout: 5 * time.Second,
}

// DialContext resolves the host, skips every private or reserved address, and
// dials the first public one directly, never the hostname, so a DNS answer
// that changes between the check and the dial cannot reach a private address.
// It refuses with ErrPrivateIP when no public address is left. Every client
// built on it, New's and the OAuth metadata fetcher's, gets the same guard.
func DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address %s: %w", addr, err)
	}

	// Resolve DNS
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("DNS resolution failed for %s: %w", host, err)
	}

	// Find a non-private IP and dial it directly (not the hostname)
	for _, ipAddr := range ips {
		if IsPrivateIP(ipAddr.IP) {
			continue
		}
		// Dial the resolved IP directly to prevent DNS rebinding
		resolvedAddr := net.JoinHostPort(ipAddr.IP.String(), port)
		return dialer.DialContext(ctx, network, resolvedAddr)
	}

	return nil, ErrPrivateIP
}

// New creates an *http.Client with SSRF protection.
// It validates all resolved IPs at DialContext time (preventing DNS rebinding),
// blocks private/reserved IP ranges, and applies a 10-second timeout.
// Redirects are safe because each new connection goes through the same DialContext.
func New() *http.Client {
	transport := &http.Transport{
		DialContext:         DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return ErrBlockedScheme
			}
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}
