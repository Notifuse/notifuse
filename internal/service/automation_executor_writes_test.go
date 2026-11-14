package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stepFunc is a node executor whose behaviour each test gives.
type stepFunc func(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error)

func (f stepFunc) Execute(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
	return f(ctx, params)
}

func (stepFunc) NodeType() domain.NodeType { return domain.NodeTypeWebhook }

// writesHarness runs Execute on a two-step automation, X (a webhook step whose behaviour the
// test gives) then Y (a delay), with strict mocks.
type writesHarness struct {
	repo     *mocks.MockAutomationRepository
	timeline *mocks.MockContactTimelineRepository
	executor *AutomationExecutor
	ca       *domain.ContactAutomation
}

const writesWorkspace = "ws1"

func newWritesHarness(t *testing.T, step stepFunc, xNext *string) *writesHarness {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockAutomationRepository(ctrl)
	contacts := mocks.NewMockContactRepository(ctrl)
	timeline := mocks.NewMockContactTimelineRepository(ctrl)
	x := "x"
	automation := &domain.Automation{ID: "auto1", Status: domain.AutomationStatusLive, Nodes: []*domain.AutomationNode{
		{ID: "x", Type: domain.NodeTypeWebhook, NextNodeID: xNext},
		{ID: "y", Type: domain.NodeTypeDelay, Config: map[string]interface{}{"duration": 1, "unit": "hours"}},
	}}
	repo.EXPECT().GetByID(gomock.Any(), writesWorkspace, "auto1").Return(automation, nil).AnyTimes()
	contacts.EXPECT().GetContactByEmail(gomock.Any(), writesWorkspace, "c@example.com").Return(&domain.Contact{Email: "c@example.com"}, nil).AnyTimes()
	repo.EXPECT().GetNodeExecutions(gomock.Any(), writesWorkspace, "ca1").Return(nil, nil).AnyTimes()
	return &writesHarness{
		repo:     repo,
		timeline: timeline,
		executor: &AutomationExecutor{
			automationRepo: repo,
			contactRepo:    contacts,
			timelineRepo:   timeline,
			nodeExecutors: map[domain.NodeType]NodeExecutor{
				domain.NodeTypeWebhook: step,
				domain.NodeTypeDelay:   NewDelayNodeExecutor(),
			},
			logger: setupMockLogger(ctrl),
		},
		ca: &domain.ContactAutomation{
			ID: "ca1", AutomationID: "auto1", ContactEmail: "c@example.com", CurrentNodeID: &x,
			Status: domain.ContactAutomationStatusActive, MaxRetries: 3,
		},
	}
}

// expectProcessingRow expects the processing row a step's run creates. Execute drops this
// write's error, so the row is checked by assertions.
func (h *writesHarness) expectProcessingRow(t *testing.T, nodeID string) *gomock.Call {
	return h.repo.EXPECT().CreateNodeExecution(gomock.Any(), writesWorkspace, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, ne *domain.NodeExecution) error {
			assert.Equal(t, nodeID, ne.NodeID)
			assert.Equal(t, domain.NodeActionProcessing, ne.Action)
			return nil
		})
}

// live checks that a write runs on a context the run's cancellation did not end. Its bound
// once the run is cancelled is TestWriteContext's.
func live(t *testing.T, ctx context.Context) {
	t.Helper()
	assert.NoError(t, ctx.Err(), "the write outlives the cancelled run")
	_, hasDeadline := ctx.Deadline()
	assert.False(t, hasDeadline, "the write has no deadline of its own")
}

// The writes that record a step have no deadline while the run is live: a slow save waits as
// the run's other writes do. Once the run is cancelled they go on for afterStepWriteTimeout.
func TestWriteContext(t *testing.T) {
	t.Run("a live run puts no deadline on the writes", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		writeCtx, done := writeContext(ctx)
		defer done()
		_, hasDeadline := writeCtx.Deadline()
		assert.False(t, hasDeadline)
		select {
		case <-writeCtx.Done():
			t.Fatal("the write context ended while the run is live")
		case <-time.After(50 * time.Millisecond):
		}
	})

	t.Run("a cancelled run ends the writes afterStepWriteTimeout later", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		writeCtx, done := writeContext(ctx)
		defer done()
		cancelledAt := time.Now()
		cancel()
		assert.NoError(t, writeCtx.Err(), "the writes outlive the cancellation")
		select {
		case <-writeCtx.Done():
			assert.GreaterOrEqual(t, time.Since(cancelledAt), afterStepWriteTimeout)
		case <-time.After(afterStepWriteTimeout + 2*time.Second):
			t.Fatal("the write context outlived its bound")
		}
	})

	t.Run("done releases the context", func(t *testing.T) {
		writeCtx, done := writeContext(context.Background())
		done()
		assert.Error(t, writeCtx.Err())
	})
}

