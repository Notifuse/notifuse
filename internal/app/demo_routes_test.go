package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	pkgDatabase "github.com/Notifuse/notifuse/pkg/database"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// demo.seed replaces only the workspace it created, so it is served on every
// environment, production included; demo.reset deletes every workspace and is
// served only off production.
func TestInitHandlersRegistersTheDemoRoutesPerEnvironment(t *testing.T) {
	testCases := []struct {
		environment string
		wantReset   bool
	}{
		{"production", false},
		{"development", true},
		{"demo", true},
	}

	for _, tc := range testCases {
		t.Run(tc.environment, func(t *testing.T) {
			mockDB, _, err := setupTestDBMock()
			require.NoError(t, err)
			defer func() { _ = mockDB.Close() }()

			cfg := createTestConfig()
			cfg.Environment = tc.environment
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockLogger := pkgmocks.NewMockLogger(ctrl)
			mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Fatal(gomock.Any()).AnyTimes()

			app := NewApp(cfg, WithLogger(mockLogger), WithMockDB(mockDB))
			require.NoError(t, pkgDatabase.InitializeConnectionManager(cfg, mockDB))
			defer pkgDatabase.ResetConnectionManager()
			require.NoError(t, app.InitRepositories())
			require.NoError(t, app.InitServices())
			require.NoError(t, app.InitHandlers())

			pattern := func(path string) string {
				_, p := app.GetMux().Handler(httptest.NewRequest(http.MethodGet, path, nil))
				return p
			}

			assert.Equal(t, "/api/demo.seed", pattern("/api/demo.seed"))
			if tc.wantReset {
				assert.Equal(t, "/api/demo.reset", pattern("/api/demo.reset"))
			} else {
				assert.NotEqual(t, "/api/demo.reset", pattern("/api/demo.reset"),
					"the reset deletes every workspace and must not be served on production")
			}

			// The seed route reaches the demo handler: a missing HMAC is its 400.
			rec := httptest.NewRecorder()
			app.GetMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/demo.seed", nil))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}
