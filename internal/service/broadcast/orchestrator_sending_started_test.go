package broadcast_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service/broadcast"
	"github.com/Notifuse/notifuse/internal/service/broadcast/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// eventOfType matches an EventPayload by its Type, so a test can keep a precise
// expectation on one event while the orchestrator publishes others in the same run.
type eventOfType domain.EventType

func (m eventOfType) Matches(x interface{}) bool {
	event, ok := x.(domain.EventPayload)
	return ok && event.Type == domain.EventType(m)
}

func (m eventOfType) String() string {
	return fmt.Sprintf("is an event of type %q", string(m))
}

// sendingStartedMocks is the mock set shared by the sending-started tests.
type sendingStartedMocks struct {
	ctrl          *gomock.Controller
	messageSender *mocks.MockMessageSender
	broadcastRepo *domainmocks.MockBroadcastRepository
	templateRepo  *domainmocks.MockTemplateRepository
	contactRepo   *domainmocks.MockContactRepository
	taskRepo      *domainmocks.MockTaskRepository
	workspaceRepo *domainmocks.MockWorkspaceRepository
	logger        *pkgmocks.MockLogger
	timeProvider  *mocks.MockTimeProvider
	eventBus      *domainmocks.MockEventBus
}

// sendingStartedTime is the instant the mocked time provider reports, so the test can
// assert the exact started_at string that ends up on the event.
var sendingStartedTime = time.Date(2026, 8, 15, 9, 30, 0, 0, time.UTC)

func newSendingStartedMocks(t *testing.T) *sendingStartedMocks {
	ctrl := gomock.NewController(t)
	m := &sendingStartedMocks{
		ctrl:          ctrl,
		messageSender: mocks.NewMockMessageSender(ctrl),
		broadcastRepo: domainmocks.NewMockBroadcastRepository(ctrl),
		templateRepo:  domainmocks.NewMockTemplateRepository(ctrl),
		contactRepo:   domainmocks.NewMockContactRepository(ctrl),
		taskRepo:      domainmocks.NewMockTaskRepository(ctrl),
		workspaceRepo: domainmocks.NewMockWorkspaceRepository(ctrl),
		logger:        pkgmocks.NewMockLogger(ctrl),
		timeProvider:  mocks.NewMockTimeProvider(ctrl),
		eventBus:      domainmocks.NewMockEventBus(ctrl),
	}

	m.logger.EXPECT().WithFields(gomock.Any()).Return(m.logger).AnyTimes()
	m.logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(m.logger).AnyTimes()
	m.logger.EXPECT().Info(gomock.Any()).AnyTimes()
	m.logger.EXPECT().Debug(gomock.Any()).AnyTimes()
	m.logger.EXPECT().Error(gomock.Any()).AnyTimes()
	m.logger.EXPECT().Warn(gomock.Any()).AnyTimes()

	m.timeProvider.EXPECT().Now().Return(sendingStartedTime).AnyTimes()
	m.timeProvider.EXPECT().Since(gomock.Any()).Return(time.Second).AnyTimes()

	return m
}

// expectConfiguredWorkspace makes the workspace lookup and the email provider validation
// that sit before the publish point succeed.
func (m *sendingStartedMocks) expectConfiguredWorkspace() {
	workspace := &domain.Workspace{
		ID: "workspace-123",
		Settings: domain.WorkspaceSettings{
			SecretKey:                "secret-key",
			EmailTrackingEnabled:     true,
			MarketingEmailProviderID: "marketing-provider-id",
		},
		Integrations: []domain.Integration{
			{
				ID:   "marketing-provider-id",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSES,
					SES:  &domain.AmazonSESSettings{AccessKey: "ak", SecretKey: "sk", Region: "us-east-1"},
				},
			},
		},
	}
	m.workspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(workspace, nil).AnyTimes()
}

// expectValidTemplate clears the two template gates that now sit before the publish:
// the template loads, and it carries the subject and code-mode content ValidateTemplates
// insists on.
func (m *sendingStartedMocks) expectValidTemplate() {
	mjmlSource := "<mjml><mj-body><mj-text>Hello</mj-text></mj-body></mjml>"
	template := &domain.Template{
		ID:      "template-1",
		Name:    "Summer sale template",
		Channel: "email",
		Email: &domain.EmailTemplate{
			Subject:    "Summer sale is on",
			EditorMode: domain.EditorModeCode,
			MjmlSource: &mjmlSource,
		},
	}
	m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-1", int64(0)).
		Return(template, nil).AnyTimes()
}

// expectFailingTemplateLoad blocks the run at template loading, before the publish point.
func (m *sendingStartedMocks) expectFailingTemplateLoad() {
	m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-1", int64(0)).
		Return(nil, errors.New("template store unavailable")).AnyTimes()
}

