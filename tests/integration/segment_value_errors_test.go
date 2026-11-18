//go:build integration
// +build integration

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
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSegmentValueErrors runs a segment comparing custom_json_1.renewed_at as a date against a
// contact holding "2026-02-30", which PostgreSQL cannot convert: the preview, the build and the
// contact queue say which contact holds which value, the queue keeps the contact's other segments
// up to date, and automation filter and branch steps read the value as missing.
func TestSegmentValueErrors(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory
	ctx := context.Background()
	appInstance := suite.ServerManager.GetApp()
	wsRepo := appInstance.GetWorkspaceRepository()
	taskRepo := appInstance.GetTaskRepository()
	segmentRepo := repository.NewSegmentRepository(wsRepo)

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(owner.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	ws := workspace.ID

	db, err := factory.GetWorkspaceDB(ws)
	require.NoError(t, err)

	tag := strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
	withRenewedAt := func(t *testing.T, name, value string) string {
		t.Helper()
		email := fmt.Sprintf("value-%s-%s@example.com", name, tag)
		_, err := factory.CreateContact(ws, testutil.WithContactEmail(email))
		require.NoError(t, err)
		setRenewedAt(t, db, email, value)
		return email
	}
	bad := withRenewedAt(t, "bad", "2026-02-30")
	good := withRenewedAt(t, "good", "2026-06-01")

	// The raw JSON an API client sends for the segment's tree.
	renewedTree := `{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[{"field_name":"custom_json_1","field_type":"time","operator":"after_date","json_path":["renewed_at"],"string_values":["2026-01-01T00:00:00Z"]}]}}}`

	post := func(t *testing.T, path, body string) (int, map[string]interface{}) {
		t.Helper()
		resp, err := client.PostRaw(path, body)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		var decoded map[string]interface{}
		require.NoError(t, json.Unmarshal(raw, &decoded), "body: %s", raw)
		return resp.StatusCode, decoded
	}
	get := func(t *testing.T, path string, params map[string]string) (int, map[string]interface{}) {
		t.Helper()
		resp, err := client.Get(path, params)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		var decoded map[string]interface{}
		require.NoError(t, json.Unmarshal(raw, &decoded), "body: %s", raw)
		return resp.StatusCode, decoded
	}
	getSegment := func(t *testing.T, id string) map[string]interface{} {
		t.Helper()
		status, body := get(t, "/api/segments.get", map[string]string{"workspace_id": ws, "id": id})
		require.Equal(t, http.StatusOK, status, "%v", body)
		segment, ok := body["segment"].(map[string]interface{})
		require.True(t, ok, "%v", body)
		return segment
	}

	t.Run("the preview answers 422 naming the contact, the value and the path", func(t *testing.T) {
		status, body := post(t, "/api/segments.preview", `{"workspace_id":"`+ws+`","tree":`+renewedTree+`}`)
		require.Equal(t, http.StatusUnprocessableEntity, status, "%v", body)
		assert.Equal(t, map[string]interface{}{
			"column": "custom_json_1", "path": []interface{}{"renewed_at"}, "type": "date",
			"email": bad, "value": "2026-02-30",
		}, body["value_error"])
		assert.Equal(t, fmt.Sprintf(`custom_json_1.renewed_at: contact %s holds "2026-02-30", which is not a valid date`, bad), body["error"])
	})

	buildSegmentID := "valbuild" + tag
	t.Run("the build ends with the segment active and the error on it", func(t *testing.T) {
		status, body := post(t, "/api/segments.create", `{"workspace_id":"`+ws+`","id":"`+buildSegmentID+`","name":"Renewed","color":"#1677ff","timezone":"UTC","tree":`+renewedTree+`}`)
		require.Equal(t, http.StatusCreated, status, "%v", body)

		// The build runs on its own right after the create.
		var segment map[string]interface{}
		require.Eventually(t, func() bool {
			segment = getSegment(t, buildSegmentID)
			return segment["status"] == "active"
		}, 20*time.Second, 200*time.Millisecond, "the build sets the segment back to active")

		lastErr, ok := segment["last_error"].(map[string]interface{})
		require.True(t, ok, "the segment carries its last error: %v", segment)
		assert.Equal(t, map[string]interface{}{
			"kind": "value_conversion", "column": "custom_json_1", "path": []interface{}{"renewed_at"}, "type": "date",
			"email": bad, "value": "2026-02-30", "cause": `date/time field value out of range: "2026-02-30"`,
			"source": "build",
		}, lastErr)
		at, ok := segment["last_error_at"].(string)
		require.True(t, ok)
		_, err := time.Parse(time.RFC3339, at)
		assert.NoError(t, err)

		builds := buildTasks(t, ctx, taskRepo, ws, buildSegmentID)
		require.Len(t, builds, 1)
		assert.Equal(t, domain.TaskStatusCompleted, builds[0].Status, "retrying cannot help: the task completes")
		assert.Equal(t, 0, builds[0].RetryCount)
		// Tasks are readable with segments read alone: the message names the field, never the
		// contact nor the value, which the segment keeps behind contacts read.
		assert.Contains(t, builds[0].State.Message, "custom_json_1.renewed_at")
		assert.NotContains(t, builds[0].State.Message, bad)
		assert.NotContains(t, builds[0].State.Message, "2026-02-30")

		// The rebuild an API client asks for, once the value is fixed, clears it.
		setRenewedAt(t, db, bad, "2026-03-01")
		defer setRenewedAt(t, db, bad, "2026-02-30")
		status, body = post(t, "/api/segments.rebuild", `{"workspace_id":"`+ws+`","segment_id":"`+buildSegmentID+`"}`)
		require.Equal(t, http.StatusOK, status, "%v", body)
		require.Eventually(t, func() bool {
			segment = getSegment(t, buildSegmentID)
			return segment["status"] == "active"
		}, 20*time.Second, 200*time.Millisecond)
		assert.NotContains(t, segment, "last_error")
		assert.NotContains(t, segment, "last_error_at")
		assert.Equal(t, 1, memberCount(t, db, bad, buildSegmentID))
		assert.Equal(t, 1, memberCount(t, db, good, buildSegmentID))
	})

	t.Run("a member without contacts:read sees the error without the contact", func(t *testing.T) {
		segID := insertJSONDateSegment(t, db, "valred"+tag)
		_, err := segmentRepo.SetSegmentLastError(ctx, ws, segID, 1, &domain.SegmentLastError{
			Kind: domain.SegmentLastErrorKindValueConversion, Column: "custom_json_1", Path: []string{"renewed_at"},
			Type: domain.SegmentValueTypeDate, Email: bad, Value: "2026-02-30", Cause: "out of range",
			Source: domain.SegmentLastErrorSourceBuild,
		})
		require.NoError(t, err)

		member, err := factory.CreateUser()
		require.NoError(t, err)
		perms := domain.NewFullPermissions()
		perms[domain.PermissionResourceContacts] = domain.ResourcePermissions{Read: false, Write: false}
		require.NoError(t, factory.AddUserToWorkspaceWithPermissions(member.ID, ws, "member", perms))
		require.NoError(t, client.Login(member.Email, "password"))
		defer func() { require.NoError(t, client.Login(owner.Email, "password")) }()

		redacted := map[string]interface{}{
			"kind": "value_conversion", "column": "custom_json_1", "path": []interface{}{"renewed_at"}, "type": "date",
			"email": "", "value": "", "cause": "", "source": "build",
		}
		assert.Equal(t, redacted, getSegment(t, segID)["last_error"])

		status, body := get(t, "/api/segments.list", map[string]string{"workspace_id": ws})
		require.Equal(t, http.StatusOK, status)
		found := false
		for _, s := range body["segments"].([]interface{}) {
			seg := s.(map[string]interface{})
			if seg["id"] == segID {
				found = true
				assert.Equal(t, redacted, seg["last_error"])
			}
			if lastErr, ok := seg["last_error"].(map[string]interface{}); ok {
				assert.Empty(t, lastErr["email"], "no segment names a contact to this member")
			}
		}
		assert.True(t, found)

		// The preview needs contacts:read: the member never reaches the culprit through it.
		status, _ = post(t, "/api/segments.preview", `{"workspace_id":"`+ws+`","tree":`+renewedTree+`}`)
		assert.Equal(t, http.StatusForbidden, status)
	})

	t.Run("the queue keeps the contact's other segments up to date and lets it go", func(t *testing.T) {
		qtag := tag[:4]
		jsonSeg := insertJSONDateSegment(t, db, "valq"+qtag)
		plainSeg := createSegmentWithSQL(t, db, "valp"+qtag, "SELECT email FROM contacts WHERE email LIKE $1",
			domain.JSONArray{"value-%-" + tag + "@example.com"}, string(domain.SegmentStatusActive))
		// The bad contact was a member of the date segment before its value went bad: the queue
		// must neither remove it nor add it.
		addMember(t, db, bad, jsonSeg, 1)

		processor := service.NewContactSegmentQueueProcessor(
			repository.NewContactSegmentQueueRepository(wsRepo), segmentRepo,
			repository.NewContactRepository(wsRepo), wsRepo, logger.NewLoggerWithLevel("error"))
		_, err := db.ExecContext(ctx, `DELETE FROM contact_segment_queue`)
		require.NoError(t, err)
		enqueueAged(t, db, bad)

		count, err := processor.ProcessQueue(ctx, ws)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		assert.Equal(t, 0, segQueueSize(t, db), "the contact is not re-enqueued")
		assert.Equal(t, 1, memberCount(t, db, bad, plainSeg), "the unrelated segment gets the contact")
		assert.Equal(t, int64(1), memberVersion(t, db, bad, jsonSeg), "the failing segment's membership stays as it was")

		lastErr := storedLastError(t, db, jsonSeg)
		require.NotNil(t, lastErr)
		assert.Equal(t, domain.SegmentLastErrorKindValueConversion, lastErr.Kind)
		assert.Equal(t, domain.SegmentLastErrorSourceContactQueue, lastErr.Source)
		assert.Equal(t, bad, lastErr.Email)
		assert.Equal(t, "2026-02-30", lastErr.Value)
		assert.Nil(t, storedLastError(t, db, plainSeg))

		// Fixing the value clears the error on the contact's next pass, without a rebuild.
		setRenewedAt(t, db, bad, "2026-03-01")
		defer setRenewedAt(t, db, bad, "2026-02-30")
		_, err = db.ExecContext(ctx, `DELETE FROM contact_segment_queue`)
		require.NoError(t, err)
		enqueueAged(t, db, bad)
		count, err = processor.ProcessQueue(ctx, ws)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		assert.Nil(t, storedLastError(t, db, jsonSeg), "the error naming the contact is cleared")
		assert.Equal(t, 1, memberCount(t, db, bad, jsonSeg))
		assert.Equal(t, 1, memberCount(t, db, bad, plainSeg))

		// Both segments out of the way for the subtests that follow.
		require.NoError(t, segmentRepo.DeleteSegment(ctx, ws, jsonSeg))
		require.NoError(t, segmentRepo.DeleteSegment(ctx, ws, plainSeg))
	})

	t.Run("the last error follows the segment's version", func(t *testing.T) {
		// A valid value meanwhile, so the build the edit below starts records no error of its own.
		setRenewedAt(t, db, bad, "2026-03-01")
		defer setRenewedAt(t, db, bad, "2026-02-30")
		segID := insertJSONDateSegment(t, db, "valver"+tag)
		queueErr := &domain.SegmentLastError{Kind: domain.SegmentLastErrorKindValueConversion, Column: "custom_json_1",
			Path: []string{"renewed_at"}, Type: domain.SegmentValueTypeDate, Email: bad, Value: "2026-02-30",
			Cause: "out of range", Source: domain.SegmentLastErrorSourceContactQueue}

		written, err := segmentRepo.SetSegmentLastError(ctx, ws, segID, 1, queueErr)
		require.NoError(t, err)
		assert.True(t, written)

		other := *queueErr
		other.Email = good
		written, err = segmentRepo.SetSegmentLastError(ctx, ws, segID, 1, &other)
		require.NoError(t, err)
		assert.False(t, written, "the first error recorded stays")
		assert.Equal(t, bad, storedLastError(t, db, segID).Email)

		cleared, err := segmentRepo.ClearSegmentLastErrorForEmail(ctx, ws, segID, 1, good)
		require.NoError(t, err)
		assert.False(t, cleared, "another contact's pass does not clear it")

		// A write at the same version (a rename) keeps the error.
		segment, err := segmentRepo.GetSegmentByID(ctx, ws, segID)
		require.NoError(t, err)
		segment.Name = "Renamed"
		segment.LastError = nil
		require.NoError(t, segmentRepo.UpdateSegment(ctx, ws, segment))
		assert.NotNil(t, storedLastError(t, db, segID), "a rename keeps the error")

		// An edit through the API moves the version: the error described the old query.
		status, body := post(t, "/api/segments.update", `{"workspace_id":"`+ws+`","id":"`+segID+`","name":"Edited","color":"#000000","timezone":"UTC","tree":`+renewedTree+`}`)
		require.Equal(t, http.StatusOK, status, "%v", body)
		updated := body["segment"].(map[string]interface{})
		assert.NotContains(t, updated, "last_error", "the segment returned carries no error")
		assert.Nil(t, storedLastError(t, db, segID))

		// A queue pass that read the segment before the edit writes nothing.
		written, err = segmentRepo.SetSegmentLastError(ctx, ws, segID, 1, queueErr)
		require.NoError(t, err)
		assert.False(t, written, "a pass on the old version leaves no stale error")
		assert.Nil(t, storedLastError(t, db, segID))

		// Nor on a deleted segment.
		require.NoError(t, segmentRepo.DeleteSegment(ctx, ws, segID))
		version, _ := segmentVersionStatus(t, db, segID)
		written, err = segmentRepo.SetSegmentLastError(ctx, ws, segID, version, queueErr)
		require.NoError(t, err)
		assert.False(t, written)
		released, err := segmentRepo.ReleaseSegmentWithError(ctx, ws, segID, version, queueErr)
		require.NoError(t, err)
		assert.False(t, released, "a deleted segment is not brought back")
	})

	t.Run("filter and branch steps read the value as missing", func(t *testing.T) {
		condition := func(operator string, values ...string) map[string]interface{} {
			return map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{"source": "contacts", "contact": map[string]interface{}{
					"filters": []interface{}{map[string]interface{}{
						"field_name": "custom_json_1", "field_type": "time", "operator": operator,
						"json_path": []interface{}{"renewed_at"}, "string_values": values,
					}},
				}},
			}
		}
		filter := service.NewFilterNodeExecutor(service.NewQueryBuilder(), wsRepo)
		runFilter := func(t *testing.T, email string, conditions map[string]interface{}) bool {
			t.Helper()
			result, err := filter.Execute(ctx, service.NodeExecutionParams{
				WorkspaceID: ws,
				Node: &domain.AutomationNode{ID: "f", Type: domain.NodeTypeFilter, Config: map[string]interface{}{
					"continue_node_id": "yes", "exit_node_id": "no", "conditions": conditions,
				}},
				ContactData: &domain.Contact{Email: email},
			})
			require.NoError(t, err, "the step never fails on the value")
			return result.Output["filter_passed"].(bool)
		}

		assert.False(t, runFilter(t, bad, condition("after_date", "2026-01-01T00:00:00Z")), "no match for after")
		assert.True(t, runFilter(t, good, condition("after_date", "2026-01-01T00:00:00Z")))
		assert.True(t, runFilter(t, bad, condition("not_in_the_last_days", "30")), "a missing value is not in the last N days")

		branch := service.NewBranchNodeExecutor(service.NewQueryBuilder(), wsRepo)
		result, err := branch.Execute(ctx, service.NodeExecutionParams{
			WorkspaceID: ws,
			Node: &domain.AutomationNode{ID: "b", Type: domain.NodeTypeBranch, Config: map[string]interface{}{
				"paths": []interface{}{
					map[string]interface{}{"id": "renewed", "name": "Renewed", "next_node_id": "r",
						"conditions": condition("after_date", "2026-01-01T00:00:00Z")},
					map[string]interface{}{"id": "other", "name": "Other", "next_node_id": "o"},
				},
				"default_path_id": "other",
			}},
			ContactData: &domain.Contact{Email: bad},
		})
		require.NoError(t, err)
		assert.Equal(t, "default", result.Output["path_taken"])
	})
}

