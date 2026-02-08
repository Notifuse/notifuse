package broadcast

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/google/uuid"
	"golang.org/x/sync/semaphore"
)

//go:generate mockgen -destination=./mocks/mock_message_sender.go -package=mocks github.com/Notifuse/notifuse/internal/service/broadcast MessageSender

// MessageSender is the interface for sending messages to recipients
type MessageSender interface {
	// SendToRecipient sends a message to a single recipient
	SendToRecipient(ctx context.Context, workspaceID string, integrationID string, endpoint string, trackingEnabled bool, broadcast *domain.Broadcast, messageID string, email string,
		template *domain.Template, data map[string]interface{}, emailProvider *domain.EmailProvider, timeoutAt time.Time) error

	// SendBatch sends messages to a batch of recipients
	SendBatch(ctx context.Context, workspaceID string, integrationID string, workspaceSecretKey string, endpoint string, trackingEnabled bool, broadcastID string, recipients []*domain.ContactWithList,
		templates map[string]*domain.Template, emailProvider *domain.EmailProvider, timeoutAt time.Time) (sent int, failed int, err error)
}

// CircuitBreaker provides circuit breaking functionality
type CircuitBreaker struct {
	failures       int
	threshold      int
	cooldownPeriod time.Duration
	lastFailure    time.Time
	lastError      error
	isOpen         bool
	mutex          sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(threshold int, cooldownPeriod time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:      threshold,
		cooldownPeriod: cooldownPeriod,
	}
}

// IsOpen checks if the circuit is open (preventing further calls)
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	// If circuit is open, check if cooldown period has passed
	if cb.isOpen {
		if time.Since(cb.lastFailure) > cb.cooldownPeriod {
			// Reset circuit after cooldown
			cb.mutex.RUnlock()
			cb.mutex.Lock()
			cb.isOpen = false
			cb.failures = 0
			cb.lastError = nil
			cb.mutex.Unlock()
			cb.mutex.RLock()
		}
	}

	return cb.isOpen
}

// RecordSuccess records a successful call
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures = 0
	cb.lastError = nil
	cb.isOpen = false
}

// RecordFailure records a failed call and opens circuit if threshold is reached
func (cb *CircuitBreaker) RecordFailure(err error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()
	cb.lastError = err

	if cb.failures >= cb.threshold {
		cb.isOpen = true
	}
}

// GetLastError returns the last error that caused a failure
func (cb *CircuitBreaker) GetLastError() error {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.lastError
}

// messageSender implements the MessageSender interface
type messageSender struct {
	broadcastRepo      domain.BroadcastRepository
	messageHistoryRepo domain.MessageHistoryRepository
	templateRepo       domain.TemplateRepository
	emailService       domain.EmailServiceInterface
	dataFeedFetcher    DataFeedFetcher
	logger             logger.Logger
	config             *Config
	circuitBreaker     *CircuitBreaker
	rateLimiter        *semaphore.Weighted
	lastSendTime       time.Time
	sendMutex          sync.Mutex
	apiEndpoint        string
}

// NewMessageSender creates a new message sender
func NewMessageSender(broadcastRepo domain.BroadcastRepository, messageHistoryRepo domain.MessageHistoryRepository, templateRepo domain.TemplateRepository,
	emailService domain.EmailServiceInterface, dataFeedFetcher DataFeedFetcher, logger logger.Logger, config *Config, apiEndpoint string) MessageSender {
	if config == nil {
		config = DefaultConfig()
	}

	var cb *CircuitBreaker
	if config.EnableCircuitBreaker {
		cb = NewCircuitBreaker(config.CircuitBreakerThreshold, config.CircuitBreakerCooldown)
	}

	// Calculate permits per second based on rate limit (per minute)
	permitsPerSecond := int64(config.DefaultRateLimit) / 60
	if permitsPerSecond < 1 {
		permitsPerSecond = 1
	}

	return &messageSender{
		broadcastRepo:      broadcastRepo,
		messageHistoryRepo: messageHistoryRepo,
		templateRepo:       templateRepo,
		emailService:       emailService,
		dataFeedFetcher:    dataFeedFetcher,
		logger:             logger,
		config:             config,
		circuitBreaker:     cb,
		rateLimiter:        semaphore.NewWeighted(permitsPerSecond),
		lastSendTime:       time.Now(),
		apiEndpoint:        apiEndpoint,
	}
}

