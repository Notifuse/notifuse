package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/repository/testutil"
)

func TestAuthRepository_GetSessionByID(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	repo := NewSQLAuthRepository(db, mockLogger)

	// Test case 1: Session found
	sessionID := "session-id-1"
	userID := "user-id-1"
	expiresAt := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)

	rows := sqlmock.NewRows([]string{"expires_at"}).
		AddRow(expiresAt)

	mock.ExpectQuery(`SELECT expires_at FROM user_sessions WHERE id = \$1 AND user_id = \$2`).
		WithArgs(sessionID, userID).
		WillReturnRows(rows)

	result, err := repo.GetSessionByID(context.Background(), sessionID, userID)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expiresAt.Unix(), result.Unix())

	// Test case 2: Session not found
	mock.ExpectQuery(`SELECT expires_at FROM user_sessions WHERE id = \$1 AND user_id = \$2`).
		WithArgs("nonexistent", userID).
		WillReturnError(sql.ErrNoRows)

	result, err = repo.GetSessionByID(context.Background(), "nonexistent", userID)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, sql.ErrNoRows, err)
}

func TestAuthRepository_GetUserByID(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	repo := NewSQLAuthRepository(db, mockLogger)

	// Test case 1: User found
	userID := "user-id-1"
	email := "test@example.com"
	createdAt := time.Now().UTC().Truncate(time.Second)

	rows := sqlmock.NewRows([]string{"id", "email", "created_at"}).
		AddRow(userID, email, createdAt)

	mock.ExpectQuery(`SELECT id, email, created_at FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(rows)

	user, err := repo.GetUserByID(context.Background(), userID)
	require.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, userID, user.ID)
	assert.Equal(t, email, user.Email)
	assert.Equal(t, createdAt.Unix(), user.CreatedAt.Unix())

	// Test case 2: User not found
	mock.ExpectQuery(`SELECT id, email, created_at FROM users WHERE id = \$1`).
		WithArgs("nonexistent").
		WillReturnError(sql.ErrNoRows)

	user, err = repo.GetUserByID(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Nil(t, user)
	assert.Equal(t, sql.ErrNoRows, err)

	// Test case 3: Database error
	mock.ExpectQuery(`SELECT id, email, created_at FROM users WHERE id = \$1`).
		WithArgs("error-id").
		WillReturnError(errors.New("database error"))

	user, err = repo.GetUserByID(context.Background(), "error-id")
	require.Error(t, err)
	assert.Nil(t, user)
	assert.Contains(t, err.Error(), "database error")
}
