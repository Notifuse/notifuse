package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type replyTestDeps struct {
	svc         *InboundWebhookEventService
	repo        *mocks.MockInboundWebhookEventRepository
	contactRepo *mocks.MockContactRepository
	mhRepo      *mocks.MockMessageHistoryRepository
	autoRepo    *mocks.MockAutomationRepository
	workspaceID string
	integID     string
}

func newReplyTestDeps(t *testing.T, kind domain.EmailProviderKind) replyTestDeps {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	repo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)
	mhRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	autoRepo := mocks.NewMockAutomationRepository(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	log.EXPECT().Debug(gomock.Any()).AnyTimes()

	workspaceID, integID := "ws-1", "int-1"

	// No Mailgun signing key set → parser.Verify skips (auth is the provider
	// signature; the workspace from GetByID is already decrypted in production).
	provider := domain.EmailProvider{Kind: kind}
	if kind == domain.EmailProviderKindMailgun {
		provider.Mailgun = &domain.MailgunSettings{}
	}

	workspace := &domain.Workspace{
		ID:           workspaceID,
		Integrations: []domain.Integration{{ID: integID, EmailProvider: provider}},
	}
	workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil).AnyTimes()

	svc := NewInboundWebhookEventService(repo, authService, log, workspaceRepo, mhRepo, contactRepo, autoRepo, &fakeListUnsubscriber{})

	return replyTestDeps{svc, repo, contactRepo, mhRepo, autoRepo, workspaceID, integID}
}

func replyReq(messageHeaders string) *domain.InboundRequest {
	return &domain.InboundRequest{Form: mailgunInboundForm(messageHeaders)}
}

func TestProcessInboundReply_MessageIDMatchExitsExactJourney(t *testing.T) {
	d := newReplyTestDeps(t, domain.EmailProviderKindMailgun)
	autoID := "auto-1"

	d.mhRepo.EXPECT().GetBySMTPMessageID(gomock.Any(), d.workspaceID, "orig-1@example.com").
		Return(&domain.MessageHistory{ContactEmail: "jane@example.com", AutomationID: &autoID}, nil)

	d.repo.EXPECT().StoreReplyEvent(gomock.Any(), d.workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, event *domain.InboundWebhookEvent) (bool, error) {
			assert.Equal(t, domain.EmailEventReply, event.Type)
			assert.Equal(t, "jane@example.com", event.RecipientEmail)
			return true, nil // newly stored
		})

	d.autoRepo.EXPECT().
		ExitContactJourneysOnReply(gomock.Any(), d.workspaceID, "jane@example.com", &autoID, "replied", gomock.Any()).
		Return(1, nil)

	headers := `[["In-Reply-To","<orig-1@example.com>"],["Message-Id","<reply-1@x>"]]`
	err := d.svc.ProcessInboundReply(context.Background(), d.workspaceID, d.integID, replyReq(headers))
	assert.NoError(t, err)
}

func TestProcessInboundReply_SESHostIndependentMatch(t *testing.T) {
	// SES stores the bare local part as smtp_message_id. A reply's In-Reply-To carries
	// "<localpart@<sub>.amazonses.com>" where the host is not contractually stable, so the
	// match must succeed regardless of which amazonses.com host SES used.
	for _, host := range []string{"email.amazonses.com", "mail.amazonses.com", "us-east-1.amazonses.com"} {
		t.Run(host, func(t *testing.T) {
			d := newReplyTestDeps(t, domain.EmailProviderKindMailgun) // Mailgun parser; SES-shaped In-Reply-To
			autoID := "auto-1"

			// The lookup tries the full In-Reply-To first (miss), then the host-stripped local
			// part — which equals the stored bare SES MessageId.
			d.mhRepo.EXPECT().GetBySMTPMessageID(gomock.Any(), d.workspaceID, "ses-mid-123@"+host).
				Return(nil, nil)
			d.mhRepo.EXPECT().GetBySMTPMessageID(gomock.Any(), d.workspaceID, "ses-mid-123").
				Return(&domain.MessageHistory{ContactEmail: "jane@example.com", AutomationID: &autoID}, nil)
			d.repo.EXPECT().StoreReplyEvent(gomock.Any(), d.workspaceID, gomock.Any()).Return(true, nil)
			d.autoRepo.EXPECT().
				ExitContactJourneysOnReply(gomock.Any(), d.workspaceID, "jane@example.com", &autoID, "replied", gomock.Any()).
				Return(1, nil)

			headers := `[["In-Reply-To","<ses-mid-123@` + host + `>"],["Message-Id","<reply-9@x>"]]`
			err := d.svc.ProcessInboundReply(context.Background(), d.workspaceID, d.integID, replyReq(headers))
			assert.NoError(t, err)
		})
	}
}

