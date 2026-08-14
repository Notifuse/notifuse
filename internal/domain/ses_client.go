package domain

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/aws/aws-sdk-go/service/sns"
)

//go:generate mockgen -destination mocks/mock_ses_webhook_client.go -package mocks github.com/Notifuse/notifuse/internal/domain SESWebhookClient

// SESWebhookClient defines the interface for interacting with AWS SES service.
//
// This side deliberately stays on AWS SDK **v1** while the send path uses v2, and the
// two coexist in one binary on purpose. Only sending needed v2 (tenant support), and
// configuration-set CRUD, SNS topics, event destinations and inbound receipt rules —
// roughly nine hundred lines — gain nothing from being ported. Linters flag every v1
// import as deprecated, which makes "finish the migration" a recurring suggestion:
// it is a large, risky rewrite of working code with no feature behind it, so weigh it
// on its own merits rather than as cleanup.
type SESWebhookClient interface {
	CreateConfigurationSetWithContext(ctx aws.Context, input *ses.CreateConfigurationSetInput, opts ...request.Option) (*ses.CreateConfigurationSetOutput, error)
	DeleteConfigurationSetWithContext(ctx aws.Context, input *ses.DeleteConfigurationSetInput, opts ...request.Option) (*ses.DeleteConfigurationSetOutput, error)
	ListConfigurationSetsWithContext(ctx aws.Context, input *ses.ListConfigurationSetsInput, opts ...request.Option) (*ses.ListConfigurationSetsOutput, error)
	DescribeConfigurationSetWithContext(ctx aws.Context, input *ses.DescribeConfigurationSetInput, opts ...request.Option) (*ses.DescribeConfigurationSetOutput, error)
	CreateConfigurationSetEventDestinationWithContext(ctx aws.Context, input *ses.CreateConfigurationSetEventDestinationInput, opts ...request.Option) (*ses.CreateConfigurationSetEventDestinationOutput, error)
	UpdateConfigurationSetEventDestinationWithContext(ctx aws.Context, input *ses.UpdateConfigurationSetEventDestinationInput, opts ...request.Option) (*ses.UpdateConfigurationSetEventDestinationOutput, error)
	DeleteConfigurationSetEventDestinationWithContext(ctx aws.Context, input *ses.DeleteConfigurationSetEventDestinationInput, opts ...request.Option) (*ses.DeleteConfigurationSetEventDestinationOutput, error)

	// Inbound (stop-on-reply) receipt-rule management. SES delivers inbound mail via a
	// receipt rule in the single active rule set per region, so provisioning must inspect the
	// active set and insert into it rather than blindly activate a new one.
	DescribeActiveReceiptRuleSetWithContext(ctx aws.Context, input *ses.DescribeActiveReceiptRuleSetInput, opts ...request.Option) (*ses.DescribeActiveReceiptRuleSetOutput, error)
	CreateReceiptRuleSetWithContext(ctx aws.Context, input *ses.CreateReceiptRuleSetInput, opts ...request.Option) (*ses.CreateReceiptRuleSetOutput, error)
	SetActiveReceiptRuleSetWithContext(ctx aws.Context, input *ses.SetActiveReceiptRuleSetInput, opts ...request.Option) (*ses.SetActiveReceiptRuleSetOutput, error)
	CreateReceiptRuleWithContext(ctx aws.Context, input *ses.CreateReceiptRuleInput, opts ...request.Option) (*ses.CreateReceiptRuleOutput, error)
	DeleteReceiptRuleWithContext(ctx aws.Context, input *ses.DeleteReceiptRuleInput, opts ...request.Option) (*ses.DeleteReceiptRuleOutput, error)
	ListIdentitiesWithContext(ctx aws.Context, input *ses.ListIdentitiesInput, opts ...request.Option) (*ses.ListIdentitiesOutput, error)
}

//go:generate mockgen -destination mocks/mock_sns_client.go -package mocks -mock_names SNSWebhookClient=MockSNSClient github.com/Notifuse/notifuse/internal/domain SNSWebhookClient

// SNSWebhookClient defines the interface for interacting with AWS SNS service
type SNSWebhookClient interface {
	CreateTopicWithContext(ctx aws.Context, input *sns.CreateTopicInput, opts ...request.Option) (*sns.CreateTopicOutput, error)
	DeleteTopicWithContext(ctx aws.Context, input *sns.DeleteTopicInput, opts ...request.Option) (*sns.DeleteTopicOutput, error)
	SubscribeWithContext(ctx aws.Context, input *sns.SubscribeInput, opts ...request.Option) (*sns.SubscribeOutput, error)
	GetTopicAttributesWithContext(ctx aws.Context, input *sns.GetTopicAttributesInput, opts ...request.Option) (*sns.GetTopicAttributesOutput, error)
	// SetTopicAttributesWithContext lets the inbound topic enforce SignatureVersion 2 and grant
	// SES permission to publish (topic access policy).
	SetTopicAttributesWithContext(ctx aws.Context, input *sns.SetTopicAttributesInput, opts ...request.Option) (*sns.SetTopicAttributesOutput, error)
}
