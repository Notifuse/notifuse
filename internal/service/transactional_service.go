package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/Notifuse/notifuse/pkg/tracing"
	"go.opencensus.io/trace"
)

// TransactionalNotificationService provides operations for managing and sending transactional notifications
type TransactionalNotificationService struct {
	transactionalRepo  domain.TransactionalNotificationRepository
	messageHistoryRepo domain.MessageHistoryRepository
	templateService    domain.TemplateService
	contactService     domain.ContactService
	emailService       domain.EmailServiceInterface
	authService        domain.AuthService
	logger             logger.Logger
	workspaceRepo      domain.WorkspaceRepository
	apiEndpoint        string
}

// NewTransactionalNotificationService creates a new instance of the transactional notification service
func NewTransactionalNotificationService(
	transactionalRepo domain.TransactionalNotificationRepository,
	messageHistoryRepo domain.MessageHistoryRepository,
	templateService domain.TemplateService,
	contactService domain.ContactService,
	emailService domain.EmailServiceInterface,
	authService domain.AuthService,
	logger logger.Logger,
	workspaceRepo domain.WorkspaceRepository,
	apiEndpoint string,
) *TransactionalNotificationService {
	return &TransactionalNotificationService{
		transactionalRepo:  transactionalRepo,
		messageHistoryRepo: messageHistoryRepo,
		templateService:    templateService,
		contactService:     contactService,
		emailService:       emailService,
		authService:        authService,
		logger:             logger,
		workspaceRepo:      workspaceRepo,
		apiEndpoint:        apiEndpoint,
	}
}

// CreateNotification creates a new transactional notification
func (s *TransactionalNotificationService) CreateNotification(
	ctx context.Context,
	workspace string,
	params domain.TransactionalNotificationCreateParams,
) (*domain.TransactionalNotification, error) {
	ctx, span := tracing.StartServiceSpan(ctx, "TransactionalNotificationService", "CreateNotification")
	defer span.End()

	span.AddAttributes(
		trace.StringAttribute("workspace", workspace),
		trace.StringAttribute("notification_id", params.ID),
		trace.StringAttribute("notification_name", params.Name),
	)

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user for workspace: %w", err)
	}

	// Check permission for writing transactional notifications
	if !userWorkspace.HasPermission(domain.PermissionResourceTransactional, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceTransactional,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to transactional notifications required",
		)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace": workspace,
		"id":        params.ID,
		"name":      params.Name,
	}).Debug("Creating new transactional notification")

	// Create the notification object
	notification := &domain.TransactionalNotification{
		ID:               params.ID,
		Name:             params.Name,
		Description:      params.Description,
		Channels:         params.Channels,
		TrackingSettings: params.TrackingSettings,
		Metadata:         params.Metadata,
	}

	// Validate the notification templates exist
	for channel, template := range notification.Channels {
		tracing.AddAttribute(ctx, fmt.Sprintf("channel.%s.template_id", channel), template.TemplateID)

		_, err := s.templateService.GetTemplateByID(ctx, workspace, template.TemplateID, 0)
		if err != nil {
			s.logger.WithFields(map[string]interface{}{
				"error":       err.Error(),
				"channel":     channel,
				"template_id": template.TemplateID,
			}).Error("Invalid template for channel")

			tracing.MarkSpanError(ctx, err)
			return nil, fmt.Errorf("invalid template for channel %s: %w", channel, err)
		}
	}

	// Save the notification to the repository
	if err := s.transactionalRepo.Create(ctx, workspace, notification); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":     err.Error(),
			"workspace": workspace,
			"id":        notification.ID,
		}).Error("Failed to create notification")

		tracing.MarkSpanError(ctx, err)
		return nil, fmt.Errorf("failed to create notification: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace": workspace,
		"id":        notification.ID,
		"name":      notification.Name,
	}).Info("Transactional notification created successfully")
	return notification, nil
}

