package testutil

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/stretchr/testify/require"
)

// IntegrationTestSuite provides a complete testing environment
type IntegrationTestSuite struct {
	DBManager     *DatabaseManager
	ServerManager *ServerManager
	APIClient     *APIClient
	DataFactory   *TestDataFactory
	Config        *config.Config
	T             *testing.T
}

// NewIntegrationTestSuite creates a new integration test suite
func NewIntegrationTestSuite(t *testing.T, appFactory func(*config.Config) AppInterface) *IntegrationTestSuite {
	// Skip if not running integration tests
	if os.Getenv("INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TESTS=true to run.")
	}

	suite := &IntegrationTestSuite{T: t}

	// Setup database
	suite.DBManager = NewDatabaseManager()
	err := suite.DBManager.Setup()
	require.NoError(t, err, "Failed to setup test database")

	// Wait for database to be ready
	err = suite.DBManager.WaitForDatabase(30)
	require.NoError(t, err, "Database not ready")

	// Setup server
	suite.ServerManager = NewServerManager(appFactory, suite.DBManager)
	err = suite.ServerManager.Start()
	require.NoError(t, err, "Failed to start test server")

	// Setup API client
	suite.APIClient = NewAPIClient(suite.ServerManager.GetURL())

	// Setup data factory with repositories from the app
	app := suite.ServerManager.GetApp()
	suite.DataFactory = NewTestDataFactory(
		suite.DBManager.GetDB(),
		app.GetUserRepository(),
		app.GetWorkspaceRepository(),
		app.GetContactRepository(),
		app.GetListRepository(),
		app.GetTemplateRepository(),
		app.GetBroadcastRepository(),
		app.GetMessageHistoryRepository(),
		app.GetContactListRepository(),
		app.GetTransactionalNotificationRepository(),
	)

	// Seed initial test data
	err = suite.DBManager.SeedTestData()
	require.NoError(t, err, "Failed to seed test data")

	// Set workspace ID for API client
	suite.APIClient.SetWorkspaceID("test-workspace-id")

	suite.Config = suite.ServerManager.GetApp().GetConfig()

	return suite
}

// Cleanup cleans up all test resources
func (s *IntegrationTestSuite) Cleanup() {
	if s.ServerManager != nil {
		s.ServerManager.Stop()
	}
	if s.DBManager != nil {
		s.DBManager.Cleanup()
	}
}

// ResetData cleans and reseeds test data
func (s *IntegrationTestSuite) ResetData() {
	err := s.DBManager.CleanupTestData()
	require.NoError(s.T, err, "Failed to cleanup test data")

	err = s.DBManager.SeedTestData()
	require.NoError(s.T, err, "Failed to seed test data")
}

// WaitForBroadcastCompletion waits for a broadcast to reach a terminal state
// Returns the final broadcast status or error if timeout/failure occurs
func WaitForBroadcastCompletion(t *testing.T, client *APIClient, broadcastID string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	checkInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		resp, err := client.GetBroadcast(broadcastID)
		if err != nil {
			return "", fmt.Errorf("failed to get broadcast: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return "", fmt.Errorf("unexpected status code %d when getting broadcast", resp.StatusCode)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", fmt.Errorf("failed to decode broadcast response: %w", err)
		}

		broadcastData, ok := result["broadcast"].(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("invalid broadcast response format")
		}

		status, ok := broadcastData["status"].(string)
		if !ok {
			return "", fmt.Errorf("broadcast status not found or invalid type")
		}

		// Check for terminal states
		switch status {
		case "sent", "completed":
			return status, nil // Success!
		case "failed", "cancelled":
			return status, fmt.Errorf("broadcast reached terminal state: %s", status)
		case "draft", "scheduled", "sending", "testing", "test_completed", "paused", "winner_selected":
			// Still in progress, keep waiting
		default:
			t.Logf("Unknown broadcast status: %s, continuing to wait", status)
		}

		time.Sleep(checkInterval)
	}

	return "", fmt.Errorf("timeout waiting for broadcast completion after %v", timeout)
}

// WaitForCondition waits for a condition to be true within a timeout
func WaitForCondition(t *testing.T, condition func() bool, timeout time.Duration, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for condition: %s", message)
		case <-ticker.C:
			if condition() {
				return
			}
		}
	}
}

