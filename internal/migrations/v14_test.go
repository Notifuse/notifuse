package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestV14Migration_GetMajorVersion(t *testing.T) {
	migration := &V14Migration{}
	assert.Equal(t, 14.0, migration.GetMajorVersion())
}

func TestV14Migration_HasSystemUpdate(t *testing.T) {
	migration := &V14Migration{}
	assert.True(t, migration.HasSystemUpdate())
}

func TestV14Migration_HasWorkspaceUpdate(t *testing.T) {
	migration := &V14Migration{}
	assert.True(t, migration.HasWorkspaceUpdate())
}
