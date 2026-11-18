package migrations

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/lib/pq"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/trackingtoken"
)

// V44Migration, on workspaces that already exist, makes a click on a link write its own
// timeline entry, adds the partial indexes of the dashboard's recent failures, dates the
// broadcasts that started without a start date, removes the segment memberships of deleted
// contacts and the instance's own links recorded in clicked_links, recounts the exited
// journeys of each automation, passes the event that started a journey to its emails, marks
// processed the scheduled A/B broadcasts sent before 44 that stayed scheduled, recompiles the
// segments whose stored query changed, adds segments.last_error and last_error_at, and adds
// broadcasts.test_paused_seconds.
//
// track_message_history_changes() is reinstalled from the shared schema package. It now
// writes an email.link_clicked entry, with the URL, each time a click records a link of
// clicked_links through a verified token for the first time, so an automation can start on
// a click of a given link and a segment filtering on a clicked link is recomputed at once.
// See schema.TrackMessageHistoryChangesFunction. Replacing a function body takes no table
// lock, and message_history_changes_trigger stays attached. Clicks recorded before the
// upgrade are not replayed: inserting their entries here would start automations for old
// clicks, inside one transaction.
//
// Three partial indexes then hold the failed, bounced and complained messages in
// messages.list order (created_at DESC, id DESC), so the latest few are read without walking
// the whole table when none matches. The builds are plain CREATE INDEX (the migration runs in
// a transaction) and hold a SHARE lock on message_history until commit: writes to it wait,
// about a second for 2 million rows in memory, longer once the table no longer fits. A rerun
// takes the lock too, even when the indexes exist. statement_timeout is lifted for the
// builds, since a build cut short fails the migration again at every start (see v42).
// lock_timeout bounds the wait for the lock behind a transaction already writing to the
// table, every later write queuing behind that wait: past 5 seconds the migration fails and
// is retried at the next start.
//
// Before the builds, so outside their lock, the broadcasts that started sending without a
// start date get the creation time of their first message as one (v44StartedAtBackfillSQL).
// Until v44 only "send now" and a resume set started_at, so a scheduled broadcast never had
// it; a sending run now records it. Drafts and scheduled broadcasts are left alone even when
// they have messages: those are test sends, or a scheduled send still in progress, which its
// next sending run dates. Only a NULL is filled, so a rerun writes nothing, and a broadcast
// without any message keeps no date. The first messages are found in one grouped pass over
// message_history, not one walk per broadcast.
//
// After the builds, under their SHARE lock and before contact_segments is locked (so that
// membership writes do not wait for it), the links of clicked_links on the instance's own
// hosts are removed (v44OwnLinksPurgeSQL): those of the API endpoint and of the workspace's
// current custom endpoint, on any port, in any case, with or without one trailing dot, as
// the click recorder leaves them out since 44. Before, behind a reverse proxy, unsubscribe
// and notification center links were recorded with the contact's address. The lock is held
// already, so no other transaction writing message_history holds a row lock on it, and as
// no statement takes SELECT ... FOR UPDATE or FOR SHARE on message_history, the purge cannot
// deadlock with another transaction; it writes no timeline entry, so it does not touch automations. It
// reads every row holding clicked links while writes to message_history wait, so sending and
// tracking wait for it too. clicked_at, the other keys and updated_at stay. A rerun matches
// no row. Without any readable own host the statement is skipped.
//
// After the builds and the purge, stats.exited of each automation is raised to the number of its journeys
// that exited (v44ExitedRecountSQL). Until 44 a journey ended by a reply exited without being
// counted there, so the counter fell short of the journeys it describes. The recount comes
// after the builds and the purge because it row-locks automations: a tracking write locks message_history first and
// then, through the timeline trigger, updates automations, so taking automations before the
// builds' SHARE lock could deadlock with it. This order holds only while no transaction
// updates automations and then writes message_history: one that did could deadlock with the
// recount. The recount also waits for a row lock while it holds the SHARE lock, so a
// transaction holding an automations row delays writes to message_history too, up to the
// same 5-second lock_timeout for each row it waits on. A stored whole number below 10^18 is
// only ever raised; any other value counts as 0. An automation whose stats are not a JSON
// object, or a deleted one, is left as it is. A rerun writes nothing.
//
// Between the builds and the recount, the segment memberships whose contact no longer exists
// are deleted (v44OrphanMembershipsSQL), with the two membership triggers of contact_segments
// disabled around the delete. Until 38.0 deleting a contact left its memberships in place.
//
// Since then a deletion failing between the contact row and its memberships still leaves
// some, as does a segment build or queue recomputation that matches a contact and adds its
// membership just after a deletion removed it. contact_segments has no foreign key to
// contacts, so these rows stay until a rebuild at a higher version, a recomputation of the
// address by the segment queue or the deletion of the segment removes them, each delete
// writing a segment.left timeline entry and webhook delivery with the erased address. A
// daily rebuild of relative-date segments at a new version would do this for every such
// segment at once.
//
// The disabled state is a catalog change seen by no other session before commit, and
// ALTER TABLE ... DISABLE TRIGGER holds a SHARE ROW EXCLUSIVE lock on contact_segments until
// commit: it lets reads through and makes every membership write wait, so none runs without
// its triggers; a rollback restores them. The lock is taken after the builds so membership
// writes (segment builds, the segment queue, contact deletions) do not wait for their
// duration, and before the recount because a membership write locks contact_segments and
// then, through the timeline trigger, automations: taking contact_segments while holding
// automations rows could deadlock with it. No membership write touches message_history or
// broadcasts, so waiting for contact_segments while holding the builds' SHARE lock and the
// backfill's locks forms no cycle; the wait is bounded by the same lock_timeout. Writes to
// message_history, already waiting behind the builds' SHARE lock, also wait for the delete.
// Enabling a trigger sets it to fire normally, whatever its state before the migration. A
// rerun takes the lock again and deletes only the memberships orphaned since the previous
// run, without events either.
//
// The system update reads the send_broadcast tasks that completed with no phase in their state
// (v44FinishedSendsSQL), the mark of a scheduled A/B broadcast sent before 44: the run chose no
// phase for a scheduled broadcast, sent every variation to the whole audience, completed its task
// and left the broadcast scheduled. A broadcast has one task (idx_tasks_workspace_broadcast_id).
// tasks.completed_at is a TIMESTAMP holding UTC, written out with its Z. The rows are kept on the
// migration for the workspace updates that follow in the same run, replaced at each run. In each
// workspace, before the start dates are backfilled (so that they are dated too), those still
// scheduled become processed, completed when their task did (v44RepairSQL); a rerun finds none.
// A scheduled broadcast whose task is still to run, or that only had test sends, has no such task.
//
// After the recount, the five-argument automation_enroll_contact with a VARCHAR p_list_id is
// dropped (v44LegacyEnrolDropSQL). v20 and v21 created it; v25 meant to replace it with the
// four-argument signature but CREATE OR REPLACE with other arguments adds an overload, so every
// workspace created before v25 kept it. No trigger body calls it: pausing or deleting an
// automation has dropped its trigger since automations exist, v25 rewrote every live one to the
// four-argument call in one statement that failed the migration on any error, and the bodies
// are replaced again below, after the drop. Beside the JSONB overload installed
// next, a call whose fifth argument has no type, such as a bare NULL or a quoted literal, would
// resolve to it without an error (PostgreSQL prefers the string type for an unknown argument),
// reading the frequency as a list ID. A rerun, or a workspace created after v25, finds nothing
// to drop.
//
// Then an automation's emails get the event that started the journey, as {{ trigger_event }}.
// The five-argument automation_enroll_contact stores it as contact_automations.context.trigger
// (the column exists since v20 and nothing wrote it), and the four-argument signature becomes a
// relay to it, for a trigger left with its old body. Then the function of every live
// automation's trigger is replaced by the current generator's, which builds the event from the
// contact_timeline row and calls the five-argument signature. Bodies only: the triggers and their
// WHEN clauses stay, so no lock is taken on contact_timeline while this transaction holds
// message_history and automations rows, and no trigger can be broadened. One savepoint per
// automation: one whose condition no longer resolves keeps its old body and enrols without the
// event. Journeys already running have no event. A rerun rewrites the same bodies.
//
// The same two steps fix three enrolment defects, with no statement of their own. The new
// enrolment enrols a 'once' automation only when its insert added the automation_trigger_log
// row, so two transactions enrolling one contact at once start one journey; and it dates a
// journey with clock_timestamp(), so two enrolments of one contact in one transaction (two
// events of one name in a batch) start two journeys instead of failing on
// UNIQUE(automation_id, contact_email, entered_at). The replaced body of a custom event trigger
// enrols only for a timeline row whose operation is 'insert': an update, soft delete or restore
// of the event no longer starts the automation again. A trigger whose body is not replaced (see
// above) keeps enrolling on those rows until the automation is paused and activated again; the
// other two fixes reach it through the four-argument relay.
//
// The replaced bodies also read a value inside a custom JSON column as the text it holds, not as
// its JSON text, which kept a string's quotes: a text « equals » on such a value now matches, and
// a JSON date now converts in an entry condition. Their defensive casts test the conversion as
// well as the shape, so a value such as 2026-02-30 gives no match instead of aborting the write.
//
// Then every segment's tree is recompiled with the current query builder, and a segment whose
// stored generated_sql / generated_args differ gets the new ones and recompute_after = NOW(), so
// the recompute check rebuilds it (RegenerateSegmentSQL). A segment's query is compiled when its
// tree is saved and read from the row from then on, so the change above reaches no segment by
// itself; a changed query can change membership, and only a rebuild removes the contacts that
// leave. Each rebuild writes segment.joined and segment.left entries for the contacts that move.
// The updates take row locks on segments, held until the workspace commits, under the same
// lock_timeout; a segment build waiting for one waits that long. A tree that no longer compiles
// keeps its query. A rerun finds no difference and writes nothing.
//
// Right after the recompilation, segments gets last_error and last_error_at
// (schema.SegmentLastErrorColumn, schema.SegmentLastErrorAtColumn): the last failure of a
// segment's computation, such as a contact value its query could not convert, and when it was
// recorded, so that a failure nobody watched is shown on the segment. Both are NULL, so the
// columns are added without rewriting the table, but the statement takes an ACCESS EXCLUSIVE lock
// on segments until the workspace commits: the segment builds, the contact queue and the segment
// pages of another instance wait for the commit. It comes after the recompilation, which reads
// and updates segments under row locks only, and waits for its lock under the same 5-second
// lock_timeout. IF NOT EXISTS makes a rerun add nothing.
//
// Last, after the trigger bodies and the segments, broadcasts gets test_paused_seconds, the time an A/B test
// waiting for its winner spent paused (schema.BroadcastTestPausedSecondsColumn). With a constant
// default the column is added without rewriting the table, but it takes an ACCESS EXCLUSIVE lock
// on broadcasts until the workspace commits, upgrading the row locks the repair and the backfill
// hold: reads of broadcasts by another instance wait for the commit, so the statement is the last
// one. It waits for the lock under the same 5-second lock_timeout while this transaction holds the
// builds' SHARE lock on message_history, contact_segments and automations rows. Pause, cancel,
// resume and a sending run write email_queue and broadcasts, not message_history, under those,
// so no cycle forms with them. When a transaction holding a lock on broadcasts waits for a row
// the repair or the backfill locked, PostgreSQL's deadlock detection aborts one of the two; if it
// is this one, the migration runs again at the next start.
//
// No column is renamed or dropped, so no trigger function needs regenerating for a schema change.
type V44Migration struct {
	// finishedSends are the send tasks completed with no phase, by workspace, read by
	// UpdateSystem for the UpdateWorkspace calls of the same run.
	finishedSends map[string][]v44FinishedSend
}

