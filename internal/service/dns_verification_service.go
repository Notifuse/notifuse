package service

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

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

// VerifyDomainOwnership checks if the domain has correct CNAME pointing to our service
func (s *DNSVerificationService) VerifyDomainOwnership(ctx context.Context, domainURL string) error {
	// Extract hostname from custom_endpoint_url
	parsed, err := url.Parse(domainURL)
	if err != nil {
		return fmt.Errorf("invalid domain URL: %w", err)
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return fmt.Errorf("no hostname found in URL")
	}

	s.logger.WithFields(map[string]interface{}{
		"hostname":        hostname,
		"expected_target": s.expectedTarget,
	}).Debug("Verifying domain ownership")

	// Look up CNAME record
	cname, err := net.LookupCNAME(hostname)
	if err != nil {
		return fmt.Errorf("CNAME lookup failed: %w (ensure DNS is configured with CNAME record)", err)
	}

	// Verify CNAME points to expected target
	cname = strings.TrimSuffix(cname, ".")
	expectedTarget := strings.TrimSuffix(s.expectedTarget, ".")

	s.logger.WithFields(map[string]interface{}{
		"hostname":        hostname,
		"cname":           cname,
		"expected_target": expectedTarget,
	}).Debug("CNAME lookup result")

	// Check if CNAME ends with expected target (allows subdomains)
	if !strings.HasSuffix(cname, expectedTarget) && cname != hostname {
		return fmt.Errorf("CNAME verification failed: %s points to %s, expected %s",
			hostname, cname, expectedTarget)
	}

	// If CNAME points to itself, it means no CNAME record exists (A record instead)
	if cname == hostname+"." || cname == hostname {
		return fmt.Errorf("no CNAME record found for %s. Please create a CNAME record pointing to %s",
			hostname, expectedTarget)
	}

	s.logger.WithFields(map[string]interface{}{
		"hostname": hostname,
		"cname":    cname,
	}).Info("Domain ownership verified successfully")

	return nil
}

// VerifyTXTRecord verifies domain ownership via TXT record (alternative method)
// This is useful for apex domains that cannot use CNAME
func (s *DNSVerificationService) VerifyTXTRecord(ctx context.Context, domainURL, expectedToken string) error {
	parsed, err := url.Parse(domainURL)
	if err != nil {
		return fmt.Errorf("invalid domain URL: %w", err)
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return fmt.Errorf("no hostname found in URL")
	}

	// Look up TXT records
	txtRecords, err := net.LookupTXT(hostname)
	if err != nil {
		return fmt.Errorf("TXT lookup failed: %w", err)
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

	return fmt.Errorf("TXT verification failed: no matching verification record found. Please add TXT record: %s", expectedRecord)
}
