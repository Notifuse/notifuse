package domain

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gapValidSettings() WorkspaceSettings {
	return WorkspaceSettings{
		Timezone:        "UTC",
		DefaultLanguage: "en",
		Languages:       []string{"en"},
	}
}

func TestWorkspaceSettingsValidateGap_CustomEndpointAndDownstream(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("invalid custom endpoint URL", func(t *testing.T) {
		ws := gapValidSettings()
		bad := "not a url"
		ws.CustomEndpointURL = &bad
		err := ws.Validate(passphrase)
		require.Error(t, err)
		assert.EqualError(t, err, "invalid custom endpoint URL: not a url")
	})

	t.Run("custom endpoint with non-http scheme", func(t *testing.T) {
		ws := gapValidSettings()
		ftp := "ftp://example.com"
		ws.CustomEndpointURL = &ftp
		err := ws.Validate(passphrase)
		require.Error(t, err)
		assert.EqualError(t, err, "custom endpoint URL must use http or https scheme: ftp://example.com")
	})

	t.Run("empty custom endpoint is skipped", func(t *testing.T) {
		ws := gapValidSettings()
		empty := ""
		ws.CustomEndpointURL = &empty
		assert.NoError(t, ws.Validate(passphrase))
	})

	t.Run("valid https custom endpoint passes", func(t *testing.T) {
		ws := gapValidSettings()
		good := "https://api.example.com"
		ws.CustomEndpointURL = &good
		assert.NoError(t, ws.Validate(passphrase))
	})
}

func TestWorkspaceSettingsValidateGap_FileManagerAndWebAnalyticsWrap(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("file manager failure is wrapped and downstream language checks still run ordering", func(t *testing.T) {
		ws := gapValidSettings()
		ws.FileManager = FileManagerSettings{Endpoint: "https://s3.amazonaws.com"}
		err := ws.Validate(passphrase)
		require.Error(t, err)
		assert.EqualError(t, err, "invalid file manager settings: bucket is required when file manager is configured")
	})

	t.Run("web analytics failure is wrapped exactly", func(t *testing.T) {
		ws := gapValidSettings()
		ws.WebAnalytics = &WebAnalyticsSettings{BounceThresholdSeconds: -1}
		err := ws.Validate(passphrase)
		require.Error(t, err)
		assert.EqualError(t, err, "invalid web analytics settings: bounce_threshold_seconds must be >= 0")
	})

	t.Run("web analytics geo precision boundary", func(t *testing.T) {
		ws := gapValidSettings()
		ws.WebAnalytics = &WebAnalyticsSettings{GeoCoordsPrecision: 3}
		err := ws.Validate(passphrase)
		require.Error(t, err)
		assert.EqualError(t, err, "invalid web analytics settings: geo_coordinates_precision must be between 0 and 2")
	})
}

