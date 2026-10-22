package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

const usageTestSecret = "installation-secret-key"

func newUsageHandlerForTest(t *testing.T, now time.Time) (*UsageHandler, *mocks.MockUsageService) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	svc := mocks.NewMockUsageService(ctrl)
	h := NewUsageHandler(svc, usageTestSecret, logger.NewLogger())
	h.nowFn = func() time.Time { return now }
	return h, svc
}

func usageRequest(t *testing.T, secret string, ts int64) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/usage.get", nil)
	key := domain.UsageSigningKey(secret)
	req.Header.Set(UsageTimestampHeader, strconv.FormatInt(ts, 10))
	req.Header.Set(UsageSignatureHeader, domain.SignUsageRequest(key, ts, "/api/usage.get"))
	return req
}

func TestUsageHandler_Get(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	august := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	july := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	t.Run("a correctly signed request returns the usage report", func(t *testing.T) {
		h, svc := newUsageHandlerForTest(t, now)

		// The handler derives the months itself: the open one and the one before it.
		svc.EXPECT().
			GetUsage(gomock.Any(), []time.Time{july, august}).
			Return(&domain.UsageReport{
				Months: []*domain.InstanceUsage{
					{PeriodMonth: july, Pageviews: 400, TimelineEntries: 20, Workspaces: 2},
					{PeriodMonth: august, Pageviews: 1200, TimelineEntries: 55, Workspaces: 2},
				},
				WorkspaceCount: 2,
				GeneratedAt:    now,
			}, nil)

		rec := httptest.NewRecorder()
		h.handleGet(rec, usageRequest(t, usageTestSecret, now.Unix()))

		require.Equal(t, http.StatusOK, rec.Code)
		var report domain.UsageReport
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &report))
		require.Len(t, report.Months, 2)
		assert.Equal(t, int64(1200), report.Months[1].Pageviews)
		assert.Equal(t, 2, report.WorkspaceCount)
	})

	t.Run("an unsigned request is refused and never reaches the service", func(t *testing.T) {
		h, _ := newUsageHandlerForTest(t, now)
		// No EXPECT on the service: work must not begin before the signature is
		// checked, or the endpoint is a way to make an installation do work.
		rec := httptest.NewRecorder()
		h.handleGet(rec, httptest.NewRequest(http.MethodGet, "/api/usage.get", nil))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("another installation's key is refused", func(t *testing.T) {
		h, _ := newUsageHandlerForTest(t, now)
		rec := httptest.NewRecorder()
		h.handleGet(rec, usageRequest(t, "someone-elses-secret", now.Unix()))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("a replayed request is refused once it is stale", func(t *testing.T) {
		h, _ := newUsageHandlerForTest(t, now)
		stale := now.Add(-domain.UsageSignatureMaxSkew - time.Minute).Unix()
		rec := httptest.NewRecorder()
		h.handleGet(rec, usageRequest(t, usageTestSecret, stale))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("an installation with no SECRET_KEY refuses rather than deriving from empty", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		h := NewUsageHandler(mocks.NewMockUsageService(ctrl), "", logger.NewLogger())
		h.nowFn = func() time.Time { return now }

		// Signed with the key an empty secret would derive — it must still refuse,
		// or every installation missing SECRET_KEY would accept a key anyone can
		// compute.
		rec := httptest.NewRecorder()
		h.handleGet(rec, usageRequest(t, "", now.Unix()))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("a failing read answers 500, never 403", func(t *testing.T) {
		h, svc := newUsageHandlerForTest(t, now)
		svc.EXPECT().GetUsage(gomock.Any(), gomock.Any()).Return(nil, errors.New("workspace unreachable"))

		rec := httptest.NewRecorder()
		h.handleGet(rec, usageRequest(t, usageTestSecret, now.Unix()))

		// 403 would read as "this installation denied you". There is no permission
		// check here at all, and being over a quota is not a permission state.
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.NotEqual(t, http.StatusForbidden, rec.Code)
	})

	t.Run("rejects a non-GET method", func(t *testing.T) {
		h, _ := newUsageHandlerForTest(t, now)
		req := httptest.NewRequest(http.MethodPost, "/api/usage.get", nil)
		rec := httptest.NewRecorder()
		h.handleGet(rec, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("the route is registered on the bare mux, with no auth middleware", func(t *testing.T) {
		h, _ := newUsageHandlerForTest(t, now)
		mux := http.NewServeMux()
		h.RegisterRoutes(mux)

		// Reached without a JWT: the caller is a machine with no user session, and
		// the signature is what authenticates it. Unauthorized (not 404) proves the
		// route exists and that the signature check is what refused.
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/usage.get", nil))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}
