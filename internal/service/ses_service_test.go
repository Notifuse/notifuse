package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/quotedprintable"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/smithy-go"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a mock SES service for testing
func createMockSESService(t *testing.T) (*SESService, *mocks.MockSESWebhookClient, *mocks.MockSNSClient, *mocks.MockAuthService, *pkgmocks.MockLogger) {
	svc, ses, sns, auth, log, _ := createMockSESServiceWithV2(t)
	return svc, ses, sns, auth, log
}

// createMockSESServiceWithV2 also exposes the SES v2 client, which is the one the send path uses.
func createMockSESServiceWithV2(t *testing.T) (*SESService, *mocks.MockSESWebhookClient, *mocks.MockSNSClient, *mocks.MockAuthService, *pkgmocks.MockLogger, *mocks.MockSESv2Client) {
	ctrl := gomock.NewController(t)
	mockSES := mocks.NewMockSESWebhookClient(ctrl)
	mockSESv2 := mocks.NewMockSESv2Client(ctrl)
	mockSNS := mocks.NewMockSNSClient(ctrl)
	mockAuth := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger to handle any calls
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Fatal(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()

	return NewSESServiceWithClients(
		mockAuth,
		mockLogger,
		func(_ domain.AmazonSESSettings) (*session.Session, error) {
			return &session.Session{}, nil
		},
		func(_ *session.Session) domain.SESWebhookClient {
			return mockSES
		},
		func(_ *session.Session) domain.SNSWebhookClient {
			return mockSNS
		},
		func(_ domain.AmazonSESSettings) domain.SESv2Client {
			return mockSESv2
		},
	), mockSES, mockSNS, mockAuth, mockLogger, mockSESv2
}

func createMockSESServiceWithSessionError(t *testing.T) (*SESService, *mocks.MockSESWebhookClient, *mocks.MockSNSClient, *mocks.MockAuthService, *pkgmocks.MockLogger) {
	ctrl := gomock.NewController(t)
	mockSES := mocks.NewMockSESWebhookClient(ctrl)
	mockSESv2 := mocks.NewMockSESv2Client(ctrl)
	mockSNS := mocks.NewMockSNSClient(ctrl)
	mockAuth := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger to handle any calls
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Fatal(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()

	return NewSESServiceWithClients(
		mockAuth,
		mockLogger,
		func(_ domain.AmazonSESSettings) (*session.Session, error) {
			return nil, errors.New("session creation failed")
		},
		func(_ *session.Session) domain.SESWebhookClient {
			return mockSES
		},
		func(_ *session.Session) domain.SNSWebhookClient {
			return mockSNS
		},
		func(_ domain.AmazonSESSettings) domain.SESv2Client {
			return mockSESv2
		},
	), mockSES, mockSNS, mockAuth, mockLogger
}

func getValidSESConfig() domain.AmazonSESSettings {
	return domain.AmazonSESSettings{
		AccessKey: "test-access-key",
		SecretKey: "test-secret-key",
		Region:    "us-east-1",
	}
}

func getInvalidSESConfig() domain.AmazonSESSettings {
	return domain.AmazonSESSettings{
		AccessKey: "",
		SecretKey: "",
		Region:    "us-east-1",
	}
}

// Test NewSESService
func TestNewSESService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuth := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSESService(mockAuth, mockLogger)

	assert.NotNil(t, service)
	assert.Equal(t, mockAuth, service.authService)
	assert.Equal(t, mockLogger, service.logger)
	assert.NotNil(t, service.sessionFactory)
	assert.NotNil(t, service.sesClientFactory)
	assert.NotNil(t, service.snsClientFactory)
	assert.NotNil(t, service.sesV2ClientFactory)

	// Test that the factories work correctly
	config := getValidSESConfig()

	// Test session factory
	session, err := service.sessionFactory(config)
	assert.NoError(t, err)
	assert.NotNil(t, session)

	// Test client factories
	sesClient := service.sesClientFactory(session)
	assert.NotNil(t, sesClient)

	snsClient := service.snsClientFactory(session)
	assert.NotNil(t, snsClient)

	sesEmailClient := service.sesV2ClientFactory(config)
	assert.NotNil(t, sesEmailClient)
}

// Test NewSESServiceWithClients
func TestNewSESServiceWithClients(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuth := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	sessionFactory := func(_ domain.AmazonSESSettings) (*session.Session, error) {
		return &session.Session{}, nil
	}
	sesClientFactory := func(_ *session.Session) domain.SESWebhookClient {
		return nil
	}
	snsClientFactory := func(_ *session.Session) domain.SNSWebhookClient {
		return nil
	}
	sesV2ClientFactory := func(_ domain.AmazonSESSettings) domain.SESv2Client {
		return nil
	}

	service := NewSESServiceWithClients(mockAuth, mockLogger, sessionFactory, sesClientFactory, snsClientFactory, sesV2ClientFactory)

	assert.NotNil(t, service)
	assert.Equal(t, mockAuth, service.authService)
	assert.Equal(t, mockLogger, service.logger)
}

// Test createSession
func TestCreateSession(t *testing.T) {
	config := getValidSESConfig()
	session, err := createSession(config)

	assert.NoError(t, err)
	assert.NotNil(t, session)
}

// Test getClients - success
func TestGetClients_Success(t *testing.T) {
	service, _, _, _, _ := createMockSESService(t)
	config := getValidSESConfig()

	sesClient, snsClient, err := service.getClients(config)

	assert.NoError(t, err)
	assert.NotNil(t, sesClient)
	assert.NotNil(t, snsClient)
}

// Test getClients - invalid credentials
func TestGetClients_InvalidCredentials(t *testing.T) {
	service, _, _, _, _ := createMockSESService(t)
	config := getInvalidSESConfig()

	sesClient, snsClient, err := service.getClients(config)

	assert.Error(t, err)
	assert.Equal(t, ErrInvalidAWSCredentials, err)
	assert.Nil(t, sesClient)
	assert.Nil(t, snsClient)
}

// Test getClients - session creation error
func TestGetClients_SessionError(t *testing.T) {
	service, _, _, _, _ := createMockSESServiceWithSessionError(t)
	config := getValidSESConfig()

	sesClient, snsClient, err := service.getClients(config)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create AWS session")
	assert.Nil(t, sesClient)
	assert.Nil(t, snsClient)
}

// Test ListConfigurationSets - success
func TestListConfigurationSets_Success(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()

	mockOutput := &ses.ListConfigurationSetsOutput{
		ConfigurationSets: []*ses.ConfigurationSet{
			{Name: aws.String("config-set-1")},
			{Name: aws.String("config-set-2")},
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(mockOutput, nil)

	result, err := service.ListConfigurationSets(context.Background(), config)

	assert.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Contains(t, result, "config-set-1")
	assert.Contains(t, result, "config-set-2")
}

// Test ListConfigurationSets - error
func TestListConfigurationSets_Error(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("AWS error"))

	result, err := service.ListConfigurationSets(context.Background(), config)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list SES configuration sets")
	assert.Nil(t, result)
}

// Test ListConfigurationSets - invalid credentials
func TestListConfigurationSets_InvalidCredentials(t *testing.T) {
	service, _, _, _, _ := createMockSESService(t)
	config := getInvalidSESConfig()

	result, err := service.ListConfigurationSets(context.Background(), config)

	assert.Error(t, err)
	assert.Equal(t, ErrInvalidAWSCredentials, err)
	assert.Nil(t, result)
}

// Test CreateConfigurationSet - success
func TestCreateConfigurationSet_Success(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()
	configSetName := "test-config-set"

	mockSESClient.EXPECT().
		CreateConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *ses.CreateConfigurationSetInput, _ ...request.Option) (*ses.CreateConfigurationSetOutput, error) {
			assert.Equal(t, configSetName, *input.ConfigurationSet.Name)
			return &ses.CreateConfigurationSetOutput{}, nil
		})

	err := service.CreateConfigurationSet(context.Background(), config, configSetName)

	assert.NoError(t, err)
}

// Test CreateConfigurationSet - error
func TestCreateConfigurationSet_Error(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()
	configSetName := "test-config-set"

	mockSESClient.EXPECT().
		CreateConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("AWS error"))

	err := service.CreateConfigurationSet(context.Background(), config, configSetName)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create SES configuration set")
}

// Test DeleteConfigurationSet - success
func TestDeleteConfigurationSet_Success(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()
	configSetName := "test-config-set"

	mockSESClient.EXPECT().
		DeleteConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *ses.DeleteConfigurationSetInput, _ ...request.Option) (*ses.DeleteConfigurationSetOutput, error) {
			assert.Equal(t, configSetName, *input.ConfigurationSetName)
			return &ses.DeleteConfigurationSetOutput{}, nil
		})

	err := service.DeleteConfigurationSet(context.Background(), config, configSetName)

	assert.NoError(t, err)
}

// Test DeleteConfigurationSet - error
func TestDeleteConfigurationSet_Error(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()
	configSetName := "test-config-set"

	mockSESClient.EXPECT().
		DeleteConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("AWS error"))

	err := service.DeleteConfigurationSet(context.Background(), config, configSetName)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete SES configuration set")
}

// Test CreateSNSTopic - success with new topic
func TestCreateSNSTopic_NewTopic_Success(t *testing.T) {
	service, _, mockSNSClient, _, _ := createMockSESService(t)
	config := getValidSESConfig()

	topicConfig := domain.SESTopicConfig{
		TopicName:            "test-topic",
		Protocol:             "https",
		NotificationEndpoint: "https://example.com/webhook",
	}

	mockCreateOutput := &sns.CreateTopicOutput{
		TopicArn: aws.String("arn:aws:sns:us-east-1:123456789012:test-topic"),
	}
	mockSubscribeOutput := &sns.SubscribeOutput{
		SubscriptionArn: aws.String("arn:aws:sns:us-east-1:123456789012:test-topic:subscription-id"),
	}

	mockSNSClient.EXPECT().
		CreateTopicWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sns.CreateTopicInput, _ ...request.Option) (*sns.CreateTopicOutput, error) {
			assert.Equal(t, topicConfig.TopicName, *input.Name)
			return mockCreateOutput, nil
		})

	mockSNSClient.EXPECT().
		SubscribeWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sns.SubscribeInput, _ ...request.Option) (*sns.SubscribeOutput, error) {
			assert.Equal(t, topicConfig.Protocol, *input.Protocol)
			assert.Equal(t, topicConfig.NotificationEndpoint, *input.Endpoint)
			assert.Equal(t, *mockCreateOutput.TopicArn, *input.TopicArn)
			return mockSubscribeOutput, nil
		})

	result, err := service.CreateSNSTopic(context.Background(), config, topicConfig)

	assert.NoError(t, err)
	assert.Equal(t, *mockCreateOutput.TopicArn, result)
}

