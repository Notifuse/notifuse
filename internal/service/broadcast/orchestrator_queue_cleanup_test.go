package broadcast

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// Pause/cancel cleanup is best-effort: it must call the queue repo with the
// broadcast as its source, and it must never turn a queue hiccup into an
// orchestration failure (the functions return nothing on purpose). It must
// also stay nil-safe, since some orchestrator constructions have no queue.

func TestOrchestrator_PauseQueueEntries(t *testing.T) {
	ctx := context.Background()

	t.Run("nil queue repo is a silent no-op", func(t *testing.T) {
		o := &BroadcastOrchestrator{logger: logger.NewLoggerWithLevel("disabled")}
		assert.NotPanics(t, func() { o.pauseQueueEntries(ctx, "ws-1", "b-1") })
	})

	t.Run("pauses by broadcast source identity", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		queueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		queueRepo.EXPECT().PauseBySource(ctx, "ws-1", domain.EmailQueueSourceBroadcast, "b-1").
			Return(int64(4), nil).Times(1)
		o := &BroadcastOrchestrator{
			logger:         logger.NewLoggerWithLevel("disabled"),
			emailQueueRepo: queueRepo,
		}
		o.pauseQueueEntries(ctx, "ws-1", "b-1")
	})

	t.Run("repo failure is swallowed, never propagated as a panic", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		queueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		queueRepo.EXPECT().PauseBySource(ctx, "ws-1", domain.EmailQueueSourceBroadcast, "b-1").
			Return(int64(0), errors.New("queue locked")).Times(1)
		o := &BroadcastOrchestrator{
			logger:         logger.NewLoggerWithLevel("disabled"),
			emailQueueRepo: queueRepo,
		}
		assert.NotPanics(t, func() { o.pauseQueueEntries(ctx, "ws-1", "b-1") })
	})

	t.Run("zero paused rows still issues the call", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		queueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		queueRepo.EXPECT().PauseBySource(ctx, "ws-1", domain.EmailQueueSourceBroadcast, "b-1").
			Return(int64(0), nil).Times(1)
		o := &BroadcastOrchestrator{
			logger:         logger.NewLoggerWithLevel("disabled"),
			emailQueueRepo: queueRepo,
		}
		o.pauseQueueEntries(ctx, "ws-1", "b-1")
	})
}

func TestOrchestrator_DeleteQueueEntries(t *testing.T) {
	ctx := context.Background()

	t.Run("nil queue repo is a silent no-op", func(t *testing.T) {
		o := &BroadcastOrchestrator{logger: logger.NewLoggerWithLevel("disabled")}
		assert.NotPanics(t, func() { o.deleteQueueEntries(ctx, "ws-1", "b-1") })
	})

	t.Run("deletes by broadcast source identity", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		queueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		queueRepo.EXPECT().DeleteBySource(ctx, "ws-1", domain.EmailQueueSourceBroadcast, "b-1").
			Return(int64(7), nil).Times(1)
		o := &BroadcastOrchestrator{
			logger:         logger.NewLoggerWithLevel("disabled"),
			emailQueueRepo: queueRepo,
		}
		o.deleteQueueEntries(ctx, "ws-1", "b-1")
	})

	t.Run("repo failure is swallowed, never propagated as a panic", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		queueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		queueRepo.EXPECT().DeleteBySource(ctx, "ws-1", domain.EmailQueueSourceBroadcast, "b-1").
			Return(int64(0), errors.New("queue locked")).Times(1)
		o := &BroadcastOrchestrator{
			logger:         logger.NewLoggerWithLevel("disabled"),
			emailQueueRepo: queueRepo,
		}
		assert.NotPanics(t, func() { o.deleteQueueEntries(ctx, "ws-1", "b-1") })
	})
}

func TestOrchestrator_CanProcess(t *testing.T) {
	o := &BroadcastOrchestrator{}
	assert.True(t, o.CanProcess("send_broadcast"))
	assert.False(t, o.CanProcess("send_transactional"))
	assert.False(t, o.CanProcess(""))
}