func TestIntegrationValidateGap_LLMFirecrawlZapier(t *testing.T) {
	passphrase := "test-passphrase"
	base := Integration{ID: "int1", Name: "My Integration"}

	t.Run("llm settings required", func(t *testing.T) {
		integ := base
		integ.Type = IntegrationTypeLLM
		err := integ.Validate(passphrase)
		require.Error(t, err)
		assert.EqualError(t, err, "llm provider settings are required for llm integration")
	})

	t.Run("llm invalid kind is wrapped", func(t *testing.T) {
		integ := base
		integ.Type = IntegrationTypeLLM
		integ.LLMProvider = &LLMProvider{}
		err := integ.Validate(passphrase)
		require.Error(t, err)
		assert.EqualError(t, err, "invalid llm provider settings: LLM provider kind is required")
	})

	t.Run("llm anthropic missing settings is wrapped", func(t *testing.T) {
		integ := base
		integ.Type = IntegrationTypeLLM
		integ.LLMProvider = &LLMProvider{Kind: LLMProviderKindAnthropic}
		err := integ.Validate(passphrase)
		require.Error(t, err)
		assert.EqualError(t, err, "invalid llm provider settings: Anthropic settings required when LLM provider kind is anthropic")
	})

	t.Run("firecrawl settings required", func(t *testing.T) {
		integ := base
		integ.Type = IntegrationTypeFirecrawl
		err := integ.Validate(passphrase)
		require.Error(t, err)
		assert.EqualError(t, err, "firecrawl settings are required for firecrawl integration")
	})

	t.Run("firecrawl missing api key is wrapped", func(t *testing.T) {
		integ := base
		integ.Type = IntegrationTypeFirecrawl
		integ.FirecrawlSettings = &FirecrawlSettings{}
		err := integ.Validate(passphrase)
		require.Error(t, err)
		assert.EqualError(t, err, "invalid firecrawl settings: API key is required for Firecrawl configuration")
	})

	t.Run("zapier empty email", func(t *testing.T) {
		integ := base
		integ.Type = IntegrationTypeZapier
		integ.ZapierSettings = &ZapierSettings{}
		err := integ.Validate(passphrase)
		require.Error(t, err)
		assert.EqualError(t, err, "invalid zapier settings: api key email is required for Zapier configuration")
	})

	t.Run("zapier email without at sign", func(t *testing.T) {
		integ := base
		integ.Type = IntegrationTypeZapier
		integ.ZapierSettings = &ZapierSettings{APIKeyEmail: "not-an-email"}
		err := integ.Validate(passphrase)
		require.Error(t, err)
		assert.EqualError(t, err, "invalid zapier settings: api key email must be an email address")
	})
}

func TestIntegrationScanGap_NilAndTypeAssertion(t *testing.T) {
	t.Run("nil value is a no-op", func(t *testing.T) {
		var integ Integration
		require.NoError(t, integ.Scan(nil))
		assert.Equal(t, Integration{}, integ)
	})

	t.Run("non-byte value fails with exact message", func(t *testing.T) {
		var integ Integration
		err := integ.Scan("a string")
		require.Error(t, err)
		assert.EqualError(t, err, "type assertion to []byte failed")
	})

	t.Run("int value fails with exact message", func(t *testing.T) {
		var integ Integration
		err := integ.Scan(42)
		require.Error(t, err)
		assert.EqualError(t, err, "type assertion to []byte failed")
	})

	t.Run("invalid JSON propagates error and leaves zero value", func(t *testing.T) {
		var integ Integration
		err := integ.Scan([]byte("{not json"))
		require.Error(t, err)
		assert.Empty(t, integ.ID)
	})
}

func TestIntegrationAfterLoadGap_DecryptFailures(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("email SES bad ciphertext", func(t *testing.T) {
		integ := Integration{
			ID:   "int1",
			Name: "Email",
			Type: IntegrationTypeEmail,
			EmailProvider: EmailProvider{
				Kind: EmailProviderKindSES,
				SES:  &AmazonSESSettings{EncryptedSecretKey: "nothex!!"},
			},
		}
		err := integ.AfterLoad(passphrase)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt integration provider secrets: failed to decrypt SES secret key:")
	})

	t.Run("supabase bad ciphertext", func(t *testing.T) {
		integ := Integration{
			ID:   "int1",
			Name: "Supabase",
			Type: IntegrationTypeSupabase,
			SupabaseSettings: &SupabaseIntegrationSettings{
				AuthEmailHook: SupabaseAuthEmailHookSettings{EncryptedSignatureKey: "nothex!!"},
			},
		}
		err := integ.AfterLoad(passphrase)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt supabase signature keys: failed to decrypt auth email hook signature key:")
	})

	t.Run("llm anthropic bad ciphertext", func(t *testing.T) {
		integ := Integration{
			ID:   "int1",
			Name: "LLM",
			Type: IntegrationTypeLLM,
			LLMProvider: &LLMProvider{
				Kind:      LLMProviderKindAnthropic,
				Anthropic: &AnthropicSettings{EncryptedAPIKey: "nothex!!"},
			},
		}
		err := integ.AfterLoad(passphrase)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt llm provider secrets: failed to decrypt Anthropic API key:")
	})

	t.Run("firecrawl bad ciphertext", func(t *testing.T) {
		integ := Integration{
			ID:                "int1",
			Name:              "Firecrawl",
			Type:              IntegrationTypeFirecrawl,
			FirecrawlSettings: &FirecrawlSettings{EncryptedAPIKey: "nothex!!"},
		}
		err := integ.AfterLoad(passphrase)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt firecrawl secret keys: failed to decrypt Firecrawl API key:")
	})

	t.Run("zapier type is a no-op on AfterLoad", func(t *testing.T) {
		integ := Integration{
			ID:             "int1",
			Name:           "Zapier",
			Type:           IntegrationTypeZapier,
			ZapierSettings: &ZapierSettings{APIKeyEmail: "zapier-x@example.com"},
		}
		assert.NoError(t, integ.AfterLoad(passphrase))
		assert.Equal(t, "zapier-x@example.com", integ.ZapierSettings.APIKeyEmail)
	})
}