// Test CreateSNSTopic - success with existing topic ARN
func TestCreateSNSTopic_ExistingTopic_Success(t *testing.T) {
	service, _, mockSNSClient, _, _ := createMockSESService(t)
	config := getValidSESConfig()

	existingTopicARN := "arn:aws:sns:us-east-1:123456789012:existing-topic"
	topicConfig := domain.SESTopicConfig{
		TopicARN:             existingTopicARN,
		Protocol:             "https",
		NotificationEndpoint: "https://example.com/webhook",
	}

	mockSNSClient.EXPECT().
		GetTopicAttributesWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sns.GetTopicAttributesInput, _ ...request.Option) (*sns.GetTopicAttributesOutput, error) {
			assert.Equal(t, existingTopicARN, *input.TopicArn)
			return &sns.GetTopicAttributesOutput{}, nil
		})

	result, err := service.CreateSNSTopic(context.Background(), config, topicConfig)

	assert.NoError(t, err)
	assert.Equal(t, existingTopicARN, result)
}

// Test CreateSNSTopic - default topic name
func TestCreateSNSTopic_DefaultTopicName_Success(t *testing.T) {
	service, _, mockSNSClient, _, _ := createMockSESService(t)
	config := getValidSESConfig()

	topicConfig := domain.SESTopicConfig{
		Protocol:             "https",
		NotificationEndpoint: "https://example.com/webhook",
	}

	mockCreateOutput := &sns.CreateTopicOutput{
		TopicArn: aws.String("arn:aws:sns:us-east-1:123456789012:notifuse-email-webhooks"),
	}

	mockSNSClient.EXPECT().
		CreateTopicWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sns.CreateTopicInput, _ ...request.Option) (*sns.CreateTopicOutput, error) {
			assert.Equal(t, "notifuse-email-webhooks", *input.Name)
			return mockCreateOutput, nil
		})

	mockSNSClient.EXPECT().
		SubscribeWithContext(gomock.Any(), gomock.Any()).
		Return(&sns.SubscribeOutput{}, nil)

	result, err := service.CreateSNSTopic(context.Background(), config, topicConfig)

	assert.NoError(t, err)
	assert.Equal(t, *mockCreateOutput.TopicArn, result)
}

// Test CreateSNSTopic - existing topic error
func TestCreateSNSTopic_ExistingTopicError(t *testing.T) {
	service, _, mockSNSClient, _, _ := createMockSESService(t)
	config := getValidSESConfig()

	existingTopicARN := "arn:aws:sns:us-east-1:123456789012:existing-topic"
	topicConfig := domain.SESTopicConfig{
		TopicARN:             existingTopicARN,
		Protocol:             "https",
		NotificationEndpoint: "https://example.com/webhook",
	}

	mockSNSClient.EXPECT().
		GetTopicAttributesWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("topic not found"))

	result, err := service.CreateSNSTopic(context.Background(), config, topicConfig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get SNS topic attributes")
	assert.Empty(t, result)
}

// Test CreateSNSTopic - create topic error
func TestCreateSNSTopic_CreateTopicError(t *testing.T) {
	service, _, mockSNSClient, _, _ := createMockSESService(t)
	config := getValidSESConfig()

	topicConfig := domain.SESTopicConfig{
		TopicName:            "test-topic",
		Protocol:             "https",
		NotificationEndpoint: "https://example.com/webhook",
	}

	mockSNSClient.EXPECT().
		CreateTopicWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("create topic failed"))

	result, err := service.CreateSNSTopic(context.Background(), config, topicConfig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create SNS topic")
	assert.Empty(t, result)
}

// Test CreateSNSTopic - subscribe error
func TestCreateSNSTopic_SubscribeError(t *testing.T) {
	service, _, mockSNSClient, _, _ := createMockSESService(t)
	config := getValidSESConfig()

	topicConfig := domain.SESTopicConfig{
		TopicName:            "test-topic",
		Protocol:             "https",
		NotificationEndpoint: "https://example.com/webhook",
	}

	mockCreateOutput := &sns.CreateTopicOutput{
		TopicArn: aws.String("arn:aws:sns:us-east-1:123456789012:test-topic"),
	}

	mockSNSClient.EXPECT().
		CreateTopicWithContext(gomock.Any(), gomock.Any()).
		Return(mockCreateOutput, nil)

	mockSNSClient.EXPECT().
		SubscribeWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("subscribe failed"))

	result, err := service.CreateSNSTopic(context.Background(), config, topicConfig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create SNS subscription")
	assert.Empty(t, result)
}

// Test DeleteSNSTopic - success
func TestDeleteSNSTopic_Success(t *testing.T) {
	service, _, mockSNSClient, _, _ := createMockSESService(t)
	config := getValidSESConfig()
	topicARN := "arn:aws:sns:us-east-1:123456789012:test-topic"

	mockSNSClient.EXPECT().
		DeleteTopicWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sns.DeleteTopicInput, _ ...request.Option) (*sns.DeleteTopicOutput, error) {
			assert.Equal(t, topicARN, *input.TopicArn)
			return &sns.DeleteTopicOutput{}, nil
		})

	err := service.DeleteSNSTopic(context.Background(), config, topicARN)

	assert.NoError(t, err)
}

// Test DeleteSNSTopic - error
func TestDeleteSNSTopic_Error(t *testing.T) {
	service, _, mockSNSClient, _, _ := createMockSESService(t)
	config := getValidSESConfig()
	topicARN := "arn:aws:sns:us-east-1:123456789012:test-topic"

	mockSNSClient.EXPECT().
		DeleteTopicWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("delete failed"))

	err := service.DeleteSNSTopic(context.Background(), config, topicARN)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete SNS topic")
}

// Test CreateEventDestination - success
func TestCreateEventDestination_Success(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()

	destination := domain.SESConfigurationSetEventDestination{
		ConfigurationSetName: "test-config-set",
		Name:                 "test-destination",
		Enabled:              true,
		MatchingEventTypes:   []string{"send", "bounce"},
		SNSDestination: &domain.SESTopicConfig{
			TopicARN: "arn:aws:sns:us-east-1:123456789012:test-topic",
		},
	}

	mockSESClient.EXPECT().
		CreateConfigurationSetEventDestinationWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *ses.CreateConfigurationSetEventDestinationInput, _ ...request.Option) (*ses.CreateConfigurationSetEventDestinationOutput, error) {
			assert.Equal(t, destination.ConfigurationSetName, *input.ConfigurationSetName)
			assert.Equal(t, destination.Name, *input.EventDestination.Name)
			assert.Equal(t, destination.Enabled, *input.EventDestination.Enabled)
			assert.Equal(t, destination.SNSDestination.TopicARN, *input.EventDestination.SNSDestination.TopicARN)
			return &ses.CreateConfigurationSetEventDestinationOutput{}, nil
		})

	err := service.CreateEventDestination(context.Background(), config, destination)

	assert.NoError(t, err)
}

// Test CreateEventDestination - invalid SNS destination
func TestCreateEventDestination_InvalidSNSDestination(t *testing.T) {
	service, _, _, _, _ := createMockSESService(t)
	config := getValidSESConfig()

	destination := domain.SESConfigurationSetEventDestination{
		ConfigurationSetName: "test-config-set",
		Name:                 "test-destination",
		Enabled:              true,
		MatchingEventTypes:   []string{"send", "bounce"},
		SNSDestination:       nil,
	}

	err := service.CreateEventDestination(context.Background(), config, destination)

	assert.Error(t, err)
	assert.Equal(t, ErrInvalidSNSDestination, err)
}

// Test CreateEventDestination - empty topic ARN
func TestCreateEventDestination_EmptyTopicARN(t *testing.T) {
	service, _, _, _, _ := createMockSESService(t)
	config := getValidSESConfig()

	destination := domain.SESConfigurationSetEventDestination{
		ConfigurationSetName: "test-config-set",
		Name:                 "test-destination",
		Enabled:              true,
		MatchingEventTypes:   []string{"send", "bounce"},
		SNSDestination: &domain.SESTopicConfig{
			TopicARN: "",
		},
	}

	err := service.CreateEventDestination(context.Background(), config, destination)

	assert.Error(t, err)
	assert.Equal(t, ErrInvalidSNSDestination, err)
}

// Test CreateEventDestination - AWS error
func TestCreateEventDestination_AWSError(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()

	destination := domain.SESConfigurationSetEventDestination{
		ConfigurationSetName: "test-config-set",
		Name:                 "test-destination",
		Enabled:              true,
		MatchingEventTypes:   []string{"send", "bounce"},
		SNSDestination: &domain.SESTopicConfig{
			TopicARN: "arn:aws:sns:us-east-1:123456789012:test-topic",
		},
	}

	mockSESClient.EXPECT().
		CreateConfigurationSetEventDestinationWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("AWS error"))

	err := service.CreateEventDestination(context.Background(), config, destination)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create SES event destination")
}

