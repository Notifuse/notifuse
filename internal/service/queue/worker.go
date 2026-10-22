package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/emailerror"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// defaultRatePerMinute is the rate of an email integration whose limit is not set.
const defaultRatePerMinute = 60

// orphanBatchSize is how many rows of unknown integrations one orphan pass handles.
const orphanBatchSize = 50

// EmailQueueWorkerConfig holds configuration for the worker pool
type EmailQueueWorkerConfig struct {
	WorkerCount  int           // Number of workspaces processed in parallel (default: 5)
	PollInterval time.Duration // How often to poll for new work (default: 1s)
	BatchSize    int           // Most rows one email integration fetches per round (default: 200)
	MaxRetries   int           // Max retry attempts before permanent failure (default: 3)

	// Emails sent at once per email integration. SMTP has its own setting because each
	// email opens its own connection, and relays often limit how many they accept.
	SendConcurrency     int // default: 10
	SMTPSendConcurrency int // default: 1

	// RoundWindow is how far ahead a round dispatches. An integration fetches what its
	// rate allows in this window, and stops dispatching once its next token falls due
	// after it, so one round lasts about this long whatever the rate.
	RoundWindow time.Duration // default: 2s

	// Stop lets the sends in flight finish for ShutdownGrace, then cancels them and
	// waits at most AbortWait more for those that ignore cancellation (SMTP).
	ShutdownGrace time.Duration // default: 10s
	AbortWait     time.Duration // default: 5s

	// OrphanCheckInterval is how often each workspace is checked for rows whose email
	// integration no longer exists.
	OrphanCheckInterval time.Duration // default: 1 minute

	// Circuit breaker settings
	CircuitBreakerThreshold int           // Provider errors before opening circuit (default: 5)
	CircuitBreakerCooldown  time.Duration // Time before a probe send is let through (default: 1 minute)

	// Retention for entries that spent every attempt. They are kept so the
	// recipients they name can be retried, and they hold one compiled email each,
	// so they cannot be kept indefinitely. Seven days matches the lifetime of the
	// web-identify token baked into that email: nothing recoverable is discarded,
	// and no live credential outlives its own validity.
	TerminalRetentionDays int           // default: 7
	CleanupInterval       time.Duration // how often the sweep may run (default: 1 hour)
}

// DefaultWorkerConfig returns sensible default configuration
func DefaultWorkerConfig() *EmailQueueWorkerConfig {
	return &EmailQueueWorkerConfig{
		WorkerCount:             5,
		PollInterval:            1 * time.Second,
		BatchSize:               200,
		MaxRetries:              3,
		SendConcurrency:         10,
		SMTPSendConcurrency:     1,
		RoundWindow:             2 * time.Second,
		ShutdownGrace:           10 * time.Second,
		AbortWait:               5 * time.Second,
		OrphanCheckInterval:     time.Minute,
		CircuitBreakerThreshold: 5,
		CircuitBreakerCooldown:  getCircuitBreakerCooldown(),
		TerminalRetentionDays:   7,
		CleanupInterval:         time.Hour,
	}
}

// withSendDefaults returns a copy of c whose sending settings left at zero take their
// default, so a partial configuration cannot fetch nothing or send nothing.
func (c EmailQueueWorkerConfig) withSendDefaults() *EmailQueueWorkerConfig {
	d := DefaultWorkerConfig()
	if c.BatchSize <= 0 {
		c.BatchSize = d.BatchSize
	}
	if c.SendConcurrency <= 0 {
		c.SendConcurrency = d.SendConcurrency
	}
	if c.SMTPSendConcurrency <= 0 {
		c.SMTPSendConcurrency = d.SMTPSendConcurrency
	}
	if c.RoundWindow <= 0 {
		c.RoundWindow = d.RoundWindow
	}
	if c.ShutdownGrace <= 0 {
		c.ShutdownGrace = d.ShutdownGrace
	}
	if c.AbortWait <= 0 {
		c.AbortWait = d.AbortWait
	}
	if c.OrphanCheckInterval <= 0 {
		c.OrphanCheckInterval = d.OrphanCheckInterval
	}
	return &c
}

// laneBatchSize is how many rows an email integration fetches for one round: what its
// rate allows within the round window, at least one and at most maxBatch.
func laneBatchSize(ratePerMinute int, window time.Duration, maxBatch int) int {
	if ratePerMinute <= 0 {
		ratePerMinute = defaultRatePerMinute
	}
	const minuteMs = int64(time.Minute / time.Millisecond)
	n := (int64(ratePerMinute)*window.Milliseconds() + minuteMs - 1) / minuteMs
	if n < 1 {
		n = 1
	}
	if n > int64(maxBatch) {
		n = int64(maxBatch)
	}
	return int(n)
}

