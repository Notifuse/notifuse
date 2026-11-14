package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSESProvider stands for the SES service: a webhook provider that also re-attaches
// tenants, registers the inbound route and tears down its sending resources. It records
// the calls it receives, in order.
type fakeSESProvider struct {
	status        *domain.WebhookRegistrationStatus
	inboundARN    string // set on the provider config by EnsureInboundRoute, as SES does
	associateErr  error
	unregisterErr error
	teardownErr   error

	calls        []string
	associatedTo *domain.AmazonSESSettings
	senders      []domain.EmailSender
}

func (f *fakeSESProvider) RegisterWebhooks(_ context.Context, _, _ string, _ string, _ []domain.EmailEventType, _ *domain.EmailProvider) (*domain.WebhookRegistrationStatus, error) {
	f.calls = append(f.calls, "register")
	return f.status, nil
}

func (f *fakeSESProvider) GetWebhookStatus(_ context.Context, _, _ string, _ *domain.EmailProvider) (*domain.WebhookRegistrationStatus, error) {
	return f.status, nil
}

func (f *fakeSESProvider) UnregisterWebhooks(_ context.Context, _, _ string, _ *domain.EmailProvider) error {
	f.calls = append(f.calls, "unregister")
	return f.unregisterErr
}

func (f *fakeSESProvider) AssociateExistingTenant(_ context.Context, config domain.AmazonSESSettings, _ string, senders []domain.EmailSender) (*domain.SESTenantProvisionResult, error) {
	f.calls = append(f.calls, "associate")
	f.associatedTo = &config
	f.senders = senders
	return nil, f.associateErr
}

func (f *fakeSESProvider) EnsureInboundRoute(_ context.Context, provider *domain.EmailProvider, _ string) error {
	f.calls = append(f.calls, "inbound")
	if f.inboundARN != "" && provider.SES != nil {
		provider.SES.InboundTopicARN = f.inboundARN
	}
	return nil
}

func (f *fakeSESProvider) DeleteSendingResources(_ context.Context, _, _ string, _ *domain.EmailProvider) error {
	f.calls = append(f.calls, "teardown")
	return f.teardownErr
}

// webhooksOnlyProvider registers webhooks and owns nothing else, and counts the
// unregistrations it receives.
type webhooksOnlyProvider struct {
	unregistered int
}

func (p *webhooksOnlyProvider) RegisterWebhooks(_ context.Context, _, _ string, _ string, _ []domain.EmailEventType, _ *domain.EmailProvider) (*domain.WebhookRegistrationStatus, error) {
	return &domain.WebhookRegistrationStatus{}, nil
}

func (p *webhooksOnlyProvider) GetWebhookStatus(_ context.Context, _, _ string, _ *domain.EmailProvider) (*domain.WebhookRegistrationStatus, error) {
	return &domain.WebhookRegistrationStatus{}, nil
}

func (p *webhooksOnlyProvider) UnregisterWebhooks(_ context.Context, _, _ string, _ *domain.EmailProvider) error {
	p.unregistered++
	return nil
}

type sesRegistrationFixture struct {
	svc           *WebhookRegistrationService
	workspaceRepo *mocks.MockWorkspaceRepository
	provider      *fakeSESProvider
}

// sesIntegrationWorkspace returns a new workspace holding one SES integration. A test
// whose reads must not share it returns a fresh one per read (DoAndReturn).
func sesIntegrationWorkspace(ses domain.AmazonSESSettings) *domain.Workspace {
	return &domain.Workspace{ID: "ws-1", Integrations: domain.Integrations{{
		ID: "int-1",
		EmailProvider: domain.EmailProvider{
			Kind:    domain.EmailProviderKindSES,
			SES:     &ses,
			Senders: []domain.EmailSender{{ID: "sender-1", Email: "news@example.com"}},
		},
	}}}
}

