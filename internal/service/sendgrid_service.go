package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

const (
	sendgridAPIBaseURL = "https://api.sendgrid.com"
)

// SendGridService implements the domain.SendGridServiceInterface
type SendGridService struct {
	httpClient  domain.HTTPClient
	authService domain.AuthService
	logger      logger.Logger
}

// NewSendGridService creates a new instance of SendGridService
func NewSendGridService(httpClient domain.HTTPClient, authService domain.AuthService, logger logger.Logger) *SendGridService {
	return &SendGridService{
		httpClient:  httpClient,
		authService: authService,
		logger:      logger,
	}
}

// GetWebhookSettings retrieves the current webhook configuration from SendGrid
func (s *SendGridService) GetWebhookSettings(ctx context.Context, config domain.SendGridSettings) (*domain.SendGridWebhookSettings, error) {
	apiURL := fmt.Sprintf("%s/v3/user/webhooks/event/settings", sendgridAPIBaseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for getting SendGrid webhook settings: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.APIKey))
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for getting SendGrid webhook settings: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("SendGrid API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	var settings domain.SendGridWebhookSettings
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode SendGrid webhook settings response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &settings, nil
}

// UpdateWebhookSettings updates the webhook configuration in SendGrid
func (s *SendGridService) UpdateWebhookSettings(ctx context.Context, config domain.SendGridSettings, settings domain.SendGridWebhookSettings) error {
	apiURL := fmt.Sprintf("%s/v3/user/webhooks/event/settings", sendgridAPIBaseURL)

	requestBody, err := json.Marshal(settings)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to marshal webhook settings: %v", err))
		return fmt.Errorf("failed to marshal webhook settings: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, apiURL, bytes.NewBuffer(requestBody))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for updating SendGrid webhook settings: %v", err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.APIKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for updating SendGrid webhook settings: %v", err))
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("SendGrid API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	return nil
}

// RegisterWebhooks implements the domain.WebhookProvider interface for SendGrid
func (s *SendGridService) RegisterWebhooks(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	baseURL string,
	eventTypes []domain.EmailEventType,
	providerConfig *domain.EmailProvider,
) (*domain.WebhookRegistrationStatus, error) {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.SendGrid == nil || providerConfig.SendGrid.APIKey == "" {
		return nil, fmt.Errorf("SendGrid configuration is missing or invalid")
	}

	// Generate webhook URL that includes workspace_id and integration_id
	webhookURL := domain.GenerateWebhookCallbackURL(baseURL, domain.EmailProviderKindSendGrid, workspaceID, integrationID)

	// Build webhook settings from requested event types
	settings := domain.SendGridWebhookSettings{
		Enabled: true,
		URL:     webhookURL,
	}

	// Map our event types to SendGrid webhook event flags
	for _, eventType := range eventTypes {
		switch eventType {
		case domain.EmailEventDelivered:
			settings.Delivered = true
		case domain.EmailEventBounce:
			settings.Bounce = true
			settings.Dropped = true // Include dropped events as they're delivery failures
		case domain.EmailEventComplaint:
			settings.SpamReport = true
		}
	}

	// Also enable deferred events for better tracking
	settings.Deferred = true

	// Update webhook settings in SendGrid
	if err := s.UpdateWebhookSettings(ctx, *providerConfig.SendGrid, settings); err != nil {
		return nil, fmt.Errorf("failed to update SendGrid webhook settings: %w", err)
	}

	// Create webhook registration status
	status := &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindSendGrid,
		IsRegistered:      true,
		Endpoints:         []domain.WebhookEndpointStatus{},
		ProviderDetails: map[string]interface{}{
			"integration_id": integrationID,
			"workspace_id":   workspaceID,
		},
	}

	// Add endpoints for each event type
	for _, eventType := range eventTypes {
		status.Endpoints = append(status.Endpoints, domain.WebhookEndpointStatus{
			URL:       webhookURL,
			EventType: eventType,
			Active:    true,
		})
	}

	return status, nil
}

// GetWebhookStatus implements the domain.WebhookProvider interface for SendGrid
func (s *SendGridService) GetWebhookStatus(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	providerConfig *domain.EmailProvider,
) (*domain.WebhookRegistrationStatus, error) {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.SendGrid == nil || providerConfig.SendGrid.APIKey == "" {
		return nil, fmt.Errorf("SendGrid configuration is missing or invalid")
	}

	// Get current webhook settings from SendGrid
	settings, err := s.GetWebhookSettings(ctx, *providerConfig.SendGrid)
	if err != nil {
		return nil, fmt.Errorf("failed to get SendGrid webhook settings: %w", err)
	}

	// Create webhook status response
	status := &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindSendGrid,
		IsRegistered:      settings.Enabled,
		Endpoints:         []domain.WebhookEndpointStatus{},
		ProviderDetails: map[string]interface{}{
			"integration_id": integrationID,
			"workspace_id":   workspaceID,
			"url":            settings.URL,
		},
	}

	// Add endpoints based on enabled event types
	if settings.Enabled {
		if settings.Delivered {
			status.Endpoints = append(status.Endpoints, domain.WebhookEndpointStatus{
				URL:       settings.URL,
				EventType: domain.EmailEventDelivered,
				Active:    true,
			})
		}
		if settings.Bounce {
			status.Endpoints = append(status.Endpoints, domain.WebhookEndpointStatus{
				URL:       settings.URL,
				EventType: domain.EmailEventBounce,
				Active:    true,
			})
		}
		if settings.SpamReport {
			status.Endpoints = append(status.Endpoints, domain.WebhookEndpointStatus{
				URL:       settings.URL,
				EventType: domain.EmailEventComplaint,
				Active:    true,
			})
		}
	}

	return status, nil
}

