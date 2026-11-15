package broadcast

import "github.com/Notifuse/notifuse/internal/domain"

// ParkPausedTaskForTest gives the external tests the requeue a run returns when it finds its
// broadcast paused.
func ParkPausedTaskForTest(o BroadcastOrchestratorInterface, task *domain.Task, broadcastID string) error {
	return o.(*BroadcastOrchestrator).parkPausedTask(task, broadcastID)
}
