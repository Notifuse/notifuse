package broadcast

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueueMessageSender_BuildQueueEntry_CustomText(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	sender := NewQueueMessageSender(
		mocks.NewMockEmailQueueRepository(ctrl),
		mocks.NewMockBroadcastRepository(ctrl),
		mocks.NewMockMessageHistoryRepository(ctrl),
		mocks.NewMockTemplateRepository(ctrl),
		nil,
		mockLogger,
		nil,
		"https://api.example.com",
	)
	qms := sender.(*queueMessageSender)

	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	provider := &domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{emailSender}, RateLimitPerMinute: 100}
	broadcast := &domain.Broadcast{ID: "broadcast-1", WorkspaceID: "workspace-1"}
	data := map[string]interface{}{"contact": map[string]interface{}{"first_name": "Ada"}}
	text := func(s string) *string { return &s }
	email := func(body string, custom *string) *domain.EmailTemplate {
		return &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Subject",
			VisualEditorTree: createQueueValidTestTree(createQueueTestTextBlock("txt1", body)),
			Text:             custom,
		}
	}
	build := func(t *testing.T, template *domain.Template, language string) *domain.EmailQueueEntry {
		t.Helper()
		entry, err := qms.buildQueueEntry(context.Background(), "workspace-1", "integration-1", "", "https://api.test.com",
			testTrackingDecision(false), nil, broadcast, "msg-1", "ada@example.com", template, data, provider, language, "en")
		require.NoError(t, err)
		return entry
	}

	t.Run("a custom text is rendered into the payload", func(t *testing.T) {
		entry := build(t, &domain.Template{ID: "t1", Version: 1, Email: email("Hello", text("Hi {{ contact.first_name }}"))}, "")
		assert.Equal(t, "Hi Ada", entry.Payload.TextContent)
	})

	t.Run("an automatic template leaves the text to the worker", func(t *testing.T) {
		entry := build(t, &domain.Template{ID: "t1", Version: 1, Email: email("Hello", nil)}, "")
		assert.Equal(t, "", entry.Payload.TextContent)
	})

	translated := &domain.Template{
		ID:      "t1",
		Version: 1,
		Email:   email("Hello", text("Hi {{ contact.first_name }}")),
		Translations: map[string]domain.TemplateTranslation{
			"fr": {Email: email("Bonjour", text("Salut {{ contact.first_name }}"))},
			"de": {Email: email("Hallo", nil)},
		},
	}

	t.Run("a translation with its own text gets it", func(t *testing.T) {
		assert.Equal(t, "Salut Ada", build(t, translated, "fr").Payload.TextContent)
	})

	t.Run("a translation without a text gets the automatic one, not the default's", func(t *testing.T) {
		entry := build(t, translated, "de")
		assert.Equal(t, "", entry.Payload.TextContent)
		assert.Contains(t, entry.Payload.HTMLContent, "Hallo")
	})

	t.Run("a text that cannot be rendered sends the automatic one and is logged once per template version", func(t *testing.T) {
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).Times(1)
		mockLogger.EXPECT().Warn(gomock.Any()).Times(1)
		broken := &domain.Template{ID: "t2", Version: 3, Email: email("Hello", text("{% if x %}never closed"))}
		assert.Equal(t, "", build(t, broken, "").Payload.TextContent)
		assert.Equal(t, "", build(t, broken, "").Payload.TextContent)
	})
}