// v44FinishedSend is a broadcast whose send task completed with no phase, and when, as an
// RFC 3339 UTC timestamp.
type v44FinishedSend struct {
	broadcastID string
	completedAt string
}

// v44SegmentLastErrorSQL adds the columns keeping a segment's last failure.
var v44SegmentLastErrorSQL = `ALTER TABLE segments ADD COLUMN IF NOT EXISTS ` + schema.SegmentLastErrorColumn +
	`, ADD COLUMN IF NOT EXISTS ` + schema.SegmentLastErrorAtColumn

// v44FinishedSendsSQL lists the send tasks that completed with no phase.
const v44FinishedSendsSQL = `SELECT workspace_id, broadcast_id,
       to_char(COALESCE(completed_at, updated_at), 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
FROM tasks
WHERE type = 'send_broadcast' AND status = 'completed' AND broadcast_id IS NOT NULL
  AND COALESCE(state->'send_broadcast'->>'phase', '') = ''`

// v44LegacyEnrolDropSQL drops the enrolment v20 and v21 created with a VARCHAR p_list_id.
const v44LegacyEnrolDropSQL = `DROP FUNCTION IF EXISTS automation_enroll_contact(VARCHAR, VARCHAR, VARCHAR, VARCHAR, VARCHAR)`

// v44RepairSQL marks the broadcasts of $1 still scheduled as processed, completed at $2.
const v44RepairSQL = `UPDATE broadcasts b
SET status = 'processed', completed_at = COALESCE(b.completed_at, d.done::timestamptz, NOW()), updated_at = NOW()
FROM unnest($1::text[], $2::text[]) AS d(id, done)
WHERE b.id = d.id AND b.status = 'scheduled'`

