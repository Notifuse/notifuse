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
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
)

type twoFactorHandlerFixture struct {
	handler *TwoFactorHandler
	stub    *stubTwoFactor
	auth    *mocks.MockAuthService
}

func newTwoFactorHandlerFixture(t *testing.T, limiter *ratelimiter.RateLimiter) *twoFactorHandlerFixture {
	ctrl := gomock.NewController(t)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	f := &twoFactorHandlerFixture{stub: &stubTwoFactor{}, auth: mocks.NewMockAuthService(ctrl)}
	f.handler = NewTwoFactorHandler(f.stub, f.auth, limiter, func() ([]byte, error) { return []byte("secret"), nil }, false, log)
	return f
}

func consoleCtx(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, domain.UserIDKey, "u1")
	ctx = context.WithValue(ctx, domain.SessionIDKey, "s1")
	return context.WithValue(ctx, domain.UserTypeKey, string(domain.UserTypeUser))
}

func postTF(t *testing.T, h http.HandlerFunc, body string, ctx func(context.Context) context.Context) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/x", bytes.NewBufferString(body))
	if ctx != nil {
		req = req.WithContext(ctx(req.Context()))
	}
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func decodeTF(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func TestTwoFactorHandler_Verify(t *testing.T) {
	t.Run("a right code returns the session token, never cached", func(t *testing.T) {
		f := newTwoFactorHandlerFixture(t, nil)
		f.stub.verify = func(token string, proof service.TwoFactorProof, meta service.TwoFactorRequestMeta) (*domain.AuthResponse, error) {
			assert.Equal(t, "challenge", token)
			assert.Equal(t, "123456", proof.Code)
			assert.Equal(t, "198.51.100.7", meta.IP)
			return &domain.AuthResponse{Token: "jwt", User: domain.User{ID: "u1"}, ExpiresAt: time.Now(), InvitationStatus: service.InvitationTeamMemberLimit}, nil
		}
		req := httptest.NewRequest(http.MethodPost, "/api/user.twoFactor.verify", bytes.NewBufferString(`{"challenge_token":"challenge","code":"123456"}`))
		req.Header.Set("X-Forwarded-For", "198.51.100.7")
		w := httptest.NewRecorder()
		f.handler.Verify(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		body := decodeTF(t, w)
		assert.Equal(t, "jwt", body["token"])
		assert.Equal(t, "team_member_limit", body["invitation_status"])
	})

	// A wrong code must not sign the console out: 422, not 401.
	t.Run("a wrong code answers 422", func(t *testing.T) {
		f := newTwoFactorHandlerFixture(t, nil)
		f.stub.verify = func(string, service.TwoFactorProof, service.TwoFactorRequestMeta) (*domain.AuthResponse, error) {
			return nil, domain.ErrTwoFactorInvalidCode
		}
		w := postTF(t, f.handler.Verify, `{"challenge_token":"c","code":"000000"}`, nil)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		assert.Equal(t, "two_factor_invalid_code", decodeTF(t, w)["error"])
	})

	t.Run("past the address's budget, 429 before any lookup", func(t *testing.T) {
		limiter := ratelimiter.NewRateLimiter()
		limiter.SetPolicy(TwoFactorVerifyPolicy, 1, time.Minute)
		f := newTwoFactorHandlerFixture(t, limiter)
		calls := 0
		f.stub.verify = func(string, service.TwoFactorProof, service.TwoFactorRequestMeta) (*domain.AuthResponse, error) {
			calls++
			return nil, domain.ErrTwoFactorInvalidCode
		}
		postTF(t, f.handler.Verify, `{"challenge_token":"c","code":"000000"}`, nil)
		w := postTF(t, f.handler.Verify, `{"challenge_token":"c","code":"000000"}`, nil)
		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Equal(t, 1, calls)
	})

	// A public route: a body naming no challenge writes no audit row.
	t.Run("a malformed body is not audited", func(t *testing.T) {
		f := newTwoFactorHandlerFixture(t, nil)
		record := domain.NewAuditRecord("203.0.113.1", "test", "req-1")
		req := httptest.NewRequest(http.MethodPost, "/api/user.twoFactor.verify", bytes.NewBufferString(`{`))
		w := httptest.NewRecorder()
		f.handler.Verify(w, req.WithContext(domain.WithAuditRecord(req.Context(), record)))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		_, skipped := record.Finalize(w.Code)
		assert.True(t, skipped)
	})

	t.Run("only POST", func(t *testing.T) {
		f := newTwoFactorHandlerFixture(t, nil)
		w := httptest.NewRecorder()
		f.handler.Verify(w, httptest.NewRequest(http.MethodGet, "/api/user.twoFactor.verify", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestTwoFactorHandler_SetupAndEnable(t *testing.T) {
	// Open to a session that has not met the instance's requirement: it is how it meets it.
	t.Run("setup passes the session that has not met the requirement", func(t *testing.T) {
		f := newTwoFactorHandlerFixture(t, nil)
		f.auth.EXPECT().AuthenticateSession(gomock.Any()).Return(&domain.User{ID: "u1"}, &domain.SessionAuth{}, nil)
		f.stub.setup = func(userID, sessionID string, _ service.TwoFactorProof) (*service.TwoFactorSetup, error) {
			assert.Equal(t, "u1", userID)
			assert.Equal(t, "s1", sessionID)
			return &service.TwoFactorSetup{Secret: "ABC", OtpauthURI: "otpauth://totp/x"}, nil
		}
		w := postTF(t, f.handler.Setup, `{}`, consoleCtx)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		assert.Equal(t, "otpauth://totp/x", decodeTF(t, w)["otpauth_uri"])
	})

	t.Run("a stale session is told to sign in again, with 403", func(t *testing.T) {
		f := newTwoFactorHandlerFixture(t, nil)
		f.auth.EXPECT().AuthenticateSession(gomock.Any()).Return(&domain.User{ID: "u1"}, &domain.SessionAuth{}, nil)
		f.stub.setup = func(string, string, service.TwoFactorProof) (*service.TwoFactorSetup, error) {
			return nil, domain.ErrReauthenticationRequired
		}
		w := postTF(t, f.handler.Setup, `{}`, consoleCtx)
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Equal(t, "reauthentication_required", decodeTF(t, w)["error"])
	})

	t.Run("an API key gets 403", func(t *testing.T) {
		f := newTwoFactorHandlerFixture(t, nil)
		w := postTF(t, f.handler.Setup, `{}`, func(ctx context.Context) context.Context {
			ctx = context.WithValue(ctx, domain.UserIDKey, "key")
			return context.WithValue(ctx, domain.UserTypeKey, string(domain.UserTypeAPIKey))
		})
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("enable signs the other sessions out unless told not to", func(t *testing.T) {
		for body, want := range map[string]bool{`{"code":"1"}`: true, `{"code":"1","sign_out_other_sessions":false}`: false} {
			f := newTwoFactorHandlerFixture(t, nil)
			f.auth.EXPECT().AuthenticateSession(gomock.Any()).Return(&domain.User{ID: "u1"}, &domain.SessionAuth{}, nil)
			f.stub.enable = func(_, _, _ string, signOut bool) ([]string, error) {
				assert.Equal(t, want, signOut)
				return nil, nil
			}
			w := postTF(t, f.handler.Enable, body, consoleCtx)
			require.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, []interface{}{}, decodeTF(t, w)["recovery_codes"], "an empty list, never null")
		}
	})

	t.Run("an expired setup answers 410", func(t *testing.T) {
		f := newTwoFactorHandlerFixture(t, nil)
		f.auth.EXPECT().AuthenticateSession(gomock.Any()).Return(&domain.User{ID: "u1"}, &domain.SessionAuth{}, nil)
		f.stub.enable = func(string, string, string, bool) ([]string, error) { return nil, domain.ErrTwoFactorSetupExpired }
		w := postTF(t, f.handler.Enable, `{"code":"1"}`, consoleCtx)
		assert.Equal(t, http.StatusGone, w.Code)
	})
}

func TestTwoFactorHandler_SensitiveActions(t *testing.T) {
	// Disabling needs a session that meets the requirement: one that has not met it has no
	// two-factor authentication to turn off.
	t.Run("disable goes through the requirement", func(t *testing.T) {
		f := newTwoFactorHandlerFixture(t, nil)
		f.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(nil, domain.ErrTwoFactorRequired)
		w := postTF(t, f.handler.Disable, `{"code":"1"}`, consoleCtx)
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Equal(t, "two_factor_required", decodeTF(t, w)["error"])
	})

	t.Run("disable refused while the instance requires it: 409", func(t *testing.T) {
		f := newTwoFactorHandlerFixture(t, nil)
		f.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(&domain.User{ID: "u1"}, nil)
		f.stub.disable = func(string, service.TwoFactorProof) error { return domain.ErrTwoFactorRequiredByInstance }
		w := postTF(t, f.handler.Disable, `{"code":"1"}`, consoleCtx)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("regenerate during a wait: 429 with retry_after", func(t *testing.T) {
		f := newTwoFactorHandlerFixture(t, nil)
		f.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(&domain.User{ID: "u1"}, nil)
		f.stub.regen = func(string, service.TwoFactorProof) ([]string, error) {
			return nil, &domain.TwoFactorLockedError{RetryAfter: 2 * time.Minute}
		}
		w := postTF(t, f.handler.RegenerateRecoveryCodes, `{"recovery_code":"x"}`, consoleCtx)
		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Equal(t, float64(120), decodeTF(t, w)["retry_after"])
	})

	t.Run("reset takes user_id or email, not both", func(t *testing.T) {
		f := newTwoFactorHandlerFixture(t, nil)
		assert.Equal(t, http.StatusBadRequest, postTF(t, f.handler.ResetForUser, `{}`, consoleCtx).Code)
		assert.Equal(t, http.StatusBadRequest, postTF(t, f.handler.ResetForUser, `{"user_id":"a","email":"b@x.io"}`, consoleCtx).Code)
	})

	t.Run("reset by a non-root user: 403", func(t *testing.T) {
		f := newTwoFactorHandlerFixture(t, nil)
		f.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(&domain.User{ID: "u1"}, nil)
		f.stub.reset = func(rootID, rootSession string, _ service.TwoFactorProof, userID, _ string) error {
			assert.Equal(t, "u1", rootID)
			assert.Equal(t, "s1", rootSession)
			assert.Equal(t, "u2", userID)
			return &domain.ErrUnauthorized{Message: "root user access required"}
		}
		w := postTF(t, f.handler.ResetForUser, `{"user_id":"u2"}`, consoleCtx)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("reset of one's own account: 400", func(t *testing.T) {
		f := newTwoFactorHandlerFixture(t, nil)
		f.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(&domain.User{ID: "u1"}, nil)
		f.stub.reset = func(string, string, service.TwoFactorProof, string, string) error { return service.ErrCannotResetOwnTwoFactor }
		w := postTF(t, f.handler.ResetForUser, `{"user_id":"u1"}`, consoleCtx)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// The routes are registered, and closed on a demo instance.
func TestTwoFactorHandler_RegisterRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	log := pkgmocks.NewMockLogger(ctrl)
	h := NewTwoFactorHandler(&stubTwoFactor{}, mocks.NewMockAuthService(ctrl), nil, func() ([]byte, error) { return []byte("secret"), nil }, true, log)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/user.twoFactor.verify", bytes.NewBufferString(`{}`)))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "demo")
	for _, route := range []string{"setup", "enable", "disable", "regenerateRecoveryCodes"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/user.twoFactor."+route, nil))
		assert.Equal(t, http.StatusUnauthorized, w.Code, route+" needs a token")
	}
}
