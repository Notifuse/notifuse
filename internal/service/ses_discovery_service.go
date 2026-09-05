package service

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// sesTenantOperator is the slice of SESService this service needs. Declared here, in the
// consuming package, so the dependency stays visible and narrow.
type sesTenantOperator interface {
	ListSESTenants(ctx context.Context, config domain.AmazonSESSettings) ([]domain.SESTenant, bool, error)
	ListConfigurationSets(ctx context.Context, config domain.AmazonSESSettings) ([]string, error)
	VerifyTenantAssociation(ctx context.Context, config domain.AmazonSESSettings, tenantName, configSetName string) (*domain.SESTenantVerification, error)
	EnsureTenantIsolation(ctx context.Context, config domain.AmazonSESSettings, integrationID string, senders []domain.EmailSender) (*domain.SESTenantProvisionResult, error)
}

// SESDiscoveryService answers "what is in this AWS account" and provisions managed tenant
// isolation. It is deliberately its own small service rather than more methods on
// WorkspaceService, which already takes fifteen positional dependencies across thirty-seven
// construction sites.
type SESDiscoveryService struct {
	workspaceRepo domain.WorkspaceRepository
	authService   domain.AuthService
	sesService    sesTenantOperator
	logger        logger.Logger
	entitlements  domain.EntitlementProvider
}

// NewSESDiscoveryService builds the service. The entitlement provider is a REQUIRED
// positional parameter, not a trailing variadic: a variadic made forgetting it compile
// silently, and the G4 gate below then read a nil provider and let every deployment provision
// SES tenants for free. Omitting the argument must be a compile error, because a licence gate
// that is inert in production and green in CI is worse than no gate at all.
//
// An unlicensed deployment does NOT pass nil here — it passes a provider that answers
// CommunityEntitlements, and provisioning is refused.
func NewSESDiscoveryService(
	workspaceRepo domain.WorkspaceRepository,
	authService domain.AuthService,
	sesService sesTenantOperator,
	logger logger.Logger,
	entitlements domain.EntitlementProvider,
) *SESDiscoveryService {
	return &SESDiscoveryService{
		workspaceRepo: workspaceRepo,
		authService:   authService,
		sesService:    sesService,
		logger:        logger,
		entitlements:  entitlements,
	}
}

func (s *SESDiscoveryService) ListTenants(ctx context.Context, ref domain.SESCredentialsRef) (*domain.ListSESTenantsResponse, error) {
	settings, err := s.resolveSettings(ctx, ref)
	if err != nil {
		return nil, err
	}

	tenants, hasMore, err := s.sesService.ListSESTenants(ctx, *settings)
	if err != nil {
		return nil, err
	}

	return &domain.ListSESTenantsResponse{Tenants: tenants, HasMore: hasMore}, nil
}

func (s *SESDiscoveryService) ListConfigurationSets(ctx context.Context, ref domain.SESCredentialsRef) (*domain.ListSESConfigurationSetsResponse, error) {
	settings, err := s.resolveSettings(ctx, ref)
	if err != nil {
		return nil, err
	}

	sets, err := s.sesService.ListConfigurationSets(ctx, *settings)
	if err != nil {
		if domain.IsSESAccessDenied(err) {
			return nil, domain.ErrSESAccessDenied
		}
		return nil, err
	}

	return &domain.ListSESConfigurationSetsResponse{ConfigurationSets: sets}, nil
}

func (s *SESDiscoveryService) VerifyTenant(ctx context.Context, req domain.VerifySESTenantRequest) (*domain.SESTenantVerification, error) {
	settings, err := s.resolveSettings(ctx, req.SESCredentialsRef)
	if err != nil {
		return nil, err
	}

	configSetName := req.ConfigurationSetName
	if configSetName == "" && req.IntegrationID != "" {
		configSetName, _ = configurationSetFor(settings, req.IntegrationID)
	}

	return s.sesService.VerifyTenantAssociation(ctx, *settings, req.TenantName, configSetName)
}

