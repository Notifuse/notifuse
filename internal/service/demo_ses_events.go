package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/google/uuid"
)

// The demo's email history reads as sent through Amazon SES: its delivery, bounce and complaint
// webhooks are the SNS notifications SES posts, which processSESWebhook parses into the same
// events, and each message carries the timestamps ProcessWebhook would write for them. The
// workspace's integrations are left as they are: the seeded SMTP relay is what the preview
// instance really sends through.

const (
	demoSESTopicARN       = "arn:aws:sns:us-east-1:123456789012:notifuse-demo-email-events"
	demoSESBounceType     = "Permanent"
	demoSESBounceSubType  = "General"
	demoSESBounceDiag     = "smtp; 550 5.1.1 The email account that you tried to reach does not exist"
	demoSESComplaintType  = "abuse"
	demoSESDefaultSender  = "demo@notifuse.com"
	demoSESReportingMTA   = "a8-30.smtp-out.amazonses.com"
	demoSESBounceMTA      = "dns; amazonses.com"
	demoSESProcessingTime = 812
)

// demoSESMessageID is the id SES gives the email it sends: the provider's own, beside the
// notifuse_message_id tag that names the message.
func demoSESMessageID(messageID string) string {
	hex := strings.ReplaceAll(uuid.NewSHA1(uuid.NameSpaceOID, []byte(messageID)).String(), "-", "")
	return fmt.Sprintf("010001%s-%s-%s-000000", hex[:10], hex[10:18], hex[18:30])
}

// demoSESEvent builds the inbound webhook event of one SES notification about a message: the
// SNS envelope as raw payload, and the fields processSESWebhook reads out of it.
func demoSESEvent(eventType domain.EmailEventType, integrationID, sender string, message *domain.MessageHistory, at time.Time) (*domain.InboundWebhookEvent, error) {
	at = at.UTC()
	stamp := at.Format(time.RFC3339)
	mail := domain.SESMail{
		Timestamp:   message.SentAt.UTC().Format(time.RFC3339),
		MessageID:   demoSESMessageID(message.ID),
		Source:      sender,
		Destination: []string{message.ContactEmail},
		Tags:        map[string][]string{"notifuse_message_id": {message.ID}},
	}
	messageID := message.ID
	event := &domain.InboundWebhookEvent{
		ID:             uuid.New().String(),
		Type:           eventType,
		Source:         domain.WebhookSourceSES,
		IntegrationID:  integrationID,
		RecipientEmail: message.ContactEmail,
		MessageID:      &messageID,
		Timestamp:      at,
		CreatedAt:      at,
	}

	var notification interface{}
	switch eventType {
	case domain.EmailEventDelivered:
		notification = domain.SESDeliveryNotification{
			EventType: "Delivery",
			Mail:      mail,
			Delivery: domain.SESDelivery{
				Timestamp:            stamp,
				ProcessingTimeMillis: demoSESProcessingTime,
				Recipients:           []string{message.ContactEmail},
				SMTPResponse:         "250 2.0.0 OK",
				ReportingMTA:         demoSESReportingMTA,
			},
		}
	case domain.EmailEventBounce:
		notification = domain.SESBounceNotification{
			EventType: "Bounce",
			Mail:      mail,
			Bounce: domain.SESBounce{
				BounceType:    demoSESBounceType,
				BounceSubType: demoSESBounceSubType,
				BouncedRecipients: []domain.SESBouncedRecipient{{
					EmailAddress:   message.ContactEmail,
					Action:         "failed",
					Status:         "5.1.1",
					DiagnosticCode: demoSESBounceDiag,
				}},
				Timestamp:    stamp,
				FeedbackID:   uuid.New().String(),
				ReportingMTA: demoSESBounceMTA,
			},
		}
		event.BounceType = demoSESBounceType
		event.BounceCategory = demoSESBounceSubType
		event.BounceDiagnostic = demoSESBounceDiag
	case domain.EmailEventComplaint:
		notification = domain.SESComplaintNotification{
			EventType: "Complaint",
			Mail:      mail,
			Complaint: domain.SESComplaint{
				ComplainedRecipients:  []domain.SESComplainedRecipient{{EmailAddress: message.ContactEmail}},
				Timestamp:             stamp,
				FeedbackID:            uuid.New().String(),
				ComplaintFeedbackType: demoSESComplaintType,
			},
		}
		event.ComplaintFeedbackType = demoSESComplaintType
	default:
		return nil, fmt.Errorf("no SES notification for a %s event", eventType)
	}

	inner, err := json.Marshal(notification)
	if err != nil {
		return nil, err
	}
	envelope, err := json.Marshal(domain.SESWebhookPayload{
		Type:             "Notification",
		MessageID:        uuid.New().String(),
		TopicARN:         demoSESTopicARN,
		Message:          string(inner),
		Timestamp:        stamp,
		SignatureVersion: "1",
	})
	if err != nil {
		return nil, err
	}
	event.RawPayload = string(envelope)
	return event, nil
}

