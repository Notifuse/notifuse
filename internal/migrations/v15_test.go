package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestV15Migration_UpdateWorkspace(t *testing.T) {
	migration := &V15Migration{}
	ctx := context.Background()

	// Should be a no-op since HasWorkspaceUpdate returns false
	err := migration.UpdateWorkspace(ctx, nil, nil, nil)
	assert.NoError(t, err)
}
