package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository/testutil"
)

func TestCreateUser(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewUserRepository(db)

	// Test case 1: Successful user creation
	user := &domain.User{
		ID:    uuid.New().String(),
		Email: "test@example.com",
		Name:  "Test User",
		Type:  domain.UserTypeUser,
	}

	mock.ExpectExec(`INSERT INTO users \(id, email, name, type, created_at, updated_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6\)`).
		WithArgs(user.ID, user.Email, user.Name, user.Type, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.CreateUser(context.Background(), user)
	require.NoError(t, err)

	// Test case 2: Error during user creation
	userWithError := &domain.User{
		ID:    uuid.New().String(),
		Email: "error@example.com",
		Name:  "Error User",
		Type:  domain.UserTypeUser,
	}

	mock.ExpectExec(`INSERT INTO users \(id, email, name, type, created_at, updated_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6\)`).
		WithArgs(userWithError.ID, userWithError.Email, userWithError.Name, userWithError.Type, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("database error"))

	err = repo.CreateUser(context.Background(), userWithError)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create user")

	// Test case 3: Duplicate key constraint violation
	duplicateUser := &domain.User{
		ID:    uuid.New().String(),
		Email: "duplicate@example.com",
		Name:  "Duplicate User",
		Type:  domain.UserTypeUser,
	}

	mock.ExpectExec(`INSERT INTO users \(id, email, name, type, created_at, updated_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6\)`).
		WithArgs(duplicateUser.ID, duplicateUser.Email, duplicateUser.Name, duplicateUser.Type, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("pq: duplicate key value violates unique constraint \"users_email_key\""))

	err = repo.CreateUser(context.Background(), duplicateUser)
	require.Error(t, err)
	assert.IsType(t, &domain.ErrUserExists{}, err)
	assert.Equal(t, "user already exists", err.Error())
}

func TestGetUserByEmail(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewUserRepository(db)

	// Test case 1: User found
	email := "test@example.com"
	expectedUser := &domain.User{
		ID:        "user-id-1",
		Email:     email,
		Name:      "Test User",
		Type:      domain.UserTypeUser,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		UpdatedAt: time.Now().UTC().Truncate(time.Second),
	}

	rows := sqlmock.NewRows([]string{"id", "email", "name", "type", "created_at", "updated_at"}).
		AddRow(expectedUser.ID, expectedUser.Email, expectedUser.Name, expectedUser.Type, expectedUser.CreatedAt, expectedUser.UpdatedAt)

	mock.ExpectQuery(`SELECT id, email, name, type, created_at, updated_at FROM users WHERE email = \$1`).
		WithArgs(email).
		WillReturnRows(rows)

	user, err := repo.GetUserByEmail(context.Background(), email)
	require.NoError(t, err)
	assert.Equal(t, expectedUser.ID, user.ID)
	assert.Equal(t, expectedUser.Email, user.Email)
	assert.Equal(t, expectedUser.Name, user.Name)
	assert.Equal(t, expectedUser.Type, user.Type)

	// Test case 2: User not found
	mock.ExpectQuery(`SELECT id, email, name, type, created_at, updated_at FROM users WHERE email = \$1`).
		WithArgs("nonexistent@example.com").
		WillReturnError(sql.ErrNoRows)

	user, err = repo.GetUserByEmail(context.Background(), "nonexistent@example.com")
	require.Error(t, err)
	assert.Nil(t, user)
	assert.IsType(t, &domain.ErrUserNotFound{}, err)

	// Test case 3: Database error
	mock.ExpectQuery(`SELECT id, email, name, type, created_at, updated_at FROM users WHERE email = \$1`).
		WithArgs("error@example.com").
		WillReturnError(errors.New("database error"))

	user, err = repo.GetUserByEmail(context.Background(), "error@example.com")
	require.Error(t, err)
	assert.Nil(t, user)
	assert.Contains(t, err.Error(), "failed to get user")
}

func TestGetUserByID(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewUserRepository(db)

	// Test case 1: User found
	userID := "user-id-1"
	expectedUser := &domain.User{
		ID:        userID,
		Email:     "test@example.com",
		Name:      "Test User",
		Type:      domain.UserTypeUser,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		UpdatedAt: time.Now().UTC().Truncate(time.Second),
	}

	rows := sqlmock.NewRows([]string{"id", "email", "name", "type", "created_at", "updated_at"}).
		AddRow(expectedUser.ID, expectedUser.Email, expectedUser.Name, expectedUser.Type, expectedUser.CreatedAt, expectedUser.UpdatedAt)

	mock.ExpectQuery(`SELECT id, email, name, type, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnRows(rows)

	user, err := repo.GetUserByID(context.Background(), userID)
	require.NoError(t, err)
	assert.Equal(t, expectedUser.ID, user.ID)
	assert.Equal(t, expectedUser.Email, user.Email)
	assert.Equal(t, expectedUser.Name, user.Name)
	assert.Equal(t, expectedUser.Type, user.Type)

	// Test case 2: User not found
	mock.ExpectQuery(`SELECT id, email, name, type, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs("nonexistent-id").
		WillReturnError(sql.ErrNoRows)

	user, err = repo.GetUserByID(context.Background(), "nonexistent-id")
	require.Error(t, err)
	assert.Nil(t, user)
	assert.IsType(t, &domain.ErrUserNotFound{}, err)
}

func TestCreateSession(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewUserRepository(db)

	userID := "user-id-1"
	sessionID := uuid.New().String()
	expiresAt := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	magicCode := "123456"
	magicCodeExpires := time.Now().Add(15 * time.Minute).UTC().Truncate(time.Second)

	session := &domain.Session{
		ID:               sessionID,
		UserID:           userID,
		ExpiresAt:        expiresAt,
		MagicCode:        magicCode,
		MagicCodeExpires: magicCodeExpires,
	}

	// Use a more permissive regex pattern that allows for whitespace variations
	mock.ExpectExec(`INSERT INTO user_sessions.*VALUES.*\$1.*\$2.*\$3.*\$4.*\$5.*\$6`).
		WithArgs(sessionID, userID, expiresAt, sqlmock.AnyArg(), magicCode, magicCodeExpires).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.CreateSession(context.Background(), session)
	require.NoError(t, err)
}

func TestGetSessionByID(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewUserRepository(db)

	// Test case 1: Session found
	sessionID := "session-id-1"
	userID := "user-id-1"
	createdAt := time.Now().UTC().Truncate(time.Second)
	expiresAt := createdAt.Add(24 * time.Hour)
	magicCode := "123456"
	magicCodeExpires := createdAt.Add(15 * time.Minute)

	rows := sqlmock.NewRows([]string{"id", "user_id", "expires_at", "created_at", "magic_code", "magic_code_expires_at"}).
		AddRow(sessionID, userID, expiresAt, createdAt, magicCode, magicCodeExpires)

	mock.ExpectQuery(`SELECT id, user_id, expires_at, created_at, magic_code, magic_code_expires_at FROM user_sessions WHERE id = \$1`).
		WithArgs(sessionID).
		WillReturnRows(rows)

	session, err := repo.GetSessionByID(context.Background(), sessionID)
	require.NoError(t, err)
	assert.Equal(t, sessionID, session.ID)
	assert.Equal(t, userID, session.UserID)
	assert.Equal(t, expiresAt.Unix(), session.ExpiresAt.Unix())
	assert.Equal(t, createdAt.Unix(), session.CreatedAt.Unix())
	assert.Equal(t, magicCode, session.MagicCode)
	assert.Equal(t, magicCodeExpires.Unix(), session.MagicCodeExpires.Unix())

	// Test case 2: Session not found
	mock.ExpectQuery(`SELECT id, user_id, expires_at, created_at, magic_code, magic_code_expires_at FROM user_sessions WHERE id = \$1`).
		WithArgs("nonexistent-id").
		WillReturnError(sql.ErrNoRows)

	session, err = repo.GetSessionByID(context.Background(), "nonexistent-id")
	require.Error(t, err)
	assert.Nil(t, session)
	assert.IsType(t, &domain.ErrSessionNotFound{}, err)
}

func TestDeleteSession(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewUserRepository(db)

	// Test case 1: Session deleted successfully
	sessionID := "session-id-1"

	mock.ExpectExec(`DELETE FROM user_sessions WHERE id = \$1`).
		WithArgs(sessionID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.DeleteSession(context.Background(), sessionID)
	require.NoError(t, err)

	// Test case 2: Session not found
	mock.ExpectExec(`DELETE FROM user_sessions WHERE id = \$1`).
		WithArgs("nonexistent-id").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.DeleteSession(context.Background(), "nonexistent-id")
	require.Error(t, err)
	assert.IsType(t, &domain.ErrSessionNotFound{}, err)
}

func TestGetSessionsByUserID(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewUserRepository(db)

	userID := "user-id-1"
	now := time.Now().UTC().Truncate(time.Second)

	// Create two sessions for the same user
	session1 := &domain.Session{
		ID:               "session-id-1",
		UserID:           userID,
		ExpiresAt:        now.Add(24 * time.Hour),
		CreatedAt:        now,
		MagicCode:        "123456",
		MagicCodeExpires: now.Add(15 * time.Minute),
	}

	session2 := &domain.Session{
		ID:               "session-id-2",
		UserID:           userID,
		ExpiresAt:        now.Add(48 * time.Hour),
		CreatedAt:        now.Add(1 * time.Hour),
		MagicCode:        "654321",
		MagicCodeExpires: now.Add(16 * time.Minute),
	}

	rows := sqlmock.NewRows([]string{"id", "user_id", "expires_at", "created_at", "magic_code", "magic_code_expires_at"}).
		AddRow(session1.ID, session1.UserID, session1.ExpiresAt, session1.CreatedAt, session1.MagicCode, session1.MagicCodeExpires).
		AddRow(session2.ID, session2.UserID, session2.ExpiresAt, session2.CreatedAt, session2.MagicCode, session2.MagicCodeExpires)

	mock.ExpectQuery(`SELECT id, user_id, expires_at, created_at, magic_code, magic_code_expires_at FROM user_sessions WHERE user_id = \$1 ORDER BY created_at DESC`).
		WithArgs(userID).
		WillReturnRows(rows)

	sessions, err := repo.GetSessionsByUserID(context.Background(), userID)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
	assert.Equal(t, session1.ID, sessions[0].ID)
	assert.Equal(t, session2.ID, sessions[1].ID)
}

func TestUpdateSession(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewUserRepository(db)

	// Test case 1: Session updated successfully
	sessionID := "session-id-1"
	expiresAt := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	magicCode := "updated-code"
	magicCodeExpires := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)

	session := &domain.Session{
		ID:               sessionID,
		ExpiresAt:        expiresAt,
		MagicCode:        magicCode,
		MagicCodeExpires: magicCodeExpires,
	}

	mock.ExpectExec(`UPDATE user_sessions SET expires_at = \$1, magic_code = \$2, magic_code_expires_at = \$3 WHERE id = \$4`).
		WithArgs(expiresAt, magicCode, magicCodeExpires, sessionID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.UpdateSession(context.Background(), session)
	require.NoError(t, err)

	// Test case 2: Session not found
	mock.ExpectExec(`UPDATE user_sessions SET expires_at = \$1, magic_code = \$2, magic_code_expires_at = \$3 WHERE id = \$4`).
		WithArgs(expiresAt, magicCode, magicCodeExpires, "nonexistent-id").
		WillReturnResult(sqlmock.NewResult(0, 0))

	session.ID = "nonexistent-id"
	err = repo.UpdateSession(context.Background(), session)
	require.Error(t, err)
	assert.IsType(t, &domain.ErrSessionNotFound{}, err)
}

func TestDeleteAllSessionsByUserID(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewUserRepository(db)

	// Test case 1: Successfully delete multiple sessions
	userID := "user-id-1"

	mock.ExpectExec(`DELETE FROM user_sessions WHERE user_id = \$1`).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 3)) // 3 sessions deleted

	err := repo.DeleteAllSessionsByUserID(context.Background(), userID)
	require.NoError(t, err)

	// Test case 2: Successfully delete one session
	mock.ExpectExec(`DELETE FROM user_sessions WHERE user_id = \$1`).
		WithArgs("user-id-2").
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 session deleted

	err = repo.DeleteAllSessionsByUserID(context.Background(), "user-id-2")
	require.NoError(t, err)

	// Test case 3: No sessions to delete (user already logged out or never logged in)
	mock.ExpectExec(`DELETE FROM user_sessions WHERE user_id = \$1`).
		WithArgs("user-id-no-sessions").
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 sessions deleted

	err = repo.DeleteAllSessionsByUserID(context.Background(), "user-id-no-sessions")
	require.NoError(t, err) // Should not return error when no sessions exist

	// Test case 4: Database error during deletion
	mock.ExpectExec(`DELETE FROM user_sessions WHERE user_id = \$1`).
		WithArgs("user-id-error").
		WillReturnError(errors.New("database connection error"))

	err = repo.DeleteAllSessionsByUserID(context.Background(), "user-id-error")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete sessions")

	// Test case 5: Error getting rows affected
	mock.ExpectExec(`DELETE FROM user_sessions WHERE user_id = \$1`).
		WithArgs("user-id-rows-error").
		WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected error")))

	err = repo.DeleteAllSessionsByUserID(context.Background(), "user-id-rows-error")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get rows affected")
}

func TestDeleteUser(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewUserRepository(db)
	userID := "user-id-to-delete"

	// Test case 1: User deleted successfully
	// First, expect deletion of user's sessions
	mock.ExpectExec(`DELETE FROM user_sessions WHERE user_id = \$1`).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 2)) // Assuming 2 sessions were deleted

	// Then, expect deletion of the user
	mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 user deleted

	err := repo.Delete(context.Background(), userID)
	require.NoError(t, err)

	// Test case 2: User not found
	mock.ExpectExec(`DELETE FROM user_sessions WHERE user_id = \$1`).
		WithArgs("nonexistent-id").
		WillReturnResult(sqlmock.NewResult(0, 0)) // No sessions deleted

	mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).
		WithArgs("nonexistent-id").
		WillReturnResult(sqlmock.NewResult(0, 0)) // No users deleted

	err = repo.Delete(context.Background(), "nonexistent-id")
	require.Error(t, err)
	assert.IsType(t, &domain.ErrUserNotFound{}, err)

	// Test case 3: Error deleting sessions
	mock.ExpectExec(`DELETE FROM user_sessions WHERE user_id = \$1`).
		WithArgs("error-id").
		WillReturnError(errors.New("database error"))

	err = repo.Delete(context.Background(), "error-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete user sessions")

	// Test case 4: Error deleting user
	mock.ExpectExec(`DELETE FROM user_sessions WHERE user_id = \$1`).
		WithArgs("user-error-id").
		WillReturnResult(sqlmock.NewResult(0, 1)) // Sessions deleted successfully

	mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).
		WithArgs("user-error-id").
		WillReturnError(errors.New("database error"))

	err = repo.Delete(context.Background(), "user-error-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete user")
}
