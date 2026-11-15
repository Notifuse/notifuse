package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/tracing"
)

// Maximum time a task can run before timing out
const defaultMaxTaskRuntime = 50 // 50 seconds

// taskExecutionGrace is how long past a task's own timeout its execution context
// is allowed to run before it is hard-cancelled. Processors are expected to stop
// on their own before their timeout, so this only ever fires on a call that is
// genuinely stuck (e.g. blocked on a DB lock); the grace ensures a task that
// merely runs right up to its budget is never cut. The deadline is applied
// inside ExecuteTask so every entry point gets the same bound: the in-process
// scheduler (whose batch-wide wait a single stuck task would otherwise freeze
// forever), the HTTP dispatch handler, and the synchronous segment-service
// triggers that call with a background context. The stored lease is extended by
// the same grace so a re-dispatched execution cannot overlap one that is still
// inside its grace window.
const taskExecutionGrace = 30 * time.Second

// interruptedProcessorWait is how long an execution cancelled from outside (a stop) waits for
// its processor before it hands the task back. The processor saves where it got to (a
// broadcast's cursor) on its lease, which the hand-back ends: released first, that save matches
// no row, and the next run queues the batch just queued again.
const interruptedProcessorWait = 3 * time.Second

// dispatchResponseMargin is the slack added on top of a task's own execution
// bound before the dispatcher stops waiting for the handler's reply.
const dispatchResponseMargin = 10 * time.Second

// dispatchTimeout returns how long a dispatch should wait for /api/tasks.execute
// to answer. The handler executes the task synchronously and only replies when
// it is done, so this has to cover the task's whole permitted lifetime:
// MaxRuntime plus the grace window ExecuteTask applies, plus a little slack for
// the round trip.
func dispatchTimeout(t *domain.Task) time.Duration {
	maxRuntime := t.MaxRuntime
	if maxRuntime <= 0 {
		maxRuntime = defaultMaxTaskRuntime
	}
	return time.Duration(maxRuntime)*time.Second + taskExecutionGrace + dispatchResponseMargin
}

// newDispatchHTTPClient builds the client shared by all task dispatches.
//
// CheckRedirect: refuse to follow redirects. The dispatch target is our own
// /api/tasks.execute; any 3xx response means something in front of the API
// (auth proxy, TLS-upgrading ingress, CDN) has intercepted the request. The
// Go default would follow a 302 as a GET to the Location URL — if that URL
// is an auth-wall login page returning 200 HTML, the status check treats it
// as success and the task never runs. ErrUseLastResponse returns the 3xx
// response unfollowed so the non-200 branch catches it and logs loudly.
// See #320 / #317 (Cloudflare Access intercepting dispatch).
//
// Timeout is only a backstop against a connection that never answers: every
// dispatch carries its own, shorter deadline (see newDispatchRequest). It must
// stay above dispatchTimeout for the longest-running task type, or it becomes
// the binding constraint again — which is what the old fixed 53s was.
func newDispatchHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
}

// newDispatchRequest builds one dispatch bounded by the task's own budget: the
// handler runs the task to completion before answering, so the dispatcher has
// to wait at least as long as the task may legitimately run. A fixed timeout
// here truncated longer task types and left only seconds of margin on a
// broadcast slice — the gap an enqueue transaction was cancelled in.
//
// The returned cancel func must be called by the caller when the dispatch is
// done. Using NewRequestWithContext (rather than NewRequest) is also what
// propagates the trace to the handler.
func (s *TaskService) newDispatchRequest(ctx context.Context, t *domain.Task, body []byte) (*http.Request, context.CancelFunc, error) {
	reqCtx, cancel := context.WithTimeout(ctx, dispatchTimeout(t))

	endpoint := fmt.Sprintf("%s/api/tasks.execute", s.apiEndpoint)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		cancel()
		return nil, nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Task-ID", t.ID) // Add task ID for tracing

	// Sign the dispatch. /api/tasks.execute has no session to authenticate — the
	// caller is this process talking to itself through the ingress — so the
	// signature is what tells the handler the request came from the scheduler and
	// not from anyone who learned a task id. The path signed is the one the
	// handler sees; the body is in the signed content because every dispatch hits
	// the same path.
	if secret := s.signingSecret(); secret != "" {
		timestamp := time.Now().UTC().Unix()
		req.Header.Set(domain.TaskExecuteTimestampHeader, strconv.FormatInt(timestamp, 10))
		req.Header.Set(domain.TaskExecuteSignatureHeader,
			domain.SignTaskExecuteRequest(domain.TaskExecuteSigningKey(secret), timestamp, req.URL.Path, body))
	}

	return req, cancel, nil
}

// TaskService manages task execution and state
type TaskService struct {
	repo        domain.TaskRepository
	settingRepo domain.SettingRepository
	logger      logger.Logger
	authService *AuthService
	processors  map[string]domain.TaskProcessor
	lock        sync.RWMutex
	apiEndpoint string
	// autoExecuteImmediate controls whether tasks are automatically executed when set to immediate
	// This is mainly used to disable auto-execution during testing
	autoExecuteImmediate bool
	// sendNowDelay is how far ahead a send-now broadcast task is due; see
	// sendNowTaskDelay and SetSendNowDelay.
	sendNowDelay time.Duration
	// directExecution controls how due tasks are executed by ExecutePendingTasks: when true,
	// each task runs in-process instead of being dispatched over HTTP to /api/tasks.execute.
	// Wired from TaskScheduler.Enabled — an instance running its own scheduler executes its
	// own tasks directly (no self-call); see SetDirectExecution.
	directExecution bool
	// secretKey signs HTTP dispatches to /api/tasks.execute. Set through
	// SetSecretKey rather than taken by the constructor, which has 37 call sites.
	secretKey string
	// interruptedWait overrides interruptedProcessorWait when positive (tests).
	interruptedWait time.Duration

	// stopCtx is the app's shutdown context (SetShutdownContext): the runs started outside
	// the task scheduler stop with it, and runs counts them so the stop can wait for them.
	stopCtx context.Context
	runs    runCounter
}

// runCounter counts the runs in flight, and lets a stop wait until there are none.
type runCounter struct {
	mu   sync.Mutex
	n    int
	idle chan struct{} // closed when n falls to zero while a wait is under way
}

// SetShutdownContext gives the service the app's shutdown context: the runs BeginRun starts
// are cancelled with it.
func (s *TaskService) SetShutdownContext(ctx context.Context) {
	s.runs.mu.Lock()
	defer s.runs.mu.Unlock()
	s.stopCtx = ctx
}

// BeginRun starts a task run outside the task scheduler ("send now", a cron call). Its context
// keeps parent's values but not its cancellation, since a caller that gives up must not cut a
// run mid-batch, and is cancelled when the app stops; the stop then waits for done (WaitRuns).
// done does not cancel the context: work the run handed off before it ended, such as the HTTP
// dispatches of a cron call, keeps it, and is no longer cancelled by the stop either.
// It reports false, and starts nothing, once the stop has begun.
func (s *TaskService) BeginRun(parent context.Context) (context.Context, func(), bool) {
	s.runs.mu.Lock()
	defer s.runs.mu.Unlock()
	stopCtx := s.stopCtx
	if stopCtx != nil && stopCtx.Err() != nil {
		return nil, nil, false
	}
	s.runs.n++
	ctx := context.WithoutCancel(parent)
	stopCancelling := func() bool { return false }
	if stopCtx != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		stopCancelling = context.AfterFunc(stopCtx, cancel)
	}
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			stopCancelling()
			s.runs.mu.Lock()
			defer s.runs.mu.Unlock()
			s.runs.n--
			if s.runs.n == 0 && s.runs.idle != nil {
				close(s.runs.idle)
				s.runs.idle = nil
			}
		})
	}, true
}

// WaitRuns waits, at most timeout, for the runs BeginRun started to end. It reports whether
// they did.
func (s *TaskService) WaitRuns(timeout time.Duration) bool {
	s.runs.mu.Lock()
	if s.runs.n == 0 {
		s.runs.mu.Unlock()
		return true
	}
	if s.runs.idle == nil {
		s.runs.idle = make(chan struct{})
	}
	idle := s.runs.idle
	s.runs.mu.Unlock()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-idle:
		return true
	case <-timer.C:
		return false
	}
}

