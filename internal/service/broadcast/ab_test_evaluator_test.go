package broadcast

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupEvaluator(t *testing.T) (*gomock.Controller, *domainmocks.MockMessageHistoryRepository, *domainmocks.MockBroadcastRepository, *pkgmocks.MockLogger, *ABTestEvaluator) {
	t.Helper()
	ctrl := gomock.NewController(t)
	msgRepo := domainmocks.NewMockMessageHistoryRepository(ctrl)
	bcRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	logger := pkgmocks.NewMockLogger(ctrl)

	// Allow structured logging in tests without brittle expectations
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Warn(gomock.Any()).AnyTimes()

	evaluator := NewABTestEvaluator(msgRepo, bcRepo, logger)
	return ctrl, msgRepo, bcRepo, logger, evaluator
}

func newTestBroadcast(workspaceID, broadcastID string) *domain.Broadcast {
	return &domain.Broadcast{
		ID:          broadcastID,
		WorkspaceID: workspaceID,
		Name:        "Test",
		ChannelType: "email",
		Status:      domain.BroadcastStatusTestCompleted,
		Audience:    domain.AudienceSettings{List: "list1"},
		Schedule:    domain.ScheduleSettings{IsScheduled: false},
		TestSettings: domain.BroadcastTestSettings{
			Enabled:              true,
			SamplePercentage:     50,
			AutoSendWinner:       true,
			AutoSendWinnerMetric: domain.TestWinnerMetricOpenRate,
			TestDurationHours:    24,
			Variations: []domain.BroadcastVariation{
				{VariationName: "A", TemplateID: "tplA"},
				{VariationName: "B", TemplateID: "tplB"},
			},
		},
	}
}

func TestABTestEvaluator_EvaluateAndSelectWinner_OpenRate_Success(t *testing.T) {
	ctrl, msgRepo, bcRepo, _, evaluator := setupEvaluator(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	b := newTestBroadcast(workspaceID, broadcastID)
	b.TestSettings.AutoSendWinnerMetric = domain.TestWinnerMetricOpenRate

	// Fetch broadcast
	bcRepo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	// Stats: A wins on open rate (0.40 vs 0.30)
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpened: 40}, nil)
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpened: 30}, nil)

	// Transaction update/assert broadcast updated fields
	bcRepo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			return fn(nil)
		},
	)
	bcRepo.EXPECT().UpdateBroadcastTx(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *sql.Tx, updated *domain.Broadcast) error {
			assert.Equal(t, domain.BroadcastStatusWinnerSelected, updated.Status)
			assert.Equal(t, "tplA", updated.WinningTemplate)
			assert.Equal(t, broadcastID, updated.ID)
			return nil
		},
	)

	winner, err := evaluator.EvaluateAndSelectWinner(ctx, workspaceID, broadcastID)
	require.NoError(t, err)
	assert.Equal(t, "tplA", winner)
}

func TestABTestEvaluator_EvaluateAndSelectWinner_ClickRate_Success(t *testing.T) {
	ctrl, msgRepo, bcRepo, _, evaluator := setupEvaluator(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	b := newTestBroadcast(workspaceID, broadcastID)
	b.TestSettings.AutoSendWinnerMetric = domain.TestWinnerMetricClickRate

	bcRepo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	// Stats: B wins on click rate (10/100 vs 5/100)
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalClicked: 5}, nil)
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalClicked: 10}, nil)

	bcRepo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)
	bcRepo.EXPECT().UpdateBroadcastTx(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *sql.Tx, updated *domain.Broadcast) error {
			assert.Equal(t, domain.BroadcastStatusWinnerSelected, updated.Status)
			assert.Equal(t, "tplB", updated.WinningTemplate)
			return nil
		},
	)

	winner, err := evaluator.EvaluateAndSelectWinner(ctx, workspaceID, broadcastID)
	require.NoError(t, err)
	assert.Equal(t, "tplB", winner)
}