// GenerateRandomString generates a random string of specified length
func GenerateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback to a deterministic approach if random fails
		for i := range b {
			b[i] = charset[i%len(charset)]
		}
	} else {
		for i := range b {
			b[i] = charset[b[i]%byte(len(charset))]
		}
	}
	return string(b)
}

// GenerateTestEmail generates a test email address
func GenerateTestEmail() string {
	return fmt.Sprintf("test-%s@example.com", GenerateRandomString(8))
}

// CreateTestLogger creates a logger for testing
func CreateTestLogger() logger.Logger {
	return logger.NewLogger()
}

// AssertEventuallyTrue asserts that a condition becomes true within a timeout
func AssertEventuallyTrue(t *testing.T, condition func() bool, timeout time.Duration, message string) {
	require.Eventually(t, condition, timeout, 100*time.Millisecond, message)
}

// AssertNeverTrue asserts that a condition never becomes true within a duration
func AssertNeverTrue(t *testing.T, condition func() bool, duration time.Duration, message string) {
	require.Never(t, condition, duration, 100*time.Millisecond, message)
}

// SkipIfShort skips the test if running in short mode
func SkipIfShort(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
}

// RequireEnvironmentVar requires an environment variable to be set
func RequireEnvironmentVar(t *testing.T, envVar string) string {
	value := os.Getenv(envVar)
	if value == "" {
		t.Fatalf("Required environment variable %s is not set", envVar)
	}
	return value
}

// SetupTestEnvironment sets up environment variables for testing
func SetupTestEnvironment() {
	// Don't set TEST_DB_HOST here - let it use the default or be set externally
	// This allows for flexibility between local and containerized environments
	// os.Setenv("TEST_DB_HOST", "localhost") // Default handled in connection_pool.go
	// os.Setenv("TEST_DB_PORT", "5433")      // Default handled in connection_pool.go
	os.Setenv("TEST_DB_USER", "notifuse_test")
	os.Setenv("TEST_DB_PASSWORD", "test_password")
	os.Setenv("ENVIRONMENT", "test")
}

// CleanupTestEnvironment cleans up test environment variables and connections
func CleanupTestEnvironment() {
	// Clean up the global connection pool to prevent connection leaks between tests
	CleanupAllTestConnections()

	os.Unsetenv("TEST_DB_HOST")
	os.Unsetenv("TEST_DB_PORT")
	os.Unsetenv("TEST_DB_USER")
	os.Unsetenv("TEST_DB_PASSWORD")
	os.Unsetenv("ENVIRONMENT")
}

// CleanupAllTestConnections cleans up the global connection pool
// This should be called at the end of test runs to ensure no connections leak
func CleanupAllTestConnections() error {
	return CleanupGlobalTestPool()
}

// GetTestConnectionCount returns the current number of active test connections
func GetTestConnectionCount() int {
	pool := GetGlobalTestPool()
	return pool.GetConnectionCount()
}

// WaitAndExecuteTasks is a helper method for A/B testing integration tests
// It executes pending tasks multiple times with delays to simulate real task execution
func WaitAndExecuteTasks(client *APIClient, rounds int, delayBetweenRounds time.Duration) error {
	for i := 0; i < rounds; i++ {
		if i > 0 {
			time.Sleep(delayBetweenRounds)
		}

		resp, err := client.ExecutePendingTasks(10)
		if err != nil {
			return fmt.Errorf("failed to execute tasks on round %d: %w", i+1, err)
		}
		resp.Body.Close()
	}
	return nil
}

// WaitForBroadcastStatus polls a broadcast until it reaches one of the expected statuses
// This is useful for A/B testing scenarios where we need to wait for phase transitions
// Returns the actual status reached, or error if timeout or failure occurs
func WaitForBroadcastStatus(t *testing.T, client *APIClient, broadcastID string, acceptableStatuses []string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		resp, err := client.GetBroadcast(broadcastID)
		if err != nil {
			return "", fmt.Errorf("failed to get broadcast: %w", err)
		}

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if err != nil {
			return "", fmt.Errorf("failed to decode broadcast response: %w", err)
		}

		if broadcastData, ok := result["broadcast"].(map[string]interface{}); ok {
			if status, ok := broadcastData["status"].(string); ok {
				// Log current status for debugging
				t.Logf("Broadcast %s current status: %s", broadcastID, status)

				// Check if we've reached an acceptable status
				for _, acceptable := range acceptableStatuses {
					if status == acceptable {
						return status, nil
					}
				}

				// Check for failure states
				if status == "failed" || status == "cancelled" {
					return status, fmt.Errorf("broadcast reached terminal failure state: %s", status)
				}
			}
		}

		time.Sleep(pollInterval)
	}

	return "", fmt.Errorf("timeout waiting for broadcast to reach status %v after %v", acceptableStatuses, timeout)
}

