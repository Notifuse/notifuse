package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/google/uuid"
)

// Journey history for the demo's showcase automations.
//
// Nothing in the demo ever executes an automation — the scheduler is disabled whenever
// ENVIRONMENT=demo — and the enrollment trigger is AFTER INSERT, so it cannot see the history the
// seed wrote moments earlier. Left alone, all five cards would read 0/0/0/0 next to 100k web
// sessions and 3,000 messages: the one dead feature in the demo. So the journeys are written here,
// directly.
//
// Two rules govern every row.
//
// **Inert.** The scheduler claims a journey only when contact_automations.status = 'active' AND
// scheduled_at <= now AND the parent automation is live and not deleted. Every seeded row gets a
// terminal status AND a NULL scheduled_at. Either alone would be enough today; both together mean
// the rows stay inert even if a visitor clicks Activate on one of the paused flows, because a NULL
// scheduled_at can never satisfy `scheduled_at <= now`. Pause does not touch these rows, so relying
// on the automation's status alone would leave a backlog that floods the queue the moment someone
// un-pauses.
//
// **Indistinguishable from real.** action='entered' is written by exactly one thing — the enroll
// function, for the root node only, with node_type hardcoded to 'trigger'. Every other node gets
// 'processing' on entry and is then updated in place to 'completed' or 'failed'. So a real funnel
// shows the enrollment total on the trigger's "Inflight" tile and 0 on every node below it, with the
// drop-off legible down the Completed column. We reproduce that rather than the prettier shape,
// because a non-zero Inflight on a delay node is a number no customer's own workspace can produce.
// action='skipped' is never written anywhere in the codebase, so it is not written here either.
const (
	// Fixed so a reset is reproducible. Contact creation dates come from the unseeded global rand,
	// so the cohorts themselves still shift between resets — this only pins the journey shapes.
	demoAutomationHistorySeed = 20260814

	// Journeys are spread over the same window the contact drawer shows web navigation for.
	demoAutomationHistoryDays = 90

	// Rows per multi-row INSERT. Postgres caps a statement at 65535 parameters; the widest row here
	// uses 10, so this leaves an order of magnitude of headroom.
	demoHistoryInsertBatch = 400

	// Rows per committed batch for the writes that cascade into contact_segment_queue. Small and
	// committed often, because each one takes a queue-row lock per contact.
	demoHistorySideEffectBatch = 100
)

// demoJourneyOutcome is one shape a journey can take through an automation, in the engine's terms:
// the ordered node ids visited, and how it ended.
type demoJourneyOutcome struct {
	Path     []string
	Status   domain.ContactAutomationStatus
	FailLast bool // the last node errored instead of completing
	Weight   int
}

// demoCohortMember is a contact eligible for an automation, with the newsletter status that decides
// whether the engine's subscription guard would have stopped them.
type demoCohortMember struct {
	Email      string
	ListStatus string
}

type demoNodeVisit struct {
	NodeID      string
	NodeType    domain.NodeType
	Action      domain.NodeAction
	EnteredAt   time.Time
	CompletedAt time.Time
	DurationMs  int
}

type demoJourney struct {
	ID            string
	AutomationID  string
	Email         string
	CurrentNodeID string
	Status        domain.ContactAutomationStatus
	ExitReason    *string
	EnteredAt     time.Time
	EndedAt       time.Time
	Visits        []demoNodeVisit
	// TimelineReason is what the automation.end row carries. It deliberately differs from
	// ExitReason: the engine leaves the column NULL on a normal completion while still writing
	// "completed" to the timeline.
	TimelineReason string
}

