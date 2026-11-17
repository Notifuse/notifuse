//go:build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Before 44, a scheduled A/B broadcast sent every variation to its whole audience, completed
// its task with no phase and stayed scheduled. The upgrade, run by the migration manager from
// 43, marks it processed, completed when its task did and started at its first message. Other
// scheduled broadcasts are left as they are. The workspace database's sessions are not in UTC.
//
// Plan: Notifuse/architecture plans/notifuse/fix-scheduled-ab-repair-and-pause-clock-plan.md
func TestV44RepairsTheScheduledABBroadcastsSentBefore44(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	ctx := context.Background()
	factory := suite.DataFactory
	systemDB := suite.DBManager.GetDB()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	wsDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)

	var wsName, sysName string
	require.NoError(t, wsDB.QueryRow(`SELECT current_database()`).Scan(&wsName))
	require.NoError(t, systemDB.QueryRow(`SELECT current_database()`).Scan(&sysName))
	for _, name := range []string{wsName, sysName} {
		_, err := systemDB.Exec(fmt.Sprintf(`ALTER DATABASE %q SET timezone = 'America/New_York'`, name))
		require.NoError(t, err)
	}

	firstMessage := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)
	taskDone := time.Now().UTC().Add(-47 * time.Hour).Truncate(time.Microsecond)

	abScheduled := func() string {
		t.Helper()
		b, err := factory.CreateBroadcast(workspace.ID, testutil.WithBroadcastStatus(domain.BroadcastStatusScheduled),
			func(b *domain.Broadcast) {
				b.TestSettings = domain.BroadcastTestSettings{Enabled: true, SamplePercentage: 20, Variations: []domain.BroadcastVariation{
					{VariationName: "A", TemplateID: template.ID}, {VariationName: "B", TemplateID: template.ID},
				}}
			})
		require.NoError(t, err)
		_, err = factory.CreateMessageHistory(workspace.ID, testutil.WithMessageTemplate(template.ID), testutil.WithMessageBroadcast(b.ID),
			func(m *domain.MessageHistory) {
				m.CreatedAt, m.SentAt, m.UpdatedAt = firstMessage, firstMessage, firstMessage
			})
		require.NoError(t, err)
		return b.ID
	}
	task := func(broadcastID, status, phase string, completedAt *time.Time) {
		t.Helper()
		_, err := systemDB.Exec(`INSERT INTO tasks (id, workspace_id, type, status, state, created_at, updated_at, completed_at, broadcast_id)
			VALUES ($1, $2, 'send_broadcast', $3, jsonb_build_object('send_broadcast', jsonb_build_object('broadcast_id', $5::text, 'phase', $4::text)),
			        now(), now(), $6, $5)`,
			uuid.New().String(), workspace.ID, status, phase, broadcastID, completedAt)
		require.NoError(t, err)
	}

	sentBefore44 := abScheduled()
	task(sentBefore44, "completed", "", &taskDone)
	stillToSend := abScheduled()
	task(stillToSend, "pending", "", nil)
	underWay := abScheduled()
	task(underWay, "completed", "test", &taskDone)
	noTask := abScheduled()

	// The manager migrates every registered workspace: this test's is the only one with a database.
	_, err = systemDB.Exec(`DELETE FROM workspaces WHERE id <> $1`, workspace.ID)
	require.NoError(t, err)
	cfg := &config.Config{Database: *suite.DBManager.GetConfig(), Security: config.SecurityConfig{SecretKey: "test-secret-key-for-migration"}}
	for run := 1; run <= 2; run++ {
		_, err := systemDB.Exec(`UPDATE settings SET value = '43' WHERE key = 'db_version'`)
		require.NoError(t, err)
		err = migrations.NewManager(logger.NewLoggerWithLevel("disabled")).RunMigrations(ctx, cfg, systemDB)
		if err != nil && err != migrations.ErrRestartRequired {
			require.NoError(t, err, "run %d", run)
		}

		row := func(id string) (status string, completedAt, startedAt sql.NullTime) {
			t.Helper()
			require.NoError(t, wsDB.QueryRow(`SELECT status, completed_at, started_at FROM broadcasts WHERE id = $1`, id).
				Scan(&status, &completedAt, &startedAt))
			return
		}
		status, completedAt, startedAt := row(sentBefore44)
		assert.Equal(t, "processed", status, "run %d: the broadcast sent before 44 is processed", run)
		require.True(t, completedAt.Valid, "run %d", run)
		assert.True(t, taskDone.Equal(completedAt.Time), "run %d: completed when its task did: got %s, want %s", run, completedAt.Time, taskDone)
		require.True(t, startedAt.Valid, "run %d", run)
		assert.True(t, firstMessage.Equal(startedAt.Time), "run %d: started at its first message", run)

		for name, id := range map[string]string{"a send still to run": stillToSend, "a send with a phase": underWay, "no task": noTask} {
			status, completedAt, _ := row(id)
			assert.Equal(t, "scheduled", status, "run %d: %s is left scheduled", run, name)
			assert.False(t, completedAt.Valid, "run %d: %s", run, name)
		}
	}
}
