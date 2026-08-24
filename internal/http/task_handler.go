package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// maxExecuteBodyBytes bounds the dispatch body read. The request carries a
// workspace id and a task id; anything larger is not a dispatch, and the read
// has to be bounded because it happens before the signature is verified.
const maxExecuteBodyBytes = 64 << 10

// taskTypeResources maps every task type this deployment runs to the resource
// that owns it, in a stable order (tasks.list walks it).
//
// Tasks have no permission resource of their own: authorizing a task means
// authorizing the thing the task acts on, so triggering a send_broadcast task
// needs exactly what sending the broadcast needs. A type absent from this table
// has no owner and therefore fails closed — a new task type is unreachable over
// HTTP until it is mapped, rather than open to every member.
var taskTypeResources = []struct {
	taskType string
	resource domain.PermissionResource
}{
	{"send_broadcast", domain.PermissionResourceBroadcasts},
	{"build_segment", domain.PermissionResourceSegments},
	{"check_segment_recompute", domain.PermissionResourceSegments},
	{"process_contact_segment_queue", domain.PermissionResourceSegments},
	{"sync_integration", domain.PermissionResourceWorkspace},
	{domain.WebAnalyticsBackfillTaskType, domain.PermissionResourceWebAnalytics},
}

// taskTypeResource returns the resource that owns a task type, and whether the
// type is mapped at all.
func taskTypeResource(taskType string) (domain.PermissionResource, bool) {
	for _, mapping := range taskTypeResources {
		if mapping.taskType == taskType {
			return mapping.resource, true
		}
	}
	return "", false
}

// authorizeTaskType checks the permission that owns a task's type.
//
// An unmapped type is denied for everyone, owners included: there is no grant
// that could describe it, so consulting HasPermission would hand it to the role
// that short-circuits to true. The returned error carries no resource, because
// naming one would tell the caller to go and ask for a grant that changes
// nothing.
func authorizeTaskType(userWorkspace *domain.UserWorkspace, taskType string, permission domain.PermissionType) error {
	resource, ok := taskTypeResource(taskType)
	if !ok {
		return domain.NewPermissionError("", permission,
			fmt.Sprintf("Task type %q is not authorizable", taskType))
	}

	if !userWorkspace.HasPermission(resource, permission) {
		return domain.NewPermissionError(resource, permission,
			fmt.Sprintf("You do not have %s permission on %s, which owns tasks of type %q", permission, resource, taskType))
	}
	return nil
}

// readableTaskTypes returns the task types the caller may read, in the order of
// taskTypeResources so the resulting filter is deterministic.
func readableTaskTypes(userWorkspace *domain.UserWorkspace) []string {
	types := make([]string, 0, len(taskTypeResources))
	for _, mapping := range taskTypeResources {
		if userWorkspace.HasPermission(mapping.resource, domain.PermissionTypeRead) {
			types = append(types, mapping.taskType)
		}
	}
	return types
}

// TaskHandler handles HTTP requests related to tasks
type TaskHandler struct {
	taskService domain.TaskService
	// authService authorizes the five authenticated task routes here rather than
	// in TaskService. The service is also driven by the scheduler, by segment
	// recompute and by scheduled broadcasts, all on contexts that carry no user:
	// a gate down there would break task execution instead of protecting it.
	authService  domain.AuthService
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
	secretKey    string
	// httpDispatchEnabled mirrors "the internal scheduler is off". Only then does
	// TaskService dispatch executions over HTTP, and only then does
	// /api/tasks.execute do anything — a default install runs its scheduler
	// in-process and answers the route with 404. The route itself is always
	// registered; see RegisterRoutes for why.
	httpDispatchEnabled bool
	// nowFn is the clock the dispatch signature's skew is measured against.
	nowFn func() time.Time
	// cronRunning serialises /api/cron so a public, unauthenticated endpoint
	// that now answers immediately cannot be used to spawn unbounded
	// background runs.
	cronRunning atomic.Bool
}

