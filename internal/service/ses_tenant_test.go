package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sesv2types "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/aws/smithy-go"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTenantARN = "arn:aws:ses:us-east-1:123456789012:tenant/notifuse-int-1/t-1"

func accessDenied() error {
	return &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "no"}
}

// expectManagedConfigSet allows the v1 calls that ensure the managed configuration set exists.
func expectManagedConfigSet(mockSES *mocks.MockSESWebhookClient) {
	mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{{Name: aws.String("notifuse-int-1")}},
		}, nil).AnyTimes()
	mockSES.EXPECT().CreateConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.CreateConfigurationSetOutput{}, nil).AnyTimes()
}

// TestEnsureTenantIsolation_CreatesTenantWithTenantScopedSuppression is the single assertion
// that guards the second half of what #400 asks for: AWS defaults a tenant to the shared
// account suppression list, which silently delivers reputation isolation only.
func TestEnsureTenantIsolation_CreatesTenantWithTenantScopedSuppression(t *testing.T) {
	service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)
	expectManagedConfigSet(mockSES)

	mockSESv2.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.CreateTenantInput, _ ...func(*sesv2.Options)) (*sesv2.CreateTenantOutput, error) {
			assert.Equal(t, "notifuse-int-1", awsv2.ToString(input.TenantName))
			require.NotNil(t, input.SuppressionAttributes)
			assert.Equal(t, sesv2types.SuppressionListScopeTenant, input.SuppressionAttributes.SuppressionScope)
			assert.ElementsMatch(t,
				[]sesv2types.SuppressionListReason{sesv2types.SuppressionListReasonBounce, sesv2types.SuppressionListReasonComplaint},
				input.SuppressionAttributes.SuppressedReasons)
			return &sesv2.CreateTenantOutput{TenantArn: awsv2.String(testTenantARN)}, nil
		})

	mockSESv2.EXPECT().CreateTenantResourceAssociation(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.CreateTenantResourceAssociationInput, _ ...func(*sesv2.Options)) (*sesv2.CreateTenantResourceAssociationOutput, error) {
			assert.Equal(t, "arn:aws:ses:us-east-1:123456789012:configuration-set/notifuse-int-1",
				awsv2.ToString(input.ResourceArn))
			return &sesv2.CreateTenantResourceAssociationOutput{}, nil
		})

	result, err := service.EnsureTenantIsolation(context.Background(), *sesSettings(), "int-1", nil)

	require.NoError(t, err)
	assert.True(t, result.Created)
	assert.True(t, result.SuppressionScoped)
	assert.Equal(t, "notifuse-int-1", result.TenantName)
	assert.Len(t, result.Associated, 1)
	assert.Empty(t, result.MissingPermissions)
}

// TestEnsureTenantIsolation_ConvergesExistingTenant covers the tenant an operator made by hand,
// which defaults to the shared account suppression list.
func TestEnsureTenantIsolation_ConvergesExistingTenant(t *testing.T) {
	service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)
	expectManagedConfigSet(mockSES)

	mockSESv2.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).
		Return(nil, &sesv2types.AlreadyExistsException{})
	mockSESv2.EXPECT().GetTenant(gomock.Any(), gomock.Any()).
		Return(&sesv2.GetTenantOutput{Tenant: &sesv2types.Tenant{
			TenantArn:     awsv2.String(testTenantARN),
			SendingStatus: sesv2types.SendingStatusEnabled,
			SuppressionAttributes: &sesv2types.TenantSuppressionAttributes{
				SuppressionScope: sesv2types.SuppressionListScopeAccount,
			},
		}}, nil)
	mockSESv2.EXPECT().PutTenantSuppressionAttributes(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.PutTenantSuppressionAttributesInput, _ ...func(*sesv2.Options)) (*sesv2.PutTenantSuppressionAttributesOutput, error) {
			assert.Equal(t, sesv2types.SuppressionListScopeTenant, input.SuppressionScope)
			return &sesv2.PutTenantSuppressionAttributesOutput{}, nil
		})
	mockSESv2.EXPECT().CreateTenantResourceAssociation(gomock.Any(), gomock.Any()).
		Return(&sesv2.CreateTenantResourceAssociationOutput{}, nil)

	result, err := service.EnsureTenantIsolation(context.Background(), *sesSettings(), "int-1", nil)

	require.NoError(t, err)
	assert.False(t, result.Created, "the tenant already existed")
	assert.True(t, result.SuppressionScoped, "scope must be converged, not inherited")
	assert.Equal(t, "ENABLED", result.SendingStatus)
}

