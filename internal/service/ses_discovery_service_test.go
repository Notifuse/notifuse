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
	return setupDiscoveryWithEntitlements(t, role, nil)
}

// setupDiscoveryWithEntitlements builds the service with a stubbed licence.
//
// A nil ent wires no provider at all, which leaves the tenant gate inert — that is the shape
// every test written before licensing relies on, and it is deliberately NOT the same thing as
// an unlicensed deployment: that one passes domain.CommunityEntitlements() and gets refused.
func setupDiscoveryWithEntitlements(t *testing.T, role string, ent *domain.Entitlements) (*SESDiscoveryService, *fakeTenantOperator, *mocks.MockWorkspaceRepository) {
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

	var provider domain.EntitlementProvider
	if ent != nil {
		entitlements := mocks.NewMockEntitlementProvider(ctrl)
		entitlements.EXPECT().Entitlements().Return(*ent).AnyTimes()
		provider = entitlements
	}

	return NewSESDiscoveryService(repo, auth, operator, logger, provider), operator, repo
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

// TestSESDiscoveryService_EnableTenantIsolation_LicenceGate covers G4. The gate is on
// PROVISIONING only, and the subtests below pin both halves of that: a new tenant is refused
// without the licence, and an existing one keeps resolving in the send path with no key at all.
func TestSESDiscoveryService_EnableTenantIsolation_LicenceGate(t *testing.T) {
	community := domain.CommunityEntitlements()
	licensed := domain.Entitlements{
		Tier:          "studio",
		MaxWorkspaces: 5,
		Features:      []domain.Feature{domain.FeatureSESTenant},
		State:         domain.LicenseStateActive,
	}
	// A key that verifies but does not carry ses_tenant refuses exactly like no key at all:
	// the gate reads the feature list, never the tier and never the state.
	licensedWithoutTenant := domain.Entitlements{
		Tier:          "studio",
		MaxWorkspaces: 5,
		Features:      []domain.Feature{domain.FeatureRBAC},
		State:         domain.LicenseStateActive,
	}

	provision := func(t *testing.T, ent domain.Entitlements) (*domain.SESTenantProvisionResult, error, *fakeTenantOperator) {
		t.Helper()
		service, operator, repo := setupDiscoveryWithEntitlements(t, "owner", &ent)
		repo.EXPECT().GetByID(gomock.Any(), "ws").Return(workspaceWithSES(&domain.AmazonSESSettings{
			Region: "eu-west-3", AccessKey: "k", SecretKey: "s",
		}), nil).AnyTimes()
		repo.EXPECT().PatchIntegrationSESSettings(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		operator.provisionResult = &domain.SESTenantProvisionResult{
			TenantName:                 "notifuse-int-1",
			Created:                    true,
			SuppressionScoped:          true,
			ConfigurationSetAssociated: true,
		}

		result, err := service.EnableTenantIsolation(context.Background(), domain.EnableSESTenantIsolationRequest{
			WorkspaceID: "ws", IntegrationID: "int-1",
		})
		return result, err, operator
	}

	t.Run("provisioning is refused without a licence, before anything reaches AWS", func(t *testing.T) {
		_, err, operator := provision(t, community)

		require.Error(t, err)
		var notLicensed *domain.ErrFeatureNotLicensed
		require.True(t, errors.As(err, &notLicensed))
		assert.Equal(t, domain.FeatureSESTenant, notLicensed.Feature)
		assert.NotEmpty(t, notLicensed.Message)
		// Nothing is created in AWS, so nothing is billed for a refusal.
		assert.Equal(t, 0, operator.provisionCall)
	})

	t.Run("a licence without the ses_tenant feature is refused the same way", func(t *testing.T) {
		_, err, operator := provision(t, licensedWithoutTenant)

		var notLicensed *domain.ErrFeatureNotLicensed
		require.True(t, errors.As(err, &notLicensed))
		assert.Equal(t, 0, operator.provisionCall)
	})

	t.Run("provisioning proceeds with the ses_tenant feature", func(t *testing.T) {
		result, err, operator := provision(t, licensed)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "notifuse-int-1", result.TenantName)
		assert.Equal(t, 1, operator.provisionCall)
	})

	t.Run("an unwired provider leaves provisioning exactly as it was", func(t *testing.T) {
		service, operator, repo := setupDiscovery(t, "owner")
		repo.EXPECT().GetByID(gomock.Any(), "ws").Return(workspaceWithSES(&domain.AmazonSESSettings{
			Region: "eu-west-3", AccessKey: "k", SecretKey: "s",
		}), nil)
		repo.EXPECT().PatchIntegrationSESSettings(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		operator.provisionResult = &domain.SESTenantProvisionResult{
			TenantName: "notifuse-int-1", Created: true, ConfigurationSetAssociated: true,
		}

		_, err := service.EnableTenantIsolation(context.Background(), domain.EnableSESTenantIsolationRequest{
			WorkspaceID: "ws", IntegrationID: "int-1",
		})
		require.NoError(t, err)
		assert.Equal(t, 1, operator.provisionCall)
	})

	t.Run("an already provisioned tenant still resolves for sending with no licence", func(t *testing.T) {
		// The disaster this prevents: a check on the send path would leave the SendEmailInput's
		// tenant field nil, silently moving a paying customer's mail from tenant-scoped
		// suppression to ACCOUNT-WIDE suppression — the exact outcome the feature was bought to
		// avoid. Licensing stops at provisioning, so an existing tenant sends forever.
		provisioned := &domain.AmazonSESSettings{
			Region:                 "eu-west-3",
			AccessKey:              "k",
			SecretKey:              "s",
			TenantIsolationEnabled: true,
			ManagedTenantName:      "notifuse-int-1",
		}

		assert.Equal(t, "notifuse-int-1", provisioned.ResolveTenant())
		assert.Equal(t, "notifuse-int-1", provisioned.KnownTenant())

		// And the same unlicensed deployment refuses to provision a NEW one.
		service, operator, repo := setupDiscoveryWithEntitlements(t, "owner", &community)
		repo.EXPECT().GetByID(gomock.Any(), "ws").Return(workspaceWithSES(&domain.AmazonSESSettings{
			Region: "eu-west-3", AccessKey: "k", SecretKey: "s",
		}), nil)

		_, err := service.EnableTenantIsolation(context.Background(), domain.EnableSESTenantIsolationRequest{
			WorkspaceID: "ws", IntegrationID: "int-1",
		})
		require.Error(t, err)
		assert.Equal(t, 0, operator.provisionCall)
	})

	t.Run("a non-owner is refused as a non-owner, not as an unlicensed deployment", func(t *testing.T) {
		// Authorization comes first: a caller who may not touch this integration is not told
		// what the deployment has or has not bought.
		service, operator, _ := setupDiscoveryWithEntitlements(t, "member", &community)

		_, err := service.EnableTenantIsolation(context.Background(), domain.EnableSESTenantIsolationRequest{
			WorkspaceID: "ws", IntegrationID: "int-1",
		})

		require.Error(t, err)
		var unauthorized *domain.ErrUnauthorized
		assert.ErrorAs(t, err, &unauthorized)
		var notLicensed *domain.ErrFeatureNotLicensed
		assert.False(t, errors.As(err, &notLicensed))
		assert.Equal(t, 0, operator.provisionCall)
	})
}
