package testutil

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
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
	os.Setenv("TEST_DB_HOST", "localhost")
	os.Setenv("TEST_DB_PORT", "5433")
	os.Setenv("TEST_DB_USER", "notifuse_test")
	os.Setenv("TEST_DB_PASSWORD", "test_password")
	os.Setenv("ENVIRONMENT", "test")
}

// CleanupTestEnvironment cleans up test environment variables
func CleanupTestEnvironment() {
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
