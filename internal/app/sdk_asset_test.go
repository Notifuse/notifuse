package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgDatabase "github.com/Notifuse/notifuse/pkg/database"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// writeBundle materialises the SDK asset under root at the path the loader
// expects, and returns its contents.
func writeBundle(t *testing.T, root string, content []byte) []byte {
	t.Helper()
	path := filepath.Join(root, webAnalyticsSDKRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, content, 0o644))
	return content
}

// quietLogger expects no call: a successful load must log nothing.
func quietLogger(t *testing.T) *pkgmocks.MockLogger {
	t.Helper()
	return pkgmocks.NewMockLogger(gomock.NewController(t))
}

// warningLogger expects exactly one warning, which is the only signal an
// operator gets that /na.js will be missing.
func warningLogger(t *testing.T) *pkgmocks.MockLogger {
	t.Helper()
	ctrl := gomock.NewController(t)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField("path", webAnalyticsSDKRelPath).Return(log).Times(1)
	log.EXPECT().Warn(gomock.Any()).Times(1)
	return log
}

func TestLoadWebAnalyticsSDK(t *testing.T) {
	t.Run("reads the bundle from the working directory", func(t *testing.T) {
		dir := t.TempDir()
		want := writeBundle(t, dir, []byte("!function(){/* sdk */}()"))
		t.Chdir(dir)

		assert.Equal(t, want, loadWebAnalyticsSDK(quietLogger(t)))
	})

	t.Run("finds the bundle from a subdirectory", func(t *testing.T) {
		// The integration suite runs from tests/integration, two levels below
		// the tree that carries the asset. Without the upward search those
		// tests would exercise a server with no SDK route and still pass.
		root := t.TempDir()
		want := writeBundle(t, root, []byte("!function(){/* sdk */}()"))
		sub := filepath.Join(root, "tests", "integration")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		t.Chdir(sub)

		assert.Equal(t, want, loadWebAnalyticsSDK(quietLogger(t)))
	})

	t.Run("returns nil and warns when the bundle is absent", func(t *testing.T) {
		t.Chdir(t.TempDir())

		assert.Nil(t, loadWebAnalyticsSDK(warningLogger(t)))
	})

	t.Run("returns nil and warns when the bundle is empty", func(t *testing.T) {
		// An empty file is what a failed or interrupted build leaves behind.
		// Returning it would register /na.js serving a zero-byte script, which
		// fails on the customer's page rather than here.
		dir := t.TempDir()
		writeBundle(t, dir, nil)
		t.Chdir(dir)

		assert.Nil(t, loadWebAnalyticsSDK(warningLogger(t)))
	})

	t.Run("does not search beyond the bounded depth", func(t *testing.T) {
		root := t.TempDir()
		writeBundle(t, root, []byte("!function(){}()"))
		deep := filepath.Join(root, "a", "b", "c", "d", "e", "f", "g")
		require.NoError(t, os.MkdirAll(deep, 0o755))
		t.Chdir(deep)

		assert.Nil(t, loadWebAnalyticsSDK(warningLogger(t)))
	})
}

// TestInitHandlersServesSDKFromDisk closes the one gap the unit tests above
// leave open: that InitHandlers actually hands the bytes read from disk to the
// web analytics handler.
//
// It is worth a full app wiring because the previous arrangement — //go:embed —
// could not fail this way. The bundle was linked into the binary, so it was
// either there at compile time or the build broke. Reading it at run time moves
// that failure to startup, where nothing but a warning marks it, and a
// regression would show up as tracking silently not working.
//
// The route is only registered when a bundle is found, so this also asserts the
// loader resolves the committed bundle from the package directory.
func TestInitHandlersServesSDKFromDisk(t *testing.T) {
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

	rec := httptest.NewRecorder()
	app.GetMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/na.js", nil))

	require.Equal(t, http.StatusOK, rec.Code,
		"/na.js is unregistered, which means no SDK bundle reached the handler")
	assert.Contains(t, rec.Header().Get("Content-Type"), "javascript")

	// The notices are required by AGPL section 4 and are stripped by default,
	// so assert them on the bytes actually served rather than on the config
	// that is supposed to preserve them.
	body := rec.Body.String()
	assert.True(t, strings.HasPrefix(body, "/*!"),
		"the served bundle must open with its licence banner")
	assert.Contains(t, body, "AGPL-3.0-or-later")
	assert.Contains(t, body, "ua-parser-js")
}
