package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"strings"
	"unicode"

	"github.com/Notifuse/notifuse/internal/domain"
	"golang.org/x/net/idna"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/aws/aws-sdk-go/service/sns"
)

// Custom domain errors for better testability
var (
	ErrInvalidAWSCredentials = fmt.Errorf("invalid AWS credentials")
	ErrInvalidSNSDestination = fmt.Errorf("SNS destination and Topic ARN are required")
	ErrInvalidSESConfig      = fmt.Errorf("SES configuration is missing or invalid")
)

// isASCII checks if a string contains only ASCII characters
func isASCII(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// encodeRFC2047 encodes a string for use in email headers if it contains non-ASCII characters
func encodeRFC2047(s string) string {
	if isASCII(s) {
		return s
	}
	return mime.BEncoding.Encode("UTF-8", s)
}

// encodeEmailAddress encodes an email address for SES compatibility
// Local part is encoded using RFC 2047 B encoding if it contains non-ASCII characters
// Domain part is converted to Punycode (IDNA) for international domains
func encodeEmailAddress(email string) (string, error) {
	atIndex := strings.LastIndex(email, "@")
	if atIndex == -1 {
		return email, nil // Invalid email format, return as-is
	}

	local := email[:atIndex]
	domain := email[atIndex+1:]

	// Encode local part using RFC 2047 B encoding if it contains non-ASCII characters
	if !isASCII(local) {
		local = mime.BEncoding.Encode("UTF-8", local)
	}

	// Convert international domain to Punycode
	asciiDomain, err := idna.ToASCII(domain)
	if err != nil {
		return "", fmt.Errorf("failed to encode domain: %w", err)
	}

	return local + "@" + asciiDomain, nil
}

// formatFromHeader formats the From header with proper RFC 2047 encoding
func formatFromHeader(name, address string) (string, error) {
	encodedAddr, err := encodeEmailAddress(address)
	if err != nil {
		return "", err
	}

	if name == "" {
		return encodedAddr, nil
	}

	encodedName := encodeRFC2047(name)
	return fmt.Sprintf("%s <%s>", encodedName, encodedAddr), nil
}

// SESService implements the domain.SESServiceInterface
type SESService struct {
	authService           domain.AuthService
	logger                logger.Logger
	sessionFactory        func(config domain.AmazonSESSettings) (*session.Session, error)
	sesClientFactory      func(sess *session.Session) domain.SESWebhookClient
	snsClientFactory      func(sess *session.Session) domain.SNSWebhookClient
	sesEmailClientFactory func(sess *session.Session) domain.SESClient
}

// NewSESService creates a new instance of SESService with default factories
func NewSESService(authService domain.AuthService, logger logger.Logger) *SESService {
	return &SESService{
		authService: authService,
		logger:      logger,
		sessionFactory: func(config domain.AmazonSESSettings) (*session.Session, error) {
			return createSession(config)
		},
		sesClientFactory: func(sess *session.Session) domain.SESWebhookClient {
			return ses.New(sess)
		},
		snsClientFactory: func(sess *session.Session) domain.SNSWebhookClient {
			return sns.New(sess)
		},
		sesEmailClientFactory: func(sess *session.Session) domain.SESClient {
			return ses.New(sess)
		},
	}
}

// NewSESServiceWithClients creates a new instance of SESService with custom factories for testing
func NewSESServiceWithClients(
	authService domain.AuthService,
	logger logger.Logger,
	sessionFactory func(config domain.AmazonSESSettings) (*session.Session, error),
	sesClientFactory func(sess *session.Session) domain.SESWebhookClient,
	snsClientFactory func(sess *session.Session) domain.SNSWebhookClient,
	sesEmailClientFactory func(sess *session.Session) domain.SESClient,
) *SESService {
	return &SESService{
		authService:           authService,
		logger:                logger,
		sessionFactory:        sessionFactory,
		sesClientFactory:      sesClientFactory,
		snsClientFactory:      snsClientFactory,
		sesEmailClientFactory: sesEmailClientFactory,
	}
}

// createSession creates an AWS session with the given configuration
func createSession(config domain.AmazonSESSettings) (*session.Session, error) {
	return session.NewSession(&aws.Config{
		Region:      aws.String(config.Region),
		Credentials: credentials.NewStaticCredentials(config.AccessKey, config.SecretKey, ""),
	})
}

// getClients creates AWS session and returns SES and SNS clients
func (s *SESService) getClients(config domain.AmazonSESSettings) (domain.SESWebhookClient, domain.SNSWebhookClient, error) {
	if config.AccessKey == "" || config.SecretKey == "" {
		return nil, nil, ErrInvalidAWSCredentials
	}

	sess, err := s.sessionFactory(config)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create AWS session: %v", err))
		return nil, nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	sesClient := s.sesClientFactory(sess)
	snsClient := s.snsClientFactory(sess)

	return sesClient, snsClient, nil
}

// ListConfigurationSets lists all configuration sets
func (s *SESService) ListConfigurationSets(ctx context.Context, config domain.AmazonSESSettings) ([]string, error) {
	sesClient, _, err := s.getClients(config)
	if err != nil {
		return nil, err
	}

	// List configuration sets
	input := &ses.ListConfigurationSetsInput{}
	result, err := sesClient.ListConfigurationSetsWithContext(ctx, input)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to list SES configuration sets: %v", err))
		return nil, fmt.Errorf("failed to list SES configuration sets: %w", err)
	}

	// Extract configuration set names
	var configSets []string
	for _, configSet := range result.ConfigurationSets {
		configSets = append(configSets, *configSet.Name)
	}

	return configSets, nil
}

