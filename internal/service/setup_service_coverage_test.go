package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service"
)

const covSetupSecret = "cov-setup-secret-key-32-bytes!!"

// covSetupHarness wires a SetupService to a recording setting repository and
// a capturing user repository.
type covSetupHarness struct {
	svc          *service.SetupService
	stored       map[string]string
	setOrder     []string
	createdUsers []*domain.User
	setFailKey   string
	setSentinel  error
	createErr    error
	callbackErr  error
	callbackRuns int
}

func newCovSetupHarness(t *testing.T, env *service.EnvironmentConfig) *covSetupHarness {
	t.Helper()
	h := &covSetupHarness{stored: map[string]string{}}

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	settingRepo := mocks.NewMockSettingRepository(ctrl)
	settingRepo.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, key, value string) error {
			h.setOrder = append(h.setOrder, key)
			if key == h.setFailKey {
				return h.setSentinel
			}
			h.stored[key] = value
			return nil
		})

	userRepo := mocks.NewMockUserRepository(ctrl)
	userRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, user *domain.User) error {
			h.createdUsers = append(h.createdUsers, user)
			return h.createErr
		})

	h.svc = service.NewSetupService(
		service.NewSettingService(settingRepo),
		&service.UserService{},
		userRepo,
		&mockLogger{},
		covSetupSecret,
		func() error {
			h.callbackRuns++
			return h.callbackErr
		},
		env,
	)
	return h
}

func covSetupBaseConfig() *service.SetupConfig {
	return &service.SetupConfig{
		RootEmail:     "cov-admin@example.com",
		APIEndpoint:   "https://cov-app.example.com///",
		SMTPHost:      "cov-smtp.example.com",
		SMTPPort:      2525,
		SMTPUsername:  "cov-user",
		SMTPPassword:  "cov-pass",
		SMTPFromEmail: "cov-from@example.com",
		SMTPFromName:  "Cov Sender",
		SMTPUseTLS:    true,
	}
}

// ---------------------------------------------------------------------------
// GetEnvConfig
// ---------------------------------------------------------------------------

func TestSetupServiceCoverage_GetEnvConfig(t *testing.T) {
	t.Run("nil env config is returned as nil", func(t *testing.T) {
		svc := service.NewSetupService(&service.SettingService{}, &service.UserService{}, nil, &mockLogger{}, "k", nil, nil)
		assert.Nil(t, svc.GetEnvConfig())
	})

	t.Run("the exact constructor pointer is returned, not a copy", func(t *testing.T) {
		env := &service.EnvironmentConfig{RootEmail: "cov-env@example.com", SMTPPort: 2525}
		svc := service.NewSetupService(&service.SettingService{}, &service.UserService{}, nil, &mockLogger{}, "k", nil, env)
		got := svc.GetEnvConfig()
		require.NotNil(t, got)
		assert.True(t, got == env, "GetEnvConfig must return the stored pointer")
		assert.Equal(t, "cov-env@example.com", got.RootEmail)
		assert.Equal(t, 2525, got.SMTPPort)
	})

	t.Run("an empty but non-nil env config is returned as-is", func(t *testing.T) {
		env := &service.EnvironmentConfig{}
		svc := service.NewSetupService(&service.SettingService{}, &service.UserService{}, nil, &mockLogger{}, "k", nil, env)
		got := svc.GetEnvConfig()
		require.NotNil(t, got)
		assert.True(t, got == env)
	})
}

// ---------------------------------------------------------------------------
// GetEnvOverrides — every key/flag mapping pinned with distinct sentinels
// ---------------------------------------------------------------------------