// UpdateNotification updates an existing transactional notification
func (s *TransactionalNotificationService) UpdateNotification(
	ctx context.Context,
	workspace, id string,
	params domain.TransactionalNotificationUpdateParams,
) (*domain.TransactionalNotification, error) {
	ctx, span := tracing.StartServiceSpan(ctx, "TransactionalNotificationService", "UpdateNotification")
	defer span.End()

	span.AddAttributes(
		trace.StringAttribute("workspace", workspace),
		trace.StringAttribute("notification_id", id),
	)

	// Authenticate user for workspace
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user for workspace: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace": workspace,
		"id":        id,
	}).Debug("Updating transactional notification")

	// Get the existing notification
	notification, err := s.transactionalRepo.Get(ctx, workspace, id)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":     err.Error(),
			"workspace": workspace,
			"id":        id,
		}).Error("Failed to get notification for update")

		tracing.MarkSpanError(ctx, err)
		return nil, fmt.Errorf("failed to get notification: %w", err)
	}

	// Update fields if provided
	if params.Name != "" {
		notification.Name = params.Name
	}
	if params.Description != "" {
		notification.Description = params.Description
	}
	// Validate the updated templates exist
	if params.Channels != nil {
		for channel, template := range params.Channels {
			tracing.AddAttribute(ctx, fmt.Sprintf("channel.%s.template_id", channel), template.TemplateID)

			_, err := s.templateService.GetTemplateByID(ctx, workspace, template.TemplateID, int64(0))
			if err != nil {
				s.logger.WithFields(map[string]interface{}{
					"error":       err.Error(),
					"channel":     channel,
					"template_id": template.TemplateID,
				}).Error("Invalid template for channel in update")

				tracing.MarkSpanError(ctx, err)
				return nil, fmt.Errorf("invalid template for channel %s: %w", channel, err)
			}
		}
	}
	notification.Channels = params.Channels
	notification.TrackingSettings = params.TrackingSettings
	notification.Metadata = params.Metadata

	// Save the updated notification
	if err := s.transactionalRepo.Update(ctx, workspace, notification); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":     err.Error(),
			"workspace": workspace,
			"id":        notification.ID,
		}).Error("Failed to update notification")

		tracing.MarkSpanError(ctx, err)
		return nil, fmt.Errorf("failed to update notification: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace": workspace,
		"id":        notification.ID,
	}).Info("Transactional notification updated successfully")
	return notification, nil
}

// GetNotification retrieves a transactional notification by ID
func (s *TransactionalNotificationService) GetNotification(
	ctx context.Context,
	workspace, id string,
) (*domain.TransactionalNotification, error) {
	ctx, span := tracing.StartServiceSpan(ctx, "TransactionalNotificationService", "GetNotification")
	defer span.End()

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user for workspace: %w", err)
	}

	// Check permission for reading transactional notifications
	if !userWorkspace.HasPermission(domain.PermissionResourceTransactional, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceTransactional,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to transactional notifications required",
		)
	}

	span.AddAttributes(
		trace.StringAttribute("workspace", workspace),
		trace.StringAttribute("notification_id", id),
	)

	s.logger.WithFields(map[string]interface{}{
		"workspace": workspace,
		"id":        id,
	}).Debug("Retrieving transactional notification")

	notification, err := s.transactionalRepo.Get(ctx, workspace, id)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":     err.Error(),
			"workspace": workspace,
			"id":        id,
		}).Error("Failed to get notification")

		tracing.MarkSpanError(ctx, err)
		return nil, fmt.Errorf("failed to get notification: %w", err)
	}

	// Add notification details to span
	span.AddAttributes(
		trace.StringAttribute("notification.name", notification.Name),
		trace.Int64Attribute("notification.channels_count", int64(len(notification.Channels))),
	)

	return notification, nil
}