// WithTransaction executes a function within a transaction
func (s *TaskService) WithTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	// The repository should handle the transaction
	repo, ok := s.repo.(interface {
		WithTransaction(ctx context.Context, fn func(*sql.Tx) error) error
	})
	if !ok {
		return fmt.Errorf("repository does not support transactions")
	}

	return repo.WithTransaction(ctx, fn)
}

// NewTaskService creates a new task service instance
func NewTaskService(repository domain.TaskRepository, settingRepo domain.SettingRepository, logger logger.Logger, authService *AuthService, apiEndpoint string) *TaskService {

	return &TaskService{
		repo:                 repository,
		settingRepo:          settingRepo,
		logger:               logger,
		authService:          authService,
		processors:           make(map[string]domain.TaskProcessor),
		apiEndpoint:          apiEndpoint,
		autoExecuteImmediate: true, // Enable auto-execution by default
		sendNowDelay:         sendNowTaskDelay,
	}
}

// SetSendNowDelay sets how far ahead a send-now broadcast task is due. A value of
// zero or less keeps sendNowTaskDelay. The integration harness shortens it. A
// shorter delay is still safe but not free: a run that finds the broadcast still a
// draft hands the task back for the orchestrator's draft recheck interval, far
// longer than the delay saves. The harness can afford it because its tests start
// running tasks only once the schedule request has returned, after the broadcast
// has committed.
func (s *TaskService) SetSendNowDelay(d time.Duration) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if d <= 0 {
		d = sendNowTaskDelay
	}
	s.sendNowDelay = d
}

// SetAutoExecuteImmediate sets whether tasks should be automatically executed immediately
// This is mainly used for testing to disable auto-execution
func (s *TaskService) SetAutoExecuteImmediate(enabled bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.autoExecuteImmediate = enabled
}

// IsAutoExecuteEnabled returns whether automatic task execution is enabled
func (s *TaskService) IsAutoExecuteEnabled() bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.autoExecuteImmediate
}

// SetDirectExecution controls how ExecutePendingTasks runs due tasks. When true, each task
// is executed in-process; when false, it is dispatched over HTTP to {apiEndpoint}/api/tasks.execute.
// The app wires this from TaskScheduler.Enabled: an instance that runs its own scheduler executes
// its own tasks directly (avoiding a self-call through the public ingress), while a scale-out node
// with the scheduler disabled keeps HTTP fan-out. The apiEndpoint=="" fallback to direct execution
// is preserved independently of this flag.
func (s *TaskService) SetDirectExecution(enabled bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.directExecution = enabled
}

// SetSecretKey sets the installation's SECRET_KEY, from which the key that signs
// HTTP dispatches to /api/tasks.execute is derived. A service left without one
// dispatches unsigned, which the handler rejects — that is the intended failure:
// an unsigned dispatch is indistinguishable from anyone else's.
func (s *TaskService) SetSecretKey(secretKey string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.secretKey = secretKey
}

// signingSecret reads the dispatch signing secret. Dispatches are built from
// per-task goroutines, so the read is locked like every other field here.
func (s *TaskService) signingSecret() string {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.secretKey
}

// IsDirectExecution returns whether in-process (direct) task execution is enabled.
func (s *TaskService) IsDirectExecution() bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.directExecution
}

// RegisterProcessor registers a task processor for a specific task type
func (s *TaskService) RegisterProcessor(processor domain.TaskProcessor) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Determine which task types this processor can handle
	for _, taskType := range getTaskTypes() {
		if processor.CanProcess(taskType) {
			s.processors[taskType] = processor
			s.logger.WithField("task_type", taskType).Info("Registered task processor")
		}
	}
}

// getTaskTypes returns all supported task types
func getTaskTypes() []string {
	// This could be expanded with more task types as needed
	return []string{
		"import_contacts",
		"export_contacts",
		"send_broadcast",
		"generate_report",
		"build_segment",
		"process_contact_segment_queue",
		"check_segment_recompute",
		"sync_integration",
		domain.WebAnalyticsBackfillTaskType,
	}
}

// GetProcessor returns the processor for a given task type
func (s *TaskService) GetProcessor(taskType string) (domain.TaskProcessor, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	processor, ok := s.processors[taskType]
	if !ok {
		return nil, fmt.Errorf("no processor registered for task type: %s", taskType)
	}

	return processor, nil
}

// CreateTask creates a new task
func (s *TaskService) CreateTask(ctx context.Context, workspace string, task *domain.Task) error {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "CreateTask")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", workspace)
	if task.BroadcastID != nil {
		tracing.AddAttribute(ctx, "broadcast_id", *task.BroadcastID)
	}
	tracing.AddAttribute(ctx, "task_type", task.Type)

	if task.MaxRuntime <= 0 {
		task.MaxRuntime = defaultMaxTaskRuntime
	}

	// Set default retry settings if not provided
	if task.MaxRetries <= 0 {
		task.MaxRetries = 3
	}
	if task.RetryInterval <= 0 {
		task.RetryInterval = 60 // Default to 1 minute between retries
	}

	err := s.repo.Create(ctx, workspace, task)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
	}
	return err
}

// GetTask retrieves a task by ID
func (s *TaskService) GetTask(ctx context.Context, workspace, id string) (*domain.Task, error) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "GetTask")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", workspace)
	tracing.AddAttribute(ctx, "task_id", id)

	task, err := s.repo.Get(ctx, workspace, id)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
	} else if task != nil && task.BroadcastID != nil {
		tracing.AddAttribute(ctx, "broadcast_id", *task.BroadcastID)
	}

	return task, err
}

// ListTasks lists tasks based on filter criteria
func (s *TaskService) ListTasks(ctx context.Context, workspace string, filter domain.TaskFilter) (*domain.TaskListResponse, error) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "ListTasks")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", workspace)
	tracing.AddAttribute(ctx, "limit", filter.Limit)
	tracing.AddAttribute(ctx, "offset", filter.Offset)

	// Removed Status and Type tracing attributes to fix compilation issues

	tasks, totalCount, err := s.repo.List(ctx, workspace, filter)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		return nil, err
	}

	// Calculate if there are more results
	hasMore := (filter.Offset + len(tasks)) < totalCount
	tracing.AddAttribute(ctx, "total_count", totalCount)
	tracing.AddAttribute(ctx, "result_count", len(tasks))
	tracing.AddAttribute(ctx, "has_more", hasMore)

	return &domain.TaskListResponse{
		Tasks:      tasks,
		TotalCount: totalCount,
		Limit:      filter.Limit,
		Offset:     filter.Offset,
		HasMore:    hasMore,
	}, nil
}

// DeleteTask removes a task
func (s *TaskService) DeleteTask(ctx context.Context, workspace, id string) error {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "DeleteTask")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", workspace)
	tracing.AddAttribute(ctx, "task_id", id)

	err := s.repo.Delete(ctx, workspace, id)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
	}
	return err
}

