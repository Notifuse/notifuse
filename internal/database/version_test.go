package database

import (
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var versionQuery = regexp.QuoteMeta(`SELECT current_setting('server_version_num')::int, current_setting('server_version')`)

func TestRequireServerVersion(t *testing.T) {
	check := func(t *testing.T, rows *sqlmock.Rows, queryErr error) error {
		t.Helper()
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		q := mock.ExpectQuery(versionQuery)
		if queryErr != nil {
			q.WillReturnError(queryErr)
		} else {
			q.WillReturnRows(rows)
		}
		result := RequireServerVersion(db, MinServerVersionNum)
		require.NoError(t, mock.ExpectationsWereMet())
		return result
	}

	t.Run("refuses PostgreSQL 16, naming both versions", func(t *testing.T) {
		err := check(t, sqlmock.NewRows([]string{"num", "version"}).AddRow(160004, "16.4"), nil)
		require.Error(t, err)
		assert.Equal(t, "PostgreSQL 17 or later is required; this server runs 16.4", err.Error())
	})

	t.Run("accepts 17.0 and later", func(t *testing.T) {
		assert.NoError(t, check(t, sqlmock.NewRows([]string{"num", "version"}).AddRow(170000, "17.0"), nil))
		assert.NoError(t, check(t, sqlmock.NewRows([]string{"num", "version"}).AddRow(170005, "17.5 (Debian 17.5-1)"), nil))
	})

	t.Run("reports a version it cannot read", func(t *testing.T) {
		err := check(t, nil, errors.New("connection reset"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection reset")
	})
}
