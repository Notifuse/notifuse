package database

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Notifuse/notifuse/config"
	internaldb "github.com/Notifuse/notifuse/internal/database"
)

func TestConnectionManager_EnsureDatabase(t *testing.T) {
	t.Run("defaults to EnsureWorkspaceDatabaseExists", func(t *testing.T) {
		cm := &connectionManager{}
		assert.Equal(t, reflect.ValueOf(internaldb.EnsureWorkspaceDatabaseExists).Pointer(), reflect.ValueOf(cm.ensureDatabase()).Pointer())
	})

	t.Run("an injected one is used", func(t *testing.T) {
		called := false
		cm := &connectionManager{ensureWorkspaceDatabase: func(context.Context, *config.DatabaseConfig, string, func(context.Context) error) error {
			called = true
			return nil
		}}
		assert.NoError(t, cm.ensureDatabase()(context.Background(), nil, "ws", nil))
		assert.True(t, called)
	})
}
