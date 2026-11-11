package service

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/Notifuse/notifuse/pkg/plaintext"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailService_SendEmailForTemplate_CustomText(t *testing.T) {
	text := func(s string) *string { return &s }
	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	tree := &notifuse_mjml.MJMLBlock{BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)}
	template := &domain.Template{
		ID:      "template-1",
		Version: 2,
		Email:   &domain.EmailTemplate{Subject: "Subject", SenderID: emailSender.ID, VisualEditorTree: tree, Text: text("Hi {{ name }}")},
		Translations: map[string]domain.TemplateTranslation{
			"fr": {Email: &domain.EmailTemplate{Subject: "Sujet", SenderID: emailSender.ID, VisualEditorTree: tree, Text: text("Salut {{ name }}")}},
			"de": {Email: &domain.EmailTemplate{Subject: "Betreff", SenderID: emailSender.ID, VisualEditorTree: tree}},
		},
	}
	html := "<p>Hello from the HTML</p>"
	rendered := "Hi Ada"

	cases := []struct {
		name         string
		language     string
		wantReqText  *string
		response     *domain.CompileTemplateResponse
		wantText     string
		wantWarnings int
	}{
		{
			name: "default language: its custom text reaches the provider", language: "",
			wantReqText: text("Hi {{ name }}"),
			response:    &domain.CompileTemplateResponse{Success: true, HTML: &html, Text: &rendered, TextSource: notifuse_mjml.TextSourceCustom},
			wantText:    "Hi Ada",
		},
		{
			name: "translation with its own text compiles it", language: "fr",
			wantReqText: text("Salut {{ name }}"),
			response:    &domain.CompileTemplateResponse{Success: true, HTML: &html, Text: text("Salut Ada"), TextSource: notifuse_mjml.TextSourceCustom},
			wantText:    "Salut Ada",
		},
		{
			name: "translation without a text compiles none, not the default's", language: "de",
			wantReqText: nil,
			response:    &domain.CompileTemplateResponse{Success: true, HTML: &html},
			wantText:    plaintext.FromHTML(html),
		},
		{
			name: "a text that cannot be rendered sends the automatic one and is logged", language: "",
			wantReqText:  text("Hi {{ name }}"),
			response:     &domain.CompileTemplateResponse{Success: true, HTML: &html, TextError: "Liquid syntax error"},
			wantText:     plaintext.FromHTML(html),
			wantWarnings: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockLogger := pkgmocks.NewMockLogger(ctrl)
			mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Warn(gomock.Any()).Times(tc.wantWarnings)

			mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			mockTemplateService := mocks.NewMockTemplateService(ctrl)
			mockMessageRepo := mocks.NewMockMessageHistoryRepository(ctrl)
			mockSES := mocks.NewMockEmailProviderService(ctrl)
			svc := EmailService{
				logger:          mockLogger,
				workspaceRepo:   mockWorkspaceRepo,
				templateService: mockTemplateService,
				messageRepo:     mockMessageRepo,
				sesService:      mockSES,
			}

			mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "ws").
				Return(&domain.Workspace{ID: "ws", Settings: domain.WorkspaceSettings{DefaultLanguage: "en"}}, nil)
			mockTemplateService.EXPECT().GetTemplateByID(gomock.Any(), "ws", "template-1", int64(0)).Return(template, nil)
			mockTemplateService.EXPECT().CompileTemplate(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, req domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
					assert.Equal(t, tc.wantReqText, req.Text)
					return tc.response, nil
				})
			mockMessageRepo.EXPECT().Create(gomock.Any(), "ws", gomock.Any(), gomock.Any()).Return(nil)
			var sent domain.SendEmailProviderRequest
			mockSES.EXPECT().SendEmail(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, req domain.SendEmailProviderRequest) error {
					sent = req
					return nil
				})

			contact := &domain.Contact{Email: "ada@example.com"}
			if tc.language != "" {
				contact.Language = &domain.NullableString{String: tc.language}
			}
			err := svc.SendEmailForTemplate(context.Background(), domain.SendEmailRequest{
				WorkspaceID:    "ws",
				IntegrationID:  "int",
				MessageID:      "msg",
				Contact:        contact,
				TemplateConfig: domain.ChannelTemplate{TemplateID: "template-1"},
				MessageData:    domain.MessageData{Data: map[string]interface{}{"name": "Ada"}},
				EmailProvider: &domain.EmailProvider{
					Kind:    domain.EmailProviderKindSES,
					Senders: []domain.EmailSender{emailSender},
					SES:     &domain.AmazonSESSettings{Region: "us-east-1", AccessKey: "a", SecretKey: "s"},
				},
			})
			require.NoError(t, err)
			assert.Equal(t, tc.wantText, sent.TextContent)
		})
	}
}
