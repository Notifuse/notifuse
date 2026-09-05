package app

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"unsafe"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	pkgDatabase "github.com/Notifuse/notifuse/pkg/database"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The licence feature has already been shipped inert once. Every constructor took the
// entitlement provider as a TRAILING VARIADIC, app.go kept calling each of them with the old
// argument count, and the result compiled, vetted and tested green while three of the four
// gates were dead in production.
//
// The primary defence is the signature: those parameters are now required and positional, so
// omitting one does not compile. This file is the second line, and it exists for the failures
// the compiler cannot see — a caller that satisfies a signature by passing nil, and a
// struct-literal config whose omitted field is simply a nil nobody typed.
//
// It builds the App exactly the way main does (InitRepositories → InitServices →
// InitHandlers) and then asserts that each consumer holds the SAME licence service the App
// built, not merely something non-nil.

// licenceConsumers names, in one place, every object that must end up holding the licence
// service, and the unexported field it holds it in. Adding a gate means adding a line here.
var licenceConsumers = []struct {
	name  string
	field string
	// holder picks the consumer out of a fully initialised App.
	holder func(a *App) interface{}
}{
	{
		name:   "the workspace service",
		field:  "entitlements",
		holder: func(a *App) interface{} { return a.workspaceService },
	},
	{
		name:   "the SES discovery service",
		field:  "entitlements",
		holder: func(a *App) interface{} { return a.sesDiscoveryService },
	},
	{
		name:   "the telemetry service",
		field:  "entitlements",
		holder: func(a *App) interface{} { return a.telemetryService },
	},
	{
		name:   "the settings handler",
		field:  "licenseService",
		holder: func(a *App) interface{} { return a.settingsHandler },
	},
	{
		// G5, the template-translations gate.
		name:   "the template service",
		field:  "entitlements",
		holder: func(a *App) interface{} { return a.templateService },
	},
	{
		// The SSO gate. OIDCServiceConfig is a struct literal, so omitting this field is a
		// nil no compiler and no unit test would otherwise see: IsEnabled tolerates nil by
		// design — an unwired licence subsystem must not remove a capability — and every
		// OIDC test in the service package constructs the service without a provider on
		// purpose. This assertion is the only thing standing between that tolerance and
		// shipping SSO ungated.
		name:   "the OIDC service",
		field:  "entitlements",
		holder: func(a *App) interface{} { return a.oidcService },
	},
}

func TestAppLicenceWiring(t *testing.T) {
	t.Run("every licence consumer holds the app's licence service", func(t *testing.T) {
		app := initialisedApp(t, nil)

		require.NotNil(t, app.licenseService, "the app must build a licence service")

		for _, consumer := range licenceConsumers {
			t.Run(consumer.name, func(t *testing.T) {
				holder := consumer.holder(app)
				require.NotNil(t, holder, "%s must be constructed", consumer.name)

				got := unexportedField(t, holder, consumer.field)
				require.False(t, got.IsNil(),
					"%s holds a nil %s: the licence gate it owns is dead in production",
					consumer.name, consumer.field)

				assert.Same(t, app.licenseService, readInterface(got),
					"%s must hold the app's own licence service, so a key pasted at runtime "+
						"reaches it without a restart", consumer.name)
			})
		}
	})

	// A gate that cannot be lifted is the failure this whole design refuses. Every licensed
	// capability is bought by pasting a key into the console, so the two endpoints that
	// accept and report one have to exist and have to be wired — a deployment that has paid
	// and cannot install what it paid for is worse off than one that never bought anything.
	t.Run("the licence endpoints are registered and wired", func(t *testing.T) {
		app := initialisedApp(t, nil)

		require.NotNil(t, app.settingsHandler, "the settings handler owns /api/licence.set")
		require.False(t, unexportedField(t, app.settingsHandler, "licenseService").IsNil(),
			"a key could be pasted and would reach nothing")

		_, pattern := app.mux.Handler(httptest.NewRequest(http.MethodGet, "/api/licence.set", nil))
		assert.Equal(t, "/api/licence.set", pattern,
			"the only route that can install a licence must be registered")

		_, pattern = app.mux.Handler(httptest.NewRequest(http.MethodGet, "/api/licence.get", nil))
		assert.Equal(t, "/api/licence.get", pattern)
	})

	// config/oidc.go resolves Enabled env-wins-else-database, so a deployment that switched SSO
	// on from the settings drawer has cfg.OIDC.Enabled true with no environment variable in
	// sight. Telemetry reading cfg.EnvValues.OIDCEnabled would under-count exactly the
	// installations the number is about — the population the SSO gate applies to.
	t.Run("telemetry reports the resolved sso setting not the environment variable", func(t *testing.T) {
		app := initialisedApp(t, func(cfg *config.Config) {
			cfg.OIDC.Enabled = true
			cfg.EnvValues.OIDCEnabled = "" // nothing in the environment; the database decided
		})

		require.NotNil(t, app.telemetryService)
		assert.True(t, unexportedField(t, app.telemetryService, "oidcEnabled").Bool(),
			"telemetry must report the resolved SSO flag, not OIDC_ENABLED")
	})
}

// initialisedApp builds an App through the same three phases main does, against a mock
// database. mutate, when non-nil, adjusts the config before anything is constructed.
func initialisedApp(t *testing.T, mutate func(*config.Config)) *App {
	t.Helper()

	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mockDB.Close() })

	cfg := createTestConfig()
	if mutate != nil {
		mutate(cfg)
	}

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Fatal(gomock.Any()).AnyTimes()

	require.NoError(t, pkgDatabase.InitializeConnectionManager(cfg, mockDB))
	t.Cleanup(pkgDatabase.ResetConnectionManager)

	app, ok := NewApp(cfg, WithLogger(mockLogger), WithMockDB(mockDB)).(*App)
	require.True(t, ok, "NewApp must return *App")

	require.NoError(t, app.InitRepositories())
	require.NoError(t, app.InitServices())
	require.NoError(t, app.InitHandlers())

	return app
}

// unexportedField reads a field the consuming package cannot name. The licence dependency is
// unexported on every consumer — it is nobody's business but the gate's — and exporting it, or
// adding an accessor whose only caller is this file, would widen the production API to make a
// test convenient. reflect reads it without either.
func unexportedField(t *testing.T, holder interface{}, name string) reflect.Value {
	t.Helper()

	v := reflect.ValueOf(holder)
	require.Equal(t, reflect.Ptr, v.Kind(), "expected a pointer to a struct, got %T", holder)

	field := v.Elem().FieldByName(name)
	require.True(t, field.IsValid(), "%T has no field %q — was it renamed?", holder, name)

	return field
}

// readInterface lifts the value out of an unexported field so it can be compared by identity.
// reflect.Value.Interface refuses on an unexported field; reflect.NewAt on its address is the
// documented way around that, and it is confined to this one helper.
func readInterface(field reflect.Value) interface{} {
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
}