func TestAutomationExecutor_Execute_StepCancelledByTheRunIsNotAFailedAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	h := newWritesHarness(t, func(context.Context, NodeExecutionParams) (*NodeExecutionResult, error) {
		cancel()
		return nil, context.Canceled
	}, nil)

	h.expectProcessingRow(t, "x")
	h.repo.EXPECT().UpdateNodeExecution(gomock.Any(), writesWorkspace, gomock.Any()).
		DoAndReturn(func(wctx context.Context, _ string, ne *domain.NodeExecution) error {
			live(t, wctx)
			assert.Equal(t, domain.NodeActionFailed, ne.Action)
			assert.NotNil(t, ne.CompletedAt, "a closed row counts no pass")
			return nil
		})
	// No retry spent: no journey write, no failed entry, no stat.

	require.NoError(t, h.executor.Execute(ctx, writesWorkspace, h.ca))
	assert.Equal(t, 0, h.ca.RetryCount)
	assert.Equal(t, domain.ContactAutomationStatusActive, h.ca.Status)
}

func TestAutomationExecutor_Execute_StepThatRanIsSavedWhenTheRunIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := newWritesHarness(t, func(context.Context, NodeExecutionParams) (*NodeExecutionResult, error) {
		cancel() // the shutdown lands while the step runs; its work is done
		return &NodeExecutionResult{Status: domain.ContactAutomationStatusActive}, nil
	}, nil)

	h.expectProcessingRow(t, "x")
	h.repo.EXPECT().UpdateContactAutomationIfActive(gomock.Any(), writesWorkspace, gomock.Any()).
		DoAndReturn(func(wctx context.Context, _ string, ca *domain.ContactAutomation) (bool, error) {
			live(t, wctx)
			assert.Equal(t, domain.ContactAutomationStatusCompleted, ca.Status)
			return true, nil
		})
	h.repo.EXPECT().UpdateNodeExecution(gomock.Any(), writesWorkspace, gomock.Any()).
		DoAndReturn(func(wctx context.Context, _ string, ne *domain.NodeExecution) error {
			live(t, wctx)
			assert.Equal(t, domain.NodeActionCompleted, ne.Action)
			return nil
		})
	h.repo.EXPECT().IncrementAutomationStat(gomock.Any(), writesWorkspace, "auto1", "completed").
		DoAndReturn(func(wctx context.Context, _, _, _ string) error { live(t, wctx); return nil })
	h.timeline.EXPECT().Create(gomock.Any(), writesWorkspace, gomock.Any()).
		DoAndReturn(func(wctx context.Context, _ string, _ *domain.ContactTimelineEntry) error { live(t, wctx); return nil })

	require.NoError(t, h.executor.Execute(ctx, writesWorkspace, h.ca))
}

func TestAutomationExecutor_Execute_CancelledRunStartsNoOtherStep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	y := "y"
	h := newWritesHarness(t, func(context.Context, NodeExecutionParams) (*NodeExecutionResult, error) {
		return &NodeExecutionResult{NextNodeID: &y, Status: domain.ContactAutomationStatusActive}, nil
	}, &y)

	h.expectProcessingRow(t, "x")
	h.repo.EXPECT().UpdateContactAutomationIfActive(gomock.Any(), writesWorkspace, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, ca *domain.ContactAutomation) (bool, error) {
			cancel() // the shutdown lands during X's save
			assert.Equal(t, "y", *ca.CurrentNodeID)
			return true, nil
		})
	h.repo.EXPECT().UpdateNodeExecution(gomock.Any(), writesWorkspace, gomock.Any()).Return(nil)
	// Y, the delay, is neither run (no processing row) nor persisted.

	require.NoError(t, h.executor.Execute(ctx, writesWorkspace, h.ca))
}