// NewTaskHandler creates a new task handler
func NewTaskHandler(
	taskService domain.TaskService,
	authService domain.AuthService,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
	secretKey string,
	httpDispatchEnabled bool,
) *TaskHandler {
	return &TaskHandler{
		taskService:         taskService,
		authService:         authService,
		getJWTSecret:        getJWTSecret,
		logger:              logger,
		secretKey:           secretKey,
		httpDispatchEnabled: httpDispatchEnabled,
		nowFn:               time.Now,
	}
}

// RegisterRoutes registers the task-related routes
func (h *TaskHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// Register RPC-style endpoints with dot notation.
	mux.Handle("/api/tasks.list", requireAuth(http.HandlerFunc(h.ListTasks)))
	mux.Handle("/api/tasks.get", requireAuth(http.HandlerFunc(h.GetTask)))
	mux.Handle("/api/tasks.delete", requireAuth(http.HandlerFunc(h.DeleteTask)))
	mux.Handle("/api/tasks.reset", requireAuth(http.HandlerFunc(h.ResetTask)))
	mux.Handle("/api/tasks.trigger", requireAuth(http.HandlerFunc(h.TriggerTask)))

	// tasks.execute is the scheduler's dispatch target, authenticated by
	// signature rather than by a session. An instance running its own scheduler
	// executes in-process and has no use for it, and ExecuteTask answers 404
	// there — the same thing an attacker sees whether the route is registered or
	// not.
	//
	// Registered unconditionally all the same, because an unrouted /api/* path
	// falls through to the console SPA catch-all (root_handler.go's Handle
	// returns without writing for any /api prefix it does not recognise), which
	// answers 200 with an empty body. A dispatcher pointed at an instance whose
	// scheduler is on would read that as success and drop the execution
	// silently. Owning the path is what turns that into a hard failure.
	mux.Handle("/api/tasks.execute", http.HandlerFunc(h.ExecuteTask))

	// tasks.create is retired: every legitimate task is created by a service that
	// already gates its own resource. Creating one over HTTP meant naming a type
	// and a state, which for send_broadcast is a draft's id and a send.
	//
	// Its path is owned rather than left unrouted, for the reason spelled out
	// above: an unregistered /api/* answers 200 with an empty body, which a caller
	// reads as "task created". 410 says the endpoint existed and is gone.
	mux.Handle("/api/tasks.create", http.HandlerFunc(h.CreateTaskGone))

	// /api/cron and /api/cron.status stay public: external crontabs rely on it.
	mux.Handle("/api/cron", http.HandlerFunc(h.ExecutePendingTasks))
	mux.Handle("/api/cron.status", http.HandlerFunc(h.GetCronStatus))
}

// CreateTaskGone answers the retired /api/tasks.create with 410 Gone, on every
// method: there is no request shape that would make it succeed.
func (h *TaskHandler) CreateTaskGone(w http.ResponseWriter, r *http.Request) {
	WriteJSONError(w, "tasks.create has been removed: a task is created by the service that owns the resource it acts on", http.StatusGone)
}

// GetTask handles retrieval of a task by ID
func (h *TaskHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var getRequest domain.GetTaskRequest
	if err := getRequest.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, userWorkspace, ok := h.authorizeWorkspace(w, r, getRequest.WorkspaceID, "Failed to get task")
	if !ok {
		return
	}

	task, err := h.taskService.GetTask(ctx, getRequest.WorkspaceID, getRequest.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			WriteJSONError(w, "Task not found", http.StatusNotFound)
		} else {
			h.logger.WithField("error", err.Error()).Error("Failed to get task")
			WriteJSONError(w, "Failed to get task", http.StatusInternalServerError)
		}
		return
	}

	if err := authorizeTaskType(userWorkspace, task.Type, domain.PermissionTypeRead); err != nil {
		writePermissionError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"task": task,
	})
}

