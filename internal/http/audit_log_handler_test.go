package http

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

func setupAuditLogHandlerTest(t *testing.T) (*mocks.MockAuditLogService, *AuditLogHandler) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockService := mocks.NewMockAuditLogService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	handler := NewAuditLogHandler(mockService, func() ([]byte, error) { return []byte("test-jwt-secret-key-for-testing-32bytes"), nil }, mockLogger)
	return mockService, handler
}

func TestAuditLogHandler_RegisterRoutes(t *testing.T) {
	_, handler := setupAuditLogHandlerTest(t)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	for _, route := range []string{"/api/auditLogs.list", "/api/auditLogs.get", "/api/auditLogs.actions", "/api/auditLogs.export"} {
		_, pattern := mux.Handler(httptest.NewRequest(http.MethodGet, route, nil))
		assert.Equal(t, route, pattern)
	}
}

func TestAuditLogHandler_List(t *testing.T) {
	t.Run("method", func(t *testing.T) {
		_, handler := setupAuditLogHandlerTest(t)
		rec := httptest.NewRecorder()
		handler.handleList(rec, httptest.NewRequest(http.MethodPost, "/api/auditLogs.list", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("a malformed filter is a 400", func(t *testing.T) {
		_, handler := setupAuditLogHandlerTest(t)
		rec := httptest.NewRecorder()
		handler.handleList(rec, httptest.NewRequest(http.MethodGet, "/api/auditLogs.list?workspace_id=ws&from=yesterday", nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("returns the page", func(t *testing.T) {
		mockService, handler := setupAuditLogHandlerTest(t)
		mockService.EXPECT().List(gomock.Any(), gomock.Any()).DoAndReturn(func(_ interface{}, filter domain.AuditLogFilter) (*domain.ListAuditLogsResponse, error) {
			assert.Equal(t, "ws1", filter.WorkspaceID)
			assert.Equal(t, []string{"templates.update"}, filter.Actions)
			assert.Equal(t, 10, filter.Limit)
			return &domain.ListAuditLogsResponse{Logs: []*domain.AuditLog{{ID: "a", Action: "templates.update"}}, NextCursor: "n", HasMore: true}, nil
		})
		rec := httptest.NewRecorder()
		handler.handleList(rec, httptest.NewRequest(http.MethodGet, "/api/auditLogs.list?workspace_id=ws1&actions=templates.update&limit=10", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		var body domain.ListAuditLogsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Len(t, body.Logs, 1)
		assert.Equal(t, "n", body.NextCursor)
		assert.True(t, body.HasMore)
	})

	t.Run("a permission error is a 403", func(t *testing.T) {
		mockService, handler := setupAuditLogHandlerTest(t)
		mockService.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, domain.NewPermissionError(domain.PermissionResourceAuditLogs, domain.PermissionTypeRead, "no"))
		rec := httptest.NewRecorder()
		handler.handleList(rec, httptest.NewRequest(http.MethodGet, "/api/auditLogs.list?workspace_id=ws1", nil))
		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Contains(t, rec.Body.String(), "audit_logs")
	})

	t.Run("root-only scope refused is a 403", func(t *testing.T) {
		mockService, handler := setupAuditLogHandlerTest(t)
		mockService.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, &domain.ErrUnauthorized{Message: "root user access required"})
		rec := httptest.NewRecorder()
		handler.handleList(rec, httptest.NewRequest(http.MethodGet, "/api/auditLogs.list?scope=deployment", nil))
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("an unknown error is a 500", func(t *testing.T) {
		mockService, handler := setupAuditLogHandlerTest(t)
		mockService.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errors.New("db down"))
		rec := httptest.NewRecorder()
		handler.handleList(rec, httptest.NewRequest(http.MethodGet, "/api/auditLogs.list?workspace_id=ws1", nil))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestAuditLogHandler_Get(t *testing.T) {
	mockService, handler := setupAuditLogHandlerTest(t)

	rec := httptest.NewRecorder()
	handler.handleGet(rec, httptest.NewRequest(http.MethodGet, "/api/auditLogs.get?workspace_id=ws1", nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code, "id is required")

	mockService.EXPECT().Get(gomock.Any(), "", "ws1", "a").Return(&domain.AuditLog{ID: "a"}, nil)
	rec = httptest.NewRecorder()
	handler.handleGet(rec, httptest.NewRequest(http.MethodGet, "/api/auditLogs.get?workspace_id=ws1&id=a", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"log":{`)

	mockService.EXPECT().Get(gomock.Any(), "", "ws1", "missing").Return(nil, domain.ErrAuditLogNotFound)
	rec = httptest.NewRecorder()
	handler.handleGet(rec, httptest.NewRequest(http.MethodGet, "/api/auditLogs.get?workspace_id=ws1&id=missing", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAuditLogHandler_Actions(t *testing.T) {
	mockService, handler := setupAuditLogHandlerTest(t)
	mockService.EXPECT().Actions().Return(domain.AuditActionCatalogue())
	rec := httptest.NewRecorder()
	handler.handleActions(rec, httptest.NewRequest(http.MethodGet, "/api/auditLogs.actions", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Actions    []domain.AuditActionDescriptor `json:"actions"`
		Categories []string                       `json:"categories"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.NotEmpty(t, body.Actions)
	assert.Equal(t, domain.AuditCategories, body.Categories)
}

type flushingRecorder struct {
	*httptest.ResponseRecorder
	flushes int
}

func (f *flushingRecorder) Flush() { f.flushes++ }

func TestAuditLogHandler_Export(t *testing.T) {
	exportRequest := func(body string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/auditLogs.export", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		return req
	}

	t.Run("method and body", func(t *testing.T) {
		_, handler := setupAuditLogHandlerTest(t)
		rec := httptest.NewRecorder()
		handler.handleExport(rec, httptest.NewRequest(http.MethodGet, "/api/auditLogs.export", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)

		rec = httptest.NewRecorder()
		handler.handleExport(rec, exportRequest(`{`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		rec = httptest.NewRecorder()
		handler.handleExport(rec, exportRequest(`{"workspace_id":"ws1","format":"xlsx"}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("streams csv with download headers and flushes", func(t *testing.T) {
		mockService, handler := setupAuditLogHandlerTest(t)
		mockService.EXPECT().Export(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ interface{}, req domain.ExportAuditLogsRequest, w io.Writer, flush func()) (int64, bool, error) {
				assert.Equal(t, domain.AuditExportFormatCSV, req.Format)
				writer := csv.NewWriter(w)
				_ = writer.Write([]string{"id", "action"})
				_ = writer.Write([]string{"a", "templates.update"})
				writer.Flush()
				flush()
				return 1, false, nil
			})
		rec := &flushingRecorder{ResponseRecorder: httptest.NewRecorder()}
		handler.handleExport(rec, exportRequest(`{"workspace_id":"ws1"}`))

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "text/csv; charset=utf-8", rec.Header().Get("Content-Type"))
		assert.Contains(t, rec.Header().Get("Content-Disposition"), `attachment; filename="audit-logs-ws1-`)
		assert.Contains(t, rec.Header().Get("Content-Disposition"), `.csv"`)
		assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
		assert.GreaterOrEqual(t, rec.flushes, 1)
		assert.Contains(t, rec.Body.String(), "templates.update")
	})

	t.Run("ndjson content type and deployment filename", func(t *testing.T) {
		mockService, handler := setupAuditLogHandlerTest(t)
		mockService.EXPECT().Export(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ interface{}, req domain.ExportAuditLogsRequest, w io.Writer, flush func()) (int64, bool, error) {
				_, _ = w.Write([]byte(`{"id":"a"}` + "\n"))
				return 1, false, nil
			})
		rec := httptest.NewRecorder()
		handler.handleExport(rec, exportRequest(`{"scope":"deployment","format":"ndjson"}`))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/x-ndjson", rec.Header().Get("Content-Type"))
		assert.Contains(t, rec.Header().Get("Content-Disposition"), "audit-logs-deployment-")
	})

	t.Run("a refusal before the first byte is a JSON 403", func(t *testing.T) {
		mockService, handler := setupAuditLogHandlerTest(t)
		mockService.EXPECT().Export(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), false,
			domain.NewPermissionError(domain.PermissionResourceAuditLogs, domain.PermissionTypeRead, "no"))
		rec := httptest.NewRecorder()
		handler.handleExport(rec, exportRequest(`{"workspace_id":"ws1"}`))
		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
		assert.Empty(t, rec.Header().Get("Content-Disposition"))
	})

	t.Run("an error after the first byte is logged, not a 500, and the audit row says failed", func(t *testing.T) {
		mockService, handler := setupAuditLogHandlerTest(t)
		mockService.EXPECT().Export(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ interface{}, req domain.ExportAuditLogsRequest, w io.Writer, flush func()) (int64, bool, error) {
				_, _ = w.Write([]byte("id,action\n"))
				return 500, false, errors.New("connection reset")
			})
		record := domain.NewAuditRecord("", "", "")
		req := exportRequest(`{"workspace_id":"ws1"}`)
		req = req.WithContext(domain.WithAuditRecord(req.Context(), record))
		rec := httptest.NewRecorder()
		handler.handleExport(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "id,action\n", rec.Body.String())

		event, _ := record.Finalize(http.StatusOK)
		assert.Equal(t, domain.AuditOutcomeFailure, event.Outcome, "the client got a short file")
		assert.Equal(t, "stream_interrupted", event.Metadata["reason"])
		assert.Equal(t, int64(500), event.Metadata["rows"])
	})
}