// integrationRate is the integration's rate limit, with the default when it is not set.
// It is read from the integration on every round, never from the queued payload, so an
// edited limit applies at once.
func integrationRate(integration *domain.Integration) int {
	if rate := integration.EmailProvider.RateLimitPerMinute; rate > 0 {
		return rate
	}
	return defaultRatePerMinute
}

// EmailSentCallback is called when an email is successfully sent
type EmailSentCallback func(workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string, messageID string)

// CircuitOpenCallback is called the moment an integration's circuit breaker opens.
// The transition is the signal, not the state: one outage is observed by every send in
// flight. It is called once per opening, after the sends that were in flight have
// recorded their outcome, so whatever it pauses includes them. It fires again each time
// a probe fails after a cooldown, so whatever it triggers has to be idempotent.
type CircuitOpenCallback func(workspaceID string, integrationID string, lastErr *emailerror.ClassifiedError)

// EmailFailedCallback is called when an email fails to send
type EmailFailedCallback func(workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string, messageID string, err error, isPermanent bool)

// EmailQueueWorker processes queued emails
type EmailQueueWorker struct {
	queueRepo          domain.EmailQueueRepository
	workspaceRepo      domain.WorkspaceRepository
	emailService       domain.EmailServiceInterface
	messageHistoryRepo domain.MessageHistoryRepository
	// automationRepo is optional; when set, the worker performs the stop-on-reply
	// just-in-time guard before sending automation emails flagged with a
	// contact_automation_id. Injected via SetAutomationRepo (kept out of the
	// constructor to avoid churning every caller).
	automationRepo  domain.AutomationRepository
	rateLimiter     *IntegrationRateLimiter
	circuitBreaker  *IntegrationCircuitBreaker
	errorClassifier *emailerror.Classifier
	config          *EmailQueueWorkerConfig
	logger          logger.Logger

	// lastCleanupTime throttles the retention sweep, which walks every workspace.
	lastCleanupTime time.Time

	// lastOrphanCheck throttles, per workspace, the search for rows whose email
	// integration no longer exists.
	orphanMu        sync.Mutex
	lastOrphanCheck map[string]time.Time

	// Control. Stop cancels ctx: no new round, token or claim. A send already claimed
	// runs on contexts the stop does not cancel. sendCtx carries what leads up to the
	// email leaving and is cancelled only once the shutdown grace runs out; writeCtx
	// records the outcome and is never cancelled, since a lost MarkAsSent means the
	// email is sent again after the restart.
	ctx        context.Context
	cancel     context.CancelFunc
	sendCtx    context.Context
	abortSends context.CancelFunc
	writeCtx   context.Context
	running    bool
	mu         sync.RWMutex

	// loopDone is closed when the loop of the latest Start returns, which can be after
	// Stop does: Stop gives up on a send that ignores its cancellation.
	loopDone chan struct{}

	// inFlight counts the rows dispatched and not yet done, for the shutdown log.
	inFlight atomic.Int64

	// Callbacks for progress tracking
	onEmailSent   EmailSentCallback
	onEmailFailed EmailFailedCallback

	// onCircuitOpen is optional; when unset an opening circuit only throttles.
	onCircuitOpen CircuitOpenCallback
}

// NewEmailQueueWorker creates a new EmailQueueWorker
func NewEmailQueueWorker(
	queueRepo domain.EmailQueueRepository,
	workspaceRepo domain.WorkspaceRepository,
	emailService domain.EmailServiceInterface,
	messageHistoryRepo domain.MessageHistoryRepository,
	config *EmailQueueWorkerConfig,
	log logger.Logger,
) *EmailQueueWorker {
	if config == nil {
		config = DefaultWorkerConfig()
	}
	config = config.withSendDefaults()

	// Setup circuit breaker config with defaults
	cbConfig := CircuitBreakerConfig{
		Threshold:      config.CircuitBreakerThreshold,
		CooldownPeriod: config.CircuitBreakerCooldown,
	}
	if cbConfig.Threshold == 0 {
		cbConfig.Threshold = 5
	}
	if cbConfig.CooldownPeriod == 0 {
		cbConfig.CooldownPeriod = getCircuitBreakerCooldown()
	}

	return &EmailQueueWorker{
		queueRepo:          queueRepo,
		workspaceRepo:      workspaceRepo,
		emailService:       emailService,
		messageHistoryRepo: messageHistoryRepo,
		// Seeded so the first sweep happens one interval from now. Left at the zero
		// time it would DELETE across every workspace database on the first poll tick,
		// during startup, for no reason — nothing has aged out in the meantime.
		lastCleanupTime: time.Now(),
		lastOrphanCheck: make(map[string]time.Time),
		rateLimiter:     NewIntegrationRateLimiter(),
		circuitBreaker:  NewIntegrationCircuitBreaker(cbConfig),
		errorClassifier: emailerror.NewClassifier(),
		config:          config,
		logger:          log,
	}
}

