package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opencensus.io/trace"
)

type countHandlerMocks struct {
	handler *MessageHistoryHandler
	service *mocks.MockMessageHistoryService
	auth    *mocks.MockAuthService
}

func newCountHandlerMocks(t *testing.T) *countHandlerMocks {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	m := &countHandlerMocks{service: mocks.NewMockMessageHistoryService(ctrl), auth: mocks.NewMockAuthService(ctrl)}
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().Error(gomock.Any()).AnyTimes()
	tracer := pkgmocks.NewMockTracer(ctrl)
	tracer.EXPECT().StartSpan(gomock.Any(), "MessageHistoryHandler.handleCount").Return(context.Background(), &trace.Span{}).AnyTimes()
	tracer.EXPECT().EndSpan(gomock.Any(), gomock.Any()).AnyTimes()
	tracer.EXPECT().MarkSpanError(gomock.Any(), gomock.Any()).AnyTimes()
	m.handler = NewMessageHistoryHandlerWithTracer(m.service, m.auth,
		func() ([]byte, error) { return []byte("test-jwt-secret-key-for-testing-32bytes"), nil }, logger, tracer)
	return m
}

func (m *countHandlerMocks) authenticated() {
	m.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(context.Background(), &domain.User{ID: "u1"}, nil, nil)
}

func (m *countHandlerMocks) call(method, query string) (int, map[string]interface{}) {
	w := httptest.NewRecorder()
	m.handler.handleCount(w, httptest.NewRequest(method, "/api/messages.count?"+query, nil))
	var body map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&body)
	return w.Code, body
}

func TestMessageHistoryHandler_handleCount(t *testing.T) {
	t.Run("a non-GET request is refused", func(t *testing.T) {
		code, _ := newCountHandlerMocks(t).call(http.MethodPost, "workspace_id=ws1")
		assert.Equal(t, http.StatusMethodNotAllowed, code)
	})

	t.Run("no workspace_id", func(t *testing.T) {
		code, body := newCountHandlerMocks(t).call(http.MethodGet, "created_after=2026-11-01T00:00:00Z")
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Equal(t, "Missing workspace ID", body["error"])
	})

	t.Run("unauthenticated", func(t *testing.T) {
		m := newCountHandlerMocks(t)
		m.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(nil, nil, nil, errors.New("bad token"))
		code, _ := m.call(http.MethodGet, "workspace_id=ws1&created_after=2026-11-01T00:00:00Z")
		assert.Equal(t, http.StatusUnauthorized, code)
	})

	for name, tc := range map[string]struct{ query, message string }{
		"no bound":           {"", "messages.count needs created_after or an indexed filter"},
		"external_id alone":  {"&external_id=e1", "messages.count needs created_after or an indexed filter"},
		"list_id alone":      {"&list_id=l1", "messages.count needs created_after or an indexed filter"},
		"unknown origin":     {"&broadcast_id=b1&origin=api", "origin must be broadcast, automation, transactional or other"},
		"unreadable boolean": {"&broadcast_id=b1&is_opened=yes", "invalid is_opened value: yes"},
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			m := newCountHandlerMocks(t)
			m.authenticated()
			code, body := m.call(http.MethodGet, "workspace_id=ws1"+tc.query)
			assert.Equal(t, http.StatusBadRequest, code)
			assert.Equal(t, tc.message, body["error"])
		})
	}

	t.Run("every count, cursor and limit ignored", func(t *testing.T) {
		m := newCountHandlerMocks(t)
		m.authenticated()
		after := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
		m.service.EXPECT().CountMessages(gomock.Any(), "ws1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, f domain.MessageFilter) (*domain.MessageCounts, error) {
				assert.Equal(t, domain.MessageOriginTransactional, f.Origin)
				require.NotNil(t, f.CreatedAfter)
				assert.True(t, f.CreatedAfter.Equal(after))
				return &domain.MessageCounts{Total: 7, Delivered: 6, Opened: 5, Clicked: 4, Bounced: 3, Failed: 2, Complained: 1}, nil
			})

		code, body := m.call(http.MethodGet, "workspace_id=ws1&created_after=2026-11-01T00:00:00Z&origin=transactional&cursor=bad&limit=bad")
		require.Equal(t, http.StatusOK, code)
		require.Len(t, body, 7)
		for field, want := range map[string]float64{"total": 7, "delivered": 6, "opened": 5, "clicked": 4, "bounced": 3, "failed": 2, "complained": 1} {
			assert.Equal(t, want, body[field], field)
		}
	})

	t.Run("an indexed filter alone is enough", func(t *testing.T) {
		m := newCountHandlerMocks(t)
		m.authenticated()
		m.service.EXPECT().CountMessages(gomock.Any(), "ws1", domain.MessageFilter{BroadcastID: "b1"}).Return(&domain.MessageCounts{}, nil)
		code, _ := m.call(http.MethodGet, "workspace_id=ws1&broadcast_id=b1")
		assert.Equal(t, http.StatusOK, code)
	})

	t.Run("a permission error answers 403", func(t *testing.T) {
		m := newCountHandlerMocks(t)
		m.authenticated()
		m.service.EXPECT().CountMessages(gomock.Any(), "ws1", gomock.Any()).
			Return(nil, domain.NewPermissionError(domain.PermissionResourceMessageHistory, domain.PermissionTypeRead, "Insufficient permissions"))
		code, _ := m.call(http.MethodGet, "workspace_id=ws1&broadcast_id=b1")
		assert.Equal(t, http.StatusForbidden, code)
	})

	t.Run("a service error answers 500", func(t *testing.T) {
		m := newCountHandlerMocks(t)
		m.authenticated()
		m.service.EXPECT().CountMessages(gomock.Any(), "ws1", gomock.Any()).Return(nil, errors.New("db down"))
		code, body := m.call(http.MethodGet, "workspace_id=ws1&broadcast_id=b1")
		assert.Equal(t, http.StatusInternalServerError, code)
		assert.Equal(t, "Failed to count messages", body["error"])
	})
}

func TestMessageHistoryHandler_RegistersMessagesCount(t *testing.T) {
	m := newCountHandlerMocks(t)
	mux := http.NewServeMux()
	m.handler.RegisterRoutes(mux)
	_, pattern := mux.Handler(httptest.NewRequest(http.MethodGet, "/api/messages.count", nil))
	assert.Equal(t, "/api/messages.count", pattern)
}
