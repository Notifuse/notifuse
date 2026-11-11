package broadcast

import (
	"context"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendToRecipient_CustomText(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	sender := NewMessageSender(
		mocks.NewMockBroadcastRepository(ctrl),
		mocks.NewMockMessageHistoryRepository(ctrl),
		mocks.NewMockTemplateRepository(ctrl),
		mockEmailService,
		nil,
		mockLogger,
		TestConfig(),
		"",
	)

	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	provider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindSMTP,
		Senders: []domain.EmailSender{emailSender},
		SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
	}
	broadcast := &domain.Broadcast{ID: "broadcast-1", WorkspaceID: "workspace-1", ChannelType: "email"}
	data := map[string]interface{}{"contact": map[string]interface{}{"first_name": "Ada"}}
	text := func(s string) *string { return &s }
	email := func(body string, custom *string) *domain.EmailTemplate {
		return &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Subject",
			VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", body)),
			Text:             custom,
		}
	}
	template := &domain.Template{
		ID:    "template-1",
		Email: email("Hello", text("Hi {{ contact.first_name }}")),
		Translations: map[string]domain.TemplateTranslation{
			"fr": {Email: email("Bonjour", text("Salut {{ contact.first_name }}"))},
			"de": {Email: email("Hallo", nil)},
		},
	}
	sendFor := func(t *testing.T, tpl *domain.Template, language string) domain.SendEmailProviderRequest {
		t.Helper()
		var got domain.SendEmailProviderRequest
		mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req domain.SendEmailProviderRequest, _ bool) error {
				got = req
				return nil
			})
		err := sender.SendToRecipient(context.Background(), "workspace-1", "integration-1", "", "https://api.test.com",
			testTrackingDecision(false), nil, broadcast, "msg-"+language, "ada@example.com", tpl, data, provider,
			time.Now().Add(30*time.Second), language, "en")
		require.NoError(t, err)
		return got
	}

	t.Run("default language custom text", func(t *testing.T) {
		assert.Equal(t, "Hi Ada", sendFor(t, template, "").TextContent)
	})

	t.Run("translation with its own text", func(t *testing.T) {
		assert.Equal(t, "Salut Ada", sendFor(t, template, "fr").TextContent)
	})

	t.Run("translation without a text gets the automatic one, not the default's", func(t *testing.T) {
		req := sendFor(t, template, "de")
		assert.Equal(t, "", req.TextContent)
		assert.Contains(t, req.Content, "Hallo")
	})

	t.Run("automatic template", func(t *testing.T) {
		assert.Equal(t, "", sendFor(t, &domain.Template{ID: "t2", Email: email("Hello", nil)}, "").TextContent)
	})

	t.Run("a text that cannot be rendered sends the automatic one and is logged", func(t *testing.T) {
		mockLogger.EXPECT().Warn(gomock.Any()).Times(1)
		broken := &domain.Template{ID: "t3", Email: email("Hello", text("{% if x %}never closed"))}
		assert.Equal(t, "", sendFor(t, broken, "").TextContent)
	})
}
