package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	http_handler "github.com/Notifuse/notifuse/internal/http"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// broadcastStateCase is one broadcast action posted with a raw JSON body: how the service
// mock returns err, and which handler serves it.
type broadcastStateCase struct {
	name   string
	body   string
	expect func(service *mocks.MockBroadcastService, err error)
	serve  func(handler *http_handler.BroadcastHandler, w http.ResponseWriter, r *http.Request)
}

func broadcastStateCases() []broadcastStateCase {
	return []broadcastStateCase{
		{
			name: "schedule",
			body: `{"workspace_id":"w1","id":"b1","send_now":true}`,
			expect: func(s *mocks.MockBroadcastService, err error) {
				s.EXPECT().ScheduleBroadcast(gomock.Any(), gomock.Any()).Return(err)
			},
			serve: (*http_handler.BroadcastHandler).HandleSchedule,
		},
		{
			name: "pause",
			body: `{"workspace_id":"w1","id":"b1"}`,
			expect: func(s *mocks.MockBroadcastService, err error) {
				s.EXPECT().PauseBroadcast(gomock.Any(), gomock.Any()).Return(err)
			},
			serve: (*http_handler.BroadcastHandler).HandlePause,
		},
		{
			name: "resume",
			body: `{"workspace_id":"w1","id":"b1"}`,
			expect: func(s *mocks.MockBroadcastService, err error) {
				s.EXPECT().ResumeBroadcast(gomock.Any(), gomock.Any()).Return(err)
			},
			serve: (*http_handler.BroadcastHandler).HandleResume,
		},
		{
			name: "cancel",
			body: `{"workspace_id":"w1","id":"b1"}`,
			expect: func(s *mocks.MockBroadcastService, err error) {
				s.EXPECT().CancelBroadcast(gomock.Any(), gomock.Any()).Return(err)
			},
			serve: (*http_handler.BroadcastHandler).HandleCancel,
		},
		{
			name: "delete",
			body: `{"workspace_id":"w1","id":"b1"}`,
			expect: func(s *mocks.MockBroadcastService, err error) {
				s.EXPECT().DeleteBroadcast(gomock.Any(), gomock.Any()).Return(err)
			},
			serve: (*http_handler.BroadcastHandler).HandleDelete,
		},
		{
			name: "selectWinner",
			body: `{"workspace_id":"w1","id":"b1","template_id":"tplA"}`,
			expect: func(s *mocks.MockBroadcastService, err error) {
				s.EXPECT().SelectWinner(gomock.Any(), "w1", "b1", "tplA").Return(err)
			},
			serve: (*http_handler.BroadcastHandler).HandleSelectWinner,
		},
		{
			name: "sendToIndividual",
			body: `{"workspace_id":"w1","broadcast_id":"b1","recipient_email":"ada@example.com","template_id":"tplA"}`,
			expect: func(s *mocks.MockBroadcastService, err error) {
				s.EXPECT().SendToIndividual(gomock.Any(), gomock.Any()).Return(err)
			},
			serve: (*http_handler.BroadcastHandler).HandleSendToIndividual,
		},
	}
}

func postBroadcastAction(handler *http_handler.BroadcastHandler, c broadcastStateCase) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/broadcasts."+c.name, strings.NewReader(c.body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c.serve(handler, w, req)
	return w
}

// A broadcast whose status refuses the action is the caller's request meeting the broadcast as
// it is: 409 with the state, never the 500 these handlers answered before. The strict logger
// mock also proves the error is not logged as a server fault.
func TestBroadcastHandlers_InvalidStateAnswers409(t *testing.T) {
	for _, c := range broadcastStateCases() {
		t.Run(c.name, func(t *testing.T) {
			handler, service, _, ctrl := setupBroadcastHandler(t)
			defer ctrl.Finish()
			c.expect(service, domain.NewInvalidStateError("broadcast", "b1", c.name, "draft"))

			w := postBroadcastAction(handler, c)

			require.Equal(t, http.StatusConflict, w.Code)
			var body struct {
				Error        string                   `json:"error"`
				InvalidState domain.InvalidStateError `json:"invalid_state"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, "cannot "+c.name+" broadcast b1: its status is draft", body.Error)
			assert.Equal(t, domain.InvalidStateError{Entity: "broadcast", ID: "b1", Action: c.name, Status: "draft"}, body.InvalidState)
		})
	}
}

// A variation the broadcast does not have answers 404 where it answered 500.
func TestBroadcastHandlers_UnknownVariationAnswers404(t *testing.T) {
	for _, c := range broadcastStateCases() {
		if c.name != "selectWinner" && c.name != "sendToIndividual" {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			handler, service, _, ctrl := setupBroadcastHandler(t)
			defer ctrl.Finish()
			c.expect(service, &domain.ErrNotFound{Entity: "broadcast_variation", ID: "tplA"})

			w := postBroadcastAction(handler, c)

			assert.Equal(t, http.StatusNotFound, w.Code)
			assert.Equal(t, "broadcast_variation not found with ID: tplA", errorMessage(t, w))
		})
	}
}

// Select winner on a broadcast that does not exist answers 404, like the other actions.
func TestHandleSelectWinner_BroadcastNotFoundAnswers404(t *testing.T) {
	for _, c := range broadcastStateCases() {
		if c.name != "selectWinner" {
			continue
		}
		handler, service, _, ctrl := setupBroadcastHandler(t)
		defer ctrl.Finish()
		c.expect(service, &domain.ErrBroadcastNotFound{ID: "b1"})

		w := postBroadcastAction(handler, c)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, "Broadcast not found", errorMessage(t, w))
	}
}
