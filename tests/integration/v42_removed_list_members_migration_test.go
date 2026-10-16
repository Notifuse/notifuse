package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestV42RemovedListMembersMigration builds the state a workspace is in before 42 —
// stored segment SQL and an installed automation trigger function that count removed
// list members, and the old contact_lists trigger functions — then runs the v42
// migration the way the manager does and checks the workspace behaves as a fresh one.
func TestV42RemovedListMembersMigration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		cfg.Webhook.AllowPrivateDeliveryHosts = true
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory
	bg := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	wsRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	processor := service.NewContactSegmentQueueProcessor(
		repository.NewContactSegmentQueueRepository(wsRepo),
		repository.NewSegmentRepository(wsRepo),
		repository.NewContactRepository(wsRepo),
		wsRepo,
		logger.NewLoggerWithLevel("error"),
	)

	tag := strings.ToLower(strings.ReplaceAll(uuid.New().String(), "-", ""))[:8]

	post := func(t *testing.T, path string, body map[string]interface{}) (int, string) {
		t.Helper()
		resp, err := client.Post(path, body)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode, string(raw)
	}
	newContact := func(t *testing.T, name string) string {
		t.Helper()
		email := fmt.Sprintf("v42-%s-%s@example.com", name, tag)
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		return email
	}
	addTo := func(t *testing.T, email, listID string) {
		t.Helper()
		_, err := factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(listID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
	}
	removeViaAPI := func(t *testing.T, email, listID string) {
		t.Helper()
		status, body := post(t, "/api/contactLists.removeContact", map[string]interface{}{
			"workspace_id": workspace.ID, "email": email, "list_id": listID,
		})
		require.Equal(t, http.StatusOK, status, "removeContact: %s", body)
	}
	drainQueue := func(t *testing.T) {
		t.Helper()
		for i := 0; i < 20; i++ {
			_, err := db.ExecContext(bg, `UPDATE contact_segment_queue SET queued_at = NOW() - INTERVAL '1 minute'`)
			require.NoError(t, err)
			n, err := processor.ProcessQueue(bg, workspace.ID)
			require.NoError(t, err)
			if n == 0 {
				return
			}
		}
		t.Fatal("the segment queue never drained")
	}
	queued := func(t *testing.T, email string) bool {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT COUNT(*) FROM contact_segment_queue WHERE email = $1`, email).Scan(&n))
		return n == 1
	}
	storedSQL := func(t *testing.T, segmentID string) string {
		t.Helper()
		var s sql.NullString
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT generated_sql FROM segments WHERE id = $1`, segmentID).Scan(&s))
		return s.String
	}
	storedArgs := func(t *testing.T, segmentID string) string {
		t.Helper()
		var raw []byte
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT generated_args FROM segments WHERE id = $1`, segmentID).Scan(&raw))
		return string(raw)
	}
	versionAndStatus := func(t *testing.T, segmentID string) string {
		t.Helper()
		var version int
		var status string
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT version, status FROM segments WHERE id = $1`, segmentID).Scan(&version, &status))
		return fmt.Sprintf("%d/%s", version, status)
	}
	// matches runs a segment's stored SQL with its stored arguments, as the segment queue
	// does, and returns how many contacts it matches.
	matches := func(t *testing.T, segmentID string) int {
		t.Helper()
		var args []interface{}
		require.NoError(t, json.Unmarshal([]byte(storedArgs(t, segmentID)), &args))
		rows, err := db.QueryContext(bg, storedSQL(t, segmentID), args...)
		require.NoError(t, err, "segment %s: stored SQL must execute", segmentID)
		defer func() { _ = rows.Close() }()
		matched := 0
		for rows.Next() {
			matched++
		}
		require.NoError(t, rows.Err())
		return matched
	}
	listLeaf := func(operator, listID, status string) map[string]interface{} {
		contactList := map[string]interface{}{"operator": operator, "list_id": listID}
		if status != "" {
			contactList["status"] = status
		}
		return map[string]interface{}{
			"kind": "leaf",
			"leaf": map[string]interface{}{"source": "contact_lists", "contact_list": contactList},
		}
	}
	createSegment := func(t *testing.T, name string, tree map[string]interface{}) string {
		t.Helper()
		segmentID := "v42" + name + tag
		status, body := post(t, "/api/segments.create", map[string]interface{}{
			"workspace_id": workspace.ID, "id": segmentID, "name": "V42 " + name,
			"color": "#00BFFF", "timezone": "UTC", "tree": tree,
		})
		require.Contains(t, []int{http.StatusOK, http.StatusCreated}, status, "segments.create: %s", body)
		_, err := testutil.WaitForSegmentBuilt(t, client, workspace.ID, segmentID, 60*time.Second)
		require.NoError(t, err)
		return segmentID
	}
	functionName := func(automationID string) string {
		return strings.ToLower("automation_trigger_" + strings.ReplaceAll(automationID, "-", ""))
	}
	functionSource := func(t *testing.T, automationID string) string {
		t.Helper()
		var src string
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT prosrc FROM pg_proc WHERE proname = $1`, functionName(automationID)).Scan(&src))
		return src
	}
	triggerOID := func(t *testing.T, automationID string) (int64, bool) {
		t.Helper()
		var oid int64
		err := db.QueryRowContext(bg,
			`SELECT oid FROM pg_trigger WHERE tgname = $1 AND NOT tgisinternal`, functionName(automationID)).Scan(&oid)
		if err == sql.ErrNoRows {
			return 0, false
		}
		require.NoError(t, err)
		return oid, true
	}
	createAutomation := func(t *testing.T, name, eventName string, conditions map[string]interface{}, marker string) string {
		t.Helper()
		automationID := shortuuid.New()
		triggerNodeID := shortuuid.New()
		addNodeID := shortuuid.New()
		status, body := post(t, "/api/automations.create", map[string]interface{}{
			"workspace_id": workspace.ID,
			"automation": map[string]interface{}{
				"id":           automationID,
				"workspace_id": workspace.ID,
				"name":         name,
				"status":       "draft",
				"trigger": map[string]interface{}{
					"event_kind": "custom_event", "custom_event_name": eventName,
					"frequency": "once", "conditions": conditions,
				},
				"root_node_id": triggerNodeID,
				"nodes": []map[string]interface{}{
					{
						"id": triggerNodeID, "automation_id": automationID, "type": "trigger",
						"config": map[string]interface{}{}, "next_node_id": addNodeID,
						"position": map[string]interface{}{"x": 0, "y": 0},
					},
					{
						"id": addNodeID, "automation_id": automationID, "type": "add_to_list",
						"config":   map[string]interface{}{"list_id": marker, "status": "active"},
						"position": map[string]interface{}{"x": 0, "y": 100},
					},
				},
				"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
			},
		})
		require.Equal(t, http.StatusCreated, status, "automations.create: %s", body)
		status, body = post(t, "/api/automations.activate", map[string]interface{}{
			"workspace_id": workspace.ID, "automation_id": automationID,
		})
		require.Equal(t, http.StatusOK, status, "automations.activate: %s", body)
		return automationID
	}
	runV42 := func(t *testing.T) {
		t.Helper()
		// Each workspace migration runs in its own transaction, which v42's savepoints need.
		tx, err := db.BeginTx(bg, nil)
		require.NoError(t, err)
		if mErr := (&migrations.V42Migration{}).UpdateWorkspace(bg, &config.Config{}, workspace, tx); mErr != nil {
			_ = tx.Rollback()
			t.Fatalf("v42 UpdateWorkspace failed: %v", mErr)
		}
		require.NoError(t, tx.Commit())
	}

	// --- pre-42 state ----------------------------------------------------------------

	listX, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	listW, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	marker, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)

	member := newContact(t, "member")
	addTo(t, member, listX.ID)
	removed := newContact(t, "removed")
	addTo(t, removed, listX.ID)
	wRemoved := newContact(t, "wremoved")
	addTo(t, wRemoved, listW.ID)

	inSegment := createSegment(t, "in", listLeaf("in", listX.ID, "active"))
	notInSegment := createSegment(t, "notin", listLeaf("not_in", listX.ID, ""))
	// Stands for a segment v36 or v37 already recompiled with the fixed builder.
	wSegment := createSegment(t, "w", listLeaf("in", listW.ID, ""))
	// Saved by an older builder: a custom_json key check compiled before the builder
	// started guarding it with jsonb_typeof.
	olderSegment := createSegment(t, "older", map[string]interface{}{
		"kind": "branch",
		"branch": map[string]interface{}{"operator": "and", "leaves": []interface{}{
			listLeaf("in", listX.ID, "active"),
			map[string]interface{}{"kind": "leaf", "leaf": map[string]interface{}{
				"source": "contacts",
				"contact": map[string]interface{}{"filters": []interface{}{map[string]interface{}{
					"field_name": "custom_json_1", "field_type": "json", "operator": "is_not_set",
					"json_path": []string{"tier"},
				}}},
			}},
		}},
	})

	eventName := "v42_condition_" + tag
	liveID := createAutomation(t, "V42 live", eventName, listLeaf("in", listX.ID, "active"), marker.ID)
	freshBody := functionSource(t, liveID)
	require.Contains(t, freshBody, "cl.deleted_at IS NULL", "the fixed generator installed the new SQL")

	pausedID := createAutomation(t, "V42 paused", "v42_paused_"+tag, listLeaf("in", listX.ID, "active"), marker.ID)
	status, body := post(t, "/api/automations.pause", map[string]interface{}{
		"workspace_id": workspace.ID, "automation_id": pausedID,
	})
	require.Equal(t, http.StatusOK, status, "automations.pause: %s", body)
	_, armed := triggerOID(t, pausedID)
	require.False(t, armed)

	// The old contact_lists timeline function is the real old code: v23 installed it.
	require.NoError(t, (&migrations.V23Migration{}).UpdateWorkspace(bg, &config.Config{}, workspace, db))
	// A stub, so asserting that v42 installs the webhook function means something.
	_, err = db.ExecContext(bg, `CREATE OR REPLACE FUNCTION webhook_contact_lists_trigger()
		RETURNS TRIGGER AS $$ BEGIN RETURN NEW; END; $$ LANGUAGE plpgsql`)
	require.NoError(t, err)
	subscriptionID := createZapierSubscription(t, suite, workspace.ID, "V42 webhooks",
		"https://hooks.example.com/v42", []string{"list.subscribed"}, []string{listX.ID}, nil)

	// What the fixed builder stores, for comparing with what v42 leaves.
	freshIn, freshNotIn := storedSQL(t, inSegment), storedSQL(t, notInSegment)
	freshOlder := storedSQL(t, olderSegment)
	require.Contains(t, freshOlder, "jsonb_typeof(custom_json_1) = 'object' AND ")

	// Turn the stored SQL into what the builder produced before 42.
	_, err = db.ExecContext(bg, `UPDATE segments SET generated_sql = replace(generated_sql, ' AND cl.deleted_at IS NULL', '')
		WHERE id IN ($1, $2, $3)`, inSegment, notInSegment, olderSegment)
	require.NoError(t, err)
	_, err = db.ExecContext(bg, `UPDATE segments SET generated_sql = replace(generated_sql, 'jsonb_typeof(custom_json_1) = ''object'' AND ', '')
		WHERE id = $1`, olderSegment)
	require.NoError(t, err)
	olderSQL := storedSQL(t, olderSegment)
	require.Contains(t, olderSQL, "NOT (custom_json_1 ? $3)", "the older builder's key check")
	require.NotContains(t, olderSQL, "cl.deleted_at IS NULL")
	assert.Equal(t, "SELECT email FROM contacts WHERE EXISTS (SELECT 1 FROM contact_lists cl JOIN lists l ON cl.list_id = l.id WHERE cl.email = contacts.email AND cl.list_id = $1 AND cl.status = $2 AND l.deleted_at IS NULL)", storedSQL(t, inSegment))
	assert.Equal(t, "SELECT email FROM contacts WHERE NOT EXISTS (SELECT 1 FROM contact_lists cl JOIN lists l ON cl.list_id = l.id WHERE cl.email = contacts.email AND cl.list_id = $1 AND l.deleted_at IS NULL)", storedSQL(t, notInSegment))
	newWSQL := storedSQL(t, wSegment)
	require.Contains(t, newWSQL, "cl.deleted_at IS NULL")

	// Turn the live automation's function into what the generator installed before 42.
	oidBefore, armed := triggerOID(t, liveID)
	require.True(t, armed)
	var def string
	require.NoError(t, db.QueryRowContext(bg,
		`SELECT pg_get_functiondef(oid) FROM pg_proc WHERE proname = $1`, functionName(liveID)).Scan(&def))
	_, err = db.ExecContext(bg, strings.ReplaceAll(def, " AND cl.deleted_at IS NULL", ""))
	require.NoError(t, err)
	assert.Contains(t, functionSource(t, liveID),
		"IF (EXISTS (SELECT 1 FROM contact_lists cl JOIN lists l ON cl.list_id = l.id WHERE cl.email = NEW.email AND cl.list_id = '"+listX.ID+"' AND cl.status = 'active' AND l.deleted_at IS NULL)) THEN")

	// A segment whose tree no longer compiles (a list condition without a list), with old
	// SQL. Its tree still parses: an active segment whose tree doesn't parse fails
	// GetSegments, which stops the segment queue for the whole workspace, so no working
	// workspace has one.
	uncompilable := "v42broken" + tag
	_, err = db.ExecContext(bg, `
		INSERT INTO segments (id, name, color, tree, timezone, version, status, generated_sql, generated_args, db_created_at, db_updated_at)
		VALUES ($1, 'V42 broken', '#FF5733', $2, 'UTC', 1, 'active', $3, $4, NOW(), NOW())`,
		uncompilable, mustJSON(t, listLeaf("in", "", "active")), storedSQL(t, inSegment),
		mustJSON(t, []interface{}{listX.ID, "active"}))
	require.NoError(t, err)

	// The old behaviour: removing a contact keeps them in "in list".
	drainQueue(t)
	removeViaAPI(t, removed, listX.ID)
	removeViaAPI(t, wRemoved, listW.ID)
	drainQueue(t)
	require.Equal(t, 1, memberCount(t, db, removed, inSegment), "pre-42, a removed contact stays in \"in list\"")
	require.Equal(t, 0, memberCount(t, db, removed, notInSegment))
	require.False(t, queued(t, member))

	// --- migrate ---------------------------------------------------------------------

	segments := []string{inSegment, notInSegment, wSegment, olderSegment, uncompilable}
	argsBefore := map[string]string{}
	stateBefore := map[string]string{}
	for _, segmentID := range segments {
		argsBefore[segmentID] = storedArgs(t, segmentID)
		stateBefore[segmentID] = versionAndStatus(t, segmentID)
	}

	runV42(t)

	t.Run("stored segment SQL is patched in place and still executes", func(t *testing.T) {
		assert.Equal(t, freshIn, storedSQL(t, inSegment), "byte-identical to what the fixed builder stores")
		assert.Equal(t, freshNotIn, storedSQL(t, notInSegment), "byte-identical to what the fixed builder stores")
		assert.Positive(t, matches(t, inSegment))
		assert.Positive(t, matches(t, notInSegment))

		assert.Equal(t, newWSQL, storedSQL(t, wSegment), "an already-fixed segment is left alone")
		assert.Equal(t, freshIn, storedSQL(t, uncompilable), "a segment whose tree no longer compiles is patched too")

		older := storedSQL(t, olderSegment)
		assert.Equal(t, strings.ReplaceAll(olderSQL, " AND l.deleted_at IS NULL)", " AND cl.deleted_at IS NULL AND l.deleted_at IS NULL)"), older,
			"only the list condition changes: no other builder change since the segment was saved is applied")
		assert.NotContains(t, older, "jsonb_typeof")
		matches(t, olderSegment)

		for _, segmentID := range segments {
			assert.Equal(t, argsBefore[segmentID], storedArgs(t, segmentID), "segment %s: generated_args untouched", segmentID)
			assert.Equal(t, stateBefore[segmentID], versionAndStatus(t, segmentID), "segment %s: version and status untouched", segmentID)
		}
	})

	t.Run("removed members of lists segments use are re-queued", func(t *testing.T) {
		assert.True(t, queued(t, removed))
		assert.True(t, queued(t, wRemoved), "list IDs come from every segment, including one already fixed")
		assert.False(t, queued(t, member))

		drainQueue(t)
		assert.Equal(t, 0, memberCount(t, db, removed, inSegment), "the removed contact leaves \"in list\"")
		assert.Equal(t, 1, memberCount(t, db, removed, notInSegment), "and joins \"not in list\"")
		assert.Equal(t, 1, memberCount(t, db, member, inSegment))
	})

	t.Run("the live automation's function is patched in place", func(t *testing.T) {
		assert.Equal(t, freshBody, functionSource(t, liveID), "byte-identical to what the fixed generator installs")
		oidAfter, armed := triggerOID(t, liveID)
		require.True(t, armed)
		assert.Equal(t, oidBefore, oidAfter, "the trigger itself is untouched")

		for _, email := range []string{member, removed} {
			status, body := post(t, "/api/customEvents.upsert", map[string]interface{}{
				"workspace_id": workspace.ID, "email": email, "event_name": eventName,
				"external_id": shortuuid.New(), "properties": map[string]interface{}{},
				"occurred_at": time.Now().UTC().Format(time.RFC3339),
			})
			require.Equal(t, http.StatusOK, status, "customEvents.upsert: %s", body)
		}
		enrolled, err := factory.FindContactAutomation(workspace.ID, liveID, member)
		require.NoError(t, err)
		assert.NotNil(t, enrolled)
		notEnrolled, err := factory.FindContactAutomation(workspace.ID, liveID, removed)
		require.NoError(t, err)
		assert.Nil(t, notEnrolled)

		_, armed = triggerOID(t, pausedID)
		assert.False(t, armed, "a paused automation is not armed by the migration")
	})

	t.Run("a re-add records a subscription on the timeline and as a webhook", func(t *testing.T) {
		before := b24TimelineInserts(t, db, removed, "list.subscribed", listX.ID)
		status, body := post(t, "/api/lists.subscribe", map[string]interface{}{
			"workspace_id": workspace.ID,
			"contact":      map[string]interface{}{"email": removed},
			"list_ids":     []string{listX.ID},
		})
		require.Equal(t, http.StatusOK, status, "lists.subscribe: %s", body)
		assert.Equal(t, before+1, b24TimelineInserts(t, db, removed, "list.subscribed", listX.ID))

		deliveries := b24Deliveries(t, db, subscriptionID, removed)
		require.Len(t, deliveries, 1)
		assert.Equal(t, "list.subscribed", deliveries[0].eventType)
		prev, present := deliveries[0].payload["previous_status"]
		assert.True(t, present)
		assert.Nil(t, prev)

		// Back to the removed state for the re-run check.
		removeViaAPI(t, removed, listX.ID)
		drainQueue(t)
	})

	t.Run("re-running the migration changes nothing", func(t *testing.T) {
		sqlBefore := map[string]string{}
		for _, segmentID := range segments {
			sqlBefore[segmentID] = storedSQL(t, segmentID)
		}
		body := functionSource(t, liveID)

		runV42(t)

		for _, segmentID := range segments {
			assert.Equal(t, sqlBefore[segmentID], storedSQL(t, segmentID), "segment %s", segmentID)
		}
		assert.Equal(t, body, functionSource(t, liveID))

		drainQueue(t)
		assert.Equal(t, 0, memberCount(t, db, removed, inSegment))
		assert.Equal(t, 1, memberCount(t, db, removed, notInSegment))
		assert.Equal(t, 1, memberCount(t, db, member, inSegment))
	})
}
