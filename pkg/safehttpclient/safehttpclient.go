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
		"127.0.0.0/8",    // loopback
		"10.0.0.0/8",     // private class A
		"172.16.0.0/12",  // private class B
		"192.168.0.0/16", // private class C
		"169.254.0.0/16", // link-local
		"0.0.0.0/8",      // unspecified
		"100.64.0.0/10",  // carrier-grade NAT (RFC 6598)
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 unique local
		"fe80::/10",      // IPv6 link-local
		"::/128",         // IPv6 unspecified
		"ff00::/8",       // IPv6 multicast
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

// New creates an *http.Client with SSRF protection.
// It validates all resolved IPs at DialContext time (preventing DNS rebinding),
// blocks private/reserved IP ranges, and applies a 10-second timeout.
// Redirects are safe because each new connection goes through the same DialContext.
func New() *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
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
		},
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