// SetCallbacks sets callback functions for progress tracking. They are called from
// concurrent sends, so they must be safe for concurrent use.
func (w *EmailQueueWorker) SetCallbacks(onSent EmailSentCallback, onFailed EmailFailedCallback) {
	w.onEmailSent = onSent
	w.onEmailFailed = onFailed
}

// SetCircuitOpenCallback registers what happens when an integration's circuit opens.
// Without one the breaker only slows the queue down and tells nobody, which is how a
// provider refusing every message could burn through a campaign's retries unnoticed.
func (w *EmailQueueWorker) SetCircuitOpenCallback(cb CircuitOpenCallback) {
	w.onCircuitOpen = cb
}

// initContexts derives the worker's contexts from the one it is started with.
func (w *EmailQueueWorker) initContexts(ctx context.Context) {
	w.ctx, w.cancel = context.WithCancel(ctx)
	w.sendCtx, w.abortSends = context.WithCancel(context.WithoutCancel(ctx))
	w.writeCtx = context.WithoutCancel(ctx)
}

// Start begins processing queued emails
func (w *EmailQueueWorker) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	if w.loopDone != nil {
		select {
		case <-w.loopDone:
		default:
			// The previous run's loop still reads the contexts Start would replace, and
			// two loops would fetch the rows the other is sending.
			w.mu.Unlock()
			return errors.New("email queue worker is still finishing a send of its previous run")
		}
	}
	w.initContexts(ctx)
	w.running = true
	loopDone := make(chan struct{})
	w.loopDone = loopDone
	w.mu.Unlock()

	w.logger.WithFields(map[string]interface{}{
		"worker_count":          w.config.WorkerCount,
		"poll_interval":         w.config.PollInterval.String(),
		"batch_size":            w.config.BatchSize,
		"send_concurrency":      w.config.SendConcurrency,
		"smtp_send_concurrency": w.config.SMTPSendConcurrency,
		"round_window":          w.config.RoundWindow.String(),
	}).Info("Starting email queue worker")

	// Start the main processing loop
	go func() {
		defer close(loopDone)
		w.processLoop()
	}()

	return nil
}

// Stop stops taking new work and waits for the sends in flight. Those still running
// after ShutdownGrace are cancelled; Stop then waits at most AbortWait for them, since
// an SMTP send cannot be interrupted, and returns.
func (w *EmailQueueWorker) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	w.cancel()
	// Read under the lock: once the loop is done, a Start can replace them.
	done, abortSends := w.loopDone, w.abortSends
	w.mu.Unlock()

	w.logger.Info("Stopping email queue worker...")

	select {
	case <-done:
		w.logger.Info("Email queue worker stopped")
		return
	case <-time.After(w.config.ShutdownGrace):
	}

	w.logger.WithFields(map[string]interface{}{
		"in_flight":      w.inFlight.Load(),
		"shutdown_grace": w.config.ShutdownGrace.String(),
	}).Warn("Emails still sending after the shutdown grace; cancelling them")
	abortSends()

	select {
	case <-done:
		w.logger.Info("Email queue worker stopped")
	case <-time.After(w.config.AbortWait):
		// Their rows stay in processing and are sent again two minutes after the restart.
		w.logger.WithField("in_flight", w.inFlight.Load()).
			Error("Email queue worker stopped with sends still running")
	}
}

// IsRunning returns whether the worker is currently running
func (w *EmailQueueWorker) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.running
}

// processLoop is the main processing loop that polls for work
func (w *EmailQueueWorker) processLoop() {

	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.processAllWorkspaces()
			if w.ctx.Err() != nil {
				return
			}
			w.cleanupTerminallyFailed()
		}
	}
}

// processAllWorkspaces runs one round: every workspace, WorkerCount at a time. It
// returns once every workspace it started is done, including after a stop, so no send
// it started outlives it.
func (w *EmailQueueWorker) processAllWorkspaces() {
	// Get list of all workspaces
	workspaces, err := w.workspaceRepo.List(w.ctx)
	if err != nil {
		if w.ctx.Err() == nil {
			w.logger.WithField("error", err.Error()).Error("Failed to list workspaces")
		}
		return
	}

	// Process each workspace concurrently
	var processWg sync.WaitGroup
	semaphore := make(chan struct{}, w.config.WorkerCount)

	for _, workspace := range workspaces {
		select {
		case <-w.ctx.Done():
		case semaphore <- struct{}{}:
		}
		if w.ctx.Err() != nil {
			break
		}

		processWg.Add(1)
		go func(ws *domain.Workspace) {
			defer processWg.Done()
			defer func() { <-semaphore }()

			w.processWorkspace(ws)
		}(workspace)
	}

	processWg.Wait()
}