// demoAutomationOutcomes returns the journey shapes for an automation.
//
// The weights are a model, but the shapes are not: each path is one the engine could really produce.
// In particular a contact rejected by cart-recovery's filter ends 'completed' at the filter node, not
// 'exited' — the filter's exit branch is empty, and FilterNodeExecutor returns Completed with no exit
// reason when there is nowhere to route. ("filter_rejected" appears nowhere in the codebase except a
// doc comment; nothing writes it.)
func demoAutomationOutcomes(automationID string) []demoJourneyOutcome {
	switch automationID {
	case demoAutomationWelcome:
		full := []string{"ws-trigger", "ws-delay-15m", "ws-email-welcome", "ws-delay-3d", "ws-status-check"}
		return []demoJourneyOutcome{
			{Path: append(append([]string{}, full...), "ws-email-digest"), Status: domain.ContactAutomationStatusCompleted, Weight: 80},
			{Path: append(append([]string{}, full...), "ws-email-reengage"), Status: domain.ContactAutomationStatusCompleted, Weight: 16},
			{Path: []string{"ws-trigger", "ws-delay-15m", "ws-email-welcome"}, Status: domain.ContactAutomationStatusFailed, FailLast: true, Weight: 4},
		}
	case demoAutomationCartRecovery:
		toTest := []string{"cr-trigger", "cr-delay-4h", "cr-filter", "cr-abtest"}
		return []demoJourneyOutcome{
			{Path: append(append([]string{}, toTest...), "cr-email-a"), Status: domain.ContactAutomationStatusCompleted, Weight: 37},
			{Path: append(append([]string{}, toTest...), "cr-email-b"), Status: domain.ContactAutomationStatusCompleted, Weight: 37},
			// Rejected by the filter. Its exit_node_id is empty, so the journey simply completes there.
			{Path: []string{"cr-trigger", "cr-delay-4h", "cr-filter"}, Status: domain.ContactAutomationStatusCompleted, Weight: 22},
			{Path: append(append([]string{}, toTest...), "cr-email-a"), Status: domain.ContactAutomationStatusFailed, FailLast: true, Weight: 2},
			{Path: append(append([]string{}, toTest...), "cr-email-b"), Status: domain.ContactAutomationStatusFailed, FailLast: true, Weight: 2},
		}
	case demoAutomationPostPurchase:
		full := []string{"pp-trigger", "pp-delay-1h", "pp-add-vip", "pp-email-thanks"}
		return []demoJourneyOutcome{
			{Path: full, Status: domain.ContactAutomationStatusCompleted, Weight: 96},
			{Path: full, Status: domain.ContactAutomationStatusFailed, FailLast: true, Weight: 4},
		}
	case demoAutomationVIPConcierge:
		full := []string{"vc-trigger", "vc-add-vip", "vc-webhook"}
		return []demoJourneyOutcome{
			{Path: full, Status: domain.ContactAutomationStatusCompleted, Weight: 92},
			{Path: full, Status: domain.ContactAutomationStatusFailed, FailLast: true, Weight: 8},
		}
	case demoAutomationWinback:
		toFilter := []string{"wb-trigger", "wb-email-offer", "wb-delay-7d", "wb-filter"}
		return []demoJourneyOutcome{
			// They opened something: the filter passes, continue_node_id is empty, journey completes.
			{Path: toFilter, Status: domain.ContactAutomationStatusCompleted, Weight: 31},
			// They did not: the filter routes to the removal, which is the point of the flow.
			{Path: append(append([]string{}, toFilter...), "wb-remove"), Status: domain.ContactAutomationStatusCompleted, Weight: 63},
			{Path: []string{"wb-trigger", "wb-email-offer"}, Status: domain.ContactAutomationStatusFailed, FailLast: true, Weight: 6},
		}
	}
	return nil
}

// demoAutomationExitPath returns the path a contact takes when the email node's subscription guard
// stops them, and nil when the automation cannot produce that outcome.
//
// The guard is the only thing in the engine that really produces status='exited' from a node, and it
// only applies to subscription-sensitive template categories — marketing and blog. So post-purchase
// (a transactional thank-you) and vip-concierge (no email at all) have no exit path, and their Exited
// counters are honestly zero.
func demoAutomationExitPath(automationID string) []string {
	switch automationID {
	case demoAutomationWelcome:
		// The welcome email itself is category "welcome", which the guard does not cover, so an
		// unsubscribed contact receives it, reaches the status check, is routed down the non-active
		// branch, and is stopped at the marketing re-engagement email.
		return []string{"ws-trigger", "ws-delay-15m", "ws-email-welcome", "ws-delay-3d", "ws-status-check", "ws-email-reengage"}
	case demoAutomationCartRecovery:
		return []string{"cr-trigger", "cr-delay-4h", "cr-filter", "cr-abtest", "cr-email-a"}
	case demoAutomationWinback:
		return []string{"wb-trigger", "wb-email-offer"}
	}
	return nil
}

