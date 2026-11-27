package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type undoListServiceStub struct {
	domain.ListService
	unsubscribeWithResultsFn func(ctx context.Context, payload *domain.UnsubscribeFromListsRequest, hasBearerToken bool) ([]domain.UnsubscribedListResult, error)
	resubscribeWithTokenFn   func(ctx context.Context, payload *domain.ResubscribeRequest) (*domain.ContactList, error)
}

func (s *undoListServiceStub) UnsubscribeFromListsWithResults(ctx context.Context, payload *domain.UnsubscribeFromListsRequest, hasBearerToken bool) ([]domain.UnsubscribedListResult, error) {
	return s.unsubscribeWithResultsFn(ctx, payload, hasBearerToken)
}

func (s *undoListServiceStub) ResubscribeWithToken(ctx context.Context, payload *domain.ResubscribeRequest) (*domain.ContactList, error) {
	return s.resubscribeWithTokenFn(ctx, payload)
}

func TestNotificationCenterHandler_UnsubscribeAndResubscribeUndo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockNotifCenter := mocks.NewMockNotificationCenterService(ctrl)
	stub := &undoListServiceStub{ListService: mocks.NewMockListService(ctrl)}
	handler := NewNotificationCenterHandler(mockNotifCenter, stub, &mockLogger{}, nil)

	t.Run("POST /unsubscribe returns unsubscribed_lists with undo token", func(t *testing.T) {
		stub.unsubscribeWithResultsFn = func(_ context.Context, payload *domain.UnsubscribeFromListsRequest, hasBearerToken bool) ([]domain.UnsubscribedListResult, error) {
			assert.False(t, hasBearerToken)
			assert.Equal(t, []string{"vip"}, payload.ListIDs)
			return []domain.UnsubscribedListResult{{
				ID:               "vip",
				Name:             "VIP Members",
				ResubscribeToken: "1790000000.sig",
			}}, nil
		}

		req := httptest.NewRequest(http.MethodPost, "/unsubscribe", strings.NewReader(`{"wid":"ws1","email":"a@example.com","email_hmac":"hm","lids":["vip"]}`))
		rec := httptest.NewRecorder()
		handler.handleUnsubscribe(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var decoded struct {
			Success           bool                            `json:"success"`
			UnsubscribedLists []domain.UnsubscribedListResult `json:"unsubscribed_lists"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &decoded))
		assert.True(t, decoded.Success)
		require.Len(t, decoded.UnsubscribedLists, 1)
		assert.Equal(t, "VIP Members", decoded.UnsubscribedLists[0].Name)
		assert.Equal(t, "1790000000.sig", decoded.UnsubscribedLists[0].ResubscribeToken)
	})

	t.Run("POST /resubscribe rejects non-POST", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/resubscribe", nil)
		rec := httptest.NewRecorder()
		handler.handleResubscribe(rec, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("POST /resubscribe rejects missing fields and oversized bodies", func(t *testing.T) {
		badBodies := []struct {
			name string
			body string
		}{
			{"missing wid", `{"email":"a@example.com","email_hmac":"hm","lid":"vip","token":"tok"}`},
			{"missing email", `{"wid":"ws1","email_hmac":"hm","lid":"vip","token":"tok"}`},
			{"missing email_hmac", `{"wid":"ws1","email":"a@example.com","lid":"vip","token":"tok"}`},
			{"missing lid", `{"wid":"ws1","email":"a@example.com","email_hmac":"hm","token":"tok"}`},
			{"missing token", `{"wid":"ws1","email":"a@example.com","email_hmac":"hm","lid":"vip"}`},
			{"malformed json", `{"wid":`},
			{"oversized body", `{"wid":"ws1","email":"a@example.com","email_hmac":"hm","lid":"vip","token":"` + strings.Repeat("a", maxUnsubscribeBodyBytes+16) + `"}`},
		}
		for _, tc := range badBodies {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, "/resubscribe", strings.NewReader(tc.body))
				rec := httptest.NewRecorder()
				handler.handleResubscribe(rec, req)
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			})
		}
	})

	t.Run("POST /resubscribe enforces subscribe:email and subscribe:ip rate limits", func(t *testing.T) {
		rl := ratelimiter.NewRateLimiter()
		rl.SetPolicy("subscribe:email", 1, time.Minute)
		rl.SetPolicy("subscribe:ip", 2, time.Minute)

		stub.resubscribeWithTokenFn = func(_ context.Context, payload *domain.ResubscribeRequest) (*domain.ContactList, error) {
			return &domain.ContactList{ListID: payload.ListID, ListName: "VIP", Status: domain.ContactListStatusActive}, nil
		}
		rlHandler := NewNotificationCenterHandler(mockNotifCenter, stub, &mockLogger{}, rl)

		send := func(email, ip string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPost, "/resubscribe", strings.NewReader(`{"wid":"ws1","email":"`+email+`","email_hmac":"hm","lid":"vip","token":"tok"}`))
			req.RemoteAddr = ip + ":12345"
			rec := httptest.NewRecorder()
			rlHandler.handleResubscribe(rec, req)
			return rec
		}

		// First call for a@example.com succeeds
		assert.Equal(t, http.StatusOK, send("a@example.com", "203.0.113.1").Code)

		// Second call for same email hits subscribe:email limit
		emailLimited := send("a@example.com", "203.0.113.2")
		assert.Equal(t, http.StatusTooManyRequests, emailLimited.Code)
		assert.NotEmpty(t, emailLimited.Header().Get("Retry-After"))

		// Second call from same IP with different email succeeds (IP limit = 2)
		assert.Equal(t, http.StatusOK, send("b@example.com", "203.0.113.1").Code)

		// Third call from same IP hits subscribe:ip limit
		ipLimited := send("c@example.com", "203.0.113.1")
		assert.Equal(t, http.StatusTooManyRequests, ipLimited.Code)
		assert.NotEmpty(t, ipLimited.Header().Get("Retry-After"))
	})

	t.Run("POST /resubscribe restores subscription", func(t *testing.T) {
		stub.resubscribeWithTokenFn = func(_ context.Context, payload *domain.ResubscribeRequest) (*domain.ContactList, error) {
			assert.Equal(t, "ws1", payload.WorkspaceID)
			assert.Equal(t, "a@example.com", payload.Email)
			assert.Equal(t, "vip", payload.ListID)
			assert.Equal(t, "1790000000.sig", payload.Token)
			return &domain.ContactList{ListID: "vip", ListName: "VIP Members", Status: domain.ContactListStatusActive}, nil
		}

		req := httptest.NewRequest(http.MethodPost, "/resubscribe", strings.NewReader(`{"wid":"ws1","email":"a@example.com","email_hmac":"hm","lid":"vip","token":"1790000000.sig"}`))
		rec := httptest.NewRecorder()
		handler.handleResubscribe(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"success":true,"list":{"id":"vip","name":"VIP Members"}}`, rec.Body.String())
	})

	errorCases := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"invalid hmac", errors.New("invalid email verification"), http.StatusUnauthorized},
		{"expired token", domain.ErrResubscribeTokenExpired, http.StatusBadRequest},
		{"invalid token", domain.ErrResubscribeTokenInvalid, http.StatusBadRequest},
		{"not allowed", domain.ErrResubscribeNotAllowed, http.StatusConflict},
		{"list deleted", &domain.ErrListNotFound{Message: "not found"}, http.StatusNotFound},
		{"unexpected error", errors.New("db down"), http.StatusInternalServerError},
	}
	for _, tc := range errorCases {
		t.Run("POST /resubscribe error mapping: "+tc.name, func(t *testing.T) {
			stub.resubscribeWithTokenFn = func(_ context.Context, _ *domain.ResubscribeRequest) (*domain.ContactList, error) {
				return nil, tc.err
			}
			req := httptest.NewRequest(http.MethodPost, "/resubscribe", strings.NewReader(`{"wid":"ws1","email":"a@example.com","email_hmac":"hm","lid":"vip","token":"tok"}`))
			rec := httptest.NewRecorder()
			handler.handleResubscribe(rec, req)
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}
