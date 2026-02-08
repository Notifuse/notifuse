package broadcast

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/google/uuid"
)

// queueMessageSender implements the MessageSender interface by enqueueing to the email queue
// instead of sending directly. This allows rate limiting to be handled by the queue workers
// and provides a unified queue for both broadcasts and automations.
type queueMessageSender struct {
	queueRepo          domain.EmailQueueRepository
	broadcastRepo      domain.BroadcastRepository
	messageHistoryRepo domain.MessageHistoryRepository
	templateRepo       domain.TemplateRepository
	dataFeedFetcher    DataFeedFetcher
	logger             logger.Logger
	config             *Config
	apiEndpoint        string
}

// NewQueueMessageSender creates a new message sender that enqueues to the email queue
func NewQueueMessageSender(
	queueRepo domain.EmailQueueRepository,
	broadcastRepo domain.BroadcastRepository,
	messageHistoryRepo domain.MessageHistoryRepository,
	templateRepo domain.TemplateRepository,
	dataFeedFetcher DataFeedFetcher,
	logger logger.Logger,
	config *Config,
	apiEndpoint string,
) MessageSender {
	if config == nil {
		config = DefaultConfig()
	}

	return &queueMessageSender{
		queueRepo:          queueRepo,
		broadcastRepo:      broadcastRepo,
		messageHistoryRepo: messageHistoryRepo,
		templateRepo:       templateRepo,
		dataFeedFetcher:    dataFeedFetcher,
		logger:             logger,
		config:             config,
		apiEndpoint:        apiEndpoint,
	}
}

// SendToRecipient enqueues a message for a single recipient
func (s *queueMessageSender) SendToRecipient(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	endpoint string,
	trackingEnabled bool,
	broadcast *domain.Broadcast,
	messageID string,
	email string,
	template *domain.Template,
	data map[string]interface{},
	emailProvider *domain.EmailProvider,
	timeoutAt time.Time,
) error {
	// Build the email payload
	entry, err := s.buildQueueEntry(ctx, workspaceID, integrationID, endpoint, trackingEnabled, broadcast, messageID, email, template, data, emailProvider)
	if err != nil {
		return err
	}

	// Enqueue the email
	if err := s.queueRepo.Enqueue(ctx, workspaceID, []*domain.EmailQueueEntry{entry}); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcast.ID,
			"workspace_id": workspaceID,
			"recipient":    email,
			"error":        err.Error(),
		}).Error("Failed to enqueue email")
		return NewBroadcastError(ErrCodeSendFailed, "failed to enqueue email", true, err)
	}

	return nil
}

