package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
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
		d.repo.EXPECT().GetBroadcast(ctx, "w1", "b1").Return(testBroadcast("w1", "b1"), nil)
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
		d.repo.EXPECT().GetBroadcast(ctx, "w1", "b1").Return(stored, nil)
		d.workspaceRepo.EXPECT().GetByID(gomock.Any(), "w1").Return(f.workspace, nil)
		d.repo.EXPECT().UpdateBroadcast(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
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
		d.repo.EXPECT().GetBroadcast(ctx, "w1", "b1").Return(stored, nil)
		d.repo.EXPECT().UpdateBroadcast(ctx, gomock.Any()).Return(nil)

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
	d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(draft, nil)
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
		d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(paused, nil)
		d.workspaceRepo.EXPECT().GetByID(gomock.Any(), "w1").Return(f.workspace, nil)
		expectResumed(d)

		require.NoError(t, d.svc.ResumeBroadcast(ctx, &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"}))
	})
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
