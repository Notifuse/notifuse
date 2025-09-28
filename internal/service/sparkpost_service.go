package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// SparkPostService implements the domain.SparkPostServiceInterface
type SparkPostService struct {
	httpClient  domain.HTTPClient
	authService domain.AuthService
	logger      logger.Logger
}

// NewSparkPostService creates a new instance of SparkPostService
func NewSparkPostService(httpClient domain.HTTPClient, authService domain.AuthService, logger logger.Logger) *SparkPostService {
	return &SparkPostService{
		httpClient:  httpClient,
		authService: authService,
		logger:      logger,
	}
}

// ListWebhooks retrieves all registered webhooks
func (s *SparkPostService) ListWebhooks(ctx context.Context, config domain.SparkPostSettings) (*domain.SparkPostWebhookListResponse, error) {

	apiURL := fmt.Sprintf("%s/api/v1/webhooks", config.Endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for listing SparkPost webhooks: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// SparkPost uses API key in header
	authHeader := fmt.Sprintf("Bearer %s", config.APIKey)
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for listing SparkPost webhooks: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		responseBody := string(body)
		s.logger.Error(fmt.Sprintf("SparkPost API returned non-OK status code %d: %s", resp.StatusCode, responseBody))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	// Parse the response
	var response domain.SparkPostWebhookListResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode SparkPost webhook list response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// CreateWebhook creates a new webhook
func (s *SparkPostService) CreateWebhook(ctx context.Context, config domain.SparkPostSettings, webhook domain.SparkPostWebhook) (*domain.SparkPostWebhookResponse, error) {

	// Construct the API URL
	apiURL := fmt.Sprintf("%s/api/v1/webhooks", config.Endpoint)

	// Prepare the request body
	requestBody, err := json.Marshal(webhook)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to marshal webhook configuration: %v", err))
		return nil, fmt.Errorf("failed to marshal webhook configuration: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(requestBody))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for creating SparkPost webhook: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// SparkPost uses API key in header
	authHeader := fmt.Sprintf("Bearer %s", config.APIKey)
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for creating SparkPost webhook: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("SparkPost API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	// Parse the response to get the created webhook details
	var response domain.SparkPostWebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode SparkPost webhook response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// GetWebhook retrieves a webhook by ID
func (s *SparkPostService) GetWebhook(ctx context.Context, config domain.SparkPostSettings, webhookID string) (*domain.SparkPostWebhookResponse, error) {

	// Construct the API URL
	apiURL := fmt.Sprintf("%s/api/v1/webhooks/%s", config.Endpoint, webhookID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for getting SparkPost webhook: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// SparkPost uses API key in header
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.APIKey))
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for getting SparkPost webhook: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("SparkPost API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	// Parse the response to get the webhook details
	var response domain.SparkPostWebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode SparkPost webhook response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// UpdateWebhook updates an existing webhook
func (s *SparkPostService) UpdateWebhook(ctx context.Context, config domain.SparkPostSettings, webhookID string, webhook domain.SparkPostWebhook) (*domain.SparkPostWebhookResponse, error) {

	// Construct the API URL
	apiURL := fmt.Sprintf("%s/api/v1/webhooks/%s", config.Endpoint, webhookID)

	// Prepare the request body
	requestBody, err := json.Marshal(webhook)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to marshal webhook configuration: %v", err))
		return nil, fmt.Errorf("failed to marshal webhook configuration: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, apiURL, bytes.NewBuffer(requestBody))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for updating SparkPost webhook: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// SparkPost uses API key in header
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.APIKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for updating SparkPost webhook: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("SparkPost API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	// Parse the response to get the updated webhook details
	var response domain.SparkPostWebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode SparkPost webhook response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// DeleteWebhook deletes a webhook by ID
func (s *SparkPostService) DeleteWebhook(ctx context.Context, config domain.SparkPostSettings, webhookID string) error {

	// Log webhook ID for debugging
	s.logger = s.logger.WithField("webhook_id", webhookID)

	// Construct the API URL
	apiURL := fmt.Sprintf("%s/api/v1/webhooks/%s", config.Endpoint, webhookID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiURL, nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for deleting SparkPost webhook: %v", err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	// SparkPost uses API key in header
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.APIKey))
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for deleting SparkPost webhook: %v", err))
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("SparkPost API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	return nil
}

// TestWebhook sends a test event to a webhook
func (s *SparkPostService) TestWebhook(ctx context.Context, config domain.SparkPostSettings, webhookID string) error {

	// Log webhook ID for debugging
	s.logger = s.logger.WithField("webhook_id", webhookID)

	// Construct the API URL
	apiURL := fmt.Sprintf("%s/api/v1/webhooks/%s/validate", config.Endpoint, webhookID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for testing SparkPost webhook: %v", err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	// SparkPost uses API key in header
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.APIKey))
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for testing SparkPost webhook: %v", err))
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("SparkPost API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	return nil
}

// ValidateWebhook validates a webhook's configuration
func (s *SparkPostService) ValidateWebhook(ctx context.Context, config domain.SparkPostSettings, webhook domain.SparkPostWebhook) (bool, error) {

	// Construct the API URL
	apiURL := fmt.Sprintf("%s/api/v1/webhooks/validate", config.Endpoint)

	// Prepare the request body with just the target URL to validate
	requestBody := map[string]string{
		"target": webhook.Target,
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to marshal validation request: %v", err))
		return false, fmt.Errorf("failed to marshal validation request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for validating SparkPost webhook: %v", err))
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	// SparkPost uses API key in header
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.APIKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for validating SparkPost webhook: %v", err))
		return false, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Parse the response to check if the webhook is valid
	var response struct {
		Results struct {
			Valid bool `json:"valid"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode SparkPost webhook validation response: %v", err))
		return false, fmt.Errorf("failed to decode validation response: %w", err)
	}

	return response.Results.Valid, nil
}

// RegisterWebhooks implements the domain.WebhookProvider interface for SparkPost
func (s *SparkPostService) RegisterWebhooks(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	baseURL string,
	eventTypes []domain.EmailEventType,
	providerConfig *domain.EmailProvider,
) (*domain.WebhookRegistrationStatus, error) {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.SparkPost == nil || providerConfig.SparkPost.APIKey == "" {
		return nil, fmt.Errorf("SparkPost configuration is missing or invalid")
	}

	// Check if sandbox mode is enabled
	if providerConfig.SparkPost.SandboxMode {
		// In sandbox mode, we don't actually register webhooks with SparkPost
		// but we return a simulated successful response
		s.logger.Info("SparkPost sandbox mode is enabled, simulating webhook registration")

		webhookURL := domain.GenerateWebhookCallbackURL(baseURL, domain.EmailProviderKindSparkPost, workspaceID, integrationID)

		// Create webhook registration status
		status := &domain.WebhookRegistrationStatus{
			EmailProviderKind: domain.EmailProviderKindSparkPost,
			IsRegistered:      true,
			Endpoints:         []domain.WebhookEndpointStatus{},
			ProviderDetails: map[string]interface{}{
				"webhook_id":     "sandbox-mode-webhook",
				"integration_id": integrationID,
				"workspace_id":   workspaceID,
				"sandbox_mode":   true,
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

	// Generate webhook URL that includes workspace_id and integration_id
	webhookURL := domain.GenerateWebhookCallbackURL(baseURL, domain.EmailProviderKindSparkPost, workspaceID, integrationID)

	// Map our event types to SparkPost event types
	sparkpostEvents := []string{}
	for _, eventType := range eventTypes {
		switch eventType {
		case domain.EmailEventDelivered:
			sparkpostEvents = append(sparkpostEvents, "delivery")
		case domain.EmailEventBounce:
			sparkpostEvents = append(sparkpostEvents, "bounce")
		case domain.EmailEventComplaint:
			sparkpostEvents = append(sparkpostEvents, "spam_complaint")
		}
	}

	// First check for existing webhooks using the direct SparkPostSettings
	existingWebhooks, err := s.directListWebhooks(ctx, providerConfig.SparkPost)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to list SparkPost webhooks: %v", err))
		return nil, fmt.Errorf("failed to list SparkPost webhooks: %w", err)
	}

	// Check if we already have a webhook with our URL
	var existingWebhook *domain.SparkPostWebhook
	for _, webhook := range existingWebhooks.Results {
		if strings.Contains(webhook.Target, baseURL) &&
			strings.Contains(webhook.Target, fmt.Sprintf("workspace_id=%s", workspaceID)) &&
			strings.Contains(webhook.Target, fmt.Sprintf("integration_id=%s", integrationID)) {
			existingWebhook = &webhook
			break
		}
	}

	var webhookResponse *domain.SparkPostWebhookResponse
	if existingWebhook != nil {
		// Update the webhook with new events
		existingWebhook.Events = sparkpostEvents
		webhookResponse, err = s.directUpdateWebhook(ctx, providerConfig.SparkPost, existingWebhook.ID, *existingWebhook)
		if err != nil {
			return nil, fmt.Errorf("failed to update SparkPost webhook: %w", err)
		}
	} else {
		// Create a new webhook
		newWebhook := domain.SparkPostWebhook{
			Name:     fmt.Sprintf("Notifuse-%s", integrationID),
			Target:   webhookURL,
			Events:   sparkpostEvents,
			Active:   true,
			AuthType: "none",
		}

		webhookResponse, err = s.directCreateWebhook(ctx, providerConfig.SparkPost, newWebhook)
		if err != nil {
			return nil, fmt.Errorf("failed to create SparkPost webhook: %w", err)
		}
	}

	// Create webhook registration status
	status := &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindSparkPost,
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
			WebhookID: webhookResponse.Results.ID,
			URL:       webhookURL,
			EventType: eventType,
			Active:    true,
		})
	}

	return status, nil
}

// directListWebhooks is a helper method that uses SparkPostSettings directly
func (s *SparkPostService) directListWebhooks(ctx context.Context, settings *domain.SparkPostSettings) (*domain.SparkPostWebhookListResponse, error) {

	apiURL := fmt.Sprintf("%s/api/v1/webhooks", settings.Endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for listing SparkPost webhooks: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// SparkPost uses API key in header
	authHeader := fmt.Sprintf("Bearer %s", settings.APIKey)
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for listing SparkPost webhooks: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		responseBody := string(body)
		s.logger.Error(fmt.Sprintf("SparkPost API returned non-OK status code %d: %s", resp.StatusCode, responseBody))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	// Parse the response
	var response domain.SparkPostWebhookListResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode SparkPost webhook list response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// directUpdateWebhook is a helper method that uses SparkPostSettings directly
func (s *SparkPostService) directUpdateWebhook(ctx context.Context, settings *domain.SparkPostSettings, webhookID string, webhook domain.SparkPostWebhook) (*domain.SparkPostWebhookResponse, error) {
	// Construct the API URL
	apiURL := fmt.Sprintf("%s/api/v1/webhooks/%s", settings.Endpoint, webhookID)

	// Prepare the request body
	requestBody, err := json.Marshal(webhook)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to marshal webhook configuration: %v", err))
		return nil, fmt.Errorf("failed to marshal webhook configuration: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, apiURL, bytes.NewBuffer(requestBody))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for updating SparkPost webhook: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// SparkPost uses API key in header
	authHeader := fmt.Sprintf("Bearer %s", settings.APIKey)
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for updating SparkPost webhook: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("SparkPost API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	// Parse the response to get the updated webhook details
	var response domain.SparkPostWebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode SparkPost webhook response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// directCreateWebhook is a helper method that uses SparkPostSettings directly
func (s *SparkPostService) directCreateWebhook(ctx context.Context, settings *domain.SparkPostSettings, webhook domain.SparkPostWebhook) (*domain.SparkPostWebhookResponse, error) {

	apiURL := fmt.Sprintf("%s/api/v1/webhooks", settings.Endpoint)

	// Prepare the request body
	requestBody, err := json.Marshal(webhook)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to marshal webhook configuration: %v", err))
		return nil, fmt.Errorf("failed to marshal webhook configuration: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(requestBody))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for creating SparkPost webhook: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// SparkPost uses API key in header
	authHeader := fmt.Sprintf("Bearer %s", settings.APIKey)
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for creating SparkPost webhook: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("SparkPost API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	// Parse the response to get the created webhook details
	var response domain.SparkPostWebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode SparkPost webhook response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// GetWebhookStatus implements the domain.WebhookProvider interface for SparkPost
func (s *SparkPostService) GetWebhookStatus(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	providerConfig *domain.EmailProvider,
) (*domain.WebhookRegistrationStatus, error) {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.SparkPost == nil || providerConfig.SparkPost.APIKey == "" {
		return nil, fmt.Errorf("SparkPost configuration is missing or invalid")
	}

	// Check if sandbox mode is enabled
	if providerConfig.SparkPost.SandboxMode {
		// In sandbox mode, we don't actually check webhooks with SparkPost
		// but we return a simulated response
		s.logger.Info("SparkPost sandbox mode is enabled, simulating webhook status check")

		webhookURL := domain.GenerateWebhookCallbackURL("https://api.example.com", domain.EmailProviderKindSparkPost, workspaceID, integrationID)

		// Create webhook status
		status := &domain.WebhookRegistrationStatus{
			EmailProviderKind: domain.EmailProviderKindSparkPost,
			IsRegistered:      true,
			Endpoints:         []domain.WebhookEndpointStatus{},
			ProviderDetails: map[string]interface{}{
				"webhook_id":     "sandbox-mode-webhook",
				"integration_id": integrationID,
				"workspace_id":   workspaceID,
				"sandbox_mode":   true,
			},
		}

		// Add endpoints for common event types in sandbox mode
		registeredEventTypes := []domain.EmailEventType{domain.EmailEventDelivered, domain.EmailEventBounce, domain.EmailEventComplaint}
		for _, eventType := range registeredEventTypes {
			status.Endpoints = append(status.Endpoints, domain.WebhookEndpointStatus{
				URL:       webhookURL,
				EventType: eventType,
				Active:    true,
			})
		}

		return status, nil
	}

	// Create webhook status response
	status := &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindSparkPost,
		IsRegistered:      false,
		Endpoints:         []domain.WebhookEndpointStatus{},
		ProviderDetails: map[string]interface{}{
			"integration_id": integrationID,
			"workspace_id":   workspaceID,
		},
	}

	// Get existing webhooks using direct SparkPostSettings
	existingWebhooks, err := s.directListWebhooks(ctx, providerConfig.SparkPost)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to list SparkPost webhooks: %v", err))
		return nil, fmt.Errorf("failed to list SparkPost webhooks: %w", err)
	}

	// Check each webhook in the results for our integrationID
	for _, webhook := range existingWebhooks.Results {
		if strings.Contains(webhook.Target, fmt.Sprintf("workspace_id=%s", workspaceID)) &&
			strings.Contains(webhook.Target, fmt.Sprintf("integration_id=%s", integrationID)) {

			// Add endpoints for registered event types
			registeredEventTypes := []domain.EmailEventType{domain.EmailEventDelivered, domain.EmailEventBounce, domain.EmailEventComplaint}

			for _, eventType := range registeredEventTypes {
				status.Endpoints = append(status.Endpoints, domain.WebhookEndpointStatus{
					WebhookID: webhook.ID,
					URL:       webhook.Target,
					EventType: eventType,
					Active:    true,
				})
			}

			status.IsRegistered = true
			break
		}
	}

	return status, nil
}

// UnregisterWebhooks implements the domain.WebhookProvider interface for SparkPost
func (s *SparkPostService) UnregisterWebhooks(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	providerConfig *domain.EmailProvider,
) error {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.SparkPost == nil || providerConfig.SparkPost.APIKey == "" {
		return fmt.Errorf("SparkPost configuration is missing or invalid")
	}

	// Check if sandbox mode is enabled
	if providerConfig.SparkPost.SandboxMode {
		// In sandbox mode, we don't actually unregister webhooks with SparkPost
		s.logger.Info("SparkPost sandbox mode is enabled, simulating webhook unregistration")
		return nil
	}

	// Get existing webhooks using the direct SparkPostSettings
	existingWebhooks, err := s.directListWebhooks(ctx, providerConfig.SparkPost)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to list SparkPost webhooks: %v", err))
		return fmt.Errorf("failed to list SparkPost webhooks: %w", err)
	}

	// Delete webhooks that match our integration
	var lastError error
	for _, webhook := range existingWebhooks.Results {
		if strings.Contains(webhook.Target, fmt.Sprintf("workspace_id=%s", workspaceID)) &&
			strings.Contains(webhook.Target, fmt.Sprintf("integration_id=%s", integrationID)) {

			err := s.directDeleteWebhook(ctx, providerConfig.SparkPost, webhook.ID)
			if err != nil {
				s.logger.WithField("webhook_id", webhook.ID).
					Error(fmt.Sprintf("Failed to delete SparkPost webhook: %v", err))
				lastError = err
				// Continue deleting other webhooks even if one fails
			} else {
				s.logger.WithField("webhook_id", webhook.ID).
					Info("Successfully deleted SparkPost webhook")
			}
		}
	}

	if lastError != nil {
		return fmt.Errorf("failed to delete one or more SparkPost webhooks: %w", lastError)
	}

	return nil
}

// directDeleteWebhook is a helper method that uses SparkPostSettings directly
func (s *SparkPostService) directDeleteWebhook(ctx context.Context, settings *domain.SparkPostSettings, webhookID string) error {
	// Construct the API URL
	apiURL := fmt.Sprintf("%s/api/v1/webhooks/%s", settings.Endpoint, webhookID)

	// Log webhook ID for debugging
	s.logger = s.logger.WithField("webhook_id", webhookID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiURL, nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for deleting SparkPost webhook: %v", err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	// SparkPost uses API key in header
	authHeader := fmt.Sprintf("Bearer %s", settings.APIKey)
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for deleting SparkPost webhook: %v", err))
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("SparkPost API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	return nil
}

// SendEmail sends an email using SparkPost
func (s *SparkPostService) SendEmail(ctx context.Context, request domain.SendEmailProviderRequest) error {
	// Validate the request
	if err := request.Validate(); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	if request.Provider.SparkPost == nil {
		return fmt.Errorf("SparkPost provider is not configured")
	}

	// Check for sandbox mode
	to := request.To
	if request.Provider.SparkPost.SandboxMode {
		s.logger.Info("SparkPost is in sandbox mode, email will be accepted but not delivered")
		to = to + ".sink.sparkpostmail.com"
	}

	// Prepare the request payload
	type Address struct {
		Email string `json:"email"`
		Name  string `json:"name,omitempty"`
	}

	type Recipient struct {
		Address Address `json:"address"`
	}

	type From struct {
		Name  string `json:"name,omitempty"`
		Email string `json:"email"`
	}

	type Content struct {
		From    From   `json:"from"`
		Subject string `json:"subject"`
		ReplyTo string `json:"reply_to,omitempty"`
		HTML    string `json:"html"`
	}

	type EmailRequest struct {
		Options struct {
			OpenTracking  bool `json:"open_tracking"`
			ClickTracking bool `json:"click_tracking"`
		} `json:"options"`
		Recipients []Recipient            `json:"recipients"`
		Content    Content                `json:"content"`
		Metadata   map[string]interface{} `json:"metadata,omitempty"`
	}

	// Set up the email payload
	emailReq := EmailRequest{
		Recipients: []Recipient{
			{
				Address: Address{
					Email: to,
					Name:  "", // We don't have recipient name in the current function signature
				},
			},
		},
		Content: Content{
			From: From{
				Name:  request.FromName,
				Email: request.FromAddress,
			},
			Subject: request.Subject,
			HTML:    request.Content,
		},
		Metadata: map[string]interface{}{
			"notifuse_message_id": request.MessageID,
		},
	}

	// Tracking should be disabled as we already do it
	emailReq.Options.OpenTracking = false
	emailReq.Options.ClickTracking = false

	// Add replyTo if specified
	if request.EmailOptions.ReplyTo != "" {
		emailReq.Content.ReplyTo = request.EmailOptions.ReplyTo
	}

	// Add CC recipients if specified
	for _, ccAddress := range request.EmailOptions.CC {
		if ccAddress != "" {
			emailReq.Recipients = append(emailReq.Recipients, Recipient{
				Address: Address{
					Email: ccAddress,
				},
			})
		}
	}

	// Add BCC recipients if specified
	for _, bccAddress := range request.EmailOptions.BCC {
		if bccAddress != "" {
			emailReq.Recipients = append(emailReq.Recipients, Recipient{
				Address: Address{
					Email: bccAddress,
				},
			})
		}
	}

	// Convert to JSON
	jsonData, err := json.Marshal(emailReq)
	if err != nil {
		return fmt.Errorf("failed to marshal email request: %w", err)
	}

	// Construct the API URL
	endpoint := request.Provider.SparkPost.Endpoint
	if endpoint == "" {
		endpoint = "https://api.sparkpost.com"
	}
	apiURL := fmt.Sprintf("%s/api/v1/transmissions", endpoint)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for sending SparkPost email: %v", err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", request.Provider.SparkPost.APIKey))
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for sending SparkPost email: %v", err))
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("SparkPost API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	return nil
}