// TestEnsureTenantIsolation_AssociatesEveryMatchingIdentity: SES may resolve a send against the
// exact address, its domain or a parent domain. Associating only the "best" match is a guess
// that fails the send when SES picks the other one.
func TestEnsureTenantIsolation_AssociatesEveryMatchingIdentity(t *testing.T) {
	service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)
	expectManagedConfigSet(mockSES)

	mockSESv2.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).
		Return(&sesv2.CreateTenantOutput{TenantArn: awsv2.String(testTenantARN)}, nil)
	mockSESv2.EXPECT().ListEmailIdentities(gomock.Any(), gomock.Any()).
		Return(&sesv2.ListEmailIdentitiesOutput{EmailIdentities: []sesv2types.IdentityInfo{
			{IdentityName: awsv2.String("acme.com"), SendingEnabled: true, VerificationStatus: sesv2types.VerificationStatusSuccess},
			{IdentityName: awsv2.String("bob@acme.com"), SendingEnabled: true, VerificationStatus: sesv2types.VerificationStatusSuccess},
		}}, nil)

	var associated []string
	mockSESv2.EXPECT().CreateTenantResourceAssociation(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.CreateTenantResourceAssociationInput, _ ...func(*sesv2.Options)) (*sesv2.CreateTenantResourceAssociationOutput, error) {
			associated = append(associated, awsv2.ToString(input.ResourceArn))
			return &sesv2.CreateTenantResourceAssociationOutput{}, nil
		}).AnyTimes()

	senders := []domain.EmailSender{
		{Email: "bob@acme.com", Name: "Bob"},
		{Email: "sue@mail.acme.com", Name: "Sue"}, // covered by the parent domain
	}
	result, err := service.EnsureTenantIsolation(context.Background(), *sesSettings(), "int-1", senders)

	require.NoError(t, err)
	assert.Contains(t, associated, "arn:aws:ses:us-east-1:123456789012:configuration-set/notifuse-int-1")
	assert.Contains(t, associated, "arn:aws:ses:us-east-1:123456789012:identity/bob@acme.com")
	assert.Contains(t, associated, "arn:aws:ses:us-east-1:123456789012:identity/acme.com")
	assert.Empty(t, result.UnverifiedSenders)
}

// TestEnsureTenantIsolation_IgnoresUnusableIdentity: ListEmailIdentities returns unverified
// identities too, so presence in the list means nothing on its own.
func TestEnsureTenantIsolation_IgnoresUnusableIdentity(t *testing.T) {
	service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)
	expectManagedConfigSet(mockSES)

	mockSESv2.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).
		Return(&sesv2.CreateTenantOutput{TenantArn: awsv2.String(testTenantARN)}, nil)
	mockSESv2.EXPECT().ListEmailIdentities(gomock.Any(), gomock.Any()).
		Return(&sesv2.ListEmailIdentitiesOutput{EmailIdentities: []sesv2types.IdentityInfo{
			{IdentityName: awsv2.String("pending.com"), SendingEnabled: true, VerificationStatus: sesv2types.VerificationStatusPending},
			{IdentityName: awsv2.String("paused.com"), SendingEnabled: false, VerificationStatus: sesv2types.VerificationStatusSuccess},
		}}, nil)
	mockSESv2.EXPECT().CreateTenantResourceAssociation(gomock.Any(), gomock.Any()).
		Return(&sesv2.CreateTenantResourceAssociationOutput{}, nil).Times(1) // configuration set only

	senders := []domain.EmailSender{{Email: "a@pending.com"}, {Email: "b@paused.com"}, {Email: "c@other.com"}}
	result, err := service.EnsureTenantIsolation(context.Background(), *sesSettings(), "int-1", senders)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a@pending.com", "b@paused.com", "c@other.com"}, result.UnverifiedSenders)
}

