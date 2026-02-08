package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/disposable_emails"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// SupabaseService handles Supabase webhook processing
type SupabaseService struct {
	workspaceRepo           domain.WorkspaceRepository
	emailService            domain.EmailServiceInterface
	contactService          domain.ContactService
	listRepo                domain.ListRepository
	contactListRepo         domain.ContactListRepository
	templateRepo            domain.TemplateRepository
	templateService         domain.TemplateService
	transactionalRepo       domain.TransactionalNotificationRepository
	transactionalService    domain.TransactionalNotificationService
	inboundWebhookEventRepo domain.InboundWebhookEventRepository
	logger                  logger.Logger
}

// NewSupabaseService creates a new Supabase service
func NewSupabaseService(
	workspaceRepo domain.WorkspaceRepository,
	emailService domain.EmailServiceInterface,
	contactService domain.ContactService,
	listRepo domain.ListRepository,
	contactListRepo domain.ContactListRepository,
	templateRepo domain.TemplateRepository,
	templateService domain.TemplateService,
	transactionalRepo domain.TransactionalNotificationRepository,
	transactionalService domain.TransactionalNotificationService,
	inboundWebhookEventRepo domain.InboundWebhookEventRepository,
	logger logger.Logger,
) *SupabaseService {
	return &SupabaseService{
		workspaceRepo:           workspaceRepo,
		emailService:            emailService,
		contactService:          contactService,
		listRepo:                listRepo,
		contactListRepo:         contactListRepo,
		templateRepo:            templateRepo,
		templateService:         templateService,
		transactionalRepo:       transactionalRepo,
		transactionalService:    transactionalService,
		inboundWebhookEventRepo: inboundWebhookEventRepo,
		logger:                  logger,
	}
}

// ProcessAuthEmailHook processes a Supabase Send Email Hook webhook
func (s *SupabaseService) ProcessAuthEmailHook(ctx context.Context, workspaceID, integrationID string, payload []byte, webhookID, webhookTimestamp, webhookSignature string) error {
	// Get workspace and integration
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	// Find the integration
	var integration *domain.Integration
	for i := range workspace.Integrations {
		if workspace.Integrations[i].ID == integrationID {
			integration = &workspace.Integrations[i]
			break
		}
	}

	if integration == nil {
		return fmt.Errorf("integration not found: %s", integrationID)
	}

	if integration.Type != domain.IntegrationTypeSupabase {
		return fmt.Errorf("integration is not a Supabase integration")
	}

	if integration.SupabaseSettings == nil {
		return fmt.Errorf("supabase settings not found")
	}

	// Validate webhook signature
	signatureKey := integration.SupabaseSettings.AuthEmailHook.SignatureKey

	// Supabase provides keys in format "v1,whsec_<base64>"
	// Following Supabase's official example, we strip "v1,whsec_" to get just the base64 data
	// Reference: https://supabase.com/docs/guides/auth/auth-hooks/send-email-hook
	if len(signatureKey) > 9 && signatureKey[:9] == "v1,whsec_" {
		signatureKey = signatureKey[9:] // Remove "v1,whsec_" prefix
	}

	if err := domain.ValidateSupabaseWebhookSignature(payload, webhookSignature, webhookTimestamp, webhookID, signatureKey); err != nil {
		s.logger.WithField("error", err.Error()).
			WithField("workspace_id", workspaceID).
			WithField("integration_id", integrationID).
			WithField("signature_key_looks_valid", len(signatureKey) >= 12 && signatureKey[:6] == "whsec_").
			Error("Failed to validate webhook signature")
		return fmt.Errorf("invalid webhook signature: %w", err)
	}

	// Parse webhook payload
	var webhook domain.SupabaseAuthEmailWebhook
	if err := json.Unmarshal(payload, &webhook); err != nil {
		return fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	// Store webhook event for audit trail
	if err := s.storeSupabaseWebhook(ctx, workspaceID, integrationID, webhook.User.Email, string(domain.EmailEventAuthEmail), payload, webhookTimestamp); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to store Supabase webhook event")
		// Don't fail the webhook processing if storage fails
	}

	// Determine email action type
	actionType := domain.SupabaseEmailActionType(webhook.EmailData.EmailActionType)

	// Handle email_change special case (may need to send 2 notifications)
	if actionType == domain.SupabaseEmailActionEmailChange {
		return s.handleEmailChange(ctx, workspaceID, integrationID, &webhook)
	}

	// Get notification ID for this action type by querying transactional notifications with integration_id
	notificationID, err := s.getNotificationIDForAction(ctx, workspaceID, integrationID, actionType)
	if err != nil {
		return fmt.Errorf("failed to get notification for action %s: %w", actionType, err)
	}

	// Prepare template data from webhook
	templateData := s.buildTemplateDataFromAuthWebhook(&webhook)

	// Send transactional notification
	err = s.sendTransactionalNotification(ctx, workspaceID, notificationID, webhook.User.Email, templateData)
	if err != nil {
		return err
	}

	return nil
}

