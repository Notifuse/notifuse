package broadcast_test

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service/broadcast"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// orchestratorWithBatchSize is m.orchestrator with a smaller fetch batch, so a
// run over abAudience takes several batches.
func (m *sendingStartedMocks) orchestratorWithBatchSize(size int) broadcast.BroadcastOrchestratorInterface {
	config := &broadcast.Config{FetchBatchSize: size, MaxProcessTime: 30 * time.Second, ProgressLogInterval: 5 * time.Second}
	return broadcast.NewBroadcastOrchestrator(
		m.messageSender, m.broadcastRepo, m.templateRepo, m.contactRepo, m.taskRepo, m.workspaceRepo,
		nil, nil, m.logger, config, m.timeProvider, "https://api.example.com", nil,
	)
}

func singleTask() *domain.Task {
	return sendingStartedTask(&domain.SendBroadcastState{
		BroadcastID:     "broadcast-123",
		TotalRecipients: len(abAudience),
		Phase:           "single",
	})
}

// A cancellation that lands after the loop's refresh is caught when the batch is
// fetched: the run ends as done without an error, sends nothing and does not mark
// the broadcast processed (strict mocks: no SendBatch, no UpdateBroadcast).
func TestOrchestrator_Process_CancelledWhenTheBatchIsFetched(t *testing.T) {
	m := newSendingStartedMocks(t)
	m.expectConfiguredWorkspace()
	m.expectValidTemplate()
	m.broadcastRepo.EXPECT().MarkBroadcastStarted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// The run reads the broadcast before its loop, then once at the top of the
	// loop; the read after those is the batch fetch's.
	var reads atomic.Int32
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		DoAndReturn(func(context.Context, string, string) (*domain.Broadcast, error) {
			b := sendingStartedBroadcast("Autumn sale")
			if reads.Add(1) > 2 {
				b.Status = domain.BroadcastStatusCancelled
			}
			return b, nil
		}).AnyTimes()

	allDone, err := m.orchestrator(nil).Process(context.Background(), singleTask(), time.Now().Add(30*time.Second))

	require.NoError(t, err)
	assert.True(t, allDone)
	assert.Equal(t, int32(3), reads.Load(), "cancelled at the batch fetch, not at the loop's refresh")
}

// When the run's time is up between two batches, it stops there: one batch sent,
// its progress saved, the broadcast left in progress for the next run.
func TestOrchestrator_Process_TimeLimitBetweenBatches(t *testing.T) {
	m := newSendingStartedMocks(t)
	m.expectConfiguredWorkspace()
	m.expectValidTemplate()
	m.expectAudience()
	m.broadcastRepo.EXPECT().MarkBroadcastStarted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		DoAndReturn(func(context.Context, string, string) (*domain.Broadcast, error) {
			return sendingStartedBroadcast("Autumn sale"), nil
		}).AnyTimes()

	timeoutAt := time.Now().Add(200 * time.Millisecond)
	var batches []sentBatch
	m.recordBatches(&batches, func() { time.Sleep(time.Until(timeoutAt) + 50*time.Millisecond) })
	var saved []*domain.SendBroadcastState
	m.taskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ float64, state *domain.TaskState) error {
			copied := *state.SendBroadcast
			saved = append(saved, &copied)
			return nil
		}).AnyTimes()

	allDone, err := m.orchestratorWithBatchSize(2).Process(context.Background(), singleTask(), timeoutAt)

	require.NoError(t, err)
	assert.False(t, allDone, "the task resumes in the next run")
	require.Len(t, batches, 1)
	assert.Equal(t, []string{"a@example.com", "b@example.com"}, batches[0].recipients)
	require.NotEmpty(t, saved)
	last := saved[len(saved)-1]
	assert.Equal(t, int64(2), last.RecipientOffset)
	assert.Equal(t, "b@example.com", last.LastProcessedEmail)
}

// A test phase that ends with no winner picked sends the sample only, then waits
// for the pick with the broadcast in test_completed.
func TestOrchestrator_Process_TestPhaseWithoutAWinnerSendsTheSampleOnly(t *testing.T) {
	m := newSendingStartedMocks(t)
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

	m.broadcastRepo.EXPECT().WithTransaction(gomock.Any(), "workspace-123", gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) })
	m.broadcastRepo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "workspace-123", "broadcast-123").
		Return(abBroadcast(domain.BroadcastStatusTesting, nil), nil)
	var completed *domain.Broadcast
	m.broadcastRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *sql.Tx, b *domain.Broadcast) error {
			completed = b
			return nil
		})

	allDone, err := m.orchestrator(nil).Process(context.Background(), abTask(&domain.SendBroadcastState{Phase: "test"}), time.Now().Add(30*time.Second))

	require.NoError(t, err)
	assert.False(t, allDone, "the task waits for the winner")
	var recipients []string
	for _, batch := range batches {
		recipients = append(recipients, batch.recipients...)
	}
	assert.Equal(t, []string{"a@example.com", "b@example.com"}, recipients, "half the list, the sample, and no one else")
	require.NotNil(t, completed)
	assert.Equal(t, domain.BroadcastStatusTestCompleted, completed.Status)
}