func TestAutomationExecutor_Execute_ReadCancelledByTheRunIsNotAFailedAttempt(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockAutomationRepository(ctrl)
	executor := &AutomationExecutor{automationRepo: repo, logger: setupMockLogger(ctrl)}
	x := "x"
	ca := &domain.ContactAutomation{ID: "ca1", AutomationID: "auto1", CurrentNodeID: &x, Status: domain.ContactAutomationStatusActive, MaxRetries: 3}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo.EXPECT().GetByID(gomock.Any(), writesWorkspace, "auto1").Return(nil, context.Canceled)
	// No journey write, no failed entry, no stat.

	require.NoError(t, executor.Execute(ctx, writesWorkspace, ca))
	assert.Equal(t, 0, ca.RetryCount)
	assert.Nil(t, ca.LastError)
}

func TestAutomationExecutor_Execute_FailedSaveAfterAStep(t *testing.T) {
	saveErr := errors.New("db down")

	t.Run("a step with a next one: the step is completed and the retry goes on from the next", func(t *testing.T) {
		y := "y"
		h := newWritesHarness(t, func(context.Context, NodeExecutionParams) (*NodeExecutionResult, error) {
			return &NodeExecutionResult{NextNodeID: &y, Status: domain.ContactAutomationStatusActive}, nil
		}, &y)

		h.expectProcessingRow(t, "x") // and no failed row: no step failed
		gomock.InOrder(
			h.repo.EXPECT().UpdateContactAutomationIfActive(gomock.Any(), writesWorkspace, gomock.Any()).Return(false, saveErr),
			h.repo.EXPECT().UpdateNodeExecution(gomock.Any(), writesWorkspace, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, ne *domain.NodeExecution) error {
					assert.Equal(t, "x", ne.NodeID)
					assert.Equal(t, domain.NodeActionCompleted, ne.Action)
					return nil
				}),
			h.repo.EXPECT().UpdateContactAutomationIfActive(gomock.Any(), writesWorkspace, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, ca *domain.ContactAutomation) (bool, error) {
					assert.Equal(t, "y", *ca.CurrentNodeID)
					assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)
					assert.Equal(t, 1, ca.RetryCount)
					assert.Contains(t, *ca.LastError, "db down")
					assert.WithinDuration(t, time.Now().Add(2*time.Minute), *ca.ScheduledAt, 5*time.Second)
					return true, nil
				}),
		)

		require.NoError(t, h.executor.Execute(context.Background(), writesWorkspace, h.ca))
	})

	t.Run("a delay keeps its own, later resume time", func(t *testing.T) {
		resume := time.Now().UTC().Add(time.Hour)
		y := "y"
		h := newWritesHarness(t, func(context.Context, NodeExecutionParams) (*NodeExecutionResult, error) {
			return &NodeExecutionResult{NextNodeID: &y, Status: domain.ContactAutomationStatusActive, ScheduledAt: &resume}, nil
		}, &y)

		h.expectProcessingRow(t, "x")
		gomock.InOrder(
			h.repo.EXPECT().UpdateContactAutomationIfActive(gomock.Any(), writesWorkspace, gomock.Any()).Return(false, saveErr),
			h.repo.EXPECT().UpdateNodeExecution(gomock.Any(), writesWorkspace, gomock.Any()).Return(nil),
			h.repo.EXPECT().UpdateContactAutomationIfActive(gomock.Any(), writesWorkspace, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, ca *domain.ContactAutomation) (bool, error) {
					assert.True(t, ca.ScheduledAt.Equal(resume), "scheduled %v, want %v", ca.ScheduledAt, resume)
					return true, nil
				}),
		)

		require.NoError(t, h.executor.Execute(context.Background(), writesWorkspace, h.ca))
	})

	t.Run("the last step: the retry completes the journey with its stat and end", func(t *testing.T) {
		h := newWritesHarness(t, func(context.Context, NodeExecutionParams) (*NodeExecutionResult, error) {
			return &NodeExecutionResult{Status: domain.ContactAutomationStatusActive}, nil
		}, nil)

		h.expectProcessingRow(t, "x")
		gomock.InOrder(
			h.repo.EXPECT().UpdateContactAutomationIfActive(gomock.Any(), writesWorkspace, gomock.Any()).Return(false, saveErr),
			h.repo.EXPECT().UpdateNodeExecution(gomock.Any(), writesWorkspace, gomock.Any()).Return(nil),
			h.repo.EXPECT().UpdateContactAutomationIfActive(gomock.Any(), writesWorkspace, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, ca *domain.ContactAutomation) (bool, error) {
					assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status, "not stored completed with a retry time")
					assert.Nil(t, ca.CurrentNodeID)
					assert.NotNil(t, ca.ScheduledAt)
					return true, nil
				}),
		)
		require.NoError(t, h.executor.Execute(context.Background(), writesWorkspace, h.ca))

		// The retry finds no current node and completes the journey.
		gomock.InOrder(
			h.repo.EXPECT().UpdateContactAutomationIfActive(gomock.Any(), writesWorkspace, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, ca *domain.ContactAutomation) (bool, error) {
					assert.Equal(t, domain.ContactAutomationStatusCompleted, ca.Status)
					return true, nil
				}),
			h.repo.EXPECT().IncrementAutomationStat(gomock.Any(), writesWorkspace, "auto1", "completed").Return(nil),
		)
		h.timeline.EXPECT().Create(gomock.Any(), writesWorkspace, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, e *domain.ContactTimelineEntry) error {
				assert.Equal(t, "completed", e.Changes["exit_reason"].(map[string]interface{})["new"])
				return nil
			})
		require.NoError(t, h.executor.Execute(context.Background(), writesWorkspace, h.ca))
	})

	t.Run("an exiting step: the retry exits the journey with the step's reason", func(t *testing.T) {
		h := newWritesHarness(t, func(context.Context, NodeExecutionParams) (*NodeExecutionResult, error) {
			reason := "unsubscribed"
			return &NodeExecutionResult{Status: domain.ContactAutomationStatusExited, ExitReason: &reason}, nil
		}, nil)

		h.expectProcessingRow(t, "x")
		gomock.InOrder(
			h.repo.EXPECT().UpdateContactAutomationIfActive(gomock.Any(), writesWorkspace, gomock.Any()).Return(false, saveErr),
			h.repo.EXPECT().UpdateNodeExecution(gomock.Any(), writesWorkspace, gomock.Any()).Return(nil),
			h.repo.EXPECT().UpdateContactAutomationIfActive(gomock.Any(), writesWorkspace, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, ca *domain.ContactAutomation) (bool, error) {
					assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)
					assert.Equal(t, "unsubscribed", *ca.ExitReason)
					return true, nil
				}),
		)
		require.NoError(t, h.executor.Execute(context.Background(), writesWorkspace, h.ca))

		gomock.InOrder(
			h.repo.EXPECT().UpdateContactAutomationIfActive(gomock.Any(), writesWorkspace, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, ca *domain.ContactAutomation) (bool, error) {
					assert.Equal(t, domain.ContactAutomationStatusExited, ca.Status)
					assert.Equal(t, "unsubscribed", *ca.ExitReason)
					return true, nil
				}),
			h.repo.EXPECT().IncrementAutomationStat(gomock.Any(), writesWorkspace, "auto1", "exited").Return(nil),
		)
		h.timeline.EXPECT().Create(gomock.Any(), writesWorkspace, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, e *domain.ContactTimelineEntry) error {
				assert.Equal(t, "unsubscribed", e.Changes["exit_reason"].(map[string]interface{})["new"])
				assert.Equal(t, string(domain.ContactAutomationStatusExited), e.Changes["status"].(map[string]interface{})["new"])
				return nil
			})
		require.NoError(t, h.executor.Execute(context.Background(), writesWorkspace, h.ca))
	})
}

