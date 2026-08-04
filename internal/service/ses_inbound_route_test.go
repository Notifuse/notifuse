package service

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testInboundURL = "https://app.example.com/webhooks/email/inbound?workspace_id=ws1&integration_id=int1"

func sesProvider() *domain.EmailProvider {
	return &domain.EmailProvider{SES: &domain.AmazonSESSettings{
		AccessKey: "ak", SecretKey: "sk", Region: "us-east-1",
	}}
}

// expectIdentities stubs the two ListIdentities calls (Domain + EmailAddress) inboundRecipients
// makes, returning the given domains for the Domain query and emails for the EmailAddress query.
func expectIdentities(mockSES *mocks.MockSESWebhookClient, domains, emails []string) {
	mockSES.EXPECT().ListIdentitiesWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *ses.ListIdentitiesInput, _ ...interface{}) (*ses.ListIdentitiesOutput, error) {
			if aws.StringValue(in.IdentityType) == ses.IdentityTypeEmailAddress {
				return &ses.ListIdentitiesOutput{Identities: aws.StringSlice(emails)}, nil
			}
			return &ses.ListIdentitiesOutput{Identities: aws.StringSlice(domains)}, nil
		}).Times(2)
}

// expectInboundTopicSetup sets up the common SNS topic + policy + signature + subscribe calls
// shared by every successful EnsureInboundRoute path.
func expectInboundTopicSetup(mockSNS *mocks.MockSNSClient) {
	mockSNS.EXPECT().CreateTopicWithContext(gomock.Any(), gomock.Any()).
		Return(&sns.CreateTopicOutput{TopicArn: aws.String("arn:aws:sns:us-east-1:123456789012:notifuse-ses-inbound-int1")}, nil)
	mockSNS.EXPECT().SetTopicAttributesWithContext(gomock.Any(), gomock.Any()).
		Return(&sns.SetTopicAttributesOutput{}, nil).Times(2) // Policy + SignatureVersion
	mockSNS.EXPECT().SubscribeWithContext(gomock.Any(), gomock.Any()).
		Return(&sns.SubscribeOutput{}, nil)
}

func TestEnsureInboundRoute_NoActiveRuleSet_CreatesAndActivates(t *testing.T) {
	service, mockSES, mockSNSC, _, _ := createMockSESService(t)
	expectInboundTopicSetup(mockSNSC)

	// A domain identity AND an email-address-only identity — both must scope the rule.
	expectIdentities(mockSES, []string{"example.com"}, []string{"support@other.com"})
	// No active rule set.
	mockSES.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeActiveReceiptRuleSetOutput{}, nil)
	mockSES.EXPECT().CreateReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *ses.CreateReceiptRuleSetInput, _ ...interface{}) (*ses.CreateReceiptRuleSetOutput, error) {
			assert.Equal(t, sesInboundRuleSetName, aws.StringValue(in.RuleSetName))
			return &ses.CreateReceiptRuleSetOutput{}, nil
		})
	var createdRule *ses.ReceiptRule
	mockSES.EXPECT().CreateReceiptRuleWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *ses.CreateReceiptRuleInput, _ ...interface{}) (*ses.CreateReceiptRuleOutput, error) {
			assert.Equal(t, sesInboundRuleSetName, aws.StringValue(in.RuleSetName))
			createdRule = in.Rule
			return &ses.CreateReceiptRuleOutput{}, nil
		})
	mockSES.EXPECT().SetActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *ses.SetActiveReceiptRuleSetInput, _ ...interface{}) (*ses.SetActiveReceiptRuleSetOutput, error) {
			assert.Equal(t, sesInboundRuleSetName, aws.StringValue(in.RuleSetName))
			return &ses.SetActiveReceiptRuleSetOutput{}, nil
		})

	err := service.EnsureInboundRoute(context.Background(), sesProvider(), testInboundURL)
	require.NoError(t, err)

	require.NotNil(t, createdRule)
	assert.Equal(t, "notifuse-inbound-int1", aws.StringValue(createdRule.Name))
	assert.True(t, aws.BoolValue(createdRule.Enabled))
	assert.ElementsMatch(t, []string{"example.com", "support@other.com"},
		aws.StringValueSlice(createdRule.Recipients), "scoped to domain AND email identities")
	require.Len(t, createdRule.Actions, 1)
	assert.Equal(t, "arn:aws:sns:us-east-1:123456789012:notifuse-ses-inbound-int1", aws.StringValue(createdRule.Actions[0].SNSAction.TopicArn))
	assert.Equal(t, ses.SNSActionEncodingBase64, aws.StringValue(createdRule.Actions[0].SNSAction.Encoding))
}