// TestEnsureTenantIsolation_PartialPermissions: a denial must produce an actionable partial
// result, not an exception the operator cannot act on.
func TestEnsureTenantIsolation_PartialPermissions(t *testing.T) {
	service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)
	expectManagedConfigSet(mockSES)

	mockSESv2.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).
		Return(&sesv2.CreateTenantOutput{TenantArn: awsv2.String(testTenantARN)}, nil)
	mockSESv2.EXPECT().CreateTenantResourceAssociation(gomock.Any(), gomock.Any()).
		Return(nil, accessDenied())

	result, err := service.EnsureTenantIsolation(context.Background(), *sesSettings(), "int-1", nil)

	require.NoError(t, err, "a denial is reported, not returned as an error")
	assert.True(t, result.Created)
	assert.Contains(t, result.MissingPermissions, "ses:CreateTenantResourceAssociation")
	require.Len(t, result.FixCommands, 1)
	assert.Contains(t, result.FixCommands[0], "create-tenant-resource-association")
}

func TestEnsureTenantIsolation_CreateTenantDenied(t *testing.T) {
	service, _, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	mockSESv2.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).Return(nil, accessDenied())

	result, err := service.EnsureTenantIsolation(context.Background(), *sesSettings(), "int-1", nil)

	require.NoError(t, err)
	assert.False(t, result.Created)
	assert.Contains(t, result.MissingPermissions, "ses:CreateTenant")
}

// TestEnsureTenantIsolation_TreatsExistingAssociationAsSuccess covers concurrent provisioning:
// two instances racing must both converge.
func TestEnsureTenantIsolation_TreatsExistingAssociationAsSuccess(t *testing.T) {
	service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)
	expectManagedConfigSet(mockSES)

	mockSESv2.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).
		Return(&sesv2.CreateTenantOutput{TenantArn: awsv2.String(testTenantARN)}, nil)
	mockSESv2.EXPECT().CreateTenantResourceAssociation(gomock.Any(), gomock.Any()).
		Return(nil, &sesv2types.AlreadyExistsException{})

	result, err := service.EnsureTenantIsolation(context.Background(), *sesSettings(), "int-1", nil)

	require.NoError(t, err)
	assert.Len(t, result.Associated, 1, "an existing association still counts as associated")
	assert.Empty(t, result.MissingPermissions)
}

func TestIdentityCoverage(t *testing.T) {
	usable := map[string]bool{"acme.com": true, "bob@acme.com": true, "mail.acme.com": true}

	assert.Equal(t, []string{"bob@acme.com", "acme.com"}, identityCoverage("bob@acme.com", usable))
	assert.Equal(t, []string{"mail.acme.com", "acme.com"}, identityCoverage("sue@mail.acme.com", usable))
	assert.Equal(t, []string{"acme.com"}, identityCoverage("nobody@acme.com", usable))
	assert.Empty(t, identityCoverage("x@other.com", usable))
	assert.Empty(t, identityCoverage("not-an-email", usable))

	// Case must not decide coverage.
	assert.Equal(t, []string{"bob@acme.com", "acme.com"}, identityCoverage("Bob@Acme.COM", usable))
}

