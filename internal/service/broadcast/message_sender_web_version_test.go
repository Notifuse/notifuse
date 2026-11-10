package broadcast

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The direct send path records the web version link in message history, like the
// queue path, so the web version can be rendered from it.
func TestSendBatch_RecordsViewInBrowserURL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBroadcastRepository := mocks.NewMockBroadcastRepository(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).Return().AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).Return().AnyTimes()

	ctx := context.Background()
	broadcast := &domain.Broadcast{
		ID:            "broadcast-123",
		WorkspaceID:   "workspace-123",
		Name:          "Test Broadcast",
		ChannelType:   "email",
		Audience:      domain.AudienceSettings{List: "list-1"},
		UTMParameters: &domain.UTMParameters{},
		TestSettings: domain.BroadcastTestSettings{
			Variations: []domain.BroadcastVariation{{VariationName: "variation-1", TemplateID: "template-123"}},
		},
	}
	emailSender := domain.NewEmailSender("sender@example.com", "Sender")
	emailProvider := &domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{emailSender}}
	template := &domain.Template{
		ID: "template-123",
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "Test Subject",
			VisualEditorTree: createValidTestTree(createTestTextBlock("txt1", "Test content")),
		},
	}

	mockBroadcastRepository.EXPECT().GetBroadcast(ctx, "workspace-123", "broadcast-123").Return(broadcast, nil)
	mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	var recorded *domain.MessageHistory
	mockMessageHistoryRepo.EXPECT().Create(ctx, "workspace-123", gomock.Any(), gomock.Any()).
		Do(func(_ context.Context, _ string, _ string, msg *domain.MessageHistory) { recorded = msg }).
		Return(nil)

	sender := NewMessageSender(mockBroadcastRepository, mockMessageHistoryRepo, mocks.NewMockTemplateRepository(ctrl),
		mockEmailService, nil, mockLogger, TestConfig(), "")

	recipients := []*domain.ContactWithList{{Contact: &domain.Contact{Email: "recipient1@example.com"}, ListID: "list-1", ListName: "Test List"}}
	sent, _, err := sender.SendBatch(ctx, "workspace-123", "test-integration-id", "secret-key-123", "https://api.example.com", "",
		testTrackingPolicy(true), nil, "broadcast-123", recipients, map[string]*domain.Template{"template-123": template},
		emailProvider, time.Now().Add(30*time.Second), "")
	require.NoError(t, err)
	require.Equal(t, 1, sent)
	require.NotNil(t, recorded)

	link, ok := recorded.MessageData.Data["view_in_browser_url"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(link, "https://api.example.com/v/"), link)
}