// demoAutomationCohortSQL returns the query that finds an automation's contacts, along with their
// newsletter status.
//
// The cohorts are drawn from the behaviour that would genuinely have triggered each flow, so a
// prospect who clicks from an automation into a contact finds the event that put them there. Note
// the contact key on custom_events and contact_lists is `email` — `contact_email` is the column name
// on contact_automations and automation_node_executions only.
//
// vip-concierge and winback-sunset derive from purchases rather than from contact_segments, because
// segment membership is built asynchronously by the task scheduler and is still empty at this point
// in the seed.
func demoAutomationCohortSQL(automationID string) (string, []interface{}) {
	const listStatus = `COALESCE((
		SELECT cl.status FROM contact_lists cl
		WHERE cl.email = src.email AND cl.list_id = '` + demoListNewsletter + `' AND cl.deleted_at IS NULL
	), '')`

	switch automationID {
	case demoAutomationWelcome:
		return `SELECT src.email, ` + listStatus + `
			FROM (
				SELECT email FROM contact_lists
				WHERE list_id = $1 AND deleted_at IS NULL AND status = 'active'
			) src
			ORDER BY src.email`, []interface{}{demoListNewsletter}

	case demoAutomationCartRecovery, demoAutomationPostPurchase:
		goal := demoGoalAddToCart
		if automationID == demoAutomationPostPurchase {
			goal = demoGoalPurchase
		}
		return `SELECT src.email, ` + listStatus + `
			FROM (
				SELECT DISTINCT email FROM custom_events
				WHERE event_name = $1 AND deleted_at IS NULL
			) src
			ORDER BY src.email`, []interface{}{goal}

	case demoAutomationVIPConcierge:
		return `SELECT src.email, ` + listStatus + `
			FROM (
				SELECT DISTINCT email FROM custom_events
				WHERE event_name = $1 AND deleted_at IS NULL
			) src
			ORDER BY src.email`, []interface{}{demoGoalPurchase}

	case demoAutomationWinback:
		// Bought at some point, but not lately — the same shape as the Win-back Opportunities segment.
		return `SELECT src.email, ` + listStatus + `
			FROM (
				SELECT email FROM custom_events
				WHERE event_name = $1 AND deleted_at IS NULL
				GROUP BY email
				HAVING MAX(occurred_at) < NOW() - INTERVAL '90 days'
			) src
			ORDER BY src.email`, []interface{}{demoGoalPurchase}
	}
	return "", nil
}

// demoAutomationCohortShare caps how much of an eligible population actually enrolled. A flow that
// enrolled literally every matching contact reads as synthetic.
func demoAutomationCohortShare(automationID string) float64 {
	switch automationID {
	case demoAutomationWelcome:
		return 0.42
	case demoAutomationVIPConcierge:
		return 0.55
	}
	return 1.0
}

