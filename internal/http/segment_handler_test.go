package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"

	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"

	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/assert"
)

// Test setup helper
func setupSegmentHandlerTest(t *testing.T) (*mocks.MockSegmentService, *pkgmocks.MockLogger, *SegmentHandler) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockSegmentService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup common logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Fatal(gomock.Any()).AnyTimes()

	// Create key pair for testing
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	handler := NewSegmentHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)
	return mockService, mockLogger, handler
}

func createTestSegment() *domain.Segment {
	return &domain.Segment{
		ID:       "segment1",
		Name:     "Test Segment",
		Color:    "#FF5733",
		Timezone: "UTC",
		Version:  1,
		Status:   string(domain.SegmentStatusActive),
		Tree: &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Table: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "email",
							FieldType:    "string",
							Operator:     "contains",
							StringValues: []string{"@example.com"},
						},
					},
				},
			},
		},
		UsersCount: 10,
	}
}

func TestSegmentHandler_RegisterRoutes(t *testing.T) {
	_, _, handler := setupSegmentHandlerTest(t)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Check if routes were registered
	endpoints := []string{
		"/api/segments.list",
		"/api/segments.get",
		"/api/segments.create",
		"/api/segments.update",
		"/api/segments.delete",
		"/api/segments.rebuild",
		"/api/segments.preview",
		"/api/segments.contacts",
	}

	for _, endpoint := range endpoints {
		h, _ := mux.Handler(&http.Request{URL: &url.URL{Path: endpoint}})
		if h == nil {
			t.Errorf("Expected handler to be registered for %s, but got nil", endpoint)
		}
	}
}

func TestSegmentHandler_HandleList(t *testing.T) {
	testCases := []struct {
		name             string
		method           string
		queryParams      url.Values
		setupMock        func(*mocks.MockSegmentService)
		expectedStatus   int
		expectedSegments bool
		validateResponse func(*testing.T, map[string]interface{})
	}{
		{
			name:   "List Segments Success",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().ListSegments(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ interface{}, req *domain.GetSegmentsRequest) ([]*domain.Segment, error) {
						assert.Equal(t, "workspace123", req.WorkspaceID)
						assert.False(t, req.WithCount) // Default is false
						return []*domain.Segment{
							createTestSegment(),
							{
								ID:         "segment2",
								Name:       "Test Segment 2",
								Color:      "#33FF57",
								Timezone:   "UTC",
								Version:    1,
								Status:     string(domain.SegmentStatusActive),
								UsersCount: 0, // No count when WithCount=false
							},
						}, nil
					},
				)
			},
			expectedStatus:   http.StatusOK,
			expectedSegments: true,
			validateResponse: func(t *testing.T, response map[string]interface{}) {
				segments, ok := response["segments"].([]interface{})
				assert.True(t, ok)
				assert.Len(t, segments, 2)
			},
		},
		{
			name:   "List Segments With Count",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"with_count":   []string{"true"},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().ListSegments(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ interface{}, req *domain.GetSegmentsRequest) ([]*domain.Segment, error) {
						assert.Equal(t, "workspace123", req.WorkspaceID)
						assert.True(t, req.WithCount)
						return []*domain.Segment{
							createTestSegment(),
							{
								ID:         "segment2",
								Name:       "Test Segment 2",
								Color:      "#33FF57",
								Timezone:   "UTC",
								Version:    1,
								Status:     string(domain.SegmentStatusActive),
								UsersCount: 5,
							},
						}, nil
					},
				)
			},
			expectedStatus:   http.StatusOK,
			expectedSegments: true,
			validateResponse: func(t *testing.T, response map[string]interface{}) {
				segments, ok := response["segments"].([]interface{})
				assert.True(t, ok)
				assert.Len(t, segments, 2)
			},
		},
		{
			name:   "List Segments Without Count Explicit",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"with_count":   []string{"false"},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().ListSegments(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ interface{}, req *domain.GetSegmentsRequest) ([]*domain.Segment, error) {
						assert.Equal(t, "workspace123", req.WorkspaceID)
						assert.False(t, req.WithCount)
						return []*domain.Segment{
							createTestSegment(),
						}, nil
					},
				)
			},
			expectedStatus:   http.StatusOK,
			expectedSegments: true,
			validateResponse: func(t *testing.T, response map[string]interface{}) {
				segments, ok := response["segments"].([]interface{})
				assert.True(t, ok)
				assert.Len(t, segments, 1)
			},
		},
		{
			name:   "List Segments Service Error",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().ListSegments(gomock.Any(), gomock.Any()).Return(nil, errors.New("service error"))
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedSegments: false,
		},
		{
			name:   "Method Not Allowed",
			method: http.MethodPost,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
			},
			setupMock:        func(m *mocks.MockSegmentService) {},
			expectedStatus:   http.StatusMethodNotAllowed,
			expectedSegments: false,
		},
		{
			name:             "Missing Workspace ID",
			method:           http.MethodGet,
			queryParams:      url.Values{},
			setupMock:        func(m *mocks.MockSegmentService) {},
			expectedStatus:   http.StatusBadRequest,
			expectedSegments: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupSegmentHandlerTest(t)
			tc.setupMock(mockService)

			req := httptest.NewRequest(tc.method, "/api/segments.list?"+tc.queryParams.Encode(), nil)
			rr := httptest.NewRecorder()

			handler.handleList(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectedStatus == http.StatusOK && tc.validateResponse != nil {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				tc.validateResponse(t, response)
			}
		})
	}
}

