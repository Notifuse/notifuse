package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/smithy-go"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tenantTestRequest(provider *domain.EmailProvider) domain.SendEmailProviderRequest {
	return domain.SendEmailProviderRequest{
		WorkspaceID:   "ws",
		IntegrationID: "int-1",
		MessageID:     "msg-1",
		FromAddress:   "from@example.com",
		FromName:      "From",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       "<p>hi</p>",
		Provider:      provider,
	}
}

func sesSettings() *domain.AmazonSESSettings {
	return &domain.AmazonSESSettings{
		AccessKey: "test-access-key",
		SecretKey: "test-secret-key",
		Region:    "us-east-1",
	}
}

// TestSESService_SendEmail_SetsTenantName covers the whole point of the SDK v2 migration:
// TenantName exists only on the v2 API, and it must never be sent as an empty string.
func TestSESService_SendEmail_SetsTenantName(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*domain.AmazonSESSettings)
		expected *string
	}{
		{
			name:     "no tenant configured",
			mutate:   func(s *domain.AmazonSESSettings) {},
			expected: nil,
		},
		{
			name: "managed tenant, isolation on",
			mutate: func(s *domain.AmazonSESSettings) {
				s.TenantIsolationEnabled = true
				s.ManagedTenantName = "notifuse-int-1"
			},
			expected: aws.String("notifuse-int-1"),
		},
		{
			// Turning the switch off must take effect on the very next send.
			name: "managed tenant, isolation switched off",
			mutate: func(s *domain.AmazonSESSettings) {
				s.TenantIsolationEnabled = false
				s.ManagedTenantName = "notifuse-int-1"
			},
			expected: nil,
		},
		{
			name:     "manual tenant",
			mutate:   func(s *domain.AmazonSESSettings) { s.TenantName = "team-acme" },
			expected: aws.String("team-acme"),
		},
		{
			name: "manual tenant wins over managed",
			mutate: func(s *domain.AmazonSESSettings) {
				s.TenantIsolationEnabled = true
				s.TenantName = "team-acme"
				s.ManagedTenantName = "notifuse-int-1"
			},
			expected: aws.String("team-acme"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

			settings := sesSettings()
			settings.ManagedConfigurationSet = "notifuse-int-1" // avoids any lookup
			tc.mutate(settings)

			mockSESv2.EXPECT().SendEmail(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
					if tc.expected == nil {
						assert.Nil(t, input.TenantName)
					} else {
						require.NotNil(t, input.TenantName)
						assert.Equal(t, *tc.expected, *input.TenantName)
					}
					return &sesv2.SendEmailOutput{}, nil
				})

			require.NoError(t, service.SendEmail(context.Background(),
				tenantTestRequest(&domain.EmailProvider{SES: settings})))
		})
	}
}

// TestSESService_SendEmail_UsesConfiguredConfigurationSet asserts the send path stays off
// ListConfigurationSets, which AWS throttles to once per second per account and region.
func TestSESService_SendEmail_UsesConfiguredConfigurationSet(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings func() *domain.AmazonSESSettings
		expected string
	}{
		{
			name: "operator override",
			settings: func() *domain.AmazonSESSettings {
				s := sesSettings()
				s.ConfigurationSetName = "custom-set"
				return s
			},
			expected: "custom-set",
		},
		{
			name: "persisted managed name",
			settings: func() *domain.AmazonSESSettings {
				s := sesSettings()
				s.ManagedConfigurationSet = "notifuse-int-1"
				return s
			},
			expected: "notifuse-int-1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

			// The lookup must not happen at all.
			mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).Times(0)

			mockSESv2.EXPECT().SendEmail(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
					require.NotNil(t, input.ConfigurationSetName)
					assert.Equal(t, tc.expected, *input.ConfigurationSetName)
					return &sesv2.SendEmailOutput{}, nil
				})

			require.NoError(t, service.SendEmail(context.Background(),
				tenantTestRequest(&domain.EmailProvider{SES: tc.settings()})))
		})
	}
}

