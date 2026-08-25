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

// authorize confirms the caller is a member of the workspace they named and holds
// the webhook subscriptions permission at the requested level.
//
// INVARIANT: every method here takes workspaceID straight from the request, and
// must call this before touching a repository.
//
// Isolation is per-database, but opening a workspace database does not itself
// establish any right to it — workspaceID selects a database and asserts nothing
// more. This is what establishes the right.
//
// Read for the methods that only look, write for the ones that create, change,
// remove or fire a subscription. The permission is orthogonal to the owner check
// below and does not replace it: webhook_subscriptions:read reads subscriptions,
// it never hands out a signing secret.
func (s *WebhookSubscriptionService) authorize(ctx context.Context, workspaceID string, write bool) (context.Context, error) {
	ctx, _, err := s.authorizeWithRole(ctx, workspaceID, write)
	return ctx, err
}

// authorizeWithRole is authorize, additionally reporting whether the caller owns
// the workspace. Only the methods that return subscriptions need that: the flag
// is what redactSecret uses to decide whether to blank the signing secret.
func (s *WebhookSubscriptionService) authorizeWithRole(ctx context.Context, workspaceID string, write bool) (context.Context, bool, error) {
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return ctx, false, fmt.Errorf("failed to authenticate user: %w", err)
	}
	// No membership means no grant. Skipping the check here instead would be a
	// silent bypass rather than a nil-pointer guard.
	if userWorkspace == nil {
		return ctx, false, fmt.Errorf("failed to authenticate user: no workspace membership")
	}

	permission := domain.PermissionTypeRead
	access := "read"
	if write {
		permission = domain.PermissionTypeWrite
		access = "write"
	}

	if !userWorkspace.HasPermission(domain.PermissionResourceWebhookSubscriptions, permission) {
		return ctx, false, domain.NewPermissionError(
			domain.PermissionResourceWebhookSubscriptions,
			permission,
			fmt.Sprintf("Insufficient permissions: %s access to webhook subscriptions required", access),
		)
	}

	return ctx, userWorkspace.Role == "owner", nil
}

// redactSecret blanks a subscription's signing secret for a non-owner.
//
// In place, on the objects the repository just built: they are freshly scanned
// per call and shared with nothing, so there is no cached instance to corrupt.
//
// Every method that hands a subscription back to a handler has to make a
// deliberate decision about this call, because the handlers serialise the whole
// object and Secret carries no omitempty. Redacting only the two read methods
// left `.toggle` as a no-op route to any subscription's plaintext signing key
// for anyone holding webhook_subscriptions:write — against this file's own
// stated rule that a secret is owner-only key material. The exceptions are
// narrow and each is argued at its call site: Create and RegenerateSecret mint a
// brand-new secret and are the one moment it can be copied, and
// GetForTestDelivery is an internal accessor whose result is never serialised.
func redactSecret(sub *domain.WebhookSubscription, isOwner bool) {
	if sub != nil && !isOwner {
		sub.Secret = ""
	}
}

// authorizeOwner is authorize plus the role check, for the operations that
// involve a signing secret.
//
// A webhook secret is a key, not ordinary workspace data: whoever holds one can
// forge payloads the customer's downstream consumer will accept as genuine. That
// puts it with integrations and API keys, which are already owner-only
// (workspace_service.go:324, 457, 778), rather than under the read/write
// permission model used for contacts or lists.
func (s *WebhookSubscriptionService) authorizeOwner(ctx context.Context, workspaceID string) (context.Context, bool, error) {
	ctx, user, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return ctx, false, fmt.Errorf("failed to authenticate user: %w", err)
	}
	if userWorkspace == nil {
		return ctx, false, fmt.Errorf("failed to authenticate user: no workspace membership")
	}
	_ = user
	return ctx, userWorkspace.Role == "owner", nil
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

