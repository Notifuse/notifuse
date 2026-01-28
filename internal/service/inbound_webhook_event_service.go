package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/tracing"
	"github.com/google/uuid"
)

// InboundWebhookEventService implements the domain.InboundWebhookEventServiceInterface
type InboundWebhookEventService struct {
	repo               domain.InboundWebhookEventRepository
	authService        domain.AuthService
	logger             logger.Logger
	workspaceRepo      domain.WorkspaceRepository
	messageHistoryRepo domain.MessageHistoryRepository
}

// NewInboundWebhookEventService creates a new InboundWebhookEventService
func NewInboundWebhookEventService(
	repo domain.InboundWebhookEventRepository,
	authService domain.AuthService,
	logger logger.Logger,
	workspaceRepo domain.WorkspaceRepository,
	messageHistoryRepo domain.MessageHistoryRepository,
) *InboundWebhookEventService {
	return &InboundWebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             logger,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
	}
}

// ProcessWebhook processes a webhook event from an email provider
func (s *InboundWebhookEventService) ProcessWebhook(ctx context.Context, workspaceID string, integrationID string, rawPayload []byte) error {
	// codecov:ignore:start
	ctx, span := tracing.StartServiceSpan(ctx, "InboundWebhookEventService", "ProcessWebhook")
	defer tracing.EndSpan(span, nil)
	tracing.AddAttribute(ctx, "workspaceID", workspaceID)
	tracing.AddAttribute(ctx, "integrationID", integrationID)
	// codecov:ignore:end

	// get workspace and integration
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return fmt.Errorf("failed to get workspace: %w", err)
	}
	var integration domain.Integration
	for _, i := range workspace.Integrations {
		if i.ID == integrationID {
			integration = i
			break
		}
	}
	var events []*domain.InboundWebhookEvent

	switch integration.EmailProvider.Kind {
	case domain.EmailProviderKindSES:
		events, err = s.processSESWebhook(integration.ID, rawPayload)
	case domain.EmailProviderKindPostmark:
		events, err = s.processPostmarkWebhook(integration.ID, rawPayload)
	case domain.EmailProviderKindMailgun:
		events, err = s.processMailgunWebhook(integration.ID, rawPayload)
	case domain.EmailProviderKindSparkPost:
		events, err = s.processSparkPostWebhook(integration.ID, rawPayload)
	case domain.EmailProviderKindMailjet:
		events, err = s.processMailjetWebhook(integration.ID, rawPayload)
	case domain.EmailProviderKindSMTP:
		events, err = s.processSMTPWebhook(integration.ID, rawPayload)
	case domain.EmailProviderKindSendGrid:
		events, err = s.processSendGridWebhook(integration.ID, rawPayload)
	default:
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, fmt.Errorf("unsupported email provider kind: %s", integration.EmailProvider.Kind))
		// codecov:ignore:end
		return fmt.Errorf("unsupported email provider kind: %s", integration.EmailProvider.Kind)
	}

	if err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return fmt.Errorf("failed to process webhook: %w", err)
	}

	// Store the event
	// No authentication needed for webhook events as they come from external providers
	if err := s.repo.StoreEvents(ctx, workspaceID, events); err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return fmt.Errorf("failed to store inbound webhook events: %w", err)
	}

	updates := []domain.MessageEventUpdate{}

	for _, event := range events {
		var statusInfo *string

		// Update message history status if we have a message ID
		if event.MessageID != nil && *event.MessageID != "" {
			var messageEvent domain.MessageEvent
			switch event.Type {
			case domain.EmailEventDelivered:
				messageEvent = domain.MessageEventDelivered
			case domain.EmailEventBounce:
				// Only process HARD bounces - soft bounces are logged in webhook_events but don't update message_history
				if !isHardBounce(event.BounceType, event.BounceCategory) {
					s.logger.WithField("message_id", *event.MessageID).
						WithField("bounce_type", event.BounceType).
						WithField("bounce_category", event.BounceCategory).
						Debug("Skipping soft bounce - not updating message history")
					continue // Skip soft bounces
				}

				messageEvent = domain.MessageEventBounced
				reason := fmt.Sprintf("%s %s %s", event.BounceType, event.BounceCategory, event.BounceDiagnostic)
				// Truncate to fit VARCHAR(255) constraint
				if len(reason) > 255 {
					reason = reason[:255]
				}
				statusInfo = &reason
			case domain.EmailEventComplaint:
				messageEvent = domain.MessageEventComplained
				reason := event.ComplaintFeedbackType
				// Truncate to fit VARCHAR(255) constraint
				if len(reason) > 255 {
					reason = reason[:255]
				}
				statusInfo = &reason
			default:
				// Skip other event types
				return nil
			}

			updates = append(updates, domain.MessageEventUpdate{
				ID:         *event.MessageID,
				Event:      messageEvent,
				Timestamp:  event.Timestamp,
				StatusInfo: statusInfo,
			})
		}
	}

	if err := s.messageHistoryRepo.SetStatusesIfNotSet(ctx, workspaceID, updates); err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return fmt.Errorf("failed to update message status: %w", err)
	}

	return nil
}

