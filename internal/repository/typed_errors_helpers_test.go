package repository

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// requireNotFound asserts that err is, or wraps, an ErrNotFound with these fields.
func requireNotFound(t *testing.T, err error, entity, id string) {
	t.Helper()
	var notFound *domain.ErrNotFound
	require.True(t, errors.As(err, &notFound), "expected an ErrNotFound, got %v", err)
	assert.Equal(t, domain.ErrNotFound{Entity: entity, ID: id}, *notFound)
}

// requireInvalidState asserts that err is, or wraps, an InvalidStateError with these fields.
func requireInvalidState(t *testing.T, err error, entity, id, action, status string) {
	t.Helper()
	var stateErr *domain.InvalidStateError
	require.True(t, errors.As(err, &stateErr), "expected an InvalidStateError, got %v", err)
	assert.Equal(t, domain.InvalidStateError{Entity: entity, ID: id, Action: action, Status: status}, *stateErr)
}
