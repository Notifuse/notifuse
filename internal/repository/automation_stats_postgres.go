package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/lib/pq"
)

// CountJourneysByStatus counts the journeys of each automation by status, keyed by
// automation ID. It reads idx_contact_automations_automation (automation_id, status).
func (r *AutomationRepository) CountJourneysByStatus(ctx context.Context, workspaceID string, automationIDs []string) (map[string]domain.JourneyCounts, error) {
	counts := map[string]domain.JourneyCounts{}
	if len(automationIDs) == 0 {
		return counts, nil
	}

	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	rows, err := db.QueryContext(ctx,
		`SELECT automation_id, status, COUNT(*) FROM contact_automations WHERE automation_id = ANY($1) GROUP BY automation_id, status`,
		pq.Array(automationIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to count journeys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var automationID, status string
		var n int
		if err := rows.Scan(&automationID, &status, &n); err != nil {
			return nil, fmt.Errorf("failed to scan journey count: %w", err)
		}
		c := counts[automationID]
		c.Add(domain.ContactAutomationStatus(status), n)
		counts[automationID] = c
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read journey counts: %w", err)
	}
	return counts, nil
}

// periodCondition bounds column to [since, until] with the placeholders $2 and $3, after the
// automation ID in $1. Both nil: no bound.
func periodCondition(column string, since, until *time.Time) (string, []interface{}) {
	if since == nil || until == nil {
		return "", nil
	}
	return fmt.Sprintf(" AND %s >= $2 AND %s <= $3", column, column), []interface{}{*since, *until}
}

// The step results, one statement each, all filtered on the automation. The jsonb ?
// operator is written as is: these run through database/sql, which does not rewrite it.
const (
	// a. Visits are passes: each time a journey reached the step, so a loop counts each time
	// round and a retry is the same pass. A pass is either passed, one row of an attempt that
	// succeeded or is running ('processing', 'completed'; 'skipped' has no writer and counts
	// with them; a failed attempt's rows are 'failed'), or not passed yet, which
	// nodeStatsNotPassedSQL adds. No COUNT(DISTINCT): its sort was most of the cost.
	// A failed attempt is counted on the row handleError writes (no completed_at); the
	// processing row it marks failed carries a completed_at and is not counted again.
	// Known limits: a process killed in the middle of a step (crash, deploy, cancelled
	// context) or an attempt whose failure was not written leaves a 'processing' row behind,
	// and the step run again counts one more pass; two instances running one journey count
	// both runs, which did happen.
	nodeStatsVisitsSQL = `SELECT node_id,
		COUNT(*) FILTER (WHERE action IN ('processing', 'completed', 'skipped')),
		COUNT(*) FILTER (WHERE action = 'completed' AND NOT COALESCE(output ? 'error', false)),
		COUNT(*) FILTER (WHERE action = 'completed' AND output ? 'error'),
		COUNT(*) FILTER (WHERE action = 'failed' AND completed_at IS NULL),
		COUNT(*) FILTER (WHERE action = 'completed' AND output->>'skipped' = 'true')
	FROM automation_node_executions
	WHERE automation_id = $1%s
	GROUP BY node_id`

	// The passes not passed yet: a journey on its step now, or that ended on it, whose last
	// execution is there and is the enrollment ('entered', on the root) or a failed attempt.
	// Looking at the last execution rather than "no successful row on the step" keeps a loop's
	// earlier passes out; a journey that moved on after a delay has its last execution on the
	// delay, not on its next step. The period bounds that last execution, so a pass is dated
	// by its successful attempt, else by its last attempt.
	// (idx_contact_automations_automation, then idx_node_executions_contact_automation.)
	nodeStatsNotPassedSQL = `SELECT ca.current_node_id, COUNT(*)
	FROM contact_automations ca
	CROSS JOIN LATERAL (
		SELECT e.node_id, e.action, e.entered_at
		FROM automation_node_executions e
		WHERE e.contact_automation_id = ca.id
		ORDER BY e.entered_at DESC
		LIMIT 1
	) l
	WHERE ca.automation_id = $1 AND ca.status IN ('active', 'failed', 'exited')
		AND ca.current_node_id IS NOT NULL
		AND l.node_id = ca.current_node_id AND l.action IN ('entered', 'failed')%s
	GROUP BY 1`

	// Skipped emails by reason.
	nodeStatsSkipReasonsSQL = `SELECT node_id, output->>'skip_reason', COUNT(*)
	FROM automation_node_executions
	WHERE automation_id = $1 AND action = 'completed' AND output->>'skipped' = 'true'
		AND output->>'skip_reason' IS NOT NULL%s
	GROUP BY 1, 2`

	// b. The path each completed split took. A filter's result is compared as JSON, not
	// cast, so a value that is not a boolean gives no key instead of failing the query. An
	// output without the expected key (older data) gives NULL, not returned.
	nodeStatsBranchesSQL = `SELECT node_id, branch, COUNT(*) FROM (
		SELECT node_id,
			CASE node_type
				WHEN 'filter' THEN CASE WHEN output->'filter_passed' = 'true'::jsonb THEN 'yes'
					WHEN output->'filter_passed' = 'false'::jsonb THEN 'no' END
				WHEN 'list_status_branch' THEN output->>'branch_taken'
				WHEN 'ab_test' THEN output->>'variant_id'
				WHEN 'branch' THEN output->>'path_taken'
			END AS branch
		FROM automation_node_executions
		WHERE automation_id = $1 AND action = 'completed'
			AND node_type IN ('filter', 'list_status_branch', 'ab_test', 'branch')%s
	) b
	WHERE branch IS NOT NULL
	GROUP BY 1, 2`

	// c. The active journeys now, whatever the period, from each one's last execution
	// (idx_node_executions_contact_automation (contact_automation_id, entered_at DESC)). A journey
	// whose last execution is a delay still running is waiting on that delay, whatever its
	// current_node_id; one whose last execution failed is waiting for its retry. A journey
	// with neither a current step nor a running delay is on no step.
	nodeStatsInPlaceSQL = `SELECT state, node_id, COUNT(*) FROM (
		SELECT CASE WHEN w.waiting THEN 'waiting' WHEN l.action = 'failed' THEN 'retrying' ELSE 'in_progress' END AS state,
			CASE WHEN w.waiting THEN l.node_id ELSE ca.current_node_id END AS node_id
		FROM contact_automations ca
		CROSS JOIN LATERAL (
			SELECT e.node_id, e.node_type, e.action, e.output
			FROM automation_node_executions e
			WHERE e.contact_automation_id = ca.id
			ORDER BY e.entered_at DESC
			LIMIT 1
		) l
		CROSS JOIN LATERAL (
			SELECT l.node_type = 'delay' AND l.action = 'completed'
				AND COALESCE((l.output->>'delay_until')::timestamptz > now(), false) AS waiting
		) w
		WHERE ca.automation_id = $1 AND ca.status = 'active'
	) s
	WHERE node_id IS NOT NULL
	GROUP BY 1, 2`

	// The journeys that ended failed, on the step they failed on. The period bounds the
	// time the journey entered the automation.
	nodeStatsFailedJourneysSQL = `SELECT current_node_id, COUNT(*)
	FROM contact_automations
	WHERE automation_id = $1 AND status = 'failed' AND current_node_id IS NOT NULL%s
	GROUP BY 1`

	// d. Email counters: each completed email execution with the message it sent (primary
	// key) and, when it has none yet, whether its email is still in the queue, read in one
	// pass over the automation's queue entries (email_queue has no index on message_id). An
	// entry counts as queued while the worker can still send it: a permanent failure keeps
	// status 'failed' with next_retry_at NULL and a low attempt count, and is not queued.
	// The message filters are those of the message_history analytics measures.
	nodeStatsEmailSQL = `SELECT e.node_id,
		COUNT(*),
		COUNT(*) FILTER (WHERE m.id IS NULL AND q.message_id IS NOT NULL),
		COUNT(*) FILTER (WHERE m.id IS NULL AND q.message_id IS NULL),
		COUNT(m.id) FILTER (WHERE m.sent_at IS NOT NULL AND m.failed_at IS NULL),
		COUNT(m.id) FILTER (WHERE m.delivered_at IS NOT NULL),
		COUNT(m.id) FILTER (WHERE m.opened_at IS NOT NULL),
		COUNT(m.id) FILTER (WHERE m.sent_at IS NOT NULL AND m.failed_at IS NULL AND m.channel = 'email' AND m.opens_tracked IS NOT FALSE),
		COUNT(m.id) FILTER (WHERE m.clicked_at IS NOT NULL),
		COUNT(m.id) FILTER (WHERE m.sent_at IS NOT NULL AND m.failed_at IS NULL AND m.channel = 'email' AND m.clicks_tracked IS NOT FALSE),
		COUNT(m.id) FILTER (WHERE m.failed_at IS NOT NULL),
		COUNT(m.id) FILTER (WHERE m.bounced_at IS NOT NULL),
		COUNT(m.id) FILTER (WHERE m.complained_at IS NOT NULL),
		COUNT(m.id) FILTER (WHERE m.unsubscribed_at IS NOT NULL)
	FROM automation_node_executions e
	LEFT JOIN message_history m ON m.id = e.output->>'message_id'
	LEFT JOIN (
		SELECT DISTINCT eq.message_id
		FROM email_queue eq
		WHERE eq.source_type = 'automation' AND eq.source_id = $1
			AND (eq.status IN ('pending', 'processing', 'paused')
				OR (eq.status = 'failed' AND eq.next_retry_at IS NOT NULL AND eq.attempts < eq.max_attempts))
	) q ON q.message_id = e.output->>'message_id'
	WHERE e.automation_id = $1 AND e.node_type = 'email' AND e.action = 'completed'
		AND e.output ? 'message_id'%s
	GROUP BY e.node_id`
)

// GetNodeStats reads the results of each step of the automation that executions reached.
// The statements share one read-only snapshot, so the counts agree with each other.
func (r *AutomationRepository) GetNodeStats(ctx context.Context, workspaceID, automationID string, since, until *time.Time, withEmail bool) (*domain.NodeStatsRaw, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, fmt.Errorf("failed to begin node stats transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	raw := &domain.NodeStatsRaw{Nodes: map[string]*domain.NodeStats{}}
	node := func(id string) *domain.NodeStats {
		n, ok := raw.Nodes[id]
		if !ok {
			n = domain.NewNodeStats("")
			raw.Nodes[id] = n
		}
		return n
	}
	query := func(name, sqlText, column string, scan func(*sql.Rows) error) error {
		// A statement without a period (column "") has no %s to fill.
		args := []interface{}{automationID}
		if column != "" {
			period, periodArgs := periodCondition(column, since, until)
			sqlText = fmt.Sprintf(sqlText, period)
			args = append(args, periodArgs...)
		}
		rows, err := tx.QueryContext(ctx, sqlText, args...)
		if err != nil {
			return fmt.Errorf("failed to read node %s: %w", name, err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			if err := scan(rows); err != nil {
				return fmt.Errorf("failed to scan node %s: %w", name, err)
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("failed to read node %s: %w", name, err)
		}
		return nil
	}

	if err := query("visits", nodeStatsVisitsSQL, "entered_at", func(rows *sql.Rows) error {
		var id string
		var visits, completed, errorsCount, failedAttempts, skipped int64
		if err := rows.Scan(&id, &visits, &completed, &errorsCount, &failedAttempts, &skipped); err != nil {
			return err
		}
		n := node(id)
		n.Visits, n.Completed, n.Errors, n.FailedAttempts, n.Skipped = visits, completed, errorsCount, failedAttempts, skipped
		return nil
	}); err != nil {
		return nil, err
	}

	if err := query("not passed", nodeStatsNotPassedSQL, "l.entered_at", func(rows *sql.Rows) error {
		var id string
		var n int64
		if err := rows.Scan(&id, &n); err != nil {
			return err
		}
		node(id).Visits += n
		return nil
	}); err != nil {
		return nil, err
	}

	if err := query("skip reasons", nodeStatsSkipReasonsSQL, "entered_at", func(rows *sql.Rows) error {
		var id, reason string
		var n int64
		if err := rows.Scan(&id, &reason, &n); err != nil {
			return err
		}
		node(id).SkipReasons[reason] = n
		return nil
	}); err != nil {
		return nil, err
	}

	if err := query("branches", nodeStatsBranchesSQL, "entered_at", func(rows *sql.Rows) error {
		var id, branch string
		var n int64
		if err := rows.Scan(&id, &branch, &n); err != nil {
			return err
		}
		node(id).Branches[branch] = n
		return nil
	}); err != nil {
		return nil, err
	}

	// Journeys in place are the present state: no period.
	if err := query("journeys in place", nodeStatsInPlaceSQL, "", func(rows *sql.Rows) error {
		var state, id string
		var n int64
		if err := rows.Scan(&state, &id, &n); err != nil {
			return err
		}
		switch state {
		case "waiting":
			node(id).Waiting = n
		case "retrying":
			node(id).Retrying = n
		default:
			node(id).InProgress = n
		}
		return nil
	}); err != nil {
		return nil, err
	}

	if err := query("failed journeys", nodeStatsFailedJourneysSQL, "entered_at", func(rows *sql.Rows) error {
		var id string
		var n int64
		if err := rows.Scan(&id, &n); err != nil {
			return err
		}
		node(id).FailedJourneys = n
		return nil
	}); err != nil {
		return nil, err
	}

	if withEmail {
		if err := query("emails", nodeStatsEmailSQL, "e.entered_at", func(rows *sql.Rows) error {
			var id string
			var s domain.NodeEmailStats
			if err := rows.Scan(&id, &s.Executions, &s.Queued, &s.NotSent, &s.Sent, &s.Delivered, &s.Opened,
				&s.OpensTracked, &s.Clicked, &s.ClicksTracked, &s.Failed, &s.Bounced, &s.Complained, &s.Unsubscribed); err != nil {
				return err
			}
			node(id).Email = &s
			return nil
		}); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to end node stats transaction: %w", err)
	}
	return raw, nil
}
