package domain

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// ValidateFeedURL validates a URL for use in data feed fetching.
// It performs SSRF protection by blocking:
// - Non-HTTP/HTTPS schemes
// - Private, loopback, and link-local IP addresses
// - Localhost and local domain names
// - Internal/restricted domain suffixes
// - Cloud metadata service endpoints
// - Query parameters and fragments
func ValidateFeedURL(urlStr string) error {
	if urlStr == "" {
		return errors.New("URL is required")
	}

	// Parse URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return errors.New("invalid URL: " + err.Error())
	}

	// Check scheme is http or https
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return errors.New("URL must use http or https scheme")
	}

	// Check host is present
	if parsedURL.Host == "" {
		return errors.New("URL must have a host")
	}

	// Check no query parameters
	if parsedURL.RawQuery != "" {
		return errors.New("URL must not contain query parameters")
	}

	// Check no fragment
	if parsedURL.Fragment != "" {
		return errors.New("URL must not contain fragment")
	}

	// Extract host without port
	host := parsedURL.Hostname()
	lowerHost := strings.ToLower(host)

	// Check for localhost and local domains
	if err := validateHostname(lowerHost); err != nil {
		return err
	}

	// Check for internal domain suffixes
	if err := validateInternalDomains(lowerHost); err != nil {
		return err
	}

	// Check if host is an IP address
	ip := net.ParseIP(host)
	if ip != nil {
		if err := validateIPAddress(ip); err != nil {
			return err
		}
	}

	return nil
}

// validateHostname checks for localhost and local domain names
func validateHostname(host string) error {
	// Check for localhost
	if host == "localhost" {
		return errors.New("URL must not use localhost or local domain")
	}

	// Check for localhost.localdomain
	if host == "localhost.localdomain" {
		return errors.New("URL must not use localhost or local domain")
	}

	// Check for subdomain of localhost
	if strings.HasSuffix(host, ".localhost") {
		return errors.New("URL must not use localhost or local domain")
	}

	// Check for .local suffix (mDNS/Bonjour)
	if strings.HasSuffix(host, ".local") {
		return errors.New("URL must not use localhost or local domain")
	}

	return nil
}

// validateInternalDomains checks for internal/restricted domain suffixes
func validateInternalDomains(host string) error {
	// Common internal domain suffixes
	internalSuffixes := []string{
		".internal",      // GCP metadata, general internal
		".corp",          // Corporate networks
		".intranet",      // Intranet domains
		".cluster.local", // Kubernetes cluster domains
	}

	for _, suffix := range internalSuffixes {
		if strings.HasSuffix(host, suffix) {
			return errors.New("URL must not use internal domain")
		}
	}

	// Exact matches for internal domains
	internalDomains := []string{
		"internal",
		"corp",
		"intranet",
	}

	for _, domain := range internalDomains {
		if host == domain {
			return errors.New("URL must not use internal domain")
		}
	}

	// Check for cloud metadata hostnames
	metadataHosts := []string{
		"metadata.google.internal",
		"metadata.azure.internal",
		"metadata",
	}

	for _, metaHost := range metadataHosts {
		if host == metaHost {
			return errors.New("URL must not use internal domain")
		}
	}

	return nil
}

// validateIPAddress checks if an IP address is private, loopback, or otherwise restricted
func validateIPAddress(ip net.IP) error {
	// Check for loopback addresses (127.0.0.0/8 for IPv4, ::1 for IPv6)
	if ip.IsLoopback() {
		return errors.New("URL must not use private or restricted IP address")
	}

	// Check for private addresses (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7)
	if ip.IsPrivate() {
		return errors.New("URL must not use private or restricted IP address")
	}

	// Check for link-local addresses (169.254.0.0/16, fe80::/10)
	// This includes the cloud metadata endpoint 169.254.169.254
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return errors.New("URL must not use private or restricted IP address")
	}

	// Check for unspecified address (0.0.0.0, ::)
	if ip.IsUnspecified() {
		return errors.New("URL must not use private or restricted IP address")
	}

	// Check for multicast addresses
	if ip.IsMulticast() {
		return errors.New("URL must not use private or restricted IP address")
	}

	// Check for IPv6 unique local addresses (fc00::/7)
	// Note: IsPrivate() should catch this, but we check explicitly for IPv6
	if len(ip) == net.IPv6len {
		// Check for unique local (fc00::/7)
		if ip[0] == 0xfc || ip[0] == 0xfd {
			return errors.New("URL must not use private or restricted IP address")
		}
	}

	// Check for IPv4-mapped IPv6 addresses - extract the IPv4 part and validate
	if ip.To4() != nil && len(ip) == net.IPv6len {
		// This is an IPv4-mapped IPv6 address, extract the IPv4 part
		ipv4 := ip.To4()
		if ipv4 != nil {
			// Recursively validate the IPv4 portion
			if ipv4.IsLoopback() || ipv4.IsPrivate() || ipv4.IsLinkLocalUnicast() || ipv4.IsUnspecified() {
				return errors.New("URL must not use private or restricted IP address")
			}
		}
	}

	// Check for broadcast address (255.255.255.255)
	if ip.Equal(net.IPv4bcast) {
		return errors.New("URL must not use private or restricted IP address")
	}

	return nil
}
