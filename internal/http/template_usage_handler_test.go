package http_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
)

func TestTemplateHandler_HandleUsage(t *testing.T) {
	workspaceID := "workspace123"
	one, zero := 1, 0

	testCases := []struct {
		name           string
		method         string
		query          url.Values
		authenticate   bool
		setupMock      func(*mocks.MockTemplateService)
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "non-GET is refused",
			method:         http.MethodPost,
			query:          url.Values{"workspace_id": {workspaceID}},
			authenticate:   true,
			setupMock:      func(*mocks.MockTemplateService) {},
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "unauthenticated",
			method:         http.MethodGet,
			query:          url.Values{"workspace_id": {workspaceID}},
			setupMock:      func(*mocks.MockTemplateService) {},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "missing workspace_id",
			method:         http.MethodGet,
			query:          url.Values{},
			authenticate:   true,
			setupMock:      func(*mocks.MockTemplateService) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:         "every template, template_id omitted",
			method:       http.MethodGet,
			query:        url.Values{"workspace_id": {workspaceID}},
			authenticate: true,
			setupMock: func(m *mocks.MockTemplateService) {
				u := domain.NewTemplateUsage(false)
				u.Usage["welcome"] = &domain.TemplateUsageCounts{Broadcasts: &one, TransactionalNotifications: &zero, Automations: &zero}
				u.HiddenTypes = []domain.TemplateUsageType{domain.TemplateUsageTypeLists}
				m.EXPECT().GetTemplateUsage(gomock.Any(), workspaceID, "").Return(&u, nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"usage":{"welcome":{"broadcasts":1,"transactional_notifications":0,"automations":0}},"hidden_types":["lists"]}`,
		},
		{
			name:         "one template, with its references",
			method:       http.MethodGet,
			query:        url.Values{"workspace_id": {workspaceID}, "template_id": {"welcome"}},
			authenticate: true,
			setupMock: func(m *mocks.MockTemplateService) {
				u := domain.NewTemplateUsage(true)
				m.EXPECT().GetTemplateUsage(gomock.Any(), workspaceID, "welcome").Return(&u, nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"usage":{},"references":[],"hidden_types":[]}`,
		},
		{
			name:         "permission error is 403",
			method:       http.MethodGet,
			query:        url.Values{"workspace_id": {workspaceID}},
			authenticate: true,
			setupMock: func(m *mocks.MockTemplateService) {
				m.EXPECT().GetTemplateUsage(gomock.Any(), workspaceID, "").
					Return(nil, domain.NewPermissionError(domain.PermissionResourceTemplates, domain.PermissionTypeRead, "Insufficient permissions: read access to templates required"))
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:         "service error is 500",
			method:       http.MethodGet,
			query:        url.Values{"workspace_id": {workspaceID}},
			authenticate: true,
			setupMock: func(m *mocks.MockTemplateService) {
				m.EXPECT().GetTemplateUsage(gomock.Any(), workspaceID, "").Return(nil, errors.New("db error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, serverURL, secretKey, cleanup := setupTemplateHandlerTest(t)
			defer cleanup()
			tc.setupMock(mockService)

			token := ""
			if tc.authenticate {
				token = createTestToken(secretKey)
			}
			resp := sendRequest(t, tc.method, fmt.Sprintf("%s/api/templates.usage?%s", serverURL, tc.query.Encode()), token, nil)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
			if tc.expectedBody != "" {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				assert.JSONEq(t, tc.expectedBody, string(body))
			} else if resp.StatusCode != http.StatusOK {
				var errResp map[string]interface{}
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
				assert.NotEmpty(t, errResp["error"])
			}
		})
	}
}
