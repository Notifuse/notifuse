package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	httpHandler "github.com/Notifuse/notifuse/internal/http"
	pkgDatabase "github.com/Notifuse/notifuse/pkg/database"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// /v/ is served by the web version handler, not the root handler's fallback, and both
// of its rate-limit policies are registered: without one, every request is refused.
func TestInitHandlersServesTheWebVersionRoute(t *testing.T) {
	mockDB, _, err := setupTestDBMock()
	require.NoError(t, err)
	defer func() { _ = mockDB.Close() }()

	cfg := createTestConfig()
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

	_, pattern := app.GetMux().Handler(httptest.NewRequest(http.MethodGet, "/v/token", nil))
	assert.Equal(t, "/v/", pattern)

	rec := httptest.NewRecorder()
	app.GetMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v/not-a-token", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code, "a 429 here means the address policy is missing")
	assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "sandbox")

	rl := app.(*App).rateLimiter
	assert.True(t, rl.Allow(httpHandler.WebVersionIPPolicy, "probe"))
	assert.True(t, rl.Allow(httpHandler.WebVersionMessagePolicy, "probe"))
}
