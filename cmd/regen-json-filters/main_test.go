package main

import (
	"bytes"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_RefusesWithoutADSN(t *testing.T) {
	var out, errOut bytes.Buffer
	opened := false
	code := run(nil, &out, &errOut, func(string) (*sql.DB, error) { opened = true; return nil, nil })
	assert.Equal(t, 2, code)
	assert.False(t, opened)
	assert.Contains(t, errOut.String(), "-dsn is required")
}

func TestRun_RefusesAnUnknownFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 2, run([]string{"-workspace", "x"}, &out, &errOut, nil))
}

func TestRun_ReportsADatabaseItCannotOpen(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"-dsn", "postgres://x"}, &out, &errOut, func(string) (*sql.DB, error) {
		return nil, errors.New("bad dsn")
	})
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "bad dsn")
}

// One transaction under a lock timeout: the segments first, then the trigger bodies, then the
// commit, and a report.
func TestRun_RegeneratesInOneTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SET LOCAL lock_timeout = '5s'`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tree, generated_sql, generated_args FROM segments`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tree", "generated_sql", "generated_args"}).
			AddRow("garbled", []byte(`not json`), "SELECT 1", []byte(`[]`)))
	mock.ExpectQuery("SELECT a.id, a.root_node_id, a.trigger_config").
		WillReturnRows(sqlmock.NewRows([]string{"id", "root_node_id", "trigger_config", "installed_def"}))
	mock.ExpectCommit()

	var out, errOut bytes.Buffer
	code := run([]string{"-dsn", "postgres://x"}, &out, &errOut, func(string) (*sql.DB, error) { return db, nil })

	assert.Equal(t, 0, code, errOut.String())
	assert.Contains(t, out.String(), "Segments recompiled and marked for a rebuild: 0\n")
	assert.Contains(t, out.String(), "Segments left with their query: 1\n  garbled: tree is not valid JSON")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRun_RollsBackOnAnError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SET LOCAL lock_timeout = '5s'`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tree, generated_sql, generated_args FROM segments`)).
		WillReturnError(errors.New("canceling statement due to lock timeout"))
	mock.ExpectRollback()

	var out, errOut bytes.Buffer
	code := run([]string{"-dsn", "postgres://x"}, &out, &errOut, func(string) (*sql.DB, error) { return db, nil })

	assert.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "lock timeout")
	assert.Contains(t, errOut.String(), "nothing was changed")
	require.NoError(t, mock.ExpectationsWereMet())
}