// CreateConfigurationSet creates a new configuration set
func (s *SESService) CreateConfigurationSet(ctx context.Context, config domain.AmazonSESSettings, name string) error {
	sesClient, _, err := s.getClients(config)
	if err != nil {
		return err
	}

	// Create configuration set
	input := &ses.CreateConfigurationSetInput{
		ConfigurationSet: &ses.ConfigurationSet{
			Name: aws.String(name),
		},
	}

	_, err = sesClient.CreateConfigurationSetWithContext(ctx, input)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create SES configuration set: %v", err))
		return fmt.Errorf("failed to create SES configuration set: %w", err)
	}

	return nil
}

// DeleteConfigurationSet deletes a configuration set
func (s *SESService) DeleteConfigurationSet(ctx context.Context, config domain.AmazonSESSettings, name string) error {
	sesClient, _, err := s.getClients(config)
	if err != nil {
		return err
	}

	// Delete configuration set
	input := &ses.DeleteConfigurationSetInput{
		ConfigurationSetName: aws.String(name),
	}

	_, err = sesClient.DeleteConfigurationSetWithContext(ctx, input)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to delete SES configuration set: %v", err))
		return fmt.Errorf("failed to delete SES configuration set: %w", err)
	}

	return nil
}

// CreateSNSTopic creates a new SNS topic for notifications
func (s *SESService) CreateSNSTopic(ctx context.Context, config domain.AmazonSESSettings, topicConfig domain.SESTopicConfig) (string, error) {
	_, snsClient, err := s.getClients(config)
	if err != nil {
		return "", err
	}

	// If a topic ARN is provided, check if it exists
	if topicConfig.TopicARN != "" {
		// Check if the topic exists
		_, err := snsClient.GetTopicAttributesWithContext(ctx, &sns.GetTopicAttributesInput{
			TopicArn: aws.String(topicConfig.TopicARN),
		})
		if err != nil {
			s.logger.Error(fmt.Sprintf("Failed to get SNS topic attributes: %v", err))
			return "", fmt.Errorf("failed to get SNS topic attributes: %w", err)
		}
		return topicConfig.TopicARN, nil
	}

	// Create a new SNS topic if no ARN was provided
	topicName := topicConfig.TopicName
	if topicName == "" {
		topicName = "notifuse-email-webhooks"
	}

	createResult, err := snsClient.CreateTopicWithContext(ctx, &sns.CreateTopicInput{
		Name: aws.String(topicName),
	})
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create SNS topic: %v", err))
		return "", fmt.Errorf("failed to create SNS topic: %w", err)
	}

	topicARN := *createResult.TopicArn

	// Configure the SNS subscription for the webhook endpoint
	_, err = snsClient.SubscribeWithContext(ctx, &sns.SubscribeInput{
		Protocol: aws.String(topicConfig.Protocol),
		TopicArn: aws.String(topicARN),
		Endpoint: aws.String(topicConfig.NotificationEndpoint),
	})
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create SNS subscription: %v", err))
		return "", fmt.Errorf("failed to create SNS subscription: %w", err)
	}

	return topicARN, nil
}

