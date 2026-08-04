package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTenantOperator records what the discovery service asks of the SES layer, so tests can
// assert on the credentials it resolved without standing up a real AWS client.
type fakeTenantOperator struct {
	gotSettings   domain.AmazonSESSettings
	gotSenders    []domain.EmailSender
	provisionCall int

	provisionResult *domain.SESTenantProvisionResult
	provisionErr    error
	tenants         []domain.SESTenant
	tenantsErr      error
	configSets      []string
	configSetsErr   error
	verification    *domain.SESTenantVerification
	verifyConfigSet string
}

func (f *fakeTenantOperator) ListSESTenants(_ context.Context, cfg domain.AmazonSESSettings) ([]domain.SESTenant, bool, error) {
	f.gotSettings = cfg
	return f.tenants, false, f.tenantsErr
}

func (f *fakeTenantOperator) ListConfigurationSets(_ context.Context, cfg domain.AmazonSESSettings) ([]string, error) {
	f.gotSettings = cfg
	return f.configSets, f.configSetsErr
}

func (f *fakeTenantOperator) VerifyTenantAssociation(_ context.Context, cfg domain.AmazonSESSettings, _, configSetName string) (*domain.SESTenantVerification, error) {
	f.gotSettings = cfg
	f.verifyConfigSet = configSetName
	return f.verification, nil
}

func (f *fakeTenantOperator) EnsureTenantIsolation(_ context.Context, cfg domain.AmazonSESSettings, _ string, senders []domain.EmailSender) (*domain.SESTenantProvisionResult, error) {
	f.gotSettings = cfg
	f.gotSenders = senders
	f.provisionCall++
	return f.provisionResult, f.provisionErr
}

func setupDiscovery(t *testing.T, role string) (*SESDiscoveryService, *fakeTenantOperator, *mocks.MockWorkspaceRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)

	repo := mocks.NewMockWorkspaceRepository(ctrl)
	auth := mocks.NewMockAuthService(ctrl)
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	for _, level := range []string{"Error", "Warn", "Info", "Debug"} {
		switch level {
		case "Error":
			logger.EXPECT().Error(gomock.Any()).AnyTimes()
		case "Warn":
			logger.EXPECT().Warn(gomock.Any()).AnyTimes()
		case "Info":
			logger.EXPECT().Info(gomock.Any()).AnyTimes()
		case "Debug":
			logger.EXPECT().Debug(gomock.Any()).AnyTimes()
		}
	}

	auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, &domain.User{ID: "user-1"}, &domain.UserWorkspace{Role: role}, nil
		}).AnyTimes()

	operator := &fakeTenantOperator{}
	return NewSESDiscoveryService(repo, auth, operator, logger), operator, repo
}

func workspaceWithSES(settings *domain.AmazonSESSettings) *domain.Workspace {
	return &domain.Workspace{
		ID: "ws",
		Integrations: domain.Integrations{{
			ID:   "int-1",
			Type: domain.IntegrationTypeEmail,
			EmailProvider: domain.EmailProvider{
				Kind:    domain.EmailProviderKindSES,
				SES:     settings,
				Senders: []domain.EmailSender{{Email: "hello@acme.com", Name: "Acme"}},
			},
		}},
	}
}

