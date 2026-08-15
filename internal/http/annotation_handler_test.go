package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAnnotationHandlerTest prepares test dependencies and creates an annotation handler
func setupAnnotationHandlerTest(t *testing.T, isDemo bool) (*mocks.MockAnnotationService, *AnnotationHandler) {
	ctrl := gomock.NewController(t)
	t.Cleanup(func() { ctrl.Finish() })

	mockService := mocks.NewMockAnnotationService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	handler := NewAnnotationHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger, isDemo)
	return mockService, handler
}

func newAnnotationJSONRequest(t *testing.T, method, target string, body interface{}) *http.Request {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(method, target, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestNewAnnotationHandler(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)

	assert.NotNil(t, handler)
	assert.Equal(t, mockService, handler.service)
	assert.NotNil(t, handler.logger)
	assert.NotNil(t, handler.getJWTSecret)
	assert.False(t, handler.isDemo)
}

func TestAnnotationHandler_RegisterRoutes(t *testing.T) {
	_, handler := setupAnnotationHandlerTest(t, false)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	endpoints := []struct {
		path   string
		method string
	}{
		{"/api/annotations.list", http.MethodGet},
		{"/api/annotations.get", http.MethodGet},
		{"/api/annotations.create", http.MethodPost},
		{"/api/annotations.update", http.MethodPost},
		{"/api/annotations.delete", http.MethodPost},
	}

	for _, endpoint := range endpoints {
		req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
		// The resolved pattern, not just "a handler came back": the mux answers every
		// unregistered path with the 404 handler, so a non-nil match proves nothing.
		match, pattern := mux.Handler(req)
		require.NotNil(t, match, "no handler registered for %s", endpoint.path)
		assert.Equal(t, endpoint.path, pattern, "unexpected pattern for %s", endpoint.path)
	}
}

func TestAnnotationHandler_MethodNotAllowed(t *testing.T) {
	_, handler := setupAnnotationHandlerTest(t, false)

	testCases := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		target  string
	}{
		{"list rejects POST", handler.handleList, http.MethodPost, "/api/annotations.list"},
		{"get rejects POST", handler.handleGet, http.MethodPost, "/api/annotations.get"},
		{"create rejects GET", handler.handleCreate, http.MethodGet, "/api/annotations.create"},
		{"update rejects GET", handler.handleUpdate, http.MethodGet, "/api/annotations.update"},
		{"delete rejects GET", handler.handleDelete, http.MethodGet, "/api/annotations.delete"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			tc.handler(rr, httptest.NewRequest(tc.method, tc.target, nil))
			assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		})
	}
}

func TestAnnotationHandler_List_ParsesRangeAndSources(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)

	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 31, 23, 59, 59, 0, time.UTC)

	var captured *domain.ListAnnotationsRequest
	mockService.EXPECT().
		ListAnnotations(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ interface{}, req *domain.ListAnnotationsRequest) ([]*domain.Annotation, error) {
			captured = req
			return []*domain.Annotation{{ID: "annot1", Title: "Launch"}}, nil
		})

	target := fmt.Sprintf("/api/annotations.list?workspace_id=ws1&start=%s&end=%s&sources=manual,broadcast&limit=25",
		start.Format(time.RFC3339), end.Format(time.RFC3339))
	rr := httptest.NewRecorder()
	handler.handleList(rr, httptest.NewRequest(http.MethodGet, target, nil))

	require.Equal(t, http.StatusOK, rr.Code)
	require.NotNil(t, captured)
	assert.Equal(t, "ws1", captured.WorkspaceID)
	require.NotNil(t, captured.Start)
	require.NotNil(t, captured.End)
	assert.True(t, captured.Start.Equal(start))
	assert.True(t, captured.End.Equal(end))
	assert.Equal(t, []string{"manual", "broadcast"}, captured.Sources)
	assert.Equal(t, 25, captured.Limit)

	var response struct {
		Annotations []*domain.Annotation `json:"annotations"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	require.Len(t, response.Annotations, 1)
	assert.Equal(t, "annot1", response.Annotations[0].ID)
}

func TestAnnotationHandler_List_MalformedStartIsBadRequest(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)

	// A zero time would silently mean "no lower bound" and answer a question the
	// caller never asked, so the request is refused instead.
	mockService.EXPECT().ListAnnotations(gomock.Any(), gomock.Any()).Times(0)

	rr := httptest.NewRecorder()
	handler.handleList(rr, httptest.NewRequest(http.MethodGet, "/api/annotations.list?workspace_id=ws1&start=not-a-date", nil))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "start")
}

func TestAnnotationHandler_List_MissingWorkspaceID(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)
	mockService.EXPECT().ListAnnotations(gomock.Any(), gomock.Any()).Times(0)

	rr := httptest.NewRecorder()
	handler.handleList(rr, httptest.NewRequest(http.MethodGet, "/api/annotations.list", nil))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "workspace_id")
}

func TestAnnotationHandler_Get_Success(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)

	mockService.EXPECT().
		GetAnnotation(gomock.Any(), &domain.GetAnnotationRequest{WorkspaceID: "ws1", ID: "annot1"}).
		Return(&domain.Annotation{ID: "annot1", Title: "Launch"}, nil)

	rr := httptest.NewRecorder()
	handler.handleGet(rr, httptest.NewRequest(http.MethodGet, "/api/annotations.get?workspace_id=ws1&id=annot1", nil))

	require.Equal(t, http.StatusOK, rr.Code)
	var response struct {
		Annotation *domain.Annotation `json:"annotation"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	require.NotNil(t, response.Annotation)
	assert.Equal(t, "annot1", response.Annotation.ID)
}