// v44StartedAtBackfillSQL dates each broadcast that sent without a start date from its first
// message.
const v44StartedAtBackfillSQL = `UPDATE broadcasts b SET started_at = m.first_at
FROM (SELECT mh.broadcast_id, MIN(mh.created_at) AS first_at
      FROM message_history mh
      WHERE mh.broadcast_id IN (SELECT id FROM broadcasts
                                WHERE started_at IS NULL AND status NOT IN ('draft', 'scheduled'))
      GROUP BY mh.broadcast_id) m
WHERE b.id = m.broadcast_id AND b.started_at IS NULL AND b.status NOT IN ('draft', 'scheduled')`

// v44OrphanMembershipsSQL removes the segment memberships whose contact no longer exists.
const v44OrphanMembershipsSQL = `DELETE FROM contact_segments cs
WHERE NOT EXISTS (SELECT 1 FROM contacts c WHERE c.email = cs.email)`

// v44KeyHostSQL is the hostname of a clicked_links key k: lowercase, without userinfo, port,
// IPv6 brackets or one trailing dot. NULL when k is not an http(s) URL with a host. Its
// patterns hold no backslash, so they read the same whether standard_conforming_strings is
// on or off.
const v44KeyHostSQL = `NULLIF(regexp_replace(btrim(lower(substring(%s from '(?i)^[[:space:]]*https?://(?:[^/?#]*@)?([[][^]]*[]]|[^/?#:]*)')), '[]'), '[.]$', ''), '')`

