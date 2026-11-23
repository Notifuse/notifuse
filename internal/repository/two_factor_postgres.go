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

// twoFactorRepository implements domain.TwoFactorRepository. It holds no connection: every
// method runs on the executor it is given.
type twoFactorRepository struct{}

// NewTwoFactorRepository returns the domain.TwoFactorRepository of the system database.
func NewTwoFactorRepository() domain.TwoFactorRepository {
	return &twoFactorRepository{}
}

const userTOTPColumns = `user_id, secret_encrypted, enabled_at, last_used_step, pending_secret_encrypted,
	pending_expires_at, pending_attempts, failed_attempts, locked_until, created_at, updated_at`

func (r *twoFactorRepository) ClockTimestamp(ctx context.Context, db domain.DBExecutor) (time.Time, error) {
	var now time.Time
	if err := db.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return time.Time{}, fmt.Errorf("failed to read the database clock: %w", err)
	}
	return now, nil
}

func (r *twoFactorRepository) GetTOTP(ctx context.Context, db domain.DBExecutor, userID string) (*domain.UserTOTP, error) {
	return r.getTOTP(ctx, db, `SELECT `+userTOTPColumns+` FROM user_totp WHERE user_id = $1`, userID)
}

func (r *twoFactorRepository) GetTOTPForUpdate(ctx context.Context, db domain.DBExecutor, userID string) (*domain.UserTOTP, error) {
	return r.getTOTP(ctx, db, `SELECT `+userTOTPColumns+` FROM user_totp WHERE user_id = $1 FOR UPDATE`, userID)
}

func (r *twoFactorRepository) getTOTP(ctx context.Context, db domain.DBExecutor, query, userID string) (*domain.UserTOTP, error) {
	var t domain.UserTOTP
	var secret, pending sql.NullString
	var enabledAt, pendingExpires, lockedUntil sql.NullTime
	err := db.QueryRowContext(ctx, query, userID).Scan(
		&t.UserID, &secret, &enabledAt, &t.LastUsedStep, &pending,
		&pendingExpires, &t.PendingAttempts, &t.FailedAttempts, &lockedUntil, &t.CreatedAt, &t.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user_totp: %w", err)
	}
	t.SecretEncrypted = nullString(secret)
	t.PendingSecretEncrypted = nullString(pending)
	t.EnabledAt = nullTime(enabledAt)
	t.PendingExpiresAt = nullTime(pendingExpires)
	t.LockedUntil = nullTime(lockedUntil)
	return &t, nil
}

