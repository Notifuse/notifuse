package service

import (
	"context"
	"net/url"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newReplyTestService wires an InboundWebhookEventService with all-mock deps and
// a Mailgun workspace/integration ready for inbound-reply tests.
func newReplyTestService(t *testing.T) (*InboundWebhookEventService, *mocks.MockInboundWebhookEventRepository, *mocks.MockWorkspaceRepository, *mocks.MockContactRepository, string, string) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)

	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()

	workspaceID := "workspace1"
	integrationID := "integration1"

	workspace := &domain.Workspace{
		ID: workspaceID,
		Integrations: []domain.Integration{
			{
				ID: integrationID,
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindMailgun,
				},
			},
		},
	}
	workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil).AnyTimes()

	svc := &InboundWebhookEventService{
		repo:               repo,
		authService:        authService,
		logger:             log,
		workspaceRepo:      workspaceRepo,
		messageHistoryRepo: messageHistoryRepo,
		contactRepo:        contactRepo,
	}
	return svc, repo, workspaceRepo, contactRepo, workspaceID, integrationID
}

func TestProcessInboundReply_KnownContact(t *testing.T) {
	svc, repo, _, contactRepo, workspaceID, integrationID := newReplyTestService(t)

	form := url.Values{}
	form.Set("sender", "jane@example.com")
	form.Set("from", "Jane Doe <jane@example.com>")
	form.Set("subject", "Re: Welcome aboard")
	form.Set("Message-Id", "<reply-456@mail.example.com>")
	form.Set("In-Reply-To", "<orig-123@notifuse>")
	form.Set("timestamp", "1700000000")

	contactRepo.EXPECT().
		GetContactByEmail(gomock.Any(), workspaceID, "jane@example.com").
		Return(&domain.Contact{Email: "jane@example.com"}, nil)

	repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, events []*domain.InboundWebhookEvent) error {
			require.Len(t, events, 1)
			ev := events[0]
			assert.Equal(t, domain.EmailEventReply, ev.Type)
			assert.Equal(t, domain.WebhookSourceMailgun, ev.Source)
			assert.Equal(t, integrationID, ev.IntegrationID)
			// RecipientEmail holds the matched contact (the reply's sender).
			assert.Equal(t, "jane@example.com", ev.RecipientEmail)
			// In-Reply-To is preferred and stripped of angle brackets.
			require.NotNil(t, ev.MessageID)
			assert.Equal(t, "orig-123@notifuse", *ev.MessageID)
			return nil
		})

	err := svc.ProcessInboundReply(context.Background(), workspaceID, integrationID, form)
	assert.NoError(t, err)
}

func TestProcessInboundReply_UnknownContactIsIgnored(t *testing.T) {
	svc, repo, _, contactRepo, workspaceID, integrationID := newReplyTestService(t)

	form := url.Values{}
	form.Set("sender", "stranger@example.com")

	contactRepo.EXPECT().
		GetContactByEmail(gomock.Any(), workspaceID, "stranger@example.com").
		Return(nil, nil)
	// No StoreEvents expected — unknown senders are skipped.
	repo.EXPECT().StoreEvents(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := svc.ProcessInboundReply(context.Background(), workspaceID, integrationID, form)
	assert.NoError(t, err)
}

func TestProcessInboundReply_FallsBackToFromHeader(t *testing.T) {
	svc, repo, _, contactRepo, workspaceID, integrationID := newReplyTestService(t)

	// No "sender" field — must extract the address from the "from" header,
	// and lowercase it.
	form := url.Values{}
	form.Set("from", "Jane Doe <Jane@Example.com>")
	form.Set("Message-Id", "<reply-456@mail.example.com>")

	contactRepo.EXPECT().
		GetContactByEmail(gomock.Any(), workspaceID, "jane@example.com").
		Return(&domain.Contact{Email: "jane@example.com"}, nil)

	repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, events []*domain.InboundWebhookEvent) error {
			require.Len(t, events, 1)
			assert.Equal(t, "jane@example.com", events[0].RecipientEmail)
			// Falls back to the reply's own Message-Id when In-Reply-To absent.
			require.NotNil(t, events[0].MessageID)
			assert.Equal(t, "reply-456@mail.example.com", *events[0].MessageID)
			return nil
		})

	err := svc.ProcessInboundReply(context.Background(), workspaceID, integrationID, form)
	assert.NoError(t, err)
}

func TestProcessInboundReply_MissingSender(t *testing.T) {
	svc, _, _, _, workspaceID, integrationID := newReplyTestService(t)

	form := url.Values{} // no sender, no from
	err := svc.ProcessInboundReply(context.Background(), workspaceID, integrationID, form)
	assert.Error(t, err)
}

func TestProcessInboundReply_UnsupportedProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceID := "workspace1"
	integrationID := "integration1"
	workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID: workspaceID,
		Integrations: []domain.Integration{
			{ID: integrationID, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSES}},
		},
	}, nil)

	svc := &InboundWebhookEventService{
		logger:        log,
		workspaceRepo: workspaceRepo,
	}

	form := url.Values{}
	form.Set("sender", "jane@example.com")
	err := svc.ProcessInboundReply(context.Background(), workspaceID, integrationID, form)
	assert.Error(t, err)
}
