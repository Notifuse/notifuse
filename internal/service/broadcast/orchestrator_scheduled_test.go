package broadcast_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A scheduled broadcast keeps the status scheduled until its task's first sending run,
// which moves it to the status its sending starts in (StartScheduledBroadcast: testing for
// an A/B test, processing otherwise) and goes on with the broadcast read again after that
// write. An A/B test then takes its test phase as one sent right away does, instead of
// sending every variation to the whole audience and staying scheduled.
//
// The task of a scheduled broadcast is created with no phase in its state
// (TaskService.ScheduleBroadcastTask), and its first run only counts the recipients: these
// runs are the second one, with the recipients counted and nothing sent yet.
//
// Plan: Notifuse/architecture plans/notifuse/fix-scheduled-ab-broadcasts-plan.md

func scheduledTask() *domain.Task {
	return abTask(&domain.SendBroadcastState{})
}

// scheduledReads serves the broadcast as the database would: first as the run read it,
// then, once the sending started, as reread. Each call returns a fresh copy.
func scheduledReads(first, reread func() *domain.Broadcast) func(context.Context, string, string) (*domain.Broadcast, error) {
	calls := 0
	return func(context.Context, string, string) (*domain.Broadcast, error) {
		calls++
		if calls == 1 {
			return first(), nil
		}
		return reread(), nil
	}
}

