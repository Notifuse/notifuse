package mcp

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

const listAutomationsName = "list_automations"

// maxJourneySteps bounds the steps get_automation_journey returns: the most
// recent ones are kept. A journey's steps are bounded by its graph, except
// that a loop or a retry adds one each time round.
const maxJourneySteps = 200

var automationsRead = Pair{Resource: domain.PermissionResourceAutomations, Verb: domain.PermissionTypeRead}

var automationStatuses = []string{
	string(domain.AutomationStatusDraft),
	string(domain.AutomationStatusLive),
	string(domain.AutomationStatusPaused),
}

// listAutomationsTool pages through a workspace's automations.
// automations.list pages by offset on created_at alone, so the tool reads
// them all and pages them by offset on (created_at, id).
func (s *Server) listAutomationsTool() *Tool {
	return &Tool{
		Name:  listAutomationsName,
		Title: "List automations",
		Description: "Lists the automations of a workspace, newest first. " +
			"Each has its name, status (draft, live or paused), list, trigger (the event that starts a journey, with its list, segment or custom event), number of steps, whether a step sends email, stored counters, and journeys by status. " +
			"get_automation has the steps and the trigger conditions. total counts every automation that matches the filters. " +
			"To get the next page, pass next_cursor back as cursor with the same filters. An automation created between two pages can show up twice.",
		Class: ClassRead,
		Pairs: []Pair{automationsRead},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"status": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string", "enum": automationStatuses},
					"description": "Only automations in one of these statuses. Default: all.",
				},
				"list_id": map[string]any{"type": "string", "description": "Only automations attached to this list."},
				"limit":   map[string]any{"type": "integer", "description": "1 to 50, default 20."},
				"cursor":  map[string]any{"type": "string", "description": "next_cursor of the previous page, with the same filters."},
			},
			"required":             []string{"workspace_id"},
			"additionalProperties": false,
		},
		Handler: s.listAutomations,
	}
}

type listAutomationsArgs struct {
	Status []string `json:"status"`
	ListID string   `json:"list_id"`
	Limit  *int     `json:"limit"`
	Cursor string   `json:"cursor"`
}

// statuses deduplicates the status argument and puts it in the tool's order,
// so a cursor matches whatever order the statuses were given in.
func (a *listAutomationsArgs) statuses() []string {
	var asked []string
	for _, status := range automationStatuses {
		if slices.Contains(a.Status, status) {
			asked = append(asked, status)
		}
	}
	return asked
}

// automationTriggerSummary is the trigger as list_automations shows it: what
// starts a journey, without the conditions and the click filters.
type automationTriggerSummary struct {
	EventKind       string  `json:"event_kind"`
	ListID          *string `json:"list_id"`
	SegmentID       *string `json:"segment_id"`
	CustomEventName *string `json:"custom_event_name"`
	Frequency       string  `json:"frequency"`
}

type automationSummary struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Status      string                    `json:"status"`
	ListID      string                    `json:"list_id"`
	ExitOnReply bool                      `json:"exit_on_reply"`
	Trigger     *automationTriggerSummary `json:"trigger"`
	NodeCount   int                       `json:"node_count"`
	SendsEmail  bool                      `json:"sends_email"`
	Stats       domain.AutomationStats    `json:"stats"`
	Journeys    domain.JourneyCounts      `json:"journeys"`
	CreatedAt   time.Time                 `json:"created_at"`
	UpdatedAt   time.Time                 `json:"updated_at"`
	Truncation
}

type listAutomationsOutput struct {
	Workspace   WorkspaceRef        `json:"workspace"`
	Automations []automationSummary `json:"automations"`
	Total       int                 `json:"total"`
	NextCursor  string              `json:"next_cursor,omitempty"`
}