// The completed, exited and failed stats and end events are written only when the journey's
// row was still active: a journey a reply or a deletion ended meanwhile keeps its own end.
func TestAutomationExecutor_EndsCountedOnlyWhenTheRowChanged(t *testing.T) {
	ends := map[string]func(e *AutomationExecutor, ca *domain.ContactAutomation) error{
		"completed": func(e *AutomationExecutor, ca *domain.ContactAutomation) error {
			return e.markAsCompleted(context.Background(), writesWorkspace, ca, "completed")
		},
		"exited": func(e *AutomationExecutor, ca *domain.ContactAutomation) error {
			return e.markAsExited(context.Background(), writesWorkspace, ca, "automation_node_deleted")
		},
		"failed": func(e *AutomationExecutor, ca *domain.ContactAutomation) error {
			ca.RetryCount = ca.MaxRetries - 1
			return e.handleError(context.Background(), writesWorkspace, ca, errors.New("boom"), "node execution failed")
		},
	}
	for stat, end := range ends {
		for _, updated := range []bool{false, true} {
			name := stat + ", row ended meanwhile"
			if updated {
				name = stat + ", row still active"
			}
			t.Run(name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				repo := mocks.NewMockAutomationRepository(ctrl)
				timeline := mocks.NewMockContactTimelineRepository(ctrl)
				e := &AutomationExecutor{automationRepo: repo, timelineRepo: timeline, logger: setupMockLogger(ctrl)}
				x := "x"
				ca := &domain.ContactAutomation{ID: "ca1", AutomationID: "auto1", CurrentNodeID: &x, Status: domain.ContactAutomationStatusActive, MaxRetries: 3}

				repo.EXPECT().CreateNodeExecution(gomock.Any(), writesWorkspace, gomock.Any()).Return(nil).AnyTimes()
				repo.EXPECT().UpdateContactAutomationIfActive(gomock.Any(), writesWorkspace, gomock.Any()).Return(updated, nil)
				times := 0
				if updated {
					times = 1
				}
				repo.EXPECT().IncrementAutomationStat(gomock.Any(), writesWorkspace, "auto1", stat).Return(nil).Times(times)
				timeline.EXPECT().Create(gomock.Any(), writesWorkspace, gomock.Any()).Return(nil).Times(times)

				require.NoError(t, end(e, ca))
			})
		}
	}
}