func TestEnsureInboundRoute_ActiveRuleSet_InsertsWithoutActivating(t *testing.T) {
	service, mockSES, mockSNSC, _, _ := createMockSESService(t)
	expectInboundTopicSetup(mockSNSC)

	expectIdentities(mockSES, []string{"example.com"}, nil)
	// A customer rule set is already active and contains an unrelated rule.
	mockSES.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeActiveReceiptRuleSetOutput{
			Metadata: &ses.ReceiptRuleSetMetadata{Name: aws.String("customer-active-set")},
			Rules:    []*ses.ReceiptRule{{Name: aws.String("workmail-rule")}},
		}, nil)
	mockSES.EXPECT().CreateReceiptRuleWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *ses.CreateReceiptRuleInput, _ ...interface{}) (*ses.CreateReceiptRuleOutput, error) {
			assert.Equal(t, "customer-active-set", aws.StringValue(in.RuleSetName), "must insert into the EXISTING active set")
			return &ses.CreateReceiptRuleOutput{}, nil
		})
	// CRITICAL coexistence guarantee: never create/activate a new set when one is active.
	mockSES.EXPECT().CreateReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).Times(0)
	mockSES.EXPECT().SetActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).Times(0)

	err := service.EnsureInboundRoute(context.Background(), sesProvider(), testInboundURL)
	require.NoError(t, err)
}

func TestEnsureInboundRoute_Idempotent_RuleAlreadyPresent(t *testing.T) {
	service, mockSES, mockSNSC, _, _ := createMockSESService(t)
	expectInboundTopicSetup(mockSNSC)

	// Already registered: the idempotency check returns BEFORE listing identities/building the
	// rule, so no ListIdentities call happens.
	mockSES.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeActiveReceiptRuleSetOutput{
			Metadata: &ses.ReceiptRuleSetMetadata{Name: aws.String("active")},
			Rules:    []*ses.ReceiptRule{{Name: aws.String("notifuse-inbound-int1")}},
		}, nil)
	// Already registered → no rule mutation at all.
	mockSES.EXPECT().CreateReceiptRuleWithContext(gomock.Any(), gomock.Any()).Times(0)
	mockSES.EXPECT().SetActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).Times(0)

	err := service.EnsureInboundRoute(context.Background(), sesProvider(), testInboundURL)
	require.NoError(t, err)
}

func TestEnsureInboundRoute_UnsupportedRegion_SoftSkips(t *testing.T) {
	// A sending-only region must NOT fail registration — it soft-skips inbound provisioning so
	// the delivery/bounce/complaint webhooks still register. No SNS/SES calls are expected (the
	// strict mocks would fail on any unexpected call).
	service, _, _, _, _ := createMockSESService(t)
	provider := sesProvider()
	provider.SES.Region = "ca-west-1" // sending-only region; no inbound

	err := service.EnsureInboundRoute(context.Background(), provider, testInboundURL)
	assert.NoError(t, err)
}

func TestEnsureInboundRoute_InvalidConfig(t *testing.T) {
	service, _, _, _, _ := createMockSESService(t)
	err := service.EnsureInboundRoute(context.Background(), &domain.EmailProvider{}, testInboundURL)
	assert.ErrorIs(t, err, ErrInvalidSESConfig)
}

func TestEnsureInboundRoute_SetsTopicPolicyAndSignatureV2(t *testing.T) {
	service, mockSES, mockSNSC, _, _ := createMockSESService(t)
	mockSNSC.EXPECT().CreateTopicWithContext(gomock.Any(), gomock.Any()).
		Return(&sns.CreateTopicOutput{TopicArn: aws.String("arn:aws:sns:us-east-1:123456789012:notifuse-ses-inbound-int1")}, nil)

	attrs := map[string]string{}
	mockSNSC.EXPECT().SetTopicAttributesWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *sns.SetTopicAttributesInput, _ ...interface{}) (*sns.SetTopicAttributesOutput, error) {
			attrs[aws.StringValue(in.AttributeName)] = aws.StringValue(in.AttributeValue)
			return &sns.SetTopicAttributesOutput{}, nil
		}).Times(2)
	mockSNSC.EXPECT().SubscribeWithContext(gomock.Any(), gomock.Any()).Return(&sns.SubscribeOutput{}, nil)
	mockSES.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeActiveReceiptRuleSetOutput{Metadata: &ses.ReceiptRuleSetMetadata{Name: aws.String("a")}}, nil)
	expectIdentities(mockSES, []string{"example.com"}, nil)
	mockSES.EXPECT().CreateReceiptRuleWithContext(gomock.Any(), gomock.Any()).Return(&ses.CreateReceiptRuleOutput{}, nil)

	err := service.EnsureInboundRoute(context.Background(), sesProvider(), testInboundURL)
	require.NoError(t, err)

	assert.Equal(t, "2", attrs["SignatureVersion"], "must enforce SNS SignatureVersion 2 (SHA-256)")
	require.Contains(t, attrs, "Policy")
	assert.Contains(t, attrs["Policy"], "ses.amazonaws.com", "policy must grant SES publish")
	assert.Contains(t, attrs["Policy"], "123456789012", "policy must scope to the topic's AWS account")
}

