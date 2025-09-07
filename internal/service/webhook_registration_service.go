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

	return svc
}

// RegisterWebhooks registers webhook URLs with the email provider
func (s *WebhookRegistrationService) RegisterWebhooks(
	ctx context.Context,
	workspaceID string,
	config *domain.WebhookRegistrationConfig,
) (*domain.WebhookRegistrationStatus, error) {
	// Authenticate the user for this workspace
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
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
	return provider.RegisterWebhooks(ctx, workspaceID, config.IntegrationID, baseURL, config.EventTypes, emailProvider)
}

// GetWebhookStatus gets the status of webhooks for an email provider
func (s *WebhookRegistrationService) GetWebhookStatus(
	ctx context.Context,
	workspaceID string,
	integrationID string,
) (*domain.WebhookRegistrationStatus, error) {
	// Authenticate the user for this workspace
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
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

// UnregisterWebhooks removes all webhook URLs associated with the integration
func (s *WebhookRegistrationService) UnregisterWebhooks(
	ctx context.Context,
	workspaceID string,
	integrationID string,
) error {
	// Authenticate the user for this workspace
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
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
