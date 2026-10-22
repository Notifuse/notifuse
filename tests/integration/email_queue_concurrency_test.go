package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service/queue"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmailQueueConcurrentSending runs the real worker against PostgreSQL and an SMTP
// server (Mailpit), with four emails sent at once per SMTP integration.
func TestEmailQueueConcurrentSending(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		cfg.EmailQueue = config.EmailQueueConfig{Concurrency: 10, SMTPConcurrency: 4}
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	application := suite.ServerManager.GetApp()
	queueRepo := application.GetEmailQueueRepository()
	worker := application.GetEmailQueueWorker()
	require.NotNil(t, worker)
	ctx := context.Background()

	newWorkspace := func(t *testing.T, ratePerMinute, integrationCount int) (*domain.Workspace, []*domain.Integration) {
		t.Helper()
		workspace, err := factory.CreateWorkspace()
		require.NoError(t, err)
		var integrations []*domain.Integration
		for i := 0; i < integrationCount; i++ {
			integration, err := factory.CreateMailpitSMTPIntegration(workspace.ID, func(integration *domain.Integration) {
				integration.EmailProvider.RateLimitPerMinute = ratePerMinute
			})
			require.NoError(t, err)
			integrations = append(integrations, integration)
		}
		return workspace, integrations
	}

	// enqueue queues one email per recipient, spread over the integrations, and returns
	// the recipients.
	enqueue := func(t *testing.T, workspaceID string, integrations []*domain.Integration, count int, broadcastID, subject string) []string {
		t.Helper()
		recipients := make([]string, count)
		entries := make([]*domain.EmailQueueEntry, count)
		for i := range entries {
			recipients[i] = strings.ToLower(fmt.Sprintf("%s-%d@example.com", broadcastID, i))
			entries[i] = testutil.CreateTestEmailQueueEntry(integrations[i%len(integrations)].ID, recipients[i], broadcastID, domain.EmailQueueSourceBroadcast)
			entries[i].Payload.Subject = subject
		}
		require.NoError(t, queueRepo.Enqueue(ctx, workspaceID, entries))
		return recipients
	}

	// requireEachOnce checks Mailpit holds exactly one email per recipient.
	requireEachOnce := func(t *testing.T, subject string, recipients []string) {
		t.Helper()
		var deliveries map[string]int
		require.Eventually(t, func() bool {
			deliveries = mailpitDeliveries(t, subject)
			total := 0
			for _, n := range deliveries {
				total += n
			}
			return total >= len(recipients)
		}, 10*time.Second, 100*time.Millisecond)
		// A late duplicate would arrive right behind the others.
		time.Sleep(500 * time.Millisecond)
		deliveries = mailpitDeliveries(t, subject)

		assert.Len(t, deliveries, len(recipients))
		for _, recipient := range recipients {
			assert.Equal(t, 1, deliveries[recipient], recipient)
		}
	}

	t.Run("each recipient gets exactly one email", func(t *testing.T) {
		workspace, integrations := newWorkspace(t, 6000, 2)
		broadcastID := "concurrent-" + strings.ToLower(testutil.GenerateRandomString(8))
		subject := "Concurrent " + broadcastID
		recipients := enqueue(t, workspace.ID, integrations, 300, broadcastID, subject)

		start := time.Now()
		require.NoError(t, worker.Start(ctx))
		require.NoError(t, testutil.WaitForQueueEmpty(t, queueRepo, workspace.ID, 60*time.Second))
		t.Logf("300 emails over two SMTP integrations drained in %s", time.Since(start))
		worker.Stop()

		requireEachOnce(t, subject, recipients)

		stats, err := queueRepo.GetStats(ctx, workspace.ID)
		require.NoError(t, err)
		assert.Zero(t, stats.Failed)

		db, err := factory.GetWorkspaceDB(workspace.ID)
		require.NoError(t, err)
		var history, failed int
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT COUNT(*), COUNT(failed_at) FROM message_history WHERE broadcast_id = $1`, broadcastID).
			Scan(&history, &failed))
		assert.Equal(t, len(recipients), history)
		assert.Zero(t, failed, "no attempt failed")
	})

	t.Run("a stop in the middle of a broadcast neither loses nor repeats an email", func(t *testing.T) {
		// 1,200/min: the 120 emails take about six seconds.
		workspace, integrations := newWorkspace(t, 1200, 1)
		broadcastID := "restart-" + strings.ToLower(testutil.GenerateRandomString(8))
		subject := "Restart " + broadcastID
		recipients := enqueue(t, workspace.ID, integrations, 120, broadcastID, subject)

		require.NoError(t, worker.Start(ctx))
		require.Eventually(t, func() bool {
			total := 0
			for _, n := range mailpitDeliveries(t, subject) {
				total += n
			}
			return total >= 20
		}, 30*time.Second, 50*time.Millisecond)
		worker.Stop()

		db, err := factory.GetWorkspaceDB(workspace.ID)
		require.NoError(t, err)
		var processing, pending int
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT COUNT(*) FILTER (WHERE status = 'processing'), COUNT(*) FILTER (WHERE status = 'pending') FROM email_queue WHERE source_id = $1`,
			broadcastID).Scan(&processing, &pending))
		assert.Zero(t, processing, "every email being sent when Stop was called has finished")
		require.Positive(t, pending, "the stop came before the end of the broadcast")

		require.NoError(t, worker.Start(ctx))
		require.NoError(t, testutil.WaitForQueueEmpty(t, queueRepo, workspace.ID, 60*time.Second))
		worker.Stop()

		requireEachOnce(t, subject, recipients)
	})

	t.Run("two workers on the same database send each email once", func(t *testing.T) {
		workspace, integrations := newWorkspace(t, 6000, 1)
		broadcastID := "two-workers-" + strings.ToLower(testutil.GenerateRandomString(8))
		subject := "Two workers " + broadcastID
		recipients := enqueue(t, workspace.ID, integrations, 200, broadcastID, subject)

		secondConfig := queue.DefaultWorkerConfig()
		secondConfig.SMTPSendConcurrency = 4
		second := queue.NewEmailQueueWorker(
			queueRepo,
			application.GetWorkspaceRepository(),
			application.GetEmailService(),
			application.GetMessageHistoryRepository(),
			secondConfig,
			application.GetLogger(),
		)

		require.NoError(t, worker.Start(ctx))
		require.NoError(t, second.Start(ctx))
		require.NoError(t, testutil.WaitForQueueEmpty(t, queueRepo, workspace.ID, 60*time.Second))
		worker.Stop()
		second.Stop()

		requireEachOnce(t, subject, recipients)
	})
}

// mailpitDeliveries counts, per recipient, the emails Mailpit holds with this subject.
func mailpitDeliveries(t *testing.T, subject string) map[string]int {
	t.Helper()
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get("http://localhost:8025/api/v1/messages?limit=10000")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	var list testutil.MailpitMessagesResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))

	deliveries := map[string]int{}
	for _, msg := range list.Messages {
		if msg.Subject != subject {
			continue
		}
		for _, to := range msg.To {
			deliveries[strings.ToLower(to.Address)]++
		}
	}
	return deliveries
}