func TestSetupServiceCoverage_GetEnvOverrides_EveryKeyIndividually(t *testing.T) {
	cases := []struct {
		name string
		env  *service.EnvironmentConfig
		key  string
	}{
		{"root_email", &service.EnvironmentConfig{RootEmail: "cov-v"}, "root_email"},
		{"api_endpoint", &service.EnvironmentConfig{APIEndpoint: "cov-v"}, "api_endpoint"},
		{"smtp_host", &service.EnvironmentConfig{SMTPHost: "cov-v"}, "smtp_host"},
		{"smtp_port", &service.EnvironmentConfig{SMTPPort: 2525}, "smtp_port"},
		{"smtp_username", &service.EnvironmentConfig{SMTPUsername: "cov-v"}, "smtp_username"},
		{"smtp_password", &service.EnvironmentConfig{SMTPPassword: "cov-v"}, "smtp_password"},
		{"smtp_from_email", &service.EnvironmentConfig{SMTPFromEmail: "cov-v"}, "smtp_from_email"},
		{"smtp_from_name", &service.EnvironmentConfig{SMTPFromName: "cov-v"}, "smtp_from_name"},
		{"smtp_use_tls true", &service.EnvironmentConfig{SMTPUseTLS: "true"}, "smtp_use_tls"},
		{"smtp_use_tls false still counts", &service.EnvironmentConfig{SMTPUseTLS: "false"}, "smtp_use_tls"},
		{"smtp_ehlo_hostname", &service.EnvironmentConfig{SMTPEHLOHostname: "cov-v"}, "smtp_ehlo_hostname"},
		{"smtp_bridge_enabled true", &service.EnvironmentConfig{SMTPBridgeEnabled: "true"}, "smtp_bridge_enabled"},
		{"smtp_bridge_enabled false still counts", &service.EnvironmentConfig{SMTPBridgeEnabled: "false"}, "smtp_bridge_enabled"},
		{"smtp_bridge_domain", &service.EnvironmentConfig{SMTPBridgeDomain: "cov-v"}, "smtp_bridge_domain"},
		{"smtp_bridge_port", &service.EnvironmentConfig{SMTPBridgePort: 8025}, "smtp_bridge_port"},
		{"smtp_bridge_tls_cert_base64", &service.EnvironmentConfig{SMTPBridgeTLSCertBase64: "cov-v"}, "smtp_bridge_tls_cert_base64"},
		{"smtp_bridge_tls_key_base64", &service.EnvironmentConfig{SMTPBridgeTLSKeyBase64: "cov-v"}, "smtp_bridge_tls_key_base64"},
		{"oidc_enabled true", &service.EnvironmentConfig{OIDCEnabled: "true"}, "oidc_enabled"},
		{"oidc_enabled false still counts", &service.EnvironmentConfig{OIDCEnabled: "false"}, "oidc_enabled"},
		{"oidc_issuer_url", &service.EnvironmentConfig{OIDCIssuerURL: "cov-v"}, "oidc_issuer_url"},
		{"oidc_client_id", &service.EnvironmentConfig{OIDCClientID: "cov-v"}, "oidc_client_id"},
		{"oidc_client_secret", &service.EnvironmentConfig{OIDCClientSecret: "cov-v"}, "oidc_client_secret"},
		{"oidc_redirect_uri", &service.EnvironmentConfig{OIDCRedirectURI: "cov-v"}, "oidc_redirect_uri"},
		{"oidc_scopes", &service.EnvironmentConfig{OIDCScopes: "cov-v"}, "oidc_scopes"},
		{"oidc_button_label", &service.EnvironmentConfig{OIDCButtonLabel: "cov-v"}, "oidc_button_label"},
		{"oidc_auto_create_users true", &service.EnvironmentConfig{OIDCAutoCreateUsers: "true"}, "oidc_auto_create_users"},
		{"oidc_auto_create_users false still counts", &service.EnvironmentConfig{OIDCAutoCreateUsers: "false"}, "oidc_auto_create_users"},
		{"oidc_allowed_domains", &service.EnvironmentConfig{OIDCAllowedDomains: "cov-v"}, "oidc_allowed_domains"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := service.NewSetupService(&service.SettingService{}, &service.UserService{}, nil, &mockLogger{}, "k", nil, tc.env)
			assert.Equal(t, map[string]bool{tc.key: true}, svc.GetEnvOverrides(),
				"a single set field must produce exactly its own override key")
		})
	}

	t.Run("zero ports do not count as overrides", func(t *testing.T) {
		svc := service.NewSetupService(&service.SettingService{}, &service.UserService{}, nil, &mockLogger{}, "k", nil,
			&service.EnvironmentConfig{SMTPPort: 0, SMTPBridgePort: 0})
		assert.Equal(t, map[string]bool{}, svc.GetEnvOverrides())
	})

	t.Run("fully populated env yields exactly the complete override map", func(t *testing.T) {
		env := &service.EnvironmentConfig{
			RootEmail: "cov-root", APIEndpoint: "cov-api", SMTPHost: "cov-host", SMTPPort: 2525,
			SMTPUsername: "cov-user", SMTPPassword: "cov-pass", SMTPFromEmail: "cov-from",
			SMTPFromName: "cov-name", SMTPUseTLS: "false", SMTPEHLOHostname: "cov-ehlo",
			SMTPBridgeEnabled: "false", SMTPBridgeDomain: "cov-bridge", SMTPBridgePort: 8025,
			SMTPBridgeTLSCertBase64: "cov-cert", SMTPBridgeTLSKeyBase64: "cov-key",
			OIDCEnabled: "false", OIDCIssuerURL: "cov-issuer", OIDCClientID: "cov-client",
			OIDCClientSecret: "cov-secret", OIDCRedirectURI: "cov-redirect", OIDCScopes: "cov-scopes",
			OIDCButtonLabel: "cov-label", OIDCAutoCreateUsers: "false", OIDCAllowedDomains: "cov-domains",
		}
		svc := service.NewSetupService(&service.SettingService{}, &service.UserService{}, nil, &mockLogger{}, "k", nil, env)
		assert.Equal(t, map[string]bool{
			"root_email": true, "api_endpoint": true,
			"smtp_host": true, "smtp_port": true, "smtp_username": true, "smtp_password": true,
			"smtp_from_email": true, "smtp_from_name": true, "smtp_use_tls": true, "smtp_ehlo_hostname": true,
			"smtp_bridge_enabled": true, "smtp_bridge_domain": true, "smtp_bridge_port": true,
			"smtp_bridge_tls_cert_base64": true, "smtp_bridge_tls_key_base64": true,
			"oidc_enabled": true, "oidc_issuer_url": true, "oidc_client_id": true, "oidc_client_secret": true,
			"oidc_redirect_uri": true, "oidc_scopes": true, "oidc_button_label": true,
			"oidc_auto_create_users": true, "oidc_allowed_domains": true,
		}, svc.GetEnvOverrides())
	})
}

