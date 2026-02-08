package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// WebhookDeliveryWorker processes pending webhook deliveries
type WebhookDeliveryWorker struct {
	subscriptionRepo domain.WebhookSubscriptionRepository
	deliveryRepo     domain.WebhookDeliveryRepository
	workspaceRepo    domain.WorkspaceRepository
	logger           logger.Logger
	httpClient       *http.Client
	pollInterval     time.Duration
	batchSize        int
	lastCleanupTime  time.Time
	cleanupInterval  time.Duration
	retentionDays    int
}

// Aggressive retry delays as per Standard Webhooks spec
var retryDelays = []time.Duration{
	30 * time.Second,
	1 * time.Minute,
	2 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	30 * time.Minute,
	1 * time.Hour,
	2 * time.Hour,
	6 * time.Hour,
	24 * time.Hour,
}

// NewWebhookDeliveryWorker creates a new webhook delivery worker
func NewWebhookDeliveryWorker(
	subscriptionRepo domain.WebhookSubscriptionRepository,
	deliveryRepo domain.WebhookDeliveryRepository,
	workspaceRepo domain.WorkspaceRepository,
	logger logger.Logger,
	httpClient *http.Client,
) *WebhookDeliveryWorker {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	return &WebhookDeliveryWorker{
		subscriptionRepo: subscriptionRepo,
		deliveryRepo:     deliveryRepo,
		workspaceRepo:    workspaceRepo,
		logger:           logger,
		httpClient:       httpClient,
		pollInterval:     10 * time.Second,
		batchSize:        100,
		cleanupInterval:  1 * time.Hour,
		retentionDays:    7,
	}
}

// Start starts the webhook delivery worker
func (w *WebhookDeliveryWorker) Start(ctx context.Context) {
	w.logger.Info("Webhook delivery worker started")

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Webhook delivery worker stopping...")
			return
		case <-ticker.C:
			w.processDeliveries(ctx)
		}
	}
}

// processDeliveries processes pending deliveries across all workspaces
func (w *WebhookDeliveryWorker) processDeliveries(ctx context.Context) {
	// Run cleanup of old deliveries (method handles timing internally)
	w.cleanupOldDeliveries(ctx)

	// Get all workspaces
	workspaces, err := w.workspaceRepo.List(ctx)
	if err != nil {
		w.logger.WithField("error", err.Error()).Error("Failed to list workspaces for webhook processing")
		return
	}

	for _, workspace := range workspaces {
		if err := w.processWorkspaceDeliveries(ctx, workspace.ID); err != nil {
			w.logger.WithFields(map[string]interface{}{
				"workspace_id": workspace.ID,
				"error":        err.Error(),
			}).Error("Failed to process webhook deliveries for workspace")
		}
	}
}

// processWorkspaceDeliveries processes pending deliveries for a specific workspace
func (w *WebhookDeliveryWorker) processWorkspaceDeliveries(ctx context.Context, workspaceID string) error {
	// Get pending deliveries
	deliveries, err := w.deliveryRepo.GetPendingForWorkspace(ctx, workspaceID, w.batchSize)
	if err != nil {
		return fmt.Errorf("failed to get pending deliveries: %w", err)
	}

	if len(deliveries) == 0 {
		return nil
	}

	w.logger.WithFields(map[string]interface{}{
		"workspace_id": workspaceID,
		"count":        len(deliveries),
	}).Debug("Processing webhook deliveries")

	// Cache subscriptions to avoid repeated lookups
	subscriptionCache := make(map[string]*domain.WebhookSubscription)

	for _, delivery := range deliveries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Get or cache subscription
			sub, ok := subscriptionCache[delivery.SubscriptionID]
			if !ok {
				sub, err = w.subscriptionRepo.GetByID(ctx, workspaceID, delivery.SubscriptionID)
				if err != nil {
					w.logger.WithFields(map[string]interface{}{
						"delivery_id":     delivery.ID,
						"subscription_id": delivery.SubscriptionID,
						"error":           err.Error(),
					}).Error("Failed to get subscription for delivery")
					continue
				}
				subscriptionCache[delivery.SubscriptionID] = sub
			}

			// Skip if subscription is disabled
			if !sub.Enabled {
				continue
			}

			// Process the delivery
			w.processDelivery(ctx, workspaceID, delivery, sub)
		}
	}

	return nil
}

