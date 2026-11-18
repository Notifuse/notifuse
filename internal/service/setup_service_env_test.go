package service_test

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newStoringSetupService returns a setup service over env and the settings it stores.
func newStoringSetupService(t *testing.T, env *service.EnvironmentConfig, users int) (*service.SetupService, map[string]string) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	stored := make(map[string]string)
	settingRepo := mocks.NewMockSettingRepository(ctrl)
	settingRepo.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, key, value string) error {
			stored[key] = value
			return nil
		})
	userRepo := mocks.NewMockUserRepository(ctrl)
	userRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(nil).Times(users)

	svc := service.NewSetupService(
		service.NewSettingService(settingRepo),
		&service.UserService{},
		userRepo,
		&mockLogger{},
		"test-secret-key-32-bytes-long!!",
		nil,
		env,
	)
	return svc, stored
}

func wizardRequest() *service.SetupConfig {
	return &service.SetupConfig{
		RootEmail:     "admin@example.com",
		APIEndpoint:   "https://app.example.com",
		SMTPHost:      "smtp.example.com",
		SMTPPort:      587,
		SMTPFromEmail: "noreply@example.com",
		SMTPUseTLS:    true,
	}
}

// The wizard leaves a field set by the environment out of its request: validation and
// storage must use the environment's value, as the runtime does.
func TestSetupService_Initialize_PartiallyEnvSetGroups(t *testing.T) {
	t.Run("SMTP host and from email from env, port from the wizard", func(t *testing.T) {
		env := &service.EnvironmentConfig{SMTPHost: "smtp.env.com", SMTPFromEmail: "env@acme.com"}
		svc, stored := newStoringSetupService(t, env, 1)
		require.False(t, svc.GetConfigurationStatus().SMTPConfigured, "no SMTP_PORT: the group stays editable")

		req := wizardRequest()
		req.SMTPHost = ""
		req.SMTPFromEmail = ""
		req.SMTPPort = 2525

		require.NoError(t, svc.Initialize(context.Background(), req))
		assert.Equal(t, "smtp.env.com", stored["smtp_host"])
		assert.Equal(t, "env@acme.com", stored["smtp_from_email"])
		assert.Equal(t, "2525", stored["smtp_port"])
	})

	t.Run("the same request without env is still refused", func(t *testing.T) {
		svc := service.NewSetupService(&service.SettingService{}, &service.UserService{}, nil, &mockLogger{}, "k", nil, &service.EnvironmentConfig{})
		req := wizardRequest()
		req.SMTPHost = ""
		assert.EqualError(t, svc.ValidateSetupConfig(req), "smtp_host is required")
	})

	t.Run("OIDC client secret from env only, SSO enabled in the wizard", func(t *testing.T) {
		env := &service.EnvironmentConfig{OIDCClientSecret: "env-secret"}
		svc, stored := newStoringSetupService(t, env, 1)
		require.False(t, svc.GetConfigurationStatus().OIDCConfigured)

		req := wizardRequest()
		req.OIDCEnabled = true
		req.OIDCIssuerURL = "https://idp.example.com"
		req.OIDCClientID = "cid"

		require.NoError(t, svc.Initialize(context.Background(), req))
		assert.Equal(t, "cid", stored["oidc_client_id"])
		assert.NotEmpty(t, stored["encrypted_oidc_client_secret"], "the env secret is stored (encrypted)")
	})

	t.Run("an env value wins over a typed one", func(t *testing.T) {
		env := &service.EnvironmentConfig{SMTPFromName: "Env name", SMTPUseTLS: "false"}
		svc, stored := newStoringSetupService(t, env, 1)
		req := wizardRequest()
		req.SMTPFromName = "Typed name"

		require.NoError(t, svc.Initialize(context.Background(), req))
		assert.Equal(t, "Env name", stored["smtp_from_name"])
		assert.Equal(t, "false", stored["smtp_use_tls"])
	})
}

func TestSetupService_Initialize_TelemetryAndUpdatesFromEnv(t *testing.T) {
	t.Run("env values win over the request", func(t *testing.T) {
		env := &service.EnvironmentConfig{Telemetry: "false", CheckForUpdates: "true"}
		svc, stored := newStoringSetupService(t, env, 1)
		req := wizardRequest()
		req.TelemetryEnabled = true
		req.CheckForUpdates = false

		require.NoError(t, svc.Initialize(context.Background(), req))
		assert.Equal(t, "false", stored["telemetry_enabled"])
		assert.Equal(t, "true", stored["check_for_updates"])
	})

	t.Run("without env the request is stored", func(t *testing.T) {
		svc, stored := newStoringSetupService(t, &service.EnvironmentConfig{}, 1)
		req := wizardRequest()
		req.TelemetryEnabled = true
		req.CheckForUpdates = true

		require.NoError(t, svc.Initialize(context.Background(), req))
		assert.Equal(t, "true", stored["telemetry_enabled"])
		assert.Equal(t, "true", stored["check_for_updates"])
	})

	t.Run("GetEnvOverrides lists them only when set", func(t *testing.T) {
		svc := service.NewSetupService(&service.SettingService{}, &service.UserService{}, nil, &mockLogger{}, "k", nil,
			&service.EnvironmentConfig{Telemetry: "true"})
		got := svc.GetEnvOverrides()
		assert.True(t, got["telemetry_enabled"])
		assert.False(t, got["check_for_updates"])
	})
}

