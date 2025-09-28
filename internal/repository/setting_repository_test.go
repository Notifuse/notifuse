package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository/testutil"
)

func TestSettingRepository_Get(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewSQLSettingRepository(db)

	// Test case 1: Setting found
	key := "test_key"
	value := "test_value"
	createdAt := time.Now().UTC().Truncate(time.Second)
	updatedAt := time.Now().UTC().Truncate(time.Second)

	rows := sqlmock.NewRows([]string{"key", "value", "created_at", "updated_at"}).
		AddRow(key, value, createdAt, updatedAt)

	mock.ExpectQuery(`SELECT key, value, created_at, updated_at FROM settings WHERE key = \$1`).
		WithArgs(key).
		WillReturnRows(rows)

	result, err := repo.Get(context.Background(), key)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, key, result.Key)
	assert.Equal(t, value, result.Value)
	assert.Equal(t, createdAt.Unix(), result.CreatedAt.Unix())
	assert.Equal(t, updatedAt.Unix(), result.UpdatedAt.Unix())

	// Test case 2: Setting not found
	mock.ExpectQuery(`SELECT key, value, created_at, updated_at FROM settings WHERE key = \$1`).
		WithArgs("nonexistent").
		WillReturnError(sql.ErrNoRows)

	result, err = repo.Get(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.IsType(t, &domain.ErrSettingNotFound{}, err)

	// Test case 3: Database error
	dbError := errors.New("database connection error")
	mock.ExpectQuery(`SELECT key, value, created_at, updated_at FROM settings WHERE key = \$1`).
		WithArgs("error_key").
		WillReturnError(dbError)

	result, err = repo.Get(context.Background(), "error_key")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, dbError, err)

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingRepository_Set(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewSQLSettingRepository(db)

	key := "test_key"
	value := "test_value"

	mock.ExpectExec(`INSERT INTO settings \(key, value, created_at, updated_at\) VALUES \(\$1, \$2, \$3, \$4\) ON CONFLICT \(key\) DO UPDATE SET value = EXCLUDED\.value, updated_at = EXCLUDED\.updated_at`).
		WithArgs(key, value, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Set(context.Background(), key, value)
	require.NoError(t, err)

	// Test case 2: Database error
	dbError := errors.New("database connection error")
	mock.ExpectExec(`INSERT INTO settings \(key, value, created_at, updated_at\) VALUES \(\$1, \$2, \$3, \$4\) ON CONFLICT \(key\) DO UPDATE SET value = EXCLUDED\.value, updated_at = EXCLUDED\.updated_at`).
		WithArgs("error_key", "error_value", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(dbError)

	err = repo.Set(context.Background(), "error_key", "error_value")
	require.Error(t, err)
	assert.Equal(t, dbError, err)

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingRepository_SetLastCronRun(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewSQLSettingRepository(db)

	mock.ExpectExec(`INSERT INTO settings \(key, value, created_at, updated_at\) VALUES \(\$1, \$2, \$3, \$4\) ON CONFLICT \(key\) DO UPDATE SET value = EXCLUDED\.value, updated_at = EXCLUDED\.updated_at`).
		WithArgs(LastCronRunKey, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.SetLastCronRun(context.Background())
	require.NoError(t, err)

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingRepository_GetLastCronRun(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewSQLSettingRepository(db)

	// Test case 1: Last cron run found
	timestamp := time.Now().UTC().Truncate(time.Second)
	timestampStr := timestamp.Format(time.RFC3339)

	rows := sqlmock.NewRows([]string{"key", "value", "created_at", "updated_at"}).
		AddRow(LastCronRunKey, timestampStr, timestamp, timestamp)

	mock.ExpectQuery(`SELECT key, value, created_at, updated_at FROM settings WHERE key = \$1`).
		WithArgs(LastCronRunKey).
		WillReturnRows(rows)

	result, err := repo.GetLastCronRun(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, timestamp.Unix(), result.Unix())

	// Test case 2: No last cron run found
	mock.ExpectQuery(`SELECT key, value, created_at, updated_at FROM settings WHERE key = \$1`).
		WithArgs(LastCronRunKey).
		WillReturnError(sql.ErrNoRows)

	result, err = repo.GetLastCronRun(context.Background())
	require.NoError(t, err)
	assert.Nil(t, result)

	// Test case 3: Database error (not ErrSettingNotFound)
	dbError := errors.New("database connection error")
	mock.ExpectQuery(`SELECT key, value, created_at, updated_at FROM settings WHERE key = \$1`).
		WithArgs(LastCronRunKey).
		WillReturnError(dbError)

	result, err = repo.GetLastCronRun(context.Background())
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, dbError, err)

	// Test case 4: Invalid timestamp format
	invalidTimestamp := "invalid-timestamp"
	rows = sqlmock.NewRows([]string{"key", "value", "created_at", "updated_at"}).
		AddRow(LastCronRunKey, invalidTimestamp, timestamp, timestamp)

	mock.ExpectQuery(`SELECT key, value, created_at, updated_at FROM settings WHERE key = \$1`).
		WithArgs(LastCronRunKey).
		WillReturnRows(rows)

	result, err = repo.GetLastCronRun(context.Background())
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "parsing time")

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingRepository_Delete(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewSQLSettingRepository(db)

	// Test case 1: Setting deleted successfully
	key := "test_key"

	mock.ExpectExec(`DELETE FROM settings WHERE key = \$1`).
		WithArgs(key).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Delete(context.Background(), key)
	require.NoError(t, err)

	// Test case 2: Setting not found
	mock.ExpectExec(`DELETE FROM settings WHERE key = \$1`).
		WithArgs("nonexistent").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.Delete(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.IsType(t, &domain.ErrSettingNotFound{}, err)

	// Test case 3: Database error on exec
	dbError := errors.New("database connection error")
	mock.ExpectExec(`DELETE FROM settings WHERE key = \$1`).
		WithArgs("error_key").
		WillReturnError(dbError)

	err = repo.Delete(context.Background(), "error_key")
	require.Error(t, err)
	assert.Equal(t, dbError, err)

	// Test case 4: Error getting rows affected
	mock.ExpectExec(`DELETE FROM settings WHERE key = \$1`).
		WithArgs("rows_error_key").
		WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected error")))

	err = repo.Delete(context.Background(), "rows_error_key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rows affected error")

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingRepository_List(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewSQLSettingRepository(db)

	timestamp := time.Now().UTC().Truncate(time.Second)

	rows := sqlmock.NewRows([]string{"key", "value", "created_at", "updated_at"}).
		AddRow("key1", "value1", timestamp, timestamp).
		AddRow("key2", "value2", timestamp, timestamp)

	mock.ExpectQuery(`SELECT key, value, created_at, updated_at FROM settings ORDER BY key`).
		WillReturnRows(rows)

	result, err := repo.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "key1", result[0].Key)
	assert.Equal(t, "value1", result[0].Value)
	assert.Equal(t, "key2", result[1].Key)
	assert.Equal(t, "value2", result[1].Value)

	// Test case 2: Database query error
	dbError := errors.New("database query error")
	mock.ExpectQuery(`SELECT key, value, created_at, updated_at FROM settings ORDER BY key`).
		WillReturnError(dbError)

	result, err = repo.List(context.Background())
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, dbError, err)

	// Test case 3: Rows.Err() error
	rowsWithRowsError := sqlmock.NewRows([]string{"key", "value", "created_at", "updated_at"}).
		AddRow("key1", "value1", timestamp, timestamp).
		RowError(0, errors.New("rows iteration error"))

	mock.ExpectQuery(`SELECT key, value, created_at, updated_at FROM settings ORDER BY key`).
		WillReturnRows(rowsWithRowsError)

	result, err = repo.List(context.Background())
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "rows iteration error")

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}