func TestAnnotationHandler_Get_NotFound(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)

	mockService.EXPECT().
		GetAnnotation(gomock.Any(), gomock.Any()).
		Return(nil, &domain.ErrNotFound{Entity: "annotation", ID: "missing"})

	rr := httptest.NewRecorder()
	handler.handleGet(rr, httptest.NewRequest(http.MethodGet, "/api/annotations.get?workspace_id=ws1&id=missing", nil))

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), "annotation not found")
}

func TestAnnotationHandler_Get_MissingParams(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)
	mockService.EXPECT().GetAnnotation(gomock.Any(), gomock.Any()).Times(0)

	rr := httptest.NewRecorder()
	handler.handleGet(rr, httptest.NewRequest(http.MethodGet, "/api/annotations.get?workspace_id=ws1", nil))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "id is required")
}

func TestAnnotationHandler_Create_Success(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)

	annotatedAt := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	created := &domain.Annotation{
		ID:          "annot1",
		AnnotatedAt: annotatedAt,
		Timezone:    "Asia/Tokyo",
		Title:       "Launch",
		Color:       domain.AnnotationDefaultColor,
		Source:      domain.AnnotationSourceManual,
	}
	mockService.EXPECT().CreateAnnotation(gomock.Any(), gomock.Any()).Return(created, nil)

	req := newAnnotationJSONRequest(t, http.MethodPost, "/api/annotations.create", domain.CreateAnnotationRequest{
		WorkspaceID: "ws1",
		AnnotatedAt: annotatedAt,
		Timezone:    "Asia/Tokyo",
		Title:       "Launch",
	})
	rr := httptest.NewRecorder()
	handler.handleCreate(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	var response struct {
		Annotation *domain.Annotation `json:"annotation"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	require.NotNil(t, response.Annotation)
	assert.Equal(t, "annot1", response.Annotation.ID)
}

func TestAnnotationHandler_Create_InvalidJSON(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)
	mockService.EXPECT().CreateAnnotation(gomock.Any(), gomock.Any()).Times(0)

	rr := httptest.NewRecorder()
	handler.handleCreate(rr, httptest.NewRequest(http.MethodPost, "/api/annotations.create", strings.NewReader("{not json")))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "Invalid request body")
}

func TestAnnotationHandler_Create_PermissionError(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)

	mockService.EXPECT().CreateAnnotation(gomock.Any(), gomock.Any()).Return(nil, domain.NewPermissionError(
		domain.PermissionResourceWebAnalytics,
		domain.PermissionTypeWrite,
		"Insufficient permissions: write access to web analytics required",
	))

	req := newAnnotationJSONRequest(t, http.MethodPost, "/api/annotations.create", domain.CreateAnnotationRequest{
		WorkspaceID: "ws1",
		Title:       "Launch",
	})
	rr := httptest.NewRecorder()
	handler.handleCreate(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "web analytics")
}

func TestAnnotationHandler_Create_ValidationError(t *testing.T) {
	testCases := []struct {
		name string
		err  error
	}{
		{"bare validation error", domain.NewValidationError("color must be a hex color like #3b82f6")},
		// A %w-wrapped one must land on the same rung: errors.As against a value
		// target catches it, a type assertion would have produced a 500.
		{"wrapped validation error", fmt.Errorf("wrapped: %w", domain.NewValidationError("color must be a hex color like #3b82f6"))},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, handler := setupAnnotationHandlerTest(t, false)
			mockService.EXPECT().CreateAnnotation(gomock.Any(), gomock.Any()).Return(nil, tc.err)

			req := newAnnotationJSONRequest(t, http.MethodPost, "/api/annotations.create", domain.CreateAnnotationRequest{
				WorkspaceID: "ws1",
				Title:       "Launch",
				Color:       "red",
			})
			rr := httptest.NewRecorder()
			handler.handleCreate(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Contains(t, rr.Body.String(), "hex color")
		})
	}
}

func TestAnnotationHandler_Create_ServiceError(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)

	mockService.EXPECT().CreateAnnotation(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("pq: relation \"annotations\" does not exist"))

	req := newAnnotationJSONRequest(t, http.MethodPost, "/api/annotations.create", domain.CreateAnnotationRequest{
		WorkspaceID: "ws1",
		Title:       "Launch",
	})
	rr := httptest.NewRecorder()
	handler.handleCreate(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "Failed to create annotation")
	assert.NotContains(t, rr.Body.String(), "pq:")
}

func TestAnnotationHandler_Update_Success(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)

	mockService.EXPECT().UpdateAnnotation(gomock.Any(), gomock.Any()).
		Return(&domain.Annotation{ID: "annot1", Title: "Renamed"}, nil)

	req := newAnnotationJSONRequest(t, http.MethodPost, "/api/annotations.update", domain.UpdateAnnotationRequest{
		WorkspaceID: "ws1",
		ID:          "annot1",
		Title:       "Renamed",
		AnnotatedAt: time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC),
	})
	rr := httptest.NewRecorder()
	handler.handleUpdate(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var response struct {
		Annotation *domain.Annotation `json:"annotation"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	require.NotNil(t, response.Annotation)
	assert.Equal(t, "Renamed", response.Annotation.Title)
}

func TestAnnotationHandler_Update_NotFound(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)

	mockService.EXPECT().UpdateAnnotation(gomock.Any(), gomock.Any()).
		Return(nil, &domain.ErrNotFound{Entity: "annotation", ID: "missing"})

	req := newAnnotationJSONRequest(t, http.MethodPost, "/api/annotations.update", domain.UpdateAnnotationRequest{
		WorkspaceID: "ws1",
		ID:          "missing",
		Title:       "Renamed",
	})
	rr := httptest.NewRecorder()
	handler.handleUpdate(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestAnnotationHandler_Update_InvalidJSON(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)
	mockService.EXPECT().UpdateAnnotation(gomock.Any(), gomock.Any()).Times(0)

	rr := httptest.NewRecorder()
	handler.handleUpdate(rr, httptest.NewRequest(http.MethodPost, "/api/annotations.update", strings.NewReader("{not json")))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestAnnotationHandler_Delete_Success(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)

	mockService.EXPECT().
		DeleteAnnotation(gomock.Any(), &domain.DeleteAnnotationRequest{WorkspaceID: "ws1", ID: "annot1"}).
		Return(nil)

	req := newAnnotationJSONRequest(t, http.MethodPost, "/api/annotations.delete", domain.DeleteAnnotationRequest{
		WorkspaceID: "ws1",
		ID:          "annot1",
	})
	rr := httptest.NewRecorder()
	handler.handleDelete(rr, req)

	// A parseable body, never 204 — the console reads one on every response.
	require.Equal(t, http.StatusOK, rr.Code)
	var response map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	assert.Equal(t, true, response["success"])
}

func TestAnnotationHandler_Delete_NotFound(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)

	mockService.EXPECT().DeleteAnnotation(gomock.Any(), gomock.Any()).
		Return(&domain.ErrNotFound{Entity: "annotation", ID: "missing"})

	req := newAnnotationJSONRequest(t, http.MethodPost, "/api/annotations.delete", domain.DeleteAnnotationRequest{
		WorkspaceID: "ws1",
		ID:          "missing",
	})
	rr := httptest.NewRecorder()
	handler.handleDelete(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), "annotation not found")
}

