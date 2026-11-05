package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/emailerror"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The refusals for the sending pace each provider returns, as its SendEmail words them.
var (
	sesThrottle     = errors.New("SES error: TooManyRequestsException: Maximum sending rate exceeded.")
	mailgunThrottle = errors.New("API returned non-OK status code 429")
	sesRecipient    = errors.New("SES error: MessageRejected: Email address is not verified.")
)

// clockedLaneTest is a lane test whose pace runs on a clock the test moves. Only tests
// that send from one goroutine use it.
func clockedLaneTest(t *testing.T, cfg *EmailQueueWorkerConfig, rows ...*domain.EmailQueueEntry) (*laneTest, *fakeClock) {
	t.Helper()
	lt := newLaneTest(t, cfg, rows...)
	clock := newFakeClock()
	lt.w.rateLimiter.now = clock.Now
	return lt, clock
}

func sesWorkspaceAt(rate int) *domain.Workspace {
	return &domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{
		emailIntegration("ses-1", domain.EmailProviderKindSES, rate),
	}}
}

func rowStatus(lt *laneTest, id string) (domain.EmailQueueStatus, int) {
	lt.queue.mu.Lock()
	defer lt.queue.mu.Unlock()
	row := lt.queue.find(id)
	return row.Status, row.Attempts
}

func TestEmailQueueWorker_Throttle(t *testing.T) {
	t.Run("a throttled broadcast row is given back with its attempt, and the pace drops", func(t *testing.T) {
		rows := queuedRows("ses-1", 1)
		lt := newLaneTest(t, DefaultWorkerConfig(), rows...)
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(sesThrottle)
		workspace := sesWorkspaceAt(6000)

		assert.Nil(t, processOne(lt.w, workspace, rows[0]))

		assert.Equal(t, 1, lt.queue.releasedCount("ses-1"))
		assert.Zero(t, lt.queue.failedCount("ses-1"))
		status, attempts := rowStatus(lt, rows[0].ID)
		assert.Equal(t, domain.EmailQueueStatusPending, status)
		assert.Zero(t, attempts)
		assert.Empty(t, lt.historyErrs(), "no message history for an email not sent yet")
		assert.Zero(t, lt.w.GetCircuitBreakerStats()["ses-1"].Failures)
		assert.Equal(t, 4500, lt.w.rateLimiter.EffectivePerMinute("ses-1", 6000))
	})

	t.Run("a throttled Mailgun automation row rewrites the history the pre-send upsert left", func(t *testing.T) {
		row := queuedRows("mg-1", 1)[0]
		row.SourceType = domain.EmailQueueSourceAutomation
		row.ProviderKind = domain.EmailProviderKindMailgun
		lt := newLaneTest(t, DefaultWorkerConfig(), row)
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(mailgunThrottle)
		workspace := &domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{
			emailIntegration("mg-1", domain.EmailProviderKindMailgun, 600),
		}}

		assert.Nil(t, processOne(lt.w, workspace, row))

		assert.Len(t, lt.historyErrs(), 2, "the pre-send upsert, then the refusal over it")
		assert.Equal(t, 1, lt.queue.releasedCount("mg-1"))
		assert.Zero(t, lt.queue.failedCount("mg-1"))
		assert.Equal(t, 450, lt.w.rateLimiter.EffectivePerMinute("mg-1", 600))
	})

	t.Run("throttles at the floor count against the provider only after the dwell", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.CircuitBreakerThreshold = 5
		rows := queuedRows("ses-1", 6)
		lt, clock := clockedLaneTest(t, cfg, rows...)
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(sesThrottle).AnyTimes()
		workspace := sesWorkspaceAt(6000)

		// Down to the floor, then through most of the dwell: every refusal gives the
		// row back.
		for i := 0; i < decreasesToFloor(); i++ {
			assert.Nil(t, processOne(lt.w, workspace, rows[0]))
		}
		require.Equal(t, 600, lt.w.rateLimiter.EffectivePerMinute("ses-1", 6000))
		clock.Advance(defaultFloorDwell - time.Second)
		for i := 0; i < 10; i++ {
			assert.Nil(t, processOne(lt.w, workspace, rows[0]))
		}
		assert.False(t, lt.w.circuitBreaker.IsOpen("ses-1"))
		assert.Zero(t, lt.w.GetCircuitBreakerStats()["ses-1"].Failures)
		assert.Zero(t, lt.queue.failedCount("ses-1"))

		// Past the dwell, each refusal is a provider failure, and the threshold opens
		// the circuit once.
		clock.Advance(time.Second)
		var opened int
		for _, row := range rows[1:] {
			if processOne(lt.w, workspace, row) != nil {
				opened++
			}
		}
		assert.Equal(t, 1, opened)
		assert.True(t, lt.w.circuitBreaker.IsOpen("ses-1"))
		assert.Equal(t, 5, lt.queue.failedCount("ses-1"))
		assert.Equal(t, 600, lt.w.rateLimiter.EffectivePerMinute("ses-1", 6000), "never below the floor")
	})

	t.Run("accepted sends of the current pace start the climb back", func(t *testing.T) {
		cases := []struct {
			name string
			err  error
		}{
			{"a success", nil},
			{"a recipient refusal", sesRecipient},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rows := queuedRows("ses-1", 2)
				lt, clock := clockedLaneTest(t, DefaultWorkerConfig(), rows...)
				gomock.InOrder(
					lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(sesThrottle),
					lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(tc.err),
				)
				workspace := sesWorkspaceAt(6000)

				processOne(lt.w, workspace, rows[0])
				clock.Advance(defaultRecoveryInterval)
				assert.Equal(t, 4500, lt.w.rateLimiter.EffectivePerMinute("ses-1", 6000), "waiting is not recovering")

				processOne(lt.w, workspace, rows[1])
				clock.Advance(defaultRecoveryInterval)
				assert.Equal(t, 4800, lt.w.rateLimiter.EffectivePerMinute("ses-1", 6000))
			})
		}
	})

	t.Run("a provider failure that is not about the pace does not start the climb back", func(t *testing.T) {
		rows := queuedRows("ses-1", 2)
		lt, clock := clockedLaneTest(t, DefaultWorkerConfig(), rows...)
		gomock.InOrder(
			lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(sesThrottle),
			lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(unavailable),
		)
		workspace := sesWorkspaceAt(6000)

		processOne(lt.w, workspace, rows[0])
		processOne(lt.w, workspace, rows[1])
		clock.Advance(defaultRecoveryInterval)
		assert.Equal(t, 4500, lt.w.rateLimiter.EffectivePerMinute("ses-1", 6000))
		assert.Equal(t, 1, lt.w.GetCircuitBreakerStats()["ses-1"].Failures)
	})

	t.Run("a row that cannot be given back is logged and left to the orphan sweep", func(t *testing.T) {
		rows := queuedRows("ses-1", 1)
		lt := newLaneTest(t, DefaultWorkerConfig(), rows...)
		lt.queue.releaseErr = errors.New("connection refused")
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(sesThrottle)

		ctrl := gomock.NewController(t)
		log := pkgmocks.NewMockLogger(ctrl)
		log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().Debug(gomock.Any()).AnyTimes()
		log.EXPECT().Info(gomock.Any()).AnyTimes()
		log.EXPECT().Warn(gomock.Any()).AnyTimes()
		log.EXPECT().Error("Failed to give back an email the provider refused for its pace").Times(1)
		lt.w.logger = log

		assert.Nil(t, processOne(lt.w, sesWorkspaceAt(6000), rows[0]))

		status, attempts := rowStatus(lt, rows[0].ID)
		assert.Equal(t, domain.EmailQueueStatusProcessing, status)
		assert.Equal(t, 1, attempts)
		assert.Zero(t, lt.queue.failedCount("ses-1"))
	})
}

