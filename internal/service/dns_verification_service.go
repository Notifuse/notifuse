package service

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// DNSVerificationService handles DNS verification for custom domains
type DNSVerificationService struct {
	logger         logger.Logger
	expectedTarget string // The CNAME target (e.g., "notifuse.com" or your main domain)
}

// NewDNSVerificationService creates a new DNS verification service
func NewDNSVerificationService(logger logger.Logger, expectedTarget string) *DNSVerificationService {
	return &DNSVerificationService{
		logger:         logger,
		expectedTarget: expectedTarget,
	}
}

// VerifyDomainOwnership checks if the domain has correct CNAME or A record pointing to our service
func (s *DNSVerificationService) VerifyDomainOwnership(ctx context.Context, domainURL string) error {
	// Extract hostname from custom_endpoint_url
	parsed, err := url.Parse(domainURL)
	if err != nil {
		return domain.ValidationError{Message: fmt.Sprintf("invalid domain URL: %v", err)}
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return domain.ValidationError{Message: "no hostname found in URL"}
	}

	s.logger.WithFields(map[string]interface{}{
		"hostname":        hostname,
		"expected_target": s.expectedTarget,
	}).Debug("Verifying domain ownership")

	// Look up CNAME record
	cname, err := net.LookupCNAME(hostname)
	if err != nil {
		return domain.ValidationError{
			Message: fmt.Sprintf("DNS lookup failed for %s: %v. Please ensure DNS is configured with a CNAME record pointing to %s or an A record pointing to the same IP address(es) as %s",
				hostname, err, s.expectedTarget, s.expectedTarget),
		}
	}

	// Verify CNAME points to expected target
	cname = strings.TrimSuffix(cname, ".")
	expectedTarget := strings.TrimSuffix(s.expectedTarget, ".")

	s.logger.WithFields(map[string]interface{}{
		"hostname":        hostname,
		"cname":           cname,
		"expected_target": expectedTarget,
	}).Debug("CNAME lookup result")

	// If CNAME points to itself, it means no CNAME record exists (A record instead)
	if cname == hostname+"." || cname == hostname {
		// Fall back to A record validation
		return s.verifyARecord(ctx, hostname, expectedTarget)
	}

	// Check if CNAME ends with expected target (allows subdomains)
	if !strings.HasSuffix(cname, expectedTarget) && cname != hostname {
		return domain.ValidationError{
			Message: fmt.Sprintf("CNAME verification failed: %s points to %s, but expected it to point to %s. Alternatively, you can use an A record pointing to the same IP address(es) as %s",
				hostname, cname, expectedTarget, expectedTarget),
		}
	}

	s.logger.WithFields(map[string]interface{}{
		"hostname": hostname,
		"cname":    cname,
	}).Info("Domain ownership verified successfully via CNAME")

	return nil
}

// verifyARecord verifies domain ownership via A record by comparing IP addresses
func (s *DNSVerificationService) verifyARecord(ctx context.Context, hostname, expectedTarget string) error {
	// Resolve expected target (API endpoint) to IP addresses
	expectedIPs, err := net.LookupIP(expectedTarget)
	if err != nil {
		return domain.ValidationError{
			Message: fmt.Sprintf("Failed to resolve expected target %s: %v. Please ensure DNS is configured with a CNAME record pointing to %s or an A record pointing to the same IP address(es)",
				expectedTarget, err, expectedTarget),
		}
	}

	if len(expectedIPs) == 0 {
		return domain.ValidationError{
			Message: fmt.Sprintf("No IP addresses found for expected target %s. Please ensure DNS is configured with a CNAME record pointing to %s or an A record pointing to the same IP address(es)",
				expectedTarget, expectedTarget),
		}
	}

	// Look up A records for the custom domain
	hostnameIPs, err := net.LookupIP(hostname)
	if err != nil {
		return domain.ValidationError{
			Message: fmt.Sprintf("A record lookup failed for %s: %v. Please ensure DNS is configured with a CNAME record pointing to %s or an A record pointing to the same IP address(es) as %s",
				hostname, err, expectedTarget, expectedTarget),
		}
	}

	if len(hostnameIPs) == 0 {
		return domain.ValidationError{
			Message: fmt.Sprintf("No A records found for %s. Please ensure DNS is configured with a CNAME record pointing to %s or an A record pointing to the same IP address(es) as %s",
				hostname, expectedTarget, expectedTarget),
		}
	}

	// Compare IP addresses - check if at least one matches
	expectedIPMap := make(map[string]bool)
	for _, ip := range expectedIPs {
		// Normalize IP addresses to strings for comparison
		expectedIPMap[ip.String()] = true
	}

	for _, ip := range hostnameIPs {
		if expectedIPMap[ip.String()] {
			s.logger.WithFields(map[string]interface{}{
				"hostname": hostname,
				"ip":       ip.String(),
			}).Info("Domain ownership verified successfully via A record")
			return nil
		}
	}

	// No matching IP found
	return domain.ValidationError{
		Message: fmt.Sprintf("A record verification failed: %s points to IP address(es) %v, but expected it to point to the same IP address(es) as %s (%v). Alternatively, you can use a CNAME record pointing to %s",
			hostname, hostnameIPs, expectedTarget, expectedIPs, expectedTarget),
	}
}

// VerifyTXTRecord verifies domain ownership via TXT record (alternative method)
// This is useful for apex domains that cannot use CNAME
func (s *DNSVerificationService) VerifyTXTRecord(ctx context.Context, domainURL, expectedToken string) error {
	parsed, err := url.Parse(domainURL)
	if err != nil {
		return domain.ValidationError{Message: fmt.Sprintf("invalid domain URL: %v", err)}
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return domain.ValidationError{Message: "no hostname found in URL"}
	}

	// Look up TXT records
	txtRecords, err := net.LookupTXT(hostname)
	if err != nil {
		return domain.ValidationError{Message: fmt.Sprintf("TXT lookup failed: %v", err)}
	}

	// Look for verification token
	expectedRecord := fmt.Sprintf("notifuse-verify=%s", expectedToken)
	for _, record := range txtRecords {
		if strings.TrimSpace(record) == expectedRecord {
			s.logger.WithFields(map[string]interface{}{
				"hostname": hostname,
				"token":    expectedToken,
			}).Info("Domain ownership verified via TXT record")
			return nil
		}
	}

	return domain.ValidationError{
		Message: fmt.Sprintf("TXT verification failed: no matching verification record found. Please add TXT record: %s", expectedRecord),
	}
}