// isHardBounce determines if a bounce is a hard/permanent bounce based on bounce type and category
// Hard bounces indicate permanent delivery failures and should update contact list status
// Soft bounces are temporary failures and should not affect contact list status
func isHardBounce(bounceType, bounceCategory string) bool {
	// Normalize to lowercase for case-insensitive comparison
	bounceType = strings.ToLower(bounceType)
	bounceCategory = strings.ToLower(bounceCategory)

	// Amazon SES bounce types
	// Reference: https://docs.aws.amazon.com/ses/latest/dg/notification-contents.html#bounce-types
	if bounceType == "permanent" {
		return true // Hard bounce - permanent failure
	}
	if bounceType == "transient" || bounceType == "undetermined" {
		return false // Soft bounce - temporary issue
	}

	// Mailgun classifications
	if bounceCategory == "hardbounce" || bounceCategory == "permanent" {
		return true
	}
	if bounceCategory == "softbounce" || bounceCategory == "temporary" {
		return false
	}

	// Mailjet classifications (uses BounceType field)
	if bounceType == "hardbounce" {
		return true
	}
	if bounceType == "softbounce" {
		return false
	}

	// Blocked emails should be treated as hard bounces
	if bounceType == "blocked" || bounceCategory == "blocked" {
		return true
	}

	// Postmark type codes (hard bounces typically have TypeCode 1 or contain "Hard" in Type field)
	if strings.Contains(bounceType, "hard") || strings.Contains(bounceCategory, "hard") {
		return true
	}
	if strings.Contains(bounceType, "soft") || strings.Contains(bounceCategory, "soft") {
		return false
	}

	// SparkPost bounce classes
	// Reference: https://support.sparkpost.com/docs/deliverability/bounce-classification-codes
	// Hard bounces: 10 (Invalid Recipient), 30 (No RCPT), 90 (Unsubscribe)
	if bounceCategory == "10" || bounceCategory == "30" || bounceCategory == "90" {
		return true // Hard bounce - permanent failure
	}
	// Soft bounces: 20-24, 40, 60, 70, 100 (temporary failures)
	// Block: 50-54 (temporary blocks - treated as soft per SparkPost docs)
	// Admin: 25, 80 (configuration issues - temporary)
	// Undetermined: 1 (unknown - temporary)
	if bounceCategory == "1" || bounceCategory == "20" || bounceCategory == "21" ||
		bounceCategory == "22" || bounceCategory == "23" || bounceCategory == "24" ||
		bounceCategory == "25" || bounceCategory == "40" || bounceCategory == "50" ||
		bounceCategory == "51" || bounceCategory == "52" || bounceCategory == "53" ||
		bounceCategory == "54" || bounceCategory == "60" || bounceCategory == "70" ||
		bounceCategory == "80" || bounceCategory == "100" {
		return false // Soft/temporary bounce
	}

	// SendGrid bounce classification
	// Reference: https://docs.sendgrid.com/glossary/bounces
	// type="bounce" = hard bounce (permanent failure)
	// type="blocked" = soft bounce (temporary rejection)
	// type="dropped" = message dropped before sending (treat as soft)
	if bounceType == "bounce" {
		return true // Hard bounce - permanent failure
	}
	if bounceType == "blocked" || bounceType == "dropped" {
		return false // Soft bounce - temporary issue
	}

	// SendGrid bounce_classification values
	// "Invalid Address" indicates permanent failure
	if bounceCategory == "invalid address" {
		return true
	}
	// Other classifications are typically temporary
	if bounceCategory == "technical" || bounceCategory == "content" ||
		bounceCategory == "reputation" || bounceCategory == "frequency/volume" ||
		bounceCategory == "mailbox unavailable" || bounceCategory == "unclassified" {
		return false
	}

	// Default to false (don't update contact lists unless we're certain it's a hard bounce)
	// This is the safe default - better to miss some hard bounces than incorrectly mark soft bounces
	return false
}

// extractXMessageIDFromHeaders searches for the X-Message-ID header in SES mail headers.
// This is used as a fallback when notifuse_message_id tag is not present
// (e.g., for emails sent via SendRawEmail before tags were added).
func extractXMessageIDFromHeaders(headers []domain.SESHeader) string {
	for _, header := range headers {
		if strings.EqualFold(header.Name, "X-Message-ID") {
			return header.Value
		}
	}
	return ""
}