func TestVerifyTenantAssociation(t *testing.T) {
	t.Run("associated", func(t *testing.T) {
		service, _, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

		mockSESv2.EXPECT().GetTenant(gomock.Any(), gomock.Any()).
			Return(&sesv2.GetTenantOutput{Tenant: &sesv2types.Tenant{
				TenantArn:     awsv2.String(testTenantARN),
				SendingStatus: sesv2types.SendingStatusEnabled,
				SuppressionAttributes: &sesv2types.TenantSuppressionAttributes{
					SuppressionScope: sesv2types.SuppressionListScopeTenant,
				},
			}}, nil)
		mockSESv2.EXPECT().ListTenantResources(gomock.Any(), gomock.Any()).
			Return(&sesv2.ListTenantResourcesOutput{TenantResources: []sesv2types.TenantResource{
				{ResourceArn: awsv2.String("arn:aws:ses:us-east-1:123456789012:configuration-set/notifuse-int-1")},
			}}, nil)

		got, err := service.VerifyTenantAssociation(context.Background(), *sesSettings(), "notifuse-int-1", "notifuse-int-1")

		require.NoError(t, err)
		assert.True(t, got.Exists)
		assert.True(t, got.ConfigurationSetAssociated)
		assert.Equal(t, "TENANT", got.SuppressionScope)
		assert.Equal(t, "ENABLED", got.SendingStatus)
		assert.Empty(t, got.FixCommand)
	})

	t.Run("a same-suffix name is not a match", func(t *testing.T) {
		service, _, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

		mockSESv2.EXPECT().GetTenant(gomock.Any(), gomock.Any()).
			Return(&sesv2.GetTenantOutput{Tenant: &sesv2types.Tenant{TenantArn: awsv2.String(testTenantARN)}}, nil)
		mockSESv2.EXPECT().ListTenantResources(gomock.Any(), gomock.Any()).
			Return(&sesv2.ListTenantResourcesOutput{TenantResources: []sesv2types.TenantResource{
				// A HasSuffix implementation would call this associated.
				{ResourceArn: awsv2.String("arn:aws:ses:us-east-1:123456789012:configuration-set/prod-notifuse-int-1")},
			}}, nil)

		got, err := service.VerifyTenantAssociation(context.Background(), *sesSettings(), "notifuse-int-1", "notifuse-int-1")

		require.NoError(t, err)
		assert.False(t, got.ConfigurationSetAssociated)
		assert.Contains(t, got.FixCommand, "create-tenant-resource-association")
	})

	t.Run("missing tenant is distinct from missing association", func(t *testing.T) {
		service, _, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

		mockSESv2.EXPECT().GetTenant(gomock.Any(), gomock.Any()).
			Return(nil, &sesv2types.NotFoundException{})

		got, err := service.VerifyTenantAssociation(context.Background(), *sesSettings(), "ghost", "notifuse-int-1")

		require.NoError(t, err)
		assert.False(t, got.Exists)
	})

	t.Run("denial degrades rather than erroring loudly", func(t *testing.T) {
		service, _, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

		mockSESv2.EXPECT().GetTenant(gomock.Any(), gomock.Any()).Return(nil, accessDenied())

		_, err := service.VerifyTenantAssociation(context.Background(), *sesSettings(), "notifuse-int-1", "notifuse-int-1")

		assert.ErrorIs(t, err, domain.ErrSESAccessDenied)
	})
}

func TestListSESTenants(t *testing.T) {
	t.Run("pages until exhausted", func(t *testing.T) {
		service, _, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

		gomock.InOrder(
			mockSESv2.EXPECT().ListTenants(gomock.Any(), gomock.Any()).
				Return(&sesv2.ListTenantsOutput{
					Tenants:   []sesv2types.TenantInfo{{TenantName: awsv2.String("a")}},
					NextToken: awsv2.String("p2"),
				}, nil),
			mockSESv2.EXPECT().ListTenants(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, input *sesv2.ListTenantsInput, _ ...func(*sesv2.Options)) (*sesv2.ListTenantsOutput, error) {
					assert.Equal(t, "p2", awsv2.ToString(input.NextToken))
					return &sesv2.ListTenantsOutput{Tenants: []sesv2types.TenantInfo{{TenantName: awsv2.String("b")}}}, nil
				}),
		)

		tenants, hasMore, err := service.ListSESTenants(context.Background(), *sesSettings())

		require.NoError(t, err)
		assert.False(t, hasMore)
		require.Len(t, tenants, 2)
		assert.Equal(t, "a", tenants[0].Name)
		assert.Equal(t, "b", tenants[1].Name)
	})

	t.Run("denial is typed so the UI can degrade", func(t *testing.T) {
		service, _, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

		mockSESv2.EXPECT().ListTenants(gomock.Any(), gomock.Any()).Return(nil, accessDenied())

		_, _, err := service.ListSESTenants(context.Background(), *sesSettings())

		assert.ErrorIs(t, err, domain.ErrSESAccessDenied)
	})
}