func newSESRegistrationFixture(t *testing.T, provider *fakeSESProvider, authenticate bool) *sesRegistrationFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	f := &sesRegistrationFixture{workspaceRepo: mocks.NewMockWorkspaceRepository(ctrl), provider: provider}
	authService := mocks.NewMockAuthService(ctrl)
	if authenticate {
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
			Return(context.Background(), &domain.User{ID: "u"}, ownerOf("ws-1", "u"), nil)
	}
	f.svc = &WebhookRegistrationService{
		workspaceRepo:    f.workspaceRepo,
		authService:      authService,
		logger:           logger.NewLoggerWithLevel("disabled"),
		apiEndpoint:      "https://api.example.com",
		webhookProviders: map[domain.EmailProviderKind]domain.WebhookProvider{domain.EmailProviderKindSES: provider},
	}
	return f
}

var sesRegistrationConfig = &domain.WebhookRegistrationConfig{
	IntegrationID: "int-1",
	EventTypes:    []domain.EmailEventType{domain.EmailEventDelivered},
}

func managedSetStatus(name string, managed bool) *domain.WebhookRegistrationStatus {
	return &domain.WebhookRegistrationStatus{
		IsRegistered:    true,
		ProviderDetails: map[string]interface{}{"configuration_set": name, "configuration_set_managed": managed},
	}
}

// The send path reads the configuration set Notifuse manages from the settings, so
// registration stores it; anything less leaves the send path on the throttled lookup.
func TestRegisterWebhooks_SES_PersistsTheManagedConfigurationSet(t *testing.T) {
	ctx := context.Background()

	t.Run("a managed set is stored on the integration", func(t *testing.T) {
		f := newSESRegistrationFixture(t, &fakeSESProvider{status: managedSetStatus("notifuse-int-1", true)}, true)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(sesIntegrationWorkspace(domain.AmazonSESSettings{}), nil).Times(2)
		f.workspaceRepo.EXPECT().PatchIntegrationSESSettings(gomock.Any(), "ws-1", "int-1",
			map[string]interface{}{"managed_configuration_set": "notifuse-int-1"}).Return(nil)

		status, err := f.svc.RegisterWebhooks(ctx, "ws-1", sesRegistrationConfig)

		require.NoError(t, err)
		assert.True(t, status.IsRegistered)
	})

	t.Run("the set already stored is not written again", func(t *testing.T) {
		f := newSESRegistrationFixture(t, &fakeSESProvider{status: managedSetStatus("notifuse-int-1", true)}, true)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").
			Return(sesIntegrationWorkspace(domain.AmazonSESSettings{ManagedConfigurationSet: "notifuse-int-1"}), nil).Times(2)

		_, err := f.svc.RegisterWebhooks(ctx, "ws-1", sesRegistrationConfig)

		require.NoError(t, err)
	})

	// A set the operator named is theirs: Notifuse does not record it as one it manages.
	t.Run("a set Notifuse does not manage is not stored", func(t *testing.T) {
		f := newSESRegistrationFixture(t, &fakeSESProvider{status: managedSetStatus("operator-set", false)}, true)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(sesIntegrationWorkspace(domain.AmazonSESSettings{}), nil).Times(1)

		_, err := f.svc.RegisterWebhooks(ctx, "ws-1", sesRegistrationConfig)

		require.NoError(t, err)
	})

	failures := map[string]func(repo *mocks.MockWorkspaceRepository){
		"the workspace cannot be read again": func(repo *mocks.MockWorkspaceRepository) {
			repo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(nil, errors.New("db gone"))
		},
		"the integration is gone": func(repo *mocks.MockWorkspaceRepository) {
			repo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(&domain.Workspace{ID: "ws-1"}, nil)
		},
		"the write fails": func(repo *mocks.MockWorkspaceRepository) {
			repo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(sesIntegrationWorkspace(domain.AmazonSESSettings{}), nil)
			repo.EXPECT().PatchIntegrationSESSettings(gomock.Any(), "ws-1", "int-1", gomock.Any()).Return(errors.New("db gone"))
		},
	}
	for name, fail := range failures {
		t.Run("storing fails without failing the registration: "+name, func(t *testing.T) {
			f := newSESRegistrationFixture(t, &fakeSESProvider{status: managedSetStatus("notifuse-int-1", true)}, true)
			f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(sesIntegrationWorkspace(domain.AmazonSESSettings{}), nil).Times(1)
			fail(f.workspaceRepo)

			status, err := f.svc.RegisterWebhooks(ctx, "ws-1", sesRegistrationConfig)

			require.NoError(t, err)
			assert.True(t, status.IsRegistered)
		})
	}
}