// Test UpdateEventDestination - success
func TestUpdateEventDestination_Success(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()

	destination := domain.SESConfigurationSetEventDestination{
		ConfigurationSetName: "test-config-set",
		Name:                 "test-destination",
		Enabled:              true,
		MatchingEventTypes:   []string{"send", "bounce"},
		SNSDestination: &domain.SESTopicConfig{
			TopicARN: "arn:aws:sns:us-east-1:123456789012:test-topic",
		},
	}

	mockSESClient.EXPECT().
		UpdateConfigurationSetEventDestinationWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *ses.UpdateConfigurationSetEventDestinationInput, _ ...request.Option) (*ses.UpdateConfigurationSetEventDestinationOutput, error) {
			assert.Equal(t, destination.ConfigurationSetName, *input.ConfigurationSetName)
			assert.Equal(t, destination.Name, *input.EventDestination.Name)
			assert.Equal(t, destination.Enabled, *input.EventDestination.Enabled)
			return &ses.UpdateConfigurationSetEventDestinationOutput{}, nil
		})

	err := service.UpdateEventDestination(context.Background(), config, destination)

	assert.NoError(t, err)
}

// Test UpdateEventDestination - error
func TestUpdateEventDestination_Error(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()

	destination := domain.SESConfigurationSetEventDestination{
		ConfigurationSetName: "test-config-set",
		Name:                 "test-destination",
		Enabled:              true,
		MatchingEventTypes:   []string{"send", "bounce"},
		SNSDestination: &domain.SESTopicConfig{
			TopicARN: "arn:aws:sns:us-east-1:123456789012:test-topic",
		},
	}

	mockSESClient.EXPECT().
		UpdateConfigurationSetEventDestinationWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("AWS error"))

	err := service.UpdateEventDestination(context.Background(), config, destination)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update SES event destination")
}

// Test DeleteEventDestination - success
func TestDeleteEventDestination_Success(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()
	configSetName := "test-config-set"
	destinationName := "test-destination"

	mockSESClient.EXPECT().
		DeleteConfigurationSetEventDestinationWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *ses.DeleteConfigurationSetEventDestinationInput, _ ...request.Option) (*ses.DeleteConfigurationSetEventDestinationOutput, error) {
			assert.Equal(t, configSetName, *input.ConfigurationSetName)
			assert.Equal(t, destinationName, *input.EventDestinationName)
			return &ses.DeleteConfigurationSetEventDestinationOutput{}, nil
		})

	err := service.DeleteEventDestination(context.Background(), config, configSetName, destinationName)

	assert.NoError(t, err)
}

// Test DeleteEventDestination - error
func TestDeleteEventDestination_Error(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()
	configSetName := "test-config-set"
	destinationName := "test-destination"

	mockSESClient.EXPECT().
		DeleteConfigurationSetEventDestinationWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("AWS error"))

	err := service.DeleteEventDestination(context.Background(), config, configSetName, destinationName)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete SES event destination")
}

// Test ListEventDestinations - success
func TestListEventDestinations_Success(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()
	configSetName := "test-config-set"

	mockOutput := &ses.DescribeConfigurationSetOutput{
		EventDestinations: []*ses.EventDestination{
			{
				Name:               aws.String("destination-1"),
				Enabled:            aws.Bool(true),
				MatchingEventTypes: aws.StringSlice([]string{"send", "bounce"}),
				SNSDestination: &ses.SNSDestination{
					TopicARN: aws.String("arn:aws:sns:us-east-1:123456789012:topic-1"),
				},
			},
			{
				Name:               aws.String("destination-2"),
				Enabled:            aws.Bool(false),
				MatchingEventTypes: aws.StringSlice([]string{"complaint"}),
				SNSDestination: &ses.SNSDestination{
					TopicARN: aws.String("arn:aws:sns:us-east-1:123456789012:topic-2"),
				},
			},
		},
	}

	mockSESClient.EXPECT().
		DescribeConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *ses.DescribeConfigurationSetInput, _ ...request.Option) (*ses.DescribeConfigurationSetOutput, error) {
			assert.Equal(t, configSetName, *input.ConfigurationSetName)
			return mockOutput, nil
		})

	result, err := service.ListEventDestinations(context.Background(), config, configSetName)

	assert.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "destination-1", result[0].Name)
	assert.Equal(t, configSetName, result[0].ConfigurationSetName)
	assert.True(t, result[0].Enabled)
	assert.Equal(t, []string{"send", "bounce"}, result[0].MatchingEventTypes)
	assert.Equal(t, "arn:aws:sns:us-east-1:123456789012:topic-1", result[0].SNSDestination.TopicARN)
}

// Test ListEventDestinations - skip non-SNS destinations
func TestListEventDestinations_SkipNonSNS(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()
	configSetName := "test-config-set"

	mockOutput := &ses.DescribeConfigurationSetOutput{
		EventDestinations: []*ses.EventDestination{
			{
				Name:               aws.String("destination-1"),
				Enabled:            aws.Bool(true),
				MatchingEventTypes: aws.StringSlice([]string{"send"}),
				SNSDestination: &ses.SNSDestination{
					TopicARN: aws.String("arn:aws:sns:us-east-1:123456789012:topic-1"),
				},
			},
			{
				Name:               aws.String("destination-2"),
				Enabled:            aws.Bool(true),
				MatchingEventTypes: aws.StringSlice([]string{"bounce"}),
				SNSDestination:     nil, // This should be skipped
			},
		},
	}

	mockSESClient.EXPECT().
		DescribeConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(mockOutput, nil)

	result, err := service.ListEventDestinations(context.Background(), config, configSetName)

	assert.NoError(t, err)
	assert.Len(t, result, 1) // Only one destination should be returned
	assert.Equal(t, "destination-1", result[0].Name)
}

// Test ListEventDestinations - error
func TestListEventDestinations_Error(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()
	configSetName := "test-config-set"

	mockSESClient.EXPECT().
		DescribeConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("AWS error"))

	result, err := service.ListEventDestinations(context.Background(), config, configSetName)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list SES event destinations")
	assert.Nil(t, result)
}

// Test SetupSNSTopic
func TestSetupSNSTopic(t *testing.T) {
	service, _, mockSNSClient, _, _ := createMockSESService(t)
	config := getValidSESConfig()

	topicConfig := domain.SESTopicConfig{
		TopicName:            "test-topic",
		Protocol:             "https",
		NotificationEndpoint: "https://example.com/webhook",
	}

	mockCreateOutput := &sns.CreateTopicOutput{
		TopicArn: aws.String("arn:aws:sns:us-east-1:123456789012:test-topic"),
	}

	mockSNSClient.EXPECT().
		CreateTopicWithContext(gomock.Any(), gomock.Any()).
		Return(mockCreateOutput, nil)

	mockSNSClient.EXPECT().
		SubscribeWithContext(gomock.Any(), gomock.Any()).
		Return(&sns.SubscribeOutput{}, nil)

	result, err := service.setupSNSTopic(context.Background(), config, topicConfig)

	assert.NoError(t, err)
	assert.Equal(t, *mockCreateOutput.TopicArn, result)
}

// Test SetupConfigurationSet - new configuration set
func TestSetupConfigurationSet_NewConfigSet(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()
	configSetName := "test-config-set"

	// Mock ListConfigurationSets to return empty list
	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{},
		}, nil)

	// Mock CreateConfigurationSet
	mockSESClient.EXPECT().
		CreateConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.CreateConfigurationSetOutput{}, nil)

	err := service.setupConfigurationSet(context.Background(), config, configSetName)

	assert.NoError(t, err)
}

// Test SetupConfigurationSet - existing configuration set
func TestSetupConfigurationSet_ExistingConfigSet(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()
	configSetName := "test-config-set"

	// Mock ListConfigurationSets to return existing config set
	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{
				{Name: aws.String(configSetName)},
			},
		}, nil)

	// CreateConfigurationSet should not be called

	err := service.setupConfigurationSet(context.Background(), config, configSetName)

	assert.NoError(t, err)
}

// Test SetupConfigurationSet - list error
func TestSetupConfigurationSet_ListError(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()
	configSetName := "test-config-set"

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("list error"))

	err := service.setupConfigurationSet(context.Background(), config, configSetName)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list configuration sets")
}

// Test SetupConfigurationSet - create error
func TestSetupConfigurationSet_CreateError(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()
	configSetName := "test-config-set"

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{},
		}, nil)

	mockSESClient.EXPECT().
		CreateConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("create error"))

	err := service.setupConfigurationSet(context.Background(), config, configSetName)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create configuration set")
}

// Test SetupEventDestination - new destination
func TestSetupEventDestination_NewDestination(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()

	eventDestination := domain.SESConfigurationSetEventDestination{
		ConfigurationSetName: "test-config-set",
		Name:                 "test-destination",
		Enabled:              true,
		MatchingEventTypes:   []string{"send", "bounce"},
		SNSDestination: &domain.SESTopicConfig{
			TopicARN: "arn:aws:sns:us-east-1:123456789012:test-topic",
		},
	}

	// Mock ListEventDestinations to return empty list
	mockSESClient.EXPECT().
		DescribeConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeConfigurationSetOutput{
			EventDestinations: []*ses.EventDestination{},
		}, nil)

	// Mock CreateEventDestination
	mockSESClient.EXPECT().
		CreateConfigurationSetEventDestinationWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.CreateConfigurationSetEventDestinationOutput{}, nil)

	err := service.setupEventDestination(context.Background(), config, eventDestination)

	assert.NoError(t, err)
}

// Test SetupEventDestination - existing destination
func TestSetupEventDestination_ExistingDestination(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()

	eventDestination := domain.SESConfigurationSetEventDestination{
		ConfigurationSetName: "test-config-set",
		Name:                 "test-destination",
		Enabled:              true,
		MatchingEventTypes:   []string{"send", "bounce"},
		SNSDestination: &domain.SESTopicConfig{
			TopicARN: "arn:aws:sns:us-east-1:123456789012:test-topic",
		},
	}

	// Mock ListEventDestinations to return existing destination
	mockSESClient.EXPECT().
		DescribeConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeConfigurationSetOutput{
			EventDestinations: []*ses.EventDestination{
				{
					Name:               aws.String("test-destination"),
					Enabled:            aws.Bool(true),
					MatchingEventTypes: aws.StringSlice([]string{"send"}),
					SNSDestination: &ses.SNSDestination{
						TopicARN: aws.String("arn:aws:sns:us-east-1:123456789012:test-topic"),
					},
				},
			},
		}, nil)

	// Mock UpdateEventDestination
	mockSESClient.EXPECT().
		UpdateConfigurationSetEventDestinationWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.UpdateConfigurationSetEventDestinationOutput{}, nil)

	err := service.setupEventDestination(context.Background(), config, eventDestination)

	assert.NoError(t, err)
}

