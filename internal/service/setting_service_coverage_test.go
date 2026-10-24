package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/crypto"
)

// covSettingStore is a recording SettingRepository mock: Set calls are
// captured in order, and one chosen key can be made to fail so a subtest can
// pin both the wrapped error and the fact that nothing after the failed key
// was written.
type covSettingStore struct {
	mock   *mocks.MockSettingRepository
	stored map[string]string
	order  []string
}

func newCovSettingStore(ctrl *gomock.Controller, failKey string, sentinel error) *covSettingStore {
	s := &covSettingStore{mock: mocks.NewMockSettingRepository(ctrl), stored: map[string]string{}}
	s.mock.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, key, value string) error {
			s.order = append(s.order, key)
			if key == failKey {
				return sentinel
			}
			s.stored[key] = value
			return nil
		})
	return s
}

func covFullSystemConfig() *SystemConfig {
	return &SystemConfig{
		IsInstalled:             true,
		RootEmail:               "cov-root@example.com",
		APIEndpoint:             "https://cov-api.example.com",
		SMTPHost:                "cov-smtp.example.com",
		SMTPPort:                2525,
		SMTPUsername:            "cov-smtp-user",
		SMTPPassword:            "cov-smtp-pass",
		SMTPFromEmail:           "cov-from@example.com",
		SMTPFromName:            "Cov Sender",
		SMTPUseTLS:              false,
		SMTPEHLOHostname:        "cov-ehlo.example.com",
		TelemetryEnabled:        true,
		CheckForUpdates:         true,
		AuditLogsRetentionDays:  90,
		SMTPBridgeEnabled:       true,
		SMTPBridgeDomain:        "cov-bridge.example.com",
		SMTPBridgePort:          8025,
		SMTPBridgeTLSCertBase64: "cov-bridge-cert",
		SMTPBridgeTLSKeyBase64:  "cov-bridge-key",
		OIDCEnabled:             true,
		OIDCIssuerURL:           "https://cov-idp.example.com",
		OIDCClientID:            "cov-client-id",
		OIDCClientSecret:        "cov-oidc-secret",
		OIDCRedirectURI:         "https://cov-app.example.com/callback",
		OIDCScopes:              "openid cov-scope",
		OIDCButtonLabel:         "Cov SSO",
		OIDCAutoCreateUsers:     true,
		OIDCAllowedDomains:      "cov.example.com",
	}
}

// The exact write order SetSystemConfig must follow for a fully populated
// config. Pinning the sequence catches a branch silently dropping or
// reordering a key, which a per-key map assertion alone would miss.
var covFullSetOrder = []string{
	"is_installed", "root_email", "api_endpoint", "smtp_host", "smtp_port",
	"smtp_from_email", "smtp_from_name", "smtp_ehlo_hostname", "smtp_use_tls",
	"encrypted_smtp_username", "encrypted_smtp_password", "telemetry_enabled",
	"check_for_updates", domain.AuditLogsRetentionSettingKey,
	"smtp_bridge_enabled", "smtp_bridge_domain", "smtp_bridge_port",
	"encrypted_smtp_bridge_tls_cert_base64", "encrypted_smtp_bridge_tls_key_base64",
	"oidc_enabled", "oidc_issuer_url", "oidc_client_id", "oidc_redirect_uri",
	"oidc_scopes", "oidc_button_label", "oidc_auto_create_users",
	"oidc_allowed_domains", "encrypted_oidc_client_secret",
}