// processSESWebhook processes a webhook event from Amazon SES
func (s *InboundWebhookEventService) processSESWebhook(integrationID string, rawPayload []byte) (events []*domain.InboundWebhookEvent, err error) {

	// First, parse the SNS message wrapper
	var snsPayload domain.SESWebhookPayload
	if err := json.Unmarshal(rawPayload, &snsPayload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal SES webhook payload: %w", err)
	}

	// Handle subscription confirmation
	if snsPayload.Type == "SubscriptionConfirmation" {
		s.logger.WithField("integration_id", integrationID).
			WithField("topic_arn", snsPayload.TopicARN).
			Info("Processing SNS subscription confirmation")

		// Make a GET request to the SubscribeURL to confirm the subscription
		resp, err := http.Get(snsPayload.SubscribeURL)
		if err != nil {
			s.logger.WithField("error", err.Error()).
				WithField("integration_id", integrationID).
				Error("Failed to confirm subscription")
			return nil, fmt.Errorf("failed to confirm subscription: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		s.logger.WithField("integration_id", integrationID).
			WithField("topic_arn", snsPayload.TopicARN).
			WithField("status_code", resp.StatusCode).
			Info("SNS subscription confirmed successfully")

		// Return empty events slice for subscription confirmations
		return []*domain.InboundWebhookEvent{}, nil
	}

	// Handle unsubscribe confirmation
	if snsPayload.Type == "UnsubscribeConfirmation" {
		s.logger.WithField("integration_id", integrationID).
			WithField("topic_arn", snsPayload.TopicARN).
			Info("Received SNS unsubscribe confirmation")
		return []*domain.InboundWebhookEvent{}, nil
	}

	if strings.Contains(snsPayload.Message, "Successfully validated SNS topic") {
		return []*domain.InboundWebhookEvent{}, nil
	}

	// // Only process "Notification" type messages for actual email events
	// if snsPayload.Type != "Notification" {
	// 	s.logger.WithField("integration_id", integrationID).
	// 		WithField("message_type", snsPayload.Type).
	// 		Warn("Received unsupported SNS message type")
	// 	return []*domain.InboundWebhookEvent{}, nil
	// }

	// Then, parse the actual notification based on the message type
	messageBytes := []byte(snsPayload.Message)

	// Determine the type of notification
	var eventType domain.EmailEventType
	var recipientEmail, messageID string
	var bounceType, bounceCategory, bounceDiagnostic, complaintFeedbackType string
	var timestamp time.Time
	var notifuseMessageID string

	// Try to unmarshal as bounce notification
	var bounceNotification domain.SESBounceNotification
	if err := json.Unmarshal(messageBytes, &bounceNotification); err == nil && bounceNotification.EventType == "Bounce" {
		eventType = domain.EmailEventBounce
		if len(bounceNotification.Bounce.BouncedRecipients) > 0 {
			recipientEmail = bounceNotification.Bounce.BouncedRecipients[0].EmailAddress
			bounceDiagnostic = bounceNotification.Bounce.BouncedRecipients[0].DiagnosticCode
		}
		messageID = bounceNotification.Mail.MessageID
		bounceType = bounceNotification.Bounce.BounceType
		bounceCategory = bounceNotification.Bounce.BounceSubType

		// Check for notifuse_message_id in tags
		if len(bounceNotification.Mail.Tags) > 0 {
			if ids, ok := bounceNotification.Mail.Tags["notifuse_message_id"]; ok && len(ids) > 0 {
				notifuseMessageID = ids[0]
			}
		}
		// Fallback to X-Message-ID header if tag not found
		if notifuseMessageID == "" {
			notifuseMessageID = extractXMessageIDFromHeaders(bounceNotification.Mail.Headers)
		}

		// Parse timestamp
		if t, err := time.Parse(time.RFC3339, bounceNotification.Bounce.Timestamp); err == nil {
			timestamp = t
		} else {
			timestamp = time.Now()
		}
	} else {
		// Try to unmarshal as complaint notification
		var complaintNotification domain.SESComplaintNotification
		if err := json.Unmarshal(messageBytes, &complaintNotification); err == nil && complaintNotification.EventType == "Complaint" {
			eventType = domain.EmailEventComplaint
			if len(complaintNotification.Complaint.ComplainedRecipients) > 0 {
				recipientEmail = complaintNotification.Complaint.ComplainedRecipients[0].EmailAddress
			}
			messageID = complaintNotification.Mail.MessageID
			complaintFeedbackType = complaintNotification.Complaint.ComplaintFeedbackType

			// Check for notifuse_message_id in tags
			if len(complaintNotification.Mail.Tags) > 0 {
				if ids, ok := complaintNotification.Mail.Tags["notifuse_message_id"]; ok && len(ids) > 0 {
					notifuseMessageID = ids[0]
				}
			}
			// Fallback to X-Message-ID header if tag not found
			if notifuseMessageID == "" {
				notifuseMessageID = extractXMessageIDFromHeaders(complaintNotification.Mail.Headers)
			}

			// Parse timestamp
			if t, err := time.Parse(time.RFC3339, complaintNotification.Complaint.Timestamp); err == nil {
				timestamp = t
			} else {
				timestamp = time.Now()
			}
		} else {
			// Try to unmarshal as delivery notification
			var deliveryNotification domain.SESDeliveryNotification
			if err := json.Unmarshal(messageBytes, &deliveryNotification); err == nil && deliveryNotification.EventType == "Delivery" {
				eventType = domain.EmailEventDelivered
				if len(deliveryNotification.Delivery.Recipients) > 0 {
					recipientEmail = deliveryNotification.Delivery.Recipients[0]
				}
				messageID = deliveryNotification.Mail.MessageID

				// Check for notifuse_message_id in tags
				if len(deliveryNotification.Mail.Tags) > 0 {
					if ids, ok := deliveryNotification.Mail.Tags["notifuse_message_id"]; ok && len(ids) > 0 {
						notifuseMessageID = ids[0]
					}
				}
				// Fallback to X-Message-ID header if tag not found
				if notifuseMessageID == "" {
					notifuseMessageID = extractXMessageIDFromHeaders(deliveryNotification.Mail.Headers)
				}

				// Parse timestamp
				if t, err := time.Parse(time.RFC3339, deliveryNotification.Delivery.Timestamp); err == nil {
					timestamp = t
				} else {
					timestamp = time.Now()
				}
			} else {
				// fail silently to avoid SNS subscription pause
				s.logger.WithField("integration_id", integrationID).
					WithField("payload", string(rawPayload)).
					Warn("unrecognized SES notification")
				return []*domain.InboundWebhookEvent{}, nil
			}
		}
	}

	// Use notifuseMessageID if available, otherwise fallback to provider's messageID
	if notifuseMessageID != "" {
		messageID = notifuseMessageID
	}

	// Create the webhook event
	event := domain.NewInboundWebhookEvent(
		uuid.New().String(),
		eventType,
		domain.WebhookSourceSES,
		integrationID,
		recipientEmail,
		&messageID,
		timestamp,
		string(rawPayload),
	)

	// Set event-specific information
	switch eventType {
	case domain.EmailEventBounce:
		event.BounceType = bounceType
		event.BounceCategory = bounceCategory
		event.BounceDiagnostic = bounceDiagnostic
	case domain.EmailEventComplaint:
		event.ComplaintFeedbackType = complaintFeedbackType
	}

	return []*domain.InboundWebhookEvent{event}, nil
}

// processPostmarkWebhook processes a webhook event from Postmark
func (s *InboundWebhookEventService) processPostmarkWebhook(integrationID string, rawPayload []byte) (events []*domain.InboundWebhookEvent, err error) {

	// First, unmarshal into a map to extract the fields directly
	var jsonData map[string]interface{}
	if err := json.Unmarshal(rawPayload, &jsonData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Postmark webhook payload: %w", err)
	}

	// Then unmarshal into our struct
	var payload domain.PostmarkWebhookPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Postmark webhook payload: %w", err)
	}

	var eventType domain.EmailEventType
	var recipientEmail, messageID string
	var bounceType, bounceCategory, bounceDiagnostic, complaintFeedbackType string
	var timestamp time.Time
	var notifuseMessageID string

	// Check for notifuse_message_id in metadata
	if payload.Metadata != nil {
		if msgID, ok := payload.Metadata["notifuse_message_id"]; ok {
			notifuseMessageID = msgID
		}
	}

	// Determine the event type based on RecordType
	switch payload.RecordType {
	case "Delivery":
		eventType = domain.EmailEventDelivered

		// Extract Delivered fields from the raw JSON
		if deliveryData, ok := jsonData["Recipient"].(string); ok {
			recipientEmail = deliveryData
		}

		if t, ok := jsonData["DeliveredAt"].(string); ok && t != "" {
			if parsedTime, err := time.Parse(time.RFC3339, t); err == nil {
				timestamp = parsedTime
			} else {
				timestamp = time.Now()
			}
		} else {
			timestamp = time.Now()
		}

	case "Bounce":
		eventType = domain.EmailEventBounce

		// Extract Bounce fields from the raw JSON
		if email, ok := jsonData["Email"].(string); ok {
			recipientEmail = email
		}

		if typeStr, ok := jsonData["Type"].(string); ok {
			bounceType = typeStr
			bounceCategory = typeStr // Use the same value for both in Postmark
		}

		if details, ok := jsonData["Details"].(string); ok {
			bounceDiagnostic = details
		}

		if t, ok := jsonData["BouncedAt"].(string); ok && t != "" {
			if parsedTime, err := time.Parse(time.RFC3339, t); err == nil {
				timestamp = parsedTime
			} else {
				timestamp = time.Now()
			}
		} else {
			timestamp = time.Now()
		}

	case "SpamComplaint":
		eventType = domain.EmailEventComplaint

		// Extract Complaint fields from the raw JSON
		if email, ok := jsonData["Email"].(string); ok {
			recipientEmail = email
		}

		if typeStr, ok := jsonData["Type"].(string); ok {
			complaintFeedbackType = typeStr
		}

		if t, ok := jsonData["ComplainedAt"].(string); ok && t != "" {
			if parsedTime, err := time.Parse(time.RFC3339, t); err == nil {
				timestamp = parsedTime
			} else {
				timestamp = time.Now()
			}
		} else {
			timestamp = time.Now()
		}

	default:
		return nil, fmt.Errorf("unsupported Postmark record type: %s", payload.RecordType)
	}

	messageID = payload.MessageID

	// Use notifuseMessageID if available, otherwise fallback to provider's messageID
	if notifuseMessageID != "" {
		messageID = notifuseMessageID
	}

	// Create the webhook event
	event := domain.NewInboundWebhookEvent(
		uuid.New().String(),
		eventType,
		domain.WebhookSourcePostmark,
		integrationID,
		recipientEmail,
		&messageID,
		timestamp,
		string(rawPayload),
	)

	// Set event-specific information
	switch eventType {
	case domain.EmailEventBounce:
		event.BounceType = bounceType
		event.BounceCategory = bounceCategory
		event.BounceDiagnostic = bounceDiagnostic
	case domain.EmailEventComplaint:
		event.ComplaintFeedbackType = complaintFeedbackType
	}

	return []*domain.InboundWebhookEvent{event}, nil
}

// processMailgunWebhook processes a webhook event from Mailgun
func (s *InboundWebhookEventService) processMailgunWebhook(integrationID string, rawPayload []byte) (events []*domain.InboundWebhookEvent, err error) {

	// First unmarshal into a map to access all fields
	var jsonData map[string]interface{}
	if err := json.Unmarshal(rawPayload, &jsonData); err != nil {
		log.Printf("failed to unmarshal Mailgun webhook payload: %v, %v", err, string(rawPayload))
		return nil, fmt.Errorf("failed to unmarshal Mailgun webhook payload: %w", err)
	}

	var payload domain.MailgunWebhookPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		log.Printf("failed to unmarshal Mailgun webhook payload: %v, %v", err, string(rawPayload))
		return nil, fmt.Errorf("failed to unmarshal Mailgun webhook payload: %w", err)
	}

	var eventType domain.EmailEventType
	var recipientEmail, messageID string
	var bounceType, bounceCategory, bounceDiagnostic, complaintFeedbackType string
	var timestamp time.Time
	var notifuseMessageID string

	// Set timestamp from event data
	timestamp = time.Unix(int64(payload.EventData.Timestamp), 0)

	// Check for notifuse_message_id in the custom variables
	if eventData, ok := jsonData["event-data"].(map[string]interface{}); ok {
		if userVariables, ok := eventData["user-variables"].(map[string]interface{}); ok {
			if id, ok := userVariables["notifuse_message_id"]; ok {
				notifuseMessageID = fmt.Sprintf("%v", id)
			}
		}
	}

	// Map Mailgun event types to our event types
	switch payload.EventData.Event {
	case "delivered":
		eventType = domain.EmailEventDelivered
		recipientEmail = payload.EventData.Recipient
		messageID = payload.EventData.Message.Headers.MessageID
	case "failed":
		eventType = domain.EmailEventBounce
		recipientEmail = payload.EventData.Recipient
		messageID = payload.EventData.Message.Headers.MessageID

		// Set bounce details
		bounceType = "Failed"
		if payload.EventData.Severity == "permanent" {
			bounceCategory = "HardBounce"
		} else {
			bounceCategory = "SoftBounce"
		}
		bounceDiagnostic = payload.EventData.Reason
	case "complained":
		eventType = domain.EmailEventComplaint
		recipientEmail = payload.EventData.Recipient
		messageID = payload.EventData.Message.Headers.MessageID
		complaintFeedbackType = "abuse"
	default:
		return nil, fmt.Errorf("unsupported Mailgun event type: %s", payload.EventData.Event)
	}

	// Use notifuseMessageID if available, otherwise fallback to provider's messageID
	if notifuseMessageID != "" {
		messageID = notifuseMessageID
	}

	// Create the webhook event
	event := domain.NewInboundWebhookEvent(
		uuid.New().String(),
		eventType,
		domain.WebhookSourceMailgun,
		integrationID,
		recipientEmail,
		&messageID,
		timestamp,
		string(rawPayload),
	)

	// Set event-specific information
	switch eventType {
	case domain.EmailEventBounce:
		event.BounceType = bounceType
		event.BounceCategory = bounceCategory
		event.BounceDiagnostic = bounceDiagnostic
	case domain.EmailEventComplaint:
		event.ComplaintFeedbackType = complaintFeedbackType
	}

	return []*domain.InboundWebhookEvent{event}, nil
}

