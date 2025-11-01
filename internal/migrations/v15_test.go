package migrations

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV15Migration_GetMajorVersion(t *testing.T) {
	migration := &V15Migration{}
	assert.Equal(t, 15.0, migration.GetMajorVersion())
}

func TestV15Migration_HasSystemUpdate(t *testing.T) {
	migration := &V15Migration{}
	assert.True(t, migration.HasSystemUpdate())
}

func TestV15Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V15Migration{}
	assert.False(t, migration.HasWorkspaceUpdate())
}

func TestV15Migration_ShouldRestartServer(t *testing.T) {
	migration := &V15Migration{}
	assert.False(t, migration.ShouldRestartServer())
}

func TestV15Migration_UpdateSystem(t *testing.T) {
	t.Skip("Skipping integration test - requires test database")

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	migration := &V15Migration{}
	ctx := context.Background()
	cfg := createTestConfig()
	// Set SECRET_KEY in config (required for migration)
	cfg.Security.SecretKey = "test-secret-key-1234567890123456"
	db := setupTestDB(t, cfg)
	defer db.Close()

	// Create settings table if it doesn't exist
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS settings (
			key VARCHAR(255) PRIMARY KEY,
			value TEXT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Create users table if it doesn't exist
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY,
			type VARCHAR(20) NOT NULL,
			email VARCHAR(255) NOT NULL,
			name VARCHAR(255),
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Create workspace_invitations table if it doesn't exist
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS workspace_invitations (
			id UUID PRIMARY KEY,
			workspace_id VARCHAR(20) NOT NULL,
			inviter_id UUID NOT NULL,
			email VARCHAR(255) NOT NULL,
			permissions JSONB DEFAULT '{}'::jsonb,
			expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Create user_sessions table if it doesn't exist
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS user_sessions (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL,
			expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			magic_code VARCHAR(255),
			magic_code_expires_at TIMESTAMP WITH TIME ZONE
		)
	`)
	require.NoError(t, err)

	// Insert test PASETO settings
	_, err = db.ExecContext(ctx,
		"INSERT INTO settings (key, value) VALUES ($1, $2), ($3, $4) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value",
		"encrypted_paseto_private_key", "test-encrypted-private-key",
		"encrypted_paseto_public_key", "test-encrypted-public-key")
	require.NoError(t, err)

	// Insert test API key users
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, type, email, name)
		VALUES 
			('11111111-1111-1111-1111-111111111111', 'api_key', 'apikey1@example.com', 'API Key 1'),
			('22222222-2222-2222-2222-222222222222', 'api_key', 'apikey2@example.com', 'API Key 2')
	`)
	require.NoError(t, err)

	// Insert test workspace invitations (some expired, some not)
	_, err = db.ExecContext(ctx, `
		INSERT INTO workspace_invitations (id, workspace_id, inviter_id, email, expires_at)
		VALUES 
			('33333333-3333-3333-3333-333333333333', 'workspace1', '44444444-4444-4444-4444-444444444444', 'invite1@example.com', NOW() + INTERVAL '7 days'),
			('55555555-5555-5555-5555-555555555555', 'workspace1', '44444444-4444-4444-4444-444444444444', 'invite2@example.com', NOW() - INTERVAL '7 days')
	`)
	require.NoError(t, err)

	// Insert test user sessions
	_, err = db.ExecContext(ctx, `
		INSERT INTO user_sessions (id, user_id, expires_at, magic_code, magic_code_expires_at)
		VALUES 
			('66666666-6666-6666-6666-666666666666', '11111111-1111-1111-1111-111111111111', NOW() + INTERVAL '24 hours', 'abc123', NOW() + INTERVAL '15 minutes'),
			('77777777-7777-7777-7777-777777777777', '22222222-2222-2222-2222-222222222222', NOW() + INTERVAL '24 hours', NULL, NULL)
	`)
	require.NoError(t, err)

	// Run migration
	err = migration.UpdateSystem(ctx, cfg, db)
	assert.NoError(t, err)

	// Verify PASETO settings were deleted
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM settings WHERE key IN ('encrypted_paseto_private_key', 'encrypted_paseto_public_key')").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 0, count, "PASETO settings should be deleted")

	// Verify API key users were deleted
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE type = 'api_key'").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 0, count, "API key users should be deleted")

	// Verify only non-expired invitations were deleted
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspace_invitations").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 1, count, "Only expired invitation should remain")

	// Verify the remaining invitation is the expired one
	var remainingID string
	err = db.QueryRowContext(ctx, "SELECT id FROM workspace_invitations").Scan(&remainingID)
	assert.NoError(t, err)
	assert.Equal(t, "55555555-5555-5555-5555-555555555555", remainingID)

	// Verify all user sessions were deleted
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM user_sessions").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 0, count, "All user sessions should be deleted")

	// Test idempotency - running migration again should not error
	err = migration.UpdateSystem(ctx, cfg, db)
	assert.NoError(t, err, "Migration should be idempotent")
}

func TestV15Migration_UpdateSystem_MissingSecretKey(t *testing.T) {
	t.Skip("Skipping integration test - requires test database")

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	migration := &V15Migration{}
	ctx := context.Background()
	cfg := createTestConfig()
	// Ensure SECRET_KEY is not set (empty string)
	cfg.Security.SecretKey = ""
	db := setupTestDB(t, cfg)
	defer db.Close()

	// Run migration - should fail
	err := migration.UpdateSystem(ctx, cfg, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SECRET_KEY")
}

func TestV15Migration_UpdateWorkspace(t *testing.T) {
	t.Skip("Skipping integration test - requires test database")

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test database
	ctx := context.Background()
	cfg := createTestConfig()
	db := setupTestDB(t, cfg)
	defer db.Close()

	workspace := &domain.Workspace{
		ID:   "test-workspace",
		Name: "Test Workspace",
	}

	// Run the migration - should be a no-op
	migration := &V15Migration{}
	err := migration.UpdateWorkspace(ctx, cfg, workspace, db)
	require.NoError(t, err)

	// Test idempotency
	err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
	assert.NoError(t, err, "Migration should be idempotent")
}