// v44OwnLinksPurgeSQL removes from clicked_links the keys whose host is one of $1. A key
// whose host cannot be read is kept.
var v44OwnLinksPurgeSQL = `UPDATE message_history mh
SET clicked_links = (SELECT COALESCE(jsonb_object_agg(e.key, e.value), '{}'::jsonb)
                     FROM jsonb_each(mh.clicked_links) e
                     WHERE NOT COALESCE(` + fmt.Sprintf(v44KeyHostSQL, "e.key") + ` = ANY($1), false))
WHERE CASE WHEN jsonb_typeof(mh.clicked_links) = 'object'
           THEN EXISTS (SELECT 1 FROM jsonb_object_keys(mh.clicked_links) k(key)
                        WHERE ` + fmt.Sprintf(v44KeyHostSQL, "k.key") + ` = ANY($1))
           ELSE false END`

// v44OwnHosts returns the hostnames v44OwnLinksPurgeSQL removes: for the API endpoint and
// the workspace's current custom endpoint, the host as trackingtoken.HostOf writes it and as
// it is written, lowercase. The two differ for an IPv6 literal not written canonical, and
// for a hostname written in Unicode (Punycode, and as written). An
// endpoint whose host cannot be read gives none.
func v44OwnHosts(cfg *config.Config, workspace *domain.Workspace) []string {
	endpoints := []string{cfg.APIEndpoint}
	if workspace.Settings.CustomEndpointURL != nil {
		endpoints = append(endpoints, *workspace.Settings.CustomEndpointURL)
	}
	var hosts []string
	seen := map[string]bool{}
	add := func(host string) {
		host = strings.TrimSuffix(host, ".")
		if host != "" && !seen[host] {
			seen[host] = true
			hosts = append(hosts, host)
		}
	}
	for _, endpoint := range endpoints {
		canonical := trackingtoken.HostOf(endpoint)
		if canonical == "" {
			continue
		}
		add(canonical)
		raw := strings.TrimSpace(endpoint)
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		if u, err := url.Parse(raw); err == nil {
			add(strings.ToLower(u.Hostname()))
		}
	}
	return hosts
}

