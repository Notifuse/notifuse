package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestWithSystemTx_CommitsOnNil(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE x").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = NewSystemTxRunner(db).WithSystemTx(context.Background(), func(tx domain.DBExecutor) error {
		_, err := tx.ExecContext(context.Background(), "UPDATE x")
		return err
	})
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithSystemTx_RollsBackOnAnError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	sentinel := errors.New("boom")
	mock.ExpectBegin()
	mock.ExpectRollback()

	err = NewSystemTxRunner(db).WithSystemTx(context.Background(), func(tx domain.DBExecutor) error { return sentinel })
	assert.Equal(t, sentinel, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A wrong code answers an error, and the failure counters it incremented must persist.
func TestWithSystemTx_CommitsAndReturnsTheUnwrappedBusinessError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE user_totp").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = NewSystemTxRunner(db).WithSystemTx(context.Background(), func(tx domain.DBExecutor) error {
		if _, err := tx.ExecContext(context.Background(), "UPDATE user_totp"); err != nil {
			return err
		}
		return domain.CommitAndFail(domain.ErrTwoFactorInvalidCode)
	})
	assert.Equal(t, domain.ErrTwoFactorInvalidCode, err, "the marker never leaves the runner")
	_, marked := domain.AsCommitAndFail(err)
	assert.False(t, marked)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// When the commit of a CommitAndFail fails, the counters did not persist: the caller must
// not believe they did, nor send the alert it would send after a counted failure.
func TestWithSystemTx_ReportsAFailedCommitInPlaceOfTheBusinessError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	commitErr := errors.New("connection lost")
	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(commitErr)

	err = NewSystemTxRunner(db).WithSystemTx(context.Background(), func(tx domain.DBExecutor) error {
		return domain.CommitAndFail(domain.ErrTwoFactorInvalidCode)
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, commitErr)
	assert.NotErrorIs(t, err, domain.ErrTwoFactorInvalidCode)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithSystemTx_ReportsAFailedCommit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	commitErr := errors.New("serialization failure")
	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(commitErr)

	err = NewSystemTxRunner(db).WithSystemTx(context.Background(), func(tx domain.DBExecutor) error { return nil })
	assert.ErrorIs(t, err, commitErr)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithSystemTx_BeginFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin().WillReturnError(errors.New("no connection"))
	called := false
	err = NewSystemTxRunner(db).WithSystemTx(context.Background(), func(tx domain.DBExecutor) error {
		called = true
		return nil
	})
	assert.Error(t, err)
	assert.False(t, called)
	assert.NoError(t, mock.ExpectationsWereMet())
}