// expectFailingRecipientFetch stops the run at the first batch fetch, which is the step
// right after the publish. It keeps the tests that must reach the publish point short.
func (m *sendingStartedMocks) expectFailingRecipientFetch() {
	m.contactRepo.EXPECT().
		GetContactsForBroadcast(gomock.Any(), "workspace-123", gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("contact store unavailable")).AnyTimes()
}

func (m *sendingStartedMocks) orchestrator(eventBus domain.EventBus) broadcast.BroadcastOrchestratorInterface {
	config := &broadcast.Config{FetchBatchSize: 50, MaxProcessTime: 30 * time.Second, ProgressLogInterval: 5 * time.Second}
	return broadcast.NewBroadcastOrchestrator(
		m.messageSender,
		m.broadcastRepo,
		m.templateRepo,
		m.contactRepo,
		m.taskRepo,
		m.workspaceRepo,
		nil, // emailQueueRepo not needed: no run here reaches the enqueue step
		nil, // abTestEvaluator not needed
		m.logger,
		config,
		m.timeProvider,
		"https://api.example.com",
		eventBus,
	)
}

func sendingStartedBroadcast(name string) *domain.Broadcast {
	return &domain.Broadcast{
		ID:          "broadcast-123",
		WorkspaceID: "workspace-123",
		Name:        name,
		Status:      domain.BroadcastStatusProcessing,
		Audience:    domain.AudienceSettings{List: "list-1"},
		TestSettings: domain.BroadcastTestSettings{
			Variations: []domain.BroadcastVariation{{TemplateID: "template-1"}},
		},
	}
}

func sendingStartedTask(state *domain.SendBroadcastState) *domain.Task {
	return &domain.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Type:        "send_broadcast",
		BroadcastID: stringPtr("broadcast-123"),
		Status:      domain.TaskStatusRunning,
		RetryCount:  0,
		MaxRetries:  3,
		State:       &domain.TaskState{SendBroadcast: state},
	}
}

// TestOrchestrator_Process_PublishesSendingStartedOnFirstSendingRun covers the run that
// actually begins sending. That is never the first task run: the counting block returns
// before the broadcast row is loaded, so this is the second run, with TotalRecipients
// already set and the cursor still at zero. Every gate before the publish is cleared —
// workspace, provider, template load, template validation — and the run is then cut short
// at the first recipient batch, which is the step right after the publish.
func TestOrchestrator_Process_PublishesSendingStartedOnFirstSendingRun(t *testing.T) {
	m := newSendingStartedMocks(t)
	defer m.ctrl.Finish()

	m.expectConfiguredWorkspace()
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		Return(sendingStartedBroadcast("Summer sale"), nil).AnyTimes()
	m.expectValidTemplate()
	m.expectFailingRecipientFetch()

	var published domain.EventPayload
	m.eventBus.EXPECT().Publish(gomock.Any(), eventOfType(domain.EventBroadcastSendingStarted)).
		Times(1).
		Do(func(_ context.Context, event domain.EventPayload) {
			published = event
		})

	orchestrator := m.orchestrator(m.eventBus)
	task := sendingStartedTask(&domain.SendBroadcastState{
		BroadcastID:     "broadcast-123",
		TotalRecipients: 25,
		RecipientOffset: 0,
		EnqueuedCount:   0,
	})

	_, err := orchestrator.Process(context.Background(), task, time.Now().Add(30*time.Second))
	require.Error(t, err)

	assert.Equal(t, domain.EventBroadcastSendingStarted, published.Type)
	assert.Equal(t, "workspace-123", published.WorkspaceID)
	assert.Equal(t, "broadcast-123", published.EntityID)
	assert.Equal(t, "Summer sale", published.Data["broadcast_name"])
	assert.Equal(t, sendingStartedTime.Format(time.RFC3339), published.Data["started_at"])
}

// TestOrchestrator_Process_TemplateFailure_PublishesNothing pins the reason the publish
// sits after the template gates: a broadcast whose template cannot be loaded mails nobody,
// and the announcement it would otherwise have made is never retracted. The state is that
// of a first sending run, so only the template gate can keep the run quiet. The mock event
// bus has no Publish expectation, so any publish fails the test.
func TestOrchestrator_Process_TemplateFailure_PublishesNothing(t *testing.T) {
	m := newSendingStartedMocks(t)
	defer m.ctrl.Finish()

	m.expectConfiguredWorkspace()
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		Return(sendingStartedBroadcast("Summer sale"), nil).AnyTimes()
	m.expectFailingTemplateLoad()

	orchestrator := m.orchestrator(m.eventBus)
	task := sendingStartedTask(&domain.SendBroadcastState{
		BroadcastID:     "broadcast-123",
		TotalRecipients: 25,
		RecipientOffset: 0,
		EnqueuedCount:   0,
	})

	_, err := orchestrator.Process(context.Background(), task, time.Now().Add(30*time.Second))
	require.Error(t, err)
}

