package service

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

// sendToIndividualFixture sets up a test send of a one-variation broadcast whose template has
// the given subject, to an address that is not a contact. It returns what the send compiled,
// sent and recorded once the caller runs it.
type sendToIndividualFixture struct {
	compiled domain.CompileTemplateRequest
	sent     domain.SendEmailProviderRequest
	recorded *domain.MessageHistory
}

func setupSendToIndividual(t *testing.T, d *broadcastSvcDeps, req *domain.SendToIndividualRequest, subject string, expectSend bool) *sendToIndividualFixture {
	t.Helper()
	ctx := context.Background()
	f := &sendToIndividualFixture{}
	authOK(d.authService, ctx, req.WorkspaceID)

	sender := domain.NewEmailSender("from@example.com", "From")
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(&domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt", SecretKey: "sk_test"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{sender}}},
		},
	}, nil)

	b := testBroadcast(req.WorkspaceID, req.BroadcastID)
	b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.BroadcastID).Return(b, nil)
	d.contactRepo.EXPECT().GetContactByEmail(ctx, req.WorkspaceID, req.RecipientEmail).Return(nil, errors.New("not found")).AnyTimes()
	d.templateSvc.EXPECT().GetTemplateByID(ctx, req.WorkspaceID, "tplA", int64(0)).Return(&domain.Template{
		ID:      "tplA",
		Version: 3,
		Channel: "email",
		Email: &domain.EmailTemplate{
			SenderID:         sender.ID,
			Subject:          subject,
			VisualEditorTree: createMJMLRootBlock(),
		},
		Category:  string(domain.TemplateCategoryMarketing),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil)

	compiledHTML := "<html>ok</html>"
	d.templateSvc.EXPECT().CompileTemplate(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, payload domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
		f.compiled = payload
		return &domain.CompileTemplateResponse{Success: true, HTML: &compiledHTML}, nil
	})
	if expectSend {
		d.emailSvc.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).DoAndReturn(
			func(_ context.Context, r domain.SendEmailProviderRequest, _ bool) error { f.sent = r; return nil },
		)
		d.messageHistoryRepo.EXPECT().Create(gomock.Any(), req.WorkspaceID, gomock.Any(), gomock.Any()).Do(
			func(_ context.Context, _ string, _ string, msg *domain.MessageHistory) { f.recorded = msg },
		).Return(nil)
	}
	return f
}

// A test send is not part of the broadcast: it is recorded without the broadcast, so it does
// not count in its statistics, and, as the web version page only serves broadcast and
// automation messages, it carries no web version link that would lead to a page not found.
func TestBroadcastService_SendToIndividual_IsNotPartOfTheBroadcast(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "to@example.com"}
	f := setupSendToIndividual(t, d, req, "Hello", true)

	require.NoError(t, d.svc.SendToIndividual(context.Background(), req))
	require.NotNil(t, f.recorded)

	assert.Nil(t, f.recorded.BroadcastID, "a test send must not count in the broadcast's statistics")
	require.NotNil(t, f.recorded.ListID)
	assert.Equal(t, "list1", *f.recorded.ListID, "the list still gives the unsubscribe link its target")
	assert.Equal(t, int64(3), f.recorded.TemplateVersion)
	assert.Empty(t, f.recorded.MessageData.Data["view_in_browser_url"])
	assert.Empty(t, f.compiled.TemplateData["view_in_browser_url"])
}

// The subject is rendered with Liquid, as a real send renders it.
func TestBroadcastService_SendToIndividual_RendersTheSubject(t *testing.T) {
	t.Run("a Liquid subject arrives rendered", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()

		req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "to@example.com"}
		f := setupSendToIndividual(t, d, req, "Hello {{ broadcast.name | upcase }}", true)

		require.NoError(t, d.svc.SendToIndividual(context.Background(), req))
		assert.Equal(t, "Hello TEST BROADCAST", f.sent.Subject)
	})

	t.Run("an invalid Liquid subject fails the send", func(t *testing.T) {
		d := setupBroadcastSvc(t)
		defer d.ctrl.Finish()

		req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "to@example.com"}
		setupSendToIndividual(t, d, req, "Hello {% if %}", false)

		err := d.svc.SendToIndividual(context.Background(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to process subject")
	})
}
