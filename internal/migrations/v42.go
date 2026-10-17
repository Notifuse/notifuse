package migrations

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/lib/pq"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V42Migration makes a contact removed from a list (contact_lists.deleted_at set) a
// non-member everywhere, on workspaces that already exist.
//
// The query builder now compiles a list condition with "cl.deleted_at IS NULL", and the
// broadcast audience and the contact's memberships filter removed rows in Go. What runs
// from SQL compiled earlier has to be brought forward here:
//
//  1. The contact_lists timeline and webhook trigger functions are reinstalled: a removed
//     contact added back now records what a first subscription records (list.subscribed,
//     list.pending, …). Without it the re-add wrote no timeline row, so the segment queue
//     never re-evaluated the contact, and the query fix alone would have kept every
//     re-added contact out of their "in list" segments. Removing a contact who is already
//     removed no longer records list.removed again.
//  2. Installed automation trigger functions are patched in place. Their conditions are
//     compiled into the function body with values inlined, so the stored text is exactly
//     the generator's old output; inserting the predicate gives exactly its new output.
//     Patching rather than regenerating (the v38 pattern) is deliberate: DROP/CREATE
//     TRIGGER holds ACCESS EXCLUSIVE on contact_timeline until commit, which deadlocked
//     with a concurrent write to a re-queued contact, and regeneration trusts a stored
//     trigger_config that v38 already had to second-guess. CREATE OR REPLACE FUNCTION is
//     a pg_proc update: no table lock, and the attached trigger picks the body up on its
//     next invocation. The patch adds a column every workspace has (contact_lists has had
//     deleted_at since its creation), so no condition can start referencing a column
//     that is missing. Each function is patched in its own savepoint.
//  3. Stored segment SQL is patched in place the same way, by one UPDATE. Recompiling
//     from the tree (as v37 did) would also apply every unrelated builder change made
//     since the segment was saved, such as the custom_json guards, to segments whose other
//     members are not re-evaluated; and it would leave the bug in a tree that no longer
//     compiles. The filter takes no parameter, so generated_args stays valid, and the
//     segment's version and status are untouched.
//  4. Contacts removed from a list a segment's condition names are re-queued for segment
//     recomputation, so they leave "in list" segments and join "not in list" ones now
//     rather than on their next unrelated timeline write. This runs last, so the queue
//     row locks are held for as short a time as possible.
//
// Every step is idempotent: the selections no longer match a patched function or
// segment, and re-queueing a contact again is harmless.
type V42Migration struct{}

func (m *V42Migration) GetMajorVersion() float64 { return 42.0 }

func (m *V42Migration) HasSystemUpdate() bool { return false }

func (m *V42Migration) HasWorkspaceUpdate() bool { return true }

func (m *V42Migration) ShouldRestartServer() bool { return false }

func (m *V42Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

// v42ListPredicateOld and v42ListPredicateNew are the end of a compiled list condition
// before and after the fix. internal/service/query_builder.go emits the new form.
const (
	v42ListPredicateOld = " AND l.deleted_at IS NULL)"
	v42ListPredicateNew = " AND cl.deleted_at IS NULL AND l.deleted_at IS NULL)"
	v42ListConditionTag = "FROM contact_lists cl JOIN lists l"
	v42RemovedRowFilter = "cl.deleted_at IS NULL"
)

func (m *V42Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// The loops below can outlast a globally configured statement_timeout, and a failed
	// migration would fail again on every restart. SET LOCAL ends with the transaction.
	if _, err := db.ExecContext(ctx, "SET LOCAL statement_timeout = 0"); err != nil {
		return fmt.Errorf("v42: failed to disable statement_timeout: %w", err)
	}

	// The triggers stay attached; replacing a function body takes no table lock.
	for _, query := range []string{
		schema.TrackContactListChangesFunction(),
		schema.WebhookContactListsTriggerFunction(),
	} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("v42: failed to install contact_lists trigger function for workspace %s: %w", workspace.ID, err)
		}
	}

	if err := m.patchAutomationTriggerFunctions(ctx, db); err != nil {
		return fmt.Errorf("v42: failed to patch automation trigger functions for workspace %s: %w", workspace.ID, err)
	}

	listIDs, err := m.patchSegmentQueries(ctx, db)
	if err != nil {
		return fmt.Errorf("v42: failed to patch segments for workspace %s: %w", workspace.ID, err)
	}

	if err := m.requeueRemovedListMembers(ctx, db, listIDs); err != nil {
		return fmt.Errorf("v42: failed to re-queue removed list members for workspace %s: %w", workspace.ID, err)
	}

	return nil
}