// TestOrchestrator_Process_DoesNotRepublishOnResume pins the run gate: a resumed broadcast
// re-enters Process with the cursor already advanced and must stay silent. The templates
// load and validate, so the run reaches the publish point and only the gate under test can
// keep it quiet. The mock event bus has no Publish expectation, so any publish fails the test.
func TestOrchestrator_Process_DoesNotRepublishOnResume(t *testing.T) {
	m := newSendingStartedMocks(t)
	defer m.ctrl.Finish()

	m.expectConfiguredWorkspace()
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		Return(sendingStartedBroadcast("Summer sale"), nil).AnyTimes()
	m.expectValidTemplate()
	m.expectFailingRecipientFetch()

	orchestrator := m.orchestrator(m.eventBus)
	task := sendingStartedTask(&domain.SendBroadcastState{
		BroadcastID:     "broadcast-123",
		TotalRecipients: 25,
		RecipientOffset: 10,
		EnqueuedCount:   10,
	})

	_, err := orchestrator.Process(context.Background(), task, time.Now().Add(30*time.Second))
	require.Error(t, err)
}

// TestOrchestrator_Process_CountRun_PublishesNothing covers the counting run. It has a zero
// cursor and a zero enqueued count, but it returns before the broadcast row is loaded, so
// nothing is announced — the announcement waits for the next run.
func TestOrchestrator_Process_CountRun_PublishesNothing(t *testing.T) {
	m := newSendingStartedMocks(t)
	defer m.ctrl.Finish()

	bcast := sendingStartedBroadcast("Summer sale")
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		Return(bcast, nil).AnyTimes()
	m.contactRepo.EXPECT().CountContactsForBroadcast(gomock.Any(), "workspace-123", bcast.Audience).
		Return(42, nil).Times(1)

	orchestrator := m.orchestrator(m.eventBus)
	task := sendingStartedTask(&domain.SendBroadcastState{
		BroadcastID:     "broadcast-123",
		TotalRecipients: 0,
	})

	done, err := orchestrator.Process(context.Background(), task, time.Now().Add(30*time.Second))
	require.NoError(t, err)
	assert.False(t, done)
	assert.Equal(t, 42, task.State.SendBroadcast.TotalRecipients)
}

// TestOrchestrator_Process_NoRecipients_PublishesNothing covers a broadcast whose audience
// is empty: it completes without sending a single email, so there is nothing to announce.
func TestOrchestrator_Process_NoRecipients_PublishesNothing(t *testing.T) {
	m := newSendingStartedMocks(t)
	defer m.ctrl.Finish()

	bcast := sendingStartedBroadcast("Summer sale")
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		Return(bcast, nil).AnyTimes()
	m.contactRepo.EXPECT().CountContactsForBroadcast(gomock.Any(), "workspace-123", bcast.Audience).
		Return(0, nil).Times(1)
	m.broadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	orchestrator := m.orchestrator(m.eventBus)
	task := sendingStartedTask(&domain.SendBroadcastState{
		BroadcastID:     "broadcast-123",
		TotalRecipients: 0,
	})

	done, err := orchestrator.Process(context.Background(), task, time.Now().Add(30*time.Second))
	require.NoError(t, err)
	assert.True(t, done)
}

// TestOrchestrator_Process_NilEventBus keeps the orchestrator usable without a bus, the
// same guard the other publishes in Process carry. The state and the mocks are those of the
// positive test, so the run really does reach the publish point with no bus to publish to.
func TestOrchestrator_Process_NilEventBus(t *testing.T) {
	m := newSendingStartedMocks(t)
	defer m.ctrl.Finish()

	m.expectConfiguredWorkspace()
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		Return(sendingStartedBroadcast("Summer sale"), nil).AnyTimes()
	m.expectValidTemplate()
	m.expectFailingRecipientFetch()

	orchestrator := m.orchestrator(nil)
	task := sendingStartedTask(&domain.SendBroadcastState{
		BroadcastID:     "broadcast-123",
		TotalRecipients: 25,
		RecipientOffset: 0,
		EnqueuedCount:   0,
	})

	require.NotPanics(t, func() {
		_, err := orchestrator.Process(context.Background(), task, time.Now().Add(30*time.Second))
		require.Error(t, err)
	})
}
