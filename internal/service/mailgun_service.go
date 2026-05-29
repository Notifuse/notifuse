package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// MailgunService implements the domain.MailgunServiceInterface
type MailgunService struct {
	httpClient      domain.HTTPClient
	authService     domain.AuthService
	logger          logger.Logger
	webhookEndpoint string
}

// NewMailgunService creates a new instance of MailgunService
func NewMailgunService(httpClient domain.HTTPClient, authService domain.AuthService, logger logger.Logger, webhookEndpoint string) *MailgunService {
	return &MailgunService{
		httpClient:      httpClient,
		authService:     authService,
		logger:          logger,
		webhookEndpoint: webhookEndpoint,
	}
}

// listAllWebhooks retrieves every webhook registered for a domain WITHOUT filtering
// to Notifuse's own callbacks. RegisterWebhooks/UnregisterWebhooks need the complete
// picture so they can coexist with other consumers of the same Mailgun domain.
//
// Each event entry is normalized so that whether Mailgun returns the callbacks as a
// single "url" string or as a "urls" array, the result is exposed via URLs.
func (s *MailgunService) listAllWebhooks(ctx context.Context, config domain.MailgunSettings) (*domain.MailgunWebhookListResponse, error) {

	// Construct the API URL
	endpoint := ""
	if strings.ToLower(config.Region) == "eu" {
		endpoint = "https://api.eu.mailgun.net/v3"
	} else {
		endpoint = "https://api.mailgun.net/v3"
	}

	// Format according to Mailgun API documentation: https://api.mailgun.net/v3/domains/{domain}/webhooks
	apiURL := fmt.Sprintf("%s/domains/%s/webhooks", endpoint, config.Domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for listing Mailgun webhooks: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth("api", config.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for listing Mailgun webhooks: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Mailgun API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	// Parse the response
	var response domain.MailgunWebhookListResponse

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode Mailgun webhook list response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Normalize both wire forms ("url" string / "urls" array) into URLs.
	response.Webhooks.Delivered = domain.MailgunUrls{URLs: response.Webhooks.Delivered.All()}
	response.Webhooks.PermanentFail = domain.MailgunUrls{URLs: response.Webhooks.PermanentFail.All()}
	response.Webhooks.TemporaryFail = domain.MailgunUrls{URLs: response.Webhooks.TemporaryFail.All()}
	response.Webhooks.Complained = domain.MailgunUrls{URLs: response.Webhooks.Complained.All()}

	return &response, nil
}

// ListWebhooks retrieves the webhooks registered for a domain, keeping only the URLs
// that point at this Notifuse instance's webhook endpoint.
func (s *MailgunService) ListWebhooks(ctx context.Context, config domain.MailgunSettings) (*domain.MailgunWebhookListResponse, error) {
	response, err := s.listAllWebhooks(ctx, config)
	if err != nil {
		return nil, err
	}

	// only keep URLs that contain the webhookEndpoint
	response.Webhooks.Delivered.URLs = s.filterOwnEndpointURLs(response.Webhooks.Delivered.URLs)
	response.Webhooks.PermanentFail.URLs = s.filterOwnEndpointURLs(response.Webhooks.PermanentFail.URLs)
	response.Webhooks.TemporaryFail.URLs = s.filterOwnEndpointURLs(response.Webhooks.TemporaryFail.URLs)
	response.Webhooks.Complained.URLs = s.filterOwnEndpointURLs(response.Webhooks.Complained.URLs)

	return response, nil
}

// filterOwnEndpointURLs returns only the URLs pointing at this instance's webhook endpoint.
func (s *MailgunService) filterOwnEndpointURLs(urls []string) []string {
	filtered := []string{}
	for _, u := range urls {
		if strings.Contains(u, s.webhookEndpoint) {
			filtered = append(filtered, u)
		}
	}
	return filtered
}

// CreateWebhook creates a new webhook
func (s *MailgunService) CreateWebhook(ctx context.Context, config domain.MailgunSettings, webhook domain.MailgunWebhook) (*domain.MailgunWebhook, error) {

	if len(webhook.Events) == 0 {
		return nil, fmt.Errorf("at least one event type is required")
	}

	// Mailgun API requires a separate call for each event type
	// We'll use the first event type in the list
	eventType := webhook.Events[0]

	// Construct the API URL
	endpoint := ""
	if strings.ToLower(config.Region) == "eu" {
		endpoint = "https://api.eu.mailgun.net/v3"
	} else {
		endpoint = "https://api.mailgun.net/v3"
	}

	apiURL := fmt.Sprintf("%s/domains/%s/webhooks", endpoint, config.Domain)

	// Create the form data
	form := url.Values{}
	form.Add("id", eventType)
	form.Add("url", webhook.URL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for creating Mailgun webhook: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth("api", config.APIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for creating Mailgun webhook: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Mailgun API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	// Parse the response
	var response struct {
		Message string                 `json:"message"`
		Webhook map[string]interface{} `json:"webhook"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode Mailgun webhook response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Return the created webhook
	createdWebhook := &domain.MailgunWebhook{
		ID:     eventType,
		URL:    webhook.URL,
		Events: []string{eventType},
		Active: true,
	}

	return createdWebhook, nil
}

// GetWebhook retrieves a webhook by ID
func (s *MailgunService) GetWebhook(ctx context.Context, config domain.MailgunSettings, webhookID string) (*domain.MailgunWebhook, error) {

	// Construct the API URL
	endpoint := ""
	if strings.ToLower(config.Region) == "eu" {
		endpoint = "https://api.eu.mailgun.net/v3"
	} else {
		endpoint = "https://api.mailgun.net/v3"
	}

	apiURL := fmt.Sprintf("%s/domains/%s/webhooks/%s", endpoint, config.Domain, webhookID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for getting Mailgun webhook: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth("api", config.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for getting Mailgun webhook: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Mailgun API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	// Parse the response
	var response struct {
		Webhook map[string]interface{} `json:"webhook"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode Mailgun webhook response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract webhook details
	url, _ := response.Webhook["url"].(string)
	active, _ := response.Webhook["active"].(bool)

	// Return the webhook
	webhook := &domain.MailgunWebhook{
		ID:     webhookID,
		URL:    url,
		Events: []string{webhookID},
		Active: active,
	}

	return webhook, nil
}

// UpdateWebhook updates an existing webhook
func (s *MailgunService) UpdateWebhook(ctx context.Context, config domain.MailgunSettings, webhookID string, webhook domain.MailgunWebhook) (*domain.MailgunWebhook, error) {

	// Construct the API URL
	endpoint := ""
	if strings.ToLower(config.Region) == "eu" {
		endpoint = "https://api.eu.mailgun.net/v3"
	} else {
		endpoint = "https://api.mailgun.net/v3"
	}

	apiURL := fmt.Sprintf("%s/domains/%s/webhooks/%s", endpoint, config.Domain, webhookID)

	// Create the form data. Mailgun's v3 PUT replaces the event's callback list, and
	// accepts multiple callbacks as repeated "url" fields (matching the official
	// mailgun-go SDK). Send webhook.URLs when provided, else the single webhook.URL.
	form := url.Values{}
	urls := webhook.URLs
	if len(urls) == 0 {
		urls = []string{webhook.URL}
	}
	for _, u := range urls {
		form.Add("url", u)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for updating Mailgun webhook: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth("api", config.APIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for updating Mailgun webhook: %v", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Mailgun API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return nil, fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	// Parse the response
	var response struct {
		Message string                 `json:"message"`
		Webhook map[string]interface{} `json:"webhook"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to decode Mailgun webhook response: %v", err))
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Return the updated webhook
	updatedWebhook := &domain.MailgunWebhook{
		ID:     webhookID,
		URL:    webhook.URL,
		URLs:   urls,
		Events: []string{webhookID},
		Active: true,
	}

	return updatedWebhook, nil
}

// DeleteWebhook deletes a webhook by ID
func (s *MailgunService) DeleteWebhook(ctx context.Context, config domain.MailgunSettings, webhookID string) error {

	// Construct the API URL
	endpoint := ""
	if strings.ToLower(config.Region) == "eu" {
		endpoint = "https://api.eu.mailgun.net/v3"
	} else {
		endpoint = "https://api.mailgun.net/v3"
	}

	apiURL := fmt.Sprintf("%s/domains/%s/webhooks/%s", endpoint, config.Domain, webhookID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiURL, nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for deleting Mailgun webhook: %v", err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth("api", config.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for deleting Mailgun webhook: %v", err))
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Mailgun API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	return nil
}

// TestWebhook sends a test event to a webhook
func (s *MailgunService) TestWebhook(ctx context.Context, config domain.MailgunSettings, webhookID string, eventType string) error {
	// Mailgun doesn't support testing webhooks directly through their API
	// We could potentially simulate a webhook event, but that's beyond the scope
	// of this implementation
	return fmt.Errorf("testing webhooks is not supported by the Mailgun API")
}

// RegisterWebhooks implements the domain.WebhookProvider interface for Mailgun
func (s *MailgunService) RegisterWebhooks(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	baseURL string,
	eventTypes []domain.EmailEventType,
	providerConfig *domain.EmailProvider,
) (*domain.WebhookRegistrationStatus, error) {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.Mailgun == nil ||
		providerConfig.Mailgun.APIKey == "" || providerConfig.Mailgun.Domain == "" {
		return nil, fmt.Errorf("mailgun configuration is missing or invalid")
	}

	// Generate webhook URL that includes workspace_id and integration_id
	webhookURL := domain.GenerateWebhookCallbackURL(baseURL, domain.EmailProviderKindMailgun, workspaceID, integrationID)

	// Map our event types to Mailgun event types
	mailgunEvents := make(map[string]bool)

	for _, eventType := range eventTypes {
		switch eventType {
		case domain.EmailEventDelivered:
			mailgunEvents["delivered"] = true
		case domain.EmailEventBounce:
			mailgunEvents["permanent_fail"] = true
			mailgunEvents["temporary_fail"] = true
		case domain.EmailEventComplaint:
			mailgunEvents["complained"] = true
		}
	}

	// Get the full, unfiltered set of existing webhooks so we coexist with other
	// consumers of the same Mailgun domain (Mailgun allows up to 3 URLs per event).
	existingWebhooks, err := s.listAllWebhooks(ctx, *providerConfig.Mailgun)
	if err != nil {
		return nil, fmt.Errorf("failed to list Mailgun webhooks: %w", err)
	}

	existingByEvent := map[string][]string{
		"delivered":      existingWebhooks.Webhooks.Delivered.URLs,
		"permanent_fail": existingWebhooks.Webhooks.PermanentFail.URLs,
		"temporary_fail": existingWebhooks.Webhooks.TemporaryFail.URLs,
		"complained":     existingWebhooks.Webhooks.Complained.URLs,
	}

	// Iterate desired events in a stable order (map ranging is non-deterministic).
	events := make([]string, 0, len(mailgunEvents))
	for eventType := range mailgunEvents {
		events = append(events, eventType)
	}
	sort.Strings(events)

	// Plan pass: decide an action per event and fail fast — before mutating anything —
	// if merging our URL would exceed Mailgun's 3-URL-per-event limit.
	type webhookAction struct {
		eventType string
		method    string // "create" (POST), "update" (PUT), or "noop"
		urls      []string
	}
	actions := make([]webhookAction, 0, len(events))
	for _, eventType := range events {
		existing := existingByEvent[eventType]

		// Keep every URL that isn't ours; drop our own (possibly stale) URLs so a
		// changed base URL replaces the old entry instead of accumulating duplicates.
		coTenants := []string{}
		for _, u := range existing {
			if !isOwnedMailgunURL(u, workspaceID, integrationID) {
				coTenants = append(coTenants, u)
			}
		}

		merged := dedupeStrings(append(append([]string{}, coTenants...), webhookURL))
		if len(merged) > maxMailgunURLsPerEvent {
			return nil, fmt.Errorf(
				"cannot register Mailgun webhook for event %q: it already has %d URLs from other services and Mailgun's limit is %d — free a slot in the Mailgun dashboard and retry",
				eventType, len(coTenants), maxMailgunURLsPerEvent)
		}

		switch {
		case len(existing) == 0:
			// No webhook exists for this event yet — POST creates it.
			actions = append(actions, webhookAction{eventType: eventType, method: "create", urls: []string{webhookURL}})
		case sameStringSet(existing, merged):
			// Our URL is already registered and nothing else changed.
			actions = append(actions, webhookAction{eventType: eventType, method: "noop", urls: merged})
		default:
			// The event already exists (POST would 400) — PUT the merged list.
			actions = append(actions, webhookAction{eventType: eventType, method: "update", urls: merged})
		}
	}

	// Apply pass.
	endpoints := []domain.WebhookEndpointStatus{}
	providerDetails := map[string]interface{}{
		"integration_id": integrationID,
		"workspace_id":   workspaceID,
	}

	for _, action := range actions {
		switch action.method {
		case "create":
			if _, err := s.CreateWebhook(ctx, *providerConfig.Mailgun, domain.MailgunWebhook{
				URL:    webhookURL,
				Events: []string{action.eventType},
				Active: true,
			}); err != nil {
				return nil, fmt.Errorf("failed to create Mailgun webhook for event %s: %w", action.eventType, err)
			}
		case "update":
			if _, err := s.UpdateWebhook(ctx, *providerConfig.Mailgun, action.eventType, domain.MailgunWebhook{
				URL:    webhookURL,
				URLs:   action.urls,
				Events: []string{action.eventType},
				Active: true,
			}); err != nil {
				return nil, fmt.Errorf("failed to update Mailgun webhook for event %s: %w", action.eventType, err)
			}
		case "noop":
			// Our URL is already registered for this event; nothing to do.
		}

		endpoints = append(endpoints, domain.WebhookEndpointStatus{
			WebhookID: action.eventType,
			URL:       webhookURL,
			EventType: mapMailgunEventType(action.eventType),
			Active:    true,
		})
	}

	// Create webhook registration status
	status := &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindMailgun,
		IsRegistered:      len(endpoints) > 0,
		Endpoints:         endpoints,
		ProviderDetails:   providerDetails,
	}

	return status, nil
}

// GetWebhookStatus implements the domain.WebhookProvider interface for Mailgun
func (s *MailgunService) GetWebhookStatus(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	providerConfig *domain.EmailProvider,
) (*domain.WebhookRegistrationStatus, error) {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.Mailgun == nil ||
		providerConfig.Mailgun.APIKey == "" || providerConfig.Mailgun.Domain == "" {
		return nil, fmt.Errorf("mailgun configuration is missing or invalid")
	}

	// Create webhook status response
	status := &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindMailgun,
		IsRegistered:      false,
		Endpoints:         []domain.WebhookEndpointStatus{},
		ProviderDetails: map[string]interface{}{
			"integration_id": integrationID,
			"workspace_id":   workspaceID,
		},
	}

	// Get existing webhooks
	existingWebhooks, err := s.ListWebhooks(ctx, *providerConfig.Mailgun)
	if err != nil {
		return nil, fmt.Errorf("failed to list Mailgun webhooks: %w", err)
	}

	// Check for webhooks that match our integration
	registeredEventMap := make(map[domain.EmailEventType]bool)

	// Check for webhooks that match our integration
	for eventType, urls := range map[string]domain.MailgunUrls{
		"delivered":      existingWebhooks.Webhooks.Delivered,
		"permanent_fail": existingWebhooks.Webhooks.PermanentFail,
		"temporary_fail": existingWebhooks.Webhooks.TemporaryFail,
		"complained":     existingWebhooks.Webhooks.Complained,
	} {
		for _, url := range urls.URLs {
			if strings.Contains(url, fmt.Sprintf("workspace_id=%s", workspaceID)) &&
				strings.Contains(url, fmt.Sprintf("integration_id=%s", integrationID)) {

				status.IsRegistered = true

				// Add endpoint
				status.Endpoints = append(status.Endpoints, domain.WebhookEndpointStatus{
					WebhookID: eventType,
					URL:       url,
					EventType: mapMailgunEventType(eventType), // In Mailgun, the ID is the event type
					Active:    true,                           // Assume active if listed
				})

				// Track registered event types
				eventType := mapMailgunEventType(eventType)
				if eventType != "" {
					registeredEventMap[eventType] = true
				}
			}
		}
	}

	return status, nil
}

// UnregisterWebhooks implements the domain.WebhookProvider interface for Mailgun
func (s *MailgunService) UnregisterWebhooks(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	providerConfig *domain.EmailProvider,
) error {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.Mailgun == nil ||
		providerConfig.Mailgun.APIKey == "" || providerConfig.Mailgun.Domain == "" {
		return fmt.Errorf("mailgun configuration is missing or invalid")
	}

	// Get the full, unfiltered set of existing webhooks so we only remove our own URLs
	// and preserve any belonging to other consumers of the same Mailgun domain.
	existingWebhooks, err := s.listAllWebhooks(ctx, *providerConfig.Mailgun)
	if err != nil {
		return fmt.Errorf("failed to list Mailgun webhooks: %w", err)
	}

	var lastError error

	// For each event, strip our URL(s); keep everyone else's.
	for eventType, urls := range map[string][]string{
		"delivered":      existingWebhooks.Webhooks.Delivered.URLs,
		"permanent_fail": existingWebhooks.Webhooks.PermanentFail.URLs,
		"temporary_fail": existingWebhooks.Webhooks.TemporaryFail.URLs,
		"complained":     existingWebhooks.Webhooks.Complained.URLs,
	} {
		ownsAny := false
		remaining := []string{}
		for _, u := range urls {
			if isOwnedMailgunURL(u, workspaceID, integrationID) {
				ownsAny = true
			} else {
				remaining = append(remaining, u)
			}
		}

		if !ownsAny {
			// Nothing of ours on this event — leave it untouched.
			continue
		}

		if len(remaining) == 0 {
			// Only our URL(s) were registered — remove the whole event webhook.
			if err := s.DeleteWebhook(ctx, *providerConfig.Mailgun, eventType); err != nil {
				s.logger.WithField("webhook_id", eventType).
					Error(fmt.Sprintf("Failed to delete Mailgun webhook: %v", err))
				lastError = err
			} else {
				s.logger.WithField("webhook_id", eventType).
					Info("Successfully deleted Mailgun webhook")
			}
			continue
		}

		// Other consumers still use this event — keep their URLs, drop only ours.
		if _, err := s.UpdateWebhook(ctx, *providerConfig.Mailgun, eventType, domain.MailgunWebhook{
			URLs:   remaining,
			Events: []string{eventType},
			Active: true,
		}); err != nil {
			s.logger.WithField("webhook_id", eventType).
				Error(fmt.Sprintf("Failed to update Mailgun webhook while unregistering: %v", err))
			lastError = err
		} else {
			s.logger.WithField("webhook_id", eventType).
				Info("Removed Notifuse URL from Mailgun webhook, preserved other consumers")
		}
	}

	if lastError != nil {
		return fmt.Errorf("failed to unregister one or more Mailgun webhooks: %w", lastError)
	}

	return nil
}

// Helper function to map Mailgun event types to our domain event types
func mapMailgunEventType(eventType string) domain.EmailEventType {
	switch eventType {
	case "delivered":
		return domain.EmailEventDelivered
	case "permanent_fail", "temporary_fail":
		return domain.EmailEventBounce
	case "complained":
		return domain.EmailEventComplaint
	default:
		return ""
	}
}

// maxMailgunURLsPerEvent is Mailgun's hard limit on callback URLs per event type.
const maxMailgunURLsPerEvent = 3

// isOwnedMailgunURL reports whether a registered webhook URL belongs to this
// workspace + integration (i.e. one Notifuse registered, identified by query params).
func isOwnedMailgunURL(url, workspaceID, integrationID string) bool {
	return strings.Contains(url, fmt.Sprintf("workspace_id=%s", workspaceID)) &&
		strings.Contains(url, fmt.Sprintf("integration_id=%s", integrationID))
}

// sameStringSet reports whether a and b contain the same set of strings (ignoring
// order and duplicates).
func sameStringSet(a, b []string) bool {
	setA := make(map[string]bool)
	for _, s := range a {
		setA[s] = true
	}
	setB := make(map[string]bool)
	for _, s := range b {
		setB[s] = true
	}
	if len(setA) != len(setB) {
		return false
	}
	for s := range setA {
		if !setB[s] {
			return false
		}
	}
	return true
}

// SendEmail sends an email using Mailgun
func (s *MailgunService) SendEmail(ctx context.Context, request domain.SendEmailProviderRequest) error {
	// Validate the request
	if err := request.Validate(); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	if request.Provider.Mailgun == nil {
		return fmt.Errorf("mailgun provider is not configured")
	}

	// Determine endpoint based on region
	endpoint := ""
	if strings.ToLower(request.Provider.Mailgun.Region) == "eu" {
		endpoint = "https://api.eu.mailgun.net/v3"
	} else {
		endpoint = "https://api.mailgun.net/v3"
	}

	// Format the API URL
	apiURL := fmt.Sprintf("%s/%s/messages", endpoint, request.Provider.Mailgun.Domain)

	// If there are attachments, use multipart/form-data
	// https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/messages/post-v3--domain-name--messages
	// "Important: You must use multipart/form-data encoding for sending attachments"
	if len(request.EmailOptions.Attachments) > 0 {
		return s.sendEmailWithAttachments(ctx, apiURL, request)
	}

	// For emails without attachments, use application/x-www-form-urlencoded
	return s.sendEmailSimple(ctx, apiURL, request)
}

// sendEmailSimple sends an email without attachments using application/x-www-form-urlencoded
func (s *MailgunService) sendEmailSimple(ctx context.Context, apiURL string, request domain.SendEmailProviderRequest) error {
	// Create the form data for the email
	form := url.Values{}
	form.Add("from", fmt.Sprintf("%s <%s>", request.FromName, request.FromAddress))
	form.Add("to", request.To)
	form.Add("subject", request.Subject)
	form.Add("html", request.Content)

	// Add cc recipients if provided
	for _, ccAddress := range request.EmailOptions.CC {
		if ccAddress != "" {
			form.Add("cc", ccAddress)
		}
	}

	// Add bcc recipients if provided
	for _, bccAddress := range request.EmailOptions.BCC {
		if bccAddress != "" {
			form.Add("bcc", bccAddress)
		}
	}

	// Add reply-to if provided
	if request.EmailOptions.ReplyTo != "" {
		form.Add("h:Reply-To", request.EmailOptions.ReplyTo)
	}

	// Add RFC-8058 List-Unsubscribe headers for one-click unsubscribe
	if request.EmailOptions.ListUnsubscribeURL != "" {
		form.Add("h:List-Unsubscribe", fmt.Sprintf("<%s>", request.EmailOptions.ListUnsubscribeURL))
		form.Add("h:List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
	}

	// Add messageID as a custom variable for tracking
	form.Add("v:notifuse_message_id", request.MessageID)

	// Create the request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for sending Mailgun email: %v", err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set basic auth header
	req.SetBasicAuth("api", request.Provider.Mailgun.APIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send the request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for sending Mailgun email: %v", err))
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Mailgun API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	return nil
}

// sendEmailWithAttachments sends an email with attachments using multipart/form-data
func (s *MailgunService) sendEmailWithAttachments(ctx context.Context, apiURL string, request domain.SendEmailProviderRequest) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add basic fields
	if err := writer.WriteField("from", fmt.Sprintf("%s <%s>", request.FromName, request.FromAddress)); err != nil {
		return fmt.Errorf("failed to write from field: %w", err)
	}
	if err := writer.WriteField("to", request.To); err != nil {
		return fmt.Errorf("failed to write to field: %w", err)
	}
	if err := writer.WriteField("subject", request.Subject); err != nil {
		return fmt.Errorf("failed to write subject field: %w", err)
	}
	if err := writer.WriteField("html", request.Content); err != nil {
		return fmt.Errorf("failed to write html field: %w", err)
	}

	// Add cc recipients if provided
	for _, ccAddress := range request.EmailOptions.CC {
		if ccAddress != "" {
			if err := writer.WriteField("cc", ccAddress); err != nil {
				return fmt.Errorf("failed to write cc field: %w", err)
			}
		}
	}

	// Add bcc recipients if provided
	for _, bccAddress := range request.EmailOptions.BCC {
		if bccAddress != "" {
			if err := writer.WriteField("bcc", bccAddress); err != nil {
				return fmt.Errorf("failed to write bcc field: %w", err)
			}
		}
	}

	// Add reply-to if provided
	if request.EmailOptions.ReplyTo != "" {
		if err := writer.WriteField("h:Reply-To", request.EmailOptions.ReplyTo); err != nil {
			return fmt.Errorf("failed to write reply-to field: %w", err)
		}
	}

	// Add RFC-8058 List-Unsubscribe headers for one-click unsubscribe
	if request.EmailOptions.ListUnsubscribeURL != "" {
		if err := writer.WriteField("h:List-Unsubscribe", fmt.Sprintf("<%s>", request.EmailOptions.ListUnsubscribeURL)); err != nil {
			return fmt.Errorf("failed to write list-unsubscribe field: %w", err)
		}
		if err := writer.WriteField("h:List-Unsubscribe-Post", "List-Unsubscribe=One-Click"); err != nil {
			return fmt.Errorf("failed to write list-unsubscribe-post field: %w", err)
		}
	}

	// Add messageID as a custom variable for tracking
	if err := writer.WriteField("v:notifuse_message_id", request.MessageID); err != nil {
		return fmt.Errorf("failed to write message id field: %w", err)
	}

	// Add attachments
	for i, att := range request.EmailOptions.Attachments {
		content, err := att.DecodeContent()
		if err != nil {
			return fmt.Errorf("attachment %d: failed to decode content: %w", i, err)
		}

		// Determine the field name based on disposition
		fieldName := "attachment"
		if att.Disposition == "inline" {
			fieldName = "inline"
		}

		// Create form file
		part, err := writer.CreateFormFile(fieldName, att.Filename)
		if err != nil {
			return fmt.Errorf("attachment %d: failed to create form file: %w", i, err)
		}

		// Write the file content
		if _, err := part.Write(content); err != nil {
			return fmt.Errorf("attachment %d: failed to write content: %w", i, err)
		}

		// Log size for debugging
		s.logger.WithField("attachment_size", len(content)).
			WithField("filename", att.Filename).
			WithField("disposition", att.Disposition).
			Debug("Added attachment to Mailgun email")
	}

	// Close the multipart writer
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create the request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, &buf)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create request for sending Mailgun email: %v", err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set basic auth header
	req.SetBasicAuth("api", request.Provider.Mailgun.APIKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send the request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to execute request for sending Mailgun email: %v", err))
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error(fmt.Sprintf("Mailgun API returned non-OK status code %d: %s", resp.StatusCode, string(body)))
		return fmt.Errorf("API returned non-OK status code %d", resp.StatusCode)
	}

	return nil
}