// authorizeWorkspace authenticates the caller for the workspace named in the
// request, writing the response and reporting false when it did.
//
// This is the cross-tenant gate: the workspace comes from the request body or
// query, so without it any valid token reached any workspace's tasks by naming
// it. It returns the authenticated context, which carries the caller down into
// the service.
func (h *TaskHandler) authorizeWorkspace(w http.ResponseWriter, r *http.Request, workspaceID, fallback string) (context.Context, *domain.UserWorkspace, bool) {
	ctx, _, userWorkspace, err := h.authService.AuthenticateUserForWorkspace(r.Context(), workspaceID)
	if err != nil {
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return nil, nil, false
		}
		h.logger.WithFields(map[string]interface{}{
			"workspace_id": workspaceID,
			"error":        err.Error(),
		}).Error("Failed to authenticate task request")
		WriteJSONError(w, fallback, http.StatusInternalServerError)
		return nil, nil, false
	}
	return ctx, userWorkspace, true
}

// authorizeExistingTask checks write on the resource that owns the task being
// mutated, writing the response and reporting false when it did.
//
// The task has to be read first because the permission depends on its type, and
// the type is only in the row: delete, reset and trigger all name a task by id.
// The read is safe to do before the type check — the caller is already a member
// of the workspace by the time this runs.
func (h *TaskHandler) authorizeExistingTask(w http.ResponseWriter, ctx context.Context, userWorkspace *domain.UserWorkspace, workspaceID, taskID, fallback string) bool {
	task, err := h.taskService.GetTask(ctx, workspaceID, taskID)
	if err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) || strings.Contains(err.Error(), "not found") {
			WriteJSONError(w, "Task not found", http.StatusNotFound)
			return false
		}
		h.logger.WithFields(map[string]interface{}{
			"task_id":      taskID,
			"workspace_id": workspaceID,
			"error":        err.Error(),
		}).Error("Failed to load task for authorization")
		WriteJSONError(w, fallback, http.StatusInternalServerError)
		return false
	}

	if err := authorizeTaskType(userWorkspace, task.Type, domain.PermissionTypeWrite); err != nil {
		writePermissionError(w, err)
		return false
	}
	return true
}

// ListTasks handles listing tasks with optional filtering
func (h *TaskHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var listRequest domain.ListTasksRequest
	if err := listRequest.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	filter := listRequest.ToFilter()

	ctx, userWorkspace, ok := h.authorizeWorkspace(w, r, listRequest.WorkspaceID, "Failed to list tasks")
	if !ok {
		return
	}

	// A listing spans types, so it is narrowed rather than refused: a caller that
	// names types must be able to read every one of them, and a caller that names
	// none gets the types it can read. Narrowing the filter rather than dropping
	// rows afterwards is what keeps total_count honest.
	if len(filter.Type) > 0 {
		for _, taskType := range filter.Type {
			if err := authorizeTaskType(userWorkspace, taskType, domain.PermissionTypeRead); err != nil {
				writePermissionError(w, err)
				return
			}
		}
	} else {
		readable := readableTaskTypes(userWorkspace)
		if len(readable) == 0 {
			// An empty filter means "every type", so this cannot be left to the
			// query: a caller that may read nothing is answered with nothing.
			writeJSON(w, http.StatusOK, &domain.TaskListResponse{
				Tasks:  []*domain.Task{},
				Limit:  filter.Limit,
				Offset: filter.Offset,
			})
			return
		}
		filter.Type = readable
	}

	response, err := h.taskService.ListTasks(ctx, listRequest.WorkspaceID, filter)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list tasks")
		WriteJSONError(w, "Failed to list tasks", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, response)
}

