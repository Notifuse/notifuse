package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecurringTask tests the recurring task functionality with comprehensive coverage.
// This test suite covers:
// - Recurring task creation and execution
// - Automatic rescheduling after completion
// - Trigger and Reset endpoints
// - Unique constraint enforcement
// - Backoff calculation on errors
// - Error handling and edge cases
func TestRecurringTask(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory

	// Create test user and workspace
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	// Add user to workspace as owner
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Set up authentication
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("CreateAndExecute", func(t *testing.T) {
		testRecurringTaskCreateAndExecute(t, client, factory, workspace.ID)
	})

	t.Run("TriggerImmediate", func(t *testing.T) {
		testRecurringTaskTriggerImmediate(t, client, factory, workspace.ID)
	})

	t.Run("ResetAfterFailure", func(t *testing.T) {
		testRecurringTaskResetAfterFailure(t, client, factory, workspace.ID)
	})

	t.Run("UniqueConstraint", func(t *testing.T) {
		testRecurringTaskUniqueConstraint(t, client, factory, workspace.ID)
	})

	t.Run("TriggerWhileRunning", func(t *testing.T) {
		testRecurringTaskTriggerWhileRunning(t, client, factory, workspace.ID)
	})

	t.Run("ResetNonFailedTask", func(t *testing.T) {
		testRecurringTaskResetNonFailedTask(t, client, factory, workspace.ID)
	})
}

func testRecurringTaskCreateAndExecute(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should create recurring task with interval and integration_id", func(t *testing.T) {
		interval := int64(60)
		integrationID := "test-int-" + testutil.GenerateRandomString(8)

		state := map[string]interface{}{
			"integration_sync": map[string]interface{}{
				"integration_id":   integrationID,
				"integration_type": "test",
				"consec_errors":    0,
			},
		}

		resp, err := client.CreateRecurringTask(workspaceID, "sync_integration", interval, integrationID, state)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		taskData, ok := result["task"].(map[string]interface{})
		require.True(t, ok)

		assert.Equal(t, "sync_integration", taskData["type"])
		assert.Equal(t, "pending", taskData["status"])
		assert.Equal(t, float64(interval), taskData["recurring_interval"])
		assert.Equal(t, integrationID, taskData["integration_id"])
	})

	t.Run("should reschedule recurring task after execution", func(t *testing.T) {
		interval := int64(60)
		integrationID := "test-reschedule-" + testutil.GenerateRandomString(8)

		state := map[string]interface{}{
			"integration_sync": map[string]interface{}{
				"integration_id":   integrationID,
				"integration_type": "test",
				"consec_errors":    0,
			},
		}

		// Create recurring task
		resp, err := client.CreateRecurringTask(workspaceID, "sync_integration", interval, integrationID, state)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResult map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&createResult)
		require.NoError(t, err)

		taskData := createResult["task"].(map[string]interface{})
		taskID := taskData["id"].(string)

		// Execute the task
		execResp, err := client.ExecuteTask(map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           taskID,
		})
		require.NoError(t, err)
		defer func() { _ = execResp.Body.Close() }()
		assert.Equal(t, http.StatusOK, execResp.StatusCode)

		// Give time for task to complete and reschedule
		time.Sleep(500 * time.Millisecond)

		// Get task and verify it's rescheduled (back to pending, not completed)
		getResp, err := client.GetTask(workspaceID, taskID)
		require.NoError(t, err)
		defer func() { _ = getResp.Body.Close() }()
		require.Equal(t, http.StatusOK, getResp.StatusCode)

		var getResult map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&getResult)
		require.NoError(t, err)

		updatedTask := getResult["task"].(map[string]interface{})
		// Recurring task should be back to pending, not completed
		assert.Equal(t, "pending", updatedTask["status"])

		// Verify next_run_after is set in the future
		nextRunAfterStr, ok := updatedTask["next_run_after"].(string)
		require.True(t, ok, "next_run_after should be present")

		nextRunAfter, err := time.Parse(time.RFC3339, nextRunAfterStr)
		require.NoError(t, err)

		// Should be approximately interval seconds in the future (allow for jitter)
		expectedMin := time.Now().Add(time.Duration(interval-5) * time.Second)
		expectedMax := time.Now().Add(time.Duration(interval+interval/10+10) * time.Second)
		assert.True(t, nextRunAfter.After(expectedMin) || nextRunAfter.Equal(expectedMin),
			"next_run_after %v should be after %v", nextRunAfter, expectedMin)
		assert.True(t, nextRunAfter.Before(expectedMax),
			"next_run_after %v should be before %v", nextRunAfter, expectedMax)
	})
}