// Create creates a new webhook subscription.
//
// source attributes the subscription to whatever created it and is write-once:
// this is the only method that sets it, because nothing else records who asked
// for the row and an attribution that can be changed later is not an
// attribution. listIDs and segmentIDs narrow the fan-out for the list.* and
// segment.* events; nil or empty means no filter.
func (s *WebhookSubscriptionService) Create(ctx context.Context, workspaceID string, name, webhookURL string, eventTypes []string, customEventFilters *domain.CustomEventFilters, source string, listIDs, segmentIDs []string) (*domain.WebhookSubscription, error) {
	var err error
	if ctx, err = s.authorize(ctx, workspaceID, true); err != nil {
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

	// Re-checked here even though the handler already rejects it: the column is
	// write-once, so a bad value that reaches the repository can never be
	// corrected through the API, and this service is reachable from any future
	// in-process caller that has no HTTP layer in front of it.
	if err := domain.ValidateWebhookSubscriptionSource(source); err != nil {
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
			ListIDs:            listIDs,
			SegmentIDs:         segmentIDs,
		},
		Enabled: true,
		Source:  source,
	}

	if err := s.repo.Create(ctx, workspaceID, sub); err != nil {
		return nil, fmt.Errorf("failed to create webhook subscription: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace_id":    workspaceID,
		"subscription_id": sub.ID,
		"event_types":     eventTypes,
	}).Info("Created webhook subscription")

	// Deliberately not redacted, for whoever created it. The secret was minted a
	// few lines up for a row that did not exist a moment ago, so returning it
	// discloses nothing about any other subscription — and this response is the
	// only place it is ever readable by a non-owner. Blanking it here would leave
	// a member able to create a webhook and unable to configure its receiver.
	return sub, nil
}

// GetByID retrieves a webhook subscription by ID
func (s *WebhookSubscriptionService) GetByID(ctx context.Context, workspaceID, id string) (*domain.WebhookSubscription, error) {
	ctx, isOwner, err := s.authorizeWithRole(ctx, workspaceID, false)
	if err != nil {
		return nil, err
	}

	sub, err := s.repo.GetByID(ctx, workspaceID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook subscription: %w", err)
	}
	redactSecret(sub, isOwner)
	return sub, nil
}

// List retrieves all webhook subscriptions for a workspace
func (s *WebhookSubscriptionService) List(ctx context.Context, workspaceID string) ([]*domain.WebhookSubscription, error) {
	ctx, isOwner, err := s.authorizeWithRole(ctx, workspaceID, false)
	if err != nil {
		return nil, err
	}

	subs, err := s.repo.List(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list webhook subscriptions: %w", err)
	}
	for _, sub := range subs {
		redactSecret(sub, isOwner)
	}
	return subs, nil
}

