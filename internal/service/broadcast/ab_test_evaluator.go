package broadcast

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type ABTestEvaluator struct {
	messageHistoryRepo domain.MessageHistoryRepository
	broadcastRepo      domain.BroadcastRepository
	logger             logger.Logger
}

func NewABTestEvaluator(
	messageHistoryRepo domain.MessageHistoryRepository,
	broadcastRepo domain.BroadcastRepository,
	logger logger.Logger,
) *ABTestEvaluator {
	return &ABTestEvaluator{
		messageHistoryRepo: messageHistoryRepo,
		broadcastRepo:      broadcastRepo,
		logger:             logger,
	}
}

func (e *ABTestEvaluator) EvaluateAndSelectWinner(ctx context.Context, workspaceID, broadcastID string) (string, error) {
	// Get broadcast
	broadcast, err := e.broadcastRepo.GetBroadcast(ctx, workspaceID, broadcastID)
	if err != nil {
		return "", fmt.Errorf("failed to get broadcast: %w", err)
	}

	// Validate broadcast state
	if broadcast.Status != domain.BroadcastStatusTestCompleted {
		return "", fmt.Errorf("broadcast is not in test completed state")
	}

	if !broadcast.TestSettings.AutoSendWinner {
		return "", fmt.Errorf("auto winner selection not enabled for broadcast")
	}

	// Evaluate variations and select winner
	winnerTemplateID, err := e.selectBestVariation(ctx, workspaceID, broadcast)
	if err != nil {
		return "", fmt.Errorf("failed to select winner: %w", err)
	}

	// Update broadcast with winner
	winnerTemplateID, err = e.updateBroadcastWithWinner(ctx, workspaceID, broadcastID, winnerTemplateID)
	if err != nil {
		return "", fmt.Errorf("failed to update broadcast with winner: %w", err)
	}

	return winnerTemplateID, nil
}

func (e *ABTestEvaluator) selectBestVariation(ctx context.Context, workspaceID string, broadcast *domain.Broadcast) (string, error) {
	bestTemplateID := ""
	bestScore := -1.0

	// The rate is measured over the sends that could record the metric: an email
	// sent without the pixel (or the redirects) cannot open (or click), whatever
	// its recipient did. Sends, not deliveries: opens and clicks come from every
	// send, and delivery is known only from provider webhooks, which SMTP never
	// sends and other providers send late. The test results use the same base.
	var rate func(stats *domain.MessageHistoryStatusSum) (hits, tracked int)
	switch broadcast.TestSettings.AutoSendWinnerMetric {
	case domain.TestWinnerMetricOpenRate:
		rate = func(stats *domain.MessageHistoryStatusSum) (int, int) {
			return stats.TotalOpened, stats.TotalOpensTracked
		}
	case domain.TestWinnerMetricClickRate:
		rate = func(stats *domain.MessageHistoryStatusSum) (int, int) {
			return stats.TotalClicked, stats.TotalClicksTracked
		}
	default:
		return "", fmt.Errorf("invalid winner metric: %s", broadcast.TestSettings.AutoSendWinnerMetric)
	}

	for _, variation := range broadcast.TestSettings.Variations {
		stats, err := e.messageHistoryRepo.GetBroadcastVariationStats(ctx, workspaceID, broadcast.ID, variation.TemplateID)
		if err != nil {
			e.logger.WithFields(map[string]interface{}{
				"template_id": variation.TemplateID,
				"error":       err.Error(),
			}).Warn("Failed to get variation stats")
			continue
		}

		// A variation with no tracked send is not a candidate, rather than a
		// score of 0 that would still beat -1.
		hits, tracked := rate(stats)
		if tracked == 0 {
			e.logger.WithFields(map[string]interface{}{
				"template_id": variation.TemplateID,
				"metric":      broadcast.TestSettings.AutoSendWinnerMetric,
			}).Info("Variation has no tracked email, not a candidate")
			continue
		}
		score := float64(hits) / float64(tracked)

		if score > bestScore {
			bestScore = score
			bestTemplateID = variation.TemplateID
		}

		e.logger.WithFields(map[string]interface{}{
			"template_id": variation.TemplateID,
			"metric":      broadcast.TestSettings.AutoSendWinnerMetric,
			"score":       score,
			"is_best":     score == bestScore,
		}).Info("Variation evaluation result")
	}

	if bestTemplateID == "" {
		return "", fmt.Errorf("no winner could be determined")
	}

	e.logger.WithFields(map[string]interface{}{
		"broadcast_id":    broadcast.ID,
		"winner_template": bestTemplateID,
		"winning_score":   bestScore,
	}).Info("Auto winner selected")

	return bestTemplateID, nil
}

// updateBroadcastWithWinner records the evaluated winner and returns the winner the
// broadcast ends up with. The owner can pick a completed test's winner while the
// evaluation runs, so the broadcast is read again with its row locked, as
// SelectWinner reads it: the two take turns, and a winner the owner chose first is
// kept rather than written over.
func (e *ABTestEvaluator) updateBroadcastWithWinner(ctx context.Context, workspaceID, broadcastID, winnerTemplateID string) (string, error) {
	winner := winnerTemplateID
	err := e.broadcastRepo.WithTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		broadcast, err := e.broadcastRepo.GetBroadcastForUpdateTx(ctx, tx, workspaceID, broadcastID)
		if err != nil {
			return err
		}
		switch {
		case broadcast.Status == domain.BroadcastStatusWinnerSelected && broadcast.WinningTemplate != nil:
			winner = *broadcast.WinningTemplate
			e.logger.WithFields(map[string]interface{}{
				"broadcast_id":     broadcastID,
				"evaluated_winner": winnerTemplateID,
				"winning_template": winner,
			}).Info("A winner was chosen during the automatic evaluation; keeping it")
			return nil
		case broadcast.Status != domain.BroadcastStatusTestCompleted:
			return fmt.Errorf("broadcast left the test completed state (%s) during evaluation", broadcast.Status)
		}

		broadcast.WinningTemplate = &winnerTemplateID
		broadcast.Status = domain.BroadcastStatusWinnerSelected
		broadcast.UpdatedAt = time.Now().UTC()

		return e.broadcastRepo.UpdateBroadcastTx(ctx, tx, broadcast)
	})
	if err != nil {
		return "", err
	}
	return winner, nil
}
