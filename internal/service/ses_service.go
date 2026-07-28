package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"net/url"
	"strings"
	"unicode"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/aws/aws-sdk-go/service/sns"
	"golang.org/x/net/idna"
)

// Custom domain errors for better testability
var (
	ErrInvalidAWSCredentials = fmt.Errorf("invalid AWS credentials")
	ErrInvalidSNSDestination = fmt.Errorf("SNS destination and Topic ARN are required")
	ErrInvalidSESConfig      = fmt.Errorf("SES configuration is missing or invalid")
)

// sesReceivingRegions is the set of AWS regions where SES supports inbound email receiving
// (receipt rules), taken from the AWS General Reference "Email Receiving endpoints" table.
// SES exposes no API to query this, so it is a static allowlist — revisit when AWS adds
// regions. Notably excludes GovCloud, ap-south-2, ap-southeast-5, ca-west-1, eu-central-2,
// and me-central-1, which support sending but NOT receiving.
var sesReceivingRegions = map[string]bool{
	"us-east-1": true, "us-east-2": true, "us-west-1": true, "us-west-2": true,
	"af-south-1": true, "ap-southeast-3": true, "ap-south-1": true, "ap-northeast-3": true,
	"ap-northeast-2": true, "ap-southeast-1": true, "ap-southeast-2": true, "ap-northeast-1": true,
	"ca-central-1": true, "eu-central-1": true, "eu-west-1": true, "eu-west-2": true,
	"eu-south-1": true, "eu-west-3": true, "eu-north-1": true, "il-central-1": true,
	"me-south-1": true, "sa-east-1": true,
}