func TestSetupService_ValidateSetupConfig_RootEmails(t *testing.T) {
	svc := service.NewSetupService(&service.SettingService{}, &service.UserService{}, nil, &mockLogger{}, "k", nil, &service.EnvironmentConfig{})

	t.Run("several valid emails", func(t *testing.T) {
		req := wizardRequest()
		req.RootEmail = "a@x.com, b@y.com"
		assert.NoError(t, svc.ValidateSetupConfig(req))
	})

	t.Run("an invalid entry is named", func(t *testing.T) {
		req := wizardRequest()
		req.RootEmail = "a@x.com, nope"
		assert.EqualError(t, svc.ValidateSetupConfig(req), "root_email contains an invalid email: nope")
	})

	t.Run("separators only is empty", func(t *testing.T) {
		req := wizardRequest()
		req.RootEmail = " , ; "
		assert.EqualError(t, svc.ValidateSetupConfig(req), "root_email is required")
	})

	t.Run("an env root email is not checked again", func(t *testing.T) {
		envSvc := service.NewSetupService(&service.SettingService{}, &service.UserService{}, nil, &mockLogger{}, "k", nil,
			&service.EnvironmentConfig{RootEmail: "root@acme.com"})
		req := wizardRequest()
		req.RootEmail = ""
		assert.NoError(t, envSvc.ValidateSetupConfig(req))
	})
}

func TestSetupService_Initialize_SeveralRootEmailsCreateThePrimary(t *testing.T) {
	svc, stored := newStoringSetupService(t, &service.EnvironmentConfig{}, 1)
	req := wizardRequest()
	req.RootEmail = "a@x.com, b@x.com"

	require.NoError(t, svc.Initialize(context.Background(), req))
	assert.Equal(t, "a@x.com, b@x.com", stored["root_email"], "the others are created at startup from the stored list")
}

// One precedence rule for every group: a non-empty variable wins, the request fills the
// rest, even when the environment sets the whole group (as the runtime fills it from the
// database at boot).
func TestSetupService_Initialize_OnePrecedenceRule(t *testing.T) {
	envSMTP := func() *service.EnvironmentConfig {
		return &service.EnvironmentConfig{SMTPHost: "smtp.env.com", SMTPPort: 2525, SMTPFromEmail: "env@acme.com"}
	}

	t.Run("an env-set group stores the env values over the request's", func(t *testing.T) {
		svc, stored := newStoringSetupService(t, envSMTP(), 1)
		require.True(t, svc.GetConfigurationStatus().SMTPConfigured)
		req := wizardRequest()
		req.SMTPHost = "smtp.request.com"
		req.SMTPFromEmail = "request@acme.com"

		require.NoError(t, svc.Initialize(context.Background(), req))
		assert.Equal(t, "smtp.env.com", stored["smtp_host"])
		assert.Equal(t, "2525", stored["smtp_port"])
		assert.Equal(t, "env@acme.com", stored["smtp_from_email"])
	})

	t.Run("a field the env leaves empty in an env-set group is stored from the request", func(t *testing.T) {
		svc, stored := newStoringSetupService(t, envSMTP(), 1)
		req := &service.SetupConfig{RootEmail: "admin@example.com", SMTPFromName: "Acme", SMTPUseTLS: false}

		require.NoError(t, svc.Initialize(context.Background(), req))
		assert.Equal(t, "Acme", stored["smtp_from_name"])
		assert.Equal(t, "false", stored["smtp_use_tls"], "SMTP_USE_TLS unset: the request's value")
	})

	t.Run("an env-set OIDC group keeps a redirect URI only the request gives", func(t *testing.T) {
		env := &service.EnvironmentConfig{OIDCEnabled: "true", OIDCIssuerURL: "https://idp.acme.com", OIDCClientID: "cid", OIDCClientSecret: "secret"}
		svc, stored := newStoringSetupService(t, env, 1)
		req := wizardRequest()
		req.OIDCRedirectURI = "https://sso.acme.com/api/user.oidc.callback"

		require.NoError(t, svc.Initialize(context.Background(), req))
		assert.Equal(t, "true", stored["oidc_enabled"])
		assert.Equal(t, "https://idp.acme.com", stored["oidc_issuer_url"])
		assert.Equal(t, "https://sso.acme.com/api/user.oidc.callback", stored["oidc_redirect_uri"])
	})
}

// Values the request adds to an env-set group are stored, so they are checked like the
// rest: a half-filled SMTP login is refused even when SMTP_HOST, SMTP_PORT and
// SMTP_FROM_EMAIL set the group.
func TestSetupService_ValidateSetupConfig_ChecksEnvSetGroups(t *testing.T) {
	env := &service.EnvironmentConfig{SMTPHost: "smtp.env.com", SMTPPort: 2525, SMTPFromEmail: "env@acme.com"}
	svc := service.NewSetupService(&service.SettingService{}, &service.UserService{}, nil, &mockLogger{}, "k", nil, env)
	require.True(t, svc.GetConfigurationStatus().SMTPConfigured)

	req := &service.SetupConfig{RootEmail: "admin@example.com", SMTPUsername: "user"}
	assert.ErrorContains(t, svc.ValidateSetupConfig(req), "SMTP password is required when a username is set")
}
