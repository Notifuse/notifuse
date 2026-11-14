package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shutdownHarness runs journeys through fake nodes and records the journey writes that land. Like
// the database, its journey write refuses a context that is done, so a write made on the
// shutdown context after the stop is lost, as it is in production.
type shutdownHarness struct {
	repo     *mocks.MockAutomationRepository
	executor *AutomationExecutor

	mu             sync.Mutex
	writes         []domain.ContactAutomation
	nodeExecutions []domain.NodeExecution
}

const (
	shutdownTestWorkspace = "ws1"
	shutdownTestNodeA     = "node-a"
	shutdownTestNodeB     = "node-b"
)

// newShutdownHarness builds an executor whose live automation runs node A (a webhook-typed fake)
// then node B (an add-to-list-typed fake).
func newShutdownHarness(t *testing.T, nodeA, nodeB func(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error)) *shutdownHarness {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockAutomationRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)
	timelineRepo := mocks.NewMockContactTimelineRepository(ctrl)

	nodeBID := shutdownTestNodeB
	automation := &domain.Automation{
		ID:     "auto1",
		Status: domain.AutomationStatusLive,
		Nodes: []*domain.AutomationNode{
			{ID: shutdownTestNodeA, Type: domain.NodeTypeWebhook, NextNodeID: &nodeBID},
			{ID: shutdownTestNodeB, Type: domain.NodeTypeAddToList},
		},
	}
	if nodeB == nil {
		nodeB = func(context.Context, NodeExecutionParams) (*NodeExecutionResult, error) {
			t.Error("node B ran")
			return &NodeExecutionResult{Status: domain.ContactAutomationStatusActive}, nil
		}
	}

	h := &shutdownHarness{repo: repo}
	h.executor = &AutomationExecutor{
		automationRepo: repo,
		contactRepo:    contactRepo,
		timelineRepo:   timelineRepo,
		nodeExecutors: map[domain.NodeType]NodeExecutor{
			domain.NodeTypeWebhook:   &testNodeExecutor{nodeType: domain.NodeTypeWebhook, execute: nodeA},
			domain.NodeTypeAddToList: &testNodeExecutor{nodeType: domain.NodeTypeAddToList, execute: nodeB},
		},
		logger: setupMockLogger(ctrl),
	}

	repo.EXPECT().GetByID(gomock.Any(), shutdownTestWorkspace, "auto1").Return(automation, nil).AnyTimes()
	contactRepo.EXPECT().GetContactByEmail(gomock.Any(), shutdownTestWorkspace, gomock.Any()).
		Return(&domain.Contact{Email: "a@example.com"}, nil).AnyTimes()
	repo.EXPECT().CreateNodeExecution(gomock.Any(), shutdownTestWorkspace, gomock.Any()).Return(nil).AnyTimes()
	repo.EXPECT().GetNodeExecutions(gomock.Any(), shutdownTestWorkspace, gomock.Any()).Return(nil, nil).AnyTimes()
	repo.EXPECT().IncrementAutomationStat(gomock.Any(), shutdownTestWorkspace, "auto1", gomock.Any()).Return(nil).AnyTimes()
	timelineRepo.EXPECT().Create(gomock.Any(), shutdownTestWorkspace, gomock.Any()).Return(nil).AnyTimes()
	repo.EXPECT().UpdateNodeExecution(gomock.Any(), shutdownTestWorkspace, gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, ne *domain.NodeExecution) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			h.nodeExecutions = append(h.nodeExecutions, *ne)
			return nil
		}).AnyTimes()
	repo.EXPECT().UpdateContactAutomationIfActive(gomock.Any(), shutdownTestWorkspace, gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, ca *domain.ContactAutomation) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			h.writes = append(h.writes, *ca)
			return true, nil
		}).AnyTimes()
	return h
}

