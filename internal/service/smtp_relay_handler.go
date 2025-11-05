package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang-jwt/jwt/v5"
)

// SMTPRelayHandlerService handles incoming SMTP relay messages and converts them to transactional notifications
type SMTPRelayHandlerService struct {
	authService                      *AuthService
	transactionalNotificationService domain.TransactionalNotificationService
	workspaceRepo                    domain.WorkspaceRepository
	logger                           logger.Logger
	jwtSecret                        []byte
	rateLimiter                      *RateLimiter
}

// NewSMTPRelayHandlerService creates a new SMTP relay handler service
func NewSMTPRelayHandlerService(
	authService *AuthService,
	transactionalNotificationService domain.TransactionalNotificationService,
	workspaceRepo domain.WorkspaceRepository,
	logger logger.Logger,
	jwtSecret []byte,
	rateLimiter *RateLimiter,
) *SMTPRelayHandlerService {
	return &SMTPRelayHandlerService{
		authService:                      authService,
		transactionalNotificationService: transactionalNotificationService,
		workspaceRepo:                    workspaceRepo,
		logger:                           logger,
		jwtSecret:                        jwtSecret,
		rateLimiter:                      rateLimiter,
	}
}

// Authenticate validates the SMTP credentials (api_email and api_key)
// Returns the user_id if authentication succeeds
// Note: workspace_id will be extracted from the JSON payload in the email body
func (s *SMTPRelayHandlerService) Authenticate(username, password string) (string, error) {
	apiEmail := username
	apiKey := password

	// Check rate limit
	if !s.rateLimiter.Allow(apiEmail) {
		s.logger.WithField("api_email", apiEmail).Warn("SMTP relay: Rate limit exceeded")
		return "", fmt.Errorf("rate limit exceeded")
	}

	// Validate the API key (JWT token)
	claims := &UserClaims{}
	token, err := jwt.ParseWithClaims(apiKey, claims, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method to prevent algorithm confusion
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"api_email": apiEmail,
			"error":     err.Error(),
		}).Warn("SMTP relay: Invalid API key token")
		return "", fmt.Errorf("invalid API key: %w", err)
	}

	if !token.Valid {
		s.logger.WithField("api_email", apiEmail).Warn("SMTP relay: Invalid API key token")
		return "", fmt.Errorf("invalid API key token")
	}

	// Validate that the user is an API key type
	if claims.Type != string(domain.UserTypeAPIKey) {
		s.logger.WithFields(map[string]interface{}{
			"api_email": apiEmail,
			"user_type": claims.Type,
		}).Warn("SMTP relay: Token is not an API key")
		return "", fmt.Errorf("token must be an API key")
	}

	// Verify the email in the token matches the provided username
	if claims.Email != apiEmail {
		s.logger.WithFields(map[string]interface{}{
			"api_email":   apiEmail,
			"token_email": claims.Email,
			"user_id":     claims.UserID,
		}).Warn("SMTP relay: Email mismatch")
		return "", fmt.Errorf("email does not match token")
	}

	s.logger.WithFields(map[string]interface{}{
		"api_email": apiEmail,
		"user_id":   claims.UserID,
	}).Info("SMTP relay: Authentication successful")

	// Reset rate limit on successful authentication
	s.rateLimiter.Reset(apiEmail)

	// Return the user_id which will be used to look up workspaces when processing the message
	return claims.UserID, nil
}