// ExecutePendingTasks processes a batch of pending tasks
func (s *TaskService) ExecutePendingTasks(ctx context.Context, maxTasks int) error {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "ExecutePendingTasks")
	defer tracing.EndSpan(span, nil)

	// Set the last cron run timestamp before processing tasks
	if err := s.settingRepo.SetLastCronRun(ctx); err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithField("error", err.Error()).Error("Failed to set last cron run timestamp")
		// Continue processing tasks even if setting update fails
	} else {
		s.logger.Debug("Updated last cron run timestamp")
	}

	// Get the next batch of tasks
	// each workspace has a permanent task to process the contact segment queue
	// TODO/problem: if new number of workspaces is above 100, this will not work
	// we need to have a way to scale this
	if maxTasks <= 0 {
		maxTasks = 100 // Default value
	}

	tracing.AddAttribute(ctx, "max_tasks", maxTasks)

	tasks, err := s.repo.GetNextBatch(ctx, maxTasks)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		return fmt.Errorf("failed to get next batch of tasks: %w", err)
	}

	tracing.AddAttribute(ctx, "task_count", len(tasks))
	s.logger.WithField("task_count", len(tasks)).Info("Retrieved batch of tasks to process")

	// Direct (in-process) execution is used when this instance runs its own scheduler
	// (directExecution, wired from TaskScheduler.Enabled) or when no API endpoint is
	// configured. Otherwise tasks are dispatched over HTTP for fan-out across replicas.
	if s.directExecution || s.apiEndpoint == "" {
		tracing.AddAttribute(ctx, "execution_mode", "direct")
		if s.directExecution {
			s.logger.Debug("Executing pending tasks in-process (direct execution mode)")
		} else {
			s.logger.Warn("API endpoint not configured, falling back to direct execution")
		}
		return s.executeTasksDirectly(ctx, tasks)
	}

	tracing.AddAttribute(ctx, "execution_mode", "http")

	// One client shared across dispatches. Per Go docs: "Clients and Transports
	// are safe for concurrent use by multiple goroutines and for efficiency
	// should only be created once and re-used." See newDispatchHTTPClient for
	// the redirect and timeout rationale.
	httpClient := newDispatchHTTPClient()
	httpClient = tracing.WrapHTTPClient(httpClient)

	// Fire-and-forget dispatch: each task runs in its own goroutine bounded by
	// the http.Client's 53s timeout. The scheduler tick doesn't wait for
	// in-flight dispatches so one slow handler can't delay the next tick.
	//
	// Duplicate-dispatch safety: GetNextBatch only picks up pending/paused
	// tasks (with next_run_after elapsed) or running tasks whose timeout_after
	// has expired. Once the handler commits MarkAsRunningTx the row becomes
	// running with a fresh timeout_after and is skipped by subsequent batches.
	// The narrow race (tick N+1 fires before tick N's handler commits) is
	// handled server-side by MarkAsRunningTx returning ErrTaskAlreadyRunning,
	// which the client logs at Debug on a 409.
	for _, task := range tasks {
		go func(t *domain.Task) {
			taskCtx, taskSpan := tracing.StartServiceSpan(ctx, "TaskService", "DispatchTaskExecution")
			defer tracing.EndSpan(taskSpan, nil)

			tracing.AddAttribute(taskCtx, "task_id", t.ID)
			tracing.AddAttribute(taskCtx, "workspace_id", t.WorkspaceID)
			tracing.AddAttribute(taskCtx, "task_type", t.Type)
			if t.BroadcastID != nil {
				tracing.AddAttribute(taskCtx, "broadcast_id", *t.BroadcastID)
			}

			s.logger.WithField("task_id", t.ID).
				WithField("workspace_id", t.WorkspaceID).
				Info("Dispatching task execution via HTTP")

			// Create request payload
			reqBody, err := json.Marshal(domain.ExecuteTaskRequest{
				WorkspaceID: t.WorkspaceID,
				ID:          t.ID,
			})
			if err != nil {
				tracing.MarkSpanError(taskCtx, err)
				s.logger.WithField("task_id", t.ID).
					WithField("workspace_id", t.WorkspaceID).
					WithField("error", err.Error()).
					Error("Failed to marshal task execution request")
				return
			}

			req, cancelReq, err := s.newDispatchRequest(taskCtx, t, reqBody)
			if err != nil {
				tracing.MarkSpanError(taskCtx, err)
				s.logger.WithField("task_id", t.ID).
					WithField("workspace_id", t.WorkspaceID).
					WithField("error", err.Error()).
					Error("Failed to create HTTP request for task execution")
				return
			}
			defer cancelReq()

			// Execute request
			resp, err := httpClient.Do(req)
			if err != nil {
				tracing.MarkSpanError(taskCtx, err)
				s.logger.WithField("task_id", t.ID).
					WithField("workspace_id", t.WorkspaceID).
					WithField("error", err.Error()).
					Error("HTTP request for task execution failed")
				return
			}
			defer func() { _ = resp.Body.Close() }()

			// Check response
			if resp.StatusCode != http.StatusOK {
				// Bound the body read — a misrouted dispatch can land on an
				// auth-wall HTML page of tens of KB, and this path fires every
				// tick until the misconfig is fixed.
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))

				// 409 Conflict means task is already running - this is expected in concurrent scenarios
				if resp.StatusCode == http.StatusConflict {
					s.logger.WithField("task_id", t.ID).
						WithField("workspace_id", t.WorkspaceID).
						Debug("Task already running, skipping duplicate dispatch")
					return
				}

				// Other non-OK statuses are actual errors. For 3xx, include the
				// Location header so an auth-wall / ingress redirect is
				// immediately obvious in the log.
				logEntry := s.logger.WithField("task_id", t.ID).
					WithField("workspace_id", t.WorkspaceID).
					WithField("status_code", resp.StatusCode).
					WithField("response", string(body))
				if resp.StatusCode >= 300 && resp.StatusCode < 400 {
					logEntry = logEntry.WithField("location", resp.Header.Get("Location"))
				}
				err := fmt.Errorf("non-OK status: %d, response: %s", resp.StatusCode, string(body))
				tracing.MarkSpanError(taskCtx, err)
				logEntry.Error("HTTP request for task execution returned non-OK status")
				return
			}

			s.logger.WithField("task_id", t.ID).
				WithField("workspace_id", t.WorkspaceID).
				WithField("status_code", resp.StatusCode).
				Info("Task execution request dispatched successfully")
		}(task)
	}

	return nil
}

// executeTasksDirectly processes tasks directly without HTTP roundtrips
// This is used as a fallback when API endpoint is not configured
func (s *TaskService) executeTasksDirectly(ctx context.Context, tasks []*domain.Task) error {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "executeTasksDirectly")
	defer tracing.EndSpan(span, nil)

	now := time.Now().UTC()

	tracing.AddAttribute(ctx, "task_count", len(tasks))

	// Use a wait group to wait for all goroutines to complete
	var wg sync.WaitGroup

	for _, task := range tasks {
		// Calculate timeout time instead of using context timeout
		timeoutAt := now.Add(time.Duration(task.MaxRuntime) * time.Second)

		// Add to wait group before launching goroutine
		wg.Add(1)

		// Handle the task in a goroutine
		go func(t *domain.Task, timeout time.Time) {
			defer wg.Done() // Signal completion when goroutine finishes

			execCtx, execSpan := tracing.StartServiceSpan(ctx, "TaskService", "executeTaskDirectly")

			// ExecuteTask bounds its own execution at timeout+taskExecutionGrace,
			// so a stuck task cannot freeze the batch-wide wg.Wait() below.

			// Set task attributes
			tracing.AddAttribute(execCtx, "task_id", t.ID)
			tracing.AddAttribute(execCtx, "workspace_id", t.WorkspaceID)
			tracing.AddAttribute(execCtx, "task_type", t.Type)
			tracing.AddAttribute(execCtx, "timeout_at", timeout.Format(time.RFC3339))
			if t.BroadcastID != nil {
				tracing.AddAttribute(execCtx, "broadcast_id", *t.BroadcastID)
			}

			// Ensure we clean up and handle timeout
			defer func() {
				tracing.EndSpan(execSpan, nil)
			}()

			if err := s.ExecuteTask(execCtx, t.WorkspaceID, t.ID, timeout); err != nil {
				tracing.MarkSpanError(execCtx, err)
				logExecutionFailure(s.logger.WithField("task_id", t.ID).
					WithField("workspace_id", t.WorkspaceID), err, "Failed to execute task")
			}
		}(task, timeoutAt)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	return nil
}

// logExecutionFailure logs an error ExecuteTask returned: at Warn for a task that no longer
// exists (deleted since it was picked or named), at Error otherwise.
func logExecutionFailure(entry logger.Logger, err error, message string) {
	entry = entry.WithField("error", err.Error())
	var notFound *domain.ErrNotFound
	if errors.As(err, &notFound) || errors.Is(err, domain.ErrTaskNotFound) {
		entry.Warn(message + ": the task no longer exists")
		return
	}
	entry.Error(message)
}

