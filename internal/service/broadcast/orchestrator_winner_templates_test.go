package broadcast_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Once a winner is picked, the rest of the list gets the winning variation only.
// The queue sender picks a template at random whenever it is handed more than
// one, so these tests look at the templates each batch is sent with, both for a
// run that starts after the pick and for a run that sees the pick on a refresh.
//
// Plan: plans/fix-broadcast-duplicate-sends-plan.md (Part 4)

// abAudience is the broadcast's list, in the email order the recipient query
// pages through.
var abAudience = []string{"a@example.com", "b@example.com", "c@example.com", "d@example.com"}

func abTemplate(id, subject string) *domain.Template {
	mjmlSource := "<mjml><mj-body><mj-text>" + subject + "</mj-text></mj-body></mjml>"
	return &domain.Template{
		ID:      id,
		Name:    subject,
		Channel: "email",
		Email: &domain.EmailTemplate{
			Subject:    subject,
			EditorMode: domain.EditorModeCode,
			MjmlSource: &mjmlSource,
		},
	}
}

// abBroadcast is an A/B broadcast of two variations testing on half the list.
// Each call returns a fresh copy, since the run writes to the one it holds.
func abBroadcast(status domain.BroadcastStatus, winner *string) *domain.Broadcast {
	return &domain.Broadcast{
		ID:              "broadcast-123",
		WorkspaceID:     "workspace-123",
		Name:            "Autumn sale",
		Status:          status,
		WinningTemplate: winner,
		Audience:        domain.AudienceSettings{List: "list-1"},
		TestSettings: domain.BroadcastTestSettings{
			Enabled:          true,
			SamplePercentage: 50,
			Variations: []domain.BroadcastVariation{
				{VariationName: "A", TemplateID: "template-A"},
				{VariationName: "B", TemplateID: "template-B"},
			},
		},
	}
}

// expectAudience serves abAudience through the keyset query, as the contact
// repository would.
func (m *sendingStartedMocks) expectAudience() {
	m.contactRepo.EXPECT().
		GetContactsForBroadcast(gomock.Any(), "workspace-123", gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, _ domain.AudienceSettings, limit int, afterEmail string) ([]*domain.ContactWithList, error) {
			var page []*domain.ContactWithList
			for _, email := range abAudience {
				if email > afterEmail && len(page) < limit {
					page = append(page, &domain.ContactWithList{Contact: &domain.Contact{Email: email}, ListID: "list-1"})
				}
			}
			return page, nil
		}).AnyTimes()
}

// sentBatch is what one SendBatch call was handed.
type sentBatch struct {
	recipients []string
	templates  []string
}

// recordBatches makes every SendBatch succeed and records what it was handed.
// afterEach, when set, runs once each batch has been recorded.
func (m *sendingStartedMocks) recordBatches(batches *[]sentBatch, afterEach func()) {
	m.messageSender.EXPECT().
		SendBatch(gomock.Any(), "workspace-123", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), "broadcast-123", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _, _, _ string, _ bool, _ *domain.WebAnalyticsSettings, _ string,
			recipients []*domain.ContactWithList, templates map[string]*domain.Template, _ *domain.EmailProvider,
			_ time.Time, _ string) (int, int, error) {
			batch := sentBatch{}
			for _, recipient := range recipients {
				batch.recipients = append(batch.recipients, recipient.Contact.Email)
			}
			for id := range templates {
				batch.templates = append(batch.templates, id)
			}
			sort.Strings(batch.templates)
			*batches = append(*batches, batch)
			if afterEach != nil {
				afterEach()
			}
			return len(recipients), 0, nil
		}).AnyTimes()
}

func abTask(state *domain.SendBroadcastState) *domain.Task {
	state.BroadcastID = "broadcast-123"
	state.TotalRecipients = len(abAudience)
	return sendingStartedTask(state)
}

// TestOrchestrator_Process_RunAfterAManualPickSendsTheWinnerOnly is the console
// flow: the test phase ended, the winner was picked by hand, and the task's next
// run still carries the phase "test" in its state. That run must neither load
// the losing variation nor send the rest of the list with it.
func TestOrchestrator_Process_RunAfterAManualPickSendsTheWinnerOnly(t *testing.T) {
	m := newSendingStartedMocks(t)
	defer m.ctrl.Finish()

	m.expectConfiguredWorkspace()
	m.expectAudience()
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		DoAndReturn(func(context.Context, string, string) (*domain.Broadcast, error) {
			return abBroadcast(domain.BroadcastStatusWinnerSelected, stringPtr("template-B")), nil
		}).AnyTimes()
	m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-B", int64(0)).
		Return(abTemplate("template-B", "Variation B"), nil).AnyTimes()
	m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-A", int64(0)).
		Times(0)
	m.taskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	var batches []sentBatch
	m.recordBatches(&batches, nil)

	var final *domain.Broadcast
	m.broadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
			final = b
			return nil
		})

	task := abTask(&domain.SendBroadcastState{
		Phase:              "test",
		TestPhaseCompleted: true,
		RecipientOffset:    2,
		EnqueuedCount:      2,
		LastProcessedEmail: "b@example.com",
	})

	allDone, err := m.orchestrator(nil).Process(context.Background(), task, time.Now().Add(30*time.Second))
	require.NoError(t, err)
	assert.True(t, allDone)

	require.NotEmpty(t, batches)
	var recipients []string
	for _, batch := range batches {
		assert.Equal(t, []string{"template-B"}, batch.templates, "a batch after the pick must carry the winner only")
		recipients = append(recipients, batch.recipients...)
	}
	assert.Equal(t, []string{"c@example.com", "d@example.com"}, recipients, "the rest of the list, after the sample")

	require.NotNil(t, final)
	assert.Equal(t, domain.BroadcastStatusProcessed, final.Status)
	assert.NotNil(t, final.WinnerSentAt)
}

