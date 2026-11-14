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

// queueBatchFixture is a queue sender whose queue repository has no expectation
// unless a test adds one: an Enqueue a test does not expect fails it.
type queueBatchFixture struct {
	queueRepo     *mocks.MockEmailQueueRepository
	broadcastRepo *mocks.MockBroadcastRepository
	sender        *queueMessageSender
	provider      *domain.EmailProvider
	senderID      string
}

func newQueueBatchFixture(t *testing.T) *queueBatchFixture {
	ctrl := gomock.NewController(t)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Debug(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	f := &queueBatchFixture{
		queueRepo:     mocks.NewMockEmailQueueRepository(ctrl),
		broadcastRepo: mocks.NewMockBroadcastRepository(ctrl),
	}
	emailSender := domain.NewEmailSender("sender@example.com", "Test Sender")
	f.senderID = emailSender.ID
	f.provider = &domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{emailSender}}
	f.sender = NewQueueMessageSender(f.queueRepo, f.broadcastRepo, mocks.NewMockMessageHistoryRepository(ctrl),
		mocks.NewMockTemplateRepository(ctrl), nil, log, nil, "https://api.example.com").(*queueMessageSender)
	return f
}

func (f *queueBatchFixture) template(subject string) *domain.Template {
	return &domain.Template{
		ID: "template-1",
		Email: &domain.EmailTemplate{
			SenderID:         f.senderID,
			Subject:          subject,
			VisualEditorTree: createQueueValidTestTree(createQueueTestTextBlock("txt1", "Hello")),
		},
	}
}

func (f *queueBatchFixture) sendBatch(recipients []*domain.ContactWithList, templates map[string]*domain.Template,
	timeoutAt time.Time) (int, int, error) {
	return f.sender.SendBatch(context.Background(), "workspace-1", "integration-1", "secret-key",
		"https://api.example.com", "", testTrackingPolicy(true), nil, "broadcast-1", recipients, templates,
		f.provider, timeoutAt, "en")
}

func queueRecipients(emails ...string) []*domain.ContactWithList {
	recipients := make([]*domain.ContactWithList, 0, len(emails))
	for _, email := range emails {
		recipients = append(recipients, &domain.ContactWithList{Contact: &domain.Contact{Email: email}, ListID: "list-1"})
	}
	return recipients
}

func queueBroadcast() *domain.Broadcast {
	return &domain.Broadcast{ID: "broadcast-1", WorkspaceID: "workspace-1", Name: "Test", UTMParameters: &domain.UTMParameters{}}
}

// Without its broadcast the batch cannot be built: every recipient counts as
// failed and nothing is enqueued.
func TestQueueSendBatch_BroadcastLookupFails(t *testing.T) {
	f := newQueueBatchFixture(t)
	f.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-1", "broadcast-1").Return(nil, errors.New("db down"))

	sent, failed, err := f.sendBatch(queueRecipients("a@example.com", "b@example.com"),
		map[string]*domain.Template{"template-1": f.template("Hi")}, time.Now().Add(time.Minute))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get broadcast")
	assert.Equal(t, 0, sent)
	assert.Equal(t, 2, failed)
}

// A batch in which no entry can be built counts every recipient as failed,
// without an error that would pause the broadcast, and enqueues nothing.
func TestQueueSendBatch_NoEntryBuilt_NothingEnqueued(t *testing.T) {
	cases := []struct {
		name      string
		templates func(f *queueBatchFixture) map[string]*domain.Template
	}{
		{name: "no template to send", templates: func(*queueBatchFixture) map[string]*domain.Template {
			return map[string]*domain.Template{}
		}},
		{name: "subject that does not render", templates: func(f *queueBatchFixture) map[string]*domain.Template {
			return map[string]*domain.Template{"template-1": f.template("Hi {{ contact.first_name ")}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newQueueBatchFixture(t)
			f.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-1", "broadcast-1").Return(queueBroadcast(), nil)

			sent, failed, err := f.sendBatch(queueRecipients("a@example.com", "b@example.com"), tc.templates(f),
				time.Now().Add(time.Minute))

			require.NoError(t, err)
			assert.Equal(t, 0, sent)
			assert.Equal(t, 2, failed)
		})
	}
}

// A batch handed over after its deadline builds nothing and enqueues nothing;
// the recipients are left for the next run.
func TestQueueSendBatch_DeadlinePassed_NothingEnqueued(t *testing.T) {
	f := newQueueBatchFixture(t)
	f.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-1", "broadcast-1").Return(queueBroadcast(), nil)

	sent, failed, err := f.sendBatch(queueRecipients("a@example.com"),
		map[string]*domain.Template{"template-1": f.template("Hi")}, time.Now().Add(-time.Second))

	require.NoError(t, err)
	assert.Equal(t, 0, sent)
	assert.Equal(t, 0, failed)
}

// Each contact gets the translation in its own language, and a contact without
// a language gets the default.
func TestQueueSendBatch_ContactLanguagePicksTheTranslation(t *testing.T) {
	f := newQueueBatchFixture(t)
	f.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-1", "broadcast-1").Return(queueBroadcast(), nil)
	template := f.template("Hello")
	template.Translations = map[string]domain.TemplateTranslation{
		"fr": {Email: &domain.EmailTemplate{
			Subject:          "Bonjour",
			VisualEditorTree: createQueueValidTestTree(createQueueTestTextBlock("txt1", "Bonjour")),
		}},
	}
	recipients := queueRecipients("marie@example.com", "john@example.com")
	recipients[0].Contact.Language = &domain.NullableString{String: "fr"}

	var subjects map[string]string
	f.queueRepo.EXPECT().Enqueue(gomock.Any(), "workspace-1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, entries []*domain.EmailQueueEntry) error {
			subjects = map[string]string{}
			for _, e := range entries {
				subjects[e.ContactEmail] = e.Payload.Subject
			}
			return nil
		})

	sent, failed, err := f.sendBatch(recipients, map[string]*domain.Template{"template-1": template}, time.Now().Add(time.Minute))

	require.NoError(t, err)
	assert.Equal(t, 2, sent)
	assert.Equal(t, 0, failed)
	assert.Equal(t, map[string]string{"marie@example.com": "Bonjour", "john@example.com": "Hello"}, subjects)
}