// DeleteSNSTopic deletes an SNS topic
func (s *SESService) DeleteSNSTopic(ctx context.Context, config domain.AmazonSESSettings, topicARN string) error {
	_, snsClient, err := s.getClients(config)
	if err != nil {
		return err
	}

	// Delete the SNS topic
	_, err = snsClient.DeleteTopicWithContext(ctx, &sns.DeleteTopicInput{
		TopicArn: aws.String(topicARN),
	})
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to delete SNS topic: %v", err))
		return fmt.Errorf("failed to delete SNS topic: %w", err)
	}

	return nil
}

// CreateEventDestination creates an event destination in a configuration set
func (s *SESService) CreateEventDestination(ctx context.Context, config domain.AmazonSESSettings, destination domain.SESConfigurationSetEventDestination) error {
	sesClient, _, err := s.getClients(config)
	if err != nil {
		return err
	}

	// Validate destination
	if destination.SNSDestination == nil || destination.SNSDestination.TopicARN == "" {
		return ErrInvalidSNSDestination
	}

	// Create event destination
	input := &ses.CreateConfigurationSetEventDestinationInput{
		ConfigurationSetName: aws.String(destination.ConfigurationSetName),
		EventDestination: &ses.EventDestination{
			Name:               aws.String(destination.Name),
			Enabled:            aws.Bool(destination.Enabled),
			MatchingEventTypes: aws.StringSlice(destination.MatchingEventTypes),
			SNSDestination: &ses.SNSDestination{
				TopicARN: aws.String(destination.SNSDestination.TopicARN),
			},
		},
	}

	_, err = sesClient.CreateConfigurationSetEventDestinationWithContext(ctx, input)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create SES event destination: %v", err))
		return fmt.Errorf("failed to create SES event destination: %w", err)
	}

	return nil
}

// UpdateEventDestination updates an event destination
func (s *SESService) UpdateEventDestination(ctx context.Context, config domain.AmazonSESSettings, destination domain.SESConfigurationSetEventDestination) error {
	sesClient, _, err := s.getClients(config)
	if err != nil {
		return err
	}

	// Update event destination
	input := &ses.UpdateConfigurationSetEventDestinationInput{
		ConfigurationSetName: aws.String(destination.ConfigurationSetName),
		EventDestination: &ses.EventDestination{
			Name:               aws.String(destination.Name),
			Enabled:            aws.Bool(destination.Enabled),
			MatchingEventTypes: aws.StringSlice(destination.MatchingEventTypes),
			SNSDestination: &ses.SNSDestination{
				TopicARN: aws.String(destination.SNSDestination.TopicARN),
			},
		},
	}

	_, err = sesClient.UpdateConfigurationSetEventDestinationWithContext(ctx, input)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to update SES event destination: %v", err))
		return fmt.Errorf("failed to update SES event destination: %w", err)
	}

	return nil
}

// DeleteEventDestination deletes an event destination
func (s *SESService) DeleteEventDestination(ctx context.Context, config domain.AmazonSESSettings, configSetName, destinationName string) error {
	sesClient, _, err := s.getClients(config)
	if err != nil {
		return err
	}

	// Delete event destination
	input := &ses.DeleteConfigurationSetEventDestinationInput{
		ConfigurationSetName: aws.String(configSetName),
		EventDestinationName: aws.String(destinationName),
	}

	_, err = sesClient.DeleteConfigurationSetEventDestinationWithContext(ctx, input)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to delete SES event destination: %v", err))
		return fmt.Errorf("failed to delete SES event destination: %w", err)
	}

	return nil
}

// ListEventDestinations lists all event destinations for a configuration set
func (s *SESService) ListEventDestinations(ctx context.Context, config domain.AmazonSESSettings, configSetName string) ([]domain.SESConfigurationSetEventDestination, error) {
	sesClient, _, err := s.getClients(config)
	if err != nil {
		return nil, err
	}

	// List event destinations
	input := &ses.DescribeConfigurationSetInput{
		ConfigurationSetName: aws.String(configSetName),
		ConfigurationSetAttributeNames: []*string{
			aws.String("eventDestinations"),
		},
	}

	result, err := sesClient.DescribeConfigurationSetWithContext(ctx, input)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to list SES event destinations: %v", err))
		return nil, fmt.Errorf("failed to list SES event destinations: %w", err)
	}

	// Convert AWS response to domain model
	var destinations []domain.SESConfigurationSetEventDestination
	for _, dest := range result.EventDestinations {
		// Skip if not an SNS destination
		if dest.SNSDestination == nil {
			continue
		}

		destination := domain.SESConfigurationSetEventDestination{
			Name:                 *dest.Name,
			ConfigurationSetName: configSetName,
			Enabled:              *dest.Enabled,
			MatchingEventTypes:   aws.StringValueSlice(dest.MatchingEventTypes),
			SNSDestination: &domain.SESTopicConfig{
				TopicARN: *dest.SNSDestination.TopicARN,
			},
		}

		destinations = append(destinations, destination)
	}

	return destinations, nil
}