// requeueTask hands a task back as its processor asked (domain.TaskRequeueError): pending,
// or paused when parked, due req.After from now, with its retry count, progress and state.
// A task another decision moved on meanwhile (a cancel marked it failed) is left as it is.
func (s *TaskService) requeueTask(ctx context.Context, task *domain.Task, req *domain.TaskRequeueError) error {
	status := domain.TaskStatusPending
	if req.Park {
		status = domain.TaskStatusPaused
	}
	nextRun := time.Now().UTC().Add(req.After)
	fields := map[string]interface{}{
		"task_id":      task.ID,
		"workspace_id": task.WorkspaceID,
		"status":       string(status),
		"next_run":     nextRun,
		"reason":       req.Reason,
	}
	if err := s.repo.RequeueTask(ctx, task.WorkspaceID, task.ID, status, nextRun, task.Progress, task.State); err != nil {
		if errors.Is(err, domain.ErrTaskNotRunning) {
			s.logger.WithFields(fields).Info("Task to requeue was already finalized elsewhere - leaving it alone")
			return nil
		}
		fields["error"] = err.Error()
		s.logger.WithFields(fields).Error("Failed to requeue task")
		return &domain.ErrTaskExecution{TaskID: task.ID, Reason: "failed to requeue task", Err: err}
	}
	s.logger.WithFields(fields).Info("Task handed back for a later run")
	if req.AfterRequeue != nil {
		req.AfterRequeue(ctx)
	}
	return nil
}