// processSparkPostWebhook processes a webhook event from SparkPost
func (s *InboundWebhookEventService) processSparkPostWebhook(integrationID string, rawPayload []byte) (events []*domain.InboundWebhookEvent, err error) {
	events = []*domain.InboundWebhookEvent{}

	// payload can contain multiple events
	var payload []*domain.SparkPostWebhookPayload

	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal SparkPost webhook payload: %w", err)
	}

	for _, payload := range payload {
		var eventType domain.EmailEventType
		var recipientEmail, messageID string
		var bounceType, bounceCategory, bounceDiagnostic, complaintFeedbackType string
		var timestamp time.Time
		var notifuseMessageID string

		if payload.MSys.MessageEvent == nil {
			return nil, fmt.Errorf("no message_event found in SparkPost webhook payload")
		}

		if id, ok := payload.MSys.MessageEvent.RecipientMeta["notifuse_message_id"]; ok {
			notifuseMessageID = fmt.Sprintf("%v", id)
		}

		// Set common fields
		recipientEmail = payload.MSys.MessageEvent.RecipientTo
		messageID = payload.MSys.MessageEvent.MessageID

		// Parse timestamp - SparkPost may send Unix timestamp as a string
		if payload.MSys.MessageEvent.Timestamp != "" {
			// First try parsing as RFC3339
			if t, err := time.Parse(time.RFC3339, payload.MSys.MessageEvent.Timestamp); err == nil {
				timestamp = t
			} else {
				// If RFC3339 parsing fails, try parsing as Unix timestamp
				if unixTimestamp, err := strconv.ParseInt(payload.MSys.MessageEvent.Timestamp, 10, 64); err == nil {
					timestamp = time.Unix(unixTimestamp, 0)
				} else {
					// Fall back to current time if parsing fails
					timestamp = time.Now()
					s.logger.WithFields(map[string]interface{}{
						"timestamp_string": payload.MSys.MessageEvent.Timestamp,
						"parse_error":      err.Error(),
					}).Warn("Failed to parse SparkPost timestamp")
				}
			}
		} else {
			timestamp = time.Now()
		}

		// Determine event type based on the type field
		switch payload.MSys.MessageEvent.Type {
		case "delivery":
			eventType = domain.EmailEventDelivered

		case "bounce":
			eventType = domain.EmailEventBounce
			bounceType = "Bounce"
			bounceCategory = payload.MSys.MessageEvent.BounceClass
			bounceDiagnostic = payload.MSys.MessageEvent.Reason

		case "spam_complaint":
			eventType = domain.EmailEventComplaint
			complaintFeedbackType = payload.MSys.MessageEvent.FeedbackType

		default:
			return nil, fmt.Errorf("unsupported SparkPost event type: %s", payload.MSys.MessageEvent.Type)
		}

		// Use notifuseMessageID if available, otherwise fallback to provider's messageID
		if notifuseMessageID != "" {
			messageID = notifuseMessageID
		}

		// Create the webhook event
		event := domain.NewInboundWebhookEvent(
			uuid.New().String(),
			eventType,
			domain.WebhookSourceSparkPost,
			integrationID,
			recipientEmail,
			&messageID,
			timestamp,
			string(rawPayload),
		)

		// Set event-specific information
		switch eventType {
		case domain.EmailEventBounce:
			event.BounceType = bounceType
			event.BounceCategory = bounceCategory
			event.BounceDiagnostic = bounceDiagnostic
		case domain.EmailEventComplaint:
			event.ComplaintFeedbackType = complaintFeedbackType
		}

		events = append(events, event)
	}

	return events, nil
}

