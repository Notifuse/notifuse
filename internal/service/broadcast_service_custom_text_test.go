package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroadcastService_SendToIndividual_CustomText(t *testing.T) {
	text := func(s string) *string { return &s }
	sender := domain.NewEmailSender("from@example.com", "From")
	template := &domain.Template{
		ID: "tplA", Name: "A", Channel: "email", Category: string(domain.TemplateCategoryMarketing),
		Email: &domain.EmailTemplate{SenderID: sender.ID, Subject: "Hello", VisualEditorTree: createMJMLRootBlock(), Text: text("Hi")},
		Translations: map[string]domain.TemplateTranslation{
			"fr": {Email: &domain.EmailTemplate{SenderID: sender.ID, Subject: "Bonjour", VisualEditorTree: createMJMLRootBlock(), Text: text("Salut")}},
			"de": {Email: &domain.EmailTemplate{SenderID: sender.ID, Subject: "Hallo", VisualEditorTree: createMJMLRootBlock()}},
		},
	}
	html := "<html>ok</html>"

	cases := []struct {
		name        string
		language    string
		wantReqText *string
		response    *domain.CompileTemplateResponse
		wantText    string
	}{
		{"default language custom text", "", text("Hi"),
			&domain.CompileTemplateResponse{Success: true, HTML: &html, Text: text("Hi"), TextSource: notifuse_mjml.TextSourceCustom}, "Hi"},
		{"translation with its own text", "fr", text("Salut"),
			&domain.CompileTemplateResponse{Success: true, HTML: &html, Text: text("Salut"), TextSource: notifuse_mjml.TextSourceCustom}, "Salut"},
		{"translation without a text compiles none, not the default's", "de", nil,
			&domain.CompileTemplateResponse{Success: true, HTML: &html}, ""},
		{"a text that cannot be rendered is left to SendEmail", "", text("Hi"),
			&domain.CompileTemplateResponse{Success: true, HTML: &html, TextError: "Liquid syntax error"}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := setupBroadcastSvc(t)
			defer d.ctrl.Finish()

			ctx := context.Background()
			req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "to@example.com"}
			authOK(d.authService, ctx, req.WorkspaceID)
			d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(&domain.Workspace{
				ID:       "w1",
				Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt", SecretKey: "sk_test", DefaultLanguage: "en"},
				Integrations: domain.Integrations{
					{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{sender}}},
				},
			}, nil)
			b := testBroadcast(req.WorkspaceID, req.BroadcastID)
			b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
			d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.BroadcastID).Return(b, nil)
			if tc.language == "" {
				d.contactRepo.EXPECT().GetContactByEmail(ctx, req.WorkspaceID, req.RecipientEmail).Return(nil, errors.New("not found")).AnyTimes()
			} else {
				d.contactRepo.EXPECT().GetContactByEmail(ctx, req.WorkspaceID, req.RecipientEmail).
					Return(&domain.Contact{Email: req.RecipientEmail, Language: &domain.NullableString{String: tc.language}}, nil).AnyTimes()
			}
			d.templateSvc.EXPECT().GetTemplateByID(ctx, req.WorkspaceID, "tplA", int64(0)).Return(template, nil)
			d.templateSvc.EXPECT().CompileTemplate(ctx, gomock.Any()).
				DoAndReturn(func(_ context.Context, r domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
					assert.Equal(t, tc.wantReqText, r.Text)
					return tc.response, nil
				})
			var sent domain.SendEmailProviderRequest
			d.emailSvc.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
				DoAndReturn(func(_ context.Context, r domain.SendEmailProviderRequest, _ bool) error {
					sent = r
					return nil
				})
			d.messageHistoryRepo.EXPECT().Create(gomock.Any(), req.WorkspaceID, gomock.Any(), gomock.Any()).Return(nil)

			require.NoError(t, d.svc.SendToIndividual(ctx, req))
			assert.Equal(t, tc.wantText, sent.TextContent)
		})
	}
}
