package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A test send of a broadcast records a message like the real send, so it gets a web
// version link too: the operator sees in the test email what recipients will get.
func TestBroadcastService_SendToIndividual_RecordsViewInBrowserURL(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "to@example.com"}
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
		Channel: "email",
		Email: &domain.EmailTemplate{
			SenderID:         sender.ID,
			Subject:          "Hello",
			VisualEditorTree: createMJMLRootBlock(),
		},
		Category:  string(domain.TemplateCategoryMarketing),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil)

	var compiled domain.CompileTemplateRequest
	compiledHTML := "<html>ok</html>"
	d.templateSvc.EXPECT().CompileTemplate(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, payload domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
		compiled = payload
		return &domain.CompileTemplateResponse{Success: true, HTML: &compiledHTML}, nil
	})
	d.emailSvc.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)

	var recorded *domain.MessageHistory
	d.messageHistoryRepo.EXPECT().Create(gomock.Any(), req.WorkspaceID, gomock.Any(), gomock.Any()).Do(
		func(_ context.Context, _ string, _ string, msg *domain.MessageHistory) { recorded = msg },
	).Return(nil)

	require.NoError(t, d.svc.SendToIndividual(ctx, req))
	require.NotNil(t, recorded)

	link, ok := recorded.MessageData.Data["view_in_browser_url"].(string)
	require.True(t, ok)
	assert.True(t, strings.Contains(link, "/v/"), link)
	assert.Equal(t, link, compiled.TemplateData["view_in_browser_url"], "the compiled email carries the recorded link")
}
