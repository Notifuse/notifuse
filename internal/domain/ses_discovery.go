package domain

import (
	"context"
	"fmt"
)

//go:generate mockgen -destination mocks/mock_ses_discovery_service.go -package mocks github.com/Notifuse/notifuse/internal/domain SESDiscoveryServiceInterface

// SESCredentialsRef identifies which AWS credentials a discovery call should use. There are two
// modes on purpose:
//
//   - a saved integration, which is what the edit drawer uses (its secret-key field is blank
//     because the stored secret is never sent to the browser);
//   - credentials typed into the create drawer, where nothing is saved yet. The same secret is
//     already posted to this backend on save, so this adds no new exposure.
type SESCredentialsRef struct {
	WorkspaceID   string `json:"workspace_id"`
	IntegrationID string `json:"integration_id,omitempty"`

	Region    string `json:"region,omitempty"`
	AccessKey string `json:"access_key,omitempty"`
	SecretKey string `json:"secret_key,omitempty"`
}

// Validate checks the reference is usable before any AWS client is built.
func (r *SESCredentialsRef) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if r.IntegrationID != "" {
		return nil
	}

	if r.Region == "" || r.AccessKey == "" || r.SecretKey == "" {
		return fmt.Errorf("provide integration_id, or region, access_key and secret_key together")
	}

	// The region is interpolated into the AWS endpoint host, so it is never taken on trust.
	if !IsValidSESRegion(r.Region) {
		return fmt.Errorf("unsupported AWS region: %s", r.Region)
	}

	return nil
}

// UsesSavedIntegration reports whether stored credentials should be loaded.
func (r *SESCredentialsRef) UsesSavedIntegration() bool {
	return r.IntegrationID != ""
}

// ListSESTenantsResponse is the payload behind the tenant picker.
type ListSESTenantsResponse struct {
	Tenants []SESTenant `json:"tenants"`
	HasMore bool        `json:"has_more"`
}

// ListSESConfigurationSetsResponse is the payload behind the configuration-set picker.
type ListSESConfigurationSetsResponse struct {
	ConfigurationSets []string `json:"configuration_sets"`
	HasMore           bool     `json:"has_more"`
}

// VerifySESTenantRequest asks whether a tenant is actually usable for sending.
type VerifySESTenantRequest struct {
	SESCredentialsRef
	TenantName           string `json:"tenant_name"`
	ConfigurationSetName string `json:"configuration_set_name,omitempty"`
}

func (r *VerifySESTenantRequest) Validate() error {
	if err := r.SESCredentialsRef.Validate(); err != nil {
		return err
	}
	if r.TenantName == "" {
		return fmt.Errorf("tenant_name is required")
	}
	return nil
}

// EnableSESTenantIsolationRequest provisions managed isolation for a saved integration.
type EnableSESTenantIsolationRequest struct {
	WorkspaceID   string `json:"workspace_id"`
	IntegrationID string `json:"integration_id"`
}

func (r *EnableSESTenantIsolationRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	// Provisioning creates a billable AWS resource and writes derived state back onto the
	// integration, so it only ever runs against something already saved.
	if r.IntegrationID == "" {
		return fmt.Errorf("integration_id is required")
	}
	return nil
}

// SESDiscoveryServiceInterface exposes the SES account inspection and provisioning the console
// needs. Every method is owner-only.
type SESDiscoveryServiceInterface interface {
	ListTenants(ctx context.Context, ref SESCredentialsRef) (*ListSESTenantsResponse, error)
	ListConfigurationSets(ctx context.Context, ref SESCredentialsRef) (*ListSESConfigurationSetsResponse, error)
	VerifyTenant(ctx context.Context, req VerifySESTenantRequest) (*SESTenantVerification, error)
	EnableTenantIsolation(ctx context.Context, req EnableSESTenantIsolationRequest) (*SESTenantProvisionResult, error)
}