// Registration may recreate the configuration set, which AWS treats as a new resource
// with no tenant: the tenant is re-attached right after.
func TestRegisterWebhooks_SES_ReattachesTheTenant(t *testing.T) {
	ctx := context.Background()
	withTenant := domain.AmazonSESSettings{Region: "eu-west-1", TenantName: "acme"}

	t.Run("the tenant is re-attached after registering", func(t *testing.T) {
		provider := &fakeSESProvider{status: &domain.WebhookRegistrationStatus{IsRegistered: true}}
		f := newSESRegistrationFixture(t, provider, true)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(sesIntegrationWorkspace(withTenant), nil)

		_, err := f.svc.RegisterWebhooks(ctx, "ws-1", sesRegistrationConfig)

		require.NoError(t, err)
		assert.Equal(t, []string{"register", "associate", "inbound"}, provider.calls)
		require.NotNil(t, provider.associatedTo)
		assert.Equal(t, "acme", provider.associatedTo.TenantName)
		assert.Equal(t, []domain.EmailSender{{ID: "sender-1", Email: "news@example.com"}}, provider.senders)
	})

	t.Run("a failed re-attachment does not fail the registration", func(t *testing.T) {
		provider := &fakeSESProvider{status: &domain.WebhookRegistrationStatus{IsRegistered: true}, associateErr: errors.New("throttled")}
		f := newSESRegistrationFixture(t, provider, true)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(sesIntegrationWorkspace(withTenant), nil)

		status, err := f.svc.RegisterWebhooks(ctx, "ws-1", sesRegistrationConfig)

		require.NoError(t, err)
		assert.True(t, status.IsRegistered)
	})

	t.Run("no tenant, nothing re-attached", func(t *testing.T) {
		provider := &fakeSESProvider{status: &domain.WebhookRegistrationStatus{IsRegistered: true}}
		f := newSESRegistrationFixture(t, provider, true)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(sesIntegrationWorkspace(domain.AmazonSESSettings{}), nil)

		_, err := f.svc.RegisterWebhooks(ctx, "ws-1", sesRegistrationConfig)

		require.NoError(t, err)
		assert.NotContains(t, provider.calls, "associate")
	})
}

// The inbound parser only accepts replies from the topic stored on the integration.
func TestRegisterWebhooks_SES_PersistsTheInboundTopic(t *testing.T) {
	ctx := context.Background()

	t.Run("the topic is stored", func(t *testing.T) {
		provider := &fakeSESProvider{status: &domain.WebhookRegistrationStatus{IsRegistered: true}, inboundARN: "arn:aws:sns:eu-west-1:1:inbound"}
		f := newSESRegistrationFixture(t, provider, true)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").
			DoAndReturn(func(context.Context, string) (*domain.Workspace, error) {
				return sesIntegrationWorkspace(domain.AmazonSESSettings{}), nil
			}).Times(2)
		f.workspaceRepo.EXPECT().PatchIntegrationSESSettings(gomock.Any(), "ws-1", "int-1",
			map[string]interface{}{"inbound_topic_arn": "arn:aws:sns:eu-west-1:1:inbound"}).Return(nil)

		_, err := f.svc.RegisterWebhooks(ctx, "ws-1", sesRegistrationConfig)

		require.NoError(t, err)
	})

	t.Run("storing it fails without failing the registration", func(t *testing.T) {
		provider := &fakeSESProvider{status: &domain.WebhookRegistrationStatus{IsRegistered: true}, inboundARN: "arn:aws:sns:eu-west-1:1:inbound"}
		f := newSESRegistrationFixture(t, provider, true)
		gomock.InOrder(
			f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(sesIntegrationWorkspace(domain.AmazonSESSettings{}), nil),
			f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(nil, errors.New("db gone")),
		)

		status, err := f.svc.RegisterWebhooks(ctx, "ws-1", sesRegistrationConfig)

		require.NoError(t, err)
		assert.True(t, status.IsRegistered)
	})
}