// TestSESService_ResolveConfigurationSet_CachesResults pins the memoisation that keeps legacy
// integrations off the once-per-second lookup.
func TestSESService_ResolveConfigurationSet_CachesResults(t *testing.T) {
	t.Run("positive result is looked up once", func(t *testing.T) {
		service, mockSES, _, _, _, _ := createMockSESServiceWithV2(t)

		mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
			Return(&ses.ListConfigurationSetsOutput{
				ConfigurationSets: []*ses.ConfigurationSet{{Name: aws.String("notifuse-int-1")}},
			}, nil).
			Times(1)

		for i := 0; i < 3; i++ {
			got := service.resolveConfigurationSet(context.Background(), *sesSettings(), "int-1")
			assert.Equal(t, "notifuse-int-1", got)
		}
	})

	t.Run("miss is cached only briefly", func(t *testing.T) {
		service, mockSES, _, _, _, _ := createMockSESServiceWithV2(t)

		mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
			Return(&ses.ListConfigurationSetsOutput{ConfigurationSets: []*ses.ConfigurationSet{}}, nil).
			Times(1)

		assert.Empty(t, service.resolveConfigurationSet(context.Background(), *sesSettings(), "int-1"))
		// Second call inside the TTL must not hit AWS again.
		assert.Empty(t, service.resolveConfigurationSet(context.Background(), *sesSettings(), "int-1"))

		// Once the entry expires the lookup runs again, so registering webhooks is picked up.
		service.configSetCache.Store("int-1|us-east-1", configSetCacheEntry{})
		service.invalidateConfigurationSetCache("int-1", "us-east-1")
		_, cached := service.configSetCache.Load("int-1|us-east-1")
		assert.False(t, cached, "invalidation must drop the entry")
	})

	t.Run("a throttled lookup is not cached as a miss", func(t *testing.T) {
		service, mockSES, _, _, _, _ := createMockSESServiceWithV2(t)

		// AWS allows ListConfigurationSets once per second; a throttle must not be mistaken
		// for "the configuration set does not exist".
		mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
			Return(nil, &smithy.GenericAPIError{Code: "TooManyRequestsException"}).
			Times(2)

		assert.Empty(t, service.resolveConfigurationSet(context.Background(), *sesSettings(), "int-1"))
		assert.Empty(t, service.resolveConfigurationSet(context.Background(), *sesSettings(), "int-1"))
	})

	t.Run("concurrent sends collapse into one lookup", func(t *testing.T) {
		service, mockSES, _, _, _, _ := createMockSESServiceWithV2(t)

		release := make(chan struct{})
		mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *ses.ListConfigurationSetsInput, _ ...interface{}) (*ses.ListConfigurationSetsOutput, error) {
				<-release // hold the call open so every goroutine piles onto this one
				return &ses.ListConfigurationSetsOutput{
					ConfigurationSets: []*ses.ConfigurationSet{{Name: aws.String("notifuse-int-1")}},
				}, nil
			}).
			Times(1)

		// Every goroutine must be running before the lookup is allowed to return, otherwise the
		// stragglers simply read the answer the first one cached and the collapse is never
		// exercised.
		var started, wg sync.WaitGroup
		results := make([]string, 50)
		for i := 0; i < 50; i++ {
			started.Add(1)
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				started.Done()
				results[idx] = service.resolveConfigurationSet(context.Background(), *sesSettings(), "int-1")
			}(i)
		}
		started.Wait()
		close(release)
		wg.Wait()

		for _, got := range results {
			assert.Equal(t, "notifuse-int-1", got)
		}
	})
}

// TestSESService_ListConfigurationSets_FollowsNextToken guards against silently truncating an
// account with more than one page of configuration sets, which would make an existing set look
// absent and drop event tracking.
func TestSESService_ListConfigurationSets_FollowsNextToken(t *testing.T) {
	service, mockSES, _, _, _, _ := createMockSESServiceWithV2(t)

	gomock.InOrder(
		mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, input *ses.ListConfigurationSetsInput, _ ...interface{}) (*ses.ListConfigurationSetsOutput, error) {
				assert.Nil(t, input.NextToken)
				return &ses.ListConfigurationSetsOutput{
					ConfigurationSets: []*ses.ConfigurationSet{{Name: aws.String("first")}},
					NextToken:         aws.String("page-2"),
				}, nil
			}),
		mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, input *ses.ListConfigurationSetsInput, _ ...interface{}) (*ses.ListConfigurationSetsOutput, error) {
				require.NotNil(t, input.NextToken)
				assert.Equal(t, "page-2", *input.NextToken)
				return &ses.ListConfigurationSetsOutput{
					ConfigurationSets: []*ses.ConfigurationSet{{Name: aws.String("second")}},
				}, nil
			}),
	)

	sets, err := service.ListConfigurationSets(context.Background(), *sesSettings())
	require.NoError(t, err)
	assert.Equal(t, []string{"first", "second"}, sets)
}