// processWorkspace runs one lane per email integration of the workspace, concurrently,
// plus the orphan pass when it is due, and returns when they are all done.
func (w *EmailQueueWorker) processWorkspace(workspace *domain.Workspace) {
	integrations := workspace.GetIntegrationsByType(domain.IntegrationTypeEmail)

	var lanes sync.WaitGroup
	for _, integration := range integrations {
		lanes.Add(1)
		go func(integration *domain.Integration) {
			defer lanes.Done()
			w.processLane(workspace, integration)
		}(integration)
	}

	// The pass runs against the integrations of this round, so no row a lane of this
	// round is sending can be taken for an orphan.
	w.processOrphans(workspace, integrations)

	lanes.Wait()
}

// processLane sends one round of an integration's queue. It returns at once when the
// integration's next token falls after the round, then asks the circuit breaker,
// fetches what the integration's rate allows in a round, and dispatches the rows in
// order, at most K at a time and each when its token falls due, until the next
// token falls after the round's deadline. It returns once every send it started is done.
func (w *EmailQueueWorker) processLane(workspace *domain.Workspace, integration *domain.Integration) {
	ratePerMinute := integrationRate(integration)
	// A lane whose next token falls after this round would fetch only to send nothing.
	// Checked before Allow, so that no probe is taken for a round that cannot send it.
	if !w.rateLimiter.TokenDueBy(integration.ID, ratePerMinute, time.Now().Add(w.config.RoundWindow)) {
		return
	}

	ticket, allowed := w.circuitBreaker.Allow(integration.ID)
	if !allowed {
		// Open within its cooldown, or a probe is already out: the rows stay as they are.
		return
	}

	limit := laneBatchSize(ratePerMinute, w.config.RoundWindow, w.config.BatchSize)
	if ticket.Probe {
		// After the cooldown, one email tells whether the provider is back.
		limit = 1
	}

	probeSent := false
	defer func() {
		if ticket.Probe && !probeSent {
			w.circuitBreaker.CancelProbe(integration.ID, ticket)
		}
	}()

	entries, err := w.queueRepo.FetchPendingForIntegration(w.ctx, workspace.ID, integration.ID, limit)
	if err != nil {
		if w.ctx.Err() == nil {
			w.logger.WithFields(map[string]interface{}{
				"workspace_id":   workspace.ID,
				"integration_id": integration.ID,
				"error":          err.Error(),
			}).Error("Failed to fetch pending emails")
		}
		return
	}
	if len(entries) == 0 {
		return
	}

	// Counted from the end of the fetch, so a slow database delays the round instead
	// of leaving no time to send.
	deadline := time.Now().Add(w.config.RoundWindow)

	w.logger.WithFields(map[string]interface{}{
		"workspace_id":   workspace.ID,
		"integration_id": integration.ID,
		"count":          len(entries),
		"probe":          ticket.Probe,
	}).Debug("Processing queued emails")

	concurrency := w.config.SendConcurrency
	if integration.EmailProvider.Kind == domain.EmailProviderKindSMTP {
		concurrency = w.config.SMTPSendConcurrency
	}
	slots := make(chan struct{}, concurrency)

	var sends sync.WaitGroup
	var openedMu sync.Mutex
	var openedErr *emailerror.ClassifiedError

	for _, entry := range entries {
		select {
		case <-w.ctx.Done():
		case slots <- struct{}{}:
		}
		if w.ctx.Err() != nil {
			break
		}
		// Nothing is dispatched once a send of this round has opened the circuit.
		if !ticket.Probe && w.circuitBreaker.IsOpen(integration.ID) {
			break
		}
		reservation, delay, ok := w.rateLimiter.ReserveBefore(integration.ID, ratePerMinute, deadline)
		if !ok {
			// The next token falls in the next round; so do the rows left.
			break
		}

		var probe *Ticket
		if ticket.Probe {
			probe = &ticket
			probeSent = true
		}

		sends.Add(1)
		w.inFlight.Add(1)
		go func(entry *domain.EmailQueueEntry) {
			defer sends.Done()
			defer w.inFlight.Add(-1)
			defer func() { <-slots }()

			if !w.waitForToken(reservation, delay) {
				if probe != nil {
					w.circuitBreaker.CancelProbe(integration.ID, *probe)
				}
				return
			}
			if opened := w.processEntry(workspace, integration, entry, probe); opened != nil {
				openedMu.Lock()
				if openedErr == nil {
					openedErr = opened
				}
				openedMu.Unlock()
			}
		}(entry)
	}

	sends.Wait()

	// Every send that was in flight when the circuit opened has recorded its outcome by
	// now, so a pause triggered here holds the rows that failed alongside the one that
	// opened it, instead of leaving them to be retried.
	if openedErr != nil {
		w.logger.WithFields(map[string]interface{}{
			"integration_id": integration.ID,
			"workspace_id":   workspace.ID,
			"error":          openedErr.Error(),
		}).Warn("Circuit breaker opened for integration")
		if w.onCircuitOpen != nil {
			w.onCircuitOpen(workspace.ID, integration.ID, openedErr)
		}
	}
}

