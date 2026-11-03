package database

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsDatabaseMissingError(t *testing.T) {
	missing := &pq.Error{Code: "3D000"}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error mentioning the code", errors.New("3D000 database does not exist"), false},
		{"pq 3D000", missing, true},
		{"pq undefined_table 42P01", &pq.Error{Code: "42P01"}, false},
		{"pq 3D001 is not 3D000", &pq.Error{Code: "3D001"}, false},
		{"pq empty code", &pq.Error{}, false},
		{"wrapped pq 3D000", fmt.Errorf("failed to connect to workspace ws1 database: %w", missing), true},
		{"double wrapped pq 3D000", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", missing)), true},
		{"wrapped pq other code", fmt.Errorf("connect: %w", &pq.Error{Code: "28P01"}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsDatabaseMissingError(tt.err))
		})
	}
}

func TestWorkspaceDatabaseExists(t *testing.T) {
	const query = `SELECT EXISTS\(SELECT 1 FROM pg_database WHERE datname = \$1\)`

	for _, exists := range []bool{true, false} {
		t.Run(fmt.Sprintf("reports %v", exists), func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			mock.ExpectQuery(query).WithArgs("notifuse_ws_acme").
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))

			got, err := WorkspaceDatabaseExists(context.Background(), db, "notifuse_ws_acme")
			require.NoError(t, err)
			assert.Equal(t, exists, got)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}

	t.Run("a query error names the database once", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		boom := errors.New("connection reset")
		mock.ExpectQuery(query).WithArgs("notifuse_ws_acme").WillReturnError(boom)

		_, err = WorkspaceDatabaseExists(context.Background(), db, "notifuse_ws_acme")
		require.ErrorIs(t, err, boom)
		assert.Equal(t, "failed to check whether database notifuse_ws_acme exists: connection reset", err.Error())
	})
}