func TestSettingServiceCoverage_SetSystemConfig_FullSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := newCovSettingStore(ctrl, "", nil)
	svc := NewSettingService(store.mock)
	ctx := context.Background()

	require.NoError(t, svc.SetSystemConfig(ctx, covFullSystemConfig(), testSecretKey))

	assert.Equal(t, covFullSetOrder, store.order)

	// Plaintext keys land verbatim, with the exact string encodings.
	assert.Equal(t, "true", store.stored["is_installed"])
	assert.Equal(t, "cov-root@example.com", store.stored["root_email"])
	assert.Equal(t, "https://cov-api.example.com", store.stored["api_endpoint"])
	assert.Equal(t, "cov-smtp.example.com", store.stored["smtp_host"])
	assert.Equal(t, "2525", store.stored["smtp_port"])
	assert.Equal(t, "cov-from@example.com", store.stored["smtp_from_email"])
	assert.Equal(t, "Cov Sender", store.stored["smtp_from_name"])
	assert.Equal(t, "cov-ehlo.example.com", store.stored["smtp_ehlo_hostname"])
	assert.Equal(t, "false", store.stored["smtp_use_tls"]) // false is written, not skipped
	assert.Equal(t, "true", store.stored["telemetry_enabled"])
	assert.Equal(t, "true", store.stored["check_for_updates"])
	assert.Equal(t, "90", store.stored[domain.AuditLogsRetentionSettingKey])
	assert.Equal(t, "true", store.stored["smtp_bridge_enabled"])
	assert.Equal(t, "cov-bridge.example.com", store.stored["smtp_bridge_domain"])
	assert.Equal(t, "8025", store.stored["smtp_bridge_port"])
	assert.Equal(t, "true", store.stored["oidc_enabled"])
	assert.Equal(t, "https://cov-idp.example.com", store.stored["oidc_issuer_url"])
	assert.Equal(t, "cov-client-id", store.stored["oidc_client_id"])
	assert.Equal(t, "https://cov-app.example.com/callback", store.stored["oidc_redirect_uri"])
	assert.Equal(t, "openid cov-scope", store.stored["oidc_scopes"])
	assert.Equal(t, "Cov SSO", store.stored["oidc_button_label"])
	assert.Equal(t, "true", store.stored["oidc_auto_create_users"])
	assert.Equal(t, "cov.example.com", store.stored["oidc_allowed_domains"])

	// Every secret is stored encrypted and decrypts back to its sentinel.
	for key, want := range map[string]string{
		"encrypted_smtp_username":               "cov-smtp-user",
		"encrypted_smtp_password":               "cov-smtp-pass",
		"encrypted_smtp_bridge_tls_cert_base64": "cov-bridge-cert",
		"encrypted_smtp_bridge_tls_key_base64":  "cov-bridge-key",
		"encrypted_oidc_client_secret":          "cov-oidc-secret",
	} {
		stored := store.stored[key]
		require.NotEmpty(t, stored, key)
		assert.NotEqual(t, want, stored, "%s must be encrypted at rest", key)
		decrypted, err := crypto.DecryptFromHexString(stored, testSecretKey)
		require.NoError(t, err, key)
		assert.Equal(t, want, decrypted, key)
	}
}

func TestSettingServiceCoverage_SetSystemConfig_MinimalConfigSkipsOptionalKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := newCovSettingStore(ctrl, "", nil)
	svc := NewSettingService(store.mock)

	require.NoError(t, svc.SetSystemConfig(context.Background(), &SystemConfig{
		IsInstalled: false, AuditLogsRetentionDays: 0, // 0 = keep forever, valid
	}, testSecretKey))

	// Optional keys are never written when empty/zero.
	for _, key := range []string{"root_email", "api_endpoint", "smtp_host", "smtp_port", "smtp_from_email", "smtp_bridge_port"} {
		_, written := store.stored[key]
		assert.False(t, written, "%s must not be written for an empty config", key)
		assert.NotContains(t, store.order, key)
	}
	// Always-written keys carry their clearing/false values.
	assert.Equal(t, "false", store.stored["is_installed"])
	assert.Equal(t, "", store.stored["smtp_from_name"])
	assert.Equal(t, "", store.stored["smtp_ehlo_hostname"])
	assert.Equal(t, "false", store.stored["smtp_use_tls"]) // SMTPUseTLS false → "false", always written
	assert.Equal(t, "false", store.stored["telemetry_enabled"])
	assert.Equal(t, "false", store.stored["check_for_updates"])
	assert.Equal(t, "0", store.stored[domain.AuditLogsRetentionSettingKey])
	assert.Equal(t, "", store.stored["encrypted_smtp_username"]) // cleared, not skipped
	assert.Equal(t, "", store.stored["encrypted_oidc_client_secret"])
}