// waitForToken waits until the reserved token is due. A stop ends the wait, gives the
// token back and reports false: the row has not been claimed and stays as it is.
func (w *EmailQueueWorker) waitForToken(reservation *rate.Reservation, delay time.Duration) bool {
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-w.ctx.Done():
		}
	}
	if w.ctx.Err() != nil {
		reservation.Cancel()
		return false
	}
	return true
}

// processOrphans fails the rows whose email integration no longer exists, so that they
// still reach a terminal state. It runs at most once per OrphanCheckInterval per
// workspace, sooner while a pass finds a full batch.
func (w *EmailQueueWorker) processOrphans(workspace *domain.Workspace, integrations []*domain.Integration) {
	if !w.orphanCheckDue(workspace.ID) {
		return
	}

	integrationIDs := make([]string, 0, len(integrations))
	for _, integration := range integrations {
		integrationIDs = append(integrationIDs, integration.ID)
	}

	entries, err := w.queueRepo.FetchPendingOrphans(w.ctx, workspace.ID, integrationIDs, orphanBatchSize)
	if err != nil {
		if w.ctx.Err() == nil {
			w.logger.WithFields(map[string]interface{}{
				"workspace_id": workspace.ID,
				"error":        err.Error(),
			}).Error("Failed to fetch emails of unknown integrations")
		}
		return
	}
	if len(entries) == orphanBatchSize {
		// More are probably waiting: look again next round rather than in a minute.
		w.orphanMu.Lock()
		delete(w.lastOrphanCheck, workspace.ID)
		w.orphanMu.Unlock()
	}

	for _, entry := range entries {
		if w.ctx.Err() != nil {
			return
		}
		w.failOrphan(workspace, entry)
	}
}

// orphanCheckDue reports whether the workspace's orphan pass is due, and if so records
// that it runs now.
func (w *EmailQueueWorker) orphanCheckDue(workspaceID string) bool {
	w.orphanMu.Lock()
	defer w.orphanMu.Unlock()

	if last, ok := w.lastOrphanCheck[workspaceID]; ok && time.Since(last) < w.config.OrphanCheckInterval {
		return false
	}
	w.lastOrphanCheck[workspaceID] = time.Now()
	return true
}

// failOrphan records a failed attempt for a row whose email integration is gone. It
// spends the row's attempts like any failure, so the row ends in a terminal state
// instead of waiting forever.
func (w *EmailQueueWorker) failOrphan(workspace *domain.Workspace, entry *domain.EmailQueueEntry) {
	// Mark as processing first to increment attempts, then handle error
	if err := w.queueRepo.MarkAsProcessing(w.ctx, workspace.ID, entry.ID); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"entry_id": entry.ID,
			"error":    err.Error(),
		}).Warn("Failed to mark entry as processing")
		return
	}
	w.handleError(workspace, entry, fmt.Errorf("integration not found: %s", entry.IntegrationID), nil)
}

// SetAutomationRepo injects the automation repository used by the stop-on-reply
// just-in-time guard. Optional; when unset the guard is skipped.
func (w *EmailQueueWorker) SetAutomationRepo(repo domain.AutomationRepository) {
	w.automationRepo = repo
}

