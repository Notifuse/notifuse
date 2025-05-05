package domain

import "context"

// WebhookProvider defines a common interface for all email providers that support webhooks
type WebhookProvider interface {
	// RegisterWebhooks registers webhooks for the specified events
	RegisterWebhooks(ctx context.Context, workspaceID, integrationID string, baseURL string, eventTypes []EmailEventType, providerConfig *EmailProvider) (*WebhookRegistrationStatus, error)

	// GetWebhookStatus checks the current status of webhooks
	GetWebhookStatus(ctx context.Context, workspaceID, integrationID string, providerConfig *EmailProvider) (*WebhookRegistrationStatus, error)

	// UnregisterWebhooks removes all webhooks for this integration
	UnregisterWebhooks(ctx context.Context, workspaceID, integrationID string, providerConfig *EmailProvider) error
}