func TestSettingServiceCoverage_SetSystemConfig_FailurePerKey(t *testing.T) {
	sentinel := errors.New("cov setting store down")
	ctx := context.Background()

	// Every "set" branch: the wrapped error names the exact key, wraps the
	// sentinel, and no later key is attempted.
	for _, key := range covFullSetOrder {
		t.Run("set/"+key, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			store := newCovSettingStore(ctrl, key, sentinel)
			svc := NewSettingService(store.mock)

			err := svc.SetSystemConfig(ctx, covFullSystemConfig(), testSecretKey)
			require.Error(t, err)
			assert.EqualError(t, err, fmt.Sprintf("failed to set %s: cov setting store down", key))
			assert.ErrorIs(t, err, sentinel)
			require.NotEmpty(t, store.order)
			assert.Equal(t, key, store.order[len(store.order)-1], "no write may follow the failed key")
		})
	}

	// Every "clear" branch (empty secret): the message says "clear", not "set".
	clearCases := []struct {
		name string
		key  string
	}{
		{"smtp username", "encrypted_smtp_username"},
		{"smtp password", "encrypted_smtp_password"},
		{"bridge cert", "encrypted_smtp_bridge_tls_cert_base64"},
		{"bridge key", "encrypted_smtp_bridge_tls_key_base64"},
		{"oidc secret", "encrypted_oidc_client_secret"},
	}
	for _, tc := range clearCases {
		t.Run("clear/"+tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			store := newCovSettingStore(ctrl, tc.key, sentinel)
			svc := NewSettingService(store.mock)

			err := svc.SetSystemConfig(ctx, &SystemConfig{
				IsInstalled: true, AuditLogsRetentionDays: domain.DefaultAuditLogRetentionDays,
			}, testSecretKey)
			require.Error(t, err)
			assert.EqualError(t, err, fmt.Sprintf("failed to clear %s: cov setting store down", tc.key))
			assert.ErrorIs(t, err, sentinel)
			require.NotEmpty(t, store.order)
			assert.Equal(t, tc.key, store.order[len(store.order)-1], "no write may follow the failed clear")
		})
	}
}

func TestSettingServiceCoverage_SetSystemConfig_InvalidRetentionStopsBeforeWritingIt(t *testing.T) {
	ctx := context.Background()
	for _, days := range []int{29, 3651, -1} {
		t.Run(fmt.Sprintf("days=%d", days), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			store := newCovSettingStore(ctrl, "", nil)
			svc := NewSettingService(store.mock)

			cfg := covFullSystemConfig()
			cfg.AuditLogsRetentionDays = days
			err := svc.SetSystemConfig(ctx, cfg, testSecretKey)
			require.Error(t, err)
			// Returned unwrapped: the validation error itself, verbatim.
			assert.EqualError(t, err, "retention_days must be 0 (keep forever) or between 30 and 3650")
			// Everything before the retention check was written; nothing after it was.
			assert.Contains(t, store.order, "check_for_updates")
			assert.NotContains(t, store.order, domain.AuditLogsRetentionSettingKey)
			assert.NotContains(t, store.order, "smtp_bridge_enabled")
			assert.NotContains(t, store.order, "oidc_enabled")
		})
	}

	t.Run("boundary retentions are accepted", func(t *testing.T) {
		for _, days := range []int{0, 30, 3650} {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			store := newCovSettingStore(ctrl, "", nil)
			svc := NewSettingService(store.mock)
			cfg := covFullSystemConfig()
			cfg.AuditLogsRetentionDays = days
			require.NoError(t, svc.SetSystemConfig(ctx, cfg, testSecretKey))
			assert.Equal(t, fmt.Sprintf("%d", days), store.stored[domain.AuditLogsRetentionSettingKey])
		}
	})
}

// ---------------------------------------------------------------------------
// GetSystemConfig
// ---------------------------------------------------------------------------