// setupSNSTopic creates an SNS topic for webhook notifications
func (s *SESService) setupSNSTopic(ctx context.Context, config domain.AmazonSESSettings, topicConfig domain.SESTopicConfig) (string, error) {
	return s.CreateSNSTopic(ctx, config, topicConfig)
}

// setupConfigurationSet creates or verifies a configuration set
func (s *SESService) setupConfigurationSet(ctx context.Context, config domain.AmazonSESSettings, configSetName string) error {
	// List configuration sets to check if it already exists
	configSets, err := s.ListConfigurationSets(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to list configuration sets: %w", err)
	}

	configSetExists := false
	for _, set := range configSets {
		if set == configSetName {
			configSetExists = true
			break
		}
	}

	if !configSetExists {
		err = s.CreateConfigurationSet(ctx, config, configSetName)
		if err != nil {
			return fmt.Errorf("failed to create configuration set: %w", err)
		}
	}

	return nil
}

// setupEventDestination creates or updates an event destination
func (s *SESService) setupEventDestination(ctx context.Context, config domain.AmazonSESSettings, eventDestination domain.SESConfigurationSetEventDestination) error {
	// Check if we need to create or update the event destination
	destinations, err := s.ListEventDestinations(ctx, config, eventDestination.ConfigurationSetName)
	if err != nil {
		return fmt.Errorf("failed to list event destinations: %w", err)
	}

	destinationExists := false
	for _, dest := range destinations {
		if dest.Name == eventDestination.Name {
			destinationExists = true
			err = s.UpdateEventDestination(ctx, config, eventDestination)
			if err != nil {
				return fmt.Errorf("failed to update event destination: %w", err)
			}
			break
		}
	}

	if !destinationExists {
		err = s.CreateEventDestination(ctx, config, eventDestination)
		if err != nil {
			return fmt.Errorf("failed to create event destination: %w", err)
		}
	}

	return nil
}