// SendBatch enqueues messages for a batch of recipients
func (s *queueMessageSender) SendBatch(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	workspaceSecretKey string,
	endpoint string,
	trackingEnabled bool,
	broadcastID string,
	recipients []*domain.ContactWithList,
	templates map[string]*domain.Template,
	emailProvider *domain.EmailProvider,
	timeoutAt time.Time,
) (sent int, failed int, err error) {
	if len(recipients) == 0 {
		return 0, 0, nil
	}

	// Get broadcast for context
	broadcast, err := s.broadcastRepo.GetBroadcast(ctx, workspaceID, broadcastID)
	if err != nil {
		return 0, len(recipients), fmt.Errorf("failed to get broadcast: %w", err)
	}

	// Build queue entries
	var entries []*domain.EmailQueueEntry
	var buildErrors int

	for _, recipient := range recipients {
		// Check timeout
		if time.Now().After(timeoutAt) {
			s.logger.WithFields(map[string]interface{}{
				"broadcast_id": broadcastID,
				"workspace_id": workspaceID,
			}).Debug("Timeout reached during batch build")
			break
		}

		// Select template (for A/B testing, use first template or random selection)
		template := s.selectTemplate(templates, broadcast)
		if template == nil {
			buildErrors++
			continue
		}

		// Generate message ID
		messageID := fmt.Sprintf("%s_%s", workspaceID, uuid.New().String())

		// Ensure UTM parameters object is present
		if broadcast.UTMParameters == nil {
			broadcast.UTMParameters = &domain.UTMParameters{}
		}

		if broadcast.UTMParameters.Content == "" {
			broadcast.UTMParameters.Content = template.ID
		}

		// Build tracking settings for BuildTemplateData
		trackingSettings := notifuse_mjml.TrackingSettings{
			Endpoint:       endpoint,
			EnableTracking: trackingEnabled,
			UTMSource:      broadcast.UTMParameters.Source,
			UTMMedium:      broadcast.UTMParameters.Medium,
			UTMCampaign:    broadcast.UTMParameters.Campaign,
			UTMContent:     broadcast.UTMParameters.Content,
			UTMTerm:        broadcast.UTMParameters.Term,
			WorkspaceID:    workspaceID,
			MessageID:      messageID,
		}

		// Build template data with all system variables (unsubscribe_url, notification_center_url, etc.)
		req := domain.TemplateDataRequest{
			WorkspaceID:        workspaceID,
			WorkspaceSecretKey: workspaceSecretKey,
			ContactWithList:    *recipient,
			MessageID:          messageID,
			TrackingSettings:   trackingSettings,
			Broadcast:          broadcast,
		}
		data, err := domain.BuildTemplateData(req)
		if err != nil {
			s.logger.WithFields(map[string]interface{}{
				"broadcast_id": broadcastID,
				"workspace_id": workspaceID,
				"recipient":    recipient.Contact.Email,
				"error":        err.Error(),
			}).Warn("Failed to build template data")
			buildErrors++
			continue
		}

		// Fetch recipient feed if configured and enabled
		if broadcast.DataFeed != nil && broadcast.DataFeed.RecipientFeed != nil &&
			broadcast.DataFeed.RecipientFeed.Enabled && s.dataFeedFetcher != nil {

			payload := &domain.RecipientFeedRequestPayload{
				Contact:   domain.BuildRecipientFeedContact(recipient.Contact),
				List:      domain.RecipientFeedList{ID: recipient.ListID, Name: recipient.ListName},
				Broadcast: domain.RecipientFeedBroadcast{ID: broadcast.ID, Name: broadcast.Name},
				Workspace: domain.RecipientFeedWorkspace{ID: workspaceID},
			}

			feedData, feedErr := s.dataFeedFetcher.FetchRecipient(ctx, broadcast.DataFeed.RecipientFeed, payload)
			if feedErr != nil {
				s.logger.WithFields(map[string]interface{}{
					"broadcast_id": broadcastID,
					"workspace_id": workspaceID,
					"recipient":    recipient.Contact.Email,
					"error":        feedErr.Error(),
				}).Error("Recipient feed fetch failed, pausing broadcast")
				// Return 0,0 — no entries were enqueued (batch Enqueue happens after loop)
				// The broadcast will be paused and the entire batch re-processed on resume
				return 0, 0, fmt.Errorf("%w: recipient feed failed for %s: %v",
					ErrBroadcastShouldPause, recipient.Contact.Email, feedErr)
			}

			data["recipient_feed"] = feedData
		}

		// Build queue entry
		entry, err := s.buildQueueEntry(ctx, workspaceID, integrationID, endpoint, trackingEnabled, broadcast, messageID, recipient.Contact.Email, template, data, emailProvider)
		if err != nil {
			s.logger.WithFields(map[string]interface{}{
				"broadcast_id": broadcastID,
				"workspace_id": workspaceID,
				"recipient":    recipient.Contact.Email,
				"error":        err.Error(),
			}).Warn("Failed to build queue entry")
			buildErrors++
			continue
		}

		entries = append(entries, entry)
	}

	if len(entries) == 0 {
		return 0, buildErrors, nil
	}

	// Enqueue all entries in batch
	if err := s.queueRepo.Enqueue(ctx, workspaceID, entries); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"workspace_id": workspaceID,
			"batch_size":   len(entries),
			"error":        err.Error(),
		}).Error("Failed to enqueue batch")
		return 0, len(recipients), NewBroadcastError(ErrCodeSendFailed, "failed to enqueue batch", true, err)
	}

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id": broadcastID,
		"workspace_id": workspaceID,
		"enqueued":     len(entries),
		"build_errors": buildErrors,
	}).Debug("Batch enqueued successfully")

	// Return enqueued as "sent" since from the orchestrator's perspective, the job is done
	return len(entries), buildErrors, nil
}