// Test SetupEventDestination - list error
func TestSetupEventDestination_ListError(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	config := getValidSESConfig()

	eventDestination := domain.SESConfigurationSetEventDestination{
		ConfigurationSetName: "test-config-set",
		Name:                 "test-destination",
		Enabled:              true,
		MatchingEventTypes:   []string{"send", "bounce"},
		SNSDestination: &domain.SESTopicConfig{
			TopicARN: "arn:aws:sns:us-east-1:123456789012:test-topic",
		},
	}

	mockSESClient.EXPECT().
		DescribeConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("list error"))

	err := service.setupEventDestination(context.Background(), config, eventDestination)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list event destinations")
}

// Test RegisterWebhooks - success
func TestRegisterWebhooks_Success(t *testing.T) {
	service, mockSESClient, mockSNSClient, _, _, _ := createMockSESServiceWithV2(t)

	workspaceID := "test-workspace"
	integrationID := "test-integration"
	baseURL := "https://example.com"
	eventTypes := []domain.EmailEventType{domain.EmailEventDelivered, domain.EmailEventBounce}

	providerConfig := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	topicARN := "arn:aws:sns:us-east-1:123456789012:notifuse-ses-test-integration"

	// Mock SNS topic creation
	mockSNSClient.EXPECT().
		CreateTopicWithContext(gomock.Any(), gomock.Any()).
		Return(&sns.CreateTopicOutput{TopicArn: aws.String(topicARN)}, nil)

	mockSNSClient.EXPECT().
		SubscribeWithContext(gomock.Any(), gomock.Any()).
		Return(&sns.SubscribeOutput{}, nil)

	// Mock configuration set setup
	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{},
		}, nil)

	mockSESClient.EXPECT().
		CreateConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.CreateConfigurationSetOutput{}, nil)

	mockSESClient.EXPECT().
		DescribeConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeConfigurationSetOutput{
			EventDestinations: []*ses.EventDestination{},
		}, nil)

	mockSESClient.EXPECT().
		CreateConfigurationSetEventDestinationWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.CreateConfigurationSetEventDestinationOutput{}, nil)

	// Call the method being tested
	result, err := service.RegisterWebhooks(context.Background(), workspaceID, integrationID, baseURL, eventTypes, providerConfig)

	// Assert the results
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, domain.EmailProviderKindSES, result.EmailProviderKind)
	assert.True(t, result.IsRegistered)
	assert.Len(t, result.Endpoints, 2)
}

// Test RegisterWebhooks - invalid config
func TestRegisterWebhooks_InvalidConfig(t *testing.T) {
	service, _, _, _, _ := createMockSESService(t)

	workspaceID := "test-workspace"
	integrationID := "test-integration"

	// Test nil provider config
	result, err := service.RegisterWebhooks(context.Background(), workspaceID, integrationID, "", []domain.EmailEventType{}, nil)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidSESConfig, err)
	assert.Nil(t, result)

	// Test nil SES config
	providerConfig := &domain.EmailProvider{SES: nil}
	result, err = service.RegisterWebhooks(context.Background(), workspaceID, integrationID, "", []domain.EmailEventType{}, providerConfig)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidSESConfig, err)
	assert.Nil(t, result)
}

// Test RegisterWebhooks - SNS topic creation error
func TestRegisterWebhooks_SNSTopicError(t *testing.T) {
	service, _, mockSNSClient, _, _ := createMockSESService(t)

	workspaceID := "test-workspace"
	integrationID := "test-integration"
	baseURL := "https://example.com"
	eventTypes := []domain.EmailEventType{domain.EmailEventDelivered}

	providerConfig := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	mockSNSClient.EXPECT().
		CreateTopicWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("SNS error"))

	result, err := service.RegisterWebhooks(context.Background(), workspaceID, integrationID, baseURL, eventTypes, providerConfig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create SNS topic")
	assert.Nil(t, result)
}

// Test RegisterWebhooks - configuration set error
func TestRegisterWebhooks_ConfigSetError(t *testing.T) {
	service, mockSESClient, mockSNSClient, _, _, _ := createMockSESServiceWithV2(t)

	workspaceID := "test-workspace"
	integrationID := "test-integration"
	baseURL := "https://example.com"
	eventTypes := []domain.EmailEventType{domain.EmailEventDelivered}

	providerConfig := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	topicARN := "arn:aws:sns:us-east-1:123456789012:notifuse-ses-test-integration"

	mockSNSClient.EXPECT().
		CreateTopicWithContext(gomock.Any(), gomock.Any()).
		Return(&sns.CreateTopicOutput{TopicArn: aws.String(topicARN)}, nil)

	mockSNSClient.EXPECT().
		SubscribeWithContext(gomock.Any(), gomock.Any()).
		Return(&sns.SubscribeOutput{}, nil)

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("SES error"))

	result, err := service.RegisterWebhooks(context.Background(), workspaceID, integrationID, baseURL, eventTypes, providerConfig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list configuration sets")
	assert.Nil(t, result)
}

// Test RegisterWebhooks - event destination error
func TestRegisterWebhooks_EventDestinationError(t *testing.T) {
	service, mockSESClient, mockSNSClient, _, _, _ := createMockSESServiceWithV2(t)

	workspaceID := "test-workspace"
	integrationID := "test-integration"
	baseURL := "https://example.com"
	eventTypes := []domain.EmailEventType{domain.EmailEventDelivered}

	providerConfig := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	topicARN := "arn:aws:sns:us-east-1:123456789012:notifuse-ses-test-integration"

	mockSNSClient.EXPECT().
		CreateTopicWithContext(gomock.Any(), gomock.Any()).
		Return(&sns.CreateTopicOutput{TopicArn: aws.String(topicARN)}, nil)

	mockSNSClient.EXPECT().
		SubscribeWithContext(gomock.Any(), gomock.Any()).
		Return(&sns.SubscribeOutput{}, nil)

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{},
		}, nil)

	mockSESClient.EXPECT().
		CreateConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("SES error"))

	result, err := service.RegisterWebhooks(context.Background(), workspaceID, integrationID, baseURL, eventTypes, providerConfig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create configuration set")
	assert.Nil(t, result)
}

// Test GetWebhookStatus - success with registered webhooks
func TestGetWebhookStatus_RegisteredWebhooks(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	mockSESClient.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).Return(&ses.DescribeActiveReceiptRuleSetOutput{}, nil).AnyTimes() // Phase 3: inbound status check

	workspaceID := "test-workspace"
	integrationID := "test-integration"

	providerConfig := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	configSetName := fmt.Sprintf("notifuse-%s", integrationID)

	// Mock configuration set exists
	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{
				{Name: aws.String(configSetName)},
			},
		}, nil)

	// Mock event destinations
	mockSESClient.EXPECT().
		DescribeConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeConfigurationSetOutput{
			EventDestinations: []*ses.EventDestination{
				{
					Name:               aws.String("test-destination"),
					Enabled:            aws.Bool(true),
					MatchingEventTypes: aws.StringSlice([]string{"send", "bounce"}),
					SNSDestination: &ses.SNSDestination{
						TopicARN: aws.String("arn:aws:sns:us-east-1:123456789012:test-topic"),
					},
				},
			},
		}, nil)

	result, err := service.GetWebhookStatus(context.Background(), workspaceID, integrationID, providerConfig)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, domain.EmailProviderKindSES, result.EmailProviderKind)
	assert.True(t, result.IsRegistered)
	assert.Len(t, result.Endpoints, 3) // All event types are enabled by default
}

// Test GetWebhookStatus - not registered
func TestGetWebhookStatus_NotRegistered(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	mockSESClient.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).Return(&ses.DescribeActiveReceiptRuleSetOutput{}, nil).AnyTimes() // Phase 3: inbound status check

	workspaceID := "test-workspace"
	integrationID := "test-integration"

	providerConfig := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	// Mock configuration set does not exist
	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{},
		}, nil)

	result, err := service.GetWebhookStatus(context.Background(), workspaceID, integrationID, providerConfig)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, domain.EmailProviderKindSES, result.EmailProviderKind)
	assert.False(t, result.IsRegistered)
	assert.Len(t, result.Endpoints, 0)
}

// Test GetWebhookStatus - invalid config
func TestGetWebhookStatus_InvalidConfig(t *testing.T) {
	service, _, _, _, _ := createMockSESService(t)

	workspaceID := "test-workspace"
	integrationID := "test-integration"

	// Test nil provider config
	result, err := service.GetWebhookStatus(context.Background(), workspaceID, integrationID, nil)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidSESConfig, err)
	assert.Nil(t, result)

	// Test nil SES config
	providerConfig := &domain.EmailProvider{SES: nil}
	result, err = service.GetWebhookStatus(context.Background(), workspaceID, integrationID, providerConfig)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidSESConfig, err)
	assert.Nil(t, result)
}

// Test GetWebhookStatus - list configuration sets error
func TestGetWebhookStatus_ListConfigSetsError(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	mockSESClient.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).Return(&ses.DescribeActiveReceiptRuleSetOutput{}, nil).AnyTimes() // Phase 3: inbound status check

	workspaceID := "test-workspace"
	integrationID := "test-integration"

	providerConfig := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("AWS error"))

	result, err := service.GetWebhookStatus(context.Background(), workspaceID, integrationID, providerConfig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list configuration sets")
	assert.Nil(t, result)
}

