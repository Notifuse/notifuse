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

// A broadcast email gets a link to its web version, stored with the rest of the
// template data so the web version can be rendered from message history.
func TestQueueMessageSender_SendBatchFillsViewInBrowserURL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	emailSender := domain.NewEmailSender("sender@example.com", "Test Sender")
	emailProvider := &domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{emailSender}}
	template := &domain.Template{
		ID:      "template-1",
		Version: 3,
		Email: &domain.EmailTemplate{
			SenderID:         emailSender.ID,
			Subject:          "News",
			VisualEditorTree: createQueueValidTestTree(createQueueTestTextBlock("txt1", "<a href=\"{{ view_in_browser_url }}\">Web version</a>")),
		},
	}

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-1", "broadcast-1").Return(&domain.Broadcast{
		ID:            "broadcast-1",
		WorkspaceID:   "workspace-1",
		Name:          "News",
		Audience:      domain.AudienceSettings{List: "list-123", ExcludeUnsubscribed: true},
		UTMParameters: &domain.UTMParameters{},
	}, nil)

	var enqueued []*domain.EmailQueueEntry
	mockQueueRepo.EXPECT().Enqueue(gomock.Any(), "workspace-1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, entries []*domain.EmailQueueEntry) error {
			enqueued = entries
			return nil
		})

	sender := NewQueueMessageSender(
		mockQueueRepo, mockBroadcastRepo,
		mocks.NewMockMessageHistoryRepository(ctrl), mocks.NewMockTemplateRepository(ctrl),
		nil, mockLogger, nil, "https://api.example.com",
	)

	recipients := []*domain.ContactWithList{{Contact: &domain.Contact{Email: "user@example.com"}, ListID: "list-123", ListName: "Customers"}}
	sent, _, err := sender.SendBatch(
		context.Background(), "workspace-1", "integration-1", "test-secret-key",
		"https://api.example.com", "", testTrackingPolicy(true), nil, "broadcast-1", recipients,
		map[string]*domain.Template{"template-1": template}, emailProvider,
		time.Now().Add(5*time.Minute), "",
	)
	require.NoError(t, err)
	require.Equal(t, 1, sent)
	require.Len(t, enqueued, 1)

	link, ok := enqueued[0].Payload.TemplateData["view_in_browser_url"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(link, "https://api.example.com/v/"), link)
	// With click tracking on, the link reaches the email wrapped in a /r/ redirect: what
	// matters here is that it is no longer the empty href it rendered before.
	assert.NotContains(t, enqueued[0].Payload.HTMLContent, `href=""`, "the compiled email links to its web version")
}
