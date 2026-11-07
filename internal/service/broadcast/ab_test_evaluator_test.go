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

// lockedCopy is the broadcast as the evaluator's locking read returns it: a row read
// again, not the pointer the evaluation started from, so a test fails if the update
// writes the stale copy.
func lockedCopy(b *domain.Broadcast) *domain.Broadcast {
	c := *b
	return &c
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
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpensTracked: 100, TotalClicksTracked: 100, TotalOpened: 40}, nil)
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpensTracked: 100, TotalClicksTracked: 100, TotalOpened: 30}, nil)

	// Transaction update/assert broadcast updated fields
	bcRepo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			return fn(nil)
		},
	)
	bcRepo.EXPECT().GetBroadcastForUpdateTx(ctx, gomock.Any(), workspaceID, broadcastID).Return(lockedCopy(b), nil)
	bcRepo.EXPECT().UpdateBroadcastTx(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *sql.Tx, updated *domain.Broadcast) error {
			assert.Equal(t, domain.BroadcastStatusWinnerSelected, updated.Status)
			assert.NotNil(t, updated.WinningTemplate)
			assert.Equal(t, "tplA", *updated.WinningTemplate)
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
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpensTracked: 100, TotalClicksTracked: 100, TotalClicked: 5}, nil)
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpensTracked: 100, TotalClicksTracked: 100, TotalClicked: 10}, nil)

	bcRepo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)
	bcRepo.EXPECT().GetBroadcastForUpdateTx(ctx, gomock.Any(), workspaceID, broadcastID).Return(lockedCopy(b), nil)
	bcRepo.EXPECT().UpdateBroadcastTx(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *sql.Tx, updated *domain.Broadcast) error {
			assert.Equal(t, domain.BroadcastStatusWinnerSelected, updated.Status)
			assert.NotNil(t, updated.WinningTemplate)
			assert.Equal(t, "tplB", *updated.WinningTemplate)
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

	// The metric is checked before any stats are read
	msgRepo.EXPECT().GetBroadcastVariationStats(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

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
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpensTracked: 100, TotalClicksTracked: 100, TotalOpened: 50}, nil)

	bcRepo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)
	bcRepo.EXPECT().GetBroadcastForUpdateTx(ctx, gomock.Any(), workspaceID, broadcastID).Return(lockedCopy(b), nil)
	bcRepo.EXPECT().UpdateBroadcastTx(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *sql.Tx, updated *domain.Broadcast) error {
			assert.NotNil(t, updated.WinningTemplate)
			assert.Equal(t, "tplB", *updated.WinningTemplate)
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

	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpensTracked: 100, TotalClicksTracked: 100, TotalOpened: 60}, nil)
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpensTracked: 100, TotalClicksTracked: 100, TotalOpened: 50}, nil)

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

	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpensTracked: 100, TotalClicksTracked: 100, TotalOpened: 60}, nil)
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpensTracked: 100, TotalClicksTracked: 100, TotalOpened: 50}, nil)

	bcRepo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)
	bcRepo.EXPECT().GetBroadcastForUpdateTx(ctx, gomock.Any(), workspaceID, broadcastID).Return(lockedCopy(b), nil)
	bcRepo.EXPECT().UpdateBroadcastTx(ctx, gomock.Any(), gomock.Any()).Return(errors.New("update failed"))

	_, err := evaluator.EvaluateAndSelectWinner(ctx, workspaceID, broadcastID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update broadcast with winner")
}

func TestABTestEvaluator_SelectBestVariation_UsesTrackedDenominators(t *testing.T) {
	ctx := context.Background()
	const workspaceID, broadcastID = "w1", "b1"

	t.Run("rate over tracked sends", func(t *testing.T) {
		ctrl, msgRepo, _, _, evaluator := setupEvaluator(t)
		defer ctrl.Finish()
		b := newTestBroadcast(workspaceID, broadcastID)
		b.TestSettings.AutoSendWinnerMetric = domain.TestWinnerMetricOpenRate

		// Over sends, A wins (30/100 vs 20/100). Over tracked sends, B wins (20/25 vs 30/100).
		msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpensTracked: 100, TotalOpened: 30}, nil)
		msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpensTracked: 25, TotalOpened: 20}, nil)

		winner, err := evaluator.selectBestVariation(ctx, workspaceID, b)
		require.NoError(t, err)
		assert.Equal(t, "tplB", winner)
	})

	t.Run("a variation without tracked emails is not a candidate", func(t *testing.T) {
		ctrl, msgRepo, _, _, evaluator := setupEvaluator(t)
		defer ctrl.Finish()
		b := newTestBroadcast(workspaceID, broadcastID)
		b.TestSettings.AutoSendWinnerMetric = domain.TestWinnerMetricClickRate

		msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100}, nil)
		msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalClicksTracked: 50}, nil)

		winner, err := evaluator.selectBestVariation(ctx, workspaceID, b)
		require.NoError(t, err)
		assert.Equal(t, "tplB", winner, "B scores 0 but is the only candidate")
	})

	// Opens and clicks come from every send, and delivery is known only from
	// provider webhooks, which SMTP and some providers never send and others
	// send late: the base is the tracked sends, as in the test results.
	t.Run("rate over tracked sends, with or without delivery data", func(t *testing.T) {
		ctrl, msgRepo, _, _, evaluator := setupEvaluator(t)
		defer ctrl.Finish()
		b := newTestBroadcast(workspaceID, broadcastID)
		b.TestSettings.AutoSendWinnerMetric = domain.TestWinnerMetricClickRate

		// A has no delivery webhook yet; over its sends it leads (90/100 vs 5/100).
		msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalSent: 100, TotalClicksTracked: 100, TotalClicked: 90}, nil)
		msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalSent: 100, TotalClicksTracked: 100, TotalDelivered: 50, TotalClicked: 5}, nil)

		winner, err := evaluator.selectBestVariation(ctx, workspaceID, b)
		require.NoError(t, err)
		assert.Equal(t, "tplA", winner)
	})

	t.Run("no tracked emails anywhere: no winner", func(t *testing.T) {
		ctrl, msgRepo, _, _, evaluator := setupEvaluator(t)
		defer ctrl.Finish()
		b := newTestBroadcast(workspaceID, broadcastID)
		b.TestSettings.AutoSendWinnerMetric = domain.TestWinnerMetricOpenRate

		msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpened: 3}, nil)
		msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100}, nil)

		_, err := evaluator.selectBestVariation(ctx, workspaceID, b)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no winner could be determined")
	})
}