func TestEmailQueueWorker_LaneThrottle(t *testing.T) {
	t.Run("rows waiting for a token when the pace drops are not claimed, and the round stops", func(t *testing.T) {
		// 600/min: a token every 100 ms, so the rows dispatched after the first are still
		// waiting when its refusal cuts the pace.
		lt := newLaneTest(t, DefaultWorkerConfig(), queuedRows("ses-1", 20)...)
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(sesThrottle).Times(1)

		within(t, 5*time.Second, func() { lt.w.processWorkspace(sesWorkspaceAt(600)) })

		assert.Equal(t, 1, lt.queue.claimCount("ses-1"))
		assert.Equal(t, 1, lt.queue.releasedCount("ses-1"))
		assert.Equal(t, 20, lt.queue.pending())
		assert.Equal(t, 450, lt.w.rateLimiter.EffectivePerMinute("ses-1", 600))
	})

	t.Run("the sends throttled together cut the pace once", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.SendConcurrency = 4
		lt := newLaneTest(t, cfg, queuedRows("ses-1", 4)...)

		flight := newInFlight()
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(_ context.Context, req domain.SendEmailProviderRequest, _ bool) error {
				flight.enter(req.IntegrationID)
				for flight.max(req.IntegrationID) < 4 {
					time.Sleep(time.Millisecond)
				}
				return sesThrottle
			}).Times(4)

		within(t, 5*time.Second, func() { lt.w.processWorkspace(sesWorkspaceAt(6000)) })

		assert.Equal(t, 4, lt.queue.releasedCount("ses-1"))
		assert.Equal(t, 4, lt.queue.pending())
		assert.Equal(t, 4500, lt.w.rateLimiter.EffectivePerMinute("ses-1", 6000))
		assert.Zero(t, lt.w.GetCircuitBreakerStats()["ses-1"].Failures)
	})

	t.Run("the lane fetches what the lowered pace sends in a round", func(t *testing.T) {
		rows := queuedRows("ses-1", 3)
		lt := newLaneTest(t, DefaultWorkerConfig(), rows...)
		gomock.InOrder(
			lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(sesThrottle),
			lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil).Times(3),
		)
		workspace := sesWorkspaceAt(6000)
		processOne(lt.w, workspace, rows[0])

		within(t, 5*time.Second, func() { lt.w.processWorkspace(workspace) })

		// 4,500/min over the two-second round.
		assert.Equal(t, []int{150}, lt.queue.fetchLimits("ses-1"))
		assert.Equal(t, 3, lt.queue.sentCount("ses-1"))
	})

	t.Run("a throttled probe is given back, and the circuit stays open", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.CircuitBreakerCooldown = 50 * time.Millisecond
		lt := newLaneTest(t, cfg, queuedRows("ses-1", 3)...)
		openCircuit(t, lt.w, "ses-1")
		time.Sleep(60 * time.Millisecond)
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(sesThrottle).Times(1)
		var opened atomic.Int32
		lt.w.SetCircuitOpenCallback(func(string, string, *emailerror.ClassifiedError) { opened.Add(1) })

		within(t, 5*time.Second, func() { lt.w.processWorkspace(sesWorkspaceAt(6000)) })

		assert.Equal(t, 1, lt.queue.releasedCount("ses-1"))
		assert.True(t, lt.w.circuitBreaker.IsOpen("ses-1"), "neither back nor down")
		assert.False(t, lt.w.GetCircuitBreakerStats()["ses-1"].ProbeInFlight)
		assert.Zero(t, opened.Load(), "no second pause")
		ticket, ok := lt.w.circuitBreaker.Allow("ses-1")
		assert.True(t, ok, "the next round probes without another cooldown")
		assert.True(t, ticket.Probe)
	})

	t.Run("a probe whose token wait spans a cut of the pace is given back unsent", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.CircuitBreakerCooldown = 50 * time.Millisecond
		lt := newLaneTest(t, cfg, queuedRows("ses-1", 1)...)
		openCircuit(t, lt.w, "ses-1")
		time.Sleep(60 * time.Millisecond)
		// No SendEmail is expected: the mock fails the test on any send.

		// Spend the banked token so that the probe's is half a second away.
		_, ok := lt.w.rateLimiter.ReserveBefore("ses-1", 120, time.Now().Add(time.Minute))
		require.True(t, ok)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Once the probe waits for its token, another send's refusal cuts the pace.
			if !assert.Eventually(t, func() bool { return lt.w.inFlight.Load() > 0 }, 2*time.Second, time.Millisecond) {
				return
			}
			gen := lt.w.rateLimiter.CurrentGeneration("ses-1")
			assert.True(t, lt.w.rateLimiter.RecordThrottle("ses-1", 120, gen).Decreased)
		}()
		within(t, 5*time.Second, func() { lt.w.processWorkspace(sesWorkspaceAt(120)) })
		wg.Wait()

		assert.Zero(t, lt.queue.claimCount("ses-1"))
		assert.False(t, lt.w.GetCircuitBreakerStats()["ses-1"].ProbeInFlight)
		ticket, ok := lt.w.circuitBreaker.Allow("ses-1")
		assert.True(t, ok)
		assert.True(t, ticket.Probe)
	})

	t.Run("a provider refusing every email at any pace ends up pausing", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.PollInterval = 10 * time.Millisecond
		cfg.RoundWindow = 200 * time.Millisecond
		lt := newLaneTest(t, cfg, queuedRows("ses-1", 50)...)
		lt.w.rateLimiter.floorDwell = 0
		lt.workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{sesWorkspaceAt(60000)}, nil).AnyTimes()
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(sesThrottle).AnyTimes()
		var opened atomic.Int32
		lt.w.SetCircuitOpenCallback(func(string, string, *emailerror.ClassifiedError) { opened.Add(1) })

		require.NoError(t, lt.w.Start(context.Background()))
		require.Eventually(t, func() bool { return opened.Load() > 0 }, 10*time.Second, 10*time.Millisecond)
		lt.w.Stop()

		assert.Equal(t, int32(1), opened.Load())
		assert.Equal(t, 6000, lt.w.rateLimiter.EffectivePerMinute("ses-1", 60000), "at the floor")
		// The sends in flight when the circuit opened are refused too.
		assert.GreaterOrEqual(t, lt.queue.failedCount("ses-1"), cfg.CircuitBreakerThreshold)
		assert.Zero(t, lt.queue.sentCount("ses-1"))
	})
}
