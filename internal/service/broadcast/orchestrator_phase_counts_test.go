package broadcast_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The A/B test and winner recipient counts are worked out by the run, kept in the
// task state, and written on the broadcast so the API and the console can show
// them. The run reads the broadcast again before each batch, so the counts must be
// put on the copy it writes, not only on the one it held when it worked them out:
// GetBroadcast returns a fresh copy on every call here, as the database does.
//
// Plan: plans/fix-console-capture-bugs-plan.md (fix 1)

func TestOrchestrator_Process_StoresABPhaseCountsOnTheBroadcast(t *testing.T) {
	// abAudience has four recipients and abBroadcast tests on half of them.
	const wantTest, wantWinner = 2, 2

	t.Run("the broadcast written when the test phase completes carries both counts", func(t *testing.T) {
		m := newSendingStartedMocks(t)
		defer m.ctrl.Finish()

		m.expectConfiguredWorkspace()
		m.expectAudience()
		m.broadcastRepo.EXPECT().MarkBroadcastStarted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
			DoAndReturn(func(context.Context, string, string) (*domain.Broadcast, error) {
				return abBroadcast(domain.BroadcastStatusTesting, nil), nil
			}).AnyTimes()
		m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-A", int64(0)).
			Return(abTemplate("template-A", "Variation A"), nil).AnyTimes()
		m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-B", int64(0)).
			Return(abTemplate("template-B", "Variation B"), nil).AnyTimes()
		m.taskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).
			Return(nil).AnyTimes()

		var batches []sentBatch
		m.recordBatches(&batches, nil)

		m.broadcastRepo.EXPECT().WithTransaction(gomock.Any(), "workspace-123", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) })
		m.broadcastRepo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "workspace-123", "broadcast-123").
			Return(abBroadcast(domain.BroadcastStatusTesting, nil), nil)
		var written *domain.Broadcast
		m.broadcastRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, b *domain.Broadcast) error {
				written = b
				return nil
			})

		task := abTask(&domain.SendBroadcastState{Phase: "test"})

		allDone, err := m.orchestrator(nil).Process(context.Background(), task, time.Now().Add(30*time.Second))
		require.NoError(t, err)
		assert.False(t, allDone, "the run waits for a winner")

		require.NotNil(t, written)
		assert.Equal(t, domain.BroadcastStatusTestCompleted, written.Status)
		assert.Equal(t, wantTest, written.TestPhaseRecipientCount)
		assert.Equal(t, wantWinner, written.WinnerPhaseRecipientCount)
	})

	t.Run("the broadcast written when the winner phase completes carries both counts", func(t *testing.T) {
		m := newSendingStartedMocks(t)
		defer m.ctrl.Finish()

		m.expectConfiguredWorkspace()
		m.expectAudience()
		m.broadcastRepo.EXPECT().MarkBroadcastStarted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		// The winner was picked by hand: the stored copy SelectWinner wrote may
		// still hold no counts, and the final save is what puts them there.
		m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
			DoAndReturn(func(context.Context, string, string) (*domain.Broadcast, error) {
				return abBroadcast(domain.BroadcastStatusWinnerSelected, stringPtr("template-B")), nil
			}).AnyTimes()
		m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-B", int64(0)).
			Return(abTemplate("template-B", "Variation B"), nil).AnyTimes()
		m.taskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).
			Return(nil).AnyTimes()

		var batches []sentBatch
		m.recordBatches(&batches, nil)

		var final *domain.Broadcast
		m.broadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
				final = b
				return nil
			})

		task := abTask(&domain.SendBroadcastState{
			Phase:              "test",
			TestPhaseCompleted: true,
			RecipientOffset:    2,
			EnqueuedCount:      2,
			LastProcessedEmail: "b@example.com",
		})

		allDone, err := m.orchestrator(nil).Process(context.Background(), task, time.Now().Add(30*time.Second))
		require.NoError(t, err)
		assert.True(t, allDone)

		require.NotNil(t, final)
		assert.Equal(t, domain.BroadcastStatusProcessed, final.Status)
		assert.Equal(t, wantTest, final.TestPhaseRecipientCount)
		assert.Equal(t, wantWinner, final.WinnerPhaseRecipientCount)
	})

	t.Run("a broadcast with a single template keeps no phase counts", func(t *testing.T) {
		m := newSendingStartedMocks(t)
		defer m.ctrl.Finish()

		single := func() *domain.Broadcast {
			b := abBroadcast(domain.BroadcastStatusProcessing, nil)
			b.TestSettings = domain.BroadcastTestSettings{
				Variations: []domain.BroadcastVariation{{VariationName: "A", TemplateID: "template-A"}},
			}
			return b
		}

		m.expectConfiguredWorkspace()
		m.expectAudience()
		m.broadcastRepo.EXPECT().MarkBroadcastStarted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		m.eventBus.EXPECT().Publish(gomock.Any(), gomock.Any()).AnyTimes()
		m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
			DoAndReturn(func(context.Context, string, string) (*domain.Broadcast, error) { return single(), nil }).
			AnyTimes()
		m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-A", int64(0)).
			Return(abTemplate("template-A", "Variation A"), nil).AnyTimes()
		m.taskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).
			Return(nil).AnyTimes()

		var batches []sentBatch
		m.recordBatches(&batches, nil)

		var final *domain.Broadcast
		m.broadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
				final = b
				return nil
			})

		allDone, err := m.orchestrator(nil).Process(context.Background(), abTask(&domain.SendBroadcastState{}), time.Now().Add(30*time.Second))
		require.NoError(t, err)
		assert.True(t, allDone)

		require.NotNil(t, final)
		assert.Equal(t, domain.BroadcastStatusProcessed, final.Status)
		assert.Zero(t, final.TestPhaseRecipientCount)
		assert.Zero(t, final.WinnerPhaseRecipientCount)
	})
}
