package migrations

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV19Migration_GetMajorVersion(t *testing.T) {
	migration := &V19Migration{}
	assert.Equal(t, 19.0, migration.GetMajorVersion())
}

func TestV19Migration_HasSystemUpdate(t *testing.T) {
	migration := &V19Migration{}
	assert.False(t, migration.HasSystemUpdate(), "V19Migration should not have system updates")
}

func TestV19Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V19Migration{}
	assert.True(t, migration.HasWorkspaceUpdate(), "V19Migration should have workspace updates")
}

func TestV19Migration_ShouldRestartServer(t *testing.T) {
	migration := &V19Migration{}
	assert.False(t, migration.ShouldRestartServer(), "V19Migration should not require server restart")
}

func TestV19Migration_UpdateSystem(t *testing.T) {
	migration := &V19Migration{}
	ctx := context.Background()
	cfg := &config.Config{}

	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// UpdateSystem should do nothing and return nil
	err = migration.UpdateSystem(ctx, cfg, db)
	assert.NoError(t, err)
}

func TestV19Migration_UpdateWorkspace(t *testing.T) {
	migration := &V19Migration{}
	ctx := context.Background()
	cfg := &config.Config{}
	workspace := &domain.Workspace{
		ID:   "test-workspace",
		Name: "Test Workspace",
	}

	t.Run("Success - Full migration", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 0: Rename webhook_events to inbound_webhook_events
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger function
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger
		mock.ExpectExec("DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update contact_timeline entity_type
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update segment trees
		mock.ExpectExec("UPDATE segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// PART 1: webhook_subscriptions table
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_subscriptions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// PART 2: webhook_deliveries table
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_deliveries").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// PART 3: webhook_contacts_trigger (with DELETE support)
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contacts_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contacts ON contacts").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// PART 4: webhook_contact_lists_trigger
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contact_lists_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contact_lists ON contact_lists").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// PART 5: webhook_contact_segments_trigger
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contact_segments_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contact_segments ON contact_segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// PART 6: webhook_message_history_trigger
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_message_history_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_message_history ON message_history").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// PART 7: webhook_custom_events_trigger (with soft-delete support)
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_custom_events_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_custom_events ON custom_events").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// PART 8: Add full_name column
		mock.ExpectExec("ALTER TABLE contacts ADD COLUMN IF NOT EXISTS full_name").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// PART 9: Update track_contact_changes and webhook_contacts_trigger
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_contact_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contacts_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Failure - webhook_subscriptions table creation fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 0: Rename webhook_events to inbound_webhook_events
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger function
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger
		mock.ExpectExec("DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update contact_timeline entity_type
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update segment trees
		mock.ExpectExec("UPDATE segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_subscriptions").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook_subscriptions table")
	})

	t.Run("Failure - webhook_subscriptions index creation fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 0: Rename webhook_events to inbound_webhook_events
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger function
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger
		mock.ExpectExec("DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update contact_timeline entity_type
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update segment trees
		mock.ExpectExec("UPDATE segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_subscriptions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook_subscriptions index")
	})

	t.Run("Failure - webhook_deliveries table creation fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 0: Rename webhook_events to inbound_webhook_events
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger function
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger
		mock.ExpectExec("DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update contact_timeline entity_type
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_subscriptions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_deliveries").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook_deliveries table")
	})

	t.Run("Failure - webhook_deliveries pending index fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 0: Rename webhook_events to inbound_webhook_events
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger function
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger
		mock.ExpectExec("DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update contact_timeline entity_type
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_subscriptions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_deliveries").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook_deliveries pending index")
	})

	t.Run("Failure - webhook_deliveries subscription index fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 0: Rename webhook_events to inbound_webhook_events
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger function
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger
		mock.ExpectExec("DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update contact_timeline entity_type
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_subscriptions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_deliveries").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook_deliveries subscription index")
	})

	t.Run("Failure - webhook_contacts_trigger function fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 0: Rename webhook_events to inbound_webhook_events
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger function
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger
		mock.ExpectExec("DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update contact_timeline entity_type
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_subscriptions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_deliveries").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contacts_trigger").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook_contacts_trigger function")
	})

	t.Run("Failure - webhook_contacts trigger fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 0: Rename webhook_events to inbound_webhook_events
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger function
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger
		mock.ExpectExec("DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update contact_timeline entity_type
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_subscriptions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_deliveries").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contacts_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contacts ON contacts").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook_contacts trigger")
	})

	t.Run("Failure - webhook_contact_lists_trigger function fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 0: Rename webhook_events to inbound_webhook_events
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger function
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger
		mock.ExpectExec("DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update contact_timeline entity_type
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_subscriptions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_deliveries").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contacts_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contacts ON contacts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contact_lists_trigger").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook_contact_lists_trigger function")
	})

	t.Run("Failure - webhook_contact_lists trigger fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 0: Rename webhook_events to inbound_webhook_events
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger function
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger
		mock.ExpectExec("DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update contact_timeline entity_type
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_subscriptions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_deliveries").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contacts_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contacts ON contacts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contact_lists_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contact_lists ON contact_lists").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook_contact_lists trigger")
	})

	t.Run("Failure - webhook_contact_segments_trigger function fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 0: Rename webhook_events to inbound_webhook_events
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger function
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger
		mock.ExpectExec("DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update contact_timeline entity_type
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_subscriptions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_deliveries").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contacts_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contacts ON contacts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contact_lists_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contact_lists ON contact_lists").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contact_segments_trigger").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook_contact_segments_trigger function")
	})

	t.Run("Failure - webhook_contact_segments trigger fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 0: Rename webhook_events to inbound_webhook_events
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger function
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger
		mock.ExpectExec("DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update contact_timeline entity_type
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_subscriptions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_deliveries").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contacts_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contacts ON contacts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contact_lists_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contact_lists ON contact_lists").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contact_segments_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contact_segments ON contact_segments").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook_contact_segments trigger")
	})

	t.Run("Failure - webhook_message_history_trigger function fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 0: Rename webhook_events to inbound_webhook_events
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger function
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger
		mock.ExpectExec("DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update contact_timeline entity_type
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_subscriptions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_deliveries").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contacts_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contacts ON contacts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contact_lists_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contact_lists ON contact_lists").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contact_segments_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contact_segments ON contact_segments").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_message_history_trigger").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook_message_history_trigger function")
	})

	t.Run("Failure - webhook_message_history trigger fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 0: Rename webhook_events to inbound_webhook_events
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger function
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger
		mock.ExpectExec("DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update contact_timeline entity_type
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_subscriptions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_deliveries").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contacts_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contacts ON contacts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contact_lists_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contact_lists ON contact_lists").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contact_segments_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contact_segments ON contact_segments").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_message_history_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_message_history ON message_history").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook_message_history trigger")
	})

	t.Run("Failure - webhook_custom_events_trigger function fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 0: Rename webhook_events to inbound_webhook_events
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger function
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger
		mock.ExpectExec("DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update contact_timeline entity_type
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_subscriptions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_deliveries").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contacts_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contacts ON contacts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contact_lists_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contact_lists ON contact_lists").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contact_segments_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contact_segments ON contact_segments").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_message_history_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_message_history ON message_history").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_custom_events_trigger").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook_custom_events_trigger function")
	})

	t.Run("Failure - webhook_custom_events trigger fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// PART 0: Rename webhook_events to inbound_webhook_events
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$ BEGIN").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger function
		mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Create new inbound webhook event trigger
		mock.ExpectExec("DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Update contact_timeline entity_type
		mock.ExpectExec("UPDATE contact_timeline").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE segments").
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_subscriptions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_deliveries").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contacts_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contacts ON contacts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contact_lists_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contact_lists ON contact_lists").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_contact_segments_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_contact_segments ON contact_segments").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_message_history_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_message_history ON message_history").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION webhook_custom_events_trigger").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DROP TRIGGER IF EXISTS webhook_custom_events ON custom_events").
			WillReturnError(assert.AnError)

		err = migration.UpdateWorkspace(ctx, cfg, workspace, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook_custom_events trigger")
	})
}

func TestV19Migration_Registered(t *testing.T) {
	// Verify that V19Migration is properly registered
	found := false
	for _, m := range GetRegisteredMigrations() {
		if m.GetMajorVersion() == 19.0 {
			found = true
			break
		}
	}
	assert.True(t, found, "V19Migration should be registered")
}