// seedDemoAutomationHistory writes the journey history for the automations that were really created.
func (s *DemoService) seedDemoAutomationHistory(ctx context.Context, workspaceID string, automations []*domain.Automation) error {
	if len(automations) == 0 {
		return nil
	}

	db, err := s.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	rng := rand.New(rand.NewSource(demoAutomationHistorySeed))
	now := time.Now().UTC()

	for _, automation := range automations {
		cohort, err := s.loadDemoCohort(ctx, db, automation.ID)
		if err != nil {
			s.logger.WithFields(map[string]interface{}{
				"automation_id": automation.ID,
				"error":         err.Error(),
			}).Warn("Failed to load demo automation cohort")
			continue
		}
		if len(cohort) == 0 {
			continue
		}

		journeys := buildDemoJourneys(automation, cohort, rng, now)
		if len(journeys) == 0 {
			continue
		}

		if err := s.writeDemoJourneys(ctx, db, automation, journeys); err != nil {
			s.logger.WithFields(map[string]interface{}{
				"automation_id": automation.ID,
				"error":         err.Error(),
			}).Warn("Failed to write demo automation journeys")
			continue
		}

		// Everything below cascades into contact_segment_queue through the contact_timeline and
		// contact_lists triggers, so it runs after the journey transaction has committed and in its
		// own small batches. Held inside that transaction it would lock the queue row of every
		// contact in the cohort until commit, and the segment-queue processor — which is not
		// demo-gated — would time out and silently drop the batch it had already claimed.
		if err := s.writeDemoJourneyTimeline(ctx, db, automation, journeys); err != nil {
			s.logger.WithFields(map[string]interface{}{
				"automation_id": automation.ID,
				"error":         err.Error(),
			}).Warn("Failed to write demo automation timeline entries")
		}
		if err := s.writeDemoJourneyListChanges(ctx, db, journeys); err != nil {
			s.logger.WithFields(map[string]interface{}{
				"automation_id": automation.ID,
				"error":         err.Error(),
			}).Warn("Failed to write demo automation list changes")
		}
	}

	return nil
}

func (s *DemoService) loadDemoCohort(ctx context.Context, db *sql.DB, automationID string) ([]demoCohortMember, error) {
	query, args := demoAutomationCohortSQL(automationID)
	if query == "" {
		return nil, nil
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query cohort: %w", err)
	}
	defer rows.Close()

	var cohort []demoCohortMember
	for rows.Next() {
		var member demoCohortMember
		if err := rows.Scan(&member.Email, &member.ListStatus); err != nil {
			return nil, fmt.Errorf("failed to scan cohort row: %w", err)
		}
		cohort = append(cohort, member)
	}
	return cohort, rows.Err()
}

// buildDemoJourneys turns a cohort into journeys. The cohort arrives ordered by email from the
// database, which is what keeps a fixed seed meaningful.
func buildDemoJourneys(automation *domain.Automation, cohort []demoCohortMember, rng *rand.Rand, now time.Time) []demoJourney {
	outcomes := demoAutomationOutcomes(automation.ID)
	if len(outcomes) == 0 {
		return nil
	}

	totalWeight := 0
	for _, o := range outcomes {
		totalWeight += o.Weight
	}
	if totalWeight == 0 {
		return nil
	}

	exitPath := demoAutomationExitPath(automation.ID)
	share := demoAutomationCohortShare(automation.ID)
	window := time.Duration(demoAutomationHistoryDays) * 24 * time.Hour

	journeys := make([]demoJourney, 0, len(cohort))
	for _, member := range cohort {
		if share < 1.0 && rng.Float64() > share {
			continue
		}

		var path []string
		status := domain.ContactAutomationStatusCompleted
		failLast := false
		var exitReason *string
		timelineReason := "completed"

		if exitPath != nil && member.ListStatus != "" && member.ListStatus != "active" {
			// The email node's subscription guard: the send is refused and the journey exits.
			path = exitPath
			status = domain.ContactAutomationStatusExited
			reason := member.ListStatus
			exitReason = &reason
			timelineReason = reason
		} else {
			outcome := pickDemoOutcome(outcomes, totalWeight, rng)
			path = outcome.Path
			status = outcome.Status
			failLast = outcome.FailLast
			if status == domain.ContactAutomationStatusFailed {
				timelineReason = "failed"
			}
		}

		// A journey has to have finished before now, delays included: this one waits three days
		// between its two touches, so a contact enrolled an hour ago would "complete" it in the
		// future and the contact drawer would sort the entry above the event that caused it.
		//
		// entered_at must also be distinct per (automation, contact) — contact_automations has a
		// UNIQUE on exactly that — hence the nanosecond jitter, which does not rely on luck.
		elapsed := demoPathElapsed(automation, path)
		room := window - elapsed
		if room <= 0 {
			room = time.Hour
		}
		enteredAt := now.Add(-elapsed).
			Add(-time.Duration(rng.Int63n(int64(room)))).
			Add(time.Duration(rng.Intn(1000000)) * time.Nanosecond)

		visits := buildDemoNodeVisits(automation, path, enteredAt, failLast, status, rng)
		if len(visits) == 0 {
			continue
		}

		last := visits[len(visits)-1]
		journeys = append(journeys, demoJourney{
			ID:             uuid.NewString(),
			AutomationID:   automation.ID,
			Email:          member.Email,
			CurrentNodeID:  path[len(path)-1],
			Status:         status,
			ExitReason:     exitReason,
			EnteredAt:      enteredAt,
			EndedAt:        last.CompletedAt,
			Visits:         visits,
			TimelineReason: timelineReason,
		})
	}
	return journeys
}

