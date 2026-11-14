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
	"github.com/Notifuse/notifuse/pkg/trackingtoken"
)

// V44Migration, on workspaces that already exist, makes a click on a link write its own
// timeline entry, adds the partial indexes of the dashboard's recent failures, dates the
// broadcasts that started without a start date, removes the segment memberships of deleted
// contacts and recounts the exited journeys of each automation.
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
// After the builds, under their SHARE lock, the links of clicked_links on the instance's own
// hosts are removed (v44OwnLinksPurgeSQL): those of the API endpoint and of the workspace's
// current custom endpoint, on any port, in any case, with or without one trailing dot, as
// the click recorder leaves them out since 44. Before, behind a reverse proxy, unsubscribe
// and notification center links were recorded with the contact's address. The lock is held
// already, so no other transaction holds a row lock on message_history and the purge cannot
// deadlock with a writer; it writes no timeline entry, so it does not touch automations. It
// reads every row holding clicked links while writes to message_history wait, so sending and
// tracking wait for it too. clicked_at, the other keys and updated_at stay. A rerun matches
// no row. Without any readable own host the statement is skipped.
//
// After the builds and the purge, stats.exited of each automation is raised to the number of its journeys
// that exited (v44ExitedRecountSQL). Until 44 a journey ended by a reply exited without being
// counted there, so the counter fell short of the journeys it describes. The recount comes
// last because it row-locks automations: a tracking write locks message_history first and
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
// No column is renamed or dropped, so no automation trigger function needs regenerating.
type V44Migration struct{}

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
// IPv6 brackets or one trailing dot. NULL when k is not an http(s) URL with a host.
const v44KeyHostSQL = `NULLIF(regexp_replace(btrim(lower(substring(%s from '(?i)^\s*https?://(?:[^/?#]*@)?(\[[^]]*\]|[^/?#:]*)')), '[]'), '\.$', ''), '')`

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
// it is written, lowercase. The two differ for an IPv6 literal not written canonical. An
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

func (m *V44Migration) HasSystemUpdate() bool { return false }

func (m *V44Migration) HasWorkspaceUpdate() bool { return true }

func (m *V44Migration) ShouldRestartServer() bool { return false }

func (m *V44Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
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
func v44WorkspaceStatements(ownHosts []string) []v44WorkspaceStatement {
	purge := []v44WorkspaceStatement{}
	if len(ownHosts) > 0 {
		purge = append(purge, v44WorkspaceStatement{v44OwnLinksPurgeSQL, []interface{}{pq.Array(ownHosts)}, "remove the instance's own links from clicked_links"})
	}
	statements := []v44WorkspaceStatement{
		{schema.TrackMessageHistoryChangesFunction(), nil, "install the link click timeline step"},
		{`SET LOCAL statement_timeout = 0`, nil, "lift the statement timeout"},
		{`SET LOCAL lock_timeout = '5s'`, nil, "set the lock timeout"},
		{v44StartedAtBackfillSQL, nil, "date the broadcasts that started without a start date"},
		{`CREATE INDEX IF NOT EXISTS idx_message_history_failed_recent ON message_history(created_at DESC, id DESC) WHERE failed_at IS NOT NULL`, nil, "create idx_message_history_failed_recent"},
		{`CREATE INDEX IF NOT EXISTS idx_message_history_bounced_recent ON message_history(created_at DESC, id DESC) WHERE bounced_at IS NOT NULL`, nil, "create idx_message_history_bounced_recent"},
		{`CREATE INDEX IF NOT EXISTS idx_message_history_complained_recent ON message_history(created_at DESC, id DESC) WHERE complained_at IS NOT NULL`, nil, "create idx_message_history_complained_recent"},
		{`ALTER TABLE contact_segments DISABLE TRIGGER contact_segment_changes_trigger`, nil, "disable contact_segment_changes_trigger"},
		{`ALTER TABLE contact_segments DISABLE TRIGGER webhook_contact_segments`, nil, "disable webhook_contact_segments"},
		{v44OrphanMembershipsSQL, nil, "remove the segment memberships of deleted contacts"},
		{`ALTER TABLE contact_segments ENABLE TRIGGER webhook_contact_segments`, nil, "enable webhook_contact_segments"},
		{`ALTER TABLE contact_segments ENABLE TRIGGER contact_segment_changes_trigger`, nil, "enable contact_segment_changes_trigger"},
	}
	statements = append(statements, purge...)
	return append(statements, v44WorkspaceStatement{v44ExitedRecountSQL, nil, "recount the exited journeys of each automation"})
}

func (m *V44Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	for _, stmt := range v44WorkspaceStatements(v44OwnHosts(cfg, workspace)) {
		if _, err := db.ExecContext(ctx, stmt.sql, stmt.args...); err != nil {
			return fmt.Errorf("v44: failed to %s for workspace %s: %w", stmt.step, workspace.ID, err)
		}
	}
	return nil
}

func init() {
	Register(&V44Migration{})
}
