package domain

import (
	"fmt"
	"time"
)

// JourneyCounts is the number of journeys of an automation in each status.
type JourneyCounts struct {
	Active    int `json:"active"`
	Completed int `json:"completed"`
	Exited    int `json:"exited"`
	Failed    int `json:"failed"`
	// Total counts every journey, one in a status not listed above included.
	Total int `json:"total"`
}

// Add counts n journeys in status. A status without its own field counts in Total only.
func (c *JourneyCounts) Add(status ContactAutomationStatus, n int) {
	switch status {
	case ContactAutomationStatusActive:
		c.Active += n
	case ContactAutomationStatusCompleted:
		c.Completed += n
	case ContactAutomationStatusExited:
		c.Exited += n
	case ContactAutomationStatusFailed:
		c.Failed += n
	}
	c.Total += n
}

// NodeStatsRequest is the query of automations.nodeStats. Since and Until bound the time
// the executions entered their step; both nil means all time.
type NodeStatsRequest struct {
	WorkspaceID  string
	AutomationID string
	Since        *time.Time
	Until        *time.Time
}

// FromURLParams reads the request and validates it.
func (r *NodeStatsRequest) FromURLParams(params map[string][]string) error {
	get := func(key string) string {
		if v, ok := params[key]; ok && len(v) > 0 {
			return v[0]
		}
		return ""
	}
	r.WorkspaceID = get("workspace_id")
	r.AutomationID = get("automation_id")

	since, until := get("since"), get("until")
	if (since == "") != (until == "") {
		return fmt.Errorf("since and until must be given together")
	}
	if since != "" {
		s, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return fmt.Errorf("invalid since parameter: must be an RFC 3339 date-time")
		}
		u, err := time.Parse(time.RFC3339, until)
		if err != nil {
			return fmt.Errorf("invalid until parameter: must be an RFC 3339 date-time")
		}
		s, u = s.UTC(), u.UTC()
		r.Since, r.Until = &s, &u
	}
	return r.Validate()
}

// Validate checks the request.
func (r *NodeStatsRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.AutomationID == "" {
		return fmt.Errorf("automation_id is required")
	}
	if (r.Since == nil) != (r.Until == nil) {
		return fmt.Errorf("since and until must be given together")
	}
	if r.Since != nil && r.Since.After(*r.Until) {
		return fmt.Errorf("since must not be after until")
	}
	return nil
}

// NodeEmailStats are the email counters of an email step, from its executions joined to the
// messages they sent.
type NodeEmailStats struct {
	// Executions are the completed executions that produced a message to send.
	Executions int64 `json:"executions"`
	// Queued are executions whose email is still in the queue, without a message yet.
	Queued int64 `json:"queued"`
	// NotSent are executions with neither a message nor an email in the queue, such as a
	// send cancelled by an exit on reply.
	NotSent       int64 `json:"not_sent"`
	Sent          int64 `json:"sent"`
	Delivered     int64 `json:"delivered"`
	Opened        int64 `json:"opened"`
	OpensTracked  int64 `json:"opens_tracked"`
	Clicked       int64 `json:"clicked"`
	ClicksTracked int64 `json:"clicks_tracked"`
	Failed        int64 `json:"failed"`
	Bounced       int64 `json:"bounced"`
	Complained    int64 `json:"complained"`
	Unsubscribed  int64 `json:"unsubscribed"`
}

// NodeStats are the results of one step of an automation.
type NodeStats struct {
	NodeType NodeType `json:"node_type"`
	// Visits are the distinct journeys that reached the step: a retry is not a new visit.
	Visits    int64 `json:"visits"`
	Completed int64 `json:"completed"`
	// FailedAttempts counts each failed attempt, retries included.
	FailedAttempts int64            `json:"failed_attempts"`
	Skipped        int64            `json:"skipped"`
	SkipReasons    map[string]int64 `json:"skip_reasons"`
	// Errors are completed executions that recorded an error, such as a list step whose
	// list was missing.
	Errors int64 `json:"errors"`
	// FailedJourneys are the journeys that ended failed on this step. The period bounds the
	// time they entered the automation, not the time they failed.
	FailedJourneys int64 `json:"failed_journeys"`
	// InProgress, Waiting and Retrying are the journeys on the step now, whatever the period.
	InProgress int64 `json:"in_progress"`
	Waiting    int64 `json:"waiting"`
	Retrying   int64 `json:"retrying"`
	// Branches counts completed executions by the path they took: yes and no for a filter,
	// the branch key of a list status branch, the variant ID of an A/B test, the path ID of
	// a branch.
	Branches map[string]int64 `json:"branches"`
	// Email is set on email steps when the member may read the message history.
	Email *NodeEmailStats `json:"email,omitempty"`
}

// NewNodeStats returns zeroed stats of a step of this type, with empty maps.
func NewNodeStats(nodeType NodeType) *NodeStats {
	return &NodeStats{NodeType: nodeType, SkipReasons: map[string]int64{}, Branches: map[string]int64{}}
}

// NodeStatsRaw is what the repository reads, by node ID. Its NodeStats carry no NodeType:
// the service takes it from the automation's graph.
type NodeStatsRaw struct {
	Nodes map[string]*NodeStats
}

// NodeStatsResponse is the response of automations.nodeStats.
type NodeStatsResponse struct {
	AutomationID string                `json:"automation_id"`
	Since        *time.Time            `json:"since,omitempty"`
	Until        *time.Time            `json:"until,omitempty"`
	Nodes        map[string]*NodeStats `json:"nodes"`
}

// NewNodeStatsResponse returns a response without nodes, its map ready.
func NewNodeStatsResponse(automationID string, since, until *time.Time) *NodeStatsResponse {
	return &NodeStatsResponse{AutomationID: automationID, Since: since, Until: until, Nodes: map[string]*NodeStats{}}
}
