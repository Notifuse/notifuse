package app

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/branding"
	pkgDatabase "github.com/Notifuse/notifuse/pkg/database"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const brandingQuery = "SELECT key, value, created_at, updated_at FROM settings WHERE key = $1"

// brandingApp builds an App whose startup read of the instance brand is answered by expect. It
// returns the app before InitServices, and the logger, so a test can watch what is logged.
func brandingApp(t *testing.T, expect func(sqlmock.Sqlmock)) (*App, *pkgmocks.MockLogger) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mockDB.Close() })
	expect(mock)

	cfg := createTestConfig()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Fatal(gomock.Any()).AnyTimes()

	require.NoError(t, pkgDatabase.InitializeConnectionManager(cfg, mockDB))
	t.Cleanup(pkgDatabase.ResetConnectionManager)

	app, ok := NewApp(cfg, WithLogger(mockLogger), WithMockDB(mockDB)).(*App)
	require.True(t, ok)
	require.NoError(t, app.InitRepositories())
	return app, mockLogger
}

func storedBrandingRow(value string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"key", "value", "created_at", "updated_at"}).
		AddRow(service.InstanceBrandingSettingKey, value, time.Now(), time.Now())
}

func TestAppBrandingWiring(t *testing.T) {
	t.Run("loads the stored brand once, for the life of the process", func(t *testing.T) {
		stored := `{"name":"Acme","logo_url":"https://cdn.example.com/l.png","icon_url":"","hide_help_menu":false}`
		app, mockLogger := brandingApp(t, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(regexp.QuoteMeta(brandingQuery)).WithArgs(service.InstanceBrandingSettingKey).WillReturnRows(storedBrandingRow(stored))
		})
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		require.NoError(t, app.InitServices())

		want := branding.Branding{Name: "Acme", LogoURL: "https://cdn.example.com/l.png"}
		assert.Equal(t, want.Revision(), app.brandingService.Revision(), "the snapshot is the stored brand")
		// The test licence carries no key, so the brand is held but not shown.
		assert.False(t, app.brandingService.Effective().Custom)
		assert.Equal(t, branding.DefaultName, app.brandName())
	})

	t.Run("a database error stops the start", func(t *testing.T) {
		app, mockLogger := brandingApp(t, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(regexp.QuoteMeta(brandingQuery)).WithArgs(service.InstanceBrandingSettingKey).WillReturnError(errors.New("connection refused"))
		})
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		err := app.InitServices()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "instance branding")
	})

	t.Run("a corrupted brand is logged and replaced by the Notifuse identity", func(t *testing.T) {
		app, mockLogger := brandingApp(t, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(regexp.QuoteMeta(brandingQuery)).WithArgs(service.InstanceBrandingSettingKey).WillReturnRows(storedBrandingRow("{not json"))
		})
		mockLogger.EXPECT().Error(gomock.Any()).MinTimes(1)
		require.NoError(t, app.InitServices(), "a bad brand must not keep the instance from starting")
		assert.Equal(t, branding.Branding{}.Revision(), app.brandingService.Revision())
		assert.False(t, app.brandingService.Effective().Custom)
	})

	t.Run("refuses to build services without repositories, with a reason", func(t *testing.T) {
		app, ok := NewApp(createTestConfig()).(*App)
		require.True(t, ok)
		err := app.InitServices()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "repositories must be initialized")
	})

	t.Run("the mailer reads the brand at send time, Notifuse before the services exist", func(t *testing.T) {
		app, ok := NewApp(createTestConfig()).(*App)
		require.True(t, ok)
		assert.Equal(t, branding.DefaultName, app.brandName())
	})

	t.Run("the setup and settings handlers name the same process", func(t *testing.T) {
		app := initialisedApp(t, nil)
		require.NotNil(t, app.settingsHandler)
		bootID := unexportedField(t, app.settingsHandler, "bootID").String()
		assert.NotEmpty(t, bootID)
	})
}

type brandingLicence struct{}

func (brandingLicence) Entitlements() domain.Entitlements {
	return domain.Entitlements{Features: []domain.Feature{domain.FeatureBranding}}
}

// Every surface that names the instance reads the App's BrandingService when it names it, not
// when it was built: replacing the service after InitServices reaches all of them.
func TestAppBrandingWiring_EverySurfaceReadsTheBrandAtUseTime(t *testing.T) {
	app := initialisedApp(t, nil)
	app.brandingService = service.NewBrandingService(
		branding.Branding{Name: "Acme", LogoURL: "https://cdn.example.com/l.png"}, brandingLicence{})

	require.NoError(t, app.InitMailer())
	named, ok := app.mailer.(interface{ BrandName() string })
	require.True(t, ok, "the mailer exposes the name it prints")
	assert.Equal(t, "Acme", named.BrandName(), "system emails")

	for _, consumer := range []struct {
		name   string
		holder interface{}
		field  string
	}{
		{"provider test email", app.emailService, "brandName"},
		{"test webhook", app.webhookDeliveryWorker, "brandName"},
		{"authenticator issuer", app.twoFactorService, "issuer"},
	} {
		fn, ok := readInterface(unexportedField(t, consumer.holder, consumer.field)).(func() string)
		require.True(t, ok, consumer.name)
		require.NotNil(t, fn, consumer.name)
		assert.Equal(t, "Acme", fn(), consumer.name)
	}

	identity, ok := readInterface(unexportedField(t, app.supabaseService, "branding")).(func() branding.Resolved)
	require.True(t, ok)
	require.NotNil(t, identity)
	assert.Equal(t, "https://cdn.example.com/l.png", identity().LogoURL, "Supabase starter templates")
}
