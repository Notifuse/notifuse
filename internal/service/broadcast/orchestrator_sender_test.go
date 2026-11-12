package broadcast_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service/broadcast"
	"github.com/Notifuse/notifuse/internal/service/broadcast/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// senderOrchestratorDeps runs Process against a marketing integration that has a single
// sender, so any other sender chosen on a variation has left it.
type senderOrchestratorDeps struct {
	sender      *mocks.MockMessageSender
	broadcasts  *domainmocks.MockBroadcastRepository
	templates   *domainmocks.MockTemplateRepository
	contacts    *domainmocks.MockContactRepository
	tasks       *domainmocks.MockTaskRepository
	eventBus    *domainmocks.MockEventBus
	kept        domain.EmailSender
	orchestrate broadcast.BroadcastOrchestratorInterface
}

func newSenderOrchestratorDeps(t *testing.T) *senderOrchestratorDeps {
	ctrl := gomock.NewController(t)

	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()
	logger.EXPECT().Warn(gomock.Any()).AnyTimes()

	timeProvider := mocks.NewMockTimeProvider(ctrl)
	timeProvider.EXPECT().Now().Return(time.Now()).AnyTimes()
	timeProvider.EXPECT().Since(gomock.Any()).Return(time.Second).AnyTimes()

	kept := domain.NewEmailSender("hello@example.com", "Default")
	workspaces := domainmocks.NewMockWorkspaceRepository(ctrl)
	workspaces.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(&domain.Workspace{
		ID: "workspace-123",
		Settings: domain.WorkspaceSettings{
			SecretKey:                "secret-key",
			EmailTrackingEnabled:     true,
			MarketingEmailProviderID: "marketing-provider-id",
		},
		Integrations: []domain.Integration{{
			ID:   "marketing-provider-id",
			Type: domain.IntegrationTypeEmail,
			EmailProvider: domain.EmailProvider{
				Kind:    domain.EmailProviderKindSES,
				SES:     &domain.AmazonSESSettings{AccessKey: "ak", SecretKey: "sk", Region: "us-east-1"},
				Senders: []domain.EmailSender{kept},
			},
		}},
	}, nil).AnyTimes()

	d := &senderOrchestratorDeps{
		sender:     mocks.NewMockMessageSender(ctrl),
		broadcasts: domainmocks.NewMockBroadcastRepository(ctrl),
		templates:  domainmocks.NewMockTemplateRepository(ctrl),
		contacts:   domainmocks.NewMockContactRepository(ctrl),
		tasks:      domainmocks.NewMockTaskRepository(ctrl),
		eventBus:   domainmocks.NewMockEventBus(ctrl),
		kept:       kept,
	}
	d.tasks.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	config := &broadcast.Config{FetchBatchSize: 50, MaxProcessTime: 30 * time.Second, ProgressLogInterval: 5 * time.Second}
	d.orchestrate = broadcast.NewBroadcastOrchestrator(d.sender, d.broadcasts, d.templates, d.contacts, d.tasks, workspaces, nil, nil, logger, config, timeProvider, "https://api.example.com", d.eventBus)
	return d
}

func (d *senderOrchestratorDeps) expectTemplate(id string) {
	d.templates.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", id, int64(0)).Return(&domain.Template{
		ID: id,
		Email: &domain.EmailTemplate{
			Subject:          "Hello",
			VisualEditorTree: &notifuse_mjml.MJMLBlock{BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)},
		},
	}, nil)
}

func senderTask(phase string) *domain.Task {
	return &domain.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Type:        "send_broadcast",
		BroadcastID: stringPtr("broadcast-123"),
		State: &domain.TaskState{SendBroadcast: &domain.SendBroadcastState{
			BroadcastID: "broadcast-123", TotalRecipients: 2, Phase: phase,
		}},
		MaxRetries: 3,
	}
}

func TestProcessBroadcastTask_VariationSenderGone_PausesBeforeSending(t *testing.T) {
	d := newSenderOrchestratorDeps(t)

	d.broadcasts.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(&domain.Broadcast{
		ID:       "broadcast-123",
		Status:   domain.BroadcastStatusProcessing,
		Audience: domain.AudienceSettings{List: "list-1"},
		TestSettings: domain.BroadcastTestSettings{
			Variations: []domain.BroadcastVariation{{VariationName: "Spring", TemplateID: "template-1", SenderID: "sender-since-deleted"}},
		},
	}, nil).AnyTimes()
	d.expectTemplate("template-1")

	// Nothing is fetched, sent or enqueued, and no send is announced.
	d.broadcasts.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
		assert.Equal(t, domain.BroadcastStatusPaused, b.Status)
		assert.NotNil(t, b.PausedAt)
		require.NotNil(t, b.PauseReason)
		assert.Contains(t, *b.PauseReason, "sender-since-deleted")
		assert.Contains(t, *b.PauseReason, `variation "Spring"`)
		assert.NotContains(t, *b.PauseReason, "feed")
		return nil
	})
	d.eventBus.EXPECT().Publish(gomock.Any(), eventOfType(domain.EventBroadcastPaused)).Do(func(_ context.Context, event domain.EventPayload) {
		assert.Equal(t, "sender_unavailable", event.Data["reason"])
	})

	done, err := d.orchestrate.Process(context.Background(), senderTask("single"), time.Now().Add(30*time.Second))
	assert.False(t, done)
	require.Error(t, err)
	assert.True(t, errors.Is(err, broadcast.ErrBroadcastShouldPause))
	assert.True(t, errors.Is(err, domain.ErrSenderNotFound))
	var senderErr *broadcast.SenderUnavailableError
	require.ErrorAs(t, err, &senderErr)
	assert.Equal(t, "template-1", senderErr.TemplateID)
}