// enforceRateLimit applies rate limiting to message sending
func (s *messageSender) enforceRateLimit(ctx context.Context, integrationRateLimit int) error {
	// Use integration rate limit (required field, should always be > 0)
	effectiveRateLimit := integrationRateLimit
	if effectiveRateLimit <= 0 {
		// Fallback to config default if somehow not set (shouldn't happen due to validation)
		effectiveRateLimit = s.config.DefaultRateLimit
	}

	// If rate limiting is disabled, return immediately
	if effectiveRateLimit <= 0 {
		return nil
	}

	// Calculate permits per second based on rate limit (per minute)
	permitsPerSecond := float64(effectiveRateLimit) / 60.0

	// Calculate the ideal time between messages
	// Convert to nanoseconds first to avoid truncation for rates < 60/min
	timeBetweenMessages := time.Duration(float64(time.Second) / permitsPerSecond)

	s.sendMutex.Lock()
	defer s.sendMutex.Unlock()

	// Calculate how long to wait
	elapsed := time.Since(s.lastSendTime)
	if elapsed < timeBetweenMessages {
		sleepTime := timeBetweenMessages - elapsed

		// Create a timer for the sleep duration
		timer := time.NewTimer(sleepTime)
		defer timer.Stop()

		// Wait for either the timer to expire or the context to be canceled
		select {
		case <-timer.C:
			// Timer expired, continue
		case <-ctx.Done():
			// Context canceled
			return ctx.Err()
		}
	}

	// Update last send time
	s.lastSendTime = time.Now()
	return nil
}

// SendToRecipient sends a message to a single recipient
func (s *messageSender) SendToRecipient(ctx context.Context, workspaceID string, integrationID string, endpoint string, trackingEnabled bool, broadcast *domain.Broadcast, messageID string, email string,
	template *domain.Template, data map[string]interface{}, emailProvider *domain.EmailProvider, timeoutAt time.Time) error {

	// Ensure UTM parameters object is present to avoid nil dereference
	if broadcast.UTMParameters == nil {
		empty := &domain.UTMParameters{}
		broadcast.UTMParameters = empty
	}

	// Check circuit breaker
	if s.circuitBreaker != nil && s.circuitBreaker.IsOpen() {
		lastError := s.circuitBreaker.GetLastError()
		logFields := map[string]interface{}{
			"broadcast_id": broadcast.ID,
			"workspace_id": workspaceID,
			"recipient":    email,
		}
		if lastError != nil {
			logFields["last_error"] = lastError.Error()
		}
		s.logger.WithFields(logFields).Warn("Circuit breaker open, skipping send")
		return NewBroadcastError(ErrCodeCircuitOpen, "circuit breaker is open", true, lastError)
	}

	// Apply rate limiting
	if err := s.enforceRateLimit(ctx, emailProvider.RateLimitPerMinute); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id":           broadcast.ID,
			"workspace_id":           workspaceID,
			"recipient":              email,
			"integration_rate_limit": emailProvider.RateLimitPerMinute,
			"error":                  err.Error(),
		}).Warn("Rate limiting interrupted by context cancellation")
		return NewBroadcastError(ErrCodeRateLimitExceeded, "rate limiting interrupted", true, err)
	}

	if broadcast.UTMParameters.Content == "" {
		broadcast.UTMParameters.Content = template.ID
	}

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
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcast.ID,
			"workspace_id": workspaceID,
			"recipient":    email,
			"template_id":  template.ID,
			"error":        err.Error(),
		}).Error("Failed to compile template")
		return NewBroadcastError(ErrCodeTemplateCompile, "failed to compile template", true, err)
	}

	if !compiledTemplate.Success || compiledTemplate.HTML == nil {
		errMsg := "Template compilation failed"
		if compiledTemplate.Error != nil {
			errMsg = compiledTemplate.Error.Message
		}
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcast.ID,
			"workspace_id": workspaceID,
			"recipient":    email,
			"template_id":  template.ID,
			"error":        errMsg,
		}).Error("Failed to generate HTML from template")
		return NewBroadcastError(ErrCodeTemplateCompile, errMsg, true, nil)
	}

	emailSender := emailProvider.GetSender(template.Email.SenderID)

	if emailSender == nil {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcast.ID,
			"workspace_id": workspaceID,
			"sender_id":    template.Email.SenderID,
			"recipient":    email,
		}).Error("Sender not found")
		return NewBroadcastError(ErrCodeSenderNotFound, "sender not found", true, nil)
	}

	// Process subject line through Liquid templating if it contains Liquid tags
	processedSubject, err := notifuse_mjml.ProcessLiquidTemplate(
		template.Email.Subject,
		data,
		"email_subject",
	)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcast.ID,
			"workspace_id": workspaceID,
			"recipient":    email,
			"subject":      template.Email.Subject,
			"error":        err.Error(),
		}).Error("Failed to process subject line with Liquid templating")
		return NewBroadcastError(ErrCodeTemplateCompile, "failed to process subject with Liquid", true, err)
	}

	// Create SendEmailProviderRequest
	emailRequest := domain.SendEmailProviderRequest{
		WorkspaceID:   workspaceID,
		IntegrationID: integrationID,
		MessageID:     messageID,
		FromAddress:   emailSender.Email,
		FromName:      emailSender.Name,
		To:            email,
		Subject:       processedSubject,
		Content:       *compiledTemplate.HTML,
		Provider:      emailProvider,
		EmailOptions: domain.EmailOptions{
			ReplyTo: template.Email.ReplyTo,
		},
	}

	// Extract List-Unsubscribe URL from template data for RFC-8058 compliance (broadcast emails only)
	if unsubscribeURL, ok := data["oneclick_unsubscribe_url"].(string); ok && unsubscribeURL != "" {
		emailRequest.EmailOptions.ListUnsubscribeURL = unsubscribeURL
	}

	// Now send email directly using compiled HTML rather than passing template to broadcastRepo
	// Note: Context is checked by SendEmail; in rare cancellation cases we may complete
	// template compilation (~50-100ms) before detecting cancellation. This is acceptable
	// as cancellations are infrequent and the work minimal.
	err = s.emailService.SendEmail(ctx, emailRequest, true)

	if err != nil {
		// Record failure in circuit breaker
		if s.circuitBreaker != nil {
			s.circuitBreaker.RecordFailure(err)
		}

		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcast.ID,
			"workspace_id": workspaceID,
			"recipient":    email,
			"error":        err.Error(),
		}).Error("Failed to send message")
		return NewBroadcastError(ErrCodeSendFailed, "failed to send message", true, err)
	}

	// Record success in circuit breaker
	if s.circuitBreaker != nil {
		s.circuitBreaker.RecordSuccess()
	}

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id": broadcast.ID,
		"workspace_id": workspaceID,
		"recipient":    email,
	}).Debug("Message sent successfully")

	return nil
}

