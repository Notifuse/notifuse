package queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every claimed row has SendTimeout from its claim to the provider's answer, so its send ends
// before the row could be reclaimed and sent again. A stop cancels the sends still running at
// the end of its grace.
func TestEmailQueueWorker_SendTimeout(t *testing.T) {
	const sendTimeout = 100 * time.Millisecond
	newTest := func(t *testing.T, rows ...*domain.EmailQueueEntry) *laneTest {
		cfg := DefaultWorkerConfig()
		cfg.SendTimeout = sendTimeout
		return newLaneTest(t, cfg, rows...)
	}
	sesWorkspace := func(id string) *domain.Workspace {
		return &domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{
			emailIntegration(id, domain.EmailProviderKindSES, 6000),
		}}
	}
	caID := "ca-1"
	automationRow := func() *domain.EmailQueueEntry {
		row := queuedRows("ses-1", 1)[0]
		row.SourceType = domain.EmailQueueSourceAutomation
		row.Payload.ContactAutomationID = &caID
		return row
	}

	t.Run("a send that outlasts it is cut and retried, as a provider failure", func(t *testing.T) {
		lt := newTest(t, queuedRows("ses-1", 1)...)
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(ctx context.Context, _ domain.SendEmailProviderRequest, _ bool) error {
				<-ctx.Done()
				return fmt.Errorf("operation error SES: SendEmail, %w", ctx.Err())
			})

		took := within(t, 5*time.Second, func() { lt.w.processWorkspace(sesWorkspace("ses-1")) })

		assert.GreaterOrEqual(t, took, sendTimeout)
		assert.Equal(t, 1, lt.queue.failedCount("ses-1"), "scheduled for a retry, not left processing")
		assert.Zero(t, lt.queue.sentCount("ses-1"))
		assert.Equal(t, 1, lt.w.GetCircuitBreakerStats()["ses-1"].Failures, "a provider that does not answer in time is failing")
	})

	t.Run("a stop-on-reply lookup that outlasts it releases the row without calling the provider", func(t *testing.T) {
		lt := newTest(t, automationRow())
		lt.w.SetAutomationRepo(lt.automation)
		lt.automation.EXPECT().GetContactAutomation(gomock.Any(), "ws-1", caID).
			DoAndReturn(func(ctx context.Context, _, _ string) (*domain.ContactAutomation, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			})

		within(t, 5*time.Second, func() { lt.w.processWorkspace(sesWorkspace("ses-1")) })

		assert.Equal(t, 1, lt.queue.failedCount("ses-1"))
		assert.Zero(t, lt.w.GetCircuitBreakerStats()["ses-1"].Failures)
	})

	t.Run("a row whose budget is spent before the send is released without calling the provider", func(t *testing.T) {
		lt := newTest(t, automationRow())
		lt.w.SetAutomationRepo(lt.automation)
		lt.automation.EXPECT().GetContactAutomation(gomock.Any(), "ws-1", caID).
			DoAndReturn(func(ctx context.Context, _, _ string) (*domain.ContactAutomation, error) {
				<-ctx.Done() // a slow database: the lookup ends only with the budget
				return &domain.ContactAutomation{ID: caID, Status: domain.ContactAutomationStatusActive}, nil
			})

		within(t, 5*time.Second, func() { lt.w.processWorkspace(sesWorkspace("ses-1")) })

		assert.Equal(t, 1, lt.queue.failedCount("ses-1"), "scheduled for a retry")
		assert.Zero(t, lt.queue.sentCount("ses-1"))
		assert.Zero(t, lt.w.GetCircuitBreakerStats()["ses-1"].Failures, "the provider was not asked")
	})

	t.Run("a send within it is not cut", func(t *testing.T) {
		lt := newTest(t, queuedRows("ses-1", 1)...)
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(ctx context.Context, _ domain.SendEmailProviderRequest, _ bool) error {
				deadline, ok := ctx.Deadline()
				require.True(t, ok, "the send has a deadline")
				assert.WithinDuration(t, time.Now().Add(sendTimeout), deadline, sendTimeout)
				return nil
			})

		within(t, 5*time.Second, func() { lt.w.processWorkspace(sesWorkspace("ses-1")) })
		assert.Equal(t, 1, lt.queue.sentCount("ses-1"))
	})
}

func TestEmailQueueWorkerConfig_SendTimeoutDefaults(t *testing.T) {
	defaults := DefaultWorkerConfig()
	assert.Equal(t, 100*time.Second, defaults.SendTimeout, "below the 2-minute reclaim of a processing row")
	assert.Equal(t, 40*time.Second, defaults.ShutdownGrace)
	assert.Equal(t, 5*time.Second, defaults.AbortWait)

	filled := EmailQueueWorkerConfig{}.withSendDefaults()
	assert.Equal(t, defaults.SendTimeout, filled.SendTimeout)
}