// A broadcast whose test phase is over waits for a winner: a losing variation's missing
// sender must not pause it, or no winner could be picked.
// A paused broadcast is not paused again, nor announced as sending.
func TestProcessBroadcastTask_VariationSenderGone_NoPauseWhileNotSending(t *testing.T) {
	for _, status := range []domain.BroadcastStatus{domain.BroadcastStatusTestCompleted, domain.BroadcastStatusPaused} {
		t.Run(string(status), func(t *testing.T) {
			d := newSenderOrchestratorDeps(t)
			// The first sending run records the start date (orchestrator_started_at_test.go).
			d.broadcasts.EXPECT().MarkBroadcastStarted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

			d.broadcasts.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(&domain.Broadcast{
				ID:       "broadcast-123",
				Status:   status,
				Audience: domain.AudienceSettings{List: "list-1"},
				TestSettings: domain.BroadcastTestSettings{
					Enabled: true, SamplePercentage: 20,
					Variations: []domain.BroadcastVariation{
						{TemplateID: "template-a", SenderID: d.kept.ID},
						{TemplateID: "template-b", SenderID: "sender-since-deleted"},
					},
				},
			}, nil).AnyTimes()
			d.templates.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", gomock.Any(), int64(0)).Return(&domain.Template{
				ID: "template",
				Email: &domain.EmailTemplate{
					Subject:          "Hello",
					VisualEditorTree: &notifuse_mjml.MJMLBlock{BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)},
				},
			}, nil).AnyTimes()
			d.contacts.EXPECT().GetContactsForBroadcast(gomock.Any(), "workspace-123", gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
			d.broadcasts.EXPECT().WithTransaction(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			d.sender.EXPECT().SendBatch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(0, 0, nil).AnyTimes()
			d.eventBus.EXPECT().Publish(gomock.Any(), gomock.Any()).Do(func(_ context.Context, event domain.EventPayload) {
				assert.NotEqual(t, domain.EventBroadcastPaused, event.Type)
				if status == domain.BroadcastStatusPaused {
					// A paused broadcast sends nothing in this run: it has not started sending.
					assert.NotEqual(t, domain.EventBroadcastSendingStarted, event.Type)
				}
			}).AnyTimes()
			d.broadcasts.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
				if b.PauseReason != nil {
					assert.NotContains(t, *b.PauseReason, "sender-since-deleted")
				}
				return nil
			}).AnyTimes()

			_, err := d.orchestrate.Process(context.Background(), senderTask("test"), time.Now().Add(30*time.Second))
			assert.False(t, errors.Is(err, domain.ErrSenderNotFound), "unexpected sender error: %v", err)
		})
	}
}

func TestSenderUnavailableError_PauseReason(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  broadcast.SenderUnavailableError
		want string
	}{
		{"named variation", broadcast.SenderUnavailableError{VariationName: "Spring", VariationNumber: 2, VariationCount: 2, SenderID: "s-gone"}, `the sender chosen for variation "Spring" (sender s-gone)`},
		{"unnamed A/B variation, as the console saves them", broadcast.SenderUnavailableError{VariationNumber: 2, VariationCount: 2, TemplateID: "tpl-b", SenderID: "s-gone"}, "the sender chosen for variation 2 (sender s-gone)"},
		{"single variation", broadcast.SenderUnavailableError{VariationNumber: 1, VariationCount: 1, TemplateID: "tpl", SenderID: "s-gone"}, "the sender chosen for this broadcast (sender s-gone)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason := tc.err.PauseReason()
			assert.Contains(t, reason, tc.want)
			assert.NotContains(t, reason, "tpl")
		})
	}
}

func TestProcessBroadcastTask_WinnerPhase_OnlyTheWinnersSenderCounts(t *testing.T) {
	d := newSenderOrchestratorDeps(t)
	// The first sending run records the start date (orchestrator_started_at_test.go).
	d.broadcasts.EXPECT().MarkBroadcastStarted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	winner := "template-a"
	d.broadcasts.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(&domain.Broadcast{
		ID:              "broadcast-123",
		Status:          domain.BroadcastStatusWinnerSelected,
		Audience:        domain.AudienceSettings{List: "list-1"},
		WinningTemplate: &winner,
		TestSettings: domain.BroadcastTestSettings{
			Enabled: true, SamplePercentage: 20,
			Variations: []domain.BroadcastVariation{
				{TemplateID: "template-a", SenderID: d.kept.ID},
				{TemplateID: "template-b", SenderID: "sender-since-deleted"},
			},
		},
	}, nil).AnyTimes()
	d.expectTemplate("template-a")
	d.eventBus.EXPECT().Publish(gomock.Any(), gomock.Any()).AnyTimes()

	recipients := []*domain.ContactWithList{{Contact: &domain.Contact{Email: "user1@test.com"}, ListID: "list-1"}}
	d.contacts.EXPECT().GetContactsForBroadcast(gomock.Any(), "workspace-123", gomock.Any(), gomock.Any(), gomock.Any()).Return(recipients, nil).AnyTimes()

	// The winner is sent: the losing variation's sender is no reason to stop.
	stop := errors.New("stop after the first batch")
	d.sender.EXPECT().SendBatch(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "broadcast-123",
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(0, 0, stop).Times(1)

	_, err := d.orchestrate.Process(context.Background(), senderTask("winner"), time.Now().Add(30*time.Second))
	require.ErrorIs(t, err, stop)
}
