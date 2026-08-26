package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"

	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"

	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				Source: "contacts",
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
			// The shape the service actually returns. SegmentService.GetSegment
			// wraps the repository's typed error in "failed to get segment: %w",
			// and a bare type assertion does not see through a wrap — so this
			// handler answered 500 for every missing segment while the case
			// above went on passing.
			name:   "Segment Not Found, wrapped the way the service wraps it",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"id":           []string{"nonexistent"},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().GetSegment(gomock.Any(), gomock.Any()).Return(
					nil,
					fmt.Errorf("failed to get segment: %w", &domain.ErrSegmentNotFound{Message: "segment not found"}),
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
						Source: "contacts",
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
						Source: "contacts",
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
						Source: "contacts",
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
						Source: "contacts",
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
			// The shape the service actually returns: UpdateSegment loads the row
			// first and wraps the repository's typed error in "failed to get
			// segment: %w". A bare type assertion sees nothing through that wrap,
			// so editing a segment someone else had deleted answered 500 while the
			// unwrapped case above went on passing.
			name:   "Update Segment Not Found, wrapped the way the service wraps it",
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
						Source: "contacts",
						Contact: &domain.ContactCondition{
							Filters: []*domain.DimensionFilter{},
						},
					},
				},
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().UpdateSegment(gomock.Any(), gomock.Any()).Return(
					nil,
					fmt.Errorf("failed to get segment: %w", &domain.ErrSegmentNotFound{Message: "segment not found"}),
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
						Source: "contacts",
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
			// "failed to delete segment: %w" is what the service returns, and a
			// type assertion does not see through it — so deleting a segment twice,
			// which the console lets two open tabs do, answered 500.
			name:   "Delete Segment Not Found, wrapped the way the service wraps it",
			method: http.MethodPost,
			requestBody: &domain.DeleteSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "nonexistent",
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().DeleteSegment(gomock.Any(), gomock.Any()).Return(
					fmt.Errorf("failed to delete segment: %w", &domain.ErrSegmentNotFound{Message: "segment not found"}),
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
			// RebuildSegment loads the row first and wraps in "failed to get
			// segment: %w", the same shape handleGet was fixed for. Nothing about
			// the wrap is specific to one endpoint, which is why the mapping now
			// lives in writeServiceError instead of in each handler.
			name:   "Rebuild Segment Not Found, wrapped the way the service wraps it",
			method: http.MethodPost,
			requestBody: map[string]string{
				"workspace_id": "workspace123",
				"segment_id":   "nonexistent",
			},
			setupMock: func(m *mocks.MockSegmentService) {
				m.EXPECT().RebuildSegment(gomock.Any(), "workspace123", "nonexistent").Return(
					fmt.Errorf("failed to get segment: %w", &domain.ErrSegmentNotFound{Message: "segment not found"}),
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

// TestSegmentHandler_PermissionDenied pins the 403 on every segment endpoint. The
// service wraps its denial the way the authenticate step above the gate does, so a
// bare type assertion would degrade it into an opaque 500.
func TestSegmentHandler_PermissionDenied(t *testing.T) {
	denial := func(resource domain.PermissionResource, permission domain.PermissionType) error {
		return fmt.Errorf("failed to authenticate user: %w",
			domain.NewPermissionError(resource, permission, "Insufficient permissions"))
	}

	testCases := []struct {
		name       string
		resource   domain.PermissionResource
		permission domain.PermissionType
		request    func() *http.Request
		expect     func(*mocks.MockSegmentService, error)
		handle     func(*SegmentHandler, http.ResponseWriter, *http.Request)
	}{
		{
			name:       "list",
			resource:   domain.PermissionResourceSegments,
			permission: domain.PermissionTypeRead,
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/segments.list?workspace_id=workspace1", nil)
			},
			expect: func(s *mocks.MockSegmentService, err error) {
				s.EXPECT().ListSegments(gomock.Any(), gomock.Any()).Return(nil, err)
			},
			handle: func(h *SegmentHandler, w http.ResponseWriter, r *http.Request) { h.handleList(w, r) },
		},
		{
			name:       "get",
			resource:   domain.PermissionResourceSegments,
			permission: domain.PermissionTypeRead,
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/segments.get?workspace_id=workspace1&id=segment1", nil)
			},
			expect: func(s *mocks.MockSegmentService, err error) {
				s.EXPECT().GetSegment(gomock.Any(), gomock.Any()).Return(nil, err)
			},
			handle: func(h *SegmentHandler, w http.ResponseWriter, r *http.Request) { h.handleGet(w, r) },
		},
		{
			name:       "create",
			resource:   domain.PermissionResourceSegments,
			permission: domain.PermissionTypeWrite,
			request: func() *http.Request {
				body, _ := json.Marshal(map[string]interface{}{"workspace_id": "workspace1", "id": "segment1"})
				return httptest.NewRequest(http.MethodPost, "/api/segments.create", bytes.NewReader(body))
			},
			expect: func(s *mocks.MockSegmentService, err error) {
				s.EXPECT().CreateSegment(gomock.Any(), gomock.Any()).Return(nil, err)
			},
			handle: func(h *SegmentHandler, w http.ResponseWriter, r *http.Request) { h.handleCreate(w, r) },
		},
		{
			name:       "update",
			resource:   domain.PermissionResourceSegments,
			permission: domain.PermissionTypeWrite,
			request: func() *http.Request {
				body, _ := json.Marshal(map[string]interface{}{"workspace_id": "workspace1", "id": "segment1"})
				return httptest.NewRequest(http.MethodPost, "/api/segments.update", bytes.NewReader(body))
			},
			expect: func(s *mocks.MockSegmentService, err error) {
				s.EXPECT().UpdateSegment(gomock.Any(), gomock.Any()).Return(nil, err)
			},
			handle: func(h *SegmentHandler, w http.ResponseWriter, r *http.Request) { h.handleUpdate(w, r) },
		},
		{
			name:       "delete",
			resource:   domain.PermissionResourceSegments,
			permission: domain.PermissionTypeWrite,
			request: func() *http.Request {
				body, _ := json.Marshal(map[string]interface{}{"workspace_id": "workspace1", "id": "segment1"})
				return httptest.NewRequest(http.MethodPost, "/api/segments.delete", bytes.NewReader(body))
			},
			expect: func(s *mocks.MockSegmentService, err error) {
				s.EXPECT().DeleteSegment(gomock.Any(), gomock.Any()).Return(err)
			},
			handle: func(h *SegmentHandler, w http.ResponseWriter, r *http.Request) { h.handleDelete(w, r) },
		},
		{
			name:       "rebuild",
			resource:   domain.PermissionResourceSegments,
			permission: domain.PermissionTypeWrite,
			request: func() *http.Request {
				body, _ := json.Marshal(map[string]interface{}{"workspace_id": "workspace1", "segment_id": "segment1"})
				return httptest.NewRequest(http.MethodPost, "/api/segments.rebuild", bytes.NewReader(body))
			},
			expect: func(s *mocks.MockSegmentService, err error) {
				s.EXPECT().RebuildSegment(gomock.Any(), "workspace1", "segment1").Return(err)
			},
			handle: func(h *SegmentHandler, w http.ResponseWriter, r *http.Request) { h.handleRebuild(w, r) },
		},
		{
			// preview and contacts are denied on contacts:read, not on segments.
			name:       "preview",
			resource:   domain.PermissionResourceContacts,
			permission: domain.PermissionTypeRead,
			request: func() *http.Request {
				body, _ := json.Marshal(map[string]interface{}{
					"workspace_id": "workspace1",
					"tree":         createTestSegment().Tree,
				})
				return httptest.NewRequest(http.MethodPost, "/api/segments.preview", bytes.NewReader(body))
			},
			expect: func(s *mocks.MockSegmentService, err error) {
				s.EXPECT().PreviewSegment(gomock.Any(), "workspace1", gomock.Any(), gomock.Any()).Return(nil, err)
			},
			handle: func(h *SegmentHandler, w http.ResponseWriter, r *http.Request) { h.handlePreview(w, r) },
		},
		{
			name:       "contacts",
			resource:   domain.PermissionResourceContacts,
			permission: domain.PermissionTypeRead,
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/segments.contacts?workspace_id=workspace1&segment_id=segment1", nil)
			},
			expect: func(s *mocks.MockSegmentService, err error) {
				s.EXPECT().GetSegmentContacts(gomock.Any(), "workspace1", "segment1", gomock.Any(), gomock.Any()).Return(nil, err)
			},
			handle: func(h *SegmentHandler, w http.ResponseWriter, r *http.Request) { h.handleGetContacts(w, r) },
		},
		{
			// The expanded shape reads more contact data, so it must not be a way
			// around the denial the email-only shape answers with.
			name:       "contacts expanded",
			resource:   domain.PermissionResourceContacts,
			permission: domain.PermissionTypeRead,
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/segments.contacts?workspace_id=workspace1&segment_id=segment1&expand=contact", nil)
			},
			expect: func(s *mocks.MockSegmentService, err error) {
				s.EXPECT().GetSegmentContactDetails(gomock.Any(), "workspace1", "segment1", gomock.Any(), gomock.Any()).Return(nil, err)
			},
			handle: func(h *SegmentHandler, w http.ResponseWriter, r *http.Request) { h.handleGetContacts(w, r) },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService, _, handler := setupSegmentHandlerTest(t)
			tc.expect(mockService, denial(tc.resource, tc.permission))

			w := httptest.NewRecorder()
			tc.handle(handler, w, tc.request())

			assert.Equal(t, http.StatusForbidden, w.Code)

			var body map[string]interface{}
			assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, "Insufficient permissions", body["error"])
			assert.Equal(t, string(tc.resource), body["resource"])
			assert.Equal(t, string(tc.permission), body["permission"])
		})
	}
}

