package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestWritePermissionError_TwoFactor(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrTwoFactorRequired, http.StatusForbidden, "two_factor_required"},
		{domain.ErrReauthenticationRequired, http.StatusForbidden, "reauthentication_required"},
		{domain.ErrTwoFactorInvalidCode, http.StatusUnprocessableEntity, "two_factor_invalid_code"},
		{domain.ErrTwoFactorChallengeInvalid, http.StatusGone, "two_factor_challenge_invalid"},
		{domain.ErrTwoFactorSetupExpired, http.StatusGone, "two_factor_setup_expired"},
		{domain.ErrTwoFactorTOTPLocked, http.StatusTooManyRequests, "two_factor_totp_locked"},
		{domain.ErrTwoFactorRequiredByInstance, http.StatusConflict, "two_factor_required_by_instance"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			w := httptest.NewRecorder()
			// Wrapped, as services return it.
			require.True(t, writePermissionError(w, fmt.Errorf("failed to authenticate user: %w", tc.err)))
			assert.Equal(t, tc.status, w.Code)
			assert.NotEqual(t, http.StatusUnauthorized, w.Code, "a 401 makes the console drop its token")
			var body map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, tc.code, body["error"])
		})
	}

	t.Run("a wait carries retry_after in whole seconds", func(t *testing.T) {
		w := httptest.NewRecorder()
		require.True(t, writePermissionError(w, &domain.TwoFactorLockedError{RetryAfter: 61500 * time.Millisecond}))
		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "two_factor_locked", body["error"])
		assert.Equal(t, float64(62), body["retry_after"])
	})

	t.Run("a failed session answers 401, a database error is left to the caller", func(t *testing.T) {
		w := httptest.NewRecorder()
		require.True(t, writePermissionError(w, fmt.Errorf("%w: session expired", domain.ErrSessionAuthFailed)))
		assert.Equal(t, http.StatusUnauthorized, w.Code)

		assert.False(t, writePermissionError(httptest.NewRecorder(), errors.New("connection refused")))
	})
}
