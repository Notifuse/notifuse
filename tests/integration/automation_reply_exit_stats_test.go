package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAutomationReplyExitStats checks, against a real database, that contacts leaving an
// automation by replying ("Exit on reply") are counted in its stats.exited, like every
// exit the executor makes. ExitContactJourneysOnReply used to move the journeys to
// 'exited' without touching the counter.
func TestAutomationReplyExitStats(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	workspaceID := workspace.ID

	workspaceDB, err := factory.GetWorkspaceDB(workspaceID)
	require.NoError(t, err)

	ctx := context.Background()
	repo := repository.NewAutomationRepositoryWithDB(
		workspaceDB, service.NewAutomationTriggerGenerator(service.NewQueryBuilder()))

	// newAutomation creates an automation with the given exit_on_reply flag and raw stats
	// (a JSONB literal, or nil for SQL NULL).
	newAutomation := func(t *testing.T, exitOnReply bool, stats *string) string {
		automation, err := factory.CreateAutomation(workspaceID)
		require.NoError(t, err)
		var rawStats interface{}
		if stats != nil {
			rawStats = *stats
		}
		_, err = workspaceDB.ExecContext(ctx,
			`UPDATE automations SET exit_on_reply = $1, stats = $2::jsonb WHERE id = $3`,
			exitOnReply, rawStats, automation.ID)
		require.NoError(t, err)
		return automation.ID
	}
	enroll := func(t *testing.T, automationID, email string, enteredAt time.Time) {
		_, err := workspaceDB.ExecContext(ctx,
			`INSERT INTO contact_automations (id, automation_id, contact_email, status, entered_at)
			 VALUES ($1, $2, $3, 'active', $4)`,
			uuid.New().String(), automationID, email, enteredAt)
		require.NoError(t, err)
	}
	rawStats := func(t *testing.T, automationID string) sql.NullString {
		var s sql.NullString
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT stats::text FROM automations WHERE id = $1`, automationID).Scan(&s))
		return s
	}
	stats := func(t *testing.T, automationID string) map[string]int {
		s := rawStats(t, automationID)
		require.True(t, s.Valid, "stats should not be NULL")
		var m map[string]int
		require.NoError(t, json.Unmarshal([]byte(s.String), &m), "stats must stay an object: %s", s.String)
		return m
	}
	journeyStatus := func(t *testing.T, automationID, email string) string {
		var status string
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT status FROM contact_automations WHERE automation_id = $1 AND contact_email = $2`,
			automationID, email).Scan(&status))
		return status
	}
	str := func(s string) *string { return &s }

	before := time.Now().UTC()
	past := before.Add(-time.Hour)

	t.Run("a reply exit raises exited on each exit_on_reply automation only", func(t *testing.T) {
		email := "reply-" + uuid.New().String()[:8] + "@example.com"
		first := newAutomation(t, true, str(`{"enrolled": 9, "exited": 4}`))
		second := newAutomation(t, true, nil)
		other := newAutomation(t, false, str(`{"enrolled": 3, "exited": 1}`))
		enroll(t, first, email, past)
		enroll(t, second, email, past)
		enroll(t, other, email, past)

		n, err := repo.ExitContactJourneysOnReply(ctx, workspaceID, email, nil, "replied", before)
		require.NoError(t, err)
		assert.Equal(t, 2, n, "two journeys exited")

		assert.Equal(t, map[string]int{"enrolled": 9, "exited": 5}, stats(t, first))
		assert.Equal(t, map[string]int{"exited": 1}, stats(t, second), "SQL NULL stats start at zero")
		assert.Equal(t, map[string]int{"enrolled": 3, "exited": 1}, stats(t, other), "no exit_on_reply: untouched")
		assert.Equal(t, "exited", journeyStatus(t, first, email))
		assert.Equal(t, "active", journeyStatus(t, other, email))
	})

	t.Run("scoped to one automation, a JSON null stats becomes an object", func(t *testing.T) {
		email := "reply-" + uuid.New().String()[:8] + "@example.com"
		target := newAutomation(t, true, str(`null`))
		sibling := newAutomation(t, true, str(`{"exited": 7}`))
		enroll(t, target, email, past)
		enroll(t, sibling, email, past)

		n, err := repo.ExitContactJourneysOnReply(ctx, workspaceID, email, &target, "replied", before)
		require.NoError(t, err)
		assert.Equal(t, 1, n)

		assert.Equal(t, map[string]int{"exited": 1}, stats(t, target))
		assert.Equal(t, map[string]int{"exited": 7}, stats(t, sibling))
		assert.Equal(t, "active", journeyStatus(t, sibling, email))

		// The automation must still load: an array in stats would fail its unmarshal.
		_, err = repo.GetByID(ctx, workspaceID, target)
		require.NoError(t, err)
	})

	t.Run("a journey entered after the reply is not exited nor counted", func(t *testing.T) {
		email := "reply-" + uuid.New().String()[:8] + "@example.com"
		automationID := newAutomation(t, true, str(`{"exited": 2}`))
		enroll(t, automationID, email, before.Add(time.Minute))

		n, err := repo.ExitContactJourneysOnReply(ctx, workspaceID, email, nil, "replied", before)
		require.NoError(t, err)
		assert.Equal(t, 0, n)
		assert.Equal(t, map[string]int{"exited": 2}, stats(t, automationID))
		assert.Equal(t, "active", journeyStatus(t, automationID, email))
	})

	t.Run("concurrent reply exits on the same automation lose no count", func(t *testing.T) {
		automationID := newAutomation(t, true, str(`{"exited": 0}`))
		const contacts = 8
		emails := make([]string, contacts)
		for i := range emails {
			emails[i] = "reply-" + uuid.New().String()[:8] + "@example.com"
			enroll(t, automationID, emails[i], past)
		}

		var wg sync.WaitGroup
		errs := make(chan error, contacts)
		for _, email := range emails {
			wg.Add(1)
			go func(email string) {
				defer wg.Done()
				_, err := repo.ExitContactJourneysOnReply(ctx, workspaceID, email, nil, "replied", before)
				errs <- err
			}(email)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}

		assert.Equal(t, map[string]int{"exited": contacts}, stats(t, automationID))
	})
}
