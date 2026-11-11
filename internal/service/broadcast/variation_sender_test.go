package broadcast

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// variationSenderFixture: an integration with a default sender and a second one the
// broadcast chooses, and a template on the default one. domain.NewEmailSender marks every
// sender default, so the second is unmarked by hand.
type variationSenderFixture struct {
	provider *domain.EmailProvider
	promos   domain.EmailSender
	template *domain.Template
}

func newVariationSenderFixture() variationSenderFixture {
	byDefault := domain.NewEmailSender("hello@example.com", "Default")
	promos := domain.NewEmailSender("promos@example.com", "Promos")
	promos.IsDefault = false
	return variationSenderFixture{
		provider: &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{byDefault, promos},
			SMTP:    &domain.SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "user", Password: "pass", UseTLS: true},
		},
		promos: promos,
		template: &domain.Template{
			ID: "template-1",
			Email: &domain.EmailTemplate{
				SenderID:         byDefault.ID,
				Subject:          "Hello",
				VisualEditorTree: createQueueValidTestTree(createQueueTestTextBlock("txt1", "Hello")),
			},
		},
	}
}

func broadcastWithVariationSender(senderID string) *domain.Broadcast {
	return &domain.Broadcast{
		ID:            "broadcast-1",
		WorkspaceID:   "workspace-1",
		UTMParameters: &domain.UTMParameters{},
		TestSettings: domain.BroadcastTestSettings{
			Variations: []domain.BroadcastVariation{{VariationName: "A", TemplateID: "template-1", SenderID: senderID}},
		},
	}
}

func quietLogger(ctrl *gomock.Controller) *pkgmocks.MockLogger {
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Warn(gomock.Any()).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()
	return logger
}

func TestQueueMessageSender_VariationSender(t *testing.T) {
	t.Run("the queued email leaves from the variation's sender", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newVariationSenderFixture()
		qms := NewQueueMessageSender(mocks.NewMockEmailQueueRepository(ctrl), mocks.NewMockBroadcastRepository(ctrl),
			mocks.NewMockMessageHistoryRepository(ctrl), mocks.NewMockTemplateRepository(ctrl), nil, quietLogger(ctrl), nil,
			"https://api.example.com").(*queueMessageSender)

		entry, err := qms.buildQueueEntry(context.Background(), "workspace-1", "integration-1", "", "https://api.test.com",
			testTrackingDecision(true), nil, broadcastWithVariationSender(f.promos.ID), "msg-1", "to@example.com",
			f.template, map[string]interface{}{}, f.provider, "", "")

		require.NoError(t, err)
		assert.Equal(t, "promos@example.com", entry.Payload.FromAddress)
		assert.Equal(t, "Promos", entry.Payload.FromName)
	})

	t.Run("without a variation sender the template's is used", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newVariationSenderFixture()
		qms := NewQueueMessageSender(mocks.NewMockEmailQueueRepository(ctrl), mocks.NewMockBroadcastRepository(ctrl),
			mocks.NewMockMessageHistoryRepository(ctrl), mocks.NewMockTemplateRepository(ctrl), nil, quietLogger(ctrl), nil,
			"https://api.example.com").(*queueMessageSender)

		entry, err := qms.buildQueueEntry(context.Background(), "workspace-1", "integration-1", "", "https://api.test.com",
			testTrackingDecision(true), nil, broadcastWithVariationSender(""), "msg-1", "to@example.com",
			f.template, map[string]interface{}{}, f.provider, "", "")

		require.NoError(t, err)
		assert.Equal(t, "hello@example.com", entry.Payload.FromAddress)
	})

	t.Run("a batch whose variation sender has gone stops before any recipient", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newVariationSenderFixture()
		queueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		broadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
		broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-1", "broadcast-1").Return(broadcastWithVariationSender("sender-since-deleted"), nil)
		queueRepo.EXPECT().Enqueue(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		sender := NewQueueMessageSender(queueRepo, broadcastRepo, mocks.NewMockMessageHistoryRepository(ctrl),
			mocks.NewMockTemplateRepository(ctrl), nil, quietLogger(ctrl), nil, "https://api.example.com")

		recipients := []*domain.ContactWithList{{Contact: &domain.Contact{Email: "user@example.com"}, ListID: "list-1"}}
		sent, failed, err := sender.SendBatch(context.Background(), "workspace-1", "integration-1", "secret-key",
			"https://api.example.com", "", testTrackingPolicy(true), nil, "broadcast-1", recipients,
			map[string]*domain.Template{"template-1": f.template}, f.provider, time.Now().Add(time.Minute), "")

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrBroadcastShouldPause))
		assert.True(t, errors.Is(err, domain.ErrSenderNotFound))
		assert.Equal(t, 0, sent)
		assert.Equal(t, 0, failed, "the recipients are not dropped as failed: the batch is retried once fixed")
	})
}

