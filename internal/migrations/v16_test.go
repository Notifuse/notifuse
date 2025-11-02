package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestV16Migration_GetMajorVersion(t *testing.T) {
	migration := &V16Migration{}
	assert.Equal(t, 16.0, migration.GetMajorVersion())
}

func TestV16Migration_HasSystemUpdate(t *testing.T) {
	migration := &V16Migration{}
	assert.False(t, migration.HasSystemUpdate())
}

func TestV16Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V16Migration{}
	assert.True(t, migration.HasWorkspaceUpdate())
}

func TestV16Migration_ShouldRestartServer(t *testing.T) {
	migration := &V16Migration{}
	assert.False(t, migration.ShouldRestartServer())
}

func TestV16Migration_UpdateSystem(t *testing.T) {
	migration := &V16Migration{}
	ctx := context.Background()

	// Should be a no-op since HasSystemUpdate returns false
	err := migration.UpdateSystem(ctx, nil, nil)
	assert.NoError(t, err)
}
