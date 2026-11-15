package broadcast_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// A task is created by the schedule request before the broadcast's write commits, so a run
// can find its broadcast still a draft: it never counts, loads templates or sends for one.
// A young task is handed back to wait for the commit; an old one belongs to a schedule whose
// broadcast write rolled back, and is completed.
func TestProcess_RefusesADraftBroadcast(t *testing.T) {
	for _, tc := range []struct {
		name     string
		counting bool
	}{
		{"counting run", true},
		{"sending run", false},
	} {
		run := func(t *testing.T, createdAgo time.Duration) (bool, error) {
			m := newSendingStartedMocks(t)
			if !tc.counting {
				m.expectConfiguredWorkspace()
			}
			m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
				Return(singleTemplateBroadcast(domain.BroadcastStatusDraft), nil).Times(1)
			m.contactRepo.EXPECT().CountContactsForBroadcast(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			state := &domain.SendBroadcastState{BroadcastID: "broadcast-123"}
			if !tc.counting {
				state.TotalRecipients = len(abAudience)
			}
			task := sendingStartedTask(state)
			task.CreatedAt = time.Now().Add(-createdAgo)
			return m.orchestrator(nil).Process(context.Background(), task, time.Now().Add(30*time.Second))
		}

		t.Run(tc.name+": a young task waits for the schedule to commit", func(t *testing.T) {
			done, err := run(t, time.Minute)
			assert.False(t, done)
			var requeue *domain.TaskRequeueError
			require.True(t, errors.As(err, &requeue), "got %v", err)
			assert.Equal(t, 30*time.Second, requeue.After)
			assert.False(t, requeue.Park)
		})

		t.Run(tc.name+": an old task is completed without sending", func(t *testing.T) {
			done, err := run(t, time.Hour)
			require.NoError(t, err)
			assert.True(t, done)
		})
	}
}