func TestSegmentHandler_HandleGet(t *testing.T) {
	testCases := []struct {
		name             string
		method           string
		queryParams      url.Values
		setupMock        func(*mocks.MockSegmentService)
		expectedStatus   int
		validateResponse func(*testing.T, map[string]interface{})
	}{
		{
			name:   "Get Segment Success",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"id":           []string{"segment1"},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().GetSegment(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ interface{}, req *domain.GetSegmentRequest) (*domain.Segment, error) {
						assert.Equal(t, "workspace123", req.WorkspaceID)
						assert.Equal(t, "segment1", req.ID)
						return createTestSegment(), nil
					},
				)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, response map[string]interface{}) {
				segment, ok := response["segment"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, "segment1", segment["id"])
				assert.Equal(t, "Test Segment", segment["name"])
			},
		},
		{
			name:   "Segment Not Found",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"id":           []string{"nonexistent"},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().GetSegment(gomock.Any(), gomock.Any()).Return(
					nil,
					&domain.ErrSegmentNotFound{Message: "segment not found"},
				)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:   "Service Error",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"id":           []string{"segment1"},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().GetSegment(gomock.Any(), gomock.Any()).Return(nil, errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "Missing ID",
			method:         http.MethodGet,
			queryParams:    url.Values{"workspace_id": []string{"workspace123"}},
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Method Not Allowed",
			method:         http.MethodPost,
			queryParams:    url.Values{"workspace_id": []string{"workspace123"}, "id": []string{"segment1"}},
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupSegmentHandlerTest(t)
			tc.setupMock(mockService)

			req := httptest.NewRequest(tc.method, "/api/segments.get?"+tc.queryParams.Encode(), nil)
			rr := httptest.NewRecorder()

			handler.handleGet(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectedStatus == http.StatusOK && tc.validateResponse != nil {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				tc.validateResponse(t, response)
			}
		})
	}
}