func TestAnnotationHandler_Delete_PermissionError(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)

	mockService.EXPECT().DeleteAnnotation(gomock.Any(), gomock.Any()).Return(domain.NewPermissionError(
		domain.PermissionResourceWebAnalytics,
		domain.PermissionTypeWrite,
		"Insufficient permissions: write access to web analytics required",
	))

	req := newAnnotationJSONRequest(t, http.MethodPost, "/api/annotations.delete", domain.DeleteAnnotationRequest{
		WorkspaceID: "ws1",
		ID:          "annot1",
	})
	rr := httptest.NewRecorder()
	handler.handleDelete(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "web analytics")
}

func TestAnnotationHandler_DemoMode_BlocksMutations(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, true)
	// RestrictedInDemo sits outside RequireAuth, so the request is refused before
	// authentication and the service is never reached.
	mockService.EXPECT().CreateAnnotation(gomock.Any(), gomock.Any()).Times(0)
	mockService.EXPECT().UpdateAnnotation(gomock.Any(), gomock.Any()).Times(0)
	mockService.EXPECT().DeleteAnnotation(gomock.Any(), gomock.Any()).Times(0)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	for _, endpoint := range []string{"/api/annotations.create", "/api/annotations.update", "/api/annotations.delete"} {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, newAnnotationJSONRequest(t, http.MethodPost, endpoint, map[string]interface{}{"workspace_id": "ws1"}))

		assert.Equal(t, http.StatusBadRequest, rr.Code, endpoint)
		assert.Contains(t, rr.Body.String(), "demo mode", endpoint)
	}

	// Reads stay open in demo mode: they fall through to authentication instead.
	for _, endpoint := range []string{"/api/annotations.list", "/api/annotations.get"} {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, endpoint+"?workspace_id=ws1&id=annot1", nil))

		assert.Equal(t, http.StatusUnauthorized, rr.Code, endpoint)
		assert.NotContains(t, rr.Body.String(), "demo mode", endpoint)
	}
}

func TestAnnotationHandler_MutationsRequireAuthOutsideDemo(t *testing.T) {
	mockService, handler := setupAnnotationHandlerTest(t, false)
	mockService.EXPECT().CreateAnnotation(gomock.Any(), gomock.Any()).Times(0)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, newAnnotationJSONRequest(t, http.MethodPost, "/api/annotations.create", map[string]interface{}{"workspace_id": "ws1"}))

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}
