package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// senderFixture is a workspace whose marketing integration has a default sender and a
// second one a broadcast can choose. domain.NewEmailSender marks every sender default, so
// the second is unmarked by hand: a send that falls back to the default is told apart.
type senderFixture struct {
	workspace *domain.Workspace
	byDefault domain.EmailSender
	promos    domain.EmailSender
}

func newSenderFixture() senderFixture {
	byDefault := domain.NewEmailSender("hello@example.com", "Default")
	promos := domain.NewEmailSender("promos@example.com", "Promos")
	promos.IsDefault = false
	return senderFixture{
		byDefault: byDefault,
		promos:    promos,
		workspace: &domain.Workspace{
			ID:       "w1",
			Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt", SecretKey: "sk_test"},
			Integrations: domain.Integrations{
				{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{
					Kind:    domain.EmailProviderKindSMTP,
					Senders: []domain.EmailSender{byDefault, promos},
				}},
			},
		},
	}
}

func requireValidationError(t *testing.T, err error, contains string) {
	t.Helper()
	require.Error(t, err)
	var validationErr domain.ValidationError
	require.True(t, errors.As(err, &validationErr), "want a domain.ValidationError, got %T: %v", err, err)
	assert.Contains(t, err.Error(), contains)
}

func TestBroadcastService_CreateBroadcast_VariationSender(t *testing.T) {
	newRequest := func(senderID string) *domain.CreateBroadcastRequest {
		return &domain.CreateBroadcastRequest{
			WorkspaceID: "w1",
			Name:        "November letter",
			Audience:    domain.AudienceSettings{List: "list1"},
			TestSettings: domain.BroadcastTestSettings{
				Variations: []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA", SenderID: senderID}},
			},
		}
	}

	t.Run("a sender of the marketing integration is saved", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		f := newSenderFixture()
		ctx := context.Background()
		authOK(d.authService, ctx, "w1")
		d.workspaceRepo.EXPECT().GetByID(gomock.Any(), "w1").Return(f.workspace, nil)
		d.repo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
			assert.Equal(t, f.promos.ID, b.TestSettings.Variations[0].SenderID)
			return nil
		})

		_, err := d.svc.CreateBroadcast(ctx, newRequest(f.promos.ID))
		require.NoError(t, err)
	})

	t.Run("a sender the integration does not have is refused", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		f := newSenderFixture()
		ctx := context.Background()
		authOK(d.authService, ctx, "w1")
		d.workspaceRepo.EXPECT().GetByID(gomock.Any(), "w1").Return(f.workspace, nil)

		_, err := d.svc.CreateBroadcast(ctx, newRequest("unknown-sender"))
		requireValidationError(t, err, "unknown-sender")
	})

	t.Run("a sender without a marketing integration is refused", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		ctx := context.Background()
		authOK(d.authService, ctx, "w1")
		d.workspaceRepo.EXPECT().GetByID(gomock.Any(), "w1").Return(&domain.Workspace{ID: "w1"}, nil)

		_, err := d.svc.CreateBroadcast(ctx, newRequest("any-sender"))
		requireValidationError(t, err, "marketing")
	})

	t.Run("without a chosen sender nothing is looked up", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		ctx := context.Background()
		authOK(d.authService, ctx, "w1")
		d.repo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).Return(nil)

		_, err := d.svc.CreateBroadcast(ctx, newRequest(""))
		require.NoError(t, err)
	})
}