func pickDemoOutcome(outcomes []demoJourneyOutcome, totalWeight int, rng *rand.Rand) demoJourneyOutcome {
	roll := rng.Intn(totalWeight)
	for _, o := range outcomes {
		roll -= o.Weight
		if roll < 0 {
			return o
		}
	}
	return outcomes[len(outcomes)-1]
}

// buildDemoNodeVisits produces one execution row per node visit, plus the extra 'entered' row the
// enroll function writes for the root node.
func buildDemoNodeVisits(automation *domain.Automation, path []string, enteredAt time.Time, failLast bool, status domain.ContactAutomationStatus, rng *rand.Rand) []demoNodeVisit {
	visits := make([]demoNodeVisit, 0, len(path)+1)
	cursor := enteredAt

	for i, nodeID := range path {
		node := automation.GetNodeByID(nodeID)
		if node == nil {
			return nil
		}

		if i == 0 {
			// Enrollment. One row, action='entered', node_type hardcoded to 'trigger' exactly as
			// automation_enroll_contact writes it — this is the only place 'entered' ever appears.
			visits = append(visits, demoNodeVisit{
				NodeID:    nodeID,
				NodeType:  domain.NodeTypeTrigger,
				Action:    domain.NodeActionEntered,
				EnteredAt: cursor,
			})
		}

		// A delay node's elapsed time is the delay itself; everything else is processing time.
		elapsed := time.Duration(50+rng.Intn(750)) * time.Millisecond
		if node.Type == domain.NodeTypeDelay {
			elapsed = demoDelayDuration(node.Config)
		}

		action := domain.NodeActionCompleted
		isLast := i == len(path)-1
		if isLast && failLast {
			action = domain.NodeActionFailed
		}
		// An exited journey stops at its last node without completing it.
		if isLast && status == domain.ContactAutomationStatusExited {
			action = domain.NodeActionCompleted
		}

		visits = append(visits, demoNodeVisit{
			NodeID:      nodeID,
			NodeType:    node.Type,
			Action:      action,
			EnteredAt:   cursor,
			CompletedAt: cursor.Add(elapsed),
			DurationMs:  int(elapsed / time.Millisecond),
		})
		cursor = cursor.Add(elapsed)
	}

	return visits
}

// demoPathElapsed is how long a journey down this path takes, which is dominated by its delay nodes.
// A second of slack covers the per-node processing time, which is milliseconds.
func demoPathElapsed(automation *domain.Automation, path []string) time.Duration {
	elapsed := time.Second
	for _, nodeID := range path {
		if node := automation.GetNodeByID(nodeID); node != nil && node.Type == domain.NodeTypeDelay {
			elapsed += demoDelayDuration(node.Config)
		}
	}
	return elapsed
}

// demoDelayDuration reads a delay node's configured wait.
func demoDelayDuration(config map[string]interface{}) time.Duration {
	duration, _ := config["duration"].(int)
	if duration <= 0 {
		return time.Minute
	}
	unit, _ := config["unit"].(string)
	switch unit {
	case "minutes":
		return time.Duration(duration) * time.Minute
	case "hours":
		return time.Duration(duration) * time.Hour
	case "days":
		return time.Duration(duration) * 24 * time.Hour
	}
	return time.Duration(duration) * time.Minute
}

