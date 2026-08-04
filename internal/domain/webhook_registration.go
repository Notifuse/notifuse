package domain

import (
	"context"
	"fmt"
)

//go:generate mockgen -destination mocks/mock_webhook_registration_service.go -package mocks github.com/Notifuse/notifuse/internal/domain WebhookRegistrationService

// WebhookRegistrationService defines the interface for registering webhooks with email providers
type WebhookRegistrationService interface {
	// RegisterWebhooks registers the webhook URLs with the email provider
	RegisterWebhooks(ctx context.Context, workspaceID string, config *WebhookRegistrationConfig) (*WebhookRegistrationStatus, error)

	// UnregisterWebhooks removes all webhook URLs associated with the integration
	UnregisterWebhooks(ctx context.Context, workspaceID string, integrationID string) error

	// GetWebhookStatus gets the current status of webhooks for the email provider
	GetWebhookStatus(ctx context.Context, workspaceID string, integrationID string) (*WebhookRegistrationStatus, error)

	// DeleteIntegrationResources tears down everything the provider holds for this integration:
	// its webhooks and any sending resources that deliberately outlive them, such as an SES
	// configuration set and the tenant that bills monthly for as long as it exists.
	DeleteIntegrationResources(ctx context.Context, workspaceID string, integrationID string) error
}

// WebhookRegistrationConfig defines the configuration for registering webhooks
type WebhookRegistrationConfig struct {
	IntegrationID string           `json:"integration_id"`
	EventTypes    []EmailEventType `json:"event_types"`
}

// WebhookRegistrationStatus represents the current status of webhooks for a provider
type WebhookRegistrationStatus struct {
	EmailProviderKind EmailProviderKind       `json:"email_provider_kind"`
	IsRegistered      bool                    `json:"is_registered"`
	Endpoints         []WebhookEndpointStatus `json:"endpoints,omitempty"`
	Error             string                  `json:"error,omitempty"`
	ProviderDetails   map[string]interface{}  `json:"provider_details,omitempty"`
}

// WebhookEndpointStatus represents the status of a single webhook endpoint
type WebhookEndpointStatus struct {
	WebhookID string         `json:"webhook_id,omitempty"`
	URL       string         `json:"url"`
	EventType EmailEventType `json:"event_type"`
	Active    bool           `json:"active"`
}

// RegisterWebhookRequest defines the request to register webhooks
type RegisterWebhookRequest struct {
	WorkspaceID   string           `json:"workspace_id"`
	IntegrationID string           `json:"integration_id"`
	EventTypes    []EmailEventType `json:"event_types"`
}

// Validate validates the RegisterWebhookRequest
func (r *RegisterWebhookRequest) Validate() error {
	if r.WorkspaceID == "" {
		return NewValidationError("workspace_id is required")
	}
	if r.IntegrationID == "" {
		return NewValidationError("integration_id is required")
	}
	if len(r.EventTypes) == 0 {
		// Default to all event types if not specified
		r.EventTypes = []EmailEventType{
			EmailEventDelivered,
			EmailEventBounce,
			EmailEventComplaint,
		}
	}

	return nil
}

// GetWebhookStatusRequest defines the request to get webhook status
type GetWebhookStatusRequest struct {
	WorkspaceID   string `json:"workspace_id"`
	IntegrationID string `json:"integration_id"`
}

// Validate validates the GetWebhookStatusRequest
func (r *GetWebhookStatusRequest) Validate() error {
	if r.WorkspaceID == "" {
		return NewValidationError("workspace_id is required")
	}
	if r.IntegrationID == "" {
		return NewValidationError("integration_id is required")
	}
	return nil
}

// GenerateWebhookCallbackURL generates a standardized webhook callback URL for a specific provider
// The URL follows the format: {baseURL}/webhooks/email?provider={provider}&workspace_id={workspaceID}&integration_id={integrationID}
// This ensures a consistent URL pattern across all email provider integrations.
//
// Parameters:
//   - baseURL: The base URL of the application (e.g., "https://api.example.com")
//   - provider: The email provider kind (e.g., domain.EmailProviderKindPostmark)
//   - workspaceID: The workspace ID
//   - integrationID: The integration ID
//
// Returns:
//   - The fully formatted webhook callback URL
func GenerateWebhookCallbackURL(baseURL string, provider EmailProviderKind, workspaceID string, integrationID string) string {
	return fmt.Sprintf("%s/webhooks/email?provider=%s&workspace_id=%s&integration_id=%s",
		baseURL,
		provider,
		workspaceID,
		integrationID)
}

// GenerateInboundWebhookURL generates the callback URL a provider's inbound-mail
// feature (e.g. a Mailgun Route's forward() action) should POST replies to.
// Format: {baseURL}/webhooks/email/inbound?workspace_id={workspaceID}&integration_id={integrationID}
func GenerateInboundWebhookURL(baseURL string, workspaceID string, integrationID string) string {
	return fmt.Sprintf("%s/webhooks/email/inbound?workspace_id=%s&integration_id=%s",
		baseURL,
		workspaceID,
		integrationID)
}

// InboundRouteRegistrar is an optional capability for webhook providers whose inbound
// (reply) mail is delivered via a provider-side route/rule rather than a regular event
// webhook (e.g. Mailgun Routes). The orchestration layer invokes it, when supported,
// as part of webhook registration so stop-on-reply works without manual ESP setup.
// SendingResourceTeardown is implemented by providers that create long-lived sending resources
// which outlive webhook registration. Unregistering webhooks must not remove them — sends still
// depend on them — so they are torn down only when the integration itself is deleted.
type SendingResourceTeardown interface {
	// DeleteSendingResources removes everything the provider created for this integration.
	// Implementations are best-effort: a failure is reported but must never block deletion.
	DeleteSendingResources(ctx context.Context, workspaceID string, integrationID string, providerConfig *EmailProvider) error
}

// TenantAssociator is implemented by providers whose sending resources must be re-attached to a
// tenant after they are recreated. It must never CREATE a tenant: that is billable and belongs
// to an explicit, confirmed action.
type TenantAssociator interface {
	AssociateExistingTenant(ctx context.Context, config AmazonSESSettings, integrationID string, senders []EmailSender) (*SESTenantProvisionResult, error)
}

type InboundRouteRegistrar interface {
	// EnsureInboundRoute idempotently ensures a route exists that forwards inbound mail
	// to inboundURL. Implementations must be a no-op when the route already exists.
	EnsureInboundRoute(ctx context.Context, providerConfig *EmailProvider, inboundURL string) error
}
