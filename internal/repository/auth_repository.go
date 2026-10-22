package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// SQLAuthRepository is a SQL implementation of the AuthRepository interface
type SQLAuthRepository struct {
	systemDB *sql.DB
}

// NewSQLAuthRepository creates a new SQLAuthRepository
func NewSQLAuthRepository(db *sql.DB) *SQLAuthRepository {
	return &SQLAuthRepository{
		systemDB: db,
	}
}

// GetSessionByID retrieves a session by ID and user ID
func (r *SQLAuthRepository) GetSessionByID(ctx context.Context, sessionID string, userID string) (*time.Time, error) {
	var expiresAt time.Time
	err := r.systemDB.QueryRowContext(ctx,
		"SELECT expires_at FROM user_sessions WHERE id = $1 AND user_id = $2",
		sessionID, userID,
	).Scan(&expiresAt)

	if err != nil {
		return nil, err
	}

	return &expiresAt, nil
}

// GetUserByID retrieves a user by ID
func (r *SQLAuthRepository) GetUserByID(ctx context.Context, userID string) (*domain.User, error) {
	var user domain.User
	err := r.systemDB.QueryRowContext(ctx,
		"SELECT id, email, COALESCE(name, ''), type, created_at FROM users WHERE id = $1",
		userID,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Type, &user.CreatedAt)

	if err != nil {
		return nil, err
	}

	return &user, nil
}