// cleanupOldDeliveries removes webhook deliveries older than the retention period
func (w *WebhookDeliveryWorker) cleanupOldDeliveries(ctx context.Context) {
	// Skip if not enough time has passed since last cleanup
	if time.Since(w.lastCleanupTime) < w.cleanupInterval {
		return
	}
	w.lastCleanupTime = time.Now()

	workspaces, err := w.workspaceRepo.List(ctx)
	if err != nil {
		w.logger.WithField("error", err.Error()).Error("Failed to list workspaces for webhook cleanup")
		return
	}

	for _, workspace := range workspaces {
		deleted, err := w.deliveryRepo.CleanupOldDeliveries(ctx, workspace.ID, w.retentionDays)
		if err != nil {
			w.logger.WithFields(map[string]interface{}{
				"workspace_id": workspace.ID,
				"error":        err.Error(),
			}).Error("Failed to cleanup old webhook deliveries")
			continue
		}
		if deleted > 0 {
			w.logger.WithFields(map[string]interface{}{
				"workspace_id": workspace.ID,
				"deleted":      deleted,
			}).Info("Cleaned up old webhook deliveries")
		}
	}
}

// processDelivery sends a single webhook delivery
func (w *WebhookDeliveryWorker) processDelivery(ctx context.Context, workspaceID string, delivery *domain.WebhookDelivery, sub *domain.WebhookSubscription) {
	// Build the full payload envelope
	envelope := map[string]interface{}{
		"id":           delivery.ID,
		"type":         delivery.EventType,
		"workspace_id": workspaceID,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"data":         delivery.Payload,
	}

	payloadBytes, err := json.Marshal(envelope)
	if err != nil {
		w.logger.WithFields(map[string]interface{}{
			"delivery_id": delivery.ID,
			"error":       err.Error(),
		}).Error("Failed to marshal webhook payload")
		return
	}

	// Generate timestamp for signing
	timestamp := time.Now().Unix()

	// Sign the payload using Standard Webhooks spec
	signature := signPayload(delivery.ID, timestamp, payloadBytes, []byte(sub.Secret))

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		w.logger.WithFields(map[string]interface{}{
			"delivery_id": delivery.ID,
			"error":       err.Error(),
		}).Error("Failed to create webhook request")
		return
	}

	// Set Standard Webhooks headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("webhook-id", delivery.ID)
	req.Header.Set("webhook-timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("webhook-signature", signature)

	// Send the request
	resp, err := w.httpClient.Do(req)
	if err != nil {
		w.handleDeliveryFailure(ctx, workspaceID, delivery, sub, nil, "", err.Error())
		return
	}
	defer resp.Body.Close()

	// Read response body (limit to 1KB)
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	responseBody := string(bodyBytes)

	// Check if successful (2xx status code)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		w.handleDeliverySuccess(ctx, workspaceID, delivery, sub, resp.StatusCode, responseBody)
	} else {
		w.handleDeliveryFailure(ctx, workspaceID, delivery, sub, &resp.StatusCode, responseBody, fmt.Sprintf("HTTP %d", resp.StatusCode))
	}
}

// handleDeliverySuccess marks a delivery as successful
func (w *WebhookDeliveryWorker) handleDeliverySuccess(ctx context.Context, workspaceID string, delivery *domain.WebhookDelivery, sub *domain.WebhookSubscription, statusCode int, responseBody string) {
	now := time.Now().UTC()

	// Mark delivery as delivered
	if err := w.deliveryRepo.MarkDelivered(ctx, workspaceID, delivery.ID, statusCode, responseBody); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"delivery_id": delivery.ID,
			"error":       err.Error(),
		}).Error("Failed to mark delivery as delivered")
		return
	}

	// Update last delivery timestamp
	if err := w.subscriptionRepo.UpdateLastDeliveryAt(ctx, workspaceID, sub.ID, now); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"subscription_id": sub.ID,
			"error":           err.Error(),
		}).Error("Failed to update last delivery timestamp")
	}

	w.logger.WithFields(map[string]interface{}{
		"delivery_id":     delivery.ID,
		"subscription_id": sub.ID,
		"status_code":     statusCode,
	}).Debug("Webhook delivered successfully")
}

// handleDeliveryFailure handles a failed delivery attempt
func (w *WebhookDeliveryWorker) handleDeliveryFailure(ctx context.Context, workspaceID string, delivery *domain.WebhookDelivery, sub *domain.WebhookSubscription, statusCode *int, responseBody, errorMsg string) {
	attempts := delivery.Attempts + 1

	// Check if we've exceeded max attempts
	if attempts >= delivery.MaxAttempts {
		// Mark as permanently failed
		if err := w.deliveryRepo.MarkFailed(ctx, workspaceID, delivery.ID, attempts, errorMsg, statusCode, &responseBody); err != nil {
			w.logger.WithFields(map[string]interface{}{
				"delivery_id": delivery.ID,
				"error":       err.Error(),
			}).Error("Failed to mark delivery as permanently failed")
			return
		}

		w.logger.WithFields(map[string]interface{}{
			"delivery_id":     delivery.ID,
			"subscription_id": sub.ID,
			"attempts":        attempts,
			"error":           errorMsg,
		}).Warn("Webhook delivery permanently failed after max retries")
		return
	}

	// Calculate next retry time
	delayIndex := attempts - 1
	if delayIndex >= len(retryDelays) {
		delayIndex = len(retryDelays) - 1
	}
	nextAttempt := time.Now().UTC().Add(retryDelays[delayIndex])

	// Schedule retry
	if err := w.deliveryRepo.ScheduleRetry(ctx, workspaceID, delivery.ID, nextAttempt, attempts, statusCode, &responseBody, &errorMsg); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"delivery_id": delivery.ID,
			"error":       err.Error(),
		}).Error("Failed to schedule delivery retry")
		return
	}

	w.logger.WithFields(map[string]interface{}{
		"delivery_id":     delivery.ID,
		"subscription_id": sub.ID,
		"attempts":        attempts,
		"next_attempt":    nextAttempt.Format(time.RFC3339),
		"error":           errorMsg,
	}).Debug("Webhook delivery failed, scheduled retry")
}