func (s *Server) listAutomations(ctx context.Context, call *Call) (any, error) {
	var args listAutomationsArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	limit, toolErr := PageLimit(args.Limit)
	if toolErr != nil {
		return nil, toolErr
	}
	statuses := args.statuses()
	filters := map[string]any{}
	if len(statuses) > 0 {
		filters["status"] = statuses
	}
	if args.ListID != "" {
		filters["list_id"] = args.ListID
	}
	offset := 0
	if args.Cursor != "" {
		if toolErr := DecodeCursor(args.Cursor, listAutomationsName, call.WorkspaceID, filters, &offset); toolErr != nil {
			return nil, toolErr
		}
		if offset < 0 {
			return nil, validationError("cursor is not valid for this call: drop it to start from the first page")
		}
	}

	// A limit of 0 reads every automation: the page is cut here, on an order
	// with a tie-breaker.
	filter := domain.AutomationFilter{ListID: args.ListID}
	for _, status := range statuses {
		filter.Status = append(filter.Status, domain.AutomationStatus(status))
	}
	automations, _, err := s.deps.Automations.List(ctx, call.WorkspaceID, filter)
	if err != nil {
		return nil, err
	}
	automations = slices.DeleteFunc(automations, func(a *domain.Automation) bool {
		return a == nil || a.DeletedAt != nil
	})
	slices.SortFunc(automations, func(a, b *domain.Automation) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})

	out := listAutomationsOutput{
		Workspace:   call.WorkspaceRef(),
		Automations: []automationSummary{},
		Total:       len(automations),
	}
	if offset >= len(automations) {
		return out, nil
	}
	end := min(offset+limit, len(automations))
	page := automations[offset:end]

	ids := make([]string, 0, len(page))
	for _, automation := range page {
		ids = append(ids, automation.ID)
	}
	journeys, err := s.deps.Automations.CountJourneys(ctx, call.WorkspaceID, ids)
	if err != nil {
		return nil, err
	}
	for _, automation := range page {
		out.Automations = append(out.Automations, summarizeAutomation(automation, journeys[automation.ID]))
	}
	if end < len(automations) {
		next, err := EncodeCursor(listAutomationsName, call.WorkspaceID, filters, end)
		if err != nil {
			return nil, err
		}
		out.NextCursor = next
	}
	return out, nil
}

func summarizeAutomation(automation *domain.Automation, journeys domain.JourneyCounts) automationSummary {
	out := automationSummary{
		ID:          automation.ID,
		Status:      string(automation.Status),
		ListID:      automation.ListID,
		ExitOnReply: automation.ExitOnReply,
		Journeys:    journeys,
		CreatedAt:   automation.CreatedAt,
		UpdatedAt:   automation.UpdatedAt,
	}
	out.Name = out.Text("name", automation.Name)
	if trigger := automation.Trigger; trigger != nil {
		out.Trigger = &automationTriggerSummary{
			EventKind:       trigger.EventKind,
			ListID:          trigger.ListID,
			SegmentID:       trigger.SegmentID,
			CustomEventName: out.OptionalText("trigger.custom_event_name", trigger.CustomEventName),
			Frequency:       string(trigger.Frequency),
		}
	}
	for _, node := range automation.Nodes {
		if node == nil {
			continue
		}
		out.NodeCount++
		if node.Type == domain.NodeTypeEmail {
			out.SendsEmail = true
		}
	}
	if automation.Stats != nil {
		out.Stats = *automation.Stats
	}
	return out
}

// getAutomationTool reads one automation with its whole graph.
func (s *Server) getAutomationTool() *Tool {
	return &Tool{
		Name:  "get_automation",
		Title: "Get automation",
		Description: "Reads one automation. It returns the name, status, list and the whole trigger: " +
			"event, list, segment or custom event, the fields of a contact.updated trigger, the link filters of email.link_clicked, the conditions as a segment tree, and frequency. " +
			"It also returns every step of the automation's graph with its type, configuration and next step. A webhook step's secret is never returned. " +
			"Automations can only be edited in the console. " +
			"get_automation_stats gives the results of each step, and get_automation_journey gives one contact's path.",
		Class: ClassRead,
		Pairs: []Pair{automationsRead},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":  map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"automation_id": map[string]any{"type": "string", "description": "Automation ID from list_automations."},
			},
			"required":             []string{"workspace_id", "automation_id"},
			"additionalProperties": false,
		},
		Handler: s.getAutomation,
	}
}

type automationIDArgs struct {
	AutomationID string `json:"automation_id"`
}