func TestOrchestrator_Process_StartsAScheduledBroadcast(t *testing.T) {
	t.Run("an A/B test sends its sample with every variation, then waits for its winner", func(t *testing.T) {
		m := newSendingStartedMocks(t)
		defer m.ctrl.Finish()

		m.expectConfiguredWorkspace()
		m.expectAudience()
		m.broadcastRepo.EXPECT().MarkBroadcastStarted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
			DoAndReturn(scheduledReads(
				func() *domain.Broadcast { return abBroadcast(domain.BroadcastStatusScheduled, nil) },
				func() *domain.Broadcast { return abBroadcast(domain.BroadcastStatusTesting, nil) },
			)).AnyTimes()
		m.broadcastRepo.EXPECT().StartScheduledBroadcast(gomock.Any(), "workspace-123", "broadcast-123", gomock.Any()).
			Return(true, nil)
		m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-A", int64(0)).
			Return(abTemplate("template-A", "Variation A"), nil).AnyTimes()
		m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-B", int64(0)).
			Return(abTemplate("template-B", "Variation B"), nil).AnyTimes()
		m.taskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).
			Return(nil).AnyTimes()

		var batches []sentBatch
		m.recordBatches(&batches, nil)

		// The test phase completes on the locked row; no full-row write of testing comes
		// before it (strict mocks: UpdateBroadcast has no expectation).
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

		task := scheduledTask()
		allDone, err := m.orchestrator(nil).Process(context.Background(), task, time.Now().Add(30*time.Second))
		require.NoError(t, err)
		assert.False(t, allDone, "the run waits for a winner")

		require.Len(t, batches, 1)
		assert.Equal(t, []string{"a@example.com", "b@example.com"}, batches[0].recipients, "the sample only")
		assert.Equal(t, []string{"template-A", "template-B"}, batches[0].templates)
		assert.Equal(t, "test", task.State.SendBroadcast.Phase)
		require.NotNil(t, written)
		assert.Equal(t, domain.BroadcastStatusTestCompleted, written.Status)
		assert.Equal(t, 2, written.TestPhaseRecipientCount)
	})

	t.Run("a broadcast with one template sends to everyone and ends processed", func(t *testing.T) {
		m := newSendingStartedMocks(t)
		defer m.ctrl.Finish()

		m.expectConfiguredWorkspace()
		m.expectAudience()
		m.broadcastRepo.EXPECT().MarkBroadcastStarted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
			DoAndReturn(scheduledReads(
				func() *domain.Broadcast { return singleTemplateBroadcast(domain.BroadcastStatusScheduled) },
				func() *domain.Broadcast { return singleTemplateBroadcast(domain.BroadcastStatusProcessing) },
			)).AnyTimes()
		m.broadcastRepo.EXPECT().StartScheduledBroadcast(gomock.Any(), "workspace-123", "broadcast-123", gomock.Any()).
			Return(true, nil)
		m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-A", int64(0)).
			Return(abTemplate("template-A", "Variation A"), nil).AnyTimes()
		m.taskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).
			Return(nil).AnyTimes()

		var batches []sentBatch
		m.recordBatches(&batches, nil)

		var final *domain.Broadcast
		expectFinalWrite(m.broadcastRepo, "workspace-123", singleTemplateBroadcast(domain.BroadcastStatusProcessing),
			func(_ context.Context, b *domain.Broadcast) error {
				final = b
				return nil
			})

		allDone, err := m.orchestrator(nil).Process(context.Background(), scheduledTask(), time.Now().Add(30*time.Second))
		require.NoError(t, err)
		assert.True(t, allDone)

		require.Len(t, batches, 1)
		assert.Equal(t, abAudience, batches[0].recipients)
		require.NotNil(t, final)
		assert.Equal(t, domain.BroadcastStatusProcessed, final.Status)
	})

	t.Run("the run goes on with the broadcast as it was read again after the start", func(t *testing.T) {
		// A save committed between the run's read and the start changed variation B's
		// template: that is the template the sample goes out with.
		m := newSendingStartedMocks(t)
		defer m.ctrl.Finish()

		saved := func() *domain.Broadcast {
			b := abBroadcast(domain.BroadcastStatusTesting, nil)
			b.TestSettings.Variations[1].TemplateID = "template-C"
			return b
		}
		m.expectConfiguredWorkspace()
		m.expectAudience()
		m.broadcastRepo.EXPECT().MarkBroadcastStarted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
			DoAndReturn(scheduledReads(
				func() *domain.Broadcast { return abBroadcast(domain.BroadcastStatusScheduled, nil) },
				saved,
			)).AnyTimes()
		m.broadcastRepo.EXPECT().StartScheduledBroadcast(gomock.Any(), "workspace-123", "broadcast-123", gomock.Any()).
			Return(true, nil)
		m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-A", int64(0)).
			Return(abTemplate("template-A", "Variation A"), nil).AnyTimes()
		m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-C", int64(0)).
			Return(abTemplate("template-C", "Variation C"), nil).AnyTimes()
		m.taskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).
			Return(nil).AnyTimes()

		var batches []sentBatch
		m.recordBatches(&batches, nil)
		m.broadcastRepo.EXPECT().WithTransaction(gomock.Any(), "workspace-123", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) })
		m.broadcastRepo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "workspace-123", "broadcast-123").
			Return(saved(), nil)
		m.broadcastRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		_, err := m.orchestrator(nil).Process(context.Background(), scheduledTask(), time.Now().Add(30*time.Second))
		require.NoError(t, err)

		require.Len(t, batches, 1)
		assert.Equal(t, []string{"template-A", "template-C"}, batches[0].templates)
	})

	t.Run("a broadcast already sending is not started again", func(t *testing.T) {
		// Strict mocks: StartScheduledBroadcast has no expectation.
		m := newSendingStartedMocks(t)
		defer m.ctrl.Finish()

		m.expectConfiguredWorkspace()
		m.expectAudience()
		m.broadcastRepo.EXPECT().MarkBroadcastStarted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
			DoAndReturn(func(context.Context, string, string) (*domain.Broadcast, error) {
				return singleTemplateBroadcast(domain.BroadcastStatusProcessing), nil
			}).AnyTimes()
		m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-A", int64(0)).
			Return(abTemplate("template-A", "Variation A"), nil).AnyTimes()
		m.taskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).
			Return(nil).AnyTimes()
		var batches []sentBatch
		m.recordBatches(&batches, nil)
		expectFinalWrite(m.broadcastRepo, "workspace-123", singleTemplateBroadcast(domain.BroadcastStatusProcessing),
			func(context.Context, *domain.Broadcast) error { return nil })

		allDone, err := m.orchestrator(nil).Process(context.Background(), scheduledTask(), time.Now().Add(30*time.Second))
		require.NoError(t, err)
		assert.True(t, allDone)
	})
}