// SendBatch sends messages to a batch of recipients
func (s *messageSender) SendBatch(ctx context.Context, workspaceID string, integrationID string, workspaceSecretKey string, endpoint string, trackingEnabled bool, broadcastID string, recipients []*domain.ContactWithList,
	templates map[string]*domain.Template, emailProvider *domain.EmailProvider, timeoutAt time.Time) (sent int, failed int, err error) {

	// Track specific error types for better reporting
	errorCounts := map[string]int{
		"template_not_found":    0,
		"template_data_failed":  0,
		"send_failed":           0,
		"empty_email":           0,
		"context_cancelled":     0,
		"recipient_feed_failed": 0,
		"recipient_skipped":     0,
	}
	var firstError error

	defer func() {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"workspace_id": workspaceID,
			"total":        len(recipients),
			"sent":         sent,
			"failed":       failed,
		}).Info("Batch send completed")
	}()

	// Check if we have any recipients
	if len(recipients) == 0 {
		return 0, 0, nil
	}

	// Check circuit breaker
	if s.circuitBreaker != nil && s.circuitBreaker.IsOpen() {
		lastError := s.circuitBreaker.GetLastError()
		logFields := map[string]interface{}{
			"broadcast_id": broadcastID,
			"workspace_id": workspaceID,
			"recipients":   len(recipients),
		}
		if lastError != nil {
			logFields["last_error"] = lastError.Error()
		}
		s.logger.WithFields(logFields).Warn("Circuit breaker open, skipping batch")
		return 0, 0, NewBroadcastError(ErrCodeCircuitOpen, "circuit breaker is open", true, lastError)
	}

	// Get the broadcast to determine variations and templates
	broadcast, err := s.broadcastRepo.GetBroadcast(ctx, workspaceID, broadcastID)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"workspace_id": workspaceID,
			"error":        err.Error(),
		}).Error("Failed to get broadcast for sending")
		return 0, 0, NewBroadcastError(ErrCodeBroadcastNotFound, "broadcast not found", false, err)
	}
	if broadcast == nil {
		// Defensive: repository returned nil without error
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"workspace_id": workspaceID,
		}).Error("Nil broadcast returned from repository")
		return 0, 0, NewBroadcastError(ErrCodeBroadcastNotFound, "broadcast not found", false, fmt.Errorf("nil broadcast"))
	}

	// Ensure UTM parameters is non-nil for downstream usage
	if broadcast.UTMParameters == nil {
		broadcast.UTMParameters = &domain.UTMParameters{}
	}

	// Log rate limiting configuration for this broadcast
	integrationRateLimit := emailProvider.RateLimitPerMinute
	if integrationRateLimit <= 0 {
		integrationRateLimit = s.config.DefaultRateLimit
	}

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id":           broadcastID,
		"workspace_id":           workspaceID,
		"integration_rate_limit": integrationRateLimit,
		"recipients":             len(recipients),
	}).Info("Starting batch send with rate limiting")

	// Send to each recipient
	for _, contactWithList := range recipients {
		// Extract the contact from the ContactWithList
		contact := contactWithList.Contact

		// Skip empty emails (shouldn't happen, but just in case)
		if contact == nil || contact.Email == "" {
			failed++
			errorCounts["empty_email"]++
			if firstError == nil {
				firstError = fmt.Errorf("contact has empty email")
			}
			continue
		}

		// Check time-based timeout instead of context cancellation
		if time.Now().After(timeoutAt) {
			// Note: This is NOT an error - just time limit reached
			s.logger.WithField("broadcast_id", broadcastID).Info("Time limit reached in batch processing")
			return sent, failed, nil // Return current progress, no error
		}

		// Determine which variation to use for this contact
		var templateID string
		if broadcast.WinningTemplate != nil {
			// Winner selected: use it directly
			templateID = *broadcast.WinningTemplate
		} else if broadcast.TestSettings.Enabled {
			// Test phase (no winner yet): pick a random variation per recipient
			if len(broadcast.TestSettings.Variations) > 0 {
				var idx int
				if r, err := crand.Int(crand.Reader, big.NewInt(int64(len(broadcast.TestSettings.Variations)))); err == nil {
					idx = int(r.Int64())
				} else {
					// Fallback: deterministic mapping if randomness fails
					idx = int(contact.Email[0]) % len(broadcast.TestSettings.Variations)
				}
				templateID = broadcast.TestSettings.Variations[idx].TemplateID
			}
		} else if len(broadcast.TestSettings.Variations) > 0 {
			// Not A/B testing: use the first variation
			templateID = broadcast.TestSettings.Variations[0].TemplateID
		}

		// Skip if no template ID was found or template is missing
		if templateID == "" || templates[templateID] == nil {
			s.logger.WithFields(map[string]interface{}{
				"broadcast_id": broadcastID,
				"workspace_id": workspaceID,
				"recipient":    contact.Email,
			}).Error("No template found for recipient")
			failed++
			errorCounts["template_not_found"]++
			if firstError == nil {
				firstError = fmt.Errorf("template not found for template_id: %s", templateID)
			}
			continue
		}

		// Generate a unique message ID for tracking
		messageID := generateMessageID(workspaceID)

		trackingSettings := notifuse_mjml.TrackingSettings{
			Endpoint:       endpoint,
			EnableTracking: trackingEnabled,
			UTMSource:      broadcast.UTMParameters.Source,
			UTMMedium:      broadcast.UTMParameters.Medium,
			UTMCampaign:    broadcast.UTMParameters.Campaign,
			UTMContent:     broadcast.UTMParameters.Content,
			WorkspaceID:    workspaceID,
			MessageID:      messageID,
		}

		if broadcast.UTMParameters.Content == "" {
			broadcast.UTMParameters.Content = templateID
		}

		// Build the template data with all options
		req := domain.TemplateDataRequest{
			WorkspaceID:        workspaceID,
			WorkspaceSecretKey: workspaceSecretKey,
			ContactWithList:    *contactWithList,
			MessageID:          messageID,
			TrackingSettings:   trackingSettings,
			Broadcast:          broadcast,
		}
		recipientData, err := domain.BuildTemplateData(req)
		if err != nil {
			s.logger.WithFields(map[string]interface{}{
				"broadcast_id": broadcastID,
				"workspace_id": workspaceID,
				"recipient":    contact.Email,
				"error":        err.Error(),
			}).Error("Failed to build template data")
			failed++
			errorCounts["template_data_failed"]++
			if firstError == nil {
				firstError = fmt.Errorf("template data build failed: %w", err)
			}
			continue
		}

		// Note: global_feed data is now injected by BuildTemplateData (domain/template.go)

		// Fetch recipient feed if configured and enabled
		if broadcast.DataFeed != nil && broadcast.DataFeed.RecipientFeed != nil && broadcast.DataFeed.RecipientFeed.Enabled && s.dataFeedFetcher != nil {
			// Build the recipient feed payload
			payload := &domain.RecipientFeedRequestPayload{
				Contact: domain.BuildRecipientFeedContact(contact),
				List: domain.RecipientFeedList{
					ID:   contactWithList.ListID,
					Name: contactWithList.ListName,
				},
				Broadcast: domain.RecipientFeedBroadcast{
					ID:   broadcast.ID,
					Name: broadcast.Name,
				},
				Workspace: domain.RecipientFeedWorkspace{
					ID: workspaceID,
				},
			}

			feedData, feedErr := s.dataFeedFetcher.FetchRecipient(ctx, broadcast.DataFeed.RecipientFeed, payload)
			if feedErr != nil {
				// Feed failed after retries - pause broadcast immediately
				// If feed data is configured, it's mandatory - no skipping, no sending without data
				s.logger.WithFields(map[string]interface{}{
					"broadcast_id": broadcastID,
					"workspace_id": workspaceID,
					"recipient":    contact.Email,
					"error":        feedErr.Error(),
				}).Error("Recipient feed fetch failed, pausing broadcast")

				return sent, failed, fmt.Errorf("%w: recipient feed failed for %s: %v",
					ErrBroadcastShouldPause, contact.Email, feedErr)
			}

			// Success - add feed data to template context
			recipientData["recipient_feed"] = feedData
		}

		// Send to the recipient
		err = s.SendToRecipient(ctx, workspaceID, integrationID, endpoint, trackingEnabled, broadcast, messageID, contact.Email, templates[templateID], recipientData, emailProvider, timeoutAt)
		if err != nil {
			// SendToRecipient already logs errors
			failed++
			errorCounts["send_failed"]++
			if firstError == nil {
				firstError = fmt.Errorf("send failed: %w", err)
			}
		} else {
			sent++
		}

		now := time.Now().UTC()
		listID := broadcast.Audience.List
		message := &domain.MessageHistory{
			ID:              messageID,
			ContactEmail:    contact.Email,
			BroadcastID:     &broadcastID,
			ListID:          &listID,
			TemplateID:      templateID,
			TemplateVersion: templates[templateID].Version,
			Channel:         "email",
			MessageData: domain.MessageData{
				Data: recipientData,
			},
			SentAt:    now,
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err != nil {
			message.FailedAt = &now
			errStr := fmt.Sprintf("%.255s", err.Error())
			message.StatusInfo = &errStr
		}

		// Record the message
		if err := s.messageHistoryRepo.Create(ctx, workspaceID, workspaceSecretKey, message); err != nil {
			s.logger.WithFields(map[string]interface{}{
				"broadcast_id": broadcastID,
				"workspace_id": workspaceID,
				"recipient":    contact.Email,
				"message_id":   messageID,
				"error":        err.Error(),
			}).Warn("Failed to record message history, but email was sent")
			// Don't return an error here since the message was already sent successfully
		} else {
			s.logger.WithFields(map[string]interface{}{
				"broadcast_id": broadcastID,
				"workspace_id": workspaceID,
				"recipient":    contact.Email,
				"message_id":   messageID,
			}).Debug("Message history recorded successfully")
		}
	}

	// Record success/failure in circuit breaker based on overall success rate
	if s.circuitBreaker != nil {
		if failed > sent {
			// Create detailed error message with breakdown
			var errorDetails []string
			for errorType, count := range errorCounts {
				if count > 0 {
					errorDetails = append(errorDetails, fmt.Sprintf("%s: %d", errorType, count))
				}
			}

			var detailedError error
			if len(errorDetails) > 0 {
				detailedError = fmt.Errorf("batch send failed (sent: %d, failed: %d) - errors: %s. First error: %v",
					sent, failed, strings.Join(errorDetails, ", "), firstError)
			} else {
				detailedError = fmt.Errorf("batch send failed (sent: %d, failed: %d). First error: %v",
					sent, failed, firstError)
			}

			s.circuitBreaker.RecordFailure(detailedError)
		} else if sent > 0 {
			s.circuitBreaker.RecordSuccess()
		}
	}

	return sent, failed, nil
}

// generateMessageID creates a unique message ID for tracking
func generateMessageID(workspaceID string) string {
	return fmt.Sprintf("%s_%s", workspaceID, uuid.New().String())
}
