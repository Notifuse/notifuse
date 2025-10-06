package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// MailjetService implements the domain.MailjetServiceInterface
type MailjetService struct {
	httpClient  domain.HTTPClient
	authService domain.AuthService
	logger      logger.Logger
}

// NewMailjetService creates a new instance of MailjetService
func NewMailjetService(httpClient domain.HTTPClient, authService domain.AuthService, logger logger.Logger) *MailjetService {
	return &MailjetService{
		httpClient:  httpClient,
		authService: authService,
		logger:      logger,
	}
}

// ListWebhooks retrieves all registered webhooks
func (s *MailjetService) ListWebhooks(ctx context.Context, config domain.MailjetSettings) (*domain.MailjetWebhookResponse, error) {

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.mailjet.com/v3/REST/eventcallbackurl", nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for listing Mailjet webhooks: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Mailjet uses Basic Auth with API Key and Secret Key
	req.SetBasicAuth(config.APIKey, config.SecretKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for listing Mailjet webhooks: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Mailjet API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	// Parse the response
	var response domain.MailjetWebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode Mailjet webhook list response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// CreateWebhook creates a new webhook
func (s *MailjetService) CreateWebhook(ctx context.Context, config domain.MailjetSettings, webhook domain.MailjetWebhook) (*domain.MailjetWebhook, error) {

	// Prepare the request body
	requestBody, err := json.Marshal(webhook)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to marshal webhook configuration: %v", err))
		return nil, fmt.Errorf("failed to marshal webhook configuration: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.mailjet.com/v3/REST/eventcallbackurl", bytes.NewBuffer(requestBody))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for creating Mailjet webhook: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Mailjet uses Basic Auth with API Key and Secret Key
	req.SetBasicAuth(config.APIKey, config.SecretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for creating Mailjet webhook: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Mailjet API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	// Parse the response to get the created webhook details
	var createdWebhook domain.MailjetWebhook
	if err := json.NewDecoder(resp.Body).Decode(&createdWebhook); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode Mailjet webhook response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &createdWebhook, nil
}

// GetWebhook retrieves a webhook by ID
func (s *MailjetService) GetWebhook(ctx context.Context, config domain.MailjetSettings, webhookID int64) (*domain.MailjetWebhook, error) {

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://api.mailjet.com/v3/REST/eventcallbackurl/%d", webhookID), nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for getting Mailjet webhook: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Mailjet uses Basic Auth with API Key and Secret Key
	req.SetBasicAuth(config.APIKey, config.SecretKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for getting Mailjet webhook: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Mailjet API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	// Parse the response to get the webhook details
	var webhookResponse struct {
		Data []domain.MailjetWebhook `json:"Data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&webhookResponse); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode Mailjet webhook response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(webhookResponse.Data) == 0 {
		return nil, fmt.Errorf("webhook with ID %d not found", webhookID)
	}

	return &webhookResponse.Data[0], nil
}

// UpdateWebhook updates an existing webhook
func (s *MailjetService) UpdateWebhook(ctx context.Context, config domain.MailjetSettings, webhookID int64, webhook domain.MailjetWebhook) (*domain.MailjetWebhook, error) {

	// Ensure the webhook ID in the URL matches the one in the body
	webhook.ID = webhookID

	// Prepare the request body
	requestBody, err := json.Marshal(webhook)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to marshal webhook configuration: %v", err))
		return nil, fmt.Errorf("failed to marshal webhook configuration: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("https://api.mailjet.com/v3/REST/eventcallbackurl/%d", webhookID), bytes.NewBuffer(requestBody))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for updating Mailjet webhook: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Mailjet uses Basic Auth with API Key and Secret Key
	req.SetBasicAuth(config.APIKey, config.SecretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for updating Mailjet webhook: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Mailjet API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	// Parse the response to get the updated webhook details
	var updatedWebhook domain.MailjetWebhook
	if err := json.NewDecoder(resp.Body).Decode(&updatedWebhook); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode Mailjet webhook response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &updatedWebhook, nil
}

// DeleteWebhook deletes a webhook by ID
func (s *MailjetService) DeleteWebhook(ctx context.Context, config domain.MailjetSettings, webhookID int64) error {

	// Log webhook ID for debugging
	webhookIDStr := strconv.FormatInt(webhookID, 10)
	s.logger = s.logger.WithField("webhook_id", webhookIDStr)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("https://api.mailjet.com/v3/REST/eventcallbackurl/%d", webhookID), nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for deleting Mailjet webhook: %v", err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Mailjet uses Basic Auth with API Key and Secret Key
	req.SetBasicAuth(config.APIKey, config.SecretKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for deleting Mailjet webhook: %v", err))
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Mailjet API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	return nil
}

// RegisterWebhooks implements the domain.WebhookProvider interface for Mailjet
func (s *MailjetService) RegisterWebhooks(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	baseURL string,
	eventTypes []domain.EmailEventType,
	providerConfig *domain.EmailProvider,
) (*domain.WebhookRegistrationStatus, error) {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.Mailjet == nil ||
		providerConfig.Mailjet.APIKey == "" || providerConfig.Mailjet.SecretKey == "" {
		return nil, fmt.Errorf("Mailjet configuration is missing or invalid")
	}

	// Create webhook URL that includes workspace_id and integration_id
	webhookURL := domain.GenerateWebhookCallbackURL(baseURL, domain.EmailProviderKindMailjet, workspaceID, integrationID)

	// Map our event types to Mailjet event types
	var registeredEvents []domain.EmailEventType
	var mailjetEvents []domain.MailjetWebhookEventType

	for _, eventType := range eventTypes {
		switch eventType {
		case domain.EmailEventDelivered:
			mailjetEvents = append(mailjetEvents, domain.MailjetEventSent)
			registeredEvents = append(registeredEvents, domain.EmailEventDelivered)
		case domain.EmailEventBounce:
			// Register both bounce and blocked events for comprehensive bounce tracking
			mailjetEvents = append(mailjetEvents, domain.MailjetEventBounce)
			mailjetEvents = append(mailjetEvents, domain.MailjetEventBlocked)
			registeredEvents = append(registeredEvents, domain.EmailEventBounce)
			registeredEvents = append(registeredEvents, domain.EmailEventBounce) // One entry for each webhook
		case domain.EmailEventComplaint:
			// Register both spam and unsubscribe events for complaint tracking
			mailjetEvents = append(mailjetEvents, domain.MailjetEventSpam)
			mailjetEvents = append(mailjetEvents, domain.MailjetEventUnsub)
			registeredEvents = append(registeredEvents, domain.EmailEventComplaint)
			registeredEvents = append(registeredEvents, domain.EmailEventComplaint) // One entry for each webhook
		}
	}

	// First, get existing webhooks
	existingWebhooks, err := s.ListWebhooks(ctx, *providerConfig.Mailjet)
	if err != nil {
		return nil, fmt.Errorf("failed to list Mailjet webhooks: %w", err)
	}

	// Check for existing webhooks that match our criteria
	var notifuseWebhooks []domain.MailjetWebhook
	for _, webhook := range existingWebhooks.Data {
		if strings.Contains(webhook.Endpoint, baseURL) &&
			strings.Contains(webhook.Endpoint, fmt.Sprintf("workspace_id=%s", workspaceID)) &&
			strings.Contains(webhook.Endpoint, fmt.Sprintf("integration_id=%s", integrationID)) {
			notifuseWebhooks = append(notifuseWebhooks, webhook)
		}
	}

	// Delete existing webhooks
	for _, webhook := range notifuseWebhooks {
		err := s.DeleteWebhook(ctx, *providerConfig.Mailjet, webhook.ID)
		if err != nil {
			s.logger.WithField("webhook_id", webhook.ID).
				Error(fmt.Sprintf("Failed to delete Mailjet webhook: %v", err))
			// Continue with other webhooks even if one fails
		}
	}

	// Create webhooks for each event type
	// According to Mailjet documentation, each webhook handles one event type
	var createdWebhooks []domain.MailjetWebhook
	for _, eventType := range mailjetEvents {
		webhookConfig := domain.MailjetWebhook{
			Endpoint:  webhookURL,
			EventType: string(eventType),
			Status:    "alive",
			Version:   2,     // Use version 2 as recommended by Mailjet documentation
			IsBackup:  false, // This is not a backup webhook
		}

		webhook, err := s.CreateWebhook(ctx, *providerConfig.Mailjet, webhookConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create Mailjet webhook for event %s: %w", eventType, err)
		}
		createdWebhooks = append(createdWebhooks, *webhook)
	}

	// Create webhook registration status
	status := &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindMailjet,
		IsRegistered:      true,
		Endpoints:         []domain.WebhookEndpointStatus{},
		ProviderDetails: map[string]interface{}{
			"integration_id": integrationID,
			"workspace_id":   workspaceID,
		},
	}

	// Add endpoints for each created webhook
	for i, webhook := range createdWebhooks {
		if i < len(registeredEvents) {
			status.Endpoints = append(status.Endpoints, domain.WebhookEndpointStatus{
				WebhookID: strconv.FormatInt(webhook.ID, 10),
				URL:       webhookURL,
				EventType: registeredEvents[i],
				Active:    webhook.Status == "alive",
			})
		}
	}

	return status, nil
}

// GetWebhookStatus implements the domain.WebhookProvider interface for Mailjet
func (s *MailjetService) GetWebhookStatus(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	providerConfig *domain.EmailProvider,
) (*domain.WebhookRegistrationStatus, error) {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.Mailjet == nil ||
		providerConfig.Mailjet.APIKey == "" || providerConfig.Mailjet.SecretKey == "" {
		return nil, fmt.Errorf("Mailjet configuration is missing or invalid")
	}

	// Create webhook status response
	status := &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindMailjet,
		IsRegistered:      false,
		Endpoints:         []domain.WebhookEndpointStatus{},
		ProviderDetails: map[string]interface{}{
			"integration_id": integrationID,
			"workspace_id":   workspaceID,
		},
	}

	// Get existing webhooks
	existingWebhooks, err := s.ListWebhooks(ctx, *providerConfig.Mailjet)
	if err != nil {
		return nil, fmt.Errorf("failed to list Mailjet webhooks: %w", err)
	}

	// Look for webhooks that match our integration
	for _, webhook := range existingWebhooks.Data {

		if strings.Contains(webhook.Endpoint, fmt.Sprintf("workspace_id=%s", workspaceID)) &&
			strings.Contains(webhook.Endpoint, fmt.Sprintf("integration_id=%s", integrationID)) {

			status.IsRegistered = true

			// Map event types based on webhook.EventType
			switch domain.MailjetWebhookEventType(webhook.EventType) {
			case domain.MailjetEventSent:
				status.Endpoints = append(status.Endpoints, domain.WebhookEndpointStatus{
					WebhookID: strconv.FormatInt(webhook.ID, 10),
					URL:       webhook.Endpoint,
					EventType: domain.EmailEventDelivered,
					Active:    webhook.Status == "alive",
				})
			case domain.MailjetEventBounce, domain.MailjetEventBlocked:
				status.Endpoints = append(status.Endpoints, domain.WebhookEndpointStatus{
					WebhookID: strconv.FormatInt(webhook.ID, 10),
					URL:       webhook.Endpoint,
					EventType: domain.EmailEventBounce,
					Active:    webhook.Status == "alive",
				})
			case domain.MailjetEventSpam, domain.MailjetEventUnsub:
				status.Endpoints = append(status.Endpoints, domain.WebhookEndpointStatus{
					WebhookID: strconv.FormatInt(webhook.ID, 10),
					URL:       webhook.Endpoint,
					EventType: domain.EmailEventComplaint,
					Active:    webhook.Status == "alive",
				})
			}
		}
	}

	return status, nil
}

// UnregisterWebhooks implements the domain.WebhookProvider interface for Mailjet
func (s *MailjetService) UnregisterWebhooks(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	providerConfig *domain.EmailProvider,
) error {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.Mailjet == nil ||
		providerConfig.Mailjet.APIKey == "" || providerConfig.Mailjet.SecretKey == "" {
		return fmt.Errorf("Mailjet configuration is missing or invalid")
	}

	// Get existing webhooks
	existingWebhooks, err := s.ListWebhooks(ctx, *providerConfig.Mailjet)
	if err != nil {
		return fmt.Errorf("failed to list Mailjet webhooks: %w", err)
	}

	// Delete webhooks that match our criteria
	var lastError error
	for _, webhook := range existingWebhooks.Data {
		if strings.Contains(webhook.Endpoint, fmt.Sprintf("workspace_id=%s", workspaceID)) &&
			strings.Contains(webhook.Endpoint, fmt.Sprintf("integration_id=%s", integrationID)) {

			err := s.DeleteWebhook(ctx, *providerConfig.Mailjet, webhook.ID)
			if err != nil {
				s.logger.WithField("webhook_id", webhook.ID).
					Error(fmt.Sprintf("Failed to delete Mailjet webhook: %v", err))
				lastError = err
				// Continue deleting other webhooks even if one fails
			} else {
				s.logger.WithField("webhook_id", webhook.ID).
					Info("Successfully deleted Mailjet webhook")
			}
		}
	}

	if lastError != nil {
		return fmt.Errorf("failed to delete one or more Mailjet webhooks: %w", lastError)
	}

	return nil
}

// TestWebhook implements the domain.WebhookProvider interface for Mailjet
// Mailjet doesn't support testing webhooks directly
func (s *MailjetService) TestWebhook(ctx context.Context, config domain.MailjetSettings, webhookID string, eventType string) error {
	return fmt.Errorf("webhook testing is not supported for Mailjet")
}

// SendEmail sends an email using Mailjet
func (s *MailjetService) SendEmail(ctx context.Context, request domain.SendEmailProviderRequest) error {
	// Validate the request
	if err := request.Validate(); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	if request.Provider.Mailjet == nil {
		return fmt.Errorf("Mailjet provider is not configured")
	}

	// Prepare the request payload
	type EmailRecipient struct {
		Email string `json:"Email"`
		Name  string `json:"Name,omitempty"`
	}

	type MailjetAttachment struct {
		ContentType   string `json:"ContentType"`
		Filename      string `json:"Filename"`
		Base64Content string `json:"Base64Content"`
	}

	type MailjetInlinedAttachment struct {
		ContentType   string `json:"ContentType"`
		Filename      string `json:"Filename"`
		Base64Content string `json:"Base64Content"`
		ContentID     string `json:"ContentID"` // For inline images, e.g., "logo.png"
	}

	type EmailMessage struct {
		From struct {
			Email string `json:"Email"`
			Name  string `json:"Name,omitempty"`
		} `json:"From"`
		To                 []EmailRecipient           `json:"To"`
		Cc                 []EmailRecipient           `json:"Cc,omitempty"`
		Bcc                []EmailRecipient           `json:"Bcc,omitempty"`
		Subject            string                     `json:"Subject"`
		HTMLPart           string                     `json:"HTMLPart"`
		CustomID           string                     `json:"CustomID,omitempty"`
		TextPart           string                     `json:"TextPart,omitempty"`
		TemplateID         int                        `json:"TemplateID,omitempty"`
		TemplateLanguage   bool                       `json:"TemplateLanguage,omitempty"`
		Headers            map[string]string          `json:"Headers,omitempty"`
		Attachments        []MailjetAttachment        `json:"Attachments,omitempty"`
		InlinedAttachments []MailjetInlinedAttachment `json:"InlinedAttachments,omitempty"`
	}

	type EmailRequest struct {
		Messages    []EmailMessage `json:"Messages"`
		SandboxMode bool           `json:"SandboxMode,omitempty"`
	}

	// Create the email message
	message := EmailMessage{
		From: struct {
			Email string `json:"Email"`
			Name  string `json:"Name,omitempty"`
		}{
			Email: request.FromAddress,
			Name:  request.FromName,
		},
		To: []EmailRecipient{
			{
				Email: request.To,
			},
		},
		Subject:  request.Subject,
		HTMLPart: request.Content,
		CustomID: request.MessageID,
	}

	// Add CC recipients if specified
	if len(request.EmailOptions.CC) > 0 {
		for _, ccAddr := range request.EmailOptions.CC {
			if ccAddr != "" {
				message.Cc = append(message.Cc, EmailRecipient{Email: ccAddr})
			}
		}
	}

	// Add BCC recipients if specified
	if len(request.EmailOptions.BCC) > 0 {
		for _, bccAddr := range request.EmailOptions.BCC {
			if bccAddr != "" {
				message.Bcc = append(message.Bcc, EmailRecipient{Email: bccAddr})
			}
		}
	}

	// Initialize headers map if not already initialized
	if message.Headers == nil {
		message.Headers = make(map[string]string)
	}

	// Add Reply-To if specified
	if request.EmailOptions.ReplyTo != "" {
		message.Headers["Reply-To"] = request.EmailOptions.ReplyTo
	}

	// Add attachments if specified
	// Mailjet uses separate arrays for regular attachments and inline images
	// https://dev.mailjet.com/email/guides/send-api-v31/#send-with-attached-files
	if len(request.EmailOptions.Attachments) > 0 {
		for i, att := range request.EmailOptions.Attachments {
			content, err := att.DecodeContent()
			if err != nil {
				return fmt.Errorf("attachment %d: failed to decode content: %w", i, err)
			}

			contentType := att.ContentType
			if contentType == "" {
				contentType = "application/octet-stream"
			}

			// Mailjet uses InlinedAttachments for inline images
			if att.Disposition == "inline" {
				// For inline attachments, use InlinedAttachments array with ContentID
				message.InlinedAttachments = append(message.InlinedAttachments, MailjetInlinedAttachment{
					ContentType:   contentType,
					Filename:      att.Filename,
					Base64Content: att.Content,  // Already base64 encoded
					ContentID:     att.Filename, // ContentID for referencing in HTML
				})
			} else {
				// Regular attachments
				message.Attachments = append(message.Attachments, MailjetAttachment{
					ContentType:   contentType,
					Filename:      att.Filename,
					Base64Content: att.Content, // Already base64 encoded
				})
			}

			// Log size for debugging
			s.logger.WithField("attachment_size", len(content)).
				WithField("filename", att.Filename).
				WithField("disposition", att.Disposition).
				Debug("Added attachment to Mailjet email")
		}
	}

	// Set up the email payload
	emailReq := EmailRequest{
		Messages:    []EmailMessage{message},
		SandboxMode: request.Provider.Mailjet.SandboxMode,
	}

	// Convert to JSON
	jsonData, err := json.Marshal(emailReq)
	if err != nil {
		return fmt.Errorf("failed to marshal email request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.mailjet.com/v3.1/send", bytes.NewBuffer(jsonData))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for sending Mailjet email: %v", err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set auth and headers
	req.SetBasicAuth(request.Provider.Mailjet.APIKey, request.Provider.Mailjet.SecretKey)
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for sending Mailjet email: %v", err))
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Mailjet API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	return nil
}
