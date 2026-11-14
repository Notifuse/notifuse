package broadcast_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service/broadcast"
)

// Each batch is sent with the workspace's email provider as it is when the batch goes: a
// sender removed from the integration during the run is then refused by SendBatch (which
// checks the batch's senders against the provider it is given) and the broadcast pauses.
func TestProcess_EachBatchGetsTheProviderAsItIsNow(t *testing.T) {
	workspaceWith := func(senderEmail string) *domain.Workspace {
		return &domain.Workspace{
			ID:       "workspace-123",
			Settings: domain.WorkspaceSettings{SecretKey: "secret-key", MarketingEmailProviderID: "marketing-provider-id"},
			Integrations: []domain.Integration{{
				ID:   "marketing-provider-id",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind:    domain.EmailProviderKindSES,
					SES:     &domain.AmazonSESSettings{AccessKey: "ak", SecretKey: "sk", Region: "us-east-1"},
					Senders: []domain.EmailSender{domain.NewEmailSender(senderEmail, "Sender")},
				},
			}},
		}
	}

	// run sends abAudience in batches of two; reads lists what each workspace read returns.
	run := func(t *testing.T, reads []func() (*domain.Workspace, error)) []string {
		m := newSendingStartedMocks(t)
		m.expectAudience()
		call := 0
		m.workspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").DoAndReturn(
			func(context.Context, string) (*domain.Workspace, error) {
				read := reads[len(reads)-1]
				if call < len(reads) {
					read = reads[call]
				}
				call++
				return read()
			}).AnyTimes()
		m.broadcastRepo.EXPECT().MarkBroadcastStarted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		m.eventBus.EXPECT().Publish(gomock.Any(), gomock.Any()).AnyTimes()
		m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
			DoAndReturn(func(context.Context, string, string) (*domain.Broadcast, error) {
				return singleTemplateBroadcast(domain.BroadcastStatusProcessing), nil
			}).AnyTimes()
		m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-A", int64(0)).
			Return(abTemplate("template-A", "Variation A"), nil).AnyTimes()
		m.taskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		expectFinalWrite(m.broadcastRepo, "workspace-123", singleTemplateBroadcast(domain.BroadcastStatusProcessing),
			func(context.Context, *domain.Broadcast) error { return nil })

		var senders []string
		m.messageSender.EXPECT().
			SendBatch(gomock.Any(), "workspace-123", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				gomock.Any(), "broadcast-123", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _, _, _ string, _ domain.EmailTrackingPolicy, _ *domain.WebAnalyticsSettings, _ string,
				recipients []*domain.ContactWithList, _ map[string]*domain.Template, provider *domain.EmailProvider,
				_ time.Time, _ string) (int, int, error) {
				require.NotNil(t, provider)
				senders = append(senders, provider.Senders[0].Email)
				return len(recipients), 0, nil
			}).AnyTimes()

		config := &broadcast.Config{FetchBatchSize: 2, MaxProcessTime: 30 * time.Second, ProgressLogInterval: 5 * time.Second}
		o := broadcast.NewBroadcastOrchestrator(m.messageSender, m.broadcastRepo, m.templateRepo, m.contactRepo,
			m.taskRepo, m.workspaceRepo, nil, nil, m.logger, config, m.timeProvider, "https://api.example.com", m.eventBus)
		done, err := o.Process(context.Background(), abTask(&domain.SendBroadcastState{}), time.Now().Add(30*time.Second))
		require.NoError(t, err)
		assert.True(t, done)
		return senders
	}
	ok := func(email string) func() (*domain.Workspace, error) {
		return func() (*domain.Workspace, error) { return workspaceWith(email), nil }
	}

	t.Run("a provider changed between two batches reaches the second one", func(t *testing.T) {
		// The run's own read, then one read before each batch.
		senders := run(t, []func() (*domain.Workspace, error){ok("old@example.com"), ok("old@example.com"), ok("new@example.com")})
		assert.Equal(t, []string{"old@example.com", "new@example.com"}, senders)
	})

	t.Run("a read that fails keeps the run's provider", func(t *testing.T) {
		failing := func() (*domain.Workspace, error) { return nil, errors.New("connection reset") }
		senders := run(t, []func() (*domain.Workspace, error){ok("old@example.com"), failing})
		assert.Equal(t, []string{"old@example.com", "old@example.com"}, senders)
	})

	t.Run("a read that finds no marketing provider keeps the run's provider", func(t *testing.T) {
		none := func() (*domain.Workspace, error) {
			w := workspaceWith("x@example.com")
			w.Settings.MarketingEmailProviderID = ""
			return w, nil
		}
		senders := run(t, []func() (*domain.Workspace, error){ok("old@example.com"), none})
		assert.Equal(t, []string{"old@example.com", "old@example.com"}, senders)
	})
}