func TestPersistSESManagedConfigurationSet_NoNameNothingRead(t *testing.T) {
	f := newSESRegistrationFixture(t, &fakeSESProvider{}, false)
	require.NoError(t, f.svc.persistSESManagedConfigurationSet(context.Background(), "ws-1", "int-1", ""))
}

func TestPersistSESInboundTopicARN_Refusals(t *testing.T) {
	ctx := context.Background()

	t.Run("no topic, nothing read", func(t *testing.T) {
		f := newSESRegistrationFixture(t, &fakeSESProvider{}, false)
		require.NoError(t, f.svc.persistSESInboundTopicARN(ctx, "ws-1", "int-1", ""))
	})

	t.Run("the integration is not an SES one", func(t *testing.T) {
		f := newSESRegistrationFixture(t, &fakeSESProvider{}, false)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(&domain.Workspace{ID: "ws-1", Integrations: domain.Integrations{{
			ID: "int-1", EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindMailgun},
		}}}, nil)

		err := f.svc.persistSESInboundTopicARN(ctx, "ws-1", "int-1", "arn:topic")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "SES integration int-1 not found")
	})

	t.Run("the workspace cannot be read", func(t *testing.T) {
		f := newSESRegistrationFixture(t, &fakeSESProvider{}, false)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(nil, errors.New("db gone"))

		err := f.svc.persistSESInboundTopicARN(ctx, "ws-1", "int-1", "arn:topic")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load workspace")
	})
}

// Deleting an integration removes what it owns at the provider: webhooks first, then
// the sending resources that outlive them. A provider failure never blocks the
// deletion.
func TestDeleteIntegrationResources(t *testing.T) {
	ctx := context.Background()

	t.Run("webhooks, then the sending resources", func(t *testing.T) {
		provider := &fakeSESProvider{}
		f := newSESRegistrationFixture(t, provider, false)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(sesIntegrationWorkspace(domain.AmazonSESSettings{}), nil)

		require.NoError(t, f.svc.DeleteIntegrationResources(ctx, "ws-1", "int-1"))
		assert.Equal(t, []string{"unregister", "teardown"}, provider.calls)
	})

	t.Run("a failed unregistration still tears the resources down", func(t *testing.T) {
		provider := &fakeSESProvider{unregisterErr: errors.New("access denied")}
		f := newSESRegistrationFixture(t, provider, false)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(sesIntegrationWorkspace(domain.AmazonSESSettings{}), nil)

		require.NoError(t, f.svc.DeleteIntegrationResources(ctx, "ws-1", "int-1"))
		assert.Equal(t, []string{"unregister", "teardown"}, provider.calls)
	})

	t.Run("a failed teardown does not block the deletion", func(t *testing.T) {
		provider := &fakeSESProvider{teardownErr: errors.New("tenant in use")}
		f := newSESRegistrationFixture(t, provider, false)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(sesIntegrationWorkspace(domain.AmazonSESSettings{}), nil)

		require.NoError(t, f.svc.DeleteIntegrationResources(ctx, "ws-1", "int-1"))
	})

	t.Run("a provider without sending resources only unregisters", func(t *testing.T) {
		provider := &webhooksOnlyProvider{}
		f := newSESRegistrationFixture(t, &fakeSESProvider{}, false)
		f.svc.webhookProviders = map[domain.EmailProviderKind]domain.WebhookProvider{domain.EmailProviderKindMailgun: provider}
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(&domain.Workspace{ID: "ws-1", Integrations: domain.Integrations{{
			ID: "int-1", EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindMailgun},
		}}}, nil)

		require.NoError(t, f.svc.DeleteIntegrationResources(ctx, "ws-1", "int-1"))
		assert.Equal(t, 1, provider.unregistered)
	})

	t.Run("the integration is gone", func(t *testing.T) {
		provider := &fakeSESProvider{}
		f := newSESRegistrationFixture(t, provider, false)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(&domain.Workspace{ID: "ws-1"}, nil)

		err := f.svc.DeleteIntegrationResources(ctx, "ws-1", "int-1")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "integration with ID int-1 not found")
		assert.Empty(t, provider.calls)
	})

	t.Run("a provider kind without webhooks", func(t *testing.T) {
		provider := &fakeSESProvider{}
		f := newSESRegistrationFixture(t, provider, false)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(&domain.Workspace{ID: "ws-1", Integrations: domain.Integrations{{
			ID: "int-1", EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP},
		}}}, nil)

		err := f.svc.DeleteIntegrationResources(ctx, "ws-1", "int-1")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "webhook registration not implemented for provider: smtp")
		assert.Empty(t, provider.calls)
	})
}