// TestSegmentHandler_GetContactsLegacyShapeUnchanged pins the response the console
// reads today, byte for byte. The expanded shape is opt-in precisely so this body
// cannot move: a caller that sends no expand parameter must not start receiving
// contact objects, extra keys, or a different key order.
func TestSegmentHandler_GetContactsLegacyShapeUnchanged(t *testing.T) {
	mockService, _, handler := setupSegmentHandlerTest(t)

	mockService.EXPECT().
		GetSegmentContacts(gomock.Any(), "workspace123", "segment1", 50, 0).
		Return([]string{"user1@example.com", "user2@example.com"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/segments.contacts?workspace_id=workspace123&segment_id=segment1", nil)
	rr := httptest.NewRecorder()

	handler.handleGetContacts(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.Equal(t, `{"emails":["user1@example.com","user2@example.com"],"limit":50,"offset":0}`+"\n", rr.Body.String())
}

func TestSegmentHandler_GetContactsExpanded(t *testing.T) {
	newest := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	older := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

	details := []*domain.SegmentContactDetail{
		{Contact: &domain.Contact{Email: "newest@example.com"}, MatchedAt: newest},
		{Contact: &domain.Contact{Email: "older@example.com"}, MatchedAt: older},
	}

	t.Run("returns contact objects with their join time, newest first", func(t *testing.T) {
		mockService, _, handler := setupSegmentHandlerTest(t)

		mockService.EXPECT().
			GetSegmentContactDetails(gomock.Any(), "workspace123", "segment1", 50, 0).
			Return(details, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/segments.contacts?workspace_id=workspace123&segment_id=segment1&expand=contact", nil)
		rr := httptest.NewRecorder()

		handler.handleGetContacts(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)

		var response struct {
			Contacts []struct {
				Contact   map[string]interface{} `json:"contact"`
				MatchedAt time.Time              `json:"matched_at"`
			} `json:"contacts"`
			Limit  int `json:"limit"`
			Offset int `json:"offset"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))

		require.Len(t, response.Contacts, 2)
		assert.Equal(t, "newest@example.com", response.Contacts[0].Contact["email"])
		assert.Equal(t, newest, response.Contacts[0].MatchedAt)
		assert.Equal(t, "older@example.com", response.Contacts[1].Contact["email"])
		assert.Equal(t, older, response.Contacts[1].MatchedAt)
		assert.Equal(t, 50, response.Limit)
		assert.Equal(t, 0, response.Offset)

		// The email-only key is gone: a client asking for the expanded shape gets
		// one representation of the membership, not two to reconcile.
		var raw map[string]interface{}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &raw))
		_, hasEmails := raw["emails"]
		assert.False(t, hasEmails)
	})

	t.Run("honours limit and offset", func(t *testing.T) {
		mockService, _, handler := setupSegmentHandlerTest(t)

		mockService.EXPECT().
			GetSegmentContactDetails(gomock.Any(), "workspace123", "segment1", 5, 25).
			Return(details[:1], nil)

		req := httptest.NewRequest(http.MethodGet, "/api/segments.contacts?workspace_id=workspace123&segment_id=segment1&expand=contact&limit=5&offset=25", nil)
		rr := httptest.NewRecorder()

		handler.handleGetContacts(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)

		var response map[string]interface{}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
		assert.Equal(t, float64(5), response["limit"])
		assert.Equal(t, float64(25), response["offset"])
	})

	t.Run("segment not found answers 404 JSON", func(t *testing.T) {
		mockService, _, handler := setupSegmentHandlerTest(t)

		mockService.EXPECT().
			GetSegmentContactDetails(gomock.Any(), "workspace123", "nonexistent", 50, 0).
			Return(nil, fmt.Errorf("wrapped: %w", &domain.ErrSegmentNotFound{Message: "segment not found"}))

		req := httptest.NewRequest(http.MethodGet, "/api/segments.contacts?workspace_id=workspace123&segment_id=nonexistent&expand=contact", nil)
		rr := httptest.NewRecorder()

		handler.handleGetContacts(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
		assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
		assert.Equal(t, "Segment not found", body["error"])
	})

	t.Run("service failure answers 500 JSON", func(t *testing.T) {
		mockService, _, handler := setupSegmentHandlerTest(t)

		mockService.EXPECT().
			GetSegmentContactDetails(gomock.Any(), "workspace123", "segment1", 50, 0).
			Return(nil, errors.New("service error"))

		req := httptest.NewRequest(http.MethodGet, "/api/segments.contacts?workspace_id=workspace123&segment_id=segment1&expand=contact", nil)
		rr := httptest.NewRecorder()

		handler.handleGetContacts(rr, req)

		require.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
		assert.NotEmpty(t, body["error"])
	})

	t.Run("unknown expand value is rejected rather than silently ignored", func(t *testing.T) {
		mockService, _, handler := setupSegmentHandlerTest(t)

		// No service call at all: a typo must not quietly fall back to the
		// email-only shape the caller did not ask for.
		mockService.EXPECT().GetSegmentContacts(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockService.EXPECT().GetSegmentContactDetails(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		req := httptest.NewRequest(http.MethodGet, "/api/segments.contacts?workspace_id=workspace123&segment_id=segment1&expand=contacts", nil)
		rr := httptest.NewRecorder()

		handler.handleGetContacts(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
		assert.Contains(t, body["error"], "expand")
	})
}