// ExecuteTask executes a specific task
func (s *TaskService) ExecuteTask(ctx context.Context, workspace, taskID string, timeoutAt time.Time) error {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "ExecuteTask")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", workspace)
	tracing.AddAttribute(ctx, "task_id", taskID)

	// First check if the context is already cancelled
	if ctx.Err() != nil {
		tracing.MarkSpanError(ctx, ctx.Err())
		return ctx.Err()
	}

	// Hard backstop for the whole execution: a processor whose DB call hangs on
	// a lock is cancelled at timeout+grace instead of blocking its caller
	// forever. Applied here — the choke point every entry path funnels through —
	// so the in-process scheduler, the HTTP dispatch handler, and the
	// synchronous segment-service triggers (which pass a background context) all
	// get the same bound. If the caller's context carries an earlier deadline,
	// the earlier one wins. Terminal status writes below use a detached bgCtx
	// and are unaffected by this cancellation.
	//
	// The lease is truncated to the microsecond the timeout_after column keeps, so the
	// value the run holds is exactly the stored one: SaveState matches it (WithTaskLease).
	lease := timeoutAt.Add(taskExecutionGrace).UTC().Truncate(time.Microsecond)
	ctx, cancelExec := context.WithDeadline(ctx, lease)
	defer cancelExec()

	// Get the task
	var task *domain.Task
	var processor domain.TaskProcessor

	// Wrap the initial setup operations in a transaction
	err := s.WithTransaction(ctx, func(tx *sql.Tx) error {
		txCtx, txSpan := tracing.StartServiceSpan(ctx, "TaskService", "ExecuteTaskTransaction")
		defer tracing.EndSpan(txSpan, nil)

		var taskErr error
		task, taskErr = s.repo.GetTx(txCtx, tx, workspace, taskID)
		if taskErr != nil {
			// Not logged here: each caller logs the error it gets once, a missing task
			// at Warn (logExecutionFailure).
			tracing.MarkSpanError(txCtx, taskErr)
			if errors.Is(taskErr, domain.ErrTaskNotFound) {
				return &domain.ErrNotFound{
					Entity: "task",
					ID:     taskID,
				}
			}
			return fmt.Errorf("failed to load task for execution: %w", taskErr)
		}

		if task != nil {
			tracing.AddAttribute(txCtx, "task_type", task.Type)
			if task.BroadcastID != nil {
				tracing.AddAttribute(txCtx, "broadcast_id", *task.BroadcastID)
			}
		}

		// Get the processor for this task type
		var procErr error
		processor, procErr = s.GetProcessor(task.Type)
		if procErr != nil {
			tracing.MarkSpanError(txCtx, procErr)
			s.logger.WithFields(map[string]interface{}{
				"task_id":      taskID,
				"workspace_id": workspace,
				"task_type":    task.Type,
				"error":        procErr.Error(),
			}).Error("Failed to get processor for task type")
			return &domain.ErrTaskExecution{
				TaskID: taskID,
				Reason: "no processor registered for task type",
				Err:    procErr,
			}
		}

		// Use the passed timeoutAt parameter
		tracing.AddAttribute(txCtx, "timeout_at", timeoutAt.Format(time.RFC3339))

		// Mark task as running within the same transaction. The stored lease
		// extends past the task's own timeout by the same grace as the execution
		// context: GetNextBatch re-dispatches a running task once its lease
		// expires, and a lease equal to the bare timeout would let a second
		// execution start while the first is still inside its grace window —
		// the graced one would then write terminal state over the row the new
		// execution owns. With the grace included, the first execution is
		// hard-cancelled at or before the moment it becomes reapable.
		if markErr := s.repo.MarkAsRunningTx(txCtx, tx, workspace, taskID, lease); markErr != nil {
			tracing.MarkSpanError(txCtx, markErr)
			s.logger.WithFields(map[string]interface{}{
				"task_id":      taskID,
				"workspace_id": workspace,
				"error":        markErr.Error(),
			}).Error("Failed to mark task as running")
			return &domain.ErrTaskExecution{
				TaskID: taskID,
				Reason: "failed to mark task as running",
				Err:    markErr,
			}
		}

		// Store timeoutAt for later use in processor
		task.TimeoutAfter = &timeoutAt

		return nil
	})

	if err != nil {
		tracing.MarkSpanError(ctx, err)
		return err
	}

	// For non-parallel tasks, use the standard execution flow
	// Set up completion channel and context handling
	done := make(chan bool, 1)
	processErr := make(chan error, 1)
	bgCtx := context.Background()

	// Process the task in a goroutine
	processorDone := make(chan struct{})
	go func() {
		defer close(processorDone)
		// A panicking processor must fail its task, not kill the server: this
		// goroutine has no other recover, so without this a panic in any
		// processor (broadcast send, import, sync, ...) crashes the whole
		// process and drops every in-flight task and request. The panic is
		// converted to the normal failed-task path with its stack preserved.
		defer func() {
			if r := recover(); r != nil {
				processErr <- &domain.ErrTaskExecution{
					TaskID: taskID,
					Reason: "processor panicked",
					Err:    fmt.Errorf("%v\n%s", r, debug.Stack()),
				}
			}
		}()

		procCtx, procSpan := tracing.StartServiceSpan(domain.WithTaskLease(ctx, lease), "TaskService", "ProcessTask")
		defer tracing.EndSpan(procSpan, nil)

		tracing.AddAttribute(procCtx, "task_id", taskID)
		tracing.AddAttribute(procCtx, "workspace_id", workspace)
		tracing.AddAttribute(procCtx, "task_type", task.Type)
		if task.BroadcastID != nil {
			tracing.AddAttribute(procCtx, "broadcast_id", *task.BroadcastID)
		}

		// Check if context was cancelled before we even start
		if procCtx.Err() != nil {
			tracing.MarkSpanError(procCtx, procCtx.Err())
			processErr <- procCtx.Err()
			return
		}

		// Track task execution time
		startTime := time.Now()

		// Call the processor
		completed, err := processor.Process(procCtx, task, *task.TimeoutAfter)

		// Calculate elapsed time
		elapsed := time.Since(startTime)
		tracing.AddAttribute(procCtx, "elapsed_time_ms", elapsed.Milliseconds())
		tracing.AddAttribute(procCtx, "task_completed", completed)

		var requeue *domain.TaskRequeueError
		if err != nil && (errors.As(err, &requeue) || errors.Is(err, domain.ErrTaskNotRunning)) {
			// Not a failure: handed back below without consuming a retry, or no longer ours.
			processErr <- err
			return
		}
		if err != nil {
			tracing.MarkSpanError(procCtx, err)
			s.logger.WithFields(map[string]interface{}{
				"task_id":      taskID,
				"workspace_id": workspace,
				"elapsed_time": elapsed,
				"error":        err.Error(),
			}).Error("Task processing failed")
			processErr <- &domain.ErrTaskExecution{
				TaskID: taskID,
				Reason: "processing failed",
				Err:    err,
			}
			return
		}

		done <- completed
	}()

	// Wait for completion, error, or timeout
	select {
	case <-ctx.Done():
		// Two very different situations arrive here.
		//
		// context.Canceled means the parent went away — the server is shutting
		// down, or the dispatcher hung up mid-run. That is not the task's fault
		// and must not consume its retry budget: three deploys during a long
		// broadcast used to be enough to kill it. Re-queue it instead.
		//
		// Only Canceled counts as an interruption. The data-feed fetchers build
		// their own WithTimeout sub-contexts, so a feed timing out on its own
		// surfaces as DeadlineExceeded and must keep counting as a real failure
		// — as does our own timeout+grace backstop firing on a stuck processor,
		// handled below.
		if errors.Is(ctx.Err(), context.Canceled) {
			// Let the processor record where it got to while the lease is still ours. What it
			// returns in this wait is not used: the task is released as pending all the same,
			// and a broadcast run that finds its broadcast already processed simply ends.
			wait := interruptedProcessorWait
			if s.interruptedWait > 0 {
				wait = s.interruptedWait
			}
			waitTimer := time.NewTimer(wait)
			select {
			case <-processorDone:
			case <-waitTimer.C:
			}
			waitTimer.Stop()

			// The processor goroutine may still be unwinding. Leave a gap before the
			// task becomes re-dispatchable (GetNextBatch takes any pending task
			// whose next_run_after has elapsed) so a second execution cannot
			// start alongside the first.
			interruptedErr := &domain.ErrTaskExecution{
				TaskID: taskID,
				Reason: "execution interrupted",
				Err:    ctx.Err(),
			}
			if relErr := s.repo.ReleaseTask(bgCtx, workspace, taskID, interruptedErr.Error(),
				time.Now().UTC().Add(taskExecutionGrace)); relErr != nil {
				if errors.Is(relErr, domain.ErrTaskNotRunning) {
					// Something else already moved the task on while we were
					// being cancelled — a cancelled broadcast, or a wake or a
					// pause once its lease expired. Their decision wins.
					s.logger.WithFields(map[string]interface{}{
						"task_id":      taskID,
						"workspace_id": workspace,
					}).Info("Interrupted task was already claimed or finalized elsewhere - leaving it alone")
					return interruptedErr
				}
				s.logger.WithFields(map[string]interface{}{
					"task_id":      taskID,
					"workspace_id": workspace,
					"error":        relErr.Error(),
				}).Error("Failed to release interrupted task")
				return fmt.Errorf("failed to release interrupted task: %w", relErr)
			}
			s.logger.WithFields(map[string]interface{}{
				"task_id":      taskID,
				"workspace_id": workspace,
			}).Warn("Task execution was interrupted — re-queued without consuming a retry")
			return interruptedErr
		}

		// The grace deadline (or an earlier caller deadline) fired while the
		// processor was still running — a genuinely stuck execution. Release
		// the task with a detached write so the scheduler can re-dispatch it;
		// without this case a processor blocked in a call that ignores context
		// would hold this function (and, from the scheduler path, the whole
		// batch) forever. The processor goroutine keeps unwinding on its own:
		// its context-aware calls are cancelled by this same context, and its
		// eventual channel send lands in a buffered channel nobody reads.
		timeoutErr := &domain.ErrTaskExecution{
			TaskID: taskID,
			Reason: "execution exceeded timeout and grace",
			Err:    ctx.Err(),
		}
		if markErr := s.repo.MarkAsFailed(bgCtx, workspace, taskID, timeoutErr.Error()); markErr != nil {
			s.logger.WithFields(map[string]interface{}{
				"task_id":      taskID,
				"workspace_id": workspace,
				"error":        markErr.Error(),
			}).Error("Failed to mark timed-out task as failed")
			return fmt.Errorf("failed to mark timed-out task as failed: %w", markErr)
		}
		s.logger.WithFields(map[string]interface{}{
			"task_id":      taskID,
			"workspace_id": workspace,
		}).Error("Task execution timed out past its grace window")
		return timeoutErr
	case completed := <-done:
		if completed {
			// Check if this is a recurring task
			if task.IsRecurring() {
				// Recurring task: reschedule instead of completing
				rescheduleCtx, rescheduleSpan := tracing.StartServiceSpan(ctx, "TaskService", "RescheduleRecurringTask")
				defer tracing.EndSpan(rescheduleSpan, nil)

				tracing.AddAttribute(rescheduleCtx, "task_id", taskID)
				tracing.AddAttribute(rescheduleCtx, "workspace_id", workspace)
				tracing.AddAttribute(rescheduleCtx, "recurring_interval", *task.RecurringInterval)

				// Calculate next run with backoff and jitter. The backoff is applied
				// to a LOCAL copy on purpose: writing it back to
				// task.RecurringInterval would make a temporary slowdown permanent,
				// since nothing ever restores the original cadence once the
				// integration recovers. The stored interval stays the configured one
				// and the penalty lives in the task state instead.
				interval := *task.RecurringInterval
				if task.State != nil && task.State.IntegrationSync != nil {
					if task.State.IntegrationSync.ConsecErrors > 0 {
						// Quadratic backoff: errors^2 * 10 seconds, capped at 1 hour
						backoff := int64(task.State.IntegrationSync.ConsecErrors * task.State.IntegrationSync.ConsecErrors * 10)
						if backoff > 3600 {
							backoff = 3600
						}
						interval += backoff
						tracing.AddAttribute(rescheduleCtx, "backoff_seconds", backoff)
					}
				}

				// Add jitter (10% of interval) to prevent thundering herd
				jitter := time.Duration(rand.Int63n(interval/10+1)) * time.Second
				nextRun := time.Now().UTC().Add(time.Duration(interval)*time.Second + jitter)

				tracing.AddAttribute(rescheduleCtx, "next_run", nextRun.Format(time.RFC3339))

				if err := s.repo.MarkAsPending(bgCtx, workspace, taskID, nextRun, 0, task.State); err != nil {
					if errors.Is(err, domain.ErrTaskNotFound) {
						// Task was deleted during execution, this is fine
						s.logger.WithFields(map[string]interface{}{
							"task_id":      taskID,
							"workspace_id": workspace,
						}).Info("Recurring task deleted during execution")
						return nil
					}
					tracing.MarkSpanError(rescheduleCtx, err)
					s.logger.WithFields(map[string]interface{}{
						"task_id":      taskID,
						"workspace_id": workspace,
						"error":        err.Error(),
					}).Error("Failed to reschedule recurring task")
					return &domain.ErrTaskExecution{
						TaskID: taskID,
						Reason: "failed to reschedule recurring task",
						Err:    err,
					}
				}
				s.logger.WithFields(map[string]interface{}{
					"task_id":      taskID,
					"workspace_id": workspace,
					"next_run":     nextRun,
					"interval":     interval,
				}).Info("Recurring task rescheduled")
			} else {
				// Non-recurring task: mark as completed
				completeCtx, completeSpan := tracing.StartServiceSpan(ctx, "TaskService", "MarkTaskCompleted")
				defer tracing.EndSpan(completeSpan, nil)

				tracing.AddAttribute(completeCtx, "task_id", taskID)
				tracing.AddAttribute(completeCtx, "workspace_id", workspace)

				if err := s.repo.MarkAsCompleted(bgCtx, workspace, taskID, task.State); err != nil {
					tracing.MarkSpanError(completeCtx, err)
					s.logger.WithFields(map[string]interface{}{
						"task_id":      taskID,
						"workspace_id": workspace,
						"error":        err.Error(),
					}).Error("Failed to mark task as completed")
					return &domain.ErrTaskExecution{
						TaskID: taskID,
						Reason: "failed to mark task as completed",
						Err:    err,
					}
				}
				s.logger.WithFields(map[string]interface{}{
					"task_id":      taskID,
					"workspace_id": workspace,
				}).Info("Task completed successfully")
			}
		} else {
			// Mark task as pending for next run
			pendingCtx, pendingSpan := tracing.StartServiceSpan(ctx, "TaskService", "MarkTaskPending")
			defer tracing.EndSpan(pendingSpan, nil)

			tracing.AddAttribute(pendingCtx, "task_id", taskID)
			tracing.AddAttribute(pendingCtx, "workspace_id", workspace)

			nextRun := time.Now().UTC()
			tracing.AddAttribute(pendingCtx, "next_run", nextRun.Format(time.RFC3339))
			tracing.AddAttribute(pendingCtx, "progress", task.Progress)

			if err := s.repo.MarkAsPending(bgCtx, task.WorkspaceID, task.ID, nextRun, task.Progress, task.State); err != nil {
				tracing.MarkSpanError(pendingCtx, err)
				s.logger.WithFields(map[string]interface{}{
					"task_id":      taskID,
					"workspace_id": workspace,
					"error":        err.Error(),
				}).Error("Failed to mark task as pending")
				return &domain.ErrTaskExecution{
					TaskID: taskID,
					Reason: "failed to mark task as pending",
					Err:    err,
				}
			}
			s.logger.WithFields(map[string]interface{}{
				"task_id":      taskID,
				"workspace_id": workspace,
				"next_run":     nextRun,
			}).Info("Task pending and will continue in next run")
		}
	case err := <-processErr:
		var requeue *domain.TaskRequeueError
		if errors.As(err, &requeue) {
			return s.requeueTask(bgCtx, task, requeue)
		}
		// The processor found the task no longer its own (TaskRepository.SaveState): another
		// execution holds it, or a cancel marked it failed. Every write from here would
		// land on a row this run no longer owns.
		if errors.Is(err, domain.ErrTaskNotRunning) {
			s.logger.WithFields(map[string]interface{}{
				"task_id":      taskID,
				"workspace_id": workspace,
			}).Info("Task is no longer this run's - this run stops without writing")
			return nil
		}

		// An error that carries a cancellation is an interruption, not a
		// failure: the processor was cut short by its context going away rather
		// than by anything wrong with the task. Re-queue it immediately —
		// unlike the ctx.Done() branch above, the processor goroutine has
		// already returned, so there is no overlap window to avoid.
		// The error is not always able to say it was cancelled: a cancellation
		// that lands inside a transaction surfaces as sql.ErrTxDone, whose chain
		// carries no context error. So also ask the context itself — but only
		// about cancellation, since our own deadline firing (DeadlineExceeded)
		// is a genuinely stuck processor and must still count as a failure.
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			if relErr := s.repo.ReleaseTask(bgCtx, workspace, taskID, err.Error(), time.Now().UTC()); relErr != nil {
				if errors.Is(relErr, domain.ErrTaskNotRunning) {
					s.logger.WithFields(map[string]interface{}{
						"task_id":      taskID,
						"workspace_id": workspace,
					}).Info("Interrupted task was already claimed or finalized elsewhere - leaving it alone")
					return err
				}
				s.logger.WithFields(map[string]interface{}{
					"task_id":      taskID,
					"workspace_id": workspace,
					"error":        relErr.Error(),
				}).Error("Failed to release interrupted task")
				return fmt.Errorf("failed to release interrupted task: %w", relErr)
			}
			s.logger.WithFields(map[string]interface{}{
				"task_id":      taskID,
				"workspace_id": workspace,
				"error":        err.Error(),
			}).Warn("Task execution was interrupted — re-queued without consuming a retry")
			return err
		}

		// Task failed with an error
		failCtx, failSpan := tracing.StartServiceSpan(ctx, "TaskService", "MarkTaskFailed")
		defer tracing.EndSpan(failSpan, nil)

		tracing.AddAttribute(failCtx, "task_id", taskID)
		tracing.AddAttribute(failCtx, "workspace_id", workspace)
		tracing.AddAttribute(failCtx, "error", err.Error())

		if markErr := s.repo.MarkAsFailed(bgCtx, workspace, taskID, err.Error()); markErr != nil {
			tracing.MarkSpanError(failCtx, markErr)
			s.logger.WithFields(map[string]interface{}{
				"task_id":      taskID,
				"workspace_id": workspace,
				"error":        markErr.Error(),
				"process_err":  err.Error(),
			}).Error("Failed to mark task as failed")
			return fmt.Errorf("failed to mark task as failed: %w", markErr)
		}
		return err
	}

	return nil
}