// WaitForBroadcastStatusWithExecution waits for a broadcast to reach one of the acceptable statuses
// while continuously executing pending tasks. This is the recommended helper for A/B testing flows
// that require task orchestration to complete.
//
// This function differs from WaitForBroadcastStatus by actively executing tasks during the wait,
// which is necessary for broadcasts that need continuous task processing to transition through phases.
//
// Parameters:
//   - t: testing context for logging
//   - client: API client for making requests
//   - broadcastID: ID of the broadcast to monitor
//   - acceptableStatuses: list of statuses that indicate success
//   - timeout: maximum time to wait
//
// Returns the final status reached or an error if timeout occurs.
func WaitForBroadcastStatusWithExecution(t *testing.T, client *APIClient, broadcastID string, acceptableStatuses []string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 1 * time.Second
	taskExecutionInterval := 2 * time.Second
	lastTaskExecution := time.Now()

	t.Logf("Starting WaitForBroadcastStatusWithExecution for broadcast %s (timeout: %v)", broadcastID, timeout)
	t.Logf("Acceptable statuses: %v", acceptableStatuses)

	iterationCount := 0
	taskExecutionCount := 0

	for time.Now().Before(deadline) {
		iterationCount++

		// Execute pending tasks periodically
		if time.Since(lastTaskExecution) >= taskExecutionInterval {
			taskExecutionCount++
			t.Logf("Executing pending tasks (cycle %d)", taskExecutionCount)

			execResp, err := client.ExecutePendingTasks(10)
			if err != nil {
				t.Logf("Warning: ExecutePendingTasks failed: %v", err)
			} else {
				execResp.Body.Close()
				t.Logf("Task execution completed successfully")
			}

			lastTaskExecution = time.Now()

			// Give tasks time to process
			time.Sleep(500 * time.Millisecond)
		}

		// Check broadcast status
		resp, err := client.GetBroadcast(broadcastID)
		if err != nil {
			return "", fmt.Errorf("failed to get broadcast: %w", err)
		}

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if err != nil {
			return "", fmt.Errorf("failed to decode broadcast response: %w", err)
		}

		if broadcastData, ok := result["broadcast"].(map[string]interface{}); ok {
			if status, ok := broadcastData["status"].(string); ok {
				// Log current status every few iterations
				if iterationCount%3 == 1 {
					t.Logf("Broadcast %s current status: %s (iteration %d)", broadcastID, status, iterationCount)
				}

				// Check if we've reached an acceptable status
				for _, acceptable := range acceptableStatuses {
					if status == acceptable {
						t.Logf("✓ Broadcast reached acceptable status '%s' after %d iterations and %d task executions",
							status, iterationCount, taskExecutionCount)
						return status, nil
					}
				}

				// Check for failure states
				if status == "failed" || status == "cancelled" {
					// Get diagnostic info
					phase := ""
					progress := 0.0
					if state, ok := broadcastData["state"].(map[string]interface{}); ok {
						if phaseVal, ok := state["phase"].(string); ok {
							phase = phaseVal
						}
						if progressVal, ok := state["progress"].(float64); ok {
							progress = progressVal
						}
					}

					return status, fmt.Errorf("broadcast reached terminal failure state: %s (phase: %s, progress: %.1f%%)",
						status, phase, progress*100)
				}
			}
		}

		time.Sleep(pollInterval)
	}

	// Timeout - gather diagnostic information
	resp, err := client.GetBroadcast(broadcastID)
	if err == nil {
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
			if broadcastData, ok := result["broadcast"].(map[string]interface{}); ok {
				status, _ := broadcastData["status"].(string)

				// Extract detailed state info
				phase := "unknown"
				progress := 0.0
				sentCount := 0
				failedCount := 0
				totalRecipients := 0

				if state, ok := broadcastData["state"].(map[string]interface{}); ok {
					if phaseVal, ok := state["phase"].(string); ok {
						phase = phaseVal
					}
					if progressVal, ok := state["progress"].(float64); ok {
						progress = progressVal
					}
				}

				if sentCountVal, ok := broadcastData["sent_count"].(float64); ok {
					sentCount = int(sentCountVal)
				}
				if failedCountVal, ok := broadcastData["failed_count"].(float64); ok {
					failedCount = int(failedCountVal)
				}
				if totalVal, ok := broadcastData["total_recipients"].(float64); ok {
					totalRecipients = int(totalVal)
				}

				t.Logf("TIMEOUT DIAGNOSTICS for broadcast %s:", broadcastID)
				t.Logf("  Current status: %s", status)
				t.Logf("  Phase: %s", phase)
				t.Logf("  Progress: %.1f%%", progress*100)
				t.Logf("  Recipients: %d sent, %d failed, %d total", sentCount, failedCount, totalRecipients)
				t.Logf("  Iterations: %d", iterationCount)
				t.Logf("  Task executions: %d", taskExecutionCount)
				t.Logf("  Expected statuses: %v", acceptableStatuses)
			}
		}
		resp.Body.Close()
	}

	return "", fmt.Errorf("timeout waiting for broadcast to reach status %v after %v (executed %d task cycles)",
		acceptableStatuses, timeout, taskExecutionCount)
}