// TestSESService_SendEmail_RawPathDeliversCCWithBCC is the regression guard for a delivery bug
// that predates this change: the raw path wrote a Cc: header but built the envelope from To+BCC
// only, and only when a BCC existed — so CC recipients of a message that also had a BCC were
// never actually sent anything.
func TestSESService_SendEmail_RawPathDeliversCCWithBCC(t *testing.T) {
	service, _, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	settings := sesSettings()
	settings.ManagedConfigurationSet = "notifuse-int-1"

	mockSESv2.EXPECT().SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			require.NotNil(t, input.Content.Raw, "attachments must use the raw path")

			assert.Equal(t, []string{"to@example.com"}, input.Destination.ToAddresses)
			assert.Equal(t, []string{"cc@example.com"}, input.Destination.CcAddresses)
			assert.Equal(t, []string{"bcc@example.com"}, input.Destination.BccAddresses)

			raw := string(input.Content.Raw.Data)
			assert.Contains(t, raw, "Cc: cc@example.com", "CC still belongs in the headers")
			assert.NotContains(t, raw, "Bcc:", "BCC must stay out of the headers")

			return &sesv2.SendEmailOutput{}, nil
		})

	request := tenantTestRequest(&domain.EmailProvider{SES: settings})
	request.EmailOptions = domain.EmailOptions{
		CC:  []string{"cc@example.com"},
		BCC: []string{"bcc@example.com"},
		Attachments: []domain.Attachment{{
			Filename:    "note.txt",
			Content:     "aGVsbG8=", // "hello"
			ContentType: "text/plain",
		}},
	}

	require.NoError(t, service.SendEmail(context.Background(), request))
}

// TestSESService_SendEmail_BuildsExpectedInput asserts the whole translated request in one place.
// A field silently dropped in an SDK migration is exactly the failure per-field assertions
// scattered across tests do not catch.
func TestSESService_SendEmail_BuildsExpectedInput(t *testing.T) {
	service, _, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

	settings := sesSettings()
	settings.ManagedConfigurationSet = "notifuse-int-1"
	settings.ManagedTenantName = "notifuse-int-1"
	settings.TenantIsolationEnabled = true

	var captured *sesv2.SendEmailInput
	mockSESv2.EXPECT().SendEmail(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
			captured = input
			return &sesv2.SendEmailOutput{}, nil
		})

	request := tenantTestRequest(&domain.EmailProvider{SES: settings})
	request.EmailOptions = domain.EmailOptions{
		CC:      []string{"cc@example.com"},
		BCC:     []string{"bcc@example.com"},
		ReplyTo: "reply@example.com",
	}

	require.NoError(t, service.SendEmail(context.Background(), request))
	require.NotNil(t, captured)

	assert.Equal(t, "From <from@example.com>", aws.StringValue(captured.FromEmailAddress))
	assert.Equal(t, []string{"to@example.com"}, captured.Destination.ToAddresses)
	assert.Equal(t, []string{"cc@example.com"}, captured.Destination.CcAddresses)
	assert.Equal(t, []string{"bcc@example.com"}, captured.Destination.BccAddresses)
	assert.Equal(t, []string{"reply@example.com"}, captured.ReplyToAddresses)
	assert.Equal(t, "notifuse-int-1", aws.StringValue(captured.ConfigurationSetName))
	assert.Equal(t, "notifuse-int-1", aws.StringValue(captured.TenantName))
	assert.Equal(t, "Subject", aws.StringValue(captured.Content.Simple.Subject.Data))
	assert.Equal(t, "<p>hi</p>", aws.StringValue(captured.Content.Simple.Body.Html.Data))
	require.Len(t, captured.EmailTags, 1)
	assert.Equal(t, "notifuse_message_id", aws.StringValue(captured.EmailTags[0].Name))
	assert.Equal(t, "msg-1", aws.StringValue(captured.EmailTags[0].Value))
	assert.Nil(t, captured.Content.Raw)
}

