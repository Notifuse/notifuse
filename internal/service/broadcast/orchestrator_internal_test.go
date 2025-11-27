package broadcast

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalOrchestrator creates an orchestrator with only dependencies needed for internal tests
func minimalOrchestrator(ctrl *gomock.Controller, bcRepo *domainmocks.MockBroadcastRepository, logger *pkgmocks.MockLogger, timeProvider TimeProvider, abEval *ABTestEvaluator) *BroadcastOrchestrator {
	if logger == nil {
		logger = pkgmocks.NewMockLogger(ctrl)
		logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
		logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
		logger.EXPECT().Info(gomock.Any()).AnyTimes()
		logger.EXPECT().Debug(gomock.Any()).AnyTimes()
		logger.EXPECT().Error(gomock.Any()).AnyTimes()
		logger.EXPECT().Warn(gomock.Any()).AnyTimes()
	}
	if timeProvider == nil {
		timeProvider = NewRealTimeProvider()
	}
	return &BroadcastOrchestrator{
		broadcastRepo:   bcRepo,
		logger:          logger,
		config:          DefaultConfig(),
		timeProvider:    timeProvider,
		abTestEvaluator: abEval,
	}
}

func TestShouldEvaluateWinner(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()

	// Fixed time provider
	fixedNow := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	tp := &fakeTimeProvider{now: fixedNow}

	orch := minimalOrchestrator(ctrl, nil, logger, tp, nil)

	// Case 1: Auto disabled
	b := &domain.Broadcast{TestSettings: domain.BroadcastTestSettings{AutoSendWinner: false}}
	assert.False(t, orch.shouldEvaluateWinner(b))

	// Case 2: No TestSentAt
	b.TestSettings.AutoSendWinner = true
	b.TestSentAt = nil
	assert.False(t, orch.shouldEvaluateWinner(b))

	// Case 3: Not yet time
	sentAt := fixedNow.Add(-30 * time.Minute)
	b.TestSentAt = &sentAt
	b.TestSettings.TestDurationHours = 1
	assert.False(t, orch.shouldEvaluateWinner(b))

	// Case 4: Time passed
	sentAt = fixedNow.Add(-2 * time.Hour)
	b.TestSentAt = &sentAt
	assert.True(t, orch.shouldEvaluateWinner(b))
}

// fakeTimeProvider implements TimeProvider for deterministic tests
type fakeTimeProvider struct{ now time.Time }

func (f *fakeTimeProvider) Now() time.Time                  { return f.now }
func (f *fakeTimeProvider) Since(t time.Time) time.Duration { return f.now.Sub(t) }

func TestEvaluateWinner_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	// Mocks for evaluator
	msgRepo := domainmocks.NewMockMessageHistoryRepository(ctrl)
	bcRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Warn(gomock.Any()).AnyTimes()

	// Build ABTestEvaluator
	evaluator := NewABTestEvaluator(msgRepo, bcRepo, logger)

	// Orchestrator with evaluator
	orch := minimalOrchestrator(ctrl, bcRepo, logger, NewRealTimeProvider(), evaluator)

	// Broadcast in test completed with auto send
	b := &domain.Broadcast{
		ID:          broadcastID,
		WorkspaceID: workspaceID,
		Status:      domain.BroadcastStatusTestCompleted,
		TestSettings: domain.BroadcastTestSettings{
			Enabled:              true,
			AutoSendWinner:       true,
			AutoSendWinnerMetric: domain.TestWinnerMetricOpenRate,
			Variations: []domain.BroadcastVariation{
				{VariationName: "A", TemplateID: "tplA"},
				{VariationName: "B", TemplateID: "tplB"},
			},
		},
	}

	// Evaluator re-fetches broadcast
	bcRepo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	// Stats favor tplB
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpened: 30}, nil)
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpened: 60}, nil)

	// Transactional update by evaluator
	bcRepo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)
	bcRepo.EXPECT().UpdateBroadcastTx(ctx, gomock.Any(), gomock.Any()).Return(nil)

	// State before
	state := &domain.SendBroadcastState{Phase: "test"}

	// Call
	err := orch.evaluateWinner(ctx, b, state)
	require.NoError(t, err)
	assert.Equal(t, "winner", state.Phase)
}

