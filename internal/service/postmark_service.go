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

// PostmarkService implements the domain.PostmarkServiceInterface
type PostmarkService struct {
	httpClient  domain.HTTPClient
	authService domain.AuthService
	logger      logger.Logger
}

// NewPostmarkService creates a new instance of PostmarkService
func NewPostmarkService(httpClient domain.HTTPClient, authService domain.AuthService, logger logger.Logger) *PostmarkService {
	return &PostmarkService{
		httpClient:  httpClient,
		authService: authService,
		logger:      logger,
	}
}

// ListWebhooks retrieves all registered webhooks
func (s *PostmarkService) ListWebhooks(ctx context.Context, config domain.PostmarkSettings) (*domain.PostmarkListWebhooksResponse, error) {

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.postmarkapp.com/webhooks", nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for listing Postmark webhooks: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Postmark-Server-Token", config.ServerToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for listing Postmark webhooks: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Postmark API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	var result domain.PostmarkListWebhooksResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode Postmark webhook list response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// RegisterWebhook registers a new webhook
func (s *PostmarkService) RegisterWebhook(ctx context.Context, config domain.PostmarkSettings, webhook domain.PostmarkWebhookConfig) (*domain.PostmarkWebhookResponse, error) {

	jsonData, err := json.Marshal(webhook)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to marshal webhook configuration: %v", err))
		return nil, fmt.Errorf("failed to marshal webhook configuration: %w", err)
	}

	// log.Printf("Request: %+v", string(jsonData))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.postmarkapp.com/webhooks", bytes.NewBuffer(jsonData))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for registering Postmark webhook: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Postmark-Server-Token", config.ServerToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for registering Postmark webhook: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		// log.Printf("Response: %+v", string(body))
		s.logger.Error(fmt.Sprintf("Postmark API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	var result domain.PostmarkWebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode Postmark webhook response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// UnregisterWebhook removes a webhook by ID
func (s *PostmarkService) UnregisterWebhook(ctx context.Context, config domain.PostmarkSettings, webhookID int) error {

	webhookIDStr := strconv.Itoa(webhookID)
	s.logger = s.logger.WithField("webhook_id", webhookIDStr)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, "https://api.postmarkapp.com/webhooks/"+webhookIDStr, nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for deleting Postmark webhook: %v", err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Postmark-Server-Token", config.ServerToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for deleting Postmark webhook: %v", err))
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Postmark API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	return nil
}

// GetWebhook retrieves a specific webhook by ID
func (s *PostmarkService) GetWebhook(ctx context.Context, config domain.PostmarkSettings, webhookID int) (*domain.PostmarkWebhookResponse, error) {

	webhookIDStr := strconv.Itoa(webhookID)
	s.logger = s.logger.WithField("webhook_id", webhookIDStr)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.postmarkapp.com/webhooks/"+webhookIDStr, nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for getting Postmark webhook: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Postmark-Server-Token", config.ServerToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for getting Postmark webhook: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Postmark API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	var result domain.PostmarkWebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode Postmark webhook response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// UpdateWebhook updates an existing webhook
func (s *PostmarkService) UpdateWebhook(ctx context.Context, config domain.PostmarkSettings, webhookID int, webhook domain.PostmarkWebhookConfig) (*domain.PostmarkWebhookResponse, error) {

	webhookIDStr := strconv.Itoa(webhookID)
	s.logger = s.logger.WithField("webhook_id", webhookIDStr)

	jsonData, err := json.Marshal(webhook)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to marshal webhook configuration: %v", err))
		return nil, fmt.Errorf("failed to marshal webhook configuration: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "https://api.postmarkapp.com/webhooks/"+webhookIDStr, bytes.NewBuffer(jsonData))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for updating Postmark webhook: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Postmark-Server-Token", config.ServerToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for updating Postmark webhook: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Postmark API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	var result domain.PostmarkWebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode Postmark webhook response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// TestWebhook sends a test event to the webhook
func (s *PostmarkService) TestWebhook(ctx context.Context, config domain.PostmarkSettings, webhookID int, eventType domain.EmailEventType) error {

	webhookIDStr := strconv.Itoa(webhookID)
	s.logger = s.logger.WithField("webhook_id", webhookIDStr)

	// Map our standard event types to Postmark test trigger types
	var triggerName string
	switch eventType {
	case domain.EmailEventDelivered:
		triggerName = "Delivery"
	case domain.EmailEventBounce:
		triggerName = "Bounce"
	case domain.EmailEventComplaint:
		triggerName = "SpamComplaint"
	default:
		return fmt.Errorf("unsupported event type: %s", eventType)
	}

	payload := map[string]string{"Trigger": triggerName}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to marshal test trigger payload: %v", err))
		return fmt.Errorf("failed to marshal test trigger payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.postmarkapp.com/webhooks/"+webhookIDStr+"/trigger", bytes.NewBuffer(jsonData))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for testing Postmark webhook: %v", err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Postmark-Server-Token", config.ServerToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for testing Postmark webhook: %v", err))
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Postmark API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	return nil
}

// RegisterWebhooks implements the domain.WebhookProvider interface for Postmark
func (s *PostmarkService) RegisterWebhooks(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	baseURL string,
	eventTypes []domain.EmailEventType,
	providerConfig *domain.EmailProvider,
) (*domain.WebhookRegistrationStatus, error) {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.Postmark == nil || providerConfig.Postmark.ServerToken == "" {
		return nil, fmt.Errorf("Postmark configuration is missing or invalid")
	}

	// Create webhook URL that includes workspace_id and integration_id
	webhookURL := domain.GenerateWebhookCallbackURL(baseURL, domain.EmailProviderKindPostmark, workspaceID, integrationID)

	// Map our event types to Postmark trigger settings
	triggers := &domain.PostmarkTriggers{
		Open: &domain.PostmarkOpenTrigger{
			Enabled: false,
		},
		Click: &domain.PostmarkClickTrigger{
			Enabled: false,
		},
		Delivery: &domain.PostmarkDeliveryTrigger{
			Enabled: false,
		},
		Bounce: &domain.PostmarkBounceTrigger{
			Enabled: false,
		},
		SpamComplaint: &domain.PostmarkSpamComplaintTrigger{
			Enabled: false,
		},
		SubscriptionChange: &domain.PostmarkSubscriptionChangeTrigger{
			Enabled: false,
		},
	}

	// Enable triggers based on the event types
	var eventsAdded []domain.EmailEventType
	for _, eventType := range eventTypes {
		switch eventType {
		case domain.EmailEventDelivered:
			triggers.Delivery.Enabled = true
			eventsAdded = append(eventsAdded, eventType)
		case domain.EmailEventBounce:
			triggers.Bounce.Enabled = true
			eventsAdded = append(eventsAdded, eventType)
		case domain.EmailEventComplaint:
			triggers.SpamComplaint.Enabled = true
			eventsAdded = append(eventsAdded, eventType)
		default:
			continue // Skip unsupported event types
		}
	}

	// First, get existing webhooks
	existingWebhooks, err := s.ListWebhooks(ctx, *providerConfig.Postmark)
	if err != nil {
		return nil, fmt.Errorf("failed to list Postmark webhooks: %w", err)
	}

	// Check if we have existing webhooks
	notifuseWebhooks := s.filterPostmarkWebhooks(existingWebhooks.Webhooks, baseURL, workspaceID, integrationID)

	// If we have existing webhooks, unregister them
	for _, webhook := range notifuseWebhooks {
		err := s.UnregisterWebhook(ctx, *providerConfig.Postmark, webhook.ID)
		if err != nil {
			s.logger.WithField("webhook_id", webhook.ID).
				Error(fmt.Sprintf("Failed to unregister Postmark webhook: %v", err))
			// Continue with other webhooks
		}
	}

	// Register new webhook
	webhookConfig := domain.PostmarkWebhookConfig{
		URL:           webhookURL,
		MessageStream: "outbound",
		Triggers:      triggers,
	}

	// Debug log the webhook config
	jsonData, _ := json.Marshal(webhookConfig)
	s.logger.Info(fmt.Sprintf("Registering Postmark webhook with config: %s", string(jsonData)))

	webhookResponse, err := s.RegisterWebhook(ctx, *providerConfig.Postmark, webhookConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to register Postmark webhook: %w", err)
	}

	// Create webhook status
	status := &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindPostmark,
		IsRegistered:      true,
		Endpoints:         []domain.WebhookEndpointStatus{},
		ProviderDetails: map[string]interface{}{
			"integration_id": integrationID,
			"workspace_id":   workspaceID,
		},
	}

	// Add endpoint statuses for each event type
	for _, eventType := range eventsAdded {
		status.Endpoints = append(status.Endpoints, domain.WebhookEndpointStatus{
			WebhookID: strconv.Itoa(webhookResponse.ID),
			URL:       webhookURL,
			EventType: eventType,
			Active:    true,
		})
	}

	return status, nil
}

// GetWebhookStatus implements the domain.WebhookProvider interface for Postmark
func (s *PostmarkService) GetWebhookStatus(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	providerConfig *domain.EmailProvider,
) (*domain.WebhookRegistrationStatus, error) {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.Postmark == nil || providerConfig.Postmark.ServerToken == "" {
		return nil, fmt.Errorf("Postmark configuration is missing or invalid")
	}

	// Get existing webhooks
	existingWebhooks, err := s.ListWebhooks(ctx, *providerConfig.Postmark)
	if err != nil {
		return nil, fmt.Errorf("failed to list Postmark webhooks: %w", err)
	}

	// Create webhook status
	status := &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindPostmark,
		IsRegistered:      false,
		Endpoints:         []domain.WebhookEndpointStatus{},
		ProviderDetails: map[string]interface{}{
			"integration_id": integrationID,
			"workspace_id":   workspaceID,
		},
	}

	// Filter webhooks for our integration
	notifuseWebhooks := s.filterPostmarkWebhooks(existingWebhooks.Webhooks, "", workspaceID, integrationID)

	// Check each webhook in the response
	for _, webhook := range notifuseWebhooks {
		// Create base endpoint status
		baseEndpoint := domain.WebhookEndpointStatus{
			WebhookID: strconv.Itoa(webhook.ID),
			URL:       webhook.URL,
			Active:    true,
		}

		// Determine registered event types from the response
		if webhook.Triggers != nil {
			// Check for Delivery event
			if webhook.Triggers.Delivery != nil && webhook.Triggers.Delivery.Enabled {
				deliveryEndpoint := baseEndpoint
				deliveryEndpoint.EventType = domain.EmailEventDelivered
				status.Endpoints = append(status.Endpoints, deliveryEndpoint)
			}

			// Check for Bounce event
			if webhook.Triggers.Bounce != nil && webhook.Triggers.Bounce.Enabled {
				bounceEndpoint := baseEndpoint
				bounceEndpoint.EventType = domain.EmailEventBounce
				status.Endpoints = append(status.Endpoints, bounceEndpoint)
			}

			// Check for SpamComplaint event
			if webhook.Triggers.SpamComplaint != nil && webhook.Triggers.SpamComplaint.Enabled {
				complaintEndpoint := baseEndpoint
				complaintEndpoint.EventType = domain.EmailEventComplaint
				status.Endpoints = append(status.Endpoints, complaintEndpoint)
			}
		}

		// Mark as registered if we have any endpoints
		if len(status.Endpoints) > 0 {
			status.IsRegistered = true
		}
	}

	return status, nil
}

// UnregisterWebhooks implements the domain.WebhookProvider interface for Postmark
func (s *PostmarkService) UnregisterWebhooks(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	providerConfig *domain.EmailProvider,
) error {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.Postmark == nil || providerConfig.Postmark.ServerToken == "" {
		return fmt.Errorf("Postmark configuration is missing or invalid")
	}

	// Get existing webhooks
	existingWebhooks, err := s.ListWebhooks(ctx, *providerConfig.Postmark)
	if err != nil {
		return fmt.Errorf("failed to list Postmark webhooks: %w", err)
	}

	// Find webhooks that contain this integration or workspace ID
	notifuseWebhooks := s.filterPostmarkWebhooks(existingWebhooks.Webhooks, "", workspaceID, integrationID)

	// Unregister each webhook
	var lastError error
	for _, webhook := range notifuseWebhooks {
		err := s.UnregisterWebhook(ctx, *providerConfig.Postmark, webhook.ID)
		if err != nil {
			s.logger.WithField("webhook_id", webhook.ID).
				Error(fmt.Sprintf("Failed to unregister Postmark webhook: %v", err))
			lastError = err
			// Continue with other webhooks even if one fails
		} else {
			s.logger.WithField("webhook_id", webhook.ID).
				Info("Successfully unregistered Postmark webhook")
		}
	}

	if lastError != nil {
		return fmt.Errorf("failed to unregister one or more Postmark webhooks: %w", lastError)
	}

	return nil
}

// Helper function to filter Postmark webhooks by base URL and integration ID
func (s *PostmarkService) filterPostmarkWebhooks(
	webhooks []domain.PostmarkWebhookResponse,
	baseURL string,
	workspaceID string,
	integrationID string,
) []domain.PostmarkWebhookResponse {
	var filtered []domain.PostmarkWebhookResponse
	for _, webhook := range webhooks {
		if (baseURL == "" || strings.Contains(webhook.URL, baseURL)) &&
			strings.Contains(webhook.URL, fmt.Sprintf("workspace_id=%s", workspaceID)) &&
			strings.Contains(webhook.URL, fmt.Sprintf("integration_id=%s", integrationID)) {
			filtered = append(filtered, webhook)
		}
	}
	return filtered
}

// SendEmail sends an email using Postmark
func (s *PostmarkService) SendEmail(ctx context.Context, workspaceID string, messageID string, fromAddress, fromName, to, subject, content string, provider *domain.EmailProvider, emailOptions domain.EmailOptions) error {
	if provider.Postmark == nil {
		return fmt.Errorf("Postmark provider is not configured")
	}

	// Make sure we have a server token
	if provider.Postmark.ServerToken == "" {
		s.logger.Error("Postmark server token is empty")
		return fmt.Errorf("Postmark server token is required")
	}

	// Prepare the API endpoint
	endpoint := "https://api.postmarkapp.com/email"

	// Prepare the request body
	requestBody := map[string]interface{}{
		"From":     fmt.Sprintf("%s <%s>", fromName, fromAddress),
		"To":       to,
		"Subject":  subject,
		"HtmlBody": content,
		"Headers": []map[string]string{
			{
				"Name":  "Message-ID",
				"Value": messageID,
			},
			{
				"X-PM-KeepID": "true",
			},
		},
	}

	// Add CC if specified
	if len(emailOptions.CC) > 0 {
		var ccAddresses []string
		for _, ccAddr := range emailOptions.CC {
			if ccAddr != "" {
				ccAddresses = append(ccAddresses, ccAddr)
			}
		}
		if len(ccAddresses) > 0 {
			requestBody["Cc"] = strings.Join(ccAddresses, ",")
		}
	}

	// Add BCC if specified
	if len(emailOptions.BCC) > 0 {
		var bccAddresses []string
		for _, bccAddr := range emailOptions.BCC {
			if bccAddr != "" {
				bccAddresses = append(bccAddresses, bccAddr)
			}
		}
		if len(bccAddresses) > 0 {
			requestBody["Bcc"] = strings.Join(bccAddresses, ",")
		}
	}

	// Add ReplyTo if specified
	if emailOptions.ReplyTo != "" {
		requestBody["ReplyTo"] = emailOptions.ReplyTo
	}

	// Convert to JSON
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal Postmark request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create Postmark request: %w", err)
	}

	// Add headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Postmark-Server-Token", provider.Postmark.ServerToken)

	// Use the injected HTTP client
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to Postmark API: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read Postmark API response: %w", err)
	}

	// Check response status
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Postmark API error (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}