func TestSegmentHandler_HandleCreate(t *testing.T) {
	testCases := []struct {
		name             string
		method           string
		requestBody      interface{}
		setupMock        func(*mocks.MockSegmentService)
		expectedStatus   int
		validateResponse func(*testing.T, map[string]interface{})
	}{
		{
			name:   "Create Segment Success",
			method: http.MethodPost,
			requestBody: &domain.CreateSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "newsegment",
				Name:        "New Segment",
				Color:       "#FF5733",
				Timezone:    "UTC",
				Tree: &domain.TreeNode{
					Kind: "leaf",
					Leaf: &domain.TreeNodeLeaf{
						Table: "contacts",
						Contact: &domain.ContactCondition{
							Filters: []*domain.DimensionFilter{
								{
									FieldName:    "email",
									FieldType:    "string",
									Operator:     "contains",
									StringValues: []string{"@test.com"},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().CreateSegment(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ interface{}, req *domain.CreateSegmentRequest) (*domain.Segment, error) {
						return &domain.Segment{
							ID:       req.ID,
							Name:     req.Name,
							Color:    req.Color,
							Timezone: req.Timezone,
							Tree:     req.Tree,
							Version:  1,
							Status:   string(domain.SegmentStatusBuilding),
						}, nil
					},
				)
			},
			expectedStatus: http.StatusCreated,
			validateResponse: func(t *testing.T, response map[string]interface{}) {
				segment, ok := response["segment"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, "newsegment", segment["id"])
				assert.Equal(t, "New Segment", segment["name"])
			},
		},
		{
			name:   "Create Segment Service Error",
			method: http.MethodPost,
			requestBody: &domain.CreateSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "newsegment",
				Name:        "New Segment",
				Color:       "#FF5733",
				Timezone:    "UTC",
				Tree: &domain.TreeNode{
					Kind: "leaf",
					Leaf: &domain.TreeNodeLeaf{
						Table: "contacts",
						Contact: &domain.ContactCondition{
							Filters: []*domain.DimensionFilter{},
						},
					},
				},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().CreateSegment(gomock.Any(), gomock.Any()).Return(nil, errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "Invalid Request Body",
			method:         http.MethodPost,
			requestBody:    "invalid json",
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Method Not Allowed",
			method:         http.MethodGet,
			requestBody:    nil,
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupSegmentHandlerTest(t)
			tc.setupMock(mockService)

			var body bytes.Buffer
			if tc.requestBody != nil {
				if str, ok := tc.requestBody.(string); ok {
					body.WriteString(str)
				} else {
					_ = json.NewEncoder(&body).Encode(tc.requestBody)
				}
			}

			req := httptest.NewRequest(tc.method, "/api/segments.create", &body)
			rr := httptest.NewRecorder()

			handler.handleCreate(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectedStatus == http.StatusCreated && tc.validateResponse != nil {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				tc.validateResponse(t, response)
			}
		})
	}
}

func TestSegmentHandler_HandleUpdate(t *testing.T) {
	testCases := []struct {
		name             string
		method           string
		requestBody      interface{}
		setupMock        func(*mocks.MockSegmentService)
		expectedStatus   int
		validateResponse func(*testing.T, map[string]interface{})
	}{
		{
			name:   "Update Segment Success",
			method: http.MethodPost,
			requestBody: &domain.UpdateSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "segment1",
				Name:        "Updated Segment",
				Color:       "#33FF57",
				Timezone:    "America/New_York",
				Tree: &domain.TreeNode{
					Kind: "leaf",
					Leaf: &domain.TreeNodeLeaf{
						Table: "contacts",
						Contact: &domain.ContactCondition{
							Filters: []*domain.DimensionFilter{},
						},
					},
				},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().UpdateSegment(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ interface{}, req *domain.UpdateSegmentRequest) (*domain.Segment, error) {
						return &domain.Segment{
							ID:       req.ID,
							Name:     req.Name,
							Color:    req.Color,
							Timezone: req.Timezone,
							Tree:     req.Tree,
							Version:  2,
							Status:   string(domain.SegmentStatusActive),
						}, nil
					},
				)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, response map[string]interface{}) {
				segment, ok := response["segment"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, "segment1", segment["id"])
				assert.Equal(t, "Updated Segment", segment["name"])
			},
		},
		{
			name:   "Update Segment Not Found",
			method: http.MethodPost,
			requestBody: &domain.UpdateSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "nonexistent",
				Name:        "Updated Segment",
				Color:       "#33FF57",
				Timezone:    "UTC",
				Tree: &domain.TreeNode{
					Kind: "leaf",
					Leaf: &domain.TreeNodeLeaf{
						Table: "contacts",
						Contact: &domain.ContactCondition{
							Filters: []*domain.DimensionFilter{},
						},
					},
				},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().UpdateSegment(gomock.Any(), gomock.Any()).Return(
					nil,
					&domain.ErrSegmentNotFound{Message: "segment not found"},
				)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:   "Update Segment Service Error",
			method: http.MethodPost,
			requestBody: &domain.UpdateSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "segment1",
				Name:        "Updated Segment",
				Color:       "#33FF57",
				Timezone:    "UTC",
				Tree: &domain.TreeNode{
					Kind: "leaf",
					Leaf: &domain.TreeNodeLeaf{
						Table: "contacts",
						Contact: &domain.ContactCondition{
							Filters: []*domain.DimensionFilter{},
						},
					},
				},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().UpdateSegment(gomock.Any(), gomock.Any()).Return(nil, errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "Invalid Request Body",
			method:         http.MethodPost,
			requestBody:    "invalid json",
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Method Not Allowed",
			method:         http.MethodGet,
			requestBody:    nil,
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupSegmentHandlerTest(t)
			tc.setupMock(mockService)

			var body bytes.Buffer
			if tc.requestBody != nil {
				if str, ok := tc.requestBody.(string); ok {
					body.WriteString(str)
				} else {
					_ = json.NewEncoder(&body).Encode(tc.requestBody)
				}
			}

			req := httptest.NewRequest(tc.method, "/api/segments.update", &body)
			rr := httptest.NewRecorder()

			handler.handleUpdate(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectedStatus == http.StatusOK && tc.validateResponse != nil {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				tc.validateResponse(t, response)
			}
		})
	}
}

func TestSegmentHandler_HandleDelete(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		requestBody    interface{}
		setupMock      func(*mocks.MockSegmentService)
		expectedStatus int
	}{
		{
			name:   "Delete Segment Success",
			method: http.MethodPost,
			requestBody: &domain.DeleteSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "segment1",
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().DeleteSegment(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ interface{}, req *domain.DeleteSegmentRequest) error {
						assert.Equal(t, "workspace123", req.WorkspaceID)
						assert.Equal(t, "segment1", req.ID)
						return nil
					},
				)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Delete Segment Not Found",
			method: http.MethodPost,
			requestBody: &domain.DeleteSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "nonexistent",
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().DeleteSegment(gomock.Any(), gomock.Any()).Return(
					&domain.ErrSegmentNotFound{Message: "segment not found"},
				)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:   "Delete Segment Service Error",
			method: http.MethodPost,
			requestBody: &domain.DeleteSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "segment1",
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().DeleteSegment(gomock.Any(), gomock.Any()).Return(errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "Invalid Request Body",
			method:         http.MethodPost,
			requestBody:    "invalid json",
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Method Not Allowed",
			method:         http.MethodGet,
			requestBody:    nil,
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupSegmentHandlerTest(t)
			tc.setupMock(mockService)

			var body bytes.Buffer
			if tc.requestBody != nil {
				if str, ok := tc.requestBody.(string); ok {
					body.WriteString(str)
				} else {
					_ = json.NewEncoder(&body).Encode(tc.requestBody)
				}
			}

			req := httptest.NewRequest(tc.method, "/api/segments.delete", &body)
			rr := httptest.NewRecorder()

			handler.handleDelete(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.True(t, response["success"].(bool))
			}
		})
	}
}

func TestSegmentHandler_HandleRebuild(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		requestBody    map[string]string
		setupMock      func(*mocks.MockSegmentService)
		expectedStatus int
	}{
		{
			name:   "Rebuild Segment Success",
			method: http.MethodPost,
			requestBody: map[string]string{
				"workspace_id": "workspace123",
				"segment_id":   "segment1",
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().RebuildSegment(gomock.Any(), "workspace123", "segment1").Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Rebuild Segment Not Found",
			method: http.MethodPost,
			requestBody: map[string]string{
				"workspace_id": "workspace123",
				"segment_id":   "nonexistent",
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().RebuildSegment(gomock.Any(), "workspace123", "nonexistent").Return(
					&domain.ErrSegmentNotFound{Message: "segment not found"},
				)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:   "Rebuild Segment Service Error",
			method: http.MethodPost,
			requestBody: map[string]string{
				"workspace_id": "workspace123",
				"segment_id":   "segment1",
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().RebuildSegment(gomock.Any(), "workspace123", "segment1").Return(errors.New("service error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:   "Missing Workspace ID",
			method: http.MethodPost,
			requestBody: map[string]string{
				"segment_id": "segment1",
			},
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Missing Segment ID",
			method: http.MethodPost,
			requestBody: map[string]string{
				"workspace_id": "workspace123",
			},
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Method Not Allowed",
			method:         http.MethodGet,
			requestBody:    map[string]string{},
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupSegmentHandlerTest(t)
			tc.setupMock(mockService)

			var body bytes.Buffer
			_ = json.NewEncoder(&body).Encode(tc.requestBody)

			req := httptest.NewRequest(tc.method, "/api/segments.rebuild", &body)
			rr := httptest.NewRecorder()

			handler.handleRebuild(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.True(t, response["success"].(bool))
				assert.Equal(t, "Segment rebuild has been queued", response["message"])
			}
		})
	}
}

func TestSegmentHandler_HandlePreview(t *testing.T) {
	testCases := []struct {
		name             string
		method           string
		requestBody      map[string]interface{}
		setupMock        func(*mocks.MockSegmentService)
		expectedStatus   int
		validateResponse func(*testing.T, map[string]interface{})
	}{
		{
			name:   "Preview Segment Success",
			method: http.MethodPost,
			requestBody: map[string]interface{}{
				"workspace_id": "workspace123",
				"tree": map[string]interface{}{
					"kind": "leaf",
					"leaf": map[string]interface{}{
						"table": "contacts",
						"contact": map[string]interface{}{
							"filters": []interface{}{},
						},
					},
				},
				"limit": 5,
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().PreviewSegment(gomock.Any(), "workspace123", gomock.Any(), 5).Return(
					&domain.PreviewSegmentResponse{
						Emails:       []string{"user1@example.com", "user2@example.com", "user3@example.com"},
						TotalCount:   100,
						Limit:        5,
						GeneratedSQL: "SELECT email FROM contacts WHERE country = $1",
						SQLArgs:      []interface{}{"US"},
					},
					nil,
				)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, response map[string]interface{}) {
				emails, ok := response["emails"].([]interface{})
				assert.True(t, ok)
				assert.Len(t, emails, 3)
				assert.Equal(t, float64(100), response["total_count"])
				assert.Equal(t, float64(5), response["limit"])
			},
		},
		{
			name:   "Preview Segment Default Limit",
			method: http.MethodPost,
			requestBody: map[string]interface{}{
				"workspace_id": "workspace123",
				"tree": map[string]interface{}{
					"kind": "leaf",
					"leaf": map[string]interface{}{
						"table": "contacts",
						"contact": map[string]interface{}{
							"filters": []interface{}{},
						},
					},
				},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().PreviewSegment(gomock.Any(), "workspace123", gomock.Any(), 10).Return(
					&domain.PreviewSegmentResponse{
						Emails:       []string{"user1@example.com"},
						TotalCount:   1,
						Limit:        10,
						GeneratedSQL: "SELECT email FROM contacts",
						SQLArgs:      []interface{}{},
					},
					nil,
				)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, float64(10), response["limit"]) // Default limit
			},
		},
		{
			name:   "Preview Segment Service Error",
			method: http.MethodPost,
			requestBody: map[string]interface{}{
				"workspace_id": "workspace123",
				"tree": map[string]interface{}{
					"kind": "leaf",
					"leaf": map[string]interface{}{
						"table": "contacts",
						"contact": map[string]interface{}{
							"filters": []interface{}{},
						},
					},
				},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().PreviewSegment(gomock.Any(), "workspace123", gomock.Any(), 10).Return(
					nil, errors.New("service error"),
				)
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:   "Missing Workspace ID",
			method: http.MethodPost,
			requestBody: map[string]interface{}{
				"tree": map[string]interface{}{
					"kind": "leaf",
				},
			},
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Missing Tree",
			method: http.MethodPost,
			requestBody: map[string]interface{}{
				"workspace_id": "workspace123",
			},
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Method Not Allowed",
			method:         http.MethodGet,
			requestBody:    map[string]interface{}{},
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupSegmentHandlerTest(t)
			tc.setupMock(mockService)

			var body bytes.Buffer
			_ = json.NewEncoder(&body).Encode(tc.requestBody)

			req := httptest.NewRequest(tc.method, "/api/segments.preview", &body)
			rr := httptest.NewRecorder()

			handler.handlePreview(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectedStatus == http.StatusOK && tc.validateResponse != nil {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				tc.validateResponse(t, response)
			}
		})
	}
}

func TestSegmentHandler_HandleGetContacts(t *testing.T) {
	testCases := []struct {
		name             string
		method           string
		queryParams      url.Values
		setupMock        func(*mocks.MockSegmentService)
		expectedStatus   int
		validateResponse func(*testing.T, map[string]interface{})
	}{
		{
			name:   "Get Contacts Success",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"segment_id":   []string{"segment1"},
				"limit":        []string{"20"},
				"offset":       []string{"10"},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().GetSegmentContacts(gomock.Any(), "workspace123", "segment1", 20, 10).Return(
					[]string{"user1@example.com", "user2@example.com"},
					nil,
				)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, response map[string]interface{}) {
				emails, ok := response["emails"].([]interface{})
				assert.True(t, ok)
				assert.Len(t, emails, 2)
				assert.Equal(t, float64(20), response["limit"])
				assert.Equal(t, float64(10), response["offset"])
			},
		},
		{
			name:   "Get Contacts Default Parameters",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"segment_id":   []string{"segment1"},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().GetSegmentContacts(gomock.Any(), "workspace123", "segment1", 50, 0).Return(
					[]string{"user1@example.com"},
					nil,
				)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, float64(50), response["limit"]) // Default limit
				assert.Equal(t, float64(0), response["offset"]) // Default offset
			},
		},
		{
			name:   "Get Contacts Not Found",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"segment_id":   []string{"nonexistent"},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().GetSegmentContacts(gomock.Any(), "workspace123", "nonexistent", 50, 0).Return(
					nil,
					&domain.ErrSegmentNotFound{Message: "segment not found"},
				)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:   "Get Contacts Service Error",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"segment_id":   []string{"segment1"},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().GetSegmentContacts(gomock.Any(), "workspace123", "segment1", 50, 0).Return(
					nil, errors.New("service error"),
				)
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "Missing Workspace ID",
			method:         http.MethodGet,
			queryParams:    url.Values{"segment_id": []string{"segment1"}},
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Missing Segment ID",
			method:         http.MethodGet,
			queryParams:    url.Values{"workspace_id": []string{"workspace123"}},
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Method Not Allowed",
			method: http.MethodPost,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"segment_id":   []string{"segment1"},
			},
			setupMock:      func(m *mocks.MockSegmentService) {},
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupSegmentHandlerTest(t)
			tc.setupMock(mockService)

			req := httptest.NewRequest(tc.method, "/api/segments.contacts?"+tc.queryParams.Encode(), nil)
			rr := httptest.NewRecorder()

			handler.handleGetContacts(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectedStatus == http.StatusOK && tc.validateResponse != nil {
				var response map[string]interface{}
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				tc.validateResponse(t, response)
			}
		})
	}
}