// Test GetWebhookStatus - list event destinations error
func TestGetWebhookStatus_ListEventDestinationsError(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	mockSESClient.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).Return(&ses.DescribeActiveReceiptRuleSetOutput{}, nil).AnyTimes() // Phase 3: inbound status check

	workspaceID := "test-workspace"
	integrationID := "test-integration"

	providerConfig := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	configSetName := fmt.Sprintf("notifuse-%s", integrationID)

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{
				{Name: aws.String(configSetName)},
			},
		}, nil)

	mockSESClient.EXPECT().
		DescribeConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("AWS error"))

	result, err := service.GetWebhookStatus(context.Background(), workspaceID, integrationID, providerConfig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list event destinations")
	assert.Nil(t, result)
}

// Test UnregisterWebhooks - success
func TestUnregisterWebhooks_Success(t *testing.T) {
	service, mockSESClient, mockSNSClient, _, _, _ := createMockSESServiceWithV2(t)
	mockSESClient.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).Return(&ses.DescribeActiveReceiptRuleSetOutput{}, nil).AnyTimes() // Phase 3: inbound status check

	workspaceID := "test-workspace"
	integrationID := "test-integration"

	providerConfig := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	configSetName := fmt.Sprintf("notifuse-%s", integrationID)
	destinationName := fmt.Sprintf("notifuse-destination-%s", integrationID)
	topicARN := "arn:aws:sns:us-east-1:123456789012:test-topic"

	// Mock configuration set exists
	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{
				{Name: aws.String(configSetName)},
			},
		}, nil)

	// Mock event destinations
	mockSESClient.EXPECT().
		DescribeConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeConfigurationSetOutput{
			EventDestinations: []*ses.EventDestination{
				{
					Name:               aws.String(destinationName),
					Enabled:            aws.Bool(true),
					MatchingEventTypes: aws.StringSlice([]string{"send"}),
					SNSDestination: &ses.SNSDestination{
						TopicARN: aws.String(topicARN),
					},
				},
			},
		}, nil)

	// Mock delete event destination
	mockSESClient.EXPECT().
		DeleteConfigurationSetEventDestinationWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DeleteConfigurationSetEventDestinationOutput{}, nil)

	// Mock delete configuration set
	mockSESClient.EXPECT().
		DeleteConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DeleteConfigurationSetOutput{}, nil)

	// Mock delete SNS topic
	mockSNSClient.EXPECT().
		DeleteTopicWithContext(gomock.Any(), gomock.Any()).
		Return(&sns.DeleteTopicOutput{}, nil)

	err := service.UnregisterWebhooks(context.Background(), workspaceID, integrationID, providerConfig)

	assert.NoError(t, err)
}

// Test UnregisterWebhooks - configuration set does not exist
func TestUnregisterWebhooks_ConfigSetNotExists(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)
	mockSESClient.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).Return(&ses.DescribeActiveReceiptRuleSetOutput{}, nil).AnyTimes() // Phase 3: inbound status check

	workspaceID := "test-workspace"
	integrationID := "test-integration"

	providerConfig := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	// Mock configuration set does not exist
	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{},
		}, nil)

	err := service.UnregisterWebhooks(context.Background(), workspaceID, integrationID, providerConfig)

	assert.NoError(t, err) // Should not error when nothing to clean up
}

// Test UnregisterWebhooks - invalid config
func TestUnregisterWebhooks_InvalidConfig(t *testing.T) {
	service, _, _, _, _ := createMockSESService(t)

	workspaceID := "test-workspace"
	integrationID := "test-integration"

	// Test nil provider config
	err := service.UnregisterWebhooks(context.Background(), workspaceID, integrationID, nil)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidSESConfig, err)

	// Test nil SES config
	providerConfig := &domain.EmailProvider{SES: nil}
	err = service.UnregisterWebhooks(context.Background(), workspaceID, integrationID, providerConfig)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidSESConfig, err)
}

// Test UnregisterWebhooks - partial cleanup failure
func TestUnregisterWebhooks_PartialFailure(t *testing.T) {
	service, mockSESClient, mockSNSClient, _, _, _ := createMockSESServiceWithV2(t)
	mockSESClient.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).Return(&ses.DescribeActiveReceiptRuleSetOutput{}, nil).AnyTimes() // Phase 3: inbound status check

	workspaceID := "test-workspace"
	integrationID := "test-integration"

	providerConfig := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	configSetName := fmt.Sprintf("notifuse-%s", integrationID)
	destinationName := fmt.Sprintf("notifuse-destination-%s", integrationID)
	topicARN := "arn:aws:sns:us-east-1:123456789012:test-topic"

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{
				{Name: aws.String(configSetName)},
			},
		}, nil)

	mockSESClient.EXPECT().
		DescribeConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeConfigurationSetOutput{
			EventDestinations: []*ses.EventDestination{
				{
					Name:               aws.String(destinationName),
					Enabled:            aws.Bool(true),
					MatchingEventTypes: aws.StringSlice([]string{"send"}),
					SNSDestination: &ses.SNSDestination{
						TopicARN: aws.String(topicARN),
					},
				},
			},
		}, nil)

	// Mock delete event destination success
	mockSESClient.EXPECT().
		DeleteConfigurationSetEventDestinationWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DeleteConfigurationSetEventDestinationOutput{}, nil)

	// Mock delete configuration set success
	mockSESClient.EXPECT().
		DeleteConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DeleteConfigurationSetOutput{}, nil)

	// Mock delete SNS topic failure
	mockSNSClient.EXPECT().
		DeleteTopicWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("SNS delete failed"))

	err := service.UnregisterWebhooks(context.Background(), workspaceID, integrationID, providerConfig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete one or more AWS resources")
}

// Test SendEmail - success
func TestSendEmail_Success(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	workspaceID := "test-workspace"
	messageID := "test-message-id"
	fromAddress := "from@example.com"
	fromName := "Test Sender"
	to := "to@example.com"
	subject := "Test Subject"
	content := "<html><body>Test Content</body></html>"

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	// Mock configuration sets (empty list, so no config set will be used)
	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{},
		}, nil)

	// Mock send email
	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			assert.Nil(t, input.ConfigurationSetName)
			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   workspaceID,
		IntegrationID: "test-integration-id",
		MessageID:     messageID,
		FromAddress:   fromAddress,
		FromName:      fromName,
		To:            to,
		Subject:       subject,
		Content:       content,
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// TestSendEmail_CapturesMessageID verifies SES surfaces the API-returned MessageId via
// request.CapturedMessageID, so the worker can store the recipient-visible RFC Message-ID
// for stop-on-reply matching (SES overwrites any Message-ID we set).
func TestSendEmail_CapturesMessageID(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{AccessKey: "k", SecretKey: "s", Region: "us-east-1"},
	}
	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{ConfigurationSets: []*ses.ConfigurationSet{}}, nil)
	returnedID := "0000018f-ses-id"
	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		Return(&sesv2.SendEmailOutput{MessageId: &returnedID}, nil)

	captured := ""
	request := domain.SendEmailProviderRequest{
		WorkspaceID: "ws", IntegrationID: "int", MessageID: "local-msg", FromAddress: "h@x.com",
		FromName: "H", To: "jane@example.com", Subject: "Hi", Content: "<p>hi</p>",
		Provider: provider, CapturedMessageID: &captured,
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, returnedID, captured, "SES-returned MessageId must be captured")
	// Stored host-independent (the bare local part); matching strips the amazonses.com host
	// from a reply's In-Reply-To, so we don't depend on the exact host SES stamps.
	assert.Equal(t, returnedID, domain.SESStoredMessageID(captured))
}

// Test SendEmail - nil SES provider
func TestSendEmail_NilSESProvider(t *testing.T) {
	service, _, _, _, _ := createMockSESService(t)

	provider := &domain.EmailProvider{SES: nil}

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}
	err := service.SendEmail(context.Background(), request)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SES provider is not configured")
}

// Test SendEmail - invalid credentials
func TestSendEmail_InvalidCredentials(t *testing.T) {
	service, _, _, _, _ := createMockSESService(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "",
			SecretKey: "",
			Region:    "us-east-1",
		},
	}

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}
	err := service.SendEmail(context.Background(), request)

	assert.Error(t, err)
	assert.Equal(t, ErrInvalidAWSCredentials, err)
}

// Test SendEmail - AWS error
func TestSendEmail_AWSError(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("AWS send error"))

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}
	err := service.SendEmail(context.Background(), request)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send email")
}

// Test SendEmail - AWS API error surfaces as "SES error"
func TestSendEmail_AWSErrorWithAWSErr(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	awsErr := &smithy.GenericAPIError{Code: "MessageRejected", Message: "Email address not verified"}
	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		Return(nil, awsErr)

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}
	err := service.SendEmail(context.Background(), request)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SES error")
	assert.Contains(t, err.Error(), "MessageRejected")
}

// Test SendEmail - empty CC and BCC arrays
func TestSendEmail_EmptyCCBCC(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			assert.Nil(t, input.Destination.CcAddresses)
			assert.Nil(t, input.Destination.BccAddresses)
			assert.Nil(t, input.ReplyToAddresses)
			// Expect message ID tag
			assert.Len(t, input.EmailTags, 1)
			assert.Equal(t, "notifuse_message_id", *input.EmailTags[0].Name)
			assert.Equal(t, "test-message-id", *input.EmailTags[0].Value)
			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "test-message-id",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - CC and BCC with empty strings
func TestSendEmail_CCBCCWithEmptyStrings(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			// Should filter out empty strings
			assert.Nil(t, input.Destination.CcAddresses)
			assert.Nil(t, input.Destination.BccAddresses)
			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "test-message-id",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - list configuration sets error (should continue)
func TestSendEmail_ListConfigSetsError(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("list error"))

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			// Should not have configuration set name
			assert.Nil(t, input.ConfigurationSetName)
			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "test-message-id",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - session factory error
func TestSendEmail_DoesNotUseSessionFactory(t *testing.T) {
	// Sending moved to the SES v2 client, which is built directly from settings. A broken
	// session factory (v1, still used by webhook and inbound management) must not affect it.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockSESv2 := mocks.NewMockSESv2Client(ctrl)

	service := NewSESServiceWithClients(
		mockAuthService,
		mockLogger,
		func(config domain.AmazonSESSettings) (*session.Session, error) {
			t.Fatal("send path must not build an AWS session")
			return nil, errors.New("session creation failed")
		},
		nil, // sesClientFactory not used by the send path
		nil, // snsClientFactory not used by the send path
		func(_ domain.AmazonSESSettings) domain.SESv2Client { return mockSESv2 },
	)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey:               "test-access-key",
			SecretKey:               "test-secret-key",
			Region:                  "us-east-1",
			ManagedConfigurationSet: "notifuse-test-integration-id",
		},
	}

	mockSESv2.EXPECT().SendEmail(gomock.Any(), gomock.Any()).Return(&sesv2.SendEmailOutput{}, nil)

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}

	assert.NoError(t, service.SendEmail(context.Background(), request))
}