// VerifyBroadcastWinnerTemplate checks that a broadcast has the expected winning template
func VerifyBroadcastWinnerTemplate(client *APIClient, broadcastID, expectedTemplateID string) error {
	resp, err := client.GetBroadcast(broadcastID)
	if err != nil {
		return fmt.Errorf("failed to get broadcast: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return fmt.Errorf("failed to decode broadcast response: %w", err)
	}

	broadcastData, ok := result["broadcast"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("broadcast data not found in response")
	}

	winningTemplate, ok := broadcastData["winning_template"]
	if !ok || winningTemplate == nil {
		return fmt.Errorf("winning_template not set")
	}

	if winningTemplate.(string) != expectedTemplateID {
		return fmt.Errorf("expected winning template %s, got %s", expectedTemplateID, winningTemplate.(string))
	}

	return nil
}

// WaitForTaskCompletion waits for a task to reach a terminal state (completed, failed, or cancelled)
// Returns the final task status and any error that occurred
func WaitForTaskCompletion(t *testing.T, client *APIClient, workspaceID, taskID string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		resp, err := client.GetTask(workspaceID, taskID)
		if err != nil {
			return "", fmt.Errorf("failed to get task: %w", err)
		}

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if err != nil {
			return "", fmt.Errorf("failed to decode task response: %w", err)
		}

		if taskData, ok := result["task"].(map[string]interface{}); ok {
			if status, ok := taskData["status"].(string); ok {
				t.Logf("Task %s current status: %s", taskID, status)

				// Check for terminal states
				switch status {
				case "completed":
					return status, nil // Success!
				case "failed":
					errorMsg := ""
					if errMsg, ok := taskData["error_message"].(string); ok {
						errorMsg = errMsg
					}
					return status, fmt.Errorf("task failed: %s", errorMsg)
				case "cancelled":
					return status, fmt.Errorf("task was cancelled")
				case "pending", "running", "paused":
					// Still in progress, keep waiting
				default:
					t.Logf("Unknown task status: %s, continuing to wait", status)
				}
			}
		}

		time.Sleep(pollInterval)
	}

	return "", fmt.Errorf("timeout waiting for task completion after %v", timeout)
}

// VerifyTasksProcessed checks that tasks in the given list were attempted to be processed
// Returns a map of task IDs to their final status
func VerifyTasksProcessed(t *testing.T, client *APIClient, workspaceID string, taskIDs []string, timeout time.Duration) map[string]string {
	results := make(map[string]string)
	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond

	remainingTasks := make(map[string]bool)
	for _, id := range taskIDs {
		remainingTasks[id] = true
	}

	for time.Now().Before(deadline) && len(remainingTasks) > 0 {
		for taskID := range remainingTasks {
			resp, err := client.GetTask(workspaceID, taskID)
			if err != nil {
				t.Logf("Failed to get task %s: %v", taskID, err)
				continue
			}

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()

			if err != nil {
				t.Logf("Failed to decode task %s response: %v", taskID, err)
				continue
			}

			if taskData, ok := result["task"].(map[string]interface{}); ok {
				if status, ok := taskData["status"].(string); ok {
					// Task has been processed if it's no longer "pending"
					if status != "pending" {
						results[taskID] = status
						delete(remainingTasks, taskID)
						t.Logf("Task %s processed with status: %s", taskID, status)
					}
				}
			}
		}

		if len(remainingTasks) > 0 {
			time.Sleep(pollInterval)
		}
	}

	// Add any remaining tasks as "pending" (not processed)
	for taskID := range remainingTasks {
		results[taskID] = "pending"
		t.Logf("Task %s remained in pending state", taskID)
	}

	return results
}

