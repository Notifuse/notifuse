package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	servicemocks "github.com/Notifuse/notifuse/internal/service/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A bounce or an unsubscribe the provider posts again only changes a list row unchanged
// since Notifuse first received it. These tests deliver SendGrid and Mailjet batches whose
// events FirstStoredAt reports, by position in the delivery, as received before or not.
func TestProcessWebhook_RedeliveredEventsKeepNewerSubscriptionChanges(t *testing.T) {
	workspaceID, integrationID := "ws1", "int1"
	earlier := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
	later := time.Date(2026, 11, 17, 9, 30, 0, 0, time.UTC)

	workspaceOf := func(kind domain.EmailProviderKind) *domain.Workspace {
		return &domain.Workspace{
			ID:           workspaceID,
			Integrations: []domain.Integration{{ID: integrationID, EmailProvider: domain.EmailProvider{Kind: kind}}},
		}
	}
	// sendGrid returns a SendGrid batch: a "bounce" is a hard bounce, a "blocked" a
	// counted soft bounce
	sendGrid := func(t *testing.T, events ...[3]string) []byte {
		t.Helper()
		batch := make([]map[string]interface{}, 0, len(events))
		for _, e := range events {
			batch = append(batch, map[string]interface{}{
				"event":               e[0],
				"notifuse_message_id": e[1],
				"email":               e[2],
				"timestamp":           time.Date(2026, 11, 17, 7, 0, 0, 0, time.UTC).Unix(),
			})
		}
		raw, err := json.Marshal(batch)
		require.NoError(t, err)
		return raw
	}
	// receivedBefore answers FirstStoredAt with the given first-stored times, keyed by the
	// position of the event in the delivery, and reports the IDs it was asked for
	receivedBefore := func(t *testing.T, asked *[]string, at map[int]time.Time) func(context.Context, string, []string) (map[string]time.Time, error) {
		return func(_ context.Context, _ string, ids []string) (map[string]time.Time, error) {
			*asked = ids
			found := map[string]time.Time{}
			for position, stored := range at {
				require.Less(t, position, len(ids))
				found[ids[position]] = stored
			}
			return found, nil
		}
	}

	type sendGridCase struct {
		events     [][3]string
		recipients map[string]string
		stored     map[int]time.Time
	}
	// setup expects a SendGrid delivery up to its status update; the test expects the
	// bounces it should make, and gomock fails it on any other
	setup := func(t *testing.T, c sendGridCase) (*InboundWebhookEventService, *mockRepos, []byte) {
		service, repo, workspaceRepo, history, contactRepo, ctrl := newClassificationTestService(t)
		t.Cleanup(ctrl.Finish)
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSendGrid), nil)
		history.EXPECT().GetRecipients(gomock.Any(), workspaceID, gomock.Any()).Return(c.recipients, nil)
		var asked []string
		gomock.InOrder(
			repo.EXPECT().FirstStoredAt(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(receivedBefore(t, &asked, c.stored)),
			repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, events []*domain.InboundWebhookEvent) error {
					ids := make([]string, 0, len(events))
					for _, event := range events {
						ids = append(ids, event.ID)
					}
					assert.Equal(t, ids, asked, "the events stored are the events checked")
					return nil
				}),
			history.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).Return(nil),
		)
		return service, &mockRepos{repo: repo, contactRepo: contactRepo}, sendGrid(t, c.events...)
	}

	t.Run("a hard bounce received for the first time bounces the address with no updated_at condition", func(t *testing.T) {
		service, m, raw := setup(t, sendGridCase{
			events:     [][3]string{{"bounce", "msg-1", "a@example.com"}},
			recipients: map[string]string{"msg-1": "a@example.com"},
		})
		m.contactRepo.EXPECT().MarkEmailsAsBounced(gomock.Any(), workspaceID, []string{"a@example.com"}, gomock.Any()).Return(nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, raw))
	})

	t.Run("a hard bounce received before only bounces rows unchanged since it was first received", func(t *testing.T) {
		service, m, raw := setup(t, sendGridCase{
			events:     [][3]string{{"bounce", "msg-1", "a@example.com"}},
			recipients: map[string]string{"msg-1": "a@example.com"},
			stored:     map[int]time.Time{0: earlier},
		})
		m.contactRepo.EXPECT().MarkEmailsAsBouncedUnchangedSince(gomock.Any(), workspaceID,
			map[string]time.Time{"a@example.com": earlier}, gomock.Any()).Return(nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, raw))
	})

	t.Run("an address with one new and one received hard bounce is bounced with no updated_at condition", func(t *testing.T) {
		service, m, raw := setup(t, sendGridCase{
			events:     [][3]string{{"bounce", "msg-1", "a@example.com"}, {"bounce", "msg-2", "a@example.com"}},
			recipients: map[string]string{"msg-1": "a@example.com", "msg-2": "a@example.com"},
			stored:     map[int]time.Time{0: earlier},
		})
		m.contactRepo.EXPECT().MarkEmailsAsBounced(gomock.Any(), workspaceID, []string{"a@example.com"}, gomock.Any()).Return(nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, raw))
	})

	t.Run("several received hard bounces of one address: the latest first-received time", func(t *testing.T) {
		service, m, raw := setup(t, sendGridCase{
			events:     [][3]string{{"bounce", "msg-1", "a@example.com"}, {"bounce", "msg-2", "a@example.com"}},
			recipients: map[string]string{"msg-1": "a@example.com", "msg-2": "a@example.com"},
			stored:     map[int]time.Time{0: later, 1: earlier},
		})
		m.contactRepo.EXPECT().MarkEmailsAsBouncedUnchangedSince(gomock.Any(), workspaceID,
			map[string]time.Time{"a@example.com": later}, gomock.Any()).Return(nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, raw))
	})

	t.Run("a received hard bounce with a new soft bounce below the threshold: the hard bounce's time", func(t *testing.T) {
		service, m, raw := setup(t, sendGridCase{
			events:     [][3]string{{"bounce", "msg-1", "a@example.com"}, {"blocked", "msg-2", "a@example.com"}},
			recipients: map[string]string{"msg-1": "a@example.com", "msg-2": "a@example.com"},
			stored:     map[int]time.Time{0: earlier},
		})
		m.repo.EXPECT().CountConsecutiveSoftBounces(gomock.Any(), workspaceID, []string{"a@example.com"}).
			Return(map[string]int{"a@example.com": 1}, nil)
		m.contactRepo.EXPECT().MarkEmailsAsBouncedUnchangedSince(gomock.Any(), workspaceID,
			map[string]time.Time{"a@example.com": earlier}, gomock.Any()).Return(nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, raw))
	})

	t.Run("a mixed delivery bounces each address by its own events", func(t *testing.T) {
		service, m, raw := setup(t, sendGridCase{
			events:     [][3]string{{"bounce", "msg-1", "new@example.com"}, {"bounce", "msg-2", "old@example.com"}},
			recipients: map[string]string{"msg-1": "new@example.com", "msg-2": "old@example.com"},
			stored:     map[int]time.Time{1: earlier},
		})
		m.contactRepo.EXPECT().MarkEmailsAsBounced(gomock.Any(), workspaceID, []string{"new@example.com"}, gomock.Any()).Return(nil)
		m.contactRepo.EXPECT().MarkEmailsAsBouncedUnchangedSince(gomock.Any(), workspaceID,
			map[string]time.Time{"old@example.com": earlier}, gomock.Any()).Return(nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, raw))
	})

	t.Run("a threshold reached by received soft bounces only bounces rows unchanged since", func(t *testing.T) {
		service, m, raw := setup(t, sendGridCase{
			events:     [][3]string{{"blocked", "msg-1", "a@example.com"}, {"blocked", "msg-2", "a@example.com"}},
			recipients: map[string]string{"msg-1": "a@example.com", "msg-2": "a@example.com"},
			stored:     map[int]time.Time{0: earlier, 1: later},
		})
		m.repo.EXPECT().CountConsecutiveSoftBounces(gomock.Any(), workspaceID, []string{"a@example.com", "a@example.com"}).
			Return(map[string]int{"a@example.com": domain.DefaultSoftBounceThreshold}, nil)
		m.contactRepo.EXPECT().MarkEmailsAsBouncedUnchangedSince(gomock.Any(), workspaceID,
			map[string]time.Time{"a@example.com": later}, gomock.Any()).Return(nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, raw))
	})

	t.Run("a threshold reached with a new soft bounce bounces the address with no updated_at condition", func(t *testing.T) {
		service, m, raw := setup(t, sendGridCase{
			events:     [][3]string{{"blocked", "msg-1", "a@example.com"}, {"blocked", "msg-2", "a@example.com"}},
			recipients: map[string]string{"msg-1": "a@example.com", "msg-2": "a@example.com"},
			stored:     map[int]time.Time{0: earlier},
		})
		m.repo.EXPECT().CountConsecutiveSoftBounces(gomock.Any(), workspaceID, gomock.Any()).
			Return(map[string]int{"a@example.com": domain.DefaultSoftBounceThreshold}, nil)
		m.contactRepo.EXPECT().MarkEmailsAsBounced(gomock.Any(), workspaceID, []string{"a@example.com"}, gomock.Any()).Return(nil)

		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, raw))
	})

	t.Run("a FirstStoredAt error is returned and nothing is stored or applied", func(t *testing.T) {
		service, repo, workspaceRepo, history, _, ctrl := newClassificationTestService(t)
		defer ctrl.Finish()
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindSendGrid), nil)
		history.EXPECT().GetRecipients(gomock.Any(), workspaceID, gomock.Any()).Return(map[string]string{"msg-1": "a@example.com"}, nil)
		repo.EXPECT().FirstStoredAt(gomock.Any(), workspaceID, gomock.Any()).Return(nil, errors.New("boom"))

		err := service.ProcessWebhook(context.Background(), workspaceID, integrationID,
			sendGrid(t, [3]string{"bounce", "msg-1", "a@example.com"}))
		assert.ErrorContains(t, err, "boom")
	})

	t.Run("an unsubscribe passes when it was first received, or zero when new", func(t *testing.T) {
		listID := "list-1"
		service, repo, workspaceRepo, history, _, ctrl := newClassificationTestService(t)
		defer ctrl.Finish()
		unsubscriber := servicemocks.NewMockListUnsubscriber(ctrl)
		service.listUnsubscriber = unsubscriber
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspaceOf(domain.EmailProviderKindMailjet), nil)
		history.EXPECT().GetRecipients(gomock.Any(), workspaceID, gomock.Any()).
			Return(map[string]string{"msg-1": "old@example.com", "msg-2": "new@example.com"}, nil)
		var asked []string
		repo.EXPECT().FirstStoredAt(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(receivedBefore(t, &asked, map[int]time.Time{0: earlier}))
		repo.EXPECT().StoreEvents(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		history.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Len(0)).Return(nil)
		history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-1").
			Return(&domain.MessageHistory{ContactEmail: "old@example.com", ListID: &listID}, nil)
		history.EXPECT().GetRecipientAndList(gomock.Any(), workspaceID, "msg-2").
			Return(&domain.MessageHistory{ContactEmail: "new@example.com", ListID: &listID}, nil)
		unsubscriber.EXPECT().UnsubscribeFromMessageList(gomock.Any(), workspaceID, "old@example.com", listID, "msg-1", earlier).Return(nil)
		unsubscriber.EXPECT().UnsubscribeFromMessageList(gomock.Any(), workspaceID, "new@example.com", listID, "msg-2", time.Time{}).Return(nil)
		history.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Len(2)).Return(nil)

		at := time.Date(2026, 11, 17, 7, 0, 0, 0, time.UTC).Unix()
		raw, err := json.Marshal([]domain.MailjetWebhookPayload{
			{Event: "unsub", Time: at, Email: "old@example.com", MessageID: 1, CustomID: "msg-1"},
			{Event: "unsub", Time: at, Email: "new@example.com", MessageID: 2, CustomID: "msg-2"},
		})
		require.NoError(t, err)
		require.NoError(t, service.ProcessWebhook(context.Background(), workspaceID, integrationID, raw))
	})
}

type mockRepos struct {
	repo        *mocks.MockInboundWebhookEventRepository
	contactRepo *mocks.MockContactRepository
}