// handleEmailChange handles the email_change action which may require sending 2 emails using the same template
func (s *SupabaseService) handleEmailChange(ctx context.Context, workspaceID, integrationID string, webhook *domain.SupabaseAuthEmailWebhook) error {
	// Get notification ID for email change (single template used for both current and new addresses)
	emailChangeID, err := s.getNotificationIDForAction(ctx, workspaceID, integrationID, domain.SupabaseEmailActionEmailChange)
	if err != nil {
		return fmt.Errorf("failed to get email_change notification: %w", err)
	}

	// Check if we need to send two emails (Secure Email Change mode)
	if webhook.EmailData.TokenHashNew != "" && webhook.EmailData.TokenHash != "" {
		// Send notification to current address
		// Note: Supabase's naming is counterintuitive for backward compatibility:
		// - token_hash_new = Hash(user.email, token)
		// - token_hash = Hash(user.email_new, token_new)
		// See: https://supabase.com/docs/guides/auth/auth-hooks/send-email-hook
		currentEmailData := map[string]interface{}{
			"user":        webhook.User,
			"token":       webhook.EmailData.Token,
			"token_hash":  webhook.EmailData.TokenHashNew, // Use token_hash_new for current email
			"redirect_to": webhook.EmailData.RedirectTo,
			"site_url":    webhook.EmailData.SiteURL,
		}

		if err := s.sendTransactionalNotification(ctx, workspaceID, emailChangeID, webhook.User.Email, currentEmailData); err != nil {
			s.logger.WithField("error", err.Error()).Error("Failed to send email change notification to current address")
			// Continue to send to new address even if this fails
		}

		// Send notification to new address
		newEmailData := map[string]interface{}{
			"user":        webhook.User,
			"token":       webhook.EmailData.TokenNew,
			"token_hash":  webhook.EmailData.TokenHash, // Use token_hash for new email
			"redirect_to": webhook.EmailData.RedirectTo,
			"site_url":    webhook.EmailData.SiteURL,
		}

		return s.sendTransactionalNotification(ctx, workspaceID, emailChangeID, webhook.User.EmailNew, newEmailData)
	}

	// Single email mode - send to new address
	templateData := s.buildTemplateDataFromAuthWebhook(webhook)
	return s.sendTransactionalNotification(ctx, workspaceID, emailChangeID, webhook.User.EmailNew, templateData)
}

// getNotificationIDForAction queries transactional notifications by integration_id and matches by action type
func (s *SupabaseService) getNotificationIDForAction(ctx context.Context, workspaceID, integrationID string, actionType domain.SupabaseEmailActionType) (string, error) {
	// Use system context to bypass authentication
	systemCtx := context.WithValue(ctx, domain.SystemCallKey, true)

	// Query all transactional notifications for this workspace
	notifications, _, err := s.transactionalRepo.List(systemCtx, workspaceID, map[string]interface{}{}, 1000, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get notifications: %w", err)
	}

	// Find notification with matching integration_id and ID prefix
	actionPrefix := fmt.Sprintf("supabase_%s_", actionType)
	for _, notification := range notifications {
		if notification.IntegrationID != nil && *notification.IntegrationID == integrationID {
			// Check if notification ID starts with the action prefix
			if len(notification.ID) > len(actionPrefix) && notification.ID[:len(actionPrefix)] == actionPrefix {
				return notification.ID, nil
			}
		}
	}

	return "", fmt.Errorf("notification not found for action type: %s", actionType)
}

// buildTemplateDataFromAuthWebhook builds template data from auth webhook
func (s *SupabaseService) buildTemplateDataFromAuthWebhook(webhook *domain.SupabaseAuthEmailWebhook) map[string]interface{} {
	return map[string]interface{}{
		"user":       webhook.User,
		"email_data": webhook.EmailData,
	}
}

// sendTransactionalNotification sends a transactional notification
func (s *SupabaseService) sendTransactionalNotification(ctx context.Context, workspaceID, notificationID, toEmail string, data map[string]interface{}) error {
	// Use system context to bypass authentication
	systemCtx := context.WithValue(ctx, domain.SystemCallKey, true)

	// Create send params
	params := domain.TransactionalNotificationSendParams{
		ID: notificationID,
		Contact: &domain.Contact{
			Email: toEmail,
		},
		Data: data,
	}

	// Send the notification
	_, err := s.transactionalService.SendNotification(systemCtx, workspaceID, params)
	if err != nil {
		return fmt.Errorf("failed to send transactional notification: %w", err)
	}

	return nil
}