// processEntry claims and sends one row whose token is due. A probe row carries the
// ticket its lane was granted; any other row asks the breaker now, since the circuit
// may have opened while it waited for its token. It returns the classified error when
// this send's failure opened the circuit, for the lane to act on once its sends are done.
func (w *EmailQueueWorker) processEntry(workspace *domain.Workspace, integration *domain.Integration, entry *domain.EmailQueueEntry, probe *Ticket) *emailerror.ClassifiedError {
	var ticket Ticket
	if probe != nil {
		ticket = *probe
	} else {
		var allowed bool
		ticket, allowed = w.circuitBreaker.Allow(integration.ID)
		if !allowed {
			w.logger.WithFields(map[string]interface{}{
				"entry_id":       entry.ID,
				"integration_id": integration.ID,
			}).Debug("Circuit breaker open, leaving the email queued")
			return nil
		}
	}

	// A probe that never reaches the provider is given back, so the next round can
	// probe at once instead of after another cooldown.
	settled := false
	defer func() {
		if ticket.Probe && !settled {
			w.circuitBreaker.CancelProbe(integration.ID, ticket)
		}
	}()

	if w.ctx.Err() != nil {
		return nil
	}

	// Mark as processing (this increments attempts)
	if err := w.queueRepo.MarkAsProcessing(w.ctx, workspace.ID, entry.ID); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"entry_id": entry.ID,
			"error":    err.Error(),
		}).Warn("Failed to mark entry as processing, may be processed by another worker")
		return nil
	}

	// Build the send request
	request := entry.Payload.ToSendEmailProviderRequest(
		workspace.ID,
		entry.IntegrationID,
		entry.MessageID,
		entry.ContactEmail,
		&integration.EmailProvider,
	)

	// Stop-on-reply for capture providers (e.g. SES, which overwrites the Message-ID): give
	// the send a place to write the provider-returned MessageId so we can store the
	// recipient-visible Message-ID post-send. Only the captured value is meaningful here.
	var capturedMessageID string
	if entry.SourceType == domain.EmailQueueSourceAutomation && domain.ProviderCapturesMessageID(entry.ProviderKind) {
		request.CapturedMessageID = &capturedMessageID
	}

	// Stop-on-reply just-in-time guard: for automation sends flagged with a
	// contact_automation_id, re-check the journey is still active right before
	// sending. If a reply exited it after this email was enqueued, cancel the send.
	//
	// This check belongs HERE, at the actual provider call, and moving it earlier
	// reopens the hole it exists to close. Stopping a journey has two async windows:
	// a contact asleep in a delay, which the event interrupt closes by flipping the
	// journey status so the scheduler stops picking it up; and an email already
	// sitting in this queue, which nothing upstream can recall. The email node only
	// enqueues, so a guard there proves nothing about the moment of sending. The
	// third layer is the optimistic lock on the journey persist, which stops the
	// executor writing over an exit that landed mid-tick. Remove any one and
	// "exited" stops meaning no further mail goes out.
	if w.automationRepo != nil && entry.Payload.ContactAutomationID != nil {
		ca, lookupErr := w.automationRepo.GetContactAutomation(w.sendCtx, workspace.ID, *entry.Payload.ContactAutomationID)
		if lookupErr != nil {
			// Fail open (send) but make it observable — a silent allow could mask a
			// systematically broken guard.
			w.logger.WithFields(map[string]interface{}{
				"entry_id":              entry.ID,
				"contact_automation_id": *entry.Payload.ContactAutomationID,
				"error":                 lookupErr.Error(),
			}).Warn("Stop-on-reply JIT guard lookup failed; proceeding with send (fail-open)")
		} else if ca != nil && ca.Status != domain.ContactAutomationStatusActive {
			w.logger.WithFields(map[string]interface{}{
				"entry_id":              entry.ID,
				"contact_automation_id": *entry.Payload.ContactAutomationID,
				"status":                string(ca.Status),
			}).Info("Skipping automation email: journey no longer active (stop-on-reply)")
			// Remove the entry from the queue without sending.
			if delErr := w.queueRepo.MarkAsSent(w.writeCtx, workspace.ID, entry.ID); delErr != nil {
				w.logger.WithFields(map[string]interface{}{
					"entry_id": entry.ID,
					"error":    delErr.Error(),
				}).Warn("Failed to remove cancelled (stop-on-reply) entry")
			}
			return nil
		}
	}

	// Stop-on-reply: persist the matchable message_history row (carrying smtp_message_id)
	// BEFORE the email physically leaves the provider, so a fast inbound reply (e.g. an
	// auto-responder) can always resolve via GetBySMTPMessageID even if it arrives in the
	// window before the post-send upsert. The post-send upsert below preserves the value
	// (ON CONFLICT COALESCE). Only matters for automation sends where we set the Message-ID.
	if entry.SourceType == domain.EmailQueueSourceAutomation && domain.ProviderSetsOwnMessageID(entry.ProviderKind) {
		w.upsertMessageHistory(w.sendCtx, workspace.ID, workspace.Settings.SecretKey, entry, "", nil)
	}

	// Send the email
	err := w.emailService.SendEmail(w.sendCtx, *request, true) // isMarketing = true
	if err != nil {
		if w.sendCtx.Err() != nil {
			// Cancelled because the shutdown grace ran out: the provider did not refuse
			// the email, and may even have accepted it. Recording a failure would count
			// against the provider and schedule a retry on top of the possible send, so
			// the row stays in processing, as a send cut by a crash would, and is
			// retried two minutes after the restart.
			w.logger.WithFields(map[string]interface{}{
				"entry_id": entry.ID,
				"error":    err.Error(),
			}).Warn("Send aborted at shutdown; the email will be retried after the restart")
			return nil
		}

		// Classify the error
		classifiedErr := w.errorClassifier.Classify(err, integration.EmailProvider.Kind)

		// Log the classification for debugging
		w.logger.WithFields(map[string]interface{}{
			"entry_id":    entry.ID,
			"error_type":  classifiedErr.Type,
			"provider":    classifiedErr.Provider,
			"http_status": classifiedErr.HTTPStatus,
			"retryable":   classifiedErr.Retryable,
			"original":    err.Error(),
		}).Debug("Classified send error")

		// Record failure to circuit breaker (only counts provider errors)
		settled = true
		counted, opened := w.circuitBreaker.RecordFailure(integration.ID, ticket, classifiedErr)

		// A send allowed before the circuit opened and refused after it shares the
		// outage of the sends that opened it. It is scheduled for a retry even when the
		// provider calls the refusal permanent, so the pause that follows the opening
		// holds it with the rest instead of giving the recipient up.
		if !counted && classifiedErr.IsProviderError() && w.circuitBreaker.IsOpen(integration.ID) {
			held := *classifiedErr
			held.Retryable = true
			classifiedErr = &held
		}

		w.handleError(workspace, entry, err, classifiedErr)
		if opened {
			return classifiedErr
		}
		return nil
	}

	settled = true
	w.circuitBreaker.RecordSuccess(integration.ID, ticket)

	// Mark as sent
	if err := w.queueRepo.MarkAsSent(w.writeCtx, workspace.ID, entry.ID); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"entry_id": entry.ID,
			"error":    err.Error(),
		}).Error("Failed to mark email as sent")
		return nil
	}

	// Upsert message history (success - clears any previous failure). capturedMessageID
	// carries the SES-returned Message-ID for the reply-matching row (empty for others).
	w.upsertMessageHistory(w.writeCtx, workspace.ID, workspace.Settings.SecretKey, entry, capturedMessageID, nil)

	w.logger.WithFields(map[string]interface{}{
		"entry_id":     entry.ID,
		"message_id":   entry.MessageID,
		"recipient":    entry.ContactEmail,
		"source_type":  entry.SourceType,
		"source_id":    entry.SourceID,
		"workspace_id": workspace.ID,
	}).Debug("Email sent successfully")

	// Call success callback
	if w.onEmailSent != nil {
		w.onEmailSent(workspace.ID, entry.SourceType, entry.SourceID, entry.MessageID)
	}
	return nil
}