// TestOrchestrator_Process_WinnerPickedDuringTheRunSendsTheNextBatchWithTheWinnerOnly
// covers a run that loaded every variation for the test phase and sees the pick
// on the refresh before its next batch.
func TestOrchestrator_Process_WinnerPickedDuringTheRunSendsTheNextBatchWithTheWinnerOnly(t *testing.T) {
	m := newSendingStartedMocks(t)
	defer m.ctrl.Finish()

	m.expectConfiguredWorkspace()
	m.expectAudience()
	picked := false
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		DoAndReturn(func(context.Context, string, string) (*domain.Broadcast, error) {
			if picked {
				return abBroadcast(domain.BroadcastStatusWinnerSelected, stringPtr("template-B")), nil
			}
			return abBroadcast(domain.BroadcastStatusTesting, nil), nil
		}).AnyTimes()
	m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-A", int64(0)).
		Return(abTemplate("template-A", "Variation A"), nil).AnyTimes()
	m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-B", int64(0)).
		Return(abTemplate("template-B", "Variation B"), nil).AnyTimes()
	m.taskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	var batches []sentBatch
	m.recordBatches(&batches, func() { picked = true })

	var final *domain.Broadcast
	m.broadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
			final = b
			return nil
		})

	task := abTask(&domain.SendBroadcastState{Phase: "test"})

	allDone, err := m.orchestrator(nil).Process(context.Background(), task, time.Now().Add(30*time.Second))
	require.NoError(t, err)
	assert.True(t, allDone)

	require.Len(t, batches, 2)
	assert.Equal(t, []string{"a@example.com", "b@example.com"}, batches[0].recipients)
	assert.Equal(t, []string{"template-A", "template-B"}, batches[0].templates, "the test sample is sent with every variation")
	assert.Equal(t, []string{"c@example.com", "d@example.com"}, batches[1].recipients)
	assert.Equal(t, []string{"template-B"}, batches[1].templates, "the batch after the pick must carry the winner only")

	require.NotNil(t, final)
	assert.Equal(t, domain.BroadcastStatusProcessed, final.Status)
}

// TestOrchestrator_Process_WinnerPhaseThatCannotSendTheWinnerFails covers a
// winner phase with no winner to send: rather than fall back to the variations
// it has, the run fails and sends nothing more.
func TestOrchestrator_Process_WinnerPhaseThatCannotSendTheWinnerFails(t *testing.T) {
	t.Run("the broadcast says a winner is selected but names none", func(t *testing.T) {
		m := newSendingStartedMocks(t)
		defer m.ctrl.Finish()

		m.expectConfiguredWorkspace()
		m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
			DoAndReturn(func(context.Context, string, string) (*domain.Broadcast, error) {
				return abBroadcast(domain.BroadcastStatusWinnerSelected, nil), nil
			}).AnyTimes()
		m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", gomock.Any(), int64(0)).
			DoAndReturn(func(_ context.Context, _, id string, _ int64) (*domain.Template, error) {
				return abTemplate(id, "Variation "+id), nil
			}).AnyTimes()

		task := abTask(&domain.SendBroadcastState{
			Phase:              "test",
			TestPhaseCompleted: true,
			RecipientOffset:    2,
			EnqueuedCount:      2,
			LastProcessedEmail: "b@example.com",
		})

		allDone, err := m.orchestrator(nil).Process(context.Background(), task, time.Now().Add(30*time.Second))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no winning template")
		assert.False(t, allDone)
	})

	// The winner failed to load when the run started, so the test phase went
	// out with the other variation alone; when the pick arrives mid-run the
	// winner is loaded on its own and must pass the same checks.
	winnerAtRefresh := map[string]func(m *sendingStartedMocks){
		"the winner can't be loaded": func(m *sendingStartedMocks) {
			m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-B", int64(0)).
				Return(nil, errors.New("template store unavailable")).AnyTimes()
		},
		"the winner fails validation": func(m *sendingStartedMocks) {
			noSubject := abTemplate("template-B", "")
			gomock.InOrder(
				m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-B", int64(0)).
					Return(nil, errors.New("template store unavailable")),
				m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-B", int64(0)).
					Return(noSubject, nil).AnyTimes(),
			)
		},
	}
	for name, winner := range winnerAtRefresh {
		t.Run(name, func(t *testing.T) {
			m := newSendingStartedMocks(t)
			defer m.ctrl.Finish()

			m.expectConfiguredWorkspace()
			m.expectAudience()
			picked := false
			m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
				DoAndReturn(func(context.Context, string, string) (*domain.Broadcast, error) {
					if picked {
						return abBroadcast(domain.BroadcastStatusWinnerSelected, stringPtr("template-B")), nil
					}
					return abBroadcast(domain.BroadcastStatusTesting, nil), nil
				}).AnyTimes()
			m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-A", int64(0)).
				Return(abTemplate("template-A", "Variation A"), nil).AnyTimes()
			winner(m)
			m.taskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).
				Return(nil).AnyTimes()

			var batches []sentBatch
			m.recordBatches(&batches, func() { picked = true })

			task := abTask(&domain.SendBroadcastState{Phase: "test"})

			allDone, err := m.orchestrator(nil).Process(context.Background(), task, time.Now().Add(30*time.Second))
			require.Error(t, err)
			assert.False(t, allDone)

			require.Len(t, batches, 1, "nothing may be sent once the winner phase has no winner to send")
			assert.Equal(t, []string{"a@example.com", "b@example.com"}, batches[0].recipients)
			assert.Equal(t, []string{"template-A"}, batches[0].templates)
		})
	}
}