// writeDemoJourneys writes the enrollments, their node executions and the automation's card stats in
// one transaction, so the numbers can never disagree with the rows behind them.
func (s *DemoService) writeDemoJourneys(ctx context.Context, db *sql.DB, automation *domain.Automation, journeys []demoJourney) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := insertDemoContactAutomations(ctx, tx, journeys); err != nil {
		return err
	}
	if err := insertDemoNodeExecutions(ctx, tx, journeys); err != nil {
		return err
	}

	stats := demoAutomationStats(journeys)
	statsJSON, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("failed to marshal automation stats: %w", err)
	}
	// The status is set here, with the stats, for a reason worth stating: AutomationService.Create
	// overwrites whatever status it is handed with draft, to stop anyone writing a live row with no
	// trigger behind it. That guard is right, but it also means the three flows that should come to
	// rest paused cannot get there through the service — Update refuses status changes, and Pause
	// demands the automation already be live, which would install the very trigger we are avoiding.
	// Paused installs nothing and claims nothing, so writing it directly is safe; and doing it in
	// this transaction means the badge and the numbers land together or not at all.
	if _, err := tx.ExecContext(ctx,
		`UPDATE automations SET stats = $1, status = $2 WHERE id = $3`,
		statsJSON, string(demoIntendedStatus(automation.ID)), automation.ID,
	); err != nil {
		return fmt.Errorf("failed to update automation stats: %w", err)
	}

	return tx.Commit()
}

func demoAutomationStats(journeys []demoJourney) domain.AutomationStats {
	stats := domain.AutomationStats{Enrolled: int64(len(journeys))}
	for _, j := range journeys {
		switch j.Status {
		case domain.ContactAutomationStatusCompleted:
			stats.Completed++
		case domain.ContactAutomationStatusExited:
			stats.Exited++
		case domain.ContactAutomationStatusFailed:
			stats.Failed++
		}
	}
	return stats
}

// demoContactAutomationColumns is the column order demoContactAutomationArgs fills.
const demoContactAutomationColumns = 9

// demoContactAutomationArgs returns one enrollment row's insert arguments.
func demoContactAutomationArgs(j demoJourney) []interface{} {
	return []interface{}{
		j.ID, j.AutomationID, j.Email, j.CurrentNodeID, string(j.Status), j.ExitReason,
		j.EnteredAt,
		// scheduled_at is never a timestamp here. A NULL can never satisfy the scheduler's
		// `scheduled_at <= now`, which keeps these rows inert no matter what happens to the
		// automation's status later — including a visitor pressing Activate.
		nil,
		[]byte(`{}`),
	}
}

// demoScheduledAtArg is the scheduled_at value a journey's row carries.
func demoScheduledAtArg(j demoJourney) interface{} {
	return demoContactAutomationArgs(j)[7]
}

func insertDemoContactAutomations(ctx context.Context, tx *sql.Tx, journeys []demoJourney) error {
	return execDemoBatches(ctx, tx, len(journeys), demoHistoryInsertBatch, demoContactAutomationColumns,
		`INSERT INTO contact_automations
			(id, automation_id, contact_email, current_node_id, status, exit_reason, entered_at, scheduled_at, context) VALUES `,
		func(index int) []interface{} { return demoContactAutomationArgs(journeys[index]) })
}