// processMailjetWebhook processes a webhook event from Mailjet
func (s *InboundWebhookEventService) processMailjetWebhook(integrationID string, rawPayload []byte) (events []*domain.InboundWebhookEvent, err error) {
	// Mailjet can send either a single object or an array of events
	// First try to unmarshal as an array
	var payloadArray []domain.MailjetWebhookPayload
	if err := json.Unmarshal(rawPayload, &payloadArray); err == nil {
		// Successfully unmarshaled as array, process each event
		var allEvents []*domain.InboundWebhookEvent
		for _, payload := range payloadArray {
			event, err := s.processSingleMailjetEvent(integrationID, payload, rawPayload)
			if err != nil {
				return nil, err
			}
			allEvents = append(allEvents, event)
		}
		return allEvents, nil
	}

	// If array unmarshal failed, try as single object
	var payload domain.MailjetWebhookPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Mailjet webhook payload as single object or array: %w", err)
	}

	// Process single event
	event, err := s.processSingleMailjetEvent(integrationID, payload, rawPayload)
	if err != nil {
		return nil, err
	}
	return []*domain.InboundWebhookEvent{event}, nil
}

func (s *InboundWebhookEventService) processSingleMailjetEvent(integrationID string, payload domain.MailjetWebhookPayload, rawPayload []byte) (*domain.InboundWebhookEvent, error) {
	var eventType domain.EmailEventType
	var recipientEmail, messageID string
	var bounceType, bounceCategory, bounceDiagnostic, complaintFeedbackType string
	var timestamp time.Time
	var notifuseMessageID string

	// Set timestamp from Unix timestamp
	timestamp = time.Unix(payload.Time, 0)

	// Convert message ID to string
	messageID = fmt.Sprintf("%d", payload.MessageID)
	recipientEmail = payload.Email

	// Check for X-MJ-CustomID in the custom variables
	if payload.CustomID != "" {
		notifuseMessageID = payload.CustomID
	}

	// Map Mailjet event types to our event types
	// According to Mailjet documentation at https://dev.mailjet.com/email/guides/webhooks/
	switch payload.Event {
	case "sent":
		// Mailjet's "sent" event means the message was successfully delivered
		eventType = domain.EmailEventDelivered
	case "bounce":
		eventType = domain.EmailEventBounce

		// Set bounce details based on Mailjet's bounce classification
		if payload.HardBounce {
			bounceType = "HardBounce"
			bounceCategory = "Permanent"
		} else {
			bounceType = "SoftBounce"
			bounceCategory = "Temporary"
		}

		bounceDiagnostic = payload.Comment
		if payload.Error != "" {
			if bounceDiagnostic != "" {
				bounceDiagnostic += ": "
			}
			bounceDiagnostic += payload.Error
		}
	case "blocked":
		// Blocked messages are treated as bounces
		eventType = domain.EmailEventBounce
		bounceType = "Blocked"
		bounceCategory = "Blocked"
		bounceDiagnostic = payload.Comment
		if payload.Error != "" {
			if bounceDiagnostic != "" {
				bounceDiagnostic += ": "
			}
			bounceDiagnostic += payload.Error
		}
	case "spam":
		eventType = domain.EmailEventComplaint
		complaintFeedbackType = "spam"
		if payload.Source != "" {
			complaintFeedbackType = payload.Source
		}
	case "unsub":
		// Unsubscribe events can be treated as complaints for tracking purposes
		eventType = domain.EmailEventComplaint
		complaintFeedbackType = "unsubscribe"
	default:
		return nil, fmt.Errorf("unsupported Mailjet event type: %s", payload.Event)
	}

	// Use notifuseMessageID if available, otherwise fallback to provider's messageID
	if notifuseMessageID != "" {
		messageID = notifuseMessageID
	}

	// Create the webhook event
	event := domain.NewInboundWebhookEvent(
		uuid.New().String(),
		eventType,
		domain.WebhookSourceMailjet,
		integrationID,
		recipientEmail,
		&messageID,
		timestamp,
		string(rawPayload),
	)

	// Set event-specific information
	switch eventType {
	case domain.EmailEventBounce:
		event.BounceType = bounceType
		event.BounceCategory = bounceCategory
		event.BounceDiagnostic = bounceDiagnostic
	case domain.EmailEventComplaint:
		event.ComplaintFeedbackType = complaintFeedbackType
	}

	return event, nil
}