func TestSendEmail_WithCCAndBCC(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	cc := []string{"cc1@example.com", "cc2@example.com"}
	bcc := []string{"bcc1@example.com", "bcc2@example.com"}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			// Verify CC addresses
			assert.Len(t, input.Destination.CcAddresses, 2)
			assert.Equal(t, "cc1@example.com", input.Destination.CcAddresses[0])
			assert.Equal(t, "cc2@example.com", input.Destination.CcAddresses[1])

			// Verify BCC addresses
			assert.Len(t, input.Destination.BccAddresses, 2)
			assert.Equal(t, "bcc1@example.com", input.Destination.BccAddresses[0])
			assert.Equal(t, "bcc2@example.com", input.Destination.BccAddresses[1])

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{CC: cc, BCC: bcc},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - with ReplyTo
func TestSendEmail_WithReplyTo(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	replyTo := "reply@example.com"

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			// Verify ReplyTo address
			assert.Len(t, input.ReplyToAddresses, 1)
			assert.Equal(t, replyTo, input.ReplyToAddresses[0])

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{ReplyTo: replyTo},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - with configuration set found
func TestSendEmail_WithConfigurationSet(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	workspaceID := "test-workspace"
	integrationID := "test-integration-id"
	configSetName := fmt.Sprintf("notifuse-%s", integrationID)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	// Mock configuration sets with matching config set
	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{
				{Name: aws.String(configSetName)},
			},
		}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			// Verify configuration set is set
			assert.NotNil(t, input.ConfigurationSetName)
			assert.Equal(t, configSetName, *input.ConfigurationSetName)

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   workspaceID,
		IntegrationID: integrationID,
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - with message ID tag
func TestSendEmail_WithMessageIDTag(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	messageID := "test-message-123"

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			// Verify message ID tag
			assert.Len(t, input.EmailTags, 1)
			assert.Equal(t, "notifuse_message_id", *input.EmailTags[0].Name)
			assert.Equal(t, messageID, *input.EmailTags[0].Value)

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     messageID,
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - verify email structure
func TestSendEmail_VerifyEmailStructure(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	fromAddress := "from@example.com"
	fromName := "Test Sender"
	to := "to@example.com"
	subject := "Test Subject"
	content := "<html><body>Test Content</body></html>"

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			// Verify source format
			expectedSource := fmt.Sprintf("%s <%s>", fromName, fromAddress)
			assert.Equal(t, expectedSource, *input.FromEmailAddress)

			// Verify destination
			assert.Len(t, input.Destination.ToAddresses, 1)
			assert.Equal(t, to, input.Destination.ToAddresses[0])

			// Verify message structure
			assert.Equal(t, subject, *input.Content.Simple.Subject.Data)
			assert.Equal(t, "UTF-8", *input.Content.Simple.Subject.Charset)
			assert.Equal(t, content, *input.Content.Simple.Body.Html.Data)
			assert.Equal(t, "UTF-8", *input.Content.Simple.Body.Html.Charset)

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   fromAddress,
		FromName:      fromName,
		To:            to,
		Subject:       subject,
		Content:       content,
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - with single attachment
func TestSendEmail_WithSingleAttachment(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	// Create a simple text attachment (base64 encoded "Hello World")
	attachments := []domain.Attachment{
		{
			Filename:    "test.txt",
			Content:     "SGVsbG8gV29ybGQ=", // "Hello World" in base64
			ContentType: "text/plain",
			Disposition: "attachment",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			// Verify raw message is not nil
			assert.NotNil(t, input.Content.Raw)
			assert.NotNil(t, input.Content.Raw.Data)

			// Verify the raw message contains attachment references
			rawData := string(input.Content.Raw.Data)
			assert.Contains(t, rawData, "test.txt")
			assert.Contains(t, rawData, "text/plain")
			assert.Contains(t, rawData, "Content-Disposition: attachment")

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{Attachments: attachments},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - with multiple attachments
func TestSendEmail_WithMultipleAttachments(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	attachments := []domain.Attachment{
		{
			Filename:    "document.pdf",
			Content:     "JVBERi0xLjQKJeLjz9M=", // PDF header in base64
			ContentType: "application/pdf",
			Disposition: "attachment",
		},
		{
			Filename:    "image.png",
			Content:     "iVBORw0KGgo=", // PNG header in base64
			ContentType: "image/png",
			Disposition: "attachment",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			assert.NotNil(t, input.Content.Raw)
			rawData := string(input.Content.Raw.Data)

			// Verify both attachments are present
			assert.Contains(t, rawData, "document.pdf")
			assert.Contains(t, rawData, "application/pdf")
			assert.Contains(t, rawData, "image.png")
			assert.Contains(t, rawData, "image/png")

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "<html><body>Email with attachments</body></html>",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{Attachments: attachments},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - with inline attachment
func TestSendEmail_WithInlineAttachment(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	attachments := []domain.Attachment{
		{
			Filename:    "logo.png",
			Content:     "iVBORw0KGgo=",
			ContentType: "image/png",
			Disposition: "inline",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			assert.NotNil(t, input.Content.Raw)
			rawData := string(input.Content.Raw.Data)

			// Verify inline disposition and Content-ID (note: canonicalized as Content-Id)
			assert.Contains(t, rawData, "logo.png")
			assert.Contains(t, rawData, "Content-Disposition: inline")
			assert.Contains(t, rawData, "Content-Id: <logo.png>")

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "<html><body><img src=\"cid:logo.png\"/></body></html>",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{Attachments: attachments},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - inline attachment with an explicit content_id must be wrapped
// in a multipart/related subtree and carry the caller-provided Content-ID.
func TestSendEmail_InlineAttachment_MultipartRelatedAndContentID(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	attachments := []domain.Attachment{
		{
			Filename:    "check-in-qr.png",
			Content:     "iVBORw0KGgo=",
			ContentType: "image/png",
			Disposition: "inline",
			ContentID:   "checkInQr",
		},
		{
			Filename:    "receipt.pdf",
			Content:     "JVBERi0=",
			ContentType: "application/pdf",
			Disposition: "attachment",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			require.NotNil(t, input.Content.Raw)
			rawData := string(input.Content.Raw.Data)

			// Top-level is multipart/mixed (holds the regular attachment), and the
			// inline image lives inside a multipart/related subtree with the HTML.
			assert.Contains(t, rawData, "multipart/mixed")
			assert.Contains(t, rawData, "multipart/related")

			// The inline part uses the caller-provided content_id, not the filename.
			assert.Contains(t, rawData, "Content-Id: <checkInQr>")
			assert.Contains(t, rawData, "Content-Disposition: inline; filename=\"check-in-qr.png\"")

			// The regular attachment stays a normal attachment.
			assert.Contains(t, rawData, "Content-Disposition: attachment; filename=\"receipt.pdf\"")

			// The inline image's Content-ID must appear before the regular
			// attachment, proving it is nested in the related subtree.
			assert.Less(t, strings.Index(rawData, "checkInQr"), strings.Index(rawData, "receipt.pdf"))

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "<html><body><img src=\"cid:checkInQr\"/></body></html>",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{Attachments: attachments},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - with attachment without content type (auto-detect)
func TestSendEmail_WithAttachmentNoContentType(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	attachments := []domain.Attachment{
		{
			Filename:    "data.bin",
			Content:     "SGVsbG8=",
			ContentType: "", // Empty, should default to application/octet-stream
			Disposition: "attachment",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			assert.NotNil(t, input.Content.Raw)
			rawData := string(input.Content.Raw.Data)

			// Verify default content type is set
			assert.Contains(t, rawData, "application/octet-stream")
			assert.Contains(t, rawData, "data.bin")

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{Attachments: attachments},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - with attachment decode error
func TestSendEmail_WithAttachmentDecodeError(t *testing.T) {
	service, mockSESClient, _, _, _, _ := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	// Invalid base64 content
	attachments := []domain.Attachment{
		{
			Filename:    "test.txt",
			Content:     "not-valid-base64!!!",
			ContentType: "text/plain",
			Disposition: "attachment",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{Attachments: attachments},
	}
	err := service.SendEmail(context.Background(), request)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode content")
}

// Test SendEmail - with attachments and CC/BCC
func TestSendEmail_WithAttachmentsAndCCBCC(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	attachments := []domain.Attachment{
		{
			Filename:    "report.pdf",
			Content:     "JVBERi0xLjQ=",
			ContentType: "application/pdf",
			Disposition: "attachment",
		},
	}

	cc := []string{"cc@example.com"}
	bcc := []string{"bcc@example.com"}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			assert.NotNil(t, input.Content.Raw)
			rawData := string(input.Content.Raw.Data)

			// Verify CC is in headers (but not BCC for privacy)
			assert.Contains(t, rawData, "Cc: cc@example.com")
			assert.NotContains(t, rawData, "Bcc:")

			// Every recipient class must be in the envelope. CC used to be omitted here,
			// so a message with both CC and BCC never reached its CC recipients.
			assert.Equal(t, []string{"to@example.com"}, input.Destination.ToAddresses)
			assert.Equal(t, []string{"cc@example.com"}, input.Destination.CcAddresses)
			assert.Equal(t, []string{"bcc@example.com"}, input.Destination.BccAddresses)

			// Verify attachment
			assert.Contains(t, rawData, "report.pdf")

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions: domain.EmailOptions{
			CC:          cc,
			BCC:         bcc,
			Attachments: attachments,
		},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - with attachments and ReplyTo
func TestSendEmail_WithAttachmentsAndReplyTo(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	attachments := []domain.Attachment{
		{
			Filename:    "file.txt",
			Content:     "SGVsbG8=",
			ContentType: "text/plain",
			Disposition: "attachment",
		},
	}

	replyTo := "reply@example.com"

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			assert.NotNil(t, input.Content.Raw)
			rawData := string(input.Content.Raw.Data)

			// Verify Reply-To header
			assert.Contains(t, rawData, fmt.Sprintf("Reply-To: %s", replyTo))

			// Verify attachment
			assert.Contains(t, rawData, "file.txt")

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions: domain.EmailOptions{
			ReplyTo:     replyTo,
			Attachments: attachments,
		},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - with attachments and configuration set
func TestSendEmail_WithAttachmentsAndConfigSet(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	integrationID := "test-integration"
	configSetName := fmt.Sprintf("notifuse-%s", integrationID)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	attachments := []domain.Attachment{
		{
			Filename:    "doc.pdf",
			Content:     "JVBERi0=",
			ContentType: "application/pdf",
			Disposition: "attachment",
		},
	}

	// Mock configuration set exists
	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{
				{Name: aws.String(configSetName)},
			},
		}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			// Verify configuration set is included
			assert.NotNil(t, input.ConfigurationSetName)
			assert.Equal(t, configSetName, *input.ConfigurationSetName)

			// Verify attachment
			rawData := string(input.Content.Raw.Data)
			assert.Contains(t, rawData, "doc.pdf")

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: integrationID,
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{Attachments: attachments},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - raw email without configuration set should not include ConfigurationSetName
func TestSendEmail_RawEmail_NoConfigurationSet(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	attachments := []domain.Attachment{
		{
			Filename:    "doc.pdf",
			Content:     "JVBERi0=",
			ContentType: "application/pdf",
			Disposition: "attachment",
		},
	}

	// Mock configuration set does NOT exist (empty list)
	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{},
		}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			// Verify configuration set is NOT included (graceful degradation)
			assert.Nil(t, input.ConfigurationSetName, "ConfigurationSetName should be nil when config set doesn't exist")

			// Verify attachment is still present
			rawData := string(input.Content.Raw.Data)
			assert.Contains(t, rawData, "doc.pdf")

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{Attachments: attachments},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - with attachments, AWS SendRawEmail error
func TestSendEmail_WithAttachmentsAWSError(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	attachments := []domain.Attachment{
		{
			Filename:    "test.txt",
			Content:     "SGVsbG8=",
			ContentType: "text/plain",
			Disposition: "attachment",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	awsErr := &smithy.GenericAPIError{Code: "MessageRejected", Message: "Message too large"}
	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		Return(nil, awsErr)

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{Attachments: attachments},
	}
	err := service.SendEmail(context.Background(), request)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SES error")
	assert.Contains(t, err.Error(), "MessageRejected")
}

// Test SendEmail - with attachments, generic error
func TestSendEmail_WithAttachmentsGenericError(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	attachments := []domain.Attachment{
		{
			Filename:    "test.txt",
			Content:     "SGVsbG8=",
			ContentType: "text/plain",
			Disposition: "attachment",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("network error"))

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "Content",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{Attachments: attachments},
	}
	err := service.SendEmail(context.Background(), request)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send raw email")
}

// Test SendEmail - verify MIME structure with attachments
func TestSendEmail_VerifyMIMEStructureWithAttachments(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	attachments := []domain.Attachment{
		{
			Filename:    "test.txt",
			Content:     "SGVsbG8gV29ybGQ=", // "Hello World"
			ContentType: "text/plain",
			Disposition: "attachment",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			assert.NotNil(t, input.Content.Raw)
			rawData := string(input.Content.Raw.Data)

			// Verify MIME headers
			assert.Contains(t, rawData, "MIME-Version: 1.0")
			assert.Contains(t, rawData, "Content-Type: multipart/mixed")
			assert.Contains(t, rawData, "From: From <from@example.com>")
			assert.Contains(t, rawData, "To: to@example.com")
			assert.Contains(t, rawData, "Subject: Test Subject")
			assert.Contains(t, rawData, "X-Message-ID: test-message-id")

			// Verify HTML body part
			assert.Contains(t, rawData, "Content-Type: text/html; charset=UTF-8")
			assert.Contains(t, rawData, "<html><body>Test</body></html>")

			// Verify attachment part
			assert.Contains(t, rawData, "Content-Type: text/plain")
			assert.Contains(t, rawData, "Content-Transfer-Encoding: base64")
			assert.Contains(t, rawData, "Content-Disposition: attachment; filename=\"test.txt\"")

			// Verify tags are included in SendRawEmail
			assert.NotNil(t, input.EmailTags, "Tags should be set for SendRawEmail")
			assert.Len(t, input.EmailTags, 1)
			assert.Equal(t, "notifuse_message_id", *input.EmailTags[0].Name)
			assert.Equal(t, "test-message-id", *input.EmailTags[0].Value)

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "test-message-id",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Test Subject",
		Content:       "<html><body>Test</body></html>",
		Provider:      provider,
		EmailOptions:  domain.EmailOptions{Attachments: attachments},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - with List-Unsubscribe headers (RFC-8058)
func TestSendEmail_WithListUnsubscribeHeaders(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			assert.NotNil(t, input.Content.Raw)
			rawData := string(input.Content.Raw.Data)

			// Verify RFC-8058 List-Unsubscribe headers are present
			assert.Contains(t, rawData, "List-Unsubscribe: <https://example.com/unsubscribe/abc123>")
			assert.Contains(t, rawData, "List-Unsubscribe-Post: List-Unsubscribe=One-Click")

			// Verify tags are included in SendRawEmail
			assert.NotNil(t, input.EmailTags, "Tags should be set for SendRawEmail")
			assert.Len(t, input.EmailTags, 1)
			assert.Equal(t, "notifuse_message_id", *input.EmailTags[0].Name)
			assert.Equal(t, "test-message-id", *input.EmailTags[0].Value)

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "test-message-id",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "<html><body>Content</body></html>",
		Provider:      provider,
		EmailOptions: domain.EmailOptions{
			ListUnsubscribeURL: "https://example.com/unsubscribe/abc123",
		},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Test SendEmail - with List-Unsubscribe headers and attachments
func TestSendEmail_WithListUnsubscribeAndAttachments(t *testing.T) {
	service, mockSESClient, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	attachments := []domain.Attachment{
		{
			Filename:    "test.txt",
			Content:     "SGVsbG8gV29ybGQ=", // "Hello World" in base64
			ContentType: "text/plain",
			Disposition: "attachment",
		},
	}

	mockSESClient.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			assert.NotNil(t, input.Content.Raw)
			rawData := string(input.Content.Raw.Data)

			// Verify RFC-8058 List-Unsubscribe headers are present
			assert.Contains(t, rawData, "List-Unsubscribe: <https://example.com/unsubscribe/xyz789>")
			assert.Contains(t, rawData, "List-Unsubscribe-Post: List-Unsubscribe=One-Click")

			// Verify attachment is also present
			assert.Contains(t, rawData, "test.txt")
			assert.Contains(t, rawData, "Content-Disposition: attachment")

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "<html><body>Content</body></html>",
		Provider:      provider,
		EmailOptions: domain.EmailOptions{
			Attachments:        attachments,
			ListUnsubscribeURL: "https://example.com/unsubscribe/xyz789",
		},
	}
	err := service.SendEmail(context.Background(), request)

	assert.NoError(t, err)
}

// Tests for RFC 2047 encoding helper functions

func TestIsASCII(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"empty string", "", true},
		{"ASCII only", "Hello World", true},
		{"ASCII with numbers", "test123", true},
		{"ASCII with special chars", "test@example.com", true},
		{"Spanish characters", "José", false},
		{"German characters", "München", false},
		{"Japanese characters", "田中太郎", false},
		{"Mixed ASCII and non-ASCII", "Hello José", false},
		{"Emoji", "Hello 👋", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isASCII(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestEncodeRFC2047(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		shouldBeASCII  bool
		expectedPrefix string
	}{
		{"empty string", "", true, ""},
		{"ASCII only", "Hello World", true, "Hello World"},
		{"ASCII email name", "John Doe", true, "John Doe"},
		{"Spanish name", "José García", false, "=?UTF-8?b?"},
		{"German name", "Müller", false, "=?UTF-8?b?"},
		{"Japanese name", "田中太郎", false, "=?UTF-8?b?"},
		{"French name", "François", false, "=?UTF-8?b?"},
		{"Russian name", "Иван Петров", false, "=?UTF-8?b?"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := encodeRFC2047(tc.input)
			if tc.shouldBeASCII {
				assert.Equal(t, tc.input, result, "ASCII strings should remain unchanged")
			} else {
				assert.True(t, len(result) > 0, "Result should not be empty")
				assert.Contains(t, result, tc.expectedPrefix, "Non-ASCII should be RFC 2047 encoded")
				assert.Contains(t, result, "?=", "RFC 2047 encoded strings should end with ?=")
			}
		})
	}
}

func TestEncodeEmailAddress(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{"regular ASCII email", "user@example.com", "user@example.com", false},
		{"email with subdomain", "user@mail.example.com", "user@mail.example.com", false},
		{"international domain German", "user@münchen.de", "user@xn--mnchen-3ya.de", false},
		{"international domain Japanese", "user@日本.jp", "user@xn--wgv71a.jp", false},
		{"international domain Chinese", "user@中国.cn", "user@xn--fiqs8s.cn", false},
		{"email without @", "invalid-email", "invalid-email", false}, // Returns as-is
		{"email with plus", "user+tag@example.com", "user+tag@example.com", false},
		{"email with dots in local", "first.last@example.com", "first.last@example.com", false},
		// Non-ASCII local parts - RFC 2047 B encoding (Go's mime package uses lowercase 'b')
		{"non-ASCII local Spanish", "Jesús.dan@gmail.com", "=?UTF-8?b?SmVzw7pzLmRhbg==?=@gmail.com", false},
		{"non-ASCII local Spanish ñ", "Añejandramendo@gmail.com", "=?UTF-8?b?QcOxZWphbmRyYW1lbmRv?=@gmail.com", false},
		{"non-ASCII local German", "müller@example.com", "=?UTF-8?b?bcO8bGxlcg==?=@example.com", false},
		// Both non-ASCII local and international domain
		{"non-ASCII local and domain", "Jesús@münchen.de", "=?UTF-8?b?SmVzw7pz?=@xn--mnchen-3ya.de", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := encodeEmailAddress(tc.input)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestFormatFromHeader(t *testing.T) {
	tests := []struct {
		name        string
		fromName    string
		fromAddress string
		expected    string
		expectError bool
	}{
		{
			name:        "ASCII name and address",
			fromName:    "John Doe",
			fromAddress: "john@example.com",
			expected:    "John Doe <john@example.com>",
			expectError: false,
		},
		{
			name:        "empty name",
			fromName:    "",
			fromAddress: "john@example.com",
			expected:    "john@example.com",
			expectError: false,
		},
		{
			name:        "non-ASCII name",
			fromName:    "José García",
			fromAddress: "jose@example.com",
			expected:    "", // Will verify contains encoded name
			expectError: false,
		},
		{
			name:        "international domain",
			fromName:    "User",
			fromAddress: "user@münchen.de",
			expected:    "User <user@xn--mnchen-3ya.de>",
			expectError: false,
		},
		{
			name:        "non-ASCII name with international domain",
			fromName:    "José García",
			fromAddress: "jose@münchen.de",
			expected:    "", // Will verify contains encoded name and punycode domain
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := formatFromHeader(tc.fromName, tc.fromAddress)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tc.expected != "" {
					assert.Equal(t, tc.expected, result)
				} else {
					// For non-ASCII names, verify encoding
					if tc.fromName == "José García" {
						assert.Contains(t, result, "=?UTF-8?b?")
						assert.Contains(t, result, "?=")
					}
					// For international domains, verify punycode
					if tc.fromAddress == "user@münchen.de" || tc.fromAddress == "jose@münchen.de" {
						assert.Contains(t, result, "xn--mnchen-3ya.de")
					}
				}
			}
		})
	}
}

func TestSendEmail_WithNonASCIIFromName(t *testing.T) {
	service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	// Expect ListConfigurationSets call
	mockSES.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{},
		}, nil)

	// Capture and verify the SendEmail input
	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			// Verify From header is RFC 2047 encoded
			source := *input.FromEmailAddress
			assert.Contains(t, source, "=?UTF-8?b?")
			assert.Contains(t, source, "?=")
			assert.Contains(t, source, "<jose@example.com>")

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "jose@example.com",
		FromName:      "José García", // Non-ASCII name
		To:            "recipient@example.com",
		Subject:       "Test Subject",
		Content:       "<html><body>Test</body></html>",
		Provider:      &provider,
	}

	err := service.SendEmail(context.Background(), request)
	assert.NoError(t, err)
}

func TestSendEmail_WithInternationalDomain(t *testing.T) {
	service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	// Expect ListConfigurationSets call
	mockSES.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{},
		}, nil)

	// Capture and verify the SendEmail input
	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			// Verify To address has Punycode domain
			toAddress := input.Destination.ToAddresses[0]
			assert.Equal(t, "user@xn--mnchen-3ya.de", toAddress)

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "Sender",
		To:            "user@münchen.de", // International domain
		Subject:       "Test Subject",
		Content:       "<html><body>Test</body></html>",
		Provider:      &provider,
	}

	err := service.SendEmail(context.Background(), request)
	assert.NoError(t, err)
}

func TestSendRawEmail_WithNonASCIISubject(t *testing.T) {
	service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	// Expect ListConfigurationSets call
	mockSES.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{},
		}, nil)

	// Capture and verify the SendRawEmail input
	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			rawData := string(input.Content.Raw.Data)

			// Verify Subject is RFC 2047 encoded (Go's mime package uses lowercase 'b')
			assert.Contains(t, rawData, "Subject: =?UTF-8?b?")

			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "message",
		FromAddress:   "from@example.com",
		FromName:      "Sender",
		To:            "to@example.com",
		Subject:       "Asunto en español: ¡Hola!", // Non-ASCII subject
		Content:       "<html><body>Test</body></html>",
		Provider:      &provider,
		EmailOptions: domain.EmailOptions{
			ListUnsubscribeURL: "https://example.com/unsubscribe", // Forces raw email
		},
	}

	err := service.SendEmail(context.Background(), request)
	assert.NoError(t, err)
}

// TestSendEmail_QuotedPrintableEncoding verifies that HTML content is properly
// quoted-printable encoded when using SendRawEmail (Issue #230)
func TestSendEmail_QuotedPrintableEncoding(t *testing.T) {
	testCases := []struct {
		name            string
		htmlContent     string
		expectedEncoded string // substring that should appear in QP-encoded output
		description     string
	}{
		{
			name:            "equals sign encoding",
			htmlContent:     `<html><body>a=b test</body></html>`,
			expectedEncoded: "=3D", // '=' becomes '=3D' in QP
			description:     "equals sign must be encoded as =3D",
		},
		{
			name:            "long line soft wrapping",
			htmlContent:     `<html><body>` + strings.Repeat("x", 100) + `</body></html>`,
			expectedEncoded: "=\r\n", // soft line break
			description:     "lines > 76 chars should have soft line breaks",
		},
		{
			name:            "unicode content",
			htmlContent:     `<html><body>Héllo Wörld</body></html>`,
			expectedEncoded: "=C3=A9", // é encoded as UTF-8 bytes C3 A9
			description:     "non-ASCII characters must be QP encoded",
		},
		{
			name:            "HTML attributes with equals",
			htmlContent:     `<html><body style="background-color=#ffffff;">Test</body></html>`,
			expectedEncoded: "=3D#ffffff",
			description:     "HTML attributes with = must be encoded",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

			provider := domain.EmailProvider{
				SES: &domain.AmazonSESSettings{
					AccessKey: "test-access-key",
					SecretKey: "test-secret-key",
					Region:    "us-east-1",
				},
			}

			mockSES.EXPECT().
				ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
				Return(&ses.ListConfigurationSetsOutput{}, nil)

			mockSESv2.EXPECT().
				SendEmail(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
					assert.NotNil(t, input.Content.Raw)
					rawData := string(input.Content.Raw.Data)

					// Verify Content-Transfer-Encoding header is present
					assert.Contains(t, rawData, "Content-Transfer-Encoding: quoted-printable",
						"HTML part must have quoted-printable encoding header")

					// Verify the content is actually QP encoded
					assert.Contains(t, rawData, tc.expectedEncoded, tc.description)

					return &sesv2.SendEmailOutput{}, nil
				})

			request := domain.SendEmailProviderRequest{
				WorkspaceID:   "workspace",
				IntegrationID: "test-integration-id",
				MessageID:     "test-qp-message",
				FromAddress:   "from@example.com",
				FromName:      "From",
				To:            "to@example.com",
				Subject:       "Test QP Encoding",
				Content:       tc.htmlContent,
				Provider:      &provider,
				EmailOptions: domain.EmailOptions{
					ListUnsubscribeURL: "https://example.com/unsubscribe/test", // Forces raw email path
				},
			}
			err := service.SendEmail(context.Background(), request)
			assert.NoError(t, err)
		})
	}
}

// TestSendEmail_QuotedPrintableRoundTrip verifies that QP-encoded content
// can be decoded back to the original content (Issue #230)
func TestSendEmail_QuotedPrintableRoundTrip(t *testing.T) {
	service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	provider := domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	originalContent := `<html><body style="color=#333;">Price: $100 = €90</body></html>`
	var capturedRawData string

	mockSES.EXPECT().
		ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{}, nil)

	mockSESv2.EXPECT().
		SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			capturedRawData = string(input.Content.Raw.Data)
			return &sesv2.SendEmailOutput{}, nil
		})

	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace",
		IntegrationID: "test-integration-id",
		MessageID:     "test-roundtrip",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Round Trip Test",
		Content:       originalContent,
		Provider:      &provider,
		EmailOptions: domain.EmailOptions{
			ListUnsubscribeURL: "https://example.com/unsubscribe/test",
		},
	}
	err := service.SendEmail(context.Background(), request)
	assert.NoError(t, err)

	// Extract the HTML part from the MIME message and decode it
	// Find the QP encoding header
	qpHeaderIndex := strings.Index(capturedRawData, "Content-Transfer-Encoding: quoted-printable")
	assert.Greater(t, qpHeaderIndex, 0, "Should find QP encoding header")

	// Find the blank line after headers (start of body)
	bodyStartOffset := strings.Index(capturedRawData[qpHeaderIndex:], "\r\n\r\n")
	assert.Greater(t, bodyStartOffset, 0, "Should find blank line after headers")

	// Calculate actual content start position
	contentStart := qpHeaderIndex + bodyStartOffset + 4 // +4 for \r\n\r\n

	// Find the next boundary (end of HTML part)
	boundaryIndex := strings.Index(capturedRawData[contentStart:], "\r\n--")

	var encodedContent string
	if boundaryIndex > 0 {
		encodedContent = capturedRawData[contentStart : contentStart+boundaryIndex]
	} else {
		encodedContent = capturedRawData[contentStart:]
	}

	// Decode the QP content
	reader := quotedprintable.NewReader(strings.NewReader(encodedContent))
	decodedBytes, err := io.ReadAll(reader)
	assert.NoError(t, err, "QP decoding should succeed")

	decodedContent := string(decodedBytes)
	assert.Equal(t, originalContent, decodedContent, "Decoded content should match original")
}
