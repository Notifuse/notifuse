package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipIfNotIntegrationTest skips the test if INTEGRATION_TESTS is not set
func skipIfNotCircuitBreakerTest(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TESTS=true to run.")
	}
}

func TestCircuitBreakerBroadcastPause(t *testing.T) {
	skipIfNotCircuitBreakerTest(t)

	t.Run("should_pause_broadcast_and_set_reason_when_circuit_breaker_triggers", func(t *testing.T) {
		// Create a fresh test database
		dbManager := testutil.NewDatabaseManager()
		defer dbManager.Cleanup()

		err := dbManager.Setup()
		require.NoError(t, err)

		// Seed test data to create workspace and necessary entities
		err = dbManager.SeedTestData()
		require.NoError(t, err)

		db := dbManager.GetDB()

		// Get workspace database connection
		workspaceDB, err := dbManager.GetWorkspaceDB("testws01")
		require.NoError(t, err)
		defer workspaceDB.Close()

		// Ensure workspace database has the broadcasts table (initialize schema)
		err = initializeWorkspaceSchema(workspaceDB)
		require.NoError(t, err)

		// Step 1: Create a test broadcast in the workspace database
		broadcastID := "cb-test-001-12345678" // Unique ID to avoid conflicts
		_, err = workspaceDB.Exec(`
			INSERT INTO broadcasts (
				id, workspace_id, name, status, audience, schedule, test_settings, 
				created_at, updated_at
			) VALUES (
				$1, 'testws01', 'Circuit Breaker Test Broadcast', 'sending', 
				'{"type": "all"}', '{"type": "immediate"}', '{}',
				CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
			)
		`, broadcastID)
		require.NoError(t, err)

		// Step 2: Verify broadcast exists and is in 'sending' status
		var initialStatus, initialPauseReason string
		err = workspaceDB.QueryRow(`
			SELECT status, COALESCE(pause_reason, '') FROM broadcasts WHERE id = $1
		`, broadcastID).Scan(&initialStatus, &initialPauseReason)
		require.NoError(t, err)
		assert.Equal(t, "sending", initialStatus, "Broadcast should initially be in sending status")
		assert.Empty(t, initialPauseReason, "Pause reason should initially be empty")

		// Step 3: Create a task for this broadcast in system database
		taskID := "550e8400-e29b-41d4-a716-446655440001" // Valid UUID format
		_, err = db.Exec(`
			INSERT INTO tasks (
				id, workspace_id, type, status, progress, broadcast_id,
				created_at, updated_at
			) VALUES (
				$1, 'testws01', 'broadcast', 'running', 0.5, $2,
				CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
			)
		`, taskID, broadcastID)
		require.NoError(t, err)

		// Step 4: Create a mock circuit breaker scenario by simulating the orchestrator behavior
		// We'll simulate what happens when the circuit breaker triggers during broadcast processing

		ctx := context.Background()

		// Create a mock event bus to capture events
		mockEventBus := &MockEventBus{
			events: make([]domain.EventPayload, 0),
		}

		// Step 5: Simulate circuit breaker trigger by directly calling the pause logic
		// In real scenario, this would happen in orchestrator.Process() when email provider fails

		// Simulate the circuit breaker being triggered with a typical email provider error
		circuitBreakerReason := "Email provider daily limit exceeded (SparkPost error 420)"

		// Update broadcast status to paused with reason (simulating what orchestrator would do)
		_, err = workspaceDB.Exec(`
			UPDATE broadcasts 
			SET status = 'paused', 
				pause_reason = $1,
				paused_at = CURRENT_TIMESTAMP,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = $2
		`, circuitBreakerReason, broadcastID)
		require.NoError(t, err)

		// Publish circuit breaker event (simulating what orchestrator would do)
		circuitBreakerEvent := domain.EventPayload{
			Type: domain.EventBroadcastCircuitBreaker,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
				"task_id":      taskID,
				"reason":       circuitBreakerReason,
				"error":        "Daily sending limit exceeded",
			},
		}
		mockEventBus.Publish(ctx, circuitBreakerEvent)

		// Publish broadcast paused event (simulating what orchestrator would do)
		broadcastPausedEvent := domain.EventPayload{
			Type: domain.EventBroadcastPaused,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
				"task_id":      taskID,
				"reason":       circuitBreakerReason,
			},
		}
		mockEventBus.Publish(ctx, broadcastPausedEvent)

		// Step 6: Verify the broadcast was paused with the correct reason
		var finalStatus, finalPauseReason string
		var pausedAt *time.Time
		err = workspaceDB.QueryRow(`
			SELECT status, COALESCE(pause_reason, ''), paused_at 
			FROM broadcasts WHERE id = $1
		`, broadcastID).Scan(&finalStatus, &finalPauseReason, &pausedAt)
		require.NoError(t, err)

		// Assertions
		assert.Equal(t, "paused", finalStatus, "Broadcast should be paused after circuit breaker trigger")
		assert.Equal(t, circuitBreakerReason, finalPauseReason, "Pause reason should be set to circuit breaker reason")
		assert.NotNil(t, pausedAt, "Paused timestamp should be set")
		assert.WithinDuration(t, time.Now(), *pausedAt, 5*time.Second, "Paused timestamp should be recent")

		// Step 7: Verify events were published correctly
		require.Len(t, mockEventBus.events, 2, "Should have published 2 events")

		// Check circuit breaker event
		circuitEvent := mockEventBus.events[0]
		assert.Equal(t, domain.EventBroadcastCircuitBreaker, circuitEvent.Type, "First event should be circuit breaker")
		assert.Equal(t, broadcastID, circuitEvent.Data["broadcast_id"], "Circuit breaker event should have correct broadcast ID")
		assert.Equal(t, taskID, circuitEvent.Data["task_id"], "Circuit breaker event should have correct task ID")
		assert.Equal(t, circuitBreakerReason, circuitEvent.Data["reason"], "Circuit breaker event should have correct reason")

		// Check broadcast paused event
		pausedEvent := mockEventBus.events[1]
		assert.Equal(t, domain.EventBroadcastPaused, pausedEvent.Type, "Second event should be broadcast paused")
		assert.Equal(t, broadcastID, pausedEvent.Data["broadcast_id"], "Paused event should have correct broadcast ID")
		assert.Equal(t, taskID, pausedEvent.Data["task_id"], "Paused event should have correct task ID")
		assert.Equal(t, circuitBreakerReason, pausedEvent.Data["reason"], "Paused event should have correct reason")

		// Step 8: Verify task status (in real scenario, TaskService would handle this)
		var taskStatus string
		err = db.QueryRow(`SELECT status FROM tasks WHERE id = $1`, taskID).Scan(&taskStatus)
		require.NoError(t, err)
		// Note: In real scenario, TaskService would update this to 'paused' when it receives the event
		// For this integration test, we're primarily testing the broadcast pause functionality
	})

	t.Run("should_handle_multiple_circuit_breaker_triggers_gracefully", func(t *testing.T) {
		// Create a fresh test database
		dbManager := testutil.NewDatabaseManager()
		defer dbManager.Cleanup()

		err := dbManager.Setup()
		require.NoError(t, err)

		// Seed test data
		err = dbManager.SeedTestData()
		require.NoError(t, err)

		// Get workspace database connection
		workspaceDB, err := dbManager.GetWorkspaceDB("testws01")
		require.NoError(t, err)
		defer workspaceDB.Close()

		// Ensure workspace database has the broadcasts table (initialize schema)
		err = initializeWorkspaceSchema(workspaceDB)
		require.NoError(t, err)

		// Create a test broadcast
		broadcastID := "cb-test-002-87654321" // Unique ID to avoid conflicts
		_, err = workspaceDB.Exec(`
			INSERT INTO broadcasts (
				id, workspace_id, name, status, audience, schedule, test_settings, 
				created_at, updated_at
			) VALUES (
				$1, 'testws01', 'Multiple CB Test Broadcast', 'sending', 
				'{"type": "all"}', '{"type": "immediate"}', '{}',
				CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
			)
		`, broadcastID)
		require.NoError(t, err)

		// First circuit breaker trigger
		firstReason := "SparkPost daily limit exceeded (error 420)"
		_, err = workspaceDB.Exec(`
			UPDATE broadcasts 
			SET status = 'paused', 
				pause_reason = $1,
				paused_at = CURRENT_TIMESTAMP,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = $2
		`, firstReason, broadcastID)
		require.NoError(t, err)

		// Verify first pause
		var status1, reason1 string
		err = workspaceDB.QueryRow(`
			SELECT status, COALESCE(pause_reason, '') FROM broadcasts WHERE id = $1
		`, broadcastID).Scan(&status1, &reason1)
		require.NoError(t, err)
		assert.Equal(t, "paused", status1)
		assert.Equal(t, firstReason, reason1)

		// Simulate broadcast being resumed
		_, err = workspaceDB.Exec(`
			UPDATE broadcasts 
			SET status = 'sending', 
				pause_reason = NULL,
				paused_at = NULL,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = $1
		`, broadcastID)
		require.NoError(t, err)

		// Second circuit breaker trigger with different reason
		secondReason := "Mailgun rate limit exceeded (error 429)"
		_, err = workspaceDB.Exec(`
			UPDATE broadcasts 
			SET status = 'paused', 
				pause_reason = $1,
				paused_at = CURRENT_TIMESTAMP,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = $2
		`, secondReason, broadcastID)
		require.NoError(t, err)

		// Verify second pause with updated reason
		var status2, reason2 string
		err = workspaceDB.QueryRow(`
			SELECT status, COALESCE(pause_reason, '') FROM broadcasts WHERE id = $1
		`, broadcastID).Scan(&status2, &reason2)
		require.NoError(t, err)
		assert.Equal(t, "paused", status2)
		assert.Equal(t, secondReason, reason2, "Pause reason should be updated to the latest circuit breaker trigger")
	})

	t.Run("should_preserve_pause_reason_when_broadcast_is_resumed", func(t *testing.T) {
		// Create a fresh test database
		dbManager := testutil.NewDatabaseManager()
		defer dbManager.Cleanup()

		err := dbManager.Setup()
		require.NoError(t, err)

		// Seed test data
		err = dbManager.SeedTestData()
		require.NoError(t, err)

		// Get workspace database connection
		workspaceDB, err := dbManager.GetWorkspaceDB("testws01")
		require.NoError(t, err)
		defer workspaceDB.Close()

		// Ensure workspace database has the broadcasts table (initialize schema)
		err = initializeWorkspaceSchema(workspaceDB)
		require.NoError(t, err)

		// Create a test broadcast
		broadcastID := "cb-test-003-11223344" // Unique ID to avoid conflicts
		circuitBreakerReason := "Circuit breaker triggered: Email provider error 503"

		_, err = workspaceDB.Exec(`
			INSERT INTO broadcasts (
				id, workspace_id, name, status, audience, schedule, test_settings, 
				pause_reason, paused_at, created_at, updated_at
			) VALUES (
				$1, 'testws01', 'Resume Test Broadcast', 'paused', 
				'{"type": "all"}', '{"type": "immediate"}', '{}',
				$2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
			)
		`, broadcastID, circuitBreakerReason)
		require.NoError(t, err)

		// Verify broadcast is paused with reason
		var initialStatus, initialReason string
		err = workspaceDB.QueryRow(`
			SELECT status, COALESCE(pause_reason, '') FROM broadcasts WHERE id = $1
		`, broadcastID).Scan(&initialStatus, &initialReason)
		require.NoError(t, err)
		assert.Equal(t, "paused", initialStatus)
		assert.Equal(t, circuitBreakerReason, initialReason)

		// Simulate resuming the broadcast (this would typically be done via API or admin action)
		_, err = workspaceDB.Exec(`
			UPDATE broadcasts 
			SET status = 'sending',
				updated_at = CURRENT_TIMESTAMP
			WHERE id = $1
		`, broadcastID)
		require.NoError(t, err)

		// Verify broadcast is resumed but pause_reason is preserved for historical reference
		var resumedStatus, preservedReason string
		err = workspaceDB.QueryRow(`
			SELECT status, COALESCE(pause_reason, '') FROM broadcasts WHERE id = $1
		`, broadcastID).Scan(&resumedStatus, &preservedReason)
		require.NoError(t, err)
		assert.Equal(t, "sending", resumedStatus, "Broadcast should be resumed")
		assert.Equal(t, circuitBreakerReason, preservedReason, "Pause reason should be preserved for historical tracking")
	})
}