// TestDeleteSendingResources_OrderAndOwnership covers the teardown that integration deletion
// depends on. Order is not cosmetic: AWS refuses to delete a configuration set that is still
// associated with a tenant.
func TestDeleteSendingResources_OrderAndOwnership(t *testing.T) {
	managedSettings := func() *domain.AmazonSESSettings {
		s := sesSettings()
		s.TenantIsolationEnabled = true
		s.ManagedTenantName = "notifuse-int-1"
		s.ManagedConfigurationSet = "notifuse-int-1"
		return s
	}

	t.Run("dissociates before deleting, then removes the tenant", func(t *testing.T) {
		service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

		mockSESv2.EXPECT().GetTenant(gomock.Any(), gomock.Any()).
			Return(&sesv2.GetTenantOutput{Tenant: &sesv2types.Tenant{
				TenantArn: awsv2.String(testTenantARN),
			}}, nil)
		mockSESv2.EXPECT().ListTenantResources(gomock.Any(), gomock.Any()).
			Return(&sesv2.ListTenantResourcesOutput{TenantResources: []sesv2types.TenantResource{
				{ResourceArn: awsv2.String("arn:aws:ses:us-east-1:123456789012:identity/acme.com")},
			}}, nil)

		dissociated := []string{}
		gomock.InOrder(
			mockSESv2.EXPECT().DeleteTenantResourceAssociation(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, in *sesv2.DeleteTenantResourceAssociationInput, _ ...func(*sesv2.Options)) (*sesv2.DeleteTenantResourceAssociationOutput, error) {
					dissociated = append(dissociated, awsv2.ToString(in.ResourceArn))
					return &sesv2.DeleteTenantResourceAssociationOutput{}, nil
				}),
			mockSESv2.EXPECT().DeleteTenantResourceAssociation(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, in *sesv2.DeleteTenantResourceAssociationInput, _ ...func(*sesv2.Options)) (*sesv2.DeleteTenantResourceAssociationOutput, error) {
					dissociated = append(dissociated, awsv2.ToString(in.ResourceArn))
					return &sesv2.DeleteTenantResourceAssociationOutput{}, nil
				}),
			// Only now may the configuration set go.
			mockSES.EXPECT().DeleteConfigurationSetWithContext(gomock.Any(), gomock.Any()).
				Return(&ses.DeleteConfigurationSetOutput{}, nil),
			mockSESv2.EXPECT().DeleteTenant(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, in *sesv2.DeleteTenantInput, _ ...func(*sesv2.Options)) (*sesv2.DeleteTenantOutput, error) {
					assert.Equal(t, "notifuse-int-1", awsv2.ToString(in.TenantName))
					return &sesv2.DeleteTenantOutput{}, nil
				}),
		)

		require.NoError(t, service.DeleteSendingResources(context.Background(), "ws", "int-1",
			&domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: managedSettings()}))

		assert.Contains(t, dissociated, "arn:aws:ses:us-east-1:123456789012:configuration-set/notifuse-int-1")
		assert.Contains(t, dissociated, "arn:aws:ses:us-east-1:123456789012:identity/acme.com")
	})

	t.Run("never deletes a tenant the operator manages", func(t *testing.T) {
		service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

		settings := sesSettings()
		settings.TenantName = "operator-tenant" // theirs, not ours

		mockSESv2.EXPECT().GetTenant(gomock.Any(), gomock.Any()).
			Return(&sesv2.GetTenantOutput{Tenant: &sesv2types.Tenant{
				TenantArn: awsv2.String(testTenantARN),
			}}, nil)
		mockSESv2.EXPECT().DeleteTenantResourceAssociation(gomock.Any(), gomock.Any()).
			Return(&sesv2.DeleteTenantResourceAssociationOutput{}, nil)
		mockSES.EXPECT().DeleteConfigurationSetWithContext(gomock.Any(), gomock.Any()).
			Return(&ses.DeleteConfigurationSetOutput{}, nil)

		// Deleting it would destroy their reputation history and suppression list.
		mockSESv2.EXPECT().DeleteTenant(gomock.Any(), gomock.Any()).Times(0)
		// Their identities may be shared with other integrations.
		mockSESv2.EXPECT().ListTenantResources(gomock.Any(), gomock.Any()).Times(0)

		require.NoError(t, service.DeleteSendingResources(context.Background(), "ws", "int-1",
			&domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: settings}))
	})

	t.Run("never deletes an operator's configuration set", func(t *testing.T) {
		service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

		settings := managedSettings()
		settings.ConfigurationSetName = "operator-set"

		mockSESv2.EXPECT().GetTenant(gomock.Any(), gomock.Any()).
			Return(&sesv2.GetTenantOutput{Tenant: &sesv2types.Tenant{TenantArn: awsv2.String(testTenantARN)}}, nil)
		mockSESv2.EXPECT().ListTenantResources(gomock.Any(), gomock.Any()).
			Return(&sesv2.ListTenantResourcesOutput{}, nil)
		mockSESv2.EXPECT().DeleteTenantResourceAssociation(gomock.Any(), gomock.Any()).
			Return(&sesv2.DeleteTenantResourceAssociationOutput{}, nil).AnyTimes()
		mockSESv2.EXPECT().DeleteTenant(gomock.Any(), gomock.Any()).
			Return(&sesv2.DeleteTenantOutput{}, nil)

		mockSES.EXPECT().DeleteConfigurationSetWithContext(gomock.Any(), gomock.Any()).Times(0)

		require.NoError(t, service.DeleteSendingResources(context.Background(), "ws", "int-1",
			&domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: settings}))
	})

	t.Run("a failure never blocks deletion", func(t *testing.T) {
		service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

		mockSESv2.EXPECT().GetTenant(gomock.Any(), gomock.Any()).Return(nil, accessDenied())
		mockSES.EXPECT().DeleteConfigurationSetWithContext(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("boom"))
		mockSESv2.EXPECT().DeleteTenant(gomock.Any(), gomock.Any()).Return(nil, accessDenied())

		require.NoError(t, service.DeleteSendingResources(context.Background(), "ws", "int-1",
			&domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: managedSettings()}))
	})
}

