package integration

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
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

// TestSegmentMembershipReliability drives the segment build, the daily recompute check and the
// contact segment queue against a real workspace database with its full trigger set. Tasks run
// through tasks.execute, so the task service's retry handling is part of what is checked. Write
// failures are injected with a BEFORE INSERT trigger that raises for one email, dropped after.
func TestSegmentMembershipReliability(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient
	ctx := context.Background()
	appInstance := suite.ServerManager.GetApp()
	wsRepo := appInstance.GetWorkspaceRepository()
	taskRepo := appInstance.GetTaskRepository()
	segmentRepo := repository.NewSegmentRepository(wsRepo)

	const recentSQL = "SELECT email FROM contacts WHERE created_at > NOW() - INTERVAL '7 days'"

	t.Run("daily rebuild removes the contacts that left the window", func(t *testing.T) {
		ws, db := newReliabilityWorkspace(t, factory, suite)
		tag := uuid.New().String()[:8]
		recent := fmt.Sprintf("recent-%s@example.com", tag)
		old := fmt.Sprintf("old-%s@example.com", tag)
		createContacts(t, factory, ws, recent, old)

		segID := createSegmentWithSQL(t, db, "segday"+tag[:4], recentSQL, domain.JSONArray{}, string(domain.SegmentStatusActive))
		addMember(t, db, recent, segID, 1)
		addMember(t, db, old, segID, 1)
		require.NoError(t, factory.SetSegmentRecomputeAfter(ws, segID, time.Now().Add(-time.Hour)))
		// The old contact's date leaves the window; its own event is not processed here, so only
		// the daily rebuild can remove it.
		_, err := db.ExecContext(ctx, `UPDATE contacts SET created_at = NOW() - INTERVAL '30 days' WHERE email = $1`, old)
		require.NoError(t, err)
		joinedBefore := timelineKindCount(t, db, recent, "segment.joined", segID)

		runRecomputeCheck(t, ctx, client, taskRepo, ws)
		builds := buildTasks(t, ctx, taskRepo, ws, segID)
		require.Len(t, builds, 1)
		assert.Equal(t, int64(2), builds[0].State.BuildSegment.Version, "the build writes at a version of its own")
		version, status := segmentVersionStatus(t, db, segID)
		assert.Equal(t, int64(2), version)
		assert.Equal(t, string(domain.SegmentStatusBuilding), status)

		// A second check before the build ran queues nothing more.
		runRecomputeCheck(t, ctx, client, taskRepo, ws)
		assert.Len(t, buildTasks(t, ctx, taskRepo, ws, segID), 1)

		executeTask(t, client, ws, builds[0].ID)

		assert.Equal(t, 0, memberCount(t, db, old, segID), "the contact out of the window leaves the segment")
		assert.Equal(t, 1, timelineKindCount(t, db, old, "segment.left", segID))
		assert.Equal(t, int64(2), memberVersion(t, db, recent, segID), "the contact still matching is rewritten at the new version")
		assert.Equal(t, joinedBefore, timelineKindCount(t, db, recent, "segment.joined", segID), "rewriting a member records no join")
		assert.Equal(t, 0, timelineKindCount(t, db, recent, "segment.left", segID))

		version, status = segmentVersionStatus(t, db, segID)
		assert.Equal(t, int64(2), version)
		assert.Equal(t, string(domain.SegmentStatusActive), status)
		assert.True(t, recomputeAfter(t, db, segID).After(time.Now()), "the next daily rebuild is scheduled")
	})

	t.Run("a rebuild that matches nobody empties the segment and is rescheduled", func(t *testing.T) {
		ws, db := newReliabilityWorkspace(t, factory, suite)
		tag := uuid.New().String()[:8]
		a := fmt.Sprintf("a-%s@example.com", tag)
		b := fmt.Sprintf("b-%s@example.com", tag)
		createContacts(t, factory, ws, a, b)

		segID := createSegmentWithSQL(t, db, "segnone"+tag[:4], recentSQL, domain.JSONArray{}, string(domain.SegmentStatusActive))
		addMember(t, db, a, segID, 1)
		addMember(t, db, b, segID, 1)
		require.NoError(t, factory.SetSegmentRecomputeAfter(ws, segID, time.Now().Add(-time.Hour)))
		_, err := db.ExecContext(ctx, `UPDATE contacts SET created_at = NOW() - INTERVAL '30 days' WHERE email = ANY($1)`,
			fmt.Sprintf("{%s,%s}", a, b))
		require.NoError(t, err)

		runRecomputeCheck(t, ctx, client, taskRepo, ws)
		builds := buildTasks(t, ctx, taskRepo, ws, segID)
		require.Len(t, builds, 1)
		executeTask(t, client, ws, builds[0].ID)

		assert.Equal(t, 0, memberCount(t, db, a, segID))
		assert.Equal(t, 0, memberCount(t, db, b, segID))
		_, status := segmentVersionStatus(t, db, segID)
		assert.Equal(t, string(domain.SegmentStatusActive), status)
		assert.True(t, recomputeAfter(t, db, segID).After(time.Now()))

		// No longer due: the next check queues no build.
		runRecomputeCheck(t, ctx, client, taskRepo, ws)
		assert.Len(t, buildTasks(t, ctx, taskRepo, ws, segID), 1)
	})

	t.Run("a failed membership write fails the build and its retry completes it", func(t *testing.T) {
		ws, db := newReliabilityWorkspace(t, factory, suite)
		tag := uuid.New().String()[:8]
		a := fmt.Sprintf("a-%s@example.com", tag)
		b := fmt.Sprintf("b-%s@example.com", tag)
		c := fmt.Sprintf("c-%s@example.com", tag)
		createContacts(t, factory, ws, a, b, c)

		segID := createSegmentWithSQL(t, db, "segfail"+tag[:4], "SELECT email FROM contacts WHERE email LIKE $1",
			domain.JSONArray{"%-" + tag + "@example.com"}, string(domain.SegmentStatusActive))
		// b is already a member at the previous version: skipping it would also remove it.
		addMember(t, db, b, segID, 1)
		_, err := db.ExecContext(ctx, `UPDATE segments SET version = 2, status = 'building' WHERE id = $1`, segID)
		require.NoError(t, err)

		taskID := createBuildTask(t, ctx, taskRepo, ws, segID, 2)
		dropTrigger := failInsertsFor(t, db, "contact_segments", b)

		executeTaskExpectingFailure(t, client, ws, taskID)

		task, err := taskRepo.Get(ctx, ws, taskID)
		require.NoError(t, err)
		assert.Equal(t, domain.TaskStatusPending, task.Status, "the build is retried")
		assert.Equal(t, 1, task.RetryCount)
		assert.Equal(t, int64(0), task.State.BuildSegment.ContactOffset, "the failed batch is not counted as done")
		_, status := segmentVersionStatus(t, db, segID)
		assert.Equal(t, string(domain.SegmentStatusBuilding), status, "a build with a missing contact is not finished")
		assert.Equal(t, int64(1), memberVersion(t, db, b, segID), "the existing member is still there")

		dropTrigger()
		executeTask(t, client, ws, taskID)

		for _, email := range []string{a, b, c} {
			assert.Equal(t, int64(2), memberVersion(t, db, email, segID), email)
		}
		_, status = segmentVersionStatus(t, db, segID)
		assert.Equal(t, string(domain.SegmentStatusActive), status)
	})

	t.Run("the queue re-enqueues a contact whose membership write failed", func(t *testing.T) {
		ws, db := newReliabilityWorkspace(t, factory, suite)
		tag := uuid.New().String()[:8]
		email := fmt.Sprintf("q-%s@example.com", tag)
		createContacts(t, factory, ws, email)
		segID := createSegmentWithSQL(t, db, "segq"+tag[:4], "SELECT email FROM contacts WHERE email LIKE $1",
			domain.JSONArray{"q-%"}, string(domain.SegmentStatusActive))

		processor := service.NewContactSegmentQueueProcessor(
			repository.NewContactSegmentQueueRepository(wsRepo),
			segmentRepo,
			repository.NewContactRepository(wsRepo),
			wsRepo,
			logger.NewLoggerWithLevel("error"),
		)

		dropTrigger := failInsertsFor(t, db, "contact_segments", email)
		_, err := db.ExecContext(ctx, `DELETE FROM contact_segment_queue`)
		require.NoError(t, err)
		enqueueAged(t, db, email)

		count, err := processor.ProcessQueue(ctx, ws)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
		assert.Equal(t, 1, segQueueSize(t, db), "the contact is back in the queue")
		assert.Equal(t, 0, memberCount(t, db, email, segID))

		dropTrigger()
		enqueueAged(t, db, email)
		count, err = processor.ProcessQueue(ctx, ws)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		assert.Equal(t, 1, memberCount(t, db, email, segID))
		assert.Equal(t, 0, segQueueSize(t, db))
	})

	t.Run("a membership write never moves a row back to an older version", func(t *testing.T) {
		ws, db := newReliabilityWorkspace(t, factory, suite)
		tag := uuid.New().String()[:8]
		email := fmt.Sprintf("v-%s@example.com", tag)
		createContacts(t, factory, ws, email)
		segID := createSegmentWithSQL(t, db, "segver"+tag[:4], "SELECT email FROM contacts WHERE email LIKE $1",
			domain.JSONArray{"v-%"}, string(domain.SegmentStatusActive))

		// A rebuild wrote the row at version 2; a queue pass that read the segment at version 1
		// writes after it.
		require.NoError(t, segmentRepo.AddContactToSegment(ctx, ws, email, segID, 2))
		require.NoError(t, segmentRepo.AddContactToSegment(ctx, ws, email, segID, 1))
		assert.Equal(t, int64(2), memberVersion(t, db, email, segID))

		require.NoError(t, segmentRepo.AddContactToSegment(ctx, ws, email, segID, 3))
		assert.Equal(t, int64(3), memberVersion(t, db, email, segID))
	})

	t.Run("a segment deleted before its build ran stays deleted", func(t *testing.T) {
		ws, db := newReliabilityWorkspace(t, factory, suite)
		tag := uuid.New().String()[:8]
		email := fmt.Sprintf("d-%s@example.com", tag)
		createContacts(t, factory, ws, email)
		segID := createSegmentWithSQL(t, db, "segdel"+tag[:4], "SELECT email FROM contacts WHERE email LIKE $1",
			domain.JSONArray{"d-%"}, string(domain.SegmentStatusActive))

		taskID := createBuildTask(t, ctx, taskRepo, ws, segID, 1)
		require.NoError(t, segmentRepo.DeleteSegment(ctx, ws, segID))

		executeTask(t, client, ws, taskID)

		_, status := segmentVersionStatus(t, db, segID)
		assert.Equal(t, string(domain.SegmentStatusDeleted), status)
		assert.Equal(t, 0, memberCount(t, db, email, segID))
	})
}

