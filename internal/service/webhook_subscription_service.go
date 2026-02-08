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

// WebhookSubscriptionService handles webhook subscription business logic
type WebhookSubscriptionService struct {
	repo         domain.WebhookSubscriptionRepository
	deliveryRepo domain.WebhookDeliveryRepository
	authService  *AuthService
	logger       logger.Logger
}

// NewWebhookSubscriptionService creates a new webhook subscription service
func NewWebhookSubscriptionService(
	repo domain.WebhookSubscriptionRepository,
	deliveryRepo domain.WebhookDeliveryRepository,
	authService *AuthService,
	logger logger.Logger,
) *WebhookSubscriptionService {
	return &WebhookSubscriptionService{
		repo:         repo,
		deliveryRepo: deliveryRepo,
		authService:  authService,
		logger:       logger,
	}
}

// generateSecret generates a secure random secret for webhook signing
func generateSecret() (string, error) {
	bytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
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
	sub, err := s.repo.GetByID(ctx, workspaceID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook subscription: %w", err)
	}
	return sub, nil
}

// List retrieves all webhook subscriptions for a workspace
func (s *WebhookSubscriptionService) List(ctx context.Context, workspaceID string) ([]*domain.WebhookSubscription, error) {
	subs, err := s.repo.List(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list webhook subscriptions: %w", err)
	}
	return subs, nil
}

// Update updates an existing webhook subscription
func (s *WebhookSubscriptionService) Update(ctx context.Context, workspaceID string, id, name, webhookURL string, eventTypes []string, customEventFilters *domain.CustomEventFilters, enabled bool) (*domain.WebhookSubscription, error) {
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