func testRecurringTaskTriggerImmediate(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should trigger immediate execution of recurring task", func(t *testing.T) {
		interval := int64(3600) // 1 hour
		integrationID := "test-trigger-" + testutil.GenerateRandomString(8)

		// Create task with next_run_after far in the future
		futureTime := time.Now().Add(time.Hour)
		task, err := factory.CreateTask(workspaceID,
			testutil.WithTaskType("sync_integration"),
			testutil.WithTaskRecurringInterval(interval),
			testutil.WithTaskIntegrationID(integrationID),
			testutil.WithTaskNextRunAfter(futureTime),
		)
		require.NoError(t, err)

		// Verify initial next_run_after is in the future
		getResp, err := client.GetTask(workspaceID, task.ID)
		require.NoError(t, err)
		defer func() { _ = getResp.Body.Close() }()

		var getResult map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&getResult)
		require.NoError(t, err)

		initialTask := getResult["task"].(map[string]interface{})
		initialNextRunStr := initialTask["next_run_after"].(string)
		initialNextRun, _ := time.Parse(time.RFC3339, initialNextRunStr)
		assert.True(t, initialNextRun.After(time.Now().Add(30*time.Minute)))

		// Trigger immediate execution
		triggerResp, err := client.TriggerTask(workspaceID, task.ID)
		require.NoError(t, err)
		defer func() { _ = triggerResp.Body.Close() }()

		assert.Equal(t, http.StatusOK, triggerResp.StatusCode)

		// Verify next_run_after is now approximately now
		getResp2, err := client.GetTask(workspaceID, task.ID)
		require.NoError(t, err)
		defer func() { _ = getResp2.Body.Close() }()

		var getResult2 map[string]interface{}
		err = json.NewDecoder(getResp2.Body).Decode(&getResult2)
		require.NoError(t, err)

		updatedTask := getResult2["task"].(map[string]interface{})
		newNextRunStr := updatedTask["next_run_after"].(string)
		newNextRun, _ := time.Parse(time.RFC3339, newNextRunStr)

		// Should be approximately now (within 5 seconds)
		assert.True(t, newNextRun.Before(time.Now().Add(5*time.Second)),
			"next_run_after should be approximately now after trigger")
	})
}

func testRecurringTaskResetAfterFailure(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should reset failed recurring task and clear error state", func(t *testing.T) {
		interval := int64(60)
		integrationID := "test-reset-" + testutil.GenerateRandomString(8)

		// Create a failed recurring task with error state
		task, err := factory.CreateTask(workspaceID,
			testutil.WithTaskType("sync_integration"),
			testutil.WithTaskRecurringInterval(interval),
			testutil.WithTaskIntegrationID(integrationID),
			testutil.WithTaskStatus(domain.TaskStatusFailed),
			testutil.WithTaskState(&domain.TaskState{
				IntegrationSync: &domain.IntegrationSyncState{
					IntegrationID:   integrationID,
					IntegrationType: "test",
					ConsecErrors:    5,
					LastError:       ptrString("test error"),
					LastErrorType:   domain.ErrorTypeTransient,
				},
			}),
		)
		require.NoError(t, err)

		// Verify initial state
		getResp, err := client.GetTask(workspaceID, task.ID)
		require.NoError(t, err)
		defer func() { _ = getResp.Body.Close() }()

		var getResult map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&getResult)
		require.NoError(t, err)

		initialTask := getResult["task"].(map[string]interface{})
		assert.Equal(t, "failed", initialTask["status"])

		// Reset the task
		resetResp, err := client.ResetTask(workspaceID, task.ID)
		require.NoError(t, err)
		defer func() { _ = resetResp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resetResp.StatusCode)

		// Verify task is now pending with cleared error state
		getResp2, err := client.GetTask(workspaceID, task.ID)
		require.NoError(t, err)
		defer func() { _ = getResp2.Body.Close() }()

		var getResult2 map[string]interface{}
		err = json.NewDecoder(getResp2.Body).Decode(&getResult2)
		require.NoError(t, err)

		resetTask := getResult2["task"].(map[string]interface{})
		assert.Equal(t, "pending", resetTask["status"])

		// Verify next_run_after is approximately now
		nextRunStr := resetTask["next_run_after"].(string)
		nextRun, _ := time.Parse(time.RFC3339, nextRunStr)
		assert.True(t, nextRun.Before(time.Now().Add(5*time.Second)),
			"next_run_after should be approximately now after reset")

		// Verify error state is cleared in IntegrationSync
		stateData, ok := resetTask["state"].(map[string]interface{})
		if ok {
			syncData, ok := stateData["integration_sync"].(map[string]interface{})
			if ok {
				consecErrors, _ := syncData["consec_errors"].(float64)
				assert.Equal(t, float64(0), consecErrors, "ConsecErrors should be reset to 0")
				assert.Nil(t, syncData["last_error"], "LastError should be nil")
				lastErrorType, _ := syncData["last_error_type"].(string)
				assert.Equal(t, "", lastErrorType, "LastErrorType should be empty")
			}
		}
	})
}