// HandleMessage processes an incoming SMTP message and triggers a transactional notification
// The userID parameter is the authenticated API key user's ID
func (s *SMTPRelayHandlerService) HandleMessage(userID string, from string, to []string, data []byte) error {
	s.logger.WithFields(map[string]interface{}{
		"user_id": userID,
		"from":    from,
		"to":      to,
		"size":    len(data),
	}).Debug("SMTP relay: Processing message")

	// Parse the email message
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"user_id": userID,
			"error":   err.Error(),
		}).Error("SMTP relay: Failed to parse email message")
		return fmt.Errorf("failed to parse email: %w", err)
	}

	// Extract Message-ID header for idempotency
	messageID := msg.Header.Get("Message-ID")
	if messageID != "" {
		// Clean up Message-ID (remove angle brackets if present)
		messageID = strings.Trim(messageID, "<>")
	}

	// Extract email headers for CC, BCC, and Reply-To
	emailHeaders := s.extractEmailHeaders(msg)

	// Extract the JSON payload from the email body
	jsonPayload, err := s.extractJSONPayload(msg)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"user_id": userID,
			"error":   err.Error(),
		}).Error("SMTP relay: Failed to extract JSON payload")
		return fmt.Errorf("failed to extract JSON payload: %w", err)
	}

	// Parse the notification parameters (includes workspace_id)
	var payload struct {
		WorkspaceID  string                                     `json:"workspace_id"`
		Notification domain.TransactionalNotificationSendParams `json:"notification"`
	}

	if err := json.Unmarshal(jsonPayload, &payload); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"user_id": userID,
			"error":   err.Error(),
		}).Error("SMTP relay: Failed to parse JSON payload")
		return fmt.Errorf("email body is not valid JSON: %w", err)
	}

	// Validate workspace_id is provided
	if payload.WorkspaceID == "" {
		s.logger.WithField("user_id", userID).Error("SMTP relay: Missing workspace_id in payload")
		return fmt.Errorf("workspace_id is required in JSON payload")
	}

	workspaceID := payload.WorkspaceID

	// Create context with timeout for processing
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verify the user has access to the workspace
	userWorkspace, err := s.workspaceRepo.GetUserWorkspace(ctx, userID, workspaceID)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"user_id":      userID,
			"workspace_id": workspaceID,
			"error":        err.Error(),
		}).Warn("SMTP relay: User does not have access to workspace")
		return fmt.Errorf("user does not have access to workspace: %w", err)
	}

	if userWorkspace == nil {
		s.logger.WithFields(map[string]interface{}{
			"user_id":      userID,
			"workspace_id": workspaceID,
		}).Warn("SMTP relay: User workspace not found")
		return fmt.Errorf("user does not have access to workspace")
	}

	// Merge email headers with the JSON payload (JSON takes precedence)
	s.mergeEmailHeaders(&payload.Notification, emailHeaders)

	// Set external_id from Message-ID if not provided in JSON
	if payload.Notification.ExternalID == nil && messageID != "" {
		payload.Notification.ExternalID = &messageID
		s.logger.WithFields(map[string]interface{}{
			"user_id":    userID,
			"message_id": messageID,
		}).Debug("SMTP relay: Using Message-ID as external_id")
	}

	// Validate the notification parameters
	if payload.Notification.ID == "" {
		return fmt.Errorf("notification.id is required")
	}
	if payload.Notification.Contact == nil {
		return fmt.Errorf("notification.contact is required")
	}
	if payload.Notification.Contact.Email == "" {
		return fmt.Errorf("notification.contact.email is required")
	}

	// Send the transactional notification
	sentMessageID, err := s.transactionalNotificationService.SendNotification(ctx, workspaceID, payload.Notification)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"workspace_id":    workspaceID,
			"notification_id": payload.Notification.ID,
			"error":           err.Error(),
		}).Error("SMTP relay: Failed to send notification")
		return fmt.Errorf("failed to send notification: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace_id":    workspaceID,
		"notification_id": payload.Notification.ID,
		"message_id":      sentMessageID,
	}).Info("SMTP relay: Notification sent successfully")

	return nil
}