// When the start writes nothing, the broadcast was no longer scheduled when the write ran:
// the run reads it again and acts on what it finds, and sends nothing in these cases
// (strict mocks: no template load, no SendBatch).
func TestOrchestrator_Process_ScheduledBroadcastThatChangedBeforeItsStart(t *testing.T) {
	run := func(t *testing.T, reread *domain.Broadcast, task *domain.Task) (bool, error) {
		m := newSendingStartedMocks(t)
		defer m.ctrl.Finish()

		m.expectConfiguredWorkspace()
		m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
			DoAndReturn(scheduledReads(
				func() *domain.Broadcast { return abBroadcast(domain.BroadcastStatusScheduled, nil) },
				func() *domain.Broadcast { copied := *reread; return &copied },
			)).AnyTimes()
		m.broadcastRepo.EXPECT().StartScheduledBroadcast(gomock.Any(), "workspace-123", "broadcast-123", gomock.Any()).
			Return(false, nil)
		return m.orchestrator(nil).Process(context.Background(), task, time.Now().Add(30*time.Second))
	}

	t.Run("cancelled: the task is done", func(t *testing.T) {
		allDone, err := run(t, abBroadcast(domain.BroadcastStatusCancelled, nil), scheduledTask())
		require.NoError(t, err)
		assert.True(t, allDone)
	})

	t.Run("back to draft: the task is handed back", func(t *testing.T) {
		task := scheduledTask()
		task.CreatedAt = time.Now()
		_, err := run(t, abBroadcast(domain.BroadcastStatusDraft, nil), task)
		var requeue *domain.TaskRequeueError
		require.ErrorAs(t, err, &requeue)
	})

	t.Run("still scheduled: the run fails rather than send without a phase", func(t *testing.T) {
		allDone, err := run(t, abBroadcast(domain.BroadcastStatusScheduled, nil), scheduledTask())
		require.Error(t, err)
		assert.False(t, allDone)
		assert.Contains(t, err.Error(), "still scheduled")
	})
}

func TestOrchestrator_Process_ScheduledBroadcastWhoseStartFails(t *testing.T) {
	t.Run("a failed start write ends the run with its error, before anything is sent", func(t *testing.T) {
		m := newSendingStartedMocks(t)
		defer m.ctrl.Finish()

		m.expectConfiguredWorkspace()
		m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
			Return(abBroadcast(domain.BroadcastStatusScheduled, nil), nil)
		m.broadcastRepo.EXPECT().StartScheduledBroadcast(gomock.Any(), "workspace-123", "broadcast-123", gomock.Any()).
			Return(false, errors.New("db down"))

		allDone, err := m.orchestrator(nil).Process(context.Background(), scheduledTask(), time.Now().Add(30*time.Second))
		require.Error(t, err)
		assert.False(t, allDone)
		assert.Contains(t, err.Error(), "db down")
	})

	t.Run("a failed read after the start ends the run with its error, before anything is sent", func(t *testing.T) {
		m := newSendingStartedMocks(t)
		defer m.ctrl.Finish()

		m.expectConfiguredWorkspace()
		gomock.InOrder(
			m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
				Return(abBroadcast(domain.BroadcastStatusScheduled, nil), nil),
			m.broadcastRepo.EXPECT().StartScheduledBroadcast(gomock.Any(), "workspace-123", "broadcast-123", gomock.Any()).
				Return(true, nil),
			m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
				Return(nil, errors.New("read failed")),
		)

		allDone, err := m.orchestrator(nil).Process(context.Background(), scheduledTask(), time.Now().Add(30*time.Second))
		require.Error(t, err)
		assert.False(t, allDone)
		assert.Contains(t, err.Error(), "read failed")
	})
}