func TestIntegrationBeforeSaveGap_NilSettingsSkipAndClearHints(t *testing.T) {
	passphrase := "test-passphrase"

	for _, typ := range []IntegrationType{IntegrationTypeSupabase, IntegrationTypeLLM, IntegrationTypeFirecrawl, IntegrationTypeZapier} {
		t.Run("nil settings skipped for "+string(typ), func(t *testing.T) {
			integ := Integration{
				ID:              "int1",
				Name:            "X",
				Type:            typ,
				CredentialHints: map[string]string{"k": "hint"},
			}
			require.NoError(t, integ.BeforeSave(passphrase))
			assert.Nil(t, integ.CredentialHints)
		})
	}

	t.Run("firecrawl plaintext is encrypted and cleared", func(t *testing.T) {
		integ := Integration{
			ID:                "int1",
			Name:              "Firecrawl",
			Type:              IntegrationTypeFirecrawl,
			FirecrawlSettings: &FirecrawlSettings{APIKey: "fc-secret"},
		}
		require.NoError(t, integ.BeforeSave(passphrase))
		assert.Empty(t, integ.FirecrawlSettings.APIKey)
		assert.NotEmpty(t, integ.FirecrawlSettings.EncryptedAPIKey)
		decrypted, err := crypto.DecryptFromHexString(integ.FirecrawlSettings.EncryptedAPIKey, passphrase)
		require.NoError(t, err)
		assert.Equal(t, "fc-secret", decrypted)
	})
}

func TestWorkspaceValidateGap_IntegrationWrap(t *testing.T) {
	passphrase := "test-passphrase"
	ws := Workspace{
		ID:       "test123",
		Name:     "Test",
		Settings: gapValidSettings(),
		Integrations: []Integration{
			{ID: "int1", Name: "Bad", Type: IntegrationType("bogus")},
		},
	}
	err := ws.Validate(passphrase)
	require.Error(t, err)
	assert.EqualError(t, err, "invalid integration (int1): unsupported integration type: bogus")
}

func TestWorkspaceAfterLoadGap_FileManagerAndIntegrationFailures(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("file manager bad ciphertext is wrapped twice", func(t *testing.T) {
		ws := Workspace{ID: "test123", Name: "Test"}
		ws.Settings.FileManager.EncryptedSecretKey = "nothex!!"
		ws.Settings.EncryptedSecretKey = "nothex!!"
		err := ws.AfterLoad(passphrase)
		require.Error(t, err)
		// FileManager decrypt runs first, so the workspace secret is never attempted.
		assert.Contains(t, err.Error(), "failed to decrypt secret key: failed to decrypt secret key:")
		assert.Empty(t, ws.Settings.SecretKey)
	})

	t.Run("integration failure is wrapped with integration ID after workspace secret decrypts", func(t *testing.T) {
		encrypted, err := crypto.EncryptString("workspace-secret", passphrase)
		require.NoError(t, err)
		ws := Workspace{
			ID:   "test123",
			Name: "Test",
			Integrations: []Integration{
				{
					ID:                "int1",
					Name:              "Firecrawl",
					Type:              IntegrationTypeFirecrawl,
					FirecrawlSettings: &FirecrawlSettings{EncryptedAPIKey: "nothex!!"},
				},
			},
		}
		ws.Settings.EncryptedSecretKey = encrypted
		err = ws.AfterLoad(passphrase)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to process integration int1: failed to decrypt firecrawl secret keys:")
		// The workspace secret itself was decrypted before the integration failed.
		assert.Equal(t, "workspace-secret", ws.Settings.SecretKey)
	})
}