// extractJSONPayload extracts the JSON payload from the email body
func (s *SMTPRelayHandlerService) extractJSONPayload(msg *mail.Message) ([]byte, error) {
	contentType := msg.Header.Get("Content-Type")

	// Handle multipart messages
	if strings.HasPrefix(contentType, "multipart/") {
		mediaType, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return nil, fmt.Errorf("failed to parse content type: %w", err)
		}

		if strings.HasPrefix(mediaType, "multipart/") {
			boundary := params["boundary"]
			mr := multipart.NewReader(msg.Body, boundary)

			// Iterate through parts to find JSON content
			for {
				part, err := mr.NextPart()
				if err != nil {
					break
				}

				partContentType := part.Header.Get("Content-Type")

				// Look for text/plain or application/json parts
				if strings.HasPrefix(partContentType, "text/plain") ||
					strings.HasPrefix(partContentType, "application/json") {
					data := new(bytes.Buffer)
					if _, err := data.ReadFrom(part); err != nil {
						continue
					}

					// Try to parse as JSON
					var test interface{}
					if json.Unmarshal(data.Bytes(), &test) == nil {
						return data.Bytes(), nil
					}
				}
			}
		}
	}

	// Handle simple text/plain messages
	body := new(bytes.Buffer)
	if _, err := body.ReadFrom(msg.Body); err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	bodyBytes := body.Bytes()

	// Validate it's JSON
	var test interface{}
	if err := json.Unmarshal(bodyBytes, &test); err != nil {
		return nil, fmt.Errorf("email body is not valid JSON: %w", err)
	}

	return bodyBytes, nil
}

// extractedEmailHeaders holds email headers extracted from the message
type extractedEmailHeaders struct {
	CC      []string
	BCC     []string
	ReplyTo string
}

// extractEmailHeaders extracts CC, BCC, and Reply-To from email headers
func (s *SMTPRelayHandlerService) extractEmailHeaders(msg *mail.Message) *extractedEmailHeaders {
	headers := &extractedEmailHeaders{}

	// Extract CC
	if ccHeader := msg.Header.Get("Cc"); ccHeader != "" {
		headers.CC = parseEmailAddresses(ccHeader)
	}

	// Extract BCC
	if bccHeader := msg.Header.Get("Bcc"); bccHeader != "" {
		headers.BCC = parseEmailAddresses(bccHeader)
	}

	// Extract Reply-To
	if replyToHeader := msg.Header.Get("Reply-To"); replyToHeader != "" {
		addresses := parseEmailAddresses(replyToHeader)
		if len(addresses) > 0 {
			headers.ReplyTo = addresses[0] // Take the first Reply-To address
		}
	}

	return headers
}

// parseEmailAddresses parses a comma-separated list of email addresses
// Handles formats like: "user@example.com, User <user2@example.com>"
func parseEmailAddresses(addressList string) []string {
	addresses := []string{}

	// Split by comma
	parts := strings.Split(addressList, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Try to parse as RFC 5322 address
		addr, err := mail.ParseAddress(part)
		if err == nil {
			addresses = append(addresses, addr.Address)
		} else {
			// If parsing fails, use the string as-is (might be a simple email)
			// Basic email validation could be added here
			if strings.Contains(part, "@") {
				addresses = append(addresses, part)
			}
		}
	}

	return addresses
}

// mergeEmailHeaders merges extracted email headers into the notification payload
// JSON payload takes precedence over email headers
func (s *SMTPRelayHandlerService) mergeEmailHeaders(
	notification *domain.TransactionalNotificationSendParams,
	headers *extractedEmailHeaders,
) {
	// Only set CC if not already specified in JSON and headers have values
	if len(notification.EmailOptions.CC) == 0 && len(headers.CC) > 0 {
		notification.EmailOptions.CC = headers.CC
		s.logger.WithField("cc", headers.CC).Debug("SMTP relay: Using CC from email headers")
	}

	// Only set BCC if not already specified in JSON and headers have values
	if len(notification.EmailOptions.BCC) == 0 && len(headers.BCC) > 0 {
		notification.EmailOptions.BCC = headers.BCC
		s.logger.WithField("bcc", headers.BCC).Debug("SMTP relay: Using BCC from email headers")
	}

	// Only set Reply-To if not already specified in JSON and header has a value
	if notification.EmailOptions.ReplyTo == "" && headers.ReplyTo != "" {
		notification.EmailOptions.ReplyTo = headers.ReplyTo
		s.logger.WithField("reply_to", headers.ReplyTo).Debug("SMTP relay: Using Reply-To from email headers")
	}
}
