package domain

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/aws/aws-sdk-go/service/sns"
)

//go:generate mockgen -destination mocks/mock_ses_service.go -package mocks github.com/Notifuse/notifuse/internal/domain SESServiceInterface
//go:generate mockgen -destination mocks/mock_ses_client.go -package mocks github.com/Notifuse/notifuse/internal/domain SESClient
//go:generate mockgen -destination mocks/mock_sns_client.go -package mocks github.com/Notifuse/notifuse/internal/domain SNSClient

// SESWebhookClient defines the interface for SES client operations related to webhook management
type SESClient interface {
	ListConfigurationSetsWithContext(ctx context.Context, input *ses.ListConfigurationSetsInput, opts ...request.Option) (*ses.ListConfigurationSetsOutput, error)
	CreateConfigurationSetWithContext(ctx context.Context, input *ses.CreateConfigurationSetInput, opts ...request.Option) (*ses.CreateConfigurationSetOutput, error)
	DeleteConfigurationSetWithContext(ctx context.Context, input *ses.DeleteConfigurationSetInput, opts ...request.Option) (*ses.DeleteConfigurationSetOutput, error)
	DescribeConfigurationSetWithContext(ctx context.Context, input *ses.DescribeConfigurationSetInput, opts ...request.Option) (*ses.DescribeConfigurationSetOutput, error)
	CreateConfigurationSetEventDestinationWithContext(ctx context.Context, input *ses.CreateConfigurationSetEventDestinationInput, opts ...request.Option) (*ses.CreateConfigurationSetEventDestinationOutput, error)
	UpdateConfigurationSetEventDestinationWithContext(ctx context.Context, input *ses.UpdateConfigurationSetEventDestinationInput, opts ...request.Option) (*ses.UpdateConfigurationSetEventDestinationOutput, error)
	DeleteConfigurationSetEventDestinationWithContext(ctx context.Context, input *ses.DeleteConfigurationSetEventDestinationInput, opts ...request.Option) (*ses.DeleteConfigurationSetEventDestinationOutput, error)
	SendEmailWithContext(ctx context.Context, input *ses.SendEmailInput, opts ...request.Option) (*ses.SendEmailOutput, error)
	SendRawEmailWithContext(ctx context.Context, input *ses.SendRawEmailInput, opts ...request.Option) (*ses.SendRawEmailOutput, error)
}

// SNSWebhookClient defines the interface for SNS client operations related to webhook management
type SNSClient interface {
	CreateTopicWithContext(ctx context.Context, input *sns.CreateTopicInput, opts ...request.Option) (*sns.CreateTopicOutput, error)
	DeleteTopicWithContext(ctx context.Context, input *sns.DeleteTopicInput, opts ...request.Option) (*sns.DeleteTopicOutput, error)
	SubscribeWithContext(ctx context.Context, input *sns.SubscribeInput, opts ...request.Option) (*sns.SubscribeOutput, error)
	GetTopicAttributesWithContext(ctx context.Context, input *sns.GetTopicAttributesInput, opts ...request.Option) (*sns.GetTopicAttributesOutput, error)
}

// SESWebhookPayload represents an Amazon SES webhook payload
type SESWebhookPayload struct {
	Type              string                         `json:"Type"`
	MessageID         string                         `json:"MessageId"`
	TopicARN          string                         `json:"TopicArn"`
	Message           string                         `json:"Message"`
	Timestamp         string                         `json:"Timestamp"`
	SignatureVersion  string                         `json:"SignatureVersion"`
	Signature         string                         `json:"Signature"`
	SigningCertURL    string                         `json:"SigningCertURL"`
	UnsubscribeURL    string                         `json:"UnsubscribeURL"`
	SubscribeURL      string                         `json:"SubscribeURL,omitempty"`
	Token             string                         `json:"Token,omitempty"`
	MessageAttributes map[string]SESMessageAttribute `json:"MessageAttributes"`
}

// SESMessageAttribute represents a message attribute in SES webhook
type SESMessageAttribute struct {
	Type  string `json:"Type"`
	Value string `json:"Value"`
}

// SESBounceNotification represents an SES bounce notification
type SESBounceNotification struct {
	EventType string    `json:"eventType"`
	Bounce    SESBounce `json:"bounce"`
	Mail      SESMail   `json:"mail"`
}

// SESComplaintNotification represents an SES complaint notification
type SESComplaintNotification struct {
	EventType string       `json:"eventType"`
	Complaint SESComplaint `json:"complaint"`
	Mail      SESMail      `json:"mail"`
}

// SESDeliveryNotification represents an SES delivery notification
type SESDeliveryNotification struct {
	EventType string      `json:"eventType"`
	Delivery  SESDelivery `json:"delivery"`
	Mail      SESMail     `json:"mail"`
}

// SESMail represents the mail part of an SES notification
type SESMail struct {
	Timestamp        string              `json:"timestamp"`
	MessageID        string              `json:"messageId"`
	Source           string              `json:"source"`
	Destination      []string            `json:"destination"`
	HeadersTruncated bool                `json:"headersTruncated"`
	Headers          []SESHeader         `json:"headers"`
	CommonHeaders    SESCommonHeaders    `json:"commonHeaders"`
	Tags             map[string][]string `json:"tags"`
}

// SESHeader represents a header in an SES notification
type SESHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SESCommonHeaders represents common headers in an SES notification
type SESCommonHeaders struct {
	From      []string `json:"from"`
	To        []string `json:"to"`
	MessageID string   `json:"messageId"`
	Subject   string   `json:"subject"`
}