// ListNotifications retrieves all transactional notifications with optional filtering
func (s *TransactionalNotificationService) ListNotifications(
	ctx context.Context,
	workspace string,
	filter map[string]interface{},
	limit, offset int,
) ([]*domain.TransactionalNotification, int, error) {
	ctx, span := tracing.StartServiceSpan(ctx, "TransactionalNotificationService", "ListNotifications")
	defer span.End()

	// Authenticate user for workspace
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspace)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to authenticate user for workspace: %w", err)
	}

	span.AddAttributes(
		trace.StringAttribute("workspace", workspace),
		trace.Int64Attribute("limit", int64(limit)),
		trace.Int64Attribute("offset", int64(offset)),
	)

	// Add filter keys to span
	if filter != nil {
		filterKeys := make([]string, 0, len(filter))
		for k := range filter {
			filterKeys = append(filterKeys, k)
		}
		tracing.AddAttribute(ctx, "filter.keys", filterKeys)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace": workspace,
		"limit":     limit,
		"offset":    offset,
		"filter":    filter,
	}).Debug("Listing transactional notifications")

	notifications, total, err := s.transactionalRepo.List(ctx, workspace, filter, limit, offset)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":     err.Error(),
			"workspace": workspace,
		}).Error("Failed to list notifications")

		tracing.MarkSpanError(ctx, err)
		return nil, 0, fmt.Errorf("failed to list notifications: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace": workspace,
		"count":     len(notifications),
		"total":     total,
	}).Debug("Successfully retrieved notifications list")

	span.AddAttributes(
		trace.Int64Attribute("result.count", int64(len(notifications))),
		trace.Int64Attribute("result.total", int64(total)),
	)

	return notifications, total, nil
}

// DeleteNotification soft-deletes a transactional notification
func (s *TransactionalNotificationService) DeleteNotification(
	ctx context.Context,
	workspace, id string,
) error {
	ctx, span := tracing.StartServiceSpan(ctx, "TransactionalNotificationService", "DeleteNotification")
	defer span.End()

	span.AddAttributes(
		trace.StringAttribute("workspace", workspace),
		trace.StringAttribute("notification_id", id),
	)

	// Authenticate user for workspace
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspace)
	if err != nil {
		return fmt.Errorf("failed to authenticate user for workspace: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace": workspace,
		"id":        id,
	}).Debug("Deleting transactional notification")

	if err := s.transactionalRepo.Delete(ctx, workspace, id); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":     err.Error(),
			"workspace": workspace,
			"id":        id,
		}).Error("Failed to delete notification")

		tracing.MarkSpanError(ctx, err)
		return fmt.Errorf("failed to delete notification: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace": workspace,
		"id":        id,
	}).Info("Transactional notification deleted successfully")
	return nil
}