// Each provider service that implements webhook registration is registered under its
// kind.
func TestNewWebhookRegistrationService_RegistersWebhookProviders(t *testing.T) {
	type sparkPost struct {
		domain.SparkPostServiceInterface
		*fakeSESProvider
	}
	type postmark struct {
		domain.PostmarkServiceInterface
		*fakeSESProvider
	}
	type mailgun struct {
		domain.MailgunServiceInterface
		*fakeSESProvider
	}
	type mailjet struct {
		domain.MailjetServiceInterface
		*fakeSESProvider
	}
	type ses struct {
		domain.SESServiceInterface
		*fakeSESProvider
	}
	type sendGrid struct {
		domain.SendGridServiceInterface
		*fakeSESProvider
	}
	providers := map[domain.EmailProviderKind]*fakeSESProvider{
		domain.EmailProviderKindSparkPost: {}, domain.EmailProviderKindPostmark: {}, domain.EmailProviderKindMailgun: {},
		domain.EmailProviderKindMailjet: {}, domain.EmailProviderKindSES: {}, domain.EmailProviderKindSendGrid: {},
	}

	svc := NewWebhookRegistrationService(nil, nil,
		postmark{fakeSESProvider: providers[domain.EmailProviderKindPostmark]},
		mailgun{fakeSESProvider: providers[domain.EmailProviderKindMailgun]},
		mailjet{fakeSESProvider: providers[domain.EmailProviderKindMailjet]},
		sparkPost{fakeSESProvider: providers[domain.EmailProviderKindSparkPost]},
		ses{fakeSESProvider: providers[domain.EmailProviderKindSES]},
		sendGrid{fakeSESProvider: providers[domain.EmailProviderKindSendGrid]},
		logger.NewLoggerWithLevel("disabled"), "https://api.example.com")

	require.Len(t, svc.webhookProviders, len(providers))
	for kind, provider := range providers {
		registered, ok := svc.webhookProviders[kind]
		require.True(t, ok, "%s is registered", kind)
		// The registered value wraps this kind's fake: a call reaches it and no other.
		require.NoError(t, registered.UnregisterWebhooks(context.Background(), "ws-1", "int-1", nil))
		assert.Equal(t, []string{"unregister"}, provider.calls, "%s reaches its own service", kind)
	}
}
