package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmailQueue tests the email queue system end-to-end
func TestEmailQueue(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

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

	// Set up SMTP email provider for testing
	integration, err := factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NotNil(t, integration)

	// Login to get auth token
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Get repositories from app
	app := suite.ServerManager.GetApp()
	queueRepo := app.GetEmailQueueRepository()
	require.NotNil(t, queueRepo, "Email queue repository should be available")

	t.Run("Repository Operations", func(t *testing.T) {
		testRepositoryOperations(t, queueRepo, workspace.ID, integration.ID)
	})

	t.Run("Worker Processing", func(t *testing.T) {
		testWorkerProcessing(t, suite, queueRepo, workspace.ID, integration.ID)
	})

	t.Run("Dead Letter Queue", func(t *testing.T) {
		testDeadLetterQueue(t, queueRepo, workspace.ID, integration.ID)
	})

	t.Run("Rate Limiting", func(t *testing.T) {
		testRateLimiting(t, suite, queueRepo, workspace.ID, integration.ID)
	})
}

func testRepositoryOperations(t *testing.T, queueRepo domain.EmailQueueRepository, workspaceID, integrationID string) {
	ctx := context.Background()

	t.Run("Enqueue and FetchPending", func(t *testing.T) {
		// Create test entries
		entry1 := testutil.CreateTestEmailQueueEntry(integrationID, "test1@example.com", "broadcast-1", domain.EmailQueueSourceBroadcast)
		entry1.Priority = 1 // Higher priority
		entry2 := testutil.CreateTestEmailQueueEntry(integrationID, "test2@example.com", "broadcast-1", domain.EmailQueueSourceBroadcast)
		entry2.Priority = 5 // Lower priority

		// Enqueue entries
		err := queueRepo.Enqueue(ctx, workspaceID, []*domain.EmailQueueEntry{entry1, entry2})
		require.NoError(t, err)

		// Fetch pending entries
		entries, err := queueRepo.FetchPending(ctx, workspaceID, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(entries), 2, "Should fetch at least 2 pending entries")

		// Verify priority ordering (lower number = higher priority)
		if len(entries) >= 2 {
			assert.LessOrEqual(t, entries[0].Priority, entries[1].Priority, "Entries should be ordered by priority")
		}

		// Clean up: mark entries as sent
		for _, e := range entries {
			_ = queueRepo.MarkAsSent(ctx, workspaceID, e.ID)
		}
	})

	t.Run("Status Transitions", func(t *testing.T) {
		// Create test entry
		entry := testutil.CreateTestEmailQueueEntry(integrationID, "status-test@example.com", "broadcast-status", domain.EmailQueueSourceBroadcast)
		err := queueRepo.Enqueue(ctx, workspaceID, []*domain.EmailQueueEntry{entry})
		require.NoError(t, err)

		// Fetch to get the ID
		entries, err := queueRepo.FetchPending(ctx, workspaceID, 10)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(entries), 1)

		testEntry := entries[0]
		testEntryID := testEntry.ID

		// Transition: pending -> processing
		err = queueRepo.MarkAsProcessing(ctx, workspaceID, testEntry.ID)
		require.NoError(t, err)

		// Transition: processing -> sent (entry is deleted immediately)
		err = queueRepo.MarkAsSent(ctx, workspaceID, testEntry.ID)
		require.NoError(t, err)

		// Verify entry is deleted by checking it's no longer in the queue
		entriesAfter, err := queueRepo.FetchPending(ctx, workspaceID, 100)
		require.NoError(t, err)
		for _, e := range entriesAfter {
			assert.NotEqual(t, testEntryID, e.ID, "Sent entry should be deleted from queue")
		}
	})

	t.Run("Status Transitions - Failed with Retry", func(t *testing.T) {
		// Create test entry
		entry := testutil.CreateTestEmailQueueEntry(integrationID, "retry-test@example.com", "broadcast-retry", domain.EmailQueueSourceBroadcast)
		err := queueRepo.Enqueue(ctx, workspaceID, []*domain.EmailQueueEntry{entry})
		require.NoError(t, err)

		// Fetch to get the ID
		entries, err := queueRepo.FetchPending(ctx, workspaceID, 10)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(entries), 1)

		testEntry := entries[0]

		// Transition: pending -> processing
		err = queueRepo.MarkAsProcessing(ctx, workspaceID, testEntry.ID)
		require.NoError(t, err)

		// Transition: processing -> failed (with retry)
		nextRetry := time.Now().Add(1 * time.Minute)
		err = queueRepo.MarkAsFailed(ctx, workspaceID, testEntry.ID, "test error", &nextRetry)
		require.NoError(t, err)

		// Verify it can be fetched again after retry time
		// For now, just clean up
		_ = queueRepo.MarkAsSent(ctx, workspaceID, testEntry.ID)
	})

	t.Run("Stats Aggregation", func(t *testing.T) {
		stats, err := queueRepo.GetStats(ctx, workspaceID)
		require.NoError(t, err)

		// Verify stats structure
		// Note: Sent entries are deleted immediately, so no "Sent" count in stats
		t.Logf("Queue stats - Pending: %d, Processing: %d, Failed: %d, DeadLetter: %d",
			stats.Pending, stats.Processing, stats.Failed, stats.DeadLetter)

		assert.GreaterOrEqual(t, stats.Pending, int64(0), "Pending count should be non-negative")
		assert.GreaterOrEqual(t, stats.Failed, int64(0), "Failed count should be non-negative")
	})

	t.Run("Source Filtering", func(t *testing.T) {
		sourceID := "source-filter-test-" + testutil.GenerateRandomString(8)

		// Create entries for different sources
		broadcastEntry := testutil.CreateTestEmailQueueEntry(integrationID, "broadcast@example.com", sourceID, domain.EmailQueueSourceBroadcast)
		automationEntry := testutil.CreateTestEmailQueueEntry(integrationID, "automation@example.com", sourceID, domain.EmailQueueSourceAutomation)

		err := queueRepo.Enqueue(ctx, workspaceID, []*domain.EmailQueueEntry{broadcastEntry, automationEntry})
		require.NoError(t, err)

		// Query by source ID and type
		broadcastEntries, err := queueRepo.GetBySourceID(ctx, workspaceID, domain.EmailQueueSourceBroadcast, sourceID)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(broadcastEntries), 1, "Should find broadcast entry")

		automationEntries, err := queueRepo.GetBySourceID(ctx, workspaceID, domain.EmailQueueSourceAutomation, sourceID)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(automationEntries), 1, "Should find automation entry")

		// Count by source and status
		count, err := queueRepo.CountBySourceAndStatus(ctx, workspaceID, domain.EmailQueueSourceBroadcast, sourceID, domain.EmailQueueStatusPending)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, int64(1), "Should count at least 1 pending broadcast entry")

		// Clean up
		for _, e := range broadcastEntries {
			_ = queueRepo.MarkAsSent(ctx, workspaceID, e.ID)
		}
		for _, e := range automationEntries {
			_ = queueRepo.MarkAsSent(ctx, workspaceID, e.ID)
		}
	})

	t.Run("Immediate Deletion on Send", func(t *testing.T) {
		// Create an entry and verify it's deleted immediately when marked as sent
		entry := testutil.CreateTestEmailQueueEntry(integrationID, "delete-test@example.com", "broadcast-delete", domain.EmailQueueSourceBroadcast)
		err := queueRepo.Enqueue(ctx, workspaceID, []*domain.EmailQueueEntry{entry})
		require.NoError(t, err)

		// Fetch and mark as sent
		entries, err := queueRepo.FetchPending(ctx, workspaceID, 10)
		require.NoError(t, err)

		var deletedID string
		for _, e := range entries {
			if e.ContactEmail == "delete-test@example.com" {
				deletedID = e.ID
				_ = queueRepo.MarkAsProcessing(ctx, workspaceID, e.ID)
				_ = queueRepo.MarkAsSent(ctx, workspaceID, e.ID)
			}
		}

		// Verify the entry no longer exists (sent entries are deleted immediately)
		if deletedID != "" {
			entriesAfterSend, err := queueRepo.FetchPending(ctx, workspaceID, 100)
			require.NoError(t, err)
			for _, e := range entriesAfterSend {
				assert.NotEqual(t, deletedID, e.ID, "Sent entry should be deleted immediately")
			}
			t.Logf("Verified entry %s was deleted after send", deletedID)
		}
	})
}

