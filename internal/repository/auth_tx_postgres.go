package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/internal/domain"
)

// authTxRepository implements domain.AuthTxRepository. It holds no connection: every method
// runs on the executor it is given.
//
// The older user_sessions columns (expires_at, created_at, magic_code_expires_at) are
// TIMESTAMP written in UTC, so they are compared with now() AT TIME ZONE 'UTC'; a bare now()
// would read them in the connection's time zone. The 45.0 columns are TIMESTAMPTZ.
type authTxRepository struct{}

// NewAuthTxRepository returns the domain.AuthTxRepository of the system database.
func NewAuthTxRepository() domain.AuthTxRepository {
	return &authTxRepository{}
}

const sessionColumns = `id, user_id, expires_at, created_at, magic_code, magic_code_expires_at,
	auth_method, second_factor, second_factor_at, authenticated_at,
	challenge_hash, challenge_expires_at, challenge_attempts, pending_invitation_id`

// scanSession reads a row of sessionColumns.
func scanSession(row interface{ Scan(...any) error }) (*domain.Session, error) {
	var s domain.Session
	var magicCode, authMethod, secondFactor, challengeHash, pendingInvitation sql.NullString
	var magicExpires, secondFactorAt, authenticatedAt, challengeExpires sql.NullTime
	err := row.Scan(
		&s.ID, &s.UserID, &s.ExpiresAt, &s.CreatedAt, &magicCode, &magicExpires,
		&authMethod, &secondFactor, &secondFactorAt, &authenticatedAt,
		&challengeHash, &challengeExpires, &s.ChallengeAttempts, &pendingInvitation,
	)
	if err != nil {
		return nil, err
	}
	s.MagicCode = nullString(magicCode)
	s.MagicCodeExpires = nullTime(magicExpires)
	s.AuthMethod = domain.AuthMethod(authMethod.String)
	s.SecondFactor = domain.SecondFactor(secondFactor.String)
	s.SecondFactorAt = nullTime(secondFactorAt)
	s.AuthenticatedAt = nullTime(authenticatedAt)
	s.ChallengeHash = nullString(challengeHash)
	s.ChallengeExpiresAt = nullTime(challengeExpires)
	s.PendingInvitationID = nullString(pendingInvitation)
	return &s, nil
}

func (r *authTxRepository) LockUser(ctx context.Context, db domain.DBExecutor, userID string) error {
	var id string
	err := db.QueryRowContext(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return &domain.ErrUserNotFound{Message: "user not found"}
	}
	if err != nil {
		return fmt.Errorf("failed to lock the user: %w", err)
	}
	return nil
}

func (r *authTxRepository) CreateUserIfAbsent(ctx context.Context, db domain.DBExecutor, user *domain.User) (bool, error) {
	if user.ID == "" {
		user.ID = uuid.New().String()
	}
	if user.Type == "" {
		user.Type = domain.UserTypeUser
	}
	if !domain.IsSupportedUILanguage(user.Language) {
		user.Language = domain.DefaultLanguageCode
	}
	now := time.Now().UTC()
	user.CreatedAt, user.UpdatedAt = now, now
	result, err := db.ExecContext(ctx, `
		INSERT INTO users (id, email, name, type, language, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (email) DO NOTHING`,
		user.ID, user.Email, user.Name, user.Type, user.Language, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return false, fmt.Errorf("failed to create the user: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read affected rows: %w", err)
	}
	return rows == 1, nil
}