func TestABTestEvaluator_EvaluateAndSelectWinner_GetBroadcastError(t *testing.T) {
	ctrl, _, bcRepo, _, evaluator := setupEvaluator(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	bcRepo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(nil, errors.New("db"))

	_, err := evaluator.EvaluateAndSelectWinner(ctx, workspaceID, broadcastID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get broadcast")
}

func TestABTestEvaluator_EvaluateAndSelectWinner_InvalidStatus(t *testing.T) {
	ctrl, _, bcRepo, _, evaluator := setupEvaluator(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	b := newTestBroadcast(workspaceID, broadcastID)
	b.Status = domain.BroadcastStatusTesting

	bcRepo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	_, err := evaluator.EvaluateAndSelectWinner(ctx, workspaceID, broadcastID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in test completed state")
}

func TestABTestEvaluator_EvaluateAndSelectWinner_AutoSendDisabled(t *testing.T) {
	ctrl, _, bcRepo, _, evaluator := setupEvaluator(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	b := newTestBroadcast(workspaceID, broadcastID)
	b.TestSettings.AutoSendWinner = false

	bcRepo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	_, err := evaluator.EvaluateAndSelectWinner(ctx, workspaceID, broadcastID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auto winner selection not enabled")
}

func TestABTestEvaluator_EvaluateAndSelectWinner_InvalidMetric(t *testing.T) {
	ctrl, msgRepo, bcRepo, _, evaluator := setupEvaluator(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	b := newTestBroadcast(workspaceID, broadcastID)
	// Force invalid metric
	b.TestSettings.AutoSendWinnerMetric = domain.TestWinnerMetric("invalid")

	bcRepo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	// Provide stats so selection reaches metric switch; evaluator will error on first variation
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpened: 40, TotalClicked: 5}, nil)

	_, err := evaluator.EvaluateAndSelectWinner(ctx, workspaceID, broadcastID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid winner metric")
}

func TestABTestEvaluator_EvaluateAndSelectWinner_NoWinnerWhenAllStatsFail(t *testing.T) {
	ctrl, msgRepo, bcRepo, _, evaluator := setupEvaluator(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	b := newTestBroadcast(workspaceID, broadcastID)

	bcRepo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(nil, errors.New("boom"))
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(nil, errors.New("boom"))

	_, err := evaluator.EvaluateAndSelectWinner(ctx, workspaceID, broadcastID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no winner could be determined")
}

func TestABTestEvaluator_EvaluateAndSelectWinner_StatsErrorContinue(t *testing.T) {
	ctrl, msgRepo, bcRepo, _, evaluator := setupEvaluator(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	b := newTestBroadcast(workspaceID, broadcastID)
	b.TestSettings.AutoSendWinnerMetric = domain.TestWinnerMetricOpenRate

	bcRepo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	// A fails, B succeeds; should still pick B
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(nil, errors.New("stats error"))
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpened: 50}, nil)

	bcRepo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)
	bcRepo.EXPECT().UpdateBroadcastTx(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *sql.Tx, updated *domain.Broadcast) error {
			assert.Equal(t, "tplB", updated.WinningTemplate)
			return nil
		},
	)

	winner, err := evaluator.EvaluateAndSelectWinner(ctx, workspaceID, broadcastID)
	require.NoError(t, err)
	assert.Equal(t, "tplB", winner)
}

func TestABTestEvaluator_EvaluateAndSelectWinner_WithTransactionError(t *testing.T) {
	ctrl, msgRepo, bcRepo, _, evaluator := setupEvaluator(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	b := newTestBroadcast(workspaceID, broadcastID)

	bcRepo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpened: 60}, nil)
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpened: 50}, nil)

	bcRepo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).Return(errors.New("tx failed"))

	_, err := evaluator.EvaluateAndSelectWinner(ctx, workspaceID, broadcastID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update broadcast with winner")
}

func TestABTestEvaluator_EvaluateAndSelectWinner_UpdateBroadcastError(t *testing.T) {
	ctrl, msgRepo, bcRepo, _, evaluator := setupEvaluator(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	b := newTestBroadcast(workspaceID, broadcastID)

	bcRepo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpened: 60}, nil)
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpened: 50}, nil)

	bcRepo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)
	bcRepo.EXPECT().UpdateBroadcastTx(ctx, gomock.Any(), gomock.Any()).Return(errors.New("update failed"))

	_, err := evaluator.EvaluateAndSelectWinner(ctx, workspaceID, broadcastID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update broadcast with winner")
}