// DeleteTask handles deletion of a task
func (h *TaskHandler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var deleteRequest domain.DeleteTaskRequest
	if err := deleteRequest.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, userWorkspace, ok := h.authorizeWorkspace(w, r, deleteRequest.WorkspaceID, "Failed to delete task")
	if !ok {
		return
	}

	if !h.authorizeExistingTask(w, ctx, userWorkspace, deleteRequest.WorkspaceID, deleteRequest.ID, "Failed to delete task") {
		return
	}

	if err := h.taskService.DeleteTask(ctx, deleteRequest.WorkspaceID, deleteRequest.ID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			WriteJSONError(w, "Task not found", http.StatusNotFound)
		} else {
			h.logger.WithField("error", err.Error()).Error("Failed to delete task")
			WriteJSONError(w, "Failed to delete task", http.StatusInternalServerError)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// ExecutePendingTasks handles the cron-triggered task execution
func (h *TaskHandler) ExecutePendingTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Log that manual trigger is being used (internal scheduler should handle this)
	h.logger.Info("Manual cron trigger via HTTP endpoint - internal scheduler should handle this automatically")

	var executeRequest domain.ExecutePendingTasksRequest
	if err := executeRequest.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Only one cron run at a time. This endpoint is public, and since the
	// in-process execution mode landed it does real work rather than just
	// dispatching, so answering immediately without this guard would let any
	// caller spawn unbounded background runs. It also mirrors TaskScheduler,
	// whose ticker loop already serialises its runs.
	if !h.cronRunning.CompareAndSwap(false, true) {
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"success":         true,
			"message":         "Task execution already in progress",
			"already_running": true,
		})
		return
	}

	// Detach from the request: with in-process execution the tasks run right
	// here, so a caller that gives up (a cron client's timeout, a proxy's
	// read timeout) would otherwise cancel every task it just started, mid
	// database transaction. Answer immediately and let the work finish.
	execCtx := context.WithoutCancel(r.Context())
	maxTasks := executeRequest.MaxTasks

	go func() {
		defer h.cronRunning.Store(false)
		defer func() {
			if rec := recover(); rec != nil {
				h.logger.WithField("panic", fmt.Sprintf("%v", rec)).
					Error("Panic during background cron execution")
			}
		}()

		if err := h.taskService.ExecutePendingTasks(execCtx, maxTasks); err != nil {
			h.logger.WithField("error", err.Error()).Error("Failed to execute tasks")
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"success":   true,
		"message":   "Task execution started",
		"max_tasks": maxTasks,
	})
}

// verifyExecuteSignature authenticates one dispatch of /api/tasks.execute,
// writing the response and reporting false when it did.
//
// The rejection is logged on its own line, separate from every other tasks.execute
// log: the installations that run HTTP dispatch at all are the ones behind
// reverse proxies, and a proxy that strips or rewrites headers looks exactly like
// an attacker here. The line says which of the two headers actually arrived, which
// is the difference between "misconfigured ingress" and "someone is probing this".
func (h *TaskHandler) verifyExecuteSignature(w http.ResponseWriter, r *http.Request, body []byte) bool {
	rawTimestamp := r.Header.Get(domain.TaskExecuteTimestampHeader)
	rawSignature := r.Header.Get(domain.TaskExecuteSignatureHeader)

	reject := func(reason string) bool {
		h.logger.WithFields(map[string]interface{}{
			"reason":        reason,
			"path":          r.URL.Path,
			"has_timestamp": rawTimestamp != "",
			"has_signature": rawSignature != "",
			"remote_addr":   r.RemoteAddr,
		}).Warn("tasks.execute signature rejected")
		WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}

	// An installation with no SECRET_KEY cannot verify anything, so it must
	// refuse rather than derive a key from the empty string and accept whatever
	// matches it. Config makes SECRET_KEY mandatory, so this is defence in depth.
	if h.secretKey == "" {
		return reject("no secret key configured")
	}

	timestamp, err := domain.ParseTaskExecuteTimestamp(rawTimestamp)
	if err != nil {
		return reject("malformed timestamp")
	}

	key := domain.TaskExecuteSigningKey(h.secretKey)
	if err := domain.VerifyTaskExecuteSignature(key, timestamp, r.URL.Path, body, rawSignature, h.nowFn()); err != nil {
		return reject("signature mismatch or stale timestamp")
	}
	return true
}