// WaitForSegmentBuilt waits for a segment to reach "built" status
// Returns the final status or error if timeout/failure occurs
func WaitForSegmentBuilt(t *testing.T, client *APIClient, workspaceID, segmentID string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		// Execute tasks on each poll to ensure pending tasks are processed
		// This is important when tests run sequentially and tasks queue up
		execResp, err := client.Post("/api/tasks.execute", map[string]interface{}{"limit": 10})
		if err == nil {
			execResp.Body.Close()
		}

		resp, err := client.Get(fmt.Sprintf("/api/segments.get?workspace_id=%s&id=%s", workspaceID, segmentID))
		if err != nil {
			return "", fmt.Errorf("failed to get segment: %w", err)
		}

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if err != nil {
			return "", fmt.Errorf("failed to decode segment response: %w", err)
		}

		if segmentData, ok := result["segment"].(map[string]interface{}); ok {
			if status, ok := segmentData["status"].(string); ok {
				t.Logf("Segment %s current status: %s", segmentID, status)

				switch status {
				case "built", "active":
					return status, nil // Success! Segments become "active" after building
				case "failed":
					return status, fmt.Errorf("segment build failed")
				case "building", "pending":
					// Still in progress, keep waiting
				default:
					t.Logf("Unknown segment status: %s, continuing to wait", status)
				}
			}
		}

		time.Sleep(pollInterval)
	}

	return "", fmt.Errorf("timeout waiting for segment to build after %v", timeout)
}

// CleanupAllTasks deletes all tasks for a workspace
// This is useful for cleaning up evergreen tasks between tests
func CleanupAllTasks(t *testing.T, client *APIClient, workspaceID string) error {
	// List all tasks
	params := map[string]string{
		"workspace_id": workspaceID,
		"limit":        "1000", // High limit to get all tasks
	}

	resp, err := client.ListTasks(params)
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode task list: %w", err)
	}

	tasks, ok := result["tasks"].([]interface{})
	if !ok {
		return nil // No tasks to clean up
	}

	// Delete each task
	deletedCount := 0
	for _, taskInterface := range tasks {
		taskData := taskInterface.(map[string]interface{})
		taskID, ok := taskData["id"].(string)
		if !ok {
			continue
		}

		deleteResp, err := client.DeleteTask(workspaceID, taskID)
		if err != nil {
			t.Logf("Failed to delete task %s: %v", taskID, err)
			continue
		}
		deleteResp.Body.Close()
		deletedCount++
	}

	if deletedCount > 0 {
		t.Logf("Cleaned up %d tasks for workspace %s", deletedCount, workspaceID)
	}

	return nil
}

// WaitForBuildTaskCreated waits for a build_segment task to be created for a specific segment
// Returns the task ID or error if timeout occurs
func WaitForBuildTaskCreated(t *testing.T, client *APIClient, workspaceID, segmentID string, afterTime time.Time, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		resp, err := client.ListTasks(map[string]string{
			"workspace_id": workspaceID,
			"type":         "build_segment",
		})
		if err != nil {
			return "", fmt.Errorf("failed to list tasks: %w", err)
		}

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if err != nil {
			return "", fmt.Errorf("failed to decode tasks response: %w", err)
		}

		// Safe nil check for tasks array
		tasks, ok := result["tasks"].([]interface{})
		if !ok || tasks == nil {
			time.Sleep(pollInterval)
			continue
		}

		for _, taskInterface := range tasks {
			task := taskInterface.(map[string]interface{})

			// Check if this task is for our segment
			if state, ok := task["state"].(map[string]interface{}); ok {
				if buildSegment, ok := state["build_segment"].(map[string]interface{}); ok {
					if taskSegmentID, ok := buildSegment["segment_id"].(string); ok && taskSegmentID == segmentID {
						// Check if created after the specified time
						if createdAtStr, ok := task["created_at"].(string); ok {
							createdAt, err := time.Parse(time.RFC3339, createdAtStr)
							if err == nil && createdAt.After(afterTime) {
								taskID := task["id"].(string)
								t.Logf("Found build task %s for segment %s", taskID, segmentID)
								return taskID, nil
							}
						}
					}
				}
			}
		}

		time.Sleep(pollInterval)
	}

	return "", fmt.Errorf("timeout waiting for build task to be created for segment %s after %v", segmentID, timeout)
}

