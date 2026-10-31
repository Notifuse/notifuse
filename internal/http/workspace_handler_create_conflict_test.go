package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// The two refusals of a workspace ID are conflicts: one because a workspace has it, one
// because a leftover database that holds tables does, named in the message.
func TestWorkspaceHandler_Create_Conflicts(t *testing.T) {
	notEmpty := &domain.ErrWorkspaceDatabaseNotEmpty{WorkspaceID: "testworkspace1", DatabaseName: "notifuse_ws_testworkspace1"}

	cases := []struct {
		name       string
		serviceErr error
		wantStatus int
		wantError  string
	}{
		{"already exists", domain.ErrWorkspaceAlreadyExists, http.StatusConflict, "Workspace already exists"},
		{"already exists, wrapped", fmt.Errorf("create: %w", domain.ErrWorkspaceAlreadyExists), http.StatusConflict, "Workspace already exists"},
		{"leftover database with tables, wrapped", fmt.Errorf("create: %w", notEmpty), http.StatusConflict, notEmpty.Error()},
		{"same words, another error", errors.New("workspace already exists in another form"), http.StatusInternalServerError, "Failed to create workspace"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, workspaceSvc, mux, secretKey, _ := setupTest(t)
			workspaceSvc.EXPECT().
				CreateWorkspace(gomock.Any(), "testworkspace1", "Test Workspace", gomock.Any(), gomock.Any(), gomock.Any(), "UTC", gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil, tc.serviceErr)

			body, err := json.Marshal(domain.CreateWorkspaceRequest{
				ID:   "testworkspace1",
				Name: "Test Workspace",
				Settings: domain.WorkspaceSettings{
					Timezone:        "UTC",
					DefaultLanguage: "en",
					Languages:       []string{"en"},
				},
			})
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/api/workspaces.create", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			assert.Equal(t, tc.wantStatus, w.Code)
			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, tc.wantError, resp["error"])
		})
	}
}