// SendNotification sends a transactional notification to a contact
func (s *TransactionalNotificationService) SendNotification(
	ctx context.Context,
	workspaceID string,
	params domain.TransactionalNotificationSendParams,
) (string, error) {
	ctx, span := tracing.StartServiceSpan(ctx, "TransactionalNotificationService", "SendNotification")
	defer span.End()

	span.AddAttributes(
		trace.StringAttribute("workspace", workspaceID),
		trace.StringAttribute("notification_id", params.ID),
	)

	// Authenticate user for workspace
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("failed to authenticate user for workspace: %w", err)
	}

	// Add contact info to span if available
	if params.Contact != nil {
		span.AddAttributes(
			trace.StringAttribute("contact.email", params.Contact.Email),
		)
	}

	// Add channel info to span
	if len(params.Channels) > 0 {
		channelList := make([]string, 0, len(params.Channels))
		for _, ch := range params.Channels {
			channelList = append(channelList, string(ch))
		}
		tracing.AddAttribute(ctx, "channels", channelList)
	}

	// Get the workspace to retrieve email provider settings
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		return "", fmt.Errorf("failed to get workspace: %w", err)
	}

	// Get the notification
	notification, err := s.transactionalRepo.Get(ctx, workspaceID, params.ID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		return "", fmt.Errorf("notification not found: %w", err)
	}

	span.AddAttributes(
		trace.StringAttribute("notification.name", notification.Name),
	)

	// Upsert the contact first
	if params.Contact == nil {
		err := fmt.Errorf("contact is required")
		tracing.MarkSpanError(ctx, err)
		return "", err
	}

	contactOperation := s.contactService.UpsertContact(ctx, workspaceID, params.Contact)
	if contactOperation.Action == domain.UpsertContactOperationError {
		err := fmt.Errorf("failed to upsert contact: %s", contactOperation.Error)
		tracing.MarkSpanError(ctx, err)
		return "", err
	}

	tracing.AddAttribute(ctx, "contact.operation", string(contactOperation.Action))

	// Get the contact with complete information
	contact, err := s.contactService.GetContactByEmail(ctx, workspaceID, params.Contact.Email)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		return "", fmt.Errorf("contact not found after upsert: %w", err)
	}

	// Determine which channels to send through
	channelsToSend := make(map[domain.TransactionalChannel]struct{})
	if len(params.Channels) > 0 {
		// Use the specified channels
		for _, channel := range params.Channels {
			if _, ok := notification.Channels[channel]; ok {
				channelsToSend[channel] = struct{}{}
			}
		}
	} else {
		// Use all configured channels
		for channel := range notification.Channels {
			channelsToSend[channel] = struct{}{}
		}
	}

	if len(channelsToSend) == 0 {
		err := fmt.Errorf("no valid channels to send notification")
		tracing.MarkSpanError(ctx, err)
		return "", err
	}

	// Create message history entry
	messageID := uuid.New().String()

	// Check for idempotency if external_id is provided
	if params.ExternalID != nil && *params.ExternalID != "" {
		existingMessage, err := s.messageHistoryRepo.GetByExternalID(ctx, workspaceID, *params.ExternalID)
		if err == nil && existingMessage != nil {
			// Message with this external_id already exists, return success with existing message ID
			s.logger.WithFields(map[string]interface{}{
				"workspace":   workspaceID,
				"external_id": *params.ExternalID,
				"message_id":  existingMessage.ID,
			}).Info("Message with external_id already exists, returning existing message")

			span.AddAttributes(
				trace.StringAttribute("existing_message_id", existingMessage.ID),
				trace.BoolAttribute("idempotent_response", true),
			)

			return existingMessage.ID, nil
		}
		// If error is not "not found", it's a real error
		if err != nil && !strings.Contains(err.Error(), "not found") {
			tracing.MarkSpanError(ctx, err)
			return "", fmt.Errorf("failed to check for existing message: %w", err)
		}
	}

	successfulChannels := 0

	span.AddAttributes(
		trace.StringAttribute("message_id", messageID),
		trace.Int64Attribute("channels_to_send", int64(len(channelsToSend))),
	)

	// Process each channel
	for channel := range channelsToSend {
		templateConfig := notification.Channels[channel]

		childCtx, childSpan := tracing.StartSpan(ctx, fmt.Sprintf("Send.%s", channel))
		childSpan.AddAttributes(
			trace.StringAttribute("channel", string(channel)),
			trace.StringAttribute("template_id", templateConfig.TemplateID),
		)

		// Prepare message data with contact and custom data
		notification.TrackingSettings.EnableTracking = workspace.Settings.EmailTrackingEnabled

		// Use workspace CustomEndpointURL if provided, otherwise use the default API endpoint
		if workspace.Settings.CustomEndpointURL != nil && *workspace.Settings.CustomEndpointURL != "" {
			notification.TrackingSettings.Endpoint = *workspace.Settings.CustomEndpointURL
		} else {
			notification.TrackingSettings.Endpoint = s.apiEndpoint
		}

		notification.TrackingSettings.WorkspaceID = workspaceID
		notification.TrackingSettings.MessageID = messageID

		contactWithList := domain.ContactWithList{
			Contact: contact,
		}

		req := domain.TemplateDataRequest{
			WorkspaceID:        workspace.ID,
			WorkspaceSecretKey: workspace.Settings.SecretKey,
			ContactWithList:    contactWithList,
			MessageID:          messageID,
			ProvidedData:       params.Data,
			TrackingSettings:   notification.TrackingSettings,
			Broadcast:          nil,
		}
		templateData, err := domain.BuildTemplateData(req)
		if err != nil {
			tracing.MarkSpanError(childCtx, err)
			childSpan.End()
			return "", fmt.Errorf("failed to build template data: %w", err)
		}

		messageData := domain.MessageData{
			Data: templateData,
		}

		// Add metadata if provided
		if params.Metadata != nil {
			messageData.Metadata = params.Metadata
		}

		// Send the message based on channel type
		if channel == domain.TransactionalChannelEmail {

			// Get the email provider and integration ID using the workspace's GetEmailProviderWithIntegrationID method
			emailProvider, integrationID, err := workspace.GetEmailProviderWithIntegrationID(false)
			if err != nil {
				tracing.MarkSpanError(childCtx, err)
				childSpan.End()
				return "", err
			}

			// Validate that the provider is configured
			if emailProvider == nil || emailProvider.Kind == "" {
				err := fmt.Errorf("no email provider configured for transactional notifications")
				tracing.MarkSpanError(childCtx, err)
				childSpan.End()
				return "", err
			}

			childSpan.AddAttributes(
				trace.StringAttribute("provider.kind", string(emailProvider.Kind)),
				trace.StringAttribute("integration_id", integrationID),
			)

			notification.TrackingSettings.EnableTracking = workspace.Settings.EmailTrackingEnabled

			request := domain.SendEmailRequest{
				WorkspaceID:      workspaceID,
				IntegrationID:    integrationID,
				MessageID:        messageID,
				ExternalID:       params.ExternalID,
				Contact:          contact,
				TemplateConfig:   templateConfig,
				MessageData:      messageData,
				TrackingSettings: notification.TrackingSettings,
				EmailProvider:    emailProvider,
				EmailOptions:     params.EmailOptions,
			}
			err = s.emailService.SendEmailForTemplate(childCtx, request)
			if err == nil {
				successfulChannels++
				childSpan.End()
			} else {
				// Log the error but continue with other channels
				s.logger.WithFields(map[string]interface{}{
					"error":        err.Error(),
					"channel":      channel,
					"notification": notification.ID,
					"contact":      contact.Email,
					"message_id":   messageID,
				}).Error("Failed to send email notification")

				tracing.MarkSpanError(childCtx, err)
				childSpan.End()
			}
		}
		// Add other channel handling here as needed
	}

	if successfulChannels == 0 {
		err := fmt.Errorf("failed to send notification through any channel")
		tracing.MarkSpanError(ctx, err)
		return "", err
	}

	span.AddAttributes(
		trace.Int64Attribute("successful_channels", int64(successfulChannels)),
	)

	return messageID, nil
}