func insertDemoNodeExecutions(ctx context.Context, tx *sql.Tx, journeys []demoJourney) error {
	type row struct {
		journey demoJourney
		visit   demoNodeVisit
	}
	rows := make([]row, 0, len(journeys)*4)
	for _, j := range journeys {
		for _, v := range j.Visits {
			rows = append(rows, row{journey: j, visit: v})
		}
	}

	const columns = 10
	return execDemoBatches(ctx, tx, len(rows), demoHistoryInsertBatch, columns,
		`INSERT INTO automation_node_executions
			(id, contact_automation_id, automation_id, node_id, node_type, action, entered_at, completed_at, duration_ms, output) VALUES `,
		func(index int) []interface{} {
			r := rows[index]
			var completedAt interface{}
			var durationMs interface{}
			if r.visit.Action != domain.NodeActionEntered {
				completedAt = r.visit.CompletedAt
				durationMs = r.visit.DurationMs
			}
			return []interface{}{
				uuid.NewString(),
				r.journey.ID,
				// Nullable in the DDL, but the console's per-node stats query filters on it, so a row
				// without it is invisible in the Flow Stats drawer.
				r.journey.AutomationID,
				r.visit.NodeID,
				// The node's real type. The engine's own error path hardcodes 'trigger' here, which
				// splits one node into two GROUP BY (node_id, node_type) groups and lets the console —
				// which keys its map by node_id alone — overwrite one with the other. Reproducing that
				// would make a node's numbers vanish at random.
				string(r.visit.NodeType),
				string(r.visit.Action),
				r.visit.EnteredAt,
				completedAt,
				durationMs,
				[]byte(`{}`),
			}
		})
}

// demoTimelineEntry is one contact_timeline row for a journey.
type demoTimelineEntry struct {
	Email     string
	Operation string
	Kind      string
	Changes   []byte
	CreatedAt time.Time
}

