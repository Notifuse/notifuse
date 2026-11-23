package queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/dkim"
)

func smtpWorkspace(keys ...domain.DKIMKey) *domain.Workspace {
	return &domain.Workspace{
		ID:           "ws-1",
		Settings:     domain.WorkspaceSettings{DKIMKeys: keys},
		Integrations: []domain.Integration{emailIntegration("smtp-1", domain.EmailProviderKindSMTP, 6000)},
	}
}

var errDKIMSign = fmt.Errorf("DKIM key of acme.io: %w: failed to decrypt", dkim.ErrSign)

func TestEmailQueueWorker_DKIM(t *testing.T) {
	t.Run("each send carries the round's keys, no lookup per email", func(t *testing.T) {
		lt := newLaneTest(t, DefaultWorkerConfig(), queuedRows("smtp-1", 2)...)
		key := domain.DKIMKey{Domain: "acme.io", Status: domain.DKIMKeyStatusPublished}
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Times(2).
			DoAndReturn(func(_ context.Context, r domain.SendEmailProviderRequest, _ bool) error {
				require.NotNil(t, r.DKIMSnapshot)
				assert.Equal(t, []domain.DKIMKey{key}, *r.DKIMSnapshot)
				return nil
			})
		within(t, 5*time.Second, func() { lt.w.processWorkspace(smtpWorkspace(key)) })
		assert.Equal(t, 2, lt.queue.sentCount("smtp-1"))
	})

	t.Run("a workspace without keys sends an empty snapshot, not nil", func(t *testing.T) {
		lt := newLaneTest(t, DefaultWorkerConfig(), queuedRows("smtp-1", 1)...)
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(_ context.Context, r domain.SendEmailProviderRequest, _ bool) error {
				require.NotNil(t, r.DKIMSnapshot)
				assert.Empty(t, *r.DKIMSnapshot)
				return nil
			})
		within(t, 5*time.Second, func() { lt.w.processWorkspace(smtpWorkspace()) })
	})

	t.Run("a signing error is retried, never counted against the provider", func(t *testing.T) {
		rows := 10
		lt := newLaneTest(t, DefaultWorkerConfig(), queuedRows("smtp-1", rows)...)
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(errDKIMSign).Times(rows)

		within(t, 5*time.Second, func() { lt.w.processWorkspace(smtpWorkspace()) })

		assert.Equal(t, rows, lt.queue.failedCount("smtp-1"), "each row scheduled for a retry")
		assert.Zero(t, lt.queue.sentCount("smtp-1"))
		assert.Zero(t, lt.w.GetCircuitBreakerStats()["smtp-1"].Failures)
		assert.False(t, lt.w.circuitBreaker.IsOpen("smtp-1"), "ten signing errors open nothing")
	})

	t.Run("a probe that fails to sign is given back, and the circuit stays open", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.CircuitBreakerCooldown = 50 * time.Millisecond
		lt := newLaneTest(t, cfg, queuedRows("smtp-1", 3)...)
		openCircuit(t, lt.w, "smtp-1")
		time.Sleep(60 * time.Millisecond)
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(errDKIMSign).Times(1)

		within(t, 5*time.Second, func() { lt.w.processWorkspace(smtpWorkspace()) })

		assert.Equal(t, 1, lt.queue.failedCount("smtp-1"))
		assert.True(t, lt.w.circuitBreaker.IsOpen("smtp-1"))
		assert.False(t, lt.w.GetCircuitBreakerStats()["smtp-1"].ProbeInFlight)
		ticket, ok := lt.w.circuitBreaker.Allow("smtp-1")
		assert.True(t, ok, "the next round probes without another cooldown")
		assert.True(t, ticket.Probe)
	})
}
