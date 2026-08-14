package domain

import (
	"context"
	"fmt"
	"regexp"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

//go:generate mockgen -destination mocks/mock_ses_service.go -package mocks github.com/Notifuse/notifuse/internal/domain SESServiceInterface
// SESWebhookPayload represents an Amazon SES webhook payload
type SESWebhookPayload struct {
	Type              string                         `json:"Type"`
	MessageID         string                         `json:"MessageId"`
	TopicARN          string                         `json:"TopicArn"`
	Message           string                         `json:"Message"`
	Subject           string                         `json:"Subject,omitempty"` // SNS subject; part of the signed string when present
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

// SESReceivedNotification is the inner notification SES publishes to SNS for an INBOUND
// (received) email, when a receipt rule has an SNS action — used for stop-on-reply reply
// ingestion. Mail.Headers carries the full header set (incl. In-Reply-To/References) unless
// HeadersTruncated; Content holds the raw MIME (base64 when the action uses Base64 encoding).
type SESReceivedNotification struct {
	NotificationType string  `json:"notificationType"` // "Received"
	Mail             SESMail `json:"mail"`
	Content          string  `json:"content"`
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

	// InboundTopicARN is the SNS topic ARN provisioned for stop-on-reply inbound mail
	// (written by EnsureInboundRoute). The inbound parser binds to it: a valid SNS signature
	// only proves AWS signed the message, not that it came from OUR topic, so messages whose
	// TopicArn doesn't match this are rejected. For manual SES inbound setups this must be set
	// explicitly (the SNS topic ARN that the receipt rule publishes to).
	InboundTopicARN string `json:"inbound_topic_arn,omitempty"`

	// --- tenant isolation: operator intent -------------------------------------------------

	// TenantIsolationEnabled turns on Notifuse-managed SES tenant isolation: a tenant per
	// integration with its own reputation profile and its own suppression list, so another
	// workspace's bounces cannot pause or suppress this one. Provisioning is explicit and
	// billable, so this flag only records the request; ManagedTenantName records what was
	// actually created in AWS.
	TenantIsolationEnabled bool `json:"tenant_isolation_enabled,omitempty"`

	// --- advanced overrides (escape hatch) -------------------------------------------------

	// ConfigurationSetName overrides the auto-managed "notifuse-<integrationID>" configuration
	// set. Empty means Notifuse manages it.
	ConfigurationSetName string `json:"configuration_set_name,omitempty"`

	// TenantName scopes sends to a tenant the operator manages themselves. Mutually exclusive
	// with TenantIsolationEnabled: two sources of truth for the same value is not a state worth
	// having.
	//
	// It is a typed field rather than a generic custom-header bag for a hard reason, not a
	// stylistic one: AWS honours the X-SES-TENANT header over SMTP only and never on the v2
	// API, which is the path used here. A header mechanism could not carry a tenant at all.
	TenantName string `json:"tenant_name,omitempty"`

	// --- derived state, written by provisioning; never form fields -------------------------

	// ManagedConfigurationSet is the "notifuse-<integrationID>" set created during webhook
	// registration. It is persisted so the send path never has to call ListConfigurationSets,
	// which AWS limits to once per second per account+region.
	ManagedConfigurationSet string `json:"managed_configuration_set,omitempty"`

	// ManagedTenantName is the tenant Notifuse created and associated. Empty while isolation is
	// requested but not yet provisioned — a state the UI surfaces rather than hides.
	ManagedTenantName string `json:"managed_tenant_name,omitempty"`
}

// sesResourceNameRegex matches the SES naming rule shared by tenants and configuration sets:
// "up to 64 alphanumeric characters, including letters, numbers, hyphens (-) and underscores (_)".
var sesResourceNameRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ResolveConfigurationSet returns the configuration set to send through: the operator's override
// first, then the set Notifuse manages. An empty result means "none resolved" — the caller decides
// whether to look one up or send without.
func (a *AmazonSESSettings) ResolveConfigurationSet() string {
	if a.ConfigurationSetName != "" {
		return a.ConfigurationSetName
	}
	return a.ManagedConfigurationSet
}

// ResolveTenant returns the tenant a SEND should be scoped to: the operator's own tenant first,
// then the one Notifuse manages — and that one only while isolation is actually switched on.
//
// The flag is what makes the switch two-way. ManagedTenantName is a record of what exists in AWS,
// not permission to use it: without this check, turning isolation off would leave every message
// still scoped to a tenant the operator believes they have disabled, and the only way to stop
// would be to delete the integration.
func (a *AmazonSESSettings) ResolveTenant() string {
	if a.TenantName != "" {
		return a.TenantName
	}
	if !a.TenantIsolationEnabled {
		return ""
	}
	return a.ManagedTenantName
}

// KnownTenant returns any tenant this integration is associated with in AWS, whether or not it is
// currently used for sending. Teardown and verification need this rather than ResolveTenant:
// a tenant that isolation was switched off for still exists, still holds the configuration set
// association that blocks deletion, and still bills.
func (a *AmazonSESSettings) KnownTenant() string {
	if a.ManagedTenantName != "" {
		return a.ManagedTenantName
	}
	return a.TenantName
}

// OwnsManagedTenant reports whether Notifuse created the tenant, and may therefore delete it.
// An operator's own tenant is never ours to remove.
func (a *AmazonSESSettings) OwnsManagedTenant() bool {
	return a.ManagedTenantName != ""
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
	// Check if any field is set to determine if we should validate. The tenant fields count:
	// a blob carrying only a tenant name must report the missing region rather than pass as
	// "not configured".
	isConfigured := a.Region != "" || a.AccessKey != "" ||
		a.EncryptedSecretKey != "" || a.SecretKey != "" ||
		a.TenantIsolationEnabled || a.TenantName != "" || a.ConfigurationSetName != ""

	// If no fields are set, consider it valid (optional config)
	if !isConfigured {
		return nil
	}

	// Managed isolation and a hand-managed tenant are two sources of truth for the same value.
	if a.TenantIsolationEnabled && a.TenantName != "" {
		return fmt.Errorf("tenant isolation cannot be enabled while an explicit tenant name is set: clear one of them")
	}

	if a.ConfigurationSetName != "" && !sesResourceNameRegex.MatchString(a.ConfigurationSetName) {
		return fmt.Errorf("invalid configuration set name: up to 64 letters, numbers, hyphens or underscores")
	}

	if a.TenantName != "" && !sesResourceNameRegex.MatchString(a.TenantName) {
		return fmt.Errorf("invalid tenant name: up to 64 letters, numbers, hyphens or underscores")
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