// handleError handles a send error, scheduling retry or deleting permanently failed entries
// classifiedErr may be nil for internal errors (e.g., integration not found)
func (w *EmailQueueWorker) handleError(workspace *domain.Workspace, entry *domain.EmailQueueEntry, sendErr error, classifiedErr *emailerror.ClassifiedError) {
	entry.Attempts++ // Increment since MarkAsProcessing already did this

	// Determine if this is a permanent failure (non-retryable recipient error or max attempts)
	isPermanent := entry.Attempts >= entry.MaxAttempts
	if classifiedErr != nil && !classifiedErr.Retryable {
		isPermanent = true
	}

	logFields := map[string]interface{}{
		"entry_id":     entry.ID,
		"message_id":   entry.MessageID,
		"recipient":    entry.ContactEmail,
		"attempts":     entry.Attempts,
		"max_attempts": entry.MaxAttempts,
		"error":        sendErr.Error(),
		"is_permanent": isPermanent,
	}
	if classifiedErr != nil {
		logFields["error_type"] = classifiedErr.Type
	}
	w.logger.WithFields(logFields).Warn("Failed to send email")

	// Upsert message history with failure info (no captured Message-ID on a failed send).
	w.upsertMessageHistory(w.writeCtx, workspace.ID, workspace.Settings.SecretKey, entry, "", sendErr)

	if isPermanent {
		// The entry is kept, not deleted. Deleting it was what made the recipient
		// unrecoverable: nothing downstream knew the campaign had given up on anyone,
		// and there was nothing left to retry. Held in a terminal state it is inert —
		// the fetch skips a row with no attempts left — until a retry revives it or
		// the retention sweep collects it.
		w.logger.WithFields(map[string]interface{}{
			"entry_id":   entry.ID,
			"message_id": entry.MessageID,
			"attempts":   entry.Attempts,
		}).Warn("Email permanently failed")

		if err := w.queueRepo.MarkAsPermanentlyFailed(w.writeCtx, workspace.ID, entry.ID, sendErr.Error()); err != nil {
			w.logger.WithFields(map[string]interface{}{
				"entry_id": entry.ID,
				"error":    err.Error(),
			}).Error("Failed to record permanently failed queue entry")
		}

		// Call failure callback (isPermanent = true)
		if w.onEmailFailed != nil {
			w.onEmailFailed(workspace.ID, entry.SourceType, entry.SourceID, entry.MessageID, sendErr, true)
		}
		return
	}

	// Schedule retry with exponential backoff
	nextRetry := domain.CalculateNextRetryTime(entry.Attempts)
	if err := w.queueRepo.MarkAsFailed(w.writeCtx, workspace.ID, entry.ID, sendErr.Error(), &nextRetry); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"entry_id": entry.ID,
			"error":    err.Error(),
		}).Error("Failed to mark as failed for retry")
	}

	// Call failure callback (isPermanent = false, will retry)
	if w.onEmailFailed != nil {
		w.onEmailFailed(workspace.ID, entry.SourceType, entry.SourceID, entry.MessageID, sendErr, false)
	}
}