// demoJourneyTimelineEntries builds the automation.start and automation.end rows the contact drawer
// renders, ordered by email so concurrent writers take the contact_segment_queue locks in the same
// order.
func demoJourneyTimelineEntries(automation *domain.Automation, journeys []demoJourney) ([]demoTimelineEntry, error) {
	type entry = demoTimelineEntry

	entries := make([]entry, 0, len(journeys)*2)
	for _, j := range journeys {
		start, err := json.Marshal(map[string]interface{}{
			"automation_id": map[string]interface{}{"new": j.AutomationID},
			"root_node_id":  map[string]interface{}{"new": automation.RootNodeID},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal automation.start changes: %w", err)
		}
		end, err := json.Marshal(map[string]interface{}{
			"automation_id": map[string]interface{}{"new": j.AutomationID},
			"exit_reason":   map[string]interface{}{"new": j.TimelineReason},
			"status":        map[string]interface{}{"new": string(j.Status)},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal automation.end changes: %w", err)
		}
		// created_at is the journey's own time, not now: an entry stamped with the reset time would
		// sort above the cart event that caused it.
		entries = append(entries,
			entry{j.Email, "insert", "automation.start", start, j.EnteredAt},
			entry{j.Email, "update", "automation.end", end, j.EndedAt},
		)
	}

	sort.SliceStable(entries, func(i, k int) bool {
		if entries[i].Email != entries[k].Email {
			return entries[i].Email < entries[k].Email
		}
		return entries[i].CreatedAt.Before(entries[k].CreatedAt)
	})

	return entries, nil
}

// writeDemoJourneyTimeline writes the journeys' timeline entries in committed batches.
func (s *DemoService) writeDemoJourneyTimeline(ctx context.Context, db *sql.DB, automation *domain.Automation, journeys []demoJourney) error {
	entries, err := demoJourneyTimelineEntries(automation, journeys)
	if err != nil {
		return err
	}

	const columns = 7
	return execDemoBatches(ctx, db, len(entries), demoHistorySideEffectBatch, columns,
		`INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) VALUES `,
		func(index int) []interface{} {
			e := entries[index]
			return []interface{}{e.Email, e.Operation, "automation", e.Kind, automation.ID, e.Changes, e.CreatedAt}
		})
}

// writeDemoJourneyListChanges applies the audience changes the journeys' add_to_list and
// remove_from_list nodes would have made.
//
// Without this the VIP Club list ships empty while post-purchase's card claims hundreds of journeys
// completed through an add_to_list node — and the sunset flow claims removals from a list nobody ever
// left. Nothing executes those nodes in demo, so the seed has to.
func (s *DemoService) writeDemoJourneyListChanges(ctx context.Context, db *sql.DB, journeys []demoJourney) error {
	vipAdds, newsletterRemovals := demoJourneyListChanges(journeys)

	const addColumns = 5
	if err := execDemoBatches(ctx, db, len(vipAdds), demoHistorySideEffectBatch, addColumns,
		`INSERT INTO contact_lists (email, list_id, status, created_at, updated_at) VALUES `,
		func(index int) []interface{} {
			j := vipAdds[index]
			return []interface{}{j.Email, demoListVIPClub, "active", j.EndedAt, j.EndedAt}
		},
		` ON CONFLICT (email, list_id) DO NOTHING`); err != nil {
		return err
	}

	// A removal is a soft delete, matching RemoveContactFromList — the same UPDATE the node would
	// have run, which is also what makes the contact_lists trigger record it.
	for start := 0; start < len(newsletterRemovals); start += demoHistorySideEffectBatch {
		end := start + demoHistorySideEffectBatch
		if end > len(newsletterRemovals) {
			end = len(newsletterRemovals)
		}
		emails := make([]string, 0, end-start)
		for _, j := range newsletterRemovals[start:end] {
			emails = append(emails, j.Email)
		}
		placeholders := make([]string, len(emails))
		args := make([]interface{}, 0, len(emails)+2)
		args = append(args, time.Now().UTC(), demoListNewsletter)
		for i, email := range emails {
			placeholders[i] = fmt.Sprintf("$%d", i+3)
			args = append(args, email)
		}
		query := fmt.Sprintf(
			`UPDATE contact_lists SET deleted_at = $1, updated_at = $1 WHERE list_id = $2 AND email IN (%s) AND deleted_at IS NULL`,
			strings.Join(placeholders, ", "),
		)
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("failed to apply demo list removals: %w", err)
		}
	}

	return nil
}

// demoJourneyListChanges works out which journeys really reached an add_to_list or remove_from_list
// node, so the audiences match what the cards claim. Only a completed visit counts: a journey that
// failed or was stopped at that node never changed anything.
func demoJourneyListChanges(journeys []demoJourney) (vipAdds, newsletterRemovals []demoJourney) {
	for _, j := range journeys {
		for _, v := range j.Visits {
			if v.Action != domain.NodeActionCompleted {
				continue
			}
			switch v.NodeID {
			case "pp-add-vip", "vc-add-vip":
				vipAdds = append(vipAdds, j)
			case "wb-remove":
				newsletterRemovals = append(newsletterRemovals, j)
			}
		}
	}

	// Ordered by email so concurrent writers take the contact_segment_queue locks in the same order.
	sortDemoJourneysByEmail(vipAdds)
	sortDemoJourneysByEmail(newsletterRemovals)
	return vipAdds, newsletterRemovals
}

func sortDemoJourneysByEmail(journeys []demoJourney) {
	sort.SliceStable(journeys, func(i, k int) bool { return journeys[i].Email < journeys[k].Email })
}

// demoExecer is satisfied by both *sql.DB and *sql.Tx, so the same batching helper serves the
// transactional writes and the committed ones.
type demoExecer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// execDemoBatches runs a multi-row INSERT in batches of batchSize. suffix, if given, is appended
// after the VALUES list (an ON CONFLICT clause, say).
func execDemoBatches(ctx context.Context, execer demoExecer, total, batchSize, columns int, prefix string, argsFor func(index int) []interface{}, suffix ...string) error {
	tail := ""
	if len(suffix) > 0 {
		tail = suffix[0]
	}

	for start := 0; start < total; start += batchSize {
		end := start + batchSize
		if end > total {
			end = total
		}

		var builder strings.Builder
		builder.WriteString(prefix)
		args := make([]interface{}, 0, (end-start)*columns)

		for i := start; i < end; i++ {
			if i > start {
				builder.WriteString(", ")
			}
			builder.WriteString("(")
			for c := 0; c < columns; c++ {
				if c > 0 {
					builder.WriteString(", ")
				}
				builder.WriteString(fmt.Sprintf("$%d", len(args)+c+1))
			}
			builder.WriteString(")")
			args = append(args, argsFor(i)...)
		}
		builder.WriteString(tail)

		if _, err := execer.ExecContext(ctx, builder.String(), args...); err != nil {
			return fmt.Errorf("failed to insert demo automation rows: %w", err)
		}
	}
	return nil
}