func TestMessageSender_VariationSender(t *testing.T) {
	newSender := func(ctrl *gomock.Controller, broadcastRepo *mocks.MockBroadcastRepository, emailService *mocks.MockEmailServiceInterface) MessageSender {
		return NewMessageSender(broadcastRepo, mocks.NewMockMessageHistoryRepository(ctrl), mocks.NewMockTemplateRepository(ctrl),
			emailService, nil, quietLogger(ctrl), TestConfig(), "")
	}

	t.Run("the email leaves from the variation's sender", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newVariationSenderFixture()
		emailService := mocks.NewMockEmailServiceInterface(ctrl)
		emailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, req domain.SendEmailProviderRequest, _ bool) error {
				assert.Equal(t, "promos@example.com", req.FromAddress)
				assert.Equal(t, "Promos", req.FromName)
				return nil
			})
		sender := newSender(ctrl, mocks.NewMockBroadcastRepository(ctrl), emailService)

		err := sender.SendToRecipient(context.Background(), "workspace-1", "integration-1", "", "https://api.test.com",
			testTrackingDecision(true), nil, broadcastWithVariationSender(f.promos.ID), "msg-1", "to@example.com",
			f.template, map[string]interface{}{}, f.provider, time.Now().Add(time.Minute), "", "")
		require.NoError(t, err)
	})

	t.Run("a variation sender that has gone is an error, never the default sender", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newVariationSenderFixture()
		sender := newSender(ctrl, mocks.NewMockBroadcastRepository(ctrl), mocks.NewMockEmailServiceInterface(ctrl))

		err := sender.SendToRecipient(context.Background(), "workspace-1", "integration-1", "", "https://api.test.com",
			testTrackingDecision(true), nil, broadcastWithVariationSender("sender-since-deleted"), "msg-1", "to@example.com",
			f.template, map[string]interface{}{}, f.provider, time.Now().Add(time.Minute), "", "")
		require.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrSenderNotFound))
	})

	t.Run("a batch whose variation sender has gone stops before any recipient", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newVariationSenderFixture()
		broadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
		broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-1", "broadcast-1").Return(broadcastWithVariationSender("sender-since-deleted"), nil)
		sender := newSender(ctrl, broadcastRepo, mocks.NewMockEmailServiceInterface(ctrl))

		recipients := []*domain.ContactWithList{{Contact: &domain.Contact{Email: "user@example.com"}, ListID: "list-1"}}
		sent, failed, err := sender.SendBatch(context.Background(), "workspace-1", "integration-1", "secret-key",
			"https://api.example.com", "", testTrackingPolicy(true), nil, "broadcast-1", recipients,
			map[string]*domain.Template{"template-1": f.template}, f.provider, time.Now().Add(time.Minute), "")

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrBroadcastShouldPause))
		assert.True(t, errors.Is(err, domain.ErrSenderNotFound))
		assert.Equal(t, 0, sent)
		assert.Equal(t, 0, failed)
	})
}
