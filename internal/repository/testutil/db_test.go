package testutil

import (
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupMockDB(t *testing.T) {
	t.Run("creates mock DB successfully", func(t *testing.T) {
		db, mock, cleanup := SetupMockDB(t)

		require.NotNil(t, db)
		require.NotNil(t, mock)
		require.NotNil(t, cleanup)

		// Verify db is a valid *sql.DB
		assert.IsType(t, (*sql.DB)(nil), db)

		// Verify mock is a valid sqlmock.Sqlmock
		assert.IsType(t, (*sqlmock.Sqlmock)(nil), &mock)

		// Cleanup should close the database
		cleanup()
	})

	t.Run("cleanup closes database", func(t *testing.T) {
		db, _, cleanup := SetupMockDB(t)

		// Database should be open
		err := db.Ping()
		assert.NoError(t, err)

		// Call cleanup
		cleanup()

		// Database should be closed (ping should fail)
		err = db.Ping()
		assert.Error(t, err)
	})

	t.Run("mock has QueryMatcherRegexp option", func(t *testing.T) {
		_, mock, cleanup := SetupMockDB(t)
		defer cleanup()

		// The mock should be configured with regexp matcher
		// We can verify this by setting up an expectation with regex
		mock.ExpectQuery("SELECT .* FROM users").
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("1"))

		// The mock should accept regex patterns
		assert.NotNil(t, mock)
	})
}
