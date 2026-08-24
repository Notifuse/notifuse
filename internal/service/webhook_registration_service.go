package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// WebhookRegistrationService implements the domain.WebhookRegistrationService interface
type WebhookRegistrationService struct {
	workspaceRepo    domain.WorkspaceRepository
	authService      domain.AuthService
	logger           logger.Logger
	apiEndpoint      string
	webhookProviders map[domain.EmailProviderKind]domain.WebhookProvider
}

// NewWebhookRegistrationService creates a new webhook registration service
func NewWebhookRegistrationService(
	workspaceRepo domain.WorkspaceRepository,
	authService domain.AuthService,
	postmarkService domain.PostmarkServiceInterface,
	mailgunService domain.MailgunServiceInterface,
	mailjetService domain.MailjetServiceInterface,
	sparkPostService domain.SparkPostServiceInterface,
	sesService domain.SESServiceInterface,
	sendGridService domain.SendGridServiceInterface,
	logger logger.Logger,
	apiEndpoint string,
) *WebhookRegistrationService {
	// Create the service
	svc := &WebhookRegistrationService{
		workspaceRepo:    workspaceRepo,
		authService:      authService,
		logger:           logger,
		apiEndpoint:      apiEndpoint,
		webhookProviders: make(map[domain.EmailProviderKind]domain.WebhookProvider),
	}

	// Register services that implement the WebhookProvider interface
	if provider, ok := sparkPostService.(domain.WebhookProvider); ok {
		svc.webhookProviders[domain.EmailProviderKindSparkPost] = provider
	}

	if provider, ok := postmarkService.(domain.WebhookProvider); ok {
		svc.webhookProviders[domain.EmailProviderKindPostmark] = provider
	}

	if provider, ok := mailgunService.(domain.WebhookProvider); ok {
		svc.webhookProviders[domain.EmailProviderKindMailgun] = provider
	}

	if provider, ok := mailjetService.(domain.WebhookProvider); ok {
		svc.webhookProviders[domain.EmailProviderKindMailjet] = provider
	}

	if provider, ok := sesService.(domain.WebhookProvider); ok {
		svc.webhookProviders[domain.EmailProviderKindSES] = provider
	}

	if provider, ok := sendGridService.(domain.WebhookProvider); ok {
		svc.webhookProviders[domain.EmailProviderKindSendGrid] = provider
	}

	return svc
}

// authorizeOwner confirms the caller owns the workspace they named.
//
// ESP-side registration gets no PermissionResource of its own. It reads the
// workspace's un-redacted provider credentials through getEmailProviderConfig and
// calls the ESP's API with them, and the integrations those credentials belong to
// are already owner-only to create or edit — a member-grantable permission here
// would hand that reach back out.
func (s *WebhookRegistrationService) authorizeOwner(ctx context.Context, workspaceID string) (context.Context, error) {
	ctx, user, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return ctx, fmt.Errorf("failed to authenticate user: %w", err)
	}
	if userWorkspace == nil || userWorkspace.Role != "owner" {
		entry := s.logger.WithField("workspace_id", workspaceID)
		if user != nil {
			entry = entry.WithField("user_id", user.ID)
		}
		entry.Error("User is not an owner of the workspace")
		return ctx, &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}
	return ctx, nil
}