// signPayload signs the webhook payload using Standard Webhooks spec
// Format: v1,{base64-encoded-signature}
func signPayload(msgID string, timestamp int64, payload []byte, secret []byte) string {
	signedContent := fmt.Sprintf("%s.%d.%s", msgID, timestamp, string(payload))
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(signedContent))
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("v1,%s", signature)
}

// buildTestPayload generates a realistic test payload based on event type
func buildTestPayload(eventType string) map[string]interface{} {
	now := time.Now().UTC().Format(time.RFC3339)

	// Parse the event category (e.g., "contact" from "contact.created")
	parts := strings.Split(eventType, ".")
	category := parts[0]

	switch category {
	case "contact":
		return map[string]interface{}{
			"email":       "test@example.com",
			"id":          "test_contact_123",
			"external_id": "ext_456",
			"first_name":  "Test",
			"last_name":   "User",
			"tags":        []string{"test", "webhook"},
			"created_at":  now,
			"updated_at":  now,
		}
	case "list":
		return map[string]interface{}{
			"email":      "test@example.com",
			"contact_id": "test_contact_123",
			"list_id":    "test_list_456",
			"list_name":  "Test Newsletter",
			"status":     "active",
			"created_at": now,
		}
	case "segment":
		return map[string]interface{}{
			"email":        "test@example.com",
			"contact_id":   "test_contact_123",
			"segment_id":   "test_segment_789",
			"segment_name": "Test Segment",
			"created_at":   now,
		}
	case "email":
		payload := map[string]interface{}{
			"message_id":   "test_msg_789",
			"email":        "test@example.com",
			"subject":      "Test Email Subject",
			"broadcast_id": "test_broadcast_012",
			"template_id":  "test_template_345",
			"created_at":   now,
		}
		// Add event-specific fields
		if len(parts) > 1 {
			switch parts[1] {
			case "clicked":
				payload["url"] = "https://example.com/test-link"
			case "bounced":
				payload["bounce_type"] = "hard"
				payload["bounce_reason"] = "Test bounce reason"
			case "complained":
				payload["complaint_type"] = "abuse"
			}
		}
		return payload
	case "custom_event":
		return map[string]interface{}{
			"event_id":   "test_event_012",
			"event_name": "test_purchase",
			"goal_type":  "conversion",
			"contact_id": "test_contact_123",
			"email":      "test@example.com",
			"properties": map[string]interface{}{
				"product_id": "prod_123",
				"amount":     99.99,
				"currency":   "USD",
			},
			"created_at": now,
		}
	default:
		// Fallback for unknown event types
		return map[string]interface{}{
			"message":    "This is a test webhook from Notifuse",
			"event_type": eventType,
			"created_at": now,
		}
	}
}

// SendTestWebhook sends a test webhook to verify the endpoint
func (w *WebhookDeliveryWorker) SendTestWebhook(ctx context.Context, workspaceID string, sub *domain.WebhookSubscription, eventType string) (int, string, error) {
	// Build test payload
	testID := fmt.Sprintf("test_%d", time.Now().UnixNano())

	// Use provided event type or default to "test"
	if eventType == "" {
		eventType = "test"
	}

	envelope := map[string]interface{}{
		"id":           testID,
		"type":         eventType,
		"workspace_id": workspaceID,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"data":         buildTestPayload(eventType),
	}

	payloadBytes, err := json.Marshal(envelope)
	if err != nil {
		return 0, "", fmt.Errorf("failed to marshal test payload: %w", err)
	}

	// Generate timestamp for signing
	timestamp := time.Now().Unix()

	// Sign the payload
	signature := signPayload(testID, timestamp, payloadBytes, []byte(sub.Secret))

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		return 0, "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set Standard Webhooks headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("webhook-id", testID)
	req.Header.Set("webhook-timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("webhook-signature", signature)

	// Send the request
	resp, err := w.httpClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body (limit to 1KB)
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	responseBody := string(bodyBytes)

	return resp.StatusCode, responseBody, nil
}