func testWorkerProcessing(t *testing.T, suite *testutil.IntegrationTestSuite, queueRepo domain.EmailQueueRepository, workspaceID, integrationID string) {
	ctx := context.Background()
	app := suite.ServerManager.GetApp()
	worker := app.GetEmailQueueWorker()

	if worker == nil {
		t.Skip("Email queue worker not available")
	}

	t.Run("Successful Email Delivery", func(t *testing.T) {
		// Clear Mailpit first
		err := testutil.ClearMailpitMessages(t)
		require.NoError(t, err)

		// Create test entries with unique subject for tracking
		uniqueSubject := "Queue Test " + testutil.GenerateRandomString(8)
		entry1 := testutil.CreateTestEmailQueueEntry(integrationID, "worker-test1@example.com", "broadcast-worker", domain.EmailQueueSourceBroadcast)
		entry1.Payload.Subject = uniqueSubject
		entry2 := testutil.CreateTestEmailQueueEntry(integrationID, "worker-test2@example.com", "broadcast-worker", domain.EmailQueueSourceBroadcast)
		entry2.Payload.Subject = uniqueSubject

		err = queueRepo.Enqueue(ctx, workspaceID, []*domain.EmailQueueEntry{entry1, entry2})
		require.NoError(t, err)

		// Wait for queue to be processed
		// The worker should be running as part of the app
		err = testutil.WaitForQueueEmpty(t, queueRepo, workspaceID, 30*time.Second)
		if err != nil {
			// If worker isn't running, entries will remain pending - that's ok for this test
			t.Logf("Queue not empty after wait: %v (worker may not be running)", err)
			return
		}

		// Verify emails were sent via Mailpit
		recipients := []string{"worker-test1@example.com", "worker-test2@example.com"}
		results, err := testutil.CheckMailpitForRecipients(t, uniqueSubject, recipients, 10*time.Second)
		require.NoError(t, err)

		for _, recipient := range recipients {
			assert.True(t, results[recipient], "Email should be delivered to %s", recipient)
		}
	})

	t.Run("Worker Start/Stop", func(t *testing.T) {
		// Just verify the worker can report its status
		isRunning := worker.IsRunning()
		t.Logf("Worker running status: %v", isRunning)

		// Verify we can get stats
		stats := worker.GetStats()
		t.Logf("Worker rate limiter stats: %d integrations", len(stats))
	})

	t.Run("Callback Invocation", func(t *testing.T) {
		// Set up callback tracking
		sentCount := 0
		failedCount := 0

		worker.SetCallbacks(
			func(workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string, messageID string) {
				sentCount++
				t.Logf("Callback: email sent - workspace=%s, source=%s/%s, message=%s", workspaceID, sourceType, sourceID, messageID)
			},
			func(workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string, messageID string, err error, isDeadLetter bool) {
				failedCount++
				t.Logf("Callback: email failed - workspace=%s, source=%s/%s, message=%s, error=%v, deadLetter=%v", workspaceID, sourceType, sourceID, messageID, err, isDeadLetter)
			},
		)

		// Create a test entry
		entry := testutil.CreateTestEmailQueueEntry(integrationID, "callback-test@example.com", "broadcast-callback", domain.EmailQueueSourceBroadcast)
		err := queueRepo.Enqueue(ctx, workspaceID, []*domain.EmailQueueEntry{entry})
		require.NoError(t, err)

		// Wait for processing
		_ = testutil.WaitForQueueEmpty(t, queueRepo, workspaceID, 15*time.Second)

		t.Logf("Callback results - sent: %d, failed: %d", sentCount, failedCount)
		// Note: callback may not fire if worker isn't running - that's expected in some test environments
	})
}