// UnregisterWebhooks implements the domain.WebhookProvider interface for SendGrid
func (s *SendGridService) UnregisterWebhooks(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	providerConfig *domain.EmailProvider,
) error {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.SendGrid == nil || providerConfig.SendGrid.APIKey == "" {
		return fmt.Errorf("SendGrid configuration is missing or invalid")
	}

	// Disable webhooks by setting enabled to false
	settings := domain.SendGridWebhookSettings{
		Enabled: false,
	}

	if err := s.UpdateWebhookSettings(ctx, *providerConfig.SendGrid, settings); err != nil {
		return fmt.Errorf("failed to disable SendGrid webhooks: %w", err)
	}

	return nil
}

// SendEmail sends an email using SendGrid
func (s *SendGridService) SendEmail(ctx context.Context, request domain.SendEmailProviderRequest) error {
	// Validate the request
	if err := request.Validate(); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	if request.Provider.SendGrid == nil {
		return fmt.Errorf("SendGrid provider is not configured")
	}

	// Build the SendGrid mail send request
	// https://docs.sendgrid.com/api-reference/mail-send/mail-send

	type EmailAddress struct {
		Email string `json:"email"`
		Name  string `json:"name,omitempty"`
	}

	type Personalization struct {
		To  []EmailAddress `json:"to"`
		CC  []EmailAddress `json:"cc,omitempty"`
		BCC []EmailAddress `json:"bcc,omitempty"`
	}

	type Content struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}

	type Attachment struct {
		Content     string `json:"content"` // Base64 encoded
		Filename    string `json:"filename"`
		Type        string `json:"type,omitempty"`
		Disposition string `json:"disposition,omitempty"` // "attachment" or "inline"
		ContentID   string `json:"content_id,omitempty"`  // For inline images
	}

	type MailSendRequest struct {
		Personalizations []Personalization `json:"personalizations"`
		From             EmailAddress      `json:"from"`
		ReplyTo          *EmailAddress     `json:"reply_to,omitempty"`
		Subject          string            `json:"subject"`
		Content          []Content         `json:"content"`
		CustomArgs       map[string]string `json:"custom_args,omitempty"`
		Headers          map[string]string `json:"headers,omitempty"`
		Attachments      []Attachment      `json:"attachments,omitempty"`
	}

	// Build personalizations (recipients)
	personalization := Personalization{
		To: []EmailAddress{{Email: request.To}},
	}

	// Add CC recipients
	for _, cc := range request.EmailOptions.CC {
		if cc != "" {
			personalization.CC = append(personalization.CC, EmailAddress{Email: cc})
		}
	}

	// Add BCC recipients
	for _, bcc := range request.EmailOptions.BCC {
		if bcc != "" {
			personalization.BCC = append(personalization.BCC, EmailAddress{Email: bcc})
		}
	}

	// Build the mail request
	mailReq := MailSendRequest{
		Personalizations: []Personalization{personalization},
		From: EmailAddress{
			Email: request.FromAddress,
			Name:  request.FromName,
		},
		Subject: request.Subject,
		Content: []Content{
			{
				Type:  "text/html",
				Value: request.Content,
			},
		},
		CustomArgs: map[string]string{
			"notifuse_message_id": request.MessageID,
		},
	}

	// Add reply-to if specified
	if request.EmailOptions.ReplyTo != "" {
		mailReq.ReplyTo = &EmailAddress{Email: request.EmailOptions.ReplyTo}
	}

	// Add RFC-8058 List-Unsubscribe headers for one-click unsubscribe
	if request.EmailOptions.ListUnsubscribeURL != "" {
		mailReq.Headers = map[string]string{
			"List-Unsubscribe":      fmt.Sprintf("<%s>", request.EmailOptions.ListUnsubscribeURL),
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
		}
	}

	// Add attachments if specified
	if len(request.EmailOptions.Attachments) > 0 {
		for i, att := range request.EmailOptions.Attachments {
			// Validate content can be decoded
			content, err := att.DecodeContent()
			if err != nil {
				return fmt.Errorf("attachment %d: failed to decode content: %w", i, err)
			}

			contentType := att.ContentType
			if contentType == "" {
				contentType = "application/octet-stream"
			}

			disposition := att.Disposition
			if disposition == "" {
				disposition = "attachment"
			}

			sgAttachment := Attachment{
				Content:     att.Content, // Already base64 encoded
				Filename:    att.Filename,
				Type:        contentType,
				Disposition: disposition,
			}

			// For inline attachments, derive ContentID from filename
			if disposition == "inline" {
				sgAttachment.ContentID = att.Filename
			}

			mailReq.Attachments = append(mailReq.Attachments, sgAttachment)

			s.logger.WithField("attachment_size", len(content)).
				WithField("filename", att.Filename).
				Debug("Added attachment to SendGrid request")
		}
	}

	// Convert to JSON
	jsonData, err := json.Marshal(mailReq)
	if err != nil {
		return fmt.Errorf("failed to marshal email request: %w", err)
	}

	// Create HTTP request
	apiURL := fmt.Sprintf("%s/v3/mail/send", sendgridAPIBaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for sending SendGrid email: %v", err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", request.Provider.SendGrid.APIKey))
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for sending SendGrid email: %v", err))
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// SendGrid returns 202 Accepted on success
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("SendGrid API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	return nil
}