// ---------------------------------------------------------------------------
// GetConfigurationStatus
// ---------------------------------------------------------------------------

func TestSetupServiceCoverage_GetConfigurationStatus(t *testing.T) {
	newSvc := func(env *service.EnvironmentConfig) *service.SetupService {
		return service.NewSetupService(&service.SettingService{}, &service.UserService{}, nil, &mockLogger{}, "k", nil, env)
	}
	allFalse := &service.ConfigurationStatus{}

	t.Run("nil env config reports nothing configured", func(t *testing.T) {
		assert.Equal(t, allFalse, newSvc(nil).GetConfigurationStatus())
	})

	t.Run("empty env config reports nothing configured", func(t *testing.T) {
		assert.Equal(t, allFalse, newSvc(&service.EnvironmentConfig{}).GetConfigurationStatus())
	})

	t.Run("SMTP needs host AND port AND from-email", func(t *testing.T) {
		assert.False(t, newSvc(&service.EnvironmentConfig{SMTPHost: "h"}).GetConfigurationStatus().SMTPConfigured)
		assert.False(t, newSvc(&service.EnvironmentConfig{SMTPHost: "h", SMTPPort: 587}).GetConfigurationStatus().SMTPConfigured)
		assert.False(t, newSvc(&service.EnvironmentConfig{SMTPHost: "h", SMTPFromEmail: "f@x.com"}).GetConfigurationStatus().SMTPConfigured)
		assert.False(t, newSvc(&service.EnvironmentConfig{SMTPPort: 587, SMTPFromEmail: "f@x.com"}).GetConfigurationStatus().SMTPConfigured)
		assert.True(t, newSvc(&service.EnvironmentConfig{SMTPHost: "h", SMTPPort: 587, SMTPFromEmail: "f@x.com"}).GetConfigurationStatus().SMTPConfigured)
		// Username/password are not part of the decision.
		assert.True(t, newSvc(&service.EnvironmentConfig{SMTPHost: "h", SMTPPort: 587, SMTPFromEmail: "f@x.com", SMTPUsername: "", SMTPPassword: ""}).GetConfigurationStatus().SMTPConfigured)
	})

	t.Run("any non-empty bridge enabled flag locks the bridge, even false", func(t *testing.T) {
		assert.True(t, newSvc(&service.EnvironmentConfig{SMTPBridgeEnabled: "false"}).GetConfigurationStatus().SMTPBridgeConfigured)
		assert.True(t, newSvc(&service.EnvironmentConfig{SMTPBridgeEnabled: "true"}).GetConfigurationStatus().SMTPBridgeConfigured)
	})

	t.Run("bridge domain and certs alone do not configure the bridge", func(t *testing.T) {
		// The enabled flag is empty, so neither disjunct fires: a stored domain
		// plus cert/key material must not lock the wizard out.
		assert.False(t, newSvc(&service.EnvironmentConfig{
			SMTPBridgeDomain: "b.example.com", SMTPBridgeTLSCertBase64: "cert", SMTPBridgeTLSKeyBase64: "key",
		}).GetConfigurationStatus().SMTPBridgeConfigured)
	})

	t.Run("OIDC is configured by enabled, issuer or client id — but not by secret alone", func(t *testing.T) {
		assert.True(t, newSvc(&service.EnvironmentConfig{OIDCEnabled: "false"}).GetConfigurationStatus().OIDCConfigured)
		assert.True(t, newSvc(&service.EnvironmentConfig{OIDCIssuerURL: "https://idp"}).GetConfigurationStatus().OIDCConfigured)
		assert.True(t, newSvc(&service.EnvironmentConfig{OIDCClientID: "cid"}).GetConfigurationStatus().OIDCConfigured)
		assert.False(t, newSvc(&service.EnvironmentConfig{OIDCClientSecret: "s"}).GetConfigurationStatus().OIDCConfigured)
		assert.False(t, newSvc(&service.EnvironmentConfig{OIDCRedirectURI: "https://cb"}).GetConfigurationStatus().OIDCConfigured)
		assert.False(t, newSvc(&service.EnvironmentConfig{OIDCAllowedDomains: "x.com"}).GetConfigurationStatus().OIDCConfigured)
	})

	t.Run("root email and API endpoint flags are independent", func(t *testing.T) {
		status := newSvc(&service.EnvironmentConfig{RootEmail: "r@x.com"}).GetConfigurationStatus()
		assert.True(t, status.RootEmailConfigured)
		assert.False(t, status.APIEndpointConfigured)

		status = newSvc(&service.EnvironmentConfig{APIEndpoint: "https://api"}).GetConfigurationStatus()
		assert.False(t, status.RootEmailConfigured)
		assert.True(t, status.APIEndpointConfigured)
	})

	t.Run("fully populated env configures every group", func(t *testing.T) {
		status := newSvc(&service.EnvironmentConfig{
			RootEmail: "r@x.com", APIEndpoint: "https://api",
			SMTPHost: "h", SMTPPort: 587, SMTPFromEmail: "f@x.com",
			SMTPBridgeEnabled: "true", OIDCClientID: "cid",
		}).GetConfigurationStatus()
		assert.Equal(t, &service.ConfigurationStatus{
			SMTPConfigured: true, APIEndpointConfigured: true, RootEmailConfigured: true,
			SMTPBridgeConfigured: true, OIDCConfigured: true,
		}, status)
	})
}