func TestSettingServiceCoverage_GetSystemConfig_FullInstalledConfig(t *testing.T) {
	ctx := context.Background()
	repo := NewMockSettingRepository()
	repo.settings["is_installed"] = "true"
	repo.settings["root_email"] = "cov-root@example.com"
	repo.settings["api_endpoint"] = "https://cov-api.example.com"
	repo.settings["smtp_host"] = "cov-smtp.example.com"
	repo.settings["smtp_port"] = "2525"
	repo.settings["smtp_from_email"] = "cov-from@example.com"
	repo.settings["smtp_from_name"] = "Cov Sender"
	repo.settings["smtp_use_tls"] = "false"
	repo.settings["smtp_ehlo_hostname"] = "cov-ehlo.example.com"
	repo.settings["telemetry_enabled"] = "true"
	repo.settings["check_for_updates"] = "true"
	repo.settings[domain.AuditLogsRetentionSettingKey] = "90"
	repo.settings["smtp_bridge_enabled"] = "true"
	repo.settings["smtp_bridge_domain"] = "cov-bridge.example.com"
	repo.settings["smtp_bridge_port"] = "8025"
	repo.settings["oidc_enabled"] = "true"
	repo.settings["oidc_issuer_url"] = "https://cov-idp.example.com"
	repo.settings["oidc_client_id"] = "cov-client-id"
	repo.settings["oidc_redirect_uri"] = "https://cov-app.example.com/callback"
	repo.settings["oidc_scopes"] = "openid cov-scope"
	repo.settings["oidc_button_label"] = "Cov SSO"
	repo.settings["oidc_auto_create_users"] = "true"
	repo.settings["oidc_allowed_domains"] = "cov.example.com"

	for key, plain := range map[string]string{
		"encrypted_smtp_username":               "cov-smtp-user",
		"encrypted_smtp_password":               "cov-smtp-pass",
		"encrypted_smtp_bridge_tls_cert_base64": "cov-bridge-cert",
		"encrypted_smtp_bridge_tls_key_base64":  "cov-bridge-key",
		"encrypted_oidc_client_secret":          "cov-oidc-secret",
	} {
		enc, err := crypto.EncryptString(plain, testSecretKey)
		require.NoError(t, err)
		repo.settings[key] = enc
	}

	svc := NewSettingService(repo)
	cfg, err := svc.GetSystemConfig(ctx, testSecretKey)
	require.NoError(t, err)

	assert.True(t, cfg.IsInstalled)
	assert.Equal(t, "cov-root@example.com", cfg.RootEmail)
	assert.Equal(t, "https://cov-api.example.com", cfg.APIEndpoint)
	assert.Equal(t, "cov-smtp.example.com", cfg.SMTPHost)
	assert.Equal(t, 2525, cfg.SMTPPort)
	assert.Equal(t, "cov-from@example.com", cfg.SMTPFromEmail)
	assert.Equal(t, "Cov Sender", cfg.SMTPFromName)
	assert.False(t, cfg.SMTPUseTLS)
	assert.Equal(t, "cov-ehlo.example.com", cfg.SMTPEHLOHostname)
	assert.Equal(t, "cov-smtp-user", cfg.SMTPUsername)
	assert.Equal(t, "cov-smtp-pass", cfg.SMTPPassword)
	assert.True(t, cfg.TelemetryEnabled)
	assert.True(t, cfg.CheckForUpdates)
	assert.Equal(t, 90, cfg.AuditLogsRetentionDays)
	assert.True(t, cfg.SMTPBridgeEnabled)
	assert.Equal(t, "cov-bridge.example.com", cfg.SMTPBridgeDomain)
	assert.Equal(t, 8025, cfg.SMTPBridgePort)
	assert.Equal(t, "cov-bridge-cert", cfg.SMTPBridgeTLSCertBase64)
	assert.Equal(t, "cov-bridge-key", cfg.SMTPBridgeTLSKeyBase64)
	assert.True(t, cfg.OIDCEnabled)
	assert.Equal(t, "https://cov-idp.example.com", cfg.OIDCIssuerURL)
	assert.Equal(t, "cov-client-id", cfg.OIDCClientID)
	assert.Equal(t, "cov-oidc-secret", cfg.OIDCClientSecret)
	assert.Equal(t, "https://cov-app.example.com/callback", cfg.OIDCRedirectURI)
	assert.Equal(t, "openid cov-scope", cfg.OIDCScopes)
	assert.Equal(t, "Cov SSO", cfg.OIDCButtonLabel)
	assert.True(t, cfg.OIDCAutoCreateUsers)
	assert.Equal(t, "cov.example.com", cfg.OIDCAllowedDomains)
}