// Update updates an existing webhook subscription.
//
// There is deliberately no source parameter. Update is a full replace of
// everything a user can edit, and attribution is not one of those things: a
// caller able to rewrite it could take over a Zapier-created subscription, or
// disown its own, and the console badge and the delete-versus-disable branch
// would follow the lie. The stored value survives because it is only ever read
// back off the existing row.
func (s *WebhookSubscriptionService) Update(ctx context.Context, workspaceID string, id, name, webhookURL string, eventTypes []string, customEventFilters *domain.CustomEventFilters, enabled bool, listIDs, segmentIDs []string) (*domain.WebhookSubscription, error) {
	ctx, isOwner, err := s.authorizeWithRole(ctx, workspaceID, true)
	if err != nil {
		return nil, err
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
		ListIDs:            listIDs,
		SegmentIDs:         segmentIDs,
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

	// The row round-tripped through the repository still carries the stored
	// secret, and the handler writes this object straight to the client. Editing
	// a name must not be a way to read a key.
	redactSecret(existing, isOwner)
	return existing, nil
}

// Delete deletes a webhook subscription
func (s *WebhookSubscriptionService) Delete(ctx context.Context, workspaceID, id string) error {
	if authCtx, err := s.authorize(ctx, workspaceID, true); err != nil {
		return err
	} else {
		ctx = authCtx
	}

	// Queued deliveries go first, and a failure here aborts the whole delete.
	//
	// A delivery row whose subscription no longer exists keeps matching the
	// worker's pending predicate for the full retention window while it can
	// never be sent, so it occupies a slot in every batch until it ages out —
	// one abandoned subscription is a permanent head-of-line block. Deleting
	// them first means the subscription row never disappears while its queue
	// survives; if the sweep fails, the caller gets an error and the
	// subscription is still there to retry against. The deliveries being
	// discarded were bound for an endpoint the caller is deleting anyway.
	if err := s.deliveryRepo.DeleteBySubscriptionID(ctx, workspaceID, id); err != nil {
		return fmt.Errorf("failed to delete webhook deliveries: %w", err)
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
	ctx, isOwner, err := s.authorizeWithRole(ctx, workspaceID, true)
	if err != nil {
		return nil, err
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

	// Toggling is the cheapest possible write, which is exactly what made it the
	// easiest route to somebody else's signing key: flip a subscription off and
	// on and read the secret out of the response.
	redactSecret(existing, isOwner)
	return existing, nil
}

// RegenerateSecret generates a new secret for a webhook subscription
func (s *WebhookSubscriptionService) RegenerateSecret(ctx context.Context, workspaceID, id string) (*domain.WebhookSubscription, error) {
	ctx, isOwner, err := s.authorizeOwner(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	// Gating the read alone would be theatre: a member could rotate a secret to
	// learn it — and in doing so break the customer's live integration.
	if !isOwner {
		return nil, &domain.ErrUnauthorized{Message: "only a workspace owner may regenerate a webhook secret"}
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

	// Deliberately not redacted. Only an owner reaches this line — the guard
	// above rejects everyone else before the repository is touched — and the
	// point of rotating a secret is to receive the new one so the receiver can
	// be reconfigured. This response is the only place it is ever shown.
	return existing, nil
}

// GetDeliveries retrieves delivery history, optionally filtered by subscription
func (s *WebhookSubscriptionService) GetDeliveries(ctx context.Context, workspaceID string, subscriptionID *string, limit, offset int) ([]*domain.WebhookDelivery, int, error) {
	if authCtx, err := s.authorize(ctx, workspaceID, false); err != nil {
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

// GetForTestDelivery returns a subscription for a test delivery, signing secret
// included so the caller can sign the test payload. The secret goes to the
// subscription's own URL in a signature header and must not be written back to
// the client — no response body built from this value may carry it.
//
// Write, not read: a test fires a real outbound request at a caller-chosen host.
// /api/webhookSubscriptions.test used to authorize by calling GetByID, so under
// the read/write split a read-only key could have triggered arbitrary deliveries.
func (s *WebhookSubscriptionService) GetForTestDelivery(ctx context.Context, workspaceID, id string) (*domain.WebhookSubscription, error) {
	ctx, err := s.authorize(ctx, workspaceID, true)
	if err != nil {
		return nil, err
	}

	sub, err := s.repo.GetByID(ctx, workspaceID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook subscription: %w", err)
	}
	// Deliberately not redacted, and the one method here where that is not a
	// disclosure: the caller signs the outbound test request with this value and
	// answers the client with the receiver's status code and body, never with
	// the subscription object. Redacting it would not protect anything — it
	// would only make every test delivery fail to sign for a non-owner.
	return sub, nil
}

// GetEventTypes returns the list of available event types.
//
// Ungated: it takes no context and no workspace because it has neither — the list
// is a package constant, identical for every caller, and reveals nothing about any
// tenant. Authentication at the route is the whole of the access control here.
func (s *WebhookSubscriptionService) GetEventTypes() []string {
	return domain.WebhookEventTypes
}
