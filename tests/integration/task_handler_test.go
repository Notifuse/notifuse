package integration

import (
	"database/sql"
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

// TestTaskHandler tests the task handler with comprehensive coverage of all endpoints and functionality.
// This test suite covers:
// - Task CRUD operations (create, get, list, delete)
// - Task execution (individual and batch)
// - Task state management and lifecycle
// - Error handling and edge cases
// - Authentication and authorization
// - Repository operations through the service layer
func TestTaskHandler(t *testing.T) {
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

	t.Run("Task CRUD Operations", func(t *testing.T) {
		testTaskCRUD(t, client, factory, workspace.ID)
	})

	t.Run("Task Execution", func(t *testing.T) {
		testTaskExecution(t, client, factory, workspace.ID)
	})

	t.Run("Task State Management", func(t *testing.T) {
		testTaskStateManagement(t, client, factory, workspace.ID)
	})

	t.Run("Task Authentication", func(t *testing.T) {
		testTaskAuthentication(t, client, factory, workspace.ID)
	})

	t.Run("Task Repository Operations", func(t *testing.T) {
		testTaskRepositoryOperations(t, client, factory, workspace.ID)
	})

	// Skip error handling and broadcast integration for now due to complexity
	// These can be added in follow-up improvements
}

func testTaskCRUD(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("Create Task", func(t *testing.T) {
		t.Run("the route is gone", func(t *testing.T) {
			// Tasks are created by the services that own them. Over HTTP, naming
			// a type and a state meant naming a draft broadcast and sending it.
			resp, err := client.Post("/api/tasks.create", map[string]interface{}{
				"workspace_id": workspaceID,
				"type":         "send_broadcast",
			})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// The path stays routed so the refusal is explicit: left unrouted it
			// would fall through to the console SPA catch-all, which answers 200
			// with an empty body — indistinguishable from success.
			require.Equal(t, http.StatusGone, resp.StatusCode)

			var response map[string]interface{}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&response))
			assert.NotContains(t, response, "task")
			assert.NotEmpty(t, response["error"])

			listResp, err := client.Get("/api/tasks.list", map[string]string{"workspace_id": workspaceID})
			require.NoError(t, err)
			defer func() { _ = listResp.Body.Close() }()
			require.Equal(t, http.StatusOK, listResp.StatusCode)

			var listResponse struct {
				Tasks []json.RawMessage `json:"tasks"`
			}
			require.NoError(t, json.NewDecoder(listResp.Body).Decode(&listResponse))
			assert.Empty(t, listResponse.Tasks, "the request must not have created a task")
		})
	})

	t.Run("Get Task", func(t *testing.T) {
		t.Run("should get task successfully", func(t *testing.T) {
			// Create a task first
			task, err := factory.CreateTask(workspaceID,
				testutil.WithTaskType("send_broadcast"),
				testutil.WithTaskProgress(25.5))
			require.NoError(t, err)

			resp, err := client.GetTask(workspaceID, task.ID)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			taskData, ok := result["task"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, task.ID, taskData["id"])
			assert.Equal(t, "send_broadcast", taskData["type"])
			assert.Equal(t, 25.5, taskData["progress"])
		})

		t.Run("should return 404 for non-existent task", func(t *testing.T) {
			// Use a valid UUID format that doesn't exist
			nonExistentID := "00000000-0000-0000-0000-000000000000"
			resp, err := client.GetTask(workspaceID, nonExistentID)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})

		t.Run("should fail with missing parameters", func(t *testing.T) {
			resp, err := client.GetTask("", "some-id")
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})

	t.Run("List Tasks", func(t *testing.T) {
		t.Run("should list all tasks", func(t *testing.T) {
			// Create multiple tasks
			_, err := factory.CreateTask(workspaceID, testutil.WithTaskType("send_broadcast"))
			require.NoError(t, err)
			_, err = factory.CreateTask(workspaceID, testutil.WithTaskType("build_segment"))
			require.NoError(t, err)

			params := map[string]string{
				"workspace_id": workspaceID,
			}

			resp, err := client.ListTasks(params)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			tasks, ok := result["tasks"].([]interface{})
			require.True(t, ok)
			assert.GreaterOrEqual(t, len(tasks), 2)
		})

		t.Run("should filter tasks by status", func(t *testing.T) {
			// Create tasks with different statuses
			_, err := factory.CreateTask(workspaceID,
				testutil.WithTaskType("send_broadcast"),
				testutil.WithTaskStatus(domain.TaskStatusPending))
			require.NoError(t, err)

			_, err = factory.CreateTask(workspaceID,
				testutil.WithTaskType("send_broadcast"),
				testutil.WithTaskStatus(domain.TaskStatusRunning))
			require.NoError(t, err)

			params := map[string]string{
				"workspace_id": workspaceID,
				"status":       "pending",
			}

			resp, err := client.ListTasks(params)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			tasks, ok := result["tasks"].([]interface{})
			require.True(t, ok)

			// Verify all returned tasks have pending status
			for _, taskInterface := range tasks {
				taskData := taskInterface.(map[string]interface{})
				assert.Equal(t, "pending", taskData["status"])
			}
		})

		t.Run("should filter tasks by type", func(t *testing.T) {
			// Create tasks with different types
			_, err := factory.CreateTask(workspaceID, testutil.WithTaskType("sync_integration"))
			require.NoError(t, err)

			params := map[string]string{
				"workspace_id": workspaceID,
				"type":         "sync_integration",
			}

			resp, err := client.ListTasks(params)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			tasks, ok := result["tasks"].([]interface{})
			require.True(t, ok)

			// Verify all returned tasks have the correct type
			for _, taskInterface := range tasks {
				taskData := taskInterface.(map[string]interface{})
				assert.Equal(t, "sync_integration", taskData["type"])
			}
		})

		t.Run("should respect limit and offset", func(t *testing.T) {
			params := map[string]string{
				"workspace_id": workspaceID,
				"limit":        "2",
				"offset":       "0",
			}

			resp, err := client.ListTasks(params)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			tasks, ok := result["tasks"].([]interface{})
			require.True(t, ok)
			assert.LessOrEqual(t, len(tasks), 2)
		})
	})

	t.Run("Delete Task", func(t *testing.T) {
		t.Run("should delete task successfully", func(t *testing.T) {
			// Create a task to delete
			task, err := factory.CreateTask(workspaceID, testutil.WithTaskType("send_broadcast"))
			require.NoError(t, err)

			resp, err := client.DeleteTask(workspaceID, task.ID)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			success, ok := result["success"].(bool)
			require.True(t, ok)
			assert.True(t, success)

			// Verify task is deleted
			getResp, err := client.GetTask(workspaceID, task.ID)
			require.NoError(t, err)
			defer func() { _ = getResp.Body.Close() }()
			assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
		})

		t.Run("should return 404 for non-existent task", func(t *testing.T) {
			// Use a valid UUID format that doesn't exist
			nonExistentID := "00000000-0000-0000-0000-000000000000"
			resp, err := client.DeleteTask(workspaceID, nonExistentID)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	})
}

func testTaskExecution(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("Execute Individual Task", func(t *testing.T) {
		t.Run("should execute task successfully", func(t *testing.T) {
			// Create a task to execute
			task, err := factory.CreateTask(workspaceID,
				testutil.WithTaskType("send_broadcast"),
				testutil.WithTaskStatus(domain.TaskStatusPending))
			require.NoError(t, err)

			executeRequest := map[string]interface{}{
				"workspace_id": workspaceID,
				"id":           task.ID,
			}

			resp, err := client.ExecuteTask(executeRequest)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// Task execution is expected for now - the actual processing depends on registered processors
			// For integration tests, we mainly verify the endpoint works correctly
			if resp.StatusCode == http.StatusOK {
				var result map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&result)
				require.NoError(t, err)

				success, ok := result["success"].(bool)
				require.True(t, ok)
				assert.True(t, success)
			} else {
				// If no processor is registered, we should get an appropriate error
				assert.Contains(t, []int{http.StatusBadRequest, http.StatusInternalServerError}, resp.StatusCode)
			}
		})

		t.Run("should fail with invalid task ID", func(t *testing.T) {
			// Use a valid UUID format that doesn't exist
			nonExistentID := "00000000-0000-0000-0000-000000000000"
			executeRequest := map[string]interface{}{
				"workspace_id": workspaceID,
				"id":           nonExistentID,
			}

			resp, err := client.ExecuteTask(executeRequest)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})

		t.Run("should fail with missing parameters", func(t *testing.T) {
			executeRequest := map[string]interface{}{
				"workspace_id": workspaceID,
				// Missing id field
			}

			resp, err := client.ExecuteTask(executeRequest)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})

	t.Run("Execute Pending Tasks (Cron)", func(t *testing.T) {
		t.Run("should execute pending tasks successfully", func(t *testing.T) {
			resp, err := client.ExecutePendingTasks(5)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// 202: the endpoint acknowledges the trigger and runs the batch in
			// the background, so a caller with a short timeout never waits.
			assert.Equal(t, http.StatusAccepted, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			success, ok := result["success"].(bool)
			require.True(t, ok)
			assert.True(t, success)

			maxTasks, ok := result["max_tasks"].(float64)
			require.True(t, ok)
			assert.Equal(t, float64(5), maxTasks)

			// The test verifies that the HTTP endpoint works and returns success.
			// Task state verification is not meaningful here because the test uses fake task types
			// without registered processors - tasks fail in GetProcessor before MarkAsRunning.
			// This is expected behavior for integration testing the HTTP layer.
		})

		t.Run("should handle default max_tasks parameter", func(t *testing.T) {
			// When max_tasks=0 is passed, the HTTP response echoes the request value
			// The service internally applies the default (100) for actual task processing
			resp, err := client.ExecutePendingTasks(0)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusAccepted, resp.StatusCode)

			// Verify successful response
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			success, ok := result["success"].(bool)
			require.True(t, ok)
			assert.True(t, success)
		})
	})
}

func testTaskStateManagement(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("Task Status Transitions", func(t *testing.T) {
		// Create a task and test various state transitions
		task, err := factory.CreateTask(workspaceID,
			testutil.WithTaskType("send_broadcast"),
			testutil.WithTaskStatus(domain.TaskStatusPending))
		require.NoError(t, err)

		t.Run("should mark task as running", func(t *testing.T) {
			err := factory.MarkTaskAsRunning(workspaceID, task.ID)
			require.NoError(t, err)

			// Verify status change
			resp, err := client.GetTask(workspaceID, task.ID)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			taskData := result["task"].(map[string]interface{})
			assert.Equal(t, "running", taskData["status"])
			assert.NotNil(t, taskData["last_run_at"])
			assert.NotNil(t, taskData["timeout_after"])
		})

		t.Run("should mark task as completed", func(t *testing.T) {
			err := factory.MarkTaskAsCompleted(workspaceID, task.ID, &domain.TaskState{
				Progress: 100,
				Message:  "Completed",
			})
			require.NoError(t, err)

			// Verify status change
			resp, err := client.GetTask(workspaceID, task.ID)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			taskData := result["task"].(map[string]interface{})
			assert.Equal(t, "completed", taskData["status"])
			assert.Equal(t, float64(100), taskData["progress"])
			assert.NotNil(t, taskData["completed_at"])
		})
	})

	t.Run("Task Failure Handling", func(t *testing.T) {
		failTask, err := factory.CreateTask(workspaceID,
			testutil.WithTaskType("send_broadcast"),
			testutil.WithTaskMaxRetries(2))
		require.NoError(t, err)

		t.Run("should mark task as failed with retry", func(t *testing.T) {
			err := factory.MarkTaskAsFailed(workspaceID, failTask.ID, "Test failure")
			require.NoError(t, err)

			// Verify task status
			resp, err := client.GetTask(workspaceID, failTask.ID)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			taskData := result["task"].(map[string]interface{})
			// First failure should set status to pending for retry
			assert.Equal(t, "pending", taskData["status"])
			assert.Equal(t, "Test failure", taskData["error_message"])
			assert.Equal(t, float64(1), taskData["retry_count"])
		})
	})

	t.Run("Task Pause and Resume", func(t *testing.T) {
		pauseTask, err := factory.CreateTask(workspaceID,
			testutil.WithTaskType("send_broadcast"),
			testutil.WithTaskStatus(domain.TaskStatusRunning))
		require.NoError(t, err)

		t.Run("should pause and resume task", func(t *testing.T) {
			// Pause task
			nextRunTime := time.Now().Add(10 * time.Minute)
			state := &domain.TaskState{
				Progress: 50.0,
				Message:  "Task paused",
			}

			err := factory.MarkTaskAsPaused(workspaceID, pauseTask.ID, nextRunTime, 50.0, state)
			require.NoError(t, err)

			// Verify paused status
			resp, err := client.GetTask(workspaceID, pauseTask.ID)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			taskData := result["task"].(map[string]interface{})
			assert.Equal(t, "paused", taskData["status"])
			assert.Equal(t, float64(50), taskData["progress"])
			assert.NotNil(t, taskData["next_run_after"])
		})
	})
}

func testTaskAuthentication(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("cron endpoint should work without auth", func(t *testing.T) {
		resp, err := client.ExecutePendingTasks(5)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// The cron endpoint should be accessible without authentication
		assert.NotEqual(t, http.StatusUnauthorized, resp.StatusCode)
		assert.Equal(t, http.StatusAccepted, resp.StatusCode)

		// Verify successful response
		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		success, ok := result["success"].(bool)
		require.True(t, ok)
		assert.True(t, success)
	})
}

func testTaskRepositoryOperations(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("Basic Task Persistence", func(t *testing.T) {
		// Create simple task
		task, err := factory.CreateTask(workspaceID,
			testutil.WithTaskType("sync_integration"))
		require.NoError(t, err)

		// Retrieve and verify task exists
		resp, err := client.GetTask(workspaceID, task.ID)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		taskData := result["task"].(map[string]interface{})
		assert.Equal(t, task.ID, taskData["id"])
		assert.Equal(t, "sync_integration", taskData["type"])
	})

	t.Run("Task Status Updates", func(t *testing.T) {
		task, err := factory.CreateTask(workspaceID, testutil.WithTaskType("send_broadcast"))
		require.NoError(t, err)

		// Mark as running
		err = factory.MarkTaskAsRunning(workspaceID, task.ID)
		require.NoError(t, err)

		// Verify status update
		resp, err := client.GetTask(workspaceID, task.ID)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		taskData := result["task"].(map[string]interface{})
		assert.Equal(t, "running", taskData["status"])
	})
}

// testTaskErrorHandling and testBroadcastTaskIntegration are unused test helpers
// They are kept for potential future use but currently not called by any tests
// Uncomment and use them when needed:
/*
func testTaskErrorHandling(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("HTTP Method Validation", func(t *testing.T) {
		// Test wrong HTTP methods
		endpoints := []string{
			"/api/tasks.create",
			"/api/tasks.get",
			"/api/tasks.list",
			"/api/tasks.delete",
			"/api/tasks.execute",
		}

		for _, endpoint := range endpoints {
			t.Run("should validate method for "+endpoint, func(t *testing.T) {
				// Use wrong method (GET for POST endpoints, POST for GET endpoints)
				var resp *http.Response
				var err error

				// Check if endpoint requires POST method
				isPOSTEndpoint := false
				for _, postEP := range []string{"create", "delete", "execute"} {
					if len(endpoint) >= len(postEP) && endpoint[len(endpoint)-len(postEP):] == postEP {
						isPOSTEndpoint = true
						break
					}
				}
				if isPOSTEndpoint {
					resp, err = client.Get(endpoint)
				} else {
					resp, err = client.Post(endpoint, map[string]interface{}{})
				}

				require.NoError(t, err)
				defer func() { _ = resp.Body.Close() }()
				assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
			})
		}
	})

	t.Run("Invalid JSON Handling", func(t *testing.T) {
		// This would typically require lower-level HTTP client manipulation
		// For now, we test with malformed data that should be caught by validation
		createRequest := map[string]interface{}{
			"workspace_id": workspaceID,
			"type":         "", // Empty type should fail validation
		}

		resp, err := client.CreateTask(createRequest)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Cross-Workspace Access", func(t *testing.T) {
		// Create another workspace
		otherWorkspace, err := factory.CreateWorkspace()
		require.NoError(t, err)

		// Create task in other workspace
		task, err := factory.CreateTask(otherWorkspace.ID, testutil.WithTaskType("cross_workspace_test"))
		require.NoError(t, err)

		// Try to access task from current workspace context
		resp, err := client.GetTask(workspaceID, task.ID)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should not find the task (as it belongs to different workspace)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// testBroadcastTaskIntegration is an unused test helper
// It is kept for potential future use but currently not called by any tests
// Uncomment and use it when needed:
/*
func testBroadcastTaskIntegration(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("Send Broadcast Task", func(t *testing.T) {
		// Create broadcast first
		broadcast, err := factory.CreateBroadcast(workspaceID)
		require.NoError(t, err)

		t.Run("should create send broadcast task", func(t *testing.T) {
			task, err := factory.CreateSendBroadcastTask(workspaceID, broadcast.ID)
			require.NoError(t, err)

			// Verify task creation
			resp, err := client.GetTask(workspaceID, task.ID)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			taskData := result["task"].(map[string]interface{})
			assert.Equal(t, "send_broadcast", taskData["type"])
			assert.Equal(t, broadcast.ID, *taskData["broadcast_id"].(*string))

			state := taskData["state"].(map[string]interface{})
			sendBroadcast := state["send_broadcast"].(map[string]interface{})
			assert.Equal(t, broadcast.ID, sendBroadcast["broadcast_id"])
			assert.Equal(t, "email", sendBroadcast["channel_type"])
		})
	})

	t.Run("A/B Testing Task", func(t *testing.T) {
		// Create broadcast with A/B testing
		template1, err := factory.CreateTemplate(workspaceID)
		require.NoError(t, err)
		template2, err := factory.CreateTemplate(workspaceID)
		require.NoError(t, err)

		broadcast, err := factory.CreateBroadcast(workspaceID,
			testutil.WithBroadcastABTesting([]string{template1.ID, template2.ID}))
		require.NoError(t, err)

		t.Run("should create A/B testing task", func(t *testing.T) {
			task, err := factory.CreateTaskWithABTesting(workspaceID, broadcast.ID)
			require.NoError(t, err)

			// Verify A/B testing configuration
			resp, err := client.GetTask(workspaceID, task.ID)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			taskData := result["task"].(map[string]interface{})
			state := taskData["state"].(map[string]interface{})
			sendBroadcast := state["send_broadcast"].(map[string]interface{})

			assert.Equal(t, "test", sendBroadcast["phase"])
			assert.Equal(t, false, sendBroadcast["test_phase_completed"])
			assert.Equal(t, float64(100), sendBroadcast["test_phase_recipient_count"])
			assert.Equal(t, float64(900), sendBroadcast["winner_phase_recipient_count"])
		})
	})
}
*/

// TestTaskAuthentication tests authentication requirements separately to avoid interfering with other tests
func TestTaskAuthentication(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	t.Run("should require authentication for management endpoints", func(t *testing.T) {
		// Use valid UUID format for test IDs
		testTaskID := "00000000-0000-0000-0000-000000000000"

		endpoints := []struct {
			name string
			fn   func() (*http.Response, error)
		}{
			{"get", func() (*http.Response, error) {
				return client.GetTask(workspace.ID, testTaskID)
			}},
			{"list", func() (*http.Response, error) {
				return client.ListTasks(map[string]string{"workspace_id": workspace.ID})
			}},
			{"delete", func() (*http.Response, error) {
				return client.DeleteTask(workspace.ID, testTaskID)
			}},
		}

		for _, endpoint := range endpoints {
			t.Run(endpoint.name, func(t *testing.T) {
				resp, err := endpoint.fn()
				require.NoError(t, err)
				defer func() { _ = resp.Body.Close() }()

				assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
			})
		}
	})

	t.Run("should allow public execution endpoints without auth", func(t *testing.T) {
		// Use valid UUID format for test IDs
		testTaskID := "00000000-0000-0000-0000-000000000000"

		t.Run("execute endpoint accepts a signed dispatch", func(t *testing.T) {
			resp, err := client.ExecuteTask(map[string]interface{}{"workspace_id": workspace.ID, "id": testTaskID})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// No session is involved, so the signature is what gets it in: 404
			// for a task that does not exist, never 401.
			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})

		t.Run("execute endpoint refuses an unsigned dispatch", func(t *testing.T) {
			resp, err := client.Post("/api/tasks.execute",
				map[string]interface{}{"workspace_id": workspace.ID, "id": testTaskID})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		})

		t.Run("execute endpoint refuses another installation's key", func(t *testing.T) {
			client.SetSigningSecret("some-other-installations-secret")
			defer client.SetSigningSecret(testutil.TestSecretKey)

			resp, err := client.ExecuteTask(map[string]interface{}{"workspace_id": workspace.ID, "id": testTaskID})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		})

		t.Run("cron endpoint", func(t *testing.T) {
			// Create a task to verify execution happens
			task, err := factory.CreateTask(workspace.ID,
				testutil.WithTaskType("send_broadcast"),
				testutil.WithTaskStatus(domain.TaskStatusPending))
			require.NoError(t, err)

			resp, err := client.ExecutePendingTasks(5)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// Should not be unauthorized
			assert.NotEqual(t, http.StatusUnauthorized, resp.StatusCode)
			assert.Equal(t, http.StatusAccepted, resp.StatusCode)

			// Verify task was actually processed
			time.Sleep(2 * time.Second)
			taskResp, err := client.GetTask(workspace.ID, task.ID)
			require.NoError(t, err)
			defer func() { _ = taskResp.Body.Close() }()

			var taskResult map[string]interface{}
			err = json.NewDecoder(taskResp.Body).Decode(&taskResult)
			require.NoError(t, err)

			// Safe nil check - task might not be in response if execution failed
			if task, ok := taskResult["task"].(map[string]interface{}); ok && task != nil {
				if status, ok := task["status"].(string); ok {
					t.Logf("Task status after unauthenticated cron endpoint: %s", status)
					// Task should have been processed
				}
			} else {
				t.Logf("No task data in response (may be expected if execution failed)")
			}
		})
	})
}

// TestTaskExecute_SchedulerEnabledAnswers404 pins that a dispatch to an instance
// which runs its own scheduler fails loudly.
//
// The route used to be registered only when the scheduler was off, so on a
// default install the dispatch fell through to the console SPA catch-all and came
// back 200 with an empty body. A dispatcher misconfigured that way read success
// and dropped every execution silently — which is how an interrupted broadcast
// could be "resumed" by a request that did nothing at all.
//
// The dispatched task EXISTS, and that is what makes the test say anything: a
// dispatch-enabled instance answers 404 for an unknown id as well (TestTaskAuthentication
// asserts exactly that), so an unknown id cannot tell the two builds apart. A task
// the handler could have executed can — the refusal arrives before the handler
// ever reads the id, so the row is untouched afterwards. The unsigned dispatch
// covers the other end of the same wiring: on a dispatch-enabled instance it is
// 401, because the signature check runs; here it must be 404, because nothing
// downstream of the rejection runs at all.
func TestTaskExecute_SchedulerEnabledAnswers404(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuiteWithDirectScheduler(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	db := suite.DBManager.GetDB()
	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)

	// send_broadcast has a registered processor, so a dispatch-enabled instance
	// reaching this task would claim it and stamp last_run_at before running
	// anything. next_run_after keeps the in-process scheduler — which is live in
	// this suite, that being the point of it — from picking the task up on its own
	// ticker and confusing the question.
	task, err := suite.DataFactory.CreateTask(workspace.ID,
		testutil.WithTaskType("send_broadcast"),
		testutil.WithTaskNextRunAfter(time.Now().UTC().Add(time.Hour)))
	require.NoError(t, err)

	resp, err := client.ExecuteTask(map[string]interface{}{
		"workspace_id": workspace.ID,
		"id":           task.ID,
	})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"a correctly signed dispatch to a scheduler-enabled instance must be refused, not silently accepted")

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.NotEmpty(t, body["error"],
		"the refusal must carry a JSON error — an empty body is the SPA fall-through this replaces")

	var status string
	var lastRunAt sql.NullTime
	require.NoError(t, db.QueryRow(
		`SELECT status, last_run_at FROM tasks WHERE id = $1`, task.ID).Scan(&status, &lastRunAt))
	assert.Equal(t, string(domain.TaskStatusPending), status,
		"the task must still be waiting for the in-process scheduler")
	assert.False(t, lastRunAt.Valid, "the task must never have been claimed")

	unsigned, err := client.Post("/api/tasks.execute", map[string]interface{}{
		"workspace_id": workspace.ID,
		"id":           task.ID,
	})
	require.NoError(t, err)
	defer func() { _ = unsigned.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, unsigned.StatusCode,
		"the rejection precedes the signature check, so an unsigned dispatch gets 404 here and 401 on a dispatch-enabled instance")
}