func TestEnsureInboundRoute_FailsClosedOnNoIdentities(t *testing.T) {
	// No verified identities → an empty Recipients set would mean "match ALL inbound mail".
	// Provisioning must FAIL rather than insert an account-wide forwarder into the active set.
	service, mockSES, mockSNSC, _, _ := createMockSESService(t)
	expectInboundTopicSetup(mockSNSC)
	mockSES.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeActiveReceiptRuleSetOutput{Metadata: &ses.ReceiptRuleSetMetadata{Name: aws.String("active")}}, nil)
	expectIdentities(mockSES, nil, nil) // no domain or email identities
	mockSES.EXPECT().CreateReceiptRuleWithContext(gomock.Any(), gomock.Any()).Times(0)

	err := service.EnsureInboundRoute(context.Background(), sesProvider(), testInboundURL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no verified SES identities")
}

func TestEnsureInboundRoute_RecordsTopicARNForBinding(t *testing.T) {
	// EnsureInboundRoute must write the provisioned topic ARN onto the settings so the caller
	// can persist it and the inbound parser can bind to it.
	service, mockSES, mockSNSC, _, _ := createMockSESService(t)
	expectInboundTopicSetup(mockSNSC)
	expectIdentities(mockSES, []string{"example.com"}, nil)
	mockSES.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeActiveReceiptRuleSetOutput{Metadata: &ses.ReceiptRuleSetMetadata{Name: aws.String("a")}}, nil)
	mockSES.EXPECT().CreateReceiptRuleWithContext(gomock.Any(), gomock.Any()).Return(&ses.CreateReceiptRuleOutput{}, nil)

	provider := sesProvider()
	err := service.EnsureInboundRoute(context.Background(), provider, testInboundURL)
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:sns:us-east-1:123456789012:notifuse-ses-inbound-int1", provider.SES.InboundTopicARN)
}

func TestUnregisterWebhooks_DeletesInboundRule(t *testing.T) {
	service, mockSES, mockSNSC, _, _ := createMockSESService(t)

	// Inbound reversal: our rule is present in the active set → it must be deleted.
	mockSES.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeActiveReceiptRuleSetOutput{
			Metadata: &ses.ReceiptRuleSetMetadata{Name: aws.String("active")},
			Rules:    []*ses.ReceiptRule{{Name: aws.String("notifuse-inbound-int1")}},
		}, nil)
	deleted := false
	mockSES.EXPECT().DeleteReceiptRuleWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *ses.DeleteReceiptRuleInput, _ ...interface{}) (*ses.DeleteReceiptRuleOutput, error) {
			assert.Equal(t, "active", aws.StringValue(in.RuleSetName))
			assert.Equal(t, "notifuse-inbound-int1", aws.StringValue(in.RuleName))
			deleted = true
			return &ses.DeleteReceiptRuleOutput{}, nil
		})
	// The event-destination cleanup path: no config set exists → it returns after the inbound step.
	mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{ConfigurationSets: []*ses.ConfigurationSet{}}, nil)
	_ = mockSNSC

	provider := &domain.EmailProvider{SES: &domain.AmazonSESSettings{AccessKey: "ak", SecretKey: "sk", Region: "us-east-1"}}
	err := service.UnregisterWebhooks(context.Background(), "ws1", "int1", provider)
	require.NoError(t, err)
	assert.True(t, deleted, "the inbound receipt rule must be deleted on unregister")
}

func TestGetWebhookStatus_InboundRegistered(t *testing.T) {
	service, mockSES, _, _, _ := createMockSESService(t)

	// Active set contains our inbound rule → inbound_registered must be true.
	mockSES.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeActiveReceiptRuleSetOutput{
			Metadata: &ses.ReceiptRuleSetMetadata{Name: aws.String("active")},
			Rules:    []*ses.ReceiptRule{{Name: aws.String("notifuse-inbound-int1")}},
		}, nil)
	// No event configuration set → IsRegistered stays false, but inbound_registered is true.
	mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{ConfigurationSets: []*ses.ConfigurationSet{}}, nil)

	provider := &domain.EmailProvider{SES: &domain.AmazonSESSettings{AccessKey: "ak", SecretKey: "sk", Region: "us-east-1"}}
	status, err := service.GetWebhookStatus(context.Background(), "ws1", "int1", provider)
	require.NoError(t, err)
	assert.Equal(t, true, status.ProviderDetails["inbound_registered"])
}

// isAWSErrCode helper sanity (exercised by EnsureInboundRoute's AlreadyExists tolerance).
func TestIsAWSErrCode(t *testing.T) {
	err := awserr.New(ses.ErrCodeAlreadyExistsException, "exists", nil)
	assert.True(t, isAWSErrCode(err, ses.ErrCodeAlreadyExistsException))
	assert.False(t, isAWSErrCode(err, ses.ErrCodeRuleDoesNotExistException))
	assert.False(t, isAWSErrCode(assert.AnError, ses.ErrCodeAlreadyExistsException))
}