// newReliabilityWorkspace creates a workspace and returns its ID and database.
func newReliabilityWorkspace(t *testing.T, factory *testutil.TestDataFactory, suite *testutil.IntegrationTestSuite) (string, *sql.DB) {
	t.Helper()
	ws, err := factory.CreateWorkspace()
	require.NoError(t, err)
	db, err := suite.DBManager.GetWorkspaceDB(ws.ID)
	require.NoError(t, err)
	return ws.ID, db
}

func createContacts(t *testing.T, factory *testutil.TestDataFactory, workspaceID string, emails ...string) {
	t.Helper()
	for _, email := range emails {
		_, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
		require.NoError(t, err)
	}
}

// addMember inserts a membership row, as a previous build would have.
func addMember(t *testing.T, db *sql.DB, email, segmentID string, version int64) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO contact_segments (email, segment_id, version, matched_at, computed_at)
		VALUES ($1, $2, $3, NOW(), NOW())`, email, segmentID, version)
	require.NoError(t, err)
}

// memberVersion returns the version of a membership row, or 0 when there is none.
func memberVersion(t *testing.T, db *sql.DB, email, segmentID string) int64 {
	t.Helper()
	var v int64
	err := db.QueryRowContext(context.Background(),
		`SELECT version FROM contact_segments WHERE email = $1 AND segment_id = $2`, email, segmentID).Scan(&v)
	if err == sql.ErrNoRows {
		return 0
	}
	require.NoError(t, err)
	return v
}

func segmentVersionStatus(t *testing.T, db *sql.DB, segmentID string) (int64, string) {
	t.Helper()
	var version int64
	var status string
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT version, status FROM segments WHERE id = $1`, segmentID).Scan(&version, &status))
	return version, status
}

