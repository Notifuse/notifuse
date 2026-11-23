package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
)

type fakeRetentionService struct {
	err      error
	policy   *domain.DataRetentionPolicy
	cancelID string
}

func (f *fakeRetentionService) Get(_ context.Context, _ string) (*service.DataRetentionView, error) {
	return &service.DataRetentionView{InForce: map[domain.RetentionCategory]int{domain.RetentionWebhookDeliveries: 7}}, f.err
}
func (f *fakeRetentionService) Preview(_ context.Context, _ string, p *domain.DataRetentionPolicy) (*service.DataRetentionPreview, error) {
	f.policy = p
	return &service.DataRetentionPreview{Pending: true}, f.err
}
func (f *fakeRetentionService) SetPolicy(_ context.Context, _ string, p *domain.DataRetentionPolicy) (*domain.DataRetentionSettings, error) {
	f.policy = p
	return &domain.DataRetentionSettings{Policy: p}, f.err
}
func (f *fakeRetentionService) CancelPending(_ context.Context, id string) (*domain.DataRetentionSettings, error) {
	f.cancelID = id
	return &domain.DataRetentionSettings{}, f.err
}
func (f *fakeRetentionService) ListRuns(_ context.Context, _ string, _ int) ([]*domain.RetentionRun, error) {
	return []*domain.RetentionRun{}, f.err
}
func (f *fakeRetentionService) RetryBackfill(_ context.Context, _ string) error { return f.err }
func (f *fakeRetentionService) ExportRuns(_ context.Context, _ string, w io.Writer) error {
	if f.err != nil {
		return f.err
	}
	_, err := io.WriteString(w, "run_id\n")
	return err
}

func newRetentionHandler(t *testing.T, svc *fakeRetentionService) *DataRetentionHandler {
	ctrl := gomock.NewController(t)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	return NewDataRetentionHandler(svc, func() ([]byte, error) { return []byte("secret"), nil }, false, log)
}

func TestDataRetentionHandler(t *testing.T) {
	t.Run("methods are checked", func(t *testing.T) {
		h := newRetentionHandler(t, &fakeRetentionService{})
		for _, tc := range []struct {
			handler http.HandlerFunc
			method  string
		}{
			{h.handleGet, http.MethodPost}, {h.handleRuns, http.MethodPost}, {h.handleExportRuns, http.MethodGet},
			{h.handlePreview, http.MethodGet}, {h.handleSet, http.MethodGet}, {h.handleCancelPending, http.MethodGet},
		} {
			rec := httptest.NewRecorder()
			tc.handler(rec, httptest.NewRequest(tc.method, "/", nil))
			assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		}
	})

	t.Run("workspace_id and policy are required; a raw body is decoded as sent", func(t *testing.T) {
		svc := &fakeRetentionService{}
		h := newRetentionHandler(t, svc)
		rec := httptest.NewRecorder()
		h.handleGet(rec, httptest.NewRequest(http.MethodGet, "/api/dataRetention.get", nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		for _, body := range []string{`{"policy": {}}`, `{"workspace_id": "ws"}`, `not json`} {
			rec = httptest.NewRecorder()
			h.handleSet(rec, httptest.NewRequest(http.MethodPost, "/api/dataRetention.set", strings.NewReader(body)))
			assert.Equal(t, http.StatusBadRequest, rec.Code, body)
		}

		rec = httptest.NewRecorder()
		h.handleSet(rec, httptest.NewRequest(http.MethodPost, "/api/dataRetention.set",
			strings.NewReader(`{"workspace_id": "ws", "policy": {"categories": {"inactive_contacts": 1095}, "exempt_list_ids": ["vip"]}}`)))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, 1095, svc.policy.Categories[domain.RetentionInactiveContacts])
		assert.Equal(t, []string{"vip"}, svc.policy.ExemptListIDs)

		rec = httptest.NewRecorder()
		h.handleCancelPending(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`)))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		rec = httptest.NewRecorder()
		h.handleCancelPending(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"workspace_id": "ws"}`)))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "ws", svc.cancelID)
	})

	t.Run("errors map to their status", func(t *testing.T) {
		for _, tc := range []struct {
			err  error
			code int
		}{
			{domain.NewValidationError("bad period"), http.StatusBadRequest},
			{&domain.ErrUnauthorized{Message: "Only workspace owners can change the data retention policy"}, http.StatusForbidden},
			{&domain.ErrRetentionConflict{Message: "changed meanwhile"}, http.StatusConflict},
			{assert.AnError, http.StatusInternalServerError},
		} {
			h := newRetentionHandler(t, &fakeRetentionService{err: tc.err})
			rec := httptest.NewRecorder()
			h.handleSet(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"workspace_id": "ws", "policy": {}}`)))
			assert.Equal(t, tc.code, rec.Code, tc.err.Error())
		}
	})

	t.Run("the export is a CSV file, and an error still answers JSON", func(t *testing.T) {
		h := newRetentionHandler(t, &fakeRetentionService{})
		rec := httptest.NewRecorder()
		h.handleExportRuns(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"workspace_id": "ws"}`)))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Header().Get("Content-Type"), "text/csv")
		assert.Contains(t, rec.Header().Get("Content-Disposition"), "attachment")

		h = newRetentionHandler(t, &fakeRetentionService{err: domain.NewValidationError("nope")})
		rec = httptest.NewRecorder()
		h.handleExportRuns(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"workspace_id": "ws"}`)))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	})
}