// RegisterWebhooks registers webhook URLs with the email provider
func (s *WebhookRegistrationService) RegisterWebhooks(
	ctx context.Context,
	workspaceID string,
	config *domain.WebhookRegistrationConfig,
) (*domain.WebhookRegistrationStatus, error) {
	ctx, err := s.authorizeOwner(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	// Get email provider configuration from workspace settings
	emailProvider, err := s.getEmailProviderConfig(ctx, workspaceID, config.IntegrationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get email provider configuration: %w", err)
	}

	// Convert webhook base URL if needed (remove trailing slash)
	baseURL := strings.TrimSuffix(s.apiEndpoint, "/")

	// Get provider implementation
	provider, ok := s.webhookProviders[emailProvider.Kind]
	if !ok {
		return nil, fmt.Errorf("webhook registration not implemented for provider: %s", emailProvider.Kind)
	}

	// Delegate to provider implementation with the provider configuration
	status, err := provider.RegisterWebhooks(ctx, workspaceID, config.IntegrationID, baseURL, config.EventTypes, emailProvider)
	if err != nil {
		return nil, err
	}

	// Persist the configuration set SES sends through, so the send path can read it from
	// settings instead of calling ListConfigurationSets — an API AWS allows "no more than once
	// per second" per account and region, which every broadcast used to exceed. Best-effort:
	// on failure the send path falls back to the (throttled, memoised) lookup.
	if emailProvider.Kind == domain.EmailProviderKindSES && status != nil {
		if name, ok := status.ProviderDetails["configuration_set"].(string); ok && name != "" {
			if managed, _ := status.ProviderDetails["configuration_set_managed"].(bool); managed {
				if err := s.persistSESManagedConfigurationSet(ctx, workspaceID, config.IntegrationID, name); err != nil {
					s.logger.WithField("workspace_id", workspaceID).
						WithField("integration_id", config.IntegrationID).
						Warn("Failed to persist SES configuration set name: " + err.Error())
				}
			}
		}
	}

	// Re-attach the configuration set to the tenant. Registration may have just recreated that
	// set, which AWS treats as a new resource with no associations — leaving every tenant send
	// rejected until it is restored. Creating nothing here, so it is safe to run implicitly.
	if emailProvider.Kind == domain.EmailProviderKindSES && emailProvider.SES != nil &&
		emailProvider.SES.ResolveTenant() != "" {
		if associator, ok := provider.(domain.TenantAssociator); ok {
			if _, err := associator.AssociateExistingTenant(ctx, *emailProvider.SES, config.IntegrationID, emailProvider.Senders); err != nil {
				s.logger.WithField("workspace_id", workspaceID).
					WithField("integration_id", config.IntegrationID).
					Warn("Failed to re-associate SES tenant resources after registering webhooks: " + err.Error())
			}
		}
	}

	// For providers whose inbound (reply) mail arrives via a provider-side route rather
	// than an event webhook (e.g. Mailgun Routes), also register that route so
	// stop-on-reply works without manual ESP setup. Providers that don't support this
	// simply don't implement the interface, so this step is skipped for them.
	//
	// Inbound provisioning is BEST-EFFORT relative to the primary registration: it uses a
	// different (often broader) permission surface, so on failure we log and still return the
	// already-succeeded delivery/bounce/complaint status rather than reporting the whole
	// "Register Webhooks" action as failed. GetWebhookStatus surfaces inbound_registered=false.
	if registrar, ok := provider.(domain.InboundRouteRegistrar); ok {
		inboundURL := domain.GenerateInboundWebhookURL(baseURL, workspaceID, config.IntegrationID)
		if err := registrar.EnsureInboundRoute(ctx, emailProvider, inboundURL); err != nil {
			s.logger.WithField("workspace_id", workspaceID).
				WithField("integration_id", config.IntegrationID).
				WithField("provider", string(emailProvider.Kind)).
				Warn("Inbound reply route registration failed; stop-on-reply unavailable until resolved: " + err.Error())
		} else if emailProvider.Kind == domain.EmailProviderKindSES &&
			emailProvider.SES != nil && emailProvider.SES.InboundTopicARN != "" {
			// Persist the provisioned SNS topic ARN so the inbound parser can bind to it
			// (the authentication check that a message came from OUR topic). Best-effort:
			// a persistence failure leaves inbound unauthenticated-and-rejected, not open.
			if err := s.persistSESInboundTopicARN(ctx, workspaceID, config.IntegrationID, emailProvider.SES.InboundTopicARN); err != nil {
				s.logger.WithField("workspace_id", workspaceID).
					WithField("integration_id", config.IntegrationID).
					Warn("Failed to persist SES inbound topic ARN; inbound replies will be rejected until re-registered: " + err.Error())
			}
		}
	}

	return status, nil
}

// persistSESManagedConfigurationSet records the configuration set Notifuse manages for this
// integration so the send path never has to discover it. Mirrors persistSESInboundTopicARN.
func (s *WebhookRegistrationService) persistSESManagedConfigurationSet(ctx context.Context, workspaceID, integrationID, name string) error {
	if name == "" {
		return nil
	}
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to load workspace: %w", err)
	}
	integration := workspace.GetIntegrationByID(integrationID)
	if integration == nil || integration.EmailProvider.SES == nil {
		return fmt.Errorf("SES integration %s not found", integrationID)
	}
	if integration.EmailProvider.SES.ManagedConfigurationSet == name {
		return nil // already persisted
	}
	// Atomic single-statement merge: a full-row Update here would race with a concurrent
	// integration edit and silently lose one side.
	return s.workspaceRepo.PatchIntegrationSESSettings(ctx, workspaceID, integrationID,
		map[string]interface{}{"managed_configuration_set": name})
}