func TestWrapSESError(t *testing.T) {
	t.Run("API errors keep the SES error prefix", func(t *testing.T) {
		err := wrapSESError(&smithy.GenericAPIError{Code: "MessageRejected", Message: "nope"}, "send email")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SES error")
		assert.Contains(t, err.Error(), "MessageRejected")
	})

	t.Run("unwraps through the v2 operation wrapper", func(t *testing.T) {
		// A bare type assertion, which is what the v1 code did, would miss this.
		wrapped := &smithy.OperationError{
			ServiceID:     "SESv2",
			OperationName: "SendEmail",
			Err:           &smithy.GenericAPIError{Code: "AccountSuspendedException"},
		}
		err := wrapSESError(fmt.Errorf("send: %w", wrapped), "send email")
		assert.Contains(t, err.Error(), "SES error")
		assert.Contains(t, err.Error(), "AccountSuspendedException")
	})

	t.Run("non-API errors say what failed", func(t *testing.T) {
		err := wrapSESError(errors.New("connection reset"), "send raw email")
		assert.True(t, strings.HasPrefix(err.Error(), "failed to send raw email"))
	})
}

// TestSESService_GetWebhookStatus_OverrideSetWithNoDestinations pins the panic guard: an
// operator-supplied configuration set normally has no event destination until webhooks are
// registered, and indexing that empty slice took the whole request down.
func TestSESService_GetWebhookStatus_OverrideSetWithNoDestinations(t *testing.T) {
	service, mockSES, _, _, _, _ := createMockSESServiceWithV2(t)

	settings := sesSettings()
	settings.ConfigurationSetName = "operator-set"
	provider := &domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: settings}

	mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{{Name: aws.String("operator-set")}},
		}, nil).AnyTimes()
	mockSES.EXPECT().DescribeConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeConfigurationSetOutput{EventDestinations: nil}, nil).AnyTimes()
	mockSES.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeActiveReceiptRuleSetOutput{}, nil).AnyTimes()

	status, err := service.GetWebhookStatus(context.Background(), "ws", "int-1", provider)

	require.NoError(t, err, "must not panic or error on a destination-less configuration set")
	require.NotNil(t, status)
	assert.False(t, status.IsRegistered)
	assert.Equal(t, "operator-set", status.ProviderDetails["configuration_set"])
	assert.Equal(t, false, status.ProviderDetails["configuration_set_managed"])
}

// TestSESService_UnregisterWebhooks_KeepsConfigurationSetWhenTenantInPlay is the regression
// guard for "unregistering webhooks silently took sending down": with a tenant configured, the
// send needs a configuration set, and SES refuses to delete one that is tenant-associated.
func TestSESService_UnregisterWebhooks_KeepsConfigurationSetWhenTenantInPlay(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*domain.AmazonSESSettings)
	}{
		{"managed tenant", func(s *domain.AmazonSESSettings) { s.ManagedTenantName = "notifuse-int-1" }},
		{"manual tenant", func(s *domain.AmazonSESSettings) { s.TenantName = "team-acme" }},
		{"operator configuration set", func(s *domain.AmazonSESSettings) { s.ConfigurationSetName = "operator-set" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, mockSES, _, _, _, _ := createMockSESServiceWithV2(t)

			settings := sesSettings()
			tc.mutate(settings)
			configSet, _ := configurationSetFor(settings, "int-1")

			mockSES.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).
				Return(&ses.DescribeActiveReceiptRuleSetOutput{}, nil).AnyTimes()
			mockSES.EXPECT().DeleteReceiptRuleWithContext(gomock.Any(), gomock.Any()).
				Return(&ses.DeleteReceiptRuleOutput{}, nil).AnyTimes()
			mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
				Return(&ses.ListConfigurationSetsOutput{
					ConfigurationSets: []*ses.ConfigurationSet{{Name: aws.String(configSet)}},
				}, nil)
			mockSES.EXPECT().DescribeConfigurationSetWithContext(gomock.Any(), gomock.Any()).
				Return(&ses.DescribeConfigurationSetOutput{}, nil)

			// The whole point: the configuration set survives.
			mockSES.EXPECT().DeleteConfigurationSetWithContext(gomock.Any(), gomock.Any()).Times(0)

			err := service.UnregisterWebhooks(context.Background(), "ws", "int-1",
				&domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: settings})
			require.NoError(t, err)
		})
	}
}