// processSMTPWebhook processes a webhook event from a generic SMTP provider
func (s *InboundWebhookEventService) processSMTPWebhook(integrationID string, rawPayload []byte) (events []*domain.InboundWebhookEvent, err error) {

	var payload domain.SMTPWebhookPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal SMTP webhook payload: %w", err)
	}

	var eventType domain.EmailEventType
	var timestamp time.Time

	// Parse timestamp
	if t, err := time.Parse(time.RFC3339, payload.Timestamp); err == nil {
		timestamp = t
	} else {
		timestamp = time.Now()
	}

	// Map event types
	switch payload.Event {
	case "delivered":
		eventType = domain.EmailEventDelivered
	case "bounce":
		eventType = domain.EmailEventBounce
	case "complaint":
		eventType = domain.EmailEventComplaint
	default:
		return nil, fmt.Errorf("unsupported SMTP event type: %s", payload.Event)
	}

	// Create the webhook event
	event := domain.NewInboundWebhookEvent(
		uuid.New().String(),
		eventType,
		domain.WebhookSourceSMTP,
		integrationID,
		payload.Recipient,
		&payload.MessageID,
		timestamp,
		string(rawPayload),
	)

	// Set event-specific information
	switch eventType {
	case domain.EmailEventBounce:
		event.BounceType = "Bounce"
		event.BounceCategory = payload.BounceCategory
		event.BounceDiagnostic = payload.DiagnosticCode
	case domain.EmailEventComplaint:
		event.ComplaintFeedbackType = payload.ComplaintType
	}

	return []*domain.InboundWebhookEvent{event}, nil
}

