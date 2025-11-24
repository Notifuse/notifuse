package integration

import (
	"testing"

	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatabaseConnection(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	dbManager := testutil.NewDatabaseManager()
	defer func() { _ = dbManager.Cleanup() }()

	err := dbManager.Setup()
	require.NoError(t, err, "Failed to setup database")

	db := dbManager.GetDB()
	require.NotNil(t, db, "Database connection should not be nil")

	// Test database connectivity
	err = db.Ping()
	assert.NoError(t, err, "Should be able to ping database")
}

func TestDatabaseMigrations(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	dbManager := testutil.NewDatabaseManager()
	defer func() { _ = dbManager.Cleanup() }()

	err := dbManager.Setup()
	require.NoError(t, err, "Failed to setup database")

	db := dbManager.GetDB()

	// Check that system tables exist
	systemTables := []string{"users", "workspaces", "user_workspaces"}
	for _, table := range systemTables {
		var exists bool
		query := `SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = $1
		)`
		err := db.QueryRow(query, table).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "Table %s should exist", table)
	}

	// Check that workspace tables exist
	workspaceTables := []string{"contacts", "lists", "contact_lists", "templates", "broadcasts", "message_history"}
	for _, table := range workspaceTables {
		var exists bool
		query := `SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = $1
		)`
		err := db.QueryRow(query, table).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "Table %s should exist", table)
	}
}

func TestDatabaseSeedData(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	dbManager := testutil.NewDatabaseManager()
	defer func() { _ = dbManager.Cleanup() }()

	err := dbManager.Setup()
	require.NoError(t, err, "Failed to setup database")

	err = dbManager.SeedTestData()
	require.NoError(t, err, "Failed to seed test data")

	db := dbManager.GetDB()

	// Check that test user exists
	var userExists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = '550e8400-e29b-41d4-a716-446655440000')").Scan(&userExists)
	require.NoError(t, err)
	assert.True(t, userExists, "Test user should exist")

	// Check that test workspace exists
	var workspaceExists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = 'testws01')").Scan(&workspaceExists)
	require.NoError(t, err)
	assert.True(t, workspaceExists, "Test workspace should exist")

	// Check that workspace user association exists
	var associationExists bool
	err = db.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM user_workspaces 
		WHERE workspace_id = 'testws01' AND user_id = '550e8400-e29b-41d4-a716-446655440000'
	)`).Scan(&associationExists)
	require.NoError(t, err)
	assert.True(t, associationExists, "Workspace user association should exist")
}

func TestDatabaseCleanup(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	dbManager := testutil.NewDatabaseManager()
	defer func() { _ = dbManager.Cleanup() }()

	err := dbManager.Setup()
	require.NoError(t, err, "Failed to setup database")

	err = dbManager.SeedTestData()
	require.NoError(t, err, "Failed to seed test data")

	db := dbManager.GetDB()

	// Verify data exists
	var userCount int
	err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	require.NoError(t, err)
	assert.Greater(t, userCount, 0, "Should have test data")

	// Clean data
	err = dbManager.CleanupTestData()
	require.NoError(t, err, "Failed to cleanup test data")

	// Verify data is cleaned
	err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	require.NoError(t, err)
	assert.Equal(t, 0, userCount, "Should have no data after cleanup")

	var workspaceCount int
	err = db.QueryRow("SELECT COUNT(*) FROM workspaces").Scan(&workspaceCount)
	require.NoError(t, err)
	assert.Equal(t, 0, workspaceCount, "Should have no workspaces after cleanup")
}
