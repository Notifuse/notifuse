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
	workspaceSecretKey string,
	endpoint string,
	trackingEnabled bool,
	webAnalytics *domain.WebAnalyticsSettings,
	broadcast *domain.Broadcast,
	messageID string,
	email string,
	template *domain.Template,
	data map[string]interface{},
	emailProvider *domain.EmailProvider,
	timeoutAt time.Time,
	contactLanguage string,
	workspaceDefaultLanguage string,
) error {
	// Build the email payload
	entry, err := s.buildQueueEntry(ctx, workspaceID, integrationID, workspaceSecretKey, endpoint, trackingEnabled, webAnalytics, broadcast, messageID, email, template, data, emailProvider, contactLanguage, workspaceDefaultLanguage)
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
	websiteURL string,
	trackingEnabled bool,
	webAnalytics *domain.WebAnalyticsSettings,
	broadcastID string,
	recipients []*domain.ContactWithList,
	templates map[string]*domain.Template,
	emailProvider *domain.EmailProvider,
	timeoutAt time.Time,
	workspaceDefaultLanguage string,
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

		// Default utm_content on a per-recipient copy: the broadcast is shared
		// across recipients and A/B variants differ per recipient, so the shared
		// UTM parameters must never be mutated.
		var utmParams domain.UTMParameters
		if broadcast.UTMParameters != nil {
			utmParams = *broadcast.UTMParameters
		}
		if utmParams.Content == "" {
			utmParams.Content = template.ID
		}

		// Build tracking settings for BuildTemplateData. Deliberately no identity
		// token: BuildTemplateData reads the endpoint and the UTM fields and never
		// looks at IdentifyToken, and the settings the link rewriter sees are built
		// in buildQueueEntry below, which mints this recipient's own token there.
		// Minting a second one here would encrypt a live per-recipient credential
		// once more per contact in the batch loop and park it in a struct nothing
		// reads.
		trackingSettings := notifuse_mjml.TrackingSettings{
			Endpoint:       endpoint,
			EnableTracking: trackingEnabled,
			UTMSource:      utmParams.Source,
			UTMMedium:      utmParams.Medium,
			UTMCampaign:    utmParams.Campaign,
			UTMContent:     utmParams.Content,
			UTMTerm:        utmParams.Term,
			WorkspaceID:    workspaceID,
			MessageID:      messageID,
		}

		// Build template data with all system variables (unsubscribe_url, notification_center_url, etc.)
		req := domain.TemplateDataRequest{
			WorkspaceID:         workspaceID,
			WorkspaceSecretKey:  workspaceSecretKey,
			WorkspaceWebsiteURL: websiteURL,
			ContactWithList:     *recipient,
			MessageID:           messageID,
			TrackingSettings:    trackingSettings,
			Broadcast:           broadcast,
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

		// Extract contact language for variant resolution
		contactLanguage := ""
		if recipient.Contact.Language != nil && !recipient.Contact.Language.IsNull {
			contactLanguage = recipient.Contact.Language.String
		}

		// Build queue entry
		entry, err := s.buildQueueEntry(ctx, workspaceID, integrationID, workspaceSecretKey, endpoint, trackingEnabled, webAnalytics, broadcast, messageID, recipient.Contact.Email, template, data, emailProvider, contactLanguage, workspaceDefaultLanguage)
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
		// Nothing was written: the enqueue is a single transaction. Reporting
		// the batch as processed (sent+failed) made the orchestrator advance
		// its offset and keyset cursor past recipients that were never
		// enqueued, so they were skipped for good while the broadcast still
		// declared itself processed.
		return 0, 0, NewBroadcastError(ErrCodeSendFailed, "failed to enqueue batch", true, err)
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
	workspaceSecretKey string,
	endpoint string,
	trackingEnabled bool,
	webAnalytics *domain.WebAnalyticsSettings,
	broadcast *domain.Broadcast,
	messageID string,
	email string,
	template *domain.Template,
	data map[string]interface{},
	emailProvider *domain.EmailProvider,
	contactLanguage string,
	workspaceDefaultLanguage string,
) (*domain.EmailQueueEntry, error) {
	// Default utm_content on a per-recipient copy: the broadcast is shared
	// across recipients and A/B variants differ per recipient, so the shared
	// UTM parameters must never be mutated.
	var utmParams domain.UTMParameters
	if broadcast.UTMParameters != nil {
		utmParams = *broadcast.UTMParameters
	}
	if utmParams.Content == "" {
		utmParams.Content = template.ID
	}

	// This function builds one entry for one recipient, so the token minted
	// here is that recipient's own. A mint failure is logged and the entry is
	// built without it: an unidentified visit costs analytics precision, a
	// dropped entry costs the email.
	identifyToken, identifyAllowedHosts, identifyErr := mintIdentifyToken(email, workspaceSecretKey, webAnalytics)
	if identifyErr != nil {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcast.ID,
			"workspace_id": workspaceID,
			"recipient":    email,
			"error":        identifyErr.Error(),
		}).Warn("Failed to mint web analytics identity token, enqueueing without it")
	}

	// Build tracking settings
	trackingSettings := notifuse_mjml.TrackingSettings{
		Endpoint:             endpoint,
		EnableTracking:       trackingEnabled,
		UTMSource:            utmParams.Source,
		UTMMedium:            utmParams.Medium,
		UTMCampaign:          utmParams.Campaign,
		UTMContent:           utmParams.Content,
		UTMTerm:              utmParams.Term,
		WorkspaceID:          workspaceID,
		MessageID:            messageID,
		IdentifyToken:        identifyToken,
		IdentifyAllowedHosts: identifyAllowedHosts,
	}

	// Resolve language variant
	emailContent := template.ResolveEmailContent(contactLanguage, workspaceDefaultLanguage)
	if emailContent == nil {
		return nil, fmt.Errorf("email content not available after language resolution")
	}

	// Get sender (use template's sender ID if specified, otherwise default)
	sender := emailProvider.GetSender(emailContent.SenderID)
	if sender == nil {
		return nil, fmt.Errorf("no sender configured for email provider")
	}

	// Compile template with the provided data
	compileReq := notifuse_mjml.CompileTemplateRequest{
		WorkspaceID:      workspaceID,
		MessageID:        messageID,
		TemplateData:     data,
		TrackingSettings: trackingSettings,
	}
	// Wires the resolved variant's tree/source + its inbox-preview override.
	emailContent.ApplyToCompileRequest(&compileReq, nil)
	compiledTemplate, err := notifuse_mjml.CompileTemplate(compileReq)
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
		emailContent.Subject,
		data,
		"email_subject",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to process subject: %w", err)
	}

	// Build the queue entry.
	//
	// Note what this stores: htmlContent carries this recipient's nf_id on every
	// allowed link, and Payload is persisted as JSONB in the workspace's
	// email_queue table. The row is deleted when the entry is sent
	// (MarkAsSent) and when the worker gives up on it, but for as long as it
	// waits — a long queue, or a broadcast paused mid-send, whose entries keep
	// status 'paused' until it is resumed or deleted — a credential that stays
	// valid for domain.WebIdentifyTokenTTL sits in the database. Only the
	// TrackingSettings struct itself stays out of storage; the HTML it produced
	// does not.
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
				ReplyTo: emailContent.ReplyTo,
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