func TestProcessInboundReply_NoThreadingHeaderIgnored(t *testing.T) {
	d := newReplyTestDeps(t, domain.EmailProviderKindMailgun)

	// No In-Reply-To/References → nothing to match against. The sender-address fallback
	// was removed, so the reply is ignored entirely: no message lookup, no store, no exit.
	d.mhRepo.EXPECT().GetBySMTPMessageID(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	d.repo.EXPECT().StoreReplyEvent(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	d.autoRepo.EXPECT().ExitContactJourneysOnReply(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := d.svc.ProcessInboundReply(context.Background(), d.workspaceID, d.integID, replyReq(`[["Message-Id","<r@x>"]]`))
	assert.NoError(t, err)
}

func TestProcessInboundReply_AutoReplyRecordedNoExit(t *testing.T) {
	d := newReplyTestDeps(t, domain.EmailProviderKindMailgun)
	autoID := "auto-1"

	// An auto-reply that threads to a known send is recorded as auto_reply but never exits.
	d.mhRepo.EXPECT().GetBySMTPMessageID(gomock.Any(), d.workspaceID, "orig-1@example.com").
		Return(&domain.MessageHistory{ContactEmail: "jane@example.com", AutomationID: &autoID}, nil)
	d.repo.EXPECT().StoreReplyEvent(gomock.Any(), d.workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, event *domain.InboundWebhookEvent) (bool, error) {
			assert.Equal(t, domain.EmailEventAutoReply, event.Type)
			return true, nil
		})
	// Must NOT exit on an auto-responder.
	d.autoRepo.EXPECT().ExitContactJourneysOnReply(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := d.svc.ProcessInboundReply(context.Background(), d.workspaceID, d.integID,
		replyReq(`[["Auto-Submitted","auto-replied"],["In-Reply-To","<orig-1@example.com>"],["Message-Id","<r@x>"]]`))
	assert.NoError(t, err)
}

func TestProcessInboundReply_BounceDropped(t *testing.T) {
	d := newReplyTestDeps(t, domain.EmailProviderKindMailgun)

	// Bounce → no contact lookup, no store, no exit.
	d.repo.EXPECT().StoreReplyEvent(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	d.autoRepo.EXPECT().ExitContactJourneysOnReply(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := d.svc.ProcessInboundReply(context.Background(), d.workspaceID, d.integID,
		replyReq(`[["Content-Type","multipart/report; report-type=delivery-status"]]`))
	assert.NoError(t, err)
}

func TestProcessInboundReply_UnknownMessageIDIgnored(t *testing.T) {
	d := newReplyTestDeps(t, domain.EmailProviderKindMailgun)

	// In-Reply-To references a Message-ID we never stored → no match → ignored.
	d.mhRepo.EXPECT().GetBySMTPMessageID(gomock.Any(), d.workspaceID, "ghost@example.com").
		Return(nil, nil)
	d.repo.EXPECT().StoreReplyEvent(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	d.autoRepo.EXPECT().ExitContactJourneysOnReply(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := d.svc.ProcessInboundReply(context.Background(), d.workspaceID, d.integID,
		replyReq(`[["In-Reply-To","<ghost@example.com>"],["Message-Id","<r@x>"]]`))
	assert.NoError(t, err)
}

func TestProcessInboundReply_MessageLookupErrorPropagates(t *testing.T) {
	d := newReplyTestDeps(t, domain.EmailProviderKindMailgun)

	// A real DB error during Message-ID lookup must propagate (→ non-2xx → provider retries).
	d.mhRepo.EXPECT().GetBySMTPMessageID(gomock.Any(), d.workspaceID, "orig@example.com").
		Return(nil, errors.New("db down"))
	d.repo.EXPECT().StoreReplyEvent(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := d.svc.ProcessInboundReply(context.Background(), d.workspaceID, d.integID,
		replyReq(`[["In-Reply-To","<orig@example.com>"],["Message-Id","<r@x>"]]`))
	assert.Error(t, err)
}

func TestProcessInboundReply_UnsupportedProviderErrors(t *testing.T) {
	d := newReplyTestDeps(t, domain.EmailProviderKindPostmark) // no Postmark parser registered
	d.repo.EXPECT().StoreReplyEvent(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := d.svc.ProcessInboundReply(context.Background(), d.workspaceID, d.integID, replyReq(`[["Message-Id","<r@x>"]]`))
	assert.ErrorIs(t, err, domain.ErrInboundProviderUnsupported)
}

// fakeControlParser is a ReplyParser whose Verify always reports a provider control message,
// to exercise the service's early-ack path without a real signed SNS envelope.
type fakeControlParser struct{}

func (fakeControlParser) Source() domain.WebhookSource { return domain.WebhookSourceSES }
func (fakeControlParser) Verify(*domain.InboundRequest, *domain.Integration) error {
	return domain.ErrInboundControlMessage
}
func (fakeControlParser) Parse(*domain.InboundRequest) (*domain.InboundReply, error) {
	return nil, errors.New("Parse must not be called for a control message")
}

func TestProcessInboundReply_ControlMessageAcksWithoutIngesting(t *testing.T) {
	d := newReplyTestDeps(t, domain.EmailProviderKindSES)
	d.svc.replyParsers[domain.EmailProviderKindSES] = fakeControlParser{}

	// A control message (e.g. an SNS subscription confirmation) must ack 200 with no
	// parse, no contact lookup, no store, and no journey exit.
	d.mhRepo.EXPECT().GetBySMTPMessageID(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	d.repo.EXPECT().StoreReplyEvent(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	d.autoRepo.EXPECT().ExitContactJourneysOnReply(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := d.svc.ProcessInboundReply(context.Background(), d.workspaceID, d.integID, &domain.InboundRequest{})
	assert.NoError(t, err, "control messages are acknowledged, not errored")
}

func TestProcessInboundReply_DedupedReplyDoesNotExit(t *testing.T) {
	d := newReplyTestDeps(t, domain.EmailProviderKindMailgun)
	autoID := "auto-1"

	d.mhRepo.EXPECT().GetBySMTPMessageID(gomock.Any(), d.workspaceID, "orig-1@example.com").
		Return(&domain.MessageHistory{ContactEmail: "jane@example.com", AutomationID: &autoID}, nil)
	// The reply was already stored (a provider retry) → inserted=false.
	d.repo.EXPECT().StoreReplyEvent(gomock.Any(), d.workspaceID, gomock.Any()).Return(false, nil)
	// A deduped replay must NOT re-fire the exit — otherwise a replay landing after the
	// contact re-enrolled would wrongly kill the fresh journey instance.
	d.autoRepo.EXPECT().ExitContactJourneysOnReply(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := d.svc.ProcessInboundReply(context.Background(), d.workspaceID, d.integID,
		replyReq(`[["In-Reply-To","<orig-1@example.com>"],["Message-Id","<reply-1@x>"]]`))
	require.NoError(t, err)
}