func TestUserWorkspaceValidateGap_EmptyIDs(t *testing.T) {
	t.Run("missing user_id", func(t *testing.T) {
		uw := UserWorkspace{WorkspaceID: "ws1", Role: "owner"}
		assert.EqualError(t, uw.Validate(), "invalid user workspace: user_id is required")
	})
	t.Run("missing workspace_id", func(t *testing.T) {
		uw := UserWorkspace{UserID: "u1", Role: "owner"}
		assert.EqualError(t, uw.Validate(), "invalid user workspace: workspace_id is required")
	})
}

func TestErrTeamMemberLimitReachedGap_ErrorString(t *testing.T) {
	e := &ErrTeamMemberLimitReached{Limit: 5, Current: 7}
	assert.Equal(t, "team member limit reached: workspace has 7 members and invitations (limit: 5)", e.Error())
}

func TestCreateIntegrationRequestValidateGap_LLMFirecrawl(t *testing.T) {
	passphrase := "test-passphrase"
	base := CreateIntegrationRequest{WorkspaceID: "ws1", Name: "My Integration"}

	t.Run("llm settings required", func(t *testing.T) {
		req := base
		req.Type = IntegrationTypeLLM
		assert.EqualError(t, req.Validate(passphrase), "llm provider settings are required for llm integration")
	})

	t.Run("llm invalid is wrapped", func(t *testing.T) {
		req := base
		req.Type = IntegrationTypeLLM
		req.LLMProvider = &LLMProvider{Kind: LLMProviderKind("bogus")}
		assert.EqualError(t, req.Validate(passphrase), "invalid llm provider configuration: invalid LLM provider kind: bogus")
	})

	t.Run("firecrawl settings required", func(t *testing.T) {
		req := base
		req.Type = IntegrationTypeFirecrawl
		assert.EqualError(t, req.Validate(passphrase), "firecrawl settings are required for firecrawl integration")
	})

	t.Run("firecrawl invalid is wrapped", func(t *testing.T) {
		req := base
		req.Type = IntegrationTypeFirecrawl
		req.FirecrawlSettings = &FirecrawlSettings{}
		assert.EqualError(t, req.Validate(passphrase), "invalid firecrawl settings: API key is required for Firecrawl configuration")
	})
}

func TestUpdateIntegrationRequestGap_UnmarshalAndValidateBranches(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("malformed JSON fails UnmarshalJSON", func(t *testing.T) {
		var req UpdateIntegrationRequest
		err := json.Unmarshal([]byte("{not-json"), &req)
		require.Error(t, err)
	})

	t.Run("llm invalid is wrapped when provider kind empty", func(t *testing.T) {
		req := UpdateIntegrationRequest{
			WorkspaceID:   "ws1",
			IntegrationID: "int1",
			Name:          "X",
			LLMProvider:   &LLMProvider{},
		}
		assert.EqualError(t, req.Validate(passphrase), "invalid llm provider configuration: LLM provider kind is required")
	})

	t.Run("firecrawl invalid is wrapped when earlier branches absent", func(t *testing.T) {
		req := UpdateIntegrationRequest{
			WorkspaceID:       "ws1",
			IntegrationID:     "int1",
			Name:              "X",
			FirecrawlSettings: &FirecrawlSettings{},
		}
		assert.EqualError(t, req.Validate(passphrase), "invalid firecrawl settings: API key is required for Firecrawl configuration")
	})

	t.Run("provider kind takes precedence over LLM and does not reach it", func(t *testing.T) {
		// Provider.Kind is set, so the LLM branch must not run: the error must
		// be the provider's, proving the else-if chain short-circuits.
		req := UpdateIntegrationRequest{
			WorkspaceID:   "ws1",
			IntegrationID: "int1",
			Name:          "X",
			Provider:      EmailProvider{Kind: EmailProviderKindSES},
			LLMProvider:   &LLMProvider{},
		}
		err := req.Validate(passphrase)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid provider configuration:")
		assert.NotContains(t, err.Error(), "llm provider")
	})
}