// ---------------------------------------------------------------------------
// ValidateSetupConfig — exact error strings
// ---------------------------------------------------------------------------

func TestSetupServiceCoverage_ValidateSetupConfig_ExactErrors(t *testing.T) {
	newSvc := func(env *service.EnvironmentConfig) *service.SetupService {
		return service.NewSetupService(&service.SettingService{}, &service.UserService{}, nil, &mockLogger{}, "k", nil, env)
	}

	t.Run("each missing field names itself exactly", func(t *testing.T) {
		svc := newSvc(&service.EnvironmentConfig{})
		assert.EqualError(t, svc.ValidateSetupConfig(&service.SetupConfig{}), "root_email is required")
		assert.EqualError(t, svc.ValidateSetupConfig(&service.SetupConfig{RootEmail: "a@b.com"}), "smtp_host is required")
		assert.EqualError(t, svc.ValidateSetupConfig(&service.SetupConfig{RootEmail: "a@b.com", SMTPHost: "h"}), "smtp_from_email is required")
	})

	t.Run("a zero SMTP port is defaulted to 587 in place", func(t *testing.T) {
		svc := newSvc(&service.EnvironmentConfig{})
		cfg := &service.SetupConfig{RootEmail: "a@b.com", SMTPHost: "h", SMTPFromEmail: "f@b.com"}
		require.NoError(t, svc.ValidateSetupConfig(cfg))
		assert.Equal(t, 587, cfg.SMTPPort)
	})

	t.Run("env-configured groups skip their wizard validation", func(t *testing.T) {
		svc := newSvc(&service.EnvironmentConfig{
			RootEmail: "env@x.com", SMTPHost: "h", SMTPPort: 25, SMTPFromEmail: "f@x.com",
		})
		require.NoError(t, svc.ValidateSetupConfig(&service.SetupConfig{}))
	})

	t.Run("OIDC wizard validation names each missing field exactly", func(t *testing.T) {
		svc := newSvc(&service.EnvironmentConfig{})
		base := func() *service.SetupConfig {
			return &service.SetupConfig{RootEmail: "a@b.com", SMTPHost: "h", SMTPFromEmail: "f@b.com", OIDCEnabled: true}
		}
		assert.EqualError(t, svc.ValidateSetupConfig(base()), "oidc_issuer_url is required when OIDC is enabled")

		cfg := base()
		cfg.OIDCIssuerURL = "https://idp"
		assert.EqualError(t, svc.ValidateSetupConfig(cfg), "oidc_client_id is required when OIDC is enabled")

		cfg.OIDCClientID = "cid"
		assert.EqualError(t, svc.ValidateSetupConfig(cfg), "oidc_client_secret is required when OIDC is enabled")

		cfg.OIDCClientSecret = "s"
		cfg.OIDCAutoCreateUsers = true
		assert.EqualError(t, svc.ValidateSetupConfig(cfg), "oidc_allowed_domains is required when OIDC auto-create is enabled")

		cfg.OIDCAllowedDomains = "corp.com"
		assert.NoError(t, svc.ValidateSetupConfig(cfg))
	})

	t.Run("env-configured OIDC skips wizard OIDC validation entirely", func(t *testing.T) {
		svc := newSvc(&service.EnvironmentConfig{OIDCClientID: "env-cid"})
		cfg := &service.SetupConfig{RootEmail: "a@b.com", SMTPHost: "h", SMTPFromEmail: "f@b.com", OIDCEnabled: true}
		assert.NoError(t, svc.ValidateSetupConfig(cfg))
	})
}

