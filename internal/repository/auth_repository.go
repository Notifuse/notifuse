package repository

import (
	"context"
	"database/sql"

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

// GetSessionByID returns what a request needs to know about a usable session of the user:
// its expiry, how it was opened, the second factor it passed and when it became usable. A
// session that waits for its magic code or its second factor is not usable and comes back
// as sql.ErrNoRows, like a missing one.
func (r *SQLAuthRepository) GetSessionByID(ctx context.Context, sessionID string, userID string) (*domain.SessionAuth, error) {
	var auth domain.SessionAuth
	var authMethod, secondFactor sql.NullString
	var authenticatedAt sql.NullTime
	err := r.systemDB.QueryRowContext(ctx, `
		SELECT expires_at, auth_method, second_factor, authenticated_at
		FROM user_sessions
		WHERE id = $1 AND user_id = $2 AND challenge_hash IS NULL AND magic_code IS NULL`,
		sessionID, userID,
	).Scan(&auth.ExpiresAt, &authMethod, &secondFactor, &authenticatedAt)
	if err != nil {
		return nil, err
	}
	auth.AuthMethod = domain.AuthMethod(authMethod.String)
	auth.SecondFactor = domain.SecondFactor(secondFactor.String)
	auth.AuthenticatedAt = nullTime(authenticatedAt)
	return &auth, nil
}

// IsTwoFactorEnabled reports whether the user turned on two-factor authentication.
func (r *SQLAuthRepository) IsTwoFactorEnabled(ctx context.Context, userID string) (bool, error) {
	var enabled bool
	err := r.systemDB.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM user_totp WHERE user_id = $1 AND enabled_at IS NOT NULL AND secret_encrypted IS NOT NULL)`, userID,
	).Scan(&enabled)
	if err != nil {
		return false, err
	}
	return enabled, nil
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