func TestSettingServiceCoverage_GetSystemConfig_DecryptFailures(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		key  string
		want string
	}{
		{"bridge cert", "encrypted_smtp_bridge_tls_cert_base64", "failed to decrypt SMTP bridge TLS certificate"},
		{"bridge key", "encrypted_smtp_bridge_tls_key_base64", "failed to decrypt SMTP bridge TLS key"},
		{"oidc secret", "encrypted_oidc_client_secret", "failed to decrypt OIDC client secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewMockSettingRepository()
			repo.settings["is_installed"] = "true"
			repo.settings[tc.key] = "not-valid-hex-cov"
			svc := NewSettingService(repo)

			cfg, err := svc.GetSystemConfig(ctx, testSecretKey)
			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestSettingServiceCoverage_GetSystemConfig_ParsingEdges(t *testing.T) {
	ctx := context.Background()
	base := func() *MockSettingRepository {
		repo := NewMockSettingRepository()
		repo.settings["is_installed"] = "true"
		return repo
	}

	t.Run("retention below the floor keeps the deployment default", func(t *testing.T) {
		repo := base()
		repo.settings[domain.AuditLogsRetentionSettingKey] = "10"
		cfg, err := NewSettingService(repo).GetSystemConfig(ctx, testSecretKey)
		require.NoError(t, err)
		assert.Equal(t, domain.DefaultAuditLogRetentionDays, cfg.AuditLogsRetentionDays)
	})

	t.Run("retention with surrounding spaces is trimmed and accepted", func(t *testing.T) {
		repo := base()
		repo.settings[domain.AuditLogsRetentionSettingKey] = " 45 "
		cfg, err := NewSettingService(repo).GetSystemConfig(ctx, testSecretKey)
		require.NoError(t, err)
		assert.Equal(t, 45, cfg.AuditLogsRetentionDays)
	})

	t.Run("non-numeric retention keeps the deployment default", func(t *testing.T) {
		repo := base()
		repo.settings[domain.AuditLogsRetentionSettingKey] = "cov-not-a-number"
		cfg, err := NewSettingService(repo).GetSystemConfig(ctx, testSecretKey)
		require.NoError(t, err)
		assert.Equal(t, domain.DefaultAuditLogRetentionDays, cfg.AuditLogsRetentionDays)
	})

	t.Run("empty smtp_port keeps the default and invalid bridge port stays zero", func(t *testing.T) {
		repo := base()
		repo.settings["smtp_port"] = ""
		repo.settings["smtp_bridge_port"] = "cov-not-a-port"
		cfg, err := NewSettingService(repo).GetSystemConfig(ctx, testSecretKey)
		require.NoError(t, err)
		assert.Equal(t, 587, cfg.SMTPPort)
		assert.Equal(t, 0, cfg.SMTPBridgePort)
	})

	t.Run("smtp_use_tls is false only for the literal string false", func(t *testing.T) {
		repo := base()
		repo.settings["smtp_use_tls"] = "yes"
		cfg, err := NewSettingService(repo).GetSystemConfig(ctx, testSecretKey)
		require.NoError(t, err)
		assert.True(t, cfg.SMTPUseTLS)

		repo2 := base()
		repo2.settings["smtp_use_tls"] = "false"
		cfg2, err := NewSettingService(repo2).GetSystemConfig(ctx, testSecretKey)
		require.NoError(t, err)
		assert.False(t, cfg2.SMTPUseTLS)
	})

	t.Run("boolean settings parse only the literal string true", func(t *testing.T) {
		repo := base()
		repo.settings["telemetry_enabled"] = "yes"
		repo.settings["check_for_updates"] = "1"
		repo.settings["smtp_bridge_enabled"] = "TRUE"
		repo.settings["oidc_enabled"] = "on"
		repo.settings["oidc_auto_create_users"] = "yes"
		cfg, err := NewSettingService(repo).GetSystemConfig(ctx, testSecretKey)
		require.NoError(t, err)
		assert.False(t, cfg.TelemetryEnabled)
		assert.False(t, cfg.CheckForUpdates)
		assert.False(t, cfg.SMTPBridgeEnabled)
		assert.False(t, cfg.OIDCEnabled)
		assert.False(t, cfg.OIDCAutoCreateUsers)
	})
}