// RegisterWebhooks implements the domain.WebhookProvider interface for SES
func (s *SESService) RegisterWebhooks(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	baseURL string,
	eventTypes []domain.EmailEventType,
	providerConfig *domain.EmailProvider,
) (*domain.WebhookRegistrationStatus, error) {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.SES == nil ||
		providerConfig.SES.AccessKey == "" || providerConfig.SES.SecretKey == "" {
		return nil, ErrInvalidSESConfig
	}

	// Create webhook URL that includes workspace_id and integration_id
	webhookURL := domain.GenerateWebhookCallbackURL(baseURL, domain.EmailProviderKindSES, workspaceID, integrationID)

	// Map our event types to SES event types
	var sesEventTypes []string

	for _, eventType := range eventTypes {
		switch eventType {
		case domain.EmailEventDelivered:
			sesEventTypes = append(sesEventTypes, "delivery")
		case domain.EmailEventBounce:
			sesEventTypes = append(sesEventTypes, "bounce")
		case domain.EmailEventComplaint:
			sesEventTypes = append(sesEventTypes, "complaint")
		}
	}

	// Create configuration set name
	configSetName := fmt.Sprintf("notifuse-%s", integrationID)

	// First, create the SNS topic that will receive the events
	topicConfig := domain.SESTopicConfig{
		TopicName:            fmt.Sprintf("notifuse-ses-%s", integrationID),
		Protocol:             "https",
		NotificationEndpoint: webhookURL,
	}

	topicARN, err := s.setupSNSTopic(ctx, *providerConfig.SES, topicConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create SNS topic: %w", err)
	}

	// Create or verify configuration set
	err = s.setupConfigurationSet(ctx, *providerConfig.SES, configSetName)
	if err != nil {
		return nil, err
	}

	// Create event destination in the configuration set
	eventDestination := domain.SESConfigurationSetEventDestination{
		ConfigurationSetName: configSetName,
		Name:                 fmt.Sprintf("notifuse-destination-%s", integrationID),
		Enabled:              true,
		MatchingEventTypes:   sesEventTypes,
		SNSDestination: &domain.SESTopicConfig{
			TopicARN: topicARN,
		},
	}

	// Setup event destination
	err = s.setupEventDestination(ctx, *providerConfig.SES, eventDestination)
	if err != nil {
		return nil, err
	}

	// Now create the webhook status structure
	status := &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindSES,
		IsRegistered:      true,
		Endpoints:         []domain.WebhookEndpointStatus{},
		ProviderDetails: map[string]interface{}{
			"configuration_set": configSetName,
			"integration_id":    integrationID,
			"workspace_id":      workspaceID,
			"aws_region":        providerConfig.SES.Region,
			"delivery_topic":    topicARN,
			"bounce_topic":      topicARN,
			"complaint_topic":   topicARN,
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

// GetWebhookStatus implements the domain.WebhookProvider interface for SES
func (s *SESService) GetWebhookStatus(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	providerConfig *domain.EmailProvider,
) (*domain.WebhookRegistrationStatus, error) {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.SES == nil ||
		providerConfig.SES.AccessKey == "" || providerConfig.SES.SecretKey == "" {
		return nil, ErrInvalidSESConfig
	}

	// Create webhook status response
	status := &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindSES,
		IsRegistered:      false,
		Endpoints:         []domain.WebhookEndpointStatus{},
		ProviderDetails: map[string]interface{}{
			"integration_id": integrationID,
			"workspace_id":   workspaceID,
		},
	}

	// Check if the configuration set exists
	configSetName := fmt.Sprintf("notifuse-%s", integrationID)
	configSets, err := s.ListConfigurationSets(ctx, *providerConfig.SES)
	if err != nil {
		return nil, fmt.Errorf("failed to list configuration sets: %w", err)
	}

	configSetExists := false
	for _, set := range configSets {
		if set == configSetName {
			configSetExists = true
			break
		}
	}

	if !configSetExists {
		return status, nil
	}

	// Get event destinations
	destinations, err := s.ListEventDestinations(ctx, *providerConfig.SES, configSetName)
	if err != nil {
		return nil, fmt.Errorf("failed to list event destinations: %w", err)
	}

	// Now check which events are enabled
	var activeEndpoints []domain.WebhookEndpointStatus

	// Get the webhook URL from the provider details
	webhookURL := fmt.Sprintf("sns://%s", destinations[0].Name)

	// Get list of enabled event types from the configuration
	for _, eventType := range []domain.EmailEventType{domain.EmailEventDelivered, domain.EmailEventBounce, domain.EmailEventComplaint} {
		// Check if this event type is enabled
		isEnabled := false
		switch eventType {
		case domain.EmailEventDelivered:
			isEnabled = true
		case domain.EmailEventBounce:
			isEnabled = true
		case domain.EmailEventComplaint:
			isEnabled = true
		}

		if isEnabled {
			activeEndpoints = append(activeEndpoints, domain.WebhookEndpointStatus{
				URL:       webhookURL,
				EventType: eventType,
				Active:    true,
			})
		}
	}

	// Create the webhook status
	status = &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindSES,
		IsRegistered:      true,
		Endpoints:         activeEndpoints,
		ProviderDetails: map[string]interface{}{
			"configuration_set": configSetName,
			"integration_id":    integrationID,
			"workspace_id":      workspaceID,
		},
	}

	return status, nil
}