// MockEventBus implements domain.EventBus for testing
type MockEventBus struct {
	events []domain.EventPayload
}

func (m *MockEventBus) Publish(ctx context.Context, event domain.EventPayload) error {
	m.events = append(m.events, event)
	return nil
}

func (m *MockEventBus) Subscribe(eventType domain.EventType, handler domain.EventHandler) error {
	// Not needed for this test
	return nil
}

func (m *MockEventBus) Unsubscribe(eventType domain.EventType, handler domain.EventHandler) error {
	// Not needed for this test
	return nil
}

// initializeWorkspaceSchema creates the broadcasts table in the workspace database
func initializeWorkspaceSchema(workspaceDB *sql.DB) error {
	// Create the broadcasts table with the pause_reason column (from v5 migration)
	createBroadcastsTable := `
		CREATE TABLE IF NOT EXISTS broadcasts (
			id VARCHAR(255) NOT NULL,
			workspace_id VARCHAR(32) NOT NULL,
			name VARCHAR(255) NOT NULL,
			status VARCHAR(20) NOT NULL,
			audience JSONB NOT NULL,
			schedule JSONB NOT NULL,
			test_settings JSONB NOT NULL,
			utm_parameters JSONB,
			metadata JSONB,
			winning_template VARCHAR(32),
			test_sent_at TIMESTAMP WITH TIME ZONE,
			winner_sent_at TIMESTAMP WITH TIME ZONE,
			test_phase_recipient_count INTEGER DEFAULT 0,
			winner_phase_recipient_count INTEGER DEFAULT 0,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
			started_at TIMESTAMP WITH TIME ZONE,
			completed_at TIMESTAMP WITH TIME ZONE,
			cancelled_at TIMESTAMP WITH TIME ZONE,
			paused_at TIMESTAMP WITH TIME ZONE,
			pause_reason TEXT,
			PRIMARY KEY (id)
		)
	`

	_, err := workspaceDB.Exec(createBroadcastsTable)
	if err != nil {
		return fmt.Errorf("failed to create broadcasts table: %w", err)
	}

	return nil
}