// GetLastCronRun retrieves the last cron execution timestamp
func (s *TaskService) GetLastCronRun(ctx context.Context) (*time.Time, error) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "GetLastCronRun")
	defer tracing.EndSpan(span, nil)

	lastRun, err := s.settingRepo.GetLastCronRun(ctx)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithField("error", err.Error()).Error("Failed to get last cron run")
		return nil, err
	}

	if lastRun != nil {
		tracing.AddAttribute(ctx, "last_run", lastRun.Format(time.RFC3339))
	}

	return lastRun, nil
}

// SubscribeToBroadcastEvents registers handlers for broadcast-related events
func (s *TaskService) SubscribeToBroadcastEvents(eventBus domain.EventBus) {
	// Subscribe to broadcast events
	eventBus.Subscribe(domain.EventBroadcastPaused, s.handleBroadcastPaused)
	eventBus.Subscribe(domain.EventBroadcastResumed, s.handleBroadcastResumed)
	eventBus.Subscribe(domain.EventBroadcastSent, s.handleBroadcastSent)
	eventBus.Subscribe(domain.EventBroadcastFailed, s.handleBroadcastFailed)
	eventBus.Subscribe(domain.EventBroadcastCancelled, s.handleBroadcastCancelled)

	s.logger.Info("TaskService subscribed to broadcast events")
}

// sendNowTaskDelay is how far ahead a send-now task is due when it is created. The task
// commits before the broadcast does, so the scheduler and the cron wait this long before they
// may pick it, which the broadcast's commit takes far less than; a run that finds the
// broadcast still a draft hands the task back anyway. Where tasks run in this process,
// RunTaskNow starts it once the broadcast has committed; elsewhere (the cron, or a scheduler
// that dispatches over HTTP) the next pass after this delay does. It is the default; an
// instance can shorten it with TaskSchedulerConfig.SendNowDelay (the integration harness does).
const sendNowTaskDelay = 5 * time.Second

// ScheduleBroadcastTask creates or resets the task of a broadcast being scheduled; see
// domain.TaskService. It runs in the request, before the broadcast's own write commits, so a
// task that cannot be written fails the request instead of leaving a scheduled broadcast
// that nothing sends.
func (s *TaskService) ScheduleBroadcastTask(ctx context.Context, workspaceID, broadcastID string, sendNow bool, scheduledAt *time.Time) (string, error) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "ScheduleBroadcastTask")
	defer tracing.EndSpan(span, nil)
	tracing.AddAttribute(ctx, "workspace_id", workspaceID)
	tracing.AddAttribute(ctx, "broadcast_id", broadcastID)
	tracing.AddAttribute(ctx, "send_now", sendNow)

	s.lock.RLock()
	delay := s.sendNowDelay
	s.lock.RUnlock()
	nextRunAfter := time.Now().UTC().Add(delay)
	if !sendNow && scheduledAt != nil {
		nextRunAfter = scheduledAt.UTC()
	}
	state := &domain.TaskState{
		Progress: 0,
		Message:  "Starting broadcast",
		SendBroadcast: &domain.SendBroadcastState{
			BroadcastID: broadcastID,
			ChannelType: "email",
		},
	}

	var taskID string
	err := s.repo.WithTransaction(ctx, func(tx *sql.Tx) error {
		existing, err := s.repo.GetTaskByBroadcastIDTx(ctx, tx, workspaceID, broadcastID)
		switch {
		case err == nil:
			// Only a draft can be scheduled, and ScheduleBroadcast reads it locked, so an
			// existing task is left by a schedule whose broadcast write rolled back: it
			// starts over.
			taskID = existing.ID
			return s.repo.ResetBroadcastTaskTx(ctx, tx, workspaceID, existing.ID, state, nextRunAfter)
		case errors.Is(err, domain.ErrTaskNotFound):
			broadcastIDCopy := broadcastID
			task := &domain.Task{
				WorkspaceID:   workspaceID,
				Type:          "send_broadcast",
				Status:        domain.TaskStatusPending,
				BroadcastID:   &broadcastIDCopy,
				State:         state,
				MaxRuntime:    50,
				MaxRetries:    3,
				RetryInterval: 300,
				NextRunAfter:  &nextRunAfter,
			}
			if err := s.repo.CreateTx(ctx, tx, workspaceID, task); err != nil {
				return err
			}
			taskID = task.ID
			return nil
		default:
			return err
		}
	})
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"workspace_id": workspaceID,
			"error":        err.Error(),
		}).Error("Failed to schedule the broadcast's task")
		return "", fmt.Errorf("failed to schedule the broadcast's task: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id": broadcastID,
		"workspace_id": workspaceID,
		"task_id":      taskID,
	}).Info("Scheduled the broadcast's task")
	return taskID, nil
}