// TestTemplate sends a test email with a template to verify it works
func (s *TransactionalNotificationService) TestTemplate(ctx context.Context, workspaceID string, templateID string, integrationID string, senderID string, recipientEmail string, emailOptions domain.EmailOptions) error {
	// Authenticate user
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user for workspace: %w", err)
	}

	// Get the template
	template, err := s.templateService.GetTemplateByID(ctx, workspaceID, templateID, int64(0))
	if err != nil {
		return fmt.Errorf("failed to retrieve template: %w", err)
	}

	// Ensure the template has email content
	if template.Email == nil {
		return errors.New("template does not contain email content")
	}

	// Get the email provider for the workspace
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	// Find the integration
	var emailProvider *domain.EmailProvider
	for _, integration := range workspace.Integrations {
		if integration.ID == integrationID {
			emailProvider = &integration.EmailProvider
			break
		}
	}

	if emailProvider == nil {
		return fmt.Errorf("integration not found: %s", integrationID)
	}

	// Find the emailSender
	emailSender := emailProvider.GetSender(senderID)

	if emailSender == nil {
		return fmt.Errorf("sender not found: %s", senderID)
	}

	// upsert the contact
	contactOperation := s.contactService.UpsertContact(ctx, workspaceID, &domain.Contact{
		Email: recipientEmail,
	})

	if contactOperation.Action == domain.UpsertContactOperationError {
		return fmt.Errorf("failed to upsert contact: %s", contactOperation.Error)
	}

	contactWithList := domain.ContactWithList{
		Contact: &domain.Contact{
			Email: recipientEmail,
		},
		ListID:   "foo",
		ListName: "bar",
	}

	// Use fixed messageID for testing
	messageID := uuid.New().String()

	// Use workspace CustomEndpointURL if provided, otherwise use the default API endpoint
	endpoint := s.apiEndpoint
	if workspace.Settings.CustomEndpointURL != nil && *workspace.Settings.CustomEndpointURL != "" {
		endpoint = *workspace.Settings.CustomEndpointURL
	}

	trackingSettings := notifuse_mjml.TrackingSettings{
		EnableTracking: true,
		Endpoint:       endpoint,
		WorkspaceID:    workspaceID,
		MessageID:      messageID,
	}

	req := domain.TemplateDataRequest{
		WorkspaceID:        workspace.ID,
		WorkspaceSecretKey: workspace.Settings.SecretKey,
		ContactWithList:    contactWithList,
		MessageID:          messageID,
		TrackingSettings:   trackingSettings,
		Broadcast:          nil,
		ProvidedData:       template.TestData,
	}
	messageData, err := domain.BuildTemplateData(req)

	if err != nil {
		return fmt.Errorf("failed to build template data: %w", err)
	}

	// Compile the template with the test data
	compiledResult, err := s.templateService.CompileTemplate(ctx, domain.CompileTemplateRequest{
		WorkspaceID:      workspaceID,
		MessageID:        messageID,
		VisualEditorTree: template.Email.VisualEditorTree,
		TemplateData:     notifuse_mjml.MapOfAny(messageData),
		TrackingSettings: trackingSettings,
	})

	if err != nil {
		return fmt.Errorf("failed to compile template: %w", err)
	}

	if !compiledResult.Success || compiledResult.HTML == nil {
		errMsg := "Unknown error"
		if compiledResult.Error != nil {
			errMsg = compiledResult.Error.Message
		}
		return fmt.Errorf("template compilation failed: %s", errMsg)
	}

	// Process subject line through Liquid templating if it contains Liquid tags
	processedSubject, err := notifuse_mjml.ProcessLiquidTemplate(
		template.Email.Subject,
		messageData,
		"email_subject",
	)
	if err != nil {
		return fmt.Errorf("failed to process subject with Liquid: %w", err)
	}

	// Create SendEmailProviderRequest
	emailRequest := domain.SendEmailProviderRequest{
		WorkspaceID:   workspaceID,
		IntegrationID: integrationID,
		MessageID:     messageID,
		FromAddress:   emailSender.Email,
		FromName:      emailSender.Name,
		To:            recipientEmail,
		Subject:       processedSubject,
		Content:       *compiledResult.HTML,
		Provider:      emailProvider,
		EmailOptions:  emailOptions,
	}

	// Send the email
	err = s.emailService.SendEmail(ctx, emailRequest, false)

	if err != nil {
		return fmt.Errorf("failed to send test email: %w", err)
	}

	// record the message history
	return s.messageHistoryRepo.Create(ctx, workspaceID, &domain.MessageHistory{
		ID:              messageID,
		ExternalID:      nil, // No external ID for test messages
		ContactEmail:    recipientEmail,
		TemplateID:      templateID,
		TemplateVersion: template.Version,
		Channel:         "email",
		MessageData: domain.MessageData{
			Data: messageData,
		},
		SentAt:    time.Now().UTC(),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
}
