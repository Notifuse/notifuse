package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// Helper function to create a mock SES service for testing
func createMockSESService(t *testing.T) (*SESService, *mocks.MockSESClient, *mocks.MockSNSClient, *mocks.MockAuthService, *pkgmocks.MockLogger) {
	ctrl := gomock.NewController(t)
	mockSES := mocks.NewMockSESClient(ctrl)
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
		func(_ *session.Session) domain.SESClient {
			return mockSES
		},
	), mockSES, mockSNS, mockAuth, mockLogger
}

func createMockSESServiceWithSessionError(t *testing.T) (*SESService, *mocks.MockSESClient, *mocks.MockSNSClient, *mocks.MockAuthService, *pkgmocks.MockLogger) {
	ctrl := gomock.NewController(t)
	mockSES := mocks.NewMockSESClient(ctrl)
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
		func(_ *session.Session) domain.SESClient {
			return mockSES
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
	assert.NotNil(t, service.sesEmailClientFactory)

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

	sesEmailClient := service.sesEmailClientFactory(session)
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
	sesEmailClientFactory := func(_ *session.Session) domain.SESClient {
		return nil
	}

	service := NewSESServiceWithClients(mockAuth, mockLogger, sessionFactory, sesClientFactory, snsClientFactory, sesEmailClientFactory)

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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, _, _, _ := createMockSESService(t)
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
	service, mockSESClient, mockSNSClient, _, _ := createMockSESService(t)

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
	service, mockSESClient, mockSNSClient, _, _ := createMockSESService(t)

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
	service, mockSESClient, mockSNSClient, _, _ := createMockSESService(t)

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
	service, mockSESClient, _, _, _ := createMockSESService(t)

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
	service, mockSESClient, _, _, _ := createMockSESService(t)

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
	service, mockSESClient, _, _, _ := createMockSESService(t)

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
	service, mockSESClient, _, _, _ := createMockSESService(t)

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
	service, mockSESClient, mockSNSClient, _, _ := createMockSESService(t)

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
	service, mockSESClient, _, _, _ := createMockSESService(t)

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
	service, mockSESClient, mockSNSClient, _, _ := createMockSESService(t)

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
	service, mockSESClient, _, _, _ := createMockSESService(t)

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
	mockSESClient.EXPECT().
		SendEmailWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *ses.SendEmailInput, _ ...request.Option) (*ses.SendEmailOutput, error) {
			assert.Nil(t, input.ConfigurationSetName)
			return &ses.SendEmailOutput{}, nil
		})

	err := service.SendEmail(context.Background(), workspaceID, messageID, fromAddress, fromName, to, subject, content, provider, domain.EmailOptions{})

	assert.NoError(t, err)
}

// Test SendEmail - nil SES provider
func TestSendEmail_NilSESProvider(t *testing.T) {
	service, _, _, _, _ := createMockSESService(t)

	provider := &domain.EmailProvider{SES: nil}

	err := service.SendEmail(context.Background(), "workspace", "message", "from@example.com", "From", "to@example.com", "Subject", "Content", provider, domain.EmailOptions{})

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

	err := service.SendEmail(context.Background(), "workspace", "message", "from@example.com", "From", "to@example.com", "Subject", "Content", provider, domain.EmailOptions{})

	assert.Error(t, err)
	assert.Equal(t, ErrInvalidAWSCredentials, err)
}

