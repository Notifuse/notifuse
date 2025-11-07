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

// VerifyDomainOwnership checks if the domain has correct CNAME pointing to our service
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
			Message: fmt.Sprintf("CNAME lookup failed for %s: %v. Please ensure DNS is configured with a CNAME record pointing to %s",
				hostname, err, s.expectedTarget),
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
		return domain.ValidationError{
			Message: fmt.Sprintf("No CNAME record found for %s. Please create a CNAME record pointing to %s",
				hostname, expectedTarget),
		}
	}

	// Check if CNAME ends with expected target (allows subdomains)
	if !strings.HasSuffix(cname, expectedTarget) && cname != hostname {
		return domain.ValidationError{
			Message: fmt.Sprintf("CNAME verification failed: %s points to %s, but expected it to point to %s",
				hostname, cname, expectedTarget),
		}
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
