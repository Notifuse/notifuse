package domain

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrWorkspaceAlreadyExists(t *testing.T) {
	assert.EqualError(t, ErrWorkspaceAlreadyExists, "workspace already exists")
	assert.ErrorIs(t, fmt.Errorf("create: %w", ErrWorkspaceAlreadyExists), ErrWorkspaceAlreadyExists)
}

// The console shows this message as is.
func TestErrWorkspaceDatabaseNotEmpty(t *testing.T) {
	err := &ErrWorkspaceDatabaseNotEmpty{WorkspaceID: "acme", DatabaseName: "notifuse_ws_acme"}
	assert.EqualError(t, err, `a database for workspace ID "acme" already exists and holds tables, but no workspace uses it (notifuse_ws_acme): choose another ID, or remove that database`)

	var target *ErrWorkspaceDatabaseNotEmpty
	assert.True(t, errors.As(fmt.Errorf("create: %w", err), &target))
	assert.Equal(t, "notifuse_ws_acme", target.DatabaseName)
}