// ProcessUserCreatedHook processes a Supabase Before User Created Hook webhook
func (s *SupabaseService) ProcessUserCreatedHook(ctx context.Context, workspaceID, integrationID string, payload []byte, webhookID, webhookTimestamp, webhookSignature string) error {
	// Get workspace and integration
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		// Log error but don't fail user creation
		s.logger.WithField("error", err.Error()).Error("Failed to get workspace for user created hook")
		return nil
	}

	// Find the integration
	var integration *domain.Integration
	for i := range workspace.Integrations {
		if workspace.Integrations[i].ID == integrationID {
			integration = &workspace.Integrations[i]
			break
		}
	}

	if integration == nil {
		s.logger.WithField("integration_id", integrationID).Error("Integration not found for user created hook")
		return nil
	}

	if integration.Type != domain.IntegrationTypeSupabase {
		s.logger.Error("Integration is not a Supabase integration")
		return nil
	}

	if integration.SupabaseSettings == nil {
		s.logger.Error("Supabase settings not found")
		return nil
	}

	// Validate webhook signature
	signatureKey := integration.SupabaseSettings.BeforeUserCreatedHook.SignatureKey

	// Supabase provides keys in format "v1,whsec_<base64>"
	// Following Supabase's official example, we strip "v1,whsec_" to get just the base64 data
	// Reference: https://supabase.com/docs/guides/auth/auth-hooks/send-email-hook
	if len(signatureKey) > 9 && signatureKey[:9] == "v1,whsec_" {
		signatureKey = signatureKey[9:] // Remove "v1,whsec_" prefix
	}

	if err := domain.ValidateSupabaseWebhookSignature(payload, webhookSignature, webhookTimestamp, webhookID, signatureKey); err != nil {
		s.logger.WithField("error", err.Error()).
			WithField("workspace_id", workspaceID).
			WithField("integration_id", integrationID).
			WithField("signature_key_looks_valid", len(signatureKey) >= 12 && signatureKey[:6] == "whsec_").
			Error("Failed to validate webhook signature for before user created hook")
		return nil // Don't fail user creation
	}

	// Parse webhook payload
	var webhook domain.SupabaseBeforeUserCreatedWebhook
	if err := json.Unmarshal(payload, &webhook); err != nil {
		s.logger.WithField("error", err.Error()).Error("Failed to parse before user created webhook payload")
		return nil // Don't fail user creation
	}

	// Store webhook event for audit trail
	if err := s.storeSupabaseWebhook(ctx, workspaceID, integrationID, webhook.User.Email, string(domain.EmailEventBeforeUserCreated), payload, webhookTimestamp); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to store Supabase webhook event")
		// Don't fail the webhook processing if storage fails
	}

	// Check for disposable email if RejectDisposableEmail is enabled
	if integration.SupabaseSettings.BeforeUserCreatedHook.RejectDisposableEmail {
		// Extract domain from email
		emailParts := strings.Split(webhook.User.Email, "@")
		if len(emailParts) == 2 {
			domain := strings.ToLower(emailParts[1])
			if disposable_emails.IsDisposableEmail(domain) {
				s.logger.WithField("email", webhook.User.Email).Info("Rejected user creation: disposable email detected")
				return fmt.Errorf("disposable email addresses are not allowed")
			}
		}
	}

	// Convert Supabase user to Notifuse contact
	// Get the custom_json_field setting (optional - if not set, user_metadata won't be mapped)
	customJSONField := integration.SupabaseSettings.BeforeUserCreatedHook.CustomJSONField

	contact, err := webhook.User.ToContact(customJSONField)
	if err != nil {
		s.logger.WithField("error", err.Error()).Error("Failed to convert Supabase user to contact")
		return nil // Don't fail user creation
	}

	// Use system context to bypass authentication
	systemCtx := context.WithValue(ctx, domain.SystemCallKey, true)

	// Upsert contact
	operation := s.contactService.UpsertContact(systemCtx, workspaceID, contact)
	if operation.Action == domain.UpsertContactOperationError {
		s.logger.WithField("error", operation.Error).Error("Failed to upsert contact from Supabase user")
		return nil // Don't fail user creation
	}

	// Add to lists if configured
	targetListIDs := integration.SupabaseSettings.BeforeUserCreatedHook.AddUserToLists
	for _, targetListID := range targetListIDs {
		if targetListID != "" {
			// Check if list exists
			list, err := s.listRepo.GetListByID(systemCtx, workspaceID, targetListID)
			if err != nil {
				s.logger.WithField("list_id", targetListID).WithField("error", err.Error()).Info("Target list not found or error accessing it, skipping this list")
				continue // Skip this list and try the next one
			}

			// Add contact to list
			contactList := &domain.ContactList{
				Email:     contact.Email,
				ListID:    targetListID,
				ListName:  list.Name,
				Status:    domain.ContactListStatusActive,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}

			err = s.contactListRepo.AddContactToList(systemCtx, workspaceID, contactList)
			if err != nil {
				s.logger.WithField("list_id", targetListID).WithField("error", err.Error()).Error("Failed to add contact to list")
				// Continue with next list - contact was created successfully
			}
		}
	}

	return nil // Always return nil to not block user creation
}