type automationTrigger struct {
	EventKind       string           `json:"event_kind"`
	ListID          *string          `json:"list_id"`
	SegmentID       *string          `json:"segment_id"`
	CustomEventName *string          `json:"custom_event_name"`
	UpdatedFields   []string         `json:"updated_fields"`
	LinkURL         *string          `json:"link_url"`
	BroadcastID     *string          `json:"broadcast_id"`
	TemplateID      *string          `json:"template_id"`
	Conditions      *domain.TreeNode `json:"conditions"`
	Frequency       string           `json:"frequency"`
}

// automationNode is one step of the graph: its configuration whole but for
// a webhook step's secret, without its canvas position.
type automationNode struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Config     map[string]any `json:"config"`
	NextNodeID *string        `json:"next_node_id"`
}

type automationDetail struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Status      string                 `json:"status"`
	ListID      string                 `json:"list_id"`
	ExitOnReply bool                   `json:"exit_on_reply"`
	Trigger     *automationTrigger     `json:"trigger"`
	RootNodeID  string                 `json:"root_node_id"`
	Nodes       []automationNode       `json:"nodes"`
	Stats       domain.AutomationStats `json:"stats"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Truncation
}

type getAutomationOutput struct {
	Workspace  WorkspaceRef     `json:"workspace"`
	Automation automationDetail `json:"automation"`
}

func (s *Server) getAutomation(ctx context.Context, call *Call) (any, error) {
	var args automationIDArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if args.AutomationID == "" {
		return nil, validationError("automation_id is required")
	}
	automation, err := s.deps.Automations.Get(ctx, call.WorkspaceID, args.AutomationID)
	if err != nil {
		return nil, err
	}
	// The repository filters deleted automations; one that comes back deleted
	// is answered the same way.
	if automation == nil || automation.DeletedAt != nil {
		return nil, &domain.ErrNotFound{Entity: "automation", ID: args.AutomationID}
	}
	return getAutomationOutput{Workspace: call.WorkspaceRef(), Automation: projectAutomation(automation)}, nil
}

// projectAutomation drops every webhook step's secret, whatever the key may
// write: the service keeps it for a key with automations:write, since the
// console edits it, and no tool edits automations.
func projectAutomation(automation *domain.Automation) automationDetail {
	automation = automation.WithoutWebhookSecrets()
	out := automationDetail{
		ID:          automation.ID,
		Status:      string(automation.Status),
		ListID:      automation.ListID,
		ExitOnReply: automation.ExitOnReply,
		RootNodeID:  automation.RootNodeID,
		Nodes:       []automationNode{},
		CreatedAt:   automation.CreatedAt,
		UpdatedAt:   automation.UpdatedAt,
	}
	out.Name = out.Text("name", automation.Name)
	if trigger := automation.Trigger; trigger != nil {
		out.Trigger = &automationTrigger{
			EventKind:       trigger.EventKind,
			ListID:          trigger.ListID,
			SegmentID:       trigger.SegmentID,
			CustomEventName: out.OptionalText("trigger.custom_event_name", trigger.CustomEventName),
			UpdatedFields:   trigger.UpdatedFields,
			LinkURL:         out.OptionalText("trigger.link_url", trigger.LinkURL),
			BroadcastID:     trigger.BroadcastID,
			TemplateID:      trigger.TemplateID,
			Conditions:      trigger.Conditions,
			Frequency:       string(trigger.Frequency),
		}
		if out.Trigger.UpdatedFields == nil {
			out.Trigger.UpdatedFields = []string{}
		}
	}
	for _, node := range automation.Nodes {
		if node == nil {
			continue
		}
		config := node.Config
		if config == nil {
			config = map[string]any{}
		}
		out.Nodes = append(out.Nodes, automationNode{ID: node.ID, Type: string(node.Type), Config: config, NextNodeID: node.NextNodeID})
	}
	if automation.Stats != nil {
		out.Stats = *automation.Stats
	}
	return out
}

// getAutomationJourneyTool reads one contact's latest journey through an
// automation.
func (s *Server) getAutomationJourneyTool() *Tool {
	return &Tool{
		Name:  "get_automation_journey",
		Title: "Get automation journey",
		Description: "Reads one contact's most recent journey through an automation. " +
			"It returns the journey's status, the step it is on, why it exited, its retries and last error, and the event that started it. " +
			"It also lists each step the journey went through, oldest first, with the action (entered, completed, failed, skipped), duration, output and error. " +
			"Only the contact's latest journey is returned, with its last 200 steps. steps_truncated says when older steps were left out. " +
			"Errors and outputs can quote a webhook's response. Text over 2000 characters is cut, and an output or event longer than that is null. " +
			"Both are named in truncated_fields. Returns a not_found error when the contact never entered the automation.",
		Class: ClassRead,
		Pairs: []Pair{automationsRead},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":  map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"automation_id": map[string]any{"type": "string", "description": "Automation ID from list_automations."},
				"email":         map[string]any{"type": "string", "description": "The contact's email address."},
			},
			"required":             []string{"workspace_id", "automation_id", "email"},
			"additionalProperties": false,
		},
		Handler: s.getAutomationJourney,
	}
}

type getAutomationJourneyArgs struct {
	AutomationID string `json:"automation_id"`
	Email        string `json:"email"`
}

type automationJourney struct {
	ContactEmail  string     `json:"contact_email"`
	Status        string     `json:"status"`
	CurrentNodeID *string    `json:"current_node_id"`
	ExitReason    *string    `json:"exit_reason"`
	EnteredAt     time.Time  `json:"entered_at"`
	ScheduledAt   *time.Time `json:"scheduled_at"`
	RetryCount    int        `json:"retry_count"`
	MaxRetries    int        `json:"max_retries"`
	LastError     *string    `json:"last_error"`
	LastRetryAt   *time.Time `json:"last_retry_at"`
	// TriggerEvent is the timeline event that started the journey.
	TriggerEvent any `json:"trigger_event"`
	Truncation
}

type automationStep struct {
	NodeID      string     `json:"node_id"`
	NodeType    string     `json:"node_type"`
	Action      string     `json:"action"`
	EnteredAt   time.Time  `json:"entered_at"`
	CompletedAt *time.Time `json:"completed_at"`
	DurationMs  *int64     `json:"duration_ms"`
	Output      any        `json:"output"`
	Error       *string    `json:"error"`
	Truncation
}

type getAutomationJourneyOutput struct {
	Workspace      WorkspaceRef      `json:"workspace"`
	AutomationID   string            `json:"automation_id"`
	Journey        automationJourney `json:"journey"`
	Steps          []automationStep  `json:"steps"`
	StepsTruncated bool              `json:"steps_truncated"`
}

func (s *Server) getAutomationJourney(ctx context.Context, call *Call) (any, error) {
	var args getAutomationJourneyArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if args.AutomationID == "" {
		return nil, validationError("automation_id is required")
	}
	// The journey is matched on the email verbatim.
	email := domain.NormalizeEmail(args.Email)
	if email == "" {
		return nil, validationError("email is required")
	}

	journey, steps, err := s.deps.Automations.GetContactNodeExecutions(ctx, call.WorkspaceID, args.AutomationID, email)
	if err != nil {
		var notFound *domain.ErrNotFound
		if errors.As(err, &notFound) && notFound.Entity == "contact_automation" {
			return nil, &ToolError{Code: CodeNotFound, Message: "this contact never entered this automation"}
		}
		return nil, err
	}
	if journey == nil {
		return nil, &ToolError{Code: CodeNotFound, Message: "this contact never entered this automation"}
	}

	out := getAutomationJourneyOutput{
		Workspace:    call.WorkspaceRef(),
		AutomationID: args.AutomationID,
		Journey:      projectJourney(journey),
		Steps:        []automationStep{},
	}
	steps = slices.DeleteFunc(slices.Clone(steps), func(step *domain.NodeExecution) bool { return step == nil })
	if len(steps) > maxJourneySteps {
		steps = steps[len(steps)-maxJourneySteps:]
		out.StepsTruncated = true
	}
	for _, step := range steps {
		out.Steps = append(out.Steps, projectStep(step))
	}
	return out, nil
}

func projectJourney(journey *domain.ContactAutomation) automationJourney {
	out := automationJourney{
		Status:        string(journey.Status),
		CurrentNodeID: journey.CurrentNodeID,
		EnteredAt:     journey.EnteredAt,
		ScheduledAt:   journey.ScheduledAt,
		RetryCount:    journey.RetryCount,
		MaxRetries:    journey.MaxRetries,
		LastRetryAt:   journey.LastRetryAt,
	}
	out.ContactEmail = out.Text("contact_email", journey.ContactEmail)
	out.ExitReason = out.OptionalText("exit_reason", journey.ExitReason)
	if journey.LastError != nil {
		capped := out.Text("last_error", *journey.LastError)
		out.LastError = &capped
	}
	// The rest of the context is the executor's bookkeeping.
	if trigger, ok := journey.Context["trigger"]; ok && trigger != nil {
		out.TriggerEvent = out.JSON("trigger_event", trigger)
	}
	return out
}

func projectStep(step *domain.NodeExecution) automationStep {
	out := automationStep{
		NodeID:      step.NodeID,
		NodeType:    string(step.NodeType),
		Action:      string(step.Action),
		EnteredAt:   step.EnteredAt,
		CompletedAt: step.CompletedAt,
		DurationMs:  step.DurationMs,
	}
	if step.Output != nil {
		out.Output = out.JSON("output", map[string]any(step.Output))
	}
	if step.Error != nil {
		capped := out.Text("error", *step.Error)
		out.Error = &capped
	}
	return out
}

// getAutomationStatsTool reads the results of each step of an automation.
func (s *Server) getAutomationStatsTool() *Tool {
	return &Tool{
		Name:  "get_automation_stats",
		Title: "Get automation stats",
		Description: "Reads the results of each step of an automation, keyed by step ID. " +
			"Each step has its visits, completions, failed attempts, skips and their reasons, errors, journeys that failed on it, journeys on it now (in progress, waiting, retrying), and the path taken at each branch, filter or A/B test. " +
			"Email steps add their email counters (sent, delivered, opened, clicked, bounced, unsubscribed and others) when the key also has the message_history read permission. " +
			"A step that was never reached has zeros. Give since and until together to count only executions that entered a step in that period. " +
			"The journeys on a step now ignore the period.",
		Class: ClassRead,
		Pairs: []Pair{automationsRead},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":  map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"automation_id": map[string]any{"type": "string", "description": "Automation ID from list_automations."},
				"since":         map[string]any{"type": "string", "description": "Start of the period, as an RFC 3339 date-time. Give it with until, or give neither for all time."},
				"until":         map[string]any{"type": "string", "description": "End of the period, as an RFC 3339 date-time. Give it with since, or give neither for all time."},
			},
			"required":             []string{"workspace_id", "automation_id"},
			"additionalProperties": false,
		},
		Handler: s.getAutomationStats,
	}
}

type getAutomationStatsArgs struct {
	AutomationID string `json:"automation_id"`
	Since        string `json:"since"`
	Until        string `json:"until"`
}

type getAutomationStatsOutput struct {
	Workspace    WorkspaceRef                 `json:"workspace"`
	AutomationID string                       `json:"automation_id"`
	Since        *time.Time                   `json:"since,omitempty"`
	Until        *time.Time                   `json:"until,omitempty"`
	Nodes        map[string]*domain.NodeStats `json:"nodes"`
}

func (s *Server) getAutomationStats(ctx context.Context, call *Call) (any, error) {
	var args getAutomationStatsArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	// FromURLParams reads and checks the period as automations.nodeStats does.
	params := map[string][]string{"workspace_id": {call.WorkspaceID}, "automation_id": {args.AutomationID}}
	if args.Since != "" {
		params["since"] = []string{args.Since}
	}
	if args.Until != "" {
		params["until"] = []string{args.Until}
	}
	var req domain.NodeStatsRequest
	if err := req.FromURLParams(params); err != nil {
		return nil, validationError("%v", err)
	}

	stats, err := s.deps.Automations.GetNodeStats(ctx, req.WorkspaceID, req.AutomationID, req.Since, req.Until)
	if err != nil {
		return nil, err
	}
	out := getAutomationStatsOutput{
		Workspace:    call.WorkspaceRef(),
		AutomationID: req.AutomationID,
		Since:        req.Since,
		Until:        req.Until,
		Nodes:        map[string]*domain.NodeStats{},
	}
	if stats != nil {
		for id, node := range stats.Nodes {
			if node != nil {
				out.Nodes[id] = node
			}
		}
	}
	return out, nil
}