// persistSESInboundTopicARN saves the provisioned inbound SNS topic ARN onto the integration's
// SES settings so the inbound parser can bind to it. Re-saving the workspace round-trips the
// integration secrets through BeforeSave/AfterLoad encryption (same pattern as UpdateIntegration).
func (s *WebhookRegistrationService) persistSESInboundTopicARN(ctx context.Context, workspaceID, integrationID, arn string) error {
	if arn == "" {
		return nil
	}
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to load workspace: %w", err)
	}
	integration := workspace.GetIntegrationByID(integrationID)
	if integration == nil || integration.EmailProvider.SES == nil {
		return fmt.Errorf("SES integration %s not found", integrationID)
	}
	if integration.EmailProvider.SES.InboundTopicARN == arn {
		return nil // already persisted
	}
	return s.workspaceRepo.PatchIntegrationSESSettings(ctx, workspaceID, integrationID,
		map[string]interface{}{"inbound_topic_arn": arn})
}

// DeleteIntegrationResources removes the provider-side resources an integration owns.
//
// Webhooks come off first, then anything that deliberately outlives them. For Amazon SES that
// second step is what removes the configuration set and the tenant: unregistering webhooks
// leaves both in place because sends still resolve them, so without this an operator who
// enabled tenant isolation would keep paying AWS for a tenant they can no longer see.
func (s *WebhookRegistrationService) DeleteIntegrationResources(ctx context.Context, workspaceID string, integrationID string) error {
	emailProvider, err := s.getEmailProviderConfig(ctx, workspaceID, integrationID)
	if err != nil {
		return fmt.Errorf("failed to get email provider configuration: %w", err)
	}

	provider, ok := s.webhookProviders[emailProvider.Kind]
	if !ok {
		return fmt.Errorf("webhook registration not implemented for provider: %s", emailProvider.Kind)
	}

	// Best-effort: a provider-side failure must never prevent removing the integration.
	if err := provider.UnregisterWebhooks(ctx, workspaceID, integrationID, emailProvider); err != nil {
		s.logger.WithField("workspace_id", workspaceID).
			WithField("integration_id", integrationID).
			Warn("Failed to unregister webhooks during integration deletion: " + err.Error())
	}

	if teardown, ok := provider.(domain.SendingResourceTeardown); ok {
		if err := teardown.DeleteSendingResources(ctx, workspaceID, integrationID, emailProvider); err != nil {
			s.logger.WithField("workspace_id", workspaceID).
				WithField("integration_id", integrationID).
				Warn("Failed to delete provider sending resources during integration deletion: " + err.Error())
		}
	}

	return nil
}

// GetWebhookStatus gets the status of webhooks for an email provider
func (s *WebhookRegistrationService) GetWebhookStatus(
	ctx context.Context,
	workspaceID string,
	integrationID string,
) (*domain.WebhookRegistrationStatus, error) {
	ctx, err := s.authorizeOwner(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	// Get email provider configuration from workspace settings
	emailProvider, err := s.getEmailProviderConfig(ctx, workspaceID, integrationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get email provider configuration: %w", err)
	}

	// Get provider implementation
	provider, ok := s.webhookProviders[emailProvider.Kind]
	if !ok {
		return nil, fmt.Errorf("webhook status check not implemented for provider: %s", emailProvider.Kind)
	}

	// Delegate to provider implementation with the provider configuration
	return provider.GetWebhookStatus(ctx, workspaceID, integrationID, emailProvider)
}

// UnregisterWebhooks removes all webhook URLs associated with the integration.
//
// No route registers it and nothing in the repository calls it; the owner check is
// here so the method cannot become a member-reachable path to the provider
// credentials if a route is ever added.
func (s *WebhookRegistrationService) UnregisterWebhooks(
	ctx context.Context,
	workspaceID string,
	integrationID string,
) error {
	ctx, err := s.authorizeOwner(ctx, workspaceID)
	if err != nil {
		return err
	}

	// Get email provider configuration from workspace settings
	emailProvider, err := s.getEmailProviderConfig(ctx, workspaceID, integrationID)
	if err != nil {
		return fmt.Errorf("failed to get email provider configuration: %w", err)
	}

	// Get provider implementation
	provider, ok := s.webhookProviders[emailProvider.Kind]
	if !ok {
		return fmt.Errorf("webhook unregistration not implemented for provider: %s", emailProvider.Kind)
	}

	// Delegate to provider implementation with the provider configuration
	return provider.UnregisterWebhooks(ctx, workspaceID, integrationID, emailProvider)
}

// getEmailProviderConfig gets the email provider configuration from workspace settings
func (s *WebhookRegistrationService) getEmailProviderConfig(ctx context.Context, workspaceID string, integrationID string) (*domain.EmailProvider, error) {
	// Get workspace settings from the database
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	// Find the integration by ID
	integration := workspace.GetIntegrationByID(integrationID)
	if integration == nil {
		return nil, fmt.Errorf("integration with ID %s not found", integrationID)
	}

	return &integration.EmailProvider, nil
}