func TestSESDiscoveryService_Authorization(t *testing.T) {
	t.Run("non-owners are refused", func(t *testing.T) {
		service, _, _ := setupDiscovery(t, "member")

		_, err := service.ListTenants(context.Background(), domain.SESCredentialsRef{
			WorkspaceID: "ws", IntegrationID: "int-1",
		})

		var unauthorized *domain.ErrUnauthorized
		assert.ErrorAs(t, err, &unauthorized)
	})

	t.Run("a rejected region never reaches AWS", func(t *testing.T) {
		service, operator, _ := setupDiscovery(t, "owner")

		_, err := service.ListTenants(context.Background(), domain.SESCredentialsRef{
			WorkspaceID: "ws", Region: "evil.example.com", AccessKey: "AKIA", SecretKey: "s",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "region")
		assert.Empty(t, operator.gotSettings.Region, "no AWS call should have been attempted")
	})
}

func TestSESDiscoveryService_CredentialModes(t *testing.T) {
	t.Run("saved integration uses the stored, decrypted secret", func(t *testing.T) {
		service, operator, repo := setupDiscovery(t, "owner")

		repo.EXPECT().GetByID(gomock.Any(), "ws").Return(workspaceWithSES(&domain.AmazonSESSettings{
			Region: "eu-west-3", AccessKey: "stored-key", SecretKey: "stored-secret",
		}), nil)

		_, err := service.ListTenants(context.Background(), domain.SESCredentialsRef{
			WorkspaceID: "ws", IntegrationID: "int-1",
		})

		require.NoError(t, err)
		assert.Equal(t, "stored-secret", operator.gotSettings.SecretKey)
	})

	t.Run("inline credentials never touch the repository", func(t *testing.T) {
		service, operator, repo := setupDiscovery(t, "owner")

		// The create drawer has no saved integration to read.
		repo.EXPECT().GetByID(gomock.Any(), gomock.Any()).Times(0)

		_, err := service.ListTenants(context.Background(), domain.SESCredentialsRef{
			WorkspaceID: "ws", Region: "eu-west-3", AccessKey: "typed-key", SecretKey: "typed-secret",
		})

		require.NoError(t, err)
		assert.Equal(t, "typed-secret", operator.gotSettings.SecretKey)
	})

	t.Run("a non-SES integration is a clear error", func(t *testing.T) {
		service, _, repo := setupDiscovery(t, "owner")

		repo.EXPECT().GetByID(gomock.Any(), "ws").Return(workspaceWithSES(nil), nil)

		_, err := service.ListTenants(context.Background(), domain.SESCredentialsRef{
			WorkspaceID: "ws", IntegrationID: "int-1",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "Amazon SES")
	})
}

// TestSESDiscoveryService_EnableTenantIsolation_RequiresAssociation is the regression guard for
// the worst failure this feature can produce: recording a tenant whose configuration set is not
// associated makes SES reject EVERY subsequent send from that integration.
func TestSESDiscoveryService_EnableTenantIsolation_RequiresAssociation(t *testing.T) {
	t.Run("association failed: nothing is enabled for sending", func(t *testing.T) {
		service, operator, repo := setupDiscovery(t, "owner")

		workspace := workspaceWithSES(&domain.AmazonSESSettings{
			Region: "eu-west-3", AccessKey: "k", SecretKey: "s",
		})
		repo.EXPECT().GetByID(gomock.Any(), "ws").Return(workspace, nil)
		// The whole point: no write at all.
		repo.EXPECT().PatchIntegrationSESSettings(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		operator.provisionResult = &domain.SESTenantProvisionResult{
			TenantName:                 "notifuse-int-1",
			Created:                    true,
			SuppressionScoped:          true,
			ConfigurationSetAssociated: false,
			MissingPermissions:         []string{"ses:CreateTenantResourceAssociation"},
		}

		result, err := service.EnableTenantIsolation(context.Background(), domain.EnableSESTenantIsolationRequest{
			WorkspaceID: "ws", IntegrationID: "int-1",
		})

		require.NoError(t, err)
		assert.True(t, result.Created, "the tenant does exist in AWS and is billable")
		assert.False(t, result.ConfigurationSetAssociated)
		// No write at all: sends must not start using a tenant that would reject them.
		assert.Empty(t, workspace.Integrations[0].EmailProvider.SES.ManagedTenantName)
	})

	t.Run("fully provisioned: the tenant is recorded", func(t *testing.T) {
		service, operator, repo := setupDiscovery(t, "owner")

		workspace := workspaceWithSES(&domain.AmazonSESSettings{
			Region: "eu-west-3", AccessKey: "k", SecretKey: "s",
		})
		repo.EXPECT().GetByID(gomock.Any(), "ws").Return(workspace, nil)
		repo.EXPECT().
			PatchIntegrationSESSettings(gomock.Any(), "ws", "int-1", map[string]interface{}{
				"tenant_isolation_enabled": true,
				"managed_tenant_name":      "notifuse-int-1",
			}).
			Return(nil).Times(1)

		operator.provisionResult = &domain.SESTenantProvisionResult{
			TenantName:                 "notifuse-int-1",
			Created:                    true,
			SuppressionScoped:          true,
			ConfigurationSetAssociated: true,
		}

		result, err := service.EnableTenantIsolation(context.Background(), domain.EnableSESTenantIsolationRequest{
			WorkspaceID: "ws", IntegrationID: "int-1",
		})

		require.NoError(t, err)
		assert.False(t, result.ProvisionedButUnsaved)
		assert.Equal(t, []domain.EmailSender{{Email: "hello@acme.com", Name: "Acme"}}, operator.gotSenders)
	})

	t.Run("provisioned in AWS but unsaved is reported distinctly", func(t *testing.T) {
		service, operator, repo := setupDiscovery(t, "owner")

		workspace := workspaceWithSES(&domain.AmazonSESSettings{
			Region: "eu-west-3", AccessKey: "k", SecretKey: "s",
		})
		repo.EXPECT().GetByID(gomock.Any(), "ws").Return(workspace, nil)
		repo.EXPECT().PatchIntegrationSESSettings(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(errors.New("db down"))

		operator.provisionResult = &domain.SESTenantProvisionResult{
			TenantName:                 "notifuse-int-1",
			Created:                    true,
			ConfigurationSetAssociated: true,
		}

		result, err := service.EnableTenantIsolation(context.Background(), domain.EnableSESTenantIsolationRequest{
			WorkspaceID: "ws", IntegrationID: "int-1",
		})

		require.NoError(t, err, "AWS holds the tenant; the caller must be told to retry, not shown a failure")
		assert.True(t, result.ProvisionedButUnsaved)
	})

	t.Run("a manually managed tenant is not overwritten", func(t *testing.T) {
		service, operator, repo := setupDiscovery(t, "owner")

		repo.EXPECT().GetByID(gomock.Any(), "ws").Return(workspaceWithSES(&domain.AmazonSESSettings{
			Region: "eu-west-3", AccessKey: "k", SecretKey: "s", TenantName: "operator-tenant",
		}), nil)

		_, err := service.EnableTenantIsolation(context.Background(), domain.EnableSESTenantIsolationRequest{
			WorkspaceID: "ws", IntegrationID: "int-1",
		})

		require.Error(t, err)
		assert.Equal(t, 0, operator.provisionCall, "nothing should be provisioned")
	})
}

func TestSESDiscoveryService_VerifyTenant_DefaultsToTheSendingConfigurationSet(t *testing.T) {
	service, operator, repo := setupDiscovery(t, "owner")

	repo.EXPECT().GetByID(gomock.Any(), "ws").Return(workspaceWithSES(&domain.AmazonSESSettings{
		Region: "eu-west-3", AccessKey: "k", SecretKey: "s",
	}), nil)
	operator.verification = &domain.SESTenantVerification{TenantName: "t", Exists: true}

	_, err := service.VerifyTenant(context.Background(), domain.VerifySESTenantRequest{
		SESCredentialsRef: domain.SESCredentialsRef{WorkspaceID: "ws", IntegrationID: "int-1"},
		TenantName:        "t",
	})

	require.NoError(t, err)
	// Verification must ask about the set the send path would actually use.
	assert.Equal(t, "notifuse-int-1", operator.verifyConfigSet)
}

func TestSESDiscoveryService_ListConfigurationSets_MapsDenial(t *testing.T) {
	service, operator, repo := setupDiscovery(t, "owner")

	repo.EXPECT().GetByID(gomock.Any(), "ws").Return(workspaceWithSES(&domain.AmazonSESSettings{
		Region: "eu-west-3", AccessKey: "k", SecretKey: "s",
	}), nil)
	operator.configSetsErr = domain.ErrSESAccessDenied

	_, err := service.ListConfigurationSets(context.Background(), domain.SESCredentialsRef{
		WorkspaceID: "ws", IntegrationID: "int-1",
	})

	assert.ErrorIs(t, err, domain.ErrSESAccessDenied)
}