// claim makes the next claim return one journey per ID, each on node A with a claim token, and
// each renewal of them succeed.
func (h *shutdownHarness) claim(ids ...string) []*domain.ContactAutomationWithWorkspace {
	journeys := make([]*domain.ContactAutomationWithWorkspace, len(ids))
	for i, id := range ids {
		nodeID := shutdownTestNodeA
		token := time.Now().UTC().Add(inTickResumeLease)
		journeys[i] = &domain.ContactAutomationWithWorkspace{WorkspaceID: shutdownTestWorkspace, ContactAutomation: domain.ContactAutomation{
			ID: id, AutomationID: "auto1", ContactEmail: id + "@example.com", CurrentNodeID: &nodeID,
			Status: domain.ContactAutomationStatusActive, ScheduledAt: &token, MaxRetries: 3,
		}}
	}
	h.repo.EXPECT().ClaimScheduledContactAutomationsGlobal(gomock.Any(), 50, inTickResumeLease).Return(journeys, nil)
	return journeys
}

func (h *shutdownHarness) expectRenewal(journey *domain.ContactAutomationWithWorkspace) *gomock.Call {
	return h.repo.EXPECT().RenewContactAutomationClaim(gomock.Any(), shutdownTestWorkspace, journey.ID, *journey.ScheduledAt, inTickResumeLease).Return(true, nil)
}

func (h *shutdownHarness) landedWrites() []domain.ContactAutomation {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]domain.ContactAutomation(nil), h.writes...)
}

func (h *shutdownHarness) landedNodeExecutions() []domain.NodeExecution {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]domain.NodeExecution(nil), h.nodeExecutions...)
}