// MailhogMessage represents a simplified email message from Mailhog API
type MailhogMessage struct {
	ID   string `json:"ID"`
	From struct {
		Mailbox string `json:"Mailbox"`
		Domain  string `json:"Domain"`
	} `json:"From"`
	To []struct {
		Mailbox string `json:"Mailbox"`
		Domain  string `json:"Domain"`
	} `json:"To"`
	Content struct {
		Headers map[string][]string `json:"Headers"`
		Body    string              `json:"Body"`
	} `json:"Content"`
	Created time.Time `json:"Created"`
}

// MailhogAPIResponse represents the response from Mailhog's messages API
type MailhogAPIResponse struct {
	Total int              `json:"total"`
	Count int              `json:"count"`
	Start int              `json:"start"`
	Items []MailhogMessage `json:"items"`
}

// CheckMailhogForRecipients checks if an email was sent to all expected recipients via Mailhog
// Returns a map of recipient email addresses to whether they received the email
func CheckMailhogForRecipients(t *testing.T, subject string, expectedRecipients []string, timeout time.Duration) (map[string]bool, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond
	mailhogURL := "http://localhost:8025/api/v2/messages"

	results := make(map[string]bool)
	for _, recipient := range expectedRecipients {
		results[recipient] = false
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := httpClient.Get(mailhogURL)
		if err != nil {
			t.Logf("Failed to connect to Mailhog API: %v", err)
			time.Sleep(pollInterval)
			continue
		}

		var apiResp MailhogAPIResponse
		err = json.NewDecoder(resp.Body).Decode(&apiResp)
		resp.Body.Close()

		if err != nil {
			t.Logf("Failed to decode Mailhog response: %v", err)
			time.Sleep(pollInterval)
			continue
		}

		// Check each message for matching subject and recipients
		for _, msg := range apiResp.Items {
			subjectHeaders := msg.Content.Headers["Subject"]
			if len(subjectHeaders) == 0 {
				continue
			}

			// Check if this message matches our subject
			if !strings.Contains(subjectHeaders[0], subject) {
				continue
			}

			// Check To recipients
			for _, to := range msg.To {
				email := strings.ToLower(fmt.Sprintf("%s@%s", to.Mailbox, to.Domain))
				for _, expected := range expectedRecipients {
					if strings.ToLower(expected) == email {
						results[expected] = true
						t.Logf("Found email for recipient: %s", expected)
					}
				}
			}

			// Check CC recipients
			if ccHeaders, ok := msg.Content.Headers["Cc"]; ok {
				for _, ccHeader := range ccHeaders {
					email := strings.ToLower(extractEmailFromHeader(ccHeader))
					for _, expected := range expectedRecipients {
						if strings.ToLower(expected) == email {
							results[expected] = true
							t.Logf("Found email for CC recipient: %s", expected)
						}
					}
				}
			}

			// Note: BCC recipients won't appear in headers (that's the point of BCC)
			// but they should still receive the email, so we need to check individual messages
		}

		// Check if all recipients have been found
		allFound := true
		for _, found := range results {
			if !found {
				allFound = false
				break
			}
		}

		if allFound {
			return results, nil
		}

		time.Sleep(pollInterval)
	}

	// Return what we found even if not all recipients received the email
	return results, nil
}

// extractEmailFromHeader extracts email address from a header value like "Name <email@example.com>"
func extractEmailFromHeader(header string) string {
	// Check if email is in angle brackets
	start := strings.Index(header, "<")
	end := strings.Index(header, ">")

	if start != -1 && end != -1 && end > start {
		return strings.TrimSpace(header[start+1 : end])
	}

	// Otherwise return the whole header trimmed
	return strings.TrimSpace(header)
}

// ClearMailhogMessages deletes all messages from Mailhog
func ClearMailhogMessages(t *testing.T) error {
	httpClient := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("DELETE", "http://localhost:8025/api/v1/messages", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to clear Mailhog messages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code from Mailhog: %d", resp.StatusCode)
	}

	t.Log("Cleared all Mailhog messages")
	return nil
}