// sesInboundRuleSetName is the dedicated receipt rule set Notifuse creates and activates ONLY
// when the account has no active rule set. When one is already active, our rule is inserted
// into it instead, so an existing WorkMail / customer setup is never silently deactivated.
const sesInboundRuleSetName = "notifuse-inbound"

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

	// Whether the stop-on-reply inbound receipt rule is registered (independent of the
	// bounce/complaint event webhook below).
	inboundRegistered := s.isInboundRegistered(ctx, *providerConfig.SES, integrationID)

	// Create webhook status response
	status := &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindSES,
		IsRegistered:      false,
		Endpoints:         []domain.WebhookEndpointStatus{},
		ProviderDetails: map[string]interface{}{
			"integration_id":     integrationID,
			"workspace_id":       workspaceID,
			"inbound_registered": inboundRegistered,
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
			"configuration_set":  configSetName,
			"integration_id":     integrationID,
			"workspace_id":       workspaceID,
			"inbound_registered": inboundRegistered,
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

	// Best-effort: remove the stop-on-reply inbound receipt rule (independent of the
	// bounce/complaint event destinations cleaned up below).
	s.unregisterInboundRoute(ctx, *providerConfig.SES, integrationID)

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

// EnsureInboundRoute implements domain.InboundRouteRegistrar for SES: it idempotently
// provisions the inbound path for stop-on-reply — an SNS topic (HTTPS-subscribed to
// inboundURL, SignatureVersion 2, with a policy letting SES publish) plus a receipt rule
// with an SNS action. The receipt rule is added INTO the active rule set when one exists
// (never deactivating a customer's setup); a dedicated notifuse set is created + activated
// only when no rule set is active. DNS MX records pointing the domain at
// inbound-smtp.<region>.amazonaws.com remain the operator's responsibility (no SES API).
func (s *SESService) EnsureInboundRoute(ctx context.Context, providerConfig *domain.EmailProvider, inboundURL string) error {
	if providerConfig == nil || providerConfig.SES == nil ||
		providerConfig.SES.AccessKey == "" || providerConfig.SES.SecretKey == "" {
		return ErrInvalidSESConfig
	}
	config := *providerConfig.SES

	// SES email receiving exists only in a subset of regions. This is a permanent, expected
	// condition (e.g. a sending-only region), NOT a provisioning failure — soft-skip so the
	// delivery/bounce/complaint event-webhook registration still succeeds. The console shows
	// inbound as unregistered, and stop-on-reply just isn't available in this region.
	if !sesReceivingRegions[config.Region] {
		s.logger.WithField("region", config.Region).
			Warn("SES email receiving is not supported in this region; skipping inbound reply route")
		return nil
	}

	_, integrationID := inboundIDsFromURL(inboundURL)
	if integrationID == "" {
		return fmt.Errorf("inbound URL is missing integration_id: %q", inboundURL)
	}
	ruleName := "notifuse-inbound-" + integrationID
	topicName := "notifuse-ses-inbound-" + integrationID

	sesClient, snsClient, err := s.getClients(config)
	if err != nil {
		return err
	}

	// 1. Inbound SNS topic (kept separate from the bounce/complaint event topic).
	createTopic, err := snsClient.CreateTopicWithContext(ctx, &sns.CreateTopicInput{Name: aws.String(topicName)})
	if err != nil {
		return fmt.Errorf("failed to create inbound SNS topic: %w", err)
	}
	topicARN := aws.StringValue(createTopic.TopicArn)
	// Record the provisioned ARN on the settings so the caller can persist it; the inbound
	// parser binds to this exact ARN to authenticate that a message came from OUR topic.
	providerConfig.SES.InboundTopicARN = topicARN

	// Grant SES permission to publish to the topic — without this, the receipt rule's SNS
	// action fails validation. Scoped to this account via AWS:SourceAccount. Fail closed if the
	// account can't be parsed from the ARN: an unconditioned publish grant would let any SES
	// account publish to our topic.
	accountID := accountIDFromARN(topicARN)
	if accountID == "" {
		return fmt.Errorf("could not parse AWS account from topic ARN %q", topicARN)
	}
	policy, err := sesPublishTopicPolicy(topicARN, accountID)
	if err != nil {
		return err
	}
	if _, err := snsClient.SetTopicAttributesWithContext(ctx, &sns.SetTopicAttributesInput{
		TopicArn:       aws.String(topicARN),
		AttributeName:  aws.String("Policy"),
		AttributeValue: aws.String(policy),
	}); err != nil {
		return fmt.Errorf("failed to set inbound SNS topic policy: %w", err)
	}
	// Force SignatureVersion 2 (SHA-256) so notifications are signed with the stronger algorithm.
	if _, err := snsClient.SetTopicAttributesWithContext(ctx, &sns.SetTopicAttributesInput{
		TopicArn:       aws.String(topicARN),
		AttributeName:  aws.String("SignatureVersion"),
		AttributeValue: aws.String("2"),
	}); err != nil {
		return fmt.Errorf("failed to set inbound SNS topic signature version: %w", err)
	}
	// Subscribe the inbound endpoint. SNS dedups by protocol+endpoint, so this is idempotent;
	// the parser confirms the subscription (signature-verified) on the first delivery.
	if _, err := snsClient.SubscribeWithContext(ctx, &sns.SubscribeInput{
		Protocol: aws.String("https"),
		TopicArn: aws.String(topicARN),
		Endpoint: aws.String(inboundURL),
	}); err != nil {
		return fmt.Errorf("failed to subscribe inbound endpoint: %w", err)
	}

	// 2. Receipt rule. Check idempotency FIRST (cheap, no identity listing), then build the
	// rule only when we actually need to create it.
	active, err := sesClient.DescribeActiveReceiptRuleSetWithContext(ctx, &ses.DescribeActiveReceiptRuleSetInput{})
	if err != nil {
		return fmt.Errorf("failed to describe active receipt rule set: %w", err)
	}

	if active != nil && active.Metadata != nil && active.Metadata.Name != nil {
		ruleSetName := aws.StringValue(active.Metadata.Name)
		// Idempotent: our rule is already present in the active set — nothing to do.
		for _, r := range active.Rules {
			if r != nil && aws.StringValue(r.Name) == ruleName {
				return nil
			}
		}
		rule, err := s.buildInboundReceiptRule(ctx, sesClient, ruleName, topicARN)
		if err != nil {
			return err
		}
		if _, err := sesClient.CreateReceiptRuleWithContext(ctx, &ses.CreateReceiptRuleInput{
			RuleSetName: aws.String(ruleSetName),
			Rule:        rule,
		}); err != nil && !isAWSErrCode(err, ses.ErrCodeAlreadyExistsException) {
			return fmt.Errorf("failed to create inbound receipt rule: %w", err)
		}
		return nil
	}

	// No active rule set: create a dedicated notifuse set, add our rule, and activate it.
	// Safe here precisely because nothing else is active to deactivate.
	rule, err := s.buildInboundReceiptRule(ctx, sesClient, ruleName, topicARN)
	if err != nil {
		return err
	}
	if _, err := sesClient.CreateReceiptRuleSetWithContext(ctx, &ses.CreateReceiptRuleSetInput{
		RuleSetName: aws.String(sesInboundRuleSetName),
	}); err != nil && !isAWSErrCode(err, ses.ErrCodeAlreadyExistsException) {
		return fmt.Errorf("failed to create inbound receipt rule set: %w", err)
	}
	if _, err := sesClient.CreateReceiptRuleWithContext(ctx, &ses.CreateReceiptRuleInput{
		RuleSetName: aws.String(sesInboundRuleSetName),
		Rule:        rule,
	}); err != nil && !isAWSErrCode(err, ses.ErrCodeAlreadyExistsException) {
		return fmt.Errorf("failed to create inbound receipt rule: %w", err)
	}
	if _, err := sesClient.SetActiveReceiptRuleSetWithContext(ctx, &ses.SetActiveReceiptRuleSetInput{
		RuleSetName: aws.String(sesInboundRuleSetName),
	}); err != nil {
		return fmt.Errorf("failed to activate inbound receipt rule set: %w", err)
	}
	return nil
}

// buildInboundReceiptRule constructs the SES receipt rule that forwards inbound replies to our
// SNS topic, scoped to this account's verified identities. FAILS CLOSED on empty/error: an
// empty Recipients set means "match ALL recipients", which — inserted into the customer's
// active rule set — would forward a copy of every inbound email in the account (incl. unrelated
// WorkMail traffic) to our topic. The action has no Stop, so it coexists with other rules.
func (s *SESService) buildInboundReceiptRule(ctx context.Context, sesClient domain.SESWebhookClient, ruleName, topicARN string) (*ses.ReceiptRule, error) {
	recipients, err := s.inboundRecipients(ctx, sesClient)
	if err != nil {
		return nil, fmt.Errorf("failed to determine inbound recipients: %w", err)
	}
	if len(recipients) == 0 {
		return nil, fmt.Errorf("no verified SES identities found; refusing to provision an account-wide inbound receipt rule")
	}
	return &ses.ReceiptRule{
		Name:        aws.String(ruleName),
		Enabled:     aws.Bool(true),
		Recipients:  aws.StringSlice(recipients),
		ScanEnabled: aws.Bool(false),
		TlsPolicy:   aws.String(ses.TlsPolicyOptional),
		Actions: []*ses.ReceiptAction{{
			SNSAction: &ses.SNSAction{
				TopicArn: aws.String(topicARN),
				Encoding: aws.String(ses.SNSActionEncodingBase64),
			},
		}},
	}, nil
}

// inboundRecipients lists the account's verified identities — BOTH domain identities and
// email-address identities — to scope the receipt rule so it forwards only mail addressed to
// our own senders. Email-address-only senders (a verified address whose domain isn't a domain
// identity) would otherwise never match. Returns an error (not nil) on failure so the caller
// can fail closed rather than provision a match-all rule.
func (s *SESService) inboundRecipients(ctx context.Context, sesClient domain.SESWebhookClient) ([]string, error) {
	var recipients []string
	for _, idType := range []string{ses.IdentityTypeDomain, ses.IdentityTypeEmailAddress} {
		out, err := sesClient.ListIdentitiesWithContext(ctx, &ses.ListIdentitiesInput{
			IdentityType: aws.String(idType),
			MaxItems:     aws.Int64(100),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list SES %s identities: %w", idType, err)
		}
		for _, id := range out.Identities {
			if v := aws.StringValue(id); v != "" {
				recipients = append(recipients, v)
			}
		}
	}
	return recipients, nil
}

// inboundIDsFromURL extracts workspace_id and integration_id from a GenerateInboundWebhookURL.
func inboundIDsFromURL(inboundURL string) (workspaceID, integrationID string) {
	u, err := url.Parse(inboundURL)
	if err != nil {
		return "", ""
	}
	q := u.Query()
	return q.Get("workspace_id"), q.Get("integration_id")
}

// accountIDFromARN returns the account-id segment of an ARN
// (arn:partition:service:region:ACCOUNT:resource), or "" if it can't be parsed.
func accountIDFromARN(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) < 5 {
		return ""
	}
	return parts[4]
}

// sesPublishTopicPolicy builds an SNS access policy granting the SES service permission to
// publish to the topic, restricted to this AWS account (AWS:SourceAccount). Required for a
// receipt rule's SNS action to deliver inbound mail.
func sesPublishTopicPolicy(topicARN, accountID string) (string, error) {
	statement := map[string]interface{}{
		"Sid":       "AllowSESPublish",
		"Effect":    "Allow",
		"Principal": map[string]string{"Service": "ses.amazonaws.com"},
		"Action":    "SNS:Publish",
		"Resource":  topicARN,
	}
	if accountID != "" {
		statement["Condition"] = map[string]interface{}{
			"StringEquals": map[string]string{"AWS:SourceAccount": accountID},
		}
	}
	policy := map[string]interface{}{
		"Version":   "2012-10-17",
		"Statement": []interface{}{statement},
	}
	b, err := json.Marshal(policy)
	if err != nil {
		return "", fmt.Errorf("failed to marshal SNS topic policy: %w", err)
	}
	return string(b), nil
}

// isAWSErrCode reports whether err is an awserr.Error with the given code.
func isAWSErrCode(err error, code string) bool {
	var aerr awserr.Error
	if errors.As(err, &aerr) {
		return aerr.Code() == code
	}
	return false
}

// unregisterInboundRoute best-effort removes the stop-on-reply receipt rule for an
// integration from the active rule set. It deletes ONLY our own rule — never the rule set
// (other rules may depend on it) and never the SNS topic (left inert for cheap
// re-registration; SES won't publish to it without the rule).
func (s *SESService) unregisterInboundRoute(ctx context.Context, config domain.AmazonSESSettings, integrationID string) {
	if !sesReceivingRegions[config.Region] {
		return
	}
	sesClient, _, err := s.getClients(config)
	if err != nil {
		return
	}
	ruleName := "notifuse-inbound-" + integrationID
	active, err := sesClient.DescribeActiveReceiptRuleSetWithContext(ctx, &ses.DescribeActiveReceiptRuleSetInput{})
	if err != nil || active == nil || active.Metadata == nil {
		return
	}
	for _, r := range active.Rules {
		if r != nil && aws.StringValue(r.Name) == ruleName {
			if _, err := sesClient.DeleteReceiptRuleWithContext(ctx, &ses.DeleteReceiptRuleInput{
				RuleSetName: active.Metadata.Name,
				RuleName:    aws.String(ruleName),
			}); err != nil {
				s.logger.WithField("rule_name", ruleName).
					Error(fmt.Sprintf("Failed to delete SES inbound receipt rule: %v", err))
			}
			return
		}
	}
}

// isInboundRegistered reports whether the stop-on-reply receipt rule for an integration is
// present in the active rule set. Best-effort: returns false on any error or unsupported region.
func (s *SESService) isInboundRegistered(ctx context.Context, config domain.AmazonSESSettings, integrationID string) bool {
	if !sesReceivingRegions[config.Region] {
		return false
	}
	sesClient, _, err := s.getClients(config)
	if err != nil {
		return false
	}
	ruleName := "notifuse-inbound-" + integrationID
	active, err := sesClient.DescribeActiveReceiptRuleSetWithContext(ctx, &ses.DescribeActiveReceiptRuleSetInput{})
	if err != nil || active == nil || active.Metadata == nil {
		return false
	}
	for _, r := range active.Rules {
		if r != nil && aws.StringValue(r.Name) == ruleName {
			return true
		}
	}
	return false
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
	out, err := sesEmailClient.SendEmailWithContext(ctx, input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return fmt.Errorf("SES error: %s", aerr.Error())
		}
		return fmt.Errorf("failed to send email: %w", err)
	}
	// SES overwrites the Message-ID; capture the returned one for stop-on-reply matching.
	captureSESMessageID(request, out.MessageId)

	return nil
}