// UnregisterWebhooks implements the domain.WebhookProvider interface for SES
func (s *SESService) UnregisterWebhooks(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	providerConfig *domain.EmailProvider,
) error {
	// Validate the provider configuration
	if providerConfig == nil || providerConfig.SES == nil ||
		providerConfig.SES.AccessKey == "" || providerConfig.SES.SecretKey == "" {
		return ErrInvalidSESConfig
	}

	// Configuration set and destination naming pattern
	configSetName := fmt.Sprintf("notifuse-%s", integrationID)
	destinationPattern := fmt.Sprintf("notifuse-destination-%s", integrationID)

	// Check if the configuration set exists
	configSets, err := s.ListConfigurationSets(ctx, *providerConfig.SES)
	if err != nil {
		return fmt.Errorf("failed to list configuration sets: %w", err)
	}

	configSetExists := false
	for _, set := range configSets {
		if set == configSetName {
			configSetExists = true
			break
		}
	}

	if !configSetExists {
		// Nothing to clean up
		return nil
	}

	// Get event destinations
	destinations, err := s.ListEventDestinations(ctx, *providerConfig.SES, configSetName)
	if err != nil {
		return fmt.Errorf("failed to list event destinations: %w", err)
	}

	// Delete event destinations and collect topic ARNs
	var topicARNs []string
	for _, dest := range destinations {
		if strings.Contains(dest.Name, destinationPattern) {
			if dest.SNSDestination != nil {
				topicARNs = append(topicARNs, dest.SNSDestination.TopicARN)
			}

			err = s.DeleteEventDestination(ctx, *providerConfig.SES, configSetName, dest.Name)
			if err != nil {
				s.logger.WithField("destination_name", dest.Name).
					Error(fmt.Sprintf("Failed to delete SES event destination: %v", err))
				// Continue with other resources even if one fails
			}
		}
	}

	// Clean up the configuration set
	err = s.DeleteConfigurationSet(ctx, *providerConfig.SES, configSetName)
	if err != nil {
		s.logger.WithField("config_set_name", configSetName).
			Error(fmt.Sprintf("Failed to delete SES configuration set: %v", err))
		// Continue with SNS topics even if this fails
	}

	// Clean up SNS topics
	var lastError error
	for _, topicARN := range topicARNs {
		err = s.DeleteSNSTopic(ctx, *providerConfig.SES, topicARN)
		if err != nil {
			s.logger.WithField("topic_arn", topicARN).
				Error(fmt.Sprintf("Failed to delete SNS topic: %v", err))
			lastError = err
			// Continue with other topics even if one fails
		}
	}

	if lastError != nil {
		return fmt.Errorf("failed to delete one or more AWS resources: %w", lastError)
	}

	return nil
}

// SendEmail sends an email using AWS SES
func (s *SESService) SendEmail(ctx context.Context, request domain.SendEmailProviderRequest) error {
	// Validate the request
	if err := request.Validate(); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	if request.Provider.SES == nil {
		return fmt.Errorf("SES provider is not configured")
	}

	// Make sure we have credentials
	if request.Provider.SES.AccessKey == "" || request.Provider.SES.SecretKey == "" {
		return ErrInvalidAWSCredentials
	}

	// Get SES email client using the factory method for testability
	sess, err := s.sessionFactory(*request.Provider.SES)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to create AWS session: %v", err))
		return fmt.Errorf("failed to create AWS session: %w", err)
	}

	sesEmailClient := s.sesEmailClientFactory(sess)

	// Format the "From" header with name and email (RFC 2047 encoded for non-ASCII)
	fromHeader, err := formatFromHeader(request.FromName, request.FromAddress)
	if err != nil {
		return fmt.Errorf("failed to encode from header: %w", err)
	}

	// Encode the To address (Punycode for international domains)
	encodedTo, err := encodeEmailAddress(request.To)
	if err != nil {
		return fmt.Errorf("failed to encode recipient: %w", err)
	}

	// Create the destination with required addresses
	destination := &ses.Destination{
		ToAddresses: []*string{aws.String(encodedTo)},
	}

	// Add CC addresses if provided (encode for international domains)
	if len(request.EmailOptions.CC) > 0 {
		var ccAddresses []*string
		for _, ccAddress := range request.EmailOptions.CC {
			if ccAddress != "" {
				encodedCC, err := encodeEmailAddress(ccAddress)
				if err != nil {
					return fmt.Errorf("failed to encode CC recipient: %w", err)
				}
				ccAddresses = append(ccAddresses, aws.String(encodedCC))
			}
		}
		if len(ccAddresses) > 0 {
			destination.CcAddresses = ccAddresses
		}
	}

	// Add BCC addresses if provided (encode for international domains)
	if len(request.EmailOptions.BCC) > 0 {
		var bccAddresses []*string
		for _, bccAddress := range request.EmailOptions.BCC {
			if bccAddress != "" {
				encodedBCC, err := encodeEmailAddress(bccAddress)
				if err != nil {
					return fmt.Errorf("failed to encode BCC recipient: %w", err)
				}
				bccAddresses = append(bccAddresses, aws.String(encodedBCC))
			}
		}
		if len(bccAddresses) > 0 {
			destination.BccAddresses = bccAddresses
		}
	}

	// Create the email input
	input := &ses.SendEmailInput{
		Destination: destination,
		Message: &ses.Message{
			Body: &ses.Body{
				Html: &ses.Content{
					Charset: aws.String("UTF-8"),
					Data:    aws.String(request.Content),
				},
			},
			Subject: &ses.Content{
				Charset: aws.String("UTF-8"),
				Data:    aws.String(request.Subject),
			},
		},
		Source: aws.String(fromHeader),
	}

	// Add ReplyTo if provided (encode for international domains)
	if request.EmailOptions.ReplyTo != "" {
		encodedReplyTo, err := encodeEmailAddress(request.EmailOptions.ReplyTo)
		if err != nil {
			return fmt.Errorf("failed to encode reply-to address: %w", err)
		}
		input.ReplyToAddresses = []*string{aws.String(encodedReplyTo)}
	}

	// Add configuration set if it exists - use integrationID instead of workspaceID
	configSetName := fmt.Sprintf("notifuse-%s", request.IntegrationID)
	configSets, err := s.ListConfigurationSets(ctx, *request.Provider.SES)

	if err == nil {
		for _, set := range configSets {
			if set == configSetName {
				input.ConfigurationSetName = aws.String(configSetName)
				break
			}
		}
	}

	// Use SendRawEmail when attachments or List-Unsubscribe headers are needed
	// (AWS SES V1 SendEmail API doesn't support custom headers)
	if len(request.EmailOptions.Attachments) > 0 || request.EmailOptions.ListUnsubscribeURL != "" {
		// Only pass configSetName if it was verified to exist (graceful degradation)
		configSetToUse := ""
		if input.ConfigurationSetName != nil {
			configSetToUse = *input.ConfigurationSetName
		}
		return s.sendRawEmail(ctx, sesEmailClient, request, configSetToUse)
	}

	// Add custom messageID as a tag
	if request.MessageID != "" {
		input.Tags = []*ses.MessageTag{
			{
				Name:  aws.String("notifuse_message_id"),
				Value: aws.String(request.MessageID),
			},
		}
	}

	// Send the email
	_, err = sesEmailClient.SendEmailWithContext(ctx, input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return fmt.Errorf("SES error: %s", aerr.Error())
		}
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

