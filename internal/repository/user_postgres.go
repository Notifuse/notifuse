package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opencensus.io/trace"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/tracing"
)

type userRepository struct {
	systemDB *sql.DB
}

// NewUserRepository creates a new PostgreSQL user repository
func NewUserRepository(db *sql.DB) domain.UserRepository {
	return &userRepository{systemDB: db}
}

func (r *userRepository) CreateUser(ctx context.Context, user *domain.User) error {
	if user.ID == "" {
		user.ID = uuid.New().String()
	}
	if user.Type == "" {
		user.Type = domain.UserTypeUser
	}
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now

	query := `
		INSERT INTO users (id, email, name, type, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.systemDB.ExecContext(ctx, query,
		user.ID,
		user.Email,
		user.Name,
		user.Type,
		user.CreatedAt,
		user.UpdatedAt,
	)
	if err != nil {
		// Check for duplicate key constraint violation (PostgreSQL error code 23505)
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") ||
			strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return &domain.ErrUserExists{Message: "user already exists"}
		}
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

func (r *userRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	query := `
		SELECT id, email, name, type, created_at, updated_at
		FROM users
		WHERE email = $1
	`
	err := r.systemDB.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.Type,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &domain.ErrUserNotFound{Message: "user not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
}

func (r *userRepository) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	ctx, span := tracing.StartServiceSpan(ctx, "UserRepository", "GetUserByID")
	defer span.End()

	span.AddAttributes(trace.StringAttribute("user.id", id))

	var user domain.User
	query := `
		SELECT id, email, name, type, created_at, updated_at
		FROM users
		WHERE id = $1
	`
	startTime := time.Now()
	err := r.systemDB.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.Type,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	queryDuration := time.Since(startTime)

	// Add query duration to span
	span.AddAttributes(trace.StringAttribute("db.query", "SELECT FROM users"),
		trace.Int64Attribute("db.query_duration_ms", queryDuration.Milliseconds()))

	if err == sql.ErrNoRows {
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeNotFound,
			Message: "user not found",
		})
		return nil, &domain.ErrUserNotFound{Message: "user not found"}
	}

	if err != nil {
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeUnknown,
			Message: fmt.Sprintf("failed to get user: %s", err.Error()),
		})
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Add user email to span
	span.AddAttributes(trace.StringAttribute("user.email", user.Email))

	return &user, nil
}

func (r *userRepository) CreateSession(ctx context.Context, session *domain.Session) error {
	if session.ID == "" {
		session.ID = uuid.New().String()
	}
	session.CreatedAt = time.Now().UTC()
	session.ExpiresAt = session.ExpiresAt.UTC()

	// Handle nullable magic code expiration
	var magicCodeExpires interface{}
	if session.MagicCodeExpires != nil {
		expiresUTC := session.MagicCodeExpires.UTC()
		magicCodeExpires = expiresUTC
	}

	query := `
		INSERT INTO user_sessions (
			id, user_id, expires_at, created_at, 
			magic_code, magic_code_expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.systemDB.ExecContext(ctx, query,
		session.ID,
		session.UserID,
		session.ExpiresAt,
		session.CreatedAt,
		session.MagicCode,
		magicCodeExpires,
	)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

func (r *userRepository) GetSessionByID(ctx context.Context, id string) (*domain.Session, error) {
	var session domain.Session
	var magicCode sql.NullString
	var magicCodeExpires sql.NullTime

	query := `
		SELECT id, user_id, expires_at, created_at, 
			magic_code, magic_code_expires_at
		FROM user_sessions
		WHERE id = $1
	`
	err := r.systemDB.QueryRowContext(ctx, query, id).Scan(
		&session.ID,
		&session.UserID,
		&session.ExpiresAt,
		&session.CreatedAt,
		&magicCode,
		&magicCodeExpires,
	)
	if err == sql.ErrNoRows {
		return nil, &domain.ErrSessionNotFound{Message: "session not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Convert nullable types to pointers
	if magicCode.Valid {
		session.MagicCode = &magicCode.String
	}
	if magicCodeExpires.Valid {
		session.MagicCodeExpires = &magicCodeExpires.Time
	}

	return &session, nil
}

func (r *userRepository) DeleteSession(ctx context.Context, id string) error {
	query := `DELETE FROM user_sessions WHERE id = $1`
	result, err := r.systemDB.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return &domain.ErrSessionNotFound{Message: "session not found"}
	}
	return nil
}

func (r *userRepository) DeleteAllSessionsByUserID(ctx context.Context, userID string) error {
	query := `DELETE FROM user_sessions WHERE user_id = $1`
	result, err := r.systemDB.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete sessions: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		// It's ok if no sessions exist - user might already be logged out
		return nil
	}
	return nil
}

func (r *userRepository) GetSessionsByUserID(ctx context.Context, userID string) ([]*domain.Session, error) {
	query := `
		SELECT id, user_id, expires_at, created_at, magic_code, magic_code_expires_at
		FROM user_sessions
		WHERE user_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.systemDB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*domain.Session
	for rows.Next() {
		var session domain.Session
		var magicCode sql.NullString
		var magicCodeExpires sql.NullTime

		err := rows.Scan(
			&session.ID,
			&session.UserID,
			&session.ExpiresAt,
			&session.CreatedAt,
			&magicCode,
			&magicCodeExpires,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		// Convert nullable types to pointers
		if magicCode.Valid {
			session.MagicCode = &magicCode.String
		}
		if magicCodeExpires.Valid {
			session.MagicCodeExpires = &magicCodeExpires.Time
		}

		sessions = append(sessions, &session)
	}
	return sessions, rows.Err()
}

func (r *userRepository) UpdateSession(ctx context.Context, session *domain.Session) error {
	query := `
		UPDATE user_sessions 
		SET expires_at = $1, 
			magic_code = $2, 
			magic_code_expires_at = $3
		WHERE id = $4
	`
	result, err := r.systemDB.ExecContext(
		ctx,
		query,
		session.ExpiresAt,
		session.MagicCode,
		session.MagicCodeExpires,
		session.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return &domain.ErrSessionNotFound{Message: "session not found"}
	}

	return nil
}

// Delete removes a user by their ID
func (r *userRepository) Delete(ctx context.Context, id string) error {
	// First delete all sessions for this user
	deleteSessionsQuery := `DELETE FROM user_sessions WHERE user_id = $1`
	_, err := r.systemDB.ExecContext(ctx, deleteSessionsQuery, id)
	if err != nil {
		return fmt.Errorf("failed to delete user sessions: %w", err)
	}

	// Then delete the user
	deleteUserQuery := `DELETE FROM users WHERE id = $1`
	result, err := r.systemDB.ExecContext(ctx, deleteUserQuery, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return &domain.ErrUserNotFound{Message: "user not found"}
	}

	return nil
}
