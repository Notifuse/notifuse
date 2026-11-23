package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// An invitee with two-factor authentication: the invitation waits for the challenge, and the
// response carries no token.
func TestWorkspaceHandler_AcceptInvitation_Challenge(t *testing.T) {
	handler, workspaceSvc, _, _, authSvc := setupTest(t)
	authSvc.EXPECT().ValidateInvitationToken("valid-token").Return("inv-1", "ws1", "ann@example.com", nil)
	workspaceSvc.EXPECT().GetInvitationByID(gomock.Any(), "inv-1").
		Return(&domain.WorkspaceInvitation{ID: "inv-1", WorkspaceID: "ws1", Email: "ann@example.com", ExpiresAt: time.Now().Add(time.Hour)}, nil).AnyTimes()
	workspaceSvc.EXPECT().AcceptInvitation(gomock.Any(), "inv-1", "ws1", "ann@example.com").
		Return(&domain.AuthResponse{User: domain.User{ID: "u1"}, Challenge: &domain.TwoFactorChallenge{Token: "challenge", ExpiresAt: time.Now().Add(10 * time.Minute)}}, nil)

	w := httptest.NewRecorder()
	handler.handleAcceptInvitation(w, httptest.NewRequest(http.MethodPost, "/api/workspaces.acceptInvitation", bytes.NewBufferString(`{"token":"valid-token"}`)))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["two_factor_required"])
	assert.Equal(t, "challenge", body["challenge_token"])
	assert.NotContains(t, body, "token")
}

// workspaces.create answers the refusals of the root check with their status, not 500.
func TestWorkspaceHandler_Create_AuthenticationRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		status int
	}{
		"second factor required": {domain.ErrTwoFactorRequired, http.StatusForbidden},
		"not root":               {&domain.ErrUnauthorized{Message: "only root user can create workspaces"}, http.StatusForbidden},
		"session gone":           {fmt.Errorf("%w: expired", domain.ErrSessionAuthFailed), http.StatusUnauthorized},
	} {
		t.Run(name, func(t *testing.T) {
			_, workspaceSvc, mux, secretKey, _ := setupTest(t)
			workspaceSvc.EXPECT().CreateWorkspace(gomock.Any(), "testworkspace1", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil, tc.err)
			body, _ := json.Marshal(domain.CreateWorkspaceRequest{
				ID:   "testworkspace1",
				Name: "Test Workspace",
				Settings: domain.WorkspaceSettings{
					Timezone: "UTC", DefaultLanguage: "en", Languages: []string{"en"},
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/api/workspaces.create", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			assert.Equal(t, tc.status, w.Code, w.Body.String())
		})
	}
}