// EnableTenantIsolation provisions the tenant and then persists its name LAST, so a failure
// leaves AWS holding resources Notifuse is not using yet — billable, visible and retryable —
// rather than Notifuse routing sends through a tenant that is not ready.
func (s *SESDiscoveryService) EnableTenantIsolation(ctx context.Context, req domain.EnableSESTenantIsolationRequest) (*domain.SESTenantProvisionResult, error) {
	ctx, _, integration, err := s.authorizeIntegration(ctx, req.WorkspaceID, req.IntegrationID)
	if err != nil {
		return nil, err
	}

	// Provisioning a NEW tenant is what is licensed, and this is the only place it happens.
	//
	// The send path is deliberately untouched: AmazonSESSettings.ResolveTenant and
	// applySESSendingContext build the same SendEmailInput with one field either nil or set, so
	// a licence check there would silently repoint a paying customer's mail from
	// tenant-scoped suppression to ACCOUNT-WIDE suppression — the exact outcome the feature was
	// bought to avoid. A tenant provisioned once keeps resolving forever, with no key.
	//
	// Placed after authorizeIntegration so a caller who may not touch this integration is told
	// that, rather than being told what the deployment has or has not bought.
	if !s.isLicensedFor(domain.FeatureSESTenant) {
		return nil, domain.NewFeatureNotLicensedError(domain.FeatureSESTenant)
	}

	settings := integration.EmailProvider.SES
	if settings == nil {
		return nil, fmt.Errorf("integration %s is not an Amazon SES integration", req.IntegrationID)
	}
	if settings.TenantName != "" {
		return nil, fmt.Errorf("integration %s uses a tenant you manage; clear it to let Notifuse manage one", req.IntegrationID)
	}

	result, err := s.sesService.EnsureTenantIsolation(ctx, *settings, req.IntegrationID, integration.EmailProvider.Senders)
	if err != nil {
		return nil, err
	}

	// Record the tenant only once a send through it would actually succeed. SES rejects any
	// send whose configuration set is not associated with the tenant, so persisting the name
	// after a failed association would convert a partial provisioning failure into a total
	// outage — every message from this integration rejected until someone noticed.
	if !result.ConfigurationSetAssociated {
		s.logger.WithField("workspace_id", req.WorkspaceID).
			WithField("integration_id", req.IntegrationID).
			Warn("SES tenant provisioned but its configuration set is not associated; not enabling it for sending")
		return result, nil
	}

	if err := s.workspaceRepo.PatchIntegrationSESSettings(ctx, req.WorkspaceID, req.IntegrationID,
		map[string]interface{}{
			"tenant_isolation_enabled": true,
			"managed_tenant_name":      result.TenantName,
		}); err != nil {
		// AWS has the tenant and is billing for it; the UI must say "retry to finish", not
		// "not provisioned".
		s.logger.WithField("workspace_id", req.WorkspaceID).
			WithField("integration_id", req.IntegrationID).
			Error("Provisioned SES tenant but failed to persist it: " + err.Error())
		result.ProvisionedButUnsaved = true
	}

	return result, nil
}

// isLicensedFor reports whether this deployment may use a licensed capability. It reads the
// signed key and nothing else — never the caller's role, which a synthesised platform admin
// would satisfy on every workspace. A nil provider leaves the gate inert.
func (s *SESDiscoveryService) isLicensedFor(feature domain.Feature) bool {
	if s.entitlements == nil {
		return true
	}
	return s.entitlements.Entitlements().Has(feature)
}

// resolveSettings authenticates the caller and returns the SES settings to use.
func (s *SESDiscoveryService) resolveSettings(ctx context.Context, ref domain.SESCredentialsRef) (*domain.AmazonSESSettings, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}

	if !ref.UsesSavedIntegration() {
		if _, err := s.authorizeOwner(ctx, ref.WorkspaceID); err != nil {
			return nil, err
		}
		return &domain.AmazonSESSettings{
			Region:    ref.Region,
			AccessKey: ref.AccessKey,
			SecretKey: ref.SecretKey,
		}, nil
	}

	_, _, integration, err := s.authorizeIntegration(ctx, ref.WorkspaceID, ref.IntegrationID)
	if err != nil {
		return nil, err
	}
	if integration.EmailProvider.SES == nil {
		return nil, fmt.Errorf("integration %s is not an Amazon SES integration", ref.IntegrationID)
	}

	// Integration.AfterLoad has already decrypted the secret key.
	return integration.EmailProvider.SES, nil
}

func (s *SESDiscoveryService) authorizeOwner(ctx context.Context, workspaceID string) (context.Context, error) {
	ctx, user, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}
	if userWorkspace.Role != "owner" {
		s.logger.WithField("workspace_id", workspaceID).
			WithField("user_id", user.ID).
			Error("User is not an owner of the workspace")
		return nil, &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}
	return ctx, nil
}

func (s *SESDiscoveryService) authorizeIntegration(ctx context.Context, workspaceID, integrationID string) (context.Context, *domain.Workspace, *domain.Integration, error) {
	ctx, err := s.authorizeOwner(ctx, workspaceID)
	if err != nil {
		return nil, nil, nil, err
	}

	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load workspace: %w", err)
	}

	integration := workspace.GetIntegrationByID(integrationID)
	if integration == nil {
		return nil, nil, nil, fmt.Errorf("integration %s not found", integrationID)
	}

	return ctx, workspace, integration, nil
}
