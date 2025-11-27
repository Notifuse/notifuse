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
	err = e.updateBroadcastWithWinner(ctx, workspaceID, broadcast, winnerTemplateID)
	if err != nil {
		return "", fmt.Errorf("failed to update broadcast with winner: %w", err)
	}

	return winnerTemplateID, nil
}

func (e *ABTestEvaluator) selectBestVariation(ctx context.Context, workspaceID string, broadcast *domain.Broadcast) (string, error) {
	bestTemplateID := ""
	bestScore := -1.0

	for _, variation := range broadcast.TestSettings.Variations {
		stats, err := e.messageHistoryRepo.GetBroadcastVariationStats(ctx, workspaceID, broadcast.ID, variation.TemplateID)
		if err != nil {
			e.logger.WithFields(map[string]interface{}{
				"template_id": variation.TemplateID,
				"error":       err.Error(),
			}).Warn("Failed to get variation stats")
			continue
		}

		var score float64
		if stats.TotalDelivered > 0 {
			switch broadcast.TestSettings.AutoSendWinnerMetric {
			case domain.TestWinnerMetricOpenRate:
				score = float64(stats.TotalOpened) / float64(stats.TotalDelivered)
			case domain.TestWinnerMetricClickRate:
				score = float64(stats.TotalClicked) / float64(stats.TotalDelivered)
			default:
				return "", fmt.Errorf("invalid winner metric: %s", broadcast.TestSettings.AutoSendWinnerMetric)
			}
		}

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

func (e *ABTestEvaluator) updateBroadcastWithWinner(ctx context.Context, workspaceID string, broadcast *domain.Broadcast, winnerTemplateID string) error {
	return e.broadcastRepo.WithTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		// Update broadcast
		broadcast.WinningTemplate = &winnerTemplateID
		broadcast.Status = domain.BroadcastStatusWinnerSelected
		broadcast.UpdatedAt = time.Now().UTC()

		return e.broadcastRepo.UpdateBroadcastTx(ctx, tx, broadcast)
	})
}