// processSendGridWebhook processes webhook events from SendGrid
// SendGrid sends events as a JSON array with custom_args flattened into top-level fields
func (s *InboundWebhookEventService) processSendGridWebhook(integrationID string, rawPayload []byte) (events []*domain.InboundWebhookEvent, err error) {
	// SendGrid sends webhooks as a JSON array of events
	var payload []domain.SendGridWebhookEvent
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal SendGrid webhook payload: %w", err)
	}

	events = []*domain.InboundWebhookEvent{}

	for _, sgEvent := range payload {
		var eventType domain.EmailEventType
		var bounceType, bounceCategory, bounceDiagnostic, complaintFeedbackType string

		timestamp := time.Unix(sgEvent.Timestamp, 0)
		recipientEmail := sgEvent.Email

		// Use notifuse_message_id if present (flattened at top level by SendGrid)
		// Otherwise fall back to SendGrid's message ID
		messageID := sgEvent.SGMessageID
		if sgEvent.NotifuseMessageID != "" {
			messageID = sgEvent.NotifuseMessageID
		}

		// Map SendGrid event types to our event types
		// Reference: https://docs.sendgrid.com/for-developers/tracking-events/event
		switch sgEvent.Event {
		case "delivered":
			eventType = domain.EmailEventDelivered

		case "bounce":
			// type="bounce" indicates a hard/permanent bounce
			eventType = domain.EmailEventBounce
			bounceType = "bounce"
			bounceCategory = sgEvent.BounceClassification
			bounceDiagnostic = sgEvent.Reason
			if sgEvent.Status != "" {
				bounceDiagnostic = fmt.Sprintf("%s: %s", sgEvent.Status, sgEvent.Reason)
			}

		case "blocked":
			// type="blocked" indicates a soft/temporary bounce
			eventType = domain.EmailEventBounce
			bounceType = "blocked"
			bounceCategory = sgEvent.BounceClassification
			bounceDiagnostic = sgEvent.Reason
			if sgEvent.Status != "" {
				bounceDiagnostic = fmt.Sprintf("%s: %s", sgEvent.Status, sgEvent.Reason)
			}

		case "dropped":
			// Dropped messages were not sent due to prior issues
			eventType = domain.EmailEventBounce
			bounceType = "dropped"
			bounceCategory = "Dropped"
			bounceDiagnostic = sgEvent.Reason

		case "spamreport":
			eventType = domain.EmailEventComplaint
			complaintFeedbackType = "spam"

		default:
			// Skip event types we don't track (processed, deferred, open, click, unsubscribe, etc.)
			continue
		}

		// Create the webhook event
		event := domain.NewInboundWebhookEvent(
			uuid.New().String(),
			eventType,
			domain.WebhookSourceSendGrid,
			integrationID,
			recipientEmail,
			&messageID,
			timestamp,
			string(rawPayload),
		)

		// Set event-specific information
		switch eventType {
		case domain.EmailEventBounce:
			event.BounceType = bounceType
			event.BounceCategory = bounceCategory
			event.BounceDiagnostic = bounceDiagnostic
		case domain.EmailEventComplaint:
			event.ComplaintFeedbackType = complaintFeedbackType
		}

		events = append(events, event)
	}

	return events, nil
}

// ListEvents retrieves all webhook events for a workspace
func (s *InboundWebhookEventService) ListEvents(ctx context.Context, workspaceID string, params domain.InboundWebhookEventListParams) (*domain.InboundWebhookEventListResult, error) {
	// codecov:ignore:start
	ctx, span := tracing.StartServiceSpan(ctx, "InboundWebhookEventService", "ListEvents")
	defer tracing.EndSpan(span, nil)
	tracing.AddAttribute(ctx, "workspaceID", workspaceID)
	// codecov:ignore:end

	// Authenticate user for workspace
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Validate params
	if err := params.Validate(); err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Call repository method
	result, err := s.repo.ListEvents(ctx, workspaceID, params)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).
			WithField("params", params).
			Error(fmt.Sprintf("Failed to list inbound webhook events: %v", err))
		// codecov:ignore:start
		tracing.MarkSpanError(ctx, err)
		// codecov:ignore:end
		return nil, fmt.Errorf("failed to list inbound webhook events: %w", err)
	}

	return result, nil
}
