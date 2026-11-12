package broadcast_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The start date is stamped by a sending run while none is stored, the first one being the
// run that announces the start, so a scheduled broadcast gets one too: until now only "send
// now" and a resume set it.
// These tests reuse the sending-started mocks: every gate before the stamp is cleared and
// the run is cut short at the first recipient batch, the step right after it.

func firstSendingRunTask() *domain.Task {
	return sendingStartedTask(&domain.SendBroadcastState{
		BroadcastID:     "broadcast-123",
		TotalRecipients: 25,
		RecipientOffset: 0,
		EnqueuedCount:   0,
	})
}

func TestOrchestrator_Process_StampsStartedAtOnFirstSendingRun(t *testing.T) {
	m := newSendingStartedMocks(t)
	defer m.ctrl.Finish()

	m.expectConfiguredWorkspace()
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		Return(sendingStartedBroadcast("Summer sale"), nil).AnyTimes()
	m.expectValidTemplate()
	m.expectFailingRecipientFetch()
	m.eventBus.EXPECT().Publish(gomock.Any(), eventOfType(domain.EventBroadcastSendingStarted)).Times(1)
	m.broadcastRepo.EXPECT().
		MarkBroadcastStarted(gomock.Any(), "workspace-123", "broadcast-123", sendingStartedTime).
		Return(nil).Times(1)

	_, err := m.orchestrator(m.eventBus).Process(context.Background(), firstSendingRunTask(), time.Now().Add(30*time.Second))
	require.Error(t, err)
}

// The stamp does not depend on the event bus: without one the date is still recorded.
func TestOrchestrator_Process_StampsStartedAtWithoutEventBus(t *testing.T) {
	m := newSendingStartedMocks(t)
	defer m.ctrl.Finish()

	m.expectConfiguredWorkspace()
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		Return(sendingStartedBroadcast("Summer sale"), nil).AnyTimes()
	m.expectValidTemplate()
	m.expectFailingRecipientFetch()
	m.broadcastRepo.EXPECT().
		MarkBroadcastStarted(gomock.Any(), "workspace-123", "broadcast-123", sendingStartedTime).
		Return(nil).Times(1)

	_, err := m.orchestrator(nil).Process(context.Background(), firstSendingRunTask(), time.Now().Add(30*time.Second))
	require.Error(t, err)
}

// A start date already set, by "send now" or a resume, is kept: no write is attempted.
// The mock has no MarkBroadcastStarted expectation, so any call fails the test.
func TestOrchestrator_Process_KeepsExistingStartedAt(t *testing.T) {
	m := newSendingStartedMocks(t)
	defer m.ctrl.Finish()

	startedAt := sendingStartedTime.Add(-time.Hour)
	bcast := sendingStartedBroadcast("Summer sale")
	bcast.StartedAt = &startedAt

	m.expectConfiguredWorkspace()
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		Return(bcast, nil).AnyTimes()
	m.expectValidTemplate()
	m.expectFailingRecipientFetch()
	m.eventBus.EXPECT().Publish(gomock.Any(), eventOfType(domain.EventBroadcastSendingStarted)).Times(1)

	_, err := m.orchestrator(m.eventBus).Process(context.Background(), firstSendingRunTask(), time.Now().Add(30*time.Second))
	require.Error(t, err)
}

func laterSendingRunTask() *domain.Task {
	return sendingStartedTask(&domain.SendBroadcastState{
		BroadcastID:     "broadcast-123",
		TotalRecipients: 25,
		RecipientOffset: 10,
		EnqueuedCount:   10,
	})
}

// A later run, its cursor advanced, dates a broadcast still without a start date: a
// scheduled send under way during the upgrade, or one whose first write failed. It does not
// announce the start again.
func TestOrchestrator_Process_LaterRunStampsMissingStartedAt(t *testing.T) {
	m := newSendingStartedMocks(t)
	defer m.ctrl.Finish()

	m.expectConfiguredWorkspace()
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		Return(sendingStartedBroadcast("Summer sale"), nil).AnyTimes()
	m.expectValidTemplate()
	m.expectFailingRecipientFetch()
	m.broadcastRepo.EXPECT().
		MarkBroadcastStarted(gomock.Any(), "workspace-123", "broadcast-123", sendingStartedTime).
		Return(nil).Times(1)

	_, err := m.orchestrator(m.eventBus).Process(context.Background(), laterSendingRunTask(), time.Now().Add(30*time.Second))
	require.Error(t, err)
}