// A run cancelled while the processing row of a step is written runs no step: a delay or an
// A/B split, which do not read the context, would be saved with no row of their own. The row
// is closed as a step cut short, and no retry is spent.
func TestAutomationExecutor_Execute_CancelledDuringTheProcessingRowRunsNoStep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := newWritesHarness(t, func(context.Context, NodeExecutionParams) (*NodeExecutionResult, error) {
		t.Error("the step ran on a cancelled run")
		return nil, errors.New("ran")
	}, nil)

	h.repo.EXPECT().CreateNodeExecution(gomock.Any(), writesWorkspace, gomock.Any()).
		DoAndReturn(func(context.Context, string, *domain.NodeExecution) error {
			cancel()
			return context.Canceled
		})
	h.repo.EXPECT().UpdateNodeExecution(gomock.Any(), writesWorkspace, gomock.Any()).
		DoAndReturn(func(wctx context.Context, _ string, ne *domain.NodeExecution) error {
			live(t, wctx)
			assert.Equal(t, domain.NodeActionFailed, ne.Action)
			assert.NotNil(t, ne.CompletedAt)
			return nil
		})
	// No journey write: it stays on X, due when its claim ends.

	require.NoError(t, h.executor.Execute(ctx, writesWorkspace, h.ca))
	assert.Equal(t, 0, h.ca.RetryCount)
}

// A save that fails after a step a shutdown let finish is retried on a context the
// cancellation did not end, as the step's other writes are.
func TestAutomationExecutor_Execute_FailedSaveOnACancelledRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	y := "y"
	h := newWritesHarness(t, func(context.Context, NodeExecutionParams) (*NodeExecutionResult, error) {
		cancel() // the shutdown lands while the step runs; its work is done
		return &NodeExecutionResult{NextNodeID: &y, Status: domain.ContactAutomationStatusActive}, nil
	}, &y)

	h.expectProcessingRow(t, "x")
	gomock.InOrder(
		h.repo.EXPECT().UpdateContactAutomationIfActive(gomock.Any(), writesWorkspace, gomock.Any()).
			DoAndReturn(func(wctx context.Context, _ string, _ *domain.ContactAutomation) (bool, error) {
				live(t, wctx)
				return false, errors.New("db down")
			}),
		h.repo.EXPECT().UpdateNodeExecution(gomock.Any(), writesWorkspace, gomock.Any()).
			DoAndReturn(func(wctx context.Context, _ string, ne *domain.NodeExecution) error {
				live(t, wctx)
				assert.Equal(t, domain.NodeActionCompleted, ne.Action)
				return nil
			}),
		h.repo.EXPECT().UpdateContactAutomationIfActive(gomock.Any(), writesWorkspace, gomock.Any()).
			DoAndReturn(func(wctx context.Context, _ string, ca *domain.ContactAutomation) (bool, error) {
				live(t, wctx)
				assert.Equal(t, "y", *ca.CurrentNodeID)
				assert.Equal(t, 1, ca.RetryCount)
				return true, nil
			}),
	)

	require.NoError(t, h.executor.Execute(ctx, writesWorkspace, h.ca))
}