// captureSESMessageID writes the SES-returned MessageId into request.CapturedMessageID when
// the caller provided that pointer, so the worker can store the recipient-visible RFC
// Message-ID (domain.SESStoredMessageID) for stop-on-reply matching. SES overwrites any
// Message-ID we set, so the returned value is the only way to know what the recipient — and
// their reply's In-Reply-To — will carry. No-op when the field is nil (non-capturing callers).
func captureSESMessageID(request domain.SendEmailProviderRequest, messageID *string) {
	if request.CapturedMessageID != nil && messageID != nil {
		*request.CapturedMessageID = aws.StringValue(messageID)
	}
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

	// writeHTMLPart writes the quoted-printable HTML body into the given writer.
	writeHTMLPart := func(w *multipart.Writer) error {
		htmlPart := textproto.MIMEHeader{}
		htmlPart.Set("Content-Type", "text/html; charset=UTF-8")
		htmlPart.Set("Content-Transfer-Encoding", "quoted-printable")

		htmlWriter, err := w.CreatePart(htmlPart)
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
		return nil
	}

	// writeAttachmentPart writes a single attachment (inline or not) as a base64
	// MIME part into the given writer.
	writeAttachmentPart := func(w *multipart.Writer, att domain.Attachment, inline bool) error {
		content, err := att.DecodeContent()
		if err != nil {
			return fmt.Errorf("failed to decode content: %w", err)
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

		if inline {
			// Content-ID lets the HTML reference the image as cid:<id>. Use the
			// caller-provided content_id when present, else the filename. Spaces
			// are not valid in a Content-ID, so normalize them to underscores.
			contentID := strings.ReplaceAll(att.EffectiveContentID(), " ", "_")
			attachPart.Set("Content-ID", fmt.Sprintf("<%s>", contentID))
			attachPart.Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", att.Filename))
		} else {
			attachPart.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", att.Filename))
		}

		attachWriter, err := w.CreatePart(attachPart)
		if err != nil {
			return fmt.Errorf("failed to create part: %w", err)
		}

		// Write base64 encoded content with proper line wrapping (RFC 2045: max 76 chars per line)
		encoded := base64.StdEncoding.EncodeToString(content)
		for len(encoded) > 76 {
			if _, err := attachWriter.Write([]byte(encoded[:76] + "\r\n")); err != nil {
				return fmt.Errorf("failed to write content: %w", err)
			}
			encoded = encoded[76:]
		}
		if len(encoded) > 0 {
			if _, err := attachWriter.Write([]byte(encoded + "\r\n")); err != nil {
				return fmt.Errorf("failed to write content: %w", err)
			}
		}
		return nil
	}

	// Partition attachments: inline images must live in a multipart/related
	// subtree alongside the HTML body so clients embed them, whereas regular
	// attachments stay at the top-level multipart/mixed. Placing inline images
	// directly under multipart/mixed makes Gmail/Outlook render them as dangling
	// attachments instead of embedding them.
	var inlineAtts, regularAtts []domain.Attachment
	for _, att := range request.EmailOptions.Attachments {
		if att.Disposition == "inline" {
			inlineAtts = append(inlineAtts, att)
		} else {
			regularAtts = append(regularAtts, att)
		}
	}

	// Create top-level multipart/mixed writer
	writer := multipart.NewWriter(&buf)
	boundary := writer.Boundary()
	buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", boundary))

	// HTML body — wrapped in multipart/related when inline images are present.
	if len(inlineAtts) > 0 {
		relatedBoundary := multipart.NewWriter(&bytes.Buffer{}).Boundary()
		relatedHeader := textproto.MIMEHeader{}
		relatedHeader.Set("Content-Type", fmt.Sprintf("multipart/related; type=\"text/html\"; boundary=\"%s\"", relatedBoundary))
		relatedPart, err := writer.CreatePart(relatedHeader)
		if err != nil {
			return fmt.Errorf("failed to create related part: %w", err)
		}
		related := multipart.NewWriter(relatedPart)
		if err := related.SetBoundary(relatedBoundary); err != nil {
			return fmt.Errorf("failed to set related boundary: %w", err)
		}
		if err := writeHTMLPart(related); err != nil {
			return err
		}
		for i, att := range inlineAtts {
			if err := writeAttachmentPart(related, att, true); err != nil {
				return fmt.Errorf("inline attachment %d: %w", i, err)
			}
		}
		if err := related.Close(); err != nil {
			return fmt.Errorf("failed to close related writer: %w", err)
		}
	} else {
		if err := writeHTMLPart(writer); err != nil {
			return err
		}
	}

	// Regular (non-inline) attachments at the top level
	for i, att := range regularAtts {
		if err := writeAttachmentPart(writer, att, false); err != nil {
			return fmt.Errorf("attachment %d: %w", i, err)
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

	// Add custom messageID as a tag (same as SendEmail API)
	if request.MessageID != "" {
		rawInput.Tags = []*ses.MessageTag{
			{
				Name:  aws.String("notifuse_message_id"),
				Value: aws.String(request.MessageID),
			},
		}
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
	out, err := sesClient.SendRawEmailWithContext(ctx, rawInput)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return fmt.Errorf("SES error: %s", aerr.Error())
		}
		return fmt.Errorf("failed to send raw email: %w", err)
	}
	// SES overwrites the Message-ID; capture the returned one for stop-on-reply matching.
	captureSESMessageID(request, out.MessageId)

	return nil
}
