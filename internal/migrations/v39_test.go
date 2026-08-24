package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
)

func TestV39Migration_Metadata(t *testing.T) {
	m := &V39Migration{}
	assert.Equal(t, 39.0, m.GetMajorVersion())
	assert.True(t, m.HasSystemUpdate())
	assert.False(t, m.HasWorkspaceUpdate())
	assert.False(t, m.ShouldRestartServer())
}

func TestV39Migration_IsRegistered(t *testing.T) {
	migration, ok := GetRegisteredMigration(39.0)
	require.True(t, ok, "v39 must be registered so it runs on startup")
	assert.IsType(t, &V39Migration{}, migration)
}

// The grants must keep four properties. The defaults literal has to sit on the
// LEFT of ||, since the right operand wins on duplicate keys and a stored grant
// must survive the merge. The object check has to be jsonb_typeof, since
// concatenating onto a JSON scalar yields an array that no longer scans into
// UserPermissions. The "already granted" check has to require all three
// resources, so a row holding one of them still receives the other two. And the
// empty object has to be excluded, or a re-run grants everything to the rows the
// normalisation statements just wrote — see TestV39PermissionBackfillMigration
// in tests/integration, which runs these statements against real Postgres.
const (
	v39GrantLiteral = `'\{"segments":\s+\{"read": true, "write": true\},\s+` +
		`"webhook_subscriptions":\s+\{"read": true, "write": true\},\s+` +
		`"webhook_events":\s+\{"read": true, "write": true\}\}'::jsonb\s+\|\|\s+permissions`
	v39GrantGuards = `jsonb_typeof\(permissions\)\s+=\s+'object'\s+` +
		`AND\s+permissions\s+<>\s+'\{\}'::jsonb\s+` +
		`AND\s+NOT\s+\(permissions\s+\?\s+'segments'\s+` +
		`AND\s+permissions\s+\?\s+'webhook_subscriptions'\s+` +
		`AND\s+permissions\s+\?\s+'webhook_events'\)`

	v39UserWorkspacesGrant = `(?s)UPDATE\s+user_workspaces.*` + v39GrantLiteral + `.*` + v39GrantGuards
	v39InvitationsGrant    = `(?s)UPDATE\s+workspace_invitations.*` + v39GrantLiteral + `.*` + v39GrantGuards

	v39UserWorkspacesNormalise = `(?s)UPDATE\s+user_workspaces\s+SET\s+permissions\s+=\s+'\{\}'::jsonb\s+WHERE\s+permissions\s+IS\s+NULL`
	v39InvitationsNormalise    = `(?s)UPDATE\s+workspace_invitations\s+SET\s+permissions\s+=\s+'\{\}'::jsonb\s+WHERE\s+permissions\s+IS\s+NULL`
)

func expectV39Grants(mock sqlmock.Sqlmock) {
	mock.ExpectExec(v39UserWorkspacesGrant).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(v39InvitationsGrant).WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestV39Migration_UpdateSystem(t *testing.T) {
	m := &V39Migration{}
	ctx := context.Background()

	t.Run("grants the new resources then normalises the null rows", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Without the grants, HasPermission denies segments, webhook_subscriptions
		// and webhook_events for every non-owner member: their stored permission
		// map predates the resources.
		expectV39Grants(mock)
		mock.ExpectExec(v39UserWorkspacesNormalise).WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectExec(v39InvitationsNormalise).WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, m.UpdateSystem(ctx, &config.Config{}, db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("reports a failed user_workspaces grant", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec(v39UserWorkspacesGrant).WillReturnError(errors.New("boom"))

		err = m.UpdateSystem(ctx, &config.Config{}, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "add scoping permissions to user workspaces")
	})

	t.Run("reports a failed invitations grant", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec(v39UserWorkspacesGrant).WillReturnResult(sqlmock.NewResult(0, 3))
		mock.ExpectExec(v39InvitationsGrant).WillReturnError(errors.New("boom"))

		err = m.UpdateSystem(ctx, &config.Config{}, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "add scoping permissions to workspace invitations")
	})

	t.Run("reports a failed user_workspaces normalisation", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		expectV39Grants(mock)
		mock.ExpectExec(v39UserWorkspacesNormalise).WillReturnError(errors.New("boom"))

		err = m.UpdateSystem(ctx, &config.Config{}, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "normalise null permissions on user workspaces")
	})

	t.Run("reports a failed invitations normalisation", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		expectV39Grants(mock)
		mock.ExpectExec(v39UserWorkspacesNormalise).WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectExec(v39InvitationsNormalise).WillReturnError(errors.New("boom"))

		err = m.UpdateSystem(ctx, &config.Config{}, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "normalise null permissions on workspace invitations")
	})
}

// The normalisation statements must run after both grants. Run first, they would
// turn every SQL-NULL permissions column into '{}', which jsonb_typeof reports as
// 'object' — offering a zero-permission member to the grants. The '{}' exclusion
// pinned above refuses that row too, so this order is the second defence against
// the same escalation, not the only one. Every expectation above is satisfied
// either way, since
// each statement matches its own regex whatever position it is issued in, so the
// order is recorded and asserted separately.
func TestV39Migration_UpdateSystem_StatementOrder(t *testing.T) {
	rec := &v38StatementRecorder{}
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(rec))
	require.NoError(t, err)
	defer db.Close()

	expectV39Grants(mock)
	mock.ExpectExec(v39UserWorkspacesNormalise).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(v39InvitationsNormalise).WillReturnResult(sqlmock.NewResult(0, 0))

	m := &V39Migration{}
	require.NoError(t, m.UpdateSystem(context.Background(), &config.Config{}, db))

	require.Len(t, rec.issued, 4, "issued statements:\n%s", rec.all())
	assert.Regexp(t, v39UserWorkspacesGrant, rec.issued[0])
	assert.Regexp(t, v39InvitationsGrant, rec.issued[1])
	assert.Regexp(t, v39UserWorkspacesNormalise, rec.issued[2])
	assert.Regexp(t, v39InvitationsNormalise, rec.issued[3])
}