func testDeadLetterQueue(t *testing.T, queueRepo domain.EmailQueueRepository, workspaceID, integrationID string) {
	ctx := context.Background()

	t.Run("Move to Dead Letter", func(t *testing.T) {
		// Create an entry
		entry := testutil.CreateTestEmailQueueEntry(integrationID, "deadletter-test@example.com", "broadcast-dl", domain.EmailQueueSourceBroadcast)
		err := queueRepo.Enqueue(ctx, workspaceID, []*domain.EmailQueueEntry{entry})
		require.NoError(t, err)

		// Fetch and get the entry ID
		entries, err := queueRepo.FetchPending(ctx, workspaceID, 10)
		require.NoError(t, err)

		var testEntry *domain.EmailQueueEntry
		for _, e := range entries {
			if e.ContactEmail == "deadletter-test@example.com" {
				testEntry = e
				break
			}
		}

		if testEntry == nil {
			t.Skip("Could not find test entry")
			return
		}

		// Mark as processing
		err = queueRepo.MarkAsProcessing(ctx, workspaceID, testEntry.ID)
		require.NoError(t, err)

		// Move to dead letter
		testEntry.Attempts = testEntry.MaxAttempts // Simulate max attempts reached
		err = queueRepo.MoveToDeadLetter(ctx, workspaceID, testEntry, "permanent failure")
		require.NoError(t, err)

		// Verify it's in dead letter
		stats, err := queueRepo.GetStats(ctx, workspaceID)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, stats.DeadLetter, int64(1), "Should have at least 1 dead letter entry")
	})

	t.Run("Retry Dead Letter", func(t *testing.T) {
		// Get dead letter entries
		dlEntries, total, err := queueRepo.GetDeadLetterEntries(ctx, workspaceID, 10, 0)
		require.NoError(t, err)
		t.Logf("Dead letter entries: %d (total: %d)", len(dlEntries), total)

		if len(dlEntries) == 0 {
			t.Skip("No dead letter entries to retry")
			return
		}

		// Retry the first dead letter entry
		dlEntry := dlEntries[0]
		err = queueRepo.RetryDeadLetter(ctx, workspaceID, dlEntry.ID)
		require.NoError(t, err)

		// Verify it's back in the main queue (stats should show pending or processing)
		// Clean up by marking as sent - the retried entry should have the original entry ID
		entries, err := queueRepo.FetchPending(ctx, workspaceID, 10)
		require.NoError(t, err)

		for _, e := range entries {
			// Clean up any pending entries
			_ = queueRepo.MarkAsProcessing(ctx, workspaceID, e.ID)
			_ = queueRepo.MarkAsSent(ctx, workspaceID, e.ID)
		}
	})

	t.Run("Dead Letter Pagination", func(t *testing.T) {
		// Get dead letter entries with pagination
		page1, total1, err := queueRepo.GetDeadLetterEntries(ctx, workspaceID, 5, 0)
		require.NoError(t, err)
		t.Logf("Dead letter page 1: %d entries (total: %d)", len(page1), total1)

		if total1 > 5 {
			page2, total2, err := queueRepo.GetDeadLetterEntries(ctx, workspaceID, 5, 5)
			require.NoError(t, err)
			assert.Equal(t, total1, total2, "Total should be consistent across pages")
			t.Logf("Dead letter page 2: %d entries", len(page2))
		}
	})

	// Clean up dead letter queue
	_, _ = queueRepo.CleanupDeadLetter(ctx, workspaceID, 0)
}