// TestSESService_UnregisterWebhooks_DeletesManagedSetWithoutTenant keeps the pre-existing
// behaviour for everyone who never turns isolation on.
func TestSESService_UnregisterWebhooks_DeletesManagedSetWithoutTenant(t *testing.T) {
	service, mockSES, _, _, _, _ := createMockSESServiceWithV2(t)

	settings := sesSettings()
	mockSES.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeActiveReceiptRuleSetOutput{}, nil).AnyTimes()
	mockSES.EXPECT().DeleteReceiptRuleWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DeleteReceiptRuleOutput{}, nil).AnyTimes()
	mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.ListConfigurationSetsOutput{
			ConfigurationSets: []*ses.ConfigurationSet{{Name: aws.String("notifuse-int-1")}},
		}, nil)
	mockSES.EXPECT().DescribeConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DescribeConfigurationSetOutput{}, nil)
	mockSES.EXPECT().DeleteConfigurationSetWithContext(gomock.Any(), gomock.Any()).
		Return(&ses.DeleteConfigurationSetOutput{}, nil).Times(1)

	err := service.UnregisterWebhooks(context.Background(), "ws", "int-1",
		&domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: settings})
	require.NoError(t, err)
}

func TestConfigurationSetFor(t *testing.T) {
	managedName, managed := configurationSetFor(sesSettings(), "int-1")
	assert.Equal(t, "notifuse-int-1", managedName)
	assert.True(t, managed)

	settings := sesSettings()
	settings.ConfigurationSetName = "operator-set"
	overrideName, overrideManaged := configurationSetFor(settings, "int-1")
	assert.Equal(t, "operator-set", overrideName)
	assert.False(t, overrideManaged, "an operator-supplied set is not ours to create or delete")
}

// TestPreserveDerivedSESFields covers a bug that existed before tenants: UpdateIntegration
// assigns the whole EmailProvider from the request, so any client whose payload omits the
// server-owned SES fields used to erase them.
func TestPreserveDerivedSESFields(t *testing.T) {
	existing := &domain.Integration{
		Type: domain.IntegrationTypeEmail,
		EmailProvider: domain.EmailProvider{
			Kind: domain.EmailProviderKindSES,
			SES: &domain.AmazonSESSettings{
				ManagedConfigurationSet: "notifuse-int-1",
				ManagedTenantName:       "notifuse-int-1",
				InboundTopicARN:         "arn:aws:sns:eu-west-3:123456789012:notifuse-ses-int-1",
			},
		},
	}

	t.Run("payload omitting derived fields keeps them", func(t *testing.T) {
		updated := &domain.Integration{
			EmailProvider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES:  &domain.AmazonSESSettings{Region: "eu-west-3", AccessKey: "AKIA"},
			},
		}

		preserveDerivedSESFields(updated, existing)

		assert.Equal(t, "notifuse-int-1", updated.EmailProvider.SES.ManagedConfigurationSet)
		assert.Equal(t, "notifuse-int-1", updated.EmailProvider.SES.ManagedTenantName)
		assert.Equal(t, "arn:aws:sns:eu-west-3:123456789012:notifuse-ses-int-1", updated.EmailProvider.SES.InboundTopicARN)
	})

	t.Run("client-supplied derived values are ignored", func(t *testing.T) {
		updated := &domain.Integration{
			EmailProvider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES: &domain.AmazonSESSettings{
					ManagedConfigurationSet: "attacker-set",
					ManagedTenantName:       "attacker-tenant",
				},
			},
		}

		preserveDerivedSESFields(updated, existing)

		assert.Equal(t, "notifuse-int-1", updated.EmailProvider.SES.ManagedConfigurationSet)
		assert.Equal(t, "notifuse-int-1", updated.EmailProvider.SES.ManagedTenantName)
	})

	t.Run("operator overrides are untouched", func(t *testing.T) {
		updated := &domain.Integration{
			EmailProvider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES: &domain.AmazonSESSettings{
					ConfigurationSetName:   "operator-set",
					TenantIsolationEnabled: true,
				},
			},
		}

		preserveDerivedSESFields(updated, existing)

		assert.Equal(t, "operator-set", updated.EmailProvider.SES.ConfigurationSetName)
		assert.True(t, updated.EmailProvider.SES.TenantIsolationEnabled)
	})

	t.Run("non-SES integrations are a no-op", func(t *testing.T) {
		updated := &domain.Integration{EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP}}
		preserveDerivedSESFields(updated, existing)
		assert.Nil(t, updated.EmailProvider.SES)

		preserveDerivedSESFields(updated, nil)
		assert.Nil(t, updated.EmailProvider.SES)
	})
}