// DeleteIntegrationResources deletes all templates and transactional notifications associated with a Supabase integration
// This is called when the integration is being deleted from the workspace
func (s *SupabaseService) DeleteIntegrationResources(ctx context.Context, workspaceID, integrationID string) error {
	// Use system context to bypass authentication
	systemCtx := context.WithValue(ctx, domain.SystemCallKey, true)

	// Get all templates
	templates, err := s.templateRepo.GetTemplates(systemCtx, workspaceID, "", "")
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).
			WithField("integration_id", integrationID).
			WithField("error", err.Error()).
			Error("Failed to list templates for Supabase integration cleanup")
		return fmt.Errorf("failed to list templates: %w", err)
	}

	// Delete templates associated with this integration
	templatesDeleted := 0
	for _, template := range templates {
		if template.IntegrationID != nil && *template.IntegrationID == integrationID {
			// Force delete using repository directly (bypasses service protection)
			err := s.templateRepo.DeleteTemplate(systemCtx, workspaceID, template.ID)
			if err != nil {
				s.logger.WithField("workspace_id", workspaceID).
					WithField("integration_id", integrationID).
					WithField("template_id", template.ID).
					WithField("error", err.Error()).
					Warn("Failed to delete template during Supabase integration cleanup")
			} else {
				templatesDeleted++
			}
		}
	}

	s.logger.WithField("workspace_id", workspaceID).
		WithField("integration_id", integrationID).
		WithField("templates_deleted", templatesDeleted).
		Info("Deleted templates for Supabase integration")

	// Get all transactional notifications
	notifications, _, err := s.transactionalRepo.List(systemCtx, workspaceID, map[string]interface{}{}, 1000, 0)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).
			WithField("integration_id", integrationID).
			WithField("error", err.Error()).
			Error("Failed to list transactional notifications for Supabase integration cleanup")
		return fmt.Errorf("failed to list transactional notifications: %w", err)
	}

	// Delete transactional notifications associated with this integration
	notificationsDeleted := 0
	for _, notification := range notifications {
		if notification.IntegrationID != nil && *notification.IntegrationID == integrationID {
			// Force delete using repository directly (bypasses service protection)
			err := s.transactionalRepo.Delete(systemCtx, workspaceID, notification.ID)
			if err != nil {
				s.logger.WithField("workspace_id", workspaceID).
					WithField("integration_id", integrationID).
					WithField("notification_id", notification.ID).
					WithField("error", err.Error()).
					Warn("Failed to delete transactional notification during Supabase integration cleanup")
			} else {
				notificationsDeleted++
			}
		}
	}

	s.logger.WithField("workspace_id", workspaceID).
		WithField("integration_id", integrationID).
		WithField("notifications_deleted", notificationsDeleted).
		Info("Deleted transactional notifications for Supabase integration")

	return nil
}

// storeSupabaseWebhook stores a Supabase webhook event for audit trail
func (s *SupabaseService) storeSupabaseWebhook(ctx context.Context, workspaceID, integrationID, recipientEmail, eventType string, payload []byte, webhookTimestamp string) error {
	// Parse timestamp
	timestamp, err := time.Parse(time.RFC3339, webhookTimestamp)
	if err != nil {
		timestamp = time.Now().UTC()
	}

	// Create inbound webhook event
	event := &domain.InboundWebhookEvent{
		ID:             uuid.New().String(),
		Type:           domain.EmailEventType(eventType),
		Source:         domain.WebhookSourceSupabase,
		IntegrationID:  integrationID,
		RecipientEmail: recipientEmail,
		MessageID:      nil, // Supabase webhooks don't have message_id
		Timestamp:      timestamp,
		RawPayload:     string(payload),
		CreatedAt:      time.Now().UTC(),
	}

	// Store via repository
	return s.inboundWebhookEventRepo.StoreEvents(ctx, workspaceID, []*domain.InboundWebhookEvent{event})
}