// The owner can pick the winner of a completed test while its automatic
// evaluation runs: the evaluation reads the broadcast again inside its
// transaction and keeps the owner's choice rather than writing over it.
func TestABTestEvaluator_EvaluateAndSelectWinner_KeepsAWinnerChosenMeanwhile(t *testing.T) {
	ctrl, msgRepo, bcRepo, _, evaluator := setupEvaluator(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	b := newTestBroadcast(workspaceID, broadcastID)
	bcRepo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	// A wins on the numbers.
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpensTracked: 100, TotalOpened: 60}, nil)
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpensTracked: 100, TotalOpened: 50}, nil)

	bcRepo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)
	// Meanwhile the owner picked B.
	chosen := *b
	ownerPick := "tplB"
	chosen.Status = domain.BroadcastStatusWinnerSelected
	chosen.WinningTemplate = &ownerPick
	bcRepo.EXPECT().GetBroadcastForUpdateTx(ctx, gomock.Any(), workspaceID, broadcastID).Return(&chosen, nil)
	bcRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	winner, err := evaluator.EvaluateAndSelectWinner(ctx, workspaceID, broadcastID)
	require.NoError(t, err)
	assert.Equal(t, "tplB", winner)
}

func TestABTestEvaluator_EvaluateAndSelectWinner_RereadError(t *testing.T) {
	ctrl, msgRepo, bcRepo, _, evaluator := setupEvaluator(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	b := newTestBroadcast(workspaceID, broadcastID)
	bcRepo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalOpensTracked: 100, TotalOpened: 60}, nil)
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalOpensTracked: 100, TotalOpened: 50}, nil)
	bcRepo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)
	bcRepo.EXPECT().GetBroadcastForUpdateTx(ctx, gomock.Any(), workspaceID, broadcastID).Return(nil, errors.New("gone"))
	bcRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	_, err := evaluator.EvaluateAndSelectWinner(ctx, workspaceID, broadcastID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update broadcast with winner")
}

// A broadcast that left test_completed without a winner during the evaluation
// (paused or cancelled by its owner) is not given one.
func TestABTestEvaluator_EvaluateAndSelectWinner_LeftTestCompletedMeanwhile(t *testing.T) {
	for _, status := range []domain.BroadcastStatus{domain.BroadcastStatusPaused, domain.BroadcastStatusCancelled} {
		t.Run(string(status), func(t *testing.T) {
			ctrl, msgRepo, bcRepo, _, evaluator := setupEvaluator(t)
			defer ctrl.Finish()

			ctx := context.Background()
			workspaceID := "w1"
			broadcastID := "b1"

			b := newTestBroadcast(workspaceID, broadcastID)
			bcRepo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)
			msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalOpensTracked: 100, TotalOpened: 60}, nil)
			msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalOpensTracked: 100, TotalOpened: 50}, nil)
			bcRepo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
				func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
			)
			moved := lockedCopy(b)
			moved.Status = status
			// A winner recorded on a broadcast that is no longer waiting for one is not
			// a winner to send: cancelling keeps the column as it was.
			stale := "tplB"
			moved.WinningTemplate = &stale
			bcRepo.EXPECT().GetBroadcastForUpdateTx(ctx, gomock.Any(), workspaceID, broadcastID).Return(moved, nil)
			bcRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			_, err := evaluator.EvaluateAndSelectWinner(ctx, workspaceID, broadcastID)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "left the test completed state")
		})
	}
}

// The update writes the row read under the lock, not the copy the evaluation
// started from.
func TestABTestEvaluator_EvaluateAndSelectWinner_WritesTheLockedRow(t *testing.T) {
	ctrl, msgRepo, bcRepo, _, evaluator := setupEvaluator(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	b := newTestBroadcast(workspaceID, broadcastID)
	bcRepo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalOpensTracked: 100, TotalOpened: 60}, nil)
	msgRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalOpensTracked: 100, TotalOpened: 50}, nil)
	bcRepo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)
	locked := lockedCopy(b)
	locked.Name = "Renamed meanwhile"
	bcRepo.EXPECT().GetBroadcastForUpdateTx(ctx, gomock.Any(), workspaceID, broadcastID).Return(locked, nil)
	bcRepo.EXPECT().UpdateBroadcastTx(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *sql.Tx, updated *domain.Broadcast) error {
			assert.Same(t, locked, updated)
			assert.Equal(t, "Renamed meanwhile", updated.Name)
			require.NotNil(t, updated.WinningTemplate)
			assert.Equal(t, "tplA", *updated.WinningTemplate)
			assert.Equal(t, domain.BroadcastStatusWinnerSelected, updated.Status)
			return nil
		},
	)

	winner, err := evaluator.EvaluateAndSelectWinner(ctx, workspaceID, broadcastID)
	require.NoError(t, err)
	assert.Equal(t, "tplA", winner)
}