func (r *twoFactorRepository) SavePending(ctx context.Context, db domain.DBExecutor, userID, sealedSecret string, expiresAt, now time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO user_totp (user_id, pending_secret_encrypted, pending_expires_at, pending_attempts, created_at, updated_at)
		VALUES ($1, $2, $3, 0, $4, $4)
		ON CONFLICT (user_id) DO UPDATE
		SET pending_secret_encrypted = $2, pending_expires_at = $3, pending_attempts = 0, updated_at = $4`,
		userID, sealedSecret, expiresAt, now)
	if err != nil {
		return fmt.Errorf("failed to save the pending secret: %w", err)
	}
	return nil
}

func (r *twoFactorRepository) RecordPendingFailure(ctx context.Context, db domain.DBExecutor, userID string, maxAttempts int, now time.Time) (bool, error) {
	var attempts int
	err := db.QueryRowContext(ctx, `
		UPDATE user_totp
		SET pending_attempts = pending_attempts + 1, updated_at = $2
		WHERE user_id = $1
		RETURNING pending_attempts`, userID, now).Scan(&attempts)
	if errors.Is(err, sql.ErrNoRows) {
		// No row: two-factor authentication was turned off or reset meanwhile, so nothing
		// is pending any more.
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to count a setup failure: %w", err)
	}
	if attempts < maxAttempts {
		return false, nil
	}
	_, err = db.ExecContext(ctx, `
		UPDATE user_totp
		SET pending_secret_encrypted = NULL, pending_expires_at = NULL, pending_attempts = 0, updated_at = $2
		WHERE user_id = $1`, userID, now)
	if err != nil {
		return false, fmt.Errorf("failed to drop the pending secret: %w", err)
	}
	return true, nil
}

func (r *twoFactorRepository) Enable(ctx context.Context, db domain.DBExecutor, userID, sealedSecret string, step int64, now time.Time) error {
	result, err := db.ExecContext(ctx, `
		UPDATE user_totp
		SET secret_encrypted = $2, enabled_at = COALESCE(enabled_at, $4), last_used_step = $3,
			pending_secret_encrypted = NULL, pending_expires_at = NULL, pending_attempts = 0,
			failed_attempts = 0, locked_until = NULL, updated_at = $4
		WHERE user_id = $1`, userID, sealedSecret, step, now)
	if err != nil {
		return fmt.Errorf("failed to enable two-factor authentication: %w", err)
	}
	return expectOneRow(result, "user_totp")
}

func (r *twoFactorRepository) RecordFailure(ctx context.Context, db domain.DBExecutor, userID string, failedAttempts int, lockedUntil *time.Time, now time.Time) error {
	result, err := db.ExecContext(ctx, `
		UPDATE user_totp SET failed_attempts = $2, locked_until = $3, updated_at = $4
		WHERE user_id = $1`, userID, failedAttempts, lockedUntil, now)
	if err != nil {
		return fmt.Errorf("failed to record a two-factor failure: %w", err)
	}
	return expectOneRow(result, "user_totp")
}

func (r *twoFactorRepository) RecordSuccess(ctx context.Context, db domain.DBExecutor, userID string, step *int64, now time.Time) error {
	result, err := db.ExecContext(ctx, `
		UPDATE user_totp
		SET failed_attempts = 0, locked_until = NULL, last_used_step = COALESCE($2, last_used_step), updated_at = $3
		WHERE user_id = $1`, userID, step, now)
	if err != nil {
		return fmt.Errorf("failed to record a two-factor success: %w", err)
	}
	return expectOneRow(result, "user_totp")
}

func (r *twoFactorRepository) DeleteTOTP(ctx context.Context, db domain.DBExecutor, userID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM user_totp WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("failed to delete user_totp: %w", err)
	}
	return nil
}

func (r *twoFactorRepository) ReplaceRecoveryCodes(ctx context.Context, db domain.DBExecutor, userID string, hashes []string, now time.Time) error {
	if err := r.DeleteRecoveryCodes(ctx, db, userID); err != nil {
		return err
	}
	for _, hash := range hashes {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO user_recovery_codes (id, user_id, code_hash, created_at) VALUES ($1, $2, $3, $4)`,
			uuid.New().String(), userID, hash, now); err != nil {
			return fmt.Errorf("failed to store a recovery code: %w", err)
		}
	}
	return nil
}

func (r *twoFactorRepository) DeleteRecoveryCodes(ctx context.Context, db domain.DBExecutor, userID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM user_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("failed to delete recovery codes: %w", err)
	}
	return nil
}

func (r *twoFactorRepository) ConsumeRecoveryCode(ctx context.Context, db domain.DBExecutor, userID, hash string, now time.Time) (bool, error) {
	result, err := db.ExecContext(ctx, `
		UPDATE user_recovery_codes SET used_at = $3
		WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL`, userID, hash, now)
	if err != nil {
		return false, fmt.Errorf("failed to use a recovery code: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read affected rows: %w", err)
	}
	return rows == 1, nil
}

func (r *twoFactorRepository) CountRemainingRecoveryCodes(ctx context.Context, db domain.DBExecutor, userID string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_recovery_codes WHERE user_id = $1 AND used_at IS NULL`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count recovery codes: %w", err)
	}
	return count, nil
}

func (r *twoFactorRepository) ListUsersWithoutTwoFactor(ctx context.Context, db domain.DBExecutor) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT u.email FROM users u
		LEFT JOIN user_totp t ON t.user_id = u.id
		WHERE u.type = 'user' AND (t.enabled_at IS NULL OR t.secret_encrypted IS NULL)
		ORDER BY u.email`)
	if err != nil {
		return nil, fmt.Errorf("failed to list users without two-factor authentication: %w", err)
	}
	defer func() { _ = rows.Close() }()
	emails := []string{}
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("failed to scan an email: %w", err)
		}
		emails = append(emails, email)
	}
	return emails, rows.Err()
}

func expectOneRow(result sql.Result, table string) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read affected rows: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("expected one %s row, updated %d", table, rows)
	}
	return nil
}

func nullString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

func nullTime(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}
