package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
)

// A right magic code on an account with an authenticator: a challenge, and no token, user or
// expiry a client could mistake for a session.
func TestUserHandler_VerifyCode_Challenge(t *testing.T) {
	handler, users, _, _ := setupUserHandlerTest(t)
	expires := time.Date(2026, 11, 23, 12, 10, 0, 0, time.UTC)
	users.EXPECT().VerifyCode(gomock.Any(), gomock.Any()).Return(&domain.AuthResponse{
		User:      domain.User{ID: "u1"},
		Challenge: &domain.TwoFactorChallenge{Token: "challenge-token", ExpiresAt: expires},
	}, nil)

	w := httptest.NewRecorder()
	handler.VerifyCode(w, httptest.NewRequest(http.MethodPost, "/api/user.verify", bytes.NewBufferString(`{"email":"a@x.io","code":"123456"}`)))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["two_factor_required"])
	assert.Equal(t, "challenge-token", body["challenge_token"])
	assert.Equal(t, "2026-11-23T12:10:00Z", body["challenge_expires_at"])
	for _, key := range []string{"token", "user", "expires_at"} {
		assert.NotContains(t, body, key)
	}
}

// Every sign-in response that carries a token is never cached.
func TestUserHandler_SignInResponsesAreNotCached(t *testing.T) {
	handler, users, _, _ := setupUserHandlerTest(t)
	users.EXPECT().VerifyCode(gomock.Any(), gomock.Any()).Return(&domain.AuthResponse{Token: "jwt", User: domain.User{ID: "u1"}}, nil)
	w := httptest.NewRecorder()
	handler.VerifyCode(w, httptest.NewRequest(http.MethodPost, "/api/user.verify", bytes.NewBufferString(`{"email":"a@x.io","code":"123456"}`)))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))

	users.EXPECT().RootSignin(gomock.Any(), gomock.Any()).Return(&domain.AuthResponse{Token: "jwt", User: domain.User{ID: "root"}}, nil)
	w = httptest.NewRecorder()
	handler.RootSignIn(w, httptest.NewRequest(http.MethodPost, "/api/user.rootSignin", bytes.NewBufferString(`{"email":"root@x.io","timestamp":1,"signature":"s"}`)))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}

func meRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/user.me", nil)
	ctx := context.WithValue(req.Context(), domain.UserIDKey, "u1")
	ctx = context.WithValue(ctx, domain.UserTypeKey, string(domain.UserTypeUser))
	ctx = context.WithValue(ctx, domain.SessionIDKey, "s1")
	return req.WithContext(ctx)
}

func TestUserHandler_Me_TwoFactor(t *testing.T) {
	user := &domain.User{ID: "u1", Email: "ann@example.com", Type: domain.UserTypeUser}

	// A console session's workspace carries the file manager's S3 secret: a session that
	// has not met the requirement sees none, and learns it must set up two-factor first.
	t.Run("a session that has not met the requirement sees no workspace", func(t *testing.T) {
		handler, users, workspaces, _ := setupUserHandlerTest(t)
		handler.twoFactor = &stubTwoFactor{required: true}
		users.EXPECT().VerifyUserSession(gomock.Any(), "u1", "s1").Return(user, nil)
		users.EXPECT().GetUserByID(gomock.Any(), "u1").Return(user, nil)
		workspaces.EXPECT().ListWorkspaces(gomock.Any()).Times(0)

		w := httptest.NewRecorder()
		handler.GetCurrentUser(w, meRequest())
		require.Equal(t, http.StatusOK, w.Code)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, []interface{}{}, body["workspaces"])
		assert.Equal(t, true, body["two_factor_required"])
		assert.Equal(t, true, body["two_factor"].(map[string]interface{})["required_by_instance"])
	})

	t.Run("a session that passed it sees its workspaces and the account's state", func(t *testing.T) {
		handler, users, workspaces, _ := setupUserHandlerTest(t)
		enabledAt := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
		handler.twoFactor = &stubTwoFactor{required: true, status: &service.TwoFactorStatus{Enabled: true, EnabledAt: &enabledAt, RecoveryCodesRemaining: 7}}
		handler.authService = &sessionViaUserService{users: users, session: &domain.SessionAuth{AuthMethod: domain.AuthMethodMagicCode, SecondFactor: domain.SecondFactorTOTP}}
		users.EXPECT().VerifyUserSession(gomock.Any(), "u1", "s1").Return(user, nil)
		users.EXPECT().GetUserByID(gomock.Any(), "u1").Return(user, nil)
		workspaces.EXPECT().ListWorkspaces(gomock.Any()).Return([]*domain.Workspace{{ID: "ws1"}}, nil)
		workspaces.EXPECT().UserOwnsAWorkspace(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()

		w := httptest.NewRecorder()
		handler.GetCurrentUser(w, meRequest())
		require.Equal(t, http.StatusOK, w.Code)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Len(t, body["workspaces"], 1)
		assert.Equal(t, false, body["two_factor_required"])
		assert.Equal(t, "totp", body["session_second_factor"])
		state := body["two_factor"].(map[string]interface{})
		assert.Equal(t, true, state["enabled"])
		assert.Equal(t, float64(7), state["recovery_codes_remaining"])
	})

	// ListWorkspaces' refusals answer their status, not 500.
	t.Run("a refused workspace list answers its status", func(t *testing.T) {
		handler, users, workspaces, _ := setupUserHandlerTest(t)
		users.EXPECT().VerifyUserSession(gomock.Any(), "u1", "s1").Return(user, nil)
		users.EXPECT().GetUserByID(gomock.Any(), "u1").Return(user, nil)
		workspaces.EXPECT().ListWorkspaces(gomock.Any()).Return(nil, domain.ErrTwoFactorRequired)

		w := httptest.NewRecorder()
		handler.GetCurrentUser(w, meRequest())
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}