// SESBounce represents a bounce in an SES notification
type SESBounce struct {
	BounceType        string                `json:"bounceType"`
	BounceSubType     string                `json:"bounceSubType"`
	BouncedRecipients []SESBouncedRecipient `json:"bouncedRecipients"`
	Timestamp         string                `json:"timestamp"`
	FeedbackID        string                `json:"feedbackId"`
	ReportingMTA      string                `json:"reportingMTA"`
}

// SESBouncedRecipient represents a bounced recipient in an SES notification
type SESBouncedRecipient struct {
	EmailAddress   string `json:"emailAddress"`
	Action         string `json:"action"`
	Status         string `json:"status"`
	DiagnosticCode string `json:"diagnosticCode"`
}

// SESComplaint represents a complaint in an SES notification
type SESComplaint struct {
	ComplainedRecipients  []SESComplainedRecipient `json:"complainedRecipients"`
	Timestamp             string                   `json:"timestamp"`
	FeedbackID            string                   `json:"feedbackId"`
	ComplaintFeedbackType string                   `json:"complaintFeedbackType"`
}

// SESComplainedRecipient represents a complained recipient in an SES notification
type SESComplainedRecipient struct {
	EmailAddress string `json:"emailAddress"`
}

// SESDelivery represents a delivery in an SES notification
type SESDelivery struct {
	Timestamp            string   `json:"timestamp"`
	ProcessingTimeMillis int      `json:"processingTimeMillis"`
	Recipients           []string `json:"recipients"`
	SMTPResponse         string   `json:"smtpResponse"`
	RemoteMtaIP          string   `json:"remoteMtaIp"`
	ReportingMTA         string   `json:"reportingMTA"`
}

// SESTopicConfig represents AWS SNS topic configuration
type SESTopicConfig struct {
	TopicARN             string `json:"topic_arn"`
	TopicName            string `json:"topic_name,omitempty"`
	NotificationEndpoint string `json:"notification_endpoint"`
	Protocol             string `json:"protocol"` // Usually "https"
}

// SESConfigurationSetEventDestination represents SES event destination configuration
type SESConfigurationSetEventDestination struct {
	Name                 string          `json:"name"`
	ConfigurationSetName string          `json:"configuration_set_name"`
	Enabled              bool            `json:"enabled"`
	MatchingEventTypes   []string        `json:"matching_event_types"`
	SNSDestination       *SESTopicConfig `json:"sns_destination,omitempty"`
}

// AmazonSESSettings contains SES email provider settings
type AmazonSESSettings struct {
	Region             string `json:"region"`
	AccessKey          string `json:"access_key"`
	EncryptedSecretKey string `json:"encrypted_secret_key,omitempty"`

	// decoded secret key, not stored in the database
	SecretKey string `json:"secret_key,omitempty"`
}

func (a *AmazonSESSettings) DecryptSecretKey(passphrase string) error {
	secretKey, err := crypto.DecryptFromHexString(a.EncryptedSecretKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt SES secret key: %w", err)
	}
	a.SecretKey = secretKey
	return nil
}

func (a *AmazonSESSettings) EncryptSecretKey(passphrase string) error {
	encryptedSecretKey, err := crypto.EncryptString(a.SecretKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt SES secret key: %w", err)
	}
	a.EncryptedSecretKey = encryptedSecretKey
	return nil
}

func (a *AmazonSESSettings) Validate(passphrase string) error {
	// Check if any field is set to determine if we should validate
	isConfigured := a.Region != "" || a.AccessKey != "" ||
		a.EncryptedSecretKey != "" || a.SecretKey != ""

	// If no fields are set, consider it valid (optional config)
	if !isConfigured {
		return nil
	}

	// If any field is set, validate required fields are present
	if a.Region == "" {
		return fmt.Errorf("region is required when Amazon SES is configured")
	}

	if a.AccessKey == "" {
		return fmt.Errorf("access key is required when Amazon SES is configured")
	}

	// only encrypt secret key if it's not empty
	if a.SecretKey != "" {
		if err := a.EncryptSecretKey(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt SES secret key: %w", err)
		}
	}

	return nil
}

// SESServiceInterface defines operations for managing Amazon SES webhooks via SNS
type SESServiceInterface interface {
	// ListConfigurationSets lists all configuration sets
	ListConfigurationSets(ctx context.Context, config AmazonSESSettings) ([]string, error)

	// CreateConfigurationSet creates a new configuration set
	CreateConfigurationSet(ctx context.Context, config AmazonSESSettings, name string) error

	// DeleteConfigurationSet deletes a configuration set
	DeleteConfigurationSet(ctx context.Context, config AmazonSESSettings, name string) error

	// CreateSNSTopic creates a new SNS topic for notifications
	CreateSNSTopic(ctx context.Context, config AmazonSESSettings, topicConfig SESTopicConfig) (string, error)

	// DeleteSNSTopic deletes an SNS topic
	DeleteSNSTopic(ctx context.Context, config AmazonSESSettings, topicARN string) error

	// CreateEventDestination creates an event destination in a configuration set
	CreateEventDestination(ctx context.Context, config AmazonSESSettings, destination SESConfigurationSetEventDestination) error

	// UpdateEventDestination updates an event destination
	UpdateEventDestination(ctx context.Context, config AmazonSESSettings, destination SESConfigurationSetEventDestination) error

	// DeleteEventDestination deletes an event destination
	DeleteEventDestination(ctx context.Context, config AmazonSESSettings, configSetName, destinationName string) error

	// ListEventDestinations lists all event destinations for a configuration set
	ListEventDestinations(ctx context.Context, config AmazonSESSettings, configSetName string) ([]SESConfigurationSetEventDestination, error)
}