func TestUpdateWorkspaceRequestGap_UnmarshalErrorAndIDLength(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("malformed JSON fails UnmarshalJSON", func(t *testing.T) {
		var req UpdateWorkspaceRequest
		err := json.Unmarshal([]byte("{not-json"), &req)
		require.Error(t, err)
	})

	t.Run("ID over 32 chars fails with exact message", func(t *testing.T) {
		req := UpdateWorkspaceRequest{
			ID:       strings.Repeat("a", 33),
			Name:     "Test",
			Settings: gapValidSettings(),
		}
		assert.EqualError(t, req.Validate(passphrase), "invalid update workspace request: id length must be between 1 and 32")
	})

	t.Run("ID of exactly 32 chars passes the ID check", func(t *testing.T) {
		req := UpdateWorkspaceRequest{
			ID:       strings.Repeat("a", 32),
			Name:     "Test",
			Settings: gapValidSettings(),
		}
		assert.NoError(t, req.Validate(passphrase))
	})
}

func TestDeleteWorkspaceRequestValidateGap_Boundaries(t *testing.T) {
	t.Run("non-alphanumeric ID", func(t *testing.T) {
		req := DeleteWorkspaceRequest{ID: "bad-id!"}
		assert.EqualError(t, req.Validate(), "invalid delete workspace request: id must be alphanumeric")
	})
	t.Run("ID over 32 chars", func(t *testing.T) {
		req := DeleteWorkspaceRequest{ID: strings.Repeat("a", 33)}
		assert.EqualError(t, req.Validate(), "invalid delete workspace request: id length must be between 1 and 32")
	})
	t.Run("ID of exactly 32 chars is valid", func(t *testing.T) {
		req := DeleteWorkspaceRequest{ID: strings.Repeat("a", 32)}
		assert.NoError(t, req.Validate())
	})
}

func TestSetBlogSettingsRequestGap_UnmarshalError(t *testing.T) {
	var req SetBlogSettingsRequest
	err := json.Unmarshal([]byte("{not-json"), &req)
	require.Error(t, err)
}

func TestSetUserPermissionsAndInviteGap_Branches(t *testing.T) {
	t.Run("set user permissions propagates unknown resource error unwrapped", func(t *testing.T) {
		req := SetUserPermissionsRequest{
			WorkspaceID: "ws1",
			UserID:      "u1",
			Permissions: UserPermissions{PermissionResource("bogus_resource"): {Read: true}},
		}
		assert.EqualError(t, req.Validate(), "unknown permission resource: bogus_resource")
	})

	t.Run("invite non-alphanumeric workspace ID", func(t *testing.T) {
		req := InviteMemberRequest{WorkspaceID: "bad id!", Email: "a@example.com", Permissions: NewFullPermissions()}
		assert.EqualError(t, req.Validate(), "invalid invite member request: workspace_id must be alphanumeric")
	})

	t.Run("invite workspace ID over 32 chars", func(t *testing.T) {
		req := InviteMemberRequest{WorkspaceID: strings.Repeat("a", 33), Email: "a@example.com", Permissions: NewFullPermissions()}
		assert.EqualError(t, req.Validate(), "invalid invite member request: workspace_id length must be between 1 and 32")
	})
}