func TestBroadcastService_UpdateBroadcast_VariationSender(t *testing.T) {
	t.Run("an update carrying an unknown sender is refused", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		f := newSenderFixture()
		ctx := context.Background()
		authOK(d.authService, ctx, "w1")
		runsInTx(d, "w1")
		d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(testBroadcast("w1", "b1"), nil)
		d.workspaceRepo.EXPECT().GetByID(gomock.Any(), "w1").Return(f.workspace, nil)

		var req domain.UpdateBroadcastRequest
		require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"w1","id":"b1","name":"n",
			"test_settings":{"enabled":false,"variations":[{"variation_name":"A","template_id":"tplA","sender_id":"unknown-sender"}]}}`), &req))

		_, err := d.svc.UpdateBroadcast(ctx, &req)
		requireValidationError(t, err, "unknown-sender")
	})

	t.Run("a paused broadcast whose sender has gone takes another known sender", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		f := newSenderFixture()
		ctx := context.Background()
		authOK(d.authService, ctx, "w1")
		stored := testBroadcast("w1", "b1")
		stored.Status = domain.BroadcastStatusPaused
		stored.TestSettings.Variations[0].SenderID = "sender-since-deleted"
		runsInTx(d, "w1")
		d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(stored, nil)
		d.workspaceRepo.EXPECT().GetByID(gomock.Any(), "w1").Return(f.workspace, nil)
		d.repo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ *sql.Tx, b *domain.Broadcast) error {
			assert.Equal(t, f.promos.ID, b.TestSettings.Variations[0].SenderID)
			assert.Equal(t, domain.BroadcastStatusPaused, b.Status)
			return nil
		})

		var req domain.UpdateBroadcastRequest
		require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"w1","id":"b1","name":"n",
			"test_settings":{"enabled":false,"variations":[{"variation_name":"A","template_id":"tplA","sender_id":"`+f.promos.ID+`"}]}}`), &req))

		_, err := d.svc.UpdateBroadcast(ctx, &req)
		require.NoError(t, err)
	})

	t.Run("an update leaving test_settings out is not blocked by a stored sender that has gone", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		ctx := context.Background()
		authOK(d.authService, ctx, "w1")
		stored := testBroadcast("w1", "b1")
		stored.TestSettings.Variations[0].SenderID = "sender-since-deleted"
		runsInTx(d, "w1")
		d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(stored, nil)
		d.repo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		var req domain.UpdateBroadcastRequest
		require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"w1","id":"b1","name":"Renamed"}`), &req))

		updated, err := d.svc.UpdateBroadcast(ctx, &req)
		require.NoError(t, err)
		assert.Equal(t, "Renamed", updated.Name)
		assert.Equal(t, "sender-since-deleted", updated.TestSettings.Variations[0].SenderID)
	})
}

func TestBroadcastService_ScheduleBroadcast_VariationSenderGone(t *testing.T) {
	d := setupBroadcastSvc(t)
	f := newSenderFixture()
	ctx := context.Background()
	req := &domain.ScheduleBroadcastRequest{WorkspaceID: "w1", ID: "b1", SendNow: true}
	authOK(d.authService, ctx, "w1")
	d.workspaceRepo.EXPECT().GetByID(ctx, "w1").Return(f.workspace, nil)
	d.repo.EXPECT().WithTransaction(ctx, "w1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)
	draft := testBroadcast("w1", "b1")
	draft.TestSettings.Variations[0].SenderID = "sender-since-deleted"
	d.repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(draft, nil)
	// No UpdateBroadcastTx and no event: nothing is scheduled.

	err := d.svc.ScheduleBroadcast(ctx, req)
	requireValidationError(t, err, "sender-since-deleted")
}

func TestBroadcastService_ResumeBroadcast_VariationSender(t *testing.T) {
	expectTx := func(d *broadcastSvcDeps, ctx context.Context) {
		d.repo.EXPECT().WithTransaction(ctx, "w1", gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
		)
	}
	expectResumed := func(d *broadcastSvcDeps) {
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		d.emailQueueRepo.EXPECT().ResumeBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").Return(int64(0), nil)
		d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, _ domain.EventPayload, ack domain.EventAckCallback) { ack(nil) })
	}

	t.Run("a broadcast still to enqueue does not resume once its sender has gone", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		f := newSenderFixture()
		ctx := context.Background()
		authOK(d.authService, ctx, "w1")
		noTaskStateYet(d)
		expectTx(d, ctx)
		paused := testBroadcast("w1", "b1")
		paused.Status = domain.BroadcastStatusPaused
		paused.TestSettings.Variations[0].SenderID = "sender-since-deleted"
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(paused, nil)
		d.workspaceRepo.EXPECT().GetByID(gomock.Any(), "w1").Return(f.workspace, nil)

		err := d.svc.ResumeBroadcast(ctx, &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"})
		requireValidationError(t, err, "sender-since-deleted")
	})

	t.Run("a broadcast whose recipients are all queued resumes: its emails carry their sender", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		ctx := context.Background()
		authOK(d.authService, ctx, "w1")
		expectTx(d, ctx)
		paused := testBroadcast("w1", "b1")
		paused.Status = domain.BroadcastStatusPaused
		completed := time.Now().UTC()
		paused.CompletedAt = &completed
		paused.TestSettings.Variations[0].SenderID = "sender-since-deleted"
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(paused, nil)
		expectResumed(d)

		require.NoError(t, d.svc.ResumeBroadcast(ctx, &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"}))
	})

	t.Run("in the winner phase only the winning variation's sender counts", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		f := newSenderFixture()
		ctx := context.Background()
		authOK(d.authService, ctx, "w1")
		expectTx(d, ctx)
		paused := testBroadcast("w1", "b1")
		paused.Status = domain.BroadcastStatusPaused
		paused.TestSettings = domain.BroadcastTestSettings{Enabled: true, SamplePercentage: 20, Variations: []domain.BroadcastVariation{
			{VariationName: "A", TemplateID: "tplA", SenderID: f.promos.ID},
			{VariationName: "B", TemplateID: "tplB", SenderID: "sender-since-deleted"},
		}}
		winner := "tplA"
		paused.WinningTemplate = &winner
		noTaskStateYet(d)
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(paused, nil)
		d.workspaceRepo.EXPECT().GetByID(gomock.Any(), "w1").Return(f.workspace, nil)
		expectResumed(d)

		require.NoError(t, d.svc.ResumeBroadcast(ctx, &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"}))
	})

	// A pause can come after the run queued the last recipient and before it wrote the
	// completion date: the task's cursor then tells that nothing is left to queue.
	taskAt := func(phase string, offset int64, total int) *domain.Task {
		return &domain.Task{ID: "task-1", State: &domain.TaskState{SendBroadcast: &domain.SendBroadcastState{
			Phase: phase, RecipientOffset: offset, TotalRecipients: total,
		}}}
	}
	for _, tc := range []struct {
		name    string
		task    *domain.Task
		readErr error
		checked bool
	}{
		{"every recipient queued: no sender to resolve", taskAt("single", 40, 40), nil, false},
		{"every recipient of the winner phase queued: no sender to resolve", taskAt("winner", 40, 40), nil, false},
		{"recipients left: their sender is resolved", taskAt("single", 20, 40), nil, true},
		{"the test phase: the winner's recipients are still to queue", taskAt("test", 40, 40), nil, true},
		{"a task that cannot be read keeps the check", nil, errors.New("connection reset"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := setupBroadcastSvc(t)
			f := newSenderFixture()
			ctx := context.Background()
			authOK(d.authService, ctx, "w1")
			expectTx(d, ctx)
			paused := testBroadcast("w1", "b1")
			paused.Status = domain.BroadcastStatusPaused
			paused.TestSettings.Variations[0].SenderID = "sender-since-deleted"
			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(paused, nil)
			d.taskRepo.EXPECT().GetTaskByBroadcastID(gomock.Any(), "w1", "b1").Return(tc.task, tc.readErr)
			if tc.readErr != nil {
				// The failed read is logged at Warn once (the strict mock refuses anything else).
				log := pkgmocks.NewMockLogger(gomock.NewController(t))
				log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
				log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
				log.EXPECT().Warn("Failed to read the task of the broadcast to resume").Times(1)
				d.svc.logger = log
			}

			if tc.checked {
				d.workspaceRepo.EXPECT().GetByID(gomock.Any(), "w1").Return(f.workspace, nil)
				err := d.svc.ResumeBroadcast(ctx, &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"})
				requireValidationError(t, err, "sender-since-deleted")
				return
			}
			d.taskRepo.EXPECT().ResetRetryBudget(gomock.Any(), "w1", "task-1").Return(nil)
			expectResumed(d)
			require.NoError(t, d.svc.ResumeBroadcast(ctx, &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"}))
		})
	}
}

// noTaskStateYet answers the resume's read of the broadcast's task with no task, so the
// sender check runs as it does for a broadcast whose run has not started.
func noTaskStateYet(d *broadcastSvcDeps) {
	d.taskRepo.EXPECT().GetTaskByBroadcastID(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, domain.ErrTaskNotFound).AnyTimes()
}

func TestBroadcastService_SendToIndividual_VariationSender(t *testing.T) {
	t.Run("the variation's sender wins over the template's", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		f := newSenderFixture()
		ctx := context.Background()
		req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "to@example.com"}
		authOK(d.authService, ctx, "w1")
		d.workspaceRepo.EXPECT().GetByID(ctx, "w1").Return(f.workspace, nil)
		b := testBroadcast("w1", "b1")
		b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA", SenderID: f.promos.ID}}
		d.repo.EXPECT().GetBroadcast(ctx, "w1", "b1").Return(b, nil)
		d.contactRepo.EXPECT().GetContactByEmail(ctx, "w1", req.RecipientEmail).Return(nil, errors.New("not found")).AnyTimes()
		d.templateSvc.EXPECT().GetTemplateByID(ctx, "w1", "tplA", int64(0)).Return(&domain.Template{
			ID: "tplA", Name: "Template A", Channel: "email",
			Email: &domain.EmailTemplate{SenderID: f.byDefault.ID, Subject: "Hello", VisualEditorTree: createMJMLRootBlock()},
		}, nil)
		compiledHTML := "<html>ok</html>"
		d.templateSvc.EXPECT().CompileTemplate(ctx, gomock.Any()).Return(&domain.CompileTemplateResponse{Success: true, HTML: &compiledHTML}, nil)
		d.emailSvc.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).DoAndReturn(
			func(_ context.Context, sent domain.SendEmailProviderRequest, _ bool) error {
				assert.Equal(t, "promos@example.com", sent.FromAddress)
				assert.Equal(t, "Promos", sent.FromName)
				return nil
			})
		d.messageHistoryRepo.EXPECT().Create(gomock.Any(), "w1", gomock.Any(), gomock.Any()).Return(nil)

		require.NoError(t, d.svc.SendToIndividual(ctx, req))
	})

	t.Run("a variation sender that has gone is an error, not the default sender", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		f := newSenderFixture()
		ctx := context.Background()
		req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "to@example.com"}
		authOK(d.authService, ctx, "w1")
		d.workspaceRepo.EXPECT().GetByID(ctx, "w1").Return(f.workspace, nil)
		b := testBroadcast("w1", "b1")
		b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA", SenderID: "sender-since-deleted"}}
		d.repo.EXPECT().GetBroadcast(ctx, "w1", "b1").Return(b, nil)
		d.contactRepo.EXPECT().GetContactByEmail(ctx, "w1", req.RecipientEmail).Return(nil, errors.New("not found")).AnyTimes()
		d.templateSvc.EXPECT().GetTemplateByID(ctx, "w1", "tplA", int64(0)).Return(&domain.Template{
			ID: "tplA", Name: "Template A", Channel: "email",
			Email: &domain.EmailTemplate{SenderID: f.byDefault.ID, Subject: "Hello", VisualEditorTree: createMJMLRootBlock()},
		}, nil)

		err := d.svc.SendToIndividual(ctx, req)
		require.ErrorIs(t, err, domain.ErrSenderNotFound)
	})
}

// A run that fails while its broadcast is paused reads the broadcast, then its task, before
// it fails the broadcast for good. The resume writes the task's fresh retry budget before it
// commits, so a broadcast read as resumed comes with a task read that sees the budget.
func TestBroadcastService_ResumeBroadcast_ResetsTheRetryBudgetBeforeItCommits(t *testing.T) {
	setup := func(t *testing.T, task *domain.Task) (*broadcastSvcDeps, context.Context, *bool) {
		d := setupBroadcastSvc(t)
		ctx := context.Background()
		authOK(d.authService, ctx, "w1")
		committed := new(bool)
		d.repo.EXPECT().WithTransaction(ctx, "w1", gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
				if err := fn(nil); err != nil {
					return err
				}
				*committed = true
				return nil
			})
		paused := testBroadcast("w1", "b1")
		paused.Status = domain.BroadcastStatusPaused
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(paused, nil)
		d.taskRepo.EXPECT().GetTaskByBroadcastID(gomock.Any(), "w1", "b1").Return(task, nil)
		d.workspaceRepo.EXPECT().GetByID(gomock.Any(), "w1").Return(newSenderFixture().workspace, nil).AnyTimes()
		d.repo.EXPECT().UpdateBroadcastStatusTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		d.emailQueueRepo.EXPECT().ResumeBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "b1").Return(int64(0), nil)
		d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, _ domain.EventPayload, ack domain.EventAckCallback) { ack(nil) })
		return d, ctx, committed
	}

	t.Run("the reset precedes the commit", func(t *testing.T) {
		d, ctx, committed := setup(t, &domain.Task{ID: "task-1", Status: domain.TaskStatusPaused})
		d.taskRepo.EXPECT().ResetRetryBudget(gomock.Any(), "w1", "task-1").DoAndReturn(
			func(context.Context, string, string) error {
				assert.False(t, *committed, "the budget is reset before the resume commits")
				return nil
			})
		require.NoError(t, d.svc.ResumeBroadcast(ctx, &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"}))
		assert.True(t, *committed)
	})

	t.Run("a failed reset does not stop the resume", func(t *testing.T) {
		d, ctx, committed := setup(t, &domain.Task{ID: "task-1", Status: domain.TaskStatusPaused})
		d.taskRepo.EXPECT().ResetRetryBudget(gomock.Any(), "w1", "task-1").Return(errors.New("connection reset"))
		require.NoError(t, d.svc.ResumeBroadcast(ctx, &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"}))
		assert.True(t, *committed)
	})

	t.Run("a completed task is left alone", func(t *testing.T) {
		d, ctx, _ := setup(t, &domain.Task{ID: "task-1", Status: domain.TaskStatusCompleted})
		d.taskRepo.EXPECT().ResetRetryBudget(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		require.NoError(t, d.svc.ResumeBroadcast(ctx, &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"}))
	})
}