// ExecuteTask handles execution of a single task
func (h *TaskHandler) ExecuteTask(w http.ResponseWriter, r *http.Request) {
	// An instance that runs its own scheduler executes tasks in-process and this
	// endpoint has no caller, so it answers as if it were not routed at all —
	// before the method check and before the signature check, since neither
	// would exist either. The security posture is the one the route had when it
	// was conditionally registered: an unauthenticated prober gets 404.
	//
	// The rejection has its own log line, distinct from the signature line
	// below, because the two are indistinguishable from the outside: a
	// dispatcher whose proxy strips the signature headers gets 401, and one
	// pointed at a scheduler-enabled instance gets 404. Only the server can tell
	// a misconfigured dispatcher which of the two it is.
	if !h.httpDispatchEnabled {
		h.logger.WithFields(map[string]interface{}{
			"path":        r.URL.Path,
			"remote_addr": r.RemoteAddr,
		}).Warn("tasks.execute rejected: this instance runs its own scheduler and executes tasks in-process")
		WriteJSONError(w, "Not found", http.StatusNotFound)
		return
	}

	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the body before decoding it: the signature covers it, so it has to be
	// hashed exactly as it arrived. Bounded because this read happens before the
	// request is authenticated.
	body, err := io.ReadAll(io.LimitReader(r.Body, maxExecuteBodyBytes))
	if err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if !h.verifyExecuteSignature(w, r, body) {
		return
	}

	var executeRequest domain.ExecuteTaskRequest
	if err := json.Unmarshal(body, &executeRequest); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Entry log so the scheduler-side "Task execution request dispatched
	// successfully" log has a falsifiable counterpart on the handler side.
	// Diff the two streams to verify the handler actually ran for a given
	// dispatch (see #317 — reporter saw dispatch success logs but believed
	// the handler wasn't running). Logged before Validate so even a request
	// that fails validation is visible to debugging.
	h.logger.WithFields(map[string]interface{}{
		"task_id":      executeRequest.ID,
		"workspace_id": executeRequest.WorkspaceID,
	}).Info("tasks.execute received")

	if err := executeRequest.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Detach from the request connection. Task execution is bounded by the
	// task's own timeout+grace deadline, applied in ExecuteTask; tying it to
	// the caller's connection instead meant the dispatcher's HTTP client
	// timeout cancelled a running task mid-batch — aborting the enqueue
	// transaction and stranding the recipients it was writing. The 60s
	// graceful-shutdown window covers an in-flight run.
	execCtx := context.WithoutCancel(r.Context())

	// Get the task to calculate timeout
	task, err := h.taskService.GetTask(execCtx, executeRequest.WorkspaceID, executeRequest.ID)
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	// Calculate timeout based on task's MaxRuntime. UTC is mandatory: the
	// tasks.timeout_after column is TIMESTAMP WITHOUT TIME ZONE, so a local
	// time.Now() on a non-UTC server would write a literal that, when later
	// compared against time.Now().UTC() in GetNextBatch, leaves the task
	// "still running" for the duration of the server's UTC offset (e.g. +2h
	// in CEST) and the recurring task is never re-picked within that window.
	timeoutAt := time.Now().UTC().Add(time.Duration(task.MaxRuntime) * time.Second)

	if err := h.taskService.ExecuteTask(execCtx, executeRequest.WorkspaceID, executeRequest.ID, timeoutAt); err != nil {
		// Handle different error types with appropriate status codes
		switch e := err.(type) {
		case *domain.ErrNotFound:
			WriteJSONError(w, e.Error(), http.StatusNotFound)
		case *domain.ErrTaskAlreadyRunning:
			// Task is already being executed by another worker - this is expected in concurrent scenarios
			h.logger.WithFields(map[string]interface{}{
				"task_id":      executeRequest.ID,
				"workspace_id": executeRequest.WorkspaceID,
			}).Debug("Task already running, rejecting concurrent execution")
			WriteJSONError(w, e.Error(), http.StatusConflict)
		case *domain.ErrTaskExecution:
			// Check if this is an "already running" error - expected in concurrent environments
			if _, ok := e.Err.(*domain.ErrTaskAlreadyRunning); ok {
				h.logger.WithFields(map[string]interface{}{
					"task_id":      executeRequest.ID,
					"workspace_id": executeRequest.WorkspaceID,
				}).Debug("Task already running (wrapped), rejecting concurrent execution")
				WriteJSONError(w, "Task is already being processed", http.StatusConflict)
			} else if e.Reason == "failed to mark task as running" {
				// This happens when another executor already claimed the task
				h.logger.WithFields(map[string]interface{}{
					"task_id":      executeRequest.ID,
					"workspace_id": executeRequest.WorkspaceID,
				}).Debug("Task already claimed by another executor")
				WriteJSONError(w, "Task is already being processed", http.StatusConflict)
			} else if e.Reason == "no processor registered for task type" {
				h.logger.WithFields(map[string]interface{}{
					"task_id":      executeRequest.ID,
					"workspace_id": executeRequest.WorkspaceID,
					"error":        err.Error(),
				}).Warn("No processor registered for task type")
				WriteJSONError(w, "Unsupported task type", http.StatusBadRequest)
			} else {
				// Log genuine errors at ERROR level
				h.logger.WithFields(map[string]interface{}{
					"task_id":      executeRequest.ID,
					"workspace_id": executeRequest.WorkspaceID,
					"reason":       e.Reason,
					"error":        err.Error(),
				}).Error("Task execution failed")
				WriteJSONError(w, "Task execution failed: "+e.Reason, http.StatusInternalServerError)
			}
		case *domain.ErrTaskTimeout:
			WriteJSONError(w, e.Error(), http.StatusGatewayTimeout)
		default:
			h.logger.WithFields(map[string]interface{}{
				"task_id":      executeRequest.ID,
				"workspace_id": executeRequest.WorkspaceID,
				"error":        err.Error(),
			}).Error("Failed to execute task")
			WriteJSONError(w, "Failed to execute task", http.StatusInternalServerError)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Task execution initiated",
	})
}