// upsertMessageHistory creates or updates a message history record after a send attempt
// On success: FailedAt and StatusInfo are nil (clears any previous failure)
// On failure: FailedAt is set to now, StatusInfo contains the error
func (w *EmailQueueWorker) upsertMessageHistory(
	ctx context.Context,
	workspaceID string,
	secretKey string,
	entry *domain.EmailQueueEntry,
	capturedMessageID string,
	sendErr error,
) {
	now := time.Now().UTC()

	message := &domain.MessageHistory{
		ID:              entry.MessageID,
		ContactEmail:    entry.ContactEmail,
		TemplateID:      entry.TemplateID,
		TemplateVersion: int64(entry.Payload.TemplateVersion),
		Channel:         "email",
		MessageData:     domain.MessageData{Data: entry.Payload.TemplateData}, // Include template data for logging
		SentAt:          entry.CreatedAt,                                      // Use queue entry creation time (stable across retries)
		CreatedAt:       entry.CreatedAt,
		UpdatedAt:       now,
	}

	// Set source (broadcast or automation)
	if entry.SourceType == domain.EmailQueueSourceBroadcast {
		message.BroadcastID = &entry.SourceID
	} else if entry.SourceType == domain.EmailQueueSourceAutomation {
		message.AutomationID = &entry.SourceID
	}
	// The list the email was sent for. The complaint/bounce trigger on message_history
	// records the contact's status on this list, and only when it is set.
	if entry.Payload.ListID != "" {
		message.ListID = &entry.Payload.ListID
	}

	// Record the recipient-visible Message-ID for ALL automation sends on providers
	// where we set the Message-ID ourselves, so an inbound reply's In-Reply-To
	// matches the exact source automation (not just exit_on_reply ones). This is
	// what lets a reply to a non-exit_on_reply automation NOT wrongly exit a
	// different exit_on_reply automation for the same contact.
	if entry.SourceType == domain.EmailQueueSourceAutomation && domain.ProviderSetsOwnMessageID(entry.ProviderKind) {
		// set_own (e.g. Mailgun): store the bracket-free value we set as the header.
		smtpMessageID := domain.RFCMessageIDValue(entry.MessageID, entry.Payload.FromAddress)
		message.SMTPMessageID = &smtpMessageID
	} else if entry.SourceType == domain.EmailQueueSourceAutomation && domain.ProviderCapturesMessageID(entry.ProviderKind) && capturedMessageID != "" {
		// capture (e.g. SES overwrites the Message-ID): store the provider-returned id,
		// reconstructed into the recipient-visible RFC Message-ID value. Only known
		// post-send, so this never runs on the pre-send upsert.
		smtpMessageID := domain.SESStoredMessageID(capturedMessageID)
		message.SMTPMessageID = &smtpMessageID
	}

	// Set failure info if send failed (will be cleared on retry success via UPSERT)
	if sendErr != nil {
		message.FailedAt = &now
		errStr := sendErr.Error()
		if len(errStr) > 255 {
			errStr = errStr[:255]
		}
		message.StatusInfo = &errStr
	}
	// On success: FailedAt and StatusInfo remain nil, clearing any previous failure

	// Upsert record (log errors but don't fail the send operation)
	if err := w.messageHistoryRepo.Upsert(ctx, workspaceID, secretKey, message); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"entry_id":   entry.ID,
			"message_id": entry.MessageID,
			"error":      err.Error(),
		}).Warn("Failed to upsert message history")
	}
}

// cleanupTerminallyFailed sweeps abandoned entries past the retention window. It runs
// on the poll loop but no more often than CleanupInterval, because it walks every
// workspace and nothing here is urgent.
func (w *EmailQueueWorker) cleanupTerminallyFailed() {
	retentionDays := w.config.TerminalRetentionDays
	if retentionDays <= 0 {
		return
	}

	interval := w.config.CleanupInterval
	if interval <= 0 {
		interval = time.Hour
	}
	if time.Since(w.lastCleanupTime) < interval {
		return
	}
	w.lastCleanupTime = time.Now()

	workspaces, err := w.workspaceRepo.List(w.ctx)
	if err != nil {
		w.logger.WithField("error", err.Error()).Error("Failed to list workspaces for queue retention sweep")
		return
	}

	for _, workspace := range workspaces {
		deleted, err := w.queueRepo.DeleteTerminallyFailedOlderThan(w.ctx, workspace.ID, retentionDays)
		if err != nil {
			w.logger.WithFields(map[string]interface{}{
				"workspace_id": workspace.ID,
				"error":        err.Error(),
			}).Error("Failed to sweep expired failed queue entries")
			continue
		}
		if deleted > 0 {
			w.logger.WithFields(map[string]interface{}{
				"workspace_id": workspace.ID,
				"deleted":      deleted,
			}).Info("Swept expired failed queue entries")
		}
	}
}

// GetStats returns statistics about the rate limiters
func (w *EmailQueueWorker) GetStats() map[string]RateLimiterStats {
	return w.rateLimiter.GetStats()
}

// GetConfig returns the worker configuration
func (w *EmailQueueWorker) GetConfig() *EmailQueueWorkerConfig {
	return w.config
}

// GetCircuitBreakerStats returns statistics about all circuit breakers
func (w *EmailQueueWorker) GetCircuitBreakerStats() map[string]CircuitBreakerStats {
	return w.circuitBreaker.GetStats()
}
