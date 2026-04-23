package migrations

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

func TestV30Migration_GetMajorVersion(t *testing.T) {
	m := &V30Migration{}
	assert.Equal(t, 30.0, m.GetMajorVersion())
}

func TestV30Migration_HasSystemUpdate(t *testing.T) {
	m := &V30Migration{}
	assert.True(t, m.HasSystemUpdate())
}

func TestV30Migration_HasWorkspaceUpdate(t *testing.T) {
	m := &V30Migration{}
	assert.True(t, m.HasWorkspaceUpdate())
}

func TestV30Migration_ShouldRestartServer(t *testing.T) {
	m := &V30Migration{}
	assert.False(t, m.ShouldRestartServer())
}

func TestV30Migration_UpdateSystem_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE workspaces\s+SET settings = jsonb_set\(settings, '\{timezone\}', '"Europe/Kyiv"'\)`).
		WillReturnResult(sqlmock.NewResult(0, 2))

	m := &V30Migration{}
	err = m.UpdateSystem(context.Background(), &config.Config{}, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV30Migration_UpdateSystem_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE workspaces`).WillReturnError(assert.AnError)

	m := &V30Migration{}
	err = m.UpdateSystem(context.Background(), &config.Config{}, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to rewrite workspace timezone")
}

func TestV30Migration_UpdateWorkspace_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	workspace := &domain.Workspace{ID: "ws_abc"}

	// 1. Webhook secret rotation: SELECT ids, then per-id UPDATE with a Go-generated secret.
	mock.ExpectQuery(`SELECT id FROM webhook_subscriptions`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("sub_1").AddRow("sub_2"))
	mock.ExpectExec(`UPDATE webhook_subscriptions SET secret = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), "sub_1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE webhook_subscriptions SET secret = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), "sub_2").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 2. Timezone rewrite steps, in order.
	mock.ExpectExec(`ALTER TABLE contacts DISABLE TRIGGER webhook_contacts`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE contacts DISABLE TRIGGER contact_changes_trigger`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE contacts SET timezone = 'Europe/Kyiv' WHERE timezone = 'Europe/Kiev'`).
		WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectExec(`ALTER TABLE contacts ENABLE TRIGGER contact_changes_trigger`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE contacts ENABLE TRIGGER webhook_contacts`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE segments SET timezone = 'Europe/Kyiv' WHERE timezone = 'Europe/Kiev'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE broadcasts\s+SET schedule = jsonb_set\(schedule, '\{timezone\}', '"Europe/Kyiv"'\)`).
		WillReturnResult(sqlmock.NewResult(0, 2))

	m := &V30Migration{}
	err = m.UpdateWorkspace(context.Background(), &config.Config{}, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV30Migration_UpdateWorkspace_NoWebhookSubscriptions(t *testing.T) {
	// A workspace with no webhook subscriptions should skip the rotation loop
	// entirely and still apply the timezone rewrites.
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	workspace := &domain.Workspace{ID: "ws_empty"}

	mock.ExpectQuery(`SELECT id FROM webhook_subscriptions`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`ALTER TABLE contacts DISABLE TRIGGER webhook_contacts`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE contacts DISABLE TRIGGER contact_changes_trigger`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE contacts`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE contacts ENABLE TRIGGER contact_changes_trigger`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE contacts ENABLE TRIGGER webhook_contacts`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE segments`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE broadcasts`).WillReturnResult(sqlmock.NewResult(0, 0))

	m := &V30Migration{}
	err = m.UpdateWorkspace(context.Background(), &config.Config{}, workspace, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV30Migration_UpdateWorkspace_WebhookListError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	workspace := &domain.Workspace{ID: "ws_abc"}

	mock.ExpectQuery(`SELECT id FROM webhook_subscriptions`).WillReturnError(assert.AnError)

	m := &V30Migration{}
	err = m.UpdateWorkspace(context.Background(), &config.Config{}, workspace, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list webhook subscriptions for workspace ws_abc")
}

func TestV30Migration_UpdateWorkspace_WebhookRotationError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	workspace := &domain.Workspace{ID: "ws_abc"}

	mock.ExpectQuery(`SELECT id FROM webhook_subscriptions`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("sub_1"))
	mock.ExpectExec(`UPDATE webhook_subscriptions SET secret = \$1`).
		WithArgs(sqlmock.AnyArg(), "sub_1").
		WillReturnError(assert.AnError)

	m := &V30Migration{}
	err = m.UpdateWorkspace(context.Background(), &config.Config{}, workspace, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to rotate webhook secret for workspace ws_abc subscription sub_1")
}

func TestV30Migration_UpdateWorkspace_TimezoneStepError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	workspace := &domain.Workspace{ID: "ws_abc"}

	mock.ExpectQuery(`SELECT id FROM webhook_subscriptions`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`ALTER TABLE contacts DISABLE TRIGGER webhook_contacts`).
		WillReturnError(assert.AnError)

	m := &V30Migration{}
	err = m.UpdateWorkspace(context.Background(), &config.Config{}, workspace, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace ws_abc")
	assert.Contains(t, err.Error(), "disable webhook_contacts trigger")
}

func TestV30Migration_Registration(t *testing.T) {
	found := false
	for _, m := range GetRegisteredMigrations() {
		if m.GetMajorVersion() == 30.0 {
			found = true
			break
		}
	}
	assert.True(t, found, "V30Migration should be registered")
}
