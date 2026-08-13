package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/google/uuid"
)

// webhookSecretPrefix is the Standard Webhooks symmetric-key prefix.
// See: https://github.com/standard-webhooks/standard-webhooks/blob/main/spec/standard-webhooks.md
const webhookSecretPrefix = "whsec_"

// authorize confirms the caller is a member of the workspace they named.
//
// INVARIANT: every method here takes workspaceID straight from the request, and
// must call this before touching a repository.
//
// Isolation is per-database, but opening a workspace database does not itself
// establish any right to it — workspaceID selects a database and asserts nothing
// more. This is what establishes the right.
//
// Deliberately membership, not a permission level. There is no webhook
// PermissionResource today, and adding one is not free: a new resource is absent
// from every existing member's stored permissions, so it denies everyone until a
// system migration backfills it. Granularity is worth having, but it is a
// separate change with a migration attached, and it must not hold up closing a
// cross-tenant hole.
func (s *WebhookSubscriptionService) authorize(ctx context.Context, workspaceID string) (context.Context, error) {
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return ctx, fmt.Errorf("failed to authenticate user: %w", err)
	}
	return ctx, nil
}

// WebhookSubscriptionService handles webhook subscription business logic
type WebhookSubscriptionService struct {
	repo         domain.WebhookSubscriptionRepository
	deliveryRepo domain.WebhookDeliveryRepository
	authService  domain.AuthService
	logger       logger.Logger
}

// NewWebhookSubscriptionService creates a new webhook subscription service
func NewWebhookSubscriptionService(
	repo domain.WebhookSubscriptionRepository,
	deliveryRepo domain.WebhookDeliveryRepository,
	authService domain.AuthService,
	logger logger.Logger,
) *WebhookSubscriptionService {
	return &WebhookSubscriptionService{
		repo:         repo,
		deliveryRepo: deliveryRepo,
		authService:  authService,
		logger:       logger,
	}
}

// generateSecret generates a secure random secret for webhook signing.
// Output format is `whsec_<base64(32 random bytes)>`, per Standard Webhooks.
func generateSecret() (string, error) {
	bytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return webhookSecretPrefix + base64.StdEncoding.EncodeToString(bytes), nil
}

// decodeSecret returns the raw HMAC key bytes for a stored webhook secret.
// The stored form must be `whsec_<base64(key)>` per Standard Webhooks.
func decodeSecret(stored string) ([]byte, error) {
	if !strings.HasPrefix(stored, webhookSecretPrefix) {
		return nil, fmt.Errorf("webhook secret is missing %q prefix", webhookSecretPrefix)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, webhookSecretPrefix))
	if err != nil {
		return nil, fmt.Errorf("webhook secret is not valid base64: %w", err)
	}
	return key, nil
}

// generateID generates a unique ID for a webhook subscription
func generateWebhookID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")[:32]
}

// validateURL validates the webhook URL
func validateURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URL is required")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("URL must use http or https scheme")
	}

	if parsed.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	return nil
}

// validateEventTypes validates that all event types are valid
func validateEventTypes(eventTypes []string) error {
	if len(eventTypes) == 0 {
		return fmt.Errorf("at least one event type is required")
	}

	validTypes := make(map[string]bool)
	for _, t := range domain.WebhookEventTypes {
		validTypes[t] = true
	}

	for _, t := range eventTypes {
		if !validTypes[t] {
			return fmt.Errorf("invalid event type: %s", t)
		}
	}

	return nil
}

// Create creates a new webhook subscription
func (s *WebhookSubscriptionService) Create(ctx context.Context, workspaceID string, name, webhookURL string, eventTypes []string, customEventFilters *domain.CustomEventFilters) (*domain.WebhookSubscription, error) {
	var err error
	if ctx, err = s.authorize(ctx, workspaceID); err != nil {
		return nil, err
	}

	// Validate inputs
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	if err := validateURL(webhookURL); err != nil {
		return nil, err
	}

	if err := validateEventTypes(eventTypes); err != nil {
		return nil, err
	}

	// Generate secret
	secret, err := generateSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}

	sub := &domain.WebhookSubscription{
		ID:     generateWebhookID(),
		Name:   name,
		URL:    webhookURL,
		Secret: secret,
		Settings: domain.WebhookSubscriptionSettings{
			EventTypes:         eventTypes,
			CustomEventFilters: customEventFilters,
		},
		Enabled: true,
	}

	if err := s.repo.Create(ctx, workspaceID, sub); err != nil {
		return nil, fmt.Errorf("failed to create webhook subscription: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace_id":    workspaceID,
		"subscription_id": sub.ID,
		"event_types":     eventTypes,
	}).Info("Created webhook subscription")

	return sub, nil
}