// A normal stop (the app cancels the scheduler's context) must not cut a node whose effect is under
// way: the node finishes, the journey write that moves past it lands, and nothing new starts, so
// the node does not run again once the claim ends.
func TestAutomationExecutor_ProcessBatch_Shutdown(t *testing.T) {
	t.Run("a stop during a node lets the node finish and its write land", func(t *testing.T) {
		started, stopped := make(chan struct{}), make(chan struct{})
		h := newShutdownHarness(t, func(ctx context.Context, _ NodeExecutionParams) (*NodeExecutionResult, error) {
			close(started)
			<-stopped
			assert.NoError(t, ctx.Err(), "the node's context outlives the stop")
			return &NodeExecutionResult{Status: domain.ContactAutomationStatusCompleted}, nil
		}, nil)
		journeys := h.claim("ca-1")
		h.expectRenewal(journeys[0])

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			<-started
			cancel()
			close(stopped)
		}()
		processed, err := h.executor.ProcessBatch(ctx, 50)
		require.NoError(t, err)
		assert.Equal(t, 1, processed)

		writes := h.landedWrites()
		require.Len(t, writes, 1, "the journey write after the node")
		assert.Equal(t, domain.ContactAutomationStatusCompleted, writes[0].Status)
		executions := h.landedNodeExecutions()
		require.Len(t, executions, 1)
		assert.Equal(t, domain.NodeActionCompleted, executions[0].Action)
	})

	t.Run("no new node starts after the stop, and the journey is due again at once", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		nodeB := shutdownTestNodeB
		h := newShutdownHarness(t, func(context.Context, NodeExecutionParams) (*NodeExecutionResult, error) {
			cancel()
			return &NodeExecutionResult{NextNodeID: &nodeB, Status: domain.ContactAutomationStatusActive}, nil
		}, nil) // node B fails the test if it runs
		journeys := h.claim("ca-1")
		h.expectRenewal(journeys[0])

		processed, err := h.executor.ProcessBatch(ctx, 50)
		require.NoError(t, err)
		assert.Equal(t, 1, processed)

		writes := h.landedWrites()
		require.Len(t, writes, 1)
		require.NotNil(t, writes[0].CurrentNodeID)
		assert.Equal(t, shutdownTestNodeB, *writes[0].CurrentNodeID, "the journey moved past node A")
		assert.Equal(t, domain.ContactAutomationStatusActive, writes[0].Status)
		require.NotNil(t, writes[0].ScheduledAt)
		assert.WithinDuration(t, time.Now(), *writes[0].ScheduledAt, 5*time.Second,
			"handed back due now, not held by the in-tick lease")
	})

	t.Run("no new journey starts after the stop", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var runs []string
		h := newShutdownHarness(t, func(_ context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
			runs = append(runs, params.Contact.ID)
			cancel()
			return &NodeExecutionResult{Status: domain.ContactAutomationStatusCompleted}, nil
		}, nil)
		journeys := h.claim("ca-1", "ca-2")
		h.expectRenewal(journeys[0])
		// No renewal of ca-2: it keeps its claim and is due again when it ends.

		processed, err := h.executor.ProcessBatch(ctx, 50)
		require.NoError(t, err)
		assert.Equal(t, 1, processed)
		assert.Equal(t, []string{"ca-1"}, runs)
	})

	t.Run("the drain ends at its deadline, counted from the stop", func(t *testing.T) {
		const drain = 200 * time.Millisecond
		started := make(chan struct{})
		var nodeDoneAt time.Time
		h := newShutdownHarness(t, func(ctx context.Context, _ NodeExecutionParams) (*NodeExecutionResult, error) {
			close(started)
			<-ctx.Done()
			nodeDoneAt = time.Now()
			return nil, ctx.Err()
		}, nil)
		h.executor.shutdownDrain = drain
		journeys := h.claim("ca-1")
		h.expectRenewal(journeys[0])

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var cancelledAt time.Time
		go func() {
			<-started
			cancelledAt = time.Now()
			cancel()
		}()
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = h.executor.ProcessBatch(ctx, 50)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("ProcessBatch did not return after the drain deadline")
		}
		assert.GreaterOrEqual(t, nodeDoneAt.Sub(cancelledAt), drain, "the node keeps its context for the drain")
		assert.Empty(t, h.landedWrites(), "a node cut by the deadline records nothing")
	})

	t.Run("no deadline while the app runs", func(t *testing.T) {
		h := newShutdownHarness(t, func(ctx context.Context, _ NodeExecutionParams) (*NodeExecutionResult, error) {
			time.Sleep(300 * time.Millisecond)
			assert.NoError(t, ctx.Err(), "a slow node is not cut without a stop")
			return &NodeExecutionResult{Status: domain.ContactAutomationStatusCompleted}, nil
		}, nil)
		h.executor.shutdownDrain = 50 * time.Millisecond
		journeys := h.claim("ca-1")
		h.expectRenewal(journeys[0])

		processed, err := h.executor.ProcessBatch(context.Background(), 50)
		require.NoError(t, err)
		assert.Equal(t, 1, processed)
		require.Len(t, h.landedWrites(), 1)
	})
}

// Execute called directly (not through ProcessBatch) has no stop signal and behaves as before:
// it runs node after node.
func TestAutomationExecutor_Execute_WithoutStopSignalRunsEveryNode(t *testing.T) {
	nodeB := shutdownTestNodeB
	var ranB bool
	h := newShutdownHarness(t, func(context.Context, NodeExecutionParams) (*NodeExecutionResult, error) {
		return &NodeExecutionResult{NextNodeID: &nodeB, Status: domain.ContactAutomationStatusActive}, nil
	}, func(context.Context, NodeExecutionParams) (*NodeExecutionResult, error) {
		ranB = true
		return &NodeExecutionResult{Status: domain.ContactAutomationStatusCompleted}, nil
	})
	nodeA := shutdownTestNodeA
	ca := &domain.ContactAutomation{ID: "ca-1", AutomationID: "auto1", ContactEmail: "a@example.com",
		CurrentNodeID: &nodeA, Status: domain.ContactAutomationStatusActive, MaxRetries: 3}

	require.NoError(t, h.executor.Execute(context.Background(), shutdownTestWorkspace, ca))
	assert.True(t, ranB)
	assert.Equal(t, domain.ContactAutomationStatusCompleted, ca.Status)
}