// setRenewedAt stores value as custom_json_1.renewed_at of a contact.
func setRenewedAt(t *testing.T, db *sql.DB, email, value string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`UPDATE contacts SET custom_json_1 = jsonb_build_object('renewed_at', $2::text) WHERE email = $1`, email, value)
	require.NoError(t, err)
}

// insertJSONDateSegment inserts an active segment at version 1 comparing custom_json_1.renewed_at
// as a date, with the query the builder compiles for it.
func insertJSONDateSegment(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	tree := &domain.TreeNode{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{Source: "contacts",
		Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{{
			FieldName: "custom_json_1", FieldType: "time", Operator: "after_date",
			JSONPath: []string{"renewed_at"}, StringValues: []string{"2026-01-01T00:00:00Z"},
		}}}}}
	sqlText, args, err := service.NewQueryBuilder().BuildSQL(tree)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), `
		INSERT INTO segments (id, name, color, tree, timezone, version, status, generated_sql, generated_args, db_created_at, db_updated_at)
		VALUES ($1, $1, '#1677ff', $2, 'UTC', 1, 'active', $3, $4, NOW(), NOW())`,
		id, mustJSON(t, tree), sqlText, mustJSON(t, args))
	require.NoError(t, err)
	return id
}

// storedLastError reads a segment's last_error column, nil when NULL.
func storedLastError(t *testing.T, db *sql.DB, segmentID string) *domain.SegmentLastError {
	t.Helper()
	var raw []byte
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT last_error FROM segments WHERE id = $1`, segmentID).Scan(&raw))
	if raw == nil {
		return nil
	}
	var lastErr domain.SegmentLastError
	require.NoError(t, json.Unmarshal(raw, &lastErr))
	return &lastErr
}