// patchAutomationTriggerFunctions rewrites every installed automation trigger function
// whose list condition still counts removed rows. It covers live automations and any
// function a Pause interrupted between its two DROPs (harmless: the enroll function
// refuses non-live automations). Paused and draft automations have no function and get
// the new SQL from the generator when activated.
func (m *V42Migration) patchAutomationTriggerFunctions(ctx context.Context, db DBExecutor) error {
	rows, err := db.QueryContext(ctx, `
		SELECT p.oid, pg_get_functiondef(p.oid)
		FROM pg_proc p
		WHERE p.proname LIKE 'automation\_trigger\_%'
		  AND p.prosrc LIKE '%`+v42ListConditionTag+`%'
		  AND p.prosrc NOT LIKE '%`+v42RemovedRowFilter+`%'
	`)
	if err != nil {
		return fmt.Errorf("failed to query automation trigger functions: %w", err)
	}

	// Read every row and close the cursor before writing: the workspace migration shares
	// one connection.
	var definitions []string
	for rows.Next() {
		var oid int64
		var def string
		if scanErr := rows.Scan(&oid, &def); scanErr != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to scan automation trigger function: %w", scanErr)
		}
		definitions = append(definitions, def)
	}
	if iterErr := rows.Err(); iterErr != nil {
		_ = rows.Close()
		return fmt.Errorf("error iterating automation trigger functions: %w", iterErr)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return fmt.Errorf("failed to close automation trigger function rows: %w", closeErr)
	}

	for _, def := range definitions {
		patched := strings.ReplaceAll(def, v42ListPredicateOld, v42ListPredicateNew)
		if patched == def {
			continue
		}

		// One savepoint per function, so a failure keeps just this function as it was
		// rather than abort the workspace migration, which would re-fail on every restart.
		if _, err := db.ExecContext(ctx, "SAVEPOINT v42_automation_trigger"); err != nil {
			return fmt.Errorf("failed to create savepoint: %w", err)
		}
		if _, execErr := db.ExecContext(ctx, patched); execErr != nil {
			if _, err := db.ExecContext(ctx, "ROLLBACK TO SAVEPOINT v42_automation_trigger"); err != nil {
				return fmt.Errorf("failed to roll back savepoint: %w", err)
			}
		}
		if _, err := db.ExecContext(ctx, "RELEASE SAVEPOINT v42_automation_trigger"); err != nil {
			return fmt.Errorf("failed to release savepoint: %w", err)
		}
	}

	return nil
}

// patchSegmentQueries adds the removed-row filter to the stored SQL of every segment whose
// list condition still counts removed rows, and returns the list IDs named by a
// contact_lists condition of every non-deleted segment, patched or not: on a database
// upgraded from below 37, v36 or v37 already recompiled the segments with the fixed
// builder, and their removed contacts still need re-queueing.
//
// The builder emits the list condition's old ending nowhere else, values are bound as
// parameters, and the one thing it inlines, a JSON key, is a quoted literal the inserted
// text (which holds no quote) cannot break out of. One statement rather than a read and a
// write per segment, so a segment saved while the migration runs is never overwritten
// with SQL derived from the version read before it.
func (m *V42Migration) patchSegmentQueries(ctx context.Context, db DBExecutor) ([]string, error) {
	if _, err := db.ExecContext(ctx, `
		UPDATE segments
		SET generated_sql = replace(generated_sql, $1, $2)
		WHERE status <> 'deleted'
		  AND strpos(generated_sql, $1) > 0
		  AND strpos(generated_sql, $3) > 0
		  AND strpos(generated_sql, $4) = 0
	`, v42ListPredicateOld, v42ListPredicateNew, v42ListConditionTag, v42RemovedRowFilter); err != nil {
		return nil, fmt.Errorf("failed to patch segment queries: %w", err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT tree FROM segments
		WHERE status <> 'deleted'
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query segment trees: %w", err)
	}

	listIDSet := make(map[string]struct{})
	for rows.Next() {
		var treeJSON []byte
		if scanErr := rows.Scan(&treeJSON); scanErr != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("failed to scan segment tree: %w", scanErr)
		}
		var tree domain.TreeNode
		// A tree that does not parse names no list; skip it rather than abort the migration.
		if json.Unmarshal(treeJSON, &tree) != nil {
			continue
		}
		v42CollectListIDs(&tree, listIDSet)
	}
	if iterErr := rows.Err(); iterErr != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("error iterating segment trees: %w", iterErr)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return nil, fmt.Errorf("failed to close segment tree rows: %w", closeErr)
	}

	listIDs := make([]string, 0, len(listIDSet))
	for id := range listIDSet {
		listIDs = append(listIDs, id)
	}
	sort.Strings(listIDs)
	return listIDs, nil
}

// v42CollectListIDs adds the list ID of every contact_lists leaf of the tree to ids.
func v42CollectListIDs(node *domain.TreeNode, ids map[string]struct{}) {
	if node == nil {
		return
	}
	if node.Branch != nil {
		for _, child := range node.Branch.Leaves {
			v42CollectListIDs(child, ids)
		}
	}
	if node.Leaf != nil && node.Leaf.Source == "contact_lists" &&
		node.Leaf.ContactList != nil && node.Leaf.ContactList.ListID != "" {
		ids[node.Leaf.ContactList.ListID] = struct{}{}
	}
}

// requeueRemovedListMembers queues for segment recomputation every contact removed from
// one of the given (non-deleted) lists: only their segment results change with the fix.
// Ordered by email, in the database's collation as webNavigationRequeue is, so the two take
// the queue's row locks in the same order. Any transaction that queues contacts out of email
// order, such as an import, can still deadlock with it; Postgres then aborts one of them, and
// an aborted migration runs again on the next start.
func (m *V42Migration) requeueRemovedListMembers(ctx context.Context, db DBExecutor, listIDs []string) error {
	if len(listIDs) == 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO contact_segment_queue (email, queued_at)
		SELECT DISTINCT cl.email, NOW()
		FROM contact_lists cl
		JOIN lists l ON l.id = cl.list_id
		WHERE cl.deleted_at IS NOT NULL
		  AND l.deleted_at IS NULL
		  AND cl.list_id = ANY($1)
		ORDER BY cl.email
		ON CONFLICT (email) DO UPDATE SET queued_at = EXCLUDED.queued_at
	`, pq.Array(listIDs)); err != nil {
		return fmt.Errorf("failed to queue contacts: %w", err)
	}
	return nil
}

func init() {
	Register(&V42Migration{})
}