// v44ExitedRecountSQL raises stats.exited to the number of exited journeys of the automation.
// A stored value that is not a whole number below 10^18 counts as 0, so it is replaced.
const v44ExitedRecountSQL = `UPDATE automations a
SET stats = jsonb_set(COALESCE(a.stats, '{}'::jsonb), '{exited}', to_jsonb(c.n)),
    updated_at = NOW()
FROM (SELECT automation_id, COUNT(*) AS n FROM contact_automations
      WHERE status = 'exited' GROUP BY automation_id) c
WHERE c.automation_id = a.id
  AND a.deleted_at IS NULL
  AND (a.stats IS NULL OR jsonb_typeof(a.stats) = 'object')
  AND c.n > CASE WHEN a.stats->>'exited' ~ '^[0-9]{1,18}$'
                 THEN (a.stats->>'exited')::bigint ELSE 0 END`

func (m *V44Migration) GetMajorVersion() float64 { return 44.0 }

func (m *V44Migration) HasSystemUpdate() bool { return true }

func (m *V44Migration) HasWorkspaceUpdate() bool { return true }

func (m *V44Migration) ShouldRestartServer() bool { return false }

func (m *V44Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	rows, err := db.QueryContext(ctx, v44FinishedSendsSQL)
	if err != nil {
		return fmt.Errorf("v44: failed to read the finished broadcast sends: %w", err)
	}
	defer func() { _ = rows.Close() }()
	finished := map[string][]v44FinishedSend{}
	for rows.Next() {
		var workspaceID string
		var send v44FinishedSend
		if err := rows.Scan(&workspaceID, &send.broadcastID, &send.completedAt); err != nil {
			return fmt.Errorf("v44: failed to read a finished broadcast send: %w", err)
		}
		finished[workspaceID] = append(finished[workspaceID], send)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("v44: failed to read the finished broadcast sends: %w", err)
	}
	m.finishedSends = finished
	return nil
}

// v44WorkspaceStatement is one step of the workspace update, with what its error says.
type v44WorkspaceStatement struct {
	sql  string
	args []interface{}
	step string
}