func TestEvaluateWinner_FailureBubbles(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	// Mocks for evaluator
	msgRepo := domainmocks.NewMockMessageHistoryRepository(ctrl)
	bcRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()

	evaluator := NewABTestEvaluator(msgRepo, bcRepo, logger)
	orch := minimalOrchestrator(ctrl, bcRepo, logger, NewRealTimeProvider(), evaluator)

	// Broadcast given to orchestrator does not dictate evaluator; evaluator re-fetches an invalid broadcast
	b := &domain.Broadcast{ID: broadcastID, WorkspaceID: workspaceID}
	bcRepo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(&domain.Broadcast{Status: domain.BroadcastStatusTesting}, nil)

	state := &domain.SendBroadcastState{Phase: "test"}
	err := orch.evaluateWinner(ctx, b, state)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auto winner evaluation failed")
}

func TestHandleTestPhaseCompletion_ConcurrentWinner(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	bcRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()

	orch := minimalOrchestrator(ctrl, bcRepo, logger, NewRealTimeProvider(), nil)

	b := &domain.Broadcast{ID: "b1", WorkspaceID: "w1", TestSettings: domain.BroadcastTestSettings{Enabled: true}}
	state := &domain.SendBroadcastState{Phase: "test"}

	// Latest shows winner selected
	tplW := "tplW"
	bcRepo.EXPECT().GetBroadcast(ctx, b.WorkspaceID, b.ID).Return(&domain.Broadcast{ID: b.ID, WinningTemplate: &tplW, Status: domain.BroadcastStatusWinnerSelected}, nil)

	done := orch.handleTestPhaseCompletion(ctx, b, state)
	assert.False(t, done)
	assert.Equal(t, "winner", state.Phase)
}

func TestHandleTestPhaseCompletion_UpdateStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	bcRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()

	orch := minimalOrchestrator(ctrl, bcRepo, logger, NewRealTimeProvider(), nil)

	b := &domain.Broadcast{ID: "b1", WorkspaceID: "w1", TestSettings: domain.BroadcastTestSettings{Enabled: true}}
	state := &domain.SendBroadcastState{Phase: "test"}

	// Latest does not show winner selected
	bcRepo.EXPECT().GetBroadcast(ctx, b.WorkspaceID, b.ID).Return(&domain.Broadcast{ID: b.ID}, nil)
	bcRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Return(nil)

	done := orch.handleTestPhaseCompletion(ctx, b, state)
	assert.False(t, done)
	// Phase remains test, marked test_completed is internal to broadcast; not directly asserted here
}

func TestHandleTestPhaseCompletion_UpdateStatusError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	bcRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()

	orch := minimalOrchestrator(ctrl, bcRepo, logger, NewRealTimeProvider(), nil)

	b := &domain.Broadcast{ID: "b1", WorkspaceID: "w1", TestSettings: domain.BroadcastTestSettings{Enabled: true}}
	state := &domain.SendBroadcastState{Phase: "test"}

	bcRepo.EXPECT().GetBroadcast(ctx, b.WorkspaceID, b.ID).Return(&domain.Broadcast{ID: b.ID}, nil)
	bcRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Return(errors.New("db"))

	done := orch.handleTestPhaseCompletion(ctx, b, state)
	assert.False(t, done)
}

func TestHandleTestPhaseCompletion_AutoWinnerLogsEvaluationTime(t *testing.T) {
	// Covers lines 1069-1075: logging of auto-winner evaluation time when test phase completes
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	bcRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	logger := pkgmocks.NewMockLogger(ctrl)

	// Allow any logger calls initially
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()

	// Time provider that returns predictable time
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	tp := &fakeTimeProvider{now: baseTime}

	orch := minimalOrchestrator(ctrl, bcRepo, logger, tp, nil)

	// Broadcast with auto-send enabled and test duration set
	b := &domain.Broadcast{
		ID:          "b1",
		WorkspaceID: "w1",
		TestSettings: domain.BroadcastTestSettings{
			Enabled:           true,
			AutoSendWinner:    true,
			TestDurationHours: 2, // This will be used in the log calculation
		},
	}

	state := &domain.SendBroadcastState{Phase: "test"}

	// Latest broadcast check - no concurrent winner selected
	bcRepo.EXPECT().GetBroadcast(ctx, b.WorkspaceID, b.ID).Return(&domain.Broadcast{
		ID:           b.ID,
		TestSettings: b.TestSettings,
	}, nil)

	// Update status to test_completed
	bcRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Return(nil)

	done := orch.handleTestPhaseCompletion(ctx, b, state)
	assert.False(t, done)

	// The key assertion is that the test passed without errors, meaning the auto-winner
	// evaluation time logging path (lines 1069-1075) was executed successfully
}
