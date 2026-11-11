package http_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// TestBroadcastHandlers_SenderErrors: a sender the caller chose that the marketing integration
// does not have is the caller's to fix, so it answers 400 with the reason; any other service
// failure keeps its 500.
func TestBroadcastHandlers_SenderErrors(t *testing.T) {
	type endpoint struct {
		name   string
		body   interface{}
		expect func(m *mocks.MockBroadcastService, err error)
		call   func(w http.ResponseWriter, r *http.Request)
	}

	senderGone := domain.NewValidationError("sender promos is not a sender of the marketing email integration")
	senderNotFound := fmt.Errorf("failed to get sender for broadcast: %w", fmt.Errorf("%w: promos", domain.ErrSenderNotFound))

	run := func(t *testing.T, serviceErr error, wantCode int, wantMessage string) {
		handler, mockService, _, mockLogger, ctrl := setupBroadcastHandler(t)
		defer ctrl.Finish()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		endpoints := []endpoint{
			{
				name: "create",
				body: domain.CreateBroadcastRequest{WorkspaceID: "workspace123", Name: "Promo", Audience: domain.AudienceSettings{List: "list123"}},
				expect: func(m *mocks.MockBroadcastService, err error) {
					m.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).Return(nil, err)
				},
				call: handler.HandleCreate,
			},
			{
				name: "update",
				body: domain.UpdateBroadcastRequest{ID: "broadcast123", WorkspaceID: "workspace123", Name: "Promo", Audience: domain.AudienceSettings{List: "list123"}},
				expect: func(m *mocks.MockBroadcastService, err error) {
					m.EXPECT().GetBroadcast(gomock.Any(), "workspace123", "broadcast123").Return(createTestBroadcast(), nil)
					m.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Return(nil, err)
				},
				call: handler.HandleUpdate,
			},
			{
				name: "schedule",
				body: domain.ScheduleBroadcastRequest{WorkspaceID: "workspace123", ID: "broadcast123", SendNow: true},
				expect: func(m *mocks.MockBroadcastService, err error) {
					m.EXPECT().ScheduleBroadcast(gomock.Any(), gomock.Any()).Return(err)
				},
				call: handler.HandleSchedule,
			},
			{
				name: "resume",
				body: domain.ResumeBroadcastRequest{WorkspaceID: "workspace123", ID: "broadcast123"},
				expect: func(m *mocks.MockBroadcastService, err error) {
					m.EXPECT().ResumeBroadcast(gomock.Any(), gomock.Any()).Return(err)
				},
				call: handler.HandleResume,
			},
			{
				name: "send to individual",
				body: domain.SendToIndividualRequest{WorkspaceID: "workspace123", BroadcastID: "broadcast123", RecipientEmail: "user@example.com"},
				expect: func(m *mocks.MockBroadcastService, err error) {
					m.EXPECT().SendToIndividual(gomock.Any(), gomock.Any()).Return(err)
				},
				call: handler.HandleSendToIndividual,
			},
		}

		for _, e := range endpoints {
			t.Run(e.name, func(t *testing.T) {
				e.expect(mockService, serviceErr)
				body, _ := json.Marshal(e.body)
				w := httptest.NewRecorder()
				e.call(w, httptest.NewRequest(http.MethodPost, "/api/broadcasts", bytes.NewBuffer(body)))

				assert.Equal(t, wantCode, w.Code)
				var response map[string]interface{}
				_ = json.Unmarshal(w.Body.Bytes(), &response)
				assert.Contains(t, response["error"], wantMessage)
			})
		}
	}

	t.Run("a chosen sender the integration does not have is a 400", func(t *testing.T) {
		run(t, senderGone, http.StatusBadRequest, "sender promos is not a sender")
	})

	t.Run("a wrapped one too", func(t *testing.T) {
		run(t, fmt.Errorf("transaction failed: %w", senderGone), http.StatusBadRequest, "sender promos is not a sender")
	})

	t.Run("a sender not found is a 400", func(t *testing.T) {
		run(t, senderNotFound, http.StatusBadRequest, "sender not found: promos")
	})

	t.Run("other service errors stay 500", func(t *testing.T) {
		run(t, errors.New("database is down"), http.StatusInternalServerError, "Failed to")
	})
}
