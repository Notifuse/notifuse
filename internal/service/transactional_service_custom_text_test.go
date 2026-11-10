package service

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionalNotificationService_TestTemplate_CustomText(t *testing.T) {
	text := func(s string) *string { return &s }
	tree := &notifuse_mjml.MJMLBlock{BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)}
	template := &domain.Template{
		ID:      "tpl",
		Version: 4,
		Email:   &domain.EmailTemplate{Subject: "Subject", VisualEditorTree: tree, Text: text("Hi {{ contact.email }}")},
		Translations: map[string]domain.TemplateTranslation{
			"fr": {Email: &domain.EmailTemplate{Subject: "Sujet", VisualEditorTree: tree, Text: text("Salut")}},
			"de": {Email: &domain.EmailTemplate{Subject: "Betreff", VisualEditorTree: tree}},
		},
	}
	html := "<p>Hello</p>"

	cases := []struct {
		name         string
		language     string
		wantReqText  *string
		response     *domain.CompileTemplateResponse
		wantText     string
		wantWarnings int
	}{
		{"default language custom text", "", text("Hi {{ contact.email }}"),
			&domain.CompileTemplateResponse{Success: true, HTML: &html, Text: text("Hi a@e.com"), TextSource: notifuse_mjml.TextSourceCustom}, "Hi a@e.com", 0},
		{"translation with its own text", "fr", text("Salut"),
			&domain.CompileTemplateResponse{Success: true, HTML: &html, Text: text("Salut"), TextSource: notifuse_mjml.TextSourceCustom}, "Salut", 0},
		{"translation without a text compiles none, not the default's", "de", nil,
			&domain.CompileTemplateResponse{Success: true, HTML: &html}, "", 0},
		{"a text that cannot be rendered is logged and left to SendEmail", "", text("Hi {{ contact.email }}"),
			&domain.CompileTemplateResponse{Success: true, HTML: &html, TextError: "Liquid syntax error"}, "", 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTemplateService := mocks.NewMockTemplateService(ctrl)
			mockContactService := mocks.NewMockContactService(ctrl)
			mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
			mockMsgHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
			mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			mockAuthService := mocks.NewMockAuthService(ctrl)
			mockLogger := pkgmocks.NewMockLogger(ctrl)
			mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Warn(gomock.Any()).Times(tc.wantWarnings)

			service := &TransactionalNotificationService{
				messageHistoryRepo: mockMsgHistoryRepo,
				templateService:    mockTemplateService,
				contactService:     mockContactService,
				emailService:       mockEmailService,
				logger:             mockLogger,
				workspaceRepo:      mockWorkspaceRepo,
				apiEndpoint:        "https://api.example.com",
				authService:        mockAuthService,
			}

			mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws").
				Return(context.Background(), &domain.User{ID: "u"}, &domain.UserWorkspace{
					UserID: "u", WorkspaceID: "ws", Role: "member",
					Permissions: domain.UserPermissions{domain.PermissionResourceTransactional: {Read: true, Write: true}},
				}, nil)
			mockTemplateService.EXPECT().GetTemplateByID(gomock.Any(), "ws", "tpl", int64(0)).Return(template, nil)
			mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "ws").Return(&domain.Workspace{
				ID:       "ws",
				Settings: domain.WorkspaceSettings{SecretKey: "k", DefaultLanguage: "en"},
				Integrations: []domain.Integration{{
					ID: "int", Name: "I", Type: "email",
					EmailProvider: domain.EmailProvider{
						Kind:    domain.EmailProviderKindSparkPost,
						Senders: []domain.EmailSender{{ID: "sender", Email: "s@e.com", Name: "S"}},
					},
				}},
			}, nil)
			mockContactService.EXPECT().UpsertContact(gomock.Any(), "ws", gomock.Any()).
				Return(domain.UpsertContactOperation{Email: "a@e.com", Action: domain.UpsertContactOperationUpdate})
			mockContactService.EXPECT().GetContactByEmail(gomock.Any(), "ws", "a@e.com").Return(&domain.Contact{Email: "a@e.com"}, nil)
			mockTemplateService.EXPECT().CompileTemplate(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, req domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
					assert.Equal(t, tc.wantReqText, req.Text)
					return tc.response, nil
				})
			var sent domain.SendEmailProviderRequest
			mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, req domain.SendEmailProviderRequest, _ bool) error {
					sent = req
					return nil
				})
			mockMsgHistoryRepo.EXPECT().Create(gomock.Any(), "ws", gomock.Any(), gomock.Any()).Return(nil)

			err := service.TestTemplate(context.Background(), "ws", "tpl", "int", "sender", "a@e.com", tc.language, "", false, domain.EmailOptions{})
			require.NoError(t, err)
			assert.Equal(t, tc.wantText, sent.TextContent)
		})
	}
}