// TestAssociateExistingTenant covers the re-association that runs after webhooks are registered.
// Registration may have just recreated the configuration set, which AWS treats as a brand new
// resource with no tenant association — leaving every send rejected until it is restored.
func TestAssociateExistingTenant(t *testing.T) {
	t.Run("re-attaches the configuration set without creating anything", func(t *testing.T) {
		service, _, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

		settings := sesSettings()
		settings.ManagedTenantName = "notifuse-int-1"
		settings.TenantIsolationEnabled = true

		mockSESv2.EXPECT().GetTenant(gomock.Any(), gomock.Any()).
			Return(&sesv2.GetTenantOutput{Tenant: &sesv2types.Tenant{TenantArn: awsv2.String(testTenantARN)}}, nil)
		mockSESv2.EXPECT().CreateTenantResourceAssociation(gomock.Any(), gomock.Any()).
			Return(&sesv2.CreateTenantResourceAssociationOutput{}, nil)

		// Creating a tenant is billable and belongs to an explicit, confirmed action.
		mockSESv2.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).Times(0)

		result, err := service.AssociateExistingTenant(context.Background(), *settings, "int-1", nil)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.ConfigurationSetAssociated)
	})

	t.Run("does nothing when no tenant is configured", func(t *testing.T) {
		service, _, _, _, _, _ := createMockSESServiceWithV2(t)

		result, err := service.AssociateExistingTenant(context.Background(), *sesSettings(), "int-1", nil)

		require.NoError(t, err)
		assert.Nil(t, result)
	})
}
