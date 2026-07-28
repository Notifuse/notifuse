package migrations

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
)

// V36Migration uniformizes the message_history contact_timeline kinds to dot notation.
//
// Historically track_message_history_changes() wrote the channel-suffixed / generic forms
// ("open_email", "click_email", "bounce_email", "complain_email", "unsubscribe_email",
// "insert_message_history", "update_message_history") while the rest of the timeline
// (contact.*, list.*, segment.*, custom_event.*, automation.*) already used dot notation.
// That mismatch is why automation email.* triggers never fired and why the trigger generator
// had to translate "email.clicked" -> "click_email" before matching. This migration flips the
// message_history events to the same dot vocabulary the console/webhooks already use:
//
//	open_email             -> email.opened
//	click_email            -> email.clicked
//	bounce_email           -> email.bounced
//	complain_email         -> email.complained
//	unsubscribe_email      -> email.unsubscribed
//	insert_message_history -> email.sent
//	update_message_history -> email.updated
//
// It runs inside the per-workspace migration transaction (manager.go) and performs, in order:
//
//  1. Recreate track_message_history_changes() so new rows are written in dot notation.
//  2. Rewrite existing contact_timeline rows carrying a legacy kind (one bulk UPDATE; the
//     heaviest step on large timelines). No trigger fires from it: both triggers on
//     contact_timeline are AFTER INSERT only, so an UPDATE of kind cannot cascade.
//  3. Rewrite stored segment definitions (segments.tree) that reference a renamed kind and
//     regenerate their compiled generated_sql/generated_args — segment recomputation runs the
//     compiled query, whose bound args embed the old literal, so the tree alone is not enough.
//  4. Rewrite timeline-kind literals embedded in every live automation's trigger_config and
//     node conditions — branch/filter nodes reuse the segment condition system, so they can
//     reference contact_timeline kinds and are evaluated at runtime via the query builder —
//     then regenerate the installed DB trigger of email.* automations so its WHEN clause
//     matches the renamed kind (emitted verbatim now that the translation map is gone).
//
// Every step is idempotent: the timeline UPDATE and the segment/automation rewrites only touch
// rows still carrying a legacy kind, so a re-run is a no-op.
//
// The system update heals the setup-wizard OIDC scopes bug: the wizard had no scopes
// field, so setup persisted ParseScopes("") → bare "openid", and that stored value
// overrode the "openid email profile" default at boot — authorize requests then lacked
// the email/profile scopes and IdPs returned tokens without an email claim. Only a
// value whose scope tokens reduce to exactly "openid" is rewritten (tolerating stray
// whitespace/separators the old settings screen persisted raw); a deliberately
// customized scope list is never touched. The migration requests a server restart
// ONLY when it actually rewrote the row: the resolved OIDC config bakes the scopes in
// during config.Load(), which runs BEFORE migrations, so without a restart a healed
// row would not take effect until the next manual reboot.
type V36Migration struct {
	// healedScopes records whether UpdateSystem rewrote the oidc_scopes row; the
	// registry holds one instance per process, and the manager consults
	// ShouldRestartServer only after the migration executed.
	healedScopes bool
}

func (m *V36Migration) GetMajorVersion() float64 { return 36.0 }

func (m *V36Migration) HasSystemUpdate() bool { return true }

func (m *V36Migration) HasWorkspaceUpdate() bool { return true }

func (m *V36Migration) ShouldRestartServer() bool { return m.healedScopes }