// GetCronStatus returns the last cron run timestamp from settings
func (h *TaskHandler) GetCronStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	lastRun, err := h.taskService.GetLastCronRun(r.Context())
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get last cron run")
		WriteJSONError(w, "Failed to get cron status", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success": true,
	}

	if lastRun != nil {
		response["last_run"] = lastRun.Format(time.RFC3339)
		response["last_run_unix"] = lastRun.Unix()

		// Calculate time since last run
		timeSince := time.Since(*lastRun)
		response["time_since_last_run"] = timeSince.String()
		response["time_since_last_run_seconds"] = int64(timeSince.Seconds())
	} else {
		response["last_run"] = nil
		response["last_run_unix"] = nil
		response["time_since_last_run"] = nil
		response["time_since_last_run_seconds"] = nil
		response["message"] = "No cron run recorded yet"
	}

	writeJSON(w, http.StatusOK, response)
}

// ResetTask resets a failed recurring task, clearing error state and scheduling for immediate execution
func (h *TaskHandler) ResetTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.ResetTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, userWorkspace, ok := h.authorizeWorkspace(w, r, req.WorkspaceID, "Failed to reset task")
	if !ok {
		return
	}

	if !h.authorizeExistingTask(w, ctx, userWorkspace, req.WorkspaceID, req.ID, "Failed to reset task") {
		return
	}

	if err := h.taskService.ResetTask(ctx, req.WorkspaceID, req.ID); err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			WriteJSONError(w, "Task not found", http.StatusNotFound)
			return
		}
		h.logger.WithFields(map[string]interface{}{
			"task_id":      req.ID,
			"workspace_id": req.WorkspaceID,
			"error":        err.Error(),
		}).Error("Failed to reset task")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

// TriggerTask triggers an immediate execution of a recurring task
func (h *TaskHandler) TriggerTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.TriggerTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, userWorkspace, ok := h.authorizeWorkspace(w, r, req.WorkspaceID, "Failed to trigger task")
	if !ok {
		return
	}

	if !h.authorizeExistingTask(w, ctx, userWorkspace, req.WorkspaceID, req.ID, "Failed to trigger task") {
		return
	}

	if err := h.taskService.TriggerTask(ctx, req.WorkspaceID, req.ID); err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			WriteJSONError(w, "Task not found", http.StatusNotFound)
			return
		}
		var alreadyRunningErr *domain.ErrTaskAlreadyRunning
		if errors.As(err, &alreadyRunningErr) {
			WriteJSONError(w, "Task is already running", http.StatusConflict)
			return
		}
		h.logger.WithFields(map[string]interface{}{
			"task_id":      req.ID,
			"workspace_id": req.WorkspaceID,
			"error":        err.Error(),
		}).Error("Failed to trigger task")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}
