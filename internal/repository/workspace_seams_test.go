package repository

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database"
)

func TestWorkspaceRepository_DatabaseSeams(t *testing.T) {
	pointer := func(fn interface{}) uintptr { return reflect.ValueOf(fn).Pointer() }

	t.Run("default to the database package", func(t *testing.T) {
		r := &workspaceRepository{}
		assert.Equal(t, pointer(database.CreateWorkspaceDatabase), pointer(r.createDatabase()))
		assert.Equal(t, pointer(database.WithWorkspaceDatabaseLock), pointer(r.nameLock()))
	})

	t.Run("injected ones are used", func(t *testing.T) {
		created, locked := false, false
		r := &workspaceRepository{
			createWorkspaceDatabase: func(context.Context, *config.DatabaseConfig, string, func(context.Context) (database.WorkspaceRow, error), database.WorkspaceInsert) error {
				created = true
				return nil
			},
			workspaceDatabaseLock: func(context.Context, *config.DatabaseConfig, string, func(context.Context, *sql.DB) error) error {
				locked = true
				return nil
			},
		}
		assert.NoError(t, r.createDatabase()(context.Background(), nil, "ws", nil, database.WorkspaceInsert{}))
		assert.NoError(t, r.nameLock()(context.Background(), nil, "notifuse_ws", nil))
		assert.True(t, created)
		assert.True(t, locked)
	})
}