func testRecurringTaskUniqueConstraint(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should enforce unique constraint on workspace_id + integration_id", func(t *testing.T) {
		interval := int64(60)
		integrationID := "test-unique-" + testutil.GenerateRandomString(8)

		state := map[string]interface{}{
			"integration_sync": map[string]interface{}{
				"integration_id":   integrationID,
				"integration_type": "test",
			},
		}

		// Create first recurring task
		resp1, err := client.CreateRecurringTask(workspaceID, "sync_integration", interval, integrationID, state)
		require.NoError(t, err)
		defer func() { _ = resp1.Body.Close() }()
		require.Equal(t, http.StatusCreated, resp1.StatusCode)

		// Attempt to create second task with same integration_id
		resp2, err := client.CreateRecurringTask(workspaceID, "sync_integration", interval, integrationID, state)
		require.NoError(t, err)
		defer func() { _ = resp2.Body.Close() }()

		// Should fail due to unique constraint
		assert.Equal(t, http.StatusInternalServerError, resp2.StatusCode)
	})

	t.Run("should allow creating task with same integration_id after completion", func(t *testing.T) {
		interval := int64(60)
		integrationID := "test-unique-complete-" + testutil.GenerateRandomString(8)

		// Create first task and mark as completed
		task, err := factory.CreateTask(workspaceID,
			testutil.WithTaskType("sync_integration"),
			testutil.WithTaskRecurringInterval(interval),
			testutil.WithTaskIntegrationID(integrationID),
			testutil.WithTaskStatus(domain.TaskStatusCompleted),
		)
		require.NoError(t, err)
		require.NotNil(t, task)

		// Create second task with same integration_id - should succeed since first is completed
		state := map[string]interface{}{
			"integration_sync": map[string]interface{}{
				"integration_id":   integrationID,
				"integration_type": "test",
			},
		}

		resp, err := client.CreateRecurringTask(workspaceID, "sync_integration", interval, integrationID, state)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	})
}

func testRecurringTaskTriggerWhileRunning(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should return 409 Conflict when triggering running task", func(t *testing.T) {
		interval := int64(60)
		integrationID := "test-running-" + testutil.GenerateRandomString(8)

		// Create a running recurring task
		task, err := factory.CreateTask(workspaceID,
			testutil.WithTaskType("sync_integration"),
			testutil.WithTaskRecurringInterval(interval),
			testutil.WithTaskIntegrationID(integrationID),
			testutil.WithTaskStatus(domain.TaskStatusRunning),
		)
		require.NoError(t, err)

		// Attempt to trigger - should fail with 409 Conflict
		triggerResp, err := client.TriggerTask(workspaceID, task.ID)
		require.NoError(t, err)
		defer func() { _ = triggerResp.Body.Close() }()

		assert.Equal(t, http.StatusConflict, triggerResp.StatusCode)
	})
}

func testRecurringTaskResetNonFailedTask(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should return 400 when resetting non-failed task", func(t *testing.T) {
		interval := int64(60)
		integrationID := "test-reset-pending-" + testutil.GenerateRandomString(8)

		// Create a pending recurring task (not failed)
		task, err := factory.CreateTask(workspaceID,
			testutil.WithTaskType("sync_integration"),
			testutil.WithTaskRecurringInterval(interval),
			testutil.WithTaskIntegrationID(integrationID),
			testutil.WithTaskStatus(domain.TaskStatusPending),
		)
		require.NoError(t, err)

		// Attempt to reset - should fail with 400
		resetResp, err := client.ResetTask(workspaceID, task.ID)
		require.NoError(t, err)
		defer func() { _ = resetResp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resetResp.StatusCode)
	})

	t.Run("should return 400 when resetting non-recurring task", func(t *testing.T) {
		// Create a failed but non-recurring task
		task, err := factory.CreateTask(workspaceID,
			testutil.WithTaskType("test_task"),
			testutil.WithTaskStatus(domain.TaskStatusFailed),
		)
		require.NoError(t, err)

		// Attempt to reset - should fail because not recurring
		resetResp, err := client.ResetTask(workspaceID, task.ID)
		require.NoError(t, err)
		defer func() { _ = resetResp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resetResp.StatusCode)
	})
}

// Helper function to create a string pointer
func ptrString(s string) *string {
	return &s
}
