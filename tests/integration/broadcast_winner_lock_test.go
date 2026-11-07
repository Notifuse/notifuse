//go:build integration
// +build integration

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestBroadcastWinnerReadLocksTheRow checks against a real PostgreSQL what the
// automatic A/B evaluation and the owner's winner choice rely on to take turns:
// GetBroadcastForUpdateTx holds the broadcast row until its transaction ends, so a
// second locking read waits, and once the first writer commits it sees the winner.
func TestBroadcastWinnerReadLocksTheRow(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	factory := suite.DataFactory
	ctx := context.Background()
	repo := suite.ServerManager.GetApp().GetBroadcastRepository()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	broadcast, err := factory.CreateBroadcast(workspace.ID)
	require.NoError(t, err)

	first, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = first.Rollback() }()
	held, err := repo.GetBroadcastForUpdateTx(ctx, first, workspace.ID, broadcast.ID)
	require.NoError(t, err)

	// A second locking read gives up after its lock timeout while the first holds the row.
	second, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = second.ExecContext(ctx, `SET LOCAL lock_timeout = '200ms'`)
	require.NoError(t, err)
	_, err = repo.GetBroadcastForUpdateTx(ctx, second, workspace.ID, broadcast.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lock timeout")
	_ = second.Rollback()

	// The first writer records a winner and commits; the next locking read sees it.
	winner := "tpl-winner"
	held.WinningTemplate = &winner
	held.Status = domain.BroadcastStatusWinnerSelected
	require.NoError(t, repo.UpdateBroadcastTx(ctx, first, held))
	require.NoError(t, first.Commit())

	third, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = third.Rollback() }()
	after, err := repo.GetBroadcastForUpdateTx(ctx, third, workspace.ID, broadcast.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BroadcastStatusWinnerSelected, after.Status)
	require.NotNil(t, after.WinningTemplate)
	assert.Equal(t, winner, *after.WinningTemplate)
}
