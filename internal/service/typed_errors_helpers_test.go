package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// requireInvalidState asserts that err is, or wraps, an InvalidStateError with these fields: the
// fields are what the handler serialises, so a client reads them rather than the message.
func requireInvalidState(t *testing.T, err error, entity, id, action, status string) {
	t.Helper()
	var stateErr *domain.InvalidStateError
	require.True(t, errors.As(err, &stateErr), "expected an InvalidStateError, got %v", err)
	assert.Equal(t, domain.InvalidStateError{Entity: entity, ID: id, Action: action, Status: status}, *stateErr)
}

// requireNotFound asserts that err is, or wraps, an ErrNotFound with these fields.
func requireNotFound(t *testing.T, err error, entity, id string) {
	t.Helper()
	var notFound *domain.ErrNotFound
	require.True(t, errors.As(err, &notFound), "expected an ErrNotFound, got %v", err)
	assert.Equal(t, domain.ErrNotFound{Entity: entity, ID: id}, *notFound)
}

// requireAlreadyExists asserts that err is, or wraps, an AlreadyExistsError with these fields.
func requireAlreadyExists(t *testing.T, err error, entity, id string, deleted bool) {
	t.Helper()
	var existsErr *domain.AlreadyExistsError
	require.True(t, errors.As(err, &existsErr), "expected an AlreadyExistsError, got %v", err)
	assert.Equal(t, domain.AlreadyExistsError{Entity: entity, ID: id, Deleted: deleted}, *existsErr)
}