// Test SendEmail - AWS error
func TestSendEmail_AWSError(t *testing.T) {
	service, mockSESClient, _, _, _ := createMockSESService(t)

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

	mockSESClient.EXPECT().
		SendEmailWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("AWS send error"))

	err := service.SendEmail(context.Background(), "workspace", "message", "from@example.com", "From", "to@example.com", "Subject", "Content", provider, domain.EmailOptions{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send email")
}

// Test SendEmail - AWS error with awserr
func TestSendEmail_AWSErrorWithAWSErr(t *testing.T) {
	service, mockSESClient, _, _, _ := createMockSESService(t)

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

	awsErr := awserr.New("MessageRejected", "Email address not verified", nil)
	mockSESClient.EXPECT().
		SendEmailWithContext(gomock.Any(), gomock.Any()).
		Return(nil, awsErr)

	err := service.SendEmail(context.Background(), "workspace", "message", "from@example.com", "From", "to@example.com", "Subject", "Content", provider, domain.EmailOptions{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SES error")
	assert.Contains(t, err.Error(), "MessageRejected")
}

// Test SendEmail - empty CC and BCC arrays
func TestSendEmail_EmptyCCBCC(t *testing.T) {
	service, mockSESClient, _, _, _ := createMockSESService(t)

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

	mockSESClient.EXPECT().
		SendEmailWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *ses.SendEmailInput, _ ...request.Option) (*ses.SendEmailOutput, error) {
			assert.Nil(t, input.Destination.CcAddresses)
			assert.Nil(t, input.Destination.BccAddresses)
			assert.Nil(t, input.ReplyToAddresses)
			assert.Nil(t, input.Tags) // No message ID provided
			return &ses.SendEmailOutput{}, nil
		})

	err := service.SendEmail(context.Background(), "workspace", "", "from@example.com", "From", "to@example.com", "Subject", "Content", provider, domain.EmailOptions{})

	assert.NoError(t, err)
}

// Test SendEmail - CC and BCC with empty strings
func TestSendEmail_CCBCCWithEmptyStrings(t *testing.T) {
	service, mockSESClient, _, _, _ := createMockSESService(t)

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

	mockSESClient.EXPECT().
		SendEmailWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *ses.SendEmailInput, _ ...request.Option) (*ses.SendEmailOutput, error) {
			// Should filter out empty strings
			assert.Nil(t, input.Destination.CcAddresses)
			assert.Nil(t, input.Destination.BccAddresses)
			return &ses.SendEmailOutput{}, nil
		})

	err := service.SendEmail(context.Background(), "workspace", "", "from@example.com", "From", "to@example.com", "Subject", "Content", provider, domain.EmailOptions{})

	assert.NoError(t, err)
}

// Test SendEmail - list configuration sets error (should continue)
func TestSendEmail_ListConfigSetsError(t *testing.T) {
	service, mockSESClient, _, _, _ := createMockSESService(t)

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

	mockSESClient.EXPECT().
		SendEmailWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *ses.SendEmailInput, _ ...request.Option) (*ses.SendEmailOutput, error) {
			// Should not have configuration set name
			assert.Nil(t, input.ConfigurationSetName)
			return &ses.SendEmailOutput{}, nil
		})

	err := service.SendEmail(context.Background(), "workspace", "", "from@example.com", "From", "to@example.com", "Subject", "Content", provider, domain.EmailOptions{})

	assert.NoError(t, err)
}

// Test SendEmail - session factory error
func TestSendEmail_SessionFactoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Create service with failing session factory
	service := NewSESServiceWithClients(
		mockAuthService,
		mockLogger,
		func(config domain.AmazonSESSettings) (*session.Session, error) {
			return nil, errors.New("session creation failed")
		},
		nil, // sesClientFactory not used in this test
		nil, // snsClientFactory not used in this test
		nil, // sesEmailClientFactory not used in this test
	)

	provider := &domain.EmailProvider{
		SES: &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		},
	}

	// Expect logger to be called for the error
	mockLogger.EXPECT().Error(gomock.Any()).Times(1)

	err := service.SendEmail(context.Background(), "workspace", "message", "from@example.com", "From", "to@example.com", "Subject", "Content", provider, domain.EmailOptions{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create AWS session")
}

// Test SendEmail - with CC and BCC addresses
func TestSendEmail_WithCCAndBCC(t *testing.T) {
	service, mockSESClient, _, _, _ := createMockSESService(t)

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

	mockSESClient.EXPECT().
		SendEmailWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *ses.SendEmailInput, _ ...request.Option) (*ses.SendEmailOutput, error) {
			// Verify CC addresses
			assert.Len(t, input.Destination.CcAddresses, 2)
			assert.Equal(t, "cc1@example.com", *input.Destination.CcAddresses[0])
			assert.Equal(t, "cc2@example.com", *input.Destination.CcAddresses[1])

			// Verify BCC addresses
			assert.Len(t, input.Destination.BccAddresses, 2)
			assert.Equal(t, "bcc1@example.com", *input.Destination.BccAddresses[0])
			assert.Equal(t, "bcc2@example.com", *input.Destination.BccAddresses[1])

			return &ses.SendEmailOutput{}, nil
		})

	err := service.SendEmail(context.Background(), "workspace", "message", "from@example.com", "From", "to@example.com", "Subject", "Content", provider, domain.EmailOptions{CC: cc, BCC: bcc})

	assert.NoError(t, err)
}