// v44WorkspaceStatements are the workspace update's statements, run in this order. The purge
// of the own links is left out when there is no own host to remove.
func v44WorkspaceStatements(ownHosts []string, finished []v44FinishedSend) []v44WorkspaceStatement {
	purge := []v44WorkspaceStatement{}
	if len(ownHosts) > 0 {
		purge = append(purge, v44WorkspaceStatement{v44OwnLinksPurgeSQL, []interface{}{pq.Array(ownHosts)}, "remove the instance's own links from clicked_links"})
	}
	statements := []v44WorkspaceStatement{
		{schema.TrackMessageHistoryChangesFunction(), nil, "install the link click timeline step"},
		{`SET LOCAL statement_timeout = 0`, nil, "lift the statement timeout"},
		{`SET LOCAL lock_timeout = '5s'`, nil, "set the lock timeout"},
	}
	if len(finished) > 0 {
		ids, done := make([]string, len(finished)), make([]string, len(finished))
		for i, send := range finished {
			ids[i], done[i] = send.broadcastID, send.completedAt
		}
		statements = append(statements, v44WorkspaceStatement{v44RepairSQL, []interface{}{pq.Array(ids), pq.Array(done)},
			"mark the scheduled broadcasts whose send finished as processed"})
	}
	statements = append(statements, []v44WorkspaceStatement{
		{v44StartedAtBackfillSQL, nil, "date the broadcasts that started without a start date"},
		{`CREATE INDEX IF NOT EXISTS idx_message_history_failed_recent ON message_history(created_at DESC, id DESC) WHERE failed_at IS NOT NULL`, nil, "create idx_message_history_failed_recent"},
		{`CREATE INDEX IF NOT EXISTS idx_message_history_bounced_recent ON message_history(created_at DESC, id DESC) WHERE bounced_at IS NOT NULL`, nil, "create idx_message_history_bounced_recent"},
		{`CREATE INDEX IF NOT EXISTS idx_message_history_complained_recent ON message_history(created_at DESC, id DESC) WHERE complained_at IS NOT NULL`, nil, "create idx_message_history_complained_recent"},
	}...)
	statements = append(statements, purge...)
	statements = append(statements, []v44WorkspaceStatement{
		{`ALTER TABLE contact_segments DISABLE TRIGGER contact_segment_changes_trigger`, nil, "disable contact_segment_changes_trigger"},
		{`ALTER TABLE contact_segments DISABLE TRIGGER webhook_contact_segments`, nil, "disable webhook_contact_segments"},
		{v44OrphanMembershipsSQL, nil, "remove the segment memberships of deleted contacts"},
		{`ALTER TABLE contact_segments ENABLE TRIGGER webhook_contact_segments`, nil, "enable webhook_contact_segments"},
		{`ALTER TABLE contact_segments ENABLE TRIGGER contact_segment_changes_trigger`, nil, "enable contact_segment_changes_trigger"},
		{v44ExitedRecountSQL, nil, "recount the exited journeys of each automation"},
		{v44LegacyEnrolDropSQL, nil, "drop the enrol function with p_list_id"},
		// The enrolment that keeps the event that started a journey, then the four-argument
		// signature as a relay to it (see the type comment).
		{schema.AutomationEnrollContactWithTriggerFunction(), nil, "install the automation enrol function"},
		{schema.AutomationEnrollContactRelayFunction(), nil, "install the automation enrol relay"},
	}...)
	return statements
}

func (m *V44Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	for _, stmt := range v44WorkspaceStatements(v44OwnHosts(cfg, workspace), m.finishedSends[workspace.ID]) {
		if _, err := db.ExecContext(ctx, stmt.sql, stmt.args...); err != nil {
			return fmt.Errorf("v44: failed to %s for workspace %s: %w", stmt.step, workspace.ID, err)
		}
	}
	// Then, once the five-argument enrolment exists: every live automation's trigger
	// function is replaced by one that passes the event, bodies only (see the type comment).
	generator := service.NewAutomationTriggerGenerator(service.NewQueryBuilder())
	if _, err := regenerateAutomationTriggers(ctx, db, generator, "v44", "v44_automation_trigger_bodies", true); err != nil {
		return fmt.Errorf("v44: failed to pass the trigger event in automation triggers for workspace %s: %w", workspace.ID, err)
	}
	// Then the segments whose stored query the same change alters (see the type comment).
	if _, err := RegenerateSegmentSQL(ctx, db); err != nil {
		return fmt.Errorf("v44: failed to recompile the segment queries for workspace %s: %w", workspace.ID, err)
	}
	// Then the columns keeping a segment's last failure (see the type comment).
	if _, err := db.ExecContext(ctx, v44SegmentLastErrorSQL); err != nil {
		return fmt.Errorf("v44: failed to add segments.last_error and last_error_at for workspace %s: %w", workspace.ID, err)
	}
	// Last: its lock on broadcasts is held until the workspace commits (see the type comment).
	if _, err := db.ExecContext(ctx, `ALTER TABLE broadcasts ADD COLUMN IF NOT EXISTS `+schema.BroadcastTestPausedSecondsColumn); err != nil {
		return fmt.Errorf("v44: failed to add broadcasts.test_paused_seconds for workspace %s: %w", workspace.ID, err)
	}
	return nil
}

func init() {
	Register(&V44Migration{})
}