// demoSESBounceStatusInfo is the reason ProcessWebhook records on a hard-bounced message.
func demoSESBounceStatusInfo() string {
	return fmt.Sprintf("%s %s %s", demoSESBounceType, demoSESBounceSubType, demoSESBounceDiag)
}

// demoProviders names, per origin, the integration a message went through and the address it
// left from: newsletters and automations through the marketing provider, notifications through
// the transactional one, each standing in for the other when it is not set.
type demoProviders struct {
	marketing, transactional             string
	marketingSender, transactionalSender string
}

func demoProvidersOf(workspace *domain.Workspace) demoProviders {
	senderOf := func(id string) string {
		for _, integration := range workspace.Integrations {
			if integration.ID != id {
				continue
			}
			for _, sender := range integration.EmailProvider.Senders {
				if sender.IsDefault {
					return sender.Email
				}
			}
		}
		return demoSESDefaultSender
	}
	p := demoProviders{
		marketing:     workspace.Settings.MarketingEmailProviderID,
		transactional: workspace.Settings.TransactionalEmailProviderID,
	}
	if p.marketing == "" {
		p.marketing = p.transactional
	}
	if p.transactional == "" {
		p.transactional = p.marketing
	}
	p.marketingSender = senderOf(p.marketing)
	p.transactionalSender = senderOf(p.transactional)
	return p
}

// of is the integration and sender of a message, from its origin.
func (p demoProviders) of(message *domain.MessageHistory) (integrationID, sender string) {
	if message.TransactionalNotificationID != nil || (message.BroadcastID == nil && message.AutomationID == nil) {
		return p.transactional, p.transactionalSender
	}
	return p.marketing, p.marketingSender
}

// storeDemoDeliveredSESEvents records the SES delivery notification of each message already
// delivered (the automation history writes delivered_at itself).
func (s *DemoService) storeDemoDeliveredSESEvents(ctx context.Context, workspaceID string, messages []*domain.MessageHistory) error {
	if s.workspaceService == nil || s.inboundWebhookEventRepo == nil {
		return nil
	}
	workspace, err := s.workspaceService.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}
	providers := demoProvidersOf(workspace)
	var events []*domain.InboundWebhookEvent
	for _, message := range messages {
		if message.DeliveredAt == nil {
			continue
		}
		integrationID, sender := providers.of(message)
		if integrationID == "" {
			return nil
		}
		event, err := demoSESEvent(domain.EmailEventDelivered, integrationID, sender, message, *message.DeliveredAt)
		if err != nil {
			return err
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		return nil
	}
	if err := s.inboundWebhookEventRepo.StoreEvents(ctx, workspaceID, events); err != nil {
		return fmt.Errorf("failed to store inbound webhook events: %w", err)
	}
	return nil
}