// sendRawEmail sends email using SendRawEmail for attachments or custom headers
// Following AWS SES raw MIME message construction as documented at:
// https://docs.aws.amazon.com/ses/latest/dg/attachments.html
func (s *SESService) sendRawEmail(ctx context.Context, sesClient domain.SESClient, request domain.SendEmailProviderRequest, configSetName string) error {
	var buf bytes.Buffer

	// Encode From header (RFC 2047 for non-ASCII names, Punycode for domains)
	fromHeader, err := formatFromHeader(request.FromName, request.FromAddress)
	if err != nil {
		return fmt.Errorf("failed to encode from header: %w", err)
	}
	buf.WriteString(fmt.Sprintf("From: %s\r\n", fromHeader))

	// Encode To address
	encodedTo, err := encodeEmailAddress(request.To)
	if err != nil {
		return fmt.Errorf("failed to encode recipient: %w", err)
	}
	buf.WriteString(fmt.Sprintf("To: %s\r\n", encodedTo))

	// Encode CC addresses
	if len(request.EmailOptions.CC) > 0 {
		var encodedCCs []string
		for _, cc := range request.EmailOptions.CC {
			if cc != "" {
				encodedCC, err := encodeEmailAddress(cc)
				if err != nil {
					return fmt.Errorf("failed to encode CC recipient: %w", err)
				}
				encodedCCs = append(encodedCCs, encodedCC)
			}
		}
		if len(encodedCCs) > 0 {
			buf.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(encodedCCs, ", ")))
		}
	}

	// Encode Reply-To address
	if request.EmailOptions.ReplyTo != "" {
		encodedReplyTo, err := encodeEmailAddress(request.EmailOptions.ReplyTo)
		if err != nil {
			return fmt.Errorf("failed to encode reply-to address: %w", err)
		}
		buf.WriteString(fmt.Sprintf("Reply-To: %s\r\n", encodedReplyTo))
	}

	// Encode Subject (RFC 2047 for non-ASCII)
	encodedSubject := encodeRFC2047(request.Subject)
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", encodedSubject))
	buf.WriteString(fmt.Sprintf("X-Message-ID: %s\r\n", request.MessageID))

	// Add RFC-8058 List-Unsubscribe headers for one-click unsubscribe
	if request.EmailOptions.ListUnsubscribeURL != "" {
		buf.WriteString(fmt.Sprintf("List-Unsubscribe: <%s>\r\n", request.EmailOptions.ListUnsubscribeURL))
		buf.WriteString("List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n")
	}

	buf.WriteString("MIME-Version: 1.0\r\n")

	// Create multipart writer
	writer := multipart.NewWriter(&buf)
	boundary := writer.Boundary()
	buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", boundary))

	// Add HTML body part
	htmlPart := textproto.MIMEHeader{}
	htmlPart.Set("Content-Type", "text/html; charset=UTF-8")
	htmlPart.Set("Content-Transfer-Encoding", "quoted-printable")

	htmlWriter, err := writer.CreatePart(htmlPart)
	if err != nil {
		return fmt.Errorf("failed to create HTML part: %w", err)
	}

	// Wrap with quoted-printable encoder for RFC 2045 compliance (Issue #230)
	qpWriter := quotedprintable.NewWriter(htmlWriter)
	if _, err := qpWriter.Write([]byte(request.Content)); err != nil {
		qpWriter.Close()
		return fmt.Errorf("failed to write HTML content: %w", err)
	}
	if err := qpWriter.Close(); err != nil {
		return fmt.Errorf("failed to close quoted-printable writer: %w", err)
	}

	// Add attachments
	for i, att := range request.EmailOptions.Attachments {
		content, err := att.DecodeContent()
		if err != nil {
			return fmt.Errorf("attachment %d: failed to decode content: %w", i, err)
		}

		attachPart := textproto.MIMEHeader{}

		// Set content type (auto-detect if not provided, as per SES best practices)
		contentType := att.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		attachPart.Set("Content-Type", contentType)

		// Use base64 encoding for binary content (SES standard)
		attachPart.Set("Content-Transfer-Encoding", "base64")

		// Set disposition (attachment or inline)
		disposition := att.Disposition
		if disposition == "" {
			disposition = "attachment"
		}

		// For inline attachments, we need to set Content-ID for referencing in HTML
		if disposition == "inline" {
			// Generate a simple Content-ID from filename (AWS SES approach)
			contentID := strings.ReplaceAll(att.Filename, " ", "_")
			attachPart.Set("Content-ID", fmt.Sprintf("<%s>", contentID))
			attachPart.Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", att.Filename))
		} else {
			attachPart.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", att.Filename))
		}

		attachWriter, err := writer.CreatePart(attachPart)
		if err != nil {
			return fmt.Errorf("attachment %d: failed to create part: %w", i, err)
		}

		// Write base64 encoded content with proper line wrapping (RFC 2045: max 76 chars per line)
		encoded := base64.StdEncoding.EncodeToString(content)
		// Split into 76-character lines for RFC compliance
		for len(encoded) > 76 {
			if _, err := attachWriter.Write([]byte(encoded[:76] + "\r\n")); err != nil {
				return fmt.Errorf("attachment %d: failed to write content: %w", i, err)
			}
			encoded = encoded[76:]
		}
		// Write remaining content
		if len(encoded) > 0 {
			if _, err := attachWriter.Write([]byte(encoded + "\r\n")); err != nil {
				return fmt.Errorf("attachment %d: failed to write content: %w", i, err)
			}
		}
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create raw email input
	rawInput := &ses.SendRawEmailInput{
		RawMessage: &ses.RawMessage{
			Data: buf.Bytes(),
		},
	}

	// Add configuration set if available
	if configSetName != "" {
		rawInput.ConfigurationSetName = aws.String(configSetName)
	}

	// Add BCC addresses if provided (not in raw message headers for privacy)
	if len(request.EmailOptions.BCC) > 0 {
		var destinations []*string
		destinations = append(destinations, aws.String(encodedTo))
		for _, bcc := range request.EmailOptions.BCC {
			if bcc != "" {
				encodedBCC, err := encodeEmailAddress(bcc)
				if err != nil {
					return fmt.Errorf("failed to encode BCC recipient: %w", err)
				}
				destinations = append(destinations, aws.String(encodedBCC))
			}
		}
		rawInput.Destinations = destinations
	}

	// Send the raw email
	_, err = sesClient.SendRawEmailWithContext(ctx, rawInput)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return fmt.Errorf("SES error: %s", aerr.Error())
		}
		return fmt.Errorf("failed to send raw email: %w", err)
	}

	return nil
}