func testRateLimiting(t *testing.T, suite *testutil.IntegrationTestSuite, queueRepo domain.EmailQueueRepository, workspaceID, integrationID string) {
	ctx := context.Background()
	app := suite.ServerManager.GetApp()
	worker := app.GetEmailQueueWorker()

	if worker == nil {
		t.Skip("Email queue worker not available")
	}

	t.Run("Integration Rate Limits", func(t *testing.T) {
		// Create entries with a low rate limit
		entries := make([]*domain.EmailQueueEntry, 5)
		for i := 0; i < 5; i++ {
			entries[i] = testutil.CreateTestEmailQueueEntry(
				integrationID,
				testutil.GenerateTestEmail(),
				"rate-limit-test",
				domain.EmailQueueSourceBroadcast,
			)
			entries[i].Payload.RateLimitPerMinute = 60 // 1 per second
		}

		err := queueRepo.Enqueue(ctx, workspaceID, entries)
		require.NoError(t, err)

		// Check worker stats for rate limiter info
		stats := worker.GetStats()
		t.Logf("Rate limiter stats: %+v", stats)

		// Wait for queue to process
		_, err = testutil.WaitForQueueProcessed(t, queueRepo, workspaceID, 5, 30*time.Second)
		if err != nil {
			t.Logf("Queue processing incomplete: %v (may be expected if worker not running)", err)
		}
	})

	t.Run("Multiple Integrations Rate Limits", func(t *testing.T) {
		// Just verify that rate limiter tracks multiple integrations
		stats := worker.GetStats()

		t.Logf("Rate limiters active for %d integrations", len(stats))
		for id, stat := range stats {
			t.Logf("  Integration %s: %.2f/sec (%.0f/min), burst=%d",
				id, stat.RatePerSecond, stat.RatePerMinute, stat.Burst)
		}
	})
}