// A later run of a broadcast already dated writes nothing. The mock has no
// MarkBroadcastStarted expectation, so any call fails the test.
func TestOrchestrator_Process_LaterRunKeepsExistingStartedAt(t *testing.T) {
	m := newSendingStartedMocks(t)
	defer m.ctrl.Finish()

	startedAt := sendingStartedTime.Add(-time.Hour)
	bcast := sendingStartedBroadcast("Summer sale")
	bcast.StartedAt = &startedAt

	m.expectConfiguredWorkspace()
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		Return(bcast, nil).AnyTimes()
	m.expectValidTemplate()
	m.expectFailingRecipientFetch()

	_, err := m.orchestrator(m.eventBus).Process(context.Background(), laterSendingRunTask(), time.Now().Add(30*time.Second))
	require.Error(t, err)
}

// A broadcast read as cancelled stops without sending: it is neither dated nor announced.
// The mock has no MarkBroadcastStarted expectation, so any call fails the test, and a
// sending_started publish fails it too.
func TestOrchestrator_Process_CancelledBroadcastDoesNotStampStartedAt(t *testing.T) {
	m := newSendingStartedMocks(t)
	defer m.ctrl.Finish()

	bcast := sendingStartedBroadcast("Summer sale")
	bcast.Status = domain.BroadcastStatusCancelled

	m.expectConfiguredWorkspace()
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		Return(bcast, nil).AnyTimes()
	m.expectValidTemplate()
	m.expectFailingRecipientFetch()
	m.eventBus.EXPECT().Publish(gomock.Any(), gomock.Any()).Do(func(_ context.Context, event domain.EventPayload) {
		assert.NotEqual(t, domain.EventBroadcastSendingStarted, event.Type, "a cancelled broadcast is not announced")
	}).AnyTimes()
	m.taskRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	m.broadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	_, _ = m.orchestrator(m.eventBus).Process(context.Background(), firstSendingRunTask(), time.Now().Add(30*time.Second))
}

// A paused broadcast stops without sending, so it has not started; the resume stamps it.
func TestOrchestrator_Process_PausedBroadcastDoesNotStampStartedAt(t *testing.T) {
	m := newSendingStartedMocks(t)
	defer m.ctrl.Finish()

	bcast := sendingStartedBroadcast("Summer sale")
	bcast.Status = domain.BroadcastStatusPaused

	m.expectConfiguredWorkspace()
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		Return(bcast, nil).AnyTimes()
	m.expectValidTemplate()
	m.expectFailingRecipientFetch()
	m.taskRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	m.broadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	_, _ = m.orchestrator(m.eventBus).Process(context.Background(), firstSendingRunTask(), time.Now().Add(30*time.Second))
}

// Failing to record the date must not stop the send: the run goes on to the recipient
// batch, whose error is the one returned.
func TestOrchestrator_Process_StartedAtStampFailureDoesNotStopTheSend(t *testing.T) {
	m := newSendingStartedMocks(t)
	defer m.ctrl.Finish()

	m.expectConfiguredWorkspace()
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		Return(sendingStartedBroadcast("Summer sale"), nil).AnyTimes()
	m.expectValidTemplate()
	m.eventBus.EXPECT().Publish(gomock.Any(), eventOfType(domain.EventBroadcastSendingStarted)).Times(1)
	m.broadcastRepo.EXPECT().
		MarkBroadcastStarted(gomock.Any(), "workspace-123", "broadcast-123", sendingStartedTime).
		Return(errors.New("db down")).Times(1)
	m.contactRepo.EXPECT().
		GetContactsForBroadcast(gomock.Any(), "workspace-123", gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("contact store unavailable")).Times(1)

	_, err := m.orchestrator(m.eventBus).Process(context.Background(), firstSendingRunTask(), time.Now().Add(30*time.Second))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contact store unavailable")
	assert.NotContains(t, err.Error(), "db down")
}