func recomputeAfter(t *testing.T, db *sql.DB, segmentID string) time.Time {
	t.Helper()
	var at sql.NullTime
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT recompute_after FROM segments WHERE id = $1`, segmentID).Scan(&at))
	require.True(t, at.Valid, "recompute_after is set")
	return at.Time
}

// failInsertsFor makes every INSERT into table for email raise, until the returned func drops it.
func failInsertsFor(t *testing.T, db *sql.DB, table, email string) func() {
	t.Helper()
	name := "fail_insert_" + uuid.New().String()[:8]
	_, err := db.ExecContext(context.Background(), fmt.Sprintf(`
		CREATE FUNCTION %[1]s() RETURNS trigger AS $$
		BEGIN
			IF NEW.email = %[2]s THEN
				RAISE EXCEPTION 'injected write failure';
			END IF;
			RETURN NEW;
		END $$ LANGUAGE plpgsql;
		CREATE TRIGGER %[1]s BEFORE INSERT ON %[3]s FOR EACH ROW EXECUTE FUNCTION %[1]s();`,
		name, quoteLiteral(email), table))
	require.NoError(t, err)

	dropped := false
	drop := func() {
		if dropped {
			return
		}
		dropped = true
		_, err := db.ExecContext(context.Background(), fmt.Sprintf(
			`DROP TRIGGER IF EXISTS %[1]s ON %[2]s; DROP FUNCTION IF EXISTS %[1]s();`, name, table))
		require.NoError(t, err)
	}
	t.Cleanup(drop)
	return drop
}

func quoteLiteral(s string) string {
	out := "'"
	for _, r := range s {
		if r == '\'' {
			out += "''"
		} else {
			out += string(r)
		}
	}
	return out + "'"
}

// runRecomputeCheck runs the workspace's daily recompute check once through tasks.execute.
func runRecomputeCheck(t *testing.T, ctx context.Context, client *testutil.APIClient, taskRepo domain.TaskRepository, workspaceID string) {
	t.Helper()
	require.NoError(t, service.EnsureSegmentRecomputeTask(ctx, taskRepo, workspaceID))
	tasks, _, err := taskRepo.List(ctx, workspaceID, domain.TaskFilter{Type: []string{"check_segment_recompute"}, Limit: 1})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	executeTask(t, client, workspaceID, tasks[0].ID)
}

// buildTasks lists the build tasks of a segment.
func buildTasks(t *testing.T, ctx context.Context, taskRepo domain.TaskRepository, workspaceID, segmentID string) []*domain.Task {
	t.Helper()
	tasks, _, err := taskRepo.List(ctx, workspaceID, domain.TaskFilter{Type: []string{"build_segment"}, Limit: 100})
	require.NoError(t, err)
	out := make([]*domain.Task, 0)
	for _, task := range tasks {
		if task.State != nil && task.State.BuildSegment != nil && task.State.BuildSegment.SegmentID == segmentID {
			out = append(out, task)
		}
	}
	return out
}

func createBuildTask(t *testing.T, ctx context.Context, taskRepo domain.TaskRepository, workspaceID, segmentID string, version int64) string {
	t.Helper()
	task := &domain.Task{
		ID:            uuid.New().String(),
		WorkspaceID:   workspaceID,
		Type:          "build_segment",
		Status:        domain.TaskStatusPending,
		MaxRuntime:    300,
		MaxRetries:    3,
		RetryInterval: 60,
		State: &domain.TaskState{
			BuildSegment: &domain.BuildSegmentState{SegmentID: segmentID, Version: version, BatchSize: 100},
		},
	}
	require.NoError(t, taskRepo.Create(ctx, workspaceID, task))
	return task.ID
}

func executeTask(t *testing.T, client *testutil.APIClient, workspaceID, taskID string) {
	t.Helper()
	resp, err := client.ExecuteTask(map[string]interface{}{"workspace_id": workspaceID, "id": taskID})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func executeTaskExpectingFailure(t *testing.T, client *testutil.APIClient, workspaceID, taskID string) {
	t.Helper()
	resp, err := client.ExecuteTask(map[string]interface{}{"workspace_id": workspaceID, "id": taskID})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.NotEqual(t, http.StatusOK, resp.StatusCode)
}