// CreateSession inserts a session, with its magic code when it waits for one. A challenge is
// set on an existing session by SetSessionChallenge, never at creation: a session carrying
// one here is refused rather than stored without it, which would make it usable.
func (r *authTxRepository) CreateSession(ctx context.Context, db domain.DBExecutor, session *domain.Session) error {
	if session.ChallengeHash != nil || session.PendingInvitationID != nil {
		return fmt.Errorf("failed to create session: a challenge is set with SetSessionChallenge")
	}
	if session.ID == "" {
		session.ID = uuid.New().String()
	}
	session.CreatedAt = time.Now().UTC()
	session.ExpiresAt = session.ExpiresAt.UTC()
	var authMethod, magicCodeExpires any
	if session.AuthMethod != "" {
		authMethod = string(session.AuthMethod)
	}
	if session.MagicCodeExpires != nil {
		magicCodeExpires = session.MagicCodeExpires.UTC()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO user_sessions (id, user_id, expires_at, created_at, magic_code, magic_code_expires_at, auth_method, authenticated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		session.ID, session.UserID, session.ExpiresAt, session.CreatedAt, session.MagicCode, magicCodeExpires,
		authMethod, session.AuthenticatedAt)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

func (r *authTxRepository) ConsumeMagicCode(ctx context.Context, db domain.DBExecutor, userID, codeHash string) (*domain.Session, error) {
	// One session at most, the newest: two sign-ins can draw the same 6-digit code. If two
	// such codes are submitted at the same moment, the one that loses the row lock gets
	// ErrSessionNotFound even though an older session holds the same code: a one-in-a-million
	// coincidence that costs one more sign-in, accepted.
	row := db.QueryRowContext(ctx, `
		UPDATE user_sessions SET magic_code = NULL, magic_code_expires_at = NULL
		WHERE id = (
			SELECT id FROM user_sessions
			WHERE user_id = $1 AND magic_code = $2 AND magic_code_expires_at > (now() AT TIME ZONE 'UTC')
			ORDER BY created_at DESC
			LIMIT 1
			FOR UPDATE
		)
		RETURNING `+sessionColumns, userID, codeHash)
	session, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &domain.ErrSessionNotFound{Message: "no session holds this code"}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to consume the magic code: %w", err)
	}
	return session, nil
}

func (r *authTxRepository) HasExpiredMagicCode(ctx context.Context, db domain.DBExecutor, userID, codeHash string) (bool, error) {
	var expired bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM user_sessions
			WHERE user_id = $1 AND magic_code = $2 AND magic_code_expires_at <= (now() AT TIME ZONE 'UTC'))`,
		userID, codeHash).Scan(&expired)
	if err != nil {
		return false, fmt.Errorf("failed to check for an expired magic code: %w", err)
	}
	return expired, nil
}

func (r *authTxRepository) PurgeExpiredPendingSessions(ctx context.Context, db domain.DBExecutor, userID string) error {
	_, err := db.ExecContext(ctx, `
		DELETE FROM user_sessions
		WHERE user_id = $1 AND (
			(challenge_hash IS NOT NULL AND challenge_expires_at < now())
			OR (magic_code IS NOT NULL AND magic_code_expires_at < (now() AT TIME ZONE 'UTC'))
		)`, userID)
	if err != nil {
		return fmt.Errorf("failed to purge expired pending sessions: %w", err)
	}
	return nil
}

func (r *authTxRepository) ActivateSession(ctx context.Context, db domain.DBExecutor, sessionID string, method domain.AuthMethod, now time.Time) error {
	result, err := db.ExecContext(ctx, `
		UPDATE user_sessions SET auth_method = $2, authenticated_at = $3 WHERE id = $1`,
		sessionID, string(method), now)
	if err != nil {
		return fmt.Errorf("failed to activate the session: %w", err)
	}
	return expectSession(result)
}

func (r *authTxRepository) SetSessionChallenge(ctx context.Context, db domain.DBExecutor, sessionID string, method domain.AuthMethod, challengeHash string, expiresAt time.Time, pendingInvitationID *string) error {
	result, err := db.ExecContext(ctx, `
		UPDATE user_sessions
		SET auth_method = $2, challenge_hash = $3, challenge_expires_at = $4, challenge_attempts = 0, pending_invitation_id = $5
		WHERE id = $1`,
		sessionID, string(method), challengeHash, expiresAt, pendingInvitationID)
	if err != nil {
		return fmt.Errorf("failed to set the sign-in challenge: %w", err)
	}
	return expectSession(result)
}

func (r *authTxRepository) FindChallengeUser(ctx context.Context, db domain.DBExecutor, challengeHash string) (string, error) {
	var userID string
	err := db.QueryRowContext(ctx, `SELECT user_id FROM user_sessions WHERE challenge_hash = $1`,
		challengeHash).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrTwoFactorChallengeInvalid
	}
	if err != nil {
		return "", fmt.Errorf("failed to find the challenge: %w", err)
	}
	return userID, nil
}

func (r *authTxRepository) GetChallengeSessionForUpdate(ctx context.Context, db domain.DBExecutor, userID, challengeHash string) (*domain.Session, error) {
	session, err := scanSession(db.QueryRowContext(ctx, `
		SELECT `+sessionColumns+` FROM user_sessions
		WHERE user_id = $1 AND challenge_hash = $2
		FOR UPDATE`, userID, challengeHash))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrTwoFactorChallengeInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("failed to lock the challenge: %w", err)
	}
	return session, nil
}

func (r *authTxRepository) FailChallenge(ctx context.Context, db domain.DBExecutor, sessionID string) error {
	result, err := db.ExecContext(ctx, `UPDATE user_sessions SET challenge_attempts = challenge_attempts + 1 WHERE id = $1`, sessionID)
	if err != nil {
		return fmt.Errorf("failed to count a challenge attempt: %w", err)
	}
	return expectSession(result)
}

func (r *authTxRepository) CompleteChallenge(ctx context.Context, db domain.DBExecutor, sessionID string, factor domain.SecondFactor, now time.Time) error {
	result, err := db.ExecContext(ctx, `
		UPDATE user_sessions
		SET challenge_hash = NULL, challenge_expires_at = NULL, challenge_attempts = 0, pending_invitation_id = NULL,
			second_factor = $2, second_factor_at = $3, authenticated_at = $3
		WHERE id = $1`, sessionID, string(factor), now)
	if err != nil {
		return fmt.Errorf("failed to complete the challenge: %w", err)
	}
	return expectSession(result)
}

func (r *authTxRepository) GetSessionForUpdate(ctx context.Context, db domain.DBExecutor, userID, sessionID string) (*domain.Session, error) {
	session, err := scanSession(db.QueryRowContext(ctx, `
		SELECT `+sessionColumns+` FROM user_sessions
		WHERE id = $1 AND user_id = $2 AND challenge_hash IS NULL AND magic_code IS NULL
		FOR UPDATE`, sessionID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &domain.ErrSessionNotFound{Message: "session not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to lock the session: %w", err)
	}
	return session, nil
}

func (r *authTxRepository) SetSessionSecondFactor(ctx context.Context, db domain.DBExecutor, userID, sessionID string, factor domain.SecondFactor, now time.Time) error {
	result, err := db.ExecContext(ctx, `
		UPDATE user_sessions SET second_factor = $3, second_factor_at = $4
		WHERE id = $1 AND user_id = $2 AND challenge_hash IS NULL AND magic_code IS NULL`, sessionID, userID, string(factor), now)
	if err != nil {
		return fmt.Errorf("failed to record the session's second factor: %w", err)
	}
	return expectSession(result)
}

func (r *authTxRepository) ClearSecondFactorOnSessions(ctx context.Context, db domain.DBExecutor, userID string) error {
	if _, err := db.ExecContext(ctx, `
		UPDATE user_sessions SET second_factor = NULL, second_factor_at = NULL
		WHERE user_id = $1 AND second_factor IS NOT NULL`, userID); err != nil {
		return fmt.Errorf("failed to clear the sessions' second factor: %w", err)
	}
	return nil
}

func (r *authTxRepository) DeleteSessionsExcept(ctx context.Context, db domain.DBExecutor, userID, keep string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM user_sessions WHERE user_id = $1 AND id <> $2`, userID, keep); err != nil {
		return fmt.Errorf("failed to delete the other sessions: %w", err)
	}
	return nil
}

func (r *authTxRepository) DeleteAllSessions(ctx context.Context, db domain.DBExecutor, userID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM user_sessions WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("failed to delete the sessions: %w", err)
	}
	return nil
}

// expectSession turns an update that found no session into ErrSessionNotFound.
func expectSession(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read affected rows: %w", err)
	}
	if rows == 0 {
		return &domain.ErrSessionNotFound{Message: "session not found"}
	}
	return nil
}