// RunTaskNow starts an execution of the task in the background; see domain.TaskService.
func (s *TaskService) RunTaskNow(workspaceID, taskID string) {
	if !s.autoExecuteImmediate {
		return
	}
	ctx, done, ok := s.BeginRun(context.Background())
	if !ok {
		// Stopping: the task stays due, and the scheduler or the cron runs it after the restart.
		s.logger.WithFields(map[string]interface{}{
			"workspace_id": workspaceID,
			"task_id":      taskID,
		}).Info("Not running the task now: the server is stopping")
		return
	}
	go func() {
		defer done()
		s.runTaskNow(ctx, workspaceID, taskID)
	}()
}

// runTaskNow executes the given task, and that task only, for its own runtime. ctx is
// cancelled when the app stops.
func (s *TaskService) runTaskNow(ctx context.Context, workspaceID, taskID string) {
	logger := s.logger.WithFields(map[string]interface{}{
		"workspace_id": workspaceID,
		"task_id":      taskID,
	})
	task, err := s.repo.Get(ctx, workspaceID, taskID)
	if err != nil {
		logExecutionFailure(logger, err, "Failed to read the task to run now")
		return
	}
	timeoutAt := time.Now().Add(time.Duration(task.MaxRuntime) * time.Second)
	if err := s.ExecuteTask(ctx, workspaceID, taskID, timeoutAt); err != nil {
		var running *domain.ErrTaskAlreadyRunning
		if errors.As(err, &running) {
			// The scheduler or the cron took it first.
			logger.Debug("Task to run now is already running")
			return
		}
		logExecutionFailure(logger, err, "Failed to run the task now")
	}
}

func (s *TaskService) handleBroadcastPaused(ctx context.Context, payload domain.EventPayload) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "handleBroadcastPaused")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", payload.WorkspaceID)
	tracing.AddAttribute(ctx, "event_type", payload.Type)

	broadcastID, ok := payload.Data["broadcast_id"].(string)
	if !ok || broadcastID == "" {
		err := fmt.Errorf("missing or invalid broadcast_id")
		tracing.MarkSpanError(ctx, err)
		s.logger.Error("Failed to handle broadcast paused event: missing or invalid broadcast_id")
		return
	}

	tracing.AddAttribute(ctx, "broadcast_id", broadcastID)

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id": broadcastID,
		"workspace_id": payload.WorkspaceID,
	}).Info("Handling broadcast paused event")

	// Find associated task by broadcast ID
	task, err := s.repo.GetTaskByBroadcastID(ctx, payload.WorkspaceID, broadcastID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithField("error", err.Error()).Debug("No task found for paused broadcast")
		return
	}

	tracing.AddAttribute(ctx, "task_id", task.ID)
	tracing.AddAttribute(ctx, "current_status", string(task.Status))

	// Phase-2 pause: task already completed (orchestrator done enqueueing).
	// Leave the task alone — we only want to pause the queue rows, which the
	// service layer handled via PauseBySourceTx before publishing this event.
	if task.Status == domain.TaskStatusCompleted {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
		}).Info("Broadcast paused after task completion - leaving task completed")
		return
	}

	// Pause the task. UTC is mandatory because tasks.next_run_after is
	// TIMESTAMP WITHOUT TIME ZONE — see task_handler.go timeoutAt comment.
	nextRunAfter := time.Now().UTC().Add(24 * time.Hour) // Pause for 24 hours
	tracing.AddAttribute(ctx, "next_run_after", nextRunAfter.Format(time.RFC3339))

	var alreadyRunning *domain.ErrTaskAlreadyRunning
	if err := s.repo.MarkAsPaused(ctx, payload.WorkspaceID, task.ID, nextRunAfter); errors.As(err, &alreadyRunning) {
		// The run that holds the task reads the broadcast's status before each
		// batch: it pauses the rows it enqueued and stops by itself. Taking the
		// task from it let a resume start a second execution alongside it. A
		// run that has completed the task since it was read above leaves it
		// completed, as the check above does.
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
		}).Info("Broadcast paused while its run holds the task or has just completed it - leaving the task to that run")
	} else if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
			"error":        err.Error(),
		}).Error("Failed to pause task for paused broadcast")
	} else {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
		}).Info("Successfully paused task for paused broadcast")
	}
}

func (s *TaskService) handleBroadcastResumed(ctx context.Context, payload domain.EventPayload) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "handleBroadcastResumed")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", payload.WorkspaceID)
	tracing.AddAttribute(ctx, "event_type", payload.Type)

	broadcastID, ok := payload.Data["broadcast_id"].(string)
	if !ok || broadcastID == "" {
		err := fmt.Errorf("missing or invalid broadcast_id")
		tracing.MarkSpanError(ctx, err)
		s.logger.Error("Failed to handle broadcast resumed event: missing or invalid broadcast_id")
		return
	}

	tracing.AddAttribute(ctx, "broadcast_id", broadcastID)

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id": broadcastID,
		"workspace_id": payload.WorkspaceID,
	}).Info("Handling broadcast resumed event")

	// Find associated task by broadcast ID
	task, err := s.repo.GetTaskByBroadcastID(ctx, payload.WorkspaceID, broadcastID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithField("error", err.Error()).Debug("No task found for resumed broadcast")
		return
	}

	tracing.AddAttribute(ctx, "task_id", task.ID)
	tracing.AddAttribute(ctx, "current_status", string(task.Status))

	// Phase-2 resume: task already completed (orchestrator done enqueueing).
	// CRITICAL: do not flip the task back to Pending — the next ExecutePendingTasks
	// tick would re-run the orchestrator and re-enqueue every recipient.
	// The queue-row resume is handled by the service layer before this event fires.
	if task.Status == domain.TaskStatusCompleted {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
		}).Info("Broadcast resumed after task completion - leaving task completed")
		return
	}

	// Resume the task with a fresh retry budget: without it a broadcast that
	// was paused after burning retries on transient failures came back one
	// failure from being marked failed for good — and the console offers no
	// other way to clear it. A task its run still holds is left to that run,
	// which sees the resume on its next refresh, or on the read that follows
	// its park when it found the broadcast paused: handing the task to a second
	// execution enqueued everyone past the cursor it read a second time. The
	// budget is reset before the wake, so that whichever comes first, the
	// resume or that run's failure, the task ends up queued: a run that
	// failed for good after a refused wake left a failed task nothing woke.
	// ResumeBroadcast has already reset it before its commit; this second reset
	// covers a failure of that first one.
	if err := s.repo.ResetRetryBudget(ctx, payload.WorkspaceID, task.ID); err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
			"error":        err.Error(),
		}).Error("Failed to reset the retry budget of the resumed broadcast's task")
	}

	nextRunAfter := time.Now().UTC()
	tracing.AddAttribute(ctx, "next_run_after", nextRunAfter.Format(time.RFC3339))

	woken, err := s.repo.WakeTask(ctx, payload.WorkspaceID, task.ID, nextRunAfter)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
			"error":        err.Error(),
		}).Error("Failed to resume task for resumed broadcast")
	} else if !woken {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
		}).Info("Broadcast resumed while its run holds the task or has just completed it - leaving the task to that run")
	} else {
		tracing.AddAttribute(ctx, "new_status", string(domain.TaskStatusPending))
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
		}).Info("Successfully resumed task for resumed broadcast")

		// Check if broadcast should start immediately
		// The resume has committed (the event follows it): its task, and no other due
		// one, runs now.
		if startNow, _ := payload.Data["start_now"].(bool); startNow {
			s.RunTaskNow(payload.WorkspaceID, task.ID)
		}
	}
}

