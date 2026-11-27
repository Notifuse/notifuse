package service

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// When ROOT_EMAIL holds a list, the setup wizard must create the primary (first)
// email as the root user, not a user whose email is the raw comma-joined string.
// The full list is still persisted so InitializeDatabase can create the rest on
// startup.
func TestSetupService_Initialize_UsesPrimaryRootEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingRepo := NewMockSettingRepository()
	settingService := NewSettingService(settingRepo, nil)
	userRepo := domainmocks.NewMockUserRepository(ctrl)

	envConfig := &EnvironmentConfig{RootEmail: "primary@example.com,second@example.com"}

	setupService := NewSetupService(
		settingService,
		&UserService{},
		userRepo,
		logger.NewLogger(),
		"test-secret-key",
		nil,
		envConfig,
	)

	var createdUser *domain.User
	userRepo.EXPECT().
		CreateUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, u *domain.User) error {
			createdUser = u
			return nil
		})

	// SMTP is not env-configured, so it must be provided to pass validation.
	cfg := &SetupConfig{
		APIEndpoint:   "https://api.example.com",
		SMTPHost:      "smtp.example.com",
		SMTPPort:      587,
		SMTPFromEmail: "noreply@example.com",
	}

	err := setupService.Initialize(context.Background(), cfg)
	require.NoError(t, err)

	require.NotNil(t, createdUser)
	assert.Equal(t, "primary@example.com", createdUser.Email, "root user should use the primary email, not the raw list")

	stored, err := settingRepo.Get(context.Background(), "root_email")
	require.NoError(t, err)
	assert.Equal(t, "primary@example.com,second@example.com", stored.Value, "the full root email list should be persisted")
}

// The wizard has no branding fields. Run again on a database that holds a brand (is_installed
// set back to false to redo SMTP, for example), it must keep that brand, even without a licence.
func TestSetupService_Initialize_KeepsAStoredBrand(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingRepo := NewMockSettingRepository()
	stored := `{"name":"Acme","logo_url":"https://cdn.example.com/l.png","icon_url":"","hide_help_menu":true,"background_url":"https://cdn.example.com/bg.jpg"}`
	settingRepo.settings[InstanceBrandingSettingKey] = stored
	settingService := NewSettingService(settingRepo, nil)
	userRepo := domainmocks.NewMockUserRepository(ctrl)
	userRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(nil)

	setupService := NewSetupService(settingService, &UserService{}, userRepo, logger.NewLogger(),
		"test-secret-key", nil, &EnvironmentConfig{RootEmail: "root@example.com"})

	require.NoError(t, setupService.Initialize(context.Background(), &SetupConfig{
		APIEndpoint:   "https://api.example.com",
		SMTPHost:      "smtp.example.com",
		SMTPPort:      587,
		SMTPFromEmail: "noreply@example.com",
	}))

	got, err := settingService.GetInstanceBranding(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Acme", got.Name)
	assert.Equal(t, "https://cdn.example.com/l.png", got.LogoURL)
	assert.True(t, got.HideHelpMenu)
	assert.Equal(t, "https://cdn.example.com/bg.jpg", got.BackgroundURL)
}