// ---------------------------------------------------------------------------
// Initialize
// ---------------------------------------------------------------------------

func TestSetupServiceCoverage_Initialize(t *testing.T) {
	ctx := context.Background()

	t.Run("validation failure writes nothing and creates no user", func(t *testing.T) {
		h := newCovSetupHarness(t, &service.EnvironmentConfig{})
		err := h.svc.Initialize(ctx, &service.SetupConfig{})
		require.Error(t, err)
		assert.EqualError(t, err, "root_email is required")
		assert.Empty(t, h.setOrder)
		assert.Empty(t, h.createdUsers)
		assert.Zero(t, h.callbackRuns)
	})

	t.Run("user values are persisted and the API endpoint is de-slashed", func(t *testing.T) {
		h := newCovSetupHarness(t, &service.EnvironmentConfig{})
		require.NoError(t, h.svc.Initialize(ctx, covSetupBaseConfig()))

		assert.Equal(t, "true", h.stored["is_installed"])
		assert.Equal(t, "cov-admin@example.com", h.stored["root_email"])
		assert.Equal(t, "https://cov-app.example.com", h.stored["api_endpoint"]) // trailing slashes trimmed
		assert.Equal(t, "cov-smtp.example.com", h.stored["smtp_host"])
		assert.Equal(t, "2525", h.stored["smtp_port"])
		assert.Equal(t, "cov-from@example.com", h.stored["smtp_from_email"])
		assert.Equal(t, "Cov Sender", h.stored["smtp_from_name"])
		assert.Equal(t, "true", h.stored["smtp_use_tls"])
		assert.Equal(t, "365", h.stored[domain.AuditLogsRetentionSettingKey])

		require.Len(t, h.createdUsers, 1)
		created := h.createdUsers[0]
		assert.Equal(t, "cov-admin@example.com", created.Email)
		assert.Equal(t, "Root User", created.Name)
		assert.Equal(t, domain.UserTypeUser, created.Type)
		assert.NotEmpty(t, created.ID)
		assert.False(t, created.CreatedAt.IsZero())
		assert.Equal(t, 1, h.callbackRuns)
	})

	t.Run("zero SMTP port is persisted as the 587 default", func(t *testing.T) {
		h := newCovSetupHarness(t, &service.EnvironmentConfig{})
		cfg := covSetupBaseConfig()
		cfg.SMTPPort = 0
		require.NoError(t, h.svc.Initialize(ctx, cfg))
		assert.Equal(t, "587", h.stored["smtp_port"])
	})

	t.Run("system-config failure is wrapped and no user is created", func(t *testing.T) {
		h := newCovSetupHarness(t, &service.EnvironmentConfig{})
		h.setFailKey = "is_installed"
		h.setSentinel = errors.New("cov setting store down")

		err := h.svc.Initialize(ctx, covSetupBaseConfig())
		require.Error(t, err)
		assert.EqualError(t, err, "failed to save system configuration: failed to set is_installed: cov setting store down")
		assert.ErrorIs(t, err, h.setSentinel)
		assert.Empty(t, h.createdUsers)
		assert.Zero(t, h.callbackRuns)
	})

	t.Run("root-user creation failure is wrapped and the callback never runs", func(t *testing.T) {
		h := newCovSetupHarness(t, &service.EnvironmentConfig{})
		h.createErr = errors.New("cov user store down")

		err := h.svc.Initialize(ctx, covSetupBaseConfig())
		require.Error(t, err)
		assert.EqualError(t, err, "failed to create root user: cov user store down")
		assert.ErrorIs(t, err, h.createErr)
		assert.Zero(t, h.callbackRuns)
	})

	t.Run("an already-existing root user is tolerated", func(t *testing.T) {
		h := newCovSetupHarness(t, &service.EnvironmentConfig{})
		h.createErr = &domain.ErrUserExists{Message: "cov user exists"}

		require.NoError(t, h.svc.Initialize(ctx, covSetupBaseConfig()))
		assert.Len(t, h.createdUsers, 1)
		assert.Equal(t, 1, h.callbackRuns)
	})

	t.Run("a failing reload callback is logged, not returned", func(t *testing.T) {
		h := newCovSetupHarness(t, &service.EnvironmentConfig{})
		h.callbackErr = errors.New("cov reload failed")

		require.NoError(t, h.svc.Initialize(ctx, covSetupBaseConfig()))
		assert.Equal(t, 1, h.callbackRuns)
	})

	t.Run("a root-email list creates only the primary user", func(t *testing.T) {
		h := newCovSetupHarness(t, &service.EnvironmentConfig{})
		cfg := covSetupBaseConfig()
		cfg.RootEmail = "first@x.com, second@x.com"
		require.NoError(t, h.svc.Initialize(ctx, cfg))
		require.Len(t, h.createdUsers, 1)
		assert.Equal(t, "first@x.com", h.createdUsers[0].Email)
		// The stored setting keeps the full list; only the user is primary-only.
		assert.Equal(t, "first@x.com, second@x.com", h.stored["root_email"])
	})

	t.Run("environment SMTP wins over wizard input, including TLS false", func(t *testing.T) {
		env := &service.EnvironmentConfig{
			RootEmail: "env-root@x.com", APIEndpoint: "https://env.example.com/",
			SMTPHost: "env-smtp.example.com", SMTPPort: 465, SMTPUsername: "env-user",
			SMTPPassword: "env-pass", SMTPFromEmail: "env-from@x.com", SMTPFromName: "Env Sender",
			SMTPUseTLS: "false", SMTPEHLOHostname: "env-ehlo.example.com",
		}
		h := newCovSetupHarness(t, env)
		require.NoError(t, h.svc.Initialize(ctx, covSetupBaseConfig()))

		assert.Equal(t, "env-root@x.com", h.stored["root_email"])
		assert.Equal(t, "https://env.example.com", h.stored["api_endpoint"])
		assert.Equal(t, "env-smtp.example.com", h.stored["smtp_host"])
		assert.Equal(t, "465", h.stored["smtp_port"])
		assert.Equal(t, "env-from@x.com", h.stored["smtp_from_email"])
		assert.Equal(t, "Env Sender", h.stored["smtp_from_name"])
		assert.Equal(t, "false", h.stored["smtp_use_tls"])
		assert.Equal(t, "env-ehlo.example.com", h.stored["smtp_ehlo_hostname"])
		require.Len(t, h.createdUsers, 1)
		assert.Equal(t, "env-root@x.com", h.createdUsers[0].Email)
	})

	t.Run("environment SMTP with unset TLS defaults to TLS on", func(t *testing.T) {
		env := &service.EnvironmentConfig{
			SMTPHost: "env-smtp.example.com", SMTPPort: 587, SMTPFromEmail: "env-from@x.com",
			SMTPUseTLS: "", // not set: defaults to true
		}
		h := newCovSetupHarness(t, env)
		require.NoError(t, h.svc.Initialize(ctx, covSetupBaseConfig()))
		assert.Equal(t, "true", h.stored["smtp_use_tls"])
		assert.Equal(t, "env-smtp.example.com", h.stored["smtp_host"])
	})

	t.Run("environment bridge and OIDC win over wizard input", func(t *testing.T) {
		env := &service.EnvironmentConfig{
			SMTPBridgeEnabled: "true", SMTPBridgeDomain: "env-bridge.example.com", SMTPBridgePort: 8026,
			SMTPBridgeTLSCertBase64: "env-cert", SMTPBridgeTLSKeyBase64: "env-key",
			OIDCEnabled: "true", OIDCIssuerURL: "https://env-idp.example.com", OIDCClientID: "env-cid",
			OIDCClientSecret: "env-secret", OIDCRedirectURI: "https://env/cb", OIDCScopes: "",
			OIDCButtonLabel: "Env SSO", OIDCAutoCreateUsers: "true", OIDCAllowedDomains: "env.com",
		}
		h := newCovSetupHarness(t, env)
		cfg := covSetupBaseConfig()
		cfg.SMTPBridgeEnabled = false
		cfg.OIDCEnabled = false
		require.NoError(t, h.svc.Initialize(ctx, cfg))

		assert.Equal(t, "true", h.stored["smtp_bridge_enabled"])
		assert.Equal(t, "env-bridge.example.com", h.stored["smtp_bridge_domain"])
		assert.Equal(t, "8026", h.stored["smtp_bridge_port"])
		assert.Equal(t, "true", h.stored["oidc_enabled"])
		assert.Equal(t, "https://env-idp.example.com", h.stored["oidc_issuer_url"])
		assert.Equal(t, "env-cid", h.stored["oidc_client_id"])
		assert.Equal(t, "Env SSO", h.stored["oidc_button_label"])
		assert.Equal(t, "true", h.stored["oidc_auto_create_users"])
		assert.Equal(t, "env.com", h.stored["oidc_allowed_domains"])
		// Enabled with empty env scopes persists the full default, never bare "openid".
		assert.Equal(t, "openid email profile", h.stored["oidc_scopes"])
	})

	t.Run("wizard bridge and OIDC are used when the env does not configure them", func(t *testing.T) {
		h := newCovSetupHarness(t, &service.EnvironmentConfig{})
		cfg := covSetupBaseConfig()
		cfg.SMTPBridgeEnabled = true
		cfg.SMTPBridgeDomain = "wiz-bridge.example.com"
		cfg.SMTPBridgePort = 8027
		cfg.OIDCEnabled = true
		cfg.OIDCIssuerURL = "https://wiz-idp.example.com"
		cfg.OIDCClientID = "wiz-cid"
		cfg.OIDCClientSecret = "wiz-secret"
		cfg.OIDCScopes = "email"
		require.NoError(t, h.svc.Initialize(ctx, cfg))

		assert.Equal(t, "true", h.stored["smtp_bridge_enabled"])
		assert.Equal(t, "wiz-bridge.example.com", h.stored["smtp_bridge_domain"])
		assert.Equal(t, "8027", h.stored["smtp_bridge_port"])
		assert.Equal(t, "true", h.stored["oidc_enabled"])
		assert.Equal(t, "https://wiz-idp.example.com", h.stored["oidc_issuer_url"])
		// "openid" is forced in ahead of the caller's scopes.
		assert.Equal(t, "openid email", h.stored["oidc_scopes"])
	})
}