// TestSESService_ExistingDeploymentUpgradePath pins the behaviour an SES integration created
// before this release must keep. Such an integration has none of the new fields: no persisted
// configuration set, no tenant, no isolation flag. Nothing about its sending may change.
func TestSESService_ExistingDeploymentUpgradePath(t *testing.T) {
	// Exactly what an upgraded deployment loads from the database.
	legacySettings := func() *domain.AmazonSESSettings {
		return &domain.AmazonSESSettings{
			AccessKey: "test-access-key",
			SecretKey: "test-secret-key",
			Region:    "us-east-1",
		}
	}

	t.Run("still validates", func(t *testing.T) {
		require.NoError(t, legacySettings().Validate("passphrase"))
	})

	t.Run("resolves its configuration set by lookup, exactly as before", func(t *testing.T) {
		service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

		mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
			Return(&ses.ListConfigurationSetsOutput{
				ConfigurationSets: []*ses.ConfigurationSet{{Name: aws.String("notifuse-int-1")}},
			}, nil).Times(1)

		mockSESv2.EXPECT().SendEmail(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
				assert.Equal(t, "notifuse-int-1", aws.StringValue(input.ConfigurationSetName),
					"event tracking must keep working without any migration")
				assert.Nil(t, input.TenantName, "an untenanted integration must stay untenanted")
				return &sesv2.SendEmailOutput{}, nil
			})

		require.NoError(t, service.SendEmail(context.Background(),
			tenantTestRequest(&domain.EmailProvider{SES: legacySettings()})))
	})

	t.Run("sends without a configuration set when none exists, as before", func(t *testing.T) {
		// Integrations that never registered webhooks kept sending; that must not regress
		// into a hard failure.
		service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

		mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
			Return(&ses.ListConfigurationSetsOutput{ConfigurationSets: []*ses.ConfigurationSet{}}, nil)
		mockSESv2.EXPECT().SendEmail(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
				assert.Nil(t, input.ConfigurationSetName)
				assert.Nil(t, input.TenantName)
				return &sesv2.SendEmailOutput{}, nil
			})

		require.NoError(t, service.SendEmail(context.Background(),
			tenantTestRequest(&domain.EmailProvider{SES: legacySettings()})))
	})

	t.Run("keeps sending when the configuration-set lookup fails", func(t *testing.T) {
		// Graceful degradation predates this change and must survive it.
		service, mockSES, _, _, _, mockSESv2 := createMockSESServiceWithV2(t)

		mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("network unreachable"))
		mockSESv2.EXPECT().SendEmail(gomock.Any(), gomock.Any()).
			Return(&sesv2.SendEmailOutput{}, nil)

		require.NoError(t, service.SendEmail(context.Background(),
			tenantTestRequest(&domain.EmailProvider{SES: legacySettings()})))
	})

	t.Run("unregistering webhooks still deletes the managed configuration set", func(t *testing.T) {
		service, mockSES, _, _, _, _ := createMockSESServiceWithV2(t)

		mockSES.EXPECT().DescribeActiveReceiptRuleSetWithContext(gomock.Any(), gomock.Any()).
			Return(&ses.DescribeActiveReceiptRuleSetOutput{}, nil).AnyTimes()
		mockSES.EXPECT().DeleteReceiptRuleWithContext(gomock.Any(), gomock.Any()).
			Return(&ses.DeleteReceiptRuleOutput{}, nil).AnyTimes()
		mockSES.EXPECT().ListConfigurationSetsWithContext(gomock.Any(), gomock.Any()).
			Return(&ses.ListConfigurationSetsOutput{
				ConfigurationSets: []*ses.ConfigurationSet{{Name: aws.String("notifuse-int-1")}},
			}, nil)
		mockSES.EXPECT().DescribeConfigurationSetWithContext(gomock.Any(), gomock.Any()).
			Return(&ses.DescribeConfigurationSetOutput{}, nil)
		mockSES.EXPECT().DeleteConfigurationSetWithContext(gomock.Any(), gomock.Any()).
			Return(&ses.DeleteConfigurationSetOutput{}, nil).Times(1)

		require.NoError(t, service.UnregisterWebhooks(context.Background(), "ws", "int-1",
			&domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: legacySettings()}))
	})

	t.Run("an update that omits the new fields changes nothing", func(t *testing.T) {
		existing := &domain.Integration{
			Type: domain.IntegrationTypeEmail,
			EmailProvider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES:  legacySettings(),
			},
		}
		updated := &domain.Integration{
			EmailProvider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES:  legacySettings(),
			},
		}

		preserveDerivedSESFields(updated, existing)

		assert.Empty(t, updated.EmailProvider.SES.ManagedConfigurationSet)
		assert.Empty(t, updated.EmailProvider.SES.ManagedTenantName)
		assert.False(t, updated.EmailProvider.SES.TenantIsolationEnabled)
	})
}