func (s *TaskService) handleBroadcastSent(ctx context.Context, payload domain.EventPayload) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "handleBroadcastSent")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", payload.WorkspaceID)
	tracing.AddAttribute(ctx, "event_type", payload.Type)

	broadcastID, ok := payload.Data["broadcast_id"].(string)
	if !ok || broadcastID == "" {
		err := fmt.Errorf("missing or invalid broadcast_id")
		tracing.MarkSpanError(ctx, err)
		s.logger.Error("Failed to handle broadcast sent event: missing or invalid broadcast_id")
		return
	}

	tracing.AddAttribute(ctx, "broadcast_id", broadcastID)

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id": broadcastID,
		"workspace_id": payload.WorkspaceID,
	}).Info("Handling broadcast sent event")

	// Find associated task by broadcast ID
	task, err := s.repo.GetTaskByBroadcastID(ctx, payload.WorkspaceID, broadcastID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithField("error", err.Error()).Debug("No task found for sent broadcast")
		return
	}

	tracing.AddAttribute(ctx, "task_id", task.ID)
	tracing.AddAttribute(ctx, "current_status", string(task.Status))

	// Mark the task as completed
	if err := s.repo.MarkAsCompleted(ctx, payload.WorkspaceID, task.ID, task.State); err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
			"error":        err.Error(),
		}).Error("Failed to complete task for sent broadcast")
	} else {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
		}).Info("Successfully completed task for sent broadcast")
	}
}

func (s *TaskService) handleBroadcastFailed(ctx context.Context, payload domain.EventPayload) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "handleBroadcastFailed")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", payload.WorkspaceID)
	tracing.AddAttribute(ctx, "event_type", payload.Type)

	broadcastID, ok := payload.Data["broadcast_id"].(string)
	if !ok || broadcastID == "" {
		err := fmt.Errorf("missing or invalid broadcast_id")
		tracing.MarkSpanError(ctx, err)
		s.logger.Error("Failed to handle broadcast failed event: missing or invalid broadcast_id")
		return
	}

	tracing.AddAttribute(ctx, "broadcast_id", broadcastID)

	reason, _ := payload.Data["reason"].(string)
	if reason == "" {
		reason = "Broadcast failed"
	}

	tracing.AddAttribute(ctx, "failure_reason", reason)

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id": broadcastID,
		"workspace_id": payload.WorkspaceID,
		"reason":       reason,
	}).Info("Handling broadcast failed event")

	// Find associated task by broadcast ID
	task, err := s.repo.GetTaskByBroadcastID(ctx, payload.WorkspaceID, broadcastID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithField("error", err.Error()).Debug("No task found for failed broadcast")
		return
	}

	tracing.AddAttribute(ctx, "task_id", task.ID)
	tracing.AddAttribute(ctx, "current_status", string(task.Status))

	// Mark the task as failed
	if err := s.repo.MarkAsFailed(ctx, payload.WorkspaceID, task.ID, reason); err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
			"error":        err.Error(),
		}).Error("Failed to mark task as failed for failed broadcast")
	} else {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
		}).Info("Successfully marked task as failed for failed broadcast")
	}
}

func (s *TaskService) handleBroadcastCancelled(ctx context.Context, payload domain.EventPayload) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "handleBroadcastCancelled")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", payload.WorkspaceID)
	tracing.AddAttribute(ctx, "event_type", payload.Type)

	broadcastID, ok := payload.Data["broadcast_id"].(string)
	if !ok || broadcastID == "" {
		err := fmt.Errorf("missing or invalid broadcast_id")
		tracing.MarkSpanError(ctx, err)
		s.logger.Error("Failed to handle broadcast cancelled event: missing or invalid broadcast_id")
		return
	}

	tracing.AddAttribute(ctx, "broadcast_id", broadcastID)

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id": broadcastID,
		"workspace_id": payload.WorkspaceID,
	}).Info("Handling broadcast cancelled event")

	// Find associated task by broadcast ID
	task, err := s.repo.GetTaskByBroadcastID(ctx, payload.WorkspaceID, broadcastID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithField("error", err.Error()).Debug("No task found for cancelled broadcast")
		return
	}

	tracing.AddAttribute(ctx, "task_id", task.ID)
	tracing.AddAttribute(ctx, "current_status", string(task.Status))

	// Phase-2 cancel: the enqueue task genuinely succeeded; don't overwrite its
	// status to Failed. Queue-row deletion is handled by the service layer.
	if task.Status == domain.TaskStatusCompleted {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
		}).Info("Broadcast cancelled after task completion - leaving task completed")
		return
	}

	// Mark the task as failed with cancellation reason
	cancelReason := "Broadcast was cancelled"
	tracing.AddAttribute(ctx, "cancel_reason", cancelReason)

	if err := s.repo.MarkAsFailed(ctx, payload.WorkspaceID, task.ID, cancelReason); err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
			"error":        err.Error(),
		}).Error("Failed to mark task as failed for cancelled broadcast")
	} else {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
		}).Info("Successfully marked task as failed for cancelled broadcast")
	}
}

// ResetTask resets a failed recurring task, clearing error state and rescheduling for immediate execution
func (s *TaskService) ResetTask(ctx context.Context, workspace, taskID string) error {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "ResetTask")
	defer func() { tracing.EndSpan(span, nil) }()

	tracing.AddAttribute(ctx, "task_id", taskID)
	tracing.AddAttribute(ctx, "workspace_id", workspace)

	// Get the task. Only the repository's not-found answer is a missing task: a failed
	// read is a failure, which the API answers as one rather than as a 404.
	task, err := s.repo.Get(ctx, workspace, taskID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		if errors.Is(err, domain.ErrTaskNotFound) {
			return domain.ErrTaskNotFound
		}
		return fmt.Errorf("failed to load task: %w", err)
	}

	// Verify task is in failed state
	if task.Status != domain.TaskStatusFailed {
		return domain.NewValidationError(fmt.Sprintf("task is not in failed state, current status: %s", task.Status))
	}

	// Verify task is recurring
	if !task.IsRecurring() {
		return domain.NewValidationError("task is not a recurring task")
	}

	// Reset error state in IntegrationSync
	if task.State != nil && task.State.IntegrationSync != nil {
		task.State.IntegrationSync.ConsecErrors = 0
		task.State.IntegrationSync.LastError = nil
		task.State.IntegrationSync.LastErrorType = ""
	}

	// Schedule for immediate execution
	nextRun := time.Now().UTC()

	if err := s.repo.MarkAsPending(ctx, workspace, taskID, nextRun, 0, task.State); err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"task_id":      taskID,
			"workspace_id": workspace,
			"error":        err.Error(),
		}).Error("Failed to reset task")
		return fmt.Errorf("failed to reset task: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": workspace,
	}).Info("Task reset successfully")

	return nil
}

// TriggerTask triggers an immediate execution of a recurring task
func (s *TaskService) TriggerTask(ctx context.Context, workspace, taskID string) error {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "TriggerTask")
	defer func() { tracing.EndSpan(span, nil) }()

	tracing.AddAttribute(ctx, "task_id", taskID)
	tracing.AddAttribute(ctx, "workspace_id", workspace)

	// Get the task. Only the repository's not-found answer is a missing task: a failed
	// read is a failure, which the API answers as one rather than as a 404.
	task, err := s.repo.Get(ctx, workspace, taskID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		if errors.Is(err, domain.ErrTaskNotFound) {
			return domain.ErrTaskNotFound
		}
		return fmt.Errorf("failed to load task: %w", err)
	}

	// Verify task is recurring
	if !task.IsRecurring() {
		return domain.NewValidationError("task is not a recurring task")
	}

	// Check if task is already running
	if task.Status == domain.TaskStatusRunning {
		return &domain.ErrTaskAlreadyRunning{TaskID: taskID}
	}

	// Check if task is in a state that can be triggered
	if task.Status != domain.TaskStatusPending && task.Status != domain.TaskStatusPaused {
		return domain.NewValidationError(fmt.Sprintf("task cannot be triggered in current status: %s", task.Status))
	}

	// Schedule for immediate execution. The wake re-checks the status where it
	// writes, so a run that took the task since it was read keeps it, and the
	// state that run saves is not overwritten with the one read above.
	nextRun := time.Now().UTC()

	woken, err := s.repo.WakeTask(ctx, workspace, taskID, nextRun)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"task_id":      taskID,
			"workspace_id": workspace,
			"error":        err.Error(),
		}).Error("Failed to trigger task")
		return fmt.Errorf("failed to trigger task: %w", err)
	}
	if !woken {
		return &domain.ErrTaskAlreadyRunning{TaskID: taskID}
	}

	s.logger.WithFields(map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": workspace,
	}).Info("Task triggered for immediate execution")

	return nil
}
