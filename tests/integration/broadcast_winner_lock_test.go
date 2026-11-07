//go:build integration
// +build integration

package integration

import (
	"context"
	"testing"
	"time"

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

	// A locking read with no timeout waits for the first writer, then sees the
	// winner it committed rather than the row it was blocked on.
	type readResult struct {
		broadcast *domain.Broadcast
		err       error
	}
	waiter, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = waiter.Rollback() }()
	done := make(chan readResult, 1)
	go func() {
		b, rErr := repo.GetBroadcastForUpdateTx(ctx, waiter, workspace.ID, broadcast.ID)
		done <- readResult{b, rErr}
	}()

	select {
	case r := <-done:
		t.Fatalf("the locking read returned while the row was held: %+v", r)
	case <-time.After(300 * time.Millisecond):
	}

	winner := "tpl-winner"
	held.WinningTemplate = &winner
	held.Status = domain.BroadcastStatusWinnerSelected
	require.NoError(t, repo.UpdateBroadcastTx(ctx, first, held))
	require.NoError(t, first.Commit())

	var after readResult
	select {
	case after = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the locking read was still blocked after the first writer committed")
	}
	require.NoError(t, after.err)
	assert.Equal(t, domain.BroadcastStatusWinnerSelected, after.broadcast.Status)
	require.NotNil(t, after.broadcast.WinningTemplate)
	assert.Equal(t, winner, *after.broadcast.WinningTemplate)
}
