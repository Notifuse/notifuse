package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertTrackingPixel(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "image/png", rr.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache, no-store, must-revalidate", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", rr.Header().Get("Pragma"))
	assert.Equal(t, "0", rr.Header().Get("Expires"))
	assert.Equal(t, paddedTrackingPixel, rr.Body.Bytes())
}

func coverageEncryptedOpenRequest(t *testing.T, handler *EmailHandler, path, userAgent string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("User-Agent", userAgent)
	rr := httptest.NewRecorder()
	handler.handleEncryptedOpen(rr, req.WithContext(context.Background()))
	return rr
}

const coverageBrowserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0"

func TestCoverage_HandleEncryptedOpen(t *testing.T) {
	t.Run("empty token returns pixel and records nothing", func(t *testing.T) {
		_, _, handler, _ := setupEmailHandlerTest(t) // no OpenEmail EXPECT
		rr := coverageEncryptedOpenRequest(t, handler, "/t/", coverageBrowserUA)
		assertTrackingPixel(t, rr)
	})

	t.Run("invalid token returns pixel silently and records nothing", func(t *testing.T) {
		_, _, handler, _ := setupEmailHandlerTest(t)
		rr := coverageEncryptedOpenRequest(t, handler, "/t/not-a-valid-token", coverageBrowserUA)
		assertTrackingPixel(t, rr)
	})

	t.Run("decrypted payload with too few parts records nothing", func(t *testing.T) {
		for _, payload := range []string{"only-one-part", "message-123\nworkspace-123"} {
			_, _, handler, _ := setupEmailHandlerTest(t)
			token, err := crypto.EncryptTrackingToken(payload)
			require.NoError(t, err)
			rr := coverageEncryptedOpenRequest(t, handler, "/t/"+token, coverageBrowserUA)
			assertTrackingPixel(t, rr)
		}
	})

	t.Run("valid old token records the exact open", func(t *testing.T) {
		mockService, _, handler, _ := setupEmailHandlerTest(t)
		oldTS := strconv.FormatInt(time.Now().Add(-10*time.Second).Unix(), 10)
		token, err := crypto.EncryptTrackingToken("message-123\nworkspace-123\n" + oldTS)
		require.NoError(t, err)
		mockService.EXPECT().OpenEmail(gomock.Any(), "message-123", "workspace-123").Return(nil)

		rr := coverageEncryptedOpenRequest(t, handler, "/t/"+token, coverageBrowserUA)
		assertTrackingPixel(t, rr)
	})

	t.Run("valid token with four parts uses first three fields", func(t *testing.T) {
		// SplitN with limit 3 keeps any extra newline inside the timestamp part,
		// which then fails to parse — the open is therefore recorded (parse
		// failure leaves shouldRecord true) only when the ts parses old. Use an
		// old ts plus trailing field separated so SplitN(3) yields ts="old\nextra"
		// which is unparseable; recording still happens. Assert that behaviour.
		mockService, _, handler, _ := setupEmailHandlerTest(t)
		oldTS := strconv.FormatInt(time.Now().Add(-10*time.Second).Unix(), 10)
		token, err := crypto.EncryptTrackingToken("message-123\nworkspace-123\n" + oldTS + "\nextra")
		require.NoError(t, err)
		mockService.EXPECT().OpenEmail(gomock.Any(), "message-123", "workspace-123").Return(nil)

		rr := coverageEncryptedOpenRequest(t, handler, "/t/"+token, coverageBrowserUA)
		assertTrackingPixel(t, rr)
	})

	t.Run("bot user agent records nothing but still serves pixel", func(t *testing.T) {
		_, _, handler, _ := setupEmailHandlerTest(t) // no OpenEmail EXPECT
		oldTS := strconv.FormatInt(time.Now().Add(-10*time.Second).Unix(), 10)
		token, err := crypto.EncryptTrackingToken("message-123\nworkspace-123\n" + oldTS)
		require.NoError(t, err)

		rr := coverageEncryptedOpenRequest(t, handler, "/t/"+token, "Googlebot/2.1 (+http://www.google.com/bot.html)")
		assertTrackingPixel(t, rr)
	})

	t.Run("too-fast open records nothing but still serves pixel", func(t *testing.T) {
		_, _, handler, _ := setupEmailHandlerTest(t) // no OpenEmail EXPECT
		freshTS := strconv.FormatInt(time.Now().Unix(), 10)
		token, err := crypto.EncryptTrackingToken("message-123\nworkspace-123\n" + freshTS)
		require.NoError(t, err)

		rr := coverageEncryptedOpenRequest(t, handler, "/t/"+token, coverageBrowserUA)
		assertTrackingPixel(t, rr)
	})

	t.Run("unparseable timestamp does not suppress recording", func(t *testing.T) {
		mockService, _, handler, _ := setupEmailHandlerTest(t)
		token, err := crypto.EncryptTrackingToken("message-123\nworkspace-123\nnot-a-timestamp")
		require.NoError(t, err)
		mockService.EXPECT().OpenEmail(gomock.Any(), "message-123", "workspace-123").Return(nil)

		rr := coverageEncryptedOpenRequest(t, handler, "/t/"+token, coverageBrowserUA)
		assertTrackingPixel(t, rr)
	})

	t.Run("empty timestamp does not suppress recording", func(t *testing.T) {
		mockService, _, handler, _ := setupEmailHandlerTest(t)
		token, err := crypto.EncryptTrackingToken("message-123\nworkspace-123\n")
		require.NoError(t, err)
		mockService.EXPECT().OpenEmail(gomock.Any(), "message-123", "workspace-123").Return(nil)

		rr := coverageEncryptedOpenRequest(t, handler, "/t/"+token, coverageBrowserUA)
		assertTrackingPixel(t, rr)
	})

	t.Run("service error is swallowed and pixel still served", func(t *testing.T) {
		mockService, _, handler, _ := setupEmailHandlerTest(t)
		oldTS := strconv.FormatInt(time.Now().Add(-10*time.Second).Unix(), 10)
		token, err := crypto.EncryptTrackingToken("message-123\nworkspace-123\n" + oldTS)
		require.NoError(t, err)
		mockService.EXPECT().OpenEmail(gomock.Any(), "message-123", "workspace-123").
			Return(assert.AnError)

		rr := coverageEncryptedOpenRequest(t, handler, "/t/"+token, coverageBrowserUA)
		assertTrackingPixel(t, rr)
	})
}