func TestSESValidateGap_TenantBranches(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("tenant isolation conflicts with explicit tenant", func(t *testing.T) {
		s := AmazonSESSettings{TenantIsolationEnabled: true, TenantName: "my-tenant"}
		assert.EqualError(t, s.Validate(passphrase), "tenant isolation cannot be enabled while an explicit tenant name is set: clear one of them")
	})

	t.Run("invalid configuration set name", func(t *testing.T) {
		s := AmazonSESSettings{ConfigurationSetName: "bad name!"}
		assert.EqualError(t, s.Validate(passphrase), "invalid configuration set name: up to 64 letters, numbers, hyphens or underscores")
	})

	t.Run("configuration set name of 65 chars is invalid", func(t *testing.T) {
		s := AmazonSESSettings{ConfigurationSetName: strings.Repeat("a", 65), Region: "us-east-1", AccessKey: "ak"}
		assert.EqualError(t, s.Validate(passphrase), "invalid configuration set name: up to 64 letters, numbers, hyphens or underscores")
	})

	t.Run("configuration set name of exactly 64 chars passes that check", func(t *testing.T) {
		s := AmazonSESSettings{ConfigurationSetName: strings.Repeat("a", 64), Region: "us-east-1", AccessKey: "ak"}
		assert.NoError(t, s.Validate(passphrase))
	})

	t.Run("invalid tenant name", func(t *testing.T) {
		s := AmazonSESSettings{TenantName: "bad name!"}
		assert.EqualError(t, s.Validate(passphrase), "invalid tenant name: up to 64 letters, numbers, hyphens or underscores")
	})

	t.Run("tenant name alone counts as configured so region is required", func(t *testing.T) {
		s := AmazonSESSettings{TenantName: "my-tenant"}
		assert.EqualError(t, s.Validate(passphrase), "region is required when Amazon SES is configured")
	})
}

func TestPostmarkMailgunValidateGap(t *testing.T) {
	passphrase := "test-passphrase"

	t.Run("postmark defaults empty stream and encrypts token without clearing it", func(t *testing.T) {
		p := PostmarkSettings{ServerToken: "tok-123"}
		require.NoError(t, p.Validate(passphrase))
		assert.Equal(t, "outbound", p.MessageStream)
		assert.NotEmpty(t, p.EncryptedServerToken)
		// Real contract: Postmark.Validate does not clear the plaintext token
		// (unlike Mailgun); pinning that here guards against a silent change.
		assert.Equal(t, "tok-123", p.ServerToken)
		decrypted, err := crypto.DecryptFromHexString(p.EncryptedServerToken, passphrase)
		require.NoError(t, err)
		assert.Equal(t, "tok-123", decrypted)
	})

	t.Run("postmark preserves explicit stream", func(t *testing.T) {
		p := PostmarkSettings{MessageStream: "broadcasts"}
		require.NoError(t, p.Validate(passphrase))
		assert.Equal(t, "broadcasts", p.MessageStream)
		assert.Empty(t, p.EncryptedServerToken)
	})

	t.Run("mailgun missing domain fails before encryption runs", func(t *testing.T) {
		m := MailgunSettings{APIKey: "key-secret"}
		err := m.Validate(passphrase)
		require.Error(t, err)
		assert.EqualError(t, err, "domain is required for Mailgun configuration")
		// Guard failure must mean the encryption side effect never happened.
		assert.Empty(t, m.EncryptedAPIKey)
		assert.Equal(t, "key-secret", m.APIKey)
	})

	t.Run("mailgun valid config encrypts and clears plaintext", func(t *testing.T) {
		m := MailgunSettings{Domain: "mg.example.com", APIKey: "key-secret"}
		require.NoError(t, m.Validate(passphrase))
		assert.Empty(t, m.APIKey)
		assert.NotEmpty(t, m.EncryptedAPIKey)
		decrypted, err := crypto.DecryptFromHexString(m.EncryptedAPIKey, passphrase)
		require.NoError(t, err)
		assert.Equal(t, "key-secret", decrypted)
	})
}