// GetByID retrieves a webhook subscription by ID
func (s *WebhookSubscriptionService) GetByID(ctx context.Context, workspaceID, id string) (*domain.WebhookSubscription, error) {
	if authCtx, err := s.authorize(ctx, workspaceID); err != nil {
		return nil, err
	} else {
		ctx = authCtx
	}

	sub, err := s.repo.GetByID(ctx, workspaceID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook subscription: %w", err)
	}
	return sub, nil
}

// List retrieves all webhook subscriptions for a workspace
func (s *WebhookSubscriptionService) List(ctx context.Context, workspaceID string) ([]*domain.WebhookSubscription, error) {
	if authCtx, err := s.authorize(ctx, workspaceID); err != nil {
		return nil, err
	} else {
		ctx = authCtx
	}

	subs, err := s.repo.List(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list webhook subscriptions: %w", err)
	}
	return subs, nil
}

// Update updates an existing webhook subscription
func (s *WebhookSubscriptionService) Update(ctx context.Context, workspaceID string, id, name, webhookURL string, eventTypes []string, customEventFilters *domain.CustomEventFilters, enabled bool) (*domain.WebhookSubscription, error) {
	if authCtx, err := s.authorize(ctx, workspaceID); err != nil {
		return nil, err
	} else {
		ctx = authCtx
	}

	// Get existing subscription
	existing, err := s.repo.GetByID(ctx, workspaceID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook subscription: %w", err)
	}

	// Validate inputs
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	if err := validateURL(webhookURL); err != nil {
		return nil, err
	}

	if err := validateEventTypes(eventTypes); err != nil {
		return nil, err
	}

	// Update fields
	existing.Name = name
	existing.URL = webhookURL
	existing.Settings = domain.WebhookSubscriptionSettings{
		EventTypes:         eventTypes,
		CustomEventFilters: customEventFilters,
	}
	existing.Enabled = enabled

	if err := s.repo.Update(ctx, workspaceID, existing); err != nil {
		return nil, fmt.Errorf("failed to update webhook subscription: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace_id":    workspaceID,
		"subscription_id": id,
		"enabled":         enabled,
	}).Info("Updated webhook subscription")

	return existing, nil
}

// Delete deletes a webhook subscription
func (s *WebhookSubscriptionService) Delete(ctx context.Context, workspaceID, id string) error {
	if authCtx, err := s.authorize(ctx, workspaceID); err != nil {
		return err
	} else {
		ctx = authCtx
	}

	if err := s.repo.Delete(ctx, workspaceID, id); err != nil {
		return fmt.Errorf("failed to delete webhook subscription: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace_id":    workspaceID,
		"subscription_id": id,
	}).Info("Deleted webhook subscription")

	return nil
}

// Toggle enables or disables a webhook subscription
func (s *WebhookSubscriptionService) Toggle(ctx context.Context, workspaceID, id string, enabled bool) (*domain.WebhookSubscription, error) {
	if authCtx, err := s.authorize(ctx, workspaceID); err != nil {
		return nil, err
	} else {
		ctx = authCtx
	}

	// Get existing subscription
	existing, err := s.repo.GetByID(ctx, workspaceID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook subscription: %w", err)
	}

	existing.Enabled = enabled

	if err := s.repo.Update(ctx, workspaceID, existing); err != nil {
		return nil, fmt.Errorf("failed to toggle webhook subscription: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace_id":    workspaceID,
		"subscription_id": id,
		"enabled":         enabled,
	}).Info("Toggled webhook subscription")

	return existing, nil
}

// RegenerateSecret generates a new secret for a webhook subscription
func (s *WebhookSubscriptionService) RegenerateSecret(ctx context.Context, workspaceID, id string) (*domain.WebhookSubscription, error) {
	if authCtx, err := s.authorize(ctx, workspaceID); err != nil {
		return nil, err
	} else {
		ctx = authCtx
	}

	// Get existing subscription
	existing, err := s.repo.GetByID(ctx, workspaceID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook subscription: %w", err)
	}

	// Generate new secret
	secret, err := generateSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}

	existing.Secret = secret

	if err := s.repo.Update(ctx, workspaceID, existing); err != nil {
		return nil, fmt.Errorf("failed to regenerate webhook secret: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace_id":    workspaceID,
		"subscription_id": id,
	}).Info("Regenerated webhook secret")

	return existing, nil
}

// GetDeliveries retrieves delivery history, optionally filtered by subscription
func (s *WebhookSubscriptionService) GetDeliveries(ctx context.Context, workspaceID string, subscriptionID *string, limit, offset int) ([]*domain.WebhookDelivery, int, error) {
	if authCtx, err := s.authorize(ctx, workspaceID); err != nil {
		return nil, 0, err
	} else {
		ctx = authCtx
	}

	deliveries, total, err := s.deliveryRepo.ListAll(ctx, workspaceID, subscriptionID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get webhook deliveries: %w", err)
	}
	return deliveries, total, nil
}

// GetEventTypes returns the list of available event types
func (s *WebhookSubscriptionService) GetEventTypes() []string {
	return domain.WebhookEventTypes
}
