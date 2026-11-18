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