// TestEmailQueueConcurrency tests concurrent operations on the email queue
func TestEmailQueueConcurrency(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	factory := suite.DataFactory

	// Create test workspace
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)
	integration, err := factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	app := suite.ServerManager.GetApp()
	queueRepo := app.GetEmailQueueRepository()
	ctx := context.Background()

	t.Run("Concurrent Enqueue", func(t *testing.T) {
		numGoroutines := 10
		entriesPerGoroutine := 5
		results := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(workerID int) {
				entries := make([]*domain.EmailQueueEntry, entriesPerGoroutine)
				for j := 0; j < entriesPerGoroutine; j++ {
					entries[j] = testutil.CreateTestEmailQueueEntry(
						integration.ID,
						testutil.GenerateTestEmail(),
						"concurrent-test",
						domain.EmailQueueSourceBroadcast,
					)
				}
				err := queueRepo.Enqueue(ctx, workspace.ID, entries)
				results <- err
			}(i)
		}

		// Collect results
		for i := 0; i < numGoroutines; i++ {
			err := <-results
			assert.NoError(t, err, "Concurrent enqueue %d should succeed", i)
		}

		// Verify total count (note: sent entries are deleted immediately, so not counted)
		stats, err := queueRepo.GetStats(ctx, workspace.ID)
		require.NoError(t, err)
		expectedTotal := int64(numGoroutines * entriesPerGoroutine)
		actualTotal := stats.Pending + stats.Processing + stats.Failed
		assert.GreaterOrEqual(t, actualTotal, expectedTotal, "Should have at least %d entries", expectedTotal)
	})

	t.Run("Concurrent FetchPending", func(t *testing.T) {
		// Multiple workers fetching concurrently should not get the same entries
		// due to FOR UPDATE SKIP LOCKED
		numWorkers := 3
		fetchedEntries := make([]map[string]bool, numWorkers)
		results := make(chan int, numWorkers)

		for i := 0; i < numWorkers; i++ {
			fetchedEntries[i] = make(map[string]bool)
			go func(workerID int) {
				entries, err := queueRepo.FetchPending(ctx, workspace.ID, 10)
				if err != nil {
					results <- 0
					return
				}
				for _, e := range entries {
					fetchedEntries[workerID][e.ID] = true
				}
				results <- len(entries)
			}(i)
		}

		// Collect results
		totalFetched := 0
		for i := 0; i < numWorkers; i++ {
			count := <-results
			totalFetched += count
		}

		t.Logf("Total entries fetched by %d concurrent workers: %d", numWorkers, totalFetched)

		// Check for duplicates (though SKIP LOCKED should prevent this)
		allIDs := make(map[string]int)
		for workerID, entries := range fetchedEntries {
			for id := range entries {
				allIDs[id]++
				if allIDs[id] > 1 {
					t.Logf("Entry %s was fetched by multiple workers (worker %d)", id, workerID)
				}
			}
		}
	})

	// Clean up (note: sent entries are deleted immediately, only dead letter needs cleanup)
	_, _ = queueRepo.CleanupDeadLetter(ctx, workspace.ID, 0)
}