// buildQueueEntry creates an EmailQueueEntry for a recipient
func (s *queueMessageSender) buildQueueEntry(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	endpoint string,
	trackingEnabled bool,
	broadcast *domain.Broadcast,
	messageID string,
	email string,
	template *domain.Template,
	data map[string]interface{},
	emailProvider *domain.EmailProvider,
) (*domain.EmailQueueEntry, error) {
	// Ensure UTM parameters object is present
	if broadcast.UTMParameters == nil {
		broadcast.UTMParameters = &domain.UTMParameters{}
	}

	if broadcast.UTMParameters.Content == "" {
		broadcast.UTMParameters.Content = template.ID
	}

	// Build tracking settings
	trackingSettings := notifuse_mjml.TrackingSettings{
		Endpoint:       endpoint,
		EnableTracking: trackingEnabled,
		UTMSource:      broadcast.UTMParameters.Source,
		UTMMedium:      broadcast.UTMParameters.Medium,
		UTMCampaign:    broadcast.UTMParameters.Campaign,
		UTMContent:     broadcast.UTMParameters.Content,
		UTMTerm:        broadcast.UTMParameters.Term,
		WorkspaceID:    workspaceID,
		MessageID:      messageID,
	}

	// Get sender (use template's sender ID if specified, otherwise default)
	sender := emailProvider.GetSender(template.Email.SenderID)
	if sender == nil {
		return nil, fmt.Errorf("no sender configured for email provider")
	}

	// Compile template with the provided data
	compiledTemplate, err := notifuse_mjml.CompileTemplate(
		notifuse_mjml.CompileTemplateRequest{
			WorkspaceID:      workspaceID,
			MessageID:        messageID,
			VisualEditorTree: template.Email.VisualEditorTree,
			TemplateData:     data,
			TrackingSettings: trackingSettings,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to compile template: %w", err)
	}
	if !compiledTemplate.Success || compiledTemplate.HTML == nil {
		errMsg := "template compilation failed"
		if compiledTemplate.Error != nil {
			errMsg = compiledTemplate.Error.Message
		}
		return nil, fmt.Errorf("%s", errMsg)
	}
	htmlContent := *compiledTemplate.HTML

	// Process subject line through Liquid templating
	subject, err := notifuse_mjml.ProcessLiquidTemplate(
		template.Email.Subject,
		data,
		"email_subject",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to process subject: %w", err)
	}

	// Build the queue entry
	entry := &domain.EmailQueueEntry{
		ID:            uuid.New().String(),
		Status:        domain.EmailQueueStatusPending,
		Priority:      domain.EmailQueuePriorityMarketing,
		SourceType:    domain.EmailQueueSourceBroadcast,
		SourceID:      broadcast.ID,
		IntegrationID: integrationID,
		ProviderKind:  emailProvider.Kind,
		ContactEmail:  email,
		MessageID:     messageID,
		TemplateID:    template.ID,
		Payload: domain.EmailQueuePayload{
			FromAddress:        sender.Email,
			FromName:           sender.Name,
			Subject:            subject,
			HTMLContent:        htmlContent,
			RateLimitPerMinute: emailProvider.RateLimitPerMinute,
			EmailOptions: domain.EmailOptions{
				ReplyTo: template.Email.ReplyTo,
			},
			TemplateVersion: int(template.Version),
			ListID:          broadcast.Audience.List,
			TemplateData:    data, // Store template data for message history
		},
		MaxAttempts: 3,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	// Extract List-Unsubscribe URL from template data for RFC-8058 compliance (broadcast emails only)
	if unsubscribeURL, ok := data["oneclick_unsubscribe_url"].(string); ok && unsubscribeURL != "" {
		entry.Payload.EmailOptions.ListUnsubscribeURL = unsubscribeURL
	}

	return entry, nil
}

// selectTemplate selects a template for sending
// For A/B testing, this uses random selection; for normal sends, uses the first template
func (s *queueMessageSender) selectTemplate(templates map[string]*domain.Template, broadcast *domain.Broadcast) *domain.Template {
	if len(templates) == 0 {
		return nil
	}

	// If only one template, use it
	if len(templates) == 1 {
		for _, t := range templates {
			return t
		}
	}

	// For A/B testing, randomly select a template
	// Get template IDs in a consistent order
	var templateIDs []string
	for id := range templates {
		templateIDs = append(templateIDs, id)
	}

	// Secure random selection
	n, err := crand.Int(crand.Reader, big.NewInt(int64(len(templateIDs))))
	if err != nil {
		// Fallback to first template if random fails
		return templates[templateIDs[0]]
	}

	return templates[templateIDs[n.Int64()]]
}