func (m *V36Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	// Literal scope string (not config.DefaultOIDCScopes) so the migration stays
	// frozen in time even if the runtime default later changes. The WHERE clause
	// collapses separators/whitespace so raw legacy values like "openid " or
	// ",openid" (persisted verbatim by the old settings screen) are healed too,
	// while any value containing another scope token is left alone.
	res, err := db.ExecContext(ctx,
		`UPDATE settings SET value = 'openid email profile', updated_at = CURRENT_TIMESTAMP
		 WHERE key = 'oidc_scopes' AND regexp_replace(value, '[,;[:space:]]+', '', 'g') = 'openid'`)
	if err != nil {
		return fmt.Errorf("v36: failed to heal oidc_scopes: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil {
		m.healedScopes = n > 0
	}
	return nil
}

// timelineKindRenames maps the legacy suffixed message_history timeline kinds to the
// dot-notation kinds now written by track_message_history_changes(). The engagement kinds
// already carried the channel in their suffix (email-only in practice); the two generic kinds
// were channel-less and become email.* .
var timelineKindRenames = map[string]string{
	"open_email":             "email.opened",
	"click_email":            "email.clicked",
	"bounce_email":           "email.bounced",
	"complain_email":         "email.complained",
	"unsubscribe_email":      "email.unsubscribed",
	"insert_message_history": "email.sent",
	"update_message_history": "email.updated",
}

// updateTimelineKindsSQL rewrites existing contact_timeline rows. WHERE ... IN keeps the write
// scoped to legacy rows so the statement is idempotent and touches nothing on a re-run.
const updateTimelineKindsSQL = `
	UPDATE contact_timeline SET kind = CASE kind
		WHEN 'open_email'             THEN 'email.opened'
		WHEN 'click_email'            THEN 'email.clicked'
		WHEN 'bounce_email'           THEN 'email.bounced'
		WHEN 'complain_email'         THEN 'email.complained'
		WHEN 'unsubscribe_email'      THEN 'email.unsubscribed'
		WHEN 'insert_message_history' THEN 'email.sent'
		WHEN 'update_message_history' THEN 'email.updated'
		ELSE kind
	END
	WHERE kind IN (
		'open_email', 'click_email', 'bounce_email', 'complain_email',
		'unsubscribe_email', 'insert_message_history', 'update_message_history'
	)`

// dottedTrackMessageHistoryChangesFn is the dot-notation writer trigger function. Keep it in
// sync with track_message_history_changes() in internal/database/init.go (the statements must
// match; indentation/whitespace need not) so a migrated database and a fresh install behave
// identically.
const dottedTrackMessageHistoryChangesFn = `CREATE OR REPLACE FUNCTION track_message_history_changes()
	RETURNS TRIGGER AS $$
	DECLARE
		changes_json JSONB := '{}'::jsonb;
		op VARCHAR(20);
		kind_value VARCHAR(50);
	BEGIN
		IF TG_OP = 'INSERT' THEN
			op := 'insert';
			changes_json := jsonb_build_object('template_id', jsonb_build_object('new', NEW.template_id), 'template_version', jsonb_build_object('new', NEW.template_version), 'channel', jsonb_build_object('new', NEW.channel), 'broadcast_id', jsonb_build_object('new', NEW.broadcast_id), 'sent_at', jsonb_build_object('new', NEW.sent_at));
			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
			VALUES (NEW.contact_email, op, 'message_history', NEW.channel || '.sent', NEW.id, changes_json, NEW.updated_at);
		ELSIF TG_OP = 'UPDATE' THEN
			op := 'update';
			-- Handle engagement events separately with specific kinds
			IF OLD.opened_at IS DISTINCT FROM NEW.opened_at AND NEW.opened_at IS NOT NULL THEN
				changes_json := jsonb_build_object('opened_at', jsonb_build_object('old', OLD.opened_at, 'new', NEW.opened_at));
				INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
				VALUES (NEW.contact_email, op, 'message_history', NEW.channel || '.opened', NEW.id, changes_json, NEW.updated_at);
			END IF;
			IF OLD.clicked_at IS DISTINCT FROM NEW.clicked_at AND NEW.clicked_at IS NOT NULL THEN
				changes_json := jsonb_build_object('clicked_at', jsonb_build_object('old', OLD.clicked_at, 'new', NEW.clicked_at));
				INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
				VALUES (NEW.contact_email, op, 'message_history', NEW.channel || '.clicked', NEW.id, changes_json, NEW.updated_at);
			END IF;
			IF OLD.bounced_at IS DISTINCT FROM NEW.bounced_at AND NEW.bounced_at IS NOT NULL THEN
				changes_json := jsonb_build_object('bounced_at', jsonb_build_object('old', OLD.bounced_at, 'new', NEW.bounced_at));
				INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
				VALUES (NEW.contact_email, op, 'message_history', NEW.channel || '.bounced', NEW.id, changes_json, NEW.updated_at);
			END IF;
			IF OLD.complained_at IS DISTINCT FROM NEW.complained_at AND NEW.complained_at IS NOT NULL THEN
				changes_json := jsonb_build_object('complained_at', jsonb_build_object('old', OLD.complained_at, 'new', NEW.complained_at));
				INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
				VALUES (NEW.contact_email, op, 'message_history', NEW.channel || '.complained', NEW.id, changes_json, NEW.updated_at);
			END IF;
			IF OLD.unsubscribed_at IS DISTINCT FROM NEW.unsubscribed_at AND NEW.unsubscribed_at IS NOT NULL THEN
				changes_json := jsonb_build_object('unsubscribed_at', jsonb_build_object('old', OLD.unsubscribed_at, 'new', NEW.unsubscribed_at));
				INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
				VALUES (NEW.contact_email, op, 'message_history', NEW.channel || '.unsubscribed', NEW.id, changes_json, NEW.updated_at);
			END IF;
			-- Handle other updates (delivered, failed, status_info) as generic updates
			changes_json := '{}'::jsonb;
			IF OLD.delivered_at IS DISTINCT FROM NEW.delivered_at THEN changes_json := changes_json || jsonb_build_object('delivered_at', jsonb_build_object('old', OLD.delivered_at, 'new', NEW.delivered_at)); END IF;
			IF OLD.failed_at IS DISTINCT FROM NEW.failed_at THEN changes_json := changes_json || jsonb_build_object('failed_at', jsonb_build_object('old', OLD.failed_at, 'new', NEW.failed_at)); END IF;
			IF OLD.status_info IS DISTINCT FROM NEW.status_info THEN changes_json := changes_json || jsonb_build_object('status_info', jsonb_build_object('old', OLD.status_info, 'new', NEW.status_info)); END IF;
			IF changes_json != '{}'::jsonb THEN
				INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
				VALUES (NEW.contact_email, op, 'message_history', NEW.channel || '.updated', NEW.id, changes_json, NEW.updated_at);
			END IF;
		END IF;
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql;`

func (m *V36Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Disable statement_timeout for this migration transaction. The bulk contact_timeline
	// rewrite below can run long on a large workspace; without this, a globally-configured
	// statement_timeout would abort it, roll back the workspace migration, and — since the
	// version is never bumped — make every subsequent restart re-attempt and fail the same
	// way, bricking startup. SET LOCAL is scoped to this transaction only.
	if _, err := db.ExecContext(ctx, "SET LOCAL statement_timeout = 0"); err != nil {
		return fmt.Errorf("v36: failed to disable statement_timeout: %w", err)
	}

	// Step 1: recreate the writer trigger function so new rows use dot notation.
	if _, err := db.ExecContext(ctx, dottedTrackMessageHistoryChangesFn); err != nil {
		return fmt.Errorf("v36: failed to recreate track_message_history_changes: %w", err)
	}

	// Step 2: rewrite existing timeline rows from the legacy suffixed kinds. No trigger fires
	// from this UPDATE: every trigger on contact_timeline (the segment-recompute queue trigger
	// and the per-automation triggers) is AFTER INSERT, so it cannot cascade on an UPDATE.
	if _, err := db.ExecContext(ctx, updateTimelineKindsSQL); err != nil {
		return fmt.Errorf("v36: failed to migrate contact_timeline kinds: %w", err)
	}

	// Step 3: rewrite stored segment definitions that reference a renamed kind.
	if err := m.migrateSegmentTrees(ctx, db); err != nil {
		return err
	}

	// Step 4: rewrite embedded timeline kinds in automations + regenerate email.* triggers.
	if err := m.migrateAutomations(ctx, db); err != nil {
		return err
	}

	return nil
}

// renameTimelineKindsInTree rewrites every contact_timeline leaf kind in the tree using
// timelineKindRenames, returning true if any kind changed.
func renameTimelineKindsInTree(node *domain.TreeNode) bool {
	if node == nil {
		return false
	}
	changed := false
	if node.Leaf != nil && node.Leaf.ContactTimeline != nil {
		if newKind, ok := timelineKindRenames[node.Leaf.ContactTimeline.Kind]; ok {
			node.Leaf.ContactTimeline.Kind = newKind
			changed = true
		}
	}
	if node.Branch != nil {
		for _, child := range node.Branch.Leaves {
			if renameTimelineKindsInTree(child) {
				changed = true
			}
		}
	}
	return changed
}

// migrateSegmentTrees rewrites segments whose tree references a renamed timeline kind, then
// regenerates their compiled generated_sql/generated_args so recomputation matches the new
// kind. The result set is fully read and closed before issuing updates, because the workspace
// migration shares a single connection.
func (m *V36Migration) migrateSegmentTrees(ctx context.Context, db DBExecutor) error {
	rows, err := db.QueryContext(ctx, `SELECT id, tree FROM segments`)
	if err != nil {
		return fmt.Errorf("v36: failed to query segments: %w", err)
	}

	type segmentUpdate struct {
		id   string
		tree domain.TreeNode
	}
	var updates []segmentUpdate
	for rows.Next() {
		var id string
		var treeJSON []byte
		if scanErr := rows.Scan(&id, &treeJSON); scanErr != nil {
			_ = rows.Close()
			return fmt.Errorf("v36: failed to scan segment: %w", scanErr)
		}
		var tree domain.TreeNode
		// A malformed tree can't be rewritten; skip it rather than abort the whole migration
		// (which would block server startup).
		if json.Unmarshal(treeJSON, &tree) != nil {
			continue
		}
		if renameTimelineKindsInTree(&tree) {
			updates = append(updates, segmentUpdate{id: id, tree: tree})
		}
	}
	if iterErr := rows.Err(); iterErr != nil {
		_ = rows.Close()
		return fmt.Errorf("v36: error iterating segments: %w", iterErr)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return fmt.Errorf("v36: failed to close segments rows: %w", closeErr)
	}

	if len(updates) == 0 {
		return nil
	}

	qb := service.NewQueryBuilder()
	for _, u := range updates {
		treeJSON, err := json.Marshal(u.tree)
		if err != nil {
			continue
		}
		// Regenerate the compiled query: recomputation runs generated_sql with generated_args,
		// whose bound values embed the (now renamed) kind literal.
		sqlQuery, args, buildErr := qb.BuildSQL(&u.tree)
		if buildErr != nil {
			// Validation of the email-scoped kinds and link_url is parallel between the legacy
			// and dotted names, so a tree that was valid before the rename stays valid after
			// it. A BuildSQL failure here therefore means the tree was already invalid (would
			// have failed on the pre-migration name too), so skipping it strands nothing that
			// worked before. Skip rather than abort the whole migration.
			continue
		}
		argsJSON, err := json.Marshal(args)
		if err != nil {
			continue
		}
		if _, err := db.ExecContext(ctx, `
			UPDATE segments SET tree = $1, generated_sql = $2, generated_args = $3
			WHERE id = $4
		`, treeJSON, sqlQuery, argsJSON, u.id); err != nil {
			return fmt.Errorf("v36: failed to update segment %s: %w", u.id, err)
		}
	}
	return nil
}

// renameTimelineKindsInJSONValue walks a decoded JSON value and renames the "kind" of every
// contact_timeline condition object it finds (the leaf.contact_timeline.kind pattern that the
// segment/automation condition tree uses), returning true if any kind changed. Targeting the
// condition object specifically avoids touching unrelated string values (e.g. a filter that
// matches a custom field against the text "click_email").
func renameTimelineKindsInJSONValue(v interface{}) bool {
	changed := false
	switch node := v.(type) {
	case map[string]interface{}:
		if ctRaw, ok := node["contact_timeline"]; ok {
			if ct, ok := ctRaw.(map[string]interface{}); ok {
				if kind, ok := ct["kind"].(string); ok {
					if newKind, ok := timelineKindRenames[kind]; ok {
						ct["kind"] = newKind
						changed = true
					}
				}
			}
		}
		for _, val := range node {
			if renameTimelineKindsInJSONValue(val) {
				changed = true
			}
		}
	case []interface{}:
		for _, item := range node {
			if renameTimelineKindsInJSONValue(item) {
				changed = true
			}
		}
	}
	return changed
}

// rewriteTimelineKindsInJSON renames legacy contact_timeline kinds embedded anywhere in a JSON
// document (an automation's trigger_config or nodes), returning the re-marshaled bytes and
// whether anything changed. On empty input or a parse error it returns the input unchanged.
func rewriteTimelineKindsInJSON(raw []byte) ([]byte, bool) {
	if len(raw) == 0 {
		return raw, false
	}
	// UseNumber preserves integer precision (numbers do not become float64), and
	// SetEscapeHTML(false) keeps <, >, & literal, so re-marshaling a document only changes
	// the renamed kind — nothing else in the automation config is altered.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc interface{}
	if err := dec.Decode(&doc); err != nil {
		return raw, false
	}
	if !renameTimelineKindsInJSONValue(doc) {
		return raw, false
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return raw, false
	}
	// Encoder.Encode appends a trailing newline; strip it so the stored JSONB is clean.
	return bytes.TrimRight(buf.Bytes(), "\n"), true
}

// migrateAutomations rewrites timeline-kind literals embedded in every live automation's
// trigger_config and node conditions (branch/filter nodes reuse the segment condition system,
// so they can reference contact_timeline kinds and are evaluated at runtime via BuildSQL), then
// regenerates the installed DB trigger of email.* automations so its WHEN clause matches the
// renamed base kind. Every step is guarded so one bad config can never abort the migration and
// block server startup.
func (m *V36Migration) migrateAutomations(ctx context.Context, db DBExecutor) error {
	// Rewrite embedded kinds for EVERY non-deleted automation, not just live ones: a
	// paused/draft automation's branch/filter node condition (evaluated at runtime via
	// BuildSQL when it is later activated) would otherwise keep a legacy kind that no longer
	// exists in the (globally renamed) timeline and silently match zero rows. Only live
	// automations have an installed DB trigger to regenerate.
	rows, err := db.QueryContext(ctx, `
		SELECT id, status, root_node_id, trigger_config, nodes
		FROM automations
		WHERE deleted_at IS NULL
	`)
	if err != nil {
		return fmt.Errorf("v36: failed to query automations: %w", err)
	}

	type liveAutomation struct {
		id            string
		status        string
		rootNodeID    string
		triggerConfig []byte
		nodes         []byte
	}

	var automations []liveAutomation
	for rows.Next() {
		var a liveAutomation
		var rootNodeID sql.NullString
		if scanErr := rows.Scan(&a.id, &a.status, &rootNodeID, &a.triggerConfig, &a.nodes); scanErr != nil {
			_ = rows.Close()
			return fmt.Errorf("v36: failed to scan automation: %w", scanErr)
		}
		a.rootNodeID = rootNodeID.String
		automations = append(automations, a)
	}
	if iterErr := rows.Err(); iterErr != nil {
		_ = rows.Close()
		return fmt.Errorf("v36: error iterating automations: %w", iterErr)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return fmt.Errorf("v36: failed to close automations rows: %w", closeErr)
	}

	if len(automations) == 0 {
		return nil
	}

	generator := service.NewAutomationTriggerGenerator(service.NewQueryBuilder())
	for _, a := range automations {
		// Step 1: rewrite timeline kinds embedded anywhere in trigger_config and nodes
		// (trigger.Conditions, branch paths, and filter conditions all reuse the segment tree).
		newTriggerConfig, tcChanged := rewriteTimelineKindsInJSON(a.triggerConfig)
		newNodes, nodesChanged := rewriteTimelineKindsInJSON(a.nodes)
		if tcChanged || nodesChanged {
			if _, err := db.ExecContext(ctx,
				`UPDATE automations SET trigger_config = $1, nodes = $2 WHERE id = $3`,
				newTriggerConfig, newNodes, a.id); err != nil {
				return fmt.Errorf("v36: failed to update automation %s: %w", a.id, err)
			}
		}

		// Step 2: regenerate the installed trigger of LIVE email.* automations so the WHEN
		// clause matches the renamed base kind. Only live automations have an installed
		// trigger; non-email base kinds are unchanged, so they need no regeneration.
		// Trigger-level Conditions compile to a subquery WHEN (rejected by PostgreSQL), so
		// such an automation's installed trigger never enforced them — leave it.
		if a.status != "live" {
			continue
		}
		var trigger domain.TimelineTriggerConfig
		if unmarshalErr := json.Unmarshal(newTriggerConfig, &trigger); unmarshalErr != nil {
			// Unparseable trigger config — the base trigger was already unusable; skip regen.
			continue
		}
		if !strings.HasPrefix(trigger.EventKind, "email.") || trigger.Conditions != nil {
			continue
		}

		automation := &domain.Automation{ID: a.id, RootNodeID: a.rootNodeID, Trigger: &trigger}
		triggerSQL, genErr := generator.Generate(automation)
		if genErr != nil {
			// Incomplete/corrupt automation config — skip rather than abort.
			continue
		}

		// Guard each automation's DDL with a savepoint so an unexpected CREATE failure rolls
		// back just this automation (restoring its original trigger) instead of poisoning the
		// whole transaction and aborting startup.
		if _, err := db.ExecContext(ctx, "SAVEPOINT v36_regen"); err != nil {
			return fmt.Errorf("v36: failed to create savepoint: %w", err)
		}

		regenFailed := false
		// Same order as AutomationRepository.CreateAutomationTrigger:
		// drop trigger, drop function, create function, create trigger.
		for _, stmt := range []string{
			triggerSQL.DropTrigger,
			triggerSQL.DropFunction,
			triggerSQL.FunctionBody,
			triggerSQL.TriggerDDL,
		} {
			if _, execErr := db.ExecContext(ctx, stmt); execErr != nil {
				regenFailed = true
				break
			}
		}

		if regenFailed {
			if _, err := db.ExecContext(ctx, "ROLLBACK TO SAVEPOINT v36_regen"); err != nil {
				return fmt.Errorf("v36: failed to roll back savepoint: %w", err)
			}
		}
		if _, err := db.ExecContext(ctx, "RELEASE SAVEPOINT v36_regen"); err != nil {
			return fmt.Errorf("v36: failed to release savepoint: %w", err)
		}
	}

	return nil
}

func init() {
	Register(&V36Migration{})
}