// Test SendEmail - with ReplyTo
func TestSendEmail_WithReplyTo(t *testing.T) {
	service, mockSESClient, _, _, _ := createMockSESService(t)

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

	mockSESClient.EXPECT().
		SendEmailWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *ses.SendEmailInput, _ ...request.Option) (*ses.SendEmailOutput, error) {
			// Verify ReplyTo address
			assert.Len(t, input.ReplyToAddresses, 1)
			assert.Equal(t, replyTo, *input.ReplyToAddresses[0])

			return &ses.SendEmailOutput{}, nil
		})

	err := service.SendEmail(context.Background(), "workspace", "message", "from@example.com", "From", "to@example.com", "Subject", "Content", provider, domain.EmailOptions{ReplyTo: replyTo})

	assert.NoError(t, err)
}

// Test SendEmail - with configuration set found
func TestSendEmail_WithConfigurationSet(t *testing.T) {
	service, mockSESClient, _, _, _ := createMockSESService(t)

	workspaceID := "test-workspace"
	configSetName := fmt.Sprintf("notifuse-%s", workspaceID)

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

	mockSESClient.EXPECT().
		SendEmailWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *ses.SendEmailInput, _ ...request.Option) (*ses.SendEmailOutput, error) {
			// Verify configuration set is set
			assert.NotNil(t, input.ConfigurationSetName)
			assert.Equal(t, configSetName, *input.ConfigurationSetName)

			return &ses.SendEmailOutput{}, nil
		})

	err := service.SendEmail(context.Background(), workspaceID, "message", "from@example.com", "From", "to@example.com", "Subject", "Content", provider, domain.EmailOptions{})

	assert.NoError(t, err)
}

// Test SendEmail - with message ID tag
func TestSendEmail_WithMessageIDTag(t *testing.T) {
	service, mockSESClient, _, _, _ := createMockSESService(t)

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

	mockSESClient.EXPECT().
		SendEmailWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *ses.SendEmailInput, _ ...request.Option) (*ses.SendEmailOutput, error) {
			// Verify message ID tag
			assert.Len(t, input.Tags, 1)
			assert.Equal(t, "notifuse_message_id", *input.Tags[0].Name)
			assert.Equal(t, messageID, *input.Tags[0].Value)

			return &ses.SendEmailOutput{}, nil
		})

	err := service.SendEmail(context.Background(), "workspace", messageID, "from@example.com", "From", "to@example.com", "Subject", "Content", provider, domain.EmailOptions{})

	assert.NoError(t, err)
}

// Test SendEmail - verify email structure
func TestSendEmail_VerifyEmailStructure(t *testing.T) {
	service, mockSESClient, _, _, _ := createMockSESService(t)

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

	mockSESClient.EXPECT().
		SendEmailWithContext(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *ses.SendEmailInput, _ ...request.Option) (*ses.SendEmailOutput, error) {
			// Verify source format
			expectedSource := fmt.Sprintf("%s <%s>", fromName, fromAddress)
			assert.Equal(t, expectedSource, *input.Source)

			// Verify destination
			assert.Len(t, input.Destination.ToAddresses, 1)
			assert.Equal(t, to, *input.Destination.ToAddresses[0])

			// Verify message structure
			assert.Equal(t, subject, *input.Message.Subject.Data)
			assert.Equal(t, "UTF-8", *input.Message.Subject.Charset)
			assert.Equal(t, content, *input.Message.Body.Html.Data)
			assert.Equal(t, "UTF-8", *input.Message.Body.Html.Charset)

			return &ses.SendEmailOutput{}, nil
		})

	err := service.SendEmail(context.Background(), "workspace", "message", fromAddress, fromName, to, subject, content, provider, domain.EmailOptions{})

	assert.NoError(t, err)
}
